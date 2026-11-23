// Package reminders implementa los casos de uso de E064–E067 sobre la tabla
// única de DynamoDB. Invariantes:
//
//   - El titular llega siempre del claim `sub` verificado por el middleware.
//   - El cursor es opaco, HMAC-SHA256 y está ligado a identidad, límite, filtro
//     y última clave; un cursor manipulado o de otro usuario se rechaza.
//   - E065 es idempotente por (usuario, Idempotency-Key) con hash de carga:
//     la misma clave con distinta carga es 409, nunca una creación duplicada.
//   - E066 exige If-Match y aplica CAS sobre `version`; `frequency:null` cancela
//     la recurrencia y su ausencia la preserva.
//   - E067 es borrado lógico idempotente: cancela disparos futuros marcando el
//     registro como inactivo, sin borrar metas ni pagos de origen.
//
// Este paquete no programa envíos: la decisión de activar el scheduler durable
// y su integración con la bandeja/FCM de Oleada 4 requiere Gate puntual.
package reminders

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	_ "time/tzdata"

	"github.com/nexumpr/Nexum-Backend/internal/domain/identity"
)

const (
	DefaultLimit            = 25
	MaxLimit                = 100
	MinCursorLength         = 8
	MaxCursorLength         = 2048
	MinTitleRunes           = 1
	MaxTitleRunes           = 120
	MinTimezoneLength       = 3
	MaxTimezoneLength       = 64
	MinIdempotencyKeyLength = 16
	MaxIdempotencyKeyLength = 128

	cursorDomainMarker = "nexum:e064:cursor:v1:"
	idemDomainMarker   = "nexum:e065:idem:v1:"
)

// Errores de aplicación. El transporte los traduce a códigos del contrato sin
// revelar identificadores ajenos, claves ni cargas.
var (
	ErrInvalidRequest       = errors.New("solicitud de recordatorios inválida")
	ErrUnauthorized         = errors.New("recordatorios no autorizados")
	ErrNotFound             = errors.New("recordatorio no encontrado")
	ErrConflict             = errors.New("conflicto al procesar recordatorios")
	ErrVersionConflict      = errors.New("versión de recordatorio desactualizada")
	ErrPreconditionRequired = errors.New("If-Match requerido")
	ErrBusinessRule         = errors.New("regla de negocio de recordatorios incumplida")
	ErrUnavailable          = errors.New("servicio de recordatorios no disponible")
)

// Frequency es el discriminador cerrado del contrato.
type Frequency string

const (
	FrequencyDaily   Frequency = "daily"
	FrequencyWeekly  Frequency = "weekly"
	FrequencyMonthly Frequency = "monthly"
)

// ParseFrequency valida una cadencia sin depender del transporte.
func ParseFrequency(raw string) (Frequency, bool) {
	value := Frequency(strings.TrimSpace(raw))
	switch value {
	case FrequencyDaily, FrequencyWeekly, FrequencyMonthly:
		return value, true
	default:
		return "", false
	}
}

// Reminder es la entidad de dominio; sólo se materializa desde persistencia.
type Reminder struct {
	ID        string
	Title     string
	DueAt     time.Time
	Timezone  string
	Active    bool
	Version   int64
	Frequency *Frequency
	Deleted   bool
}

// WireReminder es la proyección estricta del esquema `Reminder`.
type WireReminder struct {
	ID        string  `json:"id"`
	Title     string  `json:"title"`
	DueAt     string  `json:"dueAt"`
	Timezone  string  `json:"timezone"`
	Active    bool    `json:"active"`
	Version   int64   `json:"version"`
	Frequency *string `json:"frequency,omitempty"`
}

// Wire convierte la entidad al contrato de transporte.
func (r Reminder) Wire() WireReminder {
	var frecuencia *string
	if r.Frequency != nil {
		value := string(*r.Frequency)
		frecuencia = &value
	}
	return WireReminder{
		ID:        r.ID,
		Title:     r.Title,
		DueAt:     r.DueAt.UTC().Truncate(time.Second).Format(time.RFC3339),
		Timezone:  r.Timezone,
		Active:    r.Active,
		Version:   r.Version,
		Frequency: frecuencia,
	}
}

