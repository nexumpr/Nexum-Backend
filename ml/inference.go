package ml

import (
	"fmt"
	"math"
	"sort"
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

// Assign calcula las features y asigna cada registro al centroide más cercano.
//
// Regla exacta de normalización (idéntica a ml/entrenar_kmeans.py):
//  1. Cada importe se recorta a [AmountClip.LowerCents, AmountClip.UpperCents]
//     antes de log1p y del ratio. Los límites son globales y vienen del entrenamiento.
//  2. El ratio se divide por `TrainedMedianCents`, la mediana FIJA del entrenamiento
//     calculada sobre importes recortados. Es un parámetro del artefacto (diseño §7
//     paso 2: "los mismos parámetros del artefacto").
//  3. No se recalcula la mediana con los importes del titular. Hacerlo cambiaría la
//     escala del ratio respecto a los centroides y produciría train/serve skew.
func (a *Artifact) Assign(registros []Registro) ([]Asignacion, error) {
	if a == nil {
		return nil, fmt.Errorf("ml: artefacto no cargado")
	}
	if err := a.AmountClip.validate(); err != nil {
		return nil, err
	}
	if !finito(a.TrainedMedianCents) || a.TrainedMedianCents <= 0 {
		return nil, fmt.Errorf("ml: mediana de entrenamiento invalida")
	}
	if len(registros) == 0 {
		return []Asignacion{}, nil
	}

	salida := make([]Asignacion, 0, len(registros))
	for i, registro := range registros {
		importe, err := a.clipAmount(registro.AmountCents)
		if err != nil {
			return nil, fmt.Errorf("ml: registro %d: %w", i, err)
		}
		vector, err := a.features(registro, importe, a.TrainedMedianCents)
		if err != nil {
			return nil, fmt.Errorf("ml: registro %d: %w", i, err)
		}
		cluster, err := a.NearestCluster(vector)
		if err != nil {
			return nil, fmt.Errorf("ml: registro %d: %w", i, err)
		}
		salida = append(salida, Asignacion{Cluster: cluster, Features: vector})
	}
	return salida, nil
}

// clipAmount aplica el recorte global del artefacto. Un importe no positivo es
// inválido: el contrato financiero no admite centavos <= 0 para estas features.
func (a *Artifact) clipAmount(cents int64) (float64, error) {
	if cents <= 0 {
		return 0, fmt.Errorf("importe no positivo")
	}
	importe := float64(cents)
	if importe < a.AmountClip.LowerCents {
		importe = a.AmountClip.LowerCents
	}
	if importe > a.AmountClip.UpperCents {
		importe = a.AmountClip.UpperCents
	}
	if !finito(importe) || importe <= 0 {
		return 0, fmt.Errorf("importe recortado invalido")
	}
	return importe, nil
}

// medianaFloat replica numpy.median: ordena y, si el conteo es par, promedia los
// dos valores centrales sin truncar a entero.
func medianaFloat(valores []float64) float64 {
	if len(valores) == 0 {
		return 0
	}
	copia := append([]float64(nil), valores...)
	sort.Float64s(copia)
	mitad := len(copia) / 2
	if len(copia)%2 == 1 {
		return copia[mitad]
	}
	return (copia[mitad-1] + copia[mitad]) / 2
}

// features construye el vector en el orden exacto del artefacto. Recibe el importe
// ya recortado y la mediana de importes recortados; no vuelve a decidir el recorte.
func (a *Artifact) features(r Registro, importeRecortado, medianaCents float64) ([]float64, error) {
	if a == nil {
		return nil, fmt.Errorf("artefacto no cargado")
	}
	if !finito(importeRecortado) || importeRecortado <= 0 {
		return nil, fmt.Errorf("importe recortado no positivo")
	}
	if !finito(medianaCents) || medianaCents <= 0 {
		return nil, fmt.Errorf("mediana no positiva")
	}
	if r.Hour < 0 || r.Hour > 23 || r.DayOfWeek < 0 || r.DayOfWeek > 6 {
		return nil, fmt.Errorf("marca temporal fuera de rango")
	}

	categoria := strings.TrimSpace(r.Category)
	if !a.tieneCategoria(categoria) {
		// Categorías heredadas o no catalogadas entran en "otros": nunca se
		// inventa una dimensión nueva que el centroide no conoce.
		categoria = "otros"
		if !a.tieneCategoria(categoria) {
			return nil, fmt.Errorf("el artefacto no define la categoria otros")
		}
	}

	hora := 2.0 * math.Pi * float64(r.Hour) / 24.0
	dia := 2.0 * math.Pi * float64(r.DayOfWeek) / 7.0
	vector := make([]float64, 0, len(a.FeatureOrder))
	vector = append(vector,
		(math.Log1p(importeRecortado)-a.LogImporteMean)/a.LogImporteStd,
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
	ratio := importeRecortado / medianaCents
	if ratio < 0 {
		ratio = 0
	}
	if ratio > 10 {
		ratio = 10
	}
	vector = append(vector, recurrente, ratio)

	if len(vector) != len(a.FeatureOrder) {
		return nil, fmt.Errorf("vector de %d features, esperado %d", len(vector), len(a.FeatureOrder))
	}
	for _, valor := range vector {
		if !finito(valor) {
			return nil, fmt.Errorf("feature no finita")
		}
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
	for _, valor := range features {
		if !finito(valor) {
			return 0, fmt.Errorf("ml: feature no finita")
		}
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
		if !finito(distancia) {
			return 0, fmt.Errorf("ml: distancia no finita")
		}
		if distancia < menor {
			menor = distancia
			mejor = i
		}
	}
	return mejor, nil
}

// TieneEstructura aplica el criterio de rechazo ML-155: silueta >= 0,35.
// Sin estructura no se emiten recomendaciones aunque haya suficientes datos.
func (a *Artifact) TieneEstructura() bool {
	return a != nil && a.SilhouetteTraining >= 0.35
}
