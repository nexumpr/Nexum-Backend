// Package notifications contiene los casos de uso de E061 (`GET /notifications`),
// E062 (`PATCH /notifications/{notificationId}/read`) y E063
// (`POST /notifications/read-all`).
//
// Invariantes de dominio:
//   - El titular llega siempre del claim `sub` verificado por el middleware; el
//     cliente nunca puede elegir la partición que se consulta.
//   - El cursor es opaco, HMAC-SHA256 y está ligado a identidad, filtros, límite
//     y LastEvaluatedKey. Un cursor modificado o de otro usuario se rechaza.
//   - `unreadCount` se calcula sobre TODA la bandeja con un Query COUNT separado;
//     nunca se deriva del tamaño de la página ni se mantiene como contador local.
//   - E062 es idempotente: la escritura condicional la hace el adaptador y el
//     servicio sólo informa del estado resultante.
//   - E063 persiste el registro READALL antes de devolver 202 y procesa páginas
//     con cursor/lease. Si falla a mitad, el mismo requestId reanuda desde el
//     último LastEvaluatedKey persistido.
package notifications

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/nexumpr/Nexum-Backend/internal/domain/identity"
)

const (
	// DefaultLimit y MaxLimit son los valores del contrato OpenAPI de E061.
	DefaultLimit = 25
	MaxLimit     = 100

	// MinCursorLength y MaxCursorLength acotan el parámetro `cursor` del contrato.
	MinCursorLength = 8
	MaxCursorLength = 2048

	// ReadAllBatchSize es el tamaño máximo de cada página del fan-out de E063.
	ReadAllBatchSize = 25

	// StatusPending/StatusCompleted son los estados del registro READALL.
	StatusPending   = "pending"
	StatusCompleted = "completed"

	readAllLeaseTTL    = 2 * time.Minute
	readAllMaxBatches  = 10000
	cursorDomainMarker = "nexum:e061:cursor:v1:"
)

// Errores de aplicación. El transporte los traduce a códigos del contrato sin
// revelar claves, cursores ni identificadores de otros titulares.
var (
	ErrInvalidRequest = errors.New("solicitud de notificaciones inválida")
	ErrUnauthorized   = errors.New("notificaciones no autorizadas")
	ErrNotFound       = errors.New("notificación no encontrada")
	ErrConflict       = errors.New("conflicto al procesar notificaciones")
	ErrUnavailable    = errors.New("servicio de notificaciones no disponible")
)

// Type es el discriminador del contrato (`budget|goal|recurring|streak|system`).
type Type string

const (
	TypeBudget    Type = "budget"
	TypeGoal      Type = "goal"
	TypeRecurring Type = "recurring"
	TypeStreak    Type = "streak"
	TypeSystem    Type = "system"
)

// ParseType valida el filtro `type` sin depender del transporte.
func ParseType(raw string) (Type, bool) {
	t := Type(strings.TrimSpace(raw))
	switch t {
	case TypeBudget, TypeGoal, TypeRecurring, TypeStreak, TypeSystem:
		return t, true
	default:
		return "", false
	}
}

// Notification es la entidad leída de la tabla única. Sólo se materializa a
// partir de items reales: el servicio nunca genera notificaciones sintéticas.
type Notification struct {
	ID             string
	Title          string
	Message        string
	Type           Type
	IsRead         bool
	CreatedAt      time.Time
	ReadAt         *time.Time
	DeliveryStatus string
	// UnreadCount se rellena sólo en E062 con el total global posterior a la
	// transición; en listados se omite y el total viaja en PageOfNotification.
	UnreadCount *int64
}

// WireNotification es la proyección estricta del esquema `Notification`.
// `unreadCount` es opcional y se incluye únicamente en E062, donde el contrato
// de la enmienda RFC-016 pide el total global también en la respuesta del ítem.
type WireNotification struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Message     string `json:"message"`
	IsRead      bool   `json:"isRead"`
	CreatedAt   string `json:"createdAt"`
	UnreadCount *int64 `json:"unreadCount,omitempty"`
}

