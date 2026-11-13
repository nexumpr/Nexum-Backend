package identity

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"
)

// Reglas de E003 (rotación de refresh token).
//
// El contrato de la operación es `POST /api/v1/auth/refresh` con `RefreshRequest` →
// `AuthTokens` (OpenAPI 3.1, operationId `nexum_e003`). Este archivo implementa la
// semántica aprobada por el SDD H06: «rotación de un solo uso por compare-and-swap;
// reutilización revoca la familia de sesión en todos los nodos».
var (
	// ErrInvalidRefreshToken es el único error público para un refresh token inexistente,
	// caducado, revocado, consumido por otra rotación o incoherente con su sesión. No
	// distingue causas: no hay enumeración de sesiones ni de cuentas.
	ErrInvalidRefreshToken = errors.New("refresh token invalido")

	// ErrRefreshReplay indica reutilización de un refresh token ya consumido. La familia
	// de la sesión se revoca de forma durable antes de devolver este error (HTTP 409).
	ErrRefreshReplay = errors.New("reutilizacion de refresh token detectada")

	// ErrRefreshConflict indica que la transacción de rotación perdió el compare-and-swap
	// sin evidencia durable de reutilización. No se emiten tokens y no se revoca nada
	// (HTTP 409): no se confunde un conflicto de concurrencia con un fallo de DynamoDB.
	ErrRefreshConflict = errors.New("conflicto de concurrencia en la rotacion")

	// ErrRefreshUnavailable indica falta de persistencia, de material criptográfico o
	// fallo de una dependencia obligatoria. Falla cerrado con 503 y nunca emite tokens.
	ErrRefreshUnavailable = errors.New("servicio de renovacion no disponible")
)

const (
	// RefreshMaxTokenRunes acota el token opaco admitido. El emisor produce 43 caracteres
	// base64url (32 bytes CSPRNG); el límite deja margen para futuras codificaciones sin
	// permitir cuerpos abusivos.
	RefreshMaxTokenRunes = 512

	// Estados persistidos de la sesión y de su localizador de refresh.
	StatusActive   = "ACTIVE"
	StatusRevoked  = "REVOKED"
	StatusConsumed = "CONSUMED"

	// RevokeReasonReplay es el motivo durable de revocación por reutilización.
	RevokeReasonReplay = "REFRESH_REPLAY_DETECTED"

	maxRefreshDeviceIDRunes = 128
)

// RefreshCommand es la entrada de aplicación de E003. No deriva identidad del cuerpo: el
// `userId` sale siempre del localizador de refresh persistido y de la sesión autoritativa.
type RefreshCommand struct {
	RefreshToken string
	DeviceID     string
	ClientIP     string
}

// RefreshLocator es el localizador `REFRESH#<digest>/SESSION`. Sólo contiene el digest
// SHA-256 del token opaco: el token en claro nunca se persiste ni se registra.
type RefreshLocator struct {
	UserID    string
	SessionID string
	FamilyID  string
	Status    string
	Used      bool
	ExpiresAt time.Time
	HasExpiry bool
}

// RefreshSession es la lectura autoritativa `USER#<id>/SESSION#<id>` creada por E002.
type RefreshSession struct {
	UserID      string
	SessionID   string
	FamilyID    string
	RefreshHash string
	AccessJTI   string
	Status      string
	DeviceID    string
	DeviceName  string
	Platform    string
	CreatedAt   time.Time
	LastUsedAt  time.Time
	ExpiresAt   time.Time
	HasExpiry   bool
	Version     int64
}

// RefreshRotation describe la transición atómica que consume el token presentado y emite
// el siguiente: localizador anterior marcado como consumido, sesión con el nuevo digest y
// nuevo localizador. Todo o nada.
type RefreshRotation struct {
	UserID          string
	SessionID       string
	FamilyID        string
	OldDigest       string
	NewDigest       string
	NewAccessJTI    string
	ExpectedVersion int64
	NewVersion      int64
	// ExpiresAt es la expiración absoluta de la sesión (no se extiende al rotar) y se
	// replica en el localizador nuevo para que ambos caduquen juntos.
	ExpiresAt time.Time
	Now       time.Time
}

// RefreshRevocation revoca de forma durable la sesión de la familia afectada.
type RefreshRevocation struct {
	UserID          string
	SessionID       string
	FamilyID        string
	PresentedDigest string
	CurrentDigest   string
	Now             time.Time
}

// RefreshUser es el estado mínimo de la cuenta propietaria. Una cuenta deshabilitada,
// eliminada o inexistente impide renovar aunque la sesión siga marcada ACTIVE.
type RefreshUser struct {
	UserID string
	Status string
}

