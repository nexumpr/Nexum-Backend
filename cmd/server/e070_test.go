package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	nexumhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
)

// Prueba de composicion de E070: la ruta queda registrada en el registro cerrado de
// la Oleada 1 y se sirve sin autenticacion, con cache publica y diez preguntas.

func TestE070RegistradoEnLaOleada1(t *testing.T) {
	montaje, existe := registroOleada1Global.porID["E070"]
	if !existe {
		t.Fatal("E070 debe estar registrado por init")
	}
	if montaje.Metodo != http.MethodGet {
		t.Fatalf("metodo inesperado: %s", montaje.Metodo)
	}
	if montaje.Ruta != "/help/faq" {
		t.Fatalf("ruta inesperada: %s", montaje.Ruta)
	}
	if montaje.Montar == nil {
		t.Fatal("el montaje de E070 no puede ser nulo")
	}
}

func TestE070RespondeSinAutenticacion(t *testing.T) {
	router := nexumhttp.NewRouter()
	registroOleada1Global.montar(&dependenciasOleada{Router: router})

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/help/faq", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("la ruta publica debe responder 200 sin Bearer, respondio %d", rec.Code)
	}
	if rec.Header().Get("Cache-Control") != "public, max-age=300" {
		t.Fatalf("Cache-Control inesperado: %q", rec.Header().Get("Cache-Control"))
	}

	var pagina struct {
		Items []struct {
			ID       string `json:"id"`
			Question string `json:"question"`
			Answer   string `json:"answer"`
		} `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &pagina); err != nil {
		t.Fatalf("respuesta no valida: %v", err)
	}
	if len(pagina.Items) != 10 {
		t.Fatalf("se esperaban 10 preguntas, hay %d", len(pagina.Items))
	}
	for _, item := range pagina.Items {
		if item.ID == "" || item.Question == "" || item.Answer == "" {
			t.Fatalf("entrada incompleta: %+v", item)
		}
	}
}
