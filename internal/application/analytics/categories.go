package analytics

import (
	"context"
	"math"
	"sort"
	"strings"

	appMovements "github.com/nexumpr/Nexum-Backend/internal/application/movements"
)

// categoryCurrency es la moneda inicial y única del contrato E053 (enum=MXN).
const categoryCurrency = "MXN"

// CategoryAnalytic es la entidad de aplicación que mapea 1:1 con `CategoryAnalytic`.
// AmountCents se conserva entero: el decimal se genera sólo en la proyección de transporte.
type CategoryAnalytic struct {
	CategoryID  string
	Period      string
	AmountCents int64
	Currency    string
	Percentage  int
}

// CategoryPage es `PageOfCategoryAnalytic`. No se emite nextCursor: E053 no declara
// parámetro de entrada para reanudar una paginación, y la agregación interna ya recorre
// todas las páginas de la partición del titular.
type CategoryPage struct {
	Items      []CategoryAnalytic
	NextCursor string
}

// WireCategoryAnalytic es la proyección JSON estricta del esquema `CategoryAnalytic`.
type WireCategoryAnalytic struct {
	CategoryID string `json:"categoryId"`
	Period     string `json:"period"`
	Amount     string `json:"amount"`
	Currency   string `json:"currency"`
	Percentage int    `json:"percentage"`
}

// WireCategoryPage es la proyección JSON estricta de `PageOfCategoryAnalytic`.
type WireCategoryPage struct {
	Items      []WireCategoryAnalytic `json:"items"`
	NextCursor string                 `json:"nextCursor,omitempty"`
}

// Wire convierte la entidad al contrato de transporte sin filtrar datos del titular.
func (p *CategoryPage) Wire() WireCategoryPage {
	if p == nil {
		return WireCategoryPage{Items: []WireCategoryAnalytic{}}
	}
	items := make([]WireCategoryAnalytic, 0, len(p.Items))
	for _, item := range p.Items {
		items = append(items, WireCategoryAnalytic{
			CategoryID: item.CategoryID,
			Period:     item.Period,
			Amount:     FormatoCents(item.AmountCents),
			Currency:   item.Currency,
			Percentage: item.Percentage,
		})
	}
	return WireCategoryPage{Items: items, NextCursor: p.NextCursor}
}

// Categories ejecuta `GET /api/v1/analytics/categories`.
//
// Invariantes:
//   - Sólo gastos efectivamente persistidos del titular autenticado; nunca reglas
//     recurrentes no ejecutadas ni particiones ajenas.
//   - Importes en int64 centavos; `amount` se formatea con el decimal canónico.
//   - `percentage` usa el método del mayor resto: los porcentajes son deterministas,
//     están en 0..100 y suman exactamente 100 cuando hay gastos.
//   - Una dependencia caída o un conjunto no acotable produce 503, nunca un resultado
//     parcial presentado como válido.
func (s *Service) Categories(ctx context.Context, userID, periodo string) (*CategoryPage, error) {
	if s == nil || s.source == nil {
		return nil, ErrUnavailable
	}
	titular := strings.TrimSpace(userID)
	if titular == "" {
		return nil, ErrUnauthorized
	}
	desde, hasta, err := s.periodoValidado(titular, periodo)
	if err != nil {
		return nil, err
	}
	movimientos, err := s.loadMovements(ctx, titular, desde, hasta)
	if err != nil {
		return nil, err
	}

	porCategoria := make(map[string]int64)
	for _, movimiento := range movimientos {
		if movimiento.Type != appMovements.TypeExpense || movimiento.AmountCents <= 0 {
			continue
		}
		categoria := strings.TrimSpace(movimiento.CategoryID)
		if categoria == "" {
			categoria = "sin-categoria"
		}
		porCategoria[categoria] += movimiento.AmountCents
	}

	pagina := &CategoryPage{Items: []CategoryAnalytic{}}
	if len(porCategoria) == 0 {
		return pagina, nil
	}

	total := int64(0)
	for _, cents := range porCategoria {
		total += cents
	}
	if total <= 0 || total > math.MaxInt64/100 {
		return nil, ErrUnavailable
	}

	type reparto struct {
		categoria  string
		cents      int64
		porcentaje int
		resto      int64
	}
	repartos := make([]reparto, 0, len(porCategoria))
	asignado := int64(0)
	for categoria, cents := range porCategoria {
		escalado := cents * 100
		base := escalado / total
		if base < 0 || base > 100 {
			return nil, ErrUnavailable
		}
		repartos = append(repartos, reparto{
			categoria:  categoria,
			cents:      cents,
			porcentaje: int(base),
			resto:      escalado - base*total,
		})
		asignado += base
	}

	// El redondeo por mayor resto garantiza que la suma sea 100 y que el empate se
	// resuelva por categoryId, de modo que dos nodos devuelvan exactamente el mismo JSON.
	faltan := int(100 - asignado)
	sort.Slice(repartos, func(i, j int) bool {
		if repartos[i].resto != repartos[j].resto {
			return repartos[i].resto > repartos[j].resto
		}
		return repartos[i].categoria < repartos[j].categoria
	})
	for i := 0; i < faltan && i < len(repartos); i++ {
		repartos[i].porcentaje++
	}

	sort.Slice(repartos, func(i, j int) bool {
		if repartos[i].cents != repartos[j].cents {
			return repartos[i].cents > repartos[j].cents
		}
		return repartos[i].categoria < repartos[j].categoria
	})
	for _, r := range repartos {
		pagina.Items = append(pagina.Items, CategoryAnalytic{
			CategoryID:  r.categoria,
			Period:      "", // se fija abajo, tras conocer el periodo normalizado
			AmountCents: r.cents,
			Currency:    categoryCurrency,
			Percentage:  r.porcentaje,
		})
	}

	// El inicio del rango ya es el mes normalizado: reconstruir YYYY-MM desde él evita
	// propagar un alias del parámetro de entrada sin validar.
	periodoNormalizado := desde.Format("2006-01")
	for i := range pagina.Items {
		pagina.Items[i].Period = periodoNormalizado
	}
	return pagina, nil
}
