package identity

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/nexumpr/Nexum-Backend/pkg/httputil"
)

// Reglas de E008 (`GET /api/v1/auth/sessions`): enumerar las sesiones del titular.
//
// El contrato (OpenAPI 3.1, operationId `nexum_e008`) exige `Authorization: Bearer` y devuelve
// `PageOfSession` con `items` (array de `Session`) y `nextCursor` opcional. `Session` es un
// objeto cerrado (`additionalProperties: false`) con EXACTAMENTE cinco campos: `id`,
// `createdAt`, `lastSeenAt`, `current` y `revoked`.
//
// Invariantes de esta capa:
//   - La identidad llega exclusivamente de `sub` del JWT verificado; el cliente no elige el
//     usuario ni puede enumerar sesiones ajenas.
//   - La lectura es un `Query` particionado (`PK = USER#<id>` + `begins_with(SK, "SESSION#")`).
//     Nunca se usa `Scan` (A-060 §2.5, E008 §4).
//   - No se expone `refreshHash`, `accessJti`, `familyId`, IP, agente de usuario ni ningún otro
//     atributo interno: sólo los cinco campos del contrato (E008 §1).
var (
	// ErrSessionsInvalidRequest cubre límite fuera de rango, cursor malformado, manipulado,
	// vencido o perteneciente a otro usuario, y un cursor emitido con otros filtros. Todos se
	// responden 400 INVALID_REQUEST sin revelar el motivo exacto.
	ErrSessionsInvalidRequest = errors.New("solicitud de sesiones invalida")

	// ErrSessionsUnauthorized indica que el token no identifica usuario o sesión. El
	// middleware ya lo impide; esta guarda es defensa en profundidad (401).
	ErrSessionsUnauthorized = errors.New("sesiones no autorizadas")

	// ErrSessionsUnavailable indica falta de persistencia o de clave de firma de cursor. Falla
	// cerrado con 503: nunca se devuelve una lista sin poder verificarla ni paginarla de forma
	// segura.
	ErrSessionsUnavailable = errors.New("servicio de sesiones no disponible")
)

const (
	// SessionsDefaultLimit es el tamaño de página por defecto del contrato (25).
	SessionsDefaultLimit = httputil.DefaultPageLimit

	// SessionsMaxLimit es el tamaño de página máximo admitido por el contrato (100).
	SessionsMaxLimit = httputil.MaxPageLimit

	// SessionsCursorMinLength y SessionsCursorMaxLength replican `minLength: 8` y
	// `maxLength: 2048` del parámetro `cursor`.
	SessionsCursorMinLength = 8
	SessionsCursorMaxLength = 2048

	// sessionsSortKeyDomain identifica el orden y el conjunto de filtros con los que se emitió
	// un cursor. Si el cliente cambia `limit` y reutiliza un cursor antiguo, el `SortBy` del
	// cursor no coincide y la petición se rechaza con 400 (E008 §5, T06).
	sessionsSortKeyDomain = "sessions:v1"
)

// SessionListItem es una sesión ya reducida a lo que el contrato permite exponer.
type SessionListItem struct {
	SessionID  string
	CreatedAt  time.Time
	LastSeenAt time.Time
	Revoked    bool
}

// SessionsPage es el resultado de aplicación de una página de sesiones.
type SessionsPage struct {
	Items      []SessionListItem
	NextCursor string
}

// SessionsQuery es la entrada de aplicación de E008. `UserID` proviene de `sub`; nunca del
// cliente.
type SessionsQuery struct {
	UserID string
	Limit  int
	Cursor string
}

// SessionListRecord es una fila leída de la partición del usuario. `SortKey` es la `SK`
// completa (`SESSION#<id>`) y es lo único que se codifica en el cursor: el cursor nunca lleva
// el identificador de usuario en claro más allá de la firma HMAC.
type SessionListRecord struct {
	SortKey    string
	SessionID  string
	CreatedAt  time.Time
	LastSeenAt time.Time
	Status     string
}

// SessionsStore es el puerto de persistencia de E008. Devuelve las filas de la página y la `SK`
// desde la que continuar (vacía cuando no hay más). Nunca usa Scan.
type SessionsStore interface {
	ListUserSessions(ctx context.Context, userID, startSK string, limit int) ([]SessionListRecord, string, error)
}

// SessionsService orquesta la enumeración paginada.
type SessionsService struct {
	store     SessionsStore
	secret    []byte
	now       func() time.Time
	cursorTTL time.Duration
}

