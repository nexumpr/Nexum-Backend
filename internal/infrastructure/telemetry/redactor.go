package telemetry

import (
	"context"
	"log/slog"
	"regexp"
	"strings"
)

var (
	bearerRegex = regexp.MustCompile(`(?i)Bearer\s+[A-Za-z0-9\-_=]+\.[A-Za-z0-9\-_=]+\.?[A-Za-z0-9\-_.+/=]*`)
	jwtRegex    = regexp.MustCompile(`eyJ[A-Za-z0-9-_]+\.eyJ[A-Za-z0-9-_]+\.[A-Za-z0-9-_]+`)
	cardRegex   = regexp.MustCompile(`\b(?:\d[ -]*?){13,16}\b`)
	emailRegex  = regexp.MustCompile(`(?i)\b([a-zA-Z0-9_.+-])[a-zA-Z0-9_.+-]*@([a-zA-Z0-9-]+\.[a-zA-Z0-9-.]+)\b`)
)

const RedactedPlaceholder = "[REDACTED]"

// RedactSensitiveString enmascara tokens, JWTs, tarjetas y credenciales de cadenas de texto.
func RedactSensitiveString(input string) string {
	if input == "" {
		return ""
	}

	result := bearerRegex.ReplaceAllString(input, "Bearer [REDACTED_TOKEN]")
	result = jwtRegex.ReplaceAllString(result, "[REDACTED_JWT]")
	result = cardRegex.ReplaceAllString(result, "[REDACTED_CARD]")
	result = emailRegex.ReplaceAllString(result, "$1***@$2")

	return result
}

// SensitiveLogFilterHandler es un wrapper de slog.Handler que inspecciona y sanitiza
// atributos y mensajes antes de emitir el log en JSON.
type SensitiveLogFilterHandler struct {
	next slog.Handler
}

// NewSensitiveLogFilterHandler crea un handler decorador con redacción automática.
func NewSensitiveLogFilterHandler(next slog.Handler) *SensitiveLogFilterHandler {
	return &SensitiveLogFilterHandler{next: next}
}

func (h *SensitiveLogFilterHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

func (h *SensitiveLogFilterHandler) Handle(ctx context.Context, r slog.Record) error {
	// Sanitizar mensaje
	r.Message = RedactSensitiveString(r.Message)

	// Crear nuevo record con atributos filtrados
	sanitizedRecord := slog.NewRecord(r.Time, r.Level, r.Message, r.PC)

	// Inyectar trace ID si existe en contexto
	if span := FromContext(ctx); span != nil {
		sanitizedRecord.AddAttrs(
			slog.String("trace_id", span.TraceID),
			slog.String("span_id", span.SpanID),
			slog.String("request_id", span.RequestID),
		)
	}

	r.Attrs(func(a slog.Attr) bool {
		sanitizedRecord.AddAttrs(sanitizeAttr(a))
		return true
	})

	return h.next.Handle(ctx, sanitizedRecord)
}

func (h *SensitiveLogFilterHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	sanitized := make([]slog.Attr, len(attrs))
	for i, a := range attrs {
		sanitized[i] = sanitizeAttr(a)
	}
	return &SensitiveLogFilterHandler{next: h.next.WithAttrs(sanitized)}
}

func (h *SensitiveLogFilterHandler) WithGroup(name string) slog.Handler {
	return &SensitiveLogFilterHandler{next: h.next.WithGroup(name)}
}

func sanitizeAttr(a slog.Attr) slog.Attr {
	keyLower := strings.ToLower(a.Key)

	// Bloqueo total de campos ultra sensibles
	if strings.Contains(keyLower, "password") ||
		strings.Contains(keyLower, "token") ||
		strings.Contains(keyLower, "authorization") ||
		strings.Contains(keyLower, "jwt") ||
		strings.Contains(keyLower, "secret") ||
		strings.Contains(keyLower, "cvv") ||
		strings.Contains(keyLower, "pin") ||
		strings.Contains(keyLower, "voice_transcript") ||
		strings.Contains(keyLower, "private_key") {
		return slog.String(a.Key, RedactedPlaceholder)
	}

	// Si es string genérico, evaluar si contiene tokens o PII
	if a.Value.Kind() == slog.KindString {
		return slog.String(a.Key, RedactSensitiveString(a.Value.String()))
	}

	// Agrupaciones recursivas
	if a.Value.Kind() == slog.KindGroup {
		attrs := a.Value.Group()
		sanitized := make([]slog.Attr, len(attrs))
		for i, subAttr := range attrs {
			sanitized[i] = sanitizeAttr(subAttr)
		}
		return slog.Group(a.Key, anySlice(sanitized)...)
	}

	return a
}

func anySlice(attrs []slog.Attr) []any {
	result := make([]any, len(attrs))
	for i, a := range attrs {
		result[i] = a
	}
	return result
}
