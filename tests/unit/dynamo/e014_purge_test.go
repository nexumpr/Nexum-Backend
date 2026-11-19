package dynamo_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	dynamoAdapter "github.com/nexumpr/Nexum-Backend/internal/adapters/dynamo"
	appAccount "github.com/nexumpr/Nexum-Backend/internal/application/account"
	domainCache "github.com/nexumpr/Nexum-Backend/internal/domain/cache"
	domainDeletion "github.com/nexumpr/Nexum-Backend/internal/domain/deletion"
	domainDynamo "github.com/nexumpr/Nexum-Backend/internal/domain/dynamo"
	"github.com/nexumpr/Nexum-Backend/internal/ports"
)

// Pruebas de E014 sobre el adaptador REAL (DeletionStore) y el worker REAL montados sobre un
// doble con estado que interpreta las mismas condiciones y actualizaciones. Cubren el
// descubrimiento sin Scan, la página sin coincidencias, el fencing, la protección del control,
// la purga parcial, la verificación previa a PURGED, los reintentos y la reanudación.

const (
	e014Tabla     = "NEXUM_CORE_TEST"
	e014Usuario   = "aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee"
	e014OtroUser  = "ffffffff-1111-4222-8333-444444444444"
	e014RequestID = "018f90ef-08e5-7a68-b09f-2b67a47c92f0"
	e014Email     = "titular@example.invalid"
	e014Digest    = "digest-sintetico-a"
)

var e014T0 = time.Date(2026, 11, 18, 10, 0, 0, 0, time.UTC)

// ---------- utilidades de prueba ----------

type relojPurga struct {
	mu sync.Mutex
	t  time.Time
}

func nuevoRelojPurga(t time.Time) *relojPurga { return &relojPurga{t: t} }

func (r *relojPurga) ahora() time.Time {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.t
}

func (r *relojPurga) avanzar(d time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.t = r.t.Add(d)
}

func (r *relojPurga) fijar(t time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.t = t
}

type e014Lider struct{}

func (e014Lider) Adquirir(context.Context, string, time.Duration) (bool, error) { return true, nil }

type e014Cache struct {
	mu       sync.Mutex
	prefijos []string
	falla    error
}

func (c *e014Cache) DeletePrefix(_ context.Context, prefix string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.falla != nil {
		return c.falla
	}
	c.prefijos = append(c.prefijos, prefix)
	return nil
}

func (c *e014Cache) registrados() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.prefijos...)
}

func e014Logger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func e014Worker(store *dynamoAdapter.DeletionStore, cache *e014Cache, reloj *relojPurga) *appAccount.PurgeWorker {
	return appAccount.NewPurgeWorker(store, cache, e014Lider{}, e014Logger()).
		WithClock(reloj.ahora).
		WithEnvironment("")
}

func e014Contexto() context.Context { return context.Background() }

