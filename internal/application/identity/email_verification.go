package identity

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/nexumpr/Nexum-Backend/internal/ports"
)

// Errores de E010 (solicitud de verificacion de correo).
//
// El contrato OpenAPI fija 202/400/401/403/404/409/422/429/500/503. La identidad se deriva
// exclusivamente del JWT verificado: el cliente nunca envia el correo ni el userId.
var (
	// ErrEmailVerificationNoDisponible: falta persistencia, correo, cuota o un fallo real de
	// DynamoDB. El endpoint responde 503 y nunca simula un envio.
	ErrEmailVerificationNoDisponible = errors.New("verificacion de correo no disponible")
	// ErrEmailVerificationCuota: se supero el limite de solicitudes por usuario (429).
	ErrEmailVerificationCuota = errors.New("limite de solicitudes de verificacion superado")
	// ErrEmailVerificationYaVerificado: el correo ya estaba verificado; no se envia nada (409).
	ErrEmailVerificationYaVerificado = errors.New("el correo ya esta verificado")
	// ErrEmailVerificationPerfilAusente: no existe el perfil autenticado (404).
	ErrEmailVerificationPerfilAusente = errors.New("perfil de usuario no encontrado")
	// ErrEmailVerificationCuentaInactiva: la cuenta no esta ACTIVE (403).
	ErrEmailVerificationCuentaInactiva = errors.New("cuenta no habilitada para verificacion")
	// ErrEmailVerificationDatos: el perfil no permite construir el envio (500, dato interno).
	ErrEmailVerificationDatos = errors.New("datos de perfil incompletos")
	// ErrEmailVerificationEnvio: SES rechazo el envio; el token persistido queda sin efecto (503).
	ErrEmailVerificationEnvio = errors.New("no se pudo enviar el correo de verificacion")
	// ErrEmailVerificationTokenDuplicado: la condicion attribute_not_exists fallo al persistir
	// el token (colision improbable). El servicio lo traduce a 503 para reintentar.
	ErrEmailVerificationTokenDuplicado = errors.New("token de verificacion ya persistido")
)

// EmailVerificationTokenTTL es la vigencia del token de un solo uso (30 minutos).
const EmailVerificationTokenTTL = 30 * time.Minute

// EmailVerificationSendTimeout acota la llamada sincrona a SES. Sin este limite, una
// latencia excepcional del proveedor puede superar el idle timeout del balanceador y
// devolver un 502 ajeno al contrato; con el, el fallo se traduce a 503.
//
// 12 s mantiene la respuesta por debajo del WriteTimeout HTTP (15 s) y del timeout de
// origen de CloudFront (30 s) sin cortar envios legitimos: en AWS se midieron envios de
// verificacion que superaban 8 s y devolvian 503 aunque SES aceptaba el mensaje. El tope
// sigue siendo finito para no exponer la peticion a una espera indefinida.
const EmailVerificationSendTimeout = 12 * time.Second

// EmailVerificationProfile es el perfil minimo necesario para E010 y E011.
//
// Version/HasVersion son el CAS del perfil que usa la confirmacion (E011); la solicitud
// (E010) no los necesita pero comparte el mismo puerto de lectura.
type EmailVerificationProfile struct {
	UserID        string
	Email         string
	Status        string
	EmailVerified bool
	Version       int64
	HasVersion    bool
}

// EmailVerificationToken es lo que se persiste de un token. El valor en claro nunca entra
// aqui: solo su SHA-256 hex.
type EmailVerificationToken struct {
	UserID    string
	Email     string
	TokenHash string
	CreatedAt time.Time
	ExpiresAt time.Time
}

// EmailVerificationStore es el puerto de persistencia de E010/E011 sobre la tabla unica.
// No usa Scan ni memoria local como autoridad.
type EmailVerificationStore interface {
	// LoadEmailVerificationProfile lee USER#<id>/PROFILE con consistencia fuerte.
	LoadEmailVerificationProfile(ctx context.Context, userID string) (*EmailVerificationProfile, error)
	// SaveEmailVerificationToken escribe ONE_USE#<sha256(token)>/VERIFY con
	// attribute_not_exists(PK): nunca sobrescribe un token existente.
	SaveEmailVerificationToken(ctx context.Context, token EmailVerificationToken) error
}

// EmailVerificationMailer envia el codigo de verificacion (SES API v2).
// Es una interfaz propia para no alterar el contrato de E005 (ports.Correo).
type EmailVerificationMailer interface {
	EnviarVerificacion(ctx context.Context, destino, token string, expiraEn time.Time) (messageID string, err error)
}

// EmailVerificationRequestService implementa E010.
//
// El envio es sincrono dentro de la peticion: solo se responde 202 cuando SES acepto el
// mensaje. Si SES falla se responde 503; el token persistido caduca por TTL y no verifica
// nada por si mismo.
type EmailVerificationRequestService struct {
	store        EmailVerificationStore
	correo       EmailVerificationMailer
	limitador    ports.Limitador
	reloj        func() time.Time
	ttl          time.Duration
	timeoutEnvio time.Duration
	logger       *slog.Logger
	generador    func(n int) ([]byte, error)
}

