// Package health contiene las comprobaciones REALES de dependencias que alimentan
// el readiness del backend (I-040/I-050/A-090).
//
// No hay simulaciones: se consulta DynamoDB con DescribeTable y Redis con PING.
// Las pruebas usan dobles; en produccion solo se registran clientes reales.
package health

import (
	"context"
	"crypto/tls"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/redis/go-redis/v9"
)

// Configuracion describe de donde salen las dependencias. Si un campo obligatorio
// esta vacio, esa dependencia NO se registra y readiness la trata como ausente.
type Configuracion struct {
	TablaDynamoDB string
	Region        string
	RedisAddr     string
	RedisUsuario  string
	RedisPassword string
	RedisTLS      bool
	Entorno       string
}

// DesdeEntorno lee la configuracion del proceso. No inventa valores por defecto:
// lo que no este configurado queda vacio y se trata como dependencia ausente.
func DesdeEntorno() Configuracion {
	tlsActivo := strings.EqualFold(os.Getenv("REDIS_TLS"), "true")
	return Configuracion{
		Entorno:       strings.TrimSpace(os.Getenv("APP_ENV")),
		TablaDynamoDB: strings.TrimSpace(os.Getenv("DYNAMODB_TABLE")),
		Region:        strings.TrimSpace(os.Getenv("AWS_REGION")),
		RedisAddr:     strings.TrimSpace(os.Getenv("REDIS_ADDR")),
		RedisUsuario:  strings.TrimSpace(os.Getenv("REDIS_USER")),
		RedisPassword: os.Getenv("REDIS_PASSWORD"),
		RedisTLS:      tlsActivo,
	}
}

// Configurada indica si hay al menos una dependencia que comprobar.
func (c Configuracion) Configurada() bool {
	return c.TablaDynamoDB != "" || c.RedisAddr != ""
}

// ClienteDynamoDB construye el cliente real. Usa la cadena de credenciales por
// defecto (IMDSv2 en EC2, sin claves estaticas en el proceso).
func ClienteDynamoDB(ctx context.Context, cfg Configuracion) (*dynamodb.Client, error) {
	if cfg.TablaDynamoDB == "" {
		return nil, fmt.Errorf("DYNAMODB_TABLE no configurado")
	}
	opciones := []func(*awsconfig.LoadOptions) error{}
	if cfg.Region != "" {
		opciones = append(opciones, awsconfig.WithRegion(cfg.Region))
	}
	carga, err := awsconfig.LoadDefaultConfig(ctx, opciones...)
	if err != nil {
		return nil, fmt.Errorf("configuracion AWS: %w", err)
	}
	return dynamodb.NewFromConfig(carga), nil
}

// ClienteRedis construye el cliente real con TLS y AUTH obligatorios en produccion.
func ClienteRedis(cfg Configuracion) (*redis.Client, error) {
	if cfg.RedisAddr == "" {
		return nil, fmt.Errorf("REDIS_ADDR no configurado")
	}
	// En produccion el modulo I-050 exige TLS y RBAC. Sin ambos, falla cerrado.
	if strings.EqualFold(cfg.Entorno, "PRODUCTION") {
		if !cfg.RedisTLS {
			return nil, fmt.Errorf("redis requiere REDIS_TLS=true en PRODUCTION")
		}
		if cfg.RedisUsuario == "" || cfg.RedisPassword == "" {
			return nil, fmt.Errorf("redis requiere usuario y contrasena RBAC en PRODUCTION")
		}
	}
	opciones := &redis.Options{
		Addr:         cfg.RedisAddr,
		Username:     cfg.RedisUsuario,
		Password:     cfg.RedisPassword,
		DialTimeout:  2 * time.Second,
		ReadTimeout:  2 * time.Second,
		WriteTimeout: 2 * time.Second,
		MaxRetries:   1,
	}
	if cfg.RedisTLS {
		opciones.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	}
	return redis.NewClient(opciones), nil
}

// ComprobarDynamoDB verifica la tabla con DescribeTable: operacion de plano de
// control, bajo coste y sin leer datos de usuario.
func ComprobarDynamoDB(ctx context.Context, cliente *dynamodb.Client, tabla string) error {
	if cliente == nil || tabla == "" {
		return fmt.Errorf("dynamodb no configurado")
	}
	salida, err := cliente.DescribeTable(ctx, &dynamodb.DescribeTableInput{
		TableName: aws.String(tabla),
	})
	if err != nil {
		return fmt.Errorf("dynamodb no disponible: %w", err)
	}
	if salida == nil || salida.Table == nil {
		return fmt.Errorf("dynamodb devolvio una tabla vacia")
	}
	if string(salida.Table.TableStatus) != "ACTIVE" {
		return fmt.Errorf("dynamodb no esta ACTIVE (estado %q)", string(salida.Table.TableStatus))
	}
	return nil
}

// ComprobarRedis verifica conectividad y autenticacion con PING.
func ComprobarRedis(ctx context.Context, cliente *redis.Client) error {
	if cliente == nil {
		return fmt.Errorf("redis no configurado")
	}
	if err := cliente.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("redis no disponible: %w", err)
	}
	return nil
}
