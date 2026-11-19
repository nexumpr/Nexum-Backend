package recurring

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	domainIdentity "github.com/nexumpr/Nexum-Backend/internal/domain/identity"
	domainRecurring "github.com/nexumpr/Nexum-Backend/internal/domain/recurring"
	"github.com/nexumpr/Nexum-Backend/pkg/httputil"
)

// Casos de uso de pagos recurrentes: E032 (`GET /recurring-payments`), E033
// (`POST /recurring-payments`), E034 (`GET /recurring-payments/{id}`), E035
// (`PATCH /recurring-payments/{id}`) y E036 (`DELETE /recurring-payments/{id}`).
//
// Invariantes de esta capa:
//
//   - La identidad llega exclusivamente de `sub` del JWT verificado. El cliente no elige el
//     usuario y una clave ajena nunca se consulta: la partición es `USER#<sub>`, de modo que
//     un identificador de otra cuenta no existe para el llamador (404, sin enumeración).
//   - Los importes viajan como céntimos `int64`; no se convierte a coma flotante en ningún
//     punto (R-030, GOV-08).
//   - La frecuencia se persiste y se emite normalizada a los siete valores canónicos
//     (RFC-021). Un ítem histórico con alias se normaliza al leer, sin reescribirlo.
//   - E033 NO genera ningún pago ni movimiento: sólo registra la regla (E033 §1).
var (
	// ErrInvalidRequest indica una solicitud mal formada en la capa de aplicación (por
	// ejemplo un límite fuera de rango, un cursor manipulado o un `type` desconocido).
	ErrInvalidRequest = errors.New("solicitud de pago recurrente invalida")

	// ErrUnauthorized indica ausencia de identidad verificada. El middleware ya la exige;
	// esta guarda es defensa en profundidad.
	ErrUnauthorized = errors.New("pago recurrente no autorizado")

	// ErrNotFound indica que el recurso no existe en la partición del titular: bien porque
	// nunca existió, bien porque pertenece a otra cuenta.
	ErrNotFound = errors.New("pago recurrente no encontrado")

	// ErrConflict indica una escritura concurrente que agotó los reintentos, o la
	// reutilización de una Idempotency-Key con otra carga útil (HTTP 409).
	ErrConflict = errors.New("conflicto de escritura de pago recurrente")

	// ErrVersionConflict indica que la versión de `If-Match` no es la vigente (HTTP 412).
	ErrVersionConflict = errors.New("version del pago recurrente desactualizada")

	// ErrUnavailable indica falta de persistencia, de clave de firma de cursor o un fallo
	// del servicio. Nunca se sustituye por datos en memoria (HTTP 503).
	ErrUnavailable = errors.New("servicio de pagos recurrentes no disponible")
)

// Constantes del contrato de E032/E033.
const (
	// OperationCreate identifica la operación en el registro de idempotencia por usuario.
	OperationCreate = "E033_CREATE_RECURRING"

	// IdempotencyTTL es la ventana de replay de la Idempotency-Key (Gate Oleada 2 §5.2).
	IdempotencyTTL = 24 * time.Hour

	// DefaultLimit es el tamaño de página por defecto de `limit` (25).
	DefaultLimit = httputil.DefaultPageLimit

	// MaxLimit es el tamaño de página máximo admitido (100).
	MaxLimit = httputil.MaxPageLimit

	// CursorMinLength y CursorMaxLength replican `minLength: 8` y `maxLength: 2048` del
	// parámetro `cursor`.
	CursorMinLength = 8
	CursorMaxLength = 2048

	// idempotencyMinBytes y idempotencyMaxBytes acotan la Idempotency-Key aceptada.
	idempotencyMinBytes = 16
	idempotencyMaxBytes = 128
)

// Violation describe un incumplimiento de REGLA DE NEGOCIO atribuible a un campo concreto.
// El transporte lo responde como 422 BUSINESS_RULE_VIOLATION con `details`.
type Violation struct {
	Field  string
	Reason string
}

