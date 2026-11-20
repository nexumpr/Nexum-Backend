package transport_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	appContrib "github.com/nexumpr/Nexum-Backend/internal/application/contributions"
	internalhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
)

// Pruebas HTTP de E043 (`POST /api/v1/goal-contributions/preview`). El almacen es un doble
// con estado; la autorizacion usa la cadena real Bearer + sesion de los helpers compartidos
// de transporte. E043 no escribe: las pruebas comprueban que el doble no recibe escrituras.

const (
	e04xHTTPMeta   = "018f90ef-08e5-7a68-b09f-2b67a47c92f1"
	e04xHTTPMetaB  = "018f90ef-08e5-7a68-b09f-2b67a47c92f3"
	e04xHTTPAporte = "018f90ef-08e5-7a68-b09f-2b67a47c92f2"
	e04xHTTPClave  = "idem-key-00000000001"
)

// storeAportacionesHTTP implementa appContrib.Store en memoria. Se usa en E043 y E044; no es
// persistencia productiva.
type storeAportacionesHTTP struct {
	mu            sync.Mutex
	contributions map[string]appContrib.Contribution
	idem          map[string]appContrib.IdempotencyRecord
	goalSaved     int64
	goalActive    int64
	goalStatus    string
	creates       int
	failCreate    error
}

func nuevoStoreAportacionesHTTP() *storeAportacionesHTTP {
	return &storeAportacionesHTTP{
		contributions: map[string]appContrib.Contribution{},
		idem:          map[string]appContrib.IdempotencyRecord{},
		goalStatus:    "active",
	}
}

func (s *storeAportacionesHTTP) clave(userID, id string) string { return userID + "|" + id }

func (s *storeAportacionesHTTP) claveIdem(userID, op, key string) string {
	return userID + "|" + op + "|" + key
}

func (s *storeAportacionesHTTP) escrituras() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.creates
}

func (s *storeAportacionesHTTP) LoadIdempotency(_ context.Context, userID, op, key string) (*appContrib.IdempotencyRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.idem[s.claveIdem(userID, op, key)]
	if !ok {
		return nil, nil
	}
	copia := r
	return &copia, nil
}

