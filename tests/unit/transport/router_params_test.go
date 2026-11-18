package transport_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	internalhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
)

// Pruebas del soporte mínimo de rutas parametrizadas del router (necesario para E009).
// Verifican prioridad literal, un único segmento por parámetro, 405 con `Allow`, plantilla
// correcta para métricas y que las rutas literales de E001–E008 no cambian.

const e009RutaSesiones = "/api/v1/auth/sessions"

func e009HandlerRuta(nombre string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Ruta", nombre)
		w.WriteHeader(http.StatusOK)
	}
}

func e009RouterConLiterales() *internalhttp.Router {
	router := internalhttp.NewRouter()
	literales := []struct {
		metodo string
		ruta   string
		nombre string
	}{
		{http.MethodPost, "/auth/register", "register"},
		{http.MethodPost, "/auth/login", "login"},
		{http.MethodPost, "/auth/refresh", "refresh"},
		{http.MethodPost, "/auth/logout", "logout"},
		{http.MethodPost, "/auth/password/forgot", "forgot"},
		{http.MethodPost, "/auth/password/reset", "reset"},
		{http.MethodPost, "/auth/password/change", "change"},
		{http.MethodGet, "/auth/sessions", "sessions"},
	}
	for _, literal := range literales {
		router.HandleFunc(literal.metodo, literal.ruta, e009HandlerRuta(literal.nombre))
	}
	return router
}

// TestE009RouterLiteralGanaSobreParametrica cubre la prioridad absoluta de la ruta literal.
func TestE009RouterLiteralGanaSobreParametrica(t *testing.T) {
	router := e009RouterConLiterales()
	router.HandleFunc(http.MethodGet, "/auth/sessions/{sessionId}", e009HandlerRuta("param"))

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, e009RutaSesiones, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("esperado 200 del literal, obtenido %d", rec.Code)
	}
	if ruta := rec.Header().Get("X-Ruta"); ruta != "sessions" {
		t.Fatalf("la ruta literal debe ganar, obtenido %q", ruta)
	}

	recParam := httptest.NewRecorder()
	router.ServeHTTP(recParam, httptest.NewRequest(http.MethodGet, e009RutaSesiones+"/abc", nil))
	if ruta := recParam.Header().Get("X-Ruta"); ruta != "param" {
		t.Fatalf("un segmento extra debe resolver a la plantilla, obtenido %q", ruta)
	}
}

// TestE009RouterParametroExponeValorYPlantilla cubre `SetPathValue` y `TemplateFor`.
func TestE009RouterParametroExponeValorYPlantilla(t *testing.T) {
	router := e009RouterConLiterales()
	var capturado string
	router.HandleFunc(http.MethodDelete, "/auth/sessions/{sessionId}", func(w http.ResponseWriter, r *http.Request) {
		capturado = r.PathValue("sessionId")
		w.WriteHeader(http.StatusNoContent)
	})

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, e009RutaSesiones+"/abc", nil))

	if rec.Code != http.StatusNoContent {
		t.Fatalf("esperado 204, obtenido %d (%s)", rec.Code, rec.Body.String())
	}
	if capturado != "abc" {
		t.Fatalf("el handler debe recibir el parametro, obtenido %q", capturado)
	}
	if plantilla := router.TemplateFor(e009RutaSesiones + "/abc"); plantilla != e009RutaSesiones+"/{sessionId}" {
		t.Fatalf("TemplateFor debe devolver la plantilla, obtenido %q", plantilla)
	}
	if plantilla := router.TemplateFor(e009RutaSesiones); plantilla != e009RutaSesiones {
		t.Fatalf("TemplateFor debe devolver la ruta literal, obtenido %q", plantilla)
	}
	if plantilla := router.TemplateFor("/api/v1/no-existe"); plantilla != "/unknown" {
		t.Fatalf("una ruta desconocida debe ser /unknown, obtenido %q", plantilla)
	}
}

// TestE009RouterUnSoloSegmentoPorParametro cubre que el parámetro no cruza `/`.
func TestE009RouterUnSoloSegmentoPorParametro(t *testing.T) {
	router := e009RouterConLiterales()
	router.HandleFunc(http.MethodDelete, "/auth/sessions/{sessionId}", e009HandlerRuta("param"))

	casos := []struct {
		nombre string
		ruta   string
	}{
		{"dos segmentos", e009RutaSesiones + "/abc/def"},
		{"segmento vacio", e009RutaSesiones + "//"},
		{"sin parametro", e009RutaSesiones + "/"},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, caso.ruta, nil))
			if rec.Code != http.StatusNotFound {
				t.Fatalf("esperado 404, obtenido %d (%s)", rec.Code, rec.Body.String())
			}
		})
	}
}

// TestE009RouterMetodoIncorrecto405 cubre 405 con `Allow` en una ruta parametrizada.
func TestE009RouterMetodoIncorrecto405(t *testing.T) {
	router := e009RouterConLiterales()
	router.HandleFunc(http.MethodDelete, "/auth/sessions/{sessionId}", e009HandlerRuta("param"))

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, e009RutaSesiones+"/abc", nil))

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("esperado 405, obtenido %d", rec.Code)
	}
	if allow := rec.Header().Get("Allow"); allow != http.MethodDelete {
		t.Fatalf("Allow debe contener DELETE, obtenido %q", allow)
	}
}

// TestE009RouterConservaRutasE001E008 cubre que ninguna ruta literal existente cambia de
// método ni de handler.
func TestE009RouterConservaRutasE001E008(t *testing.T) {
	router := e009RouterConLiterales()
	router.HandleFunc(http.MethodDelete, "/auth/sessions/{sessionId}", e009HandlerRuta("param"))

	casos := []struct {
		metodo string
		ruta   string
		nombre string
	}{
		{http.MethodPost, "/api/v1/auth/register", "register"},
		{http.MethodPost, "/api/v1/auth/login", "login"},
		{http.MethodPost, "/api/v1/auth/refresh", "refresh"},
		{http.MethodPost, "/api/v1/auth/logout", "logout"},
		{http.MethodPost, "/api/v1/auth/password/forgot", "forgot"},
		{http.MethodPost, "/api/v1/auth/password/reset", "reset"},
		{http.MethodPost, "/api/v1/auth/password/change", "change"},
		{http.MethodGet, "/api/v1/auth/sessions", "sessions"},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest(caso.metodo, caso.ruta, nil))
			if rec.Code != http.StatusOK {
				t.Fatalf("%s %s: esperado 200, obtenido %d", caso.metodo, caso.ruta, rec.Code)
			}
			if ruta := rec.Header().Get("X-Ruta"); ruta != caso.nombre {
				t.Fatalf("%s %s: handler inesperado %q", caso.metodo, caso.ruta, ruta)
			}
		})
	}
}
