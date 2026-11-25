package transport_test

// Pruebas de la cuota D-429 en E008 (`GET /auth/sessions`) y E009
// (`DELETE /auth/sessions/{sessionId}`): el 429 debe llevar el código contractual
// `RATE_LIMITED` y la cabecera `Retry-After`, usar una clase de cuota propia y
// pasarse con identidad verificada (el `sub` real, nunca una clave compartida).

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	appIdentity "github.com/nexumpr/Nexum-Backend/internal/application/identity"
	internalhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
)

type cuotaSesionesFake struct {
	permitido bool
	retry     time.Duration
	clases    []string
	sujetos   []string
}

func (c *cuotaSesionesFake) Allow(_ context.Context, clase, identificador string) (bool, time.Duration, error) {
	c.clases = append(c.clases, clase)
	c.sujetos = append(c.sujetos, identificador)
	return c.permitido, c.retry, nil
}

func e008ConCuota(limitador internalhttp.LimitadorRuta) *internalhttp.SessionsHandler {
	servicio := appIdentity.NewSessionsService(newE008HTTPStore(), []byte(e008ClaveCursores)).
		WithClock(func() time.Time { return e008HTTPAhora })
	return internalhttp.NewSessionsHandler(servicio, e004TokensValidos(), newE004Sesiones(), limitador, silencioso())
}

func e009ConCuota(limitador internalhttp.LimitadorRuta) *internalhttp.SessionRevokeHandler {
	servicio := appIdentity.NewSessionRevokeService(newE009HTTPStore()).
		WithClock(func() time.Time { return e004HTTPAhora })
	return internalhttp.NewSessionRevokeHandler(servicio, e004TokensValidos(), newE004Sesiones(), limitador, silencioso())
}

func TestE008CuotaD429EmiteRateLimited(t *testing.T) {
	limitador := &cuotaSesionesFake{permitido: false, retry: 15 * time.Second}
	handler := e008ConCuota(limitador)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, e008HTTPPeticion("jwt-valido", ""))

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("HTTP=%d, esperado 429", rec.Code)
	}
	if got := rec.Header().Get("Retry-After"); got != "15" {
		t.Fatalf("Retry-After=%q, esperado 15", got)
	}
	var envelope struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("envelope inválido: %v", err)
	}
	if envelope.Error.Code != "RATE_LIMITED" {
		t.Fatalf("code=%q, esperado RATE_LIMITED", envelope.Error.Code)
	}
	if len(limitador.clases) != 1 || limitador.clases[0] != "sessions" {
		t.Fatalf("clase de cuota=%v, esperado [sessions]", limitador.clases)
	}
	if len(limitador.sujetos) != 1 || limitador.sujetos[0] != e004HTTPUsuario {
		t.Fatalf("sujeto de cuota=%v, esperado el sub verificado", limitador.sujetos)
	}
}

func TestE009CuotaD429EmiteRateLimited(t *testing.T) {
	limitador := &cuotaSesionesFake{permitido: false, retry: 2 * time.Second}
	handler := e009ConCuota(limitador)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, e009HTTPPeticion("jwt-valido", e004HTTPSesion))

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("HTTP=%d, esperado 429", rec.Code)
	}
	if got := rec.Header().Get("Retry-After"); got != "2" {
		t.Fatalf("Retry-After=%q, esperado 2", got)
	}
	if len(limitador.clases) != 1 || limitador.clases[0] != "sessions_revoke" {
		t.Fatalf("clase de cuota=%v, esperado [sessions_revoke]", limitador.clases)
	}
}

func TestE008E009CuotaPermitidaNoInterfiere(t *testing.T) {
	permitido := &cuotaSesionesFake{permitido: true}
	rec := httptest.NewRecorder()
	e008ConCuota(permitido).ServeHTTP(rec, e008HTTPPeticion("jwt-valido", ""))
	if rec.Code == http.StatusTooManyRequests {
		t.Fatalf("la cuota permitida no debe producir 429")
	}
	rec = httptest.NewRecorder()
	e009ConCuota(&cuotaSesionesFake{permitido: true}).ServeHTTP(rec, e009HTTPPeticion("jwt-valido", "sesion-inexistente"))
	if rec.Code == http.StatusTooManyRequests {
		t.Fatalf("la cuota permitida no debe producir 429 en E009")
	}
}

// El 429 no debe exponer detalles internos ni la identidad en el cuerpo.
func TestE008CuotaNoExponeIdentidad(t *testing.T) {
	limitador := &cuotaSesionesFake{permitido: false, retry: time.Second}
	rec := httptest.NewRecorder()
	e008ConCuota(limitador).ServeHTTP(rec, e008HTTPPeticion("jwt-valido", ""))
	cuerpo := rec.Body.String()
	if strings.Contains(cuerpo, e004HTTPUsuario) {
		t.Fatalf("el cuerpo del 429 no debe contener el userID")
	}
	if strings.Contains(cuerpo, "jwt-valido") {
		t.Fatalf("el cuerpo del 429 no debe contener el token")
	}
}
