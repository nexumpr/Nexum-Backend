package application_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	appIdentity "github.com/nexumpr/Nexum-Backend/internal/application/identity"
)

// Pruebas de aplicación de E011: consumo del token, caducidad lógica, replay, perfil no ACTIVE,
// conflicto de versión y concurrencia de dos confirmaciones.

const (
	e011Usuario = "aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee"
	e011Token   = "token-sintetico-de-verificacion-0123456789"
)

var e011Ahora = time.Date(2026, 11, 17, 11, 0, 0, 0, time.UTC)

type e011Store struct {
	token      *appIdentity.EmailVerificationTokenRecord
	tokenErr   error
	profile    *appIdentity.EmailVerificationProfile
	profileErr error
	confirmErr error
	applied    []appIdentity.EmailVerificationConfirmation
	// trasConflicto permite simular que el perfil desaparece entre la lectura y el conflicto.
	trasConflicto bool
}

func (s *e011Store) LoadEmailVerificationProfile(context.Context, string) (*appIdentity.EmailVerificationProfile, error) {
	if s.trasConflicto && len(s.applied) > 0 {
		return nil, nil
	}
	return s.profile, s.profileErr
}

func (s *e011Store) SaveEmailVerificationToken(context.Context, appIdentity.EmailVerificationToken) error {
	return nil
}

func (s *e011Store) LoadEmailVerificationToken(context.Context, string) (*appIdentity.EmailVerificationTokenRecord, error) {
	return s.token, s.tokenErr
}

func (s *e011Store) ConfirmEmailVerification(_ context.Context, confirmation appIdentity.EmailVerificationConfirmation) error {
	s.applied = append(s.applied, confirmation)
	return s.confirmErr
}

func e011TokenValido() *appIdentity.EmailVerificationTokenRecord {
	return &appIdentity.EmailVerificationTokenRecord{
		UserID:     e011Usuario,
		Email:      "verificacion@example.invalid",
		Purpose:    appIdentity.EmailVerificationPurpose,
		EntityType: appIdentity.EmailVerificationEntityType,
		ExpiresAt:  e011Ahora.Add(10 * time.Minute),
		HasExpiry:  true,
	}
}

func e011PerfilActivo() *appIdentity.EmailVerificationProfile {
	return &appIdentity.EmailVerificationProfile{
		UserID:     e011Usuario,
		Email:      "verificacion@example.invalid",
		Status:     appIdentity.StatusActive,
		Version:    4,
		HasVersion: true,
	}
}

func e011Servicio(store *e011Store) *appIdentity.EmailVerificationConfirmService {
	return appIdentity.NewEmailVerificationConfirmService(store).
		WithClock(func() time.Time { return e011Ahora })
}

func TestE011ConfirmacionAplicaLaTransaccion(t *testing.T) {
	store := &e011Store{token: e011TokenValido(), profile: e011PerfilActivo()}
	if err := e011Servicio(store).Confirmar(context.Background(), e011Token); err != nil {
		t.Fatalf("confirmacion valida fallo: %v", err)
	}
	if len(store.applied) != 1 {
		t.Fatalf("debe aplicarse exactamente una transaccion, obtenido %d", len(store.applied))
	}
	aplicada := store.applied[0]
	if aplicada.UserID != e011Usuario || aplicada.TokenHash != appIdentity.HashToken(e011Token) {
		t.Fatalf("la transaccion debe ligar usuario y sha256 del token: %+v", aplicada)
	}
	if !aplicada.HasVersion || aplicada.ExpectedVersion != 4 {
		t.Fatalf("CAS de version esperado 4, obtenido %+v", aplicada)
	}
	if !aplicada.Now.Equal(e011Ahora) {
		t.Fatalf("la transaccion debe usar el reloj del servicio")
	}
}

func TestE011ReplayDevuelveConflicto(t *testing.T) {
	token := e011TokenValido()
	token.Used = true
	store := &e011Store{token: token, profile: e011PerfilActivo()}

	err := e011Servicio(store).Confirmar(context.Background(), e011Token)
	if !errors.Is(err, appIdentity.ErrEmailVerificationTokenUsado) {
		t.Fatalf("se esperaba ErrEmailVerificationTokenUsado, obtenido %v", err)
	}
	if len(store.applied) != 0 {
		t.Fatalf("un token usado no puede aplicar la transaccion")
	}
}

