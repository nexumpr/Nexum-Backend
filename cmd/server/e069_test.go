package main

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	nexumhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
)

// TestE069RegistroOleada1 comprueba el contrato de registro de E069: PATCH sobre la ruta de
// preferencias, en la lista cerrada de la Oleada 1 y con funcion de montaje.
func TestE069RegistroOleada1(t *testing.T) {
	registroOleada1Global.mu.Lock()
	montaje, ok := registroOleada1Global.porID["E069"]
	registroOleada1Global.mu.Unlock()

	if !ok {
		t.Fatal("E069 debe estar registrado por su init()")
	}
	if montaje.Metodo != http.MethodPatch {
		t.Fatalf("E069 debe registrarse como PATCH, obtenido %q", montaje.Metodo)
	}
	if montaje.Ruta != "/users/me/preferences" {
		t.Fatalf("ruta inesperada para E069: %q", montaje.Ruta)
	}
	if montaje.Montar == nil {
		t.Fatal("E069 debe declarar su funcion de montaje")
	}
}

// TestE069MontarFallaCerrado: sin persistencia, sin claves JWT y sin autorizador, el endpoint
// queda montado y responde 503 en lugar de autorizar o escribir con datos en memoria.
func TestE069MontarFallaCerrado(t *testing.T) {
	router := nexumhttp.NewRouter()
	montarE069(&dependenciasOleada{
		Router: router,
		Logger: slog.New(slog.NewJSONHandler(io.Discard, nil)),
	})

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/users/me/preferences", nil)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("If-Match", `"v1"`)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("sin dependencias el endpoint debe responder 503, respondio %d (%s)", rec.Code, rec.Body.String())
	}
	if cache := rec.Header().Get("Cache-Control"); cache != "private, no-store" {
		t.Fatalf("Cache-Control debe ser 'private, no-store', obtenido %q", cache)
	}
}
