package httputil_test

import (
	"encoding/json"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/nexumpr/Nexum-Backend/pkg/httputil"
)

var patronUUID = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// TestWriteErrorEnvelopeNormalizaTraceID cubre la certificación productiva: el contrato
// declara `traceId` con `format: uuid`, y el middleware propaga el trace-id W3C de 32 hex.
// El envelope debe presentarlo en forma uuid sin perder el valor.
func TestWriteErrorEnvelopeNormalizaTraceID(t *testing.T) {
	casos := map[string]string{
		"7c1196d32f6be5622bd91d35f876e4ac": "7c1196d3-2f6b-e562-2bd9-1d35f876e4ac",
		"018f90ef08e57a68b09f2b67a47c92f0": "018f90ef-08e5-7a68-b09f-2b67a47c92f0",
	}
	for entrada, esperado := range casos {
		rec := httptest.NewRecorder()
		if err := httputil.WriteErrorEnvelope(rec, 409, "CONFLICT", "conflicto", entrada, nil); err != nil {
			t.Fatalf("WriteErrorEnvelope: %v", err)
		}
		var cuerpo struct {
			Error struct {
				TraceID string `json:"traceId"`
			} `json:"error"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &cuerpo); err != nil {
			t.Fatalf("json invalido: %v", err)
		}
		if cuerpo.Error.TraceID != esperado {
			t.Fatalf("traceId=%q, esperado %q", cuerpo.Error.TraceID, esperado)
		}
		if !patronUUID.MatchString(cuerpo.Error.TraceID) {
			t.Fatalf("traceId no cumple format uuid: %q", cuerpo.Error.TraceID)
		}
	}
}

// TestWriteErrorEnvelopeConservaTraceIDNoHex fija que un identificador que no es 32 hex no se
// altera (por ejemplo un UUID ya formateado o un valor opaco del cliente).
func TestWriteErrorEnvelopeConservaTraceIDNoHex(t *testing.T) {
	for _, entrada := range []string{
		"018f90ef-08e5-7a68-b09f-2b67a47c92f0",
		"trace-opaco-del-cliente",
		"",
	} {
		rec := httptest.NewRecorder()
		if err := httputil.WriteErrorEnvelope(rec, 400, "INVALID_REQUEST", "invalido", entrada, nil); err != nil {
			t.Fatalf("WriteErrorEnvelope: %v", err)
		}
		var cuerpo struct {
			Error struct {
				TraceID string `json:"traceId"`
			} `json:"error"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &cuerpo); err != nil {
			t.Fatalf("json invalido: %v", err)
		}
		if cuerpo.Error.TraceID != entrada {
			t.Fatalf("traceId=%q, esperado %q", cuerpo.Error.TraceID, entrada)
		}
	}
}
