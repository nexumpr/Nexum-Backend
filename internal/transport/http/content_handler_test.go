package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	appContent "github.com/nexumpr/Nexum-Backend/internal/application/content"
)

// Pruebas internas del handler de contenido publico. Se sustituye unicamente el
// cargador de cache para observar con que clave, TTL y cliente se invoca; en
// produccion el campo apunta siempre a cache.GetOrLoadJSON.

func TestContentHandlerUsaClaveYTTLDeLaCachePublica(t *testing.T) {
	handler := NewFAQHandler(nil, nil)

	var (
		llamadas int
		clave    string
		ttl      time.Duration
		cliente  *redis.Client
	)
	handler.cargar = func(ctx context.Context, c *redis.Client, k string, d time.Duration, load func() ([]byte, error)) ([]byte, error) {
		llamadas++
		cliente = c
		clave = k
		ttl = d
		return load()
	}

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/help/faq", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("se esperaba 200, se obtuvo %d", rec.Code)
	}
	if llamadas != 1 {
		t.Fatalf("el cargador de cache debe invocarse una vez, se invoco %d", llamadas)
	}
	if clave != appContent.CacheKeyFAQ {
		t.Fatalf("clave de cache inesperada: %q", clave)
	}
	if ttl != appContent.CacheTTL {
		t.Fatalf("TTL inesperado: %s", ttl)
	}
	if cliente != nil {
		t.Fatal("con Redis nil el cargador debe recibir nil")
	}

	esperado, err := appContent.FAQPayload()
	if err != nil {
		t.Fatalf("error al construir el payload esperado: %v", err)
	}
	if rec.Body.String() != string(esperado) {
		t.Fatal("el handler debe escribir exactamente el payload de la cache")
	}
}

func TestContentHandlerSirveElJSONCacheadoSinRemarshal(t *testing.T) {
	handler := NewFAQHandler(nil, nil)
	cacheado := []byte(`{"items":[]}`)
	handler.cargar = func(context.Context, *redis.Client, string, time.Duration, func() ([]byte, error)) ([]byte, error) {
		return cacheado, nil
	}

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/help/faq", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("se esperaba 200, se obtuvo %d", rec.Code)
	}
	if rec.Body.String() != string(cacheado) {
		t.Fatalf("se esperaba el cuerpo cacheado %s, se obtuvo %s", cacheado, rec.Body.String())
	}
}

func TestContentHandlerFallaCerradoSiLaCargaFalla(t *testing.T) {
	handler := NewFAQHandler(nil, nil)
	handler.cargar = func(context.Context, *redis.Client, string, time.Duration, func() ([]byte, error)) ([]byte, error) {
		return nil, errors.New("fallo de carga")
	}

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/help/faq", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("se esperaba 500, se obtuvo %d", rec.Code)
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("un error no debe cachearse: %q", rec.Header().Get("Cache-Control"))
	}
	var sobre struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &sobre); err != nil {
		t.Fatalf("el error debe ser JSON valido: %v", err)
	}
	if sobre.Error.Code != "INTERNAL_ERROR" {
		t.Fatalf("codigo de error inesperado: %q", sobre.Error.Code)
	}
}

func TestContentHandlerSinDependenciasDevuelve503(t *testing.T) {
	handler := &ContentHandler{}

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/help/faq", nil))

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("se esperaba 503, se obtuvo %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "DEPENDENCY_UNAVAILABLE") {
		t.Fatalf("cuerpo inesperado: %s", rec.Body.String())
	}
}
