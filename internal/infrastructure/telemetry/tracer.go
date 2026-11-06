package telemetry

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type contextKey string

const (
	TraceParentHeader           = "traceparent"
	RequestIDHeader             = "x-request-id"
	traceContextKey  contextKey = "nexum-trace-context"
)

// TraceSpan encapsula el identificador de rastreo W3C y metadatos de span local.
type TraceSpan struct {
	TraceID      string            `json:"trace_id"`
	SpanID       string            `json:"span_id"`
	ParentSpanID string            `json:"parent_span_id,omitempty"`
	TraceFlags   string            `json:"trace_flags"`
	RequestID    string            `json:"request_id"`
	Name         string            `json:"name"`
	StartTime    time.Time         `json:"start_time"`
	EndTime      time.Time         `json:"end_time,omitempty"`
	Attributes   map[string]string `json:"attributes,omitempty"`
	Err          error             `json:"-"`
}

// ExtractOrGenerateTraceContext extrae la cabecera traceparent W3C o genera un nuevo contexto.
// Formato W3C: version (00) - trace_id (32 hex) - parent_id (16 hex) - trace_flags (02 hex)
func ExtractOrGenerateTraceContext(req *http.Request) *TraceSpan {
	rawTraceParent := req.Header.Get(TraceParentHeader)
	reqID := req.Header.Get(RequestIDHeader)
	if reqID == "" {
		reqID = generateRandomHex(16)
	}

	span := &TraceSpan{
		RequestID:  reqID,
		StartTime:  time.Now(),
		Attributes: make(map[string]string),
	}

	if rawTraceParent != "" {
		parts := strings.Split(rawTraceParent, "-")
		if len(parts) == 4 && len(parts[1]) == 32 && len(parts[2]) == 16 {
			span.TraceID = parts[1]
			span.ParentSpanID = parts[2]
			span.TraceFlags = parts[3]
			span.SpanID = generateRandomHex(8) // Nuevo span ID para el servidor local
			return span
		}
	}

	// Generar nuevo contexto W3C raíz
	span.TraceID = generateRandomHex(16) // 16 bytes = 32 caracteres hex
	span.SpanID = generateRandomHex(8)   // 8 bytes = 16 caracteres hex
	span.TraceFlags = "01"               // Grabado / muestreado
	return span
}

// InjectTraceContextContext injecta el TraceSpan en el context.Context.
func InjectTraceContext(ctx context.Context, span *TraceSpan) context.Context {
	return context.WithValue(ctx, traceContextKey, span)
}

// FromContext obtiene el TraceSpan del contexto o nil si no existe.
func FromContext(ctx context.Context) *TraceSpan {
	if ctx == nil {
		return nil
	}
	span, ok := ctx.Value(traceContextKey).(*TraceSpan)
	if !ok {
		return nil
	}
	return span
}

// StartChildSpan crea un span hijo manteniendo el mismo TraceID pero con nuevo SpanID.
func StartChildSpan(ctx context.Context, name string) (context.Context, *TraceSpan) {
	parent := FromContext(ctx)
	child := &TraceSpan{
		Name:       name,
		StartTime:  time.Now(),
		Attributes: make(map[string]string),
	}

	if parent != nil {
		child.TraceID = parent.TraceID
		child.ParentSpanID = parent.SpanID
		child.TraceFlags = parent.TraceFlags
		child.RequestID = parent.RequestID
	} else {
		child.TraceID = generateRandomHex(16)
		child.TraceFlags = "01"
		child.RequestID = generateRandomHex(16)
	}
	child.SpanID = generateRandomHex(8)

	return InjectTraceContext(ctx, child), child
}

// End finaliza el span registrando el tiempo de fin.
func (s *TraceSpan) End() {
	s.EndTime = time.Now()
}

// SetAttribute añade un metadato seguro al span, ignorando datos sensibles.
func (s *TraceSpan) SetAttribute(key, value string) {
	if s.Attributes == nil {
		s.Attributes = make(map[string]string)
	}
	s.Attributes[key] = RedactSensitiveString(value)
}

// W3CTraceparent devuelve la representación canónica W3C traceparent.
func (s *TraceSpan) W3CTraceparent() string {
	return fmt.Sprintf("00-%s-%s-%s", s.TraceID, s.SpanID, s.TraceFlags)
}

func generateRandomHex(byteCount int) string {
	b := make([]byte, byteCount)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

