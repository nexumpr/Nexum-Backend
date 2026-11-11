package transport_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// El contrato E001 V6.0.2 sustituye intencionalmente al anterior (RFC-012). Esta guardian
// comprueba que el YAML y el JSON declaran lo mismo y que no reaparecen campos heredados.
func TestE001OpenAPIContractGuard(t *testing.T) {
	root := repoRoot(t)
	yamlPath := filepath.Join(root, "sdd", "03-endpoints", "openapi", "nexum-api-v1.yaml")
	jsonPath := filepath.Join(root, "sdd", "03-endpoints", "openapi", "nexum-api-v1.json")

	yamlData, err := os.ReadFile(yamlPath)
	if err != nil {
		t.Fatalf("no se pudo leer el OpenAPI YAML: %v", err)
	}
	yamlDoc := string(yamlData)

	requeridos := []string{
		"/api/v1/auth/register:",
		"operationId: nexum_e001",
		"- name: Idempotency-Key",
		"RegisterRequest:",
		"RegisterIncomeInput:",
		"RegisterRecurringExpenseInput:",
		"RegisterPrivacyInput:",
		"RegisterClientInput:",
		"RegisterResponse:",
		"RegisterUser:",
		"RegisterIncome:",
		"RegisterRecurringPayment:",
		"RegisterConsent:",
		"RegisterOnboarding:",
		"UserProfile:",
		"monthlyEquivalent:",
		"recurringPayments:",
		"acceptedByServer:",
		"'201':",
		"'400':",
		"'409':",
		"'422':",
		"'429':",
		"'503':",
		"ApiErrorDetail:",
	}
	for _, token := range requeridos {
		if !strings.Contains(yamlDoc, token) {
			t.Fatalf("el contrato E001 no contiene %q", token)
		}
	}

	requestBlock := sliceBetween(yamlDoc, "    RegisterRequest:", "    RegisterIncomeInput:")
	for _, campo := range []string{"name", "email", "password", "income", "recurringExpenses", "privacy"} {
		if !strings.Contains(requestBlock, "- "+campo) {
			t.Fatalf("RegisterRequest no declara %q como obligatorio", campo)
		}
	}
	// `client` es opcional, pero debe existir como propiedad del contrato.
	if !strings.Contains(requestBlock, "        client:") {
		t.Fatal("RegisterRequest no declara la propiedad opcional client")
	}
	for _, heredado := range []string{"displayName:", "privacyAccepted:", "privacyNoticeVersion:", "signupConfirm"} {
		if strings.Contains(requestBlock, heredado) {
			t.Fatalf("RegisterRequest no debe contener el campo heredado %q", heredado)
		}
	}

	responseBlock := sliceBetween(yamlDoc, "    RegisterResponse:", "    RegisterUser:")
	for _, campo := range []string{"- user", "- income", "- recurringPayments", "- consent", "- onboarding"} {
		if !strings.Contains(responseBlock, campo) {
			t.Fatalf("RegisterResponse no declara %q como obligatorio", campo)
		}
	}

	// Las siete frecuencias del prototipo, sin daily ni once.
	frequencyBlock := sliceBetween(yamlDoc, "    RegisterIncomeInput:", "    RegisterRecurringExpenseInput:")
	for _, frecuencia := range []string{"weekly", "fortnightly", "monthly", "bimonthly", "quarterly", "semiannual", "annual"} {
		if !strings.Contains(frequencyBlock, "- "+frecuencia) {
			t.Fatalf("el enum de frecuencias no contiene %q", frecuencia)
		}
	}
	for _, prohibida := range []string{"- daily", "- once"} {
		if strings.Contains(frequencyBlock, prohibida) {
			t.Fatalf("el enum de frecuencias no debe contener %q", prohibida)
		}
	}

	// El JSON debe reflejar exactamente lo mismo.
	var contrato map[string]any
	raw, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Fatalf("no se pudo leer el OpenAPI JSON: %v", err)
	}
	if err := json.Unmarshal(raw, &contrato); err != nil {
		t.Fatalf("el OpenAPI JSON no es valido: %v", err)
	}
	esquemas, ok := contrato["components"].(map[string]any)["schemas"].(map[string]any)
	if !ok {
		t.Fatal("el OpenAPI JSON no declara components.schemas")
	}
	for _, nombre := range []string{
		"RegisterRequest", "RegisterIncomeInput", "RegisterRecurringExpenseInput",
		"RegisterPrivacyInput", "RegisterClientInput", "RegisterResponse", "UserProfile",
	} {
		if _, existe := esquemas[nombre]; !existe {
			t.Fatalf("el OpenAPI JSON no declara el esquema %q", nombre)
		}
	}

	operacion := contrato["paths"].(map[string]any)["/api/v1/auth/register"].(map[string]any)["post"].(map[string]any)
	respuesta201 := operacion["responses"].(map[string]any)["201"].(map[string]any)
	ref := respuesta201["content"].(map[string]any)["application/json"].(map[string]any)["schema"].(map[string]any)["$ref"]
	if ref != "#/components/schemas/RegisterResponse" {
		t.Fatalf("la respuesta 201 debe referenciar RegisterResponse, referencia %q", ref)
	}
	cuerpo := operacion["requestBody"].(map[string]any)["content"].(map[string]any)["application/json"].(map[string]any)
	if refCuerpo := cuerpo["schema"].(map[string]any)["$ref"]; refCuerpo != "#/components/schemas/RegisterRequest" {
		t.Fatalf("el requestBody debe referenciar RegisterRequest, referencia %q", refCuerpo)
	}

	// `UserProfile` sigue siendo el contrato de E012 y no debe haberse alterado.
	profile, ok := esquemas["UserProfile"].(map[string]any)
	if !ok {
		t.Fatal("falta UserProfile")
	}
	requeridosProfile := profile["required"].([]any)
	if len(requeridosProfile) != 6 {
		t.Fatalf("UserProfile debe conservar sus seis campos requeridos, tiene %d", len(requeridosProfile))
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("no se pudo resolver la ruta del test")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
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