func TestE011TokenExpiradoInexistenteOAjeno(t *testing.T) {
	casos := []struct {
		nombre   string
		preparar func(*e011Store)
	}{
		{"expirado", func(s *e011Store) {
			s.token.ExpiresAt = e011Ahora
		}},
		{"sin caducidad", func(s *e011Store) {
			s.token.HasExpiry = false
		}},
		{"inexistente", func(s *e011Store) {
			s.token = nil
		}},
		{"proposito distinto", func(s *e011Store) {
			s.token.Purpose = "PASSWORD_RESET"
		}},
		{"entidad distinta", func(s *e011Store) {
			s.token.EntityType = "PASSWORD_RESET"
		}},
		{"sin usuario", func(s *e011Store) {
			s.token.UserID = " "
		}},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			store := &e011Store{token: e011TokenValido(), profile: e011PerfilActivo()}
			caso.preparar(store)
			err := e011Servicio(store).Confirmar(context.Background(), e011Token)
			if !errors.Is(err, appIdentity.ErrEmailVerificationTokenInvalido) {
				t.Fatalf("se esperaba rechazo generico, obtenido %v", err)
			}
			if len(store.applied) != 0 {
				t.Fatalf("un token invalido no puede aplicar la transaccion")
			}
		})
	}
}

func TestE011PerfilNoApto(t *testing.T) {
	casos := []struct {
		nombre   string
		preparar func(*e011Store)
		espera   error
	}{
		{"perfil ausente", func(s *e011Store) { s.profile = nil }, appIdentity.ErrEmailVerificationTokenInvalido},
		{"cuenta inactiva", func(s *e011Store) { s.profile.Status = "DISABLED" }, appIdentity.ErrEmailVerificationTokenInvalido},
		{"ya verificado", func(s *e011Store) { s.profile.EmailVerified = true }, appIdentity.ErrEmailVerificationConflicto},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			store := &e011Store{token: e011TokenValido(), profile: e011PerfilActivo()}
			caso.preparar(store)
			err := e011Servicio(store).Confirmar(context.Background(), e011Token)
			if !errors.Is(err, caso.espera) {
				t.Fatalf("se esperaba %v, obtenido %v", caso.espera, err)
			}
			if len(store.applied) != 0 {
				t.Fatalf("un perfil no apto no puede aplicar la transaccion")
			}
		})
	}
}

func TestE011ConflictoDeTransaccion(t *testing.T) {
	store := &e011Store{token: e011TokenValido(), profile: e011PerfilActivo(), confirmErr: appIdentity.ErrEmailVerificationConflicto}
	if err := e011Servicio(store).Confirmar(context.Background(), e011Token); !errors.Is(err, appIdentity.ErrEmailVerificationConflicto) {
		t.Fatalf("un conflicto con perfil existente debe ser 409, obtenido %v", err)
	}

	desaparece := &e011Store{
		token:         e011TokenValido(),
		profile:       e011PerfilActivo(),
		confirmErr:    appIdentity.ErrEmailVerificationConflicto,
		trasConflicto: true,
	}
	if err := e011Servicio(desaparece).Confirmar(context.Background(), e011Token); !errors.Is(err, appIdentity.ErrEmailVerificationTokenInvalido) {
		t.Fatalf("si el perfil desaparece el rechazo debe ser generico, obtenido %v", err)
	}
}

func TestE011FallaCerrado(t *testing.T) {
	casos := []struct {
		nombre string
		store  *e011Store
	}{
		{"error al leer token", &e011Store{tokenErr: errors.New("dynamodb caido"), profile: e011PerfilActivo()}},
		{"error al leer perfil", &e011Store{token: e011TokenValido(), profileErr: errors.New("dynamodb caido")}},
		{"error de transaccion", &e011Store{token: e011TokenValido(), profile: e011PerfilActivo(), confirmErr: errors.New("dynamodb caido")}},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			err := e011Servicio(caso.store).Confirmar(context.Background(), e011Token)
			if !errors.Is(err, appIdentity.ErrEmailVerificationConfirmNoDisponible) {
				t.Fatalf("se esperaba ErrEmailVerificationConfirmNoDisponible, obtenido %v", err)
			}
		})
	}

	if err := appIdentity.NewEmailVerificationConfirmService(nil).Confirmar(context.Background(), e011Token); !errors.Is(err, appIdentity.ErrEmailVerificationConfirmNoDisponible) {
		t.Fatalf("sin store debe fallar cerrado, obtenido %v", err)
	}
}

