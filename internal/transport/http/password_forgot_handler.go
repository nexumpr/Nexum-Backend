package http

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	appIdentity "github.com/nexumpr/Nexum-Backend/internal/application/identity"
	"github.com/nexumpr/Nexum-Backend/internal/ports"
)

// PasswordForgotHandler implementa POST /api/v1/auth/password/forgot (E005).
//
// Contrato congelado (RFC-016): 202 Accepted {status, requestId}; 400/409/422/429/500/503.
// La respuesta es indistinguible exista o no la cuenta. Si falta un componente real
// (sellado o correo) se responde 503: nunca se acepta trabajo que nadie podria enviar.
type PasswordForgotHandler struct {
	servicio *appIdentity.PasswordResetService
	buscar   appIdentity.BuscarCuenta
	logger   *slog.Logger
}

// NewPasswordForgotHandler construye el handler. `servicio` o `buscar` nil dejan el
// endpoint en 503 en lugar de autorizar sin verificar.
func NewPasswordForgotHandler(
	servicio *appIdentity.PasswordResetService,
	buscar appIdentity.BuscarCuenta,
	logger *slog.Logger,
) *PasswordForgotHandler {
	return &PasswordForgotHandler{servicio: servicio, buscar: buscar, logger: logger}
}

type passwordForgotRequest struct {
	Email string `json:"email"`
}

type acceptedResponse struct {
	Status    string `json:"status"`
	RequestID string `json:"requestId"`
}

func (h *PasswordForgotHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.servicio == nil || !h.servicio.Disponible() {
		writeResetError(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "Servicio no disponible.")
		return
	}

	cuerpo, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 4096))
	if err != nil {
		writeResetError(w, http.StatusRequestEntityTooLarge, "PAYLOAD_TOO_LARGE", "Solicitud demasiado grande.")
		return
	}
	var req passwordForgotRequest
	dec := json.NewDecoder(bytes.NewReader(cuerpo))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil || req.Email == "" || !appIdentity.ValidarEmail(req.Email) {
		writeResetError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "Solicitud invalida.")
		return
	}
	// Rechaza JSON adicional tras el objeto (p. ej. dos cuerpos concatenados).
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeResetError(w, http.StatusBadRequest, "INVALID_JSON", "Solicitud invalida.")
		return
	}

	if err := h.servicio.Solicitar(r.Context(), req.Email, h.buscar); err != nil {
		switch {
		case errors.Is(err, ports.ErrResetCuota):
			w.Header().Set("Retry-After", "900")
			writeResetError(w, http.StatusTooManyRequests, "RATE_LIMITED", "Demasiadas solicitudes.")
		case errors.Is(err, ports.ErrResetNoDisponible):
			writeResetError(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "Servicio no disponible.")
		default:
			if h.logger != nil {
				h.logger.ErrorContext(r.Context(), "fallo en recuperacion de contrasena", "motivo", "interno")
			}
			writeResetError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Error interno.")
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(acceptedResponse{Status: "accepted", RequestID: nuevoRequestID()})
}

// nuevoRequestID genera un UUID v4 sin dependencias externas.
func nuevoRequestID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return ""
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func writeResetError(w http.ResponseWriter, codigo int, code, mensaje string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(codigo)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]any{"code": code, "message": mensaje, "details": []any{}},
	})
}
