package main

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// e007Router registra las rutas montadas sin levantar el servidor: permite comprobar que el
// montaje de E007 existe y que falla cerrado cuando faltan dependencias.
type e007Router struct {
	rutas map[string]http.HandlerFunc
}

func newE007Router() *e007Router {
	return &e007Router{rutas: make(map[string]http.HandlerFunc)}
}

func (r *e007Router) HandleFunc(method, path string, handler http.HandlerFunc) {
	r.rutas[method+" "+path] = handler
}

func e007Logger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(io.Discard, nil))
}

// TestE007MontajeFallaCerradoSinDependencias garantiza que el montaje registra la ruta
// contractual y que, sin DynamoDB, Redis ni material de firma, responde 503 en lugar de
// aceptar un cambio de contraseña que no podría persistir.
func TestE007MontajeFallaCerradoSinDependencias(t *testing.T) {
	router := newE007Router()
	montarE007(router, nil, nil, "", e007Logger())

	handler, ok := router.rutas["POST /auth/password/change"]
	if !ok {
		t.Fatalf("E007 debe registrarse en POST /auth/password/change; rutas=%v", router.rutas)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/password/change",
		strings.NewReader(`{"currentPassword":"OldPassphrase#2026","newPassword":"NewPassphrase#2026"}`))
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	handler(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("sin dependencias debe responder 503, obtenido %d (%s)", rec.Code, rec.Body.String())
	}
	if cuerpo := rec.Body.String(); !strings.Contains(cuerpo, "DEPENDENCY_UNAVAILABLE") {
		t.Fatalf("el codigo debe ser DEPENDENCY_UNAVAILABLE, obtenido %q", cuerpo)
	}
}
