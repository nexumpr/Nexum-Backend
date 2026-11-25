// Package ml carga y verifica el artefacto K-Means de ML-155 y ejecuta inferencia nativa en Go.
// No hay Python, AWS ni llamadas a proveedores en la ruta de petición.
package ml

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"sync"
)

// Los archivos activos se incrustan: el binario no depende de rutas del filesystem y el
// hash del conjunto sintético se verifica en cada carga. v1 permanece en el repositorio
// como artefacto histórico, pero no se carga en producción.
//
//go:embed modelo_kmeans_v2.json datos_sinteticos_v1.json metricas_v2.json referencia_inferencia_v2.json
var files embed.FS

const (
	// ActiveModelVersion es la versión que viaja en `modelVersion`.
	ActiveModelVersion = "kmeans-v2"

	// ActiveSchemaVersion es el esquema del artefacto activo.
	ActiveSchemaVersion = 2

	// MedianDefinitionTrained documenta la única definición admitida de la mediana:
	// mediana FIJA de los importes recortados del conjunto de entrenamiento. Es un
	// parámetro del artefacto y la inferencia usa `trained_median_cents` sin
	// recalcularla por petición. Calcularla con los importes de cada titular
	// cambiaría el espacio de features y descolocaría los centroides (skew).
	MedianDefinitionTrained = "trained_median_of_clipped_amounts"

	expectedSeed  int64 = 20261116
	expectedNInit int   = 10
)

// AmountClip fija el recorte global de outliers calculado en entrenamiento.
// La inferencia aplica exactamente estos límites antes de log1p y del ratio.
type AmountClip struct {
	LowerPercentile float64 `json:"lower_percentile"`
	UpperPercentile float64 `json:"upper_percentile"`
	LowerCents      float64 `json:"lower_cents"`
	UpperCents      float64 `json:"upper_cents"`
}

// Artifact es el contrato del modelo versionado exportado por ml/entrenar_kmeans.py.
type Artifact struct {
	SchemaVersion       int         `json:"schema_version"`
	ModelVersion        string      `json:"model_version"`
	Algorithm           string      `json:"algorithm"`
	Seed                int64       `json:"seed"`
	NInit               int         `json:"n_init"`
	K                   int         `json:"k"`
	FeatureOrder        []string    `json:"feature_order"`
	Categories          []string    `json:"categories"`
	AmountClip          AmountClip  `json:"amount_clip"`
	MedianDefinition    string      `json:"median_definition"`
	LogImporteMean      float64     `json:"log_importe_mean"`
	LogImporteStd       float64     `json:"log_importe_std"`
	TrainedMedianCents  float64     `json:"trained_median_cents"`
	Centroids           [][]float64 `json:"centroids"`
	SilhouetteTraining  float64     `json:"silhouette_training"`
	Inertia             float64     `json:"inertia"`
	SyntheticDataSHA256 string      `json:"synthetic_data_sha256"`
	GeneratedBy         string      `json:"generated_by"`
}

var (
	cargarUnaVez sync.Once
	artefacto    *Artifact
	cargaError   error
)

// Load devuelve el artefacto v2 verificado. Es thread-safe y se ejecuta una sola vez.
func Load() (*Artifact, error) {
	cargarUnaVez.Do(func() {
		artefacto, cargaError = load()
	})
	return artefacto, cargaError
}

func load() (*Artifact, error) {
	crudo, err := files.ReadFile("modelo_kmeans_v2.json")
	if err != nil {
		return nil, fmt.Errorf("ml: artefacto no disponible: %w", err)
	}
	var a Artifact
	if err := json.Unmarshal(crudo, &a); err != nil {
		return nil, fmt.Errorf("ml: artefacto invalido: %w", err)
	}
	if err := a.validate(); err != nil {
		return nil, err
	}

	datos, err := files.ReadFile("datos_sinteticos_v1.json")
	if err != nil {
		return nil, fmt.Errorf("ml: datos sinteticos no disponibles: %w", err)
	}
	suma := sha256.Sum256(datos)
	if hex.EncodeToString(suma[:]) != a.SyntheticDataSHA256 {
		return nil, fmt.Errorf("ml: hash de datos sinteticos no coincide")
	}
	return &a, nil
}

