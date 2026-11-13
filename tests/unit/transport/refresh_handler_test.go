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

	appIdentity "github.com/nexumpr/Nexum-Backend/internal/application/identity"
	internalhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
)

// ---------- Dobles de prueba (nunca se usan en la composición productiva) ----------

const (
	e003UserID    = "47622cbc-36ec-4b30-9b8a-d9c26833a395"
	e003SessionID = "018f90ef-08e5-7a68-b09f-0000000000a1"
	e003FamilyID  = "018f90ef-08e5-7a68-b09f-0000000000b2"
	e003RefreshA  = "rotacion-sintetica-a"
	e003RefreshB  = "rotacion-sintetica-b"
)

var e003Ahora = time.Date(2026, 11, 12, 22, 0, 0, 0, time.UTC)

type e003Store struct {
	mu          sync.Mutex
	locators    map[string]*appIdentity.RefreshLocator
	sessions    map[string]*appIdentity.RefreshSession
	users       map[string]*appIdentity.RefreshUser
	rotate      int
	revoke      int
	rotateFallo error
}

func newE003Store() *e003Store {
	digest := appIdentity.HashToken(e003RefreshA)
	expira := e003Ahora.Add(20 * 24 * time.Hour)
	return &e003Store{
		locators: map[string]*appIdentity.RefreshLocator{
			digest: {
				UserID:    e003UserID,
				SessionID: e003SessionID,
				FamilyID:  e003FamilyID,
				Status:    "ACTIVE",
				ExpiresAt: expira,
				HasExpiry: true,
			},
		},
		sessions: map[string]*appIdentity.RefreshSession{
			e003SessionID: {
				UserID:      e003UserID,
				SessionID:   e003SessionID,
				FamilyID:    e003FamilyID,
				RefreshHash: digest,
				Status:      "ACTIVE",
				ExpiresAt:   expira,
				HasExpiry:   true,
				Version:     1,
			},
		},
		users: map[string]*appIdentity.RefreshUser{
			e003UserID: {UserID: e003UserID, Status: "ACTIVE"},
		},
	}
}

func (s *e003Store) LocateRefresh(_ context.Context, digest string) (*appIdentity.RefreshLocator, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	locator, ok := s.locators[digest]
	if !ok {
		return nil, nil
	}
	copia := *locator
	return &copia, nil
}

func (s *e003Store) LoadSession(_ context.Context, _, sessionID string) (*appIdentity.RefreshSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.sessions[sessionID]
	if !ok {
		return nil, nil
	}
	copia := *session
	return &copia, nil
}

func (s *e003Store) LoadUserProfile(_ context.Context, userID string) (*appIdentity.RefreshUser, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	user, ok := s.users[userID]
	if !ok {
		return nil, nil
	}
	copia := *user
	return &copia, nil
}

func (s *e003Store) RotateRefresh(_ context.Context, ro appIdentity.RefreshRotation) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.rotateFallo != nil {
		return s.rotateFallo
	}
	locator, ok := s.locators[ro.OldDigest]
	if !ok || locator.Used {
		return appIdentity.ErrRefreshConflict
	}
	session := s.sessions[ro.SessionID]
	if session == nil || session.RefreshHash != ro.OldDigest || session.Version != ro.ExpectedVersion {
		return appIdentity.ErrRefreshConflict
	}
	s.rotate++
	locator.Used = true
	locator.Status = "CONSUMED"
	session.RefreshHash = ro.NewDigest
	session.Version = ro.NewVersion
	s.locators[ro.NewDigest] = &appIdentity.RefreshLocator{
		UserID:    ro.UserID,
		SessionID: ro.SessionID,
		FamilyID:  ro.FamilyID,
		Status:    "ACTIVE",
		ExpiresAt: ro.ExpiresAt,
		HasExpiry: true,
	}
	return nil
}

func (s *e003Store) RevokeFamily(_ context.Context, rv appIdentity.RefreshRevocation) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.revoke++
	if session, ok := s.sessions[rv.SessionID]; ok {
		session.Status = "REVOKED"
	}
	return nil
}

