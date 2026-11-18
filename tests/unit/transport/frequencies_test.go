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

// Pruebas HTTP de E023 (`GET /api/v1/catalogs/frequencies`): contrato exacto, caché
// pública fail-open y cuota por IP. El catálogo es estático: no hay DynamoDB ni JWT en el
// camino.

const e023Ruta = "/api/v1/catalogs/frequencies"

// e023Cuota es el doble del limitador distribuido: registra la clase usada y permite
// simular denegación y fallo de Redis.
type e023Cuota struct {
	permitido bool
	retry     time.Duration
	err       error
	clases    []string
}

func (l *e023Cuota) Allow(_ context.Context, clase, _ string) (bool, time.Duration, error) {
	l.clases = append(l.clases, clase)
	return l.permitido, l.retry, l.err
}

// e023Peticion construye la petición GET con la cabecera de cadena de proxies que usa la
// política de IP efectiva.
func e023Peticion() *http.Request {
	peticion := httptest.NewRequest(http.MethodGet, e023Ruta, nil)
	peticion.Header.Set("X-Forwarded-For", "203.0.113.10")
	return peticion
}

// TestE023HTTPCatalogoExacto cubre el contrato: 200, esquema `PageOfFrequency` con
// exactamente `items` (sin `nextCursor`), siete ítems canónicos en orden, cada uno con
// exactamente `id` y `label`, y ningún alias heredado.
func TestE023HTTPCatalogoExacto(t *testing.T) {
	handler := internalhttp.NewFrequenciesHandler(nil, nil, silencioso())

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, e023Peticion())

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
		t.Fatal("el contrato de E023 no emite nextCursor")
	}

	var items []map[string]any
	if err := json.Unmarshal(cuerpo["items"], &items); err != nil {
		t.Fatalf("items no es un array de objetos: %v", err)
	}
	if len(items) != 7 {
		t.Fatalf("esperadas 7 frecuencias, obtenidas %d", len(items))
	}
	for indice, item := range items {
		if len(item) != 2 {
			t.Fatalf("la frecuencia %d debe tener exactamente 2 campos, tiene %d: %v", indice, len(item), item)
		}
		if _, existe := item["id"]; !existe {
			t.Fatalf("la frecuencia %d no incluye `id`: %v", indice, item)
		}
		if _, existe := item["label"]; !existe {
			t.Fatalf("la frecuencia %d no incluye `label`: %v", indice, item)
		}
	}

	esperado := []struct{ id, label string }{
		{"weekly", "Semanal"},
		{"fortnightly", "Quincenal"},
		{"monthly", "Mensual"},
		{"bimonthly", "Bimestral"},
		{"quarterly", "Trimestral"},
		{"semiannual", "Semestral"},
		{"annual", "Anual"},
	}
	for indice, frecuencia := range esperado {
		if items[indice]["id"] != frecuencia.id || items[indice]["label"] != frecuencia.label {
			t.Fatalf("posicion %d: esperado %q/%q, obtenido %v", indice, frecuencia.id, frecuencia.label, items[indice])
		}
	}
}

// TestE023HTTPNoEmiteAlias comprueba que `biweekly` y `yearly` no aparecen ni como id ni
// como etiqueta: siguen en el enum del esquema publicado, pero no son ítems del catálogo.
func TestE023HTTPNoEmiteAlias(t *testing.T) {
	handler := internalhttp.NewFrequenciesHandler(nil, nil, silencioso())
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, e023Peticion())

	cuerpo := rec.Body.String()
	for _, alias := range []string{catalogs.AliasBiweekly, catalogs.AliasYearly} {
		if strings.Contains(cuerpo, `"`+alias+`"`) {
			t.Fatalf("el catalogo emitio el alias heredado %q: %s", alias, cuerpo)
		}
	}
}

// TestE023HTTPUsaCachePublica comprueba que el handler consulta la clave versionada de
// frecuencias con el TTL del contrato y sirve el contenido cacheado cuando es válido.
func TestE023HTTPUsaCachePublica(t *testing.T) {
	cacheado := []byte(`{"items":[{"id":"monthly","label":"Mensual"}]}`)
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

	handler := internalhttp.NewFrequenciesHandler(nil, cargar, silencioso())
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, e023Peticion())

	if rec.Code != http.StatusOK {
		t.Fatalf("esperado 200, obtenido %d", rec.Code)
	}
	if claveUsada != catalogs.FrequenciesCacheKey {
		t.Fatalf("clave de cache inesperada: %q", claveUsada)
	}
	if claveUsada == catalogs.CategoriesCacheKey {
		t.Fatal("E023 no puede compartir la clave de cache de E022")
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

// TestE023HTTPCacheFailOpen demuestra que un fallo de Redis y una entrada corrupta no
// convierten una lectura pública en una caída: se sirve el catálogo desde memoria.
func TestE023HTTPCacheFailOpen(t *testing.T) {
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
		{
			nombre: "catalogo de categorias ajeno",
			cargar: func(context.Context, string, time.Duration, func() ([]byte, error)) ([]byte, error) {
				return []byte(`{"items":[{"id":"vivienda","name":"Vivienda","type":"expense"}]}`), nil
			},
		},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			handler := internalhttp.NewFrequenciesHandler(nil, caso.cargar, silencioso())
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, e023Peticion())
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

// TestE023HTTPCuotaExcedida verifica el 429 del contrato con `Retry-After` y sobre
// `ErrorEnvelope`, usando la clase `e023` del Gate D-429.
func TestE023HTTPCuotaExcedida(t *testing.T) {
	cuota := &e023Cuota{permitido: false, retry: 5 * time.Second}
	handler := internalhttp.NewFrequenciesHandler(cuota, nil, silencioso())

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, e023Peticion())

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("esperado 429, obtenido %d", rec.Code)
	}
	if retry := rec.Header().Get("Retry-After"); retry != "5" {
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
	if envelope.Error.Code != "TOO_MANY_REQUESTS" {
		t.Fatalf("codigo de error inesperado: %q", envelope.Error.Code)
	}
	if len(cuota.clases) != 1 || cuota.clases[0] != "e023" {
		t.Fatalf("clase de cuota inesperada: %v", cuota.clases)
	}
}

// TestE023HTTPCuotaFailOpen: un error de Redis no bloquea la lectura pública (A-080).
func TestE023HTTPCuotaFailOpen(t *testing.T) {
	cuota := &e023Cuota{permitido: false, err: errors.New("redis caido")}
	handler := internalhttp.NewFrequenciesHandler(cuota, nil, silencioso())

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, e023Peticion())

	if rec.Code != http.StatusOK {
		t.Fatalf("un fallo de cuota debe ser fail-open, obtenido %d", rec.Code)
	}
}

// TestE023HTTPHandlerIncompletoFallaCerrado: sin contenido serializado el handler responde
// 503 en lugar de un 200 vacío.
func TestE023HTTPHandlerIncompletoFallaCerrado(t *testing.T) {
	handler := &internalhttp.CatalogHandler{}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, e023Peticion())
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("esperado 503, obtenido %d", rec.Code)
	}
}
