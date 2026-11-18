package transport_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	appPrivacy "github.com/nexumpr/Nexum-Backend/internal/application/privacy"
	internalhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
)

// Pruebas HTTP de E017 (`POST /api/v1/users/me/consents`): middleware de autenticación y
// sesión autoritativa, contrato `Consent` con EXACTAMENTE cinco campos, `granted` obligatorio
// (ausente 422, `false` válido), validación de propósito y versión, conflicto 409, fallo
// cerrado 503 y aislamiento de identidad. Los dobles son exclusivamente de prueba; la
// composición productiva usa DynamoDB real.

var e017HTTPAhora = time.Date(2026, 11, 18, 14, 30, 0, 0, time.UTC)

const e017HTTPID = "018f90ef-08e5-7a68-b09f-0000000000c7"

type e017HTTPEscritor struct {
	mu         sync.Mutex
	decisiones []appPrivacy.ConsentDecision
	fallo      error
}

func (w *e017HTTPEscritor) AppendConsent(_ context.Context, decision appPrivacy.ConsentDecision) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.fallo != nil {
		return w.fallo
	}
	w.decisiones = append(w.decisiones, decision)
	return nil
}

func (w *e017HTTPEscritor) total() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return len(w.decisiones)
}

func (w *e017HTTPEscritor) unica(t *testing.T) appPrivacy.ConsentDecision {
	t.Helper()
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.decisiones) != 1 {
		t.Fatalf("se esperaba una decision persistida, hay %d", len(w.decisiones))
	}
	return w.decisiones[0]
}

func e017HTTPHandlerConLogger(escritor appPrivacy.ConsentWriter, logger *slog.Logger) *internalhttp.ConsentsWriteHandler {
	var servicio *appPrivacy.ConsentsWriteService
	if escritor != nil {
		servicio = appPrivacy.NewConsentsWriteService(escritor).
			WithClock(func() time.Time { return e017HTTPAhora }).
			WithIDGenerator(func() (string, error) { return e017HTTPID, nil })
	}
	return internalhttp.NewConsentsWriteHandler(servicio, e004TokensValidos(), newE004Sesiones(), logger)
}

func e017HTTPHandler(escritor appPrivacy.ConsentWriter) *internalhttp.ConsentsWriteHandler {
	return e017HTTPHandlerConLogger(escritor, silencioso())
}

func e017HTTPPeticion(token, cuerpo string) *http.Request {
	return e004Peticion(http.MethodPost, "/api/v1/users/me/consents", token, cuerpo)
}

func e017HTTPCuerpoValido() string {
	return `{"noticeVersion":"2026-11","purpose":"notifications","granted":true}`
}

