// Package ml carga y verifica el artefacto K-Means de ML-155 y ejecuta inferencia nativa en Go.
// No hay Python, AWS ni llamadas a proveedores en la ruta de petición.
package ml

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
)

// Los tres archivos se incrustan: el binario no depende de rutas del filesystem y el
// hash del conjunto sintético se verifica en cada carga.
//
//go:embed modelo_kmeans_v1.json datos_sinteticos_v1.json metricas_v1.json referencia_inferencia_v1.json
var files embed.FS

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
	artifacto    *Artifact
	cargaError   error
)

// Load devuelve el artefacto verificado. Es thread-safe y se ejecuta una sola vez.
func Load() (*Artifact, error) {
	cargarUnaVez.Do(func() {
		artifacto, cargaError = load()
	})
	return artifacto, cargaError
}

func load() (*Artifact, error) {
	crudo, err := files.ReadFile("modelo_kmeans_v1.json")
	if err != nil {
		return nil, fmt.Errorf("ml: artefacto no disponible: %w", err)
	}
	var a Artifact
	if err := json.Unmarshal(crudo, &a); err != nil {
		return nil, fmt.Errorf("ml: artefacto invalido: %w", err)
	}
	if a.SchemaVersion != 1 || a.ModelVersion == "" || a.Algorithm != "KMeans" ||
		a.Seed != 20261116 || a.NInit != 10 || a.K < 2 || a.K > 6 ||
		a.LogImporteStd <= 0 || len(a.Centroids) != a.K || len(a.FeatureOrder) == 0 ||
		len(a.Categories) == 0 {
		return nil, fmt.Errorf("ml: artefacto no conforme con ML-155")
	}
	dimension := len(a.FeatureOrder)
	if expected := 5 + len(a.Categories) + 2; dimension != expected {
		return nil, fmt.Errorf("ml: dimension de features %d, esperada %d", dimension, expected)
	}
	for _, centroide := range a.Centroids {
		if len(centroide) != dimension {
			return nil, fmt.Errorf("ml: centroide con dimension incorrecta")
		}
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

// ArtefactoBytes expone los bytes originales para las pruebas de integridad del CI.
func ArtefactoBytes() ([]byte, error) { return files.ReadFile("modelo_kmeans_v1.json") }

// MetricasBytes expone el registro de métricas del entrenamiento.
func MetricasBytes() ([]byte, error) { return files.ReadFile("metricas_v1.json") }

// ReferenciaBytes expone las asignaciones calculadas por el pipeline de Python. El test
// de inferencia las compara con la implementación Go para detectar desviaciones numéricas.
func ReferenciaBytes() ([]byte, error) { return files.ReadFile("referencia_inferencia_v1.json") }

// Version es el valor que debe viajar en `modelVersion`: el modelo realmente cargado.
func (a *Artifact) Version() string {
	if a == nil {
		return ""
	}
	return a.ModelVersion
}