// RefreshStore es el puerto de persistencia de E003. Se implementa sobre DynamoDB real;
// nunca usa Scan, ni memoria, ni caché como autoridad.
type RefreshStore interface {
	// LocateRefresh resuelve el digest contra `REFRESH#<digest>/SESSION` con consistencia fuerte.
	LocateRefresh(ctx context.Context, digest string) (*RefreshLocator, error)
	// LoadSession lee la sesión autoritativa con consistencia fuerte.
	LoadSession(ctx context.Context, userID, sessionID string) (*RefreshSession, error)
	// LoadUserProfile lee el perfil propietario (`USER#<id>/PROFILE`) con consistencia
	// fuerte. Devuelve (nil, nil) si el perfil no existe.
	LoadUserProfile(ctx context.Context, userID string) (*RefreshUser, error)
	// RotateRefresh aplica la rotación en una única transacción condicional (CAS).
	RotateRefresh(ctx context.Context, rotation RefreshRotation) error
	// RevokeFamily revoca durablemente la sesión de la familia afectada.
	RevokeFamily(ctx context.Context, revocation RefreshRevocation) error
}

// RefreshSessionRef es el alias `session.id` del contrato V6.0.2 (RFC-014). Repite
// exactamente el mismo identificador que `sessionId`; no crea una sesión adicional.
type RefreshSessionRef struct {
	ID string `json:"id"`
}

// AuthTokens es el contrato exacto de la respuesta 200 de E003. Conserva los cinco campos
// heredados de H06 y añade el alias `session.id` del prototipo (RFC-014), sin quitar ni
// renombrar ninguno de los aprobados.
type AuthTokens struct {
	AccessToken  string            `json:"accessToken"`
	RefreshToken string            `json:"refreshToken"`
	TokenType    string            `json:"tokenType"`
	ExpiresIn    int64             `json:"expiresIn"`
	SessionID    string            `json:"sessionId"`
	Session      RefreshSessionRef `json:"session"`
}

// RefreshService orquesta la rotación de E003.
type RefreshService struct {
	store   RefreshStore
	issuer  AccessTokenIssuer
	refresh func() (string, error)
	now     func() time.Time
}

// NewRefreshService construye el servicio. Si falta la persistencia o el emisor de JWT,
// Refresh falla cerrado con ErrRefreshUnavailable: nunca emite tokens ficticios.
func NewRefreshService(store RefreshStore, issuer AccessTokenIssuer) *RefreshService {
	return &RefreshService{
		store:   store,
		issuer:  issuer,
		refresh: newOpaqueToken,
		now:     func() time.Time { return time.Now().UTC() },
	}
}

// WithClock permite inyectar un reloj determinista en pruebas.
func (s *RefreshService) WithClock(now func() time.Time) *RefreshService {
	if now != nil {
		s.now = now
	}
	return s
}

// WithRefreshGenerator permite inyectar el generador del refresh opaco en pruebas.
func (s *RefreshService) WithRefreshGenerator(gen func() (string, error)) *RefreshService {
	if gen != nil {
		s.refresh = gen
	}
	return s
}

// ValidateRefreshCommand valida el cuerpo de E003 sin normalizar el token: el refresh
// opaco es sensible a mayúsculas y espacios, y nunca se registra en logs.
func ValidateRefreshCommand(in RefreshCommand) (RefreshCommand, error) {
	var out RefreshCommand

	if in.RefreshToken == "" {
		return out, &FieldViolation{Field: "refreshToken", Reason: "required"}
	}
	if utf8.RuneCountInString(in.RefreshToken) > RefreshMaxTokenRunes {
		return out, &FieldViolation{Field: "refreshToken", Reason: "too_long"}
	}
	for _, r := range in.RefreshToken {
		// El emisor usa base64url sin relleno; se rechaza cualquier carácter de control o
		// espacio antes de tocar la persistencia.
		if r <= 0x20 || r == 0x7f {
			return out, &FieldViolation{Field: "refreshToken", Reason: "invalid_value"}
		}
	}
	out.RefreshToken = in.RefreshToken

	deviceID := strings.TrimSpace(in.DeviceID)
	if utf8.RuneCountInString(deviceID) > maxRefreshDeviceIDRunes || hasControlRune(deviceID) {
		return out, &FieldViolation{Field: "deviceId", Reason: "invalid_value"}
	}
	out.DeviceID = deviceID
	out.ClientIP = in.ClientIP

	return out, nil
}