// e014SembrarCuenta crea los datos sintéticos de una cuenta: perfil y entidades de la
// partición, localizador de refresh, reserva de correo y entradas de la cola de recuperación
// (del titular y de otro usuario). Devuelve las SK de la partición que deben desaparecer.
func e014SembrarCuenta(m *purgaDynamo, userID string) []string {
	pk := domainDynamo.PrefixUser + userID

	m.sembrar(pk, "PROFILE", map[string]types.AttributeValue{
		"entityType":   strAttr("USER_PROFILE"),
		"userId":       strAttr(userID),
		"email":        strAttr(e014Email),
		"status":       strAttr(appAccount.ProfileStatusActive),
		"passwordHash": strAttr("h:sintetica"),
		"version":      numAttr(1),
		"sessionEpoch": numAttr(0),
	})
	m.sembrar(pk, "PREFERENCES", map[string]types.AttributeValue{"entityType": strAttr("USER_PREFERENCES")})
	m.sembrar(pk, "ONBOARDING", map[string]types.AttributeValue{"entityType": strAttr("USER_ONBOARDING")})
	m.sembrar(pk, "INCOME", map[string]types.AttributeValue{"entityType": strAttr("USER_INCOME")})
	m.sembrar(pk, "CONSENT#2026-11-v2", map[string]types.AttributeValue{"entityType": strAttr("USER_CONSENT")})
	m.sembrar(pk, "MOVEMENT#2026-11-01#a", map[string]types.AttributeValue{"entityType": strAttr("MOVEMENT")})
	m.sembrar(pk, "GOAL#g", map[string]types.AttributeValue{"entityType": strAttr("GOAL")})
	m.sembrar(pk, "RECURRING#r", map[string]types.AttributeValue{"entityType": strAttr("RECURRING")})
	m.sembrar(pk, "IDEM#e014#abc", map[string]types.AttributeValue{"entityType": strAttr("IDEMPOTENCY")})
	m.sembrar(pk, "SESSION#s1", map[string]types.AttributeValue{
		"entityType":  strAttr("USER_SESSION"),
		"userId":      strAttr(userID),
		"sessionId":   strAttr("s1"),
		"status":      strAttr(appAccount.ProfileStatusActive),
		"refreshHash": strAttr(e014Digest),
		"expiresAt":   numAttr(time.Now().UTC().Add(48 * time.Hour).Unix()),
	})

	// Referencias externas a la partición.
	emailPK, emailSK := domainDeletion.EmailReservationKeys(domainDeletion.EmailHash(e014Email))
	m.sembrar(emailPK, emailSK, map[string]types.AttributeValue{
		"entityType": strAttr("EMAIL_RESERVATION"),
		"userId":     strAttr(userID),
	})
	refreshPK, refreshSK := domainDeletion.RefreshLocatorKeys(e014Digest)
	m.sembrar(refreshPK, refreshSK, map[string]types.AttributeValue{
		"entityType": strAttr("REFRESH_LOOKUP"),
		"userId":     strAttr(userID),
		"sessionId":  strAttr("s1"),
	})
	m.sembrar(domainDeletion.PartitionQueuePasswordReset, "2026-11-18T10:00:00Z#propio", map[string]types.AttributeValue{
		"userId": strAttr(userID),
		"estado": strAttr("PENDIENTE"),
	})
	m.sembrar(domainDeletion.PartitionQueuePasswordReset, "2026-11-18T10:00:00Z#ajeno", map[string]types.AttributeValue{
		"userId": strAttr(e014OtroUser),
		"estado": strAttr("PENDIENTE"),
	})

	return []string{
		"PROFILE", "PREFERENCES", "ONBOARDING", "INCOME", "CONSENT#2026-11-v2",
		"MOVEMENT#2026-11-01#a", "GOAL#g", "RECURRING#r", "IDEM#e014#abc", "SESSION#s1",
	}
}

func e014Programar(t *testing.T, store *dynamoAdapter.DeletionStore, userID, requestID string, ahora time.Time) {
	t.Helper()
	err := store.ScheduleDeletion(e014Contexto(), appAccount.DeletionApplication{
		UserID:               userID,
		RequestID:            requestID,
		EmailHash:            domainDeletion.EmailHash(e014Email),
		ExpectedVersion:      1,
		HasVersion:           true,
		ExpectedSessionEpoch: 0,
		Now:                  ahora,
	})
	if err != nil {
		t.Fatalf("no se pudo programar la eliminacion: %v", err)
	}
}

func e014Control(m *purgaDynamo, userID, requestID string) map[string]types.AttributeValue {
	pk, sk := domainDeletion.UserControlKeys(userID, requestID)
	return m.leer(pk, sk)
}

func e014Cola(m *purgaDynamo, requestID string) map[string]types.AttributeValue {
	pk, sk := domainDeletion.QueueKeys(requestID)
	return m.leer(pk, sk)
}

func e014Candidato() appAccount.PurgeCandidate {
	return appAccount.PurgeCandidate{UserID: e014Usuario, RequestID: e014RequestID, State: appAccount.PurgeStatePending}
}

// e014CorrerHastaPurga ejecuta ciclos avanzando el reloj hasta PURGED o el tope.
func e014CorrerHastaPurga(t *testing.T, worker *appAccount.PurgeWorker, reloj *relojPurga, m *purgaDynamo, tope int) int {
	t.Helper()
	for ciclo := 0; ciclo < tope; ciclo++ {
		reloj.avanzar(time.Minute)
		worker.Ciclo(e014Contexto())
		if estado := stringAttrDe(e014Control(m, e014Usuario, e014RequestID), "state"); estado == appAccount.PurgeStatePurged {
			return ciclo + 1
		}
	}
	t.Fatalf("la purga no llego a PURGED en %d ciclos (estado=%q)",
		tope, stringAttrDe(e014Control(m, e014Usuario, e014RequestID), "state"))
	return tope
}

// ---------- pruebas ----------

