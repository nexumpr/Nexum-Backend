// Package movements implementa los casos de uso de movimientos financieros de la API Nexum
// (Oleada 2: E027, E028; y las piezas compartidas que E029/E030/E031 reutilizan).
//
// Reglas transversales que este paquete hace cumplir:
//
//   - La identidad del titular NUNCA proviene del cuerpo ni de la ruta: el llamador pasa el
//     `sub` ya verificado por el middleware de sesión.
//   - Los importes se operan como `int64` en centavos (A-060, R-030). Nunca `float64`: la
//     cadena decimal del contrato se convierte con `httputil.ParseDecimalToCents` y se vuelve a
//     serializar con `httputil.FormatCentsToDecimal`.
//   - La forma canónica del cuerpo de respuesta vive aquí (`MovementResponse`), de modo que el
//     cuerpo guardado por la idempotencia de E028 y el de un 200 posterior son byte a byte
//     idénticos: el replay no puede divergir del original.
package movements

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/nexumpr/Nexum-Backend/pkg/httputil"
)

// Tipos de movimiento publicados por el contrato (`Movement.type`).
const (
	TypeIncome  = "income"
	TypeExpense = "expense"
	TypeSaving  = "saving"
)

// CurrencyMXN es la única divisa admitida (`Movement.currency` enum de un valor).
const CurrencyMXN = "MXN"

// OperationCreateMovement es el nombre de operación que se persiste en el registro de
// idempotencia de E028. Forma parte del SK (`IDEM#<operación>#<hash>`): cambiarlo invalidaría
// los registros ya escritos por este endpoint.
const OperationCreateMovement = "E028_CREATE_MOVEMENT"

const (
	// IdempotencyTTL es el vencimiento lógico del registro de idempotencia (Gate Oleada 2 §5.2).
	IdempotencyTTL = 24 * time.Hour

	// MaxDescriptionLength es el `maxLength` publicado de `description` (code points).
	MaxDescriptionLength = 120
	// MaxCategoryIDLength acota `categoryId`. El contrato no declara enum ni longitud; se
	// valida la forma (no vacío, sin controles, sin espacios en los extremos) y se acota para
	// que el ítem de DynamoDB no crezca sin control. NO se contrasta contra el catálogo de
	// E022: el OpenAPI publica ejemplos (`food`, `transport`) que no son IDs de ese catálogo.
	MaxCategoryIDLength = 64

	// DefaultLimit y MaxLimit son los del parámetro `limit` de E027.
	DefaultLimit = 25
	MaxLimit     = 100

	// CursorMinLength y CursorMaxLength son los del parámetro `cursor` de E027.
	CursorMinLength = 8
	CursorMaxLength = 2048
)

// OccurredAtLayout es la representación canónica de `occurredAt`: UTC con exactamente tres
// decimales, que es un `date-time` válido y además ordena alfabéticamente igual que en el
// tiempo. Coincide con el formato que usa `domainDynamo.MovementKeys` para construir la SK;
// si ambos divergieran, la fecha leída dejaría de corresponder a la posición del ítem.
const OccurredAtLayout = "2006-01-02T15:04:05.000Z"

// Errores de la capa de aplicación. El transporte los traduce a códigos del contrato.
var (
	// ErrInvalidRequest es una forma que el contrato no admite (HTTP 400).
	ErrInvalidRequest = errors.New("movements: solicitud invalida")
	// ErrUnauthorized indica identidad ausente o no utilizable (HTTP 401).
	ErrUnauthorized = errors.New("movements: identidad no autorizada")
	// ErrNotFound cubre inexistente y ajeno: nunca se revela la existencia de un recurso de
	// otro titular (HTTP 404).
	ErrNotFound = errors.New("movements: recurso no encontrado")
	// ErrConflict es un conflicto de idempotencia o de estado (HTTP 409).
	ErrConflict = errors.New("movements: conflicto")
	// ErrVersionConflict indica `If-Match` desactualizado (HTTP 412).
	ErrVersionConflict = errors.New("movements: version desactualizada")
	// ErrContributionLinked indica que el movimiento está ligado a una aportación confirmada y
	// no puede borrarse sin una reversa explícita (HTTP 409).
	ErrContributionLinked = errors.New("movements: movimiento ligado a una aportacion confirmada")
	// ErrUnavailable es una dependencia no disponible (HTTP 503).
	ErrUnavailable = errors.New("movements: dependencia no disponible")
	// ErrInternal es un fallo interno no clasificable (HTTP 500).
	ErrInternal = errors.New("movements: fallo interno")
	// ErrMovementIDCollision indica que la clave del movimiento recién generado ya existe. No
	// es un conflicto de idempotencia: el servicio reintenta con otro identificador.
	ErrMovementIDCollision = errors.New("movements: colision de identificador")
)