// Wire convierte la entidad al contrato de transporte.
func (n Notification) Wire() WireNotification {
	return WireNotification{
		ID:          n.ID,
		Title:       n.Title,
		Message:     n.Message,
		IsRead:      n.IsRead,
		CreatedAt:   n.CreatedAt.UTC().Format(time.RFC3339Nano),
		UnreadCount: n.UnreadCount,
	}
}

// Page es el resultado de E061 con el total global de no leídas.
type Page struct {
	Items       []Notification
	NextCursor  string
	UnreadCount int64
}

// WirePage es la proyección estricta de `PageOfNotification`.
type WirePage struct {
	Items       []WireNotification `json:"items"`
	NextCursor  string             `json:"nextCursor,omitempty"`
	UnreadCount int64              `json:"unreadCount"`
}

// Wire convierte la página al contrato de transporte.
func (p Page) Wire() WirePage {
	items := make([]WireNotification, 0, len(p.Items))
	for _, n := range p.Items {
		items = append(items, n.Wire())
	}
	return WirePage{Items: items, NextCursor: p.NextCursor, UnreadCount: p.UnreadCount}
}

// Accepted es el resultado de E063.
type Accepted struct {
	Status    string
	RequestID string
}

// WireAccepted es la proyección estricta del esquema `Accepted`.
type WireAccepted struct {
	Status    string `json:"status"`
	RequestID string `json:"requestId"`
}

// Wire convierte la aceptación al contrato de transporte.
func (a Accepted) Wire() WireAccepted {
	return WireAccepted(a)
}

// NotificationKey identifica un item de forma inequívoca dentro de la tabla
// única. Se guarda firmada en el cursor de E061 y en el registro READALL.
type NotificationKey struct {
	PK string
	SK string
}

// ListQuery es la consulta que el servicio pide al adaptador. El filtro se
// aplica en DynamoDB (FilterExpression) y se vuelve a comprobar al proyectar.
type ListQuery struct {
	UserID string
	Limit  int
	IsRead *bool
	Type   Type
	After  *NotificationKey
}

// ListResult contiene los items de una página y el LastEvaluatedKey devuelto
// por DynamoDB. `NextKey` es nil cuando no hay más páginas.
type ListResult struct {
	Items   []Notification
	NextKey *NotificationKey
}

// ReadAllState es el registro `USER#<sub> / READALL#<requestId>`.
type ReadAllState struct {
	RequestID      string
	Status         string
	CreatedAt      time.Time
	ProcessedCount int64
	// InitialUnread se persiste al crear el registro. Con él, el total
	// procesado es inicial - final aunque un lote haya fallado a mitad.
	InitialUnread int64
	CompletedAt   *time.Time
	Cursor        *NotificationKey
	LeaseOwner    string
	LeaseUntil    time.Time
}

// Store es el puerto de persistencia. La implementación productiva es
// *dynamo.NotificationsStore; nunca se sustituye por memoria en producción.
type Store interface {
	List(ctx context.Context, query ListQuery) (*ListResult, error)
	CountUnread(ctx context.Context, userID string) (int64, error)
	MarkRead(ctx context.Context, userID, notificationID string, now time.Time) (*Notification, error)
	BeginReadAll(ctx context.Context, userID, requestID string, now time.Time) (*ReadAllState, error)
	LoadReadAll(ctx context.Context, userID, requestID string) (*ReadAllState, error)
	AdvanceReadAll(ctx context.Context, userID, expectedLeaseOwner string, next ReadAllState) error
	CompleteReadAll(ctx context.Context, userID, requestID, expectedLeaseOwner string, processed int64, now time.Time) error
}

// ListParams son los parámetros validados por el transporte para E061.
type ListParams struct {
	Limit  int
	Cursor string
	IsRead *bool
	Type   string
}

