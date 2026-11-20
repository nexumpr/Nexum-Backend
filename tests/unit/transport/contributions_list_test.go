package transport_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	appContrib "github.com/nexumpr/Nexum-Backend/internal/application/contributions"
	internalhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
)

// Pruebas HTTP de E045 (`GET /api/v1/goal-contributions`). El almacén es un doble con estado;
// la autenticación usa la cadena real Bearer + sesión de los helpers compartidos de transporte.
// El cursor se genera y valida con la implementación HMAC productiva, no con un sustituto.

const (
	e045AporteA = "018f90ef-08e5-7a68-b09f-2b67a47c92f1"
	e045AporteB = "018f90ef-08e5-7a68-b09f-2b67a47c92f2"
	e045AporteC = "018f90ef-08e5-7a68-b09f-2b67a47c92f3"
	e045AporteD = "018f90ef-08e5-7a68-b09f-2b67a47c92f4"
	e045Meta    = "018f90ef-08e5-7a68-b09f-2b67a47c9a01"
	e045MetaB   = "018f90ef-08e5-7a68-b09f-2b67a47c9a02"
	e045Clave   = "idem-e045-0000000001"
)

var e045Ahora = time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)

// storeAportacionesE04x implementa appContrib.Store con estado en memoria.
type storeAportacionesE04x struct {
	mu           sync.Mutex
	aportaciones map[string]appContrib.Contribution
	idem         map[string]appContrib.IdempotencyRecord
	fallo        error
}

func nuevoStoreAportacionesE04x() *storeAportacionesE04x {
	return &storeAportacionesE04x{
		aportaciones: map[string]appContrib.Contribution{},
		idem:         map[string]appContrib.IdempotencyRecord{},
	}
}

func (s *storeAportacionesE04x) clave(userID, id string) string { return userID + "|" + id }

func (s *storeAportacionesE04x) claveIdem(userID, operacion, clave string) string {
	return userID + "|" + operacion + "|" + clave
}

// sembrar inserta una aportación del usuario indicado.
func (s *storeAportacionesE04x) sembrar(userID, id, estado, goalID string, centavos int64, ocurrida time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.aportaciones[s.clave(userID, id)] = appContrib.Contribution{
		ID:          id,
		UserID:      userID,
		AmountCents: centavos,
		Currency:    appContrib.CurrencyMXN,
		Allocations: []appContrib.Allocation{{GoalID: goalID, AmountCents: centavos}},
		OccurredAt:  ocurrida,
		Status:      estado,
		Version:     1,
		CreatedAt:   ocurrida,
		UpdatedAt:   ocurrida,
	}
}

func (s *storeAportacionesE04x) ver(userID, id string) appContrib.Contribution {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.aportaciones[s.clave(userID, id)]
}

func (s *storeAportacionesE04x) LoadIdempotency(_ context.Context, userID, operacion, clave string) (*appContrib.IdempotencyRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fallo != nil {
		return nil, s.fallo
	}
	registro, ok := s.idem[s.claveIdem(userID, operacion, clave)]
	if !ok {
		return nil, nil
	}
	copia := registro
	return &copia, nil
}

func (s *storeAportacionesE04x) LoadGoal(_ context.Context, _, goalID string) (*appContrib.GoalSnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fallo != nil {
		return nil, s.fallo
	}
	return &appContrib.GoalSnapshot{ID: goalID, TargetCents: 1000000, Status: "active", Version: 1}, nil
}

func (s *storeAportacionesE04x) CreateContribution(_ context.Context, write appContrib.CreateWrite) (*appContrib.StoredResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fallo != nil {
		return nil, s.fallo
	}
	if registro, ok := s.idem[s.claveIdem(write.UserID, write.Operation, write.IdempotencyKey)]; ok {
		if registro.PayloadHash != write.PayloadHash {
			return nil, appContrib.ErrConflict
		}
		return &appContrib.StoredResponse{Status: registro.HTTPStatus, Body: registro.ResponseBody, Location: registro.Location, Replayed: true}, nil
	}
	s.aportaciones[s.clave(write.UserID, write.Contribution.ID)] = write.Contribution
	s.idem[s.claveIdem(write.UserID, write.Operation, write.IdempotencyKey)] = appContrib.IdempotencyRecord{
		PayloadHash:  write.PayloadHash,
		HTTPStatus:   201,
		ResponseBody: write.ResponseBody,
		Location:     write.Location,
		ExpiresAt:    write.ExpiresAt,
	}
	return &appContrib.StoredResponse{Status: 201, Body: write.ResponseBody, Location: write.Location}, nil
}

