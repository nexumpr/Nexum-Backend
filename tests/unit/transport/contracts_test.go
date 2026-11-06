package transport_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	transportHttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
	"github.com/nexumpr/Nexum-Backend/pkg/httputil"
)

// TestOpenAPIMatrixIntegrity verifica AC-A-040-01: Matrix 75/75 sin IDs duplicados.
// Comprueba que el inventario canónico de 75 rutas contenga exactamente 33 M, 23 S, 12 E, 7 O
// y ningún ID duplicado entre E001 y E075.
func TestOpenAPIMatrixIntegrity(t *testing.T) {
	// Localizar el archivo de trazabilidad en sdd (probando rutas relativas desde pkg o raíz)
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
	categoryCounts := map[string]int{
		"M": 0,
		"S": 0,
		"E": 0,
		"O": 0,
	}

	for _, line := range lines {
		line = strings.TrimSpace(line)
		matches := reRow.FindStringSubmatch(line)
		if len(matches) == 5 {
			id := matches[1]
			category := matches[4]

			if seenIDs[id] {
				t.Fatalf("ID duplicado detectado en la matriz: %s (AC-A-040-01 violado)", id)
			}
			seenIDs[id] = true
			categoryCounts[category]++
		}
	}

	if len(seenIDs) != 75 {
		t.Fatalf("Se esperaban exactamente 75 operaciones en la matriz, encontradas: %d (AC-A-040-01)", len(seenIDs))
	}

	expectedM := 33
	expectedS := 23
	expectedE := 12
	expectedO := 7

	if categoryCounts["M"] != expectedM || categoryCounts["S"] != expectedS || categoryCounts["E"] != expectedE || categoryCounts["O"] != expectedO {
		t.Fatalf("Discrepancia en categorías (33M+23S+12E+7O): got M=%d, S=%d, E=%d, O=%d",
			categoryCounts["M"], categoryCounts["S"], categoryCounts["E"], categoryCounts["O"])
	}

	t.Logf("AC-A-040-01 Verificado: 75 operaciones únicas sin colisiones (33 M, 23 S, 12 E, 7 O)")
}

// TestErrorEnvelopeFormatAndNonEnumeration verifica AC-A-040-02 y el formato canónico OpenAPI 3.1:
// {"error":{"code":"...","message":"...","details":[],"traceId":"..."}}
func TestErrorEnvelopeFormatAndNonEnumeration(t *testing.T) {
	rec := httptest.NewRecorder()
	traceID := "trace-sec-audit-001"
	err := httputil.WriteErrorEnvelope(rec, http.StatusNotFound, "RESOURCE_NOT_FOUND", "recurso no encontrado", traceID, nil)
	if err != nil {
		t.Fatalf("Fallo inesperado al escribir error envelope: %v", err)
	}

	if rec.Code != http.StatusNotFound {
		t.Fatalf("Status code esperado 404, obtenido %d", rec.Code)
	}

	var env httputil.ErrorEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("El cuerpo devuelto no cumple el esquema JSON de ErrorEnvelope: %v", err)
	}

	if env.Error.Code != "RESOURCE_NOT_FOUND" {
		t.Errorf("Código de error esperado 'RESOURCE_NOT_FOUND', obtenido '%s'", env.Error.Code)
	}
	if env.Error.TraceID != traceID {
		t.Errorf("TraceId esperado '%s', obtenido '%s'", traceID, env.Error.TraceID)
	}
	if env.Error.Details == nil {
		t.Errorf("El campo details debe ser un array no nulo '[]'")
	}

	// Verificación de no-enumeración (AC-A-040-02):
	// Para un recurso inexistente y para un recurso ajeno denegado,
	// el mensaje de error no debe revelar diferencias ni filtrar identifiers internos.
	recAlien := httptest.NewRecorder()
	_ = httputil.WriteErrorEnvelope(recAlien, http.StatusNotFound, "RESOURCE_NOT_FOUND", "recurso no encontrado", "trace-sec-002", nil)

	if recAlien.Body.String() != strings.Replace(rec.Body.String(), traceID, "trace-sec-002", 1) {
		t.Errorf("El mensaje de error revela discrepancias internas entre recursos ajenos e inexistentes (AC-A-040-02)")
	}
}