// FieldViolation es una violación de regla de negocio sobre un campo concreto (HTTP 422).
type FieldViolation struct {
	Field  string
	Reason string
}

// Error implementa error.
func (v *FieldViolation) Error() string {
	if v == nil {
		return "movements: violacion de regla de negocio"
	}
	return fmt.Sprintf("movements: campo %q invalido (%s)", v.Field, v.Reason)
}

func violacion(campo, motivo string) error { return &FieldViolation{Field: campo, Reason: motivo} }

// Movement es la entidad de aplicación de un movimiento financiero. `AmountCents` es el importe
// exacto en centavos y `OccurredAt` está siempre en UTC truncado a milisegundos (la precisión
// de la SK), de modo que el valor leído coincide exactamente con el escrito.
type Movement struct {
	ID                 string
	Type               string
	AmountCents        int64
	Currency           string
	CategoryID         string
	Description        string
	OccurredAt         time.Time
	Version            int64
	ContributionID     string
	RecurringPaymentID string
	// CreatedAt es la marca de alta persistida. No forma parte del contrato de respuesta, pero
	// E030 la conserva al mover un ítem cuya SK cambia: sin ella, una modificación reescribiría
	// el ítem con la hora actual y perdería la fecha original.
	CreatedAt time.Time
}

// Patch es el cuerpo de E030 (`MovementPatch`). Los tres campos son opcionales: un puntero nulo
// significa «no enviado». `type`, `amount` y `currency` no existen aquí porque el contrato no los
// declara editables.
type Patch struct {
	CategoryID  *string
	Description *string
	OccurredAt  *string
}

// Empty indica si el parche no contiene ningún campo aplicable.
func (p Patch) Empty() bool {
	return p.CategoryID == nil && p.Description == nil && p.OccurredAt == nil
}

// CreateCommand es el DTO ya deserializado del alta (E028). `Amount` llega como cadena decimal
// del contrato y se convierte a centavos en `NormalizeCreate`; nunca se acepta un número JSON.
type CreateCommand struct {
	Type           string
	Amount         string
	Currency       string
	CategoryID     string
	Description    string
	OccurredAt     string
	IdempotencyKey string
}

// ListQuery es la consulta de E027 ya deserializada de la query string.
type ListQuery struct {
	UserID  string
	Limit   int
	Cursor  string
	Type    string
	From    time.Time
	HasFrom bool
	To      time.Time
	HasTo   bool
}

// Page es una página de movimientos con su cursor de continuación.
type Page struct {
	Items      []Movement
	NextCursor string
}

// NormalizeCreate valida y normaliza el comando de alta.
//
// Separación de errores, alineada con el contrato: una forma inválida (importe que no es una
// cadena decimal de dos posiciones, fecha que no es `date-time`) devuelve ErrInvalidRequest
// (400); un valor con forma válida que incumple una regla financiera (importe no positivo,
// tipo fuera del enum, divisa distinta de MXN) devuelve *FieldViolation (422).
func NormalizeCreate(cmd CreateCommand, now time.Time) (CreateCommand, error) {
	if strings.TrimSpace(cmd.IdempotencyKey) == "" {
		return CreateCommand{}, fmt.Errorf("%w: Idempotency-Key requerida", ErrInvalidRequest)
	}
	cmd.IdempotencyKey = strings.TrimSpace(cmd.IdempotencyKey)
	if len(cmd.IdempotencyKey) < 16 || len(cmd.IdempotencyKey) > 128 {
		return CreateCommand{}, fmt.Errorf("%w: Idempotency-Key fuera de rango", ErrInvalidRequest)
	}

	tipo, err := NormalizeType(cmd.Type)
	if err != nil {
		return CreateCommand{}, err
	}
	cmd.Type = tipo

	centavos, err := ParseAmount(cmd.Amount)
	if err != nil {
		return CreateCommand{}, err
	}
	if centavos <= 0 {
		return CreateCommand{}, violacion("amount", "out_of_range")
	}
	cmd.Amount = httputil.FormatCentsToDecimal(centavos)

	divisa := strings.ToUpper(strings.TrimSpace(cmd.Currency))
	if divisa != CurrencyMXN {
		return CreateCommand{}, violacion("currency", "unsupported")
	}
	cmd.Currency = divisa

	categoria, err := NormalizeCategoryID(cmd.CategoryID)
	if err != nil {
		return CreateCommand{}, err
	}
	cmd.CategoryID = categoria

	descripcion, err := NormalizeDescription(cmd.Description)
	if err != nil {
		return CreateCommand{}, err
	}
	cmd.Description = descripcion

	ocurrido, err := NormalizeOccurredAt(cmd.OccurredAt, now)
	if err != nil {
		return CreateCommand{}, err
	}
	cmd.OccurredAt = FormatOccurredAt(ocurrido)

	return cmd, nil
}

