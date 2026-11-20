package transport_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	appContrib "github.com/nexumpr/Nexum-Backend/internal/application/contributions"
	internalhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
)

// Pruebas HTTP de E046 (`GET /api/v1/goal-contributions/{contributionId}`): contrato exacto,
// resolución por localizador, no enumeración entre usuarios y fallo cerrado.

// TestE046HTTPGetContratoExacto cubre T01: el cuerpo tiene exactamente los siete campos del
// objeto `Contribution`, con importes decimales string y marca de tiempo UTC.
func TestE046HTTPGetContratoExacto(t *testing.T) {
	store := nuevoStoreAportacionesE04x()
	ocurrida := time.Date(2026, 11, 19, 9, 0, 0, 0, time.UTC)
	store.sembrar(e004HTTPUsuario, e045AporteA, appContrib.StatusActive, e045Meta, 60000, ocurrida)
	handler := internalhttp.NewContributionGetHandler(servicioAportacionesE04x(store), e004TokensValidos(), newE004Sesiones(), nil, silencioso())

	req := e004Peticion(http.MethodGet, "/api/v1/goal-contributions/"+e045AporteA, "jwt-valido", "")
	req.SetPathValue("contributionId", e045AporteA)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("esperado 200, obtenido %d: %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatalf("Cache-Control inesperado: %q", rec.Header().Get("Cache-Control"))
	}

	var cuerpo map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &cuerpo); err != nil {
		t.Fatalf("json: %v", err)
	}
	if len(cuerpo) != 7 {
		t.Fatalf("se esperaban exactamente 7 campos, obtenidos %d: %s", len(cuerpo), rec.Body.String())
	}
	for _, campo := range []string{"id", "amount", "currency", "allocations", "occurredAt", "status", "version"} {
		if _, ok := cuerpo[campo]; !ok {
			t.Fatalf("falta el campo obligatorio %q: %s", campo, rec.Body.String())
		}
	}
	if cuerpo["id"] != e045AporteA || cuerpo["amount"] != "600.00" || cuerpo["currency"] != "MXN" {
		t.Fatalf("campos fuera de contrato: %s", rec.Body.String())
	}
	if cuerpo["occurredAt"] != "2026-11-19T09:00:00.000Z" || cuerpo["status"] != appContrib.StatusActive || cuerpo["version"] != float64(1) {
		t.Fatalf("estado o instante fuera de contrato: %s", rec.Body.String())
	}
	asignaciones, ok := cuerpo["allocations"].([]any)
	if !ok || len(asignaciones) != 1 {
		t.Fatalf("allocations fuera de contrato: %s", rec.Body.String())
	}
	primera, ok := asignaciones[0].(map[string]any)
	if !ok || primera["goalId"] != e045Meta || primera["amount"] != "600.00" {
		t.Fatalf("asignacion fuera de contrato: %s", rec.Body.String())
	}
}

// TestE046HTTPNoEnumeracionEntreUsuarios cubre T03: un recurso ajeno y uno inexistente producen
// el mismo 404 con el mismo cuerpo, de modo que no se filtra la existencia del recurso.
func TestE046HTTPNoEnumeracionEntreUsuarios(t *testing.T) {
	store := nuevoStoreAportacionesE04x()
	store.sembrar(e004HTTPUsuario, e045AporteA, appContrib.StatusActive, e045Meta, 60000, e045Ahora)
	handler := internalhttp.NewContributionGetHandler(servicioAportacionesE04x(store), e004TokensValidos(), newE004Sesiones(), nil, silencioso())

	ajena := e004Peticion(http.MethodGet, "/api/v1/goal-contributions/"+e045AporteA, "jwt-sujeto-ajeno", "")
	ajena.SetPathValue("contributionId", e045AporteA)
	recAjena := httptest.NewRecorder()
	handler.ServeHTTP(recAjena, ajena)
	if recAjena.Code != http.StatusNotFound {
		t.Fatalf("recurso ajeno: esperado 404, obtenido %d: %s", recAjena.Code, recAjena.Body.String())
	}

	inexistente := e004Peticion(http.MethodGet, "/api/v1/goal-contributions/"+e045AporteB, "jwt-valido", "")
	inexistente.SetPathValue("contributionId", e045AporteB)
	recInexistente := httptest.NewRecorder()
	handler.ServeHTTP(recInexistente, inexistente)
	if recInexistente.Code != http.StatusNotFound {
		t.Fatalf("recurso inexistente: esperado 404, obtenido %d", recInexistente.Code)
	}

	var cuerpoAjeno, cuerpoInexistente map[string]any
	if err := json.Unmarshal(recAjena.Body.Bytes(), &cuerpoAjeno); err != nil {
		t.Fatalf("json ajeno: %v", err)
	}
	if err := json.Unmarshal(recInexistente.Body.Bytes(), &cuerpoInexistente); err != nil {
		t.Fatalf("json inexistente: %v", err)
	}
	errorAjeno, _ := cuerpoAjeno["error"].(map[string]any)
	errorInexistente, _ := cuerpoInexistente["error"].(map[string]any)
	if errorAjeno == nil || errorInexistente == nil {
		t.Fatalf("respuesta sin envoltorio de error: ajeno=%s inexistente=%s", recAjena.Body.String(), recInexistente.Body.String())
	}
	if errorAjeno["code"] != errorInexistente["code"] || errorAjeno["message"] != errorInexistente["message"] {
		t.Fatalf("el 404 debe ser identico en ambos casos: ajeno=%v inexistente=%v", errorAjeno, errorInexistente)
	}
}

