package application_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	appIdentity "github.com/nexumpr/Nexum-Backend/internal/application/identity"
)

// fakeRefreshStore modela el contrato durable de E003 con la misma semántica que el
// adaptador DynamoDB: la rotación es un compare-and-swap y el consumo del token anterior
// es parte de la misma operación. Nunca se usa en la composición productiva.
type fakeRefreshStore struct {
	mu       sync.Mutex
	locators map[string]*appIdentity.RefreshLocator
	sessions map[string]*appIdentity.RefreshSession
	users    map[string]*appIdentity.RefreshUser

	locateErr error
	loadErr   error
	userErr   error
	rotateErr error
	revokeErr error

	rotateCalls []appIdentity.RefreshRotation
	revokeCalls []appIdentity.RefreshRevocation
	locateCalls int
}

func newFakeRefreshStore() *fakeRefreshStore {
	return &fakeRefreshStore{
		locators: make(map[string]*appIdentity.RefreshLocator),
		sessions: make(map[string]*appIdentity.RefreshSession),
		users:    make(map[string]*appIdentity.RefreshUser),
	}
}

func sessionKey(userID, sessionID string) string { return userID + "|" + sessionID }

func (f *fakeRefreshStore) LocateRefresh(_ context.Context, digest string) (*appIdentity.RefreshLocator, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.locateCalls++
	if f.locateErr != nil {
		return nil, f.locateErr
	}
	locator, ok := f.locators[digest]
	if !ok {
		return nil, nil
	}
	copia := *locator
	return &copia, nil
}

func (f *fakeRefreshStore) LoadSession(_ context.Context, userID, sessionID string) (*appIdentity.RefreshSession, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.loadErr != nil {
		return nil, f.loadErr
	}
	session, ok := f.sessions[sessionKey(userID, sessionID)]
	if !ok {
		return nil, nil
	}
	copia := *session
	return &copia, nil
}

func (f *fakeRefreshStore) LoadUserProfile(_ context.Context, userID string) (*appIdentity.RefreshUser, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.userErr != nil {
		return nil, f.userErr
	}
	user, ok := f.users[userID]
	if !ok {
		return nil, nil
	}
	copia := *user
	return &copia, nil
}

func (f *fakeRefreshStore) RotateRefresh(_ context.Context, rotation appIdentity.RefreshRotation) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.rotateErr != nil {
		return f.rotateErr
	}

	locator, ok := f.locators[rotation.OldDigest]
	if !ok || locator.Used || !statusEquals(locator.Status, "ACTIVE") {
		return appIdentity.ErrRefreshConflict
	}
	session, ok := f.sessions[sessionKey(rotation.UserID, rotation.SessionID)]
	if !ok || session.Version != rotation.ExpectedVersion ||
		session.RefreshHash != rotation.OldDigest || !statusEquals(session.Status, "ACTIVE") {
		return appIdentity.ErrRefreshConflict
	}

	f.rotateCalls = append(f.rotateCalls, rotation)
	locator.Used = true
	locator.Status = "CONSUMED"
	session.RefreshHash = rotation.NewDigest
	session.AccessJTI = rotation.NewAccessJTI
	session.Version = rotation.NewVersion
	session.LastUsedAt = rotation.Now
	f.locators[rotation.NewDigest] = &appIdentity.RefreshLocator{
		UserID:    rotation.UserID,
		SessionID: rotation.SessionID,
		FamilyID:  rotation.FamilyID,
		Status:    "ACTIVE",
		ExpiresAt: rotation.ExpiresAt,
		HasExpiry: !rotation.ExpiresAt.IsZero(),
	}
	return nil
}

func (f *fakeRefreshStore) RevokeFamily(_ context.Context, revocation appIdentity.RefreshRevocation) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.revokeErr != nil {
		return f.revokeErr
	}
	f.revokeCalls = append(f.revokeCalls, revocation)
	if session, ok := f.sessions[sessionKey(revocation.UserID, revocation.SessionID)]; ok {
		session.Status = "REVOKED"
	}
	return nil
}

