package transport_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	appIdentity "github.com/nexumpr/Nexum-Backend/internal/application/identity"
	domainIdentity "github.com/nexumpr/Nexum-Backend/internal/domain/identity"
	internalhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
)

// Pruebas HTTP de E024 (`GET /api/v1/income`) y E025 (`PUT /api/v1/income`): autenticación con
// estado autoritativo de sesión (E004), DTO cerrado de cuatro campos, ETag fuerte `"vN"`,
// precedencia `If-Match` → cuerpo, normalización de alias de frecuencia (RFC-020 §2), precisión
// de céntimos y traducción exacta de los códigos del contrato.
//
// Los dobles son exclusivamente de prueba; la composición productiva usa DynamoDB real. La
// verificación de sesión, los tokens y la política de error del middleware se reutilizan del
// arnés de E004 (`e004TokensValidos`, `newE004Sesiones`, `e004Peticion`, `silencioso`).

var e024HTTPAhora = time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)

// errIngresoHTTPDep simula un fallo de dependencia (DynamoDB inaccesible).
var errIngresoHTTPDep = errors.New("dynamodb inaccesible")

// ingresoStoreHTTP es el puerto IncomeStore en memoria de prueba.
type ingresoStoreHTTP struct {
	mu           sync.Mutex
	snapshots    map[string]appIdentity.IncomeSnapshot
	perfilExiste bool

	errLectura   error
	errPerfil    error
	errEscritura error

	escrituras []appIdentity.IncomeStoreWrite
	lecturas   []string
	perfiles   []string
}

func newIngresoStoreHTTP() *ingresoStoreHTTP {
	return &ingresoStoreHTTP{
		snapshots:    map[string]appIdentity.IncomeSnapshot{},
		perfilExiste: true,
	}
}

func (s *ingresoStoreHTTP) LoadIncome(_ context.Context, userID string) (*appIdentity.IncomeSnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lecturas = append(s.lecturas, userID)
	if s.errLectura != nil {
		return nil, s.errLectura
	}
	snapshot, ok := s.snapshots[userID]
	if !ok {
		return nil, nil
	}
	copia := snapshot
	return &copia, nil
}

func (s *ingresoStoreHTTP) ProfileExists(_ context.Context, userID string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.perfiles = append(s.perfiles, userID)
	if s.errPerfil != nil {
		return false, s.errPerfil
	}
	return s.perfilExiste, nil
}

func (s *ingresoStoreHTTP) StoreIncome(_ context.Context, write appIdentity.IncomeStoreWrite) (*appIdentity.IncomeSnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.escrituras = append(s.escrituras, write)
	if s.errEscritura != nil {
		return nil, s.errEscritura
	}
	return &appIdentity.IncomeSnapshot{
		AmountCents: write.AmountCents,
		Frequency:   write.Frequency,
		Version:     write.ExpectedVersion + 1,
	}, nil
}

func (s *ingresoStoreHTTP) escriturasCapturadas() []appIdentity.IncomeStoreWrite {
	s.mu.Lock()
	defer s.mu.Unlock()
	copia := make([]appIdentity.IncomeStoreWrite, len(s.escrituras))
	copy(copia, s.escrituras)
	return copia
}

func e024ServicioHTTP(store appIdentity.IncomeStore) *appIdentity.IncomeService {
	return appIdentity.NewIncomeService(store).WithClock(func() time.Time { return e024HTTPAhora })
}

func e024HandlerGET(store appIdentity.IncomeStore) http.Handler {
	return internalhttp.NewIncomeGetHandler(e024ServicioHTTP(store), e004TokensValidos(), newE004Sesiones(), nil, silencioso())
}

func e024HandlerGETConSesiones(store appIdentity.IncomeStore, sesiones *e004Sesiones) http.Handler {
	return internalhttp.NewIncomeGetHandler(e024ServicioHTTP(store), e004TokensValidos(), sesiones, nil, silencioso())
}

func e025HandlerPUT(store appIdentity.IncomeStore) http.Handler {
	return internalhttp.NewIncomePutHandler(e024ServicioHTTP(store), e004TokensValidos(), newE004Sesiones(), nil, silencioso())
}

// ingresoHTTPRespuesta replica el objeto `Income` cerrado del contrato.
type ingresoHTTPRespuesta struct {
	Amount    string `json:"amount"`
	Currency  string `json:"currency"`
	Frequency string `json:"frequency"`
	Version   int64  `json:"version"`
}

// e024CuerpoIngreso compone el payload `IncomeUpdate` con los tres campos requeridos.
func e024CuerpoIngreso(amount, currency, frequency string) string {
	return `{"amount":"` + amount + `","currency":"` + currency + `","frequency":"` + frequency + `"}`
}

// e025Peticion construye una petición PUT con `If-Match` opcional y cuerpo literal.
func e025Peticion(token, ifMatch, cuerpo string) *http.Request {
	req := e004Peticion(http.MethodPut, "/api/v1/income", token, cuerpo)
	if ifMatch != "" {
		req.Header.Set("If-Match", ifMatch)
	}
	return req
}