// TestE014RevocacionEfectivaDosNodos prueba que, tras la transaccion de 3 items, el mismo
// access token deja de autorizar en DOS instancias logicas sobre el mismo almacen.
func TestE014RevocacionEfectivaDosNodos(t *testing.T) {
	m := newPurgaDynamo()
	e014SembrarCuenta(m, e014Usuario)
	store := dynamoAdapter.NewDeletionStore(m, e014Tabla)

	nodoA := dynamoAdapter.NewLogoutStore(m, e014Tabla)
	nodoB := dynamoAdapter.NewLogoutStore(m, e014Tabla)
	if err := nodoA.AuthorizeSession(e014Contexto(), e014Usuario, "s1", 0); err != nil {
		t.Fatalf("antes de la solicitud la sesion debe autorizar en A: %v", err)
	}
	if err := nodoB.AuthorizeSession(e014Contexto(), e014Usuario, "s1", 0); err != nil {
		t.Fatalf("antes de la solicitud la sesion debe autorizar en B: %v", err)
	}

	e014Programar(t, store, e014Usuario, e014RequestID, e014T0)

	for _, nodo := range []struct {
		nombre string
		store  *dynamoAdapter.LogoutStore
	}{{"A", nodoA}, {"B", nodoB}} {
		for _, epoca := range []int64{0, 1} {
			err := nodo.store.AuthorizeSession(e014Contexto(), e014Usuario, "s1", epoca)
			if !errors.Is(err, ports.ErrSessionUnauthorized) {
				t.Fatalf("nodo %s, epoca %d: la sesion debe quedar revocada, obtenido %v", nodo.nombre, epoca, err)
			}
		}
	}

	// La transaccion dejo el perfil en DELETION_PENDING con la epoca incrementada.
	perfil := m.leer(domainDeletion.UserPartition(e014Usuario), "PROFILE")
	if estado := stringAttrDe(perfil, "status"); estado != domainDeletion.StatusDeletionPending {
		t.Fatalf("estado esperado DELETION_PENDING, obtenido %q", estado)
	}
	if epoca, _ := attributeIntDe(perfil, "sessionEpoch"); epoca != 1 {
		t.Fatalf("epoca esperada 1, obtenida %d", epoca)
	}
	if version, _ := attributeIntDe(perfil, "version"); version != 2 {
		t.Fatalf("version esperada 2, obtenida %d", version)
	}
}

// TestE014PurgaCompletaTombstone recorre todas las fases y comprueba el tombstone minimo.
func TestE014PurgaCompletaTombstone(t *testing.T) {
	m := newPurgaDynamo()
	esperadas := e014SembrarCuenta(m, e014Usuario)
	store := dynamoAdapter.NewDeletionStore(m, e014Tabla)
	reloj := nuevoRelojPurga(e014T0)

	e014Programar(t, store, e014Usuario, e014RequestID, e014T0)

	// Proteccion del control: borrar por SK con prefijo DELETION# no elimina nada.
	if err := store.DeletePartitionItem(e014Contexto(), e014Usuario, domainDeletion.PrefixDeletion+e014RequestID); err != nil {
		t.Fatalf("la proteccion del control no debe devolver error: %v", err)
	}
	if e014Control(m, e014Usuario, e014RequestID) == nil {
		t.Fatalf("el control de progreso no puede borrarse durante el barrido")
	}

	cache := &e014Cache{}
	worker := e014Worker(store, cache, reloj)
	e014CorrerHastaPurga(t, worker, reloj, m, 20)

	control := e014Control(m, e014Usuario, e014RequestID)
	if estado := stringAttrDe(control, "state"); estado != appAccount.PurgeStatePurged {
		t.Fatalf("estado esperado PURGED, obtenido %q", estado)
	}
	if requestID := stringAttrDe(control, "requestId"); requestID != e014RequestID {
		t.Fatalf("el tombstone conserva requestId, obtenido %q", requestID)
	}
	if _, ok := attributeIntDe(control, "deletedCount"); !ok {
		t.Fatalf("el tombstone debe conservar deletedCount")
	}
	if borrados, _ := attributeIntDe(control, "deletedCount"); borrados != int64(len(esperadas)) {
		t.Fatalf("deletedCount esperado %d, obtenido %d", len(esperadas), borrados)
	}
	if _, ok := attributeIntDe(control, "expiresAt"); !ok {
		t.Fatalf("el tombstone debe declarar expiresAt (retencion logica de 90 dias)")
	}
	for _, prohibido := range []string{"emailHash", "refreshDigests", "lastSK", "owner", "unknownKeys", "userId", "attempts", "fence", "phase", "dueAt", "leaseUntil"} {
		if _, presente := control[prohibido]; presente {
			t.Fatalf("el tombstone no puede conservar %q", prohibido)
		}
	}

	// La particion solo conserva el control.
	for _, item := range m.porParticion(domainDeletion.UserPartition(e014Usuario)) {
		if sk := stringAttrDe(item, "SK"); sk != domainDeletion.PrefixDeletion+e014RequestID {
			t.Fatalf("residuo en la particion: %q", sk)
		}
	}
	// Referencias externas y cola del titular eliminadas; la del otro usuario intacta.
	if e014Cola(m, e014RequestID) != nil {
		t.Fatalf("la entrada de cola debe borrarse al completar")
	}
	emailPK, emailSK := domainDeletion.EmailReservationKeys(domainDeletion.EmailHash(e014Email))
	if m.leer(emailPK, emailSK) != nil {
		t.Fatalf("la reserva de correo debe eliminarse")
	}
	refreshPK, refreshSK := domainDeletion.RefreshLocatorKeys(e014Digest)
	if m.leer(refreshPK, refreshSK) != nil {
		t.Fatalf("el localizador de refresh debe eliminarse")
	}
	colaRecuperacion := m.porParticion(domainDeletion.PartitionQueuePasswordReset)
	if len(colaRecuperacion) != 1 || stringAttrDe(colaRecuperacion[0], "userId") != e014OtroUser {
		t.Fatalf("solo debe sobrevivir la entrada de recuperacion del otro usuario: %v", colaRecuperacion)
	}
	// Cache invalidada con el prefijo del titular.
	prefijos := cache.registrados()
	if len(prefijos) != 1 || prefijos[0] != domainCache.UserCachePrefix("", e014Usuario) {
		t.Fatalf("prefijo de cache inesperado: %v", prefijos)
	}
}

