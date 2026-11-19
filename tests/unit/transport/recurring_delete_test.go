package transport_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	dynamoAdapter "github.com/nexumpr/Nexum-Backend/internal/adapters/dynamo"
	appRecurring "github.com/nexumpr/Nexum-Backend/internal/application/recurring"
	domainDynamo "github.com/nexumpr/Nexum-Backend/internal/domain/dynamo"
	internalhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
)

// Pruebas HTTP de E036 (`DELETE /api/v1/recurring-payments/{recurringPaymentId}`): 204 sin
// cuerpo, idempotencia sin incremento de versión, 404 sin enumeración, 400 de identificador,
// 401 y 503 fail-closed. La baja de un ítem heredado del alta (sin atributo `active`) se prueba
// contra el ADAPTADOR REAL montado sobre un doble que interpreta la condición emitida.

const (
	e036ID      = "018f90ef-08e5-7a68-b09f-2b67a47c92fb"
	e036IDAjeno = "018f90ef-08e5-7a68-b09f-2b67a47c92fc"
	e036Tabla   = "NEXUM_CORE_TEST"
)

var e036Hoy = time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)

// e036Store implementa `appRecurring.Store` con estado y baja idempotente.
type e036Store struct {
	mu    sync.Mutex
	pagos map[string]appRecurring.Payment
	bajas int
	fallo error
}

func newE036Store() *e036Store {
	return &e036Store{pagos: map[string]appRecurring.Payment{}}
}

func (s *e036Store) sembrar(userID string, pago appRecurring.Payment) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pagos[userID+"|"+pago.ID] = pago
}

func (s *e036Store) Deactivate(_ context.Context, userID, paymentID string, _ time.Time) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fallo != nil {
		return false, s.fallo
	}
	clave := userID + "|" + paymentID
	pago, ok := s.pagos[clave]
	if !ok {
		return false, nil
	}
	if pago.Active {
		pago.Active = false
		pago.Version++
		s.pagos[clave] = pago
		s.bajas++
	}
	return true, nil
}

func (s *e036Store) Get(_ context.Context, userID, paymentID string) (*appRecurring.Payment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	pago, ok := s.pagos[userID+"|"+paymentID]
	if !ok {
		return nil, nil
	}
	copia := pago
	return &copia, nil
}

func (s *e036Store) Create(context.Context, appRecurring.CreateWrite) (string, []byte, bool, error) {
	return "", nil, false, errors.New("Create no usado en E036")
}

func (s *e036Store) List(context.Context, string, string, int, appRecurring.Filter) ([]appRecurring.Payment, string, error) {
	return nil, "", errors.New("List no usado en E036")
}

func (s *e036Store) Update(context.Context, appRecurring.UpdateWrite) (*appRecurring.Payment, error) {
	return nil, errors.New("Update no usado en E036")
}

func e036Pago(id string) appRecurring.Payment {
	return appRecurring.Payment{
		ID:          id,
		Name:        "Renta",
		AmountCents: 100000,
		Currency:    "MXN",
		NextDueDate: time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC),
		Active:      true,
		Version:     1,
		Type:        "expense",
		StartDate:   time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC),
	}
}

func e036Handler(store appRecurring.Store) *internalhttp.RecurringDeleteHandler {
	servicio := appRecurring.NewService(store, nil)
	return internalhttp.NewRecurringDeleteHandler(servicio, e004TokensValidos(), newE004Sesiones(), nil, silencioso())
}

func e036Router(handler *internalhttp.RecurringDeleteHandler) *internalhttp.Router {
	router := internalhttp.NewRouter()
	router.HandleFunc(http.MethodDelete, "/recurring-payments/{recurringPaymentId}", handler.ServeHTTP)
	return router
}

func e036Peticion(id, token string) *http.Request {
	return e004Peticion(http.MethodDelete, "/api/v1/recurring-payments/"+id, token, "")
}

// e036API implementa `dynamoAdapter.RecurringAPI` con estado para `GetItem` y `UpdateItem`; es
// lo único que usa `Deactivate` del adaptador real.
type e036API struct {
	mu      sync.Mutex
	items   map[string]map[string]types.AttributeValue
	updates int
	fallo   error
}

func newE036API() *e036API {
	return &e036API{items: map[string]map[string]types.AttributeValue{}}
}