// NewSessionsService construye el servicio. Sin almacén o sin clave de firma de cursor todas
// las llamadas fallan cerrado con ErrSessionsUnavailable: no se emite un cursor sin firma.
func NewSessionsService(store SessionsStore, secret []byte) *SessionsService {
	return &SessionsService{
		store:     store,
		secret:    secret,
		now:       func() time.Time { return time.Now().UTC() },
		cursorTTL: httputil.DefaultCursorTTL,
	}
}

// WithClock permite inyectar un reloj determinista en pruebas.
func (s *SessionsService) WithClock(now func() time.Time) *SessionsService {
	if now != nil {
		s.now = now
	}
	return s
}

// WithCursorTTL permite acortar la vigencia del cursor en pruebas.
func (s *SessionsService) WithCursorTTL(ttl time.Duration) *SessionsService {
	if ttl != 0 {
		s.cursorTTL = ttl
	}
	return s
}

// ListSessions devuelve una página de sesiones del titular.
//
// Secuencia: validar límite → validar y verificar el cursor (firma, expiración, pertenencia y
// filtros) → consultar la partición del usuario → reducir cada fila a los cinco campos del
// contrato → emitir el cursor siguiente sólo si la persistencia confirmó que hay más filas.
func (s *SessionsService) ListSessions(ctx context.Context, query SessionsQuery) (*SessionsPage, error) {
	if s == nil || s.store == nil || len(s.secret) == 0 {
		return nil, ErrSessionsUnavailable
	}

	userID := strings.TrimSpace(query.UserID)
	if userID == "" {
		return nil, ErrSessionsUnauthorized
	}

	limit, err := NormalizeSessionsLimit(query.Limit)
	if err != nil {
		return nil, err
	}

	startSK := ""
	if raw := strings.TrimSpace(query.Cursor); raw != "" {
		if len(raw) < SessionsCursorMinLength || len(raw) > SessionsCursorMaxLength {
			return nil, ErrSessionsInvalidRequest
		}
		payload, errCursor := httputil.ValidateCursor(s.secret, raw, userID)
		if errCursor != nil {
			// Manipulado, corrupto, vencido o de otro usuario: un único 400 sin distinguir.
			return nil, ErrSessionsInvalidRequest
		}
		if payload.SortBy != sessionsSortKey(limit) {
			// Cursor emitido con otros filtros (por ejemplo otro `limit`).
			return nil, ErrSessionsInvalidRequest
		}
		if !strings.HasPrefix(payload.LastID, "SESSION#") {
			return nil, ErrSessionsInvalidRequest
		}
		startSK = payload.LastID
	}

	records, nextSK, err := s.store.ListUserSessions(ctx, userID, startSK, limit)
	if err != nil {
		return nil, ErrSessionsUnavailable
	}

	items := make([]SessionListItem, 0, len(records))
	for _, record := range records {
		items = append(items, SessionListItem{
			SessionID:  record.SessionID,
			CreatedAt:  record.CreatedAt.UTC(),
			LastSeenAt: record.LastSeenAt.UTC(),
			Revoked:    SessionIsRevoked(record.Status),
		})
	}

	page := &SessionsPage{Items: items}
	if nextSK != "" {
		cursor, errGen := httputil.GenerateCursor(s.secret, userID, sessionsSortKey(limit), nextSK, s.now().UTC(), s.cursorTTL)
		if errGen != nil {
			return nil, ErrSessionsUnavailable
		}
		page.NextCursor = cursor
	}
	return page, nil
}

// NormalizeSessionsLimit valida el parámetro `limit` del contrato (1..100, por defecto 25).
// Un valor cero se interpreta como ausencia del parámetro; cualquier otro fuera de rango es
// 400, nunca se recorta en silencio.
func NormalizeSessionsLimit(limit int) (int, error) {
	if limit == 0 {
		return SessionsDefaultLimit, nil
	}
	if limit < 1 || limit > SessionsMaxLimit {
		return 0, ErrSessionsInvalidRequest
	}
	return limit, nil
}

// SessionIsRevoked aplica la regla del contrato: `revoked = status != "ACTIVE"`, con la
// compatibilidad heredada de que un ítem sin `status` se considera activo (misma semántica que
// `statusMatchesActive` de `internal/adapters/dynamo/logout_store.go`).
func SessionIsRevoked(status string) bool {
	if strings.TrimSpace(status) == "" {
		return false
	}
	return !statusMatches(status, StatusActive)
}

// sessionsSortKey liga el cursor al conjunto de filtros con el que se emitió.
func sessionsSortKey(limit int) string {
	return sessionsSortKeyDomain + ":limit=" + strconv.Itoa(limit)
}