// Refresh rota el refresh token presentado y devuelve el par nuevo.
//
// Secuencia: resolver el digest → leer la sesión autoritativa con consistencia fuerte →
// validar estado, expiración y coherencia → generar el token nuevo → aplicar la rotación
// en UNA transacción condicional. Los tokens sólo se devuelven si la transacción confirma.
func (s *RefreshService) Refresh(ctx context.Context, in RefreshCommand) (*AuthTokens, error) {
	if s == nil || s.store == nil || s.issuer == nil {
		return nil, ErrRefreshUnavailable
	}

	cmd, err := ValidateRefreshCommand(in)
	if err != nil {
		return nil, err
	}

	digest := HashToken(cmd.RefreshToken)
	now := s.now().UTC()

	locator, errLocate := s.store.LocateRefresh(ctx, digest)
	if errLocate != nil {
		return nil, ErrRefreshUnavailable
	}
	if locator == nil {
		return nil, ErrInvalidRefreshToken
	}
	if err := s.rejectConsumedOrRevoked(ctx, locator, digest, now); err != nil {
		return nil, err
	}

	session, errSession := s.store.LoadSession(ctx, locator.UserID, locator.SessionID)
	if errSession != nil {
		return nil, ErrRefreshUnavailable
	}
	if session == nil {
		return nil, ErrInvalidRefreshToken
	}

	if err := verifySessionState(session, locator, digest, cmd.DeviceID, now); err != nil {
		if errors.Is(err, ErrRefreshReplay) {
			return nil, s.revokeForReplay(ctx, session, digest, now)
		}
		return nil, err
	}

	// La cuenta debe seguir ACTIVE: una sesión viva de un usuario deshabilitado o
	// eliminado no puede renovar tokens.
	user, errUser := s.store.LoadUserProfile(ctx, session.UserID)
	if errUser != nil {
		return nil, ErrRefreshUnavailable
	}
	if err := verifyUserState(user, session.UserID); err != nil {
		return nil, err
	}

	accessToken, jti, newRefresh, newDigest, errIssue := s.issueNext(ctx, session, now)
	if errIssue != nil {
		return nil, errIssue
	}

	rotation := RefreshRotation{
		UserID:          session.UserID,
		SessionID:       session.SessionID,
		FamilyID:        session.FamilyID,
		OldDigest:       digest,
		NewDigest:       newDigest,
		NewAccessJTI:    jti,
		ExpectedVersion: session.Version,
		NewVersion:      session.Version + 1,
		ExpiresAt:       session.ExpiresAt,
		Now:             now,
	}
	if err := s.store.RotateRefresh(ctx, rotation); err != nil {
		if errors.Is(err, ErrRefreshConflict) {
			return nil, s.classifyConflict(ctx, digest, now)
		}
		return nil, ErrRefreshUnavailable
	}

	return &AuthTokens{
		AccessToken:  accessToken,
		RefreshToken: newRefresh,
		TokenType:    TokenType,
		ExpiresIn:    int64(AccessTokenTTL / time.Second),
		SessionID:    session.SessionID,
		Session:      RefreshSessionRef{ID: session.SessionID},
	}, nil
}

// rejectConsumedOrRevoked aplica la política de reutilización sobre el localizador antes
// de leer la sesión. La decisión autoritativa sigue siendo la transacción condicional.
func (s *RefreshService) rejectConsumedOrRevoked(ctx context.Context, locator *RefreshLocator, digest string, now time.Time) error {
	if statusMatches(locator.Status, StatusRevoked) {
		// Revocado por logout, por borrado de cuenta o por una reutilización anterior.
		return ErrInvalidRefreshToken
	}
	if locator.Used || statusMatches(locator.Status, StatusConsumed) {
		// El token ya se consumió: reutilización. Se revoca la familia de forma durable
		// antes de responder, para que el cliente legítimo deje de tener acceso.
		session := &RefreshSession{
			UserID:    locator.UserID,
			SessionID: locator.SessionID,
			FamilyID:  locator.FamilyID,
		}
		return s.revokeForReplay(ctx, session, digest, now)
	}
	if locator.HasExpiry && !now.Before(locator.ExpiresAt) {
		return ErrInvalidRefreshToken
	}
	if locator.Status != "" && !statusMatches(locator.Status, StatusActive) {
		return ErrInvalidRefreshToken
	}
	return nil
}

