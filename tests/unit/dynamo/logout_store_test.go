package dynamo_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	dynamoAdapter "github.com/nexumpr/Nexum-Backend/internal/adapters/dynamo"
	appIdentity "github.com/nexumpr/Nexum-Backend/internal/application/identity"
	domainDynamo "github.com/nexumpr/Nexum-Backend/internal/domain/dynamo"
	"github.com/nexumpr/Nexum-Backend/internal/ports"
	"github.com/nexumpr/Nexum-Backend/internal/transport/http/middleware"
)

// Pruebas de E004 sobre los adaptadores REALES (LogoutStore, SessionStore y RefreshStore)
// montados sobre el doble con estado que interpreta las mismas condiciones que emiten.
// Cubren las invariantes que ningún endpoint puede romper por separado: cierre de sesión,
// rotación, reutilización y aislamiento entre sesiones del mismo usuario.

const (
	e004Tabla   = "NEXUM_CORE_TEST"
	e004Usuario = "11111111-2222-4333-8444-555555555555"
	e004Otra    = "99999999-8888-4777-8666-555555555555"

	e004RefreshA = "refresh-sintetico-a"
	e004RefreshB = "refresh-sintetico-b"
	e004RefreshC = "refresh-sintetico-c"
)

var e004Ahora = time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)

// e004Emisor es un AccessTokenIssuer determinista: no firma nada, sólo cuenta.
type e004Emisor struct {
	mu sync.Mutex
	n  int
}

func (e *e004Emisor) IssueAccessToken(_ context.Context, _, _ string, _ time.Time) (string, string, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.n++
	return fmt.Sprintf("jwt-sintetico-%d", e.n), fmt.Sprintf("jti-sintetico-%d", e.n), nil
}

// e004Fallo sustituye a DynamoDB para forzar indisponibilidad total.
type e004Fallo struct{}

func (e004Fallo) GetItem(context.Context, *dynamodb.GetItemInput, ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
	return nil, errors.New("dynamodb inaccesible")
}

func (e004Fallo) TransactWriteItems(context.Context, *dynamodb.TransactWriteItemsInput, ...func(*dynamodb.Options)) (*dynamodb.TransactWriteItemsOutput, error) {
	return nil, errors.New("dynamodb inaccesible")
}

// stringAttrDe / attributeIntDe leen atributos del doble sin exponer detalles del SDK.
func stringAttrDe(item map[string]types.AttributeValue, nombre string) string {
	if valor, ok := item[nombre].(*types.AttributeValueMemberS); ok {
		return valor.Value
	}
	return ""
}

func attributeIntDe(item map[string]types.AttributeValue, nombre string) (int64, bool) {
	valor, ok := item[nombre].(*types.AttributeValueMemberN)
	if !ok || valor.Value == "" {
		return 0, false
	}
	n, err := strconv.ParseInt(valor.Value, 10, 64)
	if err != nil {
		return 0, false
	}
	return n, true
}

func e004SembrarPerfil(m *memoriaDynamo, userID string) {
	pk, sk := domainDynamo.UserKeys(userID)
	m.sembrar(pk, sk, map[string]types.AttributeValue{
		"entityType": strAttr("USER_PROFILE"),
		"userId":     strAttr(userID),
		"status":     strAttr(appIdentity.StatusActive),
	})
}

// e004SembrarSesion escribe sesión y localizador reales con el adaptador de E002, no a mano.
func e004SembrarSesion(t *testing.T, m *memoriaDynamo, userID, sessionID, familyID, refreshToken string) {
	t.Helper()
	store := dynamoAdapter.NewSessionStore(m, e004Tabla)
	err := store.CreateSession(context.Background(), appIdentity.SessionRecord{
		UserID:      userID,
		SessionID:   sessionID,
		FamilyID:    familyID,
		RefreshHash: appIdentity.HashToken(refreshToken),
		AccessJTI:   "jti-inicial",
		CreatedAt:   e004Ahora,
		ExpiresAt:   e004Ahora.Add(30 * 24 * time.Hour),
		Status:      appIdentity.StatusActive,
	})
	if err != nil {
		t.Fatalf("no se pudo sembrar la sesion %s: %v", sessionID, err)
	}
}