func (s *storeAportacionesHTTP) LoadGoal(_ context.Context, _ string, goalID string) (*appContrib.GoalSnapshot, error) {
	if goalID != e04xHTTPMeta && goalID != e04xHTTPMetaB {
		return nil, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	saved := s.goalSaved
	if goalID == e04xHTTPMetaB {
		saved = 0
	}
	return &appContrib.GoalSnapshot{
		ID:                  goalID,
		TargetCents:         100000,
		SavedCents:          saved,
		Status:              s.goalStatus,
		ActiveContributions: s.goalActive,
		Version:             1,
	}, nil
}

func (s *storeAportacionesHTTP) CreateContribution(_ context.Context, write appContrib.CreateWrite) (*appContrib.StoredResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failCreate != nil {
		return nil, s.failCreate
	}
	if r, ok := s.idem[s.claveIdem(write.UserID, write.Operation, write.IdempotencyKey)]; ok {
		if r.PayloadHash != write.PayloadHash {
			return nil, appContrib.ErrConflict
		}
		return &appContrib.StoredResponse{Status: r.HTTPStatus, Body: r.ResponseBody, Location: r.Location, Replayed: true}, nil
	}
	s.creates++
	s.goalSaved += write.Contribution.AmountCents
	s.goalActive++
	s.contributions[s.clave(write.UserID, write.Contribution.ID)] = write.Contribution
	s.idem[s.claveIdem(write.UserID, write.Operation, write.IdempotencyKey)] = appContrib.IdempotencyRecord{
		PayloadHash:  write.PayloadHash,
		HTTPStatus:   201,
		ResponseBody: append([]byte(nil), write.ResponseBody...),
		Location:     write.Location,
		ExpiresAt:    write.ExpiresAt,
	}
	return &appContrib.StoredResponse{Status: 201, Body: write.ResponseBody, Location: write.Location}, nil
}

func (s *storeAportacionesHTTP) ListContributions(_ context.Context, query appContrib.ListQuery, _ string, limit int) ([]appContrib.Contribution, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	items := make([]appContrib.Contribution, 0, len(s.contributions))
	for _, c := range s.contributions {
		if c.UserID == query.UserID {
			items = append(items, c)
		}
	}
	if len(items) > limit {
		return items[:limit], "next", nil
	}
	return items, "", nil
}

func (s *storeAportacionesHTTP) LoadContribution(_ context.Context, userID, id string) (*appContrib.Contribution, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.contributions[s.clave(userID, id)]
	if !ok {
		return nil, nil
	}
	copia := c
	return &copia, nil
}

func (s *storeAportacionesHTTP) ReverseContribution(_ context.Context, write appContrib.ReverseWrite) (*appContrib.StoredResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if r, ok := s.idem[s.claveIdem(write.UserID, write.Operation, write.IdempotencyKey)]; ok {
		if r.PayloadHash != write.PayloadHash {
			return nil, appContrib.ErrConflict
		}
		return &appContrib.StoredResponse{Status: r.HTTPStatus, Body: r.ResponseBody, Location: r.Location, Replayed: true}, nil
	}
	c, ok := s.contributions[s.clave(write.UserID, write.Previous.ID)]
	if !ok {
		return nil, appContrib.ErrNotFound
	}
	if c.Status != appContrib.StatusActive {
		return nil, appContrib.ErrAlreadyReversed
	}
	c.Status = write.Next.Status
	c.Version = write.Next.Version
	c.ReversedAt = write.Next.ReversedAt
	c.ReverseReason = write.Next.ReverseReason
	s.contributions[s.clave(write.UserID, c.ID)] = c
	s.goalSaved -= c.AmountCents
	s.goalActive--
	s.idem[s.claveIdem(write.UserID, write.Operation, write.IdempotencyKey)] = appContrib.IdempotencyRecord{
		PayloadHash:  write.PayloadHash,
		HTTPStatus:   200,
		ResponseBody: append([]byte(nil), write.ResponseBody...),
		Location:     write.Location,
		ExpiresAt:    write.ExpiresAt,
	}
	return &appContrib.StoredResponse{Status: 200, Body: write.ResponseBody, Location: write.Location}, nil
}

func servicioAportacionesHTTP(store appContrib.Store) *appContrib.Service {
	return appContrib.NewService(store).
		WithClock(func() time.Time { return time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC) }).
		WithIDGenerator(func() (string, error) { return e04xHTTPAporte, nil }).
		WithCursorKey([]byte("0123456789abcdef0123456789abcdef"))
}

func peticionAportaciones(metodo, ruta, token, cuerpo, clave string) *http.Request {
	req := httptest.NewRequest(metodo, ruta, strings.NewReader(cuerpo))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if clave != "" {
		req.Header.Set("Idempotency-Key", clave)
	}
	return req
}

// limitadorAportaciones permite simular cuota agotada y fallo de Redis.
type limitadorAportaciones struct {
	permitido bool
	restante  time.Duration
	err       error
	clase     string
	consultas int
}

func (l *limitadorAportaciones) Allow(_ context.Context, class, _ string) (bool, time.Duration, error) {
	l.consultas++
	l.clase = class
	return l.permitido, l.restante, l.err
}

func codigoDeError(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var envelope struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("respuesta de error no decodificable (%d): %s", rec.Code, rec.Body.String())
	}
	return envelope.Error.Code
}

