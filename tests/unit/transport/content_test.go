package transport_test

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

	nexumhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
)

// Pruebas HTTP de E070 (GET /api/v1/help/faq). El contenido es estatico; la unica
// dependencia externa es la cache publica, que debe ser fail-open.

const faqEsperadasHTTP = 10

func TestE070FAQPublica(t *testing.T) {
	router := nexumhttp.NewRouter()
	router.HandleFunc(http.MethodGet, "/help/faq", nexumhttp.NewFAQHandler(nil, nil).ServeHTTP)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/help/faq", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("se esperaba 200, se obtuvo %d: %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Cache-Control") != "public, max-age=300" {
		t.Fatalf("Cache-Control inesperado: %q", rec.Header().Get("Cache-Control"))
	}
	if rec.Header().Get("Content-Type") != "application/json; charset=utf-8" {
		t.Fatalf("Content-Type inesperado: %q", rec.Header().Get("Content-Type"))
	}

	sobre, items := decodificarPagina(t, rec.Body.Bytes())
	if len(sobre) != 1 {
		t.Fatalf("la respuesta debe contener solo 'items', contiene %d claves", len(sobre))
	}
	if len(items) != faqEsperadasHTTP {
		t.Fatalf("se esperaban %d preguntas, hay %d", faqEsperadasHTTP, len(items))
	}
	for i, item := range items {
		for _, campo := range []string{"id", "question", "answer"} {
			if _, existe := item[campo]; !existe {
				t.Fatalf("elemento %d sin el campo %q", i, campo)
			}
		}
		if len(item) != 3 {
			t.Fatalf("elemento %d con campos adicionales al esquema Faq", i)
		}
		if !strings.HasPrefix(item["id"].(string), "faq-") {
			t.Fatalf("identificador inesperado: %v", item["id"])
		}
	}
	if strings.Contains(rec.Body.String(), "example.invalid") {
		t.Fatal("el contenido publico no puede contener correos de ejemplo")
	}
}

func TestE070FAQFailOpenConRedisInalcanzable(t *testing.T) {
	cliente := redis.NewClient(&redis.Options{
		Addr:         "127.0.0.1:1",
		DialTimeout:  200 * time.Millisecond,
		ReadTimeout:  200 * time.Millisecond,
		WriteTimeout: 200 * time.Millisecond,
		MaxRetries:   -1,
	})
	t.Cleanup(func() { _ = cliente.Close() })

	router := nexumhttp.NewRouter()
	router.HandleFunc(http.MethodGet, "/help/faq", nexumhttp.NewFAQHandler(cliente, nil).ServeHTTP)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/help/faq", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("un fallo de Redis no puede impedir servir contenido: %d", rec.Code)
	}
	if _, items := decodificarPagina(t, rec.Body.Bytes()); len(items) != faqEsperadasHTTP {
		t.Fatalf("se esperaban %d preguntas, hay %d", faqEsperadasHTTP, len(items))
	}
}

func TestE070FAQLimitaPorIP(t *testing.T) {
	router := nexumhttp.NewRouter()
	handler := nexumhttp.NewFAQHandler(nil, nil)
	router.HandleFunc(http.MethodGet, "/help/faq",
		nexumhttp.LimitarPorIP(limitadorFalso{permitido: false, retryAfter: 42 * time.Second}, "public_content", handler.ServeHTTP))

	peticion := httptest.NewRequest(http.MethodGet, "/api/v1/help/faq", nil)
	peticion.RemoteAddr = "10.42.1.10:443"
	peticion.Header.Set("X-Forwarded-For", "203.0.113.10, 130.176.10.20")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, peticion)

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("se esperaba 429, se obtuvo %d", rec.Code)
	}
	if rec.Header().Get("Retry-After") != "42" {
		t.Fatalf("Retry-After inesperado: %q", rec.Header().Get("Retry-After"))
	}
	if !strings.Contains(rec.Body.String(), "RATE_LIMITED") {
		t.Fatalf("cuerpo inesperado: %s", rec.Body.String())
	}
}

func TestE070FAQCuotaFailOpenSiElLimitadorFalla(t *testing.T) {
	router := nexumhttp.NewRouter()
	handler := nexumhttp.NewFAQHandler(nil, nil)
	router.HandleFunc(http.MethodGet, "/help/faq",
		nexumhttp.LimitarPorIP(limitadorFalso{permitido: false, err: errors.New("redis caido")}, "public_content", handler.ServeHTTP))

	peticion := httptest.NewRequest(http.MethodGet, "/api/v1/help/faq", nil)
	peticion.Header.Set("X-Forwarded-For", "203.0.113.11")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, peticion)

	if rec.Code != http.StatusOK {
		t.Fatalf("un fallo de cuota no puede impedir servir contenido: %d", rec.Code)
	}
}

func TestE070FAQRechazaMetodoNoPermitido(t *testing.T) {
	router := nexumhttp.NewRouter()
	router.HandleFunc(http.MethodGet, "/help/faq", nexumhttp.NewFAQHandler(nil, nil).ServeHTTP)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/help/faq", nil))

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("se esperaba 405, se obtuvo %d", rec.Code)
	}
	if rec.Header().Get("Allow") != http.MethodGet {
		t.Fatalf("Allow inesperado: %q", rec.Header().Get("Allow"))
	}
}

// limitadorFalso aplica la interfaz LimitadorRuta sin Redis.
type limitadorFalso struct {
	permitido  bool
	retryAfter time.Duration
	err        error
}

func (l limitadorFalso) Allow(context.Context, string, string) (bool, time.Duration, error) {
	return l.permitido, l.retryAfter, l.err
}

func decodificarPagina(t *testing.T, cuerpo []byte) (map[string]json.RawMessage, []map[string]any) {
	t.Helper()
	var sobre map[string]json.RawMessage
	if err := json.Unmarshal(cuerpo, &sobre); err != nil {
		t.Fatalf("la respuesta no es un objeto JSON: %v", err)
	}
	bruto, existe := sobre["items"]
	if !existe {
		t.Fatal("la respuesta no contiene 'items'")
	}
	var items []map[string]any
	if err := json.Unmarshal(bruto, &items); err != nil {
		t.Fatalf("'items' no es un arreglo de objetos: %v", err)
	}
	return sobre, items
}
