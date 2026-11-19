package transport_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	appGoals "github.com/nexumpr/Nexum-Backend/internal/application/goals"
	internalhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
)

// Pruebas HTTP de E038 (`GET /api/v1/goals`): 200 con página, cursor y `totals` sobre la
// colección completa, 400 de parámetros, 401 sin sesión y 503 fail-closed. El doble con estado y
// los sembrados viven en goals_queries_test.go (mismo paquete).

func e038Handler(store appGoals.Store) http.Handler {
	return internalhttp.NewGoalsListHandler(e038HTTPServicio(store), e004TokensValidos(), newE004Sesiones(), silencioso())
}

func e038Peticion(query string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/goals"+query, nil)
	req.Header.Set("Authorization", "Bearer jwt-valido")
	return req
}

// alterarCursor cambia un carácter del cursor conservando el alfabeto base64url, de modo que el
// caso «manipulado» falle siempre por firma y no dependa de que el último carácter sea distinto.
func alterarCursor(cursor string) string {
	alterado := []byte(cursor)
	if alterado[5] == 'A' {
		alterado[5] = 'B'
	} else {
		alterado[5] = 'A'
	}
	return string(alterado)
}

// TestE038HTTP200 comprueba la página, el cursor y los totales de toda la colección.
func TestE038HTTP200(t *testing.T) {
	store := e040Coleccion(t)
	rec := httptest.NewRecorder()
	e038Handler(store).ServeHTTP(rec, e038Peticion("?limit=2"))

	if rec.Code != http.StatusOK {
		t.Fatalf("esperado 200, obtenido %d (%s)", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Cache-Control"); got != "private, no-store" {
		t.Fatalf("Cache-Control inesperado: %q", got)
	}
	var cuerpo struct {
		Items []struct {
			ID           string `json:"id"`
			OpeningSaved string `json:"openingSaved"`
		} `json:"items"`
		NextCursor string `json:"nextCursor"`
		Totals     struct {
			ActiveCount int    `json:"activeCount"`
			Saved       string `json:"saved"`
		} `json:"totals"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &cuerpo); err != nil {
		t.Fatalf("respuesta no es JSON: %v", err)
	}
	if len(cuerpo.Items) != 2 {
		t.Fatalf("la pagina debe traer 2 metas, trajo %d", len(cuerpo.Items))
	}
	if cuerpo.NextCursor == "" {
		t.Fatal("con cuatro metas y pagina de dos debe haber nextCursor")
	}
	if cuerpo.Totals.ActiveCount != 3 || cuerpo.Totals.Saved != "355.00" {
		t.Fatalf("totales inesperados: %+v", cuerpo.Totals)
	}
	for _, item := range cuerpo.Items {
		if item.OpeningSaved == "" {
			t.Fatalf("cada item debe incluir openingSaved: %+v", item)
		}
	}

	// Segunda página: el cursor emitido devuelve las dos metas restantes sin repetir ninguna.
	seguimiento := httptest.NewRecorder()
	e038Handler(store).ServeHTTP(seguimiento, e038Peticion("?limit=2&cursor="+cuerpo.NextCursor))
	if seguimiento.Code != http.StatusOK {
		t.Fatalf("la segunda pagina debe responder 200, obtuvo %d (%s)", seguimiento.Code, seguimiento.Body.String())
	}
	var segunda struct {
		Items      []map[string]any `json:"items"`
		NextCursor string           `json:"nextCursor"`
	}
	if err := json.Unmarshal(seguimiento.Body.Bytes(), &segunda); err != nil {
		t.Fatalf("la segunda pagina no es JSON: %v", err)
	}
	if len(segunda.Items) != 2 || segunda.NextCursor != "" {
		t.Fatalf("la segunda pagina debe traer las dos restantes sin cursor: %d %q", len(segunda.Items), segunda.NextCursor)
	}
}

// TestE038HTTP400 cubre límite, estado y cursor inválidos.
func TestE038HTTP400(t *testing.T) {
	store := e040Coleccion(t)
	handler := e038Handler(store)

	primera := httptest.NewRecorder()
	handler.ServeHTTP(primera, e038Peticion("?limit=2"))
	var conCursor struct {
		NextCursor string `json:"nextCursor"`
	}
	if err := json.Unmarshal(primera.Body.Bytes(), &conCursor); err != nil || conCursor.NextCursor == "" {
		t.Fatalf("se necesita un cursor valido: %v", err)
	}
	manipulado := alterarCursor(conCursor.NextCursor)

	casos := []struct {
		nombre string
		query  string
	}{
		{"limite excesivo", "?limit=101"},
		{"limite no numerico", "?limit=abc"},
		{"limite cero", "?limit=0"},
		{"estado fuera del enum", "?status=pendiente"},
		{"cursor corto", "?cursor=corto"},
		{"cursor manipulado", "?limit=2&cursor=" + manipulado},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, e038Peticion(caso.query))
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("esperado 400, obtenido %d (%s)", rec.Code, rec.Body.String())
			}
			if codigo := e039CodigoError(t, rec); codigo != "INVALID_REQUEST" {
				t.Fatalf("codigo inesperado: %q", codigo)
			}
		})
	}
}

// TestE038HTTPFiltroYFalloCerrado comprueba el filtro por estado y los 503.
func TestE038HTTPFiltroYFalloCerrado(t *testing.T) {
	store := e040Coleccion(t)
	rec := httptest.NewRecorder()
	e038Handler(store).ServeHTTP(rec, e038Peticion("?status=archived"))
	if rec.Code != http.StatusOK {
		t.Fatalf("esperado 200, obtenido %d (%s)", rec.Code, rec.Body.String())
	}
	var cuerpo struct {
		Items []struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"items"`
		Totals struct {
			ActiveCount int `json:"activeCount"`
		} `json:"totals"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &cuerpo); err != nil {
		t.Fatalf("respuesta no es JSON: %v", err)
	}
	if len(cuerpo.Items) != 1 || cuerpo.Items[0].Status != "archived" {
		t.Fatalf("el filtro debe devolver solo archivadas: %+v", cuerpo.Items)
	}
	if cuerpo.Totals.ActiveCount != 3 {
		t.Fatalf("los totales no dependen del filtro: %+v", cuerpo.Totals)
	}

	t.Run("sin bearer", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/goals", nil)
		recSinCredenciales := httptest.NewRecorder()
		e038Handler(store).ServeHTTP(recSinCredenciales, req)
		if recSinCredenciales.Code != http.StatusUnauthorized {
			t.Fatalf("esperado 401, obtenido %d", recSinCredenciales.Code)
		}
	})
	t.Run("sin clave de cursor", func(t *testing.T) {
		servicio := appGoals.NewService(store).WithClock(func() time.Time { return e038HTTPAhora })
		handler := internalhttp.NewGoalsListHandler(servicio, e004TokensValidos(), newE004Sesiones(), silencioso())
		recSinClave := httptest.NewRecorder()
		handler.ServeHTTP(recSinClave, e038Peticion(""))
		if recSinClave.Code != http.StatusServiceUnavailable {
			t.Fatalf("esperado 503, obtenido %d", recSinClave.Code)
		}
	})
	t.Run("fallo de lista", func(t *testing.T) {
		storeLista := e040Coleccion(t)
		storeLista.errLista = errors.New("dynamodb inaccesible")
		recLista := httptest.NewRecorder()
		e038Handler(storeLista).ServeHTTP(recLista, e038Peticion(""))
		if recLista.Code != http.StatusServiceUnavailable {
			t.Fatalf("esperado 503, obtenido %d", recLista.Code)
		}
	})
	t.Run("fallo de totales", func(t *testing.T) {
		storeTotal := e040Coleccion(t)
		storeTotal.errTotal = errors.New("dynamodb inaccesible")
		recTotal := httptest.NewRecorder()
		e038Handler(storeTotal).ServeHTTP(recTotal, e038Peticion(""))
		if recTotal.Code != http.StatusServiceUnavailable {
			t.Fatalf("esperado 503, obtenido %d", recTotal.Code)
		}
	})
}