func statusEquals(value, expected string) bool {
	return value == expected
}

const (
	refreshUserID    = "47622cbc-36ec-4b30-9b8a-d9c26833a395"
	refreshSessionID = "018f90ef-08e5-7a68-b09f-0000000000a1"
	refreshFamilyID  = "018f90ef-08e5-7a68-b09f-0000000000b2"
	refreshPlainOld  = "refresh-opaco-anterior"
	refreshPlainNew  = "refresh-opaco-nuevo"
)

var fixedRefreshNow = time.Date(2026, 11, 12, 20, 0, 0, 0, time.UTC)

// seedRefreshStore deja una sesión ACTIVE creada por E002 con un único refresh válido.
func seedRefreshStore() *fakeRefreshStore {
	store := newFakeRefreshStore()
	digest := appIdentity.HashToken(refreshPlainOld)
	expira := fixedRefreshNow.Add(30 * 24 * time.Hour)
	store.users[refreshUserID] = &appIdentity.RefreshUser{UserID: refreshUserID, Status: "ACTIVE"}
	store.locators[digest] = &appIdentity.RefreshLocator{
		UserID:    refreshUserID,
		SessionID: refreshSessionID,
		FamilyID:  refreshFamilyID,
		Status:    "ACTIVE",
		ExpiresAt: expira,
		HasExpiry: true,
	}
	store.sessions[sessionKey(refreshUserID, refreshSessionID)] = &appIdentity.RefreshSession{
		UserID:      refreshUserID,
		SessionID:   refreshSessionID,
		FamilyID:    refreshFamilyID,
		RefreshHash: digest,
		AccessJTI:   "jti-anterior",
		Status:      "ACTIVE",
		DeviceID:    "installation-6a4f1d8c",
		Platform:    "android",
		CreatedAt:   fixedRefreshNow.Add(-time.Hour),
		ExpiresAt:   expira,
		HasExpiry:   true,
		Version:     1,
	}
	return store
}

func newRefreshService(store appIdentity.RefreshStore, issuer appIdentity.AccessTokenIssuer) *appIdentity.RefreshService {
	return appIdentity.NewRefreshService(store, issuer).
		WithClock(func() time.Time { return fixedRefreshNow }).
		WithRefreshGenerator(func() (string, error) { return refreshPlainNew, nil })
}

func TestRefreshExitosoDevuelveContratoAuthTokens(t *testing.T) {
	store := seedRefreshStore()
	svc := newRefreshService(store, &fakeIssuer{token: "jwt-nuevo", jti: "jti-nuevo"})

	tokens, err := svc.Refresh(context.Background(), appIdentity.RefreshCommand{RefreshToken: refreshPlainOld})
	if err != nil {
		t.Fatalf("refresh fallo: %v", err)
	}

	if tokens.TokenType != "Bearer" || tokens.ExpiresIn != 900 {
		t.Fatalf("tokens inesperados: %+v", tokens)
	}
	if tokens.SessionID != refreshSessionID || tokens.Session.ID != refreshSessionID {
		t.Fatalf("sessionId debe conservarse y session.id debe igualarlo: %+v", tokens)
	}
	if tokens.AccessToken != "jwt-nuevo" {
		t.Fatalf("access token inesperado: %q", tokens.AccessToken)
	}
	if tokens.RefreshToken != refreshPlainNew {
		t.Fatalf("refresh token inesperado: %q", tokens.RefreshToken)
	}
	if len(store.rotateCalls) != 1 {
		t.Fatalf("se esperaba una rotacion, hay %d", len(store.rotateCalls))
	}
	rotation := store.rotateCalls[0]
	if rotation.ExpectedVersion != 1 || rotation.NewVersion != 2 {
		t.Fatalf("versionado CAS inesperado: %+v", rotation)
	}
	if rotation.OldDigest != appIdentity.HashToken(refreshPlainOld) {
		t.Fatal("el digest consumido debe ser el del token presentado")
	}
	if rotation.NewDigest != appIdentity.HashToken(refreshPlainNew) {
		t.Fatal("el digest nuevo debe ser el del token emitido")
	}
	if !rotation.ExpiresAt.Equal(fixedRefreshNow.Add(30 * 24 * time.Hour)) {
		t.Fatalf("rotar no debe extender la expiracion absoluta: %v", rotation.ExpiresAt)
	}
	if _, claro := store.locators[refreshPlainNew]; claro {
		t.Fatal("jamas debe indexarse el refresh en claro")
	}
	if len(store.revokeCalls) != 0 {
		t.Fatal("una rotacion correcta no revoca la familia")
	}
}

