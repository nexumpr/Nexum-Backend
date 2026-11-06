package http

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/nexumpr/Nexum-Backend/internal/transport/http/middleware"
)

// ServerConfig encapsula la parametrización de red y timeouts del servidor HTTP (A-030 §2.2).
type ServerConfig struct {
	Port              int
	ReadHeaderTimeout time.Duration
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
	MaxBodyBytes      int64
	EnableTLS         bool
	CORSOptions       middleware.CORSOptions
	RateLimitMaxReq   int
	RateLimitWindow   time.Duration
}

// DefaultServerConfig genera los valores iniciales auditados de A-030 §2.2.
func DefaultServerConfig() ServerConfig {
	return ServerConfig{
		Port:              8080,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxBodyBytes:      1048576, // 1 MiB
		EnableTLS:         false,
		CORSOptions:       middleware.DefaultCORSOptions(),
		RateLimitMaxReq:   100,
		RateLimitWindow:   1 * time.Minute,
	}
}

// HTTPServer envuelve http.Server proporcionando inicialización segura y apagado suave (A-030 §2.7, AC-A-030-03).
type HTTPServer struct {
	config     ServerConfig
	httpServer *http.Server
	router     *Router
	logger     *slog.Logger
}

// NewHTTPServer construye el servidor HTTP conectando la cadena de middleware de seguridad y el router.
func NewHTTPServer(cfg ServerConfig, router *Router, logger *slog.Logger) *HTTPServer {
	// Cadena global de middleware: Recovery -> SecurityHeaders -> CORS
	handler := middleware.Chain(
		router,
		middleware.Recovery(logger),
		middleware.SecurityHeaders(cfg.EnableTLS),
		middleware.CORS(cfg.CORSOptions),
	)

	addr := fmt.Sprintf(":%d", cfg.Port)
	srv := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: cfg.ReadHeaderTimeout,
		ReadTimeout:       cfg.ReadTimeout,
		WriteTimeout:      cfg.WriteTimeout,
		IdleTimeout:       cfg.IdleTimeout,
	}

	return &HTTPServer{
		config:     cfg,
		httpServer: srv,
		router:     router,
		logger:     logger,
	}
}

// Router retorna el router interno para registrar rutas.
func (s *HTTPServer) Router() *Router {
	return s.router
}

// Start inicia la escucha de peticiones en segundo plano.
func (s *HTTPServer) Start() error {
	s.logger.Info("Iniciando servidor HTTP",
		slog.Int("port", s.config.Port),
		slog.Duration("read_header_timeout", s.config.ReadHeaderTimeout),
		slog.Duration("write_timeout", s.config.WriteTimeout),
	)
	return s.httpServer.ListenAndServe()
}

// Shutdown ejecuta el drenado seguro de conexiones activas (AC-A-030-03).
func (s *HTTPServer) Shutdown(ctx context.Context) error {
	s.logger.Info("Iniciando Graceful Shutdown del servidor HTTP...")

	// 1. Marcar probe de readiness en false (503 Service Unavailable) para retirar tráfico del balanceador
	s.router.SetReady(false)

	// 2. Drenar conexiones in-flight
	return s.httpServer.Shutdown(ctx)
}