// TestE017HTTPCreaConsentimiento201 cubre el caso feliz: 201 con el objeto `Consent` de cinco
// campos, cabecera de no caché y persistencia con la identidad del token.
func TestE017HTTPCreaConsentimiento201(t *testing.T) {
	escritor := &e017HTTPEscritor{}
	rec := httptest.NewRecorder()
	e017HTTPHandler(escritor).ServeHTTP(rec, e017HTTPPeticion("jwt-valido", e017HTTPCuerpoValido()))

	if rec.Code != http.StatusCreated {
		t.Fatalf("esperado 201, obtenido %d (%s)", rec.Code, rec.Body.String())
	}
	if cache := rec.Header().Get("Cache-Control"); !strings.Contains(cache, "private") || !strings.Contains(cache, "no-store") {
		t.Fatalf("Cache-Control debe ser 'private, no-store', obtenido %q", cache)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Fatalf("Content-Type debe ser JSON, obtenido %q", ct)
	}

	var cuerpo map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &cuerpo); err != nil {
		t.Fatalf("respuesta no es JSON: %v", err)
	}
	if len(cuerpo) != 5 {
		t.Fatalf("Consent es un objeto cerrado de 5 campos, se obtuvo %v", cuerpo)
	}
	for _, requerida := range []string{"id", "noticeVersion", "purpose", "granted", "recordedAt"} {
		if _, existe := cuerpo[requerida]; !existe {
			t.Fatalf("falta el campo requerido %q en %v", requerida, cuerpo)
		}
	}
	if cuerpo["id"] != e017HTTPID || cuerpo["noticeVersion"] != "2026-11" || cuerpo["purpose"] != "notifications" {
		t.Fatalf("valores del contrato incorrectos: %v", cuerpo)
	}
	if granted, ok := cuerpo["granted"].(bool); !ok || !granted {
		t.Fatalf("granted debe ser el booleano true: %v", cuerpo["granted"])
	}
	if cuerpo["recordedAt"] != "2026-11-18T14:30:00Z" {
		t.Fatalf("recordedAt debe ser el instante UTC en RFC3339: %v", cuerpo["recordedAt"])
	}

	// Defensa contra fugas: ningún atributo interno puede aparecer serializado.
	for _, prohibido := range []string{"source", "createdAt", "PK", "SK", "sortKey"} {
		if strings.Contains(rec.Body.String(), "\""+prohibido+"\"") {
			t.Fatalf("la respuesta no debe contener %q: %s", prohibido, rec.Body.String())
		}
	}

	decision := escritor.unica(t)
	if decision.UserID != e004HTTPUsuario {
		t.Fatalf("la identidad debe salir de sub, se persistio %q", decision.UserID)
	}
	if decision.ID != e017HTTPID || !decision.Granted || decision.Source != appPrivacy.ConsentSourceAPI {
		t.Fatalf("decision persistida incorrecta: %+v", decision)
	}
	if !decision.RecordedAt.Equal(e017HTTPAhora) || !decision.CreatedAt.Equal(e017HTTPAhora) {
		t.Fatalf("la decision debe fecharse con el reloj del servicio: %+v", decision)
	}
}

