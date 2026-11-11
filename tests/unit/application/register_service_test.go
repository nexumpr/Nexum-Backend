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

// newSequenceService usa identificadores secuenciales: imprescindible cuando la
// peticion incluye compromisos fijos, que necesitan un ID distinto por fila.
func newSequenceService(store appIdentity.RegisterStore, hasher ports.PasswordHasher) *appIdentity.RegisterService {
	var seq atomic.Int64
	return appIdentity.NewRegisterService(store, hasher).
		WithClock(fixedClock).
		WithIDGenerator(func() (string, error) {
			return fmt.Sprintf("018f90ef-08e5-4a68-b09f-%012d", seq.Add(1)), nil
		})
}

const syntheticPhrase = "ExamplePassphrase#2026"

// Segunda frase sintetica en constante propia: el literal no aparece en una linea
// junto a la palabra `password`, de modo que no se confunde con una credencial.
const otraFraseSintetica = "OtraPassphrase#2026"

func expenseOf(concept string, amount int64, frequency, category string) appIdentity.RecurringExpenseCommand {
	return appIdentity.RecurringExpenseCommand{
		Concept:     concept,
		AmountCents: amount,
		Frequency:   frequency,
		Category:    category,
		Type:        "fixed",
	}
}

func validCommand(key string) appIdentity.RegisterCommand {
	return appIdentity.RegisterCommand{
		Name:              "Usuario Ejemplo",
		Email:             "usuario@example.invalid",
		Password:          syntheticPhrase,
		Income:            appIdentity.IncomeCommand{AmountCents: 1200000, Frequency: "monthly"},
		RecurringExpenses: []appIdentity.RecurringExpenseCommand{},
		Privacy:           appIdentity.PrivacyCommand{Accepted: true, NoticeVersion: "2026-11"},
		Client:            appIdentity.ClientCommand{Locale: "es-MX", TimeZone: "America/Mexico_City"},
		IdempotencyKey:    key,
	}
}

func idempotencyKey(prefix string, n int) string {
	return fmt.Sprintf("%s-%016d", prefix, n)
}

func decodeResponse(t *testing.T, body []byte) appIdentity.RegisterResponse {
	t.Helper()
	var response appIdentity.RegisterResponse
	if err := json.Unmarshal(body, &response); err != nil {
		t.Fatalf("respuesta no es JSON valido: %v (%s)", err, string(body))
	}
	return response
}

// --- Registro completo -------------------------------------------------------

func TestRegisterServiceCreaRegistroV602Completo(t *testing.T) {
	store := memorystore.NewRegistrationStore()
	svc := newSequenceService(store, security.NewArgon2Hasher(nil))

	cmd := validCommand(idempotencyKey("register", 1))
	cmd.RecurringExpenses = []appIdentity.RecurringExpenseCommand{
		expenseOf("Internet", 69900, "monthly", "Servicios"),
		expenseOf("Transporte", 30000, "weekly", "Transporte"),
	}

	result, err := svc.Register(context.Background(), cmd)
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

	response := decodeResponse(t, result.Body)
	if response.User.ID == "" || response.User.Name != "Usuario Ejemplo" {
		t.Fatalf("bloque user inesperado: %+v", response.User)
	}
	if response.User.Email != "usuario@example.invalid" || response.User.EmailVerified {
		t.Fatalf("identidad inesperada: %+v", response.User)
	}
	if response.User.CreatedAt != "2026-11-10T12:00:00Z" {
		t.Fatalf("createdAt inesperado: %q", response.User.CreatedAt)
	}
	if response.Income.Amount != "12000.00" || response.Income.Frequency != "monthly" || response.Income.MonthlyEquivalent != "12000.00" {
		t.Fatalf("ingreso inesperado: %+v", response.Income)
	}
	if len(response.RecurringPayments) != 2 {
		t.Fatalf("se esperaban 2 compromisos, hay %d", len(response.RecurringPayments))
	}
	first := response.RecurringPayments[0]
	if first.Concept != "Internet" || first.Amount != "699.00" || first.Frequency != "monthly" ||
		first.Category != "Servicios" || first.Type != "fixed" || first.ID == "" {
		t.Fatalf("compromiso inesperado: %+v", first)
	}
	if response.Consent.NoticeVersion != "2026-11" || !response.Consent.AcceptedByServer {
		t.Fatalf("consentimiento inesperado: %+v", response.Consent)
	}
	if _, err := time.Parse(time.RFC3339, response.Consent.AcceptedAt); err != nil {
		t.Fatalf("acceptedAt no es RFC3339: %q", response.Consent.AcceptedAt)
	}
	if response.Onboarding.Microphone != "pending" || response.Onboarding.Notifications != "pending" || response.Onboarding.Configured {
		t.Fatalf("onboarding inesperado: %+v", response.Onboarding)
	}

	record, ok := store.Registration(response.User.ID)
	if !ok {
		t.Fatal("no se persistio el registro")
	}
	if strings.Contains(record.PasswordHash, syntheticPhrase) {
		t.Fatal("la contrasena se persistio en claro")
	}
	if len(record.PasswordHash) < 20 || record.PasswordHash[:10] != "$argon2id$" {
		t.Fatalf("hash Argon2id inesperado: %q", record.PasswordHash)
	}
	if record.Income.AmountCents != 1200000 || record.Income.Frequency != "monthly" {
		t.Fatalf("ingreso persistido inesperado: %+v", record.Income)
	}
	if len(record.RecurringExpenses) != 2 {
		t.Fatalf("compromisos persistidos esperados 2, hay %d", len(record.RecurringExpenses))
	}
	if record.ConsentNoticeVersion != "2026-11" || record.ConsentAcceptedAt.IsZero() {
		t.Fatalf("consentimiento persistido inesperado: %q %v", record.ConsentNoticeVersion, record.ConsentAcceptedAt)
	}
	if record.OnboardingMicrophone != "pending" || record.OnboardingConfigured {
		t.Fatalf("onboarding persistido inesperado: %+v", record)
	}
	if record.ClientTimeZone != "America/Mexico_City" {
		t.Fatalf("zona horaria persistida inesperada: %q", record.ClientTimeZone)
	}
	if store.ExpensesCount(response.User.ID) != 2 {
		t.Fatalf("compromisos en el doble: esperados 2, hay %d", store.ExpensesCount(response.User.ID))
	}
}

