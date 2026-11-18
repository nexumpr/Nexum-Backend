package transport_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	nexumhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
)

func TestE071TipsPublicos(t *testing.T) {
	router := nexumhttp.NewRouter()
	router.HandleFunc(http.MethodGet, "/education/tips", nexumhttp.NewTipsHandler(nil, nil).ServeHTTP)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/education/tips", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("se esperaba 200, se obtuvo %d: %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Cache-Control") != "public, max-age=300" {
		t.Fatalf("Cache-Control inesperado: %q", rec.Header().Get("Cache-Control"))
	}
	sobre, items := decodificarPagina(t, rec.Body.Bytes())
	if len(sobre) != 1 {
		t.Fatalf("la respuesta debe contener solo 'items', contiene %d claves", len(sobre))
	}
	if len(items) != 8 {
		t.Fatalf("se esperaban 8 consejos, hay %d", len(items))
	}
	for i, item := range items {
		for _, campo := range []string{"id", "title", "text"} {
			if _, existe := item[campo]; !existe {
				t.Fatalf("elemento %d sin el campo %q", i, campo)
			}
		}
		if len(item) != 3 {
			t.Fatalf("elemento %d con campos adicionales al esquema Tip", i)
		}
	}
	if strings.Contains(rec.Body.String(), "example.invalid") {
		t.Fatal("el contenido publico no puede contener correos de ejemplo")
	}
}

func TestE071TipsFailOpenSinRedis(t *testing.T) {
	router := nexumhttp.NewRouter()
	router.HandleFunc(http.MethodGet, "/education/tips", nexumhttp.NewTipsHandler(nil, nil).ServeHTTP)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/education/tips", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("sin Redis la ruta publica debe ser fail-open, obtuvo %d", rec.Code)
	}
}

func TestE071TipsMetodoNoPermitido(t *testing.T) {
	router := nexumhttp.NewRouter()
	router.HandleFunc(http.MethodGet, "/education/tips", nexumhttp.NewTipsHandler(nil, nil).ServeHTTP)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/education/tips", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("se esperaba 405, obtuvo %d", rec.Code)
	}
	if rec.Header().Get("Allow") != http.MethodGet {
		t.Fatalf("Allow inesperado: %q", rec.Header().Get("Allow"))
	}
}
