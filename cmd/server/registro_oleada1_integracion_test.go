package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	nexumhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
)

// TestIntegracionOleada1QuinceRutas es la prueba de ensayo conjunto: exige que el
// registro global contenga exactamente los 15 endpoints autorizados, sin
// duplicados, y que el router los resuelva (no 404). No se incluye en el
// andamiaje ni en PR individuales.
func TestIntegracionOleada1QuinceRutas(t *testing.T) {
	esperados := []string{
		"E012", "E013", "E015", "E016", "E017", "E018", "E019",
		"E022", "E023", "E024", "E025", "E068", "E069", "E070", "E071",
	}
	if len(registroOleada1Global.porID) != len(esperados) {
		t.Fatalf("se esperaban %d endpoints registrados, hay %d", len(esperados), len(registroOleada1Global.porID))
	}
	vistos := make(map[string]bool, len(esperados))
	for _, id := range esperados {
		m, existe := registroOleada1Global.porID[id]
		if !existe {
			t.Fatalf("falta el endpoint %s", id)
		}
		if m.Montar == nil || m.Metodo == "" || m.Ruta == "" {
			t.Fatalf("%s tiene un montaje incompleto", id)
		}
		clave := m.Metodo + " " + m.Ruta
		if vistos[clave] {
			t.Fatalf("ruta duplicada en la integracion: %s", clave)
		}
		vistos[clave] = true
	}

	router := nexumhttp.NewRouter()
	registroOleada1Global.montar(&dependenciasOleada{Router: router})
	for _, id := range esperados {
		m := registroOleada1Global.porID[id]
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(m.Metodo, "/api/v1"+m.Ruta, nil))
		if rec.Code == http.StatusNotFound {
			t.Fatalf("%s %s no quedo conectado al router", m.Metodo, m.Ruta)
		}
	}
}
