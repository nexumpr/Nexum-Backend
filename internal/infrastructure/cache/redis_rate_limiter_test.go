package cache

import (
	"strings"
	"testing"
	"time"
)

// Las clases documentadas de la Oleada 4 usan 60/min (POLITICAS_TRANSVERSALES_H06:
// operaciones autenticadas). El valor de analítica no cambia; solo se separan los
// contadores por dominio.
var clasesOleada4 = []string{"analytics", "notifications", "push", "preferences"}

func TestLimitesPorClaseResuelveMapaExplicito(t *testing.T) {
	limites := map[string]int64{}
	for _, clase := range clasesOleada4 {
		limites[clase] = 60
	}
	limiter := NewAuthRateLimiterPorClase(nil, 60, limites, time.Minute)
	for _, clase := range clasesOleada4 {
		if got := limiter.limitePorClase(clase); got != 60 {
			t.Fatalf("clase %q: limite=%d, esperado 60", clase, got)
		}
	}
	if got := limiter.limitePorClase("desconocida"); got != 60 {
		t.Fatalf("clase desconocida debe usar el valor por defecto: %d", got)
	}
}

func TestLimitesPorClaseIgnoraValoresInvalidos(t *testing.T) {
	limiter := NewAuthRateLimiterPorClase(nil, 60, map[string]int64{
		"analytics": 0,
		"push":      -5,
	}, time.Minute)
	if got := limiter.limitePorClase("analytics"); got != 60 {
		t.Fatalf("un limite no positivo debe ignorarse: %d", got)
	}
	if got := limiter.limitePorClase("push"); got != 60 {
		t.Fatalf("un limite negativo debe ignorarse: %d", got)
	}
}

func TestNewAuthRateLimiterConservaComportamientoPrevio(t *testing.T) {
	limiter := NewAuthRateLimiter(nil, 0, 0)
	if limiter.max != 5 {
		t.Fatalf("el maximo por defecto debe ser 5: %d", limiter.max)
	}
	if limiter.window != 15*time.Minute {
		t.Fatalf("la ventana por defecto debe ser 15m: %v", limiter.window)
	}
	if got := limiter.limitePorClase("analytics"); got != 5 {
		t.Fatalf("sin mapa, toda clase usa el maximo: %d", got)
	}
}

// TestClaveRateLimitAislaDominioYUsuario es la garantia de aislamiento: la clave
// Redis incluye la clase y el hash del sujeto, de modo que dos dominios o dos
// usuarios nunca comparten contador.
func TestClaveRateLimitAislaDominioYUsuario(t *testing.T) {
	usuarioA := "018f90ef-08e5-7a68-b09f-2b67a47c92f0"
	usuarioB := "018f90ef-08e5-7a68-b09f-000000000002"

	claves := map[string]string{}
	for _, clase := range clasesOleada4 {
		for _, usuario := range []string{usuarioA, usuarioB} {
			clave := claveRateLimit(clase, usuario)
			if otra, repetida := claves[clave]; repetida {
				t.Fatalf("clave compartida entre %s y %s/%s: %s", otra, clase, usuario, clave)
			}
			claves[clave] = clase + "/" + usuario
			if strings.Contains(clave, usuario) {
				t.Fatalf("la clave no debe contener el identificador en claro: %s", clave)
			}
		}
	}
	if len(claves) != len(clasesOleada4)*2 {
		t.Fatalf("claves distintas insuficientes: %d", len(claves))
	}
	claveA := claveRateLimit("analytics", usuarioA)
	claveB := claveRateLimit("analytics", usuarioA)
	if claveA != claveB {
		t.Fatal("la misma clase y sujeto deben producir la misma clave")
	}
}

func TestClaveRateLimitNormalizaClase(t *testing.T) {
	if claveRateLimit(" analytics ", "u1") != claveRateLimit("analytics", "u1") {
		t.Fatal("los espacios no deben crear un cubo distinto")
	}
	if claveRateLimit("", "u1") == claveRateLimit("analytics", "u1") {
		t.Fatal("una clase vacia no puede colisionar con analytics")
	}
}

func TestAllowSinClienteFallaCerrado(t *testing.T) {
	limiter := NewAuthRateLimiterPorClase(nil, 60, nil, time.Minute)
	permitido, _, err := limiter.Allow(t.Context(), "analytics", "u1")
	if err == nil || permitido {
		t.Fatalf("sin Redis debe fallar cerrado: permitido=%v err=%v", permitido, err)
	}
}
