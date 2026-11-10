package application_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nexumpr/Nexum-Backend/internal/adapters/memory"
	appIdentity "github.com/nexumpr/Nexum-Backend/internal/application/identity"
	"github.com/nexumpr/Nexum-Backend/internal/infrastructure/security"
	"github.com/nexumpr/Nexum-Backend/internal/ports"
	memorystore "github.com/nexumpr/Nexum-Backend/tests/support/memorystore"
)

const fixedUserID = "018f90ef-08e5-7a68-b09f-2b67a47c92f0"

func fixedClock() time.Time {
	return time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
}

func newFixedService(store appIdentity.RegisterStore, hasher ports.PasswordHasher, id string) *appIdentity.RegisterService {
	return appIdentity.NewRegisterService(store, hasher).
		WithClock(fixedClock).
		WithIDGenerator(func() (string, error) { return id, nil })
}

const syntheticPhrase = "ExamplePassphrase#2026"

func validCommand(key string) appIdentity.RegisterCommand {
	return appIdentity.RegisterCommand{
		Email:                "usuario@example.invalid",
		Password:             syntheticPhrase,
		DisplayName:          "Usuario Ejemplo",
		PrivacyNoticeVersion: "2026-11",
		PrivacyAccepted:      true,
		IdempotencyKey:       key,
	}
}

func idempotencyKey(prefix string, n int) string {
	return fmt.Sprintf("%s-%016d", prefix, n)
}

func TestRegisterServiceCreatesProfileAndHashesPassword(t *testing.T) {
	store := memorystore.NewRegistrationStore()
	svc := newFixedService(store, security.NewArgon2Hasher(nil), fixedUserID)

	result, err := svc.Register(context.Background(), validCommand(idempotencyKey("register", 1)))
	if err != nil {
		t.Fatalf("registro fallo: %v", err)
	}
	if result.Status != 201 {
		t.Fatalf("status esperado 201, obtenido %d", result.Status)
	}
	if result.Location != "/api/v1/users/me" {
		t.Fatalf("Location inesperado: %q", result.Location)
	}
	if result.Replayed {
		t.Fatal("la primera respuesta no debe marcarse como replay")
	}

	var profile appIdentity.UserProfile
	if err := json.Unmarshal(result.Body, &profile); err != nil {
		t.Fatalf("respuesta no es JSON valido: %v", err)
	}
	if profile.ID != fixedUserID {
		t.Fatalf("id inesperado: %q", profile.ID)
	}
	if profile.Email != "usuario@example.invalid" {
		t.Fatalf("email inesperado: %q", profile.Email)
	}
	if profile.DisplayName != "Usuario Ejemplo" {
		t.Fatalf("displayName inesperado: %q", profile.DisplayName)
	}
	if profile.EmailVerified {
		t.Fatal("emailVerified debe ser false")
	}
	if profile.CreatedAt != "2026-11-10T12:00:00Z" {
		t.Fatalf("createdAt inesperado: %q", profile.CreatedAt)
	}
	if profile.Version != 1 {
		t.Fatalf("version esperada 1, obtenida %d", profile.Version)
	}

	record, ok := store.Registration(fixedUserID)
	if !ok {
		t.Fatal("no se persistio el registro")
	}
	if record.PasswordHash == syntheticPhrase {
		t.Fatal("la contrasena se persistio en claro")
	}
	if len(record.PasswordHash) < 20 || record.PasswordHash[:10] != "$argon2id$" {
		t.Fatalf("hash Argon2id inesperado: %q", record.PasswordHash)
	}
	if record.EmailVerified {
		t.Fatal("el registro persistido debe tener emailVerified=false")
	}
	if record.PrivacyNoticeVersion != "2026-11" {
		t.Fatalf("version de privacidad inesperada: %q", record.PrivacyNoticeVersion)
	}
}

func TestRegisterServiceReplaysSameKeyAndPayload(t *testing.T) {
	store := memorystore.NewRegistrationStore()
	svc := newFixedService(store, &mockSimpleHasher{}, fixedUserID)

	first, err := svc.Register(context.Background(), validCommand(idempotencyKey("replay", 1)))
	if err != nil {
		t.Fatalf("primer registro fallo: %v", err)
	}
	second, err := svc.Register(context.Background(), validCommand(idempotencyKey("replay", 1)))
	if err != nil {
		t.Fatalf("replay fallo: %v", err)
	}

	if first.Replayed {
		t.Fatal("el primer resultado no debe ser replay")
	}
	if !second.Replayed {
		t.Fatal("el segundo resultado debe ser replay")
	}
	if !bytes.Equal(first.Body, second.Body) {
		t.Fatalf("el replay no es byte a byte estable:\n%s\n%s", first.Body, second.Body)
	}
	if store.RecordsCount() != 1 {
		t.Fatalf("se esperaba un unico registro, obtenidos %d", store.RecordsCount())
	}
}