// TestE017HTTPGrantedFalseEsValido: una denegación explícita se acepta y se emite como
// `"granted": false`, no se omite por `omitempty` ni se invierte.
func TestE017HTTPGrantedFalseEsValido(t *testing.T) {
	escritor := &e017HTTPEscritor{}
	rec := httptest.NewRecorder()
	e017HTTPHandler(escritor).ServeHTTP(rec, e017HTTPPeticion("jwt-valido",
		`{"noticeVersion":"2026-11","purpose":"analytics","granted":false}`))

	if rec.Code != http.StatusCreated {
		t.Fatalf("esperado 201, obtenido %d (%s)", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"granted":false`) {
		t.Fatalf("granted:false debe estar presente en la respuesta: %s", rec.Body.String())
	}
	if decision := escritor.unica(t); decision.Granted {
		t.Fatalf("la persistencia debe recibir granted=false: %+v", decision)
	}
}

// TestE017HTTPGrantedAusenteEs422: sin `granted` (o con `null`) no se registra nada.
func TestE017HTTPGrantedAusenteEs422(t *testing.T) {
	casos := []string{
		`{"noticeVersion":"2026-11","purpose":"analytics"}`,
		`{"noticeVersion":"2026-11","purpose":"analytics","granted":null}`,
	}
	for _, cuerpo := range casos {
		escritor := &e017HTTPEscritor{}
		rec := httptest.NewRecorder()
		e017HTTPHandler(escritor).ServeHTTP(rec, e017HTTPPeticion("jwt-valido", cuerpo))
		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("%s: esperado 422, obtenido %d (%s)", cuerpo, rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "BUSINESS_RULE_VIOLATION") {
			t.Fatalf("debe devolver BUSINESS_RULE_VIOLATION: %s", rec.Body.String())
		}
		if escritor.total() != 0 {
			t.Fatalf("%s: no debe persistirse ninguna decision", cuerpo)
		}
	}
}

// TestE017HTTPPropositoOVersionInvalidos: enum cerrado y rango de la versión del aviso -> 422.
func TestE017HTTPPropositoOVersionInvalidos(t *testing.T) {
	casos := []string{
		`{"noticeVersion":"2026-11","purpose":"marketing","granted":true}`,
		`{"noticeVersion":"2026-11","purpose":"PrivacyNotice","granted":true}`,
		`{"noticeVersion":"","purpose":"notifications","granted":true}`,
		`{"noticeVersion":"   ","purpose":"notifications","granted":true}`,
		`{"noticeVersion":"` + strings.Repeat("v", appPrivacy.ConsentNoticeVersionMaxLength+1) + `","purpose":"notifications","granted":true}`,
	}
	for _, cuerpo := range casos {
		escritor := &e017HTTPEscritor{}
		rec := httptest.NewRecorder()
		e017HTTPHandler(escritor).ServeHTTP(rec, e017HTTPPeticion("jwt-valido", cuerpo))
		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("%s: esperado 422, obtenido %d (%s)", cuerpo, rec.Code, rec.Body.String())
		}
		if escritor.total() != 0 {
			t.Fatalf("%s: no debe persistirse ninguna decision", cuerpo)
		}
	}

	// El límite exacto (50 caracteres) sí es válido.
	escritor := &e017HTTPEscritor{}
	rec := httptest.NewRecorder()
	e017HTTPHandler(escritor).ServeHTTP(rec, e017HTTPPeticion("jwt-valido",
		`{"noticeVersion":"`+strings.Repeat("v", appPrivacy.ConsentNoticeVersionMaxLength)+`","purpose":"notifications","granted":true}`))
	if rec.Code != http.StatusCreated {
		t.Fatalf("50 caracteres deben aceptarse, obtenido %d (%s)", rec.Code, rec.Body.String())
	}
}

// TestE017HTTPCuerpoInvalidoEs400: JSON malformado, campo desconocido, tipo incorrecto, cuerpo
// vacío, datos concatenados y Content-Type no JSON -> 400 sin persistir.
func TestE017HTTPCuerpoInvalidoEs400(t *testing.T) {
	casos := []struct {
		nombre      string
		cuerpo      string
		contentType string
	}{
		{"json truncado", `{"noticeVersion":"2026-11"`, ""},
		{"campo desconocido", `{"noticeVersion":"2026-11","purpose":"analytics","granted":true,"source":"api"}`, ""},
		{"granted no booleano", `{"noticeVersion":"2026-11","purpose":"analytics","granted":"true"}`, ""},
		{"cuerpo vacio", ``, ""},
		{"datos concatenados", e017HTTPCuerpoValido() + `{}`, ""},
		{"content-type no json", e017HTTPCuerpoValido(), "text/plain"},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			escritor := &e017HTTPEscritor{}
			req := e017HTTPPeticion("jwt-valido", caso.cuerpo)
			if caso.contentType != "" {
				req.Header.Set("Content-Type", caso.contentType)
			}
			rec := httptest.NewRecorder()
			e017HTTPHandler(escritor).ServeHTTP(rec, req)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("esperado 400, obtenido %d (%s)", rec.Code, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), "INVALID_REQUEST") {
				t.Fatalf("debe devolver INVALID_REQUEST: %s", rec.Body.String())
			}
			if escritor.total() != 0 {
				t.Fatal("un cuerpo invalido no debe persistirse")
			}
		})
	}
}

// TestE017HTTPAutenticacion: sin Bearer, esquema inválido, token desconocido o expirado -> 401
// sin tocar la persistencia.
func TestE017HTTPAutenticacion(t *testing.T) {
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
			escritor := &e017HTTPEscritor{}
			req := e017HTTPPeticion("", e017HTTPCuerpoValido())
			if caso.cabecera != "" {
				req.Header.Set("Authorization", caso.cabecera)
			}
			rec := httptest.NewRecorder()
			e017HTTPHandler(escritor).ServeHTTP(rec, req)
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("esperado 401, obtenido %d (%s)", rec.Code, rec.Body.String())
			}
			if escritor.total() != 0 {
				t.Fatal("una peticion no autenticada no debe escribir nada")
			}
		})
	}
}

// TestE017HTTPSesionRevocada: una sesión revocada no llega al handler (401) ni escribe.
func TestE017HTTPSesionRevocada(t *testing.T) {
	escritor := &e017HTTPEscritor{}
	sesiones := newE004Sesiones()
	sesiones.revocar(e004HTTPSesion)
	servicio := appPrivacy.NewConsentsWriteService(escritor).
		WithClock(func() time.Time { return e017HTTPAhora }).
		WithIDGenerator(func() (string, error) { return e017HTTPID, nil })
	handler := internalhttp.NewConsentsWriteHandler(servicio, e004TokensValidos(), sesiones, silencioso())

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, e017HTTPPeticion("jwt-valido", e017HTTPCuerpoValido()))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("esperado 401, obtenido %d (%s)", rec.Code, rec.Body.String())
	}
	if escritor.total() != 0 {
		t.Fatal("una sesion revocada no debe escribir nada")
	}
}

// TestE017HTTPConflicto409: una clave física ya ocupada se traduce en 409 CONFLICT.
func TestE017HTTPConflicto409(t *testing.T) {
	escritor := &e017HTTPEscritor{fallo: appPrivacy.ErrConsentConflict}
	rec := httptest.NewRecorder()
	e017HTTPHandler(escritor).ServeHTTP(rec, e017HTTPPeticion("jwt-valido", e017HTTPCuerpoValido()))
	if rec.Code != http.StatusConflict {
		t.Fatalf("esperado 409, obtenido %d (%s)", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "CONFLICT") {
		t.Fatalf("debe devolver CONFLICT: %s", rec.Body.String())
	}
}

// TestE017HTTP503SinServicio: sin servicio no hay registro, nunca un 201 ficticio.
func TestE017HTTP503SinServicio(t *testing.T) {
	rec := httptest.NewRecorder()
	e017HTTPHandler(nil).ServeHTTP(rec, e017HTTPPeticion("jwt-valido", e017HTTPCuerpoValido()))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("esperado 503, obtenido %d (%s)", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "DEPENDENCY_UNAVAILABLE") {
		t.Fatalf("debe devolver DEPENDENCY_UNAVAILABLE: %s", rec.Body.String())
	}
}

// TestE017HTTP503PersistenciaCaida: un fallo de DynamoDB se traduce en 503.
func TestE017HTTP503PersistenciaCaida(t *testing.T) {
	escritor := &e017HTTPEscritor{fallo: errors.New("dynamodb inaccesible")}
	rec := httptest.NewRecorder()
	e017HTTPHandler(escritor).ServeHTTP(rec, e017HTTPPeticion("jwt-valido", e017HTTPCuerpoValido()))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("esperado 503, obtenido %d (%s)", rec.Code, rec.Body.String())
	}
}

// TestE017HTTPAislamientoDeIdentidad: un token de otro sujeto escribe en su propia partición;
// el cuerpo no puede elegir el usuario.
func TestE017HTTPAislamientoDeIdentidad(t *testing.T) {
	escritor := &e017HTTPEscritor{}
	rec := httptest.NewRecorder()
	e017HTTPHandler(escritor).ServeHTTP(rec, e017HTTPPeticion("jwt-sujeto-ajeno", e017HTTPCuerpoValido()))
	if rec.Code != http.StatusCreated {
		t.Fatalf("esperado 201, obtenido %d (%s)", rec.Code, rec.Body.String())
	}
	if decision := escritor.unica(t); decision.UserID != e004HTTPOtroUser {
		t.Fatalf("la persistencia debe recibir el sub del token, recibio %q", decision.UserID)
	}

	// Un campo de identidad en el cuerpo se rechaza: la identidad nunca viene del cliente.
	escritor = &e017HTTPEscritor{}
	rec = httptest.NewRecorder()
	e017HTTPHandler(escritor).ServeHTTP(rec, e017HTTPPeticion("jwt-valido",
		`{"noticeVersion":"2026-11","purpose":"notifications","granted":true,"userId":"`+e004HTTPOtroUser+`"}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("un userId en el cuerpo debe rechazarse con 400, obtenido %d (%s)", rec.Code, rec.Body.String())
	}
	if escritor.total() != 0 {
		t.Fatal("no debe persistirse nada con identidad en el cuerpo")
	}
}

// TestE017HTTPMetodoNoAdmitido: el handler de escritura sólo sirve POST.
func TestE017HTTPMetodoNoAdmitido(t *testing.T) {
	rec := httptest.NewRecorder()
	req := e004Peticion(http.MethodGet, "/api/v1/users/me/consents", "jwt-valido", "")
	e017HTTPHandler(&e017HTTPEscritor{}).ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("esperado 405, obtenido %d (%s)", rec.Code, rec.Body.String())
	}
	if allow := rec.Header().Get("Allow"); allow != http.MethodPost {
		t.Fatalf("Allow debe anunciar POST, obtenido %q", allow)
	}
}

// TestE017HTTPRutaCompartidaConE016: GET y POST conviven sobre `/users/me/consents` sin
// colisiones; un tercer método recibe 405 con ambos métodos anunciados.
func TestE017HTTPRutaCompartidaConE016(t *testing.T) {
	router := internalhttp.NewRouter()
	lectura := e016HTTPHandler(newE016HTTPStore())
	escritura := e017HTTPHandler(&e017HTTPEscritor{})
	router.HandleFunc(http.MethodGet, "/users/me/consents", lectura.ServeHTTP)
	router.HandleFunc(http.MethodPost, "/users/me/consents", escritura.ServeHTTP)

	get := httptest.NewRecorder()
	router.ServeHTTP(get, e004Peticion(http.MethodGet, "/api/v1/users/me/consents", "jwt-valido", ""))
	if get.Code != http.StatusOK {
		t.Fatalf("GET debe servirse con E016: %d (%s)", get.Code, get.Body.String())
	}

	post := httptest.NewRecorder()
	router.ServeHTTP(post, e004Peticion(http.MethodPost, "/api/v1/users/me/consents", "jwt-valido", e017HTTPCuerpoValido()))
	if post.Code != http.StatusCreated {
		t.Fatalf("POST debe servirse con E017: %d (%s)", post.Code, post.Body.String())
	}

	put := httptest.NewRecorder()
	router.ServeHTTP(put, e004Peticion(http.MethodPut, "/api/v1/users/me/consents", "jwt-valido", e017HTTPCuerpoValido()))
	if put.Code != http.StatusMethodNotAllowed {
		t.Fatalf("PUT debe ser 405, obtenido %d (%s)", put.Code, put.Body.String())
	}
	allow := put.Header().Get("Allow")
	if !strings.Contains(allow, http.MethodGet) || !strings.Contains(allow, http.MethodPost) {
		t.Fatalf("Allow debe anunciar GET y POST, obtenido %q", allow)
	}
}

// TestE017HTTPLogsSinDatosPersonales: los errores se registran por clase, sin userId, sin la
// versión del aviso ni el propósito enviados.
func TestE017HTTPLogsSinDatosPersonales(t *testing.T) {
	var registro bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&registro, nil))
	escritor := &e017HTTPEscritor{fallo: errors.New("dynamodb inaccesible")}

	rec := httptest.NewRecorder()
	e017HTTPHandlerConLogger(escritor, logger).ServeHTTP(rec,
		e017HTTPPeticion("jwt-valido", `{"noticeVersion":"2031-07","purpose":"analytics","granted":true}`))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("esperado 503, obtenido %d (%s)", rec.Code, rec.Body.String())
	}
	if registro.Len() == 0 {
		t.Fatal("el fallo de persistencia debe registrarse para diagnostico")
	}
	salida := registro.String()
	for _, prohibido := range []string{e004HTTPUsuario, "2031-07", "analytics", "granted"} {
		if strings.Contains(salida, prohibido) {
			t.Fatalf("el log no debe contener %q: %s", prohibido, salida)
		}
	}
	if !strings.Contains(salida, "consents_write_unavailable") {
		t.Fatalf("el log debe identificar la clase de error: %s", salida)
	}
}