// verifySessionState comprueba coherencia localizador↔sesión, estado, expiración y, si el
// contrato recibió `deviceId`, la correspondencia con el dispositivo que creó la sesión.
// `deviceId` es una señal de consistencia: nunca autoriza por sí solo.
func verifySessionState(session *RefreshSession, locator *RefreshLocator, digest, deviceID string, now time.Time) error {
	if session.UserID != locator.UserID || session.SessionID != locator.SessionID {
		return ErrInvalidRefreshToken
	}
	if locator.FamilyID != "" && session.FamilyID != "" && locator.FamilyID != session.FamilyID {
		return ErrInvalidRefreshToken
	}
	if !statusMatches(session.Status, StatusActive) {
		return ErrInvalidRefreshToken
	}
	if session.HasExpiry && !now.Before(session.ExpiresAt) {
		return ErrInvalidRefreshToken
	}
	if session.RefreshHash != digest {
		// La sesión ya apunta a otro digest y el localizador presentado no está marcado
		// como consumido: incoherencia que sólo puede venir de una rotación previa o de
		// una manipulación. Se trata como reutilización.
		return ErrRefreshReplay
	}
	if session.DeviceID != "" && deviceID != "" && !strings.EqualFold(session.DeviceID, deviceID) {
		// No revela el dispositivo persistido y no revoca: puede ser un fallo del cliente.
		return ErrInvalidRefreshToken
	}
	return nil
}

// verifyUserState exige que el perfil exista, pertenezca al mismo usuario y esté ACTIVE.
func verifyUserState(user *RefreshUser, sessionUserID string) error {
	if user == nil {
		return ErrInvalidRefreshToken
	}
	if user.UserID != "" && user.UserID != sessionUserID {
		return ErrInvalidRefreshToken
	}
	if !statusMatches(user.Status, StatusActive) {
		return ErrInvalidRefreshToken
	}
	return nil
}

// issueNext genera el access token, su jti y el refresh opaco con su digest.
func (s *RefreshService) issueNext(ctx context.Context, session *RefreshSession, now time.Time) (string, string, string, string, error) {
	accessToken, jti, err := s.issuer.IssueAccessToken(ctx, session.UserID, session.SessionID, now)
	if err != nil {
		return "", "", "", "", ErrRefreshUnavailable
	}
	newRefresh, err := s.refresh()
	if err != nil || newRefresh == "" {
		return "", "", "", "", ErrRefreshUnavailable
	}
	newDigest := HashToken(newRefresh)
	if newDigest == session.RefreshHash {
		// Un generador defectuoso no puede repetir el digest vigente.
		return "", "", "", "", ErrRefreshUnavailable
	}
	return accessToken, jti, newRefresh, newDigest, nil
}

// revokeForReplay revoca durablemente la familia y devuelve el error público de replay.
// Si la revocación no se confirma, falla cerrado con 503: no se informa de una revocación
// que no ocurrió.
func (s *RefreshService) revokeForReplay(ctx context.Context, session *RefreshSession, presentedDigest string, now time.Time) error {
	err := s.store.RevokeFamily(ctx, RefreshRevocation{
		UserID:          session.UserID,
		SessionID:       session.SessionID,
		FamilyID:        session.FamilyID,
		PresentedDigest: presentedDigest,
		CurrentDigest:   session.RefreshHash,
		Now:             now,
	})
	if err != nil {
		return ErrRefreshUnavailable
	}
	return ErrRefreshReplay
}

// classifyConflict reacciona a un compare-and-swap perdido. Es obligatorio releer el
// estado durable: si el token presentado quedó consumido, es reutilización; si no hay
// evidencia, es un conflicto de concurrencia que no revoca nada.
func (s *RefreshService) classifyConflict(ctx context.Context, digest string, now time.Time) error {
	locator, err := s.store.LocateRefresh(ctx, digest)
	if err != nil {
		return ErrRefreshUnavailable
	}
	if locator == nil {
		return ErrInvalidRefreshToken
	}
	if statusMatches(locator.Status, StatusRevoked) {
		return ErrInvalidRefreshToken
	}
	if locator.Used || statusMatches(locator.Status, StatusConsumed) {
		session := &RefreshSession{
			UserID:    locator.UserID,
			SessionID: locator.SessionID,
			FamilyID:  locator.FamilyID,
		}
		return s.revokeForReplay(ctx, session, digest, now)
	}

	session, err := s.store.LoadSession(ctx, locator.UserID, locator.SessionID)
	if err != nil {
		return ErrRefreshUnavailable
	}
	if session == nil || !statusMatches(session.Status, StatusActive) {
		return ErrInvalidRefreshToken
	}
	if session.HasExpiry && !now.Before(session.ExpiresAt) {
		return ErrInvalidRefreshToken
	}
	if session.RefreshHash != digest {
		return s.revokeForReplay(ctx, session, digest, now)
	}
	// La condición de usuario ACTIVE pudo ser la que abortó la transacción.
	user, errUser := s.store.LoadUserProfile(ctx, session.UserID)
	if errUser != nil {
		return ErrRefreshUnavailable
	}
	if err := verifyUserState(user, session.UserID); err != nil {
		return err
	}
	return ErrRefreshConflict
}

// statusMatches compara estados persistidos sin depender de mayúsculas históricas.
func statusMatches(value, expected string) bool {
	return strings.EqualFold(strings.TrimSpace(value), expected)
}
