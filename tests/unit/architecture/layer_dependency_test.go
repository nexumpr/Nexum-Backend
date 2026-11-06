package architecture_test

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestDomainLayerImportRules valida automáticamente la regla de Clean Architecture (AC-A-020-01, A-020 §2.2):
// La capa Domain (internal/domain/...) es el núcleo puro del sistema y tiene estrictamente prohibido
// importar capas externas: adapters, transport, HTTP, o SDKs de infraestructura.
func TestDomainLayerImportRules(t *testing.T) {
	// Localizar la raíz del proyecto
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("error al obtener directorio de trabajo: %v", err)
	}

	// Buscar hacia arriba hasta encontrar el go.mod
	projectRoot := wd
	for {
		if _, err := os.Stat(filepath.Join(projectRoot, "go.mod")); err == nil {
			break
		}
		parent := filepath.Dir(projectRoot)
		if parent == projectRoot {
			t.Fatalf("no se encontro go.mod en la jerarquia de directorios")
		}
		projectRoot = parent
	}

	domainDir := filepath.Join(projectRoot, "internal", "domain")
	if _, err := os.Stat(domainDir); os.IsNotExist(err) {
		t.Fatalf("el directorio de dominio '%s' no existe", domainDir)
	}

	fset := token.NewFileSet()
	forbiddenImportPrefixes := []string{
		"net/http",
		"database/sql",
		"github.com/aws",
		"github.com/redis",
		"github.com/nexumpr/Nexum-Backend/internal/adapters",
		"github.com/nexumpr/Nexum-Backend/internal/transport",
		"github.com/nexumpr/Nexum-Backend/internal/platform",
		"github.com/nexumpr/Nexum-Backend/internal/application",
	}

	scannedFiles := 0

	err = filepath.Walk(domainDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(info.Name(), ".go") {
			return nil
		}

		scannedFiles++
		node, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			t.Errorf("error al parsear archivo '%s': %v", path, err)
			return nil
		}

		for _, imp := range node.Imports {
			importPath := strings.Trim(imp.Path.Value, `"`)
			for _, forbidden := range forbiddenImportPrefixes {
				if strings.HasPrefix(importPath, forbidden) {
					t.Errorf("VIOLACION DE CLEAN ARCHITECTURE (AC-A-020-01):\n  Archivo: %s\n  Import Ilegal: %s\n  Regla: Domain no puede depender de capas externas o infraestructura.",
						path, importPath)
				}
			}
		}

		return nil
	})

	if err != nil {
		t.Fatalf("error al recorrer el directorio de dominio: %v", err)
	}

	if scannedFiles == 0 {
		t.Fatal("no se encontraron archivos .go en internal/domain para auditar")
	}

	t.Logf("Auditoría de Clean Architecture Superada: %d archivos en internal/domain auditados con 0 violaciones de imports.", scannedFiles)
}

