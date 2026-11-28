package privacy

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"testing"
)

// avisoV3 es el artefacto recuperado de la propuesta preservada
// (sdd/_control/H08/AVISO_PRIVACIDAD_2026_11_V3_PROPUESTA.md, sección 2).
// No se publica ni se activa: E015 sigue sirviendo 2026-11-v2 hasta el Gate legal.
//
//go:embed aviso_2026_11_v3.txt
var avisoV3 []byte

func TestAvisoV3Integridad(t *testing.T) {
	const (
		bytesEsperados = 13662
		shaEsperado    = "1cec4609ff2f3fc052d3deb29271ba8aaed322d1ec97ae9757e222cc8c0563b9"
	)
	if len(avisoV3) != bytesEsperados {
		t.Fatalf("tamaño esperado %d, obtenido %d", bytesEsperados, len(avisoV3))
	}
	suma := sha256.Sum256(avisoV3)
	if hex.EncodeToString(suma[:]) != shaEsperado {
		t.Fatalf("SHA-256 esperado %s, obtenido %x", shaEsperado, suma)
	}
}