// TestE014DescubreRunningVencido simula la caida del nodo propietario: el trabajo queda
// RUNNING con lease vencido y debe aparecer en el descubrimiento y ser reclamado.
func TestE014DescubreRunningVencido(t *testing.T) {
	m := newPurgaDynamo()
	e014SembrarCuenta(m, e014Usuario)
	store := dynamoAdapter.NewDeletionStore(m, e014Tabla)
	e014Programar(t, store, e014Usuario, e014RequestID, e014T0)

	control, resultado, err := store.Claim(e014Contexto(), e014Candidato(), "nodo-A", e014T0, 60*time.Second)
	if err != nil || resultado != appAccount.ClaimClaimed {
		t.Fatalf("primer claim debe reclamar: %v / %v", resultado, err)
	}
	if control.Fence != 1 {
		t.Fatalf("fence esperado 1, obtenido %d", control.Fence)
	}

	candidatos, _, err := store.Discover(e014Contexto(), "", e014T0.Add(30*time.Second), 50)
	if err != nil {
		t.Fatalf("descubrimiento fallo: %v", err)
	}
	if len(candidatos) != 0 {
		t.Fatalf("con lease vigente el trabajo no debe descubrirse: %v", candidatos)
	}

	candidatos, _, err = store.Discover(e014Contexto(), "", e014T0.Add(61*time.Second), 50)
	if err != nil {
		t.Fatalf("descubrimiento fallo: %v", err)
	}
	if len(candidatos) != 1 || candidatos[0].RequestID != e014RequestID {
		t.Fatalf("un RUNNING con lease vencido debe descubrirse: %v", candidatos)
	}

	controlB, resultado, err := store.Claim(e014Contexto(), candidatos[0], "nodo-B", e014T0.Add(61*time.Second), 60*time.Second)
	if err != nil || resultado != appAccount.ClaimClaimed {
		t.Fatalf("el segundo nodo debe reclamar: %v / %v", resultado, err)
	}
	if controlB.Fence != 2 || controlB.Owner != "nodo-B" {
		t.Fatalf("fence/owner esperados 2/nodo-B, obtenidos %d/%q", controlB.Fence, controlB.Owner)
	}
	if controlB.Phase != appAccount.PurgePhaseCollect {
		t.Fatalf("la reanudacion debe conservar la fase COLLECT, obtenida %q", controlB.Phase)
	}

	// El worker (otro propietario) completa el trabajo tras vencer el segundo lease.
	reloj := nuevoRelojPurga(e014T0.Add(3 * time.Minute))
	worker := e014Worker(store, &e014Cache{}, reloj)
	e014CorrerHastaPurga(t, worker, reloj, m, 20)
}