func e036Attr(pk, sk string) string { return pk + "|" + sk }

func (a *e036API) GetItem(_ context.Context, params *dynamodb.GetItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.fallo != nil {
		return nil, a.fallo
	}
	pk, _ := params.Key["PK"].(*types.AttributeValueMemberS)
	sk, _ := params.Key["SK"].(*types.AttributeValueMemberS)
	if pk == nil || sk == nil {
		return &dynamodb.GetItemOutput{}, nil
	}
	item, ok := a.items[e036Attr(pk.Value, sk.Value)]
	if !ok {
		return &dynamodb.GetItemOutput{}, nil
	}
	copia := make(map[string]types.AttributeValue, len(item))
	for clave, valor := range item {
		copia[clave] = valor
	}
	return &dynamodb.GetItemOutput{Item: copia}, nil
}

func (a *e036API) UpdateItem(_ context.Context, params *dynamodb.UpdateItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.fallo != nil {
		return nil, a.fallo
	}

	condicion := aws.ToString(params.ConditionExpression)
	if condicion != "attribute_exists(PK) AND (attribute_not_exists(#active) OR #active = :true)" {
		return nil, errors.New("condicion no soportada por el doble: " + condicion)
	}
	pk, _ := params.Key["PK"].(*types.AttributeValueMemberS)
	sk, _ := params.Key["SK"].(*types.AttributeValueMemberS)
	clave := e036Attr(pk.Value, sk.Value)
	item, existe := a.items[clave]
	if !existe {
		return nil, &types.ConditionalCheckFailedException{Message: aws.String("condicion no cumplida")}
	}
	if activo, tiene := item["active"].(*types.AttributeValueMemberBOOL); tiene && !activo.Value {
		return nil, &types.ConditionalCheckFailedException{Message: aws.String("ya inactiva")}
	}

	nuevo := make(map[string]types.AttributeValue, len(item))
	for k, v := range item {
		nuevo[k] = v
	}
	nuevo["active"] = &types.AttributeValueMemberBOOL{Value: false}
	nuevo["updatedAt"] = params.ExpressionAttributeValues[":updatedAt"]
	version, _ := item["version"].(*types.AttributeValueMemberN)
	var actual int64
	if version != nil {
		for _, digito := range version.Value {
			actual = actual*10 + int64(digito-'0')
		}
	}
	nuevo["version"] = &types.AttributeValueMemberN{Value: strconvInt(actual + 1)}
	a.items[clave] = nuevo
	a.updates++
	return &dynamodb.UpdateItemOutput{Attributes: nuevo}, nil
}

func strconvInt(valor int64) string {
	cuerpo, _ := json.Marshal(valor)
	return string(cuerpo)
}

func (a *e036API) Query(context.Context, *dynamodb.QueryInput, ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
	return nil, errors.New("Query no usado en E036")
}

func (a *e036API) TransactWriteItems(context.Context, *dynamodb.TransactWriteItemsInput, ...func(*dynamodb.Options)) (*dynamodb.TransactWriteItemsOutput, error) {
	return nil, errors.New("TransactWriteItems no usado en E036")
}

// e036ItemHeredado siembra un ítem del alta de E001: sin `nextDueDate`, sin `startDate` y SIN
// atributo `active`.
func e036ItemHeredado(userID, id string) map[string]types.AttributeValue {
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
		"frequency":   e034AttrS("monthly"),
		"category":    e034AttrS("Servicios"),
		"expenseType": e034AttrS("fixed"),
		"createdAt":   e034AttrS("2026-01-15T08:00:00Z"),
		"updatedAt":   e034AttrS("2026-01-15T08:00:00Z"),
		"version":     e034AttrN(1),
	}
}

