package application_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	appProfile "github.com/nexumpr/Nexum-Backend/internal/application/profile"
)

// Pruebas de aplicación de E012 (`GET /api/v1/users/me`): proyección de los seis campos del
// contrato, identidad obligatoria, perfil inexistente, item incoherente y fallo cerrado sin
// persistencia. El almacén es un doble de prueba; la composición productiva usa DynamoDB real.

const (
	e012Usuario = "11111111-2222-4333-8444-555555555555"
	e012Otro    = "99999999-8888-4777-8666-555555555555"
)

var e012Ahora = time.Date(2026, 11, 18, 9, 30, 0, 0, time.UTC)

// e012Store es el doble del puerto: devuelve filas preparadas o un error y registra qué
// usuario recibió, para probar que la identidad no se puede sustituir.
type e012Store struct {
	registros map[string]*appProfile.Record
	fallo     error
	consultas []string
}

func (s *e012Store) LoadProfile(_ context.Context, userID string) (*appProfile.Record, error) {
	s.consultas = append(s.consultas, userID)
	if s.fallo != nil {
		return nil, s.fallo
	}
	return s.registros[userID], nil
}

func e012PerfilValido(userID string) *appProfile.Record {
	return &appProfile.Record{
		UserID:        userID,
		Email:         "ejemplo@example.invalid",
		DisplayName:   "Usuario Ejemplo",
		EmailVerified: true,
		CreatedAt:     e012Ahora.Add(-48 * time.Hour),
		HasCreatedAt:  true,
		Version:       7,
		HasVersion:    true,
	}
}

// TestE012GetProyectaContrato cubre T01: los seis campos exactos con la versión persistida.
func TestE012GetProyectaContrato(t *testing.T) {
	store := &e012Store{registros: map[string]*appProfile.Record{
		e012Usuario: e012PerfilValido(e012Usuario),
	}}
	servicio := appProfile.NewService(store).WithClock(func() time.Time { return e012Ahora })

	perfil, err := servicio.Get(context.Background(), e012Usuario)
	if err != nil {
		t.Fatalf("Get devolvio error: %v", err)
	}
	if perfil.ID != e012Usuario || perfil.Email != "ejemplo@example.invalid" ||
		perfil.DisplayName != "Usuario Ejemplo" || !perfil.EmailVerified {
		t.Fatalf("proyeccion inesperada: %+v", perfil)
	}
	if perfil.Version != 7 {
		t.Fatalf("la version debe ser la persistida (7), es %d", perfil.Version)
	}
	if !perfil.CreatedAt.Equal(e012Ahora.Add(-48 * time.Hour)) {
		t.Fatalf("createdAt inesperado: %s", perfil.CreatedAt)
	}
	if len(store.consultas) != 1 || store.consultas[0] != e012Usuario {
		t.Fatalf("la persistencia debe recibir el sujeto autenticado, recibio %v", store.consultas)
	}
}

// TestE012GetIdentidadObligatoria verifica que sin sujeto no se consulta la persistencia.
func TestE012GetIdentidadObligatoria(t *testing.T) {
	store := &e012Store{registros: map[string]*appProfile.Record{}}
	servicio := appProfile.NewService(store)

	for _, userID := range []string{"", "   "} {
		if _, err := servicio.Get(context.Background(), userID); !errors.Is(err, appProfile.ErrProfileUnauthorized) {
			t.Fatalf("userID %q debe producir 401, obtuvo %v", userID, err)
		}
	}
	if len(store.consultas) != 0 {
		t.Fatalf("no debe consultarse la persistencia sin identidad: %v", store.consultas)
	}
}

// TestE012GetAislamientoEntreUsuarios cubre T03: cada sujeto lee su propia fila y un perfil
// inexistente responde 404 sin revelar si el otro usuario existe.
func TestE012GetAislamientoEntreUsuarios(t *testing.T) {
	store := &e012Store{registros: map[string]*appProfile.Record{
		e012Usuario: e012PerfilValido(e012Usuario),
	}}
	servicio := appProfile.NewService(store)

	if _, err := servicio.Get(context.Background(), e012Usuario); err != nil {
		t.Fatalf("el titular debe leer su perfil: %v", err)
	}
	if _, err := servicio.Get(context.Background(), e012Otro); !errors.Is(err, appProfile.ErrProfileNotFound) {
		t.Fatalf("un perfil inexistente debe ser 404, obtuvo %v", err)
	}
	if len(store.consultas) != 2 || store.consultas[1] != e012Otro {
		t.Fatalf("la segunda consulta debe usar el segundo sujeto: %v", store.consultas)
	}
}

// TestE012GetItemSinVersionExponeV1 garantiza que un item heredado sin `version` no emite 0,
// que violaría `minimum: 1` del contrato.
func TestE012GetItemSinVersionExponeV1(t *testing.T) {
	registro := e012PerfilValido(e012Usuario)
	registro.Version = 0
	registro.HasVersion = false
	store := &e012Store{registros: map[string]*appProfile.Record{e012Usuario: registro}}

	perfil, err := appProfile.NewService(store).Get(context.Background(), e012Usuario)
	if err != nil {
		t.Fatalf("Get devolvio error: %v", err)
	}
	if perfil.Version != 1 {
		t.Fatalf("un item sin version debe exponerse como v1, es %d", perfil.Version)
	}
}

// TestE012GetItemSinCreatedAtEsIncoherente: sin `createdAt` legible no se inventa una fecha.
func TestE012GetItemSinCreatedAtEsIncoherente(t *testing.T) {
	registro := e012PerfilValido(e012Usuario)
	registro.HasCreatedAt = false
	store := &e012Store{registros: map[string]*appProfile.Record{e012Usuario: registro}}

	if _, err := appProfile.NewService(store).Get(context.Background(), e012Usuario); !errors.Is(err, appProfile.ErrProfileInternal) {
		t.Fatalf("un item sin createdAt debe ser incoherente, obtuvo %v", err)
	}
}

// TestE012FallosDePersistenciaSon503 cubre T04: un fallo de DynamoDB no se degrada a datos
// inventados ni a 404.
func TestE012FallosDePersistenciaSon503(t *testing.T) {
	store := &e012Store{fallo: errors.New("dynamodb no disponible")}
	if _, err := appProfile.NewService(store).Get(context.Background(), e012Usuario); !errors.Is(err, appProfile.ErrProfileUnavailable) {
		t.Fatalf("un fallo de persistencia debe ser 503, obtuvo %v", err)
	}

	if _, err := appProfile.NewService(nil).Get(context.Background(), e012Usuario); !errors.Is(err, appProfile.ErrProfileUnavailable) {
		t.Fatalf("sin almacen el servicio debe fallar cerrado, obtuvo %v", err)
	}
}

// TestE012SinDatosSensiblesEnErrores: los mensajes de error no incluyen correo ni nombres.
func TestE012SinDatosSensiblesEnErrores(t *testing.T) {
	registro := e012PerfilValido(e012Usuario)
	registro.HasCreatedAt = false
	store := &e012Store{registros: map[string]*appProfile.Record{e012Usuario: registro}}
	_, err := appProfile.NewService(store).Get(context.Background(), e012Usuario)
	if err == nil {
		t.Fatal("se esperaba error")
	}
	for _, prohibido := range []string{"ejemplo@example.invalid", "Usuario Ejemplo", e012Usuario} {
		if strings.Contains(err.Error(), prohibido) {
			t.Fatalf("el error no debe contener %q: %v", prohibido, err)
		}
	}
}