func TestRegisterServiceSinGastosRecurrentes(t *testing.T) {
	store := memorystore.NewRegistrationStore()
	svc := newSequenceService(store, security.NewArgon2Hasher(nil))

	result, err := svc.Register(context.Background(), validCommand(idempotencyKey("vacio", 1)))
	if err != nil {
		t.Fatalf("registro sin gastos fallo: %v", err)
	}
	response := decodeResponse(t, result.Body)
	if response.RecurringPayments == nil {
		t.Fatal("recurringPayments debe ser [] y nunca null")
	}
	if len(response.RecurringPayments) != 0 {
		t.Fatalf("se esperaban 0 compromisos, hay %d", len(response.RecurringPayments))
	}
	if !strings.Contains(string(result.Body), `"recurringPayments":[]`) {
		t.Fatalf("la serializacion debe ser [] explicita: %s", string(result.Body))
	}
}

func TestRegisterServiceConUnGasto(t *testing.T) {
	store := memorystore.NewRegistrationStore()
	svc := newSequenceService(store, security.NewArgon2Hasher(nil))

	cmd := validCommand(idempotencyKey("uno", 1))
	cmd.RecurringExpenses = []appIdentity.RecurringExpenseCommand{
		expenseOf("Renta", 500000, "monthly", "Vivienda"),
	}
	result, err := svc.Register(context.Background(), cmd)
	if err != nil {
		t.Fatalf("registro con un gasto fallo: %v", err)
	}
	response := decodeResponse(t, result.Body)
	if len(response.RecurringPayments) != 1 || response.RecurringPayments[0].Amount != "5000.00" {
		t.Fatalf("compromiso inesperado: %+v", response.RecurringPayments)
	}
}

func TestRegisterServiceConMuchosGastos(t *testing.T) {
	store := memorystore.NewRegistrationStore()
	svc := newSequenceService(store, security.NewArgon2Hasher(nil))

	cmd := validCommand(idempotencyKey("muchos", 1))
	cmd.RecurringExpenses = make([]appIdentity.RecurringExpenseCommand, 0, 12)
	for i := 0; i < 12; i++ {
		cmd.RecurringExpenses = append(cmd.RecurringExpenses,
			expenseOf(fmt.Sprintf("Compromiso %02d", i), int64(1000*(i+1)), "monthly", "Otros"))
	}

	result, err := svc.Register(context.Background(), cmd)
	if err != nil {
		t.Fatalf("registro con varios gastos fallo: %v", err)
	}
	response := decodeResponse(t, result.Body)
	if len(response.RecurringPayments) != 12 {
		t.Fatalf("se esperaban 12 compromisos, hay %d", len(response.RecurringPayments))
	}
	ids := make(map[string]struct{}, 12)
	for _, payment := range response.RecurringPayments {
		ids[payment.ID] = struct{}{}
	}
	if len(ids) != 12 {
		t.Fatal("los identificadores de compromiso deben ser unicos")
	}
}

