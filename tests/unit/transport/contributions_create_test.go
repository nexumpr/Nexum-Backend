package transport_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	appContrib "github.com/nexumpr/Nexum-Backend/internal/application/contributions"
	internalhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
)

// Pruebas HTTP de E044 (`POST /api/v1/goal-contributions`). Reutilizan el doble con estado de
// `contributions_preview_test.go` y la cadena real Bearer + sesion. La atomicidad real la
// cubren las pruebas del adaptador; aqui se verifica el contrato HTTP, la idempotencia
// observable y que un rechazo no escribe.

const e044HTTPCuerpo = `{"amount":"600.00","currency":"MXN","occurredAt":"2026-11-19T10:00:00Z","allocations":[{"goalId":"` + e04xHTTPMeta + `","amount":"600.00"}]}`

func TestE044HTTPCreateContrato(t *testing.T) {
	store := nuevoStoreAportacionesHTTP()
	h := internalhttp.NewContributionCreateHandler(servicioAportacionesHTTP(store), e004TokensValidos(), newE004Sesiones(), nil, silencioso())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, peticionAportaciones(http.MethodPost, "/api/v1/goal-contributions", "jwt-valido", e044HTTPCuerpo, e04xHTTPClave))

	if rec.Code != http.StatusCreated {
		t.Fatalf("esperado 201, obtenido %d: %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Location") != "/api/v1/goal-contributions/"+e04xHTTPAporte {
		t.Fatalf("Location inesperado: %q", rec.Header().Get("Location"))
	}
	if rec.Header().Get("X-Idempotent-Replay") != "" {
		t.Fatalf("una creacion nueva no debe marcar replay: %q", rec.Header().Get("X-Idempotent-Replay"))
	}
	if rec.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatalf("Cache-Control inesperado: %q", rec.Header().Get("Cache-Control"))
	}
	var body struct {
		ID          string `json:"id"`
		Amount      string `json:"amount"`
		Currency    string `json:"currency"`
		Allocations []struct {
			GoalID string `json:"goalId"`
			Amount string `json:"amount"`
		} `json:"allocations"`
		OccurredAt string `json:"occurredAt"`
		Status     string `json:"status"`
		Version    int64  `json:"version"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("json: %v (%s)", err, rec.Body.String())
	}
	if body.ID != e04xHTTPAporte || body.Amount != "600.00" || body.Currency != "MXN" ||
		body.OccurredAt != "2026-11-19T10:00:00.000Z" || body.Status != appContrib.StatusActive || body.Version != 1 {
		t.Fatalf("cuerpo inesperado: %+v", body)
	}
	if len(body.Allocations) != 1 || body.Allocations[0].GoalID != e04xHTTPMeta || body.Allocations[0].Amount != "600.00" {
		t.Fatalf("allocations inesperadas: %+v", body.Allocations)
	}
	if store.escrituras() != 1 {
		t.Fatalf("se esperaba una escritura, hay %d", store.escrituras())
	}
}

func TestE044HTTPReplayEstableYConflicto(t *testing.T) {
	store := nuevoStoreAportacionesHTTP()
	h := internalhttp.NewContributionCreateHandler(servicioAportacionesHTTP(store), e004TokensValidos(), newE004Sesiones(), nil, silencioso())

	primera := httptest.NewRecorder()
	h.ServeHTTP(primera, peticionAportaciones(http.MethodPost, "/api/v1/goal-contributions", "jwt-valido", e044HTTPCuerpo, e04xHTTPClave))
	if primera.Code != http.StatusCreated {
		t.Fatalf("primera: %d %s", primera.Code, primera.Body.String())
	}

	replay := httptest.NewRecorder()
	h.ServeHTTP(replay, peticionAportaciones(http.MethodPost, "/api/v1/goal-contributions", "jwt-valido", e044HTTPCuerpo, e04xHTTPClave))
	if replay.Code != http.StatusCreated {
		t.Fatalf("replay: esperado 201, obtenido %d: %s", replay.Code, replay.Body.String())
	}
	if replay.Header().Get("X-Idempotent-Replay") != "true" {
		t.Fatalf("el replay debe marcar X-Idempotent-Replay: %v", replay.Header())
	}
	if replay.Body.String() != primera.Body.String() {
		t.Fatalf("el replay debe devolver el mismo cuerpo:\n%s\n%s", primera.Body.String(), replay.Body.String())
	}
	if store.escrituras() != 1 {
		t.Fatalf("el replay no debe escribir de nuevo; escrituras=%d", store.escrituras())
	}

	otroCuerpo := `{"amount":"300.00","currency":"MXN","occurredAt":"2026-11-19T10:00:00Z","allocations":[{"goalId":"` + e04xHTTPMeta + `","amount":"300.00"}]}`
	conflicto := httptest.NewRecorder()
	h.ServeHTTP(conflicto, peticionAportaciones(http.MethodPost, "/api/v1/goal-contributions", "jwt-valido", otroCuerpo, e04xHTTPClave))
	if conflicto.Code != http.StatusConflict {
		t.Fatalf("misma clave con otro payload: esperado 409, obtenido %d: %s", conflicto.Code, conflicto.Body.String())
	}
	if codigo := codigoDeError(t, conflicto); codigo != "CONFLICT" {
		t.Fatalf("codigo esperado CONFLICT, obtenido %s", codigo)
	}
	if store.escrituras() != 1 {
		t.Fatalf("un conflicto no debe escribir; escrituras=%d", store.escrituras())
	}
}

func TestE044HTTPIdempotencyKeyObligatoria(t *testing.T) {
	casos := []struct {
		nombre string
		clave  string
	}{
		{"ausente", ""},
		{"demasiado corta", "corta"},
		{"demasiado larga", strings.Repeat("k", 129)},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			store := nuevoStoreAportacionesHTTP()
			h := internalhttp.NewContributionCreateHandler(servicioAportacionesHTTP(store), e004TokensValidos(), newE004Sesiones(), nil, silencioso())
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, peticionAportaciones(http.MethodPost, "/api/v1/goal-contributions", "jwt-valido", e044HTTPCuerpo, caso.clave))
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("esperado 400, obtenido %d: %s", rec.Code, rec.Body.String())
			}
			if codigo := codigoDeError(t, rec); codigo != "INVALID_REQUEST" {
				t.Fatalf("codigo esperado INVALID_REQUEST, obtenido %s", codigo)
			}
			if store.escrituras() != 0 {
				t.Fatalf("sin clave valida no debe escribirse; escrituras=%d", store.escrituras())
			}
		})
	}
}

func TestE044HTTPAutenticacionYSesionRevocada(t *testing.T) {
	casos := []struct {
		nombre string
		token  string
	}{
		{"sin bearer", ""},
		{"token desconocido", "desconocido"},
		{"token expirado", "jwt-expirado"},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			store := nuevoStoreAportacionesHTTP()
			h := internalhttp.NewContributionCreateHandler(servicioAportacionesHTTP(store), e004TokensValidos(), newE004Sesiones(), nil, silencioso())
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, peticionAportaciones(http.MethodPost, "/api/v1/goal-contributions", caso.token, e044HTTPCuerpo, e04xHTTPClave))
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("esperado 401, obtenido %d: %s", rec.Code, rec.Body.String())
			}
			if store.escrituras() != 0 {
				t.Fatalf("sin sesion valida no debe escribirse; escrituras=%d", store.escrituras())
			}
		})
	}

	store := nuevoStoreAportacionesHTTP()
	sesiones := newE004Sesiones()
	h := internalhttp.NewContributionCreateHandler(servicioAportacionesHTTP(store), e004TokensValidos(), sesiones, nil, silencioso())
	sesiones.revocar("018f90ef-08e5-7a68-b09f-0000000000c1")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, peticionAportaciones(http.MethodPost, "/api/v1/goal-contributions", "jwt-valido", e044HTTPCuerpo, e04xHTTPClave))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("sesion revocada: esperado 401, obtenido %d", rec.Code)
	}
	if store.escrituras() != 0 {
		t.Fatalf("una sesion revocada no debe escribir; escrituras=%d", store.escrituras())
	}
}

func TestE044HTTPValidacionesDeNegocio(t *testing.T) {
	casos := []struct {
		nombre   string
		cuerpo   string
		esperado int
		codigo   string
		preparar func(*storeAportacionesHTTP)
	}{
		{
			nombre:   "suma incompleta",
			cuerpo:   `{"amount":"1000.00","currency":"MXN","occurredAt":"2026-11-19T10:00:00Z","allocations":[{"goalId":"` + e04xHTTPMeta + `","amount":"600.00"}]}`,
			esperado: http.StatusUnprocessableEntity,
			codigo:   "BUSINESS_RULE_VIOLATION",
		},
		{
			nombre: "meta repetida",
			cuerpo: `{"amount":"600.00","currency":"MXN","occurredAt":"2026-11-19T10:00:00Z","allocations":[` +
				`{"goalId":"` + e04xHTTPMeta + `","amount":"300.00"},` +
				`{"goalId":"` + e04xHTTPMeta + `","amount":"300.00"}]}`,
			esperado: http.StatusUnprocessableEntity,
			codigo:   "BUSINESS_RULE_VIOLATION",
		},
		{
			nombre:   "divisa no soportada",
			cuerpo:   `{"amount":"600.00","currency":"USD","occurredAt":"2026-11-19T10:00:00Z","allocations":[{"goalId":"` + e04xHTTPMeta + `","amount":"600.00"}]}`,
			esperado: http.StatusUnprocessableEntity,
			codigo:   "BUSINESS_RULE_VIOLATION",
		},
		{
			nombre:   "fecha futura",
			cuerpo:   `{"amount":"600.00","currency":"MXN","occurredAt":"2026-11-21T00:00:00Z","allocations":[{"goalId":"` + e04xHTTPMeta + `","amount":"600.00"}]}`,
			esperado: http.StatusUnprocessableEntity,
			codigo:   "BUSINESS_RULE_VIOLATION",
		},
		{
			nombre:   "meta inexistente",
			cuerpo:   `{"amount":"600.00","currency":"MXN","occurredAt":"2026-11-19T10:00:00Z","allocations":[{"goalId":"018f90ef-08e5-7a68-b09f-2b67a47c92ff","amount":"600.00"}]}`,
			esperado: http.StatusNotFound,
			codigo:   "NOT_FOUND",
		},
		{
			nombre:   "meta inactiva",
			cuerpo:   e044HTTPCuerpo,
			esperado: http.StatusUnprocessableEntity,
			codigo:   "BUSINESS_RULE_VIOLATION",
			preparar: func(s *storeAportacionesHTTP) { s.goalStatus = "paused" },
		},
		{
			nombre:   "importe sin decimales",
			cuerpo:   `{"amount":"600","currency":"MXN","occurredAt":"2026-11-19T10:00:00Z","allocations":[{"goalId":"` + e04xHTTPMeta + `","amount":"600.00"}]}`,
			esperado: http.StatusBadRequest,
			codigo:   "INVALID_REQUEST",
		},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			store := nuevoStoreAportacionesHTTP()
			if caso.preparar != nil {
				caso.preparar(store)
			}
			h := internalhttp.NewContributionCreateHandler(servicioAportacionesHTTP(store), e004TokensValidos(), newE004Sesiones(), nil, silencioso())
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, peticionAportaciones(http.MethodPost, "/api/v1/goal-contributions", "jwt-valido", caso.cuerpo, e04xHTTPClave))
			if rec.Code != caso.esperado {
				t.Fatalf("esperado %d, obtenido %d: %s", caso.esperado, rec.Code, rec.Body.String())
			}
			if codigo := codigoDeError(t, rec); codigo != caso.codigo {
				t.Fatalf("codigo esperado %s, obtenido %s", caso.codigo, codigo)
			}
			if store.escrituras() != 0 {
				t.Fatalf("un rechazo no debe escribir; escrituras=%d", store.escrituras())
			}
		})
	}
}

func TestE044HTTPSolicitudMalformada(t *testing.T) {
	casos := []struct {
		nombre string
		cuerpo string
	}{
		{"json malformado", `{"amount":`},
		{"campo desconocido", `{"amount":"600.00","currency":"MXN","occurredAt":"2026-11-19T10:00:00Z","allocations":[{"goalId":"` + e04xHTTPMeta + `","amount":"600.00"}],"extra":true}`},
		{"cuerpo excesivo", `{"amount":"600.00","currency":"MXN","padding":"` + strings.Repeat("x", 70<<10) + `"}`},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			store := nuevoStoreAportacionesHTTP()
			h := internalhttp.NewContributionCreateHandler(servicioAportacionesHTTP(store), e004TokensValidos(), newE004Sesiones(), nil, silencioso())
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, peticionAportaciones(http.MethodPost, "/api/v1/goal-contributions", "jwt-valido", caso.cuerpo, e04xHTTPClave))
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("esperado 400, obtenido %d: %s", rec.Code, rec.Body.String())
			}
			if store.escrituras() != 0 {
				t.Fatalf("una solicitud invalida no debe escribir; escrituras=%d", store.escrituras())
			}
		})
	}
}

func TestE044HTTP503YCuota(t *testing.T) {
	store := nuevoStoreAportacionesHTTP()
	store.failCreate = appContrib.ErrUnavailable
	h := internalhttp.NewContributionCreateHandler(servicioAportacionesHTTP(store), e004TokensValidos(), newE004Sesiones(), nil, silencioso())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, peticionAportaciones(http.MethodPost, "/api/v1/goal-contributions", "jwt-valido", e044HTTPCuerpo, e04xHTTPClave))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("esperado 503, obtenido %d: %s", rec.Code, rec.Body.String())
	}
	if codigo := codigoDeError(t, rec); codigo != "DEPENDENCY_UNAVAILABLE" {
		t.Fatalf("codigo esperado DEPENDENCY_UNAVAILABLE, obtenido %s", codigo)
	}

	sinServicio := internalhttp.NewContributionCreateHandler(nil, e004TokensValidos(), newE004Sesiones(), nil, silencioso())
	recSin := httptest.NewRecorder()
	sinServicio.ServeHTTP(recSin, peticionAportaciones(http.MethodPost, "/api/v1/goal-contributions", "jwt-valido", e044HTTPCuerpo, e04xHTTPClave))
	if recSin.Code != http.StatusServiceUnavailable {
		t.Fatalf("sin servicio: esperado 503, obtenido %d", recSin.Code)
	}

	agotada := &limitadorAportaciones{permitido: false, restante: 3 * time.Second}
	storeCuota := nuevoStoreAportacionesHTTP()
	hCuota := internalhttp.NewContributionCreateHandler(servicioAportacionesHTTP(storeCuota), e004TokensValidos(), newE004Sesiones(), agotada, silencioso())
	recCuota := httptest.NewRecorder()
	hCuota.ServeHTTP(recCuota, peticionAportaciones(http.MethodPost, "/api/v1/goal-contributions", "jwt-valido", e044HTTPCuerpo, e04xHTTPClave))
	if recCuota.Code != http.StatusTooManyRequests {
		t.Fatalf("esperado 429, obtenido %d: %s", recCuota.Code, recCuota.Body.String())
	}
	if recCuota.Header().Get("Retry-After") != "3" {
		t.Fatalf("Retry-After inesperado: %q", recCuota.Header().Get("Retry-After"))
	}
	if agotada.clase != "contributions" {
		t.Fatalf("clase de cuota inesperada: %q", agotada.clase)
	}
	if storeCuota.escrituras() != 0 {
		t.Fatalf("una cuota agotada no debe escribir; escrituras=%d", storeCuota.escrituras())
	}
}

func TestE044HTTPConcurrenciaMismaClave(t *testing.T) {
	store := nuevoStoreAportacionesHTTP()
	h := internalhttp.NewContributionCreateHandler(servicioAportacionesHTTP(store), e004TokensValidos(), newE004Sesiones(), nil, silencioso())

	var wg sync.WaitGroup
	recorders := make([]*httptest.ResponseRecorder, 2)
	for i := range recorders {
		wg.Add(1)
		go func(indice int) {
			defer wg.Done()
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, peticionAportaciones(http.MethodPost, "/api/v1/goal-contributions", "jwt-valido", e044HTTPCuerpo, e04xHTTPClave))
			recorders[indice] = rec
		}(i)
	}
	wg.Wait()

	replays := 0
	for _, rec := range recorders {
		if rec.Code != http.StatusCreated {
			t.Fatalf("esperado 201 en ambas carreras, obtenido %d: %s", rec.Code, rec.Body.String())
		}
		if rec.Header().Get("X-Idempotent-Replay") == "true" {
			replays++
		}
	}
	if replays != 1 {
		t.Fatalf("se esperaba exactamente un replay, hay %d", replays)
	}
	if store.escrituras() != 1 {
		t.Fatalf("la misma clave no debe escribir dos veces; escrituras=%d", store.escrituras())
	}
}