// cursorPayload es el contenido firmado del cursor de E061.
type cursorPayload struct {
	Version   int    `json:"v"`
	UserID    string `json:"uid"`
	Limit     int    `json:"lim"`
	IsReadSet bool   `json:"irs"`
	IsRead    bool   `json:"ir"`
	Type      string `json:"typ"`
	PK        string `json:"pk"`
	SK        string `json:"sk"`
}

type signedCursor struct {
	Payload   cursorPayload `json:"p"`
	Signature string        `json:"s"`
}

// Service orquesta los tres casos de uso. Sin almacén o sin clave de firma las
// operaciones fallan cerrado: no se inventan items ni se aceptan cursores sin HMAC.
type Service struct {
	store     Store
	cursorKey []byte
	now       func() time.Time
	newID     func() (string, error)
	maxPages  int
}

// NewService construye el servicio con la clave HMAC estable inyectada por el
// montaje. Una clave vacía deja el serviciono disponible (fail-closed).
func NewService(store Store, cursorKey []byte) *Service {
	return &Service{
		store:     store,
		cursorKey: deriveCursorKey(cursorKey),
		now:       func() time.Time { return time.Now().UTC() },
		newID:     identity.NewID,
		maxPages:  readAllMaxBatches,
	}
}

// WithClock permite inyectar un reloj determinista en pruebas.
func (s *Service) WithClock(now func() time.Time) *Service {
	if s != nil && now != nil {
		s.now = now
	}
	return s
}

// WithIDGenerator permite inyectar generadores deterministas en pruebas.
func (s *Service) WithIDGenerator(newID func() (string, error)) *Service {
	if s != nil && newID != nil {
		s.newID = newID
	}
	return s
}

// Disponible indica si hay almacén y clave de cursores reales configurados.
func (s *Service) Disponible() bool { return s != nil && s.store != nil && len(s.cursorKey) > 0 }

func deriveCursorKey(base []byte) []byte {
	if len(base) == 0 {
		return nil
	}
	mac := hmac.New(sha256.New, base)
	mac.Write([]byte(cursorDomainMarker))
	return mac.Sum(nil)
}

// List devuelve una página filtrada y el total global de no leídas.
func (s *Service) List(ctx context.Context, userID string, params ListParams) (*Page, error) {
	if !s.Disponible() {
		return nil, ErrUnavailable
	}
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, ErrUnauthorized
	}
	limit := params.Limit
	if limit == 0 {
		limit = DefaultLimit
	}
	if limit < 1 || limit > MaxLimit {
		return nil, ErrInvalidRequest
	}

	tipo := Type("")
	if strings.TrimSpace(params.Type) != "" {
		parseada, ok := ParseType(params.Type)
		if !ok {
			return nil, ErrInvalidRequest
		}
		tipo = parseada
	}

	var after *NotificationKey
	if strings.TrimSpace(params.Cursor) != "" {
		payload, err := s.verifyCursor(params.Cursor, userID, limit, params.IsRead, tipo)
		if err != nil {
			return nil, err
		}
		after = &NotificationKey{PK: payload.PK, SK: payload.SK}
	}

	resultado, err := s.store.List(ctx, ListQuery{
		UserID: userID, Limit: limit, IsRead: params.IsRead, Type: tipo, After: after,
	})
	if err != nil {
		return nil, normalizeError(err)
	}
	if resultado == nil {
		return nil, ErrUnavailable
	}
	noLeidas, err := s.store.CountUnread(ctx, userID)
	if err != nil {
		return nil, normalizeError(err)
	}
	if noLeidas < 0 {
		noLeidas = 0
	}

	pagina := &Page{Items: resultado.Items, UnreadCount: noLeidas}
	if pagina.Items == nil {
		pagina.Items = []Notification{}
	}
	if resultado.NextKey != nil && resultado.NextKey.SK != "" {
		cursor, err := s.signCursor(cursorPayload{
			Version:   1,
			UserID:    userID,
			Limit:     limit,
			IsReadSet: params.IsRead != nil,
			IsRead:    params.IsRead != nil && *params.IsRead,
			Type:      string(tipo),
			PK:        resultado.NextKey.PK,
			SK:        resultado.NextKey.SK,
		})
		if err != nil {
			return nil, ErrUnavailable
		}
		pagina.NextCursor = cursor
	}
	return pagina, nil
}

