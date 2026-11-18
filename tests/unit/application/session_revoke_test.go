package application_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	appIdentity "github.com/nexumpr/Nexum-Backend/internal/application/identity"
)

// Pruebas de aplicación de E009 (`DELETE /api/v1/auth/sessions/{sessionId}`) sobre un almacén
// en memoria. No sustituyen al adaptador real: cubren la decisión de negocio (propiedad,
// idempotencia, expiración, conflicto y fail-closed).

const (
	e009Usuario = "11111111-2222-4333-8444-555555555555"
	e009Otro    = "99999999-8888-4777-8666-555555555555"

	e009Sesion      = "018f90ef-08e5-7a68-b09f-0000000000c1"
	e009SesionB     = "018f90ef-08e5-7a68-b09f-0000000000c2"
	e009Inexistente = "018f90ef-08e5-7a68-b09f-0000000000ff"
)

var e009Ahora = time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)

type e009Registro struct {
	userID      string
	sessionID   string
	refreshHash string
	status      string
	version     int64
	expiresAt   time.Time
	hasExpiry   bool
}

type e009Store struct {
	mu         sync.Mutex
	sesiones   map[string]*e009Registro
	fallo      error
	conflicto  bool
	escrituras []appIdentity.SessionRevocation
	lecturas   int
}

func newE009Store() *e009Store {
	return &e009Store{sesiones: map[string]*e009Registro{}}
}

func e009Clave(userID, sessionID string) string { return userID + "\x00" + sessionID }

func (s *e009Store) sembrar(registro e009Registro) {
	s.mu.Lock()
	defer s.mu.Unlock()
	copia := registro
	s.sesiones[e009Clave(registro.userID, registro.sessionID)] = &copia
}

func (s *e009Store) LoadSessionForRevoke(_ context.Context, userID, sessionID string) (*appIdentity.RevokeSessionRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lecturas++
	if s.fallo != nil {
		return nil, s.fallo
	}
	registro, ok := s.sesiones[e009Clave(userID, sessionID)]
	if !ok {
		return nil, nil
	}
	return &appIdentity.RevokeSessionRecord{
		UserID:      registro.userID,
		SessionID:   registro.sessionID,
		RefreshHash: registro.refreshHash,
		Status:      registro.status,
		Version:     registro.version,
		HasVersion:  true,
		ExpiresAt:   registro.expiresAt,
		HasExpiry:   registro.hasExpiry,
	}, nil
}

func (s *e009Store) RevokeOwnedSession(_ context.Context, revocacion appIdentity.SessionRevocation) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fallo != nil {
		return s.fallo
	}
	if s.conflicto {
		s.conflicto = false
		return appIdentity.ErrSessionRevokeConflict
	}
	registro, ok := s.sesiones[e009Clave(revocacion.UserID, revocacion.SessionID)]
	if !ok {
		return appIdentity.ErrSessionRevokeNotFound
	}
	if revocacion.HasVersion && registro.version != revocacion.ExpectedVersion {
		return appIdentity.ErrSessionRevokeConflict
	}
	registro.status = appIdentity.StatusRevoked
	registro.version++
	s.escrituras = append(s.escrituras, revocacion)
	return nil
}

func e009Servicio(store *e009Store) *appIdentity.SessionRevokeService {
	return appIdentity.NewSessionRevokeService(store).WithClock(func() time.Time { return e009Ahora })
}