// TestE014PaginaSinCoincidenciasNoDetiene comprueba que una pagina sin coincidencias y con
// LastEvaluatedKey no detiene el recorrido.
func TestE014PaginaSinCoincidenciasNoDetiene(t *testing.T) {
	m := newPurgaDynamo()
	e014SembrarCuenta(m, e014Usuario)
	store := dynamoAdapter.NewDeletionStore(m, e014Tabla)
	e014Programar(t, store, e014Usuario, "zz-e014-job", e014T0)

	// Ocho entradas no elegibles que ordenan antes que el trabajo real.
	for i := 0; i < 8; i++ {
		m.sembrar("QUEUE#DELETION", "aa-0"+string(rune('0'+i)), map[string]types.AttributeValue{
			"entityType": strAttr(domainDeletion.EntityQueue),
			"requestId":  strAttr("aa-0" + string(rune('0'+i))),
			"userId":     strAttr(e014Usuario),
			"state":      strAttr(appAccount.PurgeStatePurged),
			"dueAt":      numAttr(e014T0.Add(-time.Hour).Unix()),
			"leaseUntil": numAttr(0),
		})
	}

	primera, siguiente, err := store.Discover(e014Contexto(), "", e014T0, 5)
	if err != nil {
		t.Fatalf("descubrimiento fallo: %v", err)
	}
	if len(primera) != 0 {
		t.Fatalf("la primera pagina no debe aportar coincidencias: %v", primera)
	}
	if siguiente == "" {
		t.Fatalf("una pagina sin coincidencias debe devolver LastEvaluatedKey")
	}
	segunda, _, err := store.Discover(e014Contexto(), siguiente, e014T0, 5)
	if err != nil {
		t.Fatalf("descubrimiento fallo: %v", err)
	}
	if len(segunda) != 1 || segunda[0].RequestID != "zz-e014-job" {
		t.Fatalf("el trabajo debe encontrarse en la segunda pagina: %v", segunda)
	}

	reloj := nuevoRelojPurga(e014T0)
	cache := &e014Cache{}
	worker := e014Worker(store, cache, reloj)
	worker.PageSize = 5
	if procesados := worker.Ciclo(e014Contexto()); procesados != 1 {
		t.Fatalf("el worker debe procesar el trabajo hallado tras la pagina vacia, proceso %d", procesados)
	}
}

// TestE014FencingPropietarioAnterior comprueba que el dueño anterior no puede latir, avanzar
// ni cerrar tras ser reclamado por otro nodo.
func TestE014FencingPropietarioAnterior(t *testing.T) {
	m := newPurgaDynamo()
	e014SembrarCuenta(m, e014Usuario)
	store := dynamoAdapter.NewDeletionStore(m, e014Tabla)
	e014Programar(t, store, e014Usuario, e014RequestID, e014T0)

	controlA, resultado, err := store.Claim(e014Contexto(), e014Candidato(), "nodo-A", e014T0, 60*time.Second)
	if err != nil || resultado != appAccount.ClaimClaimed {
		t.Fatalf("claim A fallo: %v / %v", resultado, err)
	}

	ahoraB := e014T0.Add(2 * time.Minute)
	controlB, resultado, err := store.Claim(e014Contexto(), e014Candidato(), "nodo-B", ahoraB, 60*time.Second)
	if err != nil || resultado != appAccount.ClaimClaimed {
		t.Fatalf("claim B (lease vencido) fallo: %v / %v", resultado, err)
	}
	if controlB.Fence != controlA.Fence+1 {
		t.Fatalf("el fence debe incrementarse: A=%d B=%d", controlA.Fence, controlB.Fence)
	}

	// Latido del dueño anterior: no se aplica y no cambia el lease.
	if ok, _ := store.Heartbeat(e014Contexto(), controlA, "nodo-A", controlA.Fence, ahoraB.Add(time.Minute), 60*time.Second); ok {
		t.Fatalf("el dueño anterior no puede latir")
	}
	trasLatido := e014Control(m, e014Usuario, e014RequestID)
	if lease, _ := attributeIntDe(trasLatido, "leaseUntil"); lease != ahoraB.Add(60*time.Second).Unix() {
		t.Fatalf("el lease debe seguir siendo el de B, obtenido %d", lease)
	}

	// Avance del dueño anterior: no se aplica y no toca fase ni cursor.
	if ok, _ := store.SaveProgress(e014Contexto(), controlA, "nodo-A", controlA.Fence, appAccount.PurgePhasePartition, "MOVEMENT#x", nil, 99, 0, ahoraB, 60*time.Second); ok {
		t.Fatalf("el dueño anterior no puede avanzar")
	}
	trasAvance := e014Control(m, e014Usuario, e014RequestID)
	if fase := stringAttrDe(trasAvance, "phase"); fase != appAccount.PurgePhaseCollect {
		t.Fatalf("la fase no puede cambiar por un dueño obsoleto: %q", fase)
	}
	if cursor := stringAttrDe(trasAvance, "lastSK"); cursor != "" {
		t.Fatalf("el cursor no puede cambiar por un dueño obsoleto: %q", cursor)
	}

	// Reprogramacion del dueño anterior: no se aplica y no cambia dueAt.
	if ok, _ := store.ScheduleRetry(e014Contexto(), controlA, "nodo-A", controlA.Fence, appAccount.PurgeStateRetry, ahoraB.Add(time.Hour), 0, ahoraB); ok {
		t.Fatalf("el dueño anterior no puede reprogramar")
	}
	trasRetry := e014Control(m, e014Usuario, e014RequestID)
	if estado := stringAttrDe(trasRetry, "state"); estado != appAccount.PurgeStateRunning {
		t.Fatalf("el estado debe seguir RUNNING: %q", estado)
	}
	cola := e014Cola(m, e014RequestID)
	if estado := stringAttrDe(cola, "state"); estado != appAccount.PurgeStateRunning {
		t.Fatalf("la cola debe seguir RUNNING: %q", estado)
	}

	// Cierre del dueño anterior: no se aplica.
	if ok, _ := store.Complete(e014Contexto(), controlA, "nodo-A", controlA.Fence, 0, ahoraB); ok {
		t.Fatalf("el dueño anterior no puede cerrar el trabajo")
	}
	if estado := stringAttrDe(e014Control(m, e014Usuario, e014RequestID), "state"); estado == appAccount.PurgeStatePurged {
		t.Fatalf("un dueño obsoleto no puede declarar PURGED")
	}

	// El dueño vigente sí puede cerrar.
	if ok, err := store.Complete(e014Contexto(), controlB, "nodo-B", controlB.Fence, 0, ahoraB); err != nil || !ok {
		t.Fatalf("el dueño vigente debe poder cerrar: ok=%v err=%v", ok, err)
	}
	if estado := stringAttrOf(e014Control(m, e014Usuario, e014RequestID), "state"); estado != appAccount.PurgeStatePurged {
		t.Fatalf("estado esperado PURGED, obtenido %q", estado)
	}
	if e014Cola(m, e014RequestID) != nil {
		t.Fatalf("el cierre debe borrar la entrada de cola")
	}
}