type e003Limiter struct {
	allow  bool
	err    error
	ip     string
	digest string
	retry  time.Duration
}

func (l *e003Limiter) AllowRefresh(_ context.Context, ip, digest string) (bool, time.Duration, error) {
	l.ip = ip
	l.digest = digest
	return l.allow, l.retry, l.err
}

type e003Issuer struct{ err error }

func (i *e003Issuer) IssueAccessToken(context.Context, string, string, time.Time) (string, string, error) {
	if i.err != nil {
		return "", "", i.err
	}
	return "jwt-de-acceso-sintetico", "jti-nuevo", nil
}

func nuevoHandlerE003(store appIdentity.RefreshStore, limiter internalhttp.RefreshRateLimiter) *internalhttp.RefreshHandler {
	svc := appIdentity.NewRefreshService(store, &e003Issuer{}).
		WithClock(func() time.Time { return e003Ahora }).
		WithRefreshGenerator(func() (string, error) { return e003RefreshB, nil })
	return internalhttp.NewRefreshHandler(svc, limiter, silencioso())
}

func peticionE003(cuerpo string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", strings.NewReader(cuerpo))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Forwarded-For", "203.0.113.7, 130.176.10.20")
	req.RemoteAddr = "10.42.1.10:443"
	return req
}

// ---------- Contrato 200 ----------

func TestE003ContratoExitoso(t *testing.T) {
	store := newE003Store()
	limiter := &e003Limiter{allow: true}
	rec := httptest.NewRecorder()

	nuevoHandlerE003(store, limiter).ServeHTTP(rec, peticionE003(`{"refreshToken":"`+e003RefreshA+`"}`))

	if rec.Code != http.StatusOK {
		t.Fatalf("se esperaba 200, obtenido %d: %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatalf("Cache-Control inesperado: %q", rec.Header().Get("Cache-Control"))
	}

	var cuerpo map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &cuerpo); err != nil {
		t.Fatalf("respuesta no es JSON: %v", err)
	}
	esperadas := []string{"accessToken", "refreshToken", "tokenType", "expiresIn", "sessionId", "session"}
	if len(cuerpo) != len(esperadas) {
		t.Fatalf("propiedades inesperadas en la respuesta: %v", cuerpo)
	}
	for _, clave := range esperadas {
		if _, ok := cuerpo[clave]; !ok {
			t.Fatalf("falta la propiedad %q: %v", clave, cuerpo)
		}
	}
	if cuerpo["tokenType"] != "Bearer" {
		t.Fatalf("tokenType inesperado: %v", cuerpo["tokenType"])
	}
	if cuerpo["expiresIn"].(float64) != 900 {
		t.Fatalf("expiresIn debe ser 900: %v", cuerpo["expiresIn"])
	}
	if cuerpo["sessionId"] != e003SessionID {
		t.Fatalf("sessionId debe conservar la sesion real: %v", cuerpo["sessionId"])
	}
	sesion, ok := cuerpo["session"].(map[string]any)
	if !ok || sesion["id"] != e003SessionID {
		t.Fatalf("session.id debe igualar sessionId: %v", cuerpo["session"])
	}
	if cuerpo["refreshToken"] == e003RefreshA {
		t.Fatal("el refresh debe rotarse, no repetirse")
	}
	if cuerpo["accessToken"] != "jwt-de-acceso-sintetico" {
		t.Fatalf("access token inesperado: %v", cuerpo["accessToken"])
	}
	if store.rotate != 1 {
		t.Fatalf("debe persistirse una rotacion, hay %d", store.rotate)
	}
	if store.revoke != 0 {
		t.Fatal("una rotacion correcta no revoca")
	}
	// El limitador recibe la IP del visor (no la del borde) y el digest, nunca el token.
	if limiter.ip != "203.0.113.7" {
		t.Fatalf("IP entregada al limitador inesperada: %q", limiter.ip)
	}
	if limiter.digest != appIdentity.HashToken(e003RefreshA) {
		t.Fatalf("el limitador debe recibir el digest, no el token: %q", limiter.digest)
	}
}

