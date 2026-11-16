package identity

import (
	"context"
	"errors"
	"strings"
	"time"
)

// Reglas de E004 (cierre de sesión actual).
//
// El contrato de la operación es `POST /api/v1/auth/logout`, sin cuerpo de petición, con
// `Authorization: Bearer <accessToken>` y respuesta `204 No Content` sin contenido
// (OpenAPI 3.1, operationId `nexum_e004`). La identidad se deriva EXCLUSIVAMENTE de claims
// verificados: `sub` (usuario) y `sid` (sesión). El cliente no envía `userId`, `sessionId`,
// `refreshToken` ni `allDevices`.
var (
	// ErrLogoutUnauthorized es el único error público cuando la sesión no es utilizable: no
	// existe, no pertenece al sujeto del token, ya fue revocada o expiró. No distingue causas:
	// no hay enumeración de sesiones ni de cuentas (GOV-06).
	ErrLogoutUnauthorized = errors.New("sesion no autorizada")

	// ErrLogoutConflict indica que el compare-and-swap del cierre perdió todas sus tentativas
	// frente a rotaciones de refresh concurrentes. No se informa éxito: el estado real es
	// incierto para el llamador hasta que vuelva a consultarlo (HTTP 409).
	ErrLogoutConflict = errors.New("conflicto de concurrencia en el cierre de sesion")

	// ErrLogoutUnavailable indica falta de persistencia o fallo de una dependencia obligatoria.
	// Falla cerrado con 503: nunca se responde 204 sin revocación durable confirmada.
	ErrLogoutUnavailable = errors.New("servicio de cierre de sesion no disponible")
)

const (
	// RevokeReasonLogout es el motivo durable del cierre iniciado por el usuario. E003 usa
	// `REFRESH_REPLAY_DETECTED`; ambos comparten el mismo atributo `revokeReason`.
	RevokeReasonLogout = "USER_LOGOUT"

	// LogoutMaxAttempts acota los reintentos ante conflicto recuperable. Sólo se reintenta un
	// compare-and-swap perdido (mutación condicional, efecto idempotente); un fallo de servicio
	// no se repite y se traduce en 503.
	LogoutMaxAttempts = 3
)

// LogoutCommand es la entrada de aplicación de E004. Sus dos campos provienen de claims del
// JWT ya validados por el middleware; nunca del cuerpo ni de la URL.
type LogoutCommand struct {
	UserID    string
	SessionID string
}

// LogoutSession es la lectura autoritativa `USER#<id>/SESSION#<id>` creada por E002 y rotada
// por E003. `RefreshHash` es el digest del refresh vigente: sólo se persiste el hash, nunca el
// token opaco.
type LogoutSession struct {
	UserID      string
	SessionID   string
	FamilyID    string
	RefreshHash string
	Status      string
	ExpiresAt   time.Time
	HasExpiry   bool
	// Version es el contador de escrituras de la sesión (E002 nace en 1; E003 lo incrementa
	// en cada rotación). HasVersion distingue "0" de "atributo ausente": un ítem heredado sin
	// `version` no admite compare-and-swap de versión y se revoca sólo con CAS de estado.
	Version    int64
	HasVersion bool
}

// LogoutRevocation describe la transición atómica `ACTIVE -> REVOKED` de una sesión y el
// cierre de su localizador de refresh vigente. Todo o nada.
type LogoutRevocation struct {
	UserID          string
	SessionID       string
	RefreshHash     string
	ExpectedVersion int64
	HasVersion      bool
	Reason          string
	Now             time.Time
}

// LogoutStore es el puerto de persistencia de E004. Se implementa sobre DynamoDB real; nunca
// usa Scan, ni memoria, ni caché como autoridad.
type LogoutStore interface {
	// LoadLogoutSession lee la sesión autoritativa con consistencia fuerte. Devuelve
	// (nil, nil) si la sesión no existe o el TTL ya la purgó.
	LoadLogoutSession(ctx context.Context, userID, sessionID string) (*LogoutSession, error)
	// RevokeLogoutSession aplica la revocación con compare-and-swap de estado y versión.
	// Devuelve ErrLogoutConflict si la condición no se cumple (rotación concurrente).
	RevokeLogoutSession(ctx context.Context, revocation LogoutRevocation) error
}