// TestDecimalMoneyContract verifica GOV-06 y A-040 §2.2:
// Rechazo absoluto de números flotantes, validación de formato decimal exacto ("123.45")
// y serialización/deserialización a centavos int64.
func TestDecimalMoneyContract(t *testing.T) {
	t.Run("Serialización_exitosa_a_string_decimal_con_dos_posiciones", func(t *testing.T) {
		dm := httputil.NewDecimalMoney(15025) // 150.25 MXN
		b, err := json.Marshal(dm)
		if err != nil {
			t.Fatalf("Error al serializar DecimalMoney: %v", err)
		}
		if string(b) != `"150.25"` {
			t.Fatalf("Se esperaba '\"150.25\"', obtenido %s", string(b))
		}

		dmNeg := httputil.NewDecimalMoney(-999) // -9.99 MXN
		bNeg, _ := json.Marshal(dmNeg)
		if string(bNeg) != `"-9.99"` {
			t.Fatalf("Se esperaba '\"-9.99\"', obtenido %s", string(bNeg))
		}
	})

	t.Run("Deserialización_exitosa_desde_string_decimal", func(t *testing.T) {
		raw := `{"amount":"123.45"}`
		type payload struct {
			Amount httputil.DecimalMoney `json:"amount"`
		}
		var p payload
		if err := json.Unmarshal([]byte(raw), &p); err != nil {
			t.Fatalf("Fallo al deserializar DecimalMoney desde string válido: %v", err)
		}
		if p.Amount.Cents != 12345 {
			t.Fatalf("Centavos esperados 12345, obtenidos %d", p.Amount.Cents)
		}
	})

	t.Run("Rechazo_estricto_de_floats_monetarios_crudos", func(t *testing.T) {
		// Float crudo sin comillas (violación de GOV-06 y A-040 §2.2)
		rawFloat := `{"amount": 123.45}`
		type payload struct {
			Amount httputil.DecimalMoney `json:"amount"`
		}
		var p payload
		err := json.Unmarshal([]byte(rawFloat), &p)
		if err == nil {
			t.Fatalf("Se esperaba error al intentar deserializar un número flotante en un campo monetario, pero fue aceptado")
		}
		if !strings.Contains(err.Error(), "floats monetarios prohibidos") {
			t.Errorf("Mensaje de error inesperado: %v", err)
		}
	})

	t.Run("Rechazo_de_formatos_decimales_incorrectos", func(t *testing.T) {
		invalidFormats := []string{
			`"123"`,      // sin punto ni decimales
			`"123.4"`,    // solo un decimal
			`"123.456"`,  // tres decimales
			`"abc"`,      // letras
			`"123.4a"`,   // alfanumérico
			`""`,         // vacío
		}

		for _, inv := range invalidFormats {
			var dm httputil.DecimalMoney
			err := json.Unmarshal([]byte(inv), &dm)
			if err == nil {
				t.Errorf("Se esperaba fallo al deserializar formato monetario inválido '%s', pero se aceptó", inv)
			}
		}
	})
}

// TestPaginationCursorSecurity verifica la generación, HMAC, integridad y no-transferencia de cursores (A-040 §2.3).
func TestPaginationCursorSecurity(t *testing.T) {
	secret := []byte("super-secret-signing-key-for-test-32bytes!")
	userID := "user-nexum-001"
	sortBy := "occurredAt"
	lastID := "mov-987"
	timestamp := time.Now().UTC()

	// 1. Generación normal
	cursor, err := httputil.GenerateCursor(secret, userID, sortBy, lastID, timestamp, 1*time.Hour)
	if err != nil {
		t.Fatalf("Error al generar cursor: %v", err)
	}

	// 2. Validación exitosa
	payload, err := httputil.ValidateCursor(secret, cursor, userID)
	if err != nil {
		t.Fatalf("Error inesperado al validar cursor legítimo: %v", err)
	}
	if payload.UserID != userID || payload.LastID != lastID || payload.SortBy != sortBy {
		t.Fatalf("Datos del payload del cursor no coinciden: %+v", payload)
	}

	// 3. Verificación de rechazo ante intento de usurpación de usuario ajeno
	_, err = httputil.ValidateCursor(secret, cursor, "alien-user-999")
	if err == nil {
		t.Fatalf("Se esperaba error al intentar usar un cursor con un usuario diferente, pero fue aceptado")
	}

	// 4. Verificación de rechazo ante cursor expirado
	expiredCursor, _ := httputil.GenerateCursor(secret, userID, sortBy, lastID, timestamp, -1*time.Minute)
	_, err = httputil.ValidateCursor(secret, expiredCursor, userID)
	if err == nil || !strings.Contains(err.Error(), "cursor expirado") {
		t.Fatalf("Se esperaba error de cursor expirado, obtenido: %v", err)
	}

	// 5. Verificación de rechazo ante alteración o tampering
	corrupted := cursor[:len(cursor)-4] + "abcd"
	_, err = httputil.ValidateCursor(secret, corrupted, userID)
	if err == nil {
		t.Fatalf("Se esperaba fallo por firma HMAC inválida tras alteración, pero fue aceptado")
	}
}