func e004RefreshService(m *memoriaDynamo) *appIdentity.RefreshService {
	contador := 0
	return appIdentity.NewRefreshService(dynamoAdapter.NewRefreshStore(m, e004Tabla), &e004Emisor{}).
		WithClock(func() time.Time { return e004Ahora }).
		WithRefreshGenerator(func() (string, error) {
			contador++
			return fmt.Sprintf("refresh-rotado-%d", contador), nil
		})
}

func e004LogoutService(m *memoriaDynamo) *appIdentity.LogoutService {
	return appIdentity.NewLogoutService(dynamoAdapter.NewLogoutStore(m, e004Tabla)).
		WithClock(func() time.Time { return e004Ahora })
}

func e004EstadoSesion(m *memoriaDynamo, userID, sessionID string) map[string]types.AttributeValue {
	pk, sk := domainDynamo.UserSessionKeys(userID, sessionID)
	return m.leer(pk, sk)
}

// TestE004LogoutRevocaYBloqueaRefresh cubre T03/T04/T05: tras el cierre, la sesión queda
// REVOKED con traza durable, el refresh vigente deja de rotar y el JWT deja de autorizar.
func TestE004LogoutRevocaYBloqueaRefresh(t *testing.T) {
	m := newMemoriaDynamo()
	e004SembrarPerfil(m, e004Usuario)
	e004SembrarSesion(t, m, e004Usuario, "sesion-a", "familia-a", e004RefreshA)

	ctx := context.Background()
	store := dynamoAdapter.NewLogoutStore(m, e004Tabla)

	if err := e004LogoutService(m).Logout(ctx, appIdentity.LogoutCommand{UserID: e004Usuario, SessionID: "sesion-a"}); err != nil {
		t.Fatalf("logout valido devolvio error: %v", err)
	}

	sesion := e004EstadoSesion(m, e004Usuario, "sesion-a")
	if status := stringAttrDe(sesion, "status"); status != appIdentity.StatusRevoked {
		t.Fatalf("status esperado REVOKED, obtenido %q", status)
	}
	if motivo := stringAttrDe(sesion, "revokeReason"); motivo != appIdentity.RevokeReasonLogout {
		t.Fatalf("revokeReason esperado %q, obtenido %q", appIdentity.RevokeReasonLogout, motivo)
	}
	if stringAttrDe(sesion, "revokedAt") == "" {
		t.Fatalf("revokedAt debe quedar registrado")
	}
	if version, _ := attributeIntDe(sesion, "version"); version != 2 {
		t.Fatalf("version esperada 2 tras el CAS, obtenida %d", version)
	}

	pk, sk := domainDynamo.RefreshKeys(appIdentity.HashToken(e004RefreshA))
	if status := stringAttrDe(m.leer(pk, sk), "status"); status != appIdentity.StatusRevoked {
		t.Fatalf("el localizador del refresh vigente debe quedar REVOKED, obtenido %q", status)
	}

	// T04: el refresh ya no renueva.
	if _, err := e004RefreshService(m).Refresh(ctx, appIdentity.RefreshCommand{RefreshToken: e004RefreshA}); !errors.Is(err, appIdentity.ErrInvalidRefreshToken) {
		t.Fatalf("refresh tras logout debe ser 401, obtenido %v", err)
	}
	// T05: el JWT de la sesión cerrada ya no autoriza.
	if err := store.AuthorizeSession(ctx, e004Usuario, "sesion-a"); !errors.Is(err, ports.ErrSessionUnauthorized) {
		t.Fatalf("AuthorizeSession tras logout debe rechazar, obtenido %v", err)
	}
}

