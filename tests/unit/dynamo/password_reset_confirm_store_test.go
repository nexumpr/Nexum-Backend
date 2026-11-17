package dynamo_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	dynamoAdapter "github.com/nexumpr/Nexum-Backend/internal/adapters/dynamo"
	appIdentity "github.com/nexumpr/Nexum-Backend/internal/application/identity"
	domainDynamo "github.com/nexumpr/Nexum-Backend/internal/domain/dynamo"
	"github.com/nexumpr/Nexum-Backend/internal/ports"
)

// Pruebas de E006 y A6 sobre los adaptadores REALES y el doble con estado: el mismo que
// interpreta las condiciones que emiten los adaptadores. No sustituyen la verificación AWS,
// pero demuestran la transacción, el consumo de un solo uso, la concurrencia y la época de
// revocación de extremo a extremo sin memoria como autoridad.

const (
	e006Tabla = "NEXUM_CORE_TEST"
	e006User  = "aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee"
	e006Token = "token-sintetico-de-recuperacion-0123456789"
)

var e006Ahora = time.Date(2026, 11, 17, 9, 0, 0, 0, time.UTC)

// resetHasher es determinista y barato: comprueba el flujo, no Argon2id (cubierto en A-050).
type resetHasher struct{}

func (resetHasher) HashPassword(password string) (string, error) {
	suma := sha256.Sum256([]byte("reset:" + password))
	return "hash:" + hex.EncodeToString(suma[:]), nil
}

func (resetHasher) VerifyPassword(password, hash string) (bool, error) {
	esperado, err := resetHasher{}.HashPassword(password)
	return err == nil && hash == esperado, nil
}

// e006Emisor registra la época con la que se emitió cada access token.
type e006Emisor struct {
	mu     sync.Mutex
	epocas []int64
}

func (e *e006Emisor) IssueAccessToken(_ context.Context, _, _ string, epoch int64, _ time.Time) (string, string, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.epocas = append(e.epocas, epoch)
	n := len(e.epocas)
	return "jwt-sintetico", fmt.Sprintf("jti-sintetico-%d", n), nil
}

func (e *e006Emisor) ultimaEpoca() int64 {
	e.mu.Lock()
	defer e.mu.Unlock()
	if len(e.epocas) == 0 {
		return -1
	}
	return e.epocas[len(e.epocas)-1]
}

func e006SembrarPerfil(m *memoriaDynamo, userID, password string, conVersion bool, epoch int64) {
	attrs := map[string]types.AttributeValue{
		"entityType":    strAttr("USER_PROFILE"),
		"userId":        strAttr(userID),
		"status":        strAttr(appIdentity.StatusActive),
		"email":         strAttr("reset@example.invalid"),
		"displayName":   strAttr("Cuenta sintetica"),
		"emailVerified": &types.AttributeValueMemberBOOL{Value: true},
		"passwordHash":  strAttr(hashDePrueba(password)),
		"sessionEpoch":  numAttr(epoch),
	}
	if conVersion {
		attrs["version"] = numAttr(1)
	}
	pk, sk := domainDynamo.UserKeys(userID)
	m.sembrar(pk, sk, attrs)
	// La reserva de email es la que E002 usa para resolver la cuenta: sin ella cualquier
	// login sería indistinguible de un correo inexistente.
	emailPK, emailSK := domainDynamo.EmailReservationKeys("reset@example.invalid")
	m.sembrar(emailPK, emailSK, map[string]types.AttributeValue{
		"entityType": strAttr("EMAIL_RESERVATION"),
		"userId":     strAttr(userID),
	})
}

func hashDePrueba(password string) string {
	hash, _ := resetHasher{}.HashPassword(password)
	return hash
}