func decodificarIngreso(t *testing.T, rec *httptest.ResponseRecorder) (ingresoHTTPRespuesta, map[string]any) {
	t.Helper()
	var respuesta ingresoHTTPRespuesta
	if err := json.Unmarshal(rec.Body.Bytes(), &respuesta); err != nil {
		t.Fatalf("la respuesta no es el objeto Income: %v (%s)", err, rec.Body.String())
	}
	var crudo map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &crudo); err != nil {
		t.Fatalf("la respuesta no es un objeto JSON: %v (%s)", err, rec.Body.String())
	}
	return respuesta, crudo
}

// exigirIngreso200 comprueba código, esquema EXACTO de cuatro campos, ETag fuerte y cabeceras
// de información privada.
func exigirIngreso200(t *testing.T, rec *httptest.ResponseRecorder, esperado ingresoHTTPRespuesta, etag string) {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("se esperaba 200, obtenido %d: %s", rec.Code, rec.Body.String())
	}
	respuesta, crudo := decodificarIngreso(t, rec)
	if respuesta != esperado {
		t.Fatalf("respuesta inesperada: %+v (esperada %+v)", respuesta, esperado)
	}
	claves := make([]string, 0, len(crudo))
	for clave := range crudo {
		claves = append(claves, clave)
	}
	sort.Strings(claves)
	if !reflect.DeepEqual(claves, []string{"amount", "currency", "frequency", "version"}) {
		t.Fatalf("el objeto Income debe tener exactamente cuatro campos, tiene %v", claves)
	}
	if got := rec.Header().Get("ETag"); got != etag {
		t.Fatalf("ETag esperado %q, obtenido %q", etag, got)
	}
	if got := rec.Header().Get("Cache-Control"); !strings.Contains(got, "no-store") {
		t.Fatalf("Cache-Control debe incluir no-store, obtenido %q", got)
	}
	if got := rec.Header().Get("Content-Type"); !strings.Contains(got, "application/json") {
		t.Fatalf("Content-Type inesperado: %q", got)
	}
}

// exigirErrorHTTP comprueba el código de estado y el código del envelope canónico.
func exigirErrorHTTP(t *testing.T, rec *httptest.ResponseRecorder, estado int, codigo string) {
	t.Helper()
	if rec.Code != estado {
		t.Fatalf("se esperaba %d, obtenido %d: %s", estado, rec.Code, rec.Body.String())
	}
	envelope := decodeError(t, rec)
	if envelope.Error.Code != codigo {
		t.Fatalf("codigo de error esperado %q, obtenido %q (%s)", codigo, envelope.Error.Code, rec.Body.String())
	}
}

// ---------------- E024 ----------------

// TestE024HTTPObtieneIngresoConCuatroCamposYETag cubre el 200 del contrato con el objeto
// cerrado y el validador fuerte que E025 exigirá después.
func TestE024HTTPObtieneIngresoConCuatroCamposYETag(t *testing.T) {
	store := newIngresoStoreHTTP()
	store.snapshots[e004HTTPUsuario] = appIdentity.IncomeSnapshot{
		AmountCents: 250_000,
		Frequency:   domainIdentity.FrequencyMonthly,
		Version:     2,
	}

	rec := httptest.NewRecorder()
	e024HandlerGET(store).ServeHTTP(rec, e004Peticion(http.MethodGet, "/api/v1/income", "jwt-valido", ""))

	exigirIngreso200(t, rec, ingresoHTTPRespuesta{
		Amount:    "2500.00",
		Currency:  "MXN",
		Frequency: "monthly",
		Version:   2,
	}, `"v2"`)

	if cuerpo := rec.Body.String(); strings.Contains(cuerpo, "monthlyEquivalent") || strings.Contains(cuerpo, "USER_INCOME") {
		t.Fatalf("la respuesta no debe exponer atributos internos: %q", cuerpo)
	}
	if escrituras := store.escriturasCapturadas(); len(escrituras) != 0 {
		t.Fatalf("E024 es solo lectura, se registraron %d escrituras", len(escrituras))
	}
}

// TestE024HTTPPrecisionDeCentimos cubre el mínimo y el máximo publicados sin pérdida de
// precisión: la respuesta se formatea desde céntimos enteros.
func TestE024HTTPPrecisionDeCentimos(t *testing.T) {
	casos := []struct {
		nombre   string
		centavos int64
		esperado string
		version  int64
	}{
		{"un centimo", 1, "0.01", 1},
		{"maximo publicado", domainIdentity.MaxIncomeAmountCents, "999999999999.99", 7},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			store := newIngresoStoreHTTP()
			store.snapshots[e004HTTPUsuario] = appIdentity.IncomeSnapshot{
				AmountCents: caso.centavos,
				Frequency:   domainIdentity.FrequencyAnnual,
				Version:     caso.version,
			}
			rec := httptest.NewRecorder()
			e024HandlerGET(store).ServeHTTP(rec, e004Peticion(http.MethodGet, "/api/v1/income", "jwt-valido", ""))

			exigirIngreso200(t, rec, ingresoHTTPRespuesta{
				Amount:    caso.esperado,
				Currency:  "MXN",
				Frequency: "annual",
				Version:   caso.version,
			}, `"v`+strconv.FormatInt(caso.version, 10)+`"`)
		})
	}
}

