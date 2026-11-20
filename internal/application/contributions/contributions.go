// Package contributions implementa los casos de uso de aportaciones a metas de la Oleada 3:
// E043 (preview), E044 (alta), E045 (listado), E046 (detalle) y E047 (reversión).
//
// Invariantes de la capa:
//   - La identidad llega desde el `sub` verificado; el cliente nunca elige el usuario.
//   - Los importes son `int64` en centavos. No existe ningún camino con `float64` (R-030).
//   - E043 no escribe: valida y calcula el reparto exacto que E044 aplicaría.
//   - E044 escribe aportación, puntero, UN movimiento de ahorro, puntero del movimiento y las
//     metas afectadas en una sola `TransactWriteItems` condicionada.
//   - E047 revierte la aportación completa y las allocations de todas sus metas en una sola
//     transacción. No borra el movimiento histórico: el estado `reversed` de la aportación es
//     la autoridad para los agregados.
//   - La idempotencia financiera está aislada por usuario (`UserIdempotencyKeys`).
package contributions

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	appIdentity "github.com/nexumpr/Nexum-Backend/internal/application/identity"
	appMovements "github.com/nexumpr/Nexum-Backend/internal/application/movements"
	"github.com/nexumpr/Nexum-Backend/pkg/httputil"
)

const (
	// CurrencyMXN es la única divisa del contrato.
	CurrencyMXN = "MXN"

	// OperationCreate y OperationReverse identifican las operaciones de idempotencia.
	OperationCreate  = "E044_CREATE_CONTRIBUTION"
	OperationReverse = "E047_REVERSE_CONTRIBUTION"

	// IdempotencyTTL es el vencimiento lógico de 24 h del Gate de Oleada 2 §5.2.
	IdempotencyTTL = 24 * time.Hour

	// Límites de página del contrato de E045.
	DefaultLimit = httputil.DefaultPageLimit
	MaxLimit     = httputil.MaxPageLimit

	// CursorMinLength y CursorMaxLength replican el contrato.
	CursorMinLength = 8
	CursorMaxLength = 2048

	// MaxAllocations es el `maxItems: 12` de `ContributionCreate.allocations`.
	MaxAllocations = 12

	// MinReason y MaxReason son los límites de `ReverseContributionRequest.reason`.
	MinReason = 3
	MaxReason = 250

	// StatusActive y StatusReversed son el enum de `Contribution.status`.
	StatusActive   = "active"
	StatusReversed = "reversed"

	// idempotencyMinBytes y idempotencyMaxBytes acotan la Idempotency-Key.
	idempotencyMinBytes = 16
	idempotencyMaxBytes = 128

	// goalCategory es la categoría interna del movimiento de ahorro. No es una categoría del
	// catálogo público: el contrato no la declara y el movimiento queda vinculado a la
	// aportación para que los agregados no la cuenten como gasto.
	goalCategory = "goal"

	// movementDescription es la descripción estable del movimiento de ahorro. No incluye
	// nombres de metas para no filtrar datos personales en el listado de movimientos.
	movementDescription = "Aportacion a metas"
)

// Errores de aplicación. El transporte los traduce a los códigos del contrato.
var (
	ErrInvalidRequest  = errors.New("aportaciones: solicitud invalida")
	ErrUnauthorized    = errors.New("aportaciones: identidad no autorizada")
	ErrNotFound        = errors.New("aportaciones: recurso no encontrado")
	ErrGoalNotFound    = errors.New("aportaciones: meta no encontrada")
	ErrConflict        = errors.New("aportaciones: conflicto de idempotencia o de escritura")
	ErrAlreadyReversed = errors.New("aportaciones: la aportacion ya esta revertida")
	ErrGoalInactive    = errors.New("aportaciones: la meta no esta activa")
	ErrCapacityExceeded = errors.New("aportaciones: la asignacion excede la capacidad de la meta")
	ErrUnavailable     = errors.New("aportaciones: dependencia no disponible")
	ErrInternal        = errors.New("aportaciones: fallo interno")
	ErrIDCollision     = errors.New("aportaciones: colision de identificador")
)

