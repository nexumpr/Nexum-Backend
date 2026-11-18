// Package privacy implementa los casos de uso de privacidad del titular
// (E016 `GET /api/v1/users/me/consents` y, en su extensión, E017
// `POST /api/v1/users/me/consents`).
//
// Invariantes de esta capa:
//   - La identidad llega exclusivamente de `sub` del JWT verificado: el cliente no puede
//     elegir el usuario por query, cuerpo ni cabecera ni enumerar consentimientos ajenos
//     (E016 §1, R-031).
//   - La lectura es un `Query` particionado (`PK = USER#<id>` +
//     `begins_with(SK, "CONSENT#")`), en orden descendente y paginado por cursor opaco.
//     Nunca se usa `Scan` (A-060 §2.5, E016 §4).
//   - El historial es inmutable: un consentimiento concedido o retirado se conserva. Esta
//     capa no reescribe ni borra ítems existentes, incluidos los de E001.
//   - Ningún error contiene identificadores de usuario, versiones de aviso ni datos
//     personales: los mensajes son genéricos y no enumerables.
package privacy

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/nexumpr/Nexum-Backend/pkg/httputil"
)

// Errores de aplicación de E016. El transporte los traduce a los códigos del contrato sin
// revelar el motivo exacto al cliente.
var (
	// ErrConsentsInvalidRequest cubre límite fuera de rango, cursor malformado, manipulado,
	// vencido o perteneciente a otro usuario, y un cursor emitido con otros filtros. Todos se
	// responden 400 INVALID_REQUEST.
	ErrConsentsInvalidRequest = errors.New("solicitud de consentimientos invalida")

	// ErrConsentsUnauthorized indica que no hay sujeto autenticado (401). El middleware ya lo
	// impide; esta guarda es defensa en profundidad.
	ErrConsentsUnauthorized = errors.New("consentimientos no autorizados")

	// ErrConsentsUnavailable indica falta de persistencia o de clave de firma de cursor. Falla
	// cerrado con 503: nunca se devuelve una lista sin poder verificarla ni paginarla de forma
	// segura.
	ErrConsentsUnavailable = errors.New("servicio de consentimientos no disponible")
)

const (
	// ConsentsDefaultLimit es el tamaño de página por defecto del contrato (25).
	ConsentsDefaultLimit = httputil.DefaultPageLimit

	// ConsentsMaxLimit es el tamaño de página máximo admitido por el contrato (100).
	ConsentsMaxLimit = httputil.MaxPageLimit

	// ConsentsCursorMinLength y ConsentsCursorMaxLength replican `minLength: 8` y
	// `maxLength: 2048` del parámetro `cursor`.
	ConsentsCursorMinLength = 8
	ConsentsCursorMaxLength = 2048

	// Propósitos cerrados del contrato `Consent.purpose`.
	ConsentPurposePrivacyNotice = "privacyNotice"
	ConsentPurposeNotifications = "notifications"
	ConsentPurposeAnalytics     = "analytics"

	// consentsSortKeyDomain identifica el orden y el conjunto de filtros con los que se emitió
	// un cursor. Si el cliente cambia `limit` y reutiliza un cursor antiguo, el `SortBy` del
	// cursor no coincide y la petición se rechaza con 400.
	consentsSortKeyDomain = "consents:v1"
)

// Consent es el valor de dominio que corresponde al esquema `Consent`: exactamente los cinco
// campos del contrato, sin atributos internos del ítem.
type Consent struct {
	ID            string
	NoticeVersion string
	Purpose       string
	Granted       bool
	RecordedAt    time.Time
}

// ConsentsPage es el resultado de aplicación de una página de consentimientos.
type ConsentsPage struct {
	Items      []Consent
	NextCursor string
}

// ConsentsQuery es la entrada de aplicación de E016. `UserID` proviene de `sub`; nunca del
// cliente.
type ConsentsQuery struct {
	UserID string
	Limit  int
	Cursor string
}

// ConsentRecord es una fila leída de la partición del usuario. `SortKey` es la `SK` completa
// (`CONSENT#...`) y es lo único que se codifica en el cursor.
type ConsentRecord struct {
	SortKey       string
	ID            string
	NoticeVersion string
	Purpose       string
	Granted       bool
	RecordedAt    time.Time
}

// ConsentsStore es el puerto de persistencia de E016. Devuelve las filas de la página y la `SK`
// desde la que continuar (vacía cuando no hay más). Nunca usa Scan.
type ConsentsStore interface {
	ListUserConsents(ctx context.Context, userID, startSK string, limit int) ([]ConsentRecord, string, error)
}

// ConsentsService orquesta la enumeración paginada del historial de consentimientos.
type ConsentsService struct {
	store     ConsentsStore
	secret    []byte
	now       func() time.Time
	cursorTTL time.Duration
}

