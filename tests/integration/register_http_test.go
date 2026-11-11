package integration_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	appIdentity "github.com/nexumpr/Nexum-Backend/internal/application/identity"
	nexumhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
	memorystore "github.com/nexumpr/Nexum-Backend/tests/support/memorystore"
)

type integrationHasher struct{}

func (integrationHasher) HashPassword(password string) (string, error) {
	return "hashed:" + password, nil
}
func (integrationHasher) VerifyPassword(password, hash string) (bool, error) {
	return hash == "hashed:"+password, nil
}

type allowAllLimiter struct{}

func (allowAllLimiter) Allow(context.Context, string, string) (bool, time.Duration, error) {
	return true, 0, nil
}

type denyLimiter struct{}

func (denyLimiter) Allow(context.Context, string, string) (bool, time.Duration, error) {
	return false, 30 * time.Second, nil
}

func newRegisterHTTPServer(t *testing.T, store appIdentity.RegisterStore, limiter nexumhttp.RegisterRateLimiter) *httptest.Server {
	t.Helper()
	router := nexumhttp.NewRouter()
	svc := appIdentity.NewRegisterService(store, integrationHasher{}).
		WithClock(func() time.Time { return time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC) })
	handler := nexumhttp.NewRegisterHandler(svc, limiter, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	router.HandleFunc(http.MethodPost, "/auth/register", handler.ServeHTTP)

	server := httptest.NewServer(router)
	server.Client().Timeout = 10 * time.Second
	t.Cleanup(server.Close)
	return server
}

type httpResult struct {
	status int
	body   string
	err    error
}