// TestE004LogoutNoAfectaOtraSesion cubre T09/T10/T18: dos sesiones del mismo usuario; cerrar
// una no revoca la otra ni su refresh.
func TestE004LogoutNoAfectaOtraSesion(t *testing.T) {
	m := newMemoriaDynamo()
	e004SembrarPerfil(m, e004Usuario)
	e004SembrarSesion(t, m, e004Usuario, "sesion-a", "familia-a", e004RefreshA)
	e004SembrarSesion(t, m, e004Usuario, "sesion-b", "familia-b", e004RefreshB)

	ctx := context.Background()
	if err := e004LogoutService(m).Logout(ctx, appIdentity.LogoutCommand{UserID: e004Usuario, SessionID: "sesion-a"}); err != nil {
		t.Fatalf("logout de sesion-a fallo: %v", err)
	}

	if status := stringAttrDe(e004EstadoSesion(m, e004Usuario, "sesion-b"), "status"); status != appIdentity.StatusActive {
		t.Fatalf("sesion-b debe seguir ACTIVE, obtenido %q", status)
	}
	if err := dynamoAdapter.NewLogoutStore(m, e004Tabla).AuthorizeSession(ctx, e004Usuario, "sesion-b"); err != nil {
		t.Fatalf("sesion-b debe seguir autorizando, obtenido %v", err)
	}
	tokens, err := e004RefreshService(m).Refresh(ctx, appIdentity.RefreshCommand{RefreshToken: e004RefreshB})
	if err != nil {
		t.Fatalf("refresh de sesion-b debe seguir funcionando, obtenido %v", err)
	}
	if tokens.SessionID != "sesion-b" {
		t.Fatalf("el refresh debe conservar la sesion, obtenido %q", tokens.SessionID)
	}
	if tokens.AccessToken == "" || tokens.RefreshToken == "" {
		t.Fatalf("el refresh debe emitir tokens nuevos")
	}
}

// TestE004RotacionPreviaNoInvalidaAccessToken cubre T19 y T16: la rotación A→B→C sigue
// funcionando y un access token emitido antes de rotar no se invalida por comparar el jti.
func TestE004RotacionPreviaNoInvalidaAccessToken(t *testing.T) {
	m := newMemoriaDynamo()
	e004SembrarPerfil(m, e004Usuario)
	e004SembrarSesion(t, m, e004Usuario, "sesion-a", "familia-a", e004RefreshA)

	ctx := context.Background()
	servicio := e004RefreshService(m)

	primero, err := servicio.Refresh(ctx, appIdentity.RefreshCommand{RefreshToken: e004RefreshA})
	if err != nil {
		t.Fatalf("rotacion A->B fallo: %v", err)
	}
	segundo, err := servicio.Refresh(ctx, appIdentity.RefreshCommand{RefreshToken: primero.RefreshToken})
	if err != nil {
		t.Fatalf("rotacion B->C fallo: %v", err)
	}
	if primero.AccessToken == segundo.AccessToken || primero.RefreshToken == segundo.RefreshToken {
		t.Fatalf("cada rotacion debe emitir tokens distintos")
	}
	if primero.SessionID != segundo.SessionID || segundo.SessionID != "sesion-a" {
		t.Fatalf("la identidad de sesion debe mantenerse entre rotaciones")
	}

	// El jti cambió con cada rotación; la sesión sigue ACTIVE y debe autorizar.
	if err := dynamoAdapter.NewLogoutStore(m, e004Tabla).AuthorizeSession(ctx, e004Usuario, "sesion-a"); err != nil {
		t.Fatalf("una sesion ACTIVE rotada debe seguir autorizando (no se compara el jti): %v", err)
	}

	// Cerrar sesión después de rotar revoca el digest VIGENTE, no el primero.
	if err := e004LogoutService(m).Logout(ctx, appIdentity.LogoutCommand{UserID: e004Usuario, SessionID: "sesion-a"}); err != nil {
		t.Fatalf("logout tras rotaciones fallo: %v", err)
	}
	if _, err := servicio.Refresh(ctx, appIdentity.RefreshCommand{RefreshToken: segundo.RefreshToken}); !errors.Is(err, appIdentity.ErrInvalidRefreshToken) {
		t.Fatalf("el refresh vigente tras logout debe ser 401, obtenido %v", err)
	}
}

// TestE004ReplayConserva409 cubre T17: la reutilización sigue detectándose y revocando.
func TestE004ReplayConserva409(t *testing.T) {
	m := newMemoriaDynamo()
	e004SembrarPerfil(m, e004Usuario)
	e004SembrarSesion(t, m, e004Usuario, "sesion-a", "familia-a", e004RefreshA)

	ctx := context.Background()
	servicio := e004RefreshService(m)
	primero, err := servicio.Refresh(ctx, appIdentity.RefreshCommand{RefreshToken: e004RefreshA})
	if err != nil {
		t.Fatalf("rotacion inicial fallo: %v", err)
	}

	// Reutilizar el token ya consumido debe seguir dando replay (409) y revocar la familia.
	if _, err := servicio.Refresh(ctx, appIdentity.RefreshCommand{RefreshToken: e004RefreshA}); !errors.Is(err, appIdentity.ErrRefreshReplay) {
		t.Fatalf("replay debe devolver ErrRefreshReplay, obtenido %v", err)
	}
	if status := stringAttrDe(e004EstadoSesion(m, e004Usuario, "sesion-a"), "status"); status != appIdentity.StatusRevoked {
		t.Fatalf("el replay debe revocar la sesion, obtenido %q", status)
	}
	if _, err := servicio.Refresh(ctx, appIdentity.RefreshCommand{RefreshToken: primero.RefreshToken}); !errors.Is(err, appIdentity.ErrInvalidRefreshToken) {
		t.Fatalf("tras el replay la sesion no debe renovar, obtenido %v", err)
	}
}

