package health_test

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"

	healthinfra "github.com/nexumpr/Nexum-Backend/internal/infrastructure/health"
)

// leerComando interpreta un comando RESP (array de bulk strings) desde el stream.
// Es necesario porque go-redis puede enviar un comando en varias lecturas TCP.
func leerComando(lector *bufio.Reader) ([]string, error) {
	cabecera, err := lector.ReadString('\n')
	if err != nil {
		return nil, err
	}
	if !strings.HasPrefix(cabecera, "*") {
		return nil, fmt.Errorf("se esperaba array RESP, recibido %q", cabecera)
	}
	total, err := strconv.Atoi(strings.TrimSpace(cabecera[1:]))
	if err != nil {
		return nil, err
	}
	partes := make([]string, 0, total)
	for i := 0; i < total; i++ {
		longitud, err := lector.ReadString('\n')
		if err != nil {
			return nil, err
		}
		n, err := strconv.Atoi(strings.TrimSpace(longitud[1:]))
		if err != nil {
			return nil, err
		}
		contenido := make([]byte, n+2) // incluye CRLF
		if _, err := io.ReadFull(lector, contenido); err != nil {
			return nil, err
		}
		partes = append(partes, string(contenido[:n]))
	}
	return partes, nil
}

// servidorRESP levanta un servidor TCP que habla RESP de verdad y responde segun el
// comando: HELLO con error (go-redis cae a RESP2), PING con la respuesta indicada.
func servidorRESP(t *testing.T, respuestaPing string) string {
	t.Helper()
	escucha, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("no se pudo escuchar: %v", err)
	}
	t.Cleanup(func() { _ = escucha.Close() })
	go func() {
		for {
			conexion, err := escucha.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer func() { _ = c.Close() }()
				lector := bufio.NewReader(c)
				for {
					partes, err := leerComando(lector)
					if err != nil {
						return
					}
					if len(partes) == 0 {
						continue
					}
					switch strings.ToUpper(partes[0]) {
					case "HELLO":
						_, _ = c.Write([]byte("-ERR unknown command 'HELLO'\r\n"))
					case "PING":
						_, _ = c.Write([]byte(respuestaPing))
					default:
						_, _ = c.Write([]byte("+OK\r\n"))
					}
				}
			}(conexion)
		}
	}()
	return escucha.Addr().String()
}

func TestComprobarRedisConPingCorrecto(t *testing.T) {
	direccion := servidorRESP(t, "+PONG\r\n")
	cliente, err := healthinfra.ClienteRedis(healthinfra.Configuracion{RedisAddr: direccion})
	if err != nil {
		t.Fatalf("cliente: %v", err)
	}
	t.Cleanup(func() { _ = cliente.Close() })

	if err := healthinfra.ComprobarRedis(context.Background(), cliente); err != nil {
		t.Fatalf("se esperaba PING correcto y se obtuvo: %v", err)
	}
}

func TestComprobarRedisDetectaRespuestaInvalida(t *testing.T) {
	direccion := servidorRESP(t, "-ERR no autorizado\r\n")
	cliente, err := healthinfra.ClienteRedis(healthinfra.Configuracion{RedisAddr: direccion})
	if err != nil {
		t.Fatalf("cliente: %v", err)
	}
	t.Cleanup(func() { _ = cliente.Close() })

	if err := healthinfra.ComprobarRedis(context.Background(), cliente); err == nil {
		t.Fatal("un error de Redis debe degradar readiness")
	}
}

func TestComprobarRedisSinClienteFalla(t *testing.T) {
	if err := healthinfra.ComprobarRedis(context.Background(), nil); err == nil {
		t.Fatal("sin cliente debe fallar cerrado")
	}
}

func TestConfiguracionVaciaNoDeclaraDependencias(t *testing.T) {
	t.Setenv("DYNAMODB_TABLE", "")
	t.Setenv("REDIS_ADDR", "")
	if healthinfra.DesdeEntorno().Configurada() {
		t.Fatal("sin configuracion no debe declarar dependencias")
	}
}

func TestConfiguracionDesdeEntorno(t *testing.T) {
	t.Setenv("DYNAMODB_TABLE", "NEXUM_CORE_H08I")
	t.Setenv("REDIS_ADDR", "cache:6379")
	t.Setenv("REDIS_TLS", "true")
	t.Setenv("AWS_REGION", "us-east-2")
	cfg := healthinfra.DesdeEntorno()
	if !cfg.Configurada() || cfg.TablaDynamoDB != "NEXUM_CORE_H08I" || !cfg.RedisTLS || cfg.Region != "us-east-2" {
		t.Fatalf("configuracion inesperada: %+v", cfg)
	}
}

func TestComprobarDynamoDBConTablaExistente(t *testing.T) {
	servidor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-amz-json-1.0")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"Table":{"TableName":"NEXUM_CORE_H08I","TableStatus":"ACTIVE"}}`))
	}))
	t.Cleanup(servidor.Close)

	carga, err := awsconfig.LoadDefaultConfig(context.Background(),
		awsconfig.WithRegion("us-east-2"),
		awsconfig.WithCredentialsProvider(aws.CredentialsProviderFunc(func(context.Context) (aws.Credentials, error) {
			return aws.Credentials{AccessKeyID: "prueba", SecretAccessKey: "prueba"}, nil
		})),
	)
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	cliente := dynamodb.NewFromConfig(carga, func(o *dynamodb.Options) {
		o.BaseEndpoint = aws.String(servidor.URL)
	})

	if err := healthinfra.ComprobarDynamoDB(context.Background(), cliente, "NEXUM_CORE_H08I"); err != nil {
		t.Fatalf("se esperaba tabla disponible: %v", err)
	}
}