// TestE024HTTPAislaTitulares demuestra que la identidad sale SÓLO del JWT verificado: dos
// sujetos distintos leen su propio ingreso y ninguno ve el del otro.
func TestE024HTTPAislaTitulares(t *testing.T) {
	store := newIngresoStoreHTTP()
	store.snapshots[e004HTTPUsuario] = appIdentity.IncomeSnapshot{AmountCents: 250_000, Frequency: domainIdentity.FrequencyMonthly, Version: 2}
	store.snapshots[e004HTTPOtroUser] = appIdentity.IncomeSnapshot{AmountCents: 777_700, Frequency: domainIdentity.FrequencyAnnual, Version: 5}

	handler := e024HandlerGET(store)

	propia := httptest.NewRecorder()
	handler.ServeHTTP(propia, e004Peticion(http.MethodGet, "/api/v1/income", "jwt-valido", ""))
	exigirIngreso200(t, propia, ingresoHTTPRespuesta{Amount: "2500.00", Currency: "MXN", Frequency: "monthly", Version: 2}, `"v2"`)

	ajena := httptest.NewRecorder()
	handler.ServeHTTP(ajena, e004Peticion(http.MethodGet, "/api/v1/income", "jwt-sujeto-ajeno", ""))
	exigirIngreso200(t, ajena, ingresoHTTPRespuesta{Amount: "7777.00", Currency: "MXN", Frequency: "annual", Version: 5}, `"v5"`)

	for _, rec := range []*httptest.ResponseRecorder{propia, ajena} {
		cuerpo := rec.Body.String()
		if strings.Contains(cuerpo, e004HTTPUsuario) || strings.Contains(cuerpo, e004HTTPOtroUser) {
			t.Fatalf("la respuesta no debe incluir identificadores de usuario: %q", cuerpo)
		}
	}
}

// TestE024HTTP404SinIngreso cubre el 404 sin inventar un importe cero.
func TestE024HTTP404SinIngreso(t *testing.T) {
	rec := httptest.NewRecorder()
	e024HandlerGET(newIngresoStoreHTTP()).ServeHTTP(rec, e004Peticion(http.MethodGet, "/api/v1/income", "jwt-valido", ""))

	exigirErrorHTTP(t, rec, http.StatusNotFound, "NOT_FOUND")
	if cuerpo := rec.Body.String(); strings.Contains(cuerpo, `"amount"`) || strings.Contains(cuerpo, "0.00") {
		t.Fatalf("el 404 no debe contener importes: %q", cuerpo)
	}
}

// ---------------- E025 ----------------

// TestE025HTTPConfiguraIngresoYDevuelveETag cubre el 200 de la escritura: cuerpo persistido en
// céntimos, equivalente mensual calculado por el servidor y ETag nuevo.
func TestE025HTTPConfiguraIngresoYDevuelveETag(t *testing.T) {
	store := newIngresoStoreHTTP()

	rec := httptest.NewRecorder()
	e025HandlerPUT(store).ServeHTTP(rec, e025Peticion("jwt-valido", `"v1"`, e024CuerpoIngreso("2500.00", "MXN", "monthly")))

	exigirIngreso200(t, rec, ingresoHTTPRespuesta{
		Amount:    "2500.00",
		Currency:  "MXN",
		Frequency: "monthly",
		Version:   2,
	}, `"v2"`)

	escrituras := store.escriturasCapturadas()
	if len(escrituras) != 1 {
		t.Fatalf("se esperaba una escritura, hay %d", len(escrituras))
	}
	escritura := escrituras[0]
	if escritura.UserID != e004HTTPUsuario {
		t.Fatalf("la escritura debe usar el sujeto del JWT, obtenido %q", escritura.UserID)
	}
	if escritura.AmountCents != 250_000 {
		t.Fatalf("amountCents inesperado: %d", escritura.AmountCents)
	}
	if escritura.Frequency != domainIdentity.FrequencyMonthly {
		t.Fatalf("frecuencia persistida inesperada: %q", escritura.Frequency)
	}
	if escritura.MonthlyEquivalentCents != 250_000 {
		t.Fatalf("equivalente mensual inesperado: %d", escritura.MonthlyEquivalentCents)
	}
	if escritura.ExpectedVersion != 1 {
		t.Fatalf("version esperada inesperada: %d", escritura.ExpectedVersion)
	}
	if !escritura.Now.Equal(e024HTTPAhora) {
		t.Fatalf("la marca de tiempo debe venir del reloj del servicio: %s", escritura.Now)
	}
}

