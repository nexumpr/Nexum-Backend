package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPrivacyNoticeHandlerPublico200(t *testing.T) {
	handler := NewPrivacyNoticeHandler(nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/privacy/notices/current", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("se esperaba 200, obtuvo %d", rec.Code)
	}
	if got := rec.Header().Get("Cache-Control"); got != "public, max-age=300" {
		t.Fatalf("Cache-Control inesperado: %q", got)
	}
	var cuerpo struct {
		Version            string `json:"version"`
		EffectiveAt        string `json:"effectiveAt"`
		Text               string `json:"text"`
		URL                string `json:"url"`
		Content            string `json:"content"`
		RequiresAcceptance bool   `json:"requiresAcceptance"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &cuerpo); err != nil {
		t.Fatalf("JSON invalido: %v", err)
	}
	if cuerpo.Version != "2026-11-v2" || cuerpo.EffectiveAt == "" || cuerpo.Text == "" || cuerpo.URL == "" || cuerpo.Content == "" {
		t.Fatalf("respuesta incompleta: %+v", cuerpo)
	}
	if !cuerpo.RequiresAcceptance {
		t.Fatal("requiresAcceptance debe ser true")
	}
}

func TestPrivacyNoticeHandlerSinAuth(t *testing.T) {
	handler := NewPrivacyNoticeHandler(nil)
	rec := httptest.NewRecorder()
	// Sin cabecera Authorization: la ruta es pública y debe servir igual.
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/privacy/notices/current", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("la ruta publica no debe exigir JWT, obtuvo %d", rec.Code)
	}
}
