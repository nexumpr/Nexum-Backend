package http

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	appPrivacy "github.com/nexumpr/Nexum-Backend/internal/application/privacy"
	"github.com/nexumpr/Nexum-Backend/pkg/httputil"
)

// NoticeProvider entrega el aviso vigente o el error de bloqueo.
type NoticeProvider interface {
	Current() (appPrivacy.Notice, error)
}

// PrivacyNoticeHandler sirve GET /api/v1/privacy/notices/current.
//
// Es público: no exige JWT. Android lo consume antes del registro y muestra el
// contenido en su interfaz. No hay web separada ni dependencia de otro repositorio.
type PrivacyNoticeHandler struct {
	provider NoticeProvider
	logger   *slog.Logger
}

// NewPrivacyNoticeHandler construye el handler. Sin proveedor o sin fecha
// efectiva aprobada responde 503: no se publica un aviso con fecha inventada.
func NewPrivacyNoticeHandler(provider NoticeProvider, logger *slog.Logger) *PrivacyNoticeHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &PrivacyNoticeHandler{provider: provider, logger: logger}
}

func (h *PrivacyNoticeHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "public, max-age=300")

	if h == nil || h.provider == nil {
		writeRegisterError(w, r, http.StatusServiceUnavailable, "CONTENT_NOT_AVAILABLE", "aviso no disponible", nil)
		return
	}
	aviso, err := h.provider.Current()
	if err != nil {
		if errors.Is(err, appPrivacy.ErrFechaEfectivaNoAprobada) {
			writeRegisterError(w, r, http.StatusServiceUnavailable, "CONTENT_NOT_AVAILABLE", "aviso no disponible", nil)
			return
		}
		h.logger.ErrorContext(r.Context(), "privacy notice: fallo al cargar el aviso",
			slog.String("error_class", "notice_unavailable"),
			slog.String("trace_id", traceID(r)))
		writeRegisterError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "error interno", nil)
		return
	}
	if aviso.EffectiveAt.IsZero() {
		writeRegisterError(w, r, http.StatusServiceUnavailable, "CONTENT_NOT_AVAILABLE", "aviso no disponible", nil)
		return
	}

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