// TestE025HTTPNormalizaAliasDeFrecuencia cubre RFC-020 §2: los alias publicados se aceptan
// como ENTRADA, se persisten canónicos y nunca se emiten en la respuesta.
func TestE025HTTPNormalizaAliasDeFrecuencia(t *testing.T) {
	casos := []struct {
		nombre       string
		enviada      string
		importe      string
		canonica     string
		equivalente  int64
		centavosBase int64
	}{
		{"biweekly", "biweekly", "1000.00", "fortnightly", 200_000, 100_000},
		{"yearly", "yearly", "1200.00", "annual", 10_000, 120_000},
		{"mayusculas", "BIWEEKLY", "1000.00", "fortnightly", 200_000, 100_000},
		{"con espacios", " yearly ", "1200.00", "annual", 10_000, 120_000},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			store := newIngresoStoreHTTP()
			rec := httptest.NewRecorder()
			e025HandlerPUT(store).ServeHTTP(rec, e025Peticion("jwt-valido", `"v1"`, e024CuerpoIngreso(caso.importe, "MXN", caso.enviada)))

			exigirIngreso200(t, rec, ingresoHTTPRespuesta{
				Amount:    caso.importe,
				Currency:  "MXN",
				Frequency: caso.canonica,
				Version:   2,
			}, `"v2"`)

			escrituras := store.escriturasCapturadas()
			if len(escrituras) != 1 {
				t.Fatalf("se esperaba una escritura, hay %d", len(escrituras))
			}
			if persistida := escrituras[0].Frequency.String(); persistida != caso.canonica {
				t.Fatalf("frecuencia persistida %q, esperada %q", persistida, caso.canonica)
			}
			if escrituras[0].AmountCents != caso.centavosBase {
				t.Fatalf("amountCents inesperado: %d", escrituras[0].AmountCents)
			}
			if escrituras[0].MonthlyEquivalentCents != caso.equivalente {
				t.Fatalf("equivalente mensual %d, esperado %d", escrituras[0].MonthlyEquivalentCents, caso.equivalente)
			}
			if cuerpo := rec.Body.String(); strings.Contains(cuerpo, "biweekly") || strings.Contains(cuerpo, "yearly") {
				t.Fatalf("la respuesta no debe reemitir ningun alias: %q", cuerpo)
			}
		})
	}
}

// TestE025HTTPPrecisionDeUnCentimo cubre el mínimo representable de extremo a extremo.
func TestE025HTTPPrecisionDeUnCentimo(t *testing.T) {
	store := newIngresoStoreHTTP()
	rec := httptest.NewRecorder()
	e025HandlerPUT(store).ServeHTTP(rec, e025Peticion("jwt-valido", `"v1"`, e024CuerpoIngreso("0.01", "MXN", "monthly")))

	exigirIngreso200(t, rec, ingresoHTTPRespuesta{Amount: "0.01", Currency: "MXN", Frequency: "monthly", Version: 2}, `"v2"`)

	escrituras := store.escriturasCapturadas()
	if len(escrituras) != 1 || escrituras[0].AmountCents != 1 {
		t.Fatalf("un centimo debe persistirse como 1 centavo: %+v", escrituras)
	}
}

// TestE025HTTPRechaza400DeEsquema separa el ESQUEMA (400) de la regla de negocio (422):
// formato del importe, float/entero crudo, moneda fuera del enum, campo desconocido y cuerpo
// incompleto no llegan a la persistencia.
func TestE025HTTPRechaza400DeEsquema(t *testing.T) {
	casos := []struct {
		nombre string
		cuerpo string
	}{
		{"importe sin decimales", `{"amount":"1000","currency":"MXN","frequency":"monthly"}`},
		{"importe con tres decimales", `{"amount":"1000.000","currency":"MXN","frequency":"monthly"}`},
		{"importe float crudo", `{"amount":1000.00,"currency":"MXN","frequency":"monthly"}`},
		{"importe entero crudo", `{"amount":1000,"currency":"MXN","frequency":"monthly"}`},
		{"importe con cero a la izquierda", `{"amount":"01.00","currency":"MXN","frequency":"monthly"}`},
		{"importe negativo", `{"amount":"-1.00","currency":"MXN","frequency":"monthly"}`},
		{"importe con separador de miles", `{"amount":"1,000.00","currency":"MXN","frequency":"monthly"}`},
		{"importe vacio", `{"amount":"","currency":"MXN","frequency":"monthly"}`},
		{"importe ausente", `{"currency":"MXN","frequency":"monthly"}`},
		{"trece digitos enteros", `{"amount":"1000000000000.00","currency":"MXN","frequency":"monthly"}`},
		{"moneda fuera del enum", `{"amount":"1000.00","currency":"USD","frequency":"monthly"}`},
		{"moneda ausente", `{"amount":"1000.00","frequency":"monthly"}`},
		{"frecuencia ausente", `{"amount":"1000.00","currency":"MXN"}`},
		{"frecuencia en blanco", `{"amount":"1000.00","currency":"MXN","frequency":"   "}`},
		{"campo desconocido interno", `{"amount":"1000.00","currency":"MXN","frequency":"monthly","monthlyEquivalentCents":100000}`},
		{"campo desconocido de identidad", `{"amount":"1000.00","currency":"MXN","frequency":"monthly","userId":"otro"}`},
		{"json malformado", `{"amount":`},
		{"objeto vacio", `{}`},
		{"cuerpo vacio", ``},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			store := newIngresoStoreHTTP()
			rec := httptest.NewRecorder()
			e025HandlerPUT(store).ServeHTTP(rec, e025Peticion("jwt-valido", `"v1"`, caso.cuerpo))

			exigirErrorHTTP(t, rec, http.StatusBadRequest, "INVALID_REQUEST")
			if escrituras := store.escriturasCapturadas(); len(escrituras) != 0 {
				t.Fatalf("una solicitud invalida no debe escribir: %+v", escrituras)
			}
			if perfiles := store.perfiles; len(perfiles) != 0 {
				t.Fatalf("una solicitud invalida no debe consultar el perfil: %v", perfiles)
			}
		})
	}
}

