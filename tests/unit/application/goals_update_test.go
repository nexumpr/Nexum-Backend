package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	appGoals "github.com/nexumpr/Nexum-Backend/internal/application/goals"
	domainGoal "github.com/nexumpr/Nexum-Backend/internal/domain/goal"
)

// Pruebas de aplicación de E041 (`PATCH /api/v1/goals/{goalId}`): validación, transiciones,
// objetivo por debajo del ahorro, CAS de versión y fail-closed. El doble con estado vive en
// goals_read_test.go; aquí se le añade el CAS de escritura.

var e041Ahora = time.Date(2026, 11, 18, 17, 0, 0, 0, time.UTC)

// UpdateGoal implementa el compare-and-swap del puerto sobre el doble: versión exacta, guarda de
// ahorro y aplicación del patch de dominio. Dos escrituras con la misma versión sólo dejan ganar
// a la primera.
func (s *e038Store) UpdateGoal(_ context.Context, write appGoals.UpdateWrite) (*domainGoal.Goal, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	filas := s.porUsuario[write.UserID]
	for i := range filas {
		if filas[i].Goal.ID != write.GoalID {
			continue
		}
		if filas[i].Goal.Version != write.ExpectedVersion {
			return nil, appGoals.ErrVersionConflict
		}
		if write.Patch.TargetCents != nil && filas[i].Goal.SavedAmount.Amount() > *write.Patch.TargetCents {
			return nil, appGoals.ErrTargetBelowSaved
		}
		actualizada, err := write.Patch.Apply(filas[i].Goal, write.Now)
		if err != nil {
			return nil, err
		}
		filas[i].Goal = actualizada
		copia := actualizada
		return &copia, nil
	}
	return nil, appGoals.ErrNotFound
}

// e041StoreFallo fuerza un 503 en la escritura sin tocar el resto del doble.
type e041StoreFallo struct{ *e038Store }

func (e041StoreFallo) UpdateGoal(context.Context, appGoals.UpdateWrite) (*domainGoal.Goal, error) {
	return nil, appGoals.ErrUnavailable
}

func e041Servicio(store appGoals.Store) *appGoals.Service {
	return appGoals.NewService(store).WithClock(func() time.Time { return e041Ahora })
}

// e041Sembrada devuelve un doble con una meta activa (ahorro 50.00, objetivo 150.00, versión 1).
func e041Sembrada(t *testing.T) *e038Store {
	t.Helper()
	store := newE038Store()
	store.agregar(t, e038Meta(t, e038Usuario, "meta-01", domainGoal.StatusActive, 5000))
	return store
}

func e041Texto(valor string) *string { return &valor }

func e041Cents(valor int64) *int64 { return &valor }

// TestE041ModificaNombreYFecha comprueba el camino feliz: importes históricos intactos, versión
// incrementada y `updatedAt` con la hora del servidor.
func TestE041ModificaNombreYFecha(t *testing.T) {
	store := e041Sembrada(t)
	actualizada, err := e041Servicio(store).Update(context.Background(), appGoals.UpdateCommand{
		UserID:          e038Usuario,
		GoalID:          "meta-01",
		ExpectedVersion: 1,
		Name:            e041Texto("  Viaje largo  "),
		TargetDate:      e041Texto("2027-06-30"),
	})
	if err != nil {
		t.Fatalf("la modificacion no debe fallar: %v", err)
	}
	if actualizada.Name != "Viaje largo" {
		t.Fatalf("el nombre debe normalizarse: %q", actualizada.Name)
	}
	if actualizada.Version != 2 {
		t.Fatalf("la version debe incrementarse: %d", actualizada.Version)
	}
	if !actualizada.UpdatedAt.Equal(e041Ahora) {
		t.Fatalf("updatedAt inesperado: %v", actualizada.UpdatedAt)
	}
	if actualizada.TargetAmount.Amount() != 15000 || actualizada.SavedAmount.Amount() != 5000 || actualizada.OpeningSaved.Amount() != 5000 {
		t.Fatalf("importes alterados: %+v", actualizada)
	}
	if actualizada.FormatTargetDate() != "2027-06-30" {
		t.Fatalf("fecha inesperada: %s", actualizada.FormatTargetDate())
	}
}

// TestE041ObjetivoFrenteAlAhorro comprueba que el objetivo puede igualar el ahorro pero nunca
// quedar por debajo.
func TestE041ObjetivoFrenteAlAhorro(t *testing.T) {
	store := e041Sembrada(t)
	servicio := e041Servicio(store)

	igual, err := servicio.Update(context.Background(), appGoals.UpdateCommand{
		UserID: e038Usuario, GoalID: "meta-01", ExpectedVersion: 1, TargetAmount: e041Cents(5000),
	})
	if err != nil {
		t.Fatalf("igualar el ahorro debe permitirse: %v", err)
	}
	if igual.TargetAmount.Amount() != 5000 || igual.Version != 2 {
		t.Fatalf("objetivo inesperado: %+v", igual.TargetAmount)
	}

	_, err = servicio.Update(context.Background(), appGoals.UpdateCommand{
		UserID: e038Usuario, GoalID: "meta-01", ExpectedVersion: 2, TargetAmount: e041Cents(4999),
	})
	if !errors.Is(err, appGoals.ErrTargetBelowSaved) {
		t.Fatalf("esperado ErrTargetBelowSaved, obtenido %v", err)
	}
}

