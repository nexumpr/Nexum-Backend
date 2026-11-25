package transport_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	internalhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
	"github.com/nexumpr/Nexum-Backend/internal/transport/http/middleware"
	"github.com/nexumpr/Nexum-Backend/pkg/httputil"
)

// Regresion contractual de las 75 operaciones del OpenAPI contra el router real.
//
// No requiere AWS ni red: el router se puebla con handlers sonda. Las operaciones publicas
// (`security: []`) deben alcanzar el handler (200); las 62 con `BearerAuth` deben responder 401
// sin JWT, sin ejecutar el handler. Una ruta inexistente responde 404. No se mutan datos: los
// handlers sonda no escriben nada.

const openAPIParamSustituto = "11111111-2222-4333-8444-555555555555"

var openAPIParametro = regexp.MustCompile(`\{[^/{}]+\}`)

// openAPIOperacion es una operacion del contrato con su seguridad efectiva.
type openAPIOperacion struct {
	OperationID string
	Metodo      string
	Ruta        string
	Publica     bool
}

func cargarOperacionesOpenAPI(t *testing.T) []openAPIOperacion {
	t.Helper()
	candidatos := []string{
		filepath.Join("..", "..", "..", "sdd", "03-endpoints", "openapi", "nexum-api-v1.json"),
		filepath.Join("sdd", "03-endpoints", "openapi", "nexum-api-v1.json"),
		filepath.Join("..", "..", "sdd", "03-endpoints", "openapi", "nexum-api-v1.json"),
	}
	var datos []byte
	var err error
	for _, candidato := range candidatos {
		datos, err = os.ReadFile(candidato)
		if err == nil {
			break
		}
	}
	if err != nil {
		t.Fatalf("no se pudo leer el OpenAPI JSON: %v", err)
	}

	var documento struct {
		Security []map[string][]string `json:"security"`
		Paths    map[string]map[string]struct {
			OperationID string                `json:"operationId"`
			Security    []map[string][]string `json:"security"`
		} `json:"paths"`
	}
	if err := json.Unmarshal(datos, &documento); err != nil {
		t.Fatalf("OpenAPI JSON invalido: %v", err)
	}

	var operaciones []openAPIOperacion
	for ruta, metodos := range documento.Paths {
		for metodo, operacion := range metodos {
			if operacion.OperationID == "" {
				continue
			}
			seguridad := operacion.Security
			if seguridad == nil {
				seguridad = documento.Security
			}
			operaciones = append(operaciones, openAPIOperacion{
				OperationID: operacion.OperationID,
				Metodo:      strings.ToUpper(metodo),
				Ruta:        ruta,
				Publica:     len(seguridad) == 0,
			})
		}
	}
	if len(operaciones) != 75 {
		t.Fatalf("el contrato debe declarar 75 operaciones, hay %d", len(operaciones))
	}
	return operaciones
}

func TestOpenAPIRouterSetentaYCincoOperaciones(t *testing.T) {
	operaciones := cargarOperacionesOpenAPI(t)

	router := internalhttp.NewRouter()
	// Sonda unica: no escribe ni consulta nada; solo acredita que el router resuelve la ruta y
	// que el middleware de autenticacion deja pasar (publicas) o corta (protegidas).
	sonda := func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }
	protegida := middleware.AuthenticateBearer(&e004Tokens{})(http.HandlerFunc(sonda))

	for _, operacion := range operaciones {
		if operacion.Publica {
			router.HandleFunc(operacion.Metodo, operacion.Ruta, sonda)
			continue
		}
		router.HandleFunc(operacion.Metodo, operacion.Ruta, protegida.ServeHTTP)
	}

	rutas := make(map[string]bool, len(operaciones))
	for _, operacion := range operaciones {
		rutas[operacion.Ruta] = true
	}

	for _, operacion := range operaciones {
		t.Run(operacion.OperationID, func(t *testing.T) {
			ruta := openAPIParametro.ReplaceAllString(operacion.Ruta, openAPIParamSustituto)
			peticion := httptest.NewRequest(operacion.Metodo, ruta, nil)
			peticion.Header.Set("X-Request-ID", traceEnvelopeRequestID)
			rec := httptest.NewRecorder()

			router.ServeHTTP(rec, peticion)

			if operacion.Publica {
				if rec.Code != http.StatusOK {
					t.Fatalf("%s %s publica: esperado 200, obtenido %d", operacion.Metodo, ruta, rec.Code)
				}
				return
			}
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("%s %s protegida sin JWT: esperado 401, obtenido %d", operacion.Metodo, ruta, rec.Code)
			}
			var envelope httputil.ErrorEnvelope
			if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
				t.Fatalf("el 401 no es un ErrorEnvelope: %v", err)
			}
			if envelope.Error.TraceID != traceEnvelopeRequestID {
				t.Fatalf("el 401 debe llevar traceId: %q", envelope.Error.TraceID)
			}
		})
	}

	// Controles negativos del router: ruta inexistente y metodo no permitido.
	t.Run("ruta inexistente responde 404", func(t *testing.T) {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/ruta-inexistente", nil))
		if rec.Code != http.StatusNotFound {
			t.Fatalf("esperado 404, obtenido %d", rec.Code)
		}
	})
	t.Run("metodo no permitido responde 405", func(t *testing.T) {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/health/live", nil))
		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("esperado 405, obtenido %d", rec.Code)
		}
	})
	if len(rutas) != 55 {
		t.Fatalf("el contrato debe declarar 55 rutas, hay %d", len(rutas))
	}
}