// TestE004RevocacionObservableEnDosInstancias cubre T15: dos instancias independientes de la
// aplicación comparten la tabla, de modo que el cierre del nodo A es visible en el nodo B.
func TestE004RevocacionObservableEnDosInstancias(t *testing.T) {
	m := newMemoriaDynamo()
	e004SembrarPerfil(m, e004Usuario)
	e004SembrarSesion(t, m, e004Usuario, "sesion-a", "familia-a", e004RefreshA)

	ctx := context.Background()
	nodoA := dynamoAdapter.NewLogoutStore(m, e004Tabla)
	nodoB := dynamoAdapter.NewLogoutStore(m, e004Tabla)

	if err := nodoA.AuthorizeSession(ctx, e004Usuario, "sesion-a"); err != nil {
		t.Fatalf("antes del cierre el nodo A debe autorizar: %v", err)
	}
	if err := nodoB.AuthorizeSession(ctx, e004Usuario, "sesion-a"); err != nil {
		t.Fatalf("antes del cierre el nodo B debe autorizar: %v", err)
	}

	if err := appIdentity.NewLogoutService(nodoA).WithClock(func() time.Time { return e004Ahora }).
		Logout(ctx, appIdentity.LogoutCommand{UserID: e004Usuario, SessionID: "sesion-a"}); err != nil {
		t.Fatalf("cierre en el nodo A fallo: %v", err)
	}

	if err := nodoB.AuthorizeSession(ctx, e004Usuario, "sesion-a"); !errors.Is(err, ports.ErrSessionUnauthorized) {
		t.Fatalf("el nodo B debe rechazar la sesion revocada por A, obtenido %v", err)
	}
}

// TestE004SesionAjenaNoSeRevoca comprueba la propiedad: el `sid` de otro sujeto no se toca.
func TestE004SesionAjenaNoSeRevoca(t *testing.T) {
	m := newMemoriaDynamo()
	e004SembrarPerfil(m, e004Usuario)
	e004SembrarPerfil(m, e004Otra)
	e004SembrarSesion(t, m, e004Usuario, "sesion-a", "familia-a", e004RefreshA)

	ctx := context.Background()
	err := e004LogoutService(m).Logout(ctx, appIdentity.LogoutCommand{UserID: e004Otra, SessionID: "sesion-a"})
	if !errors.Is(err, appIdentity.ErrLogoutUnauthorized) {
		t.Fatalf("un sub ajeno no puede cerrar la sesion: %v", err)
	}
	if status := stringAttrDe(e004EstadoSesion(m, e004Usuario, "sesion-a"), "status"); status != appIdentity.StatusActive {
		t.Fatalf("la sesion del titular debe seguir ACTIVE, obtenido %q", status)
	}
	if err := dynamoAdapter.NewLogoutStore(m, e004Tabla).AuthorizeSession(ctx, e004Usuario, "sesion-a"); err != nil {
		t.Fatalf("la sesion del titular debe seguir autorizando: %v", err)
	}
}

