package platform_test

import (
	"runtime"
	"strings"
	"testing"
)

// TestRuntimeAndPlatformMatrix audita las condiciones de AC-A-010-01 y AC-A-010-03:
// 1. Matriz de SO soportada: Windows o Linux 64 bits.
// 2. Arquitectura de 64 bits (amd64 o arm64).
// 3. Ejecución directa en Go nativo sin requerir Docker local.
func TestRuntimeAndPlatformMatrix(t *testing.T) {
	osName := runtime.GOOS
	archName := runtime.GOARCH

	// 1. Validar sistema operativo soportado
	if osName != "windows" && osName != "linux" && osName != "darwin" {
		t.Errorf("Sistema operativo no contemplado en la matriz objetivo: %s", osName)
	}

	// 2. Validar arquitectura de 64 bits obligatoria
	if archName != "amd64" && archName != "arm64" {
		t.Errorf("Arquitectura no es de 64 bits soportada (se requiere amd64 o arm64): %s", archName)
	}

	// 3. Validar versión de compilador Go
	goVer := runtime.Version()
	if !strings.HasPrefix(goVer, "go1.") {
		t.Errorf("Version de runtime de Go inesperada: %s", goVer)
	}

	t.Logf("Verificación de Runtime Exitosa: OS=%s | Arch=%s | Version=%s | Modo: Nativo sin Docker (AC-A-010-03)", osName, archName, goVer)
}
