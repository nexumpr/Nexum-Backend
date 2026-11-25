package cache

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	// ReadAllPresenceKey es el registro compartido de workers READALL. Está
	// dentro del espacio `nexum:v1:*` autorizado por el ACL de A-080 y no
	// contiene identificadores de usuario: sólo el identificador aleatorio del
	// worker y su instante de vencimiento.
	ReadAllPresenceKey = "nexum:v1:readall:workers"

	// ReadAllHeartbeatTTL es el tiempo máximo que un worker ausente sigue
	// anunciándose. El worker renueva cada ciclo (5 s) y al cerrarse lo retira,
	// de modo que una caída se detecta como máximo en 60 s.
	ReadAllHeartbeatTTL = 60 * time.Second
)

// ErrPresenciaReadAllNoDisponible indica que la presencia compartida no está
// configurada. El consumidor debe tratarlo como fallo cerrado (503), nunca como
// "sin límite" ni como worker activo.
var ErrPresenciaReadAllNoDisponible = errors.New("presencia de workers READALL no configurada")

// presenciaRedis es el subconjunto de comandos Redis que necesita la presencia
// de workers. Se abstrae para probar vencimiento, dos nodos y caída de Redis sin
// un servidor real. La implementación productiva envuelve *redis.Client.
type presenciaRedis interface {
	// ZAdd registra el vencimiento (epoch ms) del miembro.
	ZAdd(ctx context.Context, clave string, expiraMS int64, miembro string) error
	// ZRemRangeByScore elimina miembros con vencimiento anterior al instante.
	ZRemRangeByScore(ctx context.Context, clave string, minimo, maximo string) error
	// PExpire fija el TTL de la clave completa.
	PExpire(ctx context.Context, clave string, ttl time.Duration) error
	// ZCount cuenta miembros con vencimiento igual o posterior al instante.
	ZCount(ctx context.Context, clave string, minimo, maximo string) (int64, error)
	// ZRem retira un miembro concreto.
	ZRem(ctx context.Context, clave string, miembro string) error
}

// redisPresenciaCliente adapta *redis.Client a presenciaRedis. Usa comandos de
// sorted set, disponibles con las categorías @read/@write/@keyspace del ACL de
// A-080; no usa SCAN ni KEYS.
type redisPresenciaCliente struct {
	cliente *redis.Client
}

func (a redisPresenciaCliente) ZAdd(ctx context.Context, clave string, expiraMS int64, miembro string) error {
	return a.cliente.ZAdd(ctx, clave, redis.Z{Score: float64(expiraMS), Member: miembro}).Err()
}

func (a redisPresenciaCliente) ZRemRangeByScore(ctx context.Context, clave string, minimo, maximo string) error {
	return a.cliente.ZRemRangeByScore(ctx, clave, minimo, maximo).Err()
}

func (a redisPresenciaCliente) PExpire(ctx context.Context, clave string, ttl time.Duration) error {
	return a.cliente.PExpire(ctx, clave, ttl).Err()
}

func (a redisPresenciaCliente) ZCount(ctx context.Context, clave string, minimo, maximo string) (int64, error) {
	return a.cliente.ZCount(ctx, clave, minimo, maximo).Result()
}

func (a redisPresenciaCliente) ZRem(ctx context.Context, clave string, miembro string) error {
	return a.cliente.ZRem(ctx, clave, miembro).Err()
}

// ReadAllPresence implementa el heartbeat y la consulta de capacidad de E063 en
// Redis distribuido. Varios nodos publican su propio miembro; la admisión exige
// al menos uno vigente, no que el nodo que atiende sea el que procesa.
type ReadAllPresence struct {
	redis presenciaRedis
	ahora func() time.Time
}

// NewReadAllPresence construye la presencia sobre el cliente Redis real. Sin
// cliente devuelve un objeto que falla cerrado en todos sus métodos: el endpoint
// responderá 503 en lugar de aceptar trabajo que nadie ejecutaría.
func NewReadAllPresence(cliente *redis.Client) *ReadAllPresence {
	p := &ReadAllPresence{ahora: func() time.Time { return time.Now().UTC() }}
	if cliente != nil {
		p.redis = redisPresenciaCliente{cliente: cliente}
	}
	return p
}

// ConReloj inyecta un reloj determinista en pruebas.
func (p *ReadAllPresence) ConReloj(ahora func() time.Time) *ReadAllPresence {
	if p != nil && ahora != nil {
		p.ahora = ahora
	}
	return p
}

// Latir registra o renueva la presencia del worker con un vencimiento a 60 s.
// También limpia miembros vencidos para que la clave no crezca sin límite.
func (p *ReadAllPresence) Latir(ctx context.Context, owner string) error {
	if p == nil || p.redis == nil {
		return ErrPresenciaReadAllNoDisponible
	}
	owner = strings.TrimSpace(owner)
	if ctx == nil || owner == "" {
		return errors.New("presencia READALL inválida")
	}
	ahora := p.instanteMillis()
	expira := ahora + ReadAllHeartbeatTTL.Milliseconds()
	if err := p.redis.ZAdd(ctx, ReadAllPresenceKey, expira, owner); err != nil {
		return fmt.Errorf("registro de heartbeat READALL: %w", err)
	}
	if err := p.redis.ZRemRangeByScore(ctx, ReadAllPresenceKey, "-inf", strconv.FormatInt(ahora, 10)); err != nil {
		return fmt.Errorf("limpieza de heartbeats READALL: %w", err)
	}
	if err := p.redis.PExpire(ctx, ReadAllPresenceKey, ReadAllHeartbeatTTL); err != nil {
		return fmt.Errorf("ttl de presencia READALL: %w", err)
	}
	return nil
}

// Retirar elimina la presencia del worker. Es idempotente: retirar dos veces no
// es un error y nunca afecta a los demás nodos.
func (p *ReadAllPresence) Retirar(ctx context.Context, owner string) error {
	if p == nil || p.redis == nil {
		return ErrPresenciaReadAllNoDisponible
	}
	owner = strings.TrimSpace(owner)
	if ctx == nil || owner == "" {
		return errors.New("presencia READALL inválida")
	}
	if err := p.redis.ZRem(ctx, ReadAllPresenceKey, owner); err != nil {
		return fmt.Errorf("retiro de heartbeat READALL: %w", err)
	}
	return nil
}

// WorkersActivos cuenta los workers cuyo heartbeat no ha vencido. Un error de
// Redis se propaga: el llamador responde 503 y no acepta trabajo.
func (p *ReadAllPresence) WorkersActivos(ctx context.Context) (int64, error) {
	if p == nil || p.redis == nil {
		return 0, ErrPresenciaReadAllNoDisponible
	}
	if ctx == nil {
		return 0, errors.New("presencia READALL inválida")
	}
	activos, err := p.redis.ZCount(ctx, ReadAllPresenceKey, strconv.FormatInt(p.instanteMillis(), 10), "+inf")
	if err != nil {
		return 0, fmt.Errorf("consulta de workers READALL: %w", err)
	}
	if activos < 0 {
		return 0, nil
	}
	return activos, nil
}

func (p *ReadAllPresence) instanteMillis() int64 {
	if p == nil || p.ahora == nil {
		return time.Now().UTC().UnixMilli()
	}
	return p.ahora().UTC().UnixMilli()
}
