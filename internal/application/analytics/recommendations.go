package analytics

import (
	"context"
	"crypto/sha256"
	"fmt"
	"sort"
	"strings"
	"time"

	appMovements "github.com/nexumpr/Nexum-Backend/internal/application/movements"
	"github.com/nexumpr/Nexum-Backend/ml"
)

const (
	// RecommendationMinimumCategories es el mínimo de categorías distintas exigido
	// por ML-155 antes de afirmar que hay historial suficiente.
	RecommendationMinimumCategories = 2

	// RecommendationMaxMovements acota el cálculo a 500 movimientos. Si la ventana
	// trae más, se muestrea de forma determinista y la razón lo declara.
	RecommendationMaxMovements = 500

	// RecommendationMinClusterShare y RecommendationMinClusterSize definen la masa
	// mínima para emitir una recomendación por cluster (§7 de ML-155).
	RecommendationMinClusterShare = 0.20
	RecommendationMinClusterSize  = 3

	// RecommendationDisclaimer es obligatorio en cada ítem y no puede omitirse.
	RecommendationDisclaimer = "Contenido educativo basado en tus movimientos; no constituye asesoramiento financiero profesional ni garantiza ahorro."
)

// Recommendation mapea 1:1 con el esquema `Recommendation` del OpenAPI.
type Recommendation struct {
	ID           string
	Title        string
	Reason       string
	Disclaimer   string
	ModelVersion string
}

// RecommendationPage es `PageOfRecommendation`. No se emite nextCursor: E055 no
// declara un parámetro de entrada para reanudarlo y la agregación consume la
// ventana completa del titular.
type RecommendationPage struct {
	Items         []Recommendation
	NextCursor    string
	HasEnoughData bool
}

// WireRecommendation es la proyección JSON estricta del esquema `Recommendation`.
type WireRecommendation struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	Reason       string `json:"reason"`
	Disclaimer   string `json:"disclaimer"`
	ModelVersion string `json:"modelVersion"`
}

// WireRecommendationPage es la proyección JSON estricta de `PageOfRecommendation`.
type WireRecommendationPage struct {
	Items         []WireRecommendation `json:"items"`
	NextCursor    string               `json:"nextCursor,omitempty"`
	HasEnoughData bool                 `json:"hasEnoughData"`
}

// Wire convierte la entidad al contrato de transporte sin filtrar datos del titular.
func (p *RecommendationPage) Wire() WireRecommendationPage {
	if p == nil {
		return WireRecommendationPage{Items: []WireRecommendation{}}
	}
	items := make([]WireRecommendation, 0, len(p.Items))
	for _, item := range p.Items {
		items = append(items, WireRecommendation(item))
	}
	return WireRecommendationPage{Items: items, NextCursor: p.NextCursor, HasEnoughData: p.HasEnoughData}
}