func TestRefreshRechazaCuerpoInvalido(t *testing.T) {
	casos := []struct {
		nombre  string
		command appIdentity.RefreshCommand
		campo   string
		motivo  string
	}{
		{"vacio", appIdentity.RefreshCommand{}, "refreshToken", "required"},
		{"espacios", appIdentity.RefreshCommand{RefreshToken: "   "}, "refreshToken", "invalid_value"},
		{"control", appIdentity.RefreshCommand{RefreshToken: "abc\x00def"}, "refreshToken", "invalid_value"},
		{"demasiado largo", appIdentity.RefreshCommand{RefreshToken: repeat('a', appIdentity.RefreshMaxTokenRunes+1)}, "refreshToken", "too_long"},
		{"device largo", appIdentity.RefreshCommand{RefreshToken: "token-valido", DeviceID: repeat('d', 129)}, "deviceId", "invalid_value"},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			store := seedRefreshStore()
			svc := newRefreshService(store, &fakeIssuer{token: "jwt", jti: "jti"})

			_, err := svc.Refresh(context.Background(), caso.command)
			var violation *appIdentity.FieldViolation
			if !errors.As(err, &violation) {
				t.Fatalf("se esperaba FieldViolation, hay %v", err)
			}
			if violation.Field != caso.campo || violation.Reason != caso.motivo {
				t.Fatalf("violacion inesperada: %+v", violation)
			}
			if len(store.rotateCalls) != 0 {
				t.Fatal("un cuerpo invalido no debe tocar la persistencia")
			}
		})
	}
}

func TestRefreshTokenDesconocidoEsGenerico(t *testing.T) {
	store := seedRefreshStore()
	svc := newRefreshService(store, &fakeIssuer{token: "jwt", jti: "jti"})

	_, err := svc.Refresh(context.Background(), appIdentity.RefreshCommand{RefreshToken: "token-que-no-existe"})
	if !errors.Is(err, appIdentity.ErrInvalidRefreshToken) {
		t.Fatalf("se esperaba ErrInvalidRefreshToken, hay %v", err)
	}
	if len(store.rotateCalls) != 0 || len(store.revokeCalls) != 0 {
		t.Fatal("un token desconocido no escribe ni revoca")
	}
}

func TestRefreshRechazaSesionRevocada(t *testing.T) {
	store := seedRefreshStore()
	store.sessions[sessionKey(refreshUserID, refreshSessionID)].Status = "REVOKED"
	svc := newRefreshService(store, &fakeIssuer{token: "jwt", jti: "jti"})

	_, err := svc.Refresh(context.Background(), appIdentity.RefreshCommand{RefreshToken: refreshPlainOld})
	if !errors.Is(err, appIdentity.ErrInvalidRefreshToken) {
		t.Fatalf("se esperaba ErrInvalidRefreshToken, hay %v", err)
	}
	if len(store.rotateCalls) != 0 {
		t.Fatal("una sesion revocada no rota")
	}
}