// ListContributions pagina de forma determinista por ID ascendente y aplica los filtros antes
// de paginar, igual que exige el contrato de E045.
func (s *storeAportacionesE04x) ListContributions(_ context.Context, query appContrib.ListQuery, startSK string, limit int) ([]appContrib.Contribution, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fallo != nil {
		return nil, "", s.fallo
	}
	filtradas := make([]appContrib.Contribution, 0, len(s.aportaciones))
	for _, aportacion := range s.aportaciones {
		if aportacion.UserID != query.UserID {
			continue
		}
		if query.GoalID != "" {
			coincide := false
			for _, asignacion := range aportacion.Allocations {
				if asignacion.GoalID == query.GoalID {
					coincide = true
					break
				}
			}
			if !coincide {
				continue
			}
		}
		if query.Status != "" && aportacion.Status != query.Status {
			continue
		}
		if query.HasFrom && aportacion.OccurredAt.Before(query.From) {
			continue
		}
		if query.HasTo && aportacion.OccurredAt.After(query.To) {
			continue
		}
		filtradas = append(filtradas, aportacion)
	}
	sort.Slice(filtradas, func(i, j int) bool { return filtradas[i].ID < filtradas[j].ID })
	inicio := 0
	for inicio < len(filtradas) && filtradas[inicio].ID <= startSK {
		inicio++
	}
	restantes := filtradas[inicio:]
	if len(restantes) == 0 {
		return []appContrib.Contribution{}, "", nil
	}
	if len(restantes) <= limit {
		return restantes, "", nil
	}
	// El cursor es la clave del último elemento devuelto: la página siguiente continúa después
	// de esa clave (`ExclusiveStartKey`), igual que en el adaptador real.
	return restantes[:limit], restantes[limit-1].ID, nil
}

func (s *storeAportacionesE04x) LoadContribution(_ context.Context, userID, id string) (*appContrib.Contribution, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fallo != nil {
		return nil, s.fallo
	}
	aportacion, ok := s.aportaciones[s.clave(userID, id)]
	if !ok {
		return nil, nil
	}
	copia := aportacion
	return &copia, nil
}

func (s *storeAportacionesE04x) ReverseContribution(_ context.Context, write appContrib.ReverseWrite) (*appContrib.StoredResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fallo != nil {
		return nil, s.fallo
	}
	if registro, ok := s.idem[s.claveIdem(write.UserID, write.Operation, write.IdempotencyKey)]; ok {
		if registro.PayloadHash != write.PayloadHash {
			return nil, appContrib.ErrConflict
		}
		return &appContrib.StoredResponse{Status: registro.HTTPStatus, Body: registro.ResponseBody, Location: registro.Location, Replayed: true}, nil
	}
	actual, ok := s.aportaciones[s.clave(write.UserID, write.Previous.ID)]
	if !ok {
		return nil, appContrib.ErrNotFound
	}
	if actual.Status != appContrib.StatusActive {
		return nil, appContrib.ErrAlreadyReversed
	}
	actual.Status = write.Next.Status
	actual.Version = write.Next.Version
	actual.ReversedAt = write.Next.ReversedAt
	actual.ReverseReason = write.Next.ReverseReason
	actual.UpdatedAt = write.Next.UpdatedAt
	s.aportaciones[s.clave(write.UserID, actual.ID)] = actual
	s.idem[s.claveIdem(write.UserID, write.Operation, write.IdempotencyKey)] = appContrib.IdempotencyRecord{
		PayloadHash:  write.PayloadHash,
		HTTPStatus:   200,
		ResponseBody: write.ResponseBody,
		Location:     write.Location,
		ExpiresAt:    write.ExpiresAt,
	}
	return &appContrib.StoredResponse{Status: 200, Body: write.ResponseBody, Location: write.Location}, nil
}

// servicioAportacionesE04x construye el servicio con reloj determinista y clave de cursor.
func servicioAportacionesE04x(store appContrib.Store) *appContrib.Service {
	return appContrib.NewService(store).
		WithClock(func() time.Time { return e045Ahora }).
		WithCursorKey([]byte("0123456789abcdef0123456789abcdef"))
}

// respaldoListadoE045 refleja exactamente el contrato `PageOfContribution`.
type respaldoListadoE045 struct {
	Items []struct {
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
	} `json:"items"`
	NextCursor string `json:"nextCursor"`
}

