package transport_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	appGoals "github.com/nexumpr/Nexum-Backend/internal/application/goals"
	domainGoal "github.com/nexumpr/Nexum-Backend/internal/domain/goal"
	"github.com/nexumpr/Nexum-Backend/internal/domain/money"
	internalhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
)

// Doble con estado del puerto completo y pruebas HTTP de E040
// (`GET /api/v1/goals/{goalId}`): 200 con ETag fuerte, 404 homogéneo propio/ajeno, 400 de
// parámetro, 401 sin sesión y 503 fail-closed. Las pruebas de E038 viven en goals_list_test.go.

const e038HTTPSecreto = "secreto-de-cursores-de-prueba-32b"

var e038HTTPAhora = time.Date(2026, 11, 18, 16, 0, 0, 0, time.UTC)

// e038UUID construye la representación canónica 8-4-4-4-12 que exige `format: uuid`.
func e038UUID(n int) string {
	return fmt.Sprintf("018f90ef-08e5-7a68-b09f-%012d", n)
}

// e040HTTPStore doble con estado del puerto completo: pagina de verdad y agrega la colección.
type e040HTTPStore struct {
	mu         sync.Mutex
	porUsuario map[string][]appGoals.GoalRow
	errGet     error
	errLista   error
	errTotal   error
}

func newE040HTTPStore() *e040HTTPStore {
	return &e040HTTPStore{porUsuario: map[string][]appGoals.GoalRow{}}
}

func (s *e040HTTPStore) agregar(t *testing.T, goal domainGoal.Goal) {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.porUsuario[goal.UserID] = append(s.porUsuario[goal.UserID], appGoals.GoalRow{
		Goal:    goal,
		SortKey: "GOAL#" + goal.ID,
	})
}

func (s *e040HTTPStore) LoadIdempotency(context.Context, string, string, string) (*appGoals.IdempotencyRecord, error) {
	return nil, nil
}

func (s *e040HTTPStore) CreateGoal(context.Context, appGoals.CreateWrite) (*appGoals.StoredResponse, error) {
	return nil, errors.New("no usado por las pruebas de lectura")
}

func (s *e040HTTPStore) LoadGoal(_ context.Context, userID, goalID string) (*domainGoal.Goal, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.errGet != nil {
		return nil, s.errGet
	}
	for _, fila := range s.porUsuario[userID] {
		if fila.Goal.ID == goalID {
			copia := fila.Goal
			return &copia, nil
		}
	}
	return nil, nil
}

func (s *e040HTTPStore) ListUserGoals(_ context.Context, query appGoals.ListStoreQuery) ([]appGoals.GoalRow, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.errLista != nil {
		return nil, "", s.errLista
	}
	filas := s.porUsuario[query.UserID]
	inicio := 0
	if query.StartSK != "" {
		for inicio < len(filas) && filas[inicio].SortKey <= query.StartSK {
			inicio++
		}
	}
	salida := make([]appGoals.GoalRow, 0, query.Limit)
	for i := inicio; i < len(filas) && len(salida) < query.Limit; i++ {
		if query.HasStatus && filas[i].Goal.Status != query.Status {
			continue
		}
		salida = append(salida, filas[i])
	}
	nextSK := ""
	if len(salida) == query.Limit && len(salida) > 0 {
		ultima := salida[len(salida)-1].SortKey
		for i := range filas {
			if filas[i].SortKey > ultima {
				nextSK = ultima
				break
			}
		}
	}
	return salida, nextSK, nil
}

func (s *e040HTTPStore) AggregateUserGoals(_ context.Context, userID string) (appGoals.Totals, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.errTotal != nil {
		return appGoals.Totals{}, s.errTotal
	}
	suma := money.MustNew(0, domainGoal.CurrencyMXN)
	activas := 0
	for _, fila := range s.porUsuario[userID] {
		if fila.Goal.Status != domainGoal.StatusActive {
			continue
		}
		var err error
		suma, err = suma.Add(fila.Goal.SavedAmount)
		if err != nil {
			return appGoals.Totals{}, err
		}
		activas++
	}
	return appGoals.Totals{ActiveCount: activas, Saved: suma}, nil
}

func e038HTTPServicio(store appGoals.Store) *appGoals.Service {
	return appGoals.NewService(store).
		WithClock(func() time.Time { return e038HTTPAhora }).
		WithCursorSecret([]byte(e038HTTPSecreto))
}

func e040Handler(store appGoals.Store) http.Handler {
	return internalhttp.NewGoalGetHandler(e038HTTPServicio(store), e004TokensValidos(), newE004Sesiones(), silencioso())
}

func e038MetaHTTP(t *testing.T, userID, id string, status domainGoal.Status, savedCents int64) domainGoal.Goal {
	t.Helper()
	meta, err := domainGoal.New(
		userID, id, "Meta "+id,
		savedCents+10000, savedCents,
		time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC),
		e038HTTPAhora,
	)
	if err != nil {
		t.Fatalf("no se pudo construir la meta %s: %v", id, err)
	}
	meta.Status = status
	return meta
}

