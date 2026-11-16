package application_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	appIdentity "github.com/nexumpr/Nexum-Backend/internal/application/identity"
)

// Doble de persistencia de E004: modela la sesión autoritativa de E002/E003 y permite inyectar
// conflictos de compare-and-swap y fallos de servicio. Nunca se usa en producción.
type e004Store struct {
	mu           sync.Mutex
	sessions     map[string]*appIdentity.LogoutSession
	revocaciones []appIdentity.LogoutRevocation
	loadErr      error
	revokeErrs   []error
	// simularRotacion, si no es nil, se ejecuta antes de resolver cada revocación: permite
	// modelar un refresh que gana la carrera entre la lectura y la escritura.
	simularRotacion func(s *appIdentity.LogoutSession)
}

func newE004Store() *e004Store {
	expira := time.Date(2026, 12, 12, 0, 0, 0, 0, time.UTC)
	return &e004Store{sessions: map[string]*appIdentity.LogoutSession{
		"sesion-a": {
			UserID:      "usuario-1",
			SessionID:   "sesion-a",
			FamilyID:    "familia-a",
			RefreshHash: "digest-a",
			Status:      appIdentity.StatusActive,
			ExpiresAt:   expira,
			HasExpiry:   true,
			Version:     1,
			HasVersion:  true,
		},
		"sesion-b": {
			UserID:      "usuario-1",
			SessionID:   "sesion-b",
			FamilyID:    "familia-b",
			RefreshHash: "digest-b",
			Status:      appIdentity.StatusActive,
			ExpiresAt:   expira,
			HasExpiry:   true,
			Version:     1,
			HasVersion:  true,
		},
	}}
}

func (s *e004Store) LoadLogoutSession(_ context.Context, _, sessionID string) (*appIdentity.LogoutSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.loadErr != nil {
		return nil, s.loadErr
	}
	sesion, ok := s.sessions[sessionID]
	if !ok {
		return nil, nil
	}
	copia := *sesion
	return &copia, nil
}

func (s *e004Store) RevokeLogoutSession(_ context.Context, rev appIdentity.LogoutRevocation) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	sesion, ok := s.sessions[rev.SessionID]
	if ok && s.simularRotacion != nil {
		s.simularRotacion(sesion)
		s.simularRotacion = nil
	}

	if len(s.revokeErrs) > 0 {
		err := s.revokeErrs[0]
		s.revokeErrs = s.revokeErrs[1:]
		return err
	}

	s.revocaciones = append(s.revocaciones, rev)
	if !ok {
		// Sin sesión no se aplica nada: la revocación no crea estado.
		return nil
	}
	// CAS real: sólo se aplica si la versión observada sigue vigente.
	if rev.HasVersion && sesion.Version != rev.ExpectedVersion {
		return appIdentity.ErrLogoutConflict
	}
	sesion.Status = appIdentity.StatusRevoked
	sesion.RefreshHash = ""
	sesion.Version++
	return nil
}

func (s *e004Store) estado(sessionID string) appIdentity.LogoutSession {
	s.mu.Lock()
	defer s.mu.Unlock()
	return *s.sessions[sessionID]
}

// listaRevocaciones evita el bloqueo anidado en las aserciones: devuelve una copia.
func (s *e004Store) listaRevocaciones() []appIdentity.LogoutRevocation {
	s.mu.Lock()
	defer s.mu.Unlock()
	copia := make([]appIdentity.LogoutRevocation, len(s.revocaciones))
	copy(copia, s.revocaciones)
	return copia
}

func e004Servicio(store appIdentity.LogoutStore) *appIdentity.LogoutService {
	return appIdentity.NewLogoutService(store).
		WithClock(func() time.Time { return time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC) })
}

// TestE004LogoutRevocaSoloLaSesionActual cubre T03/T09: el cierre afecta a la sesión del token
// y deja intacta otra sesión del mismo usuario.
func TestE004LogoutRevocaSoloLaSesionActual(t *testing.T) {
	store := newE004Store()
	servicio := e004Servicio(store)

	if err := servicio.Logout(context.Background(), appIdentity.LogoutCommand{UserID: "usuario-1", SessionID: "sesion-a"}); err != nil {
		t.Fatalf("logout valido devolvio error: %v", err)
	}

	if estado := store.estado("sesion-a"); estado.Status != appIdentity.StatusRevoked {
		t.Fatalf("sesion-a debe quedar REVOKED, obtenido %q", estado.Status)
	}
	if estado := store.estado("sesion-b"); estado.Status != appIdentity.StatusActive {
		t.Fatalf("sesion-b debe seguir ACTIVE, obtenido %q", estado.Status)
	}
	if estado := store.estado("sesion-a"); estado.Version != 2 {
		t.Fatalf("la version debe incrementarse a 2, obtenido %d", estado.Version)
	}

	revs := store.listaRevocaciones()
	if len(revs) != 1 {
		t.Fatalf("se esperaba exactamente una revocacion, obtenidas %d", len(revs))
	}
	rev := revs[0]
	if rev.Reason != appIdentity.RevokeReasonLogout {
		t.Fatalf("motivo durable esperado %q, obtenido %q", appIdentity.RevokeReasonLogout, rev.Reason)
	}
	if rev.RefreshHash != "digest-a" {
		t.Fatalf("el digest del refresh vigente debe acompanar la revocacion, obtenido %q", rev.RefreshHash)
	}
	if rev.ExpectedVersion != 1 {
		t.Fatalf("el CAS debe usar la version leida (1), obtenido %d", rev.ExpectedVersion)
	}
}

