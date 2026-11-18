package application_test

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	appOnboarding "github.com/nexumpr/Nexum-Backend/internal/application/onboarding"
)

// Pruebas de aplicación de E018 (`GET /api/v1/users/me/onboarding`): proyección
// exacta al contrato, ausencia de estado como 404, aislamiento por usuario y
// fail-closed sin persistencia. Los dobles son exclusivamente de prueba; la
// composición productiva usa DynamoDB real.

const (
	e018UsuarioA = "11111111-2222-4333-8444-555555555555"
	e018UsuarioB = "99999999-8888-4777-8666-555555555555"
)

var e018Ahora = time.Date(2026, 11, 18, 9, 0, 0, 0, time.UTC)

// e018Almacen es el puerto `Store` en memoria, indexado por usuario autenticado.
type e018Almacen struct {
	mu        sync.Mutex
	registros map[string]*appOnboarding.Record
	fallo     error
	lecturas  int
}

func newE018Almacen() *e018Almacen {
	return &e018Almacen{registros: map[string]*appOnboarding.Record{}}
}

func (a *e018Almacen) GetOnboarding(_ context.Context, userID string) (*appOnboarding.Record, bool, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.lecturas++
	if a.fallo != nil {
		return nil, false, a.fallo
	}
	registro, ok := a.registros[userID]
	if !ok {
		return nil, false, nil
	}
	copia := *registro
	return &copia, true, nil
}

func e018Sembrar(a *e018Almacen, userID string, registro appOnboarding.Record) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.registros[userID] = &registro
}

func e018Servicio(almacen appOnboarding.Store) *appOnboarding.Service {
	return appOnboarding.NewService(almacen).WithClock(func() time.Time { return e018Ahora })
}

// TestE018GetNoEncontrado cubre el 404: sin clave persistida no se inventa un
// estado vacío.
func TestE018GetNoEncontrado(t *testing.T) {
	servicio := e018Servicio(newE018Almacen())
	if _, err := servicio.Get(context.Background(), e018UsuarioA); !errors.Is(err, appOnboarding.ErrNotFound) {
		t.Fatalf("sin estado persistido debe devolver ErrNotFound, obtenido %v", err)
	}
}

// TestE018GetSinAlmacen cubre el fail-closed 503: sin persistencia no hay lectura.
func TestE018GetSinAlmacen(t *testing.T) {
	servicio := appOnboarding.NewService(nil)
	if _, err := servicio.Get(context.Background(), e018UsuarioA); !errors.Is(err, appOnboarding.ErrUnavailable) {
		t.Fatalf("sin almacen debe devolver ErrUnavailable, obtenido %v", err)
	}
}

// TestE018GetSinUsuario es defensa en profundidad: el middleware ya exige `sub`.
func TestE018GetSinUsuario(t *testing.T) {
	servicio := e018Servicio(newE018Almacen())
	if _, err := servicio.Get(context.Background(), "   "); !errors.Is(err, appOnboarding.ErrUnauthorized) {
		t.Fatalf("sin identidad debe devolver ErrUnauthorized, obtenido %v", err)
	}
}

// TestE018GetFalloDeAlmacen: un error de DynamoDB nunca se degrada a 404.
func TestE018GetFalloDeAlmacen(t *testing.T) {
	almacen := newE018Almacen()
	almacen.fallo = errors.New("dynamodb no disponible")
	servicio := e018Servicio(almacen)
	if _, err := servicio.Get(context.Background(), e018UsuarioA); !errors.Is(err, appOnboarding.ErrUnavailable) {
		t.Fatalf("un fallo de almacen debe devolver ErrUnavailable, obtenido %v", err)
	}
}

// TestE018GetProyeccion cubre la reducción exacta: pasos canónicos sin duplicados,
// permisos legacy (`pending`/`denied` → false), `completedAt` nulo mientras
// `done=false` y versión leída.
func TestE018GetProyeccion(t *testing.T) {
	almacen := newE018Almacen()
	marca := e018Ahora.Add(-2 * time.Hour)
	e018Sembrar(almacen, e018UsuarioA, appOnboarding.Record{
		CompletedSteps:   []string{"income", "perfil", "profile", "profile", "desconocido"},
		Version:          7,
		HasVersion:       true,
		MicrophoneRaw:    "granted",
		NotificationsRaw: "pending",
		Configured:       true,
		CompletedAt:      &marca,
	})

	state, err := e018Servicio(almacen).Get(context.Background(), e018UsuarioA)
	if err != nil {
		t.Fatalf("lectura valida no debe fallar: %v", err)
	}
	if want := []string{"profile", "income"}; !reflect.DeepEqual(state.CompletedSteps, want) {
		t.Fatalf("completedSteps debe ser %v, obtenido %v", want, state.CompletedSteps)
	}
	if state.Done {
		t.Fatalf("con dos pasos done debe ser false")
	}
	if state.Version != 7 {
		t.Fatalf("version debe ser 7, obtenido %d", state.Version)
	}
	if !state.Microphone {
		t.Fatalf("microphone='granted' debe proyectarse true")
	}
	if state.Notifications {
		t.Fatalf("notifications='pending' debe proyectarse false")
	}
	if !state.Configured {
		t.Fatalf("configured persistido true debe proyectarse true")
	}
	if state.CompletedAt != nil {
		t.Fatalf("completedAt debe ser null mientras done=false, obtenido %v", state.CompletedAt)
	}
}