// NewEmailVerificationRequestService construye el servicio. Cualquier dependencia nil deja
// el endpoint no disponible (503) en lugar de degradar a un comportamiento ficticio.
func NewEmailVerificationRequestService(
	store EmailVerificationStore,
	correo EmailVerificationMailer,
	limitador ports.Limitador,
	logger *slog.Logger,
) *EmailVerificationRequestService {
	return &EmailVerificationRequestService{
		store:        store,
		correo:       correo,
		limitador:    limitador,
		reloj:        func() time.Time { return time.Now().UTC() },
		ttl:          EmailVerificationTokenTTL,
		timeoutEnvio: EmailVerificationSendTimeout,
		logger:       logger,
		generador: func(n int) ([]byte, error) {
			b := make([]byte, n)
			_, err := rand.Read(b)
			return b, err
		},
	}
}

// WithClock permite inyectar un reloj determinista en pruebas.
func (s *EmailVerificationRequestService) WithClock(now func() time.Time) *EmailVerificationRequestService {
	if now != nil {
		s.reloj = now
	}
	return s
}

// WithSendTimeout permite acortar el limite de la llamada a SES en pruebas. Un valor
// no positivo conserva el limite por defecto.
func (s *EmailVerificationRequestService) WithSendTimeout(d time.Duration) *EmailVerificationRequestService {
	if s != nil && d > 0 {
		s.timeoutEnvio = d
	}
	return s
}

// Disponible indica si el endpoint puede operar de verdad.
func (s *EmailVerificationRequestService) Disponible() bool {
	return s != nil && s.store != nil && s.correo != nil && s.limitador != nil
}

// Solicitar genera, persiste y envia el codigo de verificacion del usuario autenticado.
//
// Secuencia: cuota por usuario → perfil con consistencia fuerte → estado ACTIVE → rechazo si
// ya esta verificado → token CSPRNG de 32 bytes (base64url, 43 caracteres) → persistencia con
// solo el SHA-256 → envio SES. El token en claro no se registra en logs ni se devuelve.
func (s *EmailVerificationRequestService) Solicitar(ctx context.Context, userID string) error {
	if !s.Disponible() {
		return ErrEmailVerificationNoDisponible
	}
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return ErrEmailVerificationPerfilAusente
	}

	// Cuota por usuario antes de cualquier trabajo: sin Redis el servicio falla cerrado.
	permitido, err := s.limitador.Permitir(ctx, "user:"+userID)
	if err != nil {
		return ErrEmailVerificationNoDisponible
	}
	if !permitido {
		return ErrEmailVerificationCuota
	}

	perfil, err := s.store.LoadEmailVerificationProfile(ctx, userID)
	if err != nil {
		return ErrEmailVerificationNoDisponible
	}
	if perfil == nil {
		return ErrEmailVerificationPerfilAusente
	}
	if !strings.EqualFold(strings.TrimSpace(perfil.Status), StatusActive) {
		return ErrEmailVerificationCuentaInactiva
	}
	if perfil.EmailVerified {
		// Ya verificado: no se genera ni se envia nada (409).
		return ErrEmailVerificationYaVerificado
	}

	destino := strings.TrimSpace(perfil.Email)
	if destino == "" {
		return ErrEmailVerificationDatos
	}

	crudo, err := s.generador(32)
	if err != nil {
		return ErrEmailVerificationNoDisponible
	}
	token := base64.RawURLEncoding.EncodeToString(crudo)
	ahora := s.reloj().UTC()

	err = s.store.SaveEmailVerificationToken(ctx, EmailVerificationToken{
		UserID:    userID,
		Email:     destino,
		TokenHash: HashToken(token),
		CreatedAt: ahora,
		ExpiresAt: ahora.Add(s.ttl),
	})
	if err != nil {
		if errors.Is(err, ErrEmailVerificationTokenDuplicado) {
			// Colision improbable: se falla cerrado y se puede reintentar.
			return ErrEmailVerificationNoDisponible
		}
		return ErrEmailVerificationNoDisponible
	}

	limite := s.timeoutEnvio
	if limite <= 0 {
		limite = EmailVerificationSendTimeout
	}
	envioCtx, cancelarEnvio := context.WithTimeout(ctx, limite)
	defer cancelarEnvio()
	if _, err := s.correo.EnviarVerificacion(envioCtx, destino, token, ahora.Add(s.ttl)); err != nil {
		// El item persistido caduca por TTL y no verifica nada: no se responde 202 sin envio.
		if s.logger != nil {
			s.logger.ErrorContext(ctx, "verificacion de correo: fallo de envio",
				slog.String("error_class", "ses_send_failed"))
		}
		return ErrEmailVerificationEnvio
	}

	if s.logger != nil {
		// Sin correo, sin token y sin userId: solo el hecho funcional.
		s.logger.InfoContext(ctx, "verificacion de correo solicitada",
			slog.String("resultado", "enviada"))
	}
	return nil
}