func TestE003AceptaDeviceIDOpcional(t *testing.T) {
	store := newE003Store()
	rec := httptest.NewRecorder()

	nuevoHandlerE003(store, &e003Limiter{allow: true}).
		ServeHTTP(rec, peticionE003(`{"refreshToken":"`+e003RefreshA+`","deviceId":"installation-6a4f1d8c"}`))

	if rec.Code != http.StatusOK {
		t.Fatalf("deviceId opcional no debe romper el contrato: %d %s", rec.Code, rec.Body.String())
	}
}

// ---------- Validación (400) ----------

func TestE003RechazaSolicitudesInvalidas(t *testing.T) {
	casos := []struct {
		nombre      string
		cuerpo      string
		contentType string
		motivo      string
	}{
		{"json malformado", `{"refreshToken":`, "application/json", "invalid_json"},
		{"campo desconocido", `{"refreshToken":"x","email":"a@b.c"}`, "application/json", "invalid_json"},
		{"token ausente", `{}`, "application/json", "required"},
		{"token vacio", `{"refreshToken":""}`, "application/json", "required"},
		{"token tipo incorrecto", `{"refreshToken":123}`, "application/json", "invalid_json"},
		{"device tipo incorrecto", `{"refreshToken":"x","deviceId":7}`, "application/json", "invalid_json"},
		{"content-type incorrecto", `{"refreshToken":"x"}`, "text/plain", "application_json_required"},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			store := newE003Store()
			req := peticionE003(caso.cuerpo)
			req.Header.Set("Content-Type", caso.contentType)
			rec := httptest.NewRecorder()

			nuevoHandlerE003(store, &e003Limiter{allow: true}).ServeHTTP(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("se esperaba 400, obtenido %d: %s", rec.Code, rec.Body.String())
			}
			if store.rotate != 0 {
				t.Fatal("una solicitud invalida no debe rotar")
			}
			if !strings.Contains(rec.Body.String(), caso.motivo) {
				t.Fatalf("motivo esperado %q en %s", caso.motivo, rec.Body.String())
			}
			if strings.Contains(rec.Body.String(), "sintetico-anterior") {
				t.Fatal("la respuesta no debe reflejar el token presentado")
			}
		})
	}
}

func TestE003RechazaTokenExcesivamenteLargo(t *testing.T) {
	store := newE003Store()
	rec := httptest.NewRecorder()
	cuerpo := `{"refreshToken":"` + strings.Repeat("a", int(internalhttp.RefreshMaxRequestBodyBytes)) + `"}`

	nuevoHandlerE003(store, &e003Limiter{allow: true}).ServeHTTP(rec, peticionE003(cuerpo))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("un cuerpo desmedido debe rechazarse, obtenido %d", rec.Code)
	}
	if store.rotate != 0 {
		t.Fatal("no debe rotarse nada")
	}
}

func TestE003RechazaCadenaDeProxiesInvalida(t *testing.T) {
	store := newE003Store()
	req := peticionE003(`{"refreshToken":"` + e003RefreshA + `"}`)
	req.Header.Set("X-Forwarded-For", "no-es-ip, 130.176.10.20")
	rec := httptest.NewRecorder()

	nuevoHandlerE003(store, &e003Limiter{allow: true}).ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("cadena manipulada debe rechazarse con 400, obtenido %d", rec.Code)
	}
}

// ---------- Seguridad (401 / 409 / 429 / 503) ----------

func TestE003TokenInvalidoEs401Generico(t *testing.T) {
	store := newE003Store()
	rec := httptest.NewRecorder()

	nuevoHandlerE003(store, &e003Limiter{allow: true}).ServeHTTP(rec, peticionE003(`{"refreshToken":"token-que-no-existe"}`))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("se esperaba 401, obtenido %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "UNAUTHORIZED") {
		t.Fatalf("codigo contractual inesperado: %s", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "no existe") || strings.Contains(rec.Body.String(), "sesion") {
		t.Fatalf("el mensaje no debe distinguir causas: %s", rec.Body.String())
	}
	if store.revoke != 0 {
		t.Fatal("un token desconocido no revoca")
	}
}

func TestE003SesionRevocadaEs401(t *testing.T) {
	store := newE003Store()
	store.sessions[e003SessionID].Status = "REVOKED"
	rec := httptest.NewRecorder()

	nuevoHandlerE003(store, &e003Limiter{allow: true}).ServeHTTP(rec, peticionE003(`{"refreshToken":"`+e003RefreshA+`"}`))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("se esperaba 401, obtenido %d", rec.Code)
	}
	if store.rotate != 0 {
		t.Fatal("una sesion revocada no rota")
	}
}