// TestE025HTTP422ReglaDeNegocio cubre la separación de RFC-020 §5: importe cero y frecuencia
// fuera de los nueve valores publicados son 422 con detalle de campo.
func TestE025HTTP422ReglaDeNegocio(t *testing.T) {
	casos := []struct {
		nombre string
		cuerpo string
		campo  string
		razon  string
	}{
		{"importe cero", e024CuerpoIngreso("0.00", "MXN", "monthly"), "amount", "out_of_range"},
		{"frecuencia daily", e024CuerpoIngreso("1000.00", "MXN", "daily"), "frequency", "out_of_enum"},
		{"frecuencia once", e024CuerpoIngreso("1000.00", "MXN", "once"), "frequency", "out_of_enum"},
		{"frecuencia hourly", e024CuerpoIngreso("1000.00", "MXN", "hourly"), "frequency", "out_of_enum"},
		{"frecuencia inexistente", e024CuerpoIngreso("1000.00", "MXN", "cada-mes"), "frequency", "out_of_enum"},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			store := newIngresoStoreHTTP()
			rec := httptest.NewRecorder()
			e025HandlerPUT(store).ServeHTTP(rec, e025Peticion("jwt-valido", `"v1"`, caso.cuerpo))

			exigirErrorHTTP(t, rec, http.StatusUnprocessableEntity, "BUSINESS_RULE_VIOLATION")
			envelope := decodeError(t, rec)
			if len(envelope.Error.Details) != 1 {
				t.Fatalf("se esperaba un detalle, hay %d: %s", len(envelope.Error.Details), rec.Body.String())
			}
			detalle := envelope.Error.Details[0]
			if detalle.Field != caso.campo || detalle.Reason != caso.razon {
				t.Fatalf("detalle inesperado: %+v (esperado %s/%s)", detalle, caso.campo, caso.razon)
			}
			if escrituras := store.escriturasCapturadas(); len(escrituras) != 0 {
				t.Fatalf("una regla de negocio violada no debe escribir: %+v", escrituras)
			}
		})
	}
}

// TestE025HTTPAceptaElMaximoPublicado comprueba el borde superior exacto del patrón publicado.
func TestE025HTTPAceptaElMaximoPublicado(t *testing.T) {
	store := newIngresoStoreHTTP()
	rec := httptest.NewRecorder()
	e025HandlerPUT(store).ServeHTTP(rec, e025Peticion("jwt-valido", `"v1"`, e024CuerpoIngreso("999999999999.99", "MXN", "monthly")))

	exigirIngreso200(t, rec, ingresoHTTPRespuesta{
		Amount:    "999999999999.99",
		Currency:  "MXN",
		Frequency: "monthly",
		Version:   2,
	}, `"v2"`)

	escrituras := store.escriturasCapturadas()
	if len(escrituras) != 1 || escrituras[0].AmountCents != domainIdentity.MaxIncomeAmountCents {
		t.Fatalf("el maximo publicado debe persistirse exacto: %+v", escrituras)
	}
}