// e040Coleccion siembra cuatro metas de la cuenta sintética: tres activas (100.00 + 250.00 +
// 5.00), una archivada cuyo importe NO debe entrar en `totals.saved`.
func e040Coleccion(t *testing.T) *e040HTTPStore {
	t.Helper()
	store := newE040HTTPStore()
	store.agregar(t, e038MetaHTTP(t, e004HTTPUsuario, e038UUID(1), domainGoal.StatusActive, 10000))
	store.agregar(t, e038MetaHTTP(t, e004HTTPUsuario, e038UUID(2), domainGoal.StatusActive, 25000))
	store.agregar(t, e038MetaHTTP(t, e004HTTPUsuario, e038UUID(3), domainGoal.StatusArchived, 77777))
	store.agregar(t, e038MetaHTTP(t, e004HTTPUsuario, e038UUID(4), domainGoal.StatusActive, 500))
	return store
}

func e040Peticion(token, goalID string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/goals/"+goalID, nil)
	req.SetPathValue("goalId", goalID)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return req
}

// TestE040HTTP200 comprueba el cuerpo de nueve campos, el ETag fuerte y la cabecera de caché.
func TestE040HTTP200(t *testing.T) {
	store := e040Coleccion(t)

	// La versión se publica como `ETag: "vN"`, el validador que E041 exige en If-Match.
	meta := e038MetaHTTP(t, e004HTTPUsuario, e038UUID(9), domainGoal.StatusActive, 12345)
	meta.Version = 3
	store.agregar(t, meta)

	rec := httptest.NewRecorder()
	e040Handler(store).ServeHTTP(rec, e040Peticion("jwt-valido", e038UUID(9)))

	if rec.Code != http.StatusOK {
		t.Fatalf("esperado 200, obtenido %d (%s)", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("ETag"); got != `"v3"` {
		t.Fatalf("ETag inesperado: %q", got)
	}
	if got := rec.Header().Get("Cache-Control"); got != "private, no-store" {
		t.Fatalf("Cache-Control inesperado: %q", got)
	}
	var cuerpo map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &cuerpo); err != nil {
		t.Fatalf("respuesta no es JSON: %v", err)
	}
	if len(cuerpo) != 9 {
		t.Fatalf("el DTO debe tener 9 campos, tiene %d: %v", len(cuerpo), cuerpo)
	}
	if cuerpo["id"] != e038UUID(9) || cuerpo["openingSaved"] != "123.45" || cuerpo["savedAmount"] != "123.45" {
		t.Fatalf("cuerpo inesperado: %v", cuerpo)
	}
	if cuerpo["status"] != "active" || cuerpo["version"] != float64(3) {
		t.Fatalf("estado o version inesperados: %v", cuerpo)
	}
}

// TestE040HTTP404 comprueba que un recurso ajeno no se distingue de uno inexistente.
func TestE040HTTP404(t *testing.T) {
	store := e040Coleccion(t)
	casos := []struct {
		nombre string
		token  string
		goalID string
	}{
		{"inexistente", "jwt-valido", e038UUID(99)},
		{"de otro usuario", "jwt-sujeto-ajeno", e038UUID(1)},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			rec := httptest.NewRecorder()
			e040Handler(store).ServeHTTP(rec, e040Peticion(caso.token, caso.goalID))
			if rec.Code != http.StatusNotFound {
				t.Fatalf("esperado 404, obtenido %d", rec.Code)
			}
			if codigo := e039CodigoError(t, rec); codigo != "NOT_FOUND" {
				t.Fatalf("codigo inesperado: %q", codigo)
			}
		})
	}
}

// TestE040HTTP400 cubre un identificador con formato distinto de uuid.
func TestE040HTTP400(t *testing.T) {
	rec := httptest.NewRecorder()
	e040Handler(e040Coleccion(t)).ServeHTTP(rec, e040Peticion("jwt-valido", "no-es-un-uuid"))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("esperado 400, obtenido %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "no-es-un-uuid") {
		t.Fatal("el error no debe reflejar el valor recibido")
	}
}

// TestE040HTTP401Y503 cubre la autenticación y el fail-closed.
func TestE040HTTP401Y503(t *testing.T) {
	t.Run("sin bearer", func(t *testing.T) {
		rec := httptest.NewRecorder()
		e040Handler(e040Coleccion(t)).ServeHTTP(rec, e040Peticion("", e038UUID(1)))
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("esperado 401, obtenido %d", rec.Code)
		}
	})
	t.Run("sin servicio", func(t *testing.T) {
		handler := internalhttp.NewGoalGetHandler(nil, e004TokensValidos(), newE004Sesiones(), silencioso())
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, e040Peticion("jwt-valido", e038UUID(1)))
		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("esperado 503, obtenido %d", rec.Code)
		}
	})
	t.Run("fallo de persistencia", func(t *testing.T) {
		store := e040Coleccion(t)
		store.errGet = errors.New("dynamodb inaccesible")
		rec := httptest.NewRecorder()
		e040Handler(store).ServeHTTP(rec, e040Peticion("jwt-valido", e038UUID(1)))
		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("esperado 503, obtenido %d (%s)", rec.Code, rec.Body.String())
		}
	})
}