// validate comprueba el contrato completo del artefacto. Se aplica en la carga y en
// las pruebas negativas (NaN, Inf, dimensiones, orden de features y versión).
func (a *Artifact) validate() error {
	if a == nil {
		return fmt.Errorf("ml: artefacto nil")
	}
	if a.SchemaVersion != ActiveSchemaVersion || a.ModelVersion != ActiveModelVersion ||
		a.Algorithm != "KMeans" || a.Seed != expectedSeed || a.NInit != expectedNInit ||
		a.K < 2 || a.K > 6 {
		return fmt.Errorf("ml: cabecera no conforme con ML-155 v2")
	}
	if a.MedianDefinition != MedianDefinitionTrained {
		return fmt.Errorf("ml: definicion de mediana no conforme: %q", a.MedianDefinition)
	}
	if err := a.AmountClip.validate(); err != nil {
		return err
	}
	if !finito(a.LogImporteMean) || !finito(a.LogImporteStd) || a.LogImporteStd <= 0 {
		return fmt.Errorf("ml: normalizacion log invalida")
	}
	if !finito(a.TrainedMedianCents) || a.TrainedMedianCents <= 0 ||
		a.TrainedMedianCents < a.AmountClip.LowerCents || a.TrainedMedianCents > a.AmountClip.UpperCents {
		return fmt.Errorf("ml: mediana de entrenamiento fuera del recorte")
	}
	if !finito(a.SilhouetteTraining) || a.SilhouetteTraining < 0 || a.SilhouetteTraining > 1 {
		return fmt.Errorf("ml: silueta invalida")
	}
	if !finito(a.Inertia) || a.Inertia < 0 {
		return fmt.Errorf("ml: inercia invalida")
	}
	if len(a.Categories) == 0 {
		return fmt.Errorf("ml: catalogo de categorias vacio")
	}
	vistas := make(map[string]struct{}, len(a.Categories))
	tieneOtros := false
	for _, categoria := range a.Categories {
		if categoria == "" {
			return fmt.Errorf("ml: categoria vacia")
		}
		if _, repetida := vistas[categoria]; repetida {
			return fmt.Errorf("ml: categoria duplicada %q", categoria)
		}
		vistas[categoria] = struct{}{}
		if categoria == "otros" {
			tieneOtros = true
		}
	}
	if !tieneOtros {
		return fmt.Errorf("ml: el artefacto debe definir la categoria otros")
	}

	dimension := len(a.FeatureOrder)
	if esperada := 5 + len(a.Categories) + 2; dimension != esperada {
		return fmt.Errorf("ml: dimension de features %d, esperada %d", dimension, esperada)
	}
	if !mismaSecuencia(a.FeatureOrder, nombresFeatures(a.Categories)) {
		return fmt.Errorf("ml: orden de features no conforme")
	}
	if len(a.Centroids) != a.K {
		return fmt.Errorf("ml: %d centroides para k=%d", len(a.Centroids), a.K)
	}
	for _, centroide := range a.Centroids {
		if len(centroide) != dimension {
			return fmt.Errorf("ml: centroide con dimension incorrecta")
		}
		for _, valor := range centroide {
			if !finito(valor) {
				return fmt.Errorf("ml: centroide no finito")
			}
		}
	}
	if len(a.SyntheticDataSHA256) != 64 {
		return fmt.Errorf("ml: hash de datos sinteticos invalido")
	}
	if _, err := hex.DecodeString(a.SyntheticDataSHA256); err != nil {
		return fmt.Errorf("ml: hash de datos sinteticos no hexadecimal")
	}
	return nil
}

func (c AmountClip) validate() error {
	if c.LowerPercentile != 0.5 || c.UpperPercentile != 99.5 {
		return fmt.Errorf("ml: percentiles de recorte no conformes")
	}
	if !finito(c.LowerCents) || !finito(c.UpperCents) || c.LowerCents <= 0 || c.UpperCents <= c.LowerCents {
		return fmt.Errorf("ml: limites de recorte invalidos")
	}
	return nil
}

func nombresFeatures(categorias []string) []string {
	nombres := make([]string, 0, 5+len(categorias)+2)
	nombres = append(nombres, "log_importe_z", "hora_sin", "hora_cos", "dia_sin", "dia_cos")
	for _, categoria := range categorias {
		nombres = append(nombres, "categoria_"+categoria)
	}
	return append(nombres, "es_recurrente", "ratio_sobre_mediana")
}

func mismaSecuencia(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func finito(valor float64) bool {
	return !math.IsNaN(valor) && !math.IsInf(valor, 0)
}

// ArtefactoBytes expone los bytes originales del artefacto activo para las pruebas
// de integridad del CI.
func ArtefactoBytes() ([]byte, error) { return files.ReadFile("modelo_kmeans_v2.json") }

// MetricasBytes expone el registro de métricas del entrenamiento v2.
func MetricasBytes() ([]byte, error) { return files.ReadFile("metricas_v2.json") }

// ReferenciaBytes expone las asignaciones y features calculados por el pipeline de
// Python para train y holdout. El test de inferencia las compara con Go.
func ReferenciaBytes() ([]byte, error) { return files.ReadFile("referencia_inferencia_v2.json") }

// DatosBytes expone el conjunto sintético usado en entrenamiento (v1, sin cambios).
func DatosBytes() ([]byte, error) { return files.ReadFile("datos_sinteticos_v1.json") }

// Version es el valor que debe viajar en `modelVersion`: el modelo realmente cargado.
func (a *Artifact) Version() string {
	if a == nil {
		return ""
	}
	return a.ModelVersion
}