// Recommendations ejecuta `GET /api/v1/analytics/recommendations` con el modelo
// K-Means de ML-155. Sin historial suficiente o sin estructura, responde 200 con
// `hasEnoughData=false` e `items=[]`; nunca inventa ahorros ni consejos profesionales.
func (s *Service) Recommendations(ctx context.Context, userID, periodo string) (*RecommendationPage, error) {
	if s == nil || s.source == nil {
		return nil, ErrUnavailable
	}
	titular := strings.TrimSpace(userID)
	if titular == "" {
		return nil, ErrUnauthorized
	}

	modelo := s.modelo
	if modelo == nil {
		cargado, err := ml.Load()
		if err != nil {
			return nil, ErrUnavailable
		}
		modelo = cargado
	}

	desde, hasta, etiqueta, err := s.ventanaRecomendaciones(titular, periodo)
	if err != nil {
		return nil, err
	}
	movimientos, err := s.loadMovements(ctx, titular, desde, hasta)
	if err != nil {
		return nil, err
	}

	pagina := &RecommendationPage{Items: []Recommendation{}, HasEnoughData: false}
	utiles := make([]appMovements.Movement, 0, len(movimientos))
	for _, movimiento := range movimientos {
		if movimiento.AmountCents > 0 {
			utiles = append(utiles, movimiento)
		}
	}
	sort.Slice(utiles, func(i, j int) bool {
		if !utiles[i].OccurredAt.Equal(utiles[j].OccurredAt) {
			return utiles[i].OccurredAt.Before(utiles[j].OccurredAt)
		}
		return utiles[i].ID < utiles[j].ID
	})

	categorias := make(map[string]struct{})
	for _, movimiento := range utiles {
		categoria := strings.TrimSpace(movimiento.CategoryID)
		if categoria == "" {
			categoria = "otros"
		}
		categorias[categoria] = struct{}{}
	}
	if len(utiles) < MinimumSamples || len(categorias) < RecommendationMinimumCategories || !modelo.TieneEstructura() {
		return pagina, nil
	}

	muestreados, limitado := muestraDeterminista(utiles, RecommendationMaxMovements)
	registros := make([]ml.Registro, 0, len(muestreados))
	for _, movimiento := range muestreados {
		registros = append(registros, ml.Registro{
			Category:    strings.TrimSpace(movimiento.CategoryID),
			AmountCents: movimiento.AmountCents,
			Hour:        movimiento.OccurredAt.UTC().Hour(),
			// Go usa domingo=0; el pipeline Python usa lunes=0.
			DayOfWeek: (int(movimiento.OccurredAt.UTC().Weekday()) + 6) % 7,
			Recurring: strings.TrimSpace(movimiento.RecurringPaymentID) != "",
		})
	}
	// El modelo calcula internamente el recorte global y la mediana de importes
	// recortados del titular. No se le pasa una mediana externa para que la
	// definición de train y serve sea exactamente la misma.
	asignaciones, err := modelo.Assign(registros)
	if err != nil {
		return nil, ErrUnavailable
	}

	typeInfo := make([]*infoCluster, modelo.K)
	for i := 0; i < modelo.K; i++ {
		typeInfo[i] = &infoCluster{indice: i, categorias: make(map[string]int)}
	}
	for i, asignacion := range asignaciones {
		grupo := typeInfo[asignacion.Cluster]
		movimiento := muestreados[i]
		categoria := strings.TrimSpace(movimiento.CategoryID)
		if categoria == "" {
			categoria = "otros"
		}
		grupo.movimientos++
		grupo.importes = append(grupo.importes, movimiento.AmountCents)
		grupo.categorias[categoria]++
		grupo.horas = append(grupo.horas, movimiento.OccurredAt.UTC().Hour())
	}

	type candidato struct {
		item  Recommendation
		count int
	}
	candidatos := make([]candidato, 0, modelo.K)
	for _, grupo := range typeInfo {
		porcentaje := float64(grupo.movimientos) / float64(len(muestreados))
		if grupo.movimientos < RecommendationMinClusterSize || porcentaje < RecommendationMinClusterShare {
			continue
		}
		categoria, cantidad := categoriaDominante(grupo.categorias)
		minHora, maxHora := rangoHoras(grupo.horas)
		razon := fmt.Sprintf(
			"El grupo %d reúne %d movimientos de %s; la categoría más frecuente es %s (%d de ellos), con mediana %s MXN y franja %02d–%02d h, calculado sobre tus movimientos de %s.",
			grupo.indice+1, grupo.movimientos, etiqueta, categoria, cantidad,
			FormatoCents(medianaEntera(grupo.importes)), minHora, maxHora, etiqueta,
		)
		if limitado {
			razon += fmt.Sprintf(" La muestra se limitó a %d movimientos de la ventana.", RecommendationMaxMovements)
		}
		candidatos = append(candidatos, candidato{
			count: grupo.movimientos,
			item: Recommendation{
				ID:           recommendationID(titular, etiqueta, grupo.indice, categoria),
				Title:        fmt.Sprintf("Revisar el grupo de movimientos de %s", categoria),
				Reason:       razon,
				Disclaimer:   RecommendationDisclaimer,
				ModelVersion: modelo.Version(),
			},
		})
	}
	sort.Slice(candidatos, func(i, j int) bool {
		if candidatos[i].count != candidatos[j].count {
			return candidatos[i].count > candidatos[j].count
		}
		return candidatos[i].item.ID < candidatos[j].item.ID
	})
	for _, c := range candidatos {
		pagina.Items = append(pagina.Items, c.item)
	}
	pagina.HasEnoughData = true
	return pagina, nil
}