// TestE024E025HTTP401Autenticacion cubre sin cabecera, esquema inválido, token desconocido,
// expirado y sesión revocada; en E025 la autenticación precede a la precondición.
func TestE024E025HTTP401Autenticacion(t *testing.T) {
	t.Run("get", func(t *testing.T) {
		casos := []struct {
			nombre   string
			cabecera string
			sesiones *e004Sesiones
		}{
			{"sin cabecera", "", nil},
			{"esquema basico", "Basic abc", nil},
			{"token desconocido", "Bearer desconocido", nil},
			{"token expirado", "Bearer jwt-expirado", nil},
			{"sesion revocada", "Bearer jwt-valido", revocadaE024()},
		}
		for _, caso := range casos {
			t.Run(caso.nombre, func(t *testing.T) {
				sesiones := caso.sesiones
				if sesiones == nil {
					sesiones = newE004Sesiones()
				}
				handler := e024HandlerGETConSesiones(newIngresoStoreHTTP(), sesiones)
				req := e004Peticion(http.MethodGet, "/api/v1/income", "", "")
				if caso.cabecera != "" {
					req.Header.Set("Authorization", caso.cabecera)
				}
				rec := httptest.NewRecorder()
				handler.ServeHTTP(rec, req)
				if rec.Code != http.StatusUnauthorized {
					t.Fatalf("se esperaba 401, obtenido %d: %s", rec.Code, rec.Body.String())
				}
			})
		}
	})

	t.Run("put sin token ni precondicion sigue siendo 401", func(t *testing.T) {
		store := newIngresoStoreHTTP()
		rec := httptest.NewRecorder()
		e025HandlerPUT(store).ServeHTTP(rec, e025Peticion("", "", e024CuerpoIngreso("1000.00", "MXN", "monthly")))

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("la autenticacion precede a If-Match: se esperaba 401, obtenido %d", rec.Code)
		}
		if escrituras := store.escriturasCapturadas(); len(escrituras) != 0 {
			t.Fatalf("sin identidad no debe escribirse: %+v", escrituras)
		}
	})
}

func revocadaE024() *e004Sesiones {
	sesiones := newE004Sesiones()
	sesiones.revocar(e004HTTPSesion)
	return sesiones
}

// TestE025HTTP404PerfilInexistente impide crear un ingreso huérfano de una cuenta eliminada.
func TestE025HTTP404PerfilInexistente(t *testing.T) {
	store := newIngresoStoreHTTP()
	store.perfilExiste = false

	rec := httptest.NewRecorder()
	e025HandlerPUT(store).ServeHTTP(rec, e025Peticion("jwt-valido", `"v1"`, e024CuerpoIngreso("1000.00", "MXN", "monthly")))

	exigirErrorHTTP(t, rec, http.StatusNotFound, "NOT_FOUND")
	if escrituras := store.escriturasCapturadas(); len(escrituras) != 0 {
		t.Fatalf("sin perfil no debe escribirse el ingreso: %+v", escrituras)
	}
}

// TestE025HTTP409Conflicto demuestra que un conflicto transitorio no se confunde con 412.
func TestE025HTTP409Conflicto(t *testing.T) {
	store := newIngresoStoreHTTP()
	store.errEscritura = appIdentity.ErrIncomeConflict

	rec := httptest.NewRecorder()
	e025HandlerPUT(store).ServeHTTP(rec, e025Peticion("jwt-valido", `"v1"`, e024CuerpoIngreso("1000.00", "MXN", "monthly")))

	exigirErrorHTTP(t, rec, http.StatusConflict, "CONFLICT")
}

// TestE025HTTP412Precondicion cubre el ETag débil (no equivalente a uno fuerte) y la versión
// obsoleta. En ambos casos NADA se escribe.
func TestE025HTTP412Precondicion(t *testing.T) {
	t.Run("etag debil", func(t *testing.T) {
		store := newIngresoStoreHTTP()
		rec := httptest.NewRecorder()
		e025HandlerPUT(store).ServeHTTP(rec, e025Peticion("jwt-valido", `W/"v1"`, e024CuerpoIngreso("1000.00", "MXN", "monthly")))

		exigirErrorHTTP(t, rec, http.StatusPreconditionFailed, "VERSION_CONFLICT")
		if escrituras := store.escriturasCapturadas(); len(escrituras) != 0 {
			t.Fatalf("un ETag debil no debe escribir: %+v", escrituras)
		}
	})

	t.Run("version obsoleta", func(t *testing.T) {
		store := newIngresoStoreHTTP()
		store.errEscritura = appIdentity.ErrIncomeVersionConflict
		rec := httptest.NewRecorder()
		e025HandlerPUT(store).ServeHTTP(rec, e025Peticion("jwt-valido", `"v2"`, e024CuerpoIngreso("1000.00", "MXN", "monthly")))

		exigirErrorHTTP(t, rec, http.StatusPreconditionFailed, "VERSION_CONFLICT")
	})
}

