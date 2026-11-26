package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	appIdentity "github.com/nexumpr/Nexum-Backend/internal/application/identity"
)

// Pruebas de la politica de envio de E010: un solo intento por solicitud, sin
// reintentos que puedan duplicar un correo cuyo resultado es ambiguo, y traduccion
// del fallo al error contractual (503) sin exponer detalles del proveedor.

func TestE010NoReintentaUnEnvioAmbiguo(t *testing.T) {
	store := &e010Store{profile: e010Perfil()}
	correo := &e010Mailer{err: appIdentity.ErrEnvioAmbiguo}
	servicio := e010Servicio(store, correo, &e010Limiter{allow: true})

	err := servicio.Solicitar(context.Background(), e010Usuario)
	if !errors.Is(err, appIdentity.ErrEmailVerificationEnvio) {
		t.Fatalf("un resultado ambiguo debe responder 503 contractual: %v", err)
	}
	if correo.llamadas != 1 {
		t.Fatalf("no puede repetirse un envio ambiguo: llamadas=%d", correo.llamadas)
	}
	// El token persistido sigue vigente: si SES acepto el mensaje, el codigo entregado
	// funciona; no se genera un segundo codigo valido.
	if len(store.saved) != 1 {
		t.Fatalf("debe persistirse exactamente un token, obtenido %d", len(store.saved))
	}
}

func TestE010RechazoDefinitivoNoSeReintenta(t *testing.T) {
	store := &e010Store{profile: e010Perfil()}
	correo := &e010Mailer{err: appIdentity.ErrEnvioDefinitivo}
	servicio := e010Servicio(store, correo, &e010Limiter{allow: true})

	if err := servicio.Solicitar(context.Background(), e010Usuario); !errors.Is(err, appIdentity.ErrEmailVerificationEnvio) {
		t.Fatalf("un rechazo definitivo debe responder 503 contractual: %v", err)
	}
	if correo.llamadas != 1 {
		t.Fatalf("un rechazo definitivo no se reintenta: llamadas=%d", correo.llamadas)
	}
}

func TestE010DosSolicitudesFallidasNoSeReintentanEntreSi(t *testing.T) {
	store := &e010Store{profile: e010Perfil()}
	correo := &e010Mailer{err: appIdentity.ErrEnvioTransitorio}
	servicio := e010Servicio(store, correo, &e010Limiter{allow: true})

	for i := 0; i < 2; i++ {
		if err := servicio.Solicitar(context.Background(), e010Usuario); !errors.Is(err, appIdentity.ErrEmailVerificationEnvio) {
			t.Fatalf("solicitud %d: %v", i+1, err)
		}
	}
	if correo.llamadas != 2 {
		t.Fatalf("cada solicitud hace un unico intento: llamadas=%d", correo.llamadas)
	}
	if len(store.saved) != 2 {
		t.Fatalf("cada solicitud persiste su propio token: %d", len(store.saved))
	}
	if store.saved[0].TokenHash == store.saved[1].TokenHash {
		t.Fatal("dos solicitudes no pueden compartir token")
	}
}

func TestE010CancelacionNoSeConvierteEnExito(t *testing.T) {
	store := &e010Store{profile: e010Perfil()}
	correo := &e010Mailer{err: context.Canceled}
	servicio := e010Servicio(store, correo, &e010Limiter{allow: true})

	ctx, cancelar := context.WithCancel(context.Background())
	cancelar()
	if err := servicio.Solicitar(ctx, e010Usuario); !errors.Is(err, appIdentity.ErrEmailVerificationEnvio) {
		t.Fatalf("una cancelacion no puede responder 202: %v", err)
	}
}

func TestE010PresupuestoDeEnvioCabeEnElWriteTimeout(t *testing.T) {
	limite := appIdentity.EmailVerificationSendTimeout
	if limite < 5*time.Second {
		t.Fatalf("el limite debe cubrir la latencia normal de SES: %s", limite)
	}
	if limite >= 15*time.Second {
		t.Fatalf("el limite debe dejar margen bajo el WriteTimeout HTTP de 15 s: %s", limite)
	}
}
