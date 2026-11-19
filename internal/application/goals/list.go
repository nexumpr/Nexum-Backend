package goals

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	domainGoal "github.com/nexumpr/Nexum-Backend/internal/domain/goal"
	"github.com/nexumpr/Nexum-Backend/pkg/httputil"
)

// Caso de uso de E038 (`GET /api/v1/goals`).
//
// Invariantes:
//   - `items` es la página solicitada; `totals` se calcula sobre TODA la colección del titular
//     (A8 de RFC-016), no sobre la página. Por eso el agregado se pide siempre aparte y no se
//     deriva de las filas devueltas.
//   - El cursor es opaco y firmado con HMAC: liga la identidad, el tamaño de página y el filtro
//     de estado. Reutilizarlo con otros filtros, manipularlo o dejar que caduque produce 400.
//   - Nunca se usa `Scan`: la consulta es `Query` sobre `PK = USER#<id>` con
//     `begins_with(SK, "GOAL#")`.
const (
	// ListCursorMinLength y ListCursorMaxLength replican `minLength: 8` y `maxLength: 2048`
	// del parámetro `cursor`.
	ListCursorMinLength = 8
	ListCursorMaxLength = 2048

	// listSortDomain identifica el orden y el conjunto de filtros del listado.
	listSortDomain = "goals:v1"
)

// ListQuery es la entrada de aplicación de E038. `UserID` proviene de `sub`; nunca del cliente.
type ListQuery struct {
	UserID string
	Status string
	Limit  int
	Cursor string
}

// TotalsContract es `PageOfGoal.totals`: metas activas y suma exacta de su ahorro.
type TotalsContract struct {
	ActiveCount int    `json:"activeCount"`
	Saved       string `json:"saved"`
}

// Page es `PageOfGoal`: `items` siempre presente, `nextCursor` sólo si hay más y `totals`
// calculado sobre la colección completa.
type Page struct {
	Items      []GoalContract `json:"items"`
	NextCursor string         `json:"nextCursor,omitempty"`
	Totals     TotalsContract `json:"totals"`
}

// List devuelve una página de metas del titular junto a los totales de toda su colección.
//
// Secuencia: validar límite → validar y verificar el cursor (firma, expiración, pertenencia y
// filtros) → consultar la página → agregar la colección completa → proyectar cada fila al DTO →
// emitir el cursor siguiente sólo si la persistencia confirmó que hay más.
func (s *Service) List(ctx context.Context, query ListQuery) (*Page, error) {
	if s == nil || s.store == nil {
		return nil, ErrUnavailable
	}
	userID := strings.TrimSpace(query.UserID)
	if userID == "" {
		return nil, ErrUnauthorized
	}
	if len(s.cursorSecret) == 0 {
		// Sin clave de firma no se puede emitir ni verificar un cursor: se falla cerrado en
		// lugar de paginar de forma insegura.
		return nil, ErrUnavailable
	}

	limit, errLimite := NormalizeListLimit(query.Limit)
	if errLimite != nil {
		return nil, errLimite
	}

	var status domainGoal.Status
	hasStatus := false
	if raw := strings.TrimSpace(query.Status); raw != "" {
		parsed, errEstado := domainGoal.ParseStatus(raw)
		if errEstado != nil {
			return nil, fmt.Errorf("%w: status fuera del enum", ErrInvalidRequest)
		}
		status = parsed
		hasStatus = true
	}

	startSK := ""
	if raw := strings.TrimSpace(query.Cursor); raw != "" {
		if len(raw) < ListCursorMinLength || len(raw) > ListCursorMaxLength {
			return nil, fmt.Errorf("%w: cursor fuera de rango", ErrInvalidRequest)
		}
		payload, errCursor := httputil.ValidateCursor(s.cursorSecret, raw, userID)
		if errCursor != nil {
			// Manipulado, corrupto, vencido o de otro usuario: un único 400 sin distinguir.
			return nil, fmt.Errorf("%w: cursor no valido", ErrInvalidRequest)
		}
		if payload.SortBy != listSortKey(limit, status, hasStatus) {
			return nil, fmt.Errorf("%w: cursor emitido con otros filtros", ErrInvalidRequest)
		}
		if !strings.HasPrefix(payload.LastID, "GOAL#") {
			return nil, fmt.Errorf("%w: cursor no apunta a una meta", ErrInvalidRequest)
		}
		startSK = payload.LastID
	}

	rows, nextSK, errLista := s.store.ListUserGoals(ctx, ListStoreQuery{
		UserID:    userID,
		Status:    status,
		HasStatus: hasStatus,
		StartSK:   startSK,
		Limit:     limit,
	})
	if errLista != nil {
		return nil, normalizeStoreError(errLista)
	}

	totals, errTotales := s.store.AggregateUserGoals(ctx, userID)
	if errTotales != nil {
		return nil, normalizeStoreError(errTotales)
	}

	items := make([]GoalContract, 0, len(rows))
	for i := range rows {
		contract, errProyeccion := ContractOf(&rows[i].Goal)
		if errProyeccion != nil {
			return nil, errProyeccion
		}
		items = append(items, contract)
	}

	page := &Page{
		Items: items,
		Totals: TotalsContract{
			ActiveCount: totals.ActiveCount,
			Saved:       httputil.FormatCentsToDecimal(totals.Saved.Amount()),
		},
	}
	if nextSK != "" {
		cursor, errGen := httputil.GenerateCursor(
			s.cursorSecret, userID, listSortKey(limit, status, hasStatus), nextSK, s.now().UTC(), s.cursorTTL,
		)
		if errGen != nil {
			return nil, ErrUnavailable
		}
		page.NextCursor = cursor
	}
	return page, nil
}

// NormalizeListLimit valida `limit` del contrato (1..100, por defecto 25). Un valor cero se
// interpreta como ausencia del parámetro; cualquier otro fuera de rango es 400, nunca se
// recorta en silencio.
func NormalizeListLimit(limit int) (int, error) {
	if limit == 0 {
		return DefaultListLimit, nil
	}
	if limit < 1 || limit > MaxListLimit {
		return 0, fmt.Errorf("%w: limit fuera de rango", ErrInvalidRequest)
	}
	return limit, nil
}

// listSortKey liga el cursor al tamaño de página y al filtro con el que se emitió.
func listSortKey(limit int, status domainGoal.Status, hasStatus bool) string {
	filtro := "all"
	if hasStatus {
		filtro = status.String()
	}
	return listSortDomain + ":limit=" + strconv.Itoa(limit) + ":status=" + filtro
}