func TestRegisterServiceLimiteTransaccional(t *testing.T) {
	store := memorystore.NewRegistrationStore()
	svc := newSequenceService(store, security.NewArgon2Hasher(nil))

	allowed := validCommand(idempotencyKey("limite-ok", 1))
	allowed.RecurringExpenses = make([]appIdentity.RecurringExpenseCommand, 0, 94)
	for i := 0; i < 94; i++ {
		allowed.RecurringExpenses = append(allowed.RecurringExpenses,
			expenseOf(fmt.Sprintf("Fijo %02d", i), 1000, "monthly", "Otros"))
	}
	if _, err := svc.Register(context.Background(), allowed); err != nil {
		t.Fatalf("94 compromisos deben caber en una transaccion: %v", err)
	}

	exceeded := validCommand(idempotencyKey("limite-ko", 1))
	exceeded.Email = "limite@example.invalid"
	exceeded.RecurringExpenses = make([]appIdentity.RecurringExpenseCommand, 0, 95)
	for i := 0; i < 95; i++ {
		exceeded.RecurringExpenses = append(exceeded.RecurringExpenses,
			expenseOf(fmt.Sprintf("Fijo %02d", i), 1000, "monthly", "Otros"))
	}
	_, err := svc.Register(context.Background(), exceeded)
	var violation *appIdentity.FieldViolation
	if !errors.As(err, &violation) {
		t.Fatalf("se esperaba FieldViolation, se obtuvo %v", err)
	}
	if violation.Field != "recurringExpenses" || violation.Reason != "too_many_items" {
		t.Fatalf("violacion inesperada: %+v", violation)
	}
}

// --- Idempotencia ------------------------------------------------------------

