// Package analytics implementa los casos de uso de analítica verificable de la Oleada 4.
//
// E052 no ejecuta un modelo generativo ni presenta reglas fijas como IA: produce insights
// deterministas a partir de movimientos reales del titular. `modelVersion` identifica
// exactamente la implementación que generó la respuesta.
package analytics

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	appMovements "github.com/nexumpr/Nexum-Backend/internal/application/movements"
	"github.com/nexumpr/Nexum-Backend/pkg/httputil"
)

const (
	// MinimumSamples es el mínimo de movimientos del periodo para afirmar `enoughData`.
	// Coincide con el ejemplo canónico del OpenAPI (12) y con las pruebas de la guía V2.
	MinimumSamples = 12

	// ModelVersion es la versión del cálculo determinista usado por E052. No se presenta
	// como modelo generativo ni como ML: E055 es el único endpoint con ML real.
	ModelVersion = "analytics-v1"

	// pageSize coincide con el máximo de E027. Varias páginas se recorren de forma acotada.
	pageSize = appMovements.MaxLimit

	// maxPages acota el trabajo por petición (2000 movimientos). Si se alcanza, la
	// respuesta es 503 en lugar de un insight calculado con datos incompletos.
	maxPages = 20
)

var (
	// ErrInvalidRequest es una forma de petición no admitida por el contrato (HTTP 400).
	ErrInvalidRequest = errors.New("analytics: solicitud invalida")
	// ErrUnauthorized indica identidad no utilizable (HTTP 401).
	ErrUnauthorized = errors.New("analytics: identidad no autorizada")
	// ErrUnavailable es una dependencia no disponible o un conjunto no verificable (HTTP 503).
	ErrUnavailable = errors.New("analytics: dependencia no disponible")
	// ErrInternal es un fallo interno no clasificable (HTTP 500).
	ErrInternal = errors.New("analytics: fallo interno")
)

var periodPattern = regexp.MustCompile(`^\d{4}-(0[1-9]|1[0-2])$`)

// MovementSource es la capacidad mínima que E052 necesita del almacén de movimientos.
// La implementación real es *dynamo.MovementsStore: la interfaz no expone Scan.
type MovementSource interface {
	ListMovements(ctx context.Context, query appMovements.ListQuery, startSK string, limit int) ([]appMovements.Movement, string, error)
}

// Insight es la entidad de aplicación que mapea 1:1 con el esquema `Insight` del OpenAPI.
type Insight struct {
	ID            string
	Title         string
	Explanation   string
	Period        string
	EvidenceCount int
	ModelVersion  string
}

// Page es `PageOfInsight` sin el detalle de transporte. NextCursor se omite porque el
// contrato de E052 sólo declara `period` como parámetro de entrada: no existe una forma
// autorizada de reanudar una paginación, así que nunca se emite un cursor huérfano.
type Page struct {
	Items          []Insight
	NextCursor     string
	EnoughData     bool
	MinimumSamples int
}

// WireInsight es la proyección JSON estricta del esquema `Insight`.
type WireInsight struct {
	ID            string `json:"id"`
	Title         string `json:"title"`
	Explanation   string `json:"explanation"`
	Period        string `json:"period"`
	EvidenceCount int    `json:"evidenceCount"`
	ModelVersion  string `json:"modelVersion"`
}

// WirePage es la proyección JSON estricta de `PageOfInsight`.
type WirePage struct {
	Items          []WireInsight `json:"items"`
	NextCursor     string        `json:"nextCursor,omitempty"`
	EnoughData     bool          `json:"enoughData"`
	MinimumSamples int           `json:"minimumSamples"`
}

// Wire convierte la entidad al contrato de transporte sin filtrar datos del titular.
func (p *Page) Wire() WirePage {
	if p == nil {
		return WirePage{Items: []WireInsight{}}
	}
	items := make([]WireInsight, 0, len(p.Items))
	for _, insight := range p.Items {
		items = append(items, WireInsight(insight))
	}
	return WirePage{
		Items:          items,
		NextCursor:     p.NextCursor,
		EnoughData:     p.EnoughData,
		MinimumSamples: p.MinimumSamples,
	}
}

// NormalizePeriod valida `YYYY-MM` y, cuando está ausente, usa el mes UTC del reloj inyectado.
func NormalizePeriod(valor string, now time.Time) (string, error) {
	periodo := strings.TrimSpace(valor)
	if periodo == "" {
		if now.IsZero() {
			now = time.Now().UTC()
		}
		return now.UTC().Format("2006-01"), nil
	}
	if !periodPattern.MatchString(periodo) {
		return "", fmt.Errorf("%w: period", ErrInvalidRequest)
	}
	if _, err := time.Parse("2006-01", periodo); err != nil {
		return "", fmt.Errorf("%w: period", ErrInvalidRequest)
	}
	return periodo, nil
}

// PeriodBounds devuelve el primer instante del mes y el primer instante del mes siguiente.
func PeriodBounds(periodo string) (time.Time, time.Time, error) {
	inicio, err := time.ParseInLocation("2006-01", periodo, time.UTC)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("%w: period", ErrInvalidRequest)
	}
	return inicio, inicio.AddDate(0, 1, 0), nil
}

// FormatoCents reutiliza el formateador canónico para que el texto de la explicación
// use exactamente el mismo decimal que el resto de la API.
func FormatoCents(cents int64) string { return httputil.FormatCentsToDecimal(cents) }