// TestE018GetCompletoConservaMarca: con los cuatro pasos se emite `done=true` y se
// conserva la marca persistida.
func TestE018GetCompletoConservaMarca(t *testing.T) {
	almacen := newE018Almacen()
	marca := e018Ahora.Add(-48 * time.Hour)
	e018Sembrar(almacen, e018UsuarioA, appOnboarding.Record{
		CompletedSteps: []string{"profile", "income", "privacy", "permissions"},
		Version:        3,
		HasVersion:     true,
		CompletedAt:    &marca,
	})

	state, err := e018Servicio(almacen).Get(context.Background(), e018UsuarioA)
	if err != nil {
		t.Fatalf("lectura valida no debe fallar: %v", err)
	}
	if !state.Done {
		t.Fatalf("con los cuatro pasos done debe ser true")
	}
	if state.CompletedAt == nil || !state.CompletedAt.Equal(marca) {
		t.Fatalf("completedAt debe conservarse (%v), obtenido %v", marca, state.CompletedAt)
	}
}

// TestE018GetLegacySinVersion: un ítem heredado sin atributo `version` se proyecta
// con versión lógica 1; el CAS de E019 materializa el atributo en la escritura.
func TestE018GetLegacySinVersion(t *testing.T) {
	almacen := newE018Almacen()
	e018Sembrar(almacen, e018UsuarioA, appOnboarding.Record{
		CompletedSteps:   []string{"profile"},
		MicrophoneRaw:    "denied",
		NotificationsRaw: "denied",
	})

	state, err := e018Servicio(almacen).Get(context.Background(), e018UsuarioA)
	if err != nil {
		t.Fatalf("lectura valida no debe fallar: %v", err)
	}
	if state.Version != 1 {
		t.Fatalf("la version logica de un item heredado debe ser 1, obtenido %d", state.Version)
	}
	if state.Microphone || state.Notifications {
		t.Fatalf("denied debe proyectarse false en ambos permisos")
	}
}

// TestE018GetAislamiento: la lectura se resuelve por el usuario autenticado. El
// usuario B no observa el estado de A.
func TestE018GetAislamiento(t *testing.T) {
	almacen := newE018Almacen()
	e018Sembrar(almacen, e018UsuarioA, appOnboarding.Record{
		CompletedSteps: []string{"profile", "income", "privacy", "permissions"},
		Version:        4,
		HasVersion:     true,
	})
	servicio := e018Servicio(almacen)

	stateA, err := servicio.Get(context.Background(), e018UsuarioA)
	if err != nil || !stateA.Done {
		t.Fatalf("A debe observar su estado completo: state=%v err=%v", stateA, err)
	}
	if _, err := servicio.Get(context.Background(), e018UsuarioB); !errors.Is(err, appOnboarding.ErrNotFound) {
		t.Fatalf("B no debe observar el estado de A, obtenido %v", err)
	}
}

// TestE018CanonicalSteps garantiza que la proyección nunca devuelve nil y que el
// orden y la deduplicación son deterministas.
func TestE018CanonicalSteps(t *testing.T) {
	casos := []struct {
		nombre   string
		entrada  []string
		esperado []string
	}{
		{"nil", nil, []string{}},
		{"vacio", []string{}, []string{}},
		{"duplicados y orden", []string{"permissions", "profile", "permissions", "income"}, []string{"profile", "income", "permissions"}},
		{"desconocidos", []string{"otro", "privacy"}, []string{"privacy"}},
		{"mayusculas y espacios", []string{" Privacy ", "INCOME"}, []string{"income", "privacy"}},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			obtenido := appOnboarding.CanonicalSteps(caso.entrada)
			if obtenido == nil {
				t.Fatalf("CanonicalSteps no debe devolver nil")
			}
			if !reflect.DeepEqual(obtenido, caso.esperado) {
				t.Fatalf("esperado %v, obtenido %v", caso.esperado, obtenido)
			}
		})
	}

	if appOnboarding.IsComplete([]string{"profile", "income", "privacy"}) {
		t.Fatalf("tres pasos no pueden considerarse completos")
	}
	if !appOnboarding.IsComplete([]string{"permissions", "privacy", "income", "profile"}) {
		t.Fatalf("los cuatro pasos en cualquier orden estan completos")
	}
}