func (s *Service) signCursor(p cursorPayload) (string, error) {
	raw, err := json.Marshal(p)
	if err != nil {
		return "", err
	}
	mac := hmac.New(sha256.New, s.cursorKey)
	mac.Write(raw)
	firmado := signedCursor{Payload: p, Signature: base64.RawURLEncoding.EncodeToString(mac.Sum(nil))}
	bytes, err := json.Marshal(firmado)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(bytes), nil
}

func (s *Service) verifyCursor(raw, userID string, limit int, isRead *bool, tipo Type) (*cursorPayload, error) {
	if len(raw) < MinCursorLength || len(raw) > MaxCursorLength {
		return nil, ErrInvalidRequest
	}
	bytes, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return nil, ErrInvalidRequest
	}
	var firmado signedCursor
	if err := json.Unmarshal(bytes, &firmado); err != nil {
		return nil, ErrInvalidRequest
	}
	payloadBytes, err := json.Marshal(firmado.Payload)
	if err != nil {
		return nil, ErrInvalidRequest
	}
	mac := hmac.New(sha256.New, s.cursorKey)
	mac.Write(payloadBytes)
	firma, err := base64.RawURLEncoding.DecodeString(firmado.Signature)
	if err != nil || !hmac.Equal(mac.Sum(nil), firma) {
		return nil, ErrInvalidRequest
	}
	if firmado.Payload.Version != 1 || firmado.Payload.UserID != userID ||
		firmado.Payload.Limit != limit || firmado.Payload.PK == "" || firmado.Payload.SK == "" {
		return nil, ErrInvalidRequest
	}
	if firmado.Payload.IsReadSet != (isRead != nil) {
		return nil, ErrInvalidRequest
	}
	if firmado.Payload.IsReadSet && firmado.Payload.IsRead != *isRead {
		return nil, ErrInvalidRequest
	}
	if firmado.Payload.Type != string(tipo) {
		return nil, ErrInvalidRequest
	}
	return &firmado.Payload, nil
}

// MarkRead ejecuta E062. El adaptador hace la transición atómica; repetir la
// petición devuelve el mismo item sin volver a contar su no lectura.
func (s *Service) MarkRead(ctx context.Context, userID, notificationID string) (*Notification, error) {
	if !s.Disponible() {
		return nil, ErrUnavailable
	}
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, ErrUnauthorized
	}
	if strings.TrimSpace(notificationID) == "" {
		return nil, ErrInvalidRequest
	}
	notificacion, err := s.store.MarkRead(ctx, userID, strings.TrimSpace(notificationID), s.now().UTC())
	if err != nil {
		return nil, normalizeError(err)
	}
	if notificacion == nil {
		return nil, ErrNotFound
	}
	noLeidas, err := s.store.CountUnread(ctx, userID)
	if err != nil {
		return nil, normalizeError(err)
	}
	if noLeidas < 0 {
		noLeidas = 0
	}
	notificacion.IsRead = true
	notificacion.UnreadCount = &noLeidas
	return notificacion, nil
}

// UnreadCount expone el total global para pruebas y para el montaje de E061.
func (s *Service) UnreadCount(ctx context.Context, userID string) (int64, error) {
	if !s.Disponible() {
		return 0, ErrUnavailable
	}
	n, err := s.store.CountUnread(ctx, strings.TrimSpace(userID))
	if err != nil {
		return 0, normalizeError(err)
	}
	if n < 0 {
		return 0, nil
	}
	return n, nil
}

// StartReadAll ejecuta E063: persiste el registro READALL y realiza el trabajo
// completo por páginas antes de devolver 202. Nunca confirma una escritura que
// no ha ocurrido.
func (s *Service) StartReadAll(ctx context.Context, userID string) (*Accepted, error) {
	return s.beginAndRun(ctx, userID, "")
}

