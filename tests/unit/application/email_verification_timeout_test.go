package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	appIdentity "github.com/nexumpr/Nexum-Backend/internal/application/identity"
)

// Pruebas del limite de tiempo de la llamada sincrona a SES en E010.
//
// El 502 observado en AWS lo genero el balanceador, no la aplicacion: una llamada
// externa sin deadline puede superar el idle timeout del ALB. El servicio debe cortar
// la llamada y devolver un error que el handler traduce a 503 contractual.

type e010MailerBloqueante struct {
	conDeadline bool
	ctxErr      error
}

func (m *e010MailerBloqueante) EnviarVerificacion(ctx context.Context, _, _ string, _ time.Time) (string, error) {
	_, m.conDeadline = ctx.Deadline()
	<-ctx.Done()
	m.ctxErr = ctx.Err()
	return "", ctx.Err()
}

func e010ServicioConLimite(correo *e010MailerBloqueante, limite time.Duration) *appIdentity.EmailVerificationRequestService {
	store := &e010Store{profile: e010Perfil()}
	limitador := &e010Limiter{allow: true}
	return appIdentity.NewEmailVerificationRequestService(store, correo, limitador, nil).
		WithClock(func() time.Time { return e010Ahora }).
		WithSendTimeout(limite)
}

func TestE010AplicaLimiteDeTiempoASes(t *testing.T) {
	correo := &e010MailerBloqueante{}
	servicio := e010ServicioConLimite(correo, 50*time.Millisecond)

	inicio := time.Now()
	err := servicio.Solicitar(context.Background(), e010Usuario)
	transcurrido := time.Since(inicio)

	if !errors.Is(err, appIdentity.ErrEmailVerificationEnvio) {
		t.Fatalf("un timeout de SES debe traducirse a ErrEmailVerificationEnvio, obtenido %v", err)
	}
	if transcurrido > 2*time.Second {
		t.Fatalf("el limite debe cortar la llamada, transcurrido %s", transcurrido)
	}
	if !correo.conDeadline {
		t.Fatal("la llamada a SES debe recibir un contexto con deadline")
	}
	if !errors.Is(correo.ctxErr, context.DeadlineExceeded) {
		t.Fatalf("se esperaba DeadlineExceeded en el mailer, obtenido %v", correo.ctxErr)
	}
}

func TestE010CancelacionDelClienteCortaElEnvio(t *testing.T) {
	correo := &e010MailerBloqueante{}
	servicio := e010ServicioConLimite(correo, 8*time.Second)

	ctx, cancelar := context.WithCancel(context.Background())
	go func() {
		time.Sleep(30 * time.Millisecond)
		cancelar()
	}()
	defer cancelar()

	inicio := time.Now()
	err := servicio.Solicitar(ctx, e010Usuario)
	transcurrido := time.Since(inicio)

	if !errors.Is(err, appIdentity.ErrEmailVerificationEnvio) {
		t.Fatalf("una cancelacion debe traducirse a ErrEmailVerificationEnvio, obtenido %v", err)
	}
	if transcurrido > 2*time.Second {
		t.Fatalf("la cancelacion debe cortar la llamada, transcurrido %s", transcurrido)
	}
	if !errors.Is(correo.ctxErr, context.Canceled) {
		t.Fatalf("se esperaba context.Canceled en el mailer, obtenido %v", correo.ctxErr)
	}
}

func TestE010LimitePorDefectoEsAcotado(t *testing.T) {
	if appIdentity.EmailVerificationSendTimeout <= 0 || appIdentity.EmailVerificationSendTimeout >= 15*time.Second {
		t.Fatalf("el limite por defecto debe ser positivo y menor que el WriteTimeout HTTP: %s", appIdentity.EmailVerificationSendTimeout)
	}
}
