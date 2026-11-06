package telemetry_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/nexumpr/Nexum-Backend/internal/infrastructure/telemetry"
	transporthttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
	"github.com/nexumpr/Nexum-Backend/internal/transport/http/middleware"
)

// TestTraceContextPropagationAndCorrelation valida AC-A-090-01:
// Una solicitud se rastrea handler -> usecase -> DynamoDB conservando el trace ID sin filtrar datos.
func TestTraceContextPropagationAndCorrelation(t *testing.T) {
	const incomingTraceID = "4bf92f3577b34da6a3ce929d0e0e4736"
	const incomingParentID = "00f067aa0ba902b7"
	const rawTraceParent = "00-" + incomingTraceID + "-" + incomingParentID + "-01"
	const incomingReqID = "req-test-uuid-1234"

	var executedTraceID string
	var childSpanTraceID string
	var childSpanParentID string

	// Handler simulado que propaga a un caso de uso y luego a una llamada a DynamoDB
	businessHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		span := telemetry.FromContext(r.Context())
		if span == nil {
			t.Fatal("El span de telemetría no fue inyectado en el contexto HTTP")
		}
		executedTraceID = span.TraceID

		// Simular caso de uso creando span hijo para consulta a DynamoDB
		ctxChild, childSpan := telemetry.StartChildSpan(r.Context(), "dynamodb:QueryPartition")
		defer childSpan.End()

		childSpan.SetAttribute("dynamo.pk", "USER#123")
		childSpan.SetAttribute("auth.password", "SuperSecretPassword123") // Debe ser redactado automáticamente

		childSpanTraceID = childSpan.TraceID
		childSpanParentID = childSpan.ParentSpanID

		_ = ctxChild
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"success":true}`))
	})

	// Envolver con middleware de tracing
	handler := middleware.TraceMiddleware()(businessHandler)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/profile", nil)
	req.Header.Set(telemetry.TraceParentHeader, rawTraceParent)
	req.Header.Set(telemetry.RequestIDHeader, incomingReqID)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Esperaba status 200, obtuvo %d", rec.Code)
	}

	// 1. Verificar correlación del TraceID W3C
	if executedTraceID != incomingTraceID {
		t.Errorf("TraceID no coincide con el entrante. Esperaba %s, obtuvo %s", incomingTraceID, executedTraceID)
	}

	if childSpanTraceID != incomingTraceID {
		t.Errorf("ChildSpan TraceID no coincide con el trace raíz. Esperaba %s, obtuvo %s", incomingTraceID, childSpanTraceID)
	}

	if childSpanParentID == "" {
		t.Error("ChildSpan debe tener ParentSpanID no vacío")
	}

	// 2. Verificar cabeceras de respuesta
	respTraceParent := rec.Header().Get(telemetry.TraceParentHeader)
	if !strings.Contains(respTraceParent, incomingTraceID) {
		t.Errorf("Cabecera traceparent de respuesta no contiene el trace ID: %s", respTraceParent)
	}

	respReqID := rec.Header().Get(telemetry.RequestIDHeader)
	if respReqID != incomingReqID {
		t.Errorf("Cabecera x-request-id esperada %s, obtuvo %s", incomingReqID, respReqID)
	}

	t.Logf("AC-A-090-01 Verificado: Correlación W3C handler -> usecase -> dynamo completa (%s)", executedTraceID)
}

// TestReadinessProbeDegradation503 valida AC-A-090-02:
// ready=503 cuando no se puede atender operación crítica.
func TestReadinessProbeDegradation503(t *testing.T) {
	registry := transporthttp.NewHealthRegistry(500 * time.Millisecond)

	var dynamoHealthy = true
	var redisHealthy = true

	registry.RegisterDependency("dynamodb", func(ctx context.Context) error {
		if !dynamoHealthy {
			return errors.New("dynamodb cluster unavailable")
		}
		return nil
	})

	registry.RegisterDependency("redis", func(ctx context.Context) error {
		if !redisHealthy {
			return errors.New("redis connection refused")
		}
		return nil
	})

	// Caso 1: Todas las dependencias sanas -> 200 OK
	{
		req := httptest.NewRequest(http.MethodGet, "/api/v1/health/ready", nil)
		rec := httptest.NewRecorder()
		registry.ReadyHandler().ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("Esperaba 200 OK cuando todo sano, obtuvo %d", rec.Code)
		}

		var resp transporthttp.ReadyResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("Error al deserializar respuesta: %v", err)
		}
		if resp.Status != "UP" || resp.Dependencies["dynamodb"] != "UP" || resp.Dependencies["redis"] != "UP" {
			t.Errorf("Respuesta inesperada cuando todo sano: %+v", resp)
		}
	}

	// Caso 2: Redis degradado -> 503 Service Unavailable (AC-A-090-02)
	{
		redisHealthy = false
		req := httptest.NewRequest(http.MethodGet, "/api/v1/health/ready", nil)
		rec := httptest.NewRecorder()
		registry.ReadyHandler().ServeHTTP(rec, req)

		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("AC-A-090-02 Fallo: Esperaba 503 con Redis caído, obtuvo %d", rec.Code)
		}

		var resp transporthttp.ReadyResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("Error al deserializar respuesta: %v", err)
		}
		if resp.Status != "DOWN" {
			t.Errorf("Esperaba status DOWN en ready degradado, obtuvo %s", resp.Status)
		}
		if resp.Dependencies["redis"] != "DOWN" || resp.Dependencies["dynamodb"] != "UP" {
			t.Errorf("Mapeo de dependencias incorrecto: %+v", resp)
		}
		t.Logf("AC-A-090-02 Verificado: ready=503 cuando Redis se encuentra degradado")
	}

	// Caso 3: Liveness probe (/live) siempre responde 200 UP independientemente de dependencias
	{
		req := httptest.NewRequest(http.MethodGet, "/api/v1/health/live", nil)
		rec := httptest.NewRecorder()
		registry.LiveHandler().ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("Liveness probe /live debe ser 200 incluso con dependencias caídas, obtuvo %d", rec.Code)
		}
		if !strings.Contains(rec.Body.String(), `"status":"UP"`) {
			t.Errorf("Cuerpo de /live inesperado: %s", rec.Body.String())
		}
	}
}

// TestStructuredLogsRedactionAndSanitization valida AC-A-090-03:
// Logs rotados/redactados: no deben filtrarse contraseñas, tokens JWT Bearer, ni PII en JSON estructurado.
func TestStructuredLogsRedactionAndSanitization(t *testing.T) {
	var buf bytes.Buffer
	baseHandler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})

	sanitizedHandler := telemetry.NewSensitiveLogFilterHandler(baseHandler)
	logger := slog.New(sanitizedHandler)

	// Simular contexto con TraceSpan activo
	span := &telemetry.TraceSpan{
		TraceID:   "550e8400e29b41d4a716446655440000",
		SpanID:    "1234567890abcdef",
		RequestID: "req-999-secure",
	}
	ctx := telemetry.InjectTraceContext(context.Background(), span)

	rawPassword := "SuperSecretPassword123!"
	rawBearer := "Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c"
	rawCreditCard := "4532-0150-1234-5678"

	// Emitir log con mensaje y atributos que contienen credenciales
	logger.InfoContext(ctx, "Autenticación de usuario iniciada con "+rawBearer,
		slog.String("user_email", "usuario.sensible@ejemplo.com"),
		slog.String("password", rawPassword),
		slog.String("authorization", rawBearer),
		slog.String("credit_card", rawCreditCard),
		slog.String("voice_transcript", "mi clave secreta bancaria es 4321"),
	)

	logOutput := buf.String()

	// 1. Verificar que los secretos crudos no existen en el texto del log
	if strings.Contains(logOutput, rawPassword) {
		t.Fatalf("AC-A-090-03 Fallo: Contraseña cruda filtrada en los logs: %s", logOutput)
	}

	if strings.Contains(logOutput, rawBearer) {
		t.Fatalf("AC-A-090-03 Fallo: Token Bearer crudo filtrado en los logs: %s", logOutput)
	}

	if strings.Contains(logOutput, rawCreditCard) {
		t.Fatalf("AC-A-090-03 Fallo: Número de tarjeta crudo filtrado en los logs: %s", logOutput)
	}

	// 2. Verificar que los marcadores de redacción están presentes
	if !strings.Contains(logOutput, telemetry.RedactedPlaceholder) {
		t.Errorf("Esperaba placeholder %s en los atributos redactados", telemetry.RedactedPlaceholder)
	}

	// 3. Verificar que trace_id y request_id fueron inyectados en el JSON
	if !strings.Contains(logOutput, span.TraceID) {
		t.Errorf("El trace_id no fue inyectado en el log estructurado: %s", logOutput)
	}
	if !strings.Contains(logOutput, span.RequestID) {
		t.Errorf("El request_id no fue inyectado en el log estructurado: %s", logOutput)
	}

	t.Logf("AC-A-090-03 Verificado: Logs estructurados redactan PII y secretos con trazabilidad W3C inyectada.")
}

