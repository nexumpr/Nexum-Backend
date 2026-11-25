package http

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/nexumpr/Nexum-Backend/internal/transport/http/middleware"
)

// Errores de precondición de la Oleada 1.
var (
	// ErrIfMatchRequerido indica que la cabecera If-Match no se envió (HTTP 428).
	ErrIfMatchRequerido = errors.New("If-Match requerido")
	// ErrIfMatchInvalido indica un ETag mal formado o una versión no válida (HTTP 400).
	ErrIfMatchInvalido = errors.New("If-Match invalido")
	// ErrIfMatchDebil indica un validador débil W/"vN". RFC 9110 exige comparación
	// fuerte para If-Match: un validador débil nunca satisface la precondición (412).
	ErrIfMatchDebil = errors.New("If-Match debil no valido para actualizacion")
)

// FormatETag devuelve el ETag fuerte del contrato: "v<version>".
func FormatETag(version int64) string {
	return fmt.Sprintf("\"v%d\"", version)
}

// ParseIfMatchVersion valida If-Match y extrae la versión esperada.
//
// Sólo acepta el ETag fuerte `"vN"`. Un validador débil `W/"vN"` se rechaza con
// ErrIfMatchDebil: RFC 9110 exige comparación fuerte para If-Match y un validador
// débil nunca satisface la precondición. La ausencia produce ErrIfMatchRequerido
// (428); un formato mal formado produce ErrIfMatchInvalido (400).
func ParseIfMatchVersion(valor string) (int64, error) {
	limpio := strings.TrimSpace(valor)
	if limpio == "" {
		return 0, ErrIfMatchRequerido
	}
	if strings.HasPrefix(limpio, "W/") {
		return 0, ErrIfMatchDebil
	}
	if len(limpio) < 4 || limpio[0] != '"' || limpio[len(limpio)-1] != '"' {
		return 0, ErrIfMatchInvalido
	}
	contenido := limpio[1 : len(limpio)-1]
	if !strings.HasPrefix(contenido, "v") {
		return 0, ErrIfMatchInvalido
	}
	version, err := strconv.ParseInt(strings.TrimPrefix(contenido, "v"), 10, 64)
	if err != nil || version < 1 {
		return 0, ErrIfMatchInvalido
	}
	return version, nil
}

// ValidarLocale acepta el enum cerrado del contrato.
func ValidarLocale(valor string) (string, bool) {
	switch strings.TrimSpace(valor) {
	case "es-MX":
		return "es-MX", true
	case "en-US":
		return "en-US", true
	default:
		return "", false
	}
}

// ValidarZonaIANA valida una zona horaria IANA real con la base del sistema o la
// base embebida con time/tzdata. Rechaza vacío, "Local" y nombres fuera de rango.
func ValidarZonaIANA(valor string) (string, error) {
	limpio := strings.TrimSpace(valor)
	if len(limpio) < 3 || len(limpio) > 64 {
		return "", errors.New("zona horaria fuera de rango")
	}
	if limpio == "Local" || strings.ContainsAny(limpio, " \t\r\n") {
		return "", errors.New("zona horaria no IANA")
	}
	if _, err := time.LoadLocation(limpio); err != nil {
		return "", fmt.Errorf("zona horaria IANA invalida: %w", err)
	}
	return limpio, nil
}

// LimitadorRuta es el subconjunto de cache.AuthRateLimiter que usa el transporte.
// Se declara como interfaz para no acoplar HTTP a la implementación Redis.
type LimitadorRuta interface {
	Allow(ctx context.Context, class, identifier string) (bool, time.Duration, error)
}

// LimitarPorIP aplica una cuota por IP a rutas públicas. Es fail-open: si la IP
// no se puede determinar o Redis falla, la lectura se sirve; nunca se convierte
// un fallo de cuota en una caída de disponibilidad.
func LimitarPorIP(limitador LimitadorRuta, clase string, next http.HandlerFunc) http.HandlerFunc {
	if limitador == nil {
		return next
	}
	return func(w http.ResponseWriter, r *http.Request) {
		ip, ok := clientIPFromTrustedChain(r)
		if !ok {
			next(w, r)
			return
		}
		aplicarLimite(w, r, limitador, clase, ip, next)
	}
}

// LimitarPorUsuario aplica una cuota por usuario autenticado. Debe envolver al
// handler ya autenticado; si no hay identidad, no inventa una cuota compartida.
func LimitarPorUsuario(limitador LimitadorRuta, clase string, next http.HandlerFunc) http.HandlerFunc {
	if limitador == nil {
		return next
	}
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := middleware.UserIDFromContext(r.Context())
		if !ok || strings.TrimSpace(userID) == "" {
			next(w, r)
			return
		}
		aplicarLimite(w, r, limitador, clase, userID, next)
	}
}

func aplicarLimite(w http.ResponseWriter, r *http.Request, limitador LimitadorRuta, clase, identificador string, next http.HandlerFunc) {
	permitido, retryAfter, err := limitador.Allow(r.Context(), clase, identificador)
	if err != nil {
		// Fail-open: la cuota es un control de abuso, no una dependencia de datos.
		next(w, r)
		return
	}
	if permitido {
		next(w, r)
		return
	}
	segundos := int64(retryAfter / time.Second)
	if segundos < 1 {
		segundos = 1
	}
	w.Header().Set("Retry-After", strconv.FormatInt(segundos, 10))
	writeRegisterError(w, r, http.StatusTooManyRequests, "RATE_LIMITED", "limite de solicitudes excedido", nil)
}
