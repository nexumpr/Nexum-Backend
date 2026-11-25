package cache

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"testing"
	"time"
)

// relojPresencia es un reloj determinista para probar el vencimiento del
// heartbeat sin dormir.
type relojPresencia struct {
	mu    sync.Mutex
	ahora time.Time
}

func nuevoRelojPresencia() *relojPresencia {
	return &relojPresencia{ahora: time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)}
}

func (r *relojPresencia) Ahora() time.Time {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.ahora
}

func (r *relojPresencia) Avanzar(d time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ahora = r.ahora.Add(d)
}

// redisPresenciaFake reproduce el sorted set de Redis y permite inyectar una
// caída total del servicio. Todos los comandos exigen la clave canónica: una
// clave fuera de `nexum:v1:readall:*` sería rechazada por el ACL real.
type redisPresenciaFake struct {
	mu       sync.Mutex
	miembros map[string]int64
	ttl      time.Duration
	err      error
	llamadas int
}

func nuevoRedisPresenciaFake() *redisPresenciaFake {
	return &redisPresenciaFake{miembros: map[string]int64{}}
}

func (f *redisPresenciaFake) comprobarClave(clave string) error {
	if clave != ReadAllPresenceKey {
		return fmt.Errorf("clave no autorizada: %s", clave)
	}
	return nil
}

func (f *redisPresenciaFake) ZAdd(_ context.Context, clave string, expiraMS int64, miembro string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.llamadas++
	if f.err != nil {
		return f.err
	}
	if err := f.comprobarClave(clave); err != nil {
		return err
	}
	f.miembros[miembro] = expiraMS
	return nil
}

func (f *redisPresenciaFake) ZRemRangeByScore(_ context.Context, clave string, minimo, maximo string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.llamadas++
	if f.err != nil {
		return f.err
	}
	if err := f.comprobarClave(clave); err != nil {
		return err
	}
	if minimo != "-inf" {
		return fmt.Errorf("rango inesperado: %s..%s", minimo, maximo)
	}
	tope, err := strconv.ParseInt(maximo, 10, 64)
	if err != nil {
		return err
	}
	for miembro, expira := range f.miembros {
		if expira <= tope {
			delete(f.miembros, miembro)
		}
	}
	return nil
}

func (f *redisPresenciaFake) PExpire(_ context.Context, clave string, ttl time.Duration) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.llamadas++
	if f.err != nil {
		return f.err
	}
	if err := f.comprobarClave(clave); err != nil {
		return err
	}
	f.ttl = ttl
	return nil
}

func (f *redisPresenciaFake) ZCount(_ context.Context, clave string, minimo, maximo string) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.llamadas++
	if f.err != nil {
		return 0, f.err
	}
	if err := f.comprobarClave(clave); err != nil {
		return 0, err
	}
	if maximo != "+inf" {
		return 0, fmt.Errorf("rango inesperado: %s..%s", minimo, maximo)
	}
	desde, err := strconv.ParseInt(minimo, 10, 64)
	if err != nil {
		return 0, err
	}
	var activos int64
	for _, expira := range f.miembros {
		if expira >= desde {
			activos++
		}
	}
	return activos, nil
}

func (f *redisPresenciaFake) ZRem(_ context.Context, clave string, miembro string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.llamadas++
	if f.err != nil {
		return f.err
	}
	if err := f.comprobarClave(clave); err != nil {
		return err
	}
	delete(f.miembros, miembro)
	return nil
}

func (f *redisPresenciaFake) estado() (miembros int, ttl time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.miembros), f.ttl
}

func presenciaDePrueba(fake *redisPresenciaFake, reloj *relojPresencia) *ReadAllPresence {
	return (&ReadAllPresence{redis: fake}).ConReloj(reloj.Ahora)
}

func TestPresenciaReadAllPublicaYVence(t *testing.T) {
	fake := nuevoRedisPresenciaFake()
	reloj := nuevoRelojPresencia()
	presencia := presenciaDePrueba(fake, reloj)
	ctx := context.Background()

	if err := presencia.Latir(ctx, "readall-a"); err != nil {
		t.Fatalf("Latir: %v", err)
	}
	miembros, ttl := fake.estado()
	if miembros != 1 || ttl != ReadAllHeartbeatTTL {
		t.Fatalf("heartbeat no publicado con TTL de 60 s: miembros=%d ttl=%s", miembros, ttl)
	}
	if activos, err := presencia.WorkersActivos(ctx); err != nil || activos != 1 {
		t.Fatalf("un worker vigente debe contar: %d %v", activos, err)
	}

	reloj.Avanzar(59 * time.Second)
	if activos, _ := presencia.WorkersActivos(ctx); activos != 1 {
		t.Fatalf("el heartbeat no debe vencer antes de 60 s: %d", activos)
	}
	reloj.Avanzar(2 * time.Second)
	if activos, _ := presencia.WorkersActivos(ctx); activos != 0 {
		t.Fatalf("el heartbeat vencido no debe contar: %d", activos)
	}
	if miembros, _ := fake.estado(); miembros != 1 {
		t.Fatal("la consulta no debe depender de que la limpieza ya haya borrado el miembro")
	}
}