func TestRefreshRechazaSesionExpirada(t *testing.T) {
	store := seedRefreshStore()
	expirada := fixedRefreshNow.Add(-time.Minute)
	session := store.sessions[sessionKey(refreshUserID, refreshSessionID)]
	session.ExpiresAt = expirada
	svc := newRefreshService(store, &fakeIssuer{token: "jwt", jti: "jti"})

	_, err := svc.Refresh(context.Background(), appIdentity.RefreshCommand{RefreshToken: refreshPlainOld})
	if !errors.Is(err, appIdentity.ErrInvalidRefreshToken) {
		t.Fatalf("se esperaba ErrInvalidRefreshToken, hay %v", err)
	}
	if len(store.rotateCalls) != 0 {
		t.Fatal("una sesion expirada no rota")
	}
}

func TestRefreshRechazaLocalizadorExpirado(t *testing.T) {
	store := seedRefreshStore()
	store.locators[appIdentity.HashToken(refreshPlainOld)].ExpiresAt = fixedRefreshNow.Add(-time.Second)
	svc := newRefreshService(store, &fakeIssuer{token: "jwt", jti: "jti"})

	_, err := svc.Refresh(context.Background(), appIdentity.RefreshCommand{RefreshToken: refreshPlainOld})
	if !errors.Is(err, appIdentity.ErrInvalidRefreshToken) {
		t.Fatalf("se esperaba ErrInvalidRefreshToken, hay %v", err)
	}
}

func TestRefreshReutilizacionRevocaFamilia(t *testing.T) {
	store := seedRefreshStore()
	svc := newRefreshService(store, &fakeIssuer{token: "jwt", jti: "jti"})

	if _, err := svc.Refresh(context.Background(), appIdentity.RefreshCommand{RefreshToken: refreshPlainOld}); err != nil {
		t.Fatalf("primera rotacion fallo: %v", err)
	}
	// El token nuevo funciona; el anterior ya no.
	svcConSegundo := newRefreshService(store, &fakeIssuer{token: "jwt-2", jti: "jti-2"}).
		WithRefreshGenerator(func() (string, error) { return "refresh-opaco-tercero", nil })
	if _, err := svcConSegundo.Refresh(context.Background(), appIdentity.RefreshCommand{RefreshToken: refreshPlainNew}); err != nil {
		t.Fatalf("la rotacion normal del token nuevo fallo: %v", err)
	}

	_, err := svc.Refresh(context.Background(), appIdentity.RefreshCommand{RefreshToken: refreshPlainOld})
	if !errors.Is(err, appIdentity.ErrRefreshReplay) {
		t.Fatalf("se esperaba ErrRefreshReplay, hay %v", err)
	}
	if len(store.revokeCalls) != 1 {
		t.Fatalf("se esperaba una revocacion de familia, hay %d", len(store.revokeCalls))
	}
	revocacion := store.revokeCalls[0]
	if revocacion.SessionID != refreshSessionID || revocacion.FamilyID != refreshFamilyID {
		t.Fatalf("revocacion sobre otra sesion: %+v", revocacion)
	}
	if revocacion.PresentedDigest != appIdentity.HashToken(refreshPlainOld) {
		t.Fatal("la revocacion debe registrar el digest reutilizado")
	}
	if store.sessions[sessionKey(refreshUserID, refreshSessionID)].Status != "REVOKED" {
		t.Fatal("la sesion debe quedar revocada de forma durable")
	}
}

func TestRefreshReutilizacionConEstadoConsumido(t *testing.T) {
	store := seedRefreshStore()
	locator := store.locators[appIdentity.HashToken(refreshPlainOld)]
	locator.Used = true
	locator.Status = "CONSUMED"
	svc := newRefreshService(store, &fakeIssuer{token: "jwt", jti: "jti"})

	_, err := svc.Refresh(context.Background(), appIdentity.RefreshCommand{RefreshToken: refreshPlainOld})
	if !errors.Is(err, appIdentity.ErrRefreshReplay) {
		t.Fatalf("se esperaba ErrRefreshReplay, hay %v", err)
	}
	if len(store.revokeCalls) != 1 {
		t.Fatal("la reutilizacion debe revocar la familia")
	}
}