// FieldViolation es una violación de regla de negocio atribuible a un campo (HTTP 422).
type FieldViolation struct {
	Field  string
	Reason string
}

// Error implementa error.
func (v *FieldViolation) Error() string {
	if v == nil {
		return "aportaciones: violacion de regla de negocio"
	}
	return fmt.Sprintf("aportaciones: campo %q invalido (%s)", v.Field, v.Reason)
}

func violacion(campo, motivo string) error { return &FieldViolation{Field: campo, Reason: motivo} }

// Allocation es una asignación exacta a una meta.
type Allocation struct {
	GoalID      string
	AmountCents int64
}

// Contribution es la entidad de aplicación. `Allocations` conserva el orden del request.
type Contribution struct {
	ID           string
	UserID       string
	AmountCents  int64
	Currency     string
	Allocations  []Allocation
	OccurredAt   time.Time
	Status       string
	Version      int64
	CreatedAt    time.Time
	UpdatedAt    time.Time
	ReversedAt   time.Time
	ReverseReason string
}

// GoalSnapshot es la proyección mínima de una meta necesaria para validar capacidad.
type GoalSnapshot struct {
	ID                  string
	TargetCents         int64
	SavedCents          int64
	Status              string
	Deleted             bool
	ActiveContributions int64
	Version             int64
}

// RemainingCapacityCents es la capacidad restante antes de la asignación propuesta.
func (g GoalSnapshot) RemainingCapacityCents() int64 {
	if g.TargetCents <= g.SavedCents {
		return 0
	}
	return g.TargetCents - g.SavedCents
}

// Active admite una aportación: existe, está activa y no está borrada.
func (g GoalSnapshot) Active() bool {
	return g.Status == "active" && !g.Deleted
}

// WireAllocation es el objeto `GoalAllocation` del contrato.
type WireAllocation struct {
	GoalID string `json:"goalId"`
	Amount string `json:"amount"`
}

// WireContribution es el objeto `Contribution` del contrato: exactamente siete campos.
type WireContribution struct {
	ID          string            `json:"id"`
	Amount      string            `json:"amount"`
	Currency    string            `json:"currency"`
	Allocations []WireAllocation  `json:"allocations"`
	OccurredAt  string            `json:"occurredAt"`
	Status      string            `json:"status"`
	Version     int64             `json:"version"`
}

// WirePreview es el objeto `ContributionPreview` del contrato.
type WirePreview struct {
	Amount      string           `json:"amount"`
	Currency    string           `json:"currency"`
	Allocations []WireAllocation `json:"allocations"`
	Remaining   string           `json:"remaining"`
	CanCommit   bool             `json:"canCommit"`
}

// Wire proyecta la entidad al DTO del contrato.
func (c Contribution) Wire() WireContribution {
	allocations := make([]WireAllocation, 0, len(c.Allocations))
	for _, a := range c.Allocations {
		allocations = append(allocations, WireAllocation{
			GoalID: a.GoalID,
			Amount: httputil.FormatCentsToDecimal(a.AmountCents),
		})
	}
	return WireContribution{
		ID:          c.ID,
		Amount:      httputil.FormatCentsToDecimal(c.AmountCents),
		Currency:    c.Currency,
		Allocations: allocations,
		OccurredAt:  appMovements.FormatOccurredAt(c.OccurredAt),
		Status:      c.Status,
		Version:     c.Version,
	}
}

// Body serializa la proyección canónica.
func (c Contribution) Body() ([]byte, error) { return json.Marshal(c.Wire()) }

// PreviewCommand es la entrada de E043 ya deserializada.
type PreviewCommand struct {
	UserID      string
	Amount      string
	Currency    string
	Allocations []WireAllocation
}

// CreateCommand es la entrada de E044 ya deserializada.
type CreateCommand struct {
	UserID         string
	IdempotencyKey string
	Amount         string
	Currency       string
	Allocations    []WireAllocation
	OccurredAt     string
}

