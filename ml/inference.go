package ml

import (
	"fmt"
	"math"
	"strings"
)

// Registro es la proyección mínima y sin PII de un movimiento para el modelo.
// Sólo importe, marca temporal, categoría y recurrencia; nunca id de usuario,
// correo, nombre ni texto libre.
type Registro struct {
	Category    string
	AmountCents int64
	Hour        int
	DayOfWeek   int // convención Python: lunes=0 ... domingo=6
	Recurring   bool
}

// Asignacion guarda el cluster elegido y el vector que lo produjo, para que la
// capa de explicación pueda citar sólo hechos calculados.
type Asignacion struct {
	Cluster  int
	Features []float64
}

// Features construye el vector en el orden exacto del artefacto. La mediana se
// recibe como parámetro: en producción es la del titular; en la prueba de
// referencia es la del conjunto sintético usado por Python.
func (a *Artifact) Features(r Registro, medianaCents float64) ([]float64, error) {
	if a == nil {
		return nil, fmt.Errorf("ml: artefacto no cargado")
	}
	if medianaCents <= 0 {
		return nil, fmt.Errorf("ml: mediana no positiva")
	}
	if r.Hour < 0 || r.Hour > 23 || r.DayOfWeek < 0 || r.DayOfWeek > 6 {
		return nil, fmt.Errorf("ml: marca temporal fuera de rango")
	}

	categoria := strings.TrimSpace(r.Category)
	if !a.tieneCategoria(categoria) {
		// Categorías heredadas o no catalogadas entran en "otros": nunca se
		// inventa una dimensión nueva que el centroide no conoce.
		categoria = "otros"
		if !a.tieneCategoria(categoria) {
			return nil, fmt.Errorf("ml: el artefacto no define la categoria otros")
		}
	}

	hora := 2.0 * math.Pi * float64(r.Hour) / 24.0
	dia := 2.0 * math.Pi * float64(r.DayOfWeek) / 7.0
	importe := math.Abs(float64(r.AmountCents))
	vector := make([]float64, 0, len(a.FeatureOrder))
	vector = append(vector,
		(math.Log1p(importe)-a.LogImporteMean)/a.LogImporteStd,
		math.Sin(hora), math.Cos(hora),
		math.Sin(dia), math.Cos(dia),
	)
	for _, nombre := range a.Categories {
		if nombre == categoria {
			vector = append(vector, 1)
		} else {
			vector = append(vector, 0)
		}
	}
	recurrente := 0.0
	if r.Recurring {
		recurrente = 1
	}
	ratio := math.Min(importe/medianaCents, 10.0)
	vector = append(vector, recurrente, ratio)

	if len(vector) != len(a.FeatureOrder) {
		return nil, fmt.Errorf("ml: vector de %d features, esperado %d", len(vector), len(a.FeatureOrder))
	}
	return vector, nil
}

func (a *Artifact) tieneCategoria(categoria string) bool {
	if a == nil {
		return false
	}
	for _, nombre := range a.Categories {
		if nombre == categoria {
			return true
		}
	}
	return false
}

// NearestCluster elige el centroide más cercano por distancia euclídea al
// cuadrado. El desempate estricto por índice hace la salida determinista.
func (a *Artifact) NearestCluster(features []float64) (int, error) {
	if a == nil {
		return 0, fmt.Errorf("ml: artefacto no cargado")
	}
	if len(features) != len(a.FeatureOrder) {
		return 0, fmt.Errorf("ml: dimension de features incorrecta")
	}
	mejor := 0
	menor := math.Inf(1)
	for i, centroide := range a.Centroids {
		if len(centroide) != len(features) {
			return 0, fmt.Errorf("ml: centroide con dimension incorrecta")
		}
		distancia := 0.0
		for j, valor := range features {
			diferencia := valor - centroide[j]
			distancia += diferencia * diferencia
		}
		if distancia < menor {
			menor = distancia
			mejor = i
		}
	}
	return mejor, nil
}

// Assign calcula todas las asignaciones de una sola pasada por registro.
func (a *Artifact) Assign(registros []Registro, medianaCents float64) ([]Asignacion, error) {
	salida := make([]Asignacion, 0, len(registros))
	for _, registro := range registros {
		features, err := a.Features(registro, medianaCents)
		if err != nil {
			return nil, err
		}
		cluster, err := a.NearestCluster(features)
		if err != nil {
			return nil, err
		}
		salida = append(salida, Asignacion{Cluster: cluster, Features: features})
	}
	return salida, nil
}

// TieneEstructura aplica el criterio de rechazo ML-155: silueta >= 0,35.
// Sin estructura no se emiten recomendaciones aunque haya suficientes datos.
func (a *Artifact) TieneEstructura() bool {
	return a != nil && a.SilhouetteTraining >= 0.35
}
