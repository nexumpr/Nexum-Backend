package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	appBootstrap "github.com/nexumpr/Nexum-Backend/internal/application/bootstrap"
	appSummary "github.com/nexumpr/Nexum-Backend/internal/application/financialsummary"
	appProfile "github.com/nexumpr/Nexum-Backend/internal/application/profile"
)

type o5bProfiles struct{ err error }

func (f *o5bProfiles) Get(_ context.Context, userID string) (*appProfile.Profile, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &appProfile.Profile{
		ID: userID, Email: userID + "@example.invalid", DisplayName: "Usuario " + userID,
		EmailVerified: true, CreatedAt: time.Date(2026, 11, 5, 17, 0, 0, 0, time.UTC), Version: 1,
	}, nil
}

type o5bSummaries struct{ err error }

func (f *o5bSummaries) Summary(_ context.Context, userID, _ string) (*appSummary.Summary, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &appSummary.Summary{
		Period: "2026-11", Currency: "MXN", Income: 100000, Expenses: 20000,
		Savings: 30000, Net: 50000, AsOf: time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC),
	}, nil
}

func o5bHandler(profiles *o5bProfiles, summaries *o5bSummaries) http.Handler {
	svc := appBootstrap.NewService(profiles, summaries, "2026-11")
	return NewBootstrapHandler(svc, o5rTokens{}, o5rAuthorizer{}, nil, nil)
}

func TestE021RespuestaExactaYCabecera(t *testing.T) {
	h := o5bHandler(&o5bProfiles{}, &o5bSummaries{})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/bootstrap", nil)
	req.Header.Set("Authorization", "Bearer o5r-token-a")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("esperado 200, obtenido %d (%s)", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Cache-Control"); got != "private, no-store" {
		t.Fatalf("Cache-Control: %q", got)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("JSON invalido: %v", err)
	}
	for _, clave := range []string{"profile", "financialSummary", "catalogVersion"} {
		if _, ok := body[clave]; !ok {
			t.Fatalf("falta %s: %s", clave, rec.Body.String())
		}
	}
	if body["catalogVersion"] != "2026-11" {
		t.Fatalf("catalogVersion: %v", body["catalogVersion"])
	}
	perfil := body["profile"].(map[string]any)
	for _, clave := range []string{"id", "email", "displayName", "emailVerified", "createdAt", "version"} {
		if _, ok := perfil[clave]; !ok {
			t.Fatalf("falta profile.%s", clave)
		}
	}
	if _, ok := perfil["passwordHash"]; ok {
		t.Fatal("no se debe exponer passwordHash")
	}
}

func TestE021AislamientoYFallosControlados(t *testing.T) {
	h := o5bHandler(&o5bProfiles{}, &o5bSummaries{})
	for _, caso := range []struct {
		token string
		user  string
	}{
		{"o5r-token-a", "user-a"},
		{"o5r-token-b", "user-b"},
	} {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/bootstrap", nil)
		req.Header.Set("Authorization", "Bearer "+caso.token)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: %d %s", caso.token, rec.Code, rec.Body.String())
		}
		var body map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &body)
		if body["profile"].(map[string]any)["id"] != caso.user {
			t.Fatalf("el perfil debe corresponder al titular: %s", rec.Body.String())
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/bootstrap", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("sin JWT esperado 401, obtenido %d", rec.Code)
	}

	perfiles := &o5bProfiles{err: appProfile.ErrProfileUnavailable}
	rec = httptest.NewRecorder()
	o5bHandler(perfiles, &o5bSummaries{}).ServeHTTP(rec, func() *http.Request {
		r := httptest.NewRequest(http.MethodGet, "/api/v1/bootstrap", nil)
		r.Header.Set("Authorization", "Bearer o5r-token-a")
		return r
	}())
	if rec.Code != http.StatusServiceUnavailable || o5rCodigo(t, rec) != "DEPENDENCY_UNAVAILABLE" {
		t.Fatalf("fallo de persistencia: %d %s", rec.Code, rec.Body.String())
	}
}
