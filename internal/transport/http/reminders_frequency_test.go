package http

import (
	"net/http"
	"strings"
	"testing"
)

// TestE065E066FrecuenciaInvalidaYDistincionAusenteNull cubre el borde HTTP del enum
// cerrado de `frequency`: «hourly» es una solicitud inválida (400 INVALID_REQUEST, el
// código que declara el contrato para un enum fuera de rango), omitirlo conserva la
// recurrencia y `null` la cancela.
func TestE065E066FrecuenciaInvalidaYDistincionAusenteNull(t *testing.T) {
	svc := o5rServicio(t)
	crear := o5rHandler(http.MethodPost, svc)
	patch := o5rHandler(http.MethodPatch, svc)
	headers := map[string]string{HeaderIdempotencyKey: "clave-frecuencia-http-01"}

	invalida := `{"title":"Pagar luz","dueAt":"2026-11-24T10:00:00Z","timezone":"America/Mexico_City","frequency":"hourly"}`
	rec := o5rEjecutar(crear, o5rRequest(http.MethodPost, "/api/v1/reminders", "o5r-token-a", invalida, headers))
	if rec.Code != http.StatusBadRequest || o5rCodigo(t, rec) != "INVALID_REQUEST" {
		t.Fatalf("creación con hourly: esperado 400 INVALID_REQUEST, obtenido %d %s", rec.Code, rec.Body.String())
	}

	valida := `{"title":"Pagar luz","dueAt":"2026-11-24T10:00:00Z","timezone":"America/Mexico_City","frequency":"weekly"}`
	rec = o5rEjecutar(crear, o5rRequest(http.MethodPost, "/api/v1/reminders", "o5r-token-a", valida, headers))
	if rec.Code != http.StatusCreated {
		t.Fatalf("creación válida: %d %s", rec.Code, rec.Body.String())
	}
	cuerpo := o5rBody(t, rec)
	id, _ := cuerpo["id"].(string)
	if cuerpo["frequency"] != "weekly" {
		t.Fatalf("frequency inicial: %s", rec.Body.String())
	}
	ruta := "/api/v1/reminders/" + id

	rec = o5rEjecutar(patch, o5rRequest(http.MethodPatch, ruta, "o5r-token-a", `{"frequency":"hourly"}`,
		map[string]string{"If-Match": `"v1"`}))
	if rec.Code != http.StatusBadRequest || o5rCodigo(t, rec) != "INVALID_REQUEST" {
		t.Fatalf("parche con hourly: esperado 400 INVALID_REQUEST, obtenido %d %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "hourly") {
		t.Fatalf("el error no debe repetir el valor del enum: %s", rec.Body.String())
	}

	// La versión no cambió: el mismo If-Match sigue siendo válido y la frecuencia se conserva.
	rec = o5rEjecutar(patch, o5rRequest(http.MethodPatch, ruta, "o5r-token-a", `{"title":"Pagar luz de enero"}`,
		map[string]string{"If-Match": `"v1"`}))
	if rec.Code != http.StatusOK {
		t.Fatalf("parche sin frequency: %d %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("ETag") != `"v2"` {
		t.Fatalf("ETag tras parche válido: %q", rec.Header().Get("ETag"))
	}
	if o5rBody(t, rec)["frequency"] != "weekly" {
		t.Fatalf("omitir frequency debe conservar la recurrencia: %s", rec.Body.String())
	}

	rec = o5rEjecutar(patch, o5rRequest(http.MethodPatch, ruta, "o5r-token-a", `{"frequency":null}`,
		map[string]string{"If-Match": `"v2"`}))
	if rec.Code != http.StatusOK {
		t.Fatalf("parche frequency:null: %d %s", rec.Code, rec.Body.String())
	}
	if _, presente := o5rBody(t, rec)["frequency"]; presente {
		t.Fatalf("frequency:null debe cancelar la recurrencia: %s", rec.Body.String())
	}
	if rec.Header().Get("ETag") != `"v3"` {
		t.Fatalf("ETag tras cancelar: %q", rec.Header().Get("ETag"))
	}

	// `null` en creación no es una cadencia válida: no se admite para no confundir
	// «sin recurrencia» (campo omitido) con un valor nulo explícito.
	rec = o5rEjecutar(crear, o5rRequest(http.MethodPost, "/api/v1/reminders", "o5r-token-a",
		`{"title":"Pagar luz","dueAt":"2026-11-24T10:00:00Z","timezone":"America/Mexico_City","frequency":null}`,
		map[string]string{HeaderIdempotencyKey: "clave-frecuencia-http-02"}))
	if rec.Code != http.StatusBadRequest || o5rCodigo(t, rec) != "INVALID_REQUEST" {
		t.Fatalf("creación con frequency:null: esperado 400 INVALID_REQUEST, obtenido %d %s", rec.Code, rec.Body.String())
	}
}