// TestE009RevocaSesionPropia cubre el camino feliz: la sesión del titular queda REVOKED con el
// motivo durable y la versión incrementada.
func TestE009RevocaSesionPropia(t *testing.T) {
	store := newE009Store()
	store.sembrar(e009Registro{
		userID: e009Usuario, sessionID: e009Sesion, refreshHash: "digest-a",
		status: appIdentity.StatusActive, version: 1,
	})

	if err := e009Servicio(store).RevokeOwnedSession(context.Background(), e009Usuario, e009Sesion, e009Ahora); err != nil {
		t.Fatalf("revocacion propia debe completarse, obtenido %v", err)
	}
	registro := store.sesiones[e009Clave(e009Usuario, e009Sesion)]
	if registro.status != appIdentity.StatusRevoked {
		t.Fatalf("status esperado REVOKED, obtenido %q", registro.status)
	}
	if len(store.escrituras) != 1 {
		t.Fatalf("debe escribirse exactamente una vez, obtenido %d", len(store.escrituras))
	}
	if motivo := store.escrituras[0].Reason; motivo != appIdentity.RevokeReasonUserRevokedSession {
		t.Fatalf("motivo esperado %q, obtenido %q", appIdentity.RevokeReasonUserRevokedSession, motivo)
	}
	if digest := store.escrituras[0].RefreshHash; digest != "digest-a" {
		t.Fatalf("debe propagarse el digest del refresh vigente, obtenido %q", digest)
	}
}

// TestE009SesionAjena404 cubre la no-enumeración: una sesión de otro usuario responde 404
// igual que una inexistente y no se escribe nada.
func TestE009SesionAjena404(t *testing.T) {
	store := newE009Store()
	store.sembrar(e009Registro{
		userID: e009Otro, sessionID: e009Sesion,
		status: appIdentity.StatusActive, version: 1,
	})

	err := e009Servicio(store).RevokeOwnedSession(context.Background(), e009Usuario, e009Sesion, e009Ahora)
	if !errors.Is(err, appIdentity.ErrSessionRevokeNotFound) {
		t.Fatalf("sesion ajena debe ser 404, obtenido %v", err)
	}
	if len(store.escrituras) != 0 {
		t.Fatalf("no debe escribir sobre sesiones ajenas, obtenido %d escrituras", len(store.escrituras))
	}
	if registro := store.sesiones[e009Clave(e009Otro, e009Sesion)]; registro.status != appIdentity.StatusActive {
		t.Fatalf("la sesion ajena debe seguir ACTIVE, obtenido %q", registro.status)
	}
}

// TestE009Inexistente404 cubre sesión inexistente y sesión expirada.
func TestE009Inexistente404(t *testing.T) {
	store := newE009Store()
	store.sembrar(e009Registro{
		userID: e009Usuario, sessionID: e009SesionB,
		status: appIdentity.StatusActive, version: 1,
		expiresAt: e009Ahora.Add(-time.Minute), hasExpiry: true,
	})

	casos := []struct {
		nombre    string
		sessionID string
	}{
		{"inexistente", e009Inexistente},
		{"expirada", e009SesionB},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			err := e009Servicio(store).RevokeOwnedSession(context.Background(), e009Usuario, caso.sessionID, e009Ahora)
			if !errors.Is(err, appIdentity.ErrSessionRevokeNotFound) {
				t.Fatalf("esperado 404, obtenido %v", err)
			}
		})
	}
	if len(store.escrituras) != 0 {
		t.Fatalf("no debe escribir sobre sesiones inexistentes o expiradas, obtenido %d", len(store.escrituras))
	}
}

// TestE009YaRevocada204 cubre la idempotencia: revocar dos veces devuelve nil sin reescribir.
func TestE009YaRevocada204(t *testing.T) {
	store := newE009Store()
	store.sembrar(e009Registro{
		userID: e009Usuario, sessionID: e009Sesion,
		status: appIdentity.StatusRevoked, version: 2,
	})

	if err := e009Servicio(store).RevokeOwnedSession(context.Background(), e009Usuario, e009Sesion, e009Ahora); err != nil {
		t.Fatalf("revocar una sesion ya revocada debe ser idempotente, obtenido %v", err)
	}
	if len(store.escrituras) != 0 {
		t.Fatalf("no debe reescribir una sesion ya revocada, obtenido %d", len(store.escrituras))
	}
}