func decodificarListadoE045(t *testing.T, rec *httptest.ResponseRecorder) respaldoListadoE045 {
	t.Helper()
	var cuerpo respaldoListadoE045
	if err := json.Unmarshal(rec.Body.Bytes(), &cuerpo); err != nil {
		t.Fatalf("json del listado: %v (cuerpo=%s)", err, rec.Body.String())
	}
	return cuerpo
}

// TestE045HTTPListadoContratoYPaginacion cubre T01/T07: contrato exacto, paginación sin
// duplicados y cursor ligado al usuario y a los filtros.
func TestE045HTTPListadoContratoYPaginacion(t *testing.T) {
	store := nuevoStoreAportacionesE04x()
	store.sembrar(e004HTTPUsuario, e045AporteA, appContrib.StatusActive, e045Meta, 60000, e045Ahora.Add(-3*time.Hour))
	store.sembrar(e004HTTPUsuario, e045AporteB, appContrib.StatusActive, e045Meta, 2500, e045Ahora.Add(-2*time.Hour))
	store.sembrar(e004HTTPUsuario, e045AporteC, appContrib.StatusReversed, e045MetaB, 1000, e045Ahora.Add(-1*time.Hour))
	store.sembrar(e004HTTPOtroUser, e045AporteD, appContrib.StatusActive, e045Meta, 99900, e045Ahora.Add(-90*time.Minute))

	handler := internalhttp.NewContributionListHandler(servicioAportacionesE04x(store), e004TokensValidos(), newE004Sesiones(), nil, silencioso())

	// Página 1 con limit=2: dos elementos y cursor.
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, e004Peticion(http.MethodGet, "/api/v1/goal-contributions?limit=2", "jwt-valido", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("esperado 200, obtenido %d: %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatalf("Cache-Control inesperado: %q", rec.Header().Get("Cache-Control"))
	}
	primera := decodificarListadoE045(t, rec)
	if len(primera.Items) != 2 {
		t.Fatalf("esperados 2 elementos, obtenidos %d", len(primera.Items))
	}
	if primera.NextCursor == "" {
		t.Fatalf("la primera página debe devolver nextCursor")
	}
	// Sólo aportaciones propias y con los siete campos del contrato.
	for _, item := range primera.Items {
		if item.ID == e045AporteD {
			t.Fatalf("el listado no debe incluir aportaciones de otro usuario")
		}
		if item.Currency != "MXN" || item.Version != 1 || item.OccurredAt == "" || item.Status == "" {
			t.Fatalf("item fuera de contrato: %+v", item)
		}
		if len(item.Allocations) != 1 || item.Allocations[0].GoalID == "" || item.Allocations[0].Amount != item.Amount {
			t.Fatalf("allocations fuera de contrato: %+v", item.Allocations)
		}
	}
	if primera.Items[0].Amount != "600.00" {
		t.Fatalf("importe decimal inesperado: %q", primera.Items[0].Amount)
	}

	// Página 2 con el cursor: el resto, sin duplicados y sin cursor.
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, e004Peticion(http.MethodGet, "/api/v1/goal-contributions?limit=2&cursor="+primera.NextCursor, "jwt-valido", ""))
	if rec2.Code != http.StatusOK {
		t.Fatalf("segunda página: esperado 200, obtenido %d: %s", rec2.Code, rec2.Body.String())
	}
	segunda := decodificarListadoE045(t, rec2)
	if len(segunda.Items) != 1 || segunda.NextCursor != "" {
		t.Fatalf("segunda página inesperada: %d items, cursor=%q", len(segunda.Items), segunda.NextCursor)
	}
	if segunda.Items[0].ID == primera.Items[0].ID || segunda.Items[0].ID == primera.Items[1].ID {
		t.Fatalf("página duplicada: %q", segunda.Items[0].ID)
	}

	// El cursor de un usuario no sirve para otro: está ligado al sujeto.
	recAjeno := httptest.NewRecorder()
	handler.ServeHTTP(recAjeno, e004Peticion(http.MethodGet, "/api/v1/goal-contributions?limit=2&cursor="+primera.NextCursor, "jwt-sujeto-ajeno", ""))
	if recAjeno.Code != http.StatusBadRequest {
		t.Fatalf("cursor de otro usuario: esperado 400, obtenido %d", recAjeno.Code)
	}

	// Un cursor manipulado no autoriza una lectura: se altera un carácter intermedio, de modo
	// que los bytes decodificados cambian y la firma HMAC deja de coincidir.
	alterado := []byte(primera.NextCursor)
	posicion := len(alterado) / 2
	if alterado[posicion] == 'A' {
		alterado[posicion] = 'B'
	} else {
		alterado[posicion] = 'A'
	}
	recAlterado := httptest.NewRecorder()
	handler.ServeHTTP(recAlterado, e004Peticion(http.MethodGet, "/api/v1/goal-contributions?cursor="+string(alterado), "jwt-valido", ""))
	if recAlterado.Code != http.StatusBadRequest {
		t.Fatalf("cursor manipulado: esperado 400, obtenido %d", recAlterado.Code)
	}

	// Un cursor inventado tampoco se acepta.
	recInventado := httptest.NewRecorder()
	handler.ServeHTTP(recInventado, e004Peticion(http.MethodGet, "/api/v1/goal-contributions?cursor=cursor-inventado-0000", "jwt-valido", ""))
	if recInventado.Code != http.StatusBadRequest {
		t.Fatalf("cursor inventado: esperado 400, obtenido %d", recInventado.Code)
	}

	// El cursor está ligado a la firma exacta de filtros: cambiar el filtro lo invalida.
	recFiltro := httptest.NewRecorder()
	handler.ServeHTTP(recFiltro, e004Peticion(http.MethodGet, "/api/v1/goal-contributions?limit=2&status=reversed&cursor="+primera.NextCursor, "jwt-valido", ""))
	if recFiltro.Code != http.StatusBadRequest {
		t.Fatalf("cursor con filtros distintos: esperado 400, obtenido %d", recFiltro.Code)
	}
}

