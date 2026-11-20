package main

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	nexumhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
)

// rutaOleada3 describe una operación autorizada de la Oleada 3 con su ruta de contrato
// (plantilla, relativa a /api/v1) y una ruta concreta válida para ejercitar el routing.
type rutaOleada3 struct {
	id        string
	metodo    string
	plantilla string
	concreta  string
}

// TestMontarOleada3RutasRespondenFailClosedSinJWT acredita ÚNICAMENTE registro/routing y
// fail-closed. NO es una prueba funcional positiva: no hay JWT válido, persistencia, casos de
// negocio ni datos reales. Monta las 13 operaciones en un `nexumhttp.Router` real con
// dependencias mínimas (Router y un logger que descarta; sin DynamoDB, Redis, tokens ni
// autorizador) y exige que cada método+ruta exactos respondan 401/503 —nunca 404—. El 503 se
// debe a que los handlers fallan cerrados sin componentes de autenticación; una ruta ausente
// produciría 404, que se rechaza aquí explícitamente.
//
// Además comprueba que un método distinto del contratado devuelve 405 con `Allow` (lo que
// demuestra que el método exacto quedó registrado) y que una ruta inexistente sí devuelve 404
// (control negativo que da sentido a la aserción "nunca 404").
func TestMontarOleada3RutasRespondenFailClosedSinJWT(t *testing.T) {
	casos := []rutaOleada3{
		{"E020", http.MethodGet, "/users/me/export", "/users/me/export"},
		{"E026", http.MethodGet, "/financial-summary", "/financial-summary"},
		{"E037", http.MethodPost, "/recurring-payments/{recurringPaymentId}/payments", "/recurring-payments/rp-0001/payments"},
		{"E042", http.MethodDelete, "/goals/{goalId}", "/goals/goal-0001"},
		{"E043", http.MethodPost, "/goal-contributions/preview", "/goal-contributions/preview"},
		{"E044", http.MethodPost, "/goal-contributions", "/goal-contributions"},
		{"E045", http.MethodGet, "/goal-contributions", "/goal-contributions"},
		{"E046", http.MethodGet, "/goal-contributions/{contributionId}", "/goal-contributions/contrib-0001"},
		{"E047", http.MethodPost, "/goal-contributions/{contributionId}/reverse", "/goal-contributions/contrib-0001/reverse"},
		{"E048", http.MethodGet, "/budgets", "/budgets"},
		{"E049", http.MethodPut, "/budgets", "/budgets"},
		{"E050", http.MethodGet, "/budgets/usage", "/budgets/usage"},
		{"E051", http.MethodGet, "/dashboard", "/dashboard"},
	}
	if len(casos) != len(endpointsOleada3Autorizados) {
		t.Fatalf("la tabla de rutas debe cubrir las %d operaciones autorizadas, cubre %d", len(endpointsOleada3Autorizados), len(casos))
	}
	vistos := make(map[string]struct{}, len(casos))
	for _, caso := range casos {
		clave := caso.metodo + " " + caso.plantilla
		if _, duplicado := vistos[clave]; duplicado {
			t.Fatalf("operación repetida en la tabla de rutas: %s", clave)
		}
		vistos[clave] = struct{}{}
	}

	router := nexumhttp.NewRouter()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	montarOleada3(&dependenciasOleada3{Router: router, Logger: logger})

	// Métodos contratados por ruta concreta, para elegir un método NO registrado en la sonda.
	metodosPorRuta := make(map[string]map[string]bool, len(casos))
	for _, caso := range casos {
		if metodosPorRuta[caso.concreta] == nil {
			metodosPorRuta[caso.concreta] = map[string]bool{}
		}
		metodosPorRuta[caso.concreta][caso.metodo] = true
	}

	for _, caso := range casos {
		req := httptest.NewRequest(caso.metodo, "/api/v1"+caso.concreta, nil)
		// Sin Authorization: la petición no lleva JWT por diseño.
		if req.Header.Get("Authorization") != "" {
			t.Fatalf("%s: la petición de prueba no debe llevar Authorization", caso.id)
		}
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code == http.StatusNotFound {
			t.Fatalf("%s (%s %s): ruta NO registrada (404); debía responder 401/503", caso.id, caso.metodo, caso.plantilla)
		}
		codigo := codigoError(t, caso.id, rec)
		switch rec.Code {
		case http.StatusUnauthorized:
			if codigo != "UNAUTHORIZED" {
				t.Fatalf("%s: 401 con código %q, se esperaba UNAUTHORIZED", caso.id, codigo)
			}
		case http.StatusServiceUnavailable:
			if codigo != "DEPENDENCY_UNAVAILABLE" {
				t.Fatalf("%s: 503 con código %q, se esperaba DEPENDENCY_UNAVAILABLE", caso.id, codigo)
			}
		default:
			t.Fatalf("%s (%s %s): estado %d; sin JWT y con dependencias mínimas solo se admite 401/503, nunca %d",
				caso.id, caso.metodo, caso.plantilla, rec.Code, rec.Code)
		}
	}

	// Método no contratado: el Router debe responder 405 y anunciar el método real en `Allow`.
	for concreta, permitidos := range metodosPorRuta {
		var sonda string
		for _, candidato := range []string{http.MethodPatch, http.MethodDelete, http.MethodPost, http.MethodPut, http.MethodGet, http.MethodHead, http.MethodOptions} {
			if !permitidos[candidato] {
				sonda = candidato
				break
			}
		}
		if sonda == "" {
			t.Fatalf("%s: no se encontró un método no contratado para la sonda", concreta)
		}
		req := httptest.NewRequest(sonda, "/api/v1"+concreta, nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("%s %s: estado %d, se esperaba 405 (método no contratado)", sonda, concreta, rec.Code)
		}
		allow := rec.Header().Get("Allow")
		for metodo := range permitidos {
			if !strings.Contains(allow, metodo) {
				t.Fatalf("%s: Allow=%q no anuncia el método contratado %s", concreta, allow, metodo)
			}
		}
	}

	// Control negativo: el Router sí emite 404 para rutas no registradas, por lo que la
	// aserción "nunca 404" anterior es significativa.
	reqInexistente := httptest.NewRequest(http.MethodGet, "/api/v1/__oleada3_no_registrada__", nil)
	recInexistente := httptest.NewRecorder()
	router.ServeHTTP(recInexistente, reqInexistente)
	if recInexistente.Code != http.StatusNotFound {
		t.Fatalf("control negativo: una ruta no registrada devolvió %d, se esperaba 404", recInexistente.Code)
	}
}

// codigoError decodifica el envelope de error canónico y devuelve su `error.code`.
func codigoError(t *testing.T, id string, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var envelope struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("%s: respuesta no es un envelope JSON de error: %v (cuerpo %q)", id, err, rec.Body.String())
	}
	if envelope.Error.Code == "" {
		t.Fatalf("%s: envelope de error sin código: %q", id, rec.Body.String())
	}
	return envelope.Error.Code
}