// TestE004PersistenciaNoDisponible cubre T13/T14 en el adaptador: sin DynamoDB se falla
// cerrado y nunca se responde éxito.
func TestE004PersistenciaNoDisponible(t *testing.T) {
	ctx := context.Background()
	store := dynamoAdapter.NewLogoutStore(e004Fallo{}, e004Tabla)

	err := appIdentity.NewLogoutService(store).Logout(ctx, appIdentity.LogoutCommand{UserID: e004Usuario, SessionID: "sesion-a"})
	if !errors.Is(err, appIdentity.ErrLogoutUnavailable) {
		t.Fatalf("sin persistencia el cierre debe ser ErrLogoutUnavailable, obtenido %v", err)
	}
	if err := store.AuthorizeSession(ctx, e004Usuario, "sesion-a"); !errors.Is(err, ports.ErrSessionUnavailable) {
		t.Fatalf("sin persistencia la autorizacion debe ser ErrSessionUnavailable, obtenido %v", err)
	}

	// Sin cliente ni tabla configurados: también falla cerrado.
	for _, sinConfig := range []*dynamoAdapter.LogoutStore{
		dynamoAdapter.NewLogoutStore(nil, ""),
		dynamoAdapter.NewLogoutStore(nil, e004Tabla),
		dynamoAdapter.NewLogoutStore(e004Fallo{}, ""),
	} {
		if err := sinConfig.AuthorizeSession(ctx, e004Usuario, "sesion-a"); !errors.Is(err, ports.ErrSessionUnavailable) {
			t.Fatalf("configuracion ausente debe devolver ErrSessionUnavailable, obtenido %v", err)
		}
	}
}

// TestE004SesionInexistenteNoAutoriza cubre T06 en el adaptador.
func TestE004SesionInexistenteNoAutoriza(t *testing.T) {
	m := newMemoriaDynamo()
	store := dynamoAdapter.NewLogoutStore(m, e004Tabla)
	if err := store.AuthorizeSession(context.Background(), e004Usuario, "sesion-inexistente"); !errors.Is(err, ports.ErrSessionUnauthorized) {
		t.Fatalf("sesion inexistente debe ser ErrSessionUnauthorized, obtenido %v", err)
	}
	if err := store.AuthorizeSession(context.Background(), "", ""); !errors.Is(err, ports.ErrSessionUnauthorized) {
		t.Fatalf("identificadores vacios deben ser ErrSessionUnauthorized, obtenido %v", err)
	}
}

// TestE004SesionHeredadaSinVersion cubre la compatibilidad con un ítem escrito antes de que
// existiera `version`: se revoca con CAS de estado y la versión se inicializa a 1.
func TestE004SesionHeredadaSinVersion(t *testing.T) {
	m := newMemoriaDynamo()
	e004SembrarPerfil(m, e004Usuario)
	pk, sk := domainDynamo.UserSessionKeys(e004Usuario, "sesion-heredada")
	m.sembrar(pk, sk, map[string]types.AttributeValue{
		"entityType":  strAttr("USER_SESSION"),
		"userId":      strAttr(e004Usuario),
		"sessionId":   strAttr("sesion-heredada"),
		"familyId":    strAttr("familia-h"),
		"refreshHash": strAttr(appIdentity.HashToken(e004RefreshA)),
		"status":      strAttr(appIdentity.StatusActive),
		"expiresAt":   numAttr(e004Ahora.Add(30 * 24 * time.Hour).Unix()),
	})

	ctx := context.Background()
	if err := e004LogoutService(m).Logout(ctx, appIdentity.LogoutCommand{UserID: e004Usuario, SessionID: "sesion-heredada"}); err != nil {
		t.Fatalf("un item sin version debe poder cerrarse, obtenido %v", err)
	}
	sesion := m.leer(pk, sk)
	if status := stringAttrDe(sesion, "status"); status != appIdentity.StatusRevoked {
		t.Fatalf("status esperado REVOKED, obtenido %q", status)
	}
	if version, ok := attributeIntDe(sesion, "version"); !ok || version != 1 {
		t.Fatalf("la version debe inicializarse a 1, obtenido %d (presente=%v)", version, ok)
	}
}