func TestE043HTTPPreviewContrato(t *testing.T) {
	store := nuevoStoreAportacionesHTTP()
	h := internalhttp.NewContributionPreviewHandler(servicioAportacionesHTTP(store), e004TokensValidos(), newE004Sesiones(), nil, silencioso())
	cuerpo := `{"amount":"600.00","currency":"MXN","allocations":[{"goalId":"` + e04xHTTPMeta + `","amount":"600.00"}]}`
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, peticionAportaciones(http.MethodPost, "/api/v1/goal-contributions/preview", "jwt-valido", cuerpo, ""))

	if rec.Code != http.StatusOK {
		t.Fatalf("esperado 200, obtenido %d: %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatalf("Cache-Control inesperado: %q", rec.Header().Get("Cache-Control"))
	}
	var body struct {
		Amount      string `json:"amount"`
		Currency    string `json:"currency"`
		Allocations []struct {
			GoalID string `json:"goalId"`
			Amount string `json:"amount"`
		} `json:"allocations"`
		Remaining string `json:"remaining"`
		CanCommit bool   `json:"canCommit"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("json: %v (%s)", err, rec.Body.String())
	}
	if body.Amount != "600.00" || body.Currency != "MXN" || body.Remaining != "0.00" || !body.CanCommit {
		t.Fatalf("preview inesperado: %+v", body)
	}
	if len(body.Allocations) != 1 || body.Allocations[0].GoalID != e04xHTTPMeta || body.Allocations[0].Amount != "600.00" {
		t.Fatalf("allocations inesperadas: %+v", body.Allocations)
	}
	if store.escrituras() != 0 {
		t.Fatalf("E043 no debe escribir; escrituras=%d", store.escrituras())
	}
}

func TestE043HTTPPreviewCalculoSinEscribir(t *testing.T) {
	store := nuevoStoreAportacionesHTTP()
	servicio := servicioAportacionesHTTP(store)
	h := internalhttp.NewContributionPreviewHandler(servicio, e004TokensValidos(), newE004Sesiones(), nil, silencioso())

	// Dos metas, una con capacidad y otra saturada (SavedCents == TargetCents en la meta A).
	store.mu.Lock()
	store.goalSaved = 100000
	store.mu.Unlock()
	cuerpo := `{"amount":"600.00","currency":"MXN","allocations":[` +
		`{"goalId":"` + e04xHTTPMeta + `","amount":"300.00"},` +
		`{"goalId":"` + e04xHTTPMetaB + `","amount":"300.00"}]}`
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, peticionAportaciones(http.MethodPost, "/api/v1/goal-contributions/preview", "jwt-valido", cuerpo, ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("esperado 200, obtenido %d: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Remaining string `json:"remaining"`
		CanCommit bool   `json:"canCommit"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("json: %v", err)
	}
	if body.CanCommit {
		t.Fatalf("una meta sin capacidad debe impedir el commit: %+v", body)
	}
	if body.Remaining != "300.00" {
		t.Fatalf("remanente inesperado: %q", body.Remaining)
	}
	if store.escrituras() != 0 {
		t.Fatalf("E043 no debe escribir; escrituras=%d", store.escrituras())
	}
	if store.goalSaved != 100000 {
		t.Fatalf("E043 no debe modificar metas; saved=%d", store.goalSaved)
	}
}