// TestE009UUIDInvalido400 cubre el formato del parámetro de ruta.
func TestE009UUIDInvalido400(t *testing.T) {
	store := newE009Store()
	store.sembrar(e009Registro{userID: e009Usuario, sessionID: e009Sesion, status: appIdentity.StatusActive, version: 1})

	invalido := []string{
		"",
		"abc",
		"018f90ef08e57a68b09f0000000000c1",
		"018f90ef-08e5-7a68-b09f-0000000000cz",
		"018f90ef-08e5-7a68-b09f-0000000000c1x",
		"{018f90ef-08e5-7a68-b09f-0000000000c1}",
	}
	for _, valor := range invalido {
		if err := e009Servicio(store).RevokeOwnedSession(context.Background(), e009Usuario, valor, e009Ahora); !errors.Is(err, appIdentity.ErrSessionsInvalidRequest) {
			t.Fatalf("uuid %q debe ser 400, obtenido %v", valor, err)
		}
	}
	if len(store.escrituras) != 0 {
		t.Fatalf("no debe escribir con identificadores invalidos, obtenido %d", len(store.escrituras))
	}
}

// TestE009CASPerdido409 cubre la concurrencia: el CAS falla y, tras releer, la sesión sigue
// activa con otra versión, por lo que la operación debe reintentarse (409).
func TestE009CASPerdido409(t *testing.T) {
	store := newE009Store()
	store.sembrar(e009Registro{
		userID: e009Usuario, sessionID: e009Sesion,
		status: appIdentity.StatusActive, version: 1,
	})
	store.conflicto = true

	err := e009Servicio(store).RevokeOwnedSession(context.Background(), e009Usuario, e009Sesion, e009Ahora)
	if !errors.Is(err, appIdentity.ErrSessionRevokeConflict) {
		t.Fatalf("CAS perdido debe ser 409, obtenido %v", err)
	}
	if store.lecturas != 2 {
		t.Fatalf("debe releer exactamente una vez tras el conflicto, obtenido %d lecturas", store.lecturas)
	}
}

// TestE009StoreCaido503 cubre el fail-closed: sin persistencia no se responde 204.
func TestE009StoreCaido503(t *testing.T) {
	store := newE009Store()
	store.fallo = errors.New("dynamodb inaccesible")

	err := e009Servicio(store).RevokeOwnedSession(context.Background(), e009Usuario, e009Sesion, e009Ahora)
	if !errors.Is(err, appIdentity.ErrSessionRevokeUnavailable) {
		t.Fatalf("store caido debe ser 503, obtenido %v", err)
	}

	if err := appIdentity.NewSessionRevokeService(nil).RevokeOwnedSession(context.Background(), e009Usuario, e009Sesion, e009Ahora); !errors.Is(err, appIdentity.ErrSessionRevokeUnavailable) {
		t.Fatalf("servicio sin almacen debe ser 503, obtenido %v", err)
	}
}

// TestE009SinUsuario401 cubre la defensa en profundidad: sin sujeto no hay revocación.
func TestE009SinUsuario401(t *testing.T) {
	store := newE009Store()
	if err := e009Servicio(store).RevokeOwnedSession(context.Background(), "  ", e009Sesion, e009Ahora); !errors.Is(err, appIdentity.ErrSessionRevokeUnauthorized) {
		t.Fatalf("sin usuario debe ser 401, obtenido %v", err)
	}
}

// TestE009IsUUID comprueba el validador canónico de uuid.
func TestE009IsUUID(t *testing.T) {
	validos := []string{
		"018f90ef-08e5-7a68-b09f-0000000000c1",
		"ABCDEF01-2345-6789-ABCD-EF0123456789",
	}
	for _, valor := range validos {
		if !appIdentity.IsUUID(valor) {
			t.Fatalf("%q debe aceptarse como uuid", valor)
		}
	}
	inValidos := []string{"", "abc", "018f90ef-08e5-7a68-b09f-0000000000c", "018f90ef08e57a68b09f0000000000c1", "018f90ef-08e5-7a68-b09f-0000000000cg"}
	for _, valor := range inValidos {
		if appIdentity.IsUUID(valor) {
			t.Fatalf("%q no debe aceptarse como uuid", valor)
		}
	}
}
