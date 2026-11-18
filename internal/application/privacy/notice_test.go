package privacy

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
)

const shaAvisoAprobado = "29b27611c6127e85ec3aca7601e38dec1fc188f305ffee6109d71e9cf1218a06"

func TestAvisoContenidoSHA(t *testing.T) {
	suma := sha256.Sum256([]byte(Contenido()))
	obtenido := hex.EncodeToString(suma[:])
	if obtenido != shaAvisoAprobado {
		t.Fatalf("SHA-256 del aviso no coincide: %s", obtenido)
	}
	if len([]byte(Contenido())) != 6158 {
		t.Fatalf("bytes del aviso inesperados: %d", len([]byte(Contenido())))
	}
}

func TestAvisoVigenteSinFechaFallaCerrado(t *testing.T) {
	anterior := effectiveAtAviso
	effectiveAtAviso = ""
	t.Cleanup(func() { effectiveAtAviso = anterior })

	if _, err := AvisoVigente(); !errors.Is(err, ErrFechaEfectivaNoAprobada) {
		t.Fatalf("sin fecha aprobada debe fallar cerrado, obtuvo %v", err)
	}
}

func TestAvisoVigenteConFechaAprobada(t *testing.T) {
	anterior := effectiveAtAviso
	effectiveAtAviso = "2026-11-20T00:00:00Z"
	t.Cleanup(func() { effectiveAtAviso = anterior })

	aviso, err := AvisoVigente()
	if err != nil {
		t.Fatalf("con fecha aprobada no debe fallar: %v", err)
	}
	if aviso.Version != "2026-11-v2" {
		t.Fatalf("version inesperada: %s", aviso.Version)
	}
	if aviso.EffectiveAt.IsZero() {
		t.Fatal("effectiveAt no puede ser cero")
	}
	if !strings.HasPrefix(aviso.URL, "https://") || !strings.Contains(aviso.URL, "/api/v1/privacy/notices/current") {
		t.Fatalf("url inesperada: %s", aviso.URL)
	}
	if !aviso.RequiresAcceptance {
		t.Fatal("requiresAcceptance debe ser true")
	}
	if strings.TrimSpace(aviso.Content) == "" || strings.Contains(aviso.Content, "example.invalid") {
		t.Fatal("el contenido no puede estar vacío ni usar example.invalid")
	}
	if len([]byte(aviso.Content)) != 6158 {
		t.Fatalf("bytes del contenido inesperados: %d", len([]byte(aviso.Content)))
	}
}