func TestE043HTTPPreviewValidacionesDeNegocio(t *testing.T) {
	casos := []struct {
		nombre   string
		cuerpo   string
		esperado int
		codigo   string
	}{
		{
			nombre:   "suma incompleta",
			cuerpo:   `{"amount":"1000.00","currency":"MXN","allocations":[{"goalId":"` + e04xHTTPMeta + `","amount":"600.00"}]}`,
			esperado: http.StatusUnprocessableEntity,
			codigo:   "BUSINESS_RULE_VIOLATION",
		},
		{
			nombre: "meta repetida",
			cuerpo: `{"amount":"600.00","currency":"MXN","allocations":[` +
				`{"goalId":"` + e04xHTTPMeta + `","amount":"300.00"},` +
				`{"goalId":"` + e04xHTTPMeta + `","amount":"300.00"}]}`,
			esperado: http.StatusUnprocessableEntity,
			codigo:   "BUSINESS_RULE_VIOLATION",
		},
		{
			nombre:   "divisa no soportada",
			cuerpo:   `{"amount":"600.00","currency":"USD","allocations":[{"goalId":"` + e04xHTTPMeta + `","amount":"600.00"}]}`,
			esperado: http.StatusUnprocessableEntity,
			codigo:   "BUSINESS_RULE_VIOLATION",
		},
		{
			nombre:   "importe sin decimales",
			cuerpo:   `{"amount":"600","currency":"MXN","allocations":[{"goalId":"` + e04xHTTPMeta + `","amount":"600.00"}]}`,
			esperado: http.StatusBadRequest,
			codigo:   "INVALID_REQUEST",
		},
		{
			nombre:   "meta inexistente",
			cuerpo:   `{"amount":"600.00","currency":"MXN","allocations":[{"goalId":"018f90ef-08e5-7a68-b09f-2b67a47c92ff","amount":"600.00"}]}`,
			esperado: http.StatusNotFound,
			codigo:   "NOT_FOUND",
		},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			store := nuevoStoreAportacionesHTTP()
			h := internalhttp.NewContributionPreviewHandler(servicioAportacionesHTTP(store), e004TokensValidos(), newE004Sesiones(), nil, silencioso())
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, peticionAportaciones(http.MethodPost, "/api/v1/goal-contributions/preview", "jwt-valido", caso.cuerpo, ""))
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

func TestE043HTTPPreviewSolicitudYAutenticacion(t *testing.T) {
	casos := []struct {
		nombre   string
		token    string
		cuerpo   string
		esperado int
	}{
		{"sin bearer", "", `{"amount":"1.00","currency":"MXN","allocations":[{"goalId":"` + e04xHTTPMeta + `","amount":"1.00"}]}`, http.StatusUnauthorized},
		{"token desconocido", "desconocido", `{"amount":"1.00","currency":"MXN","allocations":[{"goalId":"` + e04xHTTPMeta + `","amount":"1.00"}]}`, http.StatusUnauthorized},
		{"token expirado", "jwt-expirado", `{"amount":"1.00","currency":"MXN","allocations":[{"goalId":"` + e04xHTTPMeta + `","amount":"1.00"}]}`, http.StatusUnauthorized},
		{"json malformado", "jwt-valido", `{"amount":`, http.StatusBadRequest},
		{"campo desconocido", "jwt-valido", `{"amount":"1.00","currency":"MXN","allocations":[{"goalId":"` + e04xHTTPMeta + `","amount":"1.00"}],"extra":true}`, http.StatusBadRequest},
		{"cuerpo excesivo", "jwt-valido", `{"amount":"1.00","currency":"MXN","padding":"` + strings.Repeat("x", 70<<10) + `"}`, http.StatusBadRequest},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			store := nuevoStoreAportacionesHTTP()
			h := internalhttp.NewContributionPreviewHandler(servicioAportacionesHTTP(store), e004TokensValidos(), newE004Sesiones(), nil, silencioso())
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, peticionAportaciones(http.MethodPost, "/api/v1/goal-contributions/preview", caso.token, caso.cuerpo, ""))
			if rec.Code != caso.esperado {
				t.Fatalf("esperado %d, obtenido %d: %s", caso.esperado, rec.Code, rec.Body.String())
			}
			if strings.Contains(rec.Body.String(), "jwt-valido") || strings.Contains(rec.Body.String(), e04xHTTPMeta) {
				t.Fatalf("la respuesta no debe revelar identificadores: %s", rec.Body.String())
			}
		})
	}
}

