package transport_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/nexumpr/Nexum-Backend/internal/application/catalogs"
	internalhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
)

// Pruebas HTTP de E022 (`GET /api/v1/catalogs/categories`): contrato exacto, caché pública
// fail-open y cuota por IP. El catálogo es estático: no hay DynamoDB ni JWT en el camino.

const e022Ruta = "/api/v1/catalogs/categories"

// e022Cuota es el doble del limitador distribuido: registra la clase usada y permite
// simular denegación y fallo de Redis.
type e022Cuota struct {
	permitido bool
	retry     time.Duration
	err       error
	clases    []string
}

func (l *e022Cuota) Allow(_ context.Context, clase, _ string) (bool, time.Duration, error) {
	l.clases = append(l.clases, clase)
	return l.permitido, l.retry, l.err
}

// e022Peticion construye la petición GET con la cabecera de cadena de proxies que usa la
// política de IP efectiva.
func e022Peticion() *http.Request {
	peticion := httptest.NewRequest(http.MethodGet, e022Ruta, nil)
	peticion.RemoteAddr = "10.42.1.10:443"
	peticion.Header.Set("X-Forwarded-For", "203.0.113.10, 130.176.10.20")
	return peticion
}

// TestE022HTTPCatalogoExacto cubre T01: 200, esquema `PageOfCategory` con exactamente
// `items` (sin `nextCursor`, sin paginación) y cada ítem con exactamente tres campos.
func TestE022HTTPCatalogoExacto(t *testing.T) {
	handler := internalhttp.NewCategoriesHandler(nil, nil, silencioso())

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, e022Peticion())

	if rec.Code != http.StatusOK {
		t.Fatalf("esperado 200, obtenido %d (%s)", rec.Code, rec.Body.String())
	}
	if tipo := rec.Header().Get("Content-Type"); !strings.HasPrefix(tipo, "application/json") {
		t.Fatalf("Content-Type inesperado: %q", tipo)
	}
	if cache := rec.Header().Get("Cache-Control"); cache != "public, max-age=300" {
		t.Fatalf("Cache-Control inesperado: %q", cache)
	}

	var cuerpo map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &cuerpo); err != nil {
		t.Fatalf("respuesta no es JSON: %v", err)
	}
	if len(cuerpo) != 1 {
		t.Fatalf("el cuerpo debe tener solo `items`, tiene %d claves: %s", len(cuerpo), rec.Body.String())
	}
	if _, existe := cuerpo["nextCursor"]; existe {
		t.Fatal("el contrato de E022 no emite nextCursor")
	}

	var items []map[string]any
	if err := json.Unmarshal(cuerpo["items"], &items); err != nil {
		t.Fatalf("items no es un array de objetos: %v", err)
	}
	if len(items) != 7 {
		t.Fatalf("esperadas 7 categorias, obtenidas %d", len(items))
	}
	for indice, item := range items {
		if len(item) != 3 {
			t.Fatalf("la categoria %d debe tener exactamente 3 campos, tiene %d: %v", indice, len(item), item)
		}
		for _, campo := range []string{"id", "name", "type"} {
			if _, existe := item[campo]; !existe {
				t.Fatalf("la categoria %d no incluye %q: %v", indice, campo, item)
			}
		}
	}

	primera := items[0]
	if primera["id"] != "vivienda" || primera["name"] != "Vivienda" || primera["type"] != "expense" {
		t.Fatalf("primera categoria inesperada: %v", primera)
	}
	ultima := items[len(items)-1]
	if ultima["id"] != "ahorro" || ultima["type"] != "saving" {
		t.Fatalf("ultima categoria inesperada: %v", ultima)
	}
}

