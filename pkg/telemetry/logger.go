package telemetry

import (
	"context"
	"io"
	"log/slog"
	"strings"
)

type contextKey string

const (
	RequestIDKey     contextKey = "request_id"
	CorrelationIDKey contextKey = "correlation_id"
)

// sensitiveKeyWords define las claves cuyos valores deben ser redactados en los logs (GOV-10, CTX-007).
var sensitiveKeyWords = []string{
	"password",
	"passwd",
	"token",
	"jwt",
	"secret",
	"authorization",
	"credit_card",
	"card_number",
	"refresh_token",
	"api_key",
	"access_token",
	"cookie",
}

// RedactedMask es la constante utilizada para sustituir información sensible.
const RedactedMask = "[REDACTED]"

// isSensitiveKey determina si el nombre de una clave corresponde a un dato sensible.
func isSensitiveKey(key string) bool {
	lower := strings.ToLower(key)
	for _, kw := range sensitiveKeyWords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

// sanitizeAttr sanitiza atributos de log eliminando credenciales, tokens y datos sensibles.
func sanitizeAttr(groups []string, a slog.Attr) slog.Attr {
	if isSensitiveKey(a.Key) {
		return slog.String(a.Key, RedactedMask)
	}

	// Si el valor es un grupo de atributos, sanitizar recursivamente
	if a.Value.Kind() == slog.KindGroup {
		attrs := a.Value.Group()
		sanitizedGroup := make([]slog.Attr, len(attrs))
		for i, subAttr := range attrs {
			sanitizedGroup[i] = sanitizeAttr(append(groups, a.Key), subAttr)
		}
		return slog.Group(a.Key, sanitizedGroupToAny(sanitizedGroup)...)
	}

	return a
}

func sanitizedGroupToAny(attrs []slog.Attr) []any {
	args := make([]any, len(attrs))
	for i, a := range attrs {
		args[i] = a
	}
	return args
}

// NewJSONLogger crea un slog.Logger estructurado en JSON con sanitización automática anti-PII y secretos.
func NewJSONLogger(w io.Writer, level slog.Level) *slog.Logger {
	handlerOpts := &slog.HandlerOptions{
		Level:       level,
		ReplaceAttr: sanitizeAttr,
	}
	return slog.New(slog.NewJSONHandler(w, handlerOpts))
}

// WithRequestID agrega el identificador de solicitud al contexto.
func WithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, RequestIDKey, requestID)
}

// RequestIDFromContext recupera el identificador de solicitud del contexto si existe.
func RequestIDFromContext(ctx context.Context) string {
	if reqID, ok := ctx.Value(RequestIDKey).(string); ok {
		return reqID
	}
	return ""
}

// WithCorrelationID agrega el identificador de correlación al contexto.
func WithCorrelationID(ctx context.Context, correlationID string) context.Context {
	return context.WithValue(ctx, CorrelationIDKey, correlationID)
}

// CorrelationIDFromContext recupera el correlation_id si existe.
func CorrelationIDFromContext(ctx context.Context) string {
	if corrID, ok := ctx.Value(CorrelationIDKey).(string); ok {
		return corrID
	}
	return ""
}

// ContextAttrs extrae los atributos de trazabilidad del contexto para adjuntar al logger.
func ContextAttrs(ctx context.Context) []slog.Attr {
	var attrs []slog.Attr
	if reqID := RequestIDFromContext(ctx); reqID != "" {
		attrs = append(attrs, slog.String("request_id", reqID))
	}
	if corrID := CorrelationIDFromContext(ctx); corrID != "" {
		attrs = append(attrs, slog.String("correlation_id", corrID))
	}
	return attrs
}

