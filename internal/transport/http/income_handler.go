package http

import (
	"errors"
	"log/slog"
	"net/http"
	"regexp"
	"strings"

	appIdentity "github.com/nexumpr/Nexum-Backend/internal/application/identity"
	"github.com/nexumpr/Nexum-Backend/internal/ports"
	"github.com/nexumpr/Nexum-Backend/internal/transport/http/middleware"
	"github.com/nexumpr/Nexum-Backend/pkg/httputil"
)

// Transporte del ingreso base: E024 (`GET /api/v1/income`) y E025 (`PUT /api/v1/income`).
//
// Ambas rutas exigen `Authorization: Bearer` con JWT RS256 y estado autoritativo de la
// sesión en DynamoDB (E004): la identidad sale exclusivamente de `sub`/`sid` verificados y
// el cliente no puede leer ni escribir el ingreso de otro titular.
//
// El cuerpo de éxito es el objeto `Income` del contrato, cerrado
// (`additionalProperties: false`) con EXACTAMENTE cuatro campos. El DTO se construye campo a
// campo, de modo que `monthlyEquivalentCents`, `entityType`, `userId`, `createdAt` o
// cualquier atributo interno no tienen ningún camino de serialización.

// patronImporteIngreso es el patrón publicado por `Income.amount` e `IncomeUpdate.amount`:
// parte entera sin ceros a la izquierda de hasta doce dígitos y exactamente dos decimales.
var patronImporteIngreso = regexp.MustCompile(`^(0|[1-9][0-9]{0,11})\.[0-9]{2}$`)

// errIfMatchDebil marca un ETag débil. El contrato exige un validador FUERTE `"vN"`, así que
// `W/"vN"` no es una precondición equivalente: se rechaza con 412 (RFC 9110 §13.1.1) en
// lugar de aceptarse como si fuera la versión vigente.
var errIfMatchDebil = errors.New("ETag debil no admitido")

// incomeResponse es el objeto `Income` del contrato: exactamente cuatro campos.
type incomeResponse struct {
	Amount    string `json:"amount"`
	Currency  string `json:"currency"`
	Frequency string `json:"frequency"`
	Version   int64  `json:"version"`
}

// incomeUpdateRequest es `IncomeUpdate`: tres campos requeridos y ningún otro. La
// frecuencia se valida como regla de negocio (422); el formato del importe y el enum de una
// sola divisa se validan como esquema (400).
type incomeUpdateRequest struct {
	Amount    string `json:"amount"`
	Currency  string `json:"currency"`
	Frequency string `json:"frequency"`
}

// Validate aplica las comprobaciones de ESQUEMA del contrato. Un error aquí se responde 400
// INVALID_REQUEST. La frecuencia sólo se exige presente: su pertenencia a los nueve valores
// publicados es una regla de negocio y se responde 422 (RFC-020 §5).
func (r incomeUpdateRequest) Validate() error {
	if !patronImporteIngreso.MatchString(r.Amount) {
		return errors.New("amount debe ser una cadena decimal con dos decimales")
	}
	if r.Currency != appIdentity.RegisterCurrency {
		return errors.New("currency debe ser MXN")
	}
	if strings.TrimSpace(r.Frequency) == "" {
		return errors.New("frequency requerida")
	}
	return nil
}

// parseIfMatchFuerte exige un ETag fuerte `"vN"`.
//
// Ausencia de cabecera: ErrIfMatchRequerido (428). Forma débil `W/"vN"`: errIfMatchDebil
// (412). Cualquier otra forma, versión no numérica o menor que 1: ErrIfMatchInvalido (400).
func parseIfMatchFuerte(valor string) (int64, error) {
	limpio := strings.TrimSpace(valor)
	if strings.HasPrefix(limpio, "W/") {
		return 0, errIfMatchDebil
	}
	return ParseIfMatchVersion(limpio)
}

// nuevoIncomeResponse proyecta la instantánea al DTO exacto del contrato. El importe se
// formatea desde céntimos enteros y la frecuencia ya llega canónica.
func nuevoIncomeResponse(snapshot *appIdentity.IncomeSnapshot) incomeResponse {
	return incomeResponse{
		Amount:    httputil.FormatCentsToDecimal(snapshot.AmountCents),
		Currency:  appIdentity.RegisterCurrency,
		Frequency: snapshot.Frequency.String(),
		Version:   snapshot.Version,
	}
}

// cadenaIngreso compone la cadena de una ruta privada del ingreso: autenticación con estado
// autoritativo de sesión y, si está configurada, cuota por usuario (120/min de la Oleada 1).
//
// Devuelve nil cuando falta un componente obligatorio (servicio, proveedor de tokens o
// autorizador de sesión): el handler responde 503 en lugar de autorizar sólo con la firma o
// servir datos sin persistencia. La cuota es opcional: sin Redis se omite (429 no se emite)
// en lugar de simularse.
func cadenaIngreso(
	servicio *appIdentity.IncomeService,
	tokens ports.TokenProvider,
	sesiones ports.SessionAuthorizer,
	limitador LimitadorRuta,
	clase string,
	serve http.HandlerFunc,
) http.Handler {
	if servicio == nil || tokens == nil || sesiones == nil {
		return nil
	}
	var interior http.Handler = http.HandlerFunc(serve)
	if limitador != nil {
		interior = http.HandlerFunc(LimitarPorUsuario(limitador, clase, serve))
	}
	return middleware.AuthenticateBearerWithSession(tokens, sesiones)(interior)
}

