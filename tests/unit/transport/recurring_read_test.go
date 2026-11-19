package transport_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	dynamoAdapter "github.com/nexumpr/Nexum-Backend/internal/adapters/dynamo"
	appRecurring "github.com/nexumpr/Nexum-Backend/internal/application/recurring"
	domainDynamo "github.com/nexumpr/Nexum-Backend/internal/domain/dynamo"
	domainRecurring "github.com/nexumpr/Nexum-Backend/internal/domain/recurring"
	internalhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
)

// Pruebas HTTP de E034 (`GET /api/v1/recurring-payments/{recurringPaymentId}`) sobre el
// ADAPTADOR REAL montado sobre un doble que sólo implementa `GetItem`: la proyección de los
// ítems heredados del alta (D-E032-01) se comprueba de extremo a extremo, no con un doble de
// aplicación. La ruta se resuelve con el Router real para que `{recurringPaymentId}` llegue
// por `SetPathValue`, igual que en producción.

const (
	e034PathBase = "/api/v1/recurring-payments/"
	e034Tabla    = "NEXUM_CORE_TEST"
	e034ID       = "018f90ef-08e5-7a68-b09f-2b67a47c92f0"
	e034IDAjeno  = "018f90ef-08e5-7a68-b09f-2b67a47c92f1"
	e034TablaFmt = "2026-11-18"
)

var e034Hoy = time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)

func e034AttrS(valor string) types.AttributeValue {
	return &types.AttributeValueMemberS{Value: valor}
}

func e034AttrN(valor int64) types.AttributeValue {
	return &types.AttributeValueMemberN{Value: strconv.FormatInt(valor, 10)}
}

func e034AttrB(valor bool) types.AttributeValue {
	return &types.AttributeValueMemberBOOL{Value: valor}
}

// e034API implementa `dynamoAdapter.RecurringAPI` con estado para `GetItem`. Las demás
// operaciones no se usan en E034 y fallan de forma explícita si alguien las invoca.
type e034API struct {
	items map[string]map[string]types.AttributeValue
	fallo error
}

func newE034API() *e034API {
	return &e034API{items: map[string]map[string]types.AttributeValue{}}
}

func (a *e034API) GetItem(_ context.Context, params *dynamodb.GetItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
	if a.fallo != nil {
		return nil, a.fallo
	}
	pk, _ := params.Key["PK"].(*types.AttributeValueMemberS)
	sk, _ := params.Key["SK"].(*types.AttributeValueMemberS)
	if pk == nil || sk == nil {
		return &dynamodb.GetItemOutput{}, nil
	}
	item, ok := a.items[pk.Value+"|"+sk.Value]
	if !ok {
		return &dynamodb.GetItemOutput{}, nil
	}
	copia := make(map[string]types.AttributeValue, len(item))
	for clave, valor := range item {
		copia[clave] = valor
	}
	return &dynamodb.GetItemOutput{Item: copia}, nil
}

func (a *e034API) Query(context.Context, *dynamodb.QueryInput, ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
	return nil, errors.New("Query no usado en E034")
}

func (a *e034API) UpdateItem(context.Context, *dynamodb.UpdateItemInput, ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error) {
	return nil, errors.New("UpdateItem no usado en E034")
}

func (a *e034API) TransactWriteItems(context.Context, *dynamodb.TransactWriteItemsInput, ...func(*dynamodb.Options)) (*dynamodb.TransactWriteItemsOutput, error) {
	return nil, errors.New("TransactWriteItems no usado en E034")
}

// e034Handler compone el handler real sobre el adaptador real.
func e034Handler(api dynamoAdapter.RecurringAPI) *internalhttp.RecurringGetHandler {
	store := dynamoAdapter.NewRecurringStore(api, e034Tabla).WithClock(func() time.Time { return e034Hoy })
	servicio := appRecurring.NewService(store, nil)
	return internalhttp.NewRecurringGetHandler(servicio, e004TokensValidos(), newE004Sesiones(), nil, silencioso())
}

// e034Router registra la plantilla en el Router real.
func e034Router(handler *internalhttp.RecurringGetHandler) *internalhttp.Router {
	router := internalhttp.NewRouter()
	router.HandleFunc(http.MethodGet, "/recurring-payments/{recurringPaymentId}", handler.ServeHTTP)
	return router
}