// Page es el resultado de E064.
type Page struct {
	Items      []Reminder
	NextCursor string
}

// WirePage es la proyección estricta de `PageOfReminder`.
type WirePage struct {
	Items      []WireReminder `json:"items"`
	NextCursor string         `json:"nextCursor,omitempty"`
}

// Wire convierte la página al contrato de transporte.
func (p Page) Wire() WirePage {
	items := make([]WireReminder, 0, len(p.Items))
	for _, r := range p.Items {
		items = append(items, r.Wire())
	}
	return WirePage{Items: items, NextCursor: p.NextCursor}
}

// Key identifica un item dentro de la partición del titular.
type Key struct {
	PK string
	SK string
}

// ListQuery es la consulta pedida al adaptador.
type ListQuery struct {
	UserID string
	Limit  int
	Active *bool
	After  *Key
}

// ListResult contiene la página y la última clave entregada.
type ListResult struct {
	Items   []Reminder
	NextKey *Key
}

// IdempotencyRecord es el registro satélite de E065.
type IdempotencyRecord struct {
	ReminderID  string
	PayloadHash string
}

// CreateInput describe la escritura atómica de E065 (recordatorio + idempotencia).
type CreateInput struct {
	UserID      string
	ID          string
	KeyHash     string
	PayloadHash string
	Title       string
	DueAt       string
	Timezone    string
	Frequency   *Frequency
	Now         time.Time
}

// FrequencyPatch distingue «ausente» de «null» en E066.
type FrequencyPatch struct {
	Set   bool
	Value *Frequency
}

// UpdateInput describe la escritura condicional de E066.
type UpdateInput struct {
	UserID          string
	ID              string
	ExpectedVersion int64
	Title           *string
	DueAt           *string
	Active          *bool
	Frequency       FrequencyPatch
	Now             time.Time
}

// Store es el puerto de persistencia. La implementación productiva es
// *dynamo.RemindersStore; en pruebas se sustituye por un doble con estado.
// Ningún método expone Scan.
type Store interface {
	List(ctx context.Context, query ListQuery) (*ListResult, error)
	Get(ctx context.Context, userID, id string) (*Reminder, error)
	FindIdempotency(ctx context.Context, userID, keyHash string) (*IdempotencyRecord, error)
	Create(ctx context.Context, input CreateInput) error
	Update(ctx context.Context, input UpdateInput) (*Reminder, error)
	Delete(ctx context.Context, userID, id string, expectedVersion int64) error
}

// CreateCommand son los datos validados por el transporte para E065.
type CreateCommand struct {
	UserID         string
	IdempotencyKey string
	Title          string
	DueAt          string
	Timezone       string
	Frequency      *Frequency
}

// PatchCommand son los datos validados por el transporte para E066.
type PatchCommand struct {
	UserID          string
	ReminderID      string
	ExpectedVersion int64
	Title           *string
	DueAt           *string
	Active          *bool
	Frequency       FrequencyPatch
}

// cursorPayload es el contenido firmado del cursor de E064.
type cursorPayload struct {
	Version   int    `json:"v"`
	UserID    string `json:"uid"`
	Limit     int    `json:"lim"`
	ActiveSet bool   `json:"as"`
	Active    bool   `json:"a"`
	PK        string `json:"pk"`
	SK        string `json:"sk"`
}

type signedCursor struct {
	Payload   cursorPayload `json:"p"`
	Signature string        `json:"s"`
}

// Service orquesta los cuatro casos de uso. Sin almacén o sin clave HMAC
// todas las operaciones fallan cerrado.
type Service struct {
	store     Store
	cursorKey []byte
	now       func() time.Time
	newID     func() (string, error)
}