func TestRegisterServiceRejectsSameKeyDifferentPayload(t *testing.T) {
	store := memorystore.NewRegistrationStore()
	svc := newFixedService(store, &mockSimpleHasher{}, fixedUserID)

	if _, err := svc.Register(context.Background(), validCommand(idempotencyKey("conflict", 1))); err != nil {
		t.Fatalf("primer registro fallo: %v", err)
	}

	other := validCommand(idempotencyKey("conflict", 1))
	other.DisplayName = "Otro Nombre"
	_, err := svc.Register(context.Background(), other)
	if !errors.Is(err, appIdentity.ErrIdempotencyConflict) {
		t.Fatalf("se esperaba ErrIdempotencyConflict, obtenido %v", err)
	}
}

func TestRegisterServiceRejectsDuplicateEmail(t *testing.T) {
	store := memorystore.NewRegistrationStore()
	var seq atomic.Int64
	svc := appIdentity.NewRegisterService(store, &mockSimpleHasher{}).
		WithClock(fixedClock).
		WithIDGenerator(func() (string, error) {
			return fmt.Sprintf("00000000-0000-4000-8000-%012d", seq.Add(1)), nil
		})

	first := validCommand(idempotencyKey("email", 1))
	if _, err := svc.Register(context.Background(), first); err != nil {
		t.Fatalf("primer registro fallo: %v", err)
	}

	second := validCommand(idempotencyKey("email", 2))
	second.Email = "USUARIO@example.invalid"
	_, err := svc.Register(context.Background(), second)
	if !errors.Is(err, appIdentity.ErrEmailAlreadyRegistered) {
		t.Fatalf("se esperaba ErrEmailAlreadyRegistered, obtenido %v", err)
	}
}

func TestRegisterServiceConcurrentSameKey(t *testing.T) {
	store := memorystore.NewRegistrationStore()
	var seq atomic.Int64
	svc := appIdentity.NewRegisterService(store, &mockSimpleHasher{}).
		WithClock(fixedClock).
		WithIDGenerator(func() (string, error) {
			return fmt.Sprintf("00000000-0000-4000-8000-%012d", seq.Add(1)), nil
		})

	const workers = 64
	var wg sync.WaitGroup
	results := make(chan struct {
		body []byte
		err  error
	}, workers)

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := svc.Register(context.Background(), validCommand(idempotencyKey("same-key", 1)))
			if err != nil {
				results <- struct {
					body []byte
					err  error
				}{err: err}
				return
			}
			results <- struct {
				body []byte
				err  error
			}{body: result.Body}
		}()
	}
	wg.Wait()
	close(results)

	var expected []byte
	for result := range results {
		if result.err != nil {
			t.Fatalf("una peticion concurrente fallo: %v", result.err)
		}
		if expected == nil {
			expected = result.body
			continue
		}
		if !bytes.Equal(expected, result.body) {
			t.Fatalf("respuestas divergentes en concurrencia:\n%s\n%s", expected, result.body)
		}
	}
	if store.RecordsCount() != 1 {
		t.Fatalf("se esperaba un unico usuario, obtenidos %d", store.RecordsCount())
	}
}

func TestRegisterServiceConcurrentSameEmailDifferentKeys(t *testing.T) {
	store := memorystore.NewRegistrationStore()
	var seq atomic.Int64
	svc := appIdentity.NewRegisterService(store, &mockSimpleHasher{}).
		WithClock(fixedClock).
		WithIDGenerator(func() (string, error) {
			return fmt.Sprintf("00000000-0000-4000-8000-%012d", seq.Add(1)), nil
		})

	const workers = 64
	var wg sync.WaitGroup
	errs := make(chan error, workers)

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			_, err := svc.Register(context.Background(), validCommand(idempotencyKey("email-race", n)))
			errs <- err
		}(i)
	}
	wg.Wait()
	close(errs)

	success := 0
	conflicts := 0
	for err := range errs {
		switch {
		case err == nil:
			success++
		case errors.Is(err, appIdentity.ErrEmailAlreadyRegistered):
			conflicts++
		default:
			t.Fatalf("error inesperado en carrera de email: %v", err)
		}
	}
	if success != 1 || conflicts != workers-1 {
		t.Fatalf("se esperaba 1 exito y %d conflictos, obtenidos %d y %d", workers-1, success, conflicts)
	}
}