// escribirIngresoOK serializa el ingreso con cabeceras de información privada y el ETag
// fuerte de la versión vigente, que es el validador que E025 exige en `If-Match`.
func escribirIngresoOK(w http.ResponseWriter, r *http.Request, logger *slog.Logger, snapshot *appIdentity.IncomeSnapshot) {
	// Información financiera del titular: nunca cacheable por proxies ni por el cliente.
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("ETag", FormatETag(snapshot.Version))
	if err := httputil.WriteJSON(w, http.StatusOK, nuevoIncomeResponse(snapshot)); err != nil {
		logger.ErrorContext(r.Context(), "income: fallo al serializar la respuesta",
			slog.String("error_class", "response_write"),
			slog.String("trace_id", traceID(r)))
	}
}

// ---------------- E024: GET /api/v1/income ----------------

// IncomeGetHandler sirve `GET /api/v1/income` (E024).
type IncomeGetHandler struct {
	service   *appIdentity.IncomeService
	logger    *slog.Logger
	protegido http.Handler
}

// NewIncomeGetHandler construye el handler. Sin servicio, sin proveedor de tokens o sin
// autorizador de sesión, el endpoint responde 503 (fail-closed).
func NewIncomeGetHandler(
	servicio *appIdentity.IncomeService,
	tokens ports.TokenProvider,
	sesiones ports.SessionAuthorizer,
	limitador LimitadorRuta,
	logger *slog.Logger,
) *IncomeGetHandler {
	if logger == nil {
		logger = slog.Default()
	}
	h := &IncomeGetHandler{service: servicio, logger: logger}
	h.protegido = cadenaIngreso(servicio, tokens, sesiones, limitador, "e024", h.serve)
	return h
}

// ServeHTTP aplica la guarda de composición y delega en la cadena autenticada.
func (h *IncomeGetHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.service == nil || h.protegido == nil {
		writeRegisterError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "servicio no disponible", nil)
		return
	}
	h.protegido.ServeHTTP(w, r)
}

// serve se ejecuta con la sesión ya verificada.
func (h *IncomeGetHandler) serve(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok || strings.TrimSpace(userID) == "" {
		writeRegisterError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "sesion no valida", nil)
		return
	}

	snapshot, err := h.service.GetIncome(r.Context(), userID)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	escribirIngresoOK(w, r, h.logger, snapshot)
}

// writeError traduce los errores de aplicación de E024 a códigos del contrato.
func (h *IncomeGetHandler) writeError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, appIdentity.ErrIncomeUnauthorized):
		writeRegisterError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "sesion no valida", nil)
	case errors.Is(err, appIdentity.ErrIncomeNotFound):
		writeRegisterError(w, r, http.StatusNotFound, "NOT_FOUND", "ingreso base no configurado", nil)
	case errors.Is(err, appIdentity.ErrIncomeInconsistent):
		h.logger.ErrorContext(r.Context(), "income: item persistido incoherente",
			slog.String("error_class", "income_inconsistent"),
			slog.String("trace_id", traceID(r)))
		writeRegisterError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "error interno", nil)
	case errors.Is(err, appIdentity.ErrIncomeUnavailable):
		h.logger.ErrorContext(r.Context(), "income: dependencia no disponible",
			slog.String("error_class", "income_unavailable"),
			slog.String("trace_id", traceID(r)))
		writeRegisterError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "servicio no disponible", nil)
	default:
		h.logger.ErrorContext(r.Context(), "income: fallo interno",
			slog.String("error_class", "internal_error"),
			slog.String("trace_id", traceID(r)))
		writeRegisterError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "error interno", nil)
	}
}

// ---------------- E025: PUT /api/v1/income ----------------

// IncomePutHandler sirve `PUT /api/v1/income` (E025).
type IncomePutHandler struct {
	service   *appIdentity.IncomeService
	logger    *slog.Logger
	protegido http.Handler
}

// NewIncomePutHandler construye el handler. Sin servicio, sin proveedor de tokens o sin
// autorizador de sesión, el endpoint responde 503 (fail-closed).
func NewIncomePutHandler(
	servicio *appIdentity.IncomeService,
	tokens ports.TokenProvider,
	sesiones ports.SessionAuthorizer,
	limitador LimitadorRuta,
	logger *slog.Logger,
) *IncomePutHandler {
	if logger == nil {
		logger = slog.Default()
	}
	h := &IncomePutHandler{service: servicio, logger: logger}
	h.protegido = cadenaIngreso(servicio, tokens, sesiones, limitador, "e025", h.serve)
	return h
}

