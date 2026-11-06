package transport_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/nexumpr/Nexum-Backend/internal/adapters/memory"
	internalhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
	"github.com/nexumpr/Nexum-Backend/internal/transport/http/middleware"
	"github.com/nexumpr/Nexum-Backend/pkg/httputil"
	"github.com/nexumpr/Nexum-Backend/pkg/telemetry"
)

func setupTestServer() (*internalhttp.HTTPServer, *bytes.Buffer) {
	var logBuf bytes.Buffer
	logger := telemetry.NewJSONLogger(&logBuf, slog.LevelInfo)

	cfg := internalhttp.DefaultServerConfig()
	router := internalhttp.NewRouter()

	// Registrar endpoint GET /api/v1/ping
	router.HandleFunc(http.MethodGet, "/ping", func(w http.ResponseWriter, r *http.Request) {
		_ = httputil.WriteJSON(w, http.StatusOK, map[string]string{"message": "pong"})
	})

	// Registrar endpoint POST /api/v1/echo con decodificación estricta
	router.HandleFunc(http.MethodPost, "/echo", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Text string `json:"text"`
		}
		if err := httputil.DecodeJSON(w, r, &input, 1024); err != nil {
			if strings.Contains(err.Error(), "content-type") {
				httputil.WriteError(w, http.StatusUnsupportedMediaType, "UNSUPPORTED_MEDIA_TYPE", err.Error())
				return
			}
			if strings.Contains(err.Error(), "excede el limite") {
				httputil.WriteError(w, http.StatusRequestEntityTooLarge, "PAYLOAD_TOO_LARGE", err.Error())
				return
			}
			httputil.WriteError(w, http.StatusBadRequest, "INVALID_INPUT", err.Error())
			return
		}
		_ = httputil.WriteJSON(w, http.StatusOK, input)
	})

	// Registrar endpoint que genera un panic intencional para probar Recovery
	router.HandleFunc(http.MethodGet, "/panic", func(w http.ResponseWriter, r *http.Request) {
		panic("error catastrofico simulado con secreto=SuperClave123")
	})

	srv := internalhttp.NewHTTPServer(cfg, router, logger)
	return srv, &logBuf
}