// ResumeReadAll reanuda un requestId existente. Es la operación que se usa
// tras un fallo a mitad: el cursor y el lease viven en DynamoDB, no en memoria.
func (s *Service) ResumeReadAll(ctx context.Context, userID, requestID string) (*Accepted, error) {
	if strings.TrimSpace(requestID) == "" {
		return nil, ErrInvalidRequest
	}
	return s.beginAndRun(ctx, userID, strings.TrimSpace(requestID))
}

func (s *Service) beginAndRun(ctx context.Context, userID, requestID string) (*Accepted, error) {
	if !s.Disponible() {
		return nil, ErrUnavailable
	}
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, ErrUnauthorized
	}
	if strings.TrimSpace(requestID) == "" {
		generado, err := s.newID()
		if err != nil || strings.TrimSpace(generado) == "" {
			return nil, ErrUnavailable
		}
		requestID = strings.TrimSpace(generado)
	}
	estado, err := s.store.BeginReadAll(ctx, userID, requestID, s.now().UTC())
	if err != nil {
		return nil, normalizeError(err)
	}
	if estado == nil {
		return nil, ErrUnavailable
	}
	if estado.Status == StatusCompleted {
		return &Accepted{Status: "accepted", RequestID: requestID}, nil
	}
	if err := s.runReadAll(ctx, userID, requestID); err != nil {
		return nil, err
	}
	return &Accepted{Status: "accepted", RequestID: requestID}, nil
}

func (s *Service) runReadAll(ctx context.Context, userID, requestID string) error {
	paginas := 0
	for {
		paginas++
		if paginas > s.maxPages {
			return ErrConflict
		}
		estado, err := s.store.LoadReadAll(ctx, userID, requestID)
		if err != nil {
			return normalizeError(err)
		}
		if estado == nil {
			return ErrUnavailable
		}
		if estado.Status == StatusCompleted {
			return nil
		}

		dueño, err := s.newID()
		if err != nil || strings.TrimSpace(dueño) == "" {
			return ErrUnavailable
		}
		actual := s.now().UTC()
		pagina, err := s.store.List(ctx, ListQuery{
			UserID: userID,
			Limit:  ReadAllBatchSize,
			IsRead: boolPtr(false),
			After:  estado.Cursor,
		})
		if err != nil {
			return normalizeError(err)
		}
		if pagina == nil {
			return ErrUnavailable
		}
		for _, n := range pagina.Items {
			if _, err := s.store.MarkRead(ctx, userID, n.ID, actual); err != nil {
				return normalizeError(err)
			}
		}

		// El total procesado se deriva del diferencial de no leídas: si un
		// lote anterior falló después de marcar algunos items, ésos ya no
		// aparecen en el próximo Query y no deben contarse dos veces ni
		// perderse del informe del registro READALL.
		restantes, errConteo := s.store.CountUnread(ctx, userID)
		if errConteo != nil {
			return normalizeError(errConteo)
		}
		procesados := estado.InitialUnread - restantes
		if procesados < 0 {
			procesados = 0
		}

		siguiente := *estado
		siguiente.ProcessedCount = procesados
		siguiente.Cursor = pagina.NextKey
		siguiente.LeaseOwner = dueño
		siguiente.LeaseUntil = actual.Add(readAllLeaseTTL)
		if err := s.store.AdvanceReadAll(ctx, userID, estado.LeaseOwner, siguiente); err != nil {
			return normalizeError(err)
		}
		if pagina.NextKey == nil {
			return s.store.CompleteReadAll(ctx, userID, requestID, dueño, procesados, s.now().UTC())
		}
	}
}

func boolPtr(v bool) *bool { return &v }

func normalizeError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, ErrInvalidRequest), errors.Is(err, ErrUnauthorized),
		errors.Is(err, ErrNotFound), errors.Is(err, ErrConflict), errors.Is(err, ErrUnavailable):
		return err
	default:
		return ErrUnavailable
	}
}