// ReverseCommand es la entrada de E047.
type ReverseCommand struct {
	UserID         string
	ContributionID string
	IdempotencyKey string
	Reason         string
}

// ListQuery es la consulta de E045.
type ListQuery struct {
	UserID   string
	Limit    int
	Cursor   string
	GoalID   string
	From     time.Time
	HasFrom  bool
	To       time.Time
	HasTo    bool
	Status   string
}

// Page es una página de aportaciones.
type Page struct {
	Items      []Contribution
	NextCursor string
}

// PreviewResult es el resultado de E043.
type PreviewResult struct {
	AmountCents  int64
	Currency     string
	Allocations  []Allocation
	RemainingCents int64
	CanCommit    bool
}

// Wire proyecta el preview al contrato.
func (r PreviewResult) Wire() WirePreview {
	allocations := make([]WireAllocation, 0, len(r.Allocations))
	for _, a := range r.Allocations {
		allocations = append(allocations, WireAllocation{
			GoalID: a.GoalID,
			Amount: httputil.FormatCentsToDecimal(a.AmountCents),
		})
	}
	return WirePreview{
		Amount:      httputil.FormatCentsToDecimal(r.AmountCents),
		Currency:    r.Currency,
		Allocations: allocations,
		Remaining:   httputil.FormatCentsToDecimal(r.RemainingCents),
		CanCommit:   r.CanCommit,
	}
}

