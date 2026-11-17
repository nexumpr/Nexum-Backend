package transport_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nexumpr/Nexum-Backend/internal/infrastructure/security"
	"github.com/nexumpr/Nexum-Backend/internal/ports"
	"github.com/nexumpr/Nexum-Backend/internal/transport/http/middleware"
)

// Pruebas de A6 en el middleware: el claim `epo` se propaga al autorizador y una época
// desalineada produce 401 (nunca autoriza por defecto). Un fallo de DynamoDB produce 503.

type a6Autorizador struct {
	epocaRecibida int64
	llamadas      int
	err           error
}

func (a *a6Autorizador) AuthorizeSession(_ context.Context, _, _ string, epoch int64) error {
	a.llamadas++
	a.epocaRecibida = epoch
	return a.err
}

func a6Token(t *testing.T, epoch int64) (string, *security.RSATokenProvider) {
	t.Helper()
	clave, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("no se pudo generar la clave RSA: %v", err)
	}
	proveedor, err := security.NewRSATokenProvider(clave, "a6-test-kid")
	if err != nil {
		t.Fatalf("no se pudo construir el proveedor: %v", err)
	}
	ahora := time.Now().UTC()
	token, err := proveedor.GenerateAccessToken(context.Background(), ports.TokenClaims{
		Subject:   "usuario-a6",
		SessionID: "sesion-a6",
		IssuedAt:  ahora,
		ExpiresAt: ahora.Add(900 * time.Second),
		JTI:       "jti-a6",
		Epoch:     epoch,
	})
	if err != nil {
		t.Fatalf("no se pudo emitir el token: %v", err)
	}
	return token, proveedor
}

func a6Peticion(token string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/recurso", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	return req
}

func TestA6MiddlewarePropagaEpocaYAutoriza(t *testing.T) {
	token, proveedor := a6Token(t, 3)
	autorizador := &a6Autorizador{}
	protegido := middleware.AuthenticateBearerWithSession(proveedor, autorizador)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	rec := httptest.NewRecorder()
	protegido.ServeHTTP(rec, a6Peticion(token))
	if rec.Code != http.StatusOK {
		t.Fatalf("esperado 200, obtenido %d (%s)", rec.Code, rec.Body.String())
	}
	if autorizador.epocaRecibida != 3 {
		t.Fatalf("el autorizador debe recibir epo=3, obtenido %d", autorizador.epocaRecibida)
	}
}

func TestA6MiddlewareRechazaEpocaDesalineada(t *testing.T) {
	token, proveedor := a6Token(t, 0)
	autorizador := &a6Autorizador{err: ports.ErrSessionUnauthorized}
	handlerEjecutado := false
	protegido := middleware.AuthenticateBearerWithSession(proveedor, autorizador)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		handlerEjecutado = true
		w.WriteHeader(http.StatusOK)
	}))

	rec := httptest.NewRecorder()
	protegido.ServeHTTP(rec, a6Peticion(token))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("una epoca desalineada debe responder 401, obtenido %d", rec.Code)
	}
	if handlerEjecutado {
		t.Fatalf("el handler no puede ejecutarse sin verificacion de epoca")
	}
}

func TestA6MiddlewareFallaCerradoSiNoPuedeVerificar(t *testing.T) {
	token, proveedor := a6Token(t, 1)
	autorizador := &a6Autorizador{err: ports.ErrSessionUnavailable}
	handlerEjecutado := false
	protegido := middleware.AuthenticateBearerWithSession(proveedor, autorizador)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		handlerEjecutado = true
		w.WriteHeader(http.StatusOK)
	}))

	rec := httptest.NewRecorder()
	protegido.ServeHTTP(rec, a6Peticion(token))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("un fallo de persistencia debe responder 503, obtenido %d", rec.Code)
	}
	if handlerEjecutado {
		t.Fatalf("el handler no puede ejecutarse si no se pudo verificar la epoca")
	}
}

func TestA6TokenLegadoSinEpoSeLeeComoCero(t *testing.T) {
	// Un token sin `epo` (anterior a A6) debe entregar época 0 al autorizador.
	token, proveedor := a6Token(t, 0)
	autorizador := &a6Autorizador{}
	protegido := middleware.AuthenticateBearerWithSession(proveedor, autorizador)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	rec := httptest.NewRecorder()
	protegido.ServeHTTP(rec, a6Peticion(token))
	if rec.Code != http.StatusOK {
		t.Fatalf("un token legado debe seguir funcionando antes del reset, obtenido %d", rec.Code)
	}
	if autorizador.epocaRecibida != 0 {
		t.Fatalf("epoca esperada 0, obtenida %d", autorizador.epocaRecibida)
	}
	if errors.Is(autorizador.err, ports.ErrSessionUnauthorized) {
		t.Fatalf("no debe rechazarse un token legado sin reset")
	}
}
