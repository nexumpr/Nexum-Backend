package http

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	appPrivacy "github.com/nexumpr/Nexum-Backend/internal/application/privacy"
)

type proveedorAvisoFalso struct {
	aviso appPrivacy.Notice
	err   error
}

func (p proveedorAvisoFalso) Current() (appPrivacy.Notice, error) { return p.aviso, p.err }

func avisoValido() appPrivacy.Notice {
	return appPrivacy.Notice{
		Version:            "2026-11-v2",
		EffectiveAt:        time.Date(2026, 11, 18, 0, 0, 0, 0, time.UTC),
		Text:               "resumen",
		URL:                "https://d2lpmqy5w6x9es.cloudfront.net/api/v1/privacy/notices/current",
		Content:            "contenido",
		RequiresAcceptance: true,
	}
}

func TestPrivacyNoticeHandlerPublico200(t *testing.T) {
	handler := NewPrivacyNoticeHandler(proveedorAvisoFalso{aviso: avisoValido()}, nil)
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

func TestPrivacyNoticeHandlerSinFechaFallaCerrado(t *testing.T) {
	handler := NewPrivacyNoticeHandler(proveedorAvisoFalso{err: appPrivacy.ErrFechaEfectivaNoAprobada}, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/privacy/notices/current", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("sin fecha aprobada debe responder 503, obtuvo %d", rec.Code)
	}
}

func TestPrivacyNoticeHandlerErrorProveedor(t *testing.T) {
	handler := NewPrivacyNoticeHandler(proveedorAvisoFalso{err: errors.New("fallo")}, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/privacy/notices/current", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("un error no clasificado debe ser 500, obtuvo %d", rec.Code)
	}
}

func TestPrivacyNoticeHandlerSinAuth(t *testing.T) {
	handler := NewPrivacyNoticeHandler(proveedorAvisoFalso{aviso: avisoValido()}, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/privacy/notices/current", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("la ruta publica no debe exigir JWT, obtuvo %d", rec.Code)
	}
}
