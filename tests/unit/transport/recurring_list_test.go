package transport_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	appRecurring "github.com/nexumpr/Nexum-Backend/internal/application/recurring"
	domainDynamo "github.com/nexumpr/Nexum-Backend/internal/domain/dynamo"
	domainIdentity "github.com/nexumpr/Nexum-Backend/internal/domain/identity"
	internalhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
	"github.com/nexumpr/Nexum-Backend/pkg/httputil"
)

// Pruebas HTTP de E032 (`GET /api/v1/recurring-payments`): `PageOfRecurringPayment`, paginación
// con cursor firmado sin duplicados, filtros de servidor `active`/`type`, 400 de parámetros y
// de cursor manipulado o reutilizado con otros filtros, 401 sin identidad, 503 fail-closed y
// aislamiento entre usuarios. El doble de persistencia pagina igual que el adaptador real.

const e032Ruta = "/api/v1/recurring-payments"

// e032Store implementa `appRecurring.Store` con estado paginado.
type e032Store struct {
	mu        sync.Mutex
	recursos  map[string][]appRecurring.Payment
	consultas []e032Consulta
	fallo     error
}

type e032Consulta struct {
	UserID  string
	StartSK string
	Limit   int
	Filter  appRecurring.Filter
}

func newE032Store() *e032Store {
	return &e032Store{recursos: map[string][]appRecurring.Payment{}}
}

func (s *e032Store) sembrar(userID string, pagos ...appRecurring.Payment) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.recursos[userID] = append(s.recursos[userID], pagos...)
}

func (s *e032Store) List(_ context.Context, userID, startSK string, limit int, filter appRecurring.Filter) ([]appRecurring.Payment, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.consultas = append(s.consultas, e032Consulta{UserID: userID, StartSK: startSK, Limit: limit, Filter: filter})
	if s.fallo != nil {
		return nil, "", s.fallo
	}
	desde := strings.TrimPrefix(startSK, domainDynamo.PrefixRecurring)
	salida := make([]appRecurring.Payment, 0, limit)
	nextSK := ""
	for _, pago := range s.recursos[userID] {
		if desde != "" && pago.ID <= desde {
			continue
		}
		if filter.Active != nil && pago.Active != *filter.Active {
			continue
		}
		if filter.Type != "" && pago.Type != filter.Type {
			continue
		}
		if len(salida) == limit {
			nextSK = domainDynamo.PrefixRecurring + salida[len(salida)-1].ID
			break
		}
		salida = append(salida, pago)
	}
	return salida, nextSK, nil
}

func (s *e032Store) Create(context.Context, appRecurring.CreateWrite) (string, []byte, bool, error) {
	return "", nil, false, errors.New("Create no usado en E032")
}

func (s *e032Store) Get(context.Context, string, string) (*appRecurring.Payment, error) {
	return nil, errors.New("Get no usado en E032")
}

func (s *e032Store) Update(context.Context, appRecurring.UpdateWrite) (*appRecurring.Payment, error) {
	return nil, errors.New("Update no usado en E032")
}

func (s *e032Store) Deactivate(context.Context, string, string, time.Time) (bool, error) {
	return false, errors.New("Deactivate no usado en E032")
}

// e032Pago construye una regla de prueba con identificador y clasificación deterministas.
func e032Pago(indice int, tipo string, activo bool) appRecurring.Payment {
	dia := 5
	return appRecurring.Payment{
		ID:          fmt.Sprintf("018f90ef-08e5-7a68-b09f-%012d", indice),
		Name:        fmt.Sprintf("Regla %d", indice),
		AmountCents: 100000,
		Currency:    "MXN",
		Frequency:   domainIdentity.FrequencyMonthly,
		NextDueDate: time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC),
		Active:      activo,
		Version:     1,
		Type:        tipo,
		StartDate:   time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC),
		DueDay:      &dia,
	}
}

// e032HTTPHandler compone el handler real con clave de cursor de prueba.
func e032HTTPHandler(store appRecurring.Store) *internalhttp.RecurringListHandler {
	servicio := appRecurring.NewService(store, []byte("clave-de-cursor-de-prueba-e032"))
	return internalhttp.NewRecurringListHandler(servicio, e004TokensValidos(), newE004Sesiones(), nil, silencioso())
}

func e032Peticion(consulta, token string) *http.Request {
	ruta := e032Ruta
	if consulta != "" {
		ruta += "?" + consulta
	}
	return e004Peticion(http.MethodGet, ruta, token, "")
}

