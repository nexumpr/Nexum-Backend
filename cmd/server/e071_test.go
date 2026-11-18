package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	nexumhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
)

func TestE071RegistradoEnLaOleada1(t *testing.T) {
	montaje, existe := registroOleada1Global.porID["E071"]
	if !existe {
		t.Fatal("E071 debe estar registrado por init")
	}
	if montaje.Metodo != http.MethodGet || montaje.Ruta != "/education/tips" {
		t.Fatalf("registro inesperado: %s %s", montaje.Metodo, montaje.Ruta)
	}
	if montaje.Montar == nil {
		t.Fatal("el montaje de E071 no puede ser nulo")
	}
}

func TestE071RespondeSinAutenticacion(t *testing.T) {
	router := nexumhttp.NewRouter()
	registroOleada1Global.montar(&dependenciasOleada{Router: router})

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/education/tips", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("la ruta publica debe responder 200 sin Bearer, respondio %d", rec.Code)
	}
	if rec.Header().Get("Cache-Control") != "public, max-age=300" {
		t.Fatalf("Cache-Control inesperado: %q", rec.Header().Get("Cache-Control"))
	}

	var pagina struct {
		Items []struct {
			ID    string `json:"id"`
			Title string `json:"title"`
			Text  string `json:"text"`
		} `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &pagina); err != nil {
		t.Fatalf("respuesta no valida: %v", err)
	}
	if len(pagina.Items) != 8 {
		t.Fatalf("se esperaban 8 consejos, hay %d", len(pagina.Items))
	}
	for _, item := range pagina.Items {
		if item.ID == "" || item.Title == "" || item.Text == "" {
			t.Fatalf("entrada incompleta: %+v", item)
		}
	}
}