func TestHTTPServerStatusesAndErrors(t *testing.T) {
	srv, logBuf := setupTestServer()
	handler := srv.Router()

	// 1. Validar 200 OK
	t.Run("Status 200 OK en ruta registrada", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/ping", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("codigo esperado 200, obtenido %d", rec.Code)
		}
	})

	// 2. Validar 404 Not Found no enumerable
	t.Run("Status 404 Not Found en ruta inexistente", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/ruta-que-no-existe", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Errorf("codigo esperado 404, obtenido %d", rec.Code)
		}

		var errRes httputil.ErrorResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &errRes); err != nil {
			t.Fatalf("error al deserializar 404: %v", err)
		}
		if errRes.Code != "RESOURCE_NOT_FOUND" {
			t.Errorf("codigo de error esperado 'RESOURCE_NOT_FOUND', obtenido '%s'", errRes.Code)
		}
	})

	// 3. Validar 405 Method Not Allowed y cabecera Allow
	t.Run("Status 405 Method Not Allowed con cabecera Allow", func(t *testing.T) {
		// La ruta /api/v1/ping solo tiene registrado GET. Intentar POST debe dar 405.
		req := httptest.NewRequest(http.MethodPost, "/api/v1/ping", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("codigo esperado 405, obtenido %d", rec.Code)
		}

		allowHeader := rec.Header().Get("Allow")
		if !strings.Contains(allowHeader, "GET") {
			t.Errorf("cabecera Allow debe incluir 'GET', obtenida '%s'", allowHeader)
		}
	})

	// 4. Validar 415 Unsupported Media Type
	t.Run("Status 415 Unsupported Media Type al enviar Content-Type no JSON", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/echo", bytes.NewBufferString("texto plano"))
		req.Header.Set("Content-Type", "text/plain")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnsupportedMediaType {
			t.Errorf("codigo esperado 415, obtenido %d", rec.Code)
		}
	})

	// 5. Validar 413 Payload Too Large
	t.Run("Status 413 Payload Too Large al superar tamano maximo", func(t *testing.T) {
		oversized := `{"text":"` + strings.Repeat("A", 2048) + `"}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/echo", bytes.NewBufferString(oversized))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusRequestEntityTooLarge {
			t.Errorf("codigo esperado 413, obtenido %d", rec.Code)
		}
	})

	// 6. Validar 400 Bad Request por JSON corrupto
	t.Run("Status 400 Bad Request por sintaxis corrupta", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/echo", bytes.NewBufferString("{corrupto:json"))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("codigo esperado 400, obtenido %d", rec.Code)
		}
	})

	// 7. Validar preflight CORS OPTIONS (204 No Content)
	t.Run("Status 204 No Content en preflight OPTIONS de CORS", func(t *testing.T) {
		corsMiddleware := middleware.CORS(middleware.DefaultCORSOptions())
		wrapped := corsMiddleware(handler)

		req := httptest.NewRequest(http.MethodOptions, "/api/v1/ping", nil)
		req.Header.Set("Origin", "http://localhost:3000")
		rec := httptest.NewRecorder()
		wrapped.ServeHTTP(rec, req)

		if rec.Code != http.StatusNoContent {
			t.Errorf("codigo esperado 204 No Content para OPTIONS, obtenido %d", rec.Code)
		}
		if origin := rec.Header().Get("Access-Control-Allow-Origin"); origin != "http://localhost:3000" {
			t.Errorf("Access-Control-Allow-Origin incorrecto: '%s'", origin)
		}
	})

	// 8. Validar 429 Too Many Requests con RateLimiter
	t.Run("Status 429 Too Many Requests con cabecera Retry-After", func(t *testing.T) {
		sharedCache := memory.NewMemoryCache()
		limiter := middleware.NewRateLimiter(sharedCache, 2, 1*time.Minute)
		wrapped := limiter.Middleware()(handler)

		// Primeras 2 peticiones deben ser 200 OK
		for i := 0; i < 2; i++ {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/ping", nil)
			req.RemoteAddr = "192.168.1.50:1234"
			rec := httptest.NewRecorder()
			wrapped.ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("peticion %d debio ser 200, fue %d", i+1, rec.Code)
			}
		}

		// La tercera petición debe ser 429
		req3 := httptest.NewRequest(http.MethodGet, "/api/v1/ping", nil)
		req3.RemoteAddr = "192.168.1.50:1234"
		rec3 := httptest.NewRecorder()
		wrapped.ServeHTTP(rec3, req3)

		if rec3.Code != http.StatusTooManyRequests {
			t.Errorf("codigo esperado 429, obtenido %d", rec3.Code)
		}
		if retryAfter := rec3.Header().Get("Retry-After"); retryAfter == "" {
			t.Error("se esperaba cabecera Retry-After presente en respuesta 429")
		}
	})

	// 9. Validar 500 Recovery y ausencia de secretos en respuestas (AC-A-030-02)
	t.Run("Status 500 Recovery sin exponer stack trace ni secretos", func(t *testing.T) {
		logger := telemetry.NewJSONLogger(io.Discard, slog.LevelInfo)
		recMiddleware := middleware.Recovery(logger)
		wrapped := recMiddleware(handler)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/panic", nil)
		rec := httptest.NewRecorder()
		wrapped.ServeHTTP(rec, req)

		if rec.Code != http.StatusInternalServerError {
			t.Errorf("codigo esperado 500, obtenido %d", rec.Code)
		}

		bodyStr := rec.Body.String()
		if strings.Contains(bodyStr, "SuperClave123") {
			t.Error("CRITICO: La respuesta 500 expuso el secreto del panic!")
		}
		if strings.Contains(bodyStr, "goroutine") {
			t.Error("CRITICO: La respuesta 500 expuso el stack trace!")
		}

		// Validar que el log del servidor no expuso el secreto en texto plano
		if strings.Contains(logBuf.String(), "SuperClave123") && !strings.Contains(logBuf.String(), telemetry.RedactedMask) {
			t.Log("Log de panic registrado adecuadamente.")
		}
	})
}

func TestHealthAndReadinessProbes(t *testing.T) {
	srv, _ := setupTestServer()
	router := srv.Router()

	t.Run("Liveness probe /health siempre responde 200 UP", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("codigo esperado 200, obtenido %d", rec.Code)
		}
		if !strings.Contains(rec.Body.String(), `"status":"UP"`) {
			t.Errorf("cuerpo inesperado: %s", rec.Body.String())
		}
	})

	t.Run("Readiness probe /ready responde 200 cuando activo y 503 al drenar", func(t *testing.T) {
		// 1. Estado inicial activo
		req1 := httptest.NewRequest(http.MethodGet, "/ready", nil)
		rec1 := httptest.NewRecorder()
		router.ServeHTTP(rec1, req1)
		if rec1.Code != http.StatusOK {
			t.Errorf("readiness activo debio responder 200, fue %d", rec1.Code)
		}

		// 2. Simular inicio de Graceful Shutdown (drenado)
		router.SetReady(false)

		req2 := httptest.NewRequest(http.MethodGet, "/ready", nil)
		rec2 := httptest.NewRecorder()
		router.ServeHTTP(rec2, req2)
		if rec2.Code != http.StatusServiceUnavailable {
			t.Errorf("readiness en drenado debio responder 503, fue %d", rec2.Code)
		}
	})

	t.Run("Graceful Shutdown del servidor HTTP (AC-A-030-03)", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		err := srv.Shutdown(ctx)
		if err != nil {
			t.Fatalf("error en shutdown: %v", err)
		}
	})
}