func TestE003ReutilizacionEs409YRevoca(t *testing.T) {
	store := newE003Store()
	handler := nuevoHandlerE003(store, &e003Limiter{allow: true})

	primera := httptest.NewRecorder()
	handler.ServeHTTP(primera, peticionE003(`{"refreshToken":"`+e003RefreshA+`"}`))
	if primera.Code != http.StatusOK {
		t.Fatalf("la primera rotacion debe funcionar: %d", primera.Code)
	}

	segunda := httptest.NewRecorder()
	handler.ServeHTTP(segunda, peticionE003(`{"refreshToken":"`+e003RefreshA+`"}`))
	if segunda.Code != http.StatusConflict {
		t.Fatalf("la reutilizacion debe ser 409, obtenido %d: %s", segunda.Code, segunda.Body.String())
	}
	if !strings.Contains(segunda.Body.String(), "CONFLICT") {
		t.Fatalf("codigo contractual inesperado: %s", segunda.Body.String())
	}
	if store.revoke != 1 {
		t.Fatalf("la reutilizacion debe revocar la familia, revocaciones=%d", store.revoke)
	}
	if store.sessions[e003SessionID].Status != "REVOKED" {
		t.Fatal("la sesion debe quedar revocada")
	}
}

func TestE003CuotaExcedidaEs429(t *testing.T) {
	rec := httptest.NewRecorder()
	limiter := &e003Limiter{allow: false, retry: 42 * time.Second}

	nuevoHandlerE003(newE003Store(), limiter).ServeHTTP(rec, peticionE003(`{"refreshToken":"`+e003RefreshA+`"}`))

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("se esperaba 429, obtenido %d", rec.Code)
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Fatal("429 debe incluir Retry-After")
	}
}

func TestE003FallaCerradoSinLimitador(t *testing.T) {
	store := newE003Store()
	rec := httptest.NewRecorder()

	nuevoHandlerE003(store, nil).ServeHTTP(rec, peticionE003(`{"refreshToken":"`+e003RefreshA+`"}`))

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("sin limitador debe ser 503, obtenido %d", rec.Code)
	}
	if store.rotate != 0 {
		t.Fatal("sin limitador no se toca la persistencia")
	}
}

func TestE003FallaCerradoSiElLimitadorFalla(t *testing.T) {
	rec := httptest.NewRecorder()
	limiter := &e003Limiter{allow: true, err: errors.New("redis no disponible")}

	nuevoHandlerE003(newE003Store(), limiter).ServeHTTP(rec, peticionE003(`{"refreshToken":"`+e003RefreshA+`"}`))

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("un fallo de Redis debe ser 503, obtenido %d", rec.Code)
	}
}

func TestE003FallaCerradoSiDynamoFalla(t *testing.T) {
	store := newE003Store()
	store.rotateFallo = errors.New("dynamodb no disponible")
	rec := httptest.NewRecorder()

	nuevoHandlerE003(store, &e003Limiter{allow: true}).ServeHTTP(rec, peticionE003(`{"refreshToken":"`+e003RefreshA+`"}`))

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("un fallo de persistencia debe ser 503, obtenido %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "dynamodb") {
		t.Fatalf("no debe exponerse infraestructura: %s", rec.Body.String())
	}
}

func TestE003ServicioSinHandlerFallaCerrado(t *testing.T) {
	rec := httptest.NewRecorder()
	handler := internalhttp.NewRefreshHandler(nil, &e003Limiter{allow: true}, silencioso())

	handler.ServeHTTP(rec, peticionE003(`{"refreshToken":"x"}`))

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("sin servicio debe ser 503, obtenido %d", rec.Code)
	}
}
