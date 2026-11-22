package analytics

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	appMovements "github.com/nexumpr/Nexum-Backend/internal/application/movements"
)

// Service resuelve E052 leyendo únicamente la partición del titular autenticado.
type Service struct {
	source MovementSource
	now    func() time.Time
	maxPag int
}

// NewService construye el caso de uso con la dependencia real inyectada.
func NewService(source MovementSource) *Service {
	return &Service{source: source, now: time.Now, maxPag: maxPages}
}

// WithClock permite pruebas deterministas sin tocar el reloj del sistema.
func (s *Service) WithClock(fn func() time.Time) *Service {
	if s == nil {
		return nil
	}
	copia := *s
	if fn != nil {
		copia.now = fn
	}
	return &copia
}

// Insights ejecuta `GET /api/v1/analytics/insights`.
//
// Decisiones verificables:
//   - `period` se valida con el patrón del OpenAPI; ausente → mes UTC actual.
//   - La consulta es `Query` acotada a `USER#<sub>` y al rango del mes; nunca `Scan`.
//   - `enoughData` es false con menos de 12 movimientos y en ese caso `items=[]`.
//   - El insight se calcula sólo con gastos reales; `evidenceCount` es el número de
//     movimientos de gasto de la categoría ganadora y `explanation` cita importes exactos.
//   - Un corte de páginas antes del final produce 503: no se devuelve un análisis parcial.
func (s *Service) Insights(ctx context.Context, userID, periodo string) (*Page, error) {
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

	pagina := &Page{
		Items:          []Insight{},
		EnoughData:     len(movimientos) >= MinimumSamples,
		MinimumSamples: MinimumSamples,
	}
	if !pagina.EnoughData {
		return pagina, nil
	}

	type acumulado struct {
		conteo int
		cents  int64
	}
	porCategoria := make(map[string]acumulado)
	totalGastos := int64(0)
	for _, movimiento := range movimientos {
		if movimiento.Type != appMovements.TypeExpense {
			continue
		}
		categoria := strings.TrimSpace(movimiento.CategoryID)
		if categoria == "" {
			categoria = "sin-categoria"
		}
		actual := porCategoria[categoria]
		actual.conteo++
		actual.cents += movimiento.AmountCents
		porCategoria[categoria] = actual
		totalGastos += movimiento.AmountCents
	}
	if len(porCategoria) == 0 || totalGastos <= 0 {
		return pagina, nil
	}

	categorias := make([]string, 0, len(porCategoria))
	for categoria := range porCategoria {
		categorias = append(categorias, categoria)
	}
	sort.Strings(categorias)
	ganadora := categorias[0]
	for _, categoria := range categorias[1:] {
		if porCategoria[categoria].cents > porCategoria[ganadora].cents ||
			(porCategoria[categoria].cents == porCategoria[ganadora].cents && categoria < ganadora) {
			ganadora = categoria
		}
	}

	datos := porCategoria[ganadora]
	porcentaje := 0
	if totalGastos > 0 {
		porcentaje = int((datos.cents * 100) / totalGastos)
	}
	explicacion := fmt.Sprintf(
		"En %s se registraron %d gastos en %s por un total de %s MXN (%d%% del gasto categorizado del periodo).",
		periodo, datos.conteo, ganadora, FormatoCents(datos.cents), porcentaje,
	)

	pagina.Items = append(pagina.Items, Insight{
		ID:            insightID(titular, periodo, ganadora),
		Title:         "Ahorro posible por categoría",
		Explanation:   explicacion,
		Period:        periodo,
		EvidenceCount: datos.conteo,
		ModelVersion:  ModelVersion,
	})
	return pagina, nil
}

// insightID deriva un UUID v5 determinista sin exponer el identificador del usuario.
// El mismo titular, periodo y categoría producen siempre el mismo id; otro titular produce
// otro id sin que su valor permita reconstruir quién es.
func insightID(userID, periodo, categoria string) string {
	suma := sha256.Sum256([]byte("nexum-e052\x00" + userID + "\x00" + periodo + "\x00" + categoria))
	b := suma[:16]
	b[6] = (b[6] & 0x0f) | 0x50 // versión 5
	b[8] = (b[8] & 0x3f) | 0x80 // variante RFC 4122
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// periodoValidado normaliza `period` con el reloj inyectado y devuelve los límites UTC
// del mes. Lo comparten E052 y E053 para que un mismo parámetro produzca exactamente el
// mismo conjunto de movimientos en ambos endpoints.
func (s *Service) periodoValidado(userID, periodo string) (time.Time, time.Time, error) {
	ahora := time.Now().UTC()
	if s != nil && s.now != nil {
		ahora = s.now().UTC()
	}
	periodo, err := NormalizePeriod(periodo, ahora)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	return PeriodBounds(periodo)
}

// loadMovements recorre la partición del titular en páginas acotadas. Es la única
// entrada de datos de E052 y E053: una Query por `USER#<sub>` con rango de mes, nunca
// un Scan. Un corte antes del final devuelve ErrUnavailable para no calcular con datos
// incompletos.
func (s *Service) loadMovements(ctx context.Context, userID string, desde, hasta time.Time) ([]appMovements.Movement, error) {
	if s == nil || s.source == nil {
		return nil, ErrUnavailable
	}
	consulta := appMovements.ListQuery{
		UserID:  userID,
		Limit:   pageSize,
		From:    desde,
		HasFrom: true,
		// ListMovements usa SK < To+1ms. Restar 1ms deja el límite exclusivo en el
		// primer instante del mes siguiente, sin depender de caracteres centinela.
		To:    hasta.Add(-time.Millisecond),
		HasTo: true,
	}

	movimientos := make([]appMovements.Movement, 0, MinimumSamples)
	startSK := ""
	limite := s.maxPag
	if limite <= 0 {
		limite = maxPages
	}
	for pagina := 0; pagina < limite; pagina++ {
		filas, siguiente, errLista := s.source.ListMovements(ctx, consulta, startSK, pageSize)
		if errLista != nil {
			return nil, traducirError(errLista)
		}
		movimientos = append(movimientos, filas...)
		if strings.TrimSpace(siguiente) == "" {
			return movimientos, nil
		}
		if siguiente == startSK {
			return nil, fmt.Errorf("%w: cursor sin progreso", ErrUnavailable)
		}
		startSK = siguiente
		if pagina == limite-1 {
			return nil, fmt.Errorf("%w: conjunto demasiado grande", ErrUnavailable)
		}
	}
	return movimientos, nil
}

func traducirError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, ErrInvalidRequest),
		errors.Is(err, ErrUnauthorized),
		errors.Is(err, ErrUnavailable),
		errors.Is(err, ErrInternal):
		return err
	case errors.Is(err, appMovements.ErrUnavailable):
		return ErrUnavailable
	case errors.Is(err, appMovements.ErrInvalidRequest):
		return ErrInvalidRequest
	case errors.Is(err, appMovements.ErrUnauthorized):
		return ErrUnauthorized
	default:
		return ErrInternal
	}
}
