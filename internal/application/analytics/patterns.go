package analytics

import (
	"context"
	"fmt"
	"sort"
	"strings"

	appMovements "github.com/nexumpr/Nexum-Backend/internal/application/movements"
)

// MinimumPatternSamples es el historial mínimo para afirmar que existe un patrón.
// Por debajo de este número no se emite nada: el contrato exige resultados sustentados
// por tamaño de muestra real y prohíbe afirmar certeza estadística sin esa evidencia.
const MinimumPatternSamples = 3

// SpendingPattern mapea 1:1 con el esquema `SpendingPattern` del OpenAPI.
type SpendingPattern struct {
	CategoryID  string
	Description string
	SampleSize  int
	Period      string
}

// SpendingPatternPage es `PageOfSpendingPattern`. No se emite nextCursor porque E054
// no declara parámetro de entrada para reanudarlo y la agregación interna ya consume
// todas las páginas de la partición del titular.
type SpendingPatternPage struct {
	Items      []SpendingPattern
	NextCursor string
}

// WireSpendingPattern es la proyección JSON estricta del esquema `SpendingPattern`.
type WireSpendingPattern struct {
	CategoryID  string `json:"categoryId"`
	Description string `json:"description"`
	SampleSize  int    `json:"sampleSize"`
	Period      string `json:"period"`
}

// WireSpendingPatternPage es la proyección JSON estricta de `PageOfSpendingPattern`.
type WireSpendingPatternPage struct {
	Items      []WireSpendingPattern `json:"items"`
	NextCursor string                `json:"nextCursor,omitempty"`
}

// Wire convierte la entidad al contrato de transporte sin filtrar datos del titular.
func (p *SpendingPatternPage) Wire() WireSpendingPatternPage {
	if p == nil {
		return WireSpendingPatternPage{Items: []WireSpendingPattern{}}
	}
	items := make([]WireSpendingPattern, 0, len(p.Items))
	for _, item := range p.Items {
		items = append(items, WireSpendingPattern(item))
	}
	return WireSpendingPatternPage{Items: items, NextCursor: p.NextCursor}
}

// Patterns ejecuta `GET /api/v1/analytics/spending-patterns`.
//
// Un patrón es una regularidad OBSERVABLE en movimientos reales del periodo: la misma
// categoría con gastos positivos en al menos `MinimumPatternSamples` fechas distintas.
// `sampleSize` es el número exacto de movimientos que lo sustentan y `description` cita
// sólo esos conteos. No se calculan probabilidades ni se afirma recurrencia futura.
func (s *Service) Patterns(ctx context.Context, userID, periodo string) (*SpendingPatternPage, error) {
	if s == nil || s.source == nil {
		return nil, ErrUnavailable
	}
	titular := strings.TrimSpace(userID)
	if titular == "" {
		return nil, ErrUnauthorized
	}
	_, desde, hasta, err := s.periodoValidado(titular, periodo)
	if err != nil {
		return nil, err
	}
	movimientos, err := s.loadMovements(ctx, titular, desde, hasta)
	if err != nil {
		return nil, err
	}

	type observacion struct {
		muestras int
		dias     map[int]struct{}
	}
	porCategoria := make(map[string]*observacion)
	for _, movimiento := range movimientos {
		if movimiento.Type != appMovements.TypeExpense || movimiento.AmountCents <= 0 {
			continue
		}
		categoria := strings.TrimSpace(movimiento.CategoryID)
		if categoria == "" {
			categoria = "sin-categoria"
		}
		obs := porCategoria[categoria]
		if obs == nil {
			obs = &observacion{dias: make(map[int]struct{})}
			porCategoria[categoria] = obs
		}
		obs.muestras++
		// El día se deriva de la marca UTC persistida: sin zonas IANA del cliente,
		// dos nodos agrupan exactamente igual.
		obs.dias[movimiento.OccurredAt.UTC().Day()] = struct{}{}
	}

	pagina := &SpendingPatternPage{Items: []SpendingPattern{}}
	periodoNormalizado := desde.Format("2006-01")
	for categoria, obs := range porCategoria {
		if obs.muestras < MinimumPatternSamples || len(obs.dias) < 2 {
			continue
		}
		pagina.Items = append(pagina.Items, SpendingPattern{
			CategoryID: categoria,
			Description: fmt.Sprintf(
				"Gasto recurrente mensual en %s: %d movimientos en %d dias distintos del periodo.",
				categoria, obs.muestras, len(obs.dias),
			),
			SampleSize: obs.muestras,
			Period:     periodoNormalizado,
		})
	}
	sort.Slice(pagina.Items, func(i, j int) bool {
		if pagina.Items[i].SampleSize != pagina.Items[j].SampleSize {
			return pagina.Items[i].SampleSize > pagina.Items[j].SampleSize
		}
		return pagina.Items[i].CategoryID < pagina.Items[j].CategoryID
	})
	return pagina, nil
}