// e034ItemContrato siembra un ítem escrito por E033.
func e034ItemContrato(userID, id string, version int64) map[string]types.AttributeValue {
	pk, sk := domainDynamo.RecurringKeys(userID, id)
	return map[string]types.AttributeValue{
		"PK":          e034AttrS(pk),
		"SK":          e034AttrS(sk),
		"entityType":  e034AttrS("RECURRING_PAYMENT"),
		"userId":      e034AttrS(userID),
		"recurringId": e034AttrS(id),
		"name":        e034AttrS("Renta"),
		"amountCents": e034AttrN(100000),
		"currency":    e034AttrS("MXN"),
		"frequency":   e034AttrS("monthly"),
		"nextDueDate": e034AttrS("2026-11-05"),
		"active":      e034AttrB(true),
		"type":        e034AttrS("expense"),
		"startDate":   e034AttrS("2026-10-05"),
		"dueDay":      e034AttrN(5),
		"createdAt":   e034AttrS("2026-10-05T10:00:00Z"),
		"updatedAt":   e034AttrS("2026-10-05T10:00:00Z"),
		"version":     e034AttrN(version),
	}
}

// e034ItemHeredado siembra un ítem del alta de E001, sin nextDueDate ni startDate.
func e034ItemHeredado(userID, id string) map[string]types.AttributeValue {
	pk, sk := domainDynamo.RecurringKeys(userID, id)
	return map[string]types.AttributeValue{
		"PK":          e034AttrS(pk),
		"SK":          e034AttrS(sk),
		"entityType":  e034AttrS("RECURRING_PAYMENT"),
		"userId":      e034AttrS(userID),
		"recurringId": e034AttrS(id),
		"concept":     e034AttrS("Internet"),
		"currency":    e034AttrS("MXN"),
		"amountCents": e034AttrN(89900),
		"frequency":   e034AttrS("biweekly"),
		"category":    e034AttrS("Servicios"),
		"expenseType": e034AttrS("fixed"),
		"createdAt":   e034AttrS("2026-01-15T08:00:00Z"),
		"updatedAt":   e034AttrS("2026-01-15T08:00:00Z"),
		"version":     e034AttrN(1),
	}
}

func e034Peticion(id, token string) *http.Request {
	return e004Peticion(http.MethodGet, e034PathBase+id, token, "")
}

// TestE034HTTP200ContratoExacto cubre T01: código, ETag, referencia de caché y once campos.
func TestE034HTTP200ContratoExacto(t *testing.T) {
	api := newE034API()
	api.items[domainDynamo.PrefixUser+e004HTTPUsuario+"|"+domainDynamo.PrefixRecurring+e034ID] = e034ItemContrato(e004HTTPUsuario, e034ID, 3)

	rec := httptest.NewRecorder()
	e034Router(e034Handler(api)).ServeHTTP(rec, e034Peticion(e034ID, "jwt-valido"))

	if rec.Code != http.StatusOK {
		t.Fatalf("esperado 200, obtenido %d (%s)", rec.Code, rec.Body.String())
	}
	if etag := rec.Header().Get("ETag"); etag != "\"v3\"" {
		t.Fatalf("ETag inesperado: %q", etag)
	}
	if cache := rec.Header().Get("Cache-Control"); cache != "private, no-store" {
		t.Fatalf("Cache-Control inesperado: %q", cache)
	}
	var cuerpo map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &cuerpo); err != nil {
		t.Fatalf("respuesta no es JSON: %v", err)
	}
	if len(cuerpo) != 11 {
		t.Fatalf("se esperaban once campos, obtenidos %d: %v", len(cuerpo), cuerpo)
	}
	if cuerpo["id"] != e034ID || cuerpo["name"] != "Renta" || cuerpo["amount"] != "1000.00" {
		t.Fatalf("proyeccion inesperada: %v", cuerpo)
	}
	if cuerpo["frequency"] != "monthly" || cuerpo["nextDueDate"] != "2026-11-05" {
		t.Fatalf("fechas inesperadas: %v", cuerpo)
	}
	if cuerpo["version"] != float64(3) || cuerpo["dueDay"] != float64(5) {
		t.Fatalf("version/dueDay inesperados: %v", cuerpo)
	}
	for _, prohibido := range []string{"userId", "entityType", "PK", "SK", "createdAt"} {
		if _, ok := cuerpo[prohibido]; ok {
			t.Fatalf("la respuesta no puede exponer %q", prohibido)
		}
	}
}