// TestE004LogoutSesionInexistente cubre T06 parcial: sin sesión persistida no hay 204.
func TestE004LogoutSesionInexistente(t *testing.T) {
	store := newE004Store()
	err := e004Servicio(store).Logout(context.Background(), appIdentity.LogoutCommand{UserID: "usuario-1", SessionID: "no-existe"})
	if !errors.Is(err, appIdentity.ErrLogoutUnauthorized) {
		t.Fatalf("se esperaba ErrLogoutUnauthorized, obtenido %v", err)
	}
	if revs := store.listaRevocaciones(); len(revs) != 0 {
		t.Fatalf("no debe intentarse revocar una sesion inexistente")
	}
}

// TestE004LogoutSesionDeOtroSujeto cubre la propiedad: un `sid` que pertenece a otro `sub` no
// se revoca y no se distingue de una sesión inexistente.
func TestE004LogoutSesionDeOtroSujeto(t *testing.T) {
	store := newE004Store()
	err := e004Servicio(store).Logout(context.Background(), appIdentity.LogoutCommand{UserID: "usuario-2", SessionID: "sesion-a"})
	if !errors.Is(err, appIdentity.ErrLogoutUnauthorized) {
		t.Fatalf("se esperaba ErrLogoutUnauthorized, obtenido %v", err)
	}
	if estado := store.estado("sesion-a"); estado.Status != appIdentity.StatusActive {
		t.Fatalf("una sesion ajena no debe revocarse, estado %q", estado.Status)
	}
}

// TestE004LogoutIdempotenteEnEfectos cubre T05/T11: un segundo cierre no muta nada y no
// devuelve 204, de modo que el middleware pueda rechazar el JWT con 401.
func TestE004LogoutIdempotenteEnEfectos(t *testing.T) {
	store := newE004Store()
	servicio := e004Servicio(store)
	cmd := appIdentity.LogoutCommand{UserID: "usuario-1", SessionID: "sesion-a"}

	if err := servicio.Logout(context.Background(), cmd); err != nil {
		t.Fatalf("primer logout debe confirmar: %v", err)
	}
	err := servicio.Logout(context.Background(), cmd)
	if !errors.Is(err, appIdentity.ErrLogoutUnauthorized) {
		t.Fatalf("segundo logout debe ser 401, obtenido %v", err)
	}

	revs := store.listaRevocaciones()
	if len(revs) != 1 {
		t.Fatalf("el segundo intento no debe emitir otra revocacion, total %d", len(revs))
	}
	if estado := store.estado("sesion-a"); estado.Version != 2 {
		t.Fatalf("la version no debe incrementarse otra vez, obtenido %d", estado.Version)
	}
}

// TestE004LogoutSesionExpirada rechaza una sesión cuyo TTL lógico ya venció.
func TestE004LogoutSesionExpirada(t *testing.T) {
	store := newE004Store()
	store.mu.Lock()
	store.sessions["sesion-a"].ExpiresAt = time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	store.mu.Unlock()

	err := e004Servicio(store).Logout(context.Background(), appIdentity.LogoutCommand{UserID: "usuario-1", SessionID: "sesion-a"})
	if !errors.Is(err, appIdentity.ErrLogoutUnauthorized) {
		t.Fatalf("sesion expirada debe ser 401, obtenido %v", err)
	}
}