// ServeHTTP aplica la guarda de composición y delega en la cadena autenticada.
func (h *IncomePutHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.service == nil || h.protegido == nil {
		writeRegisterError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "servicio no disponible", nil)
		return
	}
	h.protegido.ServeHTTP(w, r)
}

// serve se ejecuta con la sesión ya verificada.
//
// Orden de decisión: identidad → precondición `If-Match` → cuerpo. La precondición precede
// al cuerpo porque una escritura sin validador no debe llegar a evaluar el payload
// (428/400/412 antes de cualquier trabajo de negocio).
func (h *IncomePutHandler) serve(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok || strings.TrimSpace(userID) == "" {
		writeRegisterError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "sesion no valida", nil)
		return
	}

	expectedVersion, errPrecondicion := parseIfMatchFuerte(r.Header.Get("If-Match"))
	if errPrecondicion != nil {
		h.writePreconditionError(w, r, errPrecondicion)
		return
	}

	var peticion incomeUpdateRequest
	if err := BindJSON(w, r, &peticion); err != nil {
		writeRegisterError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "solicitud no completada", nil)
		return
	}

	amountCents, errImporte := httputil.ParseDecimalToCents(peticion.Amount)
	if errImporte != nil {
		// Defensa en profundidad: `Validate` ya aplicó el patrón publicado.
		writeRegisterError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "solicitud no completada", []httputil.ErrorDetail{
			{Field: "amount", Reason: "invalid_format"},
		})
		return
	}

	snapshot, err := h.service.SetIncome(r.Context(), appIdentity.IncomeUpdate{
		UserID:          userID,
		AmountCents:     amountCents,
		Frequency:       peticion.Frequency,
		ExpectedVersion: expectedVersion,
	})
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	escribirIngresoOK(w, r, h.logger, snapshot)
}

// writePreconditionError traduce los fallos de `If-Match`.
func (h *IncomePutHandler) writePreconditionError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ErrIfMatchRequerido):
		writeRegisterError(w, r, http.StatusPreconditionRequired, "PRECONDITION_REQUIRED", "If-Match requerido", []httputil.ErrorDetail{
			{Field: "If-Match", Reason: "required"},
		})
	case errors.Is(err, errIfMatchDebil):
		writeRegisterError(w, r, http.StatusPreconditionFailed, "VERSION_CONFLICT", "If-Match debe ser un ETag fuerte", nil)
	default:
		writeRegisterError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "solicitud no completada", []httputil.ErrorDetail{
			{Field: "If-Match", Reason: "invalid_format"},
		})
	}
}

// writeError traduce los errores de aplicación de E025 a códigos del contrato.
func (h *IncomePutHandler) writeError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, appIdentity.ErrIncomeUnauthorized):
		writeRegisterError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "sesion no valida", nil)
	case errors.Is(err, appIdentity.ErrIncomeInvalidRequest):
		writeRegisterError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "solicitud no completada", nil)
	case errors.Is(err, appIdentity.ErrIncomeNotFound):
		writeRegisterError(w, r, http.StatusNotFound, "NOT_FOUND", "ingreso base no configurado", nil)
	case errors.Is(err, appIdentity.ErrIncomeVersionConflict):
		writeRegisterError(w, r, http.StatusPreconditionFailed, "VERSION_CONFLICT", "la version enviada no es la vigente", nil)
	case errors.Is(err, appIdentity.ErrIncomeConflict):
		writeRegisterError(w, r, http.StatusConflict, "CONFLICT", "conflicto de escritura concurrente", nil)
	case errors.Is(err, appIdentity.ErrIncomeInvalidAmount):
		writeRegisterError(w, r, http.StatusUnprocessableEntity, "BUSINESS_RULE_VIOLATION", "importe fuera de la regla de negocio", []httputil.ErrorDetail{
			{Field: "amount", Reason: "out_of_range"},
		})
	case errors.Is(err, appIdentity.ErrIncomeInvalidFrequency):
		writeRegisterError(w, r, http.StatusUnprocessableEntity, "BUSINESS_RULE_VIOLATION", "frecuencia no admitida", []httputil.ErrorDetail{
			{Field: "frequency", Reason: "out_of_enum"},
		})
	case errors.Is(err, appIdentity.ErrIncomeInconsistent):
		h.logger.ErrorContext(r.Context(), "income: item persistido incoherente",
			slog.String("error_class", "income_inconsistent"),
			slog.String("trace_id", traceID(r)))
		writeRegisterError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "error interno", nil)
	case errors.Is(err, appIdentity.ErrIncomeUnavailable):
		h.logger.ErrorContext(r.Context(), "income: dependencia no disponible",
			slog.String("error_class", "income_unavailable"),
			slog.String("trace_id", traceID(r)))
		writeRegisterError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "servicio no disponible", nil)
	default:
		h.logger.ErrorContext(r.Context(), "income: fallo interno",
			slog.String("error_class", "internal_error"),
			slog.String("trace_id", traceID(r)))
		writeRegisterError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "error interno", nil)
	}
}