// TestE014PurgaParcialRetryNoPurged inyecta un fallo en EXTERNAL: el trabajo queda RETRY y
// nunca PURGED; al reintentar completa.
func TestE014PurgaParcialRetryNoPurged(t *testing.T) {
	m := newPurgaDynamo()
	e014SembrarCuenta(m, e014Usuario)
	store := dynamoAdapter.NewDeletionStore(m, e014Tabla)
	reloj := nuevoRelojPurga(e014T0)
	e014Programar(t, store, e014Usuario, e014RequestID, e014T0)

	m.fallarDelete = func(pk, sk string) error {
		if strings.HasPrefix(pk, domainDynamo.PrefixEmail) {
			return errors.New("dynamodb inaccesible")
		}
		return nil
	}

	cache := &e014Cache{}
	worker := e014Worker(store, cache, reloj)
	reloj.avanzar(time.Minute)
	worker.Ciclo(e014Contexto())

	control := e014Control(m, e014Usuario, e014RequestID)
	if estado := stringAttrOf(control, "state"); estado != appAccount.PurgeStateRetry {
		t.Fatalf("estado esperado RETRY, obtenido %q", estado)
	}
	if estado := stringAttrOf(e014Cola(m, e014RequestID), "state"); estado != appAccount.PurgeStateRetry {
		t.Fatalf("la cola debe reflejar RETRY, obtenida %q", estado)
	}
	if _, ok := attributeIntOf(control, "completedAt"); ok {
		t.Fatalf("un trabajo en RETRY no puede tener completedAt")
	}

	// Al desaparecer el fallo, completa.
	m.fallarDelete = nil
	e014CorrerHastaPurga(t, worker, reloj, m, 20)
	if estado := stringAttrOf(e014Control(m, e014Usuario, e014RequestID), "state"); estado != appAccount.PurgeStatePurged {
		t.Fatalf("tras el reintento debe quedar PURGED, obtenido %q", estado)
	}
}

// TestE014VerificacionBloqueaPurged demuestra que un ítem que sobrevive al borrado impide
// declarar PURGED.
func TestE014VerificacionBloqueaPurged(t *testing.T) {
	m := newPurgaDynamo()
	e014SembrarCuenta(m, e014Usuario)
	store := dynamoAdapter.NewDeletionStore(m, e014Tabla)
	reloj := nuevoRelojPurga(e014T0)
	e014Programar(t, store, e014Usuario, e014RequestID, e014T0)

	atorado := clavePurga(domainDeletion.UserPartition(e014Usuario), "MOVEMENT#2026-11-01#a")
	m.ignorarDelete[atorado] = true

	worker := e014Worker(store, &e014Cache{}, reloj)
	for ciclo := 0; ciclo < 6; ciclo++ {
		reloj.avanzar(time.Minute)
		worker.Ciclo(e014Contexto())
	}
	control := e014Control(m, e014Usuario, e014RequestID)
	if estado := stringAttrOf(control, "state"); estado == appAccount.PurgeStatePurged {
		t.Fatalf("no se puede declarar PURGED con un ítem pendiente")
	}
	if m.leer(domainDeletion.UserPartition(e014Usuario), "MOVEMENT#2026-11-01#a") == nil {
		t.Fatalf("el ítem atorado debe seguir existiendo para esta prueba")
	}

	m.ignorarDelete[atorado] = false
	e014CorrerHastaPurga(t, worker, reloj, m, 20)
	if estado := stringAttrOf(e014Control(m, e014Usuario, e014RequestID), "state"); estado != appAccount.PurgeStatePurged {
		t.Fatalf("al desaparecer el residuo debe completar, obtenido %q", estado)
	}
}