// NormalizeDescription valida y normaliza `description`: sin espacios en los extremos, no vacía y
// como máximo `MaxDescriptionLength` code points (el `maxLength` del contrato).
func NormalizeDescription(valor string) (string, error) {
	limpio := strings.TrimSpace(valor)
	if limpio == "" {
		return "", violacion("description", "required")
	}
	if utf8.RuneCountInString(limpio) > MaxDescriptionLength {
		return "", violacion("description", "too_long")
	}
	return limpio, nil
}

// NormalizeOccurredAt valida una marca `occurredAt` (RFC 3339) y aplica las cotas de cordura: un
// año absurdo rompería el orden por SK y un movimiento muy futuro no es un dato financiero real.
func NormalizeOccurredAt(valor string, now time.Time) (time.Time, error) {
	instante, err := ParseOccurredAt(valor)
	if err != nil {
		return time.Time{}, err
	}
	if instante.Year() < 1970 || instante.Year() > 2200 {
		return time.Time{}, violacion("occurredAt", "out_of_range")
	}
	if now.IsZero() {
		now = time.Now()
	}
	if instante.After(now.UTC().Add(24 * time.Hour)) {
		return time.Time{}, violacion("occurredAt", "in_the_future")
	}
	return instante, nil
}

// ValidMovementID comprueba la forma canónica de uuid (8-4-4-4-12, hexadecimal) que el contrato
// declara para el parámetro de ruta. No impone una versión concreta: un identificador histórico o
// generado por otro emisor sigue siendo válido.
func ValidMovementID(valor string) bool {
	limpio := strings.TrimSpace(valor)
	if len(limpio) != 36 {
		return false
	}
	for indice := 0; indice < len(limpio); indice++ {
		caracter := limpio[indice]
		switch indice {
		case 8, 13, 18, 23:
			if caracter != '-' {
				return false
			}
		default:
			esHex := (caracter >= '0' && caracter <= '9') || (caracter >= 'a' && caracter <= 'f') || (caracter >= 'A' && caracter <= 'F')
			if !esHex {
				return false
			}
		}
	}
	return true
}

// NormalizeType acepta exactamente el enum publicado, sin alias ni variantes de caja.
func NormalizeType(valor string) (string, error) {
	switch strings.TrimSpace(valor) {
	case TypeIncome:
		return TypeIncome, nil
	case TypeExpense:
		return TypeExpense, nil
	case TypeSaving:
		return TypeSaving, nil
	default:
		return "", violacion("type", "unsupported")
	}
}

// ParseAmount convierte la cadena decimal del contrato a centavos.
//
// Se aplica primero el patrón publicado `^(0|[1-9][0-9]{0,11})\.[0-9]{2}$`, que prohíbe ceros a
// la izquierda, signos y más de doce dígitos enteros: sin él, `httputil.ParseDecimalToCents`
// aceptaría `0100.00`, que el contrato no admite. La conversión a centavos se delega para no
// duplicar la aritmética.
func ParseAmount(valor string) (int64, error) {
	limpio := strings.TrimSpace(valor)
	if limpio == "" {
		return 0, violacion("amount", "required")
	}
	if !patronImporte(limpio) {
		return 0, fmt.Errorf("%w: amount", ErrInvalidRequest)
	}
	centavos, err := httputil.ParseDecimalToCents(limpio)
	if err != nil {
		return 0, fmt.Errorf("%w: amount", ErrInvalidRequest)
	}
	return centavos, nil
}

// patronImporte replica el `pattern` de `MovementCreate.amount` del OpenAPI.
func patronImporte(valor string) bool {
	punto := strings.IndexByte(valor, '.')
	if punto < 0 {
		return false
	}
	entero, decimales := valor[:punto], valor[punto+1:]
	if len(decimales) != 2 || len(entero) == 0 || len(entero) > 12 {
		return false
	}
	for i := 0; i < len(decimales); i++ {
		if decimales[i] < '0' || decimales[i] > '9' {
			return false
		}
	}
	if entero == "0" {
		return true
	}
	if entero[0] < '1' || entero[0] > '9' {
		return false
	}
	for i := 1; i < len(entero); i++ {
		if entero[i] < '0' || entero[i] > '9' {
			return false
		}
	}
	return true
}

// NormalizeCategoryID valida la forma de `categoryId`. No es un enum.
func NormalizeCategoryID(valor string) (string, error) {
	if strings.TrimSpace(valor) == "" {
		return "", violacion("categoryId", "required")
	}
	if valor != strings.TrimSpace(valor) {
		return "", violacion("categoryId", "invalid_format")
	}
	if utf8.RuneCountInString(valor) > MaxCategoryIDLength {
		return "", violacion("categoryId", "too_long")
	}
	for _, r := range valor {
		if unicode.IsControl(r) {
			return "", violacion("categoryId", "invalid_format")
		}
	}
	return valor, nil
}

