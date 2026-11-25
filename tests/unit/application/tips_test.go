package application_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	appContent "github.com/nexumpr/Nexum-Backend/internal/application/content"
)

func TestContenidoTipsOchoEntradasReales(t *testing.T) {
	if appContent.Version != "2026-11-v2" {
		t.Fatalf("version inesperada: %q", appContent.Version)
	}
	if appContent.CacheKeyTips != "nexum:v1:content:tips:2026-11-v2" {
		t.Fatalf("clave de cache inesperada: %q", appContent.CacheKeyTips)
	}
	if appContent.CacheTTL != 300*time.Second {
		t.Fatalf("TTL de cache inesperado: %s", appContent.CacheTTL)
	}

	tips := appContent.Tips()
	if len(tips) != 8 {
		t.Fatalf("se esperaban 8 consejos, hay %d", len(tips))
	}
	vistos := make(map[string]bool, len(tips))
	for i, tip := range tips {
		esperado := fmt.Sprintf("tip-%02d", i+1)
		if tip.ID != esperado {
			t.Fatalf("identificador inestable en la posicion %d: %q", i, tip.ID)
		}
		if vistos[tip.ID] {
			t.Fatalf("identificador duplicado: %s", tip.ID)
		}
		vistos[tip.ID] = true
		if len([]rune(tip.Title)) < 10 {
			t.Fatalf("%s: titulo demasiado corto", tip.ID)
		}
		if len([]rune(tip.Text)) < 80 {
			t.Fatalf("%s: texto demasiado corto para ser util", tip.ID)
		}
		revisarTextoPublico(t, tip.ID, tip.Title+" "+tip.Text)
	}
}

func TestContenidoTipsSinRecomendacionesDeInversion(t *testing.T) {
	texto := strings.ToLower(concatenarTips(appContent.Tips()))
	for _, prohibido := range []string{"compra acciones", "invierte en", "criptomoneda", "rendimiento garantizado"} {
		if strings.Contains(texto, prohibido) {
			t.Fatalf("el contenido no debe recomendar productos o inversiones: %q", prohibido)
		}
	}
}

func concatenarTips(tips []appContent.Tip) string {
	var b strings.Builder
	for _, tip := range tips {
		b.WriteString(tip.Title)
		b.WriteString(" ")
		b.WriteString(tip.Text)
		b.WriteString(" ")
	}
	return b.String()
}