// Error implementa la interfaz de error.
func (v *Violation) Error() string {
	return fmt.Sprintf("campo %q invalido: %s", v.Field, v.Reason)
}

// newViolation construye una violación de negocio.
func newViolation(field, reason string) *Violation {
	return &Violation{Field: field, Reason: reason}
}

// Payment es una regla recurrente tal como la exponen E032, E033, E034 y E035.
//
// `Frequency` ya está normalizada al valor canónico: esta capa es la única que decide qué
// frecuencia se emite. `DueDay` es opcional (`nil` = sin día fijo).
type Payment struct {
	ID          string
	Name        string
	AmountCents int64
	Currency    string
	Frequency   domainIdentity.Frequency
	NextDueDate time.Time
	Active      bool
	Version     int64
	Type        string
	StartDate   time.Time
	DueDay      *int
}

// WirePayment es el objeto `RecurringPayment` del contrato OpenAPI: exactamente once
// campos, sin extras y sin `omitempty`. El item de DynamoDB nunca llega al cliente, de modo
// que `entityType`, `userId`, `createdAt` o cualquier atributo interno no pueden filtrarse.
//
// El orden de los campos es fijo: la serialización es determinista y por eso el mismo
// recurso produce siempre los mismos bytes (requisito del replay idempotente).
type WirePayment struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Amount      string `json:"amount"`
	Currency    string `json:"currency"`
	Frequency   string `json:"frequency"`
	NextDueDate string `json:"nextDueDate"`
	Active      bool   `json:"active"`
	Version     int64  `json:"version"`
	Type        string `json:"type"`
	StartDate   string `json:"startDate"`
	DueDay      *int   `json:"dueDay"`
}

// Wire proyecta el recurso al DTO exacto del contrato.
func (p Payment) Wire() WirePayment {
	return WirePayment{
		ID:          p.ID,
		Name:        p.Name,
		Amount:      httputil.FormatCentsToDecimal(p.AmountCents),
		Currency:    p.Currency,
		Frequency:   p.Frequency.String(),
		NextDueDate: domainRecurring.FormatDate(p.NextDueDate),
		Active:      p.Active,
		Version:     p.Version,
		Type:        p.Type,
		StartDate:   domainRecurring.FormatDate(p.StartDate),
		DueDay:      p.DueDay,
	}
}

// Body serializa el recurso al JSON del contrato. Es la representación que se persiste con
// la Idempotency-Key y la que se devuelve en el replay, de modo que ambas son idénticas
// byte a byte.
func (p Payment) Body() ([]byte, error) {
	return json.Marshal(p.Wire())
}

// PageBody serializa `PageOfRecurringPayment`: `items` siempre presente y `nextCursor`
// sólo cuando la persistencia confirmó que hay más filas.
func PageBody(items []Payment, nextCursor string) ([]byte, error) {
	wire := make([]WirePayment, 0, len(items))
	for _, item := range items {
		wire = append(wire, item.Wire())
	}
	pagina := struct {
		Items      []WirePayment `json:"items"`
		NextCursor string        `json:"nextCursor,omitempty"`
	}{Items: wire, NextCursor: nextCursor}
	return json.Marshal(pagina)
}

// payloadHash es el SHA-256 de la carga útil YA validada y normalizada (Gate Oleada 2
// §5.2). Se construye con una cadena canónica de longitud prefijada, de modo que dos
// cargas distintas no puedan producir la misma concatenación por reparto ambiguo de
// separadores.
func payloadHash(p Payment) string {
	dueDay := "null"
	if p.DueDay != nil {
		dueDay = strconv.Itoa(*p.DueDay)
	}
	partes := []string{
		p.Name,
		strconv.FormatInt(p.AmountCents, 10),
		p.Currency,
		p.Frequency.String(),
		domainRecurring.FormatDate(p.NextDueDate),
		p.Type,
		domainRecurring.FormatDate(p.StartDate),
		dueDay,
	}
	var canonica []byte
	for _, parte := range partes {
		canonica = append(canonica, fmt.Sprintf("%d:%s;", len(parte), parte)...)
	}
	suma := sha256.Sum256(canonica)
	return hex.EncodeToString(suma[:])
}