func TestRegisterServiceConcurrentSameKeyDifferentPayloads(t *testing.T) {
	store := memorystore.NewRegistrationStore()
	var seq atomic.Int64
	svc := appIdentity.NewRegisterService(store, &mockSimpleHasher{}).
		WithClock(fixedClock).
		WithIDGenerator(func() (string, error) {
			return fmt.Sprintf("00000000-0000-4000-8000-%012d", seq.Add(1)), nil
		})

	const workers = 32
	var wg sync.WaitGroup
	results := make(chan struct {
		body []byte
		err  error
	}, workers)

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			cmd := validCommand(idempotencyKey("payload-race", 1))
			if n%2 == 0 {
				cmd.DisplayName = "Payload A"
			} else {
				cmd.DisplayName = "Payload B"
			}
			result, err := svc.Register(context.Background(), cmd)
			if err != nil {
				results <- struct {
					body []byte
					err  error
				}{err: err}
				return
			}
			results <- struct {
				body []byte
				err  error
			}{body: result.Body}
		}(i)
	}
	wg.Wait()
	close(results)

	success := 0
	conflicts := 0
	var winningBody []byte
	for result := range results {
		switch {
		case result.err == nil:
			success++
			if winningBody == nil {
				winningBody = result.body
			} else if !bytes.Equal(winningBody, result.body) {
				t.Fatalf("dos payloads distintos lograron exito con la misma clave")
			}
		case errors.Is(result.err, appIdentity.ErrIdempotencyConflict):
			conflicts++
		default:
			t.Fatalf("error inesperado en carrera de payload: %v", result.err)
		}
	}
	if success < 1 {
		t.Fatal("se esperaba al menos un exito para el payload ganador")
	}
	if conflicts != workers-success {
		t.Fatalf("conflictos esperados %d, obtenidos %d", workers-success, conflicts)
	}
}

func TestRegisterServiceStoreUnavailable(t *testing.T) {
	store := memorystore.NewRegistrationStore()
	store.SetUnavailable(true)
	svc := newFixedService(store, &mockSimpleHasher{}, fixedUserID)

	_, err := svc.Register(context.Background(), validCommand(idempotencyKey("unavailable", 1)))
	if !errors.Is(err, appIdentity.ErrStoreUnavailable) {
		t.Fatalf("se esperaba ErrStoreUnavailable, obtenido %v", err)
	}
}

func TestRegisterServiceValidation(t *testing.T) {
	store := memorystore.NewRegistrationStore()
	svc := newFixedService(store, &mockSimpleHasher{}, fixedUserID)

	cases := []struct {
		name     string
		mutate   func(*appIdentity.RegisterCommand)
		field    string
		business bool
	}{
		{"email invalido", func(c *appIdentity.RegisterCommand) { c.Email = "sin-arroba" }, "email", false},
		{"password corta", func(c *appIdentity.RegisterCommand) { c.Password = "corta" }, "password", false},
		{"password vacia", func(c *appIdentity.RegisterCommand) { c.Password = "            " }, "password", true},
		{"display vacio", func(c *appIdentity.RegisterCommand) { c.DisplayName = "   " }, "displayName", true},
		{"display largo", func(c *appIdentity.RegisterCommand) { c.DisplayName = strings.Repeat("a", 101) }, "displayName", false},
		{"privacy vacia", func(c *appIdentity.RegisterCommand) { c.PrivacyNoticeVersion = " " }, "privacyNoticeVersion", false},
		{"privacy no aceptada", func(c *appIdentity.RegisterCommand) { c.PrivacyAccepted = false }, "privacyAccepted", true},
		{"idempotency corta", func(c *appIdentity.RegisterCommand) { c.IdempotencyKey = "corta" }, "Idempotency-Key", false},
		{"idempotency con control", func(c *appIdentity.RegisterCommand) { c.IdempotencyKey = "0123456789abcde\x00" }, "Idempotency-Key", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cmd := validCommand(idempotencyKey("validation", 1))
			tc.mutate(&cmd)
			_, err := svc.Register(context.Background(), cmd)

			var violation *appIdentity.FieldViolation
			if !errors.As(err, &violation) {
				t.Fatalf("se esperaba FieldViolation, obtenido %v", err)
			}
			if violation.Field != tc.field {
				t.Fatalf("campo esperado %q, obtenido %q", tc.field, violation.Field)
			}
			if violation.Business != tc.business {
				t.Fatalf("business esperado %v, obtenido %v", tc.business, violation.Business)
			}
		})
	}
}

