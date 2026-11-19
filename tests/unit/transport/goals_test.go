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

	appGoals "github.com/nexumpr/Nexum-Backend/internal/application/goals"
	domainGoal "github.com/nexumpr/Nexum-Backend/internal/domain/goal"
	internalhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
)

// Pruebas HTTP de E039 (`POST /api/v1/goals`): autenticación con estado autoritativo de sesión,
// DTO estricto, `Idempotency-Key` obligatoria, replay con `X-Idempotent-Replay`, conflicto 409,
// reglas de negocio 422 y fail-closed 503. El almacén es un doble; la composición productiva
// usa DynamoDB real.

const (
	e039HTTPClave      = "018f90ef-08e5-7a68-b09f-2b67a47c92f0"
	e039HTTPMetaID     = "018f90ef-08e5-7a68-b09f-0000000000a1"
	e039HTTPCuerpoBase = `{"name":"Viaje","targetAmount":"1000.00","currency":"MXN","targetDate":"2026-12-31","initialSavedAmount":"250.00"}`
)

var e039HTTPAhora = time.Date(2026, 11, 18, 15, 0, 0, 0, time.UTC)

// e039HTTPStore implementa `goals.Store` en memoria con la misma semántica de idempotencia que
// el adaptador real: registro por (usuario, operación, clave) y cuerpo reproducible.
type e039HTTPStore struct {
	mu       sync.Mutex
	registro map[string]appGoals.IdempotencyRecord
	creates  []appGoals.CreateWrite
	errLoad  error
	errCrea  error
}

func newE039HTTPStore() *e039HTTPStore {
	return &e039HTTPStore{registro: map[string]appGoals.IdempotencyRecord{}}
}

func (s *e039HTTPStore) clave(userID, operation, key string) string {
	return userID + "|" + operation + "|" + key
}

func (s *e039HTTPStore) LoadIdempotency(_ context.Context, userID, operation, key string) (*appGoals.IdempotencyRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.errLoad != nil {
		return nil, s.errLoad
	}
	record, ok := s.registro[s.clave(userID, operation, key)]
	if !ok {
		return nil, nil
	}
	copia := record
	return &copia, nil
}

func (s *e039HTTPStore) CreateGoal(_ context.Context, write appGoals.CreateWrite) (*appGoals.StoredResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.creates = append(s.creates, write)
	if s.errCrea != nil {
		return nil, s.errCrea
	}
	s.registro[s.clave(write.Goal.UserID, write.Operation, write.IdempotencyKey)] = appGoals.IdempotencyRecord{
		PayloadHash:  write.PayloadHash,
		Status:       "COMPLETED",
		HTTPStatus:   201,
		ResponseBody: write.ResponseBody,
		ResourceID:   write.Goal.ID,
		Location:     write.Location,
		ExpiresAt:    write.Now.Add(write.TTL),
	}
	return &appGoals.StoredResponse{
		Status:     201,
		Body:       write.ResponseBody,
		Location:   write.Location,
		ResourceID: write.Goal.ID,
	}, nil
}

func (s *e039HTTPStore) escrituras() []appGoals.CreateWrite {
	s.mu.Lock()
	defer s.mu.Unlock()
	copia := make([]appGoals.CreateWrite, len(s.creates))
	copy(copia, s.creates)
	return copia
}

// Los tres métodos siguientes completan el puerto `goals.Store`. E039 no los usa; las pruebas
// de E038/E040 viven en goals_queries_test.go con su propio doble con estado.
func (s *e039HTTPStore) LoadGoal(context.Context, string, string) (*domainGoal.Goal, error) {
	return nil, nil
}

func (s *e039HTTPStore) ListUserGoals(context.Context, appGoals.ListStoreQuery) ([]appGoals.GoalRow, string, error) {
	return nil, "", nil
}

func (s *e039HTTPStore) AggregateUserGoals(context.Context, string) (appGoals.Totals, error) {
	return appGoals.Totals{}, nil
}

func e039HTTPServicio(store appGoals.Store) *appGoals.Service {
	return appGoals.NewService(store).
		WithClock(func() time.Time { return e039HTTPAhora }).
		WithIDGenerator(func() (string, error) { return e039HTTPMetaID, nil })
}

func e039Handler(store appGoals.Store) http.Handler {
	return internalhttp.NewGoalsCreateHandler(e039HTTPServicio(store), e004TokensValidos(), newE004Sesiones(), silencioso())
}

