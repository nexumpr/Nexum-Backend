package privacy

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
	"time"
)

const shaAvisoPropuesto = "68f1052a4bcab70a6d8803552dccc67d8b5a23c5b383b5480a83f31e408505c9"

func TestAvisoContenidoSHA(t *testing.T) {
	suma := sha256.Sum256([]byte(Contenido()))
	obtenido := hex.EncodeToString(suma[:])
	if obtenido != shaAvisoPropuesto {
		t.Fatalf("SHA-256 del aviso no coincide: %s", obtenido)
	}
	if len([]byte(Contenido())) != 6761 {
		t.Fatalf("bytes del aviso inesperados: %d", len([]byte(Contenido())))
	}
}

func TestParseEffectiveAt(t *testing.T) {
	fecha, err := ParseEffectiveAt("2026-11-18T00:00:00Z")
	if err != nil || fecha.IsZero() {
		t.Fatalf("fecha valida rechazada: %v %v", fecha, err)
	}
	for _, invalida := range []string{"", "2026-11-18", "ayer", "2026-11-17T23:59:59Z"} {
		if _, err := ParseEffectiveAt(invalida); !errors.Is(err, ErrFechaEfectivaNoAprobada) {
			t.Fatalf("fecha %q debia rechazarse, obtuvo %v", invalida, err)
		}
	}
}

func TestProveedorDesdeEntorno(t *testing.T) {
	getenv := func(clave string) string {
		if clave == EffectiveAtEnvVar {
			return "2026-11-19T12:00:00-06:00"
		}
		return ""
	}
	p, err := ProveedorDesdeEntorno(getenv)
	if err != nil {
		t.Fatalf("configuracion valida rechazada: %v", err)
	}
	if p.EffectiveAt.UTC().Format(time.RFC3339) != "2026-11-19T18:00:00Z" {
		t.Fatalf("fecha no normalizada a UTC: %s", p.EffectiveAt)
	}

	if _, err := ProveedorDesdeEntorno(func(string) string { return "" }); !errors.Is(err, ErrFechaEfectivaNoAprobada) {
		t.Fatalf("sin variable debe fallar cerrado, obtuvo %v", err)
	}
	if _, err := ProveedorDesdeEntorno(func(string) string { return "invalida" }); !errors.Is(err, ErrFechaEfectivaNoAprobada) {
		t.Fatalf("variable invalida debe fallar cerrado, obtuvo %v", err)
	}
}

func TestAvisoVigenteConFecha(t *testing.T) {
	fecha := time.Date(2026, 11, 18, 0, 0, 0, 0, time.UTC)
	aviso, err := AvisoVigente(fecha)
	if err != nil {
		t.Fatalf("con fecha valida no debe fallar: %v", err)
	}
	if aviso.Version != "2026-11-v2" || aviso.EffectiveAt.IsZero() {
		t.Fatalf("cabecera inesperada: %+v", aviso)
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
	if len([]byte(aviso.Content)) != 6761 {
		t.Fatalf("bytes del contenido inesperados: %d", len([]byte(aviso.Content)))
	}
}

func TestAvisoVigenteSinFechaFallaCerrado(t *testing.T) {
	if _, err := AvisoVigente(time.Time{}); !errors.Is(err, ErrFechaEfectivaNoAprobada) {
		t.Fatalf("sin fecha debe fallar cerrado, obtuvo %v", err)
	}
}