// TestE036HTTP204Idempotente cubre el contrato de la baja: 204 sin cuerpo y una sola escritura.
func TestE036HTTP204Idempotente(t *testing.T) {
	store := newE036Store()
	store.sembrar(e004HTTPUsuario, e036Pago(e036ID))
	handler := e036Handler(store)

	primera := httptest.NewRecorder()
	e036Router(handler).ServeHTTP(primera, e036Peticion(e036ID, "jwt-valido"))
	if primera.Code != http.StatusNoContent {
		t.Fatalf("esperado 204, obtenido %d (%s)", primera.Code, primera.Body.String())
	}
	if primera.Body.Len() != 0 {
		t.Fatalf("204 no admite cuerpo, obtenido %q", primera.Body.String())
	}
	if cache := primera.Header().Get("Cache-Control"); cache != "private, no-store" {
		t.Fatalf("Cache-Control inesperado: %q", cache)
	}

	pago, _ := store.Get(context.Background(), e004HTTPUsuario, e036ID)
	if pago == nil || pago.Active || pago.Version != 2 {
		t.Fatalf("la regla debe quedar inactiva en version 2: %+v", pago)
	}

	segunda := httptest.NewRecorder()
	e036Router(handler).ServeHTTP(segunda, e036Peticion(e036ID, "jwt-valido"))
	if segunda.Code != http.StatusNoContent {
		t.Fatalf("el segundo DELETE debe ser 204, obtenido %d (%s)", segunda.Code, segunda.Body.String())
	}
	pago, _ = store.Get(context.Background(), e004HTTPUsuario, e036ID)
	if pago.Version != 2 {
		t.Fatalf("una regla ya inactiva no incrementa version: %d", pago.Version)
	}
	if store.bajas != 1 {
		t.Fatalf("solo debe haber una baja efectiva, obtenidas %d", store.bajas)
	}
}

// TestE036HTTP400UUID: el parámetro de ruta mal formado es 400.
func TestE036HTTP400UUID(t *testing.T) {
	store := newE036Store()
	for _, id := range []string{"no-es-uuid", "018f90ef", "018f90ef-08e5-7a68-b09f-2b67a47c92f"} {
		t.Run(id, func(t *testing.T) {
			rec := httptest.NewRecorder()
			e036Router(e036Handler(store)).ServeHTTP(rec, e036Peticion(id, "jwt-valido"))
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("esperado 400, obtenido %d (%s)", rec.Code, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), "recurringPaymentId") {
				t.Fatalf("el detalle debe nombrar el parametro: %s", rec.Body.String())
			}
		})
	}
}

// TestE036HTTP404NoEnumera: la regla de otra cuenta y la inexistente producen el mismo 404.
func TestE036HTTP404NoEnumera(t *testing.T) {
	store := newE036Store()
	store.sembrar(e004HTTPOtroUser, e036Pago(e036IDAjeno))

	ajeno := httptest.NewRecorder()
	e036Router(e036Handler(store)).ServeHTTP(ajeno, e036Peticion(e036IDAjeno, "jwt-valido"))
	if ajeno.Code != http.StatusNotFound {
		t.Fatalf("un recurso ajeno debe ser 404, obtenido %d (%s)", ajeno.Code, ajeno.Body.String())
	}

	inexistente := httptest.NewRecorder()
	e036Router(e036Handler(store)).ServeHTTP(inexistente, e036Peticion(e036ID, "jwt-valido"))
	if inexistente.Code != http.StatusNotFound {
		t.Fatalf("un recurso inexistente debe ser 404, obtenido %d (%s)", inexistente.Code, inexistente.Body.String())
	}

	if ajeno.Body.String() != inexistente.Body.String() {
		t.Fatalf("los dos 404 deben ser identicos:\n%s\n%s", ajeno.Body.String(), inexistente.Body.String())
	}
	if store.bajas != 0 {
		t.Fatal("un 404 no puede dar de baja nada")
	}
}

// TestE036HTTP401 cubre la ausencia de identidad verificada.
func TestE036HTTP401(t *testing.T) {
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
			store := newE036Store()
			store.sembrar(e004HTTPUsuario, e036Pago(e036ID))
			req := e036Peticion(e036ID, "")
			if caso.cabecera != "" {
				req.Header.Set("Authorization", caso.cabecera)
			}
			rec := httptest.NewRecorder()
			e036Router(e036Handler(store)).ServeHTTP(rec, req)
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("esperado 401, obtenido %d (%s)", rec.Code, rec.Body.String())
			}
			if store.bajas != 0 {
				t.Fatal("una peticion no autenticada no puede dar de baja")
			}
		})
	}
}