func e006Admitir(t *testing.T, m *memoriaDynamo, token string, userID string, expira time.Time) {
	t.Helper()
	store := dynamoAdapter.NewPasswordResetStore(m, e006Tabla)
	hash := appIdentity.HashToken(token)
	err := store.Admitir(context.Background(), ports.ResetAdmission{
		UserID:     userID,
		Destino:    "reset@example.invalid",
		TokenHash:  hash,
		TokenSello: []byte("sobre-sintetico"),
		ExpiraEn:   expira,
		OutboxID:   hash[:16],
		Ahora:      e006Ahora,
	})
	if err != nil {
		t.Fatalf("no se pudo admitir el token de prueba: %v", err)
	}
}

func e006Servicio(m *memoriaDynamo) *appIdentity.PasswordResetConfirmService {
	return appIdentity.NewPasswordResetConfirmService(
		dynamoAdapter.NewPasswordResetConfirmStore(m, e006Tabla),
		resetHasher{},
	).WithClock(func() time.Time { return e006Ahora })
}

func e006Login(m *memoriaDynamo, emisor appIdentity.AccessTokenIssuer) *appIdentity.LoginService {
	return appIdentity.NewLoginService(dynamoAdapter.NewSessionStore(m, e006Tabla), resetHasher{}, emisor).
		WithClock(func() time.Time { return e006Ahora })
}

var e006RefreshSeq atomic.Int64

func e006RefreshService(m *memoriaDynamo, emisor appIdentity.AccessTokenIssuer) *appIdentity.RefreshService {
	return appIdentity.NewRefreshService(dynamoAdapter.NewRefreshStore(m, e006Tabla), emisor).
		WithClock(func() time.Time { return e006Ahora }).
		WithRefreshGenerator(func() (string, error) {
			return fmt.Sprintf("refresh-e006-%d", e006RefreshSeq.Add(1)), nil
		})
}

// TestE006CambioRealYReutilizacion cubre 1/2/5/6: el token válido cambia el hash, se
// consume, la contraseña anterior deja de autenticar y la nueva sí.
func TestE006CambioRealYReutilizacion(t *testing.T) {
	m := newMemoriaDynamo()
	e006SembrarPerfil(m, e006User, "OldPassphrase#2026", true, 0)
	e006Admitir(t, m, e006Token, e006User, e006Ahora.Add(30*time.Minute))

	ctx := context.Background()
	if err := e006Servicio(m).Confirm(ctx, appIdentity.ResetConfirmCommand{
		Token:       e006Token,
		NewPassword: "NewPassphrase#2026",
	}); err != nil {
		t.Fatalf("cambio valido devolvio error: %v", err)
	}

	pk, sk := domainDynamo.UserKeys(e006User)
	perfil := m.leer(pk, sk)
	if hash := stringAttrDe(perfil, "passwordHash"); hash != hashDePrueba("NewPassphrase#2026") {
		t.Fatalf("el passwordHash no se actualizo")
	}
	if version, ok := attributeIntDe(perfil, "version"); !ok || version != 2 {
		t.Fatalf("version esperada 2, obtenida %v", version)
	}
	if epoch, ok := attributeIntDe(perfil, "sessionEpoch"); !ok || epoch != 1 {
		t.Fatalf("sessionEpoch esperada 1, obtenida %v", epoch)
	}

	tokenPK := "RESET#" + appIdentity.HashToken(e006Token)
	if used := m.leer(tokenPK, "RESET")["used"]; used == nil {
		t.Fatalf("el token debe quedar con used=true")
	} else if valor, ok := used.(*types.AttributeValueMemberBOOL); !ok || !valor.Value {
		t.Fatalf("used debe ser true")
	}

	// Reutilización: 409 contractual.
	if err := e006Servicio(m).Confirm(ctx, appIdentity.ResetConfirmCommand{
		Token:       e006Token,
		NewPassword: "OtraPassphrase#2026",
	}); !errors.Is(err, appIdentity.ErrResetTokenUsado) {
		t.Fatalf("token reutilizado debe ser ErrResetTokenUsado, obtenido %v", err)
	}

	emisor := &e006Emisor{}
	login := e006Login(m, emisor)
	if _, err := login.Login(ctx, appIdentity.LoginCommand{Email: "reset@example.invalid", Password: "OldPassphrase#2026"}); !errors.Is(err, appIdentity.ErrInvalidCredentials) {
		t.Fatalf("la contrasena anterior debe rechazarse, obtenido %v", err)
	}
	if _, err := login.Login(ctx, appIdentity.LoginCommand{Email: "reset@example.invalid", Password: "NewPassphrase#2026"}); err != nil {
		t.Fatalf("la contrasena nueva debe permitir login: %v", err)
	}
	if epoca := emisor.ultimaEpoca(); epoca != 1 {
		t.Fatalf("el JWT nuevo debe emitirse con epo=1, obtenido %d", epoca)
	}
}