// ParseAmount valida el patrón publicado y convierte a centavos.
func ParseAmount(raw string) (int64, error) {
	limpio := strings.TrimSpace(raw)
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

// patronImporte replica `^(0|[1-9][0-9]{0,11})\.[0-9]{2}$`.
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

// NormalizeAllocations valida forma, unicidad, límite y suma. `exactTotal` exige que la suma
// sea exactamente `total` (E044); E043 admite suma menor y devuelve el remanente.
func NormalizeAllocations(raw []WireAllocation, total int64, exactTotal bool) ([]Allocation, int64, error) {
	if len(raw) == 0 {
		return nil, 0, violacion("allocations", "required")
	}
	if len(raw) > MaxAllocations {
		return nil, 0, violacion("allocations", "too_many")
	}
	vistos := make(map[string]struct{}, len(raw))
	allocations := make([]Allocation, 0, len(raw))
	var suma int64
	for _, item := range raw {
		goalID := strings.TrimSpace(item.GoalID)
		if !appIdentity.IsUUID(goalID) {
			return nil, 0, violacion("allocations.goalId", "invalid_format")
		}
		if _, existe := vistos[goalID]; existe {
			return nil, 0, violacion("allocations.goalId", "duplicate")
		}
		vistos[goalID] = struct{}{}
		centavos, err := ParseAmount(item.Amount)
		if err != nil {
			return nil, 0, err
		}
		if centavos <= 0 {
			return nil, 0, violacion("allocations.amount", "out_of_range")
		}
		if suma > total-centavos {
			return nil, 0, violacion("allocations", "sum_exceeds_amount")
		}
		suma += centavos
		allocations = append(allocations, Allocation{GoalID: goalID, AmountCents: centavos})
	}
	if exactTotal && suma != total {
		return nil, 0, violacion("allocations", "sum_mismatch")
	}
	return allocations, suma, nil
}

// NormalizeCurrency valida el enum cerrado.
func NormalizeCurrency(raw string) (string, error) {
	divisa := strings.ToUpper(strings.TrimSpace(raw))
	if divisa != CurrencyMXN {
		return "", violacion("currency", "unsupported")
	}
	return divisa, nil
}

// NormalizeOccurredAt reutiliza la semántica de movimientos y traduce los errores.
func NormalizeOccurredAt(raw string, now time.Time) (time.Time, error) {
	instante, err := appMovements.NormalizeOccurredAt(raw, now)
	if err != nil {
		var fv *appMovements.FieldViolation
		if errors.As(err, &fv) {
			return time.Time{}, violacion("occurredAt", fv.Reason)
		}
		return time.Time{}, fmt.Errorf("%w: occurredAt", ErrInvalidRequest)
	}
	return instante, nil
}

// NormalizeReason recorta y valida el motivo de reversión.
func NormalizeReason(raw string) (string, error) {
	limpio := strings.TrimSpace(raw)
	if utf8.RuneCountInString(limpio) < MinReason {
		return "", violacion("reason", "too_short")
	}
	if utf8.RuneCountInString(limpio) > MaxReason {
		return "", violacion("reason", "too_long")
	}
	for _, r := range limpio {
		if unicode.IsControl(r) {
			return "", violacion("reason", "invalid_format")
		}
	}
	return limpio, nil
}

// ValidContributionID comprueba la forma uuid.
func ValidContributionID(valor string) bool { return appIdentity.IsUUID(strings.TrimSpace(valor)) }

// NormalizeListQuery aplica los límites y el enum de E045.
func NormalizeListQuery(q ListQuery) (ListQuery, error) {
	if q.Limit == 0 {
		q.Limit = DefaultLimit
	}
	if q.Limit < 1 || q.Limit > MaxLimit {
		return ListQuery{}, fmt.Errorf("%w: limit", ErrInvalidRequest)
	}
	q.Cursor = strings.TrimSpace(q.Cursor)
	if q.Cursor != "" && (len(q.Cursor) < CursorMinLength || len(q.Cursor) > CursorMaxLength) {
		return ListQuery{}, fmt.Errorf("%w: cursor", ErrInvalidRequest)
	}
	q.GoalID = strings.TrimSpace(q.GoalID)
	if q.GoalID != "" && !appIdentity.IsUUID(q.GoalID) {
		return ListQuery{}, fmt.Errorf("%w: goalId", ErrInvalidRequest)
	}
	if q.HasFrom {
		q.From = q.From.UTC().Truncate(time.Millisecond)
	}
	if q.HasTo {
		q.To = q.To.UTC().Truncate(time.Millisecond)
	}
	if q.HasFrom && q.HasTo && q.From.After(q.To) {
		return ListQuery{}, fmt.Errorf("%w: from", ErrInvalidRequest)
	}
	q.Status = strings.TrimSpace(q.Status)
	if q.Status != "" && q.Status != StatusActive && q.Status != StatusReversed {
		return ListQuery{}, fmt.Errorf("%w: status", ErrInvalidRequest)
	}
	return q, nil
}

// FilterSignature describe el conjunto exacto que pagina un cursor.
func FilterSignature(q ListQuery) string {
	desde, hasta := "", ""
	if q.HasFrom {
		desde = appMovements.SortableTime(q.From)
	}
	if q.HasTo {
		hasta = appMovements.SortableTime(q.To)
	}
	return fmt.Sprintf("goal=%s;from=%s;to=%s;status=%s", q.GoalID, desde, hasta, q.Status)
}

// SortableOccurredAt es la forma que se usa en la SK.
func SortableOccurredAt(t time.Time) string { return appMovements.SortableTime(t) }

// PayloadHash calcula el SHA-256 de la operación normalizada. El orden de las allocations no
// forma parte del significado: se ordenan por `goalId` antes de hashear.
func PayloadHash(amountCents int64, currency string, allocations []Allocation, occurredAt string) string {
	ordenadas := append([]Allocation(nil), allocations...)
	sort.Slice(ordenadas, func(i, j int) bool { return ordenadas[i].GoalID < ordenadas[j].GoalID })
	var b strings.Builder
	fmt.Fprintf(&b, "schema=1|amount=%d|currency=%s|occurredAt=%s", amountCents, currency, occurredAt)
	for _, a := range ordenadas {
		fmt.Fprintf(&b, "|%s=%d", a.GoalID, a.AmountCents)
	}
	suma := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(suma[:])
}

// ReversePayloadHash hashea el motivo normalizado y el identificador.
func ReversePayloadHash(contributionID, reason string) string {
	suma := sha256.Sum256([]byte("schema=1|id=" + strings.TrimSpace(contributionID) + "|reason=" + strings.TrimSpace(reason)))
	return hex.EncodeToString(suma[:])
}
