package transport_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestE001OpenAPIContractGuard(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("no se pudo resolver la ruta del test")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
	path := filepath.Join(root, "sdd", "03-endpoints", "openapi", "nexum-api-v1.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("no se pudo leer el OpenAPI: %v", err)
	}
	doc := string(data)

	required := []string{
		"/api/v1/auth/register:",
		"operationId: nexum_e001",
		"- name: Idempotency-Key",
		"RegisterRequest:",
		"UserProfile:",
		"emailVerified:",
		"privacyNoticeVersion:",
		"privacyAccepted:",
		"'201':",
		"'400':",
		"'409':",
		"'422':",
		"'429':",
		"'503':",
		"ApiErrorDetail:",
		"reason:",
	}
	for _, token := range required {
		if !strings.Contains(doc, token) {
			t.Fatalf("el contrato E001 no contiene %q", token)
		}
	}

	// Los cinco campos del request deben seguir siendo obligatorios.
	requestBlock := sliceBetween(doc, "    RegisterRequest:", "    LoginRequest:")
	for _, field := range []string{"- email", "- password", "- displayName", "- privacyNoticeVersion", "- privacyAccepted"} {
		if !strings.Contains(requestBlock, field) {
			t.Fatalf("RegisterRequest no declara %q como requerido", field)
		}
	}

	// UserProfile debe conservar los seis campos de respuesta.
	profileBlock := sliceBetween(doc, "    UserProfile:", "    ProfilePatch:")
	for _, field := range []string{"- id", "- email", "- displayName", "- emailVerified", "- createdAt", "- version"} {
		if !strings.Contains(profileBlock, field) {
			t.Fatalf("UserProfile no declara %q como requerido", field)
		}
	}
}

func sliceBetween(doc, start, end string) string {
	from := strings.Index(doc, start)
	if from < 0 {
		return ""
	}
	rest := doc[from+len(start):]
	to := strings.Index(rest, end)
	if to < 0 {
		return rest
	}
	return rest[:to]
}
