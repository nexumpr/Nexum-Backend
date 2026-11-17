package transport_test

import (
	"context"
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

// Pruebas HTTP de E009 (`DELETE /api/v1/auth/sessions/{sessionId}`): 204 sin cuerpo, 404
// homogéneo para sesión ajena, 401 sin Bearer, 400 con uuid inválido y 503 sin dependencias.
// El almacén es un doble de prueba; la composición productiva usa DynamoDB real.

type e009HTTPStore struct {
	mu        sync.Mutex
	sesiones  map[string]appIdentity.RevokeSessionRecord
	revocadas []string
	fallo     error
}

func newE009HTTPStore() *e009HTTPStore {
	return &e009HTTPStore{sesiones: map[string]appIdentity.RevokeSessionRecord{}}
}

func (s *e009HTTPStore) sembrar(registro appIdentity.RevokeSessionRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sesiones[registro.UserID+"\x00"+registro.SessionID] = registro
}

func (s *e009HTTPStore) LoadSessionForRevoke(_ context.Context, userID, sessionID string) (*appIdentity.RevokeSessionRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fallo != nil {
		return nil, s.fallo
	}
	registro, ok := s.sesiones[userID+"\x00"+sessionID]
	if !ok {
		return nil, nil
	}
	copia := registro
	return &copia, nil
}

func (s *e009HTTPStore) RevokeOwnedSession(_ context.Context, revocacion appIdentity.SessionRevocation) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fallo != nil {
		return s.fallo
	}
	registro, ok := s.sesiones[revocacion.UserID+"\x00"+revocacion.SessionID]
	if !ok {
		return appIdentity.ErrSessionRevokeNotFound
	}
	registro.Status = appIdentity.StatusRevoked
	s.sesiones[revocacion.UserID+"\x00"+revocacion.SessionID] = registro
	s.revocadas = append(s.revocadas, revocacion.SessionID)
	return nil
}

func e009HTTPHandler(store *e009HTTPStore) *internalhttp.SessionRevokeHandler {
	var servicio *appIdentity.SessionRevokeService
	if store != nil {
		servicio = appIdentity.NewSessionRevokeService(store).
			WithClock(func() time.Time { return e004HTTPAhora })
	}
	return internalhttp.NewSessionRevokeHandler(servicio, e004TokensValidos(), newE004Sesiones(), silencioso())
}

func e009HTTPPeticion(token, sessionID string) *http.Request {
	req := e004Peticion(http.MethodDelete, "/api/v1/auth/sessions/"+sessionID, token, "")
	req.SetPathValue("sessionId", sessionID)
	return req
}

func e009HTTPRegistro(userID, sessionID string) appIdentity.RevokeSessionRecord {
	return appIdentity.RevokeSessionRecord{
		UserID:      userID,
		SessionID:   sessionID,
		RefreshHash: "digest-sintetico",
		Status:      appIdentity.StatusActive,
		Version:     1,
		HasVersion:  true,
		ExpiresAt:   e004HTTPAhora.Add(24 * time.Hour),
		HasExpiry:   true,
	}
}

// TestE009HTTP204SinCuerpo cubre el camino feliz: 204, sin cuerpo, `Cache-Control: no-store` y
// revocación aplicada en el almacén.
func TestE009HTTP204SinCuerpo(t *testing.T) {
	store := newE009HTTPStore()
	store.sembrar(e009HTTPRegistro(e004HTTPUsuario, e004HTTPSesion))

	rec := httptest.NewRecorder()
	e009HTTPHandler(store).ServeHTTP(rec, e009HTTPPeticion("jwt-valido", e004HTTPSesion))

	if rec.Code != http.StatusNoContent {
		t.Fatalf("esperado 204, obtenido %d (%s)", rec.Code, rec.Body.String())
	}
	if rec.Body.Len() != 0 {
		t.Fatalf("204 no debe llevar cuerpo, obtenido %q", rec.Body.String())
	}
	if cache := rec.Header().Get("Cache-Control"); !strings.Contains(cache, "no-store") {
		t.Fatalf("Cache-Control debe incluir no-store, obtenido %q", cache)
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.revocadas) != 1 || store.revocadas[0] != e004HTTPSesion {
		t.Fatalf("la sesion indicada debe quedar revocada, obtenido %v", store.revocadas)
	}
}

// TestE009HTTPSesionAjena404 cubre la no-enumeración: sesión de otro usuario → 404 y sin
// escritura.
func TestE009HTTPSesionAjena404(t *testing.T) {
	store := newE009HTTPStore()
	store.sembrar(e009HTTPRegistro(e004HTTPOtroUser, e004HTTPSesionB))

	rec := httptest.NewRecorder()
	e009HTTPHandler(store).ServeHTTP(rec, e009HTTPPeticion("jwt-valido", e004HTTPSesionB))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("esperado 404, obtenido %d (%s)", rec.Code, rec.Body.String())
	}
	if corpo := rec.Body.String(); strings.Contains(corpo, e004HTTPSesionB) {
		t.Fatalf("la respuesta no debe reflejar el identificador, obtenido %q", corpo)
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.revocadas) != 0 {
		t.Fatalf("no debe revocarse una sesion ajena, obtenido %v", store.revocadas)
	}
}

// TestE009HTTP401SinBearer cubre la autenticación obligatoria.
func TestE009HTTP401SinBearer(t *testing.T) {
	store := newE009HTTPStore()
	store.sembrar(e009HTTPRegistro(e004HTTPUsuario, e004HTTPSesion))

	casos := []struct {
		nombre string
		token  string
	}{
		{"sin token", ""},
		{"token desconocido", "desconocido"},
		{"token expirado", "jwt-expirado"},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			rec := httptest.NewRecorder()
			e009HTTPHandler(store).ServeHTTP(rec, e009HTTPPeticion(caso.token, e004HTTPSesion))
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("esperado 401, obtenido %d (%s)", rec.Code, rec.Body.String())
			}
		})
	}
}

// TestE009HTTP400UUIDInvalido cubre el formato del parámetro de ruta.
func TestE009HTTP400UUIDInvalido(t *testing.T) {
	store := newE009HTTPStore()
	store.sembrar(e009HTTPRegistro(e004HTTPUsuario, e004HTTPSesion))

	rec := httptest.NewRecorder()
	e009HTTPHandler(store).ServeHTTP(rec, e009HTTPPeticion("jwt-valido", "abc"))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("esperado 400, obtenido %d (%s)", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "INVALID_REQUEST") {
		t.Fatalf("debe devolver INVALID_REQUEST, obtenido %q", rec.Body.String())
	}
}

// TestE009HTTP503SinDependencias cubre el fail-closed de la composición.
func TestE009HTTP503SinDependencias(t *testing.T) {
	handler := internalhttp.NewSessionRevokeHandler(nil, nil, nil, silencioso())

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, e009HTTPPeticion("jwt-valido", e004HTTPSesion))

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("esperado 503, obtenido %d (%s)", rec.Code, rec.Body.String())
	}
}

// TestE009HTTP503StoreCaido cubre que un fallo de persistencia nunca responde 204.
func TestE009HTTP503StoreCaido(t *testing.T) {
	store := newE009HTTPStore()
	store.sembrar(e009HTTPRegistro(e004HTTPUsuario, e004HTTPSesion))
	store.fallo = errors.New("dynamodb inaccesible")

	rec := httptest.NewRecorder()
	e009HTTPHandler(store).ServeHTTP(rec, e009HTTPPeticion("jwt-valido", e004HTTPSesion))

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("esperado 503, obtenido %d (%s)", rec.Code, rec.Body.String())
	}
}