// TestE041Transiciones cubre el autómata documentado: archived→achieved exige pasar por active.
func TestE041Transiciones(t *testing.T) {
	casos := []struct {
		nombre   string
		desde    domainGoal.Status
		hacia    string
		esperado error
	}{
		{"active a achieved", domainGoal.StatusActive, "achieved", nil},
		{"achieved a active", domainGoal.StatusAchieved, "active", nil},
		{"archived a active", domainGoal.StatusArchived, "active", nil},
		{"active a archived", domainGoal.StatusActive, "archived", nil},
		{"mismo estado", domainGoal.StatusActive, "active", nil},
		{"archived a achieved", domainGoal.StatusArchived, "achieved", appGoals.ErrInvalidTransition},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			store := newE038Store()
			store.agregar(t, e038Meta(t, e038Usuario, "meta-01", caso.desde, 5000))
			estado := caso.hacia
			_, err := e041Servicio(store).Update(context.Background(), appGoals.UpdateCommand{
				UserID: e038Usuario, GoalID: "meta-01", ExpectedVersion: 1, Status: &estado,
			})
			if caso.esperado == nil {
				if err != nil {
					t.Fatalf("la transicion debia aceptarse: %v", err)
				}
				return
			}
			if !errors.Is(err, caso.esperado) {
				t.Fatalf("esperado %v, obtenido %v", caso.esperado, err)
			}
		})
	}
}

// TestE041Validaciones cubre los límites y las entradas mal formadas.
func TestE041Validaciones(t *testing.T) {
	nombreVacio := "   "
	estadoInvalido := "pendiente"
	objetivoCero := int64(0)
	objetivoExcesivo := int64(100_000_000_000_000)
	fechaMala := "2026-02-30"

	casos := []struct {
		nombre   string
		command  appGoals.UpdateCommand
		esperado error
	}{
		{"sin usuario", appGoals.UpdateCommand{GoalID: "meta-01", ExpectedVersion: 1, Name: e041Texto("x")}, appGoals.ErrUnauthorized},
		{"sin meta", appGoals.UpdateCommand{UserID: e038Usuario, ExpectedVersion: 1, Name: e041Texto("x")}, appGoals.ErrInvalidRequest},
		{"sin version", appGoals.UpdateCommand{UserID: e038Usuario, GoalID: "meta-01", Name: e041Texto("x")}, appGoals.ErrInvalidRequest},
		{"nombre vacio", appGoals.UpdateCommand{UserID: e038Usuario, GoalID: "meta-01", ExpectedVersion: 1, Name: &nombreVacio}, appGoals.ErrInvalidRequest},
		{"objetivo cero", appGoals.UpdateCommand{UserID: e038Usuario, GoalID: "meta-01", ExpectedVersion: 1, TargetAmount: &objetivoCero}, appGoals.ErrTargetNotPositive},
		{"objetivo excesivo", appGoals.UpdateCommand{UserID: e038Usuario, GoalID: "meta-01", ExpectedVersion: 1, TargetAmount: &objetivoExcesivo}, appGoals.ErrInvalidRequest},
		{"fecha invalida", appGoals.UpdateCommand{UserID: e038Usuario, GoalID: "meta-01", ExpectedVersion: 1, TargetDate: &fechaMala}, appGoals.ErrInvalidRequest},
		{"estado invalido", appGoals.UpdateCommand{UserID: e038Usuario, GoalID: "meta-01", ExpectedVersion: 1, Status: &estadoInvalido}, appGoals.ErrInvalidRequest},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			store := e041Sembrada(t)
			if _, err := e041Servicio(store).Update(context.Background(), caso.command); !errors.Is(err, caso.esperado) {
				t.Fatalf("esperado %v, obtenido %v", caso.esperado, err)
			}
			if len(store.porUsuario[e038Usuario]) != 1 || store.porUsuario[e038Usuario][0].Goal.Version != 1 {
				t.Fatal("una modificacion invalida no debe escribir")
			}
		})
	}
}