// TestE032HTTP200PaginadoSinDuplicados cubre T01 y T07: dos páginas, sin duplicados y con
// `PageOfRecurringPayment` exacto.
func TestE032HTTP200PaginadoSinDuplicados(t *testing.T) {
	store := newE032Store()
	store.sembrar(e004HTTPUsuario, e032Pago(1, "expense", true), e032Pago(2, "expense", true), e032Pago(3, "income", true))
	handler := e032HTTPHandler(store)

	primera := httptest.NewRecorder()
	handler.ServeHTTP(primera, e032Peticion("limit=2", "jwt-valido"))
	if primera.Code != http.StatusOK {
		t.Fatalf("esperado 200, obtenido %d (%s)", primera.Code, primera.Body.String())
	}
	if cache := primera.Header().Get("Cache-Control"); cache != "private, no-store" {
		t.Fatalf("Cache-Control inesperado: %q", cache)
	}
	var pagina1 struct {
		Items []struct {
			ID     string `json:"id"`
			Name   string `json:"name"`
			Amount string `json:"amount"`
		} `json:"items"`
		NextCursor string `json:"nextCursor"`
	}
	if err := json.Unmarshal(primera.Body.Bytes(), &pagina1); err != nil {
		t.Fatalf("respuesta no es JSON: %v", err)
	}
	if len(pagina1.Items) != 2 || pagina1.NextCursor == "" {
		t.Fatalf("se esperaban 2 elementos y cursor: %+v", pagina1)
	}
	if pagina1.Items[0].Amount != "1000.00" {
		t.Fatalf("el importe debe serializarse como cadena decimal: %q", pagina1.Items[0].Amount)
	}

	segunda := httptest.NewRecorder()
	handler.ServeHTTP(segunda, e032Peticion("limit=2&cursor="+pagina1.NextCursor, "jwt-valido"))
	if segunda.Code != http.StatusOK {
		t.Fatalf("pagina 2: %d (%s)", segunda.Code, segunda.Body.String())
	}
	var pagina2 struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
		NextCursor string `json:"nextCursor"`
	}
	if err := json.Unmarshal(segunda.Body.Bytes(), &pagina2); err != nil {
		t.Fatalf("pagina 2 no es JSON: %v", err)
	}
	if len(pagina2.Items) != 1 || pagina2.NextCursor != "" {
		t.Fatalf("la pagina 2 debe agotar la lista: %+v", pagina2)
	}

	vistos := map[string]bool{}
	ids := make([]string, 0, 3)
	for _, item := range pagina1.Items {
		ids = append(ids, item.ID)
	}
	for _, item := range pagina2.Items {
		ids = append(ids, item.ID)
	}
	for _, id := range ids {
		if vistos[id] {
			t.Fatalf("elemento duplicado entre paginas: %s", id)
		}
		vistos[id] = true
	}
	if len(vistos) != 3 {
		t.Fatalf("se esperaban 3 reglas, obtenidas %d", len(vistos))
	}
}

// TestE032HTTPFiltrosDeServidor: `active` y `type` llegan al almacén, no se filtran en el
// cliente.
func TestE032HTTPFiltrosDeServidor(t *testing.T) {
	store := newE032Store()
	store.sembrar(e004HTTPUsuario, e032Pago(1, "expense", true), e032Pago(2, "income", false))
	handler := e032HTTPHandler(store)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, e032Peticion("active=false", "jwt-valido"))
	if rec.Code != http.StatusOK {
		t.Fatalf("esperado 200, obtenido %d (%s)", rec.Code, rec.Body.String())
	}
	var cuerpo struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &cuerpo); err != nil {
		t.Fatalf("respuesta no es JSON: %v", err)
	}
	if len(cuerpo.Items) != 1 || cuerpo.Items[0]["active"] != false {
		t.Fatalf("el filtro active debe aplicarse en servidor: %+v", cuerpo.Items)
	}
	ultima := store.consultas[len(store.consultas)-1]
	if ultima.Filter.Active == nil || *ultima.Filter.Active {
		t.Fatalf("el almacen debe recibir active=false: %+v", ultima.Filter)
	}

	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, e032Peticion("type=income&active=false", "jwt-valido"))
	if rec.Code != http.StatusOK {
		t.Fatalf("esperado 200, obtenido %d (%s)", rec.Code, rec.Body.String())
	}
	ultima = store.consultas[len(store.consultas)-1]
	if ultima.Filter.Type != "income" {
		t.Fatalf("el almacen debe recibir type=income: %+v", ultima.Filter)
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &cuerpo); err != nil {
		t.Fatalf("respuesta no es JSON: %v", err)
	}
	if len(cuerpo.Items) != 1 || cuerpo.Items[0]["type"] != "income" {
		t.Fatalf("el filtro type debe aplicarse en servidor: %+v", cuerpo.Items)
	}
}