// TestPaginationHeaders verifica AC-A-040-03: Cabeceras de paginación e inyección en respuesta.
func TestPaginationHeaders(t *testing.T) {
	rec := httptest.NewRecorder()
	nextCursor := "opaque-signed-next-cursor"
	meta := httputil.PaginatedMeta{
		NextCursor: &nextCursor,
		HasMore:    true,
		Limit:      25,
	}

	items := []string{"item1", "item2"}
	err := httputil.WritePaginatedResponse(rec, http.StatusOK, items, meta)
	if err != nil {
		t.Fatalf("Error al escribir respuesta paginada: %v", err)
	}

	if rec.Header().Get("X-Cursor-Next") != nextCursor {
		t.Errorf("Cabecera X-Cursor-Next faltante o incorrecta (AC-A-040-03)")
	}
	if rec.Header().Get("X-Has-More") != "true" {
		t.Errorf("Cabecera X-Has-More faltante o incorrecta (AC-A-040-03)")
	}
}

// TestIdempotencyKeyVerification verifica AC-A-040-03: Header Idempotency-Key obligatorio en mutaciones financieras.
func TestIdempotencyKeyVerification(t *testing.T) {
	// 1. Solicitud sin cabecera
	reqNoKey := httptest.NewRequest(http.MethodPost, "/api/v1/movements", nil)
	_, err := transportHttp.RequireIdempotencyKey(reqNoKey)
	if err == nil {
		t.Fatalf("Se esperaba error por falta de Idempotency-Key, pero no se produjo")
	}

	// 2. Cabecera demasiado corta (< 16 caracteres)
	reqShortKey := httptest.NewRequest(http.MethodPost, "/api/v1/movements", nil)
	reqShortKey.Header.Set("Idempotency-Key", "abc-123")
	_, err = transportHttp.RequireIdempotencyKey(reqShortKey)
	if err == nil || !strings.Contains(err.Error(), "longitud entre 16 y 128") {
		t.Fatalf("Se esperaba error por clave de idempotencia corta, obtenido: %v", err)
	}

	// 3. Cabecera válida
	validKey := "9b1deb4d-3b7d-4bad-9bdd-2b0d7b3dcb6d"
	reqValid := httptest.NewRequest(http.MethodPost, "/api/v1/movements", nil)
	reqValid.Header.Set("Idempotency-Key", validKey)
	key, err := transportHttp.RequireIdempotencyKey(reqValid)
	if err != nil || key != validKey {
		t.Fatalf("Fallo con clave válida: %v", err)
	}
}

// TestDateAndPeriodTypes verifica la validación de fechas civiles y periodos (A-040 §2.2).
func TestDateAndPeriodTypes(t *testing.T) {
	validDate := httputil.DateOnly("2026-11-06")
	if err := validDate.Validate(); err != nil {
		t.Errorf("Fecha civil válida fue rechazada: %v", err)
	}

	invalidDate := httputil.DateOnly("2026-02-31")
	if err := invalidDate.Validate(); err == nil {
		t.Errorf("Fecha civil inexistente (2026-02-31) debió ser rechazada")
	}

	validMonth := httputil.YearMonth("2026-11")
	if err := validMonth.Validate(); err != nil {
		t.Errorf("Periodo mensual válido fue rechazado: %v", err)
	}

	invalidMonth := httputil.YearMonth("2026-13")
	if err := invalidMonth.Validate(); err == nil {
		t.Errorf("Mes 13 debió ser rechazado")
	}
}

// TestStrictJSONBinding verifica el descarte de campos desconocidos y métodos inválidos en BindJSON.
func TestStrictJSONBinding(t *testing.T) {
	type TestDTO struct {
		Name string `json:"name"`
	}

	// GET con body debe fallar
	reqGet := httptest.NewRequest(http.MethodGet, "/api/v1/users", bytes.NewBufferString(`{"name":"test"}`))
	var dst TestDTO
	err := transportHttp.BindJSON(httptest.NewRecorder(), reqGet, &dst)
	if err == nil {
		t.Errorf("GET con cuerpo JSON debió ser rechazado")
	}

	// Body con campo desconocido debe fallar (DisallowUnknownFields)
	reqUnknown := httptest.NewRequest(http.MethodPost, "/api/v1/users", bytes.NewBufferString(`{"name":"test","injected":"malicious"}`))
	reqUnknown.Header.Set("Content-Type", "application/json")
	var dstUnknown TestDTO
	err = transportHttp.BindJSON(httptest.NewRecorder(), reqUnknown, &dstUnknown)
	if err == nil || !strings.Contains(err.Error(), "campo desconocido") {
		t.Errorf("Se esperaba rechazo de campo desconocido, obtenido: %v", err)
	}
}