func TestE011ValidacionDelToken(t *testing.T) {
	casos := []struct {
		nombre string
		token  string
	}{
		{"vacio", ""},
		{"demasiado largo", strings.Repeat("a", 513)},
		{"con espacio", "token con espacio"},
		{"con control", "token\x00valido"},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			_, err := appIdentity.ValidateVerifyEmailToken(caso.token)
			var violation *appIdentity.FieldViolation
			if !errors.As(err, &violation) || violation.Field != "token" {
				t.Fatalf("se esperaba FieldViolation en token, obtenido %v", err)
			}
		})
	}
	if _, err := appIdentity.ValidateVerifyEmailToken("token-valido-0123456789"); err != nil {
		t.Fatalf("un token valido no debe rechazarse: %v", err)
	}
}

// e011Estado implementa la transacción real sobre estado en memoria: dos confirmaciones
// simultáneas compiten por el mismo token y solo una puede ganar.
type e011Estado struct {
	mu      sync.Mutex
	perfil  appIdentity.EmailVerificationProfile
	token   appIdentity.EmailVerificationTokenRecord
}

func (s *e011Estado) LoadEmailVerificationProfile(context.Context, string) (*appIdentity.EmailVerificationProfile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	copia := s.perfil
	return &copia, nil
}

func (s *e011Estado) SaveEmailVerificationToken(context.Context, appIdentity.EmailVerificationToken) error {
	return nil
}

func (s *e011Estado) LoadEmailVerificationToken(context.Context, string) (*appIdentity.EmailVerificationTokenRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	copia := s.token
	return &copia, nil
}

func (s *e011Estado) ConfirmEmailVerification(_ context.Context, confirmation appIdentity.EmailVerificationConfirmation) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.token.Used {
		return appIdentity.ErrEmailVerificationTokenUsado
	}
	if !s.token.ExpiresAt.After(confirmation.Now) {
		return appIdentity.ErrEmailVerificationTokenUsado
	}
	if s.perfil.EmailVerified {
		return appIdentity.ErrEmailVerificationConflicto
	}
	if s.perfil.Version != confirmation.ExpectedVersion {
		return appIdentity.ErrEmailVerificationConflicto
	}
	s.token.Used = true
	s.perfil.EmailVerified = true
	s.perfil.Version++
	return nil
}

func TestE011ConcurrenciaUnSoloGanador(t *testing.T) {
	estado := &e011Estado{
		perfil: *e011PerfilActivo(),
		token:  *e011TokenValido(),
	}
	servicio := appIdentity.NewEmailVerificationConfirmService(estado).
		WithClock(func() time.Time { return e011Ahora })

	var wg sync.WaitGroup
	resultados := make([]error, 2)
	for i := range resultados {
		wg.Add(1)
		go func(indice int) {
			defer wg.Done()
			resultados[indice] = servicio.Confirmar(context.Background(), e011Token)
		}(i)
	}
	wg.Wait()

	exitos, conflictos := 0, 0
	for _, err := range resultados {
		switch {
		case err == nil:
			exitos++
		case errors.Is(err, appIdentity.ErrEmailVerificationTokenUsado), errors.Is(err, appIdentity.ErrEmailVerificationConflicto):
			conflictos++
		default:
			t.Fatalf("error inesperado en concurrencia: %v", err)
		}
	}
	if exitos != 1 || conflictos != 1 {
		t.Fatalf("solo una confirmacion puede ganar: exitos=%d conflictos=%d", exitos, conflictos)
	}
	if !estado.perfil.EmailVerified || !estado.token.Used {
		t.Fatalf("el ganador debe marcar el correo verificado y consumir el token")
	}
}
