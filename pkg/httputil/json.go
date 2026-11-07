package httputil

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// DefaultMaxRequestBodyBytes es el límite por defecto para cuerpos JSON entrantes (1 MB).
const DefaultMaxRequestBodyBytes int64 = 1048576

// ErrorResponse representa la estructura estándar de respuesta de error del Backend (A-010 §2.2, GOV-15).
type ErrorResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details string `json:"details,omitempty"`
}

// WriteJSON serializa y envía una respuesta HTTP en formato JSON con la cabecera adecuada.
func WriteJSON(w http.ResponseWriter, status int, data any) error {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if data == nil {
		return nil
	}
	return json.NewEncoder(w).Encode(data)
}

// WriteError serializa y envía una respuesta de error JSON con el código de estado indicado.
func WriteError(w http.ResponseWriter, status int, code, message string) {
	_ = WriteJSON(w, status, ErrorResponse{
		Code:    code,
		Message: message,
	})
}

// DecodeJSON decodifica el cuerpo de una solicitud HTTP en la estructura destino de forma segura:
// 1. Limita los bytes leídos con http.MaxBytesReader para prevenir agotamiento de memoria.
// 2. Aplica DisallowUnknownFields para rechazar campos no especificados.
// 3. Valida sintaxis y tipos de datos.
func DecodeJSON(w http.ResponseWriter, r *http.Request, dst any, maxBytes int64) error {
	if maxBytes <= 0 {
		maxBytes = DefaultMaxRequestBodyBytes
	}

	ct := r.Header.Get("Content-Type")
	if ct != "" && !strings.HasPrefix(strings.ToLower(ct), "application/json") {
		return fmt.Errorf("content-type no soportado: se requiere 'application/json'")
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	err := dec.Decode(dst)
	if err != nil {
		var syntaxErr *json.SyntaxError
		var unmarshalTypeErr *json.UnmarshalTypeError

		switch {
		case errors.As(err, &syntaxErr):
			return fmt.Errorf("sintaxis JSON invalida en posicion %d", syntaxErr.Offset)

		case errors.Is(err, io.ErrUnexpectedEOF):
			return errors.New("cuerpo JSON incompleto")

		case errors.As(err, &unmarshalTypeErr):
			return fmt.Errorf("tipo incorrecto para el campo '%s' en posicion %d", unmarshalTypeErr.Field, unmarshalTypeErr.Offset)

		case strings.HasPrefix(err.Error(), "json: unknown field "):
			fieldName := strings.TrimPrefix(err.Error(), "json: unknown field ")
			return fmt.Errorf("campo desconocido no permitido: %s", fieldName)

		case errors.Is(err, io.EOF):
			return errors.New("el cuerpo de la solicitud no puede estar vacio")

		case err.Error() == "http: request body too large":
			return fmt.Errorf("el tamano del cuerpo excede el limite maximo de %d bytes", maxBytes)

		default:
			return fmt.Errorf("error al decodificar JSON: %w", err)
		}
	}

	// Asegurar que solo haya un único valor JSON en el cuerpo
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("el cuerpo contiene datos adicionales despues del valor JSON principal")
	}

	return nil
}