// TestE006TokenExpiradoRechazado cubre 3: la caducidad lógica se comprueba aunque el
// registro siga físicamente en la tabla.
func TestE006TokenExpiradoRechazado(t *testing.T) {
	m := newMemoriaDynamo()
	e006SembrarPerfil(m, e006User, "OldPassphrase#2026", true, 0)
	e006Admitir(t, m, e006Token, e006User, e006Ahora.Add(-time.Minute))

	err := e006Servicio(m).Confirm(context.Background(), appIdentity.ResetConfirmCommand{
		Token:       e006Token,
		NewPassword: "NewPassphrase#2026",
	})
	if !errors.Is(err, ports.ErrResetTokenInvalido) {
		t.Fatalf("token expirado debe ser invalido, obtenido %v", err)
	}
	pk, sk := domainDynamo.UserKeys(e006User)
	if stringAttrDe(m.leer(pk, sk), "passwordHash") != hashDePrueba("OldPassphrase#2026") {
		t.Fatalf("un token expirado no puede cambiar la contrasena")
	}
}

// TestE006ConcurrenciaUnSoloGanador cubre 4: dos solicitudes simultáneas con el mismo token.
func TestE006ConcurrenciaUnSoloGanador(t *testing.T) {
	m := newMemoriaDynamo()
	e006SembrarPerfil(m, e006User, "OldPassphrase#2026", true, 0)
	e006Admitir(t, m, e006Token, e006User, e006Ahora.Add(30*time.Minute))

	servicio := e006Servicio(m)
	var wg sync.WaitGroup
	resultados := make([]error, 2)
	for i := range resultados {
		wg.Add(1)
		go func(indice int) {
			defer wg.Done()
			resultados[indice] = servicio.Confirm(context.Background(), appIdentity.ResetConfirmCommand{
				Token:       e006Token,
				NewPassword: "NewPassphrase#2026",
			})
		}(i)
	}
	wg.Wait()

	exitos, conflictos := 0, 0
	for _, err := range resultados {
		switch {
		case err == nil:
			exitos++
		case errors.Is(err, appIdentity.ErrResetTokenUsado), errors.Is(err, appIdentity.ErrResetEnProceso):
			conflictos++
		default:
			t.Fatalf("error inesperado en concurrencia: %v", err)
		}
	}
	if exitos != 1 || conflictos != 1 {
		t.Fatalf("solo una solicitud puede ganar: exitos=%d conflictos=%d", exitos, conflictos)
	}
}

// TestE006PerfilLegadoSinVersion cubre la compatibilidad con perfiles anteriores a version.
func TestE006PerfilLegadoSinVersion(t *testing.T) {
	m := newMemoriaDynamo()
	e006SembrarPerfil(m, e006User, "OldPassphrase#2026", false, 0)
	e006Admitir(t, m, e006Token, e006User, e006Ahora.Add(30*time.Minute))

	if err := e006Servicio(m).Confirm(context.Background(), appIdentity.ResetConfirmCommand{
		Token:       e006Token,
		NewPassword: "NewPassphrase#2026",
	}); err != nil {
		t.Fatalf("un perfil legado debe poder cambiar su contrasena: %v", err)
	}
	pk, sk := domainDynamo.UserKeys(e006User)
	perfil := m.leer(pk, sk)
	if version, ok := attributeIntDe(perfil, "version"); !ok || version != 1 {
		t.Fatalf("version debe inicializarse a 1, obtenido %v", version)
	}
	if epoch, ok := attributeIntDe(perfil, "sessionEpoch"); !ok || epoch != 1 {
		t.Fatalf("sessionEpoch esperada 1, obtenida %v", epoch)
	}
}