func TestRefreshRevocacionNoConfirmadaFallaCerrado(t *testing.T) {
	store := seedRefreshStore()
	locator := store.locators[appIdentity.HashToken(refreshPlainOld)]
	locator.Used = true
	store.revokeErr = errors.New("dynamodb no disponible")
	svc := newRefreshService(store, &fakeIssuer{token: "jwt", jti: "jti"})

	_, err := svc.Refresh(context.Background(), appIdentity.RefreshCommand{RefreshToken: refreshPlainOld})
	if !errors.Is(err, appIdentity.ErrRefreshUnavailable) {
		t.Fatalf("una revocacion no confirmada debe fallar cerrada con 503, hay %v", err)
	}
}

func TestRefreshDeviceIDIncoherenteNoAutoriza(t *testing.T) {
	store := seedRefreshStore()
	svc := newRefreshService(store, &fakeIssuer{token: "jwt", jti: "jti"})

	_, err := svc.Refresh(context.Background(), appIdentity.RefreshCommand{
		RefreshToken: refreshPlainOld,
		DeviceID:     "installation-otra",
	})
	if !errors.Is(err, appIdentity.ErrInvalidRefreshToken) {
		t.Fatalf("se esperaba ErrInvalidRefreshToken, hay %v", err)
	}
	if len(store.revokeCalls) != 0 {
		t.Fatal("un deviceId incoherente no prueba robo: no revoca")
	}

	// El mismo dispositivo sí rota.
	if _, err := svc.Refresh(context.Background(), appIdentity.RefreshCommand{
		RefreshToken: refreshPlainOld,
		DeviceID:     "installation-6a4f1d8c",
	}); err != nil {
		t.Fatalf("el dispositivo legitimo debe rotar: %v", err)
	}
}

func TestRefreshIncoherenciaSesionLocalizador(t *testing.T) {
	store := seedRefreshStore()
	store.sessions[sessionKey(refreshUserID, refreshSessionID)].UserID = "otro-usuario"
	svc := newRefreshService(store, &fakeIssuer{token: "jwt", jti: "jti"})

	_, err := svc.Refresh(context.Background(), appIdentity.RefreshCommand{RefreshToken: refreshPlainOld})
	if !errors.Is(err, appIdentity.ErrInvalidRefreshToken) {
		t.Fatalf("se esperaba ErrInvalidRefreshToken, hay %v", err)
	}
	if len(store.rotateCalls) != 0 {
		t.Fatal("una sesion incoherente no rota")
	}
}

func TestRefreshDigestDeSesionDistintoEsReutilizacion(t *testing.T) {
	store := seedRefreshStore()
	store.sessions[sessionKey(refreshUserID, refreshSessionID)].RefreshHash = appIdentity.HashToken("otro-refresh")
	svc := newRefreshService(store, &fakeIssuer{token: "jwt", jti: "jti"})

	_, err := svc.Refresh(context.Background(), appIdentity.RefreshCommand{RefreshToken: refreshPlainOld})
	if !errors.Is(err, appIdentity.ErrRefreshReplay) {
		t.Fatalf("se esperaba ErrRefreshReplay, hay %v", err)
	}
	if len(store.revokeCalls) != 1 {
		t.Fatal("la incoherencia se trata como reutilizacion y revoca")
	}
}

func TestRefreshConflictoSinEvidenciaNoRevoca(t *testing.T) {
	store := seedRefreshStore()
	store.rotateErr = appIdentity.ErrRefreshConflict
	svc := newRefreshService(store, &fakeIssuer{token: "jwt", jti: "jti"})

	_, err := svc.Refresh(context.Background(), appIdentity.RefreshCommand{RefreshToken: refreshPlainOld})
	if !errors.Is(err, appIdentity.ErrRefreshConflict) {
		t.Fatalf("se esperaba ErrRefreshConflict, hay %v", err)
	}
	if len(store.revokeCalls) != 0 {
		t.Fatal("un conflicto de concurrencia sin evidencia no revoca")
	}
}

