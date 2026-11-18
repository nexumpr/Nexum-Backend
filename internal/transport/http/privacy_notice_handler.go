package http

import (
	"log/slog"
	"net/http"
	"time"

	appPrivacy "github.com/nexumpr/Nexum-Backend/internal/application/privacy"
	"github.com/nexumpr/Nexum-Backend/pkg/httputil"
)

// PrivacyNoticeHandler sirve GET /api/v1/privacy/notices/current.
//
// Es público: no exige JWT. Android lo consume antes del registro y muestra el
// contenido en su interfaz. No hay web separada ni dependencia de otro repositorio.
type PrivacyNoticeHandler struct {
	logger *slog.Logger
}

// NewPrivacyNoticeHandler construye el handler. El aviso es un recurso versionado
// del binario; no hay proveedor externo ni camino 503 por contenido.
func NewPrivacyNoticeHandler(logger *slog.Logger) *PrivacyNoticeHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &PrivacyNoticeHandler{logger: logger}
}

func (h *PrivacyNoticeHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "public, max-age=300")

	aviso := appPrivacy.AvisoVigente()
	respuesta := privacyNoticeResponse{
		Version:            aviso.Version,
		EffectiveAt:        aviso.EffectiveAt.UTC().Format(time.RFC3339),
		Text:               aviso.Text,
		URL:                aviso.URL,
		Content:            aviso.Content,
		RequiresAcceptance: aviso.RequiresAcceptance,
	}

	if err := httputil.WriteJSON(w, http.StatusOK, respuesta); err != nil {
		h.logger.ErrorContext(r.Context(), "privacy notice: fallo al serializar la respuesta",
			slog.String("error_class", "response_write"),
			slog.String("trace_id", traceID(r)))
	}
}

// privacyNoticeResponse es el objeto PrivacyNotice del contrato. version,
// effectiveAt, text y url son requeridos y no usan omitempty; content y
// requiresAcceptance son opcionales en el esquema, pero esta versión los emite
// siempre para no perder valores.
type privacyNoticeResponse struct {
	Version            string `json:"version"`
	EffectiveAt        string `json:"effectiveAt"`
	Text               string `json:"text"`
	URL                string `json:"url"`
	Content            string `json:"content"`
	RequiresAcceptance bool   `json:"requiresAcceptance"`
}