// TestE014ReintentosAgotadosSlowRetry comprueba que tras 8 fallos el trabajo pasa a
// SLOW_RETRY con 24 h y que un noveno intento lo vuelve a tomar.
func TestE014ReintentosAgotadosSlowRetry(t *testing.T) {
	m := newPurgaDynamo()
	e014SembrarCuenta(m, e014Usuario)
	store := dynamoAdapter.NewDeletionStore(m, e014Tabla)
	reloj := nuevoRelojPurga(e014T0)
	e014Programar(t, store, e014Usuario, e014RequestID, e014T0)

	m.fallarDelete = func(pk, sk string) error {
		if strings.HasPrefix(pk, domainDynamo.PrefixEmail) {
			return errors.New("dynamodb inaccesible")
		}
		return nil
	}
	worker := e014Worker(store, &e014Cache{}, reloj)

	for intento := 1; intento <= 8; intento++ {
		worker.Ciclo(e014Contexto())
		if intento < 8 {
			reloj.avanzar(7 * time.Hour)
		}
	}

	control := e014Control(m, e014Usuario, e014RequestID)
	if estado := stringAttrOf(control, "state"); estado != appAccount.PurgeStateSlowRetry {
		t.Fatalf("tras 8 intentos se espera SLOW_RETRY, obtenido %q", estado)
	}
	if intentos, _ := attributeIntOf(control, "attempts"); intentos != 8 {
		t.Fatalf("attempts esperado 8, obtenido %d", intentos)
	}
	due, _ := attributeIntOf(control, "dueAt")
	if due < reloj.ahora().Add(23*time.Hour).Unix() {
		t.Fatalf("SLOW_RETRY debe reprogramarse a ~24 h, dueAt=%d", due)
	}

	// Un noveno intento lo vuelve a tomar: no queda abandonado.
	reloj.avanzar(24*time.Hour + time.Minute)
	candidatos, _, err := store.Discover(e014Contexto(), "", reloj.ahora(), 50)
	if err != nil || len(candidatos) != 1 {
		t.Fatalf("SLOW_RETRY debe ser descubrible: %v / %v", candidatos, err)
	}
	reclamado, resultado, err := store.Claim(e014Contexto(), candidatos[0], "nodo-C", reloj.ahora(), 60*time.Second)
	if err != nil || resultado != appAccount.ClaimClaimed {
		t.Fatalf("SLOW_RETRY debe ser reclamable: %v / %v", resultado, err)
	}
	if reclamado.Fence != 9 {
		t.Fatalf("fence esperado 9, obtenido %d", reclamado.Fence)
	}

	// Al cesar el fallo, completa.
	m.fallarDelete = nil
	e014CorrerHastaPurga(t, worker, reloj, m, 20)
}

// TestE014DueAtMismaEntradaCola comprueba que reprogramar actualiza la MISMA entrada de cola
// (SK=requestId) sin duplicados.
func TestE014DueAtMismaEntradaCola(t *testing.T) {
	m := newPurgaDynamo()
	e014SembrarCuenta(m, e014Usuario)
	store := dynamoAdapter.NewDeletionStore(m, e014Tabla)
	reloj := nuevoRelojPurga(e014T0)
	e014Programar(t, store, e014Usuario, e014RequestID, e014T0)

	m.fallarDelete = func(pk, sk string) error {
		if strings.HasPrefix(pk, domainDynamo.PrefixEmail) {
			return errors.New("dynamodb inaccesible")
		}
		return nil
	}
	worker := e014Worker(store, &e014Cache{}, reloj)
	reloj.avanzar(time.Minute)
	worker.Ciclo(e014Contexto())

	cola := m.porParticion(domainDeletion.PartitionQueueDeletion)
	if len(cola) != 1 {
		t.Fatalf("la cola debe tener exactamente una entrada, tiene %d", len(cola))
	}
	if sk := stringAttrOf(cola[0], "SK"); sk != e014RequestID {
		t.Fatalf("la SK de la cola debe ser el requestId, obtenida %q", sk)
	}
	control := e014Control(m, e014Usuario, e014RequestID)
	dueControl, _ := attributeIntOf(control, "dueAt")
	dueCola, _ := attributeIntOf(cola[0], "dueAt")
	if dueControl != dueCola {
		t.Fatalf("dueAt debe coincidir entre control y cola: %d vs %d", dueControl, dueCola)
	}

	// Al completar, la entrada se borra y no reaparece.
	m.fallarDelete = nil
	e014CorrerHastaPurga(t, worker, reloj, m, 20)
	if restantes := m.porParticion(domainDeletion.PartitionQueueDeletion); len(restantes) != 0 {
		t.Fatalf("la cola debe quedar vacia al completar: %v", restantes)
	}
}

