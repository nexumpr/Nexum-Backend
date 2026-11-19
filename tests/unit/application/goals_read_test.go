package application_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	appGoals "github.com/nexumpr/Nexum-Backend/internal/application/goals"
	domainGoal "github.com/nexumpr/Nexum-Backend/internal/domain/goal"
	"github.com/nexumpr/Nexum-Backend/internal/domain/money"
)

// Doble con estado del puerto completo y pruebas de aplicación de E040
// (`GET /api/v1/goals/{goalId}`): lectura propia, 404 homogéneo para recursos ajenos,
// fail-closed 503 y rechazo de items incoherentes.
//
// La lista pagina de verdad (StartSK y Limit) y el agregado recorre la colección ENTERA del
// usuario, de modo que las pruebas de E038 (goals_list_test.go) no pueden pasar por casualidad
// mirando sólo la página.

const (
	e038Usuario = "11111111-2222-4333-8444-555555555555"
	e038Otro    = "99999999-8888-4777-8666-555555555555"
	e038Secreto = "secreto-de-cursores-de-prueba-32b"
)

var e038Ahora = time.Date(2026, 11, 18, 16, 0, 0, 0, time.UTC)

// e038Store es un doble con estado del puerto completo.
type e038Store struct {
	mu         sync.Mutex
	porUsuario map[string][]appGoals.GoalRow
	consultas  []appGoals.ListStoreQuery
	agregados  []string
	errLista   error
	errTotal   error
	errLectura error
}

func newE038Store() *e038Store {
	return &e038Store{porUsuario: map[string][]appGoals.GoalRow{}}
}

func (s *e038Store) agregar(t *testing.T, goal domainGoal.Goal) {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.porUsuario[goal.UserID] = append(s.porUsuario[goal.UserID], appGoals.GoalRow{
		Goal:    goal,
		SortKey: "GOAL#" + goal.ID,
	})
}

func (s *e038Store) LoadIdempotency(context.Context, string, string, string) (*appGoals.IdempotencyRecord, error) {
	return nil, nil
}

func (s *e038Store) CreateGoal(context.Context, appGoals.CreateWrite) (*appGoals.StoredResponse, error) {
	return nil, errors.New("no usado por las pruebas de lectura")
}

func (s *e038Store) LoadGoal(_ context.Context, userID, goalID string) (*domainGoal.Goal, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.errLectura != nil {
		return nil, s.errLectura
	}
	for _, fila := range s.porUsuario[userID] {
		if fila.Goal.ID == goalID {
			copia := fila.Goal
			return &copia, nil
		}
	}
	return nil, nil
}

func (s *e038Store) ListUserGoals(_ context.Context, query appGoals.ListStoreQuery) ([]appGoals.GoalRow, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.consultas = append(s.consultas, query)
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
	// El cursor siguiente se calcula sobre la última fila devuelta: sin duplicados ni saltos.
	nextSK := ""
	if len(salida) == query.Limit && len(filas) > 0 {
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

func (s *e038Store) AggregateUserGoals(_ context.Context, userID string) (appGoals.Totals, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.agregados = append(s.agregados, userID)
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

func e038Servicio(store appGoals.Store) *appGoals.Service {
	return appGoals.NewService(store).
		WithClock(func() time.Time { return e038Ahora }).
		WithCursorSecret([]byte(e038Secreto))
}

// e038Meta construye una meta válida: objetivo por encima del ahorro y el estado indicado.
func e038Meta(t *testing.T, userID, id string, status domainGoal.Status, savedCents int64) domainGoal.Goal {
	t.Helper()
	meta, err := domainGoal.New(
		userID, id, "Meta "+id,
		savedCents+10000, savedCents,
		time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC),
		e038Ahora,
	)
	if err != nil {
		t.Fatalf("no se pudo construir la meta %s: %v", id, err)
	}
	meta.Status = status
	return meta
}

// e038Coleccion siembra cinco metas: tres activas (100.00 + 250.00 + 5.00), una alcanzada y una
// archivada cuyos importes NO deben entrar en `totals.saved`.
func e038Coleccion(t *testing.T) *e038Store {
	t.Helper()
	store := newE038Store()
	store.agregar(t, e038Meta(t, e038Usuario, "meta-01", domainGoal.StatusActive, 10000))
	store.agregar(t, e038Meta(t, e038Usuario, "meta-02", domainGoal.StatusActive, 25000))
	store.agregar(t, e038Meta(t, e038Usuario, "meta-03", domainGoal.StatusArchived, 77777))
	store.agregar(t, e038Meta(t, e038Usuario, "meta-04", domainGoal.StatusActive, 500))
	store.agregar(t, e038Meta(t, e038Usuario, "meta-05", domainGoal.StatusAchieved, 99999))
	return store
}

// TestE040GetPropioY404 comprueba la lectura y que un recurso ajeno no se distingue de uno
// inexistente.
func TestE040GetPropioY404(t *testing.T) {
	store := e038Coleccion(t)
	servicio := e038Servicio(store)

	meta, err := servicio.Get(context.Background(), e038Usuario, "meta-01")
	if err != nil {
		t.Fatalf("la meta propia debe leerse: %v", err)
	}
	if meta.ID != "meta-01" || meta.OpeningSaved.Amount() != 10000 {
		t.Fatalf("meta inesperada: %+v", meta)
	}

	casos := []struct {
		nombre string
		userID string
		goalID string
	}{
		{"inexistente", e038Usuario, "meta-99"},
		{"de otro usuario", e038Otro, "meta-01"},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			if _, err := servicio.Get(context.Background(), caso.userID, caso.goalID); !errors.Is(err, appGoals.ErrNotFound) {
				t.Fatalf("esperado ErrNotFound, obtenido %v", err)
			}
		})
	}

	if _, err := servicio.Get(context.Background(), "  ", "meta-01"); !errors.Is(err, appGoals.ErrUnauthorized) {
		t.Fatalf("sin identidad debe ser ErrUnauthorized, obtenido %v", err)
	}
	if _, err := servicio.Get(context.Background(), e038Usuario, "  "); !errors.Is(err, appGoals.ErrInvalidRequest) {
		t.Fatalf("sin identificador debe ser ErrInvalidRequest, obtenido %v", err)
	}
}

// TestE040ItemIncoherente comprueba que una meta persistida con forma inválida no se emite.
func TestE040ItemIncoherente(t *testing.T) {
	store := newE038Store()
	store.porUsuario[e038Usuario] = []appGoals.GoalRow{{
		Goal:    domainGoal.Goal{ID: "meta-rota", UserID: e038Usuario},
		SortKey: "GOAL#meta-rota",
	}}
	if _, err := e038Servicio(store).Get(context.Background(), e038Usuario, "meta-rota"); !errors.Is(err, appGoals.ErrInconsistent) {
		t.Fatalf("esperado ErrInconsistent, obtenido %v", err)
	}
}

// TestE040FalloDePersistencia comprueba el 503 y no un 404 falso.
func TestE040FalloDePersistencia(t *testing.T) {
	store := newE038Store()
	store.errLectura = errors.New("dynamodb inaccesible")
	if _, err := e038Servicio(store).Get(context.Background(), e038Usuario, "meta-01"); !errors.Is(err, appGoals.ErrUnavailable) {
		t.Fatalf("esperado ErrUnavailable, obtenido %v", err)
	}
	if _, err := appGoals.NewService(nil).Get(context.Background(), e038Usuario, "meta-01"); !errors.Is(err, appGoals.ErrUnavailable) {
		t.Fatalf("sin almacen debe ser ErrUnavailable, obtenido %v", err)
	}
}