// TestE006CuentaNoDisponible rechaza cuentas inexistentes o deshabilitadas sin enumerar.
func TestE006CuentaNoDisponible(t *testing.T) {
	casos := []struct {
		nombre  string
		sembrar func(*memoriaDynamo)
	}{
		{"perfil ausente", func(*memoriaDynamo) {}},
		{"cuenta deshabilitada", func(m *memoriaDynamo) {
			e006SembrarPerfil(m, e006User, "OldPassphrase#2026", true, 0)
			pk, sk := domainDynamo.UserKeys(e006User)
			m.sembrar(pk, sk, map[string]types.AttributeValue{
				"entityType":   strAttr("USER_PROFILE"),
				"userId":       strAttr(e006User),
				"status":       strAttr("DISABLED"),
				"passwordHash": strAttr(hashDePrueba("OldPassphrase#2026")),
				"version":      numAttr(1),
			})
		}},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			m := newMemoriaDynamo()
			caso.sembrar(m)
			e006Admitir(t, m, e006Token, e006User, e006Ahora.Add(30*time.Minute))
			err := e006Servicio(m).Confirm(context.Background(), appIdentity.ResetConfirmCommand{
				Token:       e006Token,
				NewPassword: "NewPassphrase#2026",
			})
			if !errors.Is(err, ports.ErrResetTokenInvalido) {
				t.Fatalf("se esperaba rechazo generico, obtenido %v", err)
			}
		})
	}
}

// TestE006FalloDynamoFallaCerrado comprueba que un fallo de persistencia nunca confirma.
func TestE006FalloDynamoFallaCerrado(t *testing.T) {
	servicio := appIdentity.NewPasswordResetConfirmService(
		dynamoAdapter.NewPasswordResetConfirmStore(e004Fallo{}, e006Tabla), resetHasher{},
	).WithClock(func() time.Time { return e006Ahora })
	err := servicio.Confirm(context.Background(), appIdentity.ResetConfirmCommand{
		Token:       e006Token,
		NewPassword: "NewPassphrase#2026",
	})
	if !errors.Is(err, appIdentity.ErrResetConfirmUnavailable) {
		t.Fatalf("se esperaba ErrResetConfirmUnavailable, obtenido %v", err)
	}
}

