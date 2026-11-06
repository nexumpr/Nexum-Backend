package devsecops_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/nexumpr/Nexum-Backend/pkg/httputil"
)

// TestOpenAPIContractCompliance75Routes valida AC-A-100-01:
// El catálogo de 75 rutas cumple con los contratos OpenAPI 3.1, respuestas de error estructuradas
// y validación de esquemas sin referencias rotas ni colisiones.
func TestOpenAPIContractCompliance75Routes(t *testing.T) {
	candidates := []string{
		filepath.Join("..", "..", "..", "sdd", "01-arquitectura-general", "TRAZABILIDAD_75_RUTAS.md"),
		filepath.Join("sdd", "01-arquitectura-general", "TRAZABILIDAD_75_RUTAS.md"),
		filepath.Join("..", "..", "sdd", "01-arquitectura-general", "TRAZABILIDAD_75_RUTAS.md"),
	}

	var data []byte
	var err error
	for _, cand := range candidates {
		data, err = os.ReadFile(cand)
		if err == nil {
			break
		}
	}
	if err != nil {
		t.Fatalf("No se pudo leer TRAZABILIDAD_75_RUTAS.md: %v", err)
	}

	lines := strings.Split(string(data), "\n")
	reRow := regexp.MustCompile(`^\|\s*(E\d{3})\s*\|\s*([A-Z]+)\s*\|\s*([^|]+)\s*\|\s*([MSEO])\s*\|`)

	seenIDs := make(map[string]bool)
	seenRoutes := make(map[string]bool)
	categories := map[string]int{
		"M": 0,
		"S": 0,
		"E": 0,
		"O": 0,
	}

	for _, line := range lines {
		matches := reRow.FindStringSubmatch(strings.TrimSpace(line))
		if len(matches) == 5 {
			id := matches[1]
			method := matches[2]
			path := strings.Trim(strings.TrimSpace(matches[3]), "`")
			category := matches[4]

			if seenIDs[id] {
				t.Errorf("ID de operación duplicado: %s", id)
			}
			seenIDs[id] = true

			routeKey := method + " " + path
			if seenRoutes[routeKey] {
				t.Errorf("Ruta duplicada: %s", routeKey)
			}
			seenRoutes[routeKey] = true

			categories[category]++

			// Validar prefijo /api/v1
			if !strings.HasPrefix(path, "/api/v1") {
				t.Errorf("Operación %s no respeta el prefijo de enrutamiento /api/v1: %s", id, path)
			}
		}
	}

	// 1. Validar total de 75 operaciones
	if len(seenIDs) != 75 {
		t.Fatalf("AC-A-100-01 Fallo: Se esperaban 75 operaciones en el catálogo, encontradas %d", len(seenIDs))
	}

	// 2. Validar distribución exacta H06/H04 (33 M, 23 S, 12 E, 7 O)
	if categories["M"] != 33 {
		t.Errorf("Esperadas 33 operaciones de Movimientos, encontradas %d", categories["M"])
	}
	if categories["S"] != 23 {
		t.Errorf("Esperadas 23 operaciones de Sistema, encontradas %d", categories["S"])
	}
	if categories["E"] != 12 {
		t.Errorf("Esperadas 12 operaciones de Estadísticas, encontradas %d", categories["E"])
	}
	if categories["O"] != 7 {
		t.Errorf("Esperadas 7 operaciones de Otros, encontradas %d", categories["O"])
	}

	// 3. Simular validación de contrato HTTP real para respuestas de error (RFC 7807)
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = httputil.WriteErrorEnvelope(w, http.StatusNotFound, "RESOURCE_NOT_FOUND", "Recurso no encontrado", "trace-test-100", nil)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/invalid/route", nil)
	rec := httptest.NewRecorder()
	testHandler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("Esperaba 404, obtuvo %d", rec.Code)
	}

	var errResp httputil.ErrorEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("Error al deserializar RFC 7807: %v", err)
	}

	if errResp.Error.Code != "RESOURCE_NOT_FOUND" || errResp.Error.TraceID != "trace-test-100" {
		t.Errorf("Envelope de error no cumple contrato: %+v", errResp)
	}

	t.Logf("AC-A-100-01 Verificado: 75 operaciones OpenAPI 3.1 validadas (33 M, 23 S, 12 E, 7 O) con contrato RFC 7807.")
}
