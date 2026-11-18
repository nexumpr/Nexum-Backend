package http

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/nexumpr/Nexum-Backend/internal/application/catalogs"
)

// CargadorCatalogoPublico lee —o carga y guarda— el JSON de un catálogo público.
//
// cmd/server lo implementa con cache.GetOrLoadJSON, que es fail-open: si Redis no está
// configurado o falla, ejecuta la carga y sirve el contenido. El transporte no importa el
// cliente Redis ni conoce su configuración.
type CargadorCatalogoPublico func(
	ctx context.Context,
	clave string,
	ttl time.Duration,
	load func() ([]byte, error),
) ([]byte, error)

// CatalogHandler sirve un catálogo público versionado (E022/E023).
//
// El cuerpo se serializa UNA sola vez en la construcción, a partir del DTO del contrato.
// Cada petición reutiliza exactamente esos bytes, de modo que el JSON servido en frío y el
// servido desde Redis son idénticos y no existe un camino de error de serialización por
// petición.
//
// Códigos contemplados: 200, 429 (si hay limitador distribuido), 500 y 503. El contrato no
// declara `ETag` ni parámetros de paginación: la página es única y no se emite `nextCursor`.
type CatalogHandler struct {
	payload  []byte
	cacheKey string
	ttl      time.Duration
	maxAge   int64
	cargar   CargadorCatalogoPublico
	logger   *slog.Logger
	entrada  http.Handler
}

// NewCategoriesHandler construye el handler de `GET /api/v1/catalogs/categories` (E022).
//
// `limitador` puede ser nil: en ese caso la cuota se omite (429 no se emite) y se documenta,
// en lugar de simularse. El catálogo no depende de DynamoDB ni de JWT: es público por
// contrato (`security: []`).
func NewCategoriesHandler(
	limitador LimitadorRuta,
	cargar CargadorCatalogoPublico,
	logger *slog.Logger,
) *CatalogHandler {
	items := make([]categoryResponse, 0, len(catalogs.Categories()))
	for _, categoria := range catalogs.Categories() {
		items = append(items, categoryResponse{
			ID:   categoria.ID,
			Name: categoria.Name,
			Type: categoria.Type,
		})
	}
	return nuevoCatalogoHandler(
		catalogs.CategoriesCacheKey,
		catalogs.CacheTTL,
		pageOfCategoriesResponse{Items: items},
		limitador,
		"e022",
		cargar,
		logger,
	)
}

// categoryResponse es el objeto `Category` del contrato (`additionalProperties: false`):
// exactamente `id`, `name` y `type`.
type categoryResponse struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
}

// pageOfCategoriesResponse es `PageOfCategory`: `items` siempre presente (array no nulo).
// El contrato no ofrece paginación para este catálogo, así que no se emite `nextCursor`.
type pageOfCategoriesResponse struct {
	Items []categoryResponse `json:"items"`
}

// nuevoCatalogoHandler comparte la construcción de los dos catálogos públicos. `contenido`
// es el DTO exacto del contrato; se serializa aquí una sola vez.
func nuevoCatalogoHandler(
	clave string,
	ttl time.Duration,
	contenido any,
	limitador LimitadorRuta,
	clase string,
	cargar CargadorCatalogoPublico,
	logger *slog.Logger,
) *CatalogHandler {
	if logger == nil {
		logger = slog.Default()
	}
	if ttl <= 0 {
		ttl = catalogs.CacheTTL
	}
	h := &CatalogHandler{
		cacheKey: clave,
		ttl:      ttl,
		maxAge:   int64(ttl / time.Second),
		cargar:   cargar,
		logger:   logger,
	}
	if datos, err := json.Marshal(contenido); err == nil && validoCatalogo(datos) {
		h.payload = datos
	} else {
		// Sólo puede ocurrir por un error de serialización del DTO estático: se falla
		// cerrado (503) en lugar de servir un cuerpo incompleto.
		logger.Error("catalogo: no se pudo serializar el contenido",
			slog.String("clave", clave),
			slog.String("error_class", "catalogo_serializacion"))
	}
	h.entrada = LimitarPorIP(limitador, clase, http.HandlerFunc(h.serve))
	return h
}

// ServeHTTP aplica la guarda de composición y delega en la cadena con cuota por IP.
func (h *CatalogHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.entrada == nil || len(h.payload) == 0 {
		writeRegisterError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "servicio no disponible", nil)
		return
	}
	h.entrada.ServeHTTP(w, r)
}

// serve responde el catálogo. La lectura de caché es fail-open (A-080 §2.6): un fallo de
// Redis nunca convierte una lectura pública en una caída de disponibilidad.
func (h *CatalogHandler) serve(w http.ResponseWriter, r *http.Request) {
	if len(h.payload) == 0 {
		writeRegisterError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "servicio no disponible", nil)
		return
	}

	datos := h.payload
	if h.cargar != nil && h.cacheKey != "" {
		if cacheados, err := h.cargar(r.Context(), h.cacheKey, h.ttl, func() ([]byte, error) {
			return h.payload, nil
		}); err == nil && validoCatalogo(cacheados) {
			datos = cacheados
		}
	}

	// Catálogo público y versionado: se puede cachear en proxies y en el cliente.
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", fmt.Sprintf("public, max-age=%d", h.maxAge))
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(datos); err != nil {
		h.logger.WarnContext(r.Context(), "catalogo: fallo al escribir la respuesta",
			slog.String("error_class", "response_write"),
			slog.String("trace_id", traceID(r)))
	}
}

// validoCatalogo comprueba que un JSON conserva la forma del contrato antes de servirlo:
// válido, objeto con `items` como array y —si aparece— `nextCursor` como cadena. Protege
// frente a una entrada de caché corrupta o ajena sin aceptar como catálogo cualquier JSON.
func validoCatalogo(datos []byte) bool {
	if len(datos) == 0 || !json.Valid(datos) {
		return false
	}
	var forma struct {
		Items      json.RawMessage `json:"items"`
		NextCursor *string         `json:"nextCursor"`
	}
	if err := json.Unmarshal(datos, &forma); err != nil {
		return false
	}
	return len(forma.Items) > 0 && forma.Items[0] == '['
}