// TestE045HTTPFiltros cubre los filtros `status`, `goalId`, `from` y `to` aplicados antes de
// paginar, y la validación de entrada.
func TestE045HTTPFiltros(t *testing.T) {
	store := nuevoStoreAportacionesE04x()
	store.sembrar(e004HTTPUsuario, e045AporteA, appContrib.StatusActive, e045Meta, 60000, e045Ahora.Add(-30*time.Hour))
	store.sembrar(e004HTTPUsuario, e045AporteB, appContrib.StatusActive, e045MetaB, 2500, e045Ahora.Add(-5*time.Hour))
	store.sembrar(e004HTTPUsuario, e045AporteC, appContrib.StatusReversed, e045Meta, 1000, e045Ahora.Add(-1*time.Hour))
	handler := internalhttp.NewContributionListHandler(servicioAportacionesE04x(store), e004TokensValidos(), newE004Sesiones(), nil, silencioso())

	casos := []struct {
		nombre   string
		consulta string
		esperado int
	}{
		{"status active", "?status=active", 2},
		{"status reversed", "?status=reversed", 1},
		{"goalId", "?goalId=" + e045Meta, 2},
		{"desde", "?from=" + e045Ahora.Add(-6*time.Hour).Format(time.RFC3339), 2},
		{"hasta", "?to=" + e045Ahora.Add(-6*time.Hour).Format(time.RFC3339), 1},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, e004Peticion(http.MethodGet, "/api/v1/goal-contributions"+caso.consulta, "jwt-valido", ""))
			if rec.Code != http.StatusOK {
				t.Fatalf("esperado 200, obtenido %d: %s", rec.Code, rec.Body.String())
			}
			if obtenido := len(decodificarListadoE045(t, rec).Items); obtenido != caso.esperado {
				t.Fatalf("filtro %q: esperados %d elementos, obtenidos %d", caso.consulta, caso.esperado, obtenido)
			}
		})
	}

	invalidos := []string{
		"?limit=0",
		"?limit=-3",
		"?limit=101",
		"?limit=abc",
		"?status=pendiente",
		"?goalId=no-es-uuid",
		"?from=ayer",
		"?to=2026-13-40",
		"?from=" + e045Ahora.Format(time.RFC3339) + "&to=" + e045Ahora.Add(-24*time.Hour).Format(time.RFC3339),
	}
	for _, consulta := range invalidos {
		t.Run("invalido "+consulta, func(t *testing.T) {
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, e004Peticion(http.MethodGet, "/api/v1/goal-contributions"+consulta, "jwt-valido", ""))
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("%s: esperado 400, obtenido %d", consulta, rec.Code)
			}
		})
	}
}

