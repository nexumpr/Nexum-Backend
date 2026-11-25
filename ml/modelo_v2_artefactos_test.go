package ml

import (
	"crypto/sha256"
	"encoding/hex"
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
