package http

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nexumpr/Nexum-Backend/internal/transport/http/middleware"
)

func TestFormatYParseIfMatch(t *testing.T) {
	if etag := FormatETag(7); etag != `"v7"` {
		t.Fatalf("ETag inesperado: %s", etag)
	}
	casos := []struct {
		valor string
		ok    bool
	}{
		{`"v1"`, true},
		{`W/"v12"`, true},
		{`  "v3"  `, true},
		{``, false},
		{`v1`, false},
		{`"1"`, false},
		{`"v0"`, false},
		{`"v-1"`, false},
		{`"vx"`, false},
	}
	for _, caso := range casos {
		version, err := ParseIfMatchVersion(caso.valor)
		if caso.ok {
			if err != nil || version < 1 {
				t.Fatalf("%q debia ser valido: version=%d err=%v", caso.valor, version, err)
			}
			continue
		}
		if err == nil {
			t.Fatalf("%q debia rechazarse", caso.valor)
		}
	}
	if _, err := ParseIfMatchVersion(""); !errors.Is(err, ErrIfMatchRequerido) {
		t.Fatalf("la ausencia debe devolver ErrIfMatchRequerido, obtuvo %v", err)
	}
	if _, err := ParseIfMatchVersion("nope"); !errors.Is(err, ErrIfMatchInvalido) {
		t.Fatalf("el formato invalido debe devolver ErrIfMatchInvalido, obtuvo %v", err)
	}
}

func TestValidarLocaleYZonaIANA(t *testing.T) {
	if _, ok := ValidarLocale("es-MX"); !ok {
		t.Fatal("es-MX debe ser valido")
	}
	if _, ok := ValidarLocale("fr-FR"); ok {
		t.Fatal("fr-FR no pertenece al enum")
	}
	if _, err := ValidarZonaIANA("America/Mexico_City"); err != nil {
		t.Fatalf("zona IANA valida rechazada: %v", err)
	}
	for _, invalida := range []string{"", "Local", "Not/AZone", "America/Mexico City"} {
		if _, err := ValidarZonaIANA(invalida); err == nil {
			t.Fatalf("zona %q debia rechazarse", invalida)
		}
	}
}

type limitadorFalso struct {
	permitido bool
	retry     time.Duration
	err       error
}

func (l limitadorFalso) Allow(context.Context, string, string) (bool, time.Duration, error) {
	return l.permitido, l.retry, l.err
}

func TestLimitarPorIPResponde429(t *testing.T) {
	handler := LimitarPorIP(limitadorFalso{permitido: false, retry: 5 * time.Second}, "e022", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	peticion := httptest.NewRequest(http.MethodGet, "/api/v1/catalogs/categories", nil)
	peticion.Header.Set("X-Forwarded-For", "203.0.113.10")
	rec := httptest.NewRecorder()
	handler(rec, peticion)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("se esperaba 429, obtuvo %d", rec.Code)
	}
	if rec.Header().Get("Retry-After") != "5" {
		t.Fatalf("Retry-After inesperado: %q", rec.Header().Get("Retry-After"))
	}
}

func TestLimitarPorIPFailOpen(t *testing.T) {
	handler := LimitarPorIP(limitadorFalso{err: errors.New("redis caido")}, "e022", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	peticion := httptest.NewRequest(http.MethodGet, "/api/v1/catalogs/categories", nil)
	peticion.Header.Set("X-Forwarded-For", "203.0.113.10")
	rec := httptest.NewRecorder()
	handler(rec, peticion)
	if rec.Code != http.StatusOK {
		t.Fatalf("un fallo de Redis debe ser fail-open, obtuvo %d", rec.Code)
	}
}

func TestLimitarPorUsuarioUsaContexto(t *testing.T) {
	handler := LimitarPorUsuario(limitadorFalso{permitido: false, retry: time.Second}, "e024", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	peticion := httptest.NewRequest(http.MethodGet, "/api/v1/income", nil)
	peticion = peticion.WithContext(context.WithValue(peticion.Context(), middleware.UserIDKey, "usuario-1"))
	rec := httptest.NewRecorder()
	handler(rec, peticion)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("se esperaba 429 para usuario autenticado, obtuvo %d", rec.Code)
	}
}