// TestE004LogoutConflictoReleeYRevoca cubre T12: si un refresh gana la carrera, el cierre relee
// el estado vigente y revoca con la versión y el digest nuevos. Nunca revive la sesión.
func TestE004LogoutConflictoReleeYRevoca(t *testing.T) {
	store := newE004Store()
	// Rotación concurrente: la sesión pasa a versión 2 con un digest nuevo antes de escribir.
	store.simularRotacion = func(s *appIdentity.LogoutSession) {
		s.Version = 2
		s.RefreshHash = "digest-a-rotado"
	}

	if err := e004Servicio(store).Logout(context.Background(), appIdentity.LogoutCommand{UserID: "usuario-1", SessionID: "sesion-a"}); err != nil {
		t.Fatalf("el cierre debe reconciliarse tras el conflicto: %v", err)
	}

	estado := store.estado("sesion-a")
	if estado.Status != appIdentity.StatusRevoked {
		t.Fatalf("la sesion debe quedar REVOKED, obtenido %q", estado.Status)
	}
	if estado.RefreshHash != "" {
		t.Fatalf("el digest debe limpiarse al revocar, obtenido %q", estado.RefreshHash)
	}

	revs := store.listaRevocaciones()
	if len(revs) != 2 {
		t.Fatalf("se esperaban 2 intentos (conflicto + reconciliacion), obtenidos %d", len(revs))
	}
	if revs[1].ExpectedVersion != 2 || revs[1].RefreshHash != "digest-a-rotado" {
		t.Fatalf("la reconciliacion debe usar version 2 y el digest rotado, obtenido v%d/%q",
			revs[1].ExpectedVersion, revs[1].RefreshHash)
	}
}

// TestE004LogoutConflictoPersistente nunca reporta éxito.
func TestE004LogoutConflictoPersistente(t *testing.T) {
	store := newE004Store()
	store.revokeErrs = []error{
		appIdentity.ErrLogoutConflict,
		appIdentity.ErrLogoutConflict,
		appIdentity.ErrLogoutConflict,
	}
	err := e004Servicio(store).Logout(context.Background(), appIdentity.LogoutCommand{UserID: "usuario-1", SessionID: "sesion-a"})
	if !errors.Is(err, appIdentity.ErrLogoutConflict) {
		t.Fatalf("se esperaba ErrLogoutConflict tras %d intentos, obtenido %v", appIdentity.LogoutMaxAttempts, err)
	}
}

// TestE004LogoutFalloDeLectura cubre T13 parcial: sin lectura autoritativa no hay 204.
func TestE004LogoutFalloDeLectura(t *testing.T) {
	store := newE004Store()
	store.loadErr = errors.New("dynamodb no disponible")
	err := e004Servicio(store).Logout(context.Background(), appIdentity.LogoutCommand{UserID: "usuario-1", SessionID: "sesion-a"})
	if !errors.Is(err, appIdentity.ErrLogoutUnavailable) {
		t.Fatalf("se esperaba ErrLogoutUnavailable, obtenido %v", err)
	}
}

// TestE004LogoutFalloDeEscritura cubre T13: un fallo de persistencia jamás produce éxito.
func TestE004LogoutFalloDeEscritura(t *testing.T) {
	store := newE004Store()
	store.revokeErrs = []error{errors.New("write timeout")}
	err := e004Servicio(store).Logout(context.Background(), appIdentity.LogoutCommand{UserID: "usuario-1", SessionID: "sesion-a"})
	if !errors.Is(err, appIdentity.ErrLogoutUnavailable) {
		t.Fatalf("se esperaba ErrLogoutUnavailable, obtenido %v", err)
	}
	if estado := store.estado("sesion-a"); estado.Status != appIdentity.StatusActive {
		t.Fatalf("una revocacion fallida no debe cambiar el estado, obtenido %q", estado.Status)
	}
}

// TestE004LogoutSinIdentificadores rechaza tokens sin `sub` o `sid` sin tocar la persistencia.
func TestE004LogoutSinIdentificadores(t *testing.T) {
	store := newE004Store()
	servicio := e004Servicio(store)
	for _, cmd := range []appIdentity.LogoutCommand{
		{SessionID: "sesion-a"},
		{UserID: "usuario-1"},
		{UserID: "   ", SessionID: "  "},
	} {
		if err := servicio.Logout(context.Background(), cmd); !errors.Is(err, appIdentity.ErrLogoutUnauthorized) {
			t.Fatalf("cmd %+v debe ser 401, obtenido %v", cmd, err)
		}
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.revocaciones) != 0 {
		t.Fatalf("no debe consultarse la persistencia con identificadores vacios")
	}
}

// TestE004LogoutSinPersistencia falla cerrado.
func TestE004LogoutSinPersistencia(t *testing.T) {
	if err := appIdentity.NewLogoutService(nil).Logout(context.Background(), appIdentity.LogoutCommand{UserID: "u", SessionID: "s"}); !errors.Is(err, appIdentity.ErrLogoutUnavailable) {
		t.Fatalf("sin store debe devolver ErrLogoutUnavailable, obtenido %v", err)
	}
}