// TestA6EpocaRevocaAccessYRefresh cubre 5..9 de extremo a extremo: tras el reset, el access
// token antiguo deja de autorizar (época), el refresh antiguo no renueva y un login nuevo
// nace con la época vigente. Dos instancias lógicas comparten el mismo veredicto.
func TestA6EpocaRevocaAccessYRefresh(t *testing.T) {
	m := newMemoriaDynamo()
	e006SembrarPerfil(m, e006User, "OldPassphrase#2026", true, 0)

	ctx := context.Background()
	emisor := &e006Emisor{}
	login := e006Login(m, emisor)
	sesionVieja, err := login.Login(ctx, appIdentity.LoginCommand{Email: "reset@example.invalid", Password: "OldPassphrase#2026"})
	if err != nil {
		t.Fatalf("login inicial fallo: %v", err)
	}
	if emisor.ultimaEpoca() != 0 {
		t.Fatalf("el login inicial debe emitir epo=0")
	}

	sesionPK, sesionSK := domainDynamo.UserSessionKeys(e006User, sesionVieja.SessionID)
	if epoch, ok := attributeIntDe(m.leer(sesionPK, sesionSK), "sessionEpoch"); !ok || epoch != 0 {
		t.Fatalf("la sesion inicial debe persistir sessionEpoch=0, obtenido %v", epoch)
	}

	e006Admitir(t, m, e006Token, e006User, e006Ahora.Add(30*time.Minute))
	if err := e006Servicio(m).Confirm(ctx, appIdentity.ResetConfirmCommand{
		Token:       e006Token,
		NewPassword: "NewPassphrase#2026",
	}); err != nil {
		t.Fatalf("reset fallo: %v", err)
	}

	// 7/9: el access token antiguo ya no autoriza en ninguna instancia lógica.
	nodoA := dynamoAdapter.NewLogoutStore(m, e006Tabla)
	nodoB := dynamoAdapter.NewLogoutStore(m, e006Tabla)
	for nombre, nodo := range map[string]*dynamoAdapter.LogoutStore{"A": nodoA, "B": nodoB} {
		if err := nodo.AuthorizeSession(ctx, e006User, sesionVieja.SessionID, 0); !errors.Is(err, ports.ErrSessionUnauthorized) {
			t.Fatalf("nodo %s debe rechazar el token antiguo, obtenido %v", nombre, err)
		}
	}

	// 8: el refresh de la sesión antigua no renueva.
	if _, err := e006RefreshService(m, emisor).Refresh(ctx, appIdentity.RefreshCommand{RefreshToken: sesionVieja.RefreshToken}); !errors.Is(err, appIdentity.ErrInvalidRefreshToken) {
		t.Fatalf("el refresh antiguo debe rechazarse, obtenido %v", err)
	}

	// 6: login nuevo con la contraseña nueva; nace en la época 1 y autoriza en ambos nodos.
	sesionNueva, err := login.Login(ctx, appIdentity.LoginCommand{Email: "reset@example.invalid", Password: "NewPassphrase#2026"})
	if err != nil {
		t.Fatalf("login con la contrasena nueva fallo: %v", err)
	}
	if emisor.ultimaEpoca() != 1 {
		t.Fatalf("el login nuevo debe emitir epo=1, obtenido %d", emisor.ultimaEpoca())
	}
	sesionNuevaPK, sesionNuevaSK := domainDynamo.UserSessionKeys(e006User, sesionNueva.SessionID)
	if epoch, ok := attributeIntDe(m.leer(sesionNuevaPK, sesionNuevaSK), "sessionEpoch"); !ok || epoch != 1 {
		t.Fatalf("la sesion nueva debe persistir sessionEpoch=1, obtenido %v", epoch)
	}
	for nombre, nodo := range map[string]*dynamoAdapter.LogoutStore{"A": nodoA, "B": nodoB} {
		if err := nodo.AuthorizeSession(ctx, e006User, sesionNueva.SessionID, 1); err != nil {
			t.Fatalf("nodo %s debe aceptar el token nuevo: %v", nombre, err)
		}
		// Una época declarada distinta tampoco autoriza.
		if err := nodo.AuthorizeSession(ctx, e006User, sesionNueva.SessionID, 0); !errors.Is(err, ports.ErrSessionUnauthorized) {
			t.Fatalf("nodo %s debe rechazar una epoca desalineada", nombre)
		}
	}

	// El refresh antiguo sigue rechazado tras el login nuevo (aislamiento por época).
	if _, err := e006RefreshService(m, emisor).Refresh(ctx, appIdentity.RefreshCommand{RefreshToken: sesionVieja.RefreshToken}); !errors.Is(err, appIdentity.ErrInvalidRefreshToken) {
		t.Fatalf("el refresh antiguo debe seguir rechazado, obtenido %v", err)
	}
	// El refresh nuevo sí renueva una vez.
	if _, err := e006RefreshService(m, emisor).Refresh(ctx, appIdentity.RefreshCommand{RefreshToken: sesionNueva.RefreshToken}); err != nil {
		t.Fatalf("el refresh de la sesion nueva debe renovar: %v", err)
	}
}