// TestE034HTTPUUIDInvalido cubre el 400 de esquema del parámetro de ruta.
func TestE034HTTPUUIDInvalido(t *testing.T) {
	casos := []string{"no-es-uuid", "018f90ef", "018f90ef-08e5-7a68-b09f-2b67a47c92f", "018f90ef-08e5-7a68-b09f-2b67a47c92f00"}
	for _, id := range casos {
		t.Run(id, func(t *testing.T) {
			api := newE034API()
			rec := httptest.NewRecorder()
			e034Router(e034Handler(api)).ServeHTTP(rec, e034Peticion(id, "jwt-valido"))
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("esperado 400, obtenido %d (%s)", rec.Code, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), "recurringPaymentId") {
				t.Fatalf("el detalle debe nombrar el parametro: %s", rec.Body.String())
			}
		})
	}
}

// TestE034HTTP404NoEnumera cubre T03: recurso de otra cuenta y recurso inexistente producen el
// mismo 404.
func TestE034HTTP404NoEnumera(t *testing.T) {
	api := newE034API()
	api.items[domainDynamo.PrefixUser+e004HTTPOtroUser+"|"+domainDynamo.PrefixRecurring+e034IDAjeno] = e034ItemContrato(e004HTTPOtroUser, e034IDAjeno, 1)

	ajeno := httptest.NewRecorder()
	e034Router(e034Handler(api)).ServeHTTP(ajeno, e034Peticion(e034IDAjeno, "jwt-valido"))
	if ajeno.Code != http.StatusNotFound {
		t.Fatalf("un recurso ajeno debe ser 404, obtenido %d (%s)", ajeno.Code, ajeno.Body.String())
	}

	inexistente := httptest.NewRecorder()
	e034Router(e034Handler(api)).ServeHTTP(inexistente, e034Peticion(e034ID, "jwt-valido"))
	if inexistente.Code != http.StatusNotFound {
		t.Fatalf("un recurso inexistente debe ser 404, obtenido %d (%s)", inexistente.Code, inexistente.Body.String())
	}

	if ajeno.Body.String() != inexistente.Body.String() {
		t.Fatalf("las dos respuestas 404 deben ser identicas:\n%s\n%s", ajeno.Body.String(), inexistente.Body.String())
	}
	if strings.Contains(ajeno.Body.String(), e034IDAjeno) {
		t.Fatal("el 404 no puede reflejar el identificador solicitado")
	}
}

// TestE034HTTPItemHeredado cubre D-E032-01 por HTTP: un ítem del alta sin nextDueDate ni
// startDate se proyecta de forma determinista y se sirve con el contrato completo.
func TestE034HTTPItemHeredado(t *testing.T) {
	api := newE034API()
	api.items[domainDynamo.PrefixUser+e004HTTPUsuario+"|"+domainDynamo.PrefixRecurring+e034ID] = e034ItemHeredado(e004HTTPUsuario, e034ID)

	rec := httptest.NewRecorder()
	e034Router(e034Handler(api)).ServeHTTP(rec, e034Peticion(e034ID, "jwt-valido"))

	if rec.Code != http.StatusOK {
		t.Fatalf("el item heredado debe servirse: %d (%s)", rec.Code, rec.Body.String())
	}
	var cuerpo map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &cuerpo); err != nil {
		t.Fatalf("respuesta no es JSON: %v", err)
	}
	if len(cuerpo) != 11 {
		t.Fatalf("el contrato tiene once campos, obtenidos %d: %v", len(cuerpo), cuerpo)
	}
	if cuerpo["name"] != "Internet" {
		t.Fatalf("name debe venir de concept: %v", cuerpo["name"])
	}
	if cuerpo["type"] != "expense" {
		t.Fatalf("expenseType=fixed debe proyectarse como gasto: %v", cuerpo["type"])
	}
	if cuerpo["frequency"] != "fortnightly" {
		t.Fatalf("biweekly debe normalizarse a fortnightly: %v", cuerpo["frequency"])
	}
	if cuerpo["startDate"] != "2026-01-15" {
		t.Fatalf("startDate debe ser la fecha de createdAt: %v", cuerpo["startDate"])
	}
	proximo, ok := cuerpo["nextDueDate"].(string)
	if !ok || proximo < e034TablaFmt {
		t.Fatalf("nextDueDate debe ser una ocurrencia futura: %v", cuerpo["nextDueDate"])
	}
	if cuerpo["active"] != true {
		t.Fatalf("una regla del alta no esta cancelada: %v", cuerpo["active"])
	}
	if valor, existe := cuerpo["dueDay"]; !existe || valor != nil {
		t.Fatalf("una frecuencia quincenal no admite dueDay: %v", valor)
	}
	if etag := rec.Header().Get("ETag"); etag != "\"v1\"" {
		t.Fatalf("ETag inesperado: %q", etag)
	}
}