func TestRefreshConflictoConConsumoEsReutilizacion(t *testing.T) {
	store := seedRefreshStore()
	store.rotateErr = appIdentity.ErrRefreshConflict
	locator := store.locators[appIdentity.HashToken(refreshPlainOld)]
	locator.Used = true
	locator.Status = "CONSUMED"
	svc := newRefreshService(store, &fakeIssuer{token: "jwt", jti: "jti"})

	_, err := svc.Refresh(context.Background(), appIdentity.RefreshCommand{RefreshToken: refreshPlainOld})
	if !errors.Is(err, appIdentity.ErrRefreshReplay) {
		t.Fatalf("se esperaba ErrRefreshReplay, hay %v", err)
	}
	if len(store.revokeCalls) != 1 {
		t.Fatal("la evidencia durable de consumo obliga a revocar")
	}
}

func TestRefreshFallaCerradoSinDependencias(t *testing.T) {
	store := seedRefreshStore()

	if _, err := appIdentity.NewRefreshService(nil, &fakeIssuer{}).Refresh(context.Background(), appIdentity.RefreshCommand{RefreshToken: refreshPlainOld}); !errors.Is(err, appIdentity.ErrRefreshUnavailable) {
		t.Fatalf("sin store se esperaba ErrRefreshUnavailable, hay %v", err)
	}
	if _, err := newRefreshService(store, nil).Refresh(context.Background(), appIdentity.RefreshCommand{RefreshToken: refreshPlainOld}); !errors.Is(err, appIdentity.ErrRefreshUnavailable) {
		t.Fatalf("sin emisor se esperaba ErrRefreshUnavailable, hay %v", err)
	}
	if _, err := newRefreshService(store, &fakeIssuer{err: errors.New("sin clave")}).Refresh(context.Background(), appIdentity.RefreshCommand{RefreshToken: refreshPlainOld}); !errors.Is(err, appIdentity.ErrRefreshUnavailable) {
		t.Fatalf("sin firma se esperaba ErrRefreshUnavailable, hay %v", err)
	}
}

func TestRefreshGeneratorRepetidoFallaCerrado(t *testing.T) {
	store := seedRefreshStore()
	svc := appIdentity.NewRefreshService(store, &fakeIssuer{token: "jwt", jti: "jti"}).
		WithClock(func() time.Time { return fixedRefreshNow }).
		WithRefreshGenerator(func() (string, error) { return refreshPlainOld, nil })

	if _, err := svc.Refresh(context.Background(), appIdentity.RefreshCommand{RefreshToken: refreshPlainOld}); !errors.Is(err, appIdentity.ErrRefreshUnavailable) {
		t.Fatalf("un generador que repite el digest debe fallar cerrado, hay %v", err)
	}
	if len(store.rotateCalls) != 0 {
		t.Fatal("no debe escribirse una rotacion con digest repetido")
	}
}

func TestRefreshErrorDePersistenciaEs503(t *testing.T) {
	store := seedRefreshStore()
	store.rotateErr = errors.New("dynamodb no disponible")
	svc := newRefreshService(store, &fakeIssuer{token: "jwt", jti: "jti"})

	if _, err := svc.Refresh(context.Background(), appIdentity.RefreshCommand{RefreshToken: refreshPlainOld}); !errors.Is(err, appIdentity.ErrRefreshUnavailable) {
		t.Fatalf("se esperaba ErrRefreshUnavailable, hay %v", err)
	}
}