func TestE043HTTPPreviewSesionRevocada(t *testing.T) {
	store := nuevoStoreAportacionesHTTP()
	sesiones := newE004Sesiones()
	h := internalhttp.NewContributionPreviewHandler(servicioAportacionesHTTP(store), e004TokensValidos(), sesiones, nil, silencioso())
	cuerpo := `{"amount":"600.00","currency":"MXN","allocations":[{"goalId":"` + e04xHTTPMeta + `","amount":"600.00"}]}`

	activa := httptest.NewRecorder()
	h.ServeHTTP(activa, peticionAportaciones(http.MethodPost, "/api/v1/goal-contributions/preview", "jwt-valido", cuerpo, ""))
	if activa.Code != http.StatusOK {
		t.Fatalf("sesion activa: esperado 200, obtenido %d", activa.Code)
	}

	sesiones.revocar("018f90ef-08e5-7a68-b09f-0000000000c1")
	revocada := httptest.NewRecorder()
	h.ServeHTTP(revocada, peticionAportaciones(http.MethodPost, "/api/v1/goal-contributions/preview", "jwt-valido", cuerpo, ""))
	if revocada.Code != http.StatusUnauthorized {
		t.Fatalf("sesion revocada: esperado 401, obtenido %d", revocada.Code)
	}
}

func TestE043HTTPPreview503SinComponentes(t *testing.T) {
	casos := []struct {
		nombre string
		h      http.Handler
	}{
		{"sin servicio", internalhttp.NewContributionPreviewHandler(nil, e004TokensValidos(), newE004Sesiones(), nil, silencioso())},
		{"sin tokens", internalhttp.NewContributionPreviewHandler(servicioAportacionesHTTP(nuevoStoreAportacionesHTTP()), nil, newE004Sesiones(), nil, silencioso())},
		{"sin autorizador", internalhttp.NewContributionPreviewHandler(servicioAportacionesHTTP(nuevoStoreAportacionesHTTP()), e004TokensValidos(), nil, nil, silencioso())},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			rec := httptest.NewRecorder()
			caso.h.ServeHTTP(rec, peticionAportaciones(http.MethodPost, "/api/v1/goal-contributions/preview", "jwt-valido", `{}`, ""))
			if rec.Code != http.StatusServiceUnavailable {
				t.Fatalf("esperado 503, obtenido %d: %s", rec.Code, rec.Body.String())
			}
			if codigo := codigoDeError(t, rec); codigo != "DEPENDENCY_UNAVAILABLE" {
				t.Fatalf("codigo esperado DEPENDENCY_UNAVAILABLE, obtenido %s", codigo)
			}
		})
	}
}

func TestE043HTTPPreviewCuota(t *testing.T) {
	cuerpo := `{"amount":"600.00","currency":"MXN","allocations":[{"goalId":"` + e04xHTTPMeta + `","amount":"600.00"}]}`

	agotada := &limitadorAportaciones{permitido: false, restante: 5 * time.Second}
	h := internalhttp.NewContributionPreviewHandler(servicioAportacionesHTTP(nuevoStoreAportacionesHTTP()), e004TokensValidos(), newE004Sesiones(), agotada, silencioso())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, peticionAportaciones(http.MethodPost, "/api/v1/goal-contributions/preview", "jwt-valido", cuerpo, ""))
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("esperado 429, obtenido %d: %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Retry-After") != "5" {
		t.Fatalf("Retry-After inesperado: %q", rec.Header().Get("Retry-After"))
	}
	if agotada.clase != "contributions" {
		t.Fatalf("clase de cuota inesperada: %q", agotada.clase)
	}

	// Fail-open: un fallo de Redis no convierte la lectura en una caida.
	caida := &limitadorAportaciones{permitido: true, err: errors.New("redis caido")}
	hCaida := internalhttp.NewContributionPreviewHandler(servicioAportacionesHTTP(nuevoStoreAportacionesHTTP()), e004TokensValidos(), newE004Sesiones(), caida, silencioso())
	recCaida := httptest.NewRecorder()
	hCaida.ServeHTTP(recCaida, peticionAportaciones(http.MethodPost, "/api/v1/goal-contributions/preview", "jwt-valido", cuerpo, ""))
	if recCaida.Code != http.StatusOK {
		t.Fatalf("con Redis caido el preview debe servirse: %d %s", recCaida.Code, recCaida.Body.String())
	}
	if caida.consultas != 1 {
		t.Fatalf("la cuota debe consultarse una vez: %d", caida.consultas)
	}
}
