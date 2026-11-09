package middleware

import (
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/nexumpr/Nexum-Backend/internal/infrastructure/telemetry"
)

var patronUUID = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// RequestLogOptions configura el registro canónico de peticiones (I-110, INF-110-01).
type RequestLogOptions struct {
	Logger        *slog.Logger
	Service       string
	Environment   string
	InstanceID    string
	Metrics       *telemetry.Metrics
	RouteResolver func(string) string
}

// RequestLogger emite un registro estructurado por petición y actualiza métricas.
// La ruta se sanea a plantilla: nunca se registran identificadores crudos.
func RequestLogger(opts RequestLogOptions) Middleware {
	service := opts.Service
	if service == "" {
		service = "nexum-backend"
	}
	environment := opts.Environment
	if environment == "" {
		environment = "local"
	}
	instance := opts.InstanceID
	if instance == "" {
		instance = "unknown"
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			inicio := time.Now()
			rec := &statusRecorder{ResponseWriter: w}
			next.ServeHTTP(rec, r)
			duracion := time.Since(inicio)

			ruta := "/unknown"
			if opts.RouteResolver != nil {
				if plantilla := opts.RouteResolver(r.URL.Path); plantilla != "" {
					ruta = plantilla
				}
			}
			status := rec.status
			if status == 0 {
				status = http.StatusOK
			}
			span := telemetry.FromContext(r.Context())
			traceID, requestID := "", ""
			if span != nil {
				traceID = span.TraceID
				requestID = span.RequestID
			}
			severity := slog.LevelInfo
			if status >= 500 {
				severity = slog.LevelError
			} else if status >= 400 {
				severity = slog.LevelWarn
			}
			if opts.Logger != nil {
				opts.Logger.Log(r.Context(), severity, "http_request",
					slog.String("timestamp", inicio.UTC().Format(time.RFC3339Nano)),
					slog.String("severity", strings.ToLower(severity.String())),
					slog.String("service", service),
					slog.String("environment", environment),
					slog.String("traceId", traceID),
					slog.String("requestId", requestID),
					slog.String("route", ruta),
					slog.Int("status", status),
					slog.Int64("latencyMs", duracion.Milliseconds()),
					slog.String("instanceId", instance),
				)
			}
			if opts.Metrics != nil {
				opts.Metrics.Observe(r.Method, ruta, status, duracion)
			}
		})
	}
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(codigo int) {
	if s.status == 0 {
		s.status = codigo
	}
	s.ResponseWriter.WriteHeader(codigo)
}

func (s *statusRecorder) Write(datos []byte) (int, error) {
	if s.status == 0 {
		s.status = http.StatusOK
	}
	return s.ResponseWriter.Write(datos)
}