// TestRefreshConcurrenteUnaSolaRotacion reproduce dos refresh simultáneos con el mismo
// token: el compare-and-swap deja exactamente un ganador y nunca dos pares válidos.
func TestRefreshConcurrenteUnaSolaRotacion(t *testing.T) {
	store := seedRefreshStore()
	svc := newRefreshService(store, &fakeIssuer{token: "jwt", jti: "jti"})

	var wg sync.WaitGroup
	resultados := make([]error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(indice int) {
			defer wg.Done()
			_, err := svc.Refresh(context.Background(), appIdentity.RefreshCommand{RefreshToken: refreshPlainOld})
			resultados[indice] = err
		}(i)
	}
	wg.Wait()

	exitos := 0
	for _, err := range resultados {
		if err == nil {
			exitos++
			continue
		}
		if !errors.Is(err, appIdentity.ErrRefreshReplay) && !errors.Is(err, appIdentity.ErrRefreshConflict) {
			t.Fatalf("error concurrente inesperado: %v", err)
		}
	}
	if exitos != 1 {
		t.Fatalf("debe haber exactamente un ganador, hay %d", exitos)
	}
	if len(store.rotateCalls) != 1 {
		t.Fatalf("debe aplicarse una sola rotacion, hay %d", len(store.rotateCalls))
	}
	// El perdedor detecta el consumo durable y revoca la familia: no quedan dos pares vivos.
	if len(store.revokeCalls) != 1 {
		t.Fatalf("el perdedor debe revocar la familia por reutilizacion, hay %d", len(store.revokeCalls))
	}
	if store.sessions[sessionKey(refreshUserID, refreshSessionID)].Status != "REVOKED" {
		t.Fatal("la sesion debe quedar revocada tras la carrera")
	}
}

func TestRefreshRechazaCuentaNoActiva(t *testing.T) {
	casos := []struct {
		nombre string
		estado string
		existe bool
	}{
		{"deshabilitada", "DISABLED", true},
		{"eliminada", "DELETED", true},
		{"perfil ausente", "", false},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			store := seedRefreshStore()
			if caso.existe {
				store.users[refreshUserID] = &appIdentity.RefreshUser{UserID: refreshUserID, Status: caso.estado}
			} else {
				delete(store.users, refreshUserID)
			}
			svc := newRefreshService(store, &fakeIssuer{token: "jwt", jti: "jti"})

			_, err := svc.Refresh(context.Background(), appIdentity.RefreshCommand{RefreshToken: refreshPlainOld})
			if !errors.Is(err, appIdentity.ErrInvalidRefreshToken) {
				t.Fatalf("se esperaba ErrInvalidRefreshToken, hay %v", err)
			}
			if len(store.rotateCalls) != 0 {
				t.Fatal("una cuenta no activa no renueva tokens")
			}
		})
	}
}

func TestRefreshRechazaPerfilDeOtroUsuario(t *testing.T) {
	store := seedRefreshStore()
	store.users[refreshUserID] = &appIdentity.RefreshUser{UserID: "otro-usuario", Status: "ACTIVE"}
	svc := newRefreshService(store, &fakeIssuer{token: "jwt", jti: "jti"})

	_, err := svc.Refresh(context.Background(), appIdentity.RefreshCommand{RefreshToken: refreshPlainOld})
	if !errors.Is(err, appIdentity.ErrInvalidRefreshToken) {
		t.Fatalf("un perfil incoherente no puede autorizar, hay %v", err)
	}
	if len(store.rotateCalls) != 0 {
		t.Fatal("no debe rotarse con un perfil incoherente")
	}
}

func TestRefreshFallaCerradoSiNoLeeElPerfil(t *testing.T) {
	store := seedRefreshStore()
	store.userErr = errors.New("dynamodb no disponible")
	svc := newRefreshService(store, &fakeIssuer{token: "jwt", jti: "jti"})

	_, err := svc.Refresh(context.Background(), appIdentity.RefreshCommand{RefreshToken: refreshPlainOld})
	if !errors.Is(err, appIdentity.ErrRefreshUnavailable) {
		t.Fatalf("se esperaba ErrRefreshUnavailable, hay %v", err)
	}
	if len(store.rotateCalls) != 0 {
		t.Fatal("sin perfil verificado no se rota")
	}
}

func repeat(r rune, times int) string {
	buf := make([]rune, times)
	for i := range buf {
		buf[i] = r
	}
	return string(buf)
}