// TestE032HTTP400Parametros cubre los rechazos de esquema de la consulta.
func TestE032HTTP400Parametros(t *testing.T) {
	casos := []struct {
		nombre   string
		consulta string
		campo    string
	}{
		{"limit cero", "limit=0", "limit"},
		{"limit excesivo", "limit=101", "limit"},
		{"limit no numerico", "limit=abc", "limit"},
		{"active no booleano", "active=si", "active"},
		{"active numerico", "active=1", "active"},
		{"type fuera del enum", "type=fixed", "type"},
		{"cursor corto", "cursor=abc", "cursor"},
		{"cursor larguisimo", "cursor=" + strings.Repeat("A", 2049), "cursor"},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			store := newE032Store()
			store.sembrar(e004HTTPUsuario, e032Pago(1, "expense", true))
			rec := httptest.NewRecorder()
			e032HTTPHandler(store).ServeHTTP(rec, e032Peticion(caso.consulta, "jwt-valido"))
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("esperado 400, obtenido %d (%s)", rec.Code, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), caso.campo) {
				t.Fatalf("el detalle debe nombrar %q: %s", caso.campo, rec.Body.String())
			}
			if len(store.consultas) != 0 {
				t.Fatal("una consulta invalida no debe llegar a la persistencia")
			}
		})
	}
}

// TestE032HTTP400CursorInvalidoOReutilizado cubre la firma y la ligadura del cursor.
func TestE032HTTP400CursorInvalidoOReutilizado(t *testing.T) {
	store := newE032Store()
	store.sembrar(e004HTTPUsuario, e032Pago(1, "expense", true), e032Pago(2, "expense", true), e032Pago(3, "expense", true))
	handler := e032HTTPHandler(store)

	primera := httptest.NewRecorder()
	handler.ServeHTTP(primera, e032Peticion("limit=2", "jwt-valido"))
	if primera.Code != http.StatusOK {
		t.Fatalf("pagina de referencia: %d (%s)", primera.Code, primera.Body.String())
	}
	var pagina struct {
		NextCursor string `json:"nextCursor"`
	}
	if err := json.Unmarshal(primera.Body.Bytes(), &pagina); err != nil || pagina.NextCursor == "" {
		t.Fatalf("cursor de referencia: %v / %q", err, pagina.NextCursor)
	}

	casos := []struct {
		nombre   string
		consulta string
	}{
		{"manipulado", "limit=2&cursor=" + pagina.NextCursor[:len(pagina.NextCursor)-2] + "AA"},
		{"basura", "limit=2&cursor=no-es-un-cursor"},
		{"otro limite", "limit=25&cursor=" + pagina.NextCursor},
		{"otro filtro", "limit=2&type=income&cursor=" + pagina.NextCursor},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, e032Peticion(caso.consulta, "jwt-valido"))
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("esperado 400, obtenido %d (%s)", rec.Code, rec.Body.String())
			}
		})
	}
}

// TestE032HTTPAislamiento: el titular B no ve las reglas de A.
func TestE032HTTPAislamiento(t *testing.T) {
	store := newE032Store()
	store.sembrar(e004HTTPUsuario, e032Pago(1, "expense", true))
	handler := e032HTTPHandler(store)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, e032Peticion("", "jwt-sujeto-ajeno"))
	if rec.Code != http.StatusOK {
		t.Fatalf("esperado 200, obtenido %d (%s)", rec.Code, rec.Body.String())
	}
	var cuerpo struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &cuerpo); err != nil {
		t.Fatalf("respuesta no es JSON: %v", err)
	}
	if len(cuerpo.Items) != 0 {
		t.Fatalf("B no puede ver las reglas de A: %+v", cuerpo.Items)
	}
	ultima := store.consultas[len(store.consultas)-1]
	if ultima.UserID != e004HTTPOtroUser {
		t.Fatalf("la persistencia debe recibir el sub de B: %q", ultima.UserID)
	}
}

// TestE032HTTP401 cubre la ausencia de identidad verificada.
func TestE032HTTP401(t *testing.T) {
	casos := []struct {
		nombre   string
		cabecera string
	}{
		{"sin cabecera", ""},
		{"esquema basico", "Basic abc"},
		{"token desconocido", "Bearer desconocido"},
		{"token expirado", "Bearer jwt-expirado"},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			store := newE032Store()
			store.sembrar(e004HTTPUsuario, e032Pago(1, "expense", true))
			req := e032Peticion("", "")
			if caso.cabecera != "" {
				req.Header.Set("Authorization", caso.cabecera)
			}
			rec := httptest.NewRecorder()
			e032HTTPHandler(store).ServeHTTP(rec, req)
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("esperado 401, obtenido %d (%s)", rec.Code, rec.Body.String())
			}
			if len(store.consultas) != 0 {
				t.Fatal("una peticion no autenticada no puede consultar la persistencia")
			}
		})
	}
}