// TestE025HTTP428PrecondicionRequerida cubre la ausencia de `If-Match`.
func TestE025HTTP428PrecondicionRequerida(t *testing.T) {
	casos := []struct {
		nombre  string
		ifMatch string
	}{
		{"cabecera ausente", ""},
		{"cabecera en blanco", "   "},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			store := newIngresoStoreHTTP()
			req := e025Peticion("jwt-valido", "", e024CuerpoIngreso("1000.00", "MXN", "monthly"))
			if caso.ifMatch != "" {
				req.Header.Set("If-Match", caso.ifMatch)
			}
			rec := httptest.NewRecorder()
			e025HandlerPUT(store).ServeHTTP(rec, req)

			exigirErrorHTTP(t, rec, http.StatusPreconditionRequired, "PRECONDITION_REQUIRED")
			if escrituras := store.escriturasCapturadas(); len(escrituras) != 0 {
				t.Fatalf("sin precondicion no debe escribirse: %+v", escrituras)
			}
		})
	}
}

// TestE025HTTPIfMatchMalFormadoEs400 separa una precondición MAL FORMADA (400) de una
// precondición no satisfecha (412).
func TestE025HTTPIfMatchMalFormadoEs400(t *testing.T) {
	for _, ifMatch := range []string{`v1`, `"1"`, `"v0"`, `"vX"`, `"v1", "v2"`, `*`} {
		t.Run(ifMatch, func(t *testing.T) {
			store := newIngresoStoreHTTP()
			rec := httptest.NewRecorder()
			e025HandlerPUT(store).ServeHTTP(rec, e025Peticion("jwt-valido", ifMatch, e024CuerpoIngreso("1000.00", "MXN", "monthly")))

			exigirErrorHTTP(t, rec, http.StatusBadRequest, "INVALID_REQUEST")
			envelope := decodeError(t, rec)
			if len(envelope.Error.Details) != 1 || envelope.Error.Details[0].Field != "If-Match" {
				t.Fatalf("el detalle debe señalar If-Match: %s", rec.Body.String())
			}
			if escrituras := store.escriturasCapturadas(); len(escrituras) != 0 {
				t.Fatalf("un If-Match mal formado no debe escribir: %+v", escrituras)
			}
		})
	}
}

