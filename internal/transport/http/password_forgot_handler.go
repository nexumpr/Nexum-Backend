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
	"github.com/nexumpr/Nexum-Backend/pkg/httputil"
)

// passwordForgotMaxRequestBodyBytes acota el cuerpo de E005. El unico campo del contrato
// es un correo (254 caracteres como maximo): 4 KiB deja margen para JSON bien formado y
// rechaza el resto con el 413 que el contrato declara.
const passwordForgotMaxRequestBodyBytes int64 = 4096

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
		writeResetError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "Servicio no disponible.", nil)
		return
	}

	cuerpo, err := io.ReadAll(http.MaxBytesReader(w, r.Body, passwordForgotMaxRequestBodyBytes))
	if err != nil {
		// 413 declarado por el contrato: el cuerpo supera el limite o no se puede leer.
		writeResetError(w, r, http.StatusRequestEntityTooLarge, "PAYLOAD_TOO_LARGE", "Solicitud demasiado grande.", nil)
		return
	}
	var req passwordForgotRequest
	dec := json.NewDecoder(bytes.NewReader(cuerpo))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		// JSON malformado, campo desconocido, tipo incorrecto o cuerpo vacio: 400 canonico.
		writeResetError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "Solicitud no completada.", nil)
		return
	}
	// Rechaza JSON adicional tras el objeto (p. ej. dos cuerpos concatenados).
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeResetError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "Solicitud no completada.", nil)
		return
	}
	// El contenido del correo es una regla de negocio (422) y no revela si la cuenta existe.
	if req.Email == "" || !appIdentity.ValidarEmail(req.Email) {
		writeResetError(w, r, http.StatusUnprocessableEntity, "BUSINESS_RULE_VIOLATION", "Solicitud no completada.", []httputil.ErrorDetail{
			{Field: "email", Reason: "invalid_format"},
		})
		return
	}

	if err := h.servicio.Solicitar(r.Context(), req.Email, ipDelCliente(r), h.buscar); err != nil {
		switch {
		case errors.Is(err, ports.ErrResetCuota):
			w.Header().Set("Retry-After", "900")
			writeResetError(w, r, http.StatusTooManyRequests, "RATE_LIMITED", "Demasiadas solicitudes.", nil)
		case errors.Is(err, ports.ErrResetNoDisponible):
			writeResetError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "Servicio no disponible.", nil)
		default:
			if h.logger != nil {
				h.logger.ErrorContext(r.Context(), "fallo en recuperacion de contrasena", "motivo", "interno")
			}
			writeResetError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Error interno.", nil)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(acceptedResponse{Status: "accepted", RequestID: nuevoRequestID()})
}

// ipDelCliente extrae la IP de la cadena de proxies de confianza (CloudFront/ALB).
// Si la cadena no es valida devuelve cadena vacia: la cuota por IP se omite y la de
// correo sigue aplicandose.
func ipDelCliente(r *http.Request) string {
	if ip, ok := clientIPFromTrustedChain(r); ok {
		return ip
	}
	return ""
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

// writeResetError emite el ErrorEnvelope canonico de A-040 con el traceId de la
// peticion. Antes se construia un mapa propio sin `traceId`, lo que rompia la
// correlacion con `traceparent`/`X-Request-ID` en los errores de E005.
func writeResetError(w http.ResponseWriter, r *http.Request, codigo int, code, mensaje string, detalles []httputil.ErrorDetail) {
	w.Header().Set("Cache-Control", "no-store")
	_ = httputil.WriteErrorEnvelope(w, codigo, code, mensaje, traceID(r), detalles)
}
