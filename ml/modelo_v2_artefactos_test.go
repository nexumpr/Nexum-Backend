package ml

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"
)

// TestModeloV2ArtefactosVersionados fija los hashes de los artefactos publicados de
// `kmeans-v2` y demuestra que v1 no se sobrescribió. Es evidencia reproducible para
// el Gate ML-155; no acredita precisión predictiva ni rendimiento en AWS.
func TestModeloV2ArtefactosVersionados(t *testing.T) {
	casos := map[string]string{
		"modelo_kmeans_v2.json":         "9670c91be3480057361adefa297e6f248a7e83f80af0b0c9d55c6e112bff168c",
		"metricas_v2.json":              "dd40de5ef28f3c91fdd751a7b7708372485f8da8764f227dd82216193d367e2e",
		"referencia_inferencia_v2.json": "322f2d3787474e77313f1034e0afc7bdd1aa06b08a7c8e9df759b810ee788930",
		"modelo_kmeans_v1.json":         "1997370f1c148df63e126e477d1f931c565d2b1f9b54dd57dc1e48b1a16d48cf",
		"datos_sinteticos_v1.json":      "8d2bf29691a08eee6a1cd1230ad81b8454712424721e5e3e21bb9e9bd99918fd",
	}
	for archivo, esperado := range casos {
		datos, err := os.ReadFile(archivo)
		if err != nil {
			t.Fatalf("%s: %v", archivo, err)
		}
		suma := sha256.Sum256(datos)
		if obtenido := hex.EncodeToString(suma[:]); obtenido != esperado {
			t.Fatalf("%s: sha256=%s, esperado %s", archivo, obtenido, esperado)
		}
	}
}

// TestModeloV2MetricasAprobadas fija los valores que el Gate ML-155 aprobó sin
// reentrenar: 0.780848 es la silueta de selección de k y 0.775027 la silueta del
// holdout independiente. Ambas son descriptivas, no precisión predictiva.
func TestModeloV2MetricasAprobadas(t *testing.T) {
	datos, err := os.ReadFile("metricas_v2.json")
	if err != nil {
		t.Fatal(err)
	}
	var metricas struct {
		ModelVersion       string  `json:"model_version"`
		SelectedK          int     `json:"selected_k"`
		SelectedSilhouette float64 `json:"selected_silhouette"`
		ArtifactSHA256     string  `json:"artifact_sha256"`
		MedianDefinition   string  `json:"median_definition"`
		Holdout            struct {
			Samples    int     `json:"samples"`
			Silhouette float64 `json:"silhouette"`
		} `json:"holdout"`
	}
	if err := json.Unmarshal(datos, &metricas); err != nil {
		t.Fatalf("metricas_v2.json inválido: %v", err)
	}
	if metricas.ModelVersion != "kmeans-v2" || metricas.SelectedK != 2 {
		t.Fatalf("cabecera de métricas inesperada: %+v", metricas)
	}
	if metricas.SelectedSilhouette != 0.780848 {
		t.Fatalf("selected_silhouette=%v, esperado 0.780848", metricas.SelectedSilhouette)
	}
	if metricas.Holdout.Silhouette != 0.775027 || metricas.Holdout.Samples != 500 {
		t.Fatalf("holdout inesperado: %+v", metricas.Holdout)
	}
	if metricas.ArtifactSHA256 != "9670c91be3480057361adefa297e6f248a7e83f80af0b0c9d55c6e112bff168c" {
		t.Fatalf("artifact_sha256=%s", metricas.ArtifactSHA256)
	}
	if metricas.MedianDefinition != MedianDefinitionTrained {
		t.Fatalf("median_definition=%q", metricas.MedianDefinition)
	}
}