// TestE022HTTPUsaCachePublica comprueba que el handler consulta la clave versionada con el
// TTL del contrato y sirve el contenido cacheado cuando es un catálogo válido.
func TestE022HTTPUsaCachePublica(t *testing.T) {
	cacheado := []byte(`{"items":[{"id":"cacheado","name":"Cacheado","type":"expense"}]}`)
	var (
		claveUsada string
		ttlUsado   time.Duration
		cargas     int
	)
	cargar := func(_ context.Context, clave string, ttl time.Duration, load func() ([]byte, error)) ([]byte, error) {
		claveUsada = clave
		ttlUsado = ttl
		cargas++
		if _, err := load(); err != nil {
			t.Fatalf("la carga de respaldo no debe fallar: %v", err)
		}
		return cacheado, nil
	}

	handler := internalhttp.NewCategoriesHandler(nil, cargar, silencioso())
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, e022Peticion())

	if rec.Code != http.StatusOK {
		t.Fatalf("esperado 200, obtenido %d", rec.Code)
	}
	if claveUsada != catalogs.CategoriesCacheKey {
		t.Fatalf("clave de cache inesperada: %q", claveUsada)
	}
	if ttlUsado != catalogs.CacheTTL {
		t.Fatalf("TTL inesperado: %s", ttlUsado)
	}
	if cargas != 1 {
		t.Fatalf("se esperaba una lectura de cache por peticion, hubo %d", cargas)
	}
	if rec.Body.String() != string(cacheado) {
		t.Fatalf("no se sirvio el contenido cacheado: %s", rec.Body.String())
	}
}

// TestE022HTTPCacheFailOpen demuestra que un fallo de Redis y una entrada corrupta no
// convierten una lectura pública en una caída: se sirve el catálogo desde memoria.
func TestE022HTTPCacheFailOpen(t *testing.T) {
	casos := []struct {
		nombre string
		cargar func(context.Context, string, time.Duration, func() ([]byte, error)) ([]byte, error)
	}{
		{
			nombre: "redis caido",
			cargar: func(context.Context, string, time.Duration, func() ([]byte, error)) ([]byte, error) {
				return nil, errors.New("redis caido")
			},
		},
		{
			nombre: "entrada corrupta",
			cargar: func(context.Context, string, time.Duration, func() ([]byte, error)) ([]byte, error) {
				return []byte(`{"items":"no-es-un-array"}`), nil
			},
		},
		{
			nombre: "entrada no json",
			cargar: func(context.Context, string, time.Duration, func() ([]byte, error)) ([]byte, error) {
				return []byte(`no-json`), nil
			},
		},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			handler := internalhttp.NewCategoriesHandler(nil, caso.cargar, silencioso())
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, e022Peticion())
			if rec.Code != http.StatusOK {
				t.Fatalf("esperado 200 fail-open, obtenido %d", rec.Code)
			}
			var cuerpo struct {
				Items []map[string]any `json:"items"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &cuerpo); err != nil {
				t.Fatalf("respuesta no es JSON: %v", err)
			}
			if len(cuerpo.Items) != 7 {
				t.Fatalf("se esperaba el catalogo completo, obtenidos %d items", len(cuerpo.Items))
			}
		})
	}
}

// TestE022HTTPCuotaExcedida verifica el 429 del contrato con `Retry-After` y sobre
// `ErrorEnvelope`, usando la clase `e022` del Gate D-429.
func TestE022HTTPCuotaExcedida(t *testing.T) {
	cuota := &e022Cuota{permitido: false, retry: 7 * time.Second}
	handler := internalhttp.NewCategoriesHandler(cuota, nil, silencioso())

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, e022Peticion())

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("esperado 429, obtenido %d", rec.Code)
	}
	if retry := rec.Header().Get("Retry-After"); retry != "7" {
		t.Fatalf("Retry-After inesperado: %q", retry)
	}
	var envelope struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
			Details []any  `json:"details"`
			TraceID string `json:"traceId"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("respuesta no es JSON: %v", err)
	}
	if envelope.Error.Code != "RATE_LIMITED" {
		t.Fatalf("codigo de error inesperado: %q", envelope.Error.Code)
	}
	if len(cuota.clases) != 1 || cuota.clases[0] != "e022" {
		t.Fatalf("clase de cuota inesperada: %v", cuota.clases)
	}
}

// TestE022HTTPCuotaFailOpen: un error de Redis no bloquea la lectura pública (A-080).
func TestE022HTTPCuotaFailOpen(t *testing.T) {
	cuota := &e022Cuota{permitido: false, err: errors.New("redis caido")}
	handler := internalhttp.NewCategoriesHandler(cuota, nil, silencioso())

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, e022Peticion())

	if rec.Code != http.StatusOK {
		t.Fatalf("un fallo de cuota debe ser fail-open, obtenido %d", rec.Code)
	}
}

// TestE022HTTPHandlerIncompletoFallaCerrado: sin contenido serializado el handler responde
// 503 en lugar de un 200 vacío.
func TestE022HTTPHandlerIncompletoFallaCerrado(t *testing.T) {
	handler := &internalhttp.CatalogHandler{}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, e022Peticion())
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("esperado 503, obtenido %d", rec.Code)
	}
}
