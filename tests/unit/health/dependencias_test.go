package health_test

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"

	healthinfra "github.com/nexumpr/Nexum-Backend/internal/infrastructure/health"
)

// servidorRESP levanta un servidor TCP minimo que responde +PONG a PING.
func servidorRESP(t *testing.T, responder string) string {
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
				buffer := make([]byte, 512)
				_, _ = c.Read(buffer)
				_, _ = c.Write([]byte(responder))
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