func TestPresenciaReadAllDosNodosBastaUnoActivo(t *testing.T) {
	fake := nuevoRedisPresenciaFake()
	reloj := nuevoRelojPresencia()
	presencia := presenciaDePrueba(fake, reloj)
	ctx := context.Background()

	if err := presencia.Latir(ctx, "readall-a"); err != nil {
		t.Fatal(err)
	}
	reloj.Avanzar(30 * time.Second)
	if err := presencia.Latir(ctx, "readall-b"); err != nil {
		t.Fatal(err)
	}
	if activos, _ := presencia.WorkersActivos(ctx); activos != 2 {
		t.Fatalf("ambos nodos deben publicar: %d", activos)
	}

	// Cae el nodo A de forma ordenada: B sigue habilitando la admisión.
	if err := presencia.Retirar(ctx, "readall-a"); err != nil {
		t.Fatal(err)
	}
	if activos, _ := presencia.WorkersActivos(ctx); activos != 1 {
		t.Fatalf("con un nodo activo debe bastar: %d", activos)
	}

	// El nodo B deja de latir: tras el TTL no queda capacidad conocida.
	reloj.Avanzar(ReadAllHeartbeatTTL + time.Second)
	if activos, _ := presencia.WorkersActivos(ctx); activos != 0 {
		t.Fatalf("sin nodos vigentes no debe haber capacidad: %d", activos)
	}
}

func TestPresenciaReadAllRedisCaidoFallaCerrado(t *testing.T) {
	fake := nuevoRedisPresenciaFake()
	fake.err = errors.New("redis no disponible")
	presencia := presenciaDePrueba(fake, nuevoRelojPresencia())
	ctx := context.Background()

	if err := presencia.Latir(ctx, "readall-a"); err == nil {
		t.Fatal("Latir debía fallar con Redis caído")
	}
	if activos, err := presencia.WorkersActivos(ctx); err == nil || activos != 0 {
		t.Fatalf("WorkersActivos debía fallar cerrado: %d %v", activos, err)
	}
	if err := presencia.Retirar(ctx, "readall-a"); err == nil {
		t.Fatal("Retirar debía fallar con Redis caído")
	}

	sinConfigurar := NewReadAllPresence(nil)
	if _, err := sinConfigurar.WorkersActivos(ctx); !errors.Is(err, ErrPresenciaReadAllNoDisponible) {
		t.Fatalf("sin cliente debe devolver el error de configuración: %v", err)
	}
	if err := sinConfigurar.Latir(ctx, "readall-a"); !errors.Is(err, ErrPresenciaReadAllNoDisponible) {
		t.Fatalf("sin cliente Latir debía fallar cerrado: %v", err)
	}
}

func TestPresenciaReadAllLimpiaVencidosSinScan(t *testing.T) {
	fake := nuevoRedisPresenciaFake()
	reloj := nuevoRelojPresencia()
	presencia := presenciaDePrueba(fake, reloj)
	ctx := context.Background()

	if err := presencia.Latir(ctx, "readall-a"); err != nil {
		t.Fatal(err)
	}
	reloj.Avanzar(ReadAllHeartbeatTTL + 10*time.Second)
	if err := presencia.Latir(ctx, "readall-b"); err != nil {
		t.Fatal(err)
	}
	miembros, _ := fake.estado()
	if miembros != 1 {
		t.Fatalf("la limpieza debe retirar el miembro vencido: %d", miembros)
	}
	if activos, _ := presencia.WorkersActivos(ctx); activos != 1 {
		t.Fatalf("sólo el worker renovado debe contar: %d", activos)
	}
}

func TestPresenciaReadAllRetirarEsIdempotenteYNoAfectaAOtros(t *testing.T) {
	fake := nuevoRedisPresenciaFake()
	presencia := presenciaDePrueba(fake, nuevoRelojPresencia())
	ctx := context.Background()
	if err := presencia.Latir(ctx, "readall-a"); err != nil {
		t.Fatal(err)
	}
	if err := presencia.Latir(ctx, "readall-b"); err != nil {
		t.Fatal(err)
	}
	if err := presencia.Retirar(ctx, "readall-a"); err != nil {
		t.Fatal(err)
	}
	if err := presencia.Retirar(ctx, "readall-a"); err != nil {
		t.Fatalf("retirar dos veces no es un error: %v", err)
	}
	if activos, _ := presencia.WorkersActivos(ctx); activos != 1 {
		t.Fatalf("retirar A no debe afectar a B: %d", activos)
	}
}

func TestPresenciaReadAllClaveCanonicaYTTL(t *testing.T) {
	if ReadAllPresenceKey != "nexum:v1:readall:workers" {
		t.Fatalf("clave fuera del espacio autorizado: %s", ReadAllPresenceKey)
	}
	if ReadAllHeartbeatTTL != 60*time.Second {
		t.Fatalf("TTL del heartbeat incorrecto: %s", ReadAllHeartbeatTTL)
	}
}