// TestE004LogoutConcurrenteConRefresh cubre T11/T12: cerrar sesión mientras se rota nunca deja
// la sesión viva ni permite que el refresh la reviva. Se usa el doble aislado, no DynamoDB real.
func TestE004LogoutConcurrenteConRefresh(t *testing.T) {
	for iteracion := 0; iteracion < 25; iteracion++ {
		m := newMemoriaDynamo()
		e004SembrarPerfil(m, e004Usuario)
		e004SembrarSesion(t, m, e004Usuario, "sesion-a", "familia-a", e004RefreshA)

		ctx := context.Background()
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			_, _ = e004RefreshService(m).Refresh(ctx, appIdentity.RefreshCommand{RefreshToken: e004RefreshA})
		}()
		go func() {
			defer wg.Done()
			_ = e004LogoutService(m).Logout(ctx, appIdentity.LogoutCommand{UserID: e004Usuario, SessionID: "sesion-a"})
		}()
		wg.Wait()

		// El cierre reconcilia tras un conflicto, así que la sesión debe quedar revocada.
		if status := stringAttrDe(e004EstadoSesion(m, e004Usuario, "sesion-a"), "status"); status != appIdentity.StatusRevoked {
			t.Fatalf("iteracion %d: la sesion debe quedar REVOKED, obtenido %q", iteracion, status)
		}
		if err := dynamoAdapter.NewLogoutStore(m, e004Tabla).AuthorizeSession(ctx, e004Usuario, "sesion-a"); !errors.Is(err, ports.ErrSessionUnauthorized) {
			t.Fatalf("iteracion %d: la sesion revocada no debe autorizar, obtenido %v", iteracion, err)
		}
	}
}

// e004Verificador implementa ports.TokenProvider con claims fijos: el JWT es válido
// criptográficamente en todas las llamadas; lo que cambia es el estado de la sesión.
type e004Verificador struct{ claims map[string]ports.TokenClaims }

func (e004Verificador) GenerateAccessToken(context.Context, ports.TokenClaims) (string, error) {
	return "emitido", nil
}
func (e004Verificador) GenerateRefreshToken(context.Context) (string, error) { return "refresh", nil }

func (v e004Verificador) ValidateAccessToken(_ context.Context, token string) (*ports.TokenClaims, error) {
	claims, ok := v.claims[token]
	if !ok {
		return nil, ports.ErrInvalidToken
	}
	copia := claims
	return &copia, nil
}

// TestE004MiddlewareConAdaptadorRealRechazaSesionRevocada es la prueba de extremo a extremo
// del mecanismo: adaptador DynamoDB real (sobre el doble con estado), middleware real y dos
// instancias lógicas. El logout confirmado en el nodo A hace que el nodo B rechace el MISMO
// JWT antes de ejecutar el handler. No depende del caso de uso en el camino de rechazo.
func TestE004MiddlewareConAdaptadorRealRechazaSesionRevocada(t *testing.T) {
	m := newMemoriaDynamo()
	e004SembrarPerfil(m, e004Usuario)
	e004SembrarSesion(t, m, e004Usuario, "sesion-a", "familia-a", e004RefreshA)

	nodoA := dynamoAdapter.NewLogoutStore(m, e004Tabla)
	nodoB := dynamoAdapter.NewLogoutStore(m, e004Tabla)
	verificador := e004Verificador{claims: map[string]ports.TokenClaims{
		"jwt-valido": {Subject: e004Usuario, SessionID: "sesion-a"},
	}}

	invocado := false
	protegido := middleware.AuthenticateBearerWithSession(verificador, nodoB)(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			invocado = true
			w.WriteHeader(http.StatusNoContent)
		}),
	)
	peticion := func() *http.Request {
		req := httptest.NewRequest(http.MethodPost, "/protegido", nil)
		req.Header.Set("Authorization", "Bearer jwt-valido")
		return req
	}

	// 1. Sesión ACTIVE: el nodo B deja pasar.
	activa := httptest.NewRecorder()
	protegido.ServeHTTP(activa, peticion())
	if activa.Code != http.StatusNoContent || !invocado {
		t.Fatalf("una sesion activa debe alcanzar el handler: code=%d invocado=%v", activa.Code, invocado)
	}

	// 2. El nodo A confirma el cierre con el adaptador y el caso de uso reales.
	if err := appIdentity.NewLogoutService(nodoA).WithClock(func() time.Time { return e004Ahora }).
		Logout(context.Background(), appIdentity.LogoutCommand{UserID: e004Usuario, SessionID: "sesion-a"}); err != nil {
		t.Fatalf("el nodo A debe confirmar el cierre: %v", err)
	}

	// 3. El mismo JWT llega al nodo B: el middleware lo rechaza y el handler no corre.
	invocado = false
	revocada := httptest.NewRecorder()
	protegido.ServeHTTP(revocada, peticion())
	if revocada.Code != http.StatusUnauthorized {
		t.Fatalf("el nodo B debe rechazar la sesion revocada por A, obtenido %d", revocada.Code)
	}
	if invocado {
		t.Fatalf("el handler protegido NO debe ejecutarse tras la revocacion")
	}
}