// ParseOccurredAt interpreta la marca de tiempo del contrato (`date-time`, RFC 3339) y la
// normaliza a UTC truncado a milisegundos. Una cadena que no es `date-time` es 400.
func ParseOccurredAt(valor string) (time.Time, error) {
	limpio := strings.TrimSpace(valor)
	if limpio == "" {
		return time.Time{}, violacion("occurredAt", "required")
	}
	instante, err := time.Parse(time.RFC3339Nano, limpio)
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: occurredAt", ErrInvalidRequest)
	}
	return instante.UTC().Truncate(time.Millisecond), nil
}

// FormatOccurredAt serializa en la forma canónica de `OccurredAtLayout`.
func FormatOccurredAt(t time.Time) string { return t.UTC().Format(OccurredAtLayout) }

// SortableTime es la forma ordenable de la marca de tiempo. Es la misma que usa
// `domainDynamo.MovementKeys` para la SK y la que se persiste en `occurredAt`.
func SortableTime(t time.Time) string { return FormatOccurredAt(t) }

// ParseStoredOccurredAt interpreta el atributo persistido `occurredAt`.
func ParseStoredOccurredAt(valor string) (time.Time, bool) {
	limpio := strings.TrimSpace(valor)
	if limpio == "" {
		return time.Time{}, false
	}
	if instante, err := time.Parse(OccurredAtLayout, limpio); err == nil {
		return instante.UTC(), true
	}
	if instante, err := time.Parse(time.RFC3339Nano, limpio); err == nil {
		return instante.UTC().Truncate(time.Millisecond), true
	}
	return time.Time{}, false
}

// PayloadHash es el SHA-256 del DTO YA VALIDADO Y NORMALIZADO (Gate Oleada 2 §5.2).
//
// No se hashea el cuerpo crudo: `{"amount":"1000.00"}` y `{"amount": "1000.00"}` son la misma
// operación, y una clave de idempotencia reutilizada con el mismo contenido debe reproducir la
// respuesta. Tampoco se incluye la propia clave: dos claves distintas con el mismo contenido son
// dos operaciones distintas y deben crear dos movimientos.
func PayloadHash(cmd CreateCommand) string {
	canonico := strings.Join([]string{
		cmd.Type,
		strconv.FormatInt(AmountCents(cmd.Amount), 10),
		cmd.Currency,
		cmd.CategoryID,
		cmd.Description,
		cmd.OccurredAt,
	}, "|")
	suma := sha256.Sum256([]byte(canonico))
	return hex.EncodeToString(suma[:])
}

// AmountCents convierte el importe canónico del comando a centavos. Un valor no convertible se
// trata como cero: sólo se invoca con un comando ya normalizado.
func AmountCents(canonico string) int64 {
	centavos, err := httputil.ParseDecimalToCents(canonico)
	if err != nil {
		return 0
	}
	return centavos
}

// MovementResponse es el esquema `Movement` del contrato (OpenAPI 3.1), con los dos campos
// nullable de la enmienda A8. Se emiten siempre, como `null` cuando no hay vínculo, porque el
// esquema los declara `["string","null"]` y no opcionales.
type MovementResponse struct {
	ID                 string  `json:"id"`
	Type               string  `json:"type"`
	Amount             string  `json:"amount"`
	Currency           string  `json:"currency"`
	CategoryID         string  `json:"categoryId"`
	Description        string  `json:"description"`
	OccurredAt         string  `json:"occurredAt"`
	Version            int64   `json:"version"`
	ContributionID     *string `json:"contributionId"`
	RecurringPaymentID *string `json:"recurringPaymentId"`
}

// NewMovementResponse proyecta la entidad al DTO del contrato.
func NewMovementResponse(m Movement) MovementResponse {
	return MovementResponse{
		ID:                 m.ID,
		Type:               m.Type,
		Amount:             httputil.FormatCentsToDecimal(m.AmountCents),
		Currency:           m.Currency,
		CategoryID:         m.CategoryID,
		Description:        m.Description,
		OccurredAt:         FormatOccurredAt(m.OccurredAt),
		Version:            m.Version,
		ContributionID:     nullable(m.ContributionID),
		RecurringPaymentID: nullable(m.RecurringPaymentID),
	}
}

// nullable convierte la cadena vacía en `null` sin inventar un puntero a "".
func nullable(valor string) *string {
	if strings.TrimSpace(valor) == "" {
		return nil
	}
	copia := valor
	return &copia
}

// MarshalMovement serializa la proyección canónica. Es la única fuente del cuerpo de un
// movimiento: la usan el 201 de E028 (que persiste el resultado para el replay), el 200 de
// E029/E030 y cada elemento de `items` de E027.
func MarshalMovement(m Movement) ([]byte, error) {
	return json.Marshal(NewMovementResponse(m))
}