// TestE014DosWorkersConcurrentes garantiza que solo uno reclama y que no se duplican borrados.
func TestE014DosWorkersConcurrentes(t *testing.T) {
	m := newPurgaDynamo()
	esperadas := e014SembrarCuenta(m, e014Usuario)
	store := dynamoAdapter.NewDeletionStore(m, e014Tabla)
	reloj := nuevoRelojPurga(e014T0)
	e014Programar(t, store, e014Usuario, e014RequestID, e014T0)

	workerA := e014Worker(store, &e014Cache{}, reloj)
	workerB := e014Worker(store, &e014Cache{}, reloj)

	var wg sync.WaitGroup
	resultados := make(chan int, 2)
	wg.Add(2)
	go func() { defer wg.Done(); resultados <- workerA.Ciclo(e014Contexto()) }()
	go func() { defer wg.Done(); resultados <- workerB.Ciclo(e014Contexto()) }()
	wg.Wait()
	close(resultados)

	total := 0
	for procesados := range resultados {
		total += procesados
	}
	if total != 1 {
		t.Fatalf("solo un worker debe procesar el trabajo, total=%d", total)
	}
	if m.claimsContados() != 1 {
		t.Fatalf("solo debe existir una reclamacion, claims=%d", m.claimsContados())
	}

	e014CorrerHastaPurga(t, workerA, reloj, m, 20)
	if borrados, _ := attributeIntOf(e014Control(m, e014Usuario, e014RequestID), "deletedCount"); borrados != int64(len(esperadas)) {
		t.Fatalf("deletedCount esperado %d sin duplicados, obtenido %d", len(esperadas), borrados)
	}
}

// TestE014ReanudacionTrasReinicio simula una caida a mitad de la purga: otro nodo retoma
// desde la fase y el cursor persistidos sin perder registros.
func TestE014ReanudacionTrasReinicio(t *testing.T) {
	m := newPurgaDynamo()
	esperadas := e014SembrarCuenta(m, e014Usuario)
	store := dynamoAdapter.NewDeletionStore(m, e014Tabla)
	reloj := nuevoRelojPurga(e014T0)
	e014Programar(t, store, e014Usuario, e014RequestID, e014T0)

	// Nodo A: un solo paso (COLLECT) y "cae".
	workerA := e014Worker(store, &e014Cache{}, reloj)
	workerA.MaxBatches = 1
	reloj.avanzar(time.Minute)
	workerA.Ciclo(e014Contexto())

	control := e014Control(m, e014Usuario, e014RequestID)
	if fase := stringAttrOf(control, "phase"); fase != appAccount.PurgePhaseExternal {
		t.Fatalf("tras COLLECT la fase debe ser EXTERNAL, obtenida %q", fase)
	}
	if estado := stringAttrOf(control, "state"); estado != appAccount.PurgeStateRunning {
		t.Fatalf("el trabajo debe quedar RUNNING tras la caida, obtenido %q", estado)
	}

	// Nodo B: nueva instancia, lease vencido y reanudacion hasta el final.
	reloj.avanzar(2 * time.Minute)
	workerB := e014Worker(store, &e014Cache{}, reloj)
	e014CorrerHastaPurga(t, workerB, reloj, m, 20)

	if borrados, _ := attributeIntOf(e014Control(m, e014Usuario, e014RequestID), "deletedCount"); borrados != int64(len(esperadas)) {
		t.Fatalf("no se puede perder ningun registro: esperados %d, borrados %d", len(esperadas), borrados)
	}
	for _, sk := range esperadas {
		if m.leer(domainDeletion.UserPartition(e014Usuario), sk) != nil {
			t.Fatalf("el registro %q sobrevivio a la purga", sk)
		}
	}
}

// ---------- aliases locales para no colisionar con otros archivos del paquete ----------

func stringAttrOf(item map[string]types.AttributeValue, nombre string) string {
	return stringAttrDe(item, nombre)
}

func attributeIntOf(item map[string]types.AttributeValue, nombre string) (int64, bool) {
	return attributeIntDe(item, nombre)
}