// NewConsentsService construye el servicio. Sin almacén o sin clave de firma de cursor todas
// las llamadas fallan cerrado con ErrConsentsUnavailable: no se emite un cursor sin firma.
func NewConsentsService(store ConsentsStore, secret []byte) *ConsentsService {
	return &ConsentsService{
		store:     store,
		secret:    secret,
		now:       func() time.Time { return time.Now().UTC() },
		cursorTTL: httputil.DefaultCursorTTL,
	}
}

// WithClock permite inyectar un reloj determinista en pruebas.
func (s *ConsentsService) WithClock(now func() time.Time) *ConsentsService {
	if now != nil {
		s.now = now
	}
	return s
}

// WithCursorTTL permite acortar la vigencia del cursor en pruebas.
func (s *ConsentsService) WithCursorTTL(ttl time.Duration) *ConsentsService {
	if ttl != 0 {
		s.cursorTTL = ttl
	}
	return s
}

// List devuelve una página del historial de consentimientos del titular.
//
// Secuencia: validar límite → validar y verificar el cursor (firma, expiración, pertenencia y
// filtros) → consultar la partición del usuario → reducir cada fila a los cinco campos del
// contrato → emitir el cursor siguiente sólo si la persistencia confirmó que hay más filas.
func (s *ConsentsService) List(ctx context.Context, query ConsentsQuery) (*ConsentsPage, error) {
	if s == nil || s.store == nil || len(s.secret) == 0 {
		return nil, ErrConsentsUnavailable
	}

	userID := strings.TrimSpace(query.UserID)
	if userID == "" {
		return nil, ErrConsentsUnauthorized
	}

	limit, err := NormalizeConsentsLimit(query.Limit)
	if err != nil {
		return nil, err
	}

	startSK := ""
	if raw := strings.TrimSpace(query.Cursor); raw != "" {
		if len(raw) < ConsentsCursorMinLength || len(raw) > ConsentsCursorMaxLength {
			return nil, ErrConsentsInvalidRequest
		}
		payload, errCursor := httputil.ValidateCursor(s.secret, raw, userID)
		if errCursor != nil {
			// Manipulado, corrupto, vencido o de otro usuario: un único 400 sin distinguir.
			return nil, ErrConsentsInvalidRequest
		}
		if payload.SortBy != consentsSortKey(limit) {
			// Cursor emitido con otros filtros (por ejemplo otro `limit`).
			return nil, ErrConsentsInvalidRequest
		}
		// El cursor sólo puede apuntar a una clave de consentimiento de este titular.
		if !strings.HasPrefix(payload.LastID, "CONSENT#") {
			return nil, ErrConsentsInvalidRequest
		}
		startSK = payload.LastID
	}

	records, nextSK, err := s.store.ListUserConsents(ctx, userID, startSK, limit)
	if err != nil {
		return nil, ErrConsentsUnavailable
	}

	items := make([]Consent, 0, len(records))
	for _, record := range records {
		items = append(items, Consent{
			ID:            record.ID,
			NoticeVersion: record.NoticeVersion,
			Purpose:       record.Purpose,
			Granted:       record.Granted,
			RecordedAt:    record.RecordedAt.UTC(),
		})
	}

	page := &ConsentsPage{Items: items}
	if nextSK != "" {
		cursor, errGen := httputil.GenerateCursor(s.secret, userID, consentsSortKey(limit), nextSK, s.now().UTC(), s.cursorTTL)
		if errGen != nil {
			return nil, ErrConsentsUnavailable
		}
		page.NextCursor = cursor
	}
	return page, nil
}

// NormalizeConsentsLimit valida el parámetro `limit` del contrato (1..100, por defecto 25).
// Un valor cero se interpreta como ausencia del parámetro; cualquier otro fuera de rango es
// 400, nunca se recorta en silencio.
func NormalizeConsentsLimit(limit int) (int, error) {
	if limit == 0 {
		return ConsentsDefaultLimit, nil
	}
	if limit < 1 || limit > ConsentsMaxLimit {
		return 0, ErrConsentsInvalidRequest
	}
	return limit, nil
}

// consentsSortKey liga el cursor al conjunto de filtros con el que se emitió.
func consentsSortKey(limit int) string {
	return consentsSortKeyDomain + ":limit=" + strconv.Itoa(limit)
}

// IsConsentPurpose informa de si el valor pertenece al enum cerrado del contrato.
func IsConsentPurpose(purpose string) bool {
	switch purpose {
	case ConsentPurposePrivacyNotice, ConsentPurposeNotifications, ConsentPurposeAnalytics:
		return true
	default:
		return false
	}
}