// TestE041Conflicto404Y503 cubre la versión obsoleta, el recurso ajeno y el fallo de servicio.
func TestE041Conflicto404Y503(t *testing.T) {
	t.Run("version obsoleta", func(t *testing.T) {
		store := e041Sembrada(t)
		_, err := e041Servicio(store).Update(context.Background(), appGoals.UpdateCommand{
			UserID: e038Usuario, GoalID: "meta-01", ExpectedVersion: 9, Name: e041Texto("x"),
		})
		if !errors.Is(err, appGoals.ErrVersionConflict) {
			t.Fatalf("esperado ErrVersionConflict, obtenido %v", err)
		}
	})
	t.Run("meta ajena o inexistente", func(t *testing.T) {
		store := e041Sembrada(t)
		_, err := e041Servicio(store).Update(context.Background(), appGoals.UpdateCommand{
			UserID: e038Otro, GoalID: "meta-01", ExpectedVersion: 1, Name: e041Texto("x"),
		})
		if !errors.Is(err, appGoals.ErrNotFound) {
			t.Fatalf("esperado ErrNotFound, obtenido %v", err)
		}
	})
	t.Run("fallo de escritura", func(t *testing.T) {
		store := e041Sembrada(t)
		_, err := e041Servicio(e041StoreFallo{store}).Update(context.Background(), appGoals.UpdateCommand{
			UserID: e038Usuario, GoalID: "meta-01", ExpectedVersion: 1, Name: e041Texto("x"),
		})
		if !errors.Is(err, appGoals.ErrUnavailable) {
			t.Fatalf("esperado ErrUnavailable, obtenido %v", err)
		}
	})
	t.Run("sin almacen", func(t *testing.T) {
		_, err := appGoals.NewService(nil).Update(context.Background(), appGoals.UpdateCommand{
			UserID: e038Usuario, GoalID: "meta-01", ExpectedVersion: 1, Name: e041Texto("x"),
		})
		if !errors.Is(err, appGoals.ErrUnavailable) {
			t.Fatalf("esperado ErrUnavailable, obtenido %v", err)
		}
	})
}

// TestE041PatchVacioEsNoOp fija la decision contractual: `GoalPatch.required` esta vacio en
// el OpenAPI, de modo que un cuerpo `{}` es valido. Debe responder 200 con el recurso actual,
// sin escribir, sin incrementar la version y conservando el ETag (misma Version). Si el
// `If-Match` no coincide con la version vigente, la precondicion sigue siendo 412 aunque el
// cuerpo no pida cambios (RFC 9110 evalua If-Match antes del metodo).
func TestE041PatchVacioEsNoOp(t *testing.T) {
	store := e041Sembrada(t)
	servicio := e041Servicio(store)

	actual, err := servicio.Update(context.Background(), appGoals.UpdateCommand{
		UserID: e038Usuario, GoalID: "meta-01", ExpectedVersion: 1,
	})
	if err != nil {
		t.Fatalf("un patch vacio debe ser no-op 200, obtenido %v", err)
	}
	if actual == nil || actual.Version != 1 {
		t.Fatalf("el no-op debe preservar la version 1, obtenido %+v", actual)
	}
	if actual.Name != store.porUsuario[e038Usuario][0].Goal.Name {
		t.Fatalf("el no-op no puede renombrar la meta: %q", actual.Name)
	}

	_, err = servicio.Update(context.Background(), appGoals.UpdateCommand{
		UserID: e038Usuario, GoalID: "meta-01", ExpectedVersion: 9,
	})
	if !errors.Is(err, appGoals.ErrVersionConflict) {
		t.Fatalf("un If-Match obsoleto debe ser 412 aun con cuerpo vacio, obtenido %v", err)
	}
	if got := store.porUsuario[e038Usuario][0].Goal.Version; got != 1 {
		t.Fatalf("ninguna de las dos llamadas puede escribir: version %d", got)
	}
}

// TestE041ConcurrenciaDosEscrituras simula dos nodos: la primera gana y la segunda recibe 412.
func TestE041ConcurrenciaDosEscrituras(t *testing.T) {
	store := e041Sembrada(t)
	servicio := e041Servicio(store)

	comando := appGoals.UpdateCommand{
		UserID: e038Usuario, GoalID: "meta-01", ExpectedVersion: 1, Name: e041Texto("primera"),
	}
	if _, err := servicio.Update(context.Background(), comando); err != nil {
		t.Fatalf("la primera escritura debe ganar: %v", err)
	}
	comando.Name = e041Texto("segunda")
	if _, err := servicio.Update(context.Background(), comando); !errors.Is(err, appGoals.ErrVersionConflict) {
		t.Fatalf("la segunda debe recibir 412, obtuvo %v", err)
	}
	if got := store.porUsuario[e038Usuario][0].Goal.Name; got != "primera" {
		t.Fatalf("la escritura perdedora no debe aplicarse: %q", got)
	}
	if got := store.porUsuario[e038Usuario][0].Goal.Version; got != 2 {
		t.Fatalf("la version debe ser 2: %d", got)
	}
}