type infoCluster struct {
	indice      int
	movimientos int
	importes    []int64
	categorias  map[string]int
	horas       []int
}

// ventanaRecomendaciones aplica el periodo explícito del contrato o, si falta,
// la ventana móvil de 90 días del diseño ML-155. Todas las fechas son UTC.
//
// La línea "excluir el trimestre en curso" del diseño no se aplica: contradice la
// ventana explícita de 90 días y no forma parte del contrato E055 aprobado. Queda
// registrada para enmienda del diseño; los datos del trimestre en curso sí cuentan.
func (s *Service) ventanaRecomendaciones(userID, periodo string) (time.Time, time.Time, string, error) {
	ahora := time.Now().UTC()
	if s != nil && s.now != nil {
		ahora = s.now().UTC()
	}
	if strings.TrimSpace(periodo) != "" {
		normalizado, desde, hasta, err := s.periodoValidado(userID, periodo)
		if err != nil {
			return time.Time{}, time.Time{}, "", err
		}
		return desde, hasta, normalizado, nil
	}
	hoy := time.Date(ahora.Year(), ahora.Month(), ahora.Day(), 0, 0, 0, 0, time.UTC)
	desde := hoy.AddDate(0, 0, -89)
	hasta := hoy.AddDate(0, 0, 1)
	return desde, hasta, desde.Format("2006-01-02") + ".." + hoy.Format("2006-01-02"), nil
}

// muestraDeterminista conserva el orden recibido y toma pasos regulares. Ante más
// de 500 movimientos, la selección es reproducible sin depender de mapas ni reloj.
func muestraDeterminista(movimientos []appMovements.Movement, max int) ([]appMovements.Movement, bool) {
	if max <= 0 || len(movimientos) <= max {
		return movimientos, false
	}
	paso := (len(movimientos) + max - 1) / max
	salida := make([]appMovements.Movement, 0, max)
	for i := 0; i < len(movimientos) && len(salida) < max; i += paso {
		salida = append(salida, movimientos[i])
	}
	return salida, true
}

func medianaEntera(valores []int64) int64 {
	if len(valores) == 0 {
		return 0
	}
	copia := append([]int64(nil), valores...)
	sort.Slice(copia, func(i, j int) bool { return copia[i] < copia[j] })
	mitad := len(copia) / 2
	if len(copia)%2 == 1 {
		return copia[mitad]
	}
	return (copia[mitad-1] + copia[mitad]) / 2
}

func categoriaDominante(conteos map[string]int) (string, int) {
	claves := make([]string, 0, len(conteos))
	for clave := range conteos {
		claves = append(claves, clave)
	}
	sort.Strings(claves)
	mejor := claves[0]
	for _, clave := range claves[1:] {
		if conteos[clave] > conteos[mejor] {
			mejor = clave
		}
	}
	return mejor, conteos[mejor]
}

func rangoHoras(horas []int) (int, int) {
	min, max := horas[0], horas[0]
	for _, hora := range horas[1:] {
		if hora < min {
			min = hora
		}
		if hora > max {
			max = hora
		}
	}
	return min, max
}

// recommendationID deriva un UUID v5 determinista. La identidad del titular se
// usa como entropía, nunca como texto en claro, y el mismo cluster del mismo
// periodo produce siempre el mismo identificador.
func recommendationID(userID, etiqueta string, cluster int, categoria string) string {
	suma := sha256.Sum256([]byte("nexum-e055\x00" + userID + "\x00" + etiqueta + "\x00" + fmt.Sprint(cluster) + "\x00" + categoria))
	b := suma[:16]
	b[6] = (b[6] & 0x0f) | 0x50
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