// TestE036HTTP503FallaCerrado: sin servicio o con un fallo de persistencia no se confirma nada.
func TestE036HTTP503FallaCerrado(t *testing.T) {
	casos := []struct {
		nombre  string
		handler *internalhttp.RecurringDeleteHandler
	}{
		{"sin servicio", internalhttp.NewRecurringDeleteHandler(nil, e004TokensValidos(), newE004Sesiones(), nil, silencioso())},
		{"sin tokens", internalhttp.NewRecurringDeleteHandler(appRecurring.NewService(newE036Store(), nil), nil, newE004Sesiones(), nil, silencioso())},
		{"sin autorizador", internalhttp.NewRecurringDeleteHandler(appRecurring.NewService(newE036Store(), nil), e004TokensValidos(), nil, nil, silencioso())},
		{"sin almacen", internalhttp.NewRecurringDeleteHandler(appRecurring.NewService(nil, nil), e004TokensValidos(), newE004Sesiones(), nil, silencioso())},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			rec := httptest.NewRecorder()
			e036Router(caso.handler).ServeHTTP(rec, e036Peticion(e036ID, "jwt-valido"))
			if rec.Code != http.StatusServiceUnavailable {
				t.Fatalf("esperado 503, obtenido %d (%s)", rec.Code, rec.Body.String())
			}
			if cache := rec.Header().Get("Cache-Control"); cache != "private, no-store" {
				t.Fatalf("tambien en 503 la respuesta es privada, obtenido %q", cache)
			}
		})
	}

	store := newE036Store()
	store.sembrar(e004HTTPUsuario, e036Pago(e036ID))
	store.fallo = errors.New("dynamodb no disponible")
	rec := httptest.NewRecorder()
	e036Router(e036Handler(store)).ServeHTTP(rec, e036Peticion(e036ID, "jwt-valido"))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("un fallo de persistencia debe ser 503, obtenido %d (%s)", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "dynamodb") {
		t.Fatalf("la respuesta no puede filtrar detalles de infraestructura: %s", rec.Body.String())
	}
}

// TestE036HTTPItemHeredadoSinActive es la prueba decisiva de D-E032-01: el ADAPTADOR REAL da de
// baja un ítem del alta que no tiene atributo `active`, y la segunda baja es idempotente sin
// incrementar la version.
func TestE036HTTPItemHeredadoSinActive(t *testing.T) {
	api := newE036API()
	pk, sk := domainDynamo.RecurringKeys(e004HTTPUsuario, e036ID)
	api.items[e036Attr(pk, sk)] = e036ItemHeredado(e004HTTPUsuario, e036ID)

	store := dynamoAdapter.NewRecurringStore(api, e036Tabla).WithClock(func() time.Time { return e036Hoy })
	handler := internalhttp.NewRecurringDeleteHandler(appRecurring.NewService(store, nil), e004TokensValidos(), newE004Sesiones(), nil, silencioso())
	router := e036Router(handler)

	primera := httptest.NewRecorder()
	router.ServeHTTP(primera, e036Peticion(e036ID, "jwt-valido"))
	if primera.Code != http.StatusNoContent {
		t.Fatalf("el item heredado debe darse de baja: %d (%s)", primera.Code, primera.Body.String())
	}
	if primera.Body.Len() != 0 {
		t.Fatalf("204 no admite cuerpo, obtenido %q", primera.Body.String())
	}

	item := api.items[e036Attr(pk, sk)]
	if activo, ok := item["active"].(*types.AttributeValueMemberBOOL); !ok || activo.Value {
		t.Fatalf("el item debe quedar inactivo: %v", item["active"])
	}
	if version, ok := item["version"].(*types.AttributeValueMemberN); !ok || version.Value != "2" {
		t.Fatalf("la version debe ser 2, obtenido %v", item["version"])
	}

	segunda := httptest.NewRecorder()
	router.ServeHTTP(segunda, e036Peticion(e036ID, "jwt-valido"))
	if segunda.Code != http.StatusNoContent {
		t.Fatalf("la segunda baja debe ser 204, obtenido %d (%s)", segunda.Code, segunda.Body.String())
	}
	item = api.items[e036Attr(pk, sk)]
	if version, ok := item["version"].(*types.AttributeValueMemberN); !ok || version.Value != "2" {
		t.Fatalf("una baja repetida no incrementa version: %v", item["version"])
	}
	if api.updates != 1 {
		t.Fatalf("solo debe haber una escritura efectiva, obtenidas %d", api.updates)
	}
}