// TestE034HTTP401 cubre la ausencia de identidad verificada.
func TestE034HTTP401(t *testing.T) {
	api := newE034API()
	api.items[domainDynamo.PrefixUser+e004HTTPUsuario+"|"+domainDynamo.PrefixRecurring+e034ID] = e034ItemContrato(e004HTTPUsuario, e034ID, 1)

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
			req := e034Peticion(e034ID, "")
			if caso.cabecera != "" {
				req.Header.Set("Authorization", caso.cabecera)
			}
			rec := httptest.NewRecorder()
			e034Router(e034Handler(api)).ServeHTTP(rec, req)
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("esperado 401, obtenido %d (%s)", rec.Code, rec.Body.String())
			}
		})
	}
}

// TestE034HTTP503FallaCerrado: sin servicio o sin persistencia no se sirve ningún recurso.
func TestE034HTTP503FallaCerrado(t *testing.T) {
	casos := []struct {
		nombre  string
		handler *internalhttp.RecurringGetHandler
	}{
		{"sin servicio", internalhttp.NewRecurringGetHandler(nil, e004TokensValidos(), newE004Sesiones(), nil, silencioso())},
		{"sin tokens", internalhttp.NewRecurringGetHandler(appRecurring.NewService(nil, nil), nil, newE004Sesiones(), nil, silencioso())},
		{"sin autorizador", internalhttp.NewRecurringGetHandler(appRecurring.NewService(nil, nil), e004TokensValidos(), nil, nil, silencioso())},
		{"sin almacen", internalhttp.NewRecurringGetHandler(appRecurring.NewService(nil, nil), e004TokensValidos(), newE004Sesiones(), nil, silencioso())},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			rec := httptest.NewRecorder()
			e034Router(caso.handler).ServeHTTP(rec, e034Peticion(e034ID, "jwt-valido"))
			if rec.Code != http.StatusServiceUnavailable {
				t.Fatalf("esperado 503, obtenido %d (%s)", rec.Code, rec.Body.String())
			}
			if cache := rec.Header().Get("Cache-Control"); cache != "private, no-store" {
				t.Fatalf("tambien en 503 la respuesta es privada, obtenido %q", cache)
			}
		})
	}
}

// TestE034HTTPFalloDeDependencia: un fallo del almacén no se convierte en 404 ni en datos
// inventados.
func TestE034HTTPFalloDeDependencia(t *testing.T) {
	api := newE034API()
	api.fallo = errors.New("dynamodb no disponible")

	rec := httptest.NewRecorder()
	e034Router(e034Handler(api)).ServeHTTP(rec, e034Peticion(e034ID, "jwt-valido"))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("esperado 503, obtenido %d (%s)", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "dynamodb") {
		t.Fatalf("la respuesta no puede filtrar detalles de infraestructura: %s", rec.Body.String())
	}
}

// TestE034HTTPProyeccionFechaNoPasada comprueba con otra frecuencia que la proyección sigue
// cayendo en la cadencia real sin retroceder al pasado.
func TestE034HTTPProyeccionFechaNoPasada(t *testing.T) {
	api := newE034API()
	item := e034ItemHeredado(e004HTTPUsuario, e034ID)
	item["frequency"] = e034AttrS("monthly")
	api.items[domainDynamo.PrefixUser+e004HTTPUsuario+"|"+domainDynamo.PrefixRecurring+e034ID] = item

	rec := httptest.NewRecorder()
	e034Router(e034Handler(api)).ServeHTTP(rec, e034Peticion(e034ID, "jwt-valido"))
	if rec.Code != http.StatusOK {
		t.Fatalf("esperado 200, obtenido %d (%s)", rec.Code, rec.Body.String())
	}
	var cuerpo map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &cuerpo); err != nil {
		t.Fatalf("respuesta no es JSON: %v", err)
	}
	proximo, ok := cuerpo["nextDueDate"].(string)
	if !ok || proximo < e034TablaFmt {
		t.Fatalf("nextDueDate debe ser futura: %v", cuerpo["nextDueDate"])
	}
	fecha, err := domainRecurring.ParseDate(proximo)
	if err != nil {
		t.Fatalf("nextDueDate no es una fecha civil valida: %v", err)
	}
	inicio, _ := domainRecurring.ParseDate("2026-01-15")
	if err := domainRecurring.ValidateSchedule(inicio, fecha, "monthly"); err != nil {
		t.Fatalf("la fecha proyectada debe caer en la cadencia mensual: %v", err)
	}
}