// CreateCommand es la entrada de E033 ya convertida a céntimos por el transporte (el
// formato decimal publicado lo valida el DTO). El resto de reglas se aplican aquí.
type CreateCommand struct {
	UserID         string
	IdempotencyKey string
	Name           string
	AmountCents    int64
	Currency       string
	Frequency      string
	NextDueDate    string
	Type           string
	StartDate      string
	DueDay         *int
}

// CreateResult es el resultado de E033. `Body` es la respuesta autoritativa (la misma en
// creación y en replay) y `Replayed` indica que no se creó nada nuevo.
type CreateResult struct {
	ID       string
	Body     []byte
	Replayed bool
}

// Filter son los filtros de servidor admitidos por E032. `Active` nulo significa «sin
// filtrar»; `Type` vacío, ídem.
type Filter struct {
	Active *bool
	Type   string
}

// ListQuery es la entrada de aplicación de E032.
type ListQuery struct {
	UserID string
	Limit  int
	Cursor string
	Active *bool
	Type   string
}

// ListPage es el resultado de una página de E032.
type ListPage struct {
	Items      []Payment
	NextCursor string
}

// DueDayPatch distingue las tres situaciones que `RecurringPatch.dueDay` separa: campo
// ausente (no se toca), `null` explícito (se elimina el día fijo) y entero (se fija).
type DueDayPatch struct {
	Present bool
	Value   *int
}

// PatchCommand es la entrada de E035. Todos los campos son opcionales: un campo nulo no se
// modifica. `Frequency` no existe en el comando porque `RecurringPatch` no lo admite
// (RFC-021 §2) y el DTO lo rechaza como campo desconocido.
type PatchCommand struct {
	UserID          string
	PaymentID       string
	ExpectedVersion int64
	Name            *string
	AmountCents     *int64
	Active          *bool
	NextDueDate     *string
	Type            *string
	StartDate       *string
	DueDay          DueDayPatch
}

// CreateWrite es la escritura atómica de E033: el recurso y el registro de idempotencia en
// la MISMA transacción.
type CreateWrite struct {
	UserID         string
	IdempotencyKey string
	Operation      string
	PayloadHash    string
	Body           []byte
	TTL            time.Duration
	Payment        Payment
	Now            time.Time
}

// UpdateWrite es la escritura compare-and-swap de E035: el adaptador sólo aplica el cambio
// si la versión persistida coincide con `ExpectedVersion`.
type UpdateWrite struct {
	UserID          string
	PaymentID       string
	ExpectedVersion int64
	Payment         Payment
	Now             time.Time
}

// Store es el puerto de persistencia de pagos recurrentes.
//
// Contrato del adaptador:
//   - `Create` escribe el recurso y el registro de idempotencia en una sola transacción. Si
//     la clave ya existe con el mismo `PayloadHash`, devuelve el cuerpo almacenado y
//     `replayed=true`; si existe con otro, devuelve ErrConflict.
//   - `List` consulta por prefijo de partición (nunca `Scan`) y devuelve la `SK` desde la
//     que continuar, vacía cuando no hay más.
//   - `Get` devuelve (nil, nil) cuando el recurso no existe o no es representable en el
//     contrato vigente.
//   - `Update` aplica el CAS por versión y devuelve ErrVersionConflict si no coincide.
//   - `Deactivate` es idempotente y devuelve si el recurso existía.
type Store interface {
	Create(ctx context.Context, write CreateWrite) (id string, body []byte, replayed bool, err error)
	List(ctx context.Context, userID, startSK string, limit int, filter Filter) ([]Payment, string, error)
	Get(ctx context.Context, userID, paymentID string) (*Payment, error)
	Update(ctx context.Context, write UpdateWrite) (*Payment, error)
	Deactivate(ctx context.Context, userID, paymentID string, now time.Time) (bool, error)
}