// LogoutService orquesta el cierre de la sesión actual.
type LogoutService struct {
	store LogoutStore
	now   func() time.Time
}

// NewLogoutService construye el servicio. Sin persistencia, Logout falla cerrado con
// ErrLogoutUnavailable: nunca se responde 204 sin revocación durable.
func NewLogoutService(store LogoutStore) *LogoutService {
	return &LogoutService{
		store: store,
		now:   func() time.Time { return time.Now().UTC() },
	}
}

// WithClock permite inyectar un reloj determinista en pruebas.
func (s *LogoutService) WithClock(now func() time.Time) *LogoutService {
	if now != nil {
		s.now = now
	}
	return s
}

// Logout revoca la sesión indicada por los claims del token.
//
// Secuencia: validar la entrada → leer la sesión autoritativa con consistencia fuerte →
// verificar propiedad, estado y expiración → revocar con compare-and-swap (sesión y
// localizador de refresh vigente) → confirmar sólo con la escritura durable aplicada.
//
// Carreras:
//   - Si un refresh gana primero, el CAS falla, se relee el estado vigente y se revoca con el
//     digest y la versión nuevos.
//   - Si el cierre gana primero, la rotación posterior falla porque exige `status=ACTIVE`;
//     ninguna ruta de esta operación vuelve a escribir `ACTIVE`.
//   - Sesiones distintas del mismo usuario no se tocan: no hay operación por familia ni
//     `allDevices`.
func (s *LogoutService) Logout(ctx context.Context, cmd LogoutCommand) error {
	if s == nil || s.store == nil {
		return ErrLogoutUnavailable
	}

	userID := strings.TrimSpace(cmd.UserID)
	sessionID := strings.TrimSpace(cmd.SessionID)
	if userID == "" || sessionID == "" {
		// Un token sin `sub` o `sid` no identifica sesión alguna: no se consulta la
		// persistencia para un identificador vacío.
		return ErrLogoutUnauthorized
	}

	now := s.now().UTC()

	for intento := 1; intento <= LogoutMaxAttempts; intento++ {
		session, err := s.store.LoadLogoutSession(ctx, userID, sessionID)
		if err != nil {
			return ErrLogoutUnavailable
		}
		if session == nil {
			// Sesión inexistente (o purgada por TTL): no se crea ni se inventa estado.
			return ErrLogoutUnauthorized
		}
		// Propiedad: el `sub` del token debe ser el dueño de la sesión del `sid`. Un cruce de
		// ambos no revoca nada y no revela si la sesión ajena existe.
		if session.UserID != userID || session.SessionID != sessionID {
			return ErrLogoutUnauthorized
		}
		if err := verifyLogoutState(session, now); err != nil {
			return err
		}

		err = s.store.RevokeLogoutSession(ctx, LogoutRevocation{
			UserID:          session.UserID,
			SessionID:       session.SessionID,
			RefreshHash:     session.RefreshHash,
			ExpectedVersion: session.Version,
			HasVersion:      session.HasVersion,
			Reason:          RevokeReasonLogout,
			Now:             now,
		})
		if err == nil {
			// La revocación durable se confirmó: sólo aquí puede responderse 204.
			return nil
		}
		if !errors.Is(err, ErrLogoutConflict) {
			return ErrLogoutUnavailable
		}
		// Conflicto: una rotación concurrente cambió digest y versión. Se relee y se decide con
		// el estado vigente (revocado ⇒ 401 idempotente; activo ⇒ se revoca en la nueva versión).
	}

	return ErrLogoutConflict
}

// verifyLogoutState exige sesión existente, coherente, ACTIVE y no expirada. Un estado ya
// revocado responde 401 sin nuevas mutaciones: los efectos del cierre son idempotentes aunque
// la respuesta no lo sea.
func verifyLogoutState(session *LogoutSession, now time.Time) error {
	if !statusMatches(session.Status, StatusActive) {
		return ErrLogoutUnauthorized
	}
	if session.HasExpiry && !now.Before(session.ExpiresAt) {
		return ErrLogoutUnauthorized
	}
	return nil
}
