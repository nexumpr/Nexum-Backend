package platform_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"github.com/nexumpr/Nexum-Backend/pkg/telemetry"
)

func TestJSONLoggerSanitization(t *testing.T) {
	var buf bytes.Buffer
	logger := telemetry.NewJSONLogger(&buf, slog.LevelInfo)

	logger.Info("evento de prueba",
		slog.String("user_id", "usr-123"),
		slog.String("password", "SuperSecretPass!"),
		slog.String("token", "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.xyz"),
		slog.String("authorization", "Bearer eyJ..."),
		slog.String("credit_card", "4111222233334444"),
		slog.String("normal_field", "valor_publico"),
	)

	var entry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("el logger no produjo un JSON valido: %v | Raw: %s", err, buf.String())
	}

	if entry["user_id"] != "usr-123" {
		t.Errorf("campo 'user_id' corrupto: %v", entry["user_id"])
	}
	if entry["normal_field"] != "valor_publico" {
		t.Errorf("campo 'normal_field' corrupto: %v", entry["normal_field"])
	}

	// Campos sensibles deben haber sido redactados
	sensitiveFields := []string{"password", "token", "authorization", "credit_card"}
	for _, field := range sensitiveFields {
		val, ok := entry[field].(string)
		if !ok {
			t.Errorf("el campo sensible '%s' no esta presente", field)
			continue
		}
		if val != telemetry.RedactedMask {
			t.Errorf("el campo sensible '%s' no fue redactado! Valor expuesto: %s", field, val)
		}
	}
}

func TestContextTracingAttributes(t *testing.T) {
	ctx := context.Background()
	ctx = telemetry.WithRequestID(ctx, "req-abc-001")
	ctx = telemetry.WithCorrelationID(ctx, "corr-xyz-999")

	if reqID := telemetry.RequestIDFromContext(ctx); reqID != "req-abc-001" {
		t.Errorf("request_id esperado 'req-abc-001', obtenido '%s'", reqID)
	}
	if corrID := telemetry.CorrelationIDFromContext(ctx); corrID != "corr-xyz-999" {
		t.Errorf("correlation_id esperado 'corr-xyz-999', obtenido '%s'", corrID)
	}

	var buf bytes.Buffer
	logger := telemetry.NewJSONLogger(&buf, slog.LevelInfo)

	attrs := telemetry.ContextAttrs(ctx)
	logger.LogAttrs(ctx, slog.LevelInfo, "solicitud procesada", attrs...)

	logStr := buf.String()
	if !strings.Contains(logStr, "req-abc-001") {
		t.Error("el log no contiene el request_id del contexto")
	}
	if !strings.Contains(logStr, "corr-xyz-999") {
		t.Error("el log no contiene el correlation_id del contexto")
	}
}

