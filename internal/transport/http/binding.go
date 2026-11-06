package http

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/nexumpr/Nexum-Backend/pkg/httputil"
)

// HeaderIdempotencyKey cabecera estándar para operaciones financieras sensibles (A-040 §2.5, AC-A-040-03).
const HeaderIdempotencyKey = "Idempotency-Key"

// Validator interfaz opcional que pueden implementar los DTOs para validación de negocio/sintaxis.
type Validator interface {
	Validate() error
}

// BindJSON decodifica el cuerpo de la solicitud JSON de forma estricta:
// 1. Rechaza Content-Type no compatible con JSON.
// 2. Limita bytes a 1MB contra denegación de servicio.
// 3. Prohíbe campos desconocidos (DisallowUnknownFields).
// 4. Si el destino implementa Validator, ejecuta Validate().
func BindJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
		return errors.New("solicitudes GET/HEAD/OPTIONS no admiten cuerpo JSON")
	}

	if err := httputil.DecodeJSON(w, r, dst, httputil.DefaultMaxRequestBodyBytes); err != nil {
		return err
	}

	if v, ok := dst.(Validator); ok {
		if err := v.Validate(); err != nil {
			return fmt.Errorf("validacion del payload fallida: %w", err)
		}
	}

	return nil
}

// RequireIdempotencyKey valida y extrae la clave de idempotencia obligatoria en escrituras sensibles (A-040 §2.5).
func RequireIdempotencyKey(r *http.Request) (string, error) {
	key := strings.TrimSpace(r.Header.Get(HeaderIdempotencyKey))
	if key == "" {
		return "", errors.New("cabecera 'Idempotency-Key' requerida para esta operacion")
	}
	if len(key) < 16 || len(key) > 128 {
		return "", errors.New("la cabecera 'Idempotency-Key' debe tener una longitud entre 16 y 128 caracteres")
	}
	return key, nil
}

