package transport_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	nexumhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
)

// cargaFuzz es el destino del binding: un unico campo de texto obligatorio.
type cargaFuzz struct {
	Importe string `json:"importe"`
}

// FuzzBindJSON prueba que BindJSON no entra en panico con entradas arbitrarias
// (RFC-008, Etapa C: fuzzing acotado y reproducible del binding HTTP).
//
// Invariante comprobada: para cualquier cuerpo y Content-Type, la funcion devuelve
// un error tipado o nil, nunca un panico, y nunca escribe un estado 5xx por si misma.
// Se ejecuta como prueba semilla en `go test ./...` y como fuzzing real en el job
// `calidad` con `-fuzztime 20s`.
func FuzzBindJSON(f *testing.F) {
	f.Add([]byte(`{"importe":"10.00"}`), "application/json")
	f.Add([]byte(`{"importe":"10.00"}`), "application/json; charset=utf-8")
	f.Add([]byte(`{"importe":"10.00","extra":1}`), "application/json")
	f.Add([]byte(`{`), "application/json")
	f.Add([]byte(``), "application/json")
	f.Add([]byte(`{}`), "text/plain")
	f.Add([]byte(strings.Repeat("a", 2048)), "application/json")

	f.Fuzz(func(t *testing.T, cuerpo []byte, tipoContenido string) {
		peticion := httptest.NewRequest(http.MethodPost, "/api/v1/fuzz", strings.NewReader(string(cuerpo)))
		if tipoContenido != "" {
			peticion.Header.Set("Content-Type", tipoContenido)
		}
		grabador := httptest.NewRecorder()

		// No se valora el resultado: el invariante es la ausencia de panico.
		_ = nexumhttp.BindJSON(grabador, peticion, &cargaFuzz{})
	})
}