// TestE045HTTPAutenticacionYFalloCerrado cubre 401 sin Bearer, 401 con sesión revocada, 503 sin
// servicio, 503 sin clave de cursor y 503 cuando DynamoDB no responde.
func TestE045HTTPAutenticacionYFalloCerrado(t *testing.T) {
	store := nuevoStoreAportacionesE04x()
	store.sembrar(e004HTTPUsuario, e045AporteA, appContrib.StatusActive, e045Meta, 60000, e045Ahora)

	sesiones := newE004Sesiones()
	handler := internalhttp.NewContributionListHandler(servicioAportacionesE04x(store), e004TokensValidos(), sesiones, nil, silencioso())

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, e004Peticion(http.MethodGet, "/api/v1/goal-contributions", "", ""))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("sin Bearer: esperado 401, obtenido %d", rec.Code)
	}

	sesiones.revocar(e004HTTPSesion)
	recRevocada := httptest.NewRecorder()
	handler.ServeHTTP(recRevocada, e004Peticion(http.MethodGet, "/api/v1/goal-contributions", "jwt-valido", ""))
	if recRevocada.Code != http.StatusUnauthorized {
		t.Fatalf("sesion revocada: esperado 401, obtenido %d", recRevocada.Code)
	}

	sinServicio := internalhttp.NewContributionListHandler(nil, e004TokensValidos(), newE004Sesiones(), nil, silencioso())
	recSinServicio := httptest.NewRecorder()
	sinServicio.ServeHTTP(recSinServicio, e004Peticion(http.MethodGet, "/api/v1/goal-contributions", "jwt-valido", ""))
	if recSinServicio.Code != http.StatusServiceUnavailable {
		t.Fatalf("sin servicio: esperado 503, obtenido %d", recSinServicio.Code)
	}

	sinClave := appContrib.NewService(store)
	handlerSinClave := internalhttp.NewContributionListHandler(sinClave, e004TokensValidos(), newE004Sesiones(), nil, silencioso())
	recSinClave := httptest.NewRecorder()
	handlerSinClave.ServeHTTP(recSinClave, e004Peticion(http.MethodGet, "/api/v1/goal-contributions", "jwt-valido", ""))
	if recSinClave.Code != http.StatusServiceUnavailable {
		t.Fatalf("sin clave de cursor: esperado 503, obtenido %d", recSinClave.Code)
	}

	store.fallo = appContrib.ErrUnavailable
	handlerFallo := internalhttp.NewContributionListHandler(servicioAportacionesE04x(store), e004TokensValidos(), newE004Sesiones(), nil, silencioso())
	recFallo := httptest.NewRecorder()
	handlerFallo.ServeHTTP(recFallo, e004Peticion(http.MethodGet, "/api/v1/goal-contributions", "jwt-valido", ""))
	if recFallo.Code != http.StatusServiceUnavailable {
		t.Fatalf("almacen caido: esperado 503, obtenido %d", recFallo.Code)
	}

	var cuerpo map[string]any
	if err := json.Unmarshal(recFallo.Body.Bytes(), &cuerpo); err != nil {
		t.Fatalf("json de error: %v", err)
	}
	if _, ok := cuerpo["error"]; !ok {
		t.Fatalf("error sin envoltorio comun: %s", recFallo.Body.String())
	}
}

// TestE045HTTPListadoVacio comprueba que sin aportaciones la respuesta es 200 con `items: []` y
// sin `nextCursor`, nunca un error.
func TestE045HTTPListadoVacio(t *testing.T) {
	handler := internalhttp.NewContributionListHandler(servicioAportacionesE04x(nuevoStoreAportacionesE04x()), e004TokensValidos(), newE004Sesiones(), nil, silencioso())
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, e004Peticion(http.MethodGet, "/api/v1/goal-contributions", "jwt-valido", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("esperado 200, obtenido %d: %s", rec.Code, rec.Body.String())
	}
	if cuerpo := decodificarListadoE045(t, rec); len(cuerpo.Items) != 0 || cuerpo.NextCursor != "" {
		t.Fatalf("listado vacio inesperado: %+v", cuerpo)
	}
	if !strings.Contains(rec.Body.String(), `"items":[]`) {
		t.Fatalf("items debe serializarse como arreglo vacio: %s", rec.Body.String())
	}
}

// TestE045HTTPAlmacenConErrorNoEnumerado garantiza que un error interno no se convierte en una
// lista vacia silenciosa.
func TestE045HTTPAlmacenConErrorNoEnumerado(t *testing.T) {
	store := nuevoStoreAportacionesE04x()
	store.fallo = errors.New("fallo no clasificado del almacen")
	handler := internalhttp.NewContributionListHandler(servicioAportacionesE04x(store), e004TokensValidos(), newE004Sesiones(), nil, silencioso())
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, e004Peticion(http.MethodGet, "/api/v1/goal-contributions", "jwt-valido", ""))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("error no clasificado: esperado 503, obtenido %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "fallo no clasificado") {
		t.Fatalf("la respuesta no debe filtrar el error interno: %s", rec.Body.String())
	}
}