func TestRegisterServiceEmailNormalized(t *testing.T) {
	store := memorystore.NewRegistrationStore()
	svc := newFixedService(store, &mockSimpleHasher{}, fixedUserID)

	cmd := validCommand(idempotencyKey("normalize", 1))
	cmd.Email = "  Usuario@Example.INVALID "
	result, err := svc.Register(context.Background(), cmd)
	if err != nil {
		t.Fatalf("registro fallo: %v", err)
	}

	var profile appIdentity.UserProfile
	if err := json.Unmarshal(result.Body, &profile); err != nil {
		t.Fatalf("JSON invalido: %v", err)
	}
	if profile.Email != "usuario@example.invalid" {
		t.Fatalf("email no normalizado: %q", profile.Email)
	}
}

func TestRegisterServiceGeneratesUUIDByDefault(t *testing.T) {
	store := memorystore.NewRegistrationStore()
	svc := appIdentity.NewRegisterService(store, &mockSimpleHasher{}).WithClock(fixedClock)

	result, err := svc.Register(context.Background(), validCommand(idempotencyKey("uuid", 1)))
	if err != nil {
		t.Fatalf("registro fallo: %v", err)
	}
	var profile appIdentity.UserProfile
	if err := json.Unmarshal(result.Body, &profile); err != nil {
		t.Fatalf("JSON invalido: %v", err)
	}
	pattern := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	if !pattern.MatchString(profile.ID) {
		t.Fatalf("UUID inesperado: %q", profile.ID)
	}
}

type failingHasher struct{}

func (failingHasher) HashPassword(string) (string, error) {
	return "", errors.New("fallo de hasher")
}
func (failingHasher) VerifyPassword(string, string) (bool, error) {
	return false, errors.New("fallo de hasher")
}

func TestRegisterServicePasswordHashFailure(t *testing.T) {
	store := memorystore.NewRegistrationStore()
	svc := newFixedService(store, failingHasher{}, fixedUserID)

	_, err := svc.Register(context.Background(), validCommand(idempotencyKey("hash-fail", 1)))
	if !errors.Is(err, appIdentity.ErrRegistrationInternal) {
		t.Fatalf("se esperaba ErrRegistrationInternal, obtenido %v", err)
	}
}

func TestRegisterServiceIDGenerationFailure(t *testing.T) {
	store := memorystore.NewRegistrationStore()
	svc := appIdentity.NewRegisterService(store, &mockSimpleHasher{}).
		WithClock(fixedClock).
		WithIDGenerator(func() (string, error) { return "", errors.New("sin entropia") })

	_, err := svc.Register(context.Background(), validCommand(idempotencyKey("id-fail", 1)))
	if !errors.Is(err, appIdentity.ErrRegistrationInternal) {
		t.Fatalf("se esperaba ErrRegistrationInternal, obtenido %v", err)
	}
}

func TestAuthenticateReadsCanonicalLowercaseProfile(t *testing.T) {
	ctx := context.Background()
	const table = "NEXUM_CORE_TEST"
	now := fixedClock().Format(time.RFC3339)

	repo := memory.NewMemoryRepository()
	if err := repo.PutItem(ctx, table, map[string]any{
		"PK":           "USER#u-1",
		"userId":       "u-1",
		"email":        "usuario@example.invalid",
		"passwordHash": "hashed:secreto",
		"status":       "ACTIVE",
		"createdAt":    now,
		"updatedAt":    now,
	}, nil); err != nil {
		t.Fatalf("no se pudo preparar el repositorio: %v", err)
	}

	svc := appIdentity.NewIdentityService(repo, memory.NewMemoryCache(), &mockSimpleHasher{}, &mockSimpleTokenProvider{}, table)
	result, err := svc.Authenticate(ctx, "u-1", "secreto")
	if err != nil {
		t.Fatalf("Authenticate no leyo el perfil canonico: %v", err)
	}
	if result.User.ID != "u-1" {
		t.Fatalf("usuario inesperado: %q", result.User.ID)
	}
}