// TestE024E025HTTP503Dependencia cubre el fallo controlado: nunca 200 sin datos ni escritura
// confirmada, y 503 cuando falta la composición obligatoria.
func TestE024E025HTTP503Dependencia(t *testing.T) {
	t.Run("lectura caida", func(t *testing.T) {
		store := newIngresoStoreHTTP()
		store.errLectura = errDependenciaE024()
		rec := httptest.NewRecorder()
		e024HandlerGET(store).ServeHTTP(rec, e004Peticion(http.MethodGet, "/api/v1/income", "jwt-valido", ""))
		exigirErrorHTTP(t, rec, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE")
	})

	t.Run("perfil no verificable", func(t *testing.T) {
		store := newIngresoStoreHTTP()
		store.errPerfil = errDependenciaE024()
		rec := httptest.NewRecorder()
		e025HandlerPUT(store).ServeHTTP(rec, e025Peticion("jwt-valido", `"v1"`, e024CuerpoIngreso("1000.00", "MXN", "monthly")))
		exigirErrorHTTP(t, rec, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE")
	})

	t.Run("escritura caida", func(t *testing.T) {
		store := newIngresoStoreHTTP()
		store.errEscritura = errDependenciaE024()
		rec := httptest.NewRecorder()
		e025HandlerPUT(store).ServeHTTP(rec, e025Peticion("jwt-valido", `"v1"`, e024CuerpoIngreso("1000.00", "MXN", "monthly")))
		exigirErrorHTTP(t, rec, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE")
	})

	t.Run("handler sin servicio", func(t *testing.T) {
		rec := httptest.NewRecorder()
		internalhttp.NewIncomeGetHandler(nil, e004TokensValidos(), newE004Sesiones(), nil, silencioso()).
			ServeHTTP(rec, e004Peticion(http.MethodGet, "/api/v1/income", "", ""))
		exigirErrorHTTP(t, rec, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE")

		recPut := httptest.NewRecorder()
		internalhttp.NewIncomePutHandler(nil, e004TokensValidos(), newE004Sesiones(), nil, silencioso()).
			ServeHTTP(recPut, e025Peticion("", `"v1"`, e024CuerpoIngreso("1000.00", "MXN", "monthly")))
		exigirErrorHTTP(t, recPut, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE")
	})

	t.Run("sin autorizador o sin tokens", func(t *testing.T) {
		servicio := e024ServicioHTTP(newIngresoStoreHTTP())

		sinSesiones := httptest.NewRecorder()
		internalhttp.NewIncomeGetHandler(servicio, e004TokensValidos(), nil, nil, silencioso()).
			ServeHTTP(sinSesiones, e004Peticion(http.MethodGet, "/api/v1/income", "jwt-valido", ""))
		exigirErrorHTTP(t, sinSesiones, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE")

		sinTokens := httptest.NewRecorder()
		internalhttp.NewIncomePutHandler(servicio, nil, newE004Sesiones(), nil, silencioso()).
			ServeHTTP(sinTokens, e025Peticion("jwt-valido", `"v1"`, e024CuerpoIngreso("1000.00", "MXN", "monthly")))
		exigirErrorHTTP(t, sinTokens, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE")
	})
}

func errDependenciaE024() error {
	return errIngresoHTTPDep
}

// limiteIngresoHTTP es el doble de la cuota por usuario.
type limiteIngresoHTTP struct {
	permitido  bool
	retryAfter time.Duration
	err        error
	clase      string
}

func (l *limiteIngresoHTTP) Allow(_ context.Context, clase, _ string) (bool, time.Duration, error) {
	l.clase = clase
	if l.err != nil {
		return false, 0, l.err
	}
	return l.permitido, l.retryAfter, nil
}

// TestE024E025HTTPCuotaPorUsuario cubre la composición de cuota: 429 con Retry-After cuando
// se excede y fail-open documentado si el limitador falla (nunca convierte la cuota en caída).
func TestE024E025HTTPCuotaPorUsuario(t *testing.T) {
	store := newIngresoStoreHTTP()
	store.snapshots[e004HTTPUsuario] = appIdentity.IncomeSnapshot{AmountCents: 250_000, Frequency: domainIdentity.FrequencyMonthly, Version: 2}

	negado := &limiteIngresoHTTP{permitido: false, retryAfter: 30 * time.Second}
	handler := internalhttp.NewIncomeGetHandler(e024ServicioHTTP(store), e004TokensValidos(), newE004Sesiones(), negado, silencioso())
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, e004Peticion(http.MethodGet, "/api/v1/income", "jwt-valido", ""))

	exigirErrorHTTP(t, rec, http.StatusTooManyRequests, "TOO_MANY_REQUESTS")
	if got := rec.Header().Get("Retry-After"); got != "30" {
		t.Fatalf("Retry-After esperado 30, obtenido %q", got)
	}
	if negado.clase != "e024" {
		t.Fatalf("clase de cuota inesperada: %q", negado.clase)
	}

	caido := &limiteIngresoHTTP{err: errIngresoHTTPDep}
	handlerFailOpen := internalhttp.NewIncomeGetHandler(e024ServicioHTTP(store), e004TokensValidos(), newE004Sesiones(), caido, silencioso())
	recFailOpen := httptest.NewRecorder()
	handlerFailOpen.ServeHTTP(recFailOpen, e004Peticion(http.MethodGet, "/api/v1/income", "jwt-valido", ""))
	exigirIngreso200(t, recFailOpen, ingresoHTTPRespuesta{Amount: "2500.00", Currency: "MXN", Frequency: "monthly", Version: 2}, `"v2"`)

	negadoPut := &limiteIngresoHTTP{permitido: false, retryAfter: 10 * time.Second}
	handlerPut := internalhttp.NewIncomePutHandler(e024ServicioHTTP(newIngresoStoreHTTP()), e004TokensValidos(), newE004Sesiones(), negadoPut, silencioso())
	recPut := httptest.NewRecorder()
	handlerPut.ServeHTTP(recPut, e025Peticion("jwt-valido", `"v1"`, e024CuerpoIngreso("1000.00", "MXN", "monthly")))
	exigirErrorHTTP(t, recPut, http.StatusTooManyRequests, "TOO_MANY_REQUESTS")
	if negadoPut.clase != "e025" {
		t.Fatalf("clase de cuota inesperada en E025: %q", negadoPut.clase)
	}
}

// TestE024E025HTTPErroresSinDatosSensibles comprueba que ninguna respuesta de error filtra el
// token, la sesión, el titular ni el importe enviado.
func TestE024E025HTTPErroresSinDatosSensibles(t *testing.T) {
	casos := []struct {
		nombre string
		cuerpo string
	}{
		{"token invalido", ""},
		{"ifmatch invalido", e024CuerpoIngreso("1234.56", "MXN", "monthly")},
		{"regla de negocio", e024CuerpoIngreso("0.00", "MXN", "monthly")},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			store := newIngresoStoreHTTP()
			token := "jwt-valido"
			if caso.nombre == "token invalido" {
				token = "desconocido"
			}
			ifMatch := `"v1"`
			if caso.nombre == "ifmatch invalido" {
				ifMatch = "no-es-un-etag"
			}
			rec := httptest.NewRecorder()
			e025HandlerPUT(store).ServeHTTP(rec, e025Peticion(token, ifMatch, caso.cuerpo))
			if rec.Code < 400 {
				t.Fatalf("se esperaba un error, obtenido %d", rec.Code)
			}
			cuerpo := rec.Body.String()
			for _, prohibido := range []string{"jwt-valido", "desconocido", e004HTTPUsuario, e004HTTPSesion, "1234.56", "monthlyEquivalent"} {
				if strings.Contains(cuerpo, prohibido) {
					t.Fatalf("la respuesta no debe contener %q: %q", prohibido, cuerpo)
				}
			}
		})
	}
}

// El doble satisface el puerto que consume la aplicación.
var _ appIdentity.IncomeStore = (*ingresoStoreHTTP)(nil)
