package http

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/redis/go-redis/v9"

	appContent "github.com/nexumpr/Nexum-Backend/internal/application/content"
	cacheinfra "github.com/nexumpr/Nexum-Backend/internal/infrastructure/cache"
)

// CacheControlPublico es la unica politica de cache de las rutas de contenido
// publico: el mismo cuerpo para todos los visitantes durante cinco minutos.
const CacheControlPublico = "public, max-age=300"

// cargadorContenido es la firma de la cache publica. Se declara como tipo para que
// el handler sea comprobable sin un Redis real; en produccion siempre apunta a
// cache.GetOrLoadJSON, que ya es fail-open ante un fallo de Redis.
type cargadorContenido func(
	ctx context.Context,
	client *redis.Client,
	key string,
	ttl time.Duration,
	load func() ([]byte, error),
) ([]byte, error)

// ContentHandler sirve contenido editorial publico ya serializado. No acepta
// cuerpo, no lee identidad y no toca DynamoDB: la unica dependencia es la cache
// publica, que nunca puede degradar la disponibilidad (fail-open).
type ContentHandler struct {
	cache  *redis.Client
	cargar cargadorContenido
	clave  string
	ttl    time.Duration
	load   func() ([]byte, error)
	logger *slog.Logger
}

func nuevoContentHandler(
	clienteRedis *redis.Client,
	clave string,
	ttl time.Duration,
	load func() ([]byte, error),
	logger *slog.Logger,
) *ContentHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &ContentHandler{
		cache:  clienteRedis,
		cargar: cacheinfra.GetOrLoadJSON,
		clave:  clave,
		ttl:    ttl,
		load:   load,
		logger: logger,
	}
}

// NewFAQHandler construye el handler de E070 (GET /api/v1/help/faq).
func NewFAQHandler(clienteRedis *redis.Client, logger *slog.Logger) *ContentHandler {
	return nuevoContentHandler(clienteRedis, appContent.CacheKeyFAQ, appContent.CacheTTL, appContent.FAQPayload, logger)
}

// NewTipsHandler construye el handler de E071 (GET /api/v1/education/tips).
func NewTipsHandler(clienteRedis *redis.Client, logger *slog.Logger) *ContentHandler {
	return nuevoContentHandler(clienteRedis, appContent.CacheKeyTips, appContent.CacheTTL, appContent.TipsPayload, logger)
}

// ServeHTTP responde 200 con el JSON exacto del contrato o 500 con el sobre de
// error canonico. La cabecera de cache se fija antes de escribir el estado para que
// ningun middleware posterior la sustituya por no-store.
func (h *ContentHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.cargar == nil || h.load == nil || h.clave == "" {
		w.Header().Set("Cache-Control", "no-store")
		writeRegisterError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "Servicio no disponible.", nil)
		return
	}

	cuerpo, err := h.cargar(r.Context(), h.cache, h.clave, h.ttl, h.load)
	if err != nil || len(cuerpo) == 0 {
		if h.logger != nil {
			// No se registra el contenido ni la clave: solo la clase del fallo.
			h.logger.ErrorContext(r.Context(), "contenido publico no disponible",
				slog.String("error_class", "content_load"))
		}
		w.Header().Set("Cache-Control", "no-store")
		writeRegisterError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Solicitud no completada.", nil)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", CacheControlPublico)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(cuerpo)
}