func TestComprobarDynamoDBDetectaErrorDelServicio(t *testing.T) {
	servidor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"__type":"ResourceNotFoundException","message":"tabla ausente"}`))
	}))
	t.Cleanup(servidor.Close)

	carga, _ := awsconfig.LoadDefaultConfig(context.Background(), awsconfig.WithRegion("us-east-2"),
		awsconfig.WithCredentialsProvider(aws.CredentialsProviderFunc(func(context.Context) (aws.Credentials, error) {
			return aws.Credentials{AccessKeyID: "prueba", SecretAccessKey: "prueba"}, nil
		})))
	cliente := dynamodb.NewFromConfig(carga, func(o *dynamodb.Options) {
		o.BaseEndpoint = aws.String(servidor.URL)
	})

	if err := healthinfra.ComprobarDynamoDB(context.Background(), cliente, "NEXUM_CORE_H08I"); err == nil {
		t.Fatal("un error del servicio debe degradar readiness")
	}
}

func TestClienteRedisSinDireccionFalla(t *testing.T) {
	if _, err := healthinfra.ClienteRedis(healthinfra.Configuracion{}); err == nil {
		t.Fatal("sin REDIS_ADDR debe fallar cerrado")
	}
}

func TestSinDireccionNoSeInventanValores(t *testing.T) {
	if strings.Contains(healthinfra.Configuracion{}.RedisAddr, ":") {
		t.Fatal("la configuracion vacia no debe traer valores por defecto")
	}
	_ = time.Second
}

func TestRedisProduccionExigeTLS(t *testing.T) {
	_, err := healthinfra.ClienteRedis(healthinfra.Configuracion{
		RedisAddr: "cache:6379", Entorno: "PRODUCTION",
		RedisUsuario: "nexum", RedisPassword: "x", RedisTLS: false,
	})
	if err == nil {
		t.Fatal("PRODUCTION sin TLS debe fallar cerrado")
	}
}

func TestRedisProduccionExigeRBAC(t *testing.T) {
	for _, caso := range []healthinfra.Configuracion{
		{RedisAddr: "cache:6379", Entorno: "PRODUCTION", RedisTLS: true, RedisPassword: "x"},
		{RedisAddr: "cache:6379", Entorno: "PRODUCTION", RedisTLS: true, RedisUsuario: "nexum"},
	} {
		if _, err := healthinfra.ClienteRedis(caso); err == nil {
			t.Fatalf("PRODUCTION sin RBAC completo debe fallar: %+v", caso)
		}
	}
}

func TestRedisProduccionCompletaSeConstruye(t *testing.T) {
	cliente, err := healthinfra.ClienteRedis(healthinfra.Configuracion{
		RedisAddr: "cache:6379", Entorno: "PRODUCTION",
		RedisUsuario: "nexum", RedisPassword: "secreto-de-prueba", RedisTLS: true,
	})
	if err != nil {
		t.Fatalf("configuracion completa no debe fallar: %v", err)
	}
	t.Cleanup(func() { _ = cliente.Close() })
	if cliente.Options().TLSConfig == nil || cliente.Options().TLSConfig.InsecureSkipVerify {
		t.Fatal("TLS debe validar el certificado (sin InsecureSkipVerify)")
	}
}

func TestRedisLocalSinTLSPermitido(t *testing.T) {
	if _, err := healthinfra.ClienteRedis(healthinfra.Configuracion{RedisAddr: "127.0.0.1:6379"}); err != nil {
		t.Fatalf("en local se permite sin TLS: %v", err)
	}
}

func TestDynamoDBRechazaEstadoNoActivo(t *testing.T) {
	for _, estado := range []string{"CREATING", "UPDATING", "DELETING"} {
		servidor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/x-amz-json-1.0")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"Table":{"TableName":"NEXUM_CORE_H08I","TableStatus":"` + estado + `"}}`))
		}))
		carga, _ := awsconfig.LoadDefaultConfig(context.Background(), awsconfig.WithRegion("us-east-2"),
			awsconfig.WithCredentialsProvider(aws.CredentialsProviderFunc(func(context.Context) (aws.Credentials, error) {
				return aws.Credentials{AccessKeyID: "prueba", SecretAccessKey: "prueba"}, nil
			})))
		cliente := dynamodb.NewFromConfig(carga, func(o *dynamodb.Options) {
			o.BaseEndpoint = aws.String(servidor.URL)
		})
		if err := healthinfra.ComprobarDynamoDB(context.Background(), cliente, "NEXUM_CORE_H08I"); err == nil {
			t.Fatalf("estado %s debe degradar readiness", estado)
		}
		servidor.Close()
	}
}

func TestDynamoDBRechazaTablaVacia(t *testing.T) {
	servidor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-amz-json-1.0")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(servidor.Close)
	carga, _ := awsconfig.LoadDefaultConfig(context.Background(), awsconfig.WithRegion("us-east-2"),
		awsconfig.WithCredentialsProvider(aws.CredentialsProviderFunc(func(context.Context) (aws.Credentials, error) {
			return aws.Credentials{AccessKeyID: "prueba", SecretAccessKey: "prueba"}, nil
		})))
	cliente := dynamodb.NewFromConfig(carga, func(o *dynamodb.Options) {
		o.BaseEndpoint = aws.String(servidor.URL)
	})
	if err := healthinfra.ComprobarDynamoDB(context.Background(), cliente, "NEXUM_CORE_H08I"); err == nil {
		t.Fatal("respuesta sin tabla debe degradar readiness")
	}
}
