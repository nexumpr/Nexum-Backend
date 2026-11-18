package privacy

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
	"time"
)

const shaAvisoAprobado = "ddfb0b1865e0e77a47f74c025e36bdbc102c6d424fc137c703f4316d69b66380"

func TestAvisoContenidoSHA(t *testing.T) {
	suma := sha256.Sum256([]byte(Contenido()))
	obtenido := hex.EncodeToString(suma[:])
	if obtenido != shaAvisoAprobado {
		t.Fatalf("SHA-256 del aviso no coincide: %s", obtenido)
	}
	if len([]byte(Contenido())) != 6724 {
		t.Fatalf("bytes del aviso inesperados: %d", len([]byte(Contenido())))
	}
}

func TestFechaEfectivaPorDefecto(t *testing.T) {
	if effectiveAtAviso != "2026-11-20T00:00:00Z" {
		t.Fatalf("fecha efectiva por defecto inesperada: %q", effectiveAtAviso)
	}
	if _, err := time.Parse(time.RFC3339, effectiveAtAviso); err != nil {
		t.Fatalf("la fecha efectiva debe ser RFC3339: %v", err)
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
	if !strings.HasPrefix(aviso.URL, "https://") || !strings.Contains(aviso.URL, "nexum-privacidad") {
		t.Fatalf("url inesperada: %s", aviso.URL)
	}
	if !aviso.RequiresAcceptance {
		t.Fatal("requiresAcceptance debe ser true")
	}
	if strings.TrimSpace(aviso.Content) == "" || strings.Contains(aviso.Content, "example.invalid") {
		t.Fatal("el contenido no puede estar vacío ni usar example.invalid")
	}
	if len([]byte(aviso.Content)) != 6724 {
		t.Fatalf("bytes del contenido inesperados: %d", len([]byte(aviso.Content)))
	}
}