// TestE032HTTP503FallaCerrado: sin servicio, sin identidad o sin clave de cursor no se pagina.
func TestE032HTTP503FallaCerrado(t *testing.T) {
	store := newE032Store()
	store.sembrar(e004HTTPUsuario, e032Pago(1, "expense", true))

	casos := []struct {
		nombre  string
		handler *internalhttp.RecurringListHandler
	}{
		{"sin servicio", internalhttp.NewRecurringListHandler(nil, e004TokensValidos(), newE004Sesiones(), nil, silencioso())},
		{"sin tokens", internalhttp.NewRecurringListHandler(appRecurring.NewService(store, []byte("clave")), nil, newE004Sesiones(), nil, silencioso())},
		{"sin autorizador", internalhttp.NewRecurringListHandler(appRecurring.NewService(store, []byte("clave")), e004TokensValidos(), nil, nil, silencioso())},
		{"sin clave de cursor", internalhttp.NewRecurringListHandler(appRecurring.NewService(store, nil), e004TokensValidos(), newE004Sesiones(), nil, silencioso())},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			rec := httptest.NewRecorder()
			caso.handler.ServeHTTP(rec, e032Peticion("", "jwt-valido"))
			if rec.Code != http.StatusServiceUnavailable {
				t.Fatalf("esperado 503, obtenido %d (%s)", rec.Code, rec.Body.String())
			}
			if cache := rec.Header().Get("Cache-Control"); cache != "private, no-store" {
				t.Fatalf("tambien en 503 la respuesta es privada, obtenido %q", cache)
			}
		})
	}
}

// TestE032HTTPFalloDeDependencia: un fallo del almacén no se convierte en lista vacía.
func TestE032HTTPFalloDeDependencia(t *testing.T) {
	store := newE032Store()
	store.fallo = errors.New("dynamodb no disponible")

	rec := httptest.NewRecorder()
	e032HTTPHandler(store).ServeHTTP(rec, e032Peticion("", "jwt-valido"))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("esperado 503, obtenido %d (%s)", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "dynamodb") {
		t.Fatalf("la respuesta no puede filtrar detalles de infraestructura: %s", rec.Body.String())
	}
}

// TestE032HTTPCursorVencido cubre la expiración del cursor.
//
// El cursor se fabrica con la MISMA clave de firma y el MISMO `SortBy` que emite el servicio,
// con vencimiento en el pasado: es la verificación criptográfica y temporal de producción
// (`httputil.ValidateCursor`) la que debe rechazarlo con 400.
func TestE032HTTPCursorVencido(t *testing.T) {
	store := newE032Store()
	store.sembrar(e004HTTPUsuario, e032Pago(1, "expense", true), e032Pago(2, "expense", true))

	secreto := []byte("clave-de-cursor-de-prueba-e032")
	vencido, err := httputil.GenerateCursor(secreto, e004HTTPUsuario,
		"recurring:v1:limit=1:active=all:type=all", "RECURRING#"+e032Pago(1, "expense", true).ID,
		time.Now().UTC().Add(-2*time.Hour), -time.Minute)
	if err != nil {
		t.Fatalf("no se pudo fabricar el cursor vencido: %v", err)
	}
	if _, err := httputil.ValidateCursor(secreto, vencido, e004HTTPUsuario); err == nil {
		t.Fatal("el helper debe rechazar un cursor vencido")
	}

	handler := e032HTTPHandler(store)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, e032Peticion("limit=1&cursor="+vencido, "jwt-valido"))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("un cursor vencido debe ser 400, obtenido %d (%s)", rec.Code, rec.Body.String())
	}
}

// TestE032HTTPCursorDeOtroUsuario: el cursor firmado está ligado al `sub` que lo solicitó.
func TestE032HTTPCursorDeOtroUsuario(t *testing.T) {
	store := newE032Store()
	store.sembrar(e004HTTPUsuario, e032Pago(1, "expense", true), e032Pago(2, "expense", true))
	store.sembrar(e004HTTPOtroUser, e032Pago(3, "expense", true), e032Pago(4, "expense", true))
	handler := e032HTTPHandler(store)

	primera := httptest.NewRecorder()
	handler.ServeHTTP(primera, e032Peticion("limit=1", "jwt-valido"))
	var pagina struct {
		NextCursor string `json:"nextCursor"`
	}
	if err := json.Unmarshal(primera.Body.Bytes(), &pagina); err != nil || pagina.NextCursor == "" {
		t.Fatalf("cursor de referencia: %v / %q", err, pagina.NextCursor)
	}

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, e032Peticion("limit=1&cursor="+pagina.NextCursor, "jwt-sujeto-ajeno"))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("un cursor de otra cuenta debe ser 400, obtenido %d (%s)", rec.Code, rec.Body.String())
	}
}