func TestRegisterServiceReplaysSameKeyAndPayload(t *testing.T) {
	store := memorystore.NewRegistrationStore()
	svc := newSequenceService(store, &mockSimpleHasher{})

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

// Defecto A del contrato: la contrasena no entra en payloadHash (el sal Argon2id no es
// determinista), asi que el replay debe probarla contra el verificador Argon2id.
func TestRegisterServiceRechazaMismaClaveConOtraContrasena(t *testing.T) {
	store := memorystore.NewRegistrationStore()
	svc := newSequenceService(store, security.NewArgon2Hasher(nil))

	key := idempotencyKey("otra-clave", 1)
	if _, err := svc.Register(context.Background(), validCommand(key)); err != nil {
		t.Fatalf("primer registro fallo: %v", err)
	}

	otra := validCommand(key)
	otra.Password = otraFraseSintetica
	if _, err := svc.Register(context.Background(), otra); !errors.Is(err, appIdentity.ErrIdempotencyConflict) {
		t.Fatalf("se esperaba ErrIdempotencyConflict, se obtuvo %v", err)
	}
	if store.RecordsCount() != 1 {
		t.Fatalf("no debe crearse un segundo usuario: %d", store.RecordsCount())
	}
}

func TestRegisterServiceRechazaMismaClaveConOtroPayload(t *testing.T) {
	store := memorystore.NewRegistrationStore()
	svc := newSequenceService(store, &mockSimpleHasher{})

	if _, err := svc.Register(context.Background(), validCommand(idempotencyKey("conflict", 1))); err != nil {
		t.Fatalf("primer registro fallo: %v", err)
	}

	casos := []struct {
		name   string
		mutate func(*appIdentity.RegisterCommand)
	}{
		{"otro nombre", func(c *appIdentity.RegisterCommand) { c.Name = "Otro Nombre" }},
		{"otro ingreso", func(c *appIdentity.RegisterCommand) { c.Income.AmountCents = 999900 }},
		{"otra frecuencia de ingreso", func(c *appIdentity.RegisterCommand) { c.Income.Frequency = "weekly" }},
		{"otro gasto", func(c *appIdentity.RegisterCommand) {
			c.RecurringExpenses = []appIdentity.RecurringExpenseCommand{expenseOf("Luz", 100, "monthly", "Servicios")}
		}},
		{"otra version de aviso", func(c *appIdentity.RegisterCommand) { c.Privacy.NoticeVersion = "2026-12" }},
		{"otra zona horaria", func(c *appIdentity.RegisterCommand) { c.Client.TimeZone = "Europe/Madrid" }},
	}
	for _, caso := range casos {
		t.Run(caso.name, func(t *testing.T) {
			cmd := validCommand(idempotencyKey("conflict", 1))
			caso.mutate(&cmd)
			if _, err := svc.Register(context.Background(), cmd); !errors.Is(err, appIdentity.ErrIdempotencyConflict) {
				t.Fatalf("se esperaba ErrIdempotencyConflict, se obtuvo %v", err)
			}
		})
	}
}

func TestRegisterServiceRechazaDuplicadoTrasReplay(t *testing.T) {
	store := memorystore.NewRegistrationStore()
	svc := newSequenceService(store, &mockSimpleHasher{})

	key := idempotencyKey("reintento", 1)
	cmd := validCommand(key)
	cmd.RecurringExpenses = []appIdentity.RecurringExpenseCommand{expenseOf("Internet", 69900, "monthly", "Servicios")}

	if _, err := svc.Register(context.Background(), cmd); err != nil {
		t.Fatalf("primer registro fallo: %v", err)
	}
	for i := 0; i < 5; i++ {
		result, err := svc.Register(context.Background(), cmd)
		if err != nil {
			t.Fatalf("reintento %d fallo: %v", i, err)
		}
		if !result.Replayed {
			t.Fatalf("el reintento %d debía ser replay", i)
		}
	}
	if store.RecordsCount() != 1 {
		t.Fatalf("los reintentos no deben crear usuarios: %d", store.RecordsCount())
	}
	response := decodeResponse(t, mustRegister(t, svc, cmd).Body)
	if store.ExpensesCount(response.User.ID) != 1 {
		t.Fatalf("los reintentos no deben duplicar compromisos: %d", store.ExpensesCount(response.User.ID))
	}
}

func mustRegister(t *testing.T, svc *appIdentity.RegisterService, cmd appIdentity.RegisterCommand) *appIdentity.StoredResponse {
	t.Helper()
	result, err := svc.Register(context.Background(), cmd)
	if err != nil {
		t.Fatalf("registro fallo: %v", err)
	}
	return result
}

func TestRegisterServiceEmailDuplicado(t *testing.T) {
	store := memorystore.NewRegistrationStore()
	svc := newSequenceService(store, &mockSimpleHasher{})

	if _, err := svc.Register(context.Background(), validCommand(idempotencyKey("email", 1))); err != nil {
		t.Fatalf("primer registro fallo: %v", err)
	}

	second := validCommand(idempotencyKey("email", 2))
	second.Email = "USUARIO@example.invalid"
	if _, err := svc.Register(context.Background(), second); !errors.Is(err, appIdentity.ErrEmailAlreadyRegistered) {
		t.Fatalf("se esperaba ErrEmailAlreadyRegistered, se obtuvo %v", err)
	}
}

func TestRegisterServiceConcurrenteMismaClave(t *testing.T) {
	store := memorystore.NewRegistrationStore()
	svc := newSequenceService(store, &mockSimpleHasher{})

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

func TestRegisterServiceConcurrenteMismoEmail(t *testing.T) {
	store := memorystore.NewRegistrationStore()
	svc := newSequenceService(store, &mockSimpleHasher{})

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

func TestRegisterServiceConcurrenteMismaClaveDistintoPayload(t *testing.T) {
	store := memorystore.NewRegistrationStore()
	svc := newSequenceService(store, &mockSimpleHasher{})

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
				cmd.Name = "Payload A"
			} else {
				cmd.Name = "Payload B"
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
				t.Fatal("dos payloads distintos lograron exito con la misma clave")
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

// --- Disponibilidad y fallos -------------------------------------------------

func TestRegisterServiceStoreUnavailable(t *testing.T) {
	store := memorystore.NewRegistrationStore()
	store.SetUnavailable(true)
	svc := newFixedService(store, &mockSimpleHasher{}, fixedUserID)

	_, err := svc.Register(context.Background(), validCommand(idempotencyKey("unavailable", 1)))
	if !errors.Is(err, appIdentity.ErrStoreUnavailable) {
		t.Fatalf("se esperaba ErrStoreUnavailable, se obtuvo %v", err)
	}
}

type failingHasher struct{}

func (failingHasher) HashPassword(string) (string, error) { return "", errors.New("fallo de hasher") }
func (failingHasher) VerifyPassword(string, string) (bool, error) {
	return false, errors.New("fallo de hasher")
}

func TestRegisterServicePasswordHashFailure(t *testing.T) {
	store := memorystore.NewRegistrationStore()
	svc := newFixedService(store, failingHasher{}, fixedUserID)

	_, err := svc.Register(context.Background(), validCommand(idempotencyKey("hash-fail", 1)))
	if !errors.Is(err, appIdentity.ErrRegistrationInternal) {
		t.Fatalf("se esperaba ErrRegistrationInternal, se obtuvo %v", err)
	}
}

func TestRegisterServiceIDGenerationFailure(t *testing.T) {
	store := memorystore.NewRegistrationStore()
	svc := appIdentity.NewRegisterService(store, &mockSimpleHasher{}).
		WithClock(fixedClock).
		WithIDGenerator(func() (string, error) { return "", errors.New("sin entropia") })

	_, err := svc.Register(context.Background(), validCommand(idempotencyKey("id-fail", 1)))
	if !errors.Is(err, appIdentity.ErrRegistrationInternal) {
		t.Fatalf("se esperaba ErrRegistrationInternal, se obtuvo %v", err)
	}
}

// --- Validacion --------------------------------------------------------------

func TestRegisterServiceValidacion(t *testing.T) {
	store := memorystore.NewRegistrationStore()
	svc := newSequenceService(store, &mockSimpleHasher{})

	casos := []struct {
		name     string
		mutate   func(*appIdentity.RegisterCommand)
		field    string
		business bool
	}{
		{"email invalido", func(c *appIdentity.RegisterCommand) { c.Email = "sin-arroba" }, "email", false},
		{"password corta", func(c *appIdentity.RegisterCommand) { c.Password = "corta" }, "password", false},
		{"password vacia", func(c *appIdentity.RegisterCommand) { c.Password = "            " }, "password", true},
		{"name vacio", func(c *appIdentity.RegisterCommand) { c.Name = "   " }, "name", true},
		{"name largo", func(c *appIdentity.RegisterCommand) { c.Name = strings.Repeat("a", 101) }, "name", false},
		{"ingreso cero", func(c *appIdentity.RegisterCommand) { c.Income.AmountCents = 0 }, "income.amount", true},
		{"ingreso negativo", func(c *appIdentity.RegisterCommand) { c.Income.AmountCents = -100 }, "income.amount", true},
		{"ingreso daily", func(c *appIdentity.RegisterCommand) { c.Income.Frequency = "daily" }, "income.frequency", false},
		{"ingreso once", func(c *appIdentity.RegisterCommand) { c.Income.Frequency = "once" }, "income.frequency", false},
		{"gasto sin concepto", func(c *appIdentity.RegisterCommand) {
			c.RecurringExpenses = []appIdentity.RecurringExpenseCommand{expenseOf(" ", 100, "monthly", "Otros")}
		}, "recurringExpenses[0].concept", false},
		{"gasto importe cero", func(c *appIdentity.RegisterCommand) {
			c.RecurringExpenses = []appIdentity.RecurringExpenseCommand{expenseOf("Luz", 0, "monthly", "Servicios")}
		}, "recurringExpenses[0].amount", true},
		{"gasto frecuencia daily", func(c *appIdentity.RegisterCommand) {
			c.RecurringExpenses = []appIdentity.RecurringExpenseCommand{expenseOf("Luz", 100, "daily", "Servicios")}
		}, "recurringExpenses[0].frequency", false},
		{"gasto categoria invalida", func(c *appIdentity.RegisterCommand) {
			c.RecurringExpenses = []appIdentity.RecurringExpenseCommand{expenseOf("Luz", 100, "monthly", "Mascotas")}
		}, "recurringExpenses[0].category", false},
		{"gasto tipo invalido", func(c *appIdentity.RegisterCommand) {
			item := expenseOf("Luz", 100, "monthly", "Servicios")
			item.Type = "variable"
			c.RecurringExpenses = []appIdentity.RecurringExpenseCommand{item}
		}, "recurringExpenses[0].type", false},
		{"aviso vacio", func(c *appIdentity.RegisterCommand) { c.Privacy.NoticeVersion = " " }, "privacy.noticeVersion", false},
		{"consentimiento no aceptado", func(c *appIdentity.RegisterCommand) { c.Privacy.Accepted = false }, "privacy.accepted", true},
		{"zona horaria invalida", func(c *appIdentity.RegisterCommand) { c.Client.TimeZone = "Marte/Olympus" }, "client.timeZone", false},
		{"locale invalido", func(c *appIdentity.RegisterCommand) { c.Client.Locale = "españa!" }, "client.locale", false},
		{"idempotency corta", func(c *appIdentity.RegisterCommand) { c.IdempotencyKey = "corta" }, "Idempotency-Key", false},
		{"idempotency con control", func(c *appIdentity.RegisterCommand) { c.IdempotencyKey = "0123456789abcde\x00" }, "Idempotency-Key", false},
	}

	for _, caso := range casos {
		t.Run(caso.name, func(t *testing.T) {
			cmd := validCommand(idempotencyKey("validation", 1))
			caso.mutate(&cmd)
			_, err := svc.Register(context.Background(), cmd)

			var violation *appIdentity.FieldViolation
			if !errors.As(err, &violation) {
				t.Fatalf("se esperaba FieldViolation, se obtuvo %v", err)
			}
			if violation.Field != caso.field {
				t.Fatalf("campo esperado %q, obtenido %q", caso.field, violation.Field)
			}
			if violation.Business != caso.business {
				t.Fatalf("business esperado %v, obtenido %v", caso.business, violation.Business)
			}
		})
	}
}

func TestRegisterServiceEmailNormalizado(t *testing.T) {
	store := memorystore.NewRegistrationStore()
	svc := newFixedService(store, &mockSimpleHasher{}, fixedUserID)

	cmd := validCommand(idempotencyKey("normalize", 1))
	cmd.Email = "  Usuario@Example.INVALID "
	result, err := svc.Register(context.Background(), cmd)
	if err != nil {
		t.Fatalf("registro fallo: %v", err)
	}
	response := decodeResponse(t, result.Body)
	if response.User.Email != "usuario@example.invalid" {
		t.Fatalf("email no normalizado: %q", response.User.Email)
	}
}

func TestRegisterServiceNameNormalizado(t *testing.T) {
	store := memorystore.NewRegistrationStore()
	svc := newFixedService(store, &mockSimpleHasher{}, fixedUserID)

	cmd := validCommand(idempotencyKey("name-normalize", 1))
	cmd.Name = "   Leonardo Prueba   "
	result, err := svc.Register(context.Background(), cmd)
	if err != nil {
		t.Fatalf("registro fallo: %v", err)
	}
	if response := decodeResponse(t, result.Body); response.User.Name != "Leonardo Prueba" {
		t.Fatalf("name no normalizado: %q", response.User.Name)
	}
}

func TestRegisterServiceClienteOpcional(t *testing.T) {
	store := memorystore.NewRegistrationStore()
	svc := newFixedService(store, &mockSimpleHasher{}, fixedUserID)

	cmd := validCommand(idempotencyKey("sin-cliente", 1))
	cmd.Client = appIdentity.ClientCommand{}
	result, err := svc.Register(context.Background(), cmd)
	if err != nil {
		t.Fatalf("el bloque client es opcional: %v", err)
	}
	if result.Status != 201 {
		t.Fatalf("status inesperado: %d", result.Status)
	}
}

func TestRegisterServiceGeneraUUIDPorDefecto(t *testing.T) {
	store := memorystore.NewRegistrationStore()
	svc := appIdentity.NewRegisterService(store, &mockSimpleHasher{}).WithClock(fixedClock)

	result, err := svc.Register(context.Background(), validCommand(idempotencyKey("uuid", 1)))
	if err != nil {
		t.Fatalf("registro fallo: %v", err)
	}
	response := decodeResponse(t, result.Body)
	pattern := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	if !pattern.MatchString(response.User.ID) {
		t.Fatalf("UUID inesperado: %q", response.User.ID)
	}
}

func TestRegisterServiceNoSerializaPassword(t *testing.T) {
	store := memorystore.NewRegistrationStore()
	svc := newSequenceService(store, security.NewArgon2Hasher(nil))

	cmd := validCommand(idempotencyKey("sin-password", 1))
	result := mustRegister(t, svc, cmd)
	body := string(result.Body)
	for _, prohibido := range []string{syntheticPhrase, "passwordHash", "password", "argon2"} {
		if strings.Contains(body, prohibido) {
			t.Fatalf("la respuesta no debe contener %q: %s", prohibido, body)
		}
	}
}

// --- Compatibilidad con datos previos ----------------------------------------

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