// TestE046HTTPValidacionYFalloCerrado cubre identificador inválido, autenticación, servicio
// ausente y almacén caído.
func TestE046HTTPValidacionYFalloCerrado(t *testing.T) {
	store := nuevoStoreAportacionesE04x()
	store.sembrar(e004HTTPUsuario, e045AporteA, appContrib.StatusActive, e045Meta, 60000, e045Ahora)
	sesiones := newE004Sesiones()
	handler := internalhttp.NewContributionGetHandler(servicioAportacionesE04x(store), e004TokensValidos(), sesiones, nil, silencioso())

	invalido := e004Peticion(http.MethodGet, "/api/v1/goal-contributions/no-es-uuid", "jwt-valido", "")
	invalido.SetPathValue("contributionId", "no-es-uuid")
	recInvalido := httptest.NewRecorder()
	handler.ServeHTTP(recInvalido, invalido)
	if recInvalido.Code != http.StatusBadRequest {
		t.Fatalf("identificador invalido: esperado 400, obtenido %d", recInvalido.Code)
	}

	sinBearer := e004Peticion(http.MethodGet, "/api/v1/goal-contributions/"+e045AporteA, "", "")
	sinBearer.SetPathValue("contributionId", e045AporteA)
	recSinBearer := httptest.NewRecorder()
	handler.ServeHTTP(recSinBearer, sinBearer)
	if recSinBearer.Code != http.StatusUnauthorized {
		t.Fatalf("sin Bearer: esperado 401, obtenido %d", recSinBearer.Code)
	}

	sesiones.revocar(e004HTTPSesion)
	revocada := e004Peticion(http.MethodGet, "/api/v1/goal-contributions/"+e045AporteA, "jwt-valido", "")
	revocada.SetPathValue("contributionId", e045AporteA)
	recRevocada := httptest.NewRecorder()
	handler.ServeHTTP(recRevocada, revocada)
	if recRevocada.Code != http.StatusUnauthorized {
		t.Fatalf("sesion revocada: esperado 401, obtenido %d", recRevocada.Code)
	}

	sinServicio := internalhttp.NewContributionGetHandler(nil, e004TokensValidos(), newE004Sesiones(), nil, silencioso())
	peticionSinServicio := e004Peticion(http.MethodGet, "/api/v1/goal-contributions/"+e045AporteA, "jwt-valido", "")
	peticionSinServicio.SetPathValue("contributionId", e045AporteA)
	recSinServicio := httptest.NewRecorder()
	sinServicio.ServeHTTP(recSinServicio, peticionSinServicio)
	if recSinServicio.Code != http.StatusServiceUnavailable {
		t.Fatalf("sin servicio: esperado 503, obtenido %d", recSinServicio.Code)
	}

	store.fallo = appContrib.ErrUnavailable
	handlerFallo := internalhttp.NewContributionGetHandler(servicioAportacionesE04x(store), e004TokensValidos(), newE004Sesiones(), nil, silencioso())
	peticionFallo := e004Peticion(http.MethodGet, "/api/v1/goal-contributions/"+e045AporteA, "jwt-valido", "")
	peticionFallo.SetPathValue("contributionId", e045AporteA)
	recFallo := httptest.NewRecorder()
	handlerFallo.ServeHTTP(recFallo, peticionFallo)
	if recFallo.Code != http.StatusServiceUnavailable {
		t.Fatalf("almacen caido: esperado 503, obtenido %d", recFallo.Code)
	}
}