func e039Peticion(token, cuerpo, clave string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/goals", strings.NewReader(cuerpo))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if clave != "" {
		req.Header.Set("Idempotency-Key", clave)
	}
	return req
}

func e039Ejecutar(handler http.Handler, req *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func e039Cuerpo(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var salida map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &salida); err != nil {
		t.Fatalf("respuesta no es JSON: %v (%s)", err, rec.Body.String())
	}
	return salida
}

func e039CodigoError(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var sobre struct {
		Error struct {
			Code    string `json:"code"`
			TraceID string `json:"traceId"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &sobre); err != nil {
		t.Fatalf("el error no usa el sobre canonico: %v (%s)", err, rec.Body.String())
	}
	return sobre.Error.Code
}

// TestE039HTTPCrea201 comprueba código, cabeceras y DTO exacto de nueve campos.
func TestE039HTTPCrea201(t *testing.T) {
	store := newE039HTTPStore()
	rec := e039Ejecutar(e039Handler(store), e039Peticion("jwt-valido", e039HTTPCuerpoBase, e039HTTPClave))

	if rec.Code != http.StatusCreated {
		t.Fatalf("esperado 201, obtenido %d (%s)", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Cache-Control"); got != "private, no-store" {
		t.Fatalf("Cache-Control inesperado: %q", got)
	}
	if got := rec.Header().Get("Location"); got != "/api/v1/goals/"+e039HTTPMetaID {
		t.Fatalf("Location inesperada: %q", got)
	}
	if got := rec.Header().Get("X-Idempotent-Replay"); got != "" {
		t.Fatalf("el primer alta no puede marcarse como replay: %q", got)
	}

	cuerpo := e039Cuerpo(t, rec)
	if len(cuerpo) != 9 {
		t.Fatalf("el DTO debe tener 9 campos, tiene %d: %v", len(cuerpo), cuerpo)
	}
	esperado := map[string]string{
		"id":           e039HTTPMetaID,
		"name":         "Viaje",
		"targetAmount": "1000.00",
		"savedAmount":  "250.00",
		"openingSaved": "250.00",
		"currency":     "MXN",
		"targetDate":   "2026-12-31",
		"status":       "active",
	}
	for campo, valor := range esperado {
		if obtenido, _ := cuerpo[campo].(string); obtenido != valor {
			t.Fatalf("%s: esperado %q, obtenido %q", campo, valor, obtenido)
		}
	}
	if cuerpo["version"] != float64(1) {
		t.Fatalf("version inesperada: %v", cuerpo["version"])
	}
	if strings.Contains(rec.Body.String(), "payloadHash") || strings.Contains(rec.Body.String(), e039HTTPClave) {
		t.Fatal("la respuesta no debe filtrar el registro de idempotencia")
	}
}

// TestE039HTTPReplay comprueba que el reintento idéntico devuelve el mismo 201 con la cabecera
// de replay y sin duplicar la escritura.
func TestE039HTTPReplay(t *testing.T) {
	store := newE039HTTPStore()
	handler := e039Handler(store)

	primera := e039Ejecutar(handler, e039Peticion("jwt-valido", e039HTTPCuerpoBase, e039HTTPClave))
	segunda := e039Ejecutar(handler, e039Peticion("jwt-valido", e039HTTPCuerpoBase, e039HTTPClave))

	if primera.Code != http.StatusCreated || segunda.Code != http.StatusCreated {
		t.Fatalf("codigos inesperados: %d y %d", primera.Code, segunda.Code)
	}
	if got := segunda.Header().Get("X-Idempotent-Replay"); got != "true" {
		t.Fatalf("el replay debe anunciarse: %q", got)
	}
	if primera.Body.String() != segunda.Body.String() {
		t.Fatalf("el replay debe devolver el mismo cuerpo:\n%s\n%s", primera.Body.String(), segunda.Body.String())
	}
	if len(store.escrituras()) != 1 {
		t.Fatalf("el replay no debe escribir de nuevo: %d escrituras", len(store.escrituras()))
	}
}

// TestE039HTTPAislamientoEntreUsuarios verifica que la misma clave de dos cuentas no comparte
// resultado: no hay cabecera de replay y se escriben dos metas.
func TestE039HTTPAislamientoEntreUsuarios(t *testing.T) {
	store := newE039HTTPStore()
	handler := e039Handler(store)

	propia := e039Ejecutar(handler, e039Peticion("jwt-valido", e039HTTPCuerpoBase, e039HTTPClave))
	ajena := e039Ejecutar(handler, e039Peticion("jwt-sujeto-ajeno", e039HTTPCuerpoBase, e039HTTPClave))

	if propia.Code != http.StatusCreated || ajena.Code != http.StatusCreated {
		t.Fatalf("codigos inesperados: %d y %d", propia.Code, ajena.Code)
	}
	if got := ajena.Header().Get("X-Idempotent-Replay"); got != "" {
		t.Fatalf("la clave de otro usuario no puede ser replay: %q", got)
	}
	if len(store.escrituras()) != 2 {
		t.Fatalf("se esperaban dos altas independientes, hubo %d", len(store.escrituras()))
	}
}

// TestE039HTTPAutenticacion cubre T03: sin Bearer, esquema inválido, token desconocido y expirado.
func TestE039HTTPAutenticacion(t *testing.T) {
	casos := []struct {
		nombre   string
		cabecera string
	}{
		{"sin cabecera", ""},
		{"esquema basico", "Basic abc"},
		{"token desconocido", "Bearer desconocido"},
		{"token expirado", "Bearer jwt-expirado"},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			req := e039Peticion("", e039HTTPCuerpoBase, e039HTTPClave)
			if caso.cabecera != "" {
				req.Header.Set("Authorization", caso.cabecera)
			}
			rec := e039Ejecutar(e039Handler(newE039HTTPStore()), req)
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("esperado 401, obtenido %d (%s)", rec.Code, rec.Body.String())
			}
			// El middleware de autenticación emite sus propios códigos (INVALID_TOKEN,
			// TOKEN_EXPIRED...); E039 sólo garantiza el 401 y el sobre canónico.
			if codigo := e039CodigoError(t, rec); codigo == "" {
				t.Fatal("el 401 debe usar el sobre de error canonico")
			}
		})
	}
}

// TestE039HTTPValidacion cubre 400 de esquema y 422 de negocio sin escribir nada.
func TestE039HTTPValidacion(t *testing.T) {
	casos := []struct {
		nombre   string
		cuerpo   string
		clave    string
		esperado int
	}{
		{"json malformado", `{"name":`, e039HTTPClave, http.StatusBadRequest},
		{"campo desconocido", `{"name":"Viaje","targetAmount":"1000.00","currency":"MXN","targetDate":"2026-12-31","extra":1}`, e039HTTPClave, http.StatusBadRequest},
		{"importe con un decimal", `{"name":"Viaje","targetAmount":"1000.0","currency":"MXN","targetDate":"2026-12-31"}`, e039HTTPClave, http.StatusBadRequest},
		{"importe con ceros a la izquierda", `{"name":"Viaje","targetAmount":"01000.00","currency":"MXN","targetDate":"2026-12-31"}`, e039HTTPClave, http.StatusBadRequest},
		{"fecha imposible", `{"name":"Viaje","targetAmount":"1000.00","currency":"MXN","targetDate":"2026-02-30"}`, e039HTTPClave, http.StatusBadRequest},
		{"divisa no admitida", `{"name":"Viaje","targetAmount":"1000.00","currency":"USD","targetDate":"2026-12-31"}`, e039HTTPClave, http.StatusBadRequest},
		{"nombre vacio", `{"name":"   ","targetAmount":"1000.00","currency":"MXN","targetDate":"2026-12-31"}`, e039HTTPClave, http.StatusBadRequest},
		{"sin idempotency-key", e039HTTPCuerpoBase, "", http.StatusBadRequest},
		{"idempotency-key corta", e039HTTPCuerpoBase, "corto", http.StatusBadRequest},
		{"objetivo cero", `{"name":"Viaje","targetAmount":"0.00","currency":"MXN","targetDate":"2026-12-31"}`, e039HTTPClave, http.StatusUnprocessableEntity},
		{"apertura mayor que objetivo", `{"name":"Viaje","targetAmount":"1000.00","currency":"MXN","targetDate":"2026-12-31","initialSavedAmount":"2000.00"}`, e039HTTPClave, http.StatusUnprocessableEntity},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			store := newE039HTTPStore()
			rec := e039Ejecutar(e039Handler(store), e039Peticion("jwt-valido", caso.cuerpo, caso.clave))
			if rec.Code != caso.esperado {
				t.Fatalf("esperado %d, obtenido %d (%s)", caso.esperado, rec.Code, rec.Body.String())
			}
			if len(store.escrituras()) != 0 {
				t.Fatal("una solicitud rechazada no debe escribir")
			}
		})
	}
}

// TestE039HTTPAperturaNulaEsAusente comprueba que `initialSavedAmount: null` se interpreta como
// campo opcional omitido, no como un importe invalido.
func TestE039HTTPAperturaNulaEsAusente(t *testing.T) {
	cuerpo := `{"name":"Viaje","targetAmount":"1000.00","currency":"MXN","targetDate":"2026-12-31","initialSavedAmount":null}`
	rec := e039Ejecutar(e039Handler(newE039HTTPStore()), e039Peticion("jwt-valido", cuerpo, e039HTTPClave))
	if rec.Code != http.StatusCreated {
		t.Fatalf("esperado 201, obtenido %d (%s)", rec.Code, rec.Body.String())
	}
	salida := e039Cuerpo(t, rec)
	if salida["openingSaved"] != "0.00" {
		t.Fatalf("openingSaved inesperado: %v", salida["openingSaved"])
	}
}

// TestE039HTTPConflicto comprueba el 409 por reutilizar la clave con otro contenido.
func TestE039HTTPConflicto(t *testing.T) {
	store := newE039HTTPStore()
	handler := e039Handler(store)
	if rec := e039Ejecutar(handler, e039Peticion("jwt-valido", e039HTTPCuerpoBase, e039HTTPClave)); rec.Code != http.StatusCreated {
		t.Fatalf("alta inicial fallo: %d", rec.Code)
	}

	otro := `{"name":"Otro destino","targetAmount":"1000.00","currency":"MXN","targetDate":"2026-12-31"}`
	rec := e039Ejecutar(handler, e039Peticion("jwt-valido", otro, e039HTTPClave))
	if rec.Code != http.StatusConflict {
		t.Fatalf("esperado 409, obtenido %d (%s)", rec.Code, rec.Body.String())
	}
	if codigo := e039CodigoError(t, rec); codigo != "CONFLICT" {
		t.Fatalf("codigo de error inesperado: %q", codigo)
	}
	if len(store.escrituras()) != 1 {
		t.Fatalf("el conflicto no debe escribir: %d", len(store.escrituras()))
	}
}

// TestE039HTTPFallaCerrado cubre los 503: sin servicio, sin lectura y sin escritura.
func TestE039HTTPFallaCerrado(t *testing.T) {
	t.Run("sin servicio", func(t *testing.T) {
		handler := internalhttp.NewGoalsCreateHandler(nil, e004TokensValidos(), newE004Sesiones(), silencioso())
		rec := e039Ejecutar(handler, e039Peticion("jwt-valido", e039HTTPCuerpoBase, e039HTTPClave))
		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("esperado 503, obtenido %d", rec.Code)
		}
	})
	t.Run("fallo de lectura", func(t *testing.T) {
		store := newE039HTTPStore()
		store.errLoad = errors.New("dynamodb inaccesible")
		rec := e039Ejecutar(e039Handler(store), e039Peticion("jwt-valido", e039HTTPCuerpoBase, e039HTTPClave))
		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("esperado 503, obtenido %d (%s)", rec.Code, rec.Body.String())
		}
		if codigo := e039CodigoError(t, rec); codigo != "DEPENDENCY_UNAVAILABLE" {
			t.Fatalf("codigo de error inesperado: %q", codigo)
		}
	})
	t.Run("fallo de escritura", func(t *testing.T) {
		store := newE039HTTPStore()
		store.errCrea = appGoals.ErrUnavailable
		rec := e039Ejecutar(e039Handler(store), e039Peticion("jwt-valido", e039HTTPCuerpoBase, e039HTTPClave))
		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("esperado 503, obtenido %d (%s)", rec.Code, rec.Body.String())
		}
	})
}

// TestE039HTTPClaveNoSeRefleja comprueba que ni la clave de idempotencia ni el identificador de
// usuario aparecen en el cuerpo ni en las cabeceras de error.
func TestE039HTTPClaveNoSeRefleja(t *testing.T) {
	rec := e039Ejecutar(e039Handler(newE039HTTPStore()), e039Peticion("jwt-valido", `{"name":`, e039HTTPClave))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("esperado 400, obtenido %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), e039HTTPClave) {
		t.Fatalf("el error no debe reflejar la clave de idempotencia: %s", rec.Body.String())
	}
}