// NewService construye el servicio con la clave HMAC estable inyectada.
func NewService(store Store, cursorKey []byte) *Service {
	return &Service{
		store:     store,
		cursorKey: deriveKey(cursorKey, cursorDomainMarker),
		now:       func() time.Time { return time.Now().UTC() },
		newID:     identity.NewID,
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

// Disponible indica si hay almacén y claves de firma reales configuradas.
func (s *Service) Disponible() bool {
	return s != nil && s.store != nil && len(s.cursorKey) > 0
}

func deriveKey(base []byte, marker string) []byte {
	if len(base) == 0 {
		return nil
	}
	mac := hmac.New(sha256.New, base)
	mac.Write([]byte(marker))
	return mac.Sum(nil)
}

// List devuelve una página de recordatorios del titular.
func (s *Service) List(ctx context.Context, userID string, limit int, cursor string, active *bool) (*Page, error) {
	if !s.Disponible() {
		return nil, ErrUnavailable
	}
	titular := strings.TrimSpace(userID)
	if titular == "" {
		return nil, ErrUnauthorized
	}
	if limit == 0 {
		limit = DefaultLimit
	}
	if limit < 1 || limit > MaxLimit {
		return nil, ErrInvalidRequest
	}

	var after *Key
	if strings.TrimSpace(cursor) != "" {
		payload, err := s.verifyCursor(cursor, titular, limit, active)
		if err != nil {
			return nil, err
		}
		after = &Key{PK: payload.PK, SK: payload.SK}
	}

	resultado, err := s.store.List(ctx, ListQuery{UserID: titular, Limit: limit, Active: active, After: after})
	if err != nil {
		return nil, normalizeError(err)
	}
	if resultado == nil {
		return nil, ErrUnavailable
	}
	pagina := &Page{Items: resultado.Items}
	if pagina.Items == nil {
		pagina.Items = []Reminder{}
	}
	if resultado.NextKey != nil && resultado.NextKey.SK != "" {
		cursor, err := s.signCursor(cursorPayload{
			Version: 1, UserID: titular, Limit: limit,
			ActiveSet: active != nil, Active: active != nil && *active,
			PK: resultado.NextKey.PK, SK: resultado.NextKey.SK,
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

func (s *Service) verifyCursor(raw, userID string, limit int, active *bool) (*cursorPayload, error) {
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
	p := firmado.Payload
	if p.Version != 1 || p.UserID != userID || p.Limit != limit || p.PK == "" || p.SK == "" {
		return nil, ErrInvalidRequest
	}
	if p.ActiveSet != (active != nil) {
		return nil, ErrInvalidRequest
	}
	if p.ActiveSet && p.Active != *active {
		return nil, ErrInvalidRequest
	}
	return &p, nil
}

// Create ejecuta E065 con idempotencia real por (usuario, clave) y hash de carga.
func (s *Service) Create(ctx context.Context, cmd CreateCommand) (*Reminder, error) {
	if s == nil || s.store == nil {
		return nil, ErrUnavailable
	}
	userID := strings.TrimSpace(cmd.UserID)
	if userID == "" {
		return nil, ErrUnauthorized
	}
	key := strings.TrimSpace(cmd.IdempotencyKey)
	if len(key) < MinIdempotencyKeyLength || len(key) > MaxIdempotencyKeyLength {
		return nil, ErrInvalidRequest
	}
	titulo, dueAt, zona, frecuencia, err := validarCampos(cmd.Title, cmd.DueAt, cmd.Timezone, cmd.Frequency)
	if err != nil {
		return nil, err
	}

	keyHash := hashHex(deriveKey([]byte(key), idemDomainMarker+userID))
	payloadHash := hashHex([]byte(titulo + "\n" + dueAt + "\n" + zona + "\n" + frecuenciaString(frecuencia)))

	previo, err := s.store.FindIdempotency(ctx, userID, keyHash)
	if err != nil {
		return nil, normalizeError(err)
	}
	if previo != nil {
		if previo.PayloadHash != payloadHash {
			return nil, ErrConflict
		}
		return s.store.Get(ctx, userID, previo.ReminderID)
	}

	id, err := s.newID()
	if err != nil || strings.TrimSpace(id) == "" {
		return nil, ErrUnavailable
	}
	input := CreateInput{
		UserID: userID, ID: id, KeyHash: keyHash, PayloadHash: payloadHash,
		Title: titulo, DueAt: dueAt, Timezone: zona, Frequency: frecuencia, Now: s.now().UTC(),
	}
	if err := s.store.Create(ctx, input); err != nil {
		if errors.Is(err, ErrConflict) {
			// Carrera entre dos instancias con la misma clave: gana la escritura
			// atómica; la otra devuelve el recurso ya creado o 409 si cambió la carga.
			previo, errLeer := s.store.FindIdempotency(ctx, userID, keyHash)
			if errLeer != nil || previo == nil {
				return nil, ErrConflict
			}
			if previo.PayloadHash != payloadHash {
				return nil, ErrConflict
			}
			return s.store.Get(ctx, userID, previo.ReminderID)
		}
		return nil, normalizeError(err)
	}
	return s.store.Get(ctx, userID, id)
}

// Get expone la lectura de un recordatorio propio para pruebas y composición.
func (s *Service) Get(ctx context.Context, userID, id string) (*Reminder, error) {
	if !s.Disponible() {
		return nil, ErrUnavailable
	}
	titular := strings.TrimSpace(userID)
	identificador := strings.TrimSpace(id)
	if titular == "" {
		return nil, ErrUnauthorized
	}
	if identificador == "" {
		return nil, ErrInvalidRequest
	}
	reminder, err := s.store.Get(ctx, titular, identificador)
	if err != nil {
		return nil, normalizeError(err)
	}
	if reminder == nil || reminder.Deleted {
		return nil, ErrNotFound
	}
	return reminder, nil
}

// Update ejecuta E066 con CAS sobre `version`.
func (s *Service) Update(ctx context.Context, cmd PatchCommand) (*Reminder, error) {
	if !s.Disponible() {
		return nil, ErrUnavailable
	}
	titular := strings.TrimSpace(cmd.UserID)
	identificador := strings.TrimSpace(cmd.ReminderID)
	if titular == "" {
		return nil, ErrUnauthorized
	}
	if identificador == "" || cmd.ExpectedVersion < 1 {
		return nil, ErrInvalidRequest
	}

	actual, err := s.store.Get(ctx, titular, identificador)
	if err != nil {
		return nil, normalizeError(err)
	}
	if actual == nil || actual.Deleted {
		return nil, ErrNotFound
	}
	if actual.Version != cmd.ExpectedVersion {
		return nil, ErrVersionConflict
	}

	var titulo *string
	if cmd.Title != nil {
		validado, err := validarTitulo(*cmd.Title)
		if err != nil {
			return nil, err
		}
		titulo = &validado
	}
	var dueAt *string
	if cmd.DueAt != nil {
		validado, err := validarFecha(*cmd.DueAt)
		if err != nil {
			return nil, err
		}
		dueAt = &validado
	}
	var frecuencia FrequencyPatch
	if cmd.Frequency.Set && cmd.Frequency.Value != nil {
		value, ok := ParseFrequency(string(*cmd.Frequency.Value))
		if !ok {
			return nil, ErrInvalidRequest
		}
		frecuencia = FrequencyPatch{Set: true, Value: &value}
	} else if cmd.Frequency.Set {
		frecuencia = FrequencyPatch{Set: true, Value: nil}
	}

	actualizado, err := s.store.Update(ctx, UpdateInput{
		UserID: titular, ID: identificador, ExpectedVersion: cmd.ExpectedVersion,
		Title: titulo, DueAt: dueAt, Active: cmd.Active, Frequency: frecuencia, Now: s.now().UTC(),
	})
	if err == nil {
		return actualizado, nil
	}
	if errors.Is(err, ErrNotFound) {
		return nil, ErrNotFound
	}
	// La condición CAS falló; se relee para distinguir 412 de 409 sin revelar estado ajeno.
	fresco, errLeer := s.store.Get(ctx, titular, identificador)
	if errLeer != nil {
		return nil, normalizeError(errLeer)
	}
	if fresco == nil || fresco.Deleted {
		return nil, ErrNotFound
	}
	if fresco.Version != cmd.ExpectedVersion {
		return nil, ErrVersionConflict
	}
	return nil, ErrConflict
}

// Delete ejecuta E067: borrado lógico idempotente sobre un recurso propio.
func (s *Service) Delete(ctx context.Context, userID, id string) error {
	if !s.Disponible() {
		return ErrUnavailable
	}
	titular := strings.TrimSpace(userID)
	identificador := strings.TrimSpace(id)
	if titular == "" {
		return ErrUnauthorized
	}
	if identificador == "" {
		return ErrInvalidRequest
	}
	actual, err := s.store.Get(ctx, titular, identificador)
	if err != nil {
		return normalizeError(err)
	}
	if actual == nil {
		return ErrNotFound
	}
	if actual.Deleted {
		return nil
	}
	if err := s.store.Delete(ctx, titular, identificador, actual.Version); err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil
		}
		if errors.Is(err, ErrConflict) {
			fresco, errLeer := s.store.Get(ctx, titular, identificador)
			if errLeer != nil {
				return normalizeError(errLeer)
			}
			if fresco == nil {
				return nil
			}
			if fresco.Deleted {
				return nil
			}
			return ErrConflict
		}
		return normalizeError(err)
	}
	return nil
}

func validarCampos(title, dueAt, timezone string, frequency *Frequency) (string, string, string, *Frequency, error) {
	titulo, err := validarTitulo(title)
	if err != nil {
		return "", "", "", nil, err
	}
	fecha, err := validarFecha(dueAt)
	if err != nil {
		return "", "", "", nil, err
	}
	zona, err := validarZona(timezone)
	if err != nil {
		return "", "", "", nil, err
	}
	var frecuencia *Frequency
	if frequency != nil {
		value, ok := ParseFrequency(string(*frequency))
		if !ok {
			return "", "", "", nil, ErrInvalidRequest
		}
		frecuencia = &value
	}
	return titulo, fecha, zona, frecuencia, nil
}

func validarTitulo(valor string) (string, error) {
	limpio := strings.TrimSpace(valor)
	runes := []rune(limpio)
	if len(runes) < MinTitleRunes || len(runes) > MaxTitleRunes {
		return "", ErrInvalidRequest
	}
	return limpio, nil
}

func validarFecha(valor string) (string, error) {
	limpio := strings.TrimSpace(valor)
	parsed, err := time.Parse(time.RFC3339, limpio)
	if err != nil {
		return "", ErrInvalidRequest
	}
	return parsed.UTC().Truncate(time.Second).Format(time.RFC3339), nil
}

func validarZona(valor string) (string, error) {
	limpio := strings.TrimSpace(valor)
	if len(limpio) < MinTimezoneLength || len(limpio) > MaxTimezoneLength ||
		limpio == "Local" || strings.ContainsAny(limpio, " \t\r\n") {
		return "", ErrBusinessRule
	}
	if _, err := time.LoadLocation(limpio); err != nil {
		return "", ErrBusinessRule
	}
	return limpio, nil
}

// FrecuenciaString devuelve el valor persistido de una cadencia. La ausencia
// se persiste como cadena vacía para que DynamoDB no tenga atributos nulos.
func FrecuenciaString(value *Frequency) string {
	if value == nil {
		return ""
	}
	return string(*value)
}

func frecuenciaString(value *Frequency) string {
	return FrecuenciaString(value)
}

func hashHex(value []byte) string {
	suma := sha256.Sum256(value)
	return fmt.Sprintf("%x", suma[:])
}

func normalizeError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, ErrInvalidRequest), errors.Is(err, ErrUnauthorized),
		errors.Is(err, ErrNotFound), errors.Is(err, ErrConflict),
		errors.Is(err, ErrVersionConflict), errors.Is(err, ErrPreconditionRequired),
		errors.Is(err, ErrBusinessRule), errors.Is(err, ErrUnavailable):
		return err
	default:
		return ErrUnavailable
	}
}