func postRegister(client *http.Client, url, key, body string) httpResult {
	req, err := http.NewRequest(http.MethodPost, url+"/api/v1/auth/register", strings.NewReader(body))
	if err != nil {
		return httpResult{err: err}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", key)
	req.Header.Set("x-request-id", "integration-e001")
	resp, err := client.Do(req)
	if err != nil {
		return httpResult{err: err}
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return httpResult{err: err}
	}
	return httpResult{status: resp.StatusCode, body: string(data)}
}

const syntheticPhrase = "ExamplePassphrase#2026"

// Cuerpo V6.0.2 completo: identidad, ingreso, dos compromisos fijos, consentimiento y
// contexto de cliente.
const integrationRegisterBody = `{"name":"Dos Nodos","email":"dos-nodos@example.invalid","password":"` + syntheticPhrase + `",` +
	`"income":{"amount":"12000.00","frequency":"monthly"},` +
	`"recurringExpenses":[` +
	`{"concept":"Internet","amount":"699.00","frequency":"monthly","category":"Servicios","type":"fixed"},` +
	`{"concept":"Transporte","amount":"300.00","frequency":"weekly","category":"Transporte","type":"fixed"}],` +
	`"privacy":{"accepted":true,"noticeVersion":"2026-11"},` +
	`"client":{"locale":"es-MX","timeZone":"America/Mexico_City"}}`

func TestRegisterHTTPTwoInstancesSameKeyReplay(t *testing.T) {
	store := memorystore.NewRegistrationStore()
	serverA := newRegisterHTTPServer(t, store, allowAllLimiter{})
	serverB := newRegisterHTTPServer(t, store, allowAllLimiter{})

	const key = "0123456789abcdef"
	var wg sync.WaitGroup
	results := make(chan httpResult, 2)
	for _, server := range []*httptest.Server{serverA, serverB} {
		wg.Add(1)
		go func(s *httptest.Server) {
			defer wg.Done()
			results <- postRegister(s.Client(), s.URL, key, integrationRegisterBody)
		}(server)
	}
	wg.Wait()
	close(results)

	var bodies []string
	for result := range results {
		if result.err != nil {
			t.Fatalf("peticion HTTP fallo: %v", result.err)
		}
		if result.status != http.StatusCreated {
			t.Fatalf("status esperado 201, obtenido %d: %s", result.status, result.body)
		}
		bodies = append(bodies, result.body)
	}
	if len(bodies) != 2 || bodies[0] != bodies[1] {
		t.Fatalf("las dos instancias no devolvieron el mismo replay:\n%s\n%s", bodies[0], bodies[1])
	}
	if store.RecordsCount() != 1 {
		t.Fatalf("se esperaba un unico usuario, obtenidos %d", store.RecordsCount())
	}
}

func TestRegisterHTTPRespuesta201Completa(t *testing.T) {
	store := memorystore.NewRegistrationStore()
	server := newRegisterHTTPServer(t, store, allowAllLimiter{})

	result := postRegister(server.Client(), server.URL, "respuesta-completa-0001", integrationRegisterBody)
	if result.err != nil {
		t.Fatalf("peticion HTTP fallo: %v", result.err)
	}
	if result.status != http.StatusCreated {
		t.Fatalf("status esperado 201, obtenido %d: %s", result.status, result.body)
	}
	if strings.Contains(result.body, syntheticPhrase) || strings.Contains(result.body, "password") {
		t.Fatalf("la respuesta no debe exponer credenciales: %s", result.body)
	}

	var response appIdentity.RegisterResponse
	if err := json.Unmarshal([]byte(result.body), &response); err != nil {
		t.Fatalf("respuesta no es JSON valido: %v", err)
	}
	if response.User.ID == "" || response.User.Name != "Dos Nodos" {
		t.Fatalf("bloque user inesperado: %+v", response.User)
	}
	if response.Income.Amount != "12000.00" || response.Income.Frequency != "monthly" ||
		response.Income.MonthlyEquivalent != "12000.00" {
		t.Fatalf("bloque income inesperado: %+v", response.Income)
	}
	if len(response.RecurringPayments) != 2 {
		t.Fatalf("se esperaban 2 compromisos, hay %d", len(response.RecurringPayments))
	}
	for i, esperado := range []string{"Internet", "Transporte"} {
		if response.RecurringPayments[i].Concept != esperado {
			t.Fatalf("compromiso %d: esperado %q, obtenido %q", i, esperado, response.RecurringPayments[i].Concept)
		}
		if response.RecurringPayments[i].ID == "" {
			t.Fatalf("el compromiso %d no tiene identificador", i)
		}
		if response.RecurringPayments[i].Type != "fixed" {
			t.Fatalf("tipo inesperado en el compromiso %d: %q", i, response.RecurringPayments[i].Type)
		}
	}
	if response.Consent.NoticeVersion != "2026-11" || !response.Consent.AcceptedByServer {
		t.Fatalf("bloque consent inesperado: %+v", response.Consent)
	}
	if response.Onboarding.Microphone != "pending" || response.Onboarding.Configured {
		t.Fatalf("bloque onboarding inesperado: %+v", response.Onboarding)
	}
	if store.ExpensesCount(response.User.ID) != 2 {
		t.Fatalf("compromisos persistidos: esperados 2, hay %d", store.ExpensesCount(response.User.ID))
	}
}

func TestRegisterHTTPTwoInstancesSameEmailDifferentKeys(t *testing.T) {
	store := memorystore.NewRegistrationStore()
	serverA := newRegisterHTTPServer(t, store, allowAllLimiter{})
	serverB := newRegisterHTTPServer(t, store, allowAllLimiter{})

	var wg sync.WaitGroup
	results := make(chan httpResult, 2)
	servers := []*httptest.Server{serverA, serverB}
	for i, server := range servers {
		wg.Add(1)
		go func(s *httptest.Server, n int) {
			defer wg.Done()
			results <- postRegister(s.Client(), s.URL, fmt.Sprintf("email-nodo-%016d", n), integrationRegisterBody)
		}(server, i)
	}
	wg.Wait()
	close(results)

	created := 0
	conflicts := 0
	for result := range results {
		if result.err != nil {
			t.Fatalf("peticion HTTP fallo: %v", result.err)
		}
		switch result.status {
		case http.StatusCreated:
			created++
		case http.StatusConflict:
			conflicts++
		default:
			t.Fatalf("status inesperado %d: %s", result.status, result.body)
		}
	}
	if created != 1 || conflicts != 1 {
		t.Fatalf("se esperaba 1 creado y 1 conflicto, obtenidos %d y %d", created, conflicts)
	}
	if store.RecordsCount() != 1 {
		t.Fatalf("se esperaba un unico usuario, obtenidos %d", store.RecordsCount())
	}
}

func TestRegisterHTTPRateLimited(t *testing.T) {
	store := memorystore.NewRegistrationStore()
	server := newRegisterHTTPServer(t, store, denyLimiter{})

	result := postRegister(server.Client(), server.URL, "0123456789abcdef", integrationRegisterBody)
	if result.err != nil {
		t.Fatalf("peticion HTTP fallo: %v", result.err)
	}
	if result.status != http.StatusTooManyRequests {
		t.Fatalf("status esperado 429, obtenido %d: %s", result.status, result.body)
	}
	if !strings.Contains(result.body, "RATE_LIMITED") {
		t.Fatalf("codigo de error inesperado: %s", result.body)
	}
	if store.RecordsCount() != 0 {
		t.Fatal("una peticion limitada no debe persistir nada")
	}
}

func TestRegisterHTTPUnavailable(t *testing.T) {
	store := memorystore.NewRegistrationStore()
	store.SetUnavailable(true)
	server := newRegisterHTTPServer(t, store, allowAllLimiter{})

	result := postRegister(server.Client(), server.URL, "0123456789abcdef", integrationRegisterBody)
	if result.err != nil {
		t.Fatalf("peticion HTTP fallo: %v", result.err)
	}
	if result.status != http.StatusServiceUnavailable {
		t.Fatalf("status esperado 503, obtenido %d: %s", result.status, result.body)
	}
	if !strings.Contains(result.body, "DEPENDENCY_UNAVAILABLE") {
		t.Fatalf("codigo de error inesperado: %s", result.body)
	}
	if store.RecordsCount() != 0 {
		t.Fatal("un fallo de persistencia no debe confirmar escritura parcial")
	}
}
