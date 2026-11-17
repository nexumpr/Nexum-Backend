package dynamo_test

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	dynamoAdapter "github.com/nexumpr/Nexum-Backend/internal/adapters/dynamo"
	appIdentity "github.com/nexumpr/Nexum-Backend/internal/application/identity"
	domainDynamo "github.com/nexumpr/Nexum-Backend/internal/domain/dynamo"
)

// Pruebas de E008 sobre el adaptador REAL `SessionsStore` montado sobre un doble con estado que
// interpreta la misma condición de clave que emite. Demuestran que el listado es un `Query`
// particionado (nunca `Scan`), que la paginación usa `ExclusiveStartKey` y que cada fila se
// reduce a los campos que el contrato permite.

const e008Tabla = "NEXUM_CORE_TEST"

const (
	e008UserA   = "11111111-2222-4333-8444-555555555555"
	e008UserB   = "99999999-8888-4777-8666-555555555555"
	e008ID1     = "018f90ef-08e5-7a68-b09f-0000000000a1"
	e008ID2     = "018f90ef-08e5-7a68-b09f-0000000000b2"
	e008ID3     = "018f90ef-08e5-7a68-b09f-0000000000c3"
	e008IDAjena = "018f90ef-08e5-7a68-b09f-0000000000d4"
)

var e008Creada = time.Date(2026, 11, 17, 9, 0, 0, 0, time.UTC)

// memoriaConsulta envuelve el doble con estado existente y añade `Query`. El doble base
// devuelve error para `Query` a propósito; aquí se implementa exactamente la gramática que
// emite `SessionsStore` y se falla si aparece cualquier otra condición.
type memoriaConsulta struct {
	*memoriaDynamo

	consultas          int
	ultimaCondicion    string
	ultimoStart        string
	ultimoLimite       int
	lecturaConsistente bool
}

func newMemoriaConsulta() *memoriaConsulta {
	return &memoriaConsulta{memoriaDynamo: newMemoriaDynamo()}
}

func (m *memoriaConsulta) Query(_ context.Context, params *dynamodb.QueryInput, _ ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
	condicion := aws.ToString(params.KeyConditionExpression)
	if condicion != "PK = :pk AND begins_with(SK, :prefix)" {
		return nil, fmt.Errorf("condicion de clave no soportada por el doble: %q", condicion)
	}
	pk := mapaString(params.ExpressionAttributeValues, ":pk")
	prefijo := mapaString(params.ExpressionAttributeValues, ":prefix")
	start := ""
	if params.ExclusiveStartKey != nil {
		start = mapaString(params.ExclusiveStartKey, "SK")
	}
	limite := int(aws.ToInt32(params.Limit))

	m.mu.Lock()
	defer m.mu.Unlock()
	m.consultas++
	m.ultimaCondicion = condicion
	m.ultimoStart = start
	m.ultimoLimite = limite
	m.lecturaConsistente = aws.ToBool(params.ConsistentRead)

	var claves []string
	for clave := range m.items {
		pkItem, skItem, _ := strings.Cut(clave, "\x00")
		if pkItem != pk || !strings.HasPrefix(skItem, prefijo) {
			continue
		}
		if start != "" && skItem <= start {
			continue
		}
		claves = append(claves, skItem)
	}
	sort.Strings(claves)

	salida := &dynamodb.QueryOutput{}
	for indice, sk := range claves {
		if limite > 0 && indice >= limite {
			salida.LastEvaluatedKey = map[string]types.AttributeValue{
				"PK": strAttr(pk),
				"SK": strAttr(claves[indice-1]),
			}
			break
		}
		salida.Items = append(salida.Items, copiarItem(m.items[claveDe(pk, sk)]))
	}
	return salida, nil
}

// e008CrearSesion escribe sesión y localizador con el adaptador real de E002: los nombres de
// atributo (`createdAt`, `lastUsedAt`, `status`, `entityType`) son los de producción.
func e008CrearSesion(t *testing.T, m *memoriaConsulta, userID, sessionID, refresh string, creada time.Time) {
	t.Helper()
	store := dynamoAdapter.NewSessionStore(m, e008Tabla)
	err := store.CreateSession(context.Background(), appIdentity.SessionRecord{
		UserID:      userID,
		SessionID:   sessionID,
		FamilyID:    "familia-" + sessionID,
		RefreshHash: appIdentity.HashToken(refresh),
		AccessJTI:   "jti-" + sessionID,
		CreatedAt:   creada,
		ExpiresAt:   creada.Add(30 * 24 * time.Hour),
		Status:      appIdentity.StatusActive,
	})
	if err != nil {
		t.Fatalf("no se pudo sembrar la sesion %s: %v", sessionID, err)
	}
}

// e008ModificarSesion reescribe atributos concretos de una sesión ya creada.
func e008ModificarSesion(m *memoriaConsulta, userID, sessionID string, cambios map[string]types.AttributeValue) {
	pk, sk := domainDynamo.UserSessionKeys(userID, sessionID)
	item := m.leer(pk, sk)
	for nombre, valor := range cambios {
		if valor == nil {
			delete(item, nombre)
			continue
		}
		item[nombre] = valor
	}
	m.sembrar(pk, sk, item)
}

// TestE008StoreConsultaParticionadaSinScan cubre el requisito central: el listado se resuelve
// con `Query` sobre `PK = USER#<id>` y `begins_with(SK, "SESSION#")`, ordenado y paginado por
// `ExclusiveStartKey`. Se verifica además que la interfaz del adaptador no expone `Scan`.
func TestE008StoreConsultaParticionadaSinScan(t *testing.T) {
	m := newMemoriaConsulta()
	e008CrearSesion(t, m, e008UserA, e008ID1, "refresh-1", e008Creada)
	e008CrearSesion(t, m, e008UserA, e008ID2, "refresh-2", e008Creada.Add(time.Minute))
	e008CrearSesion(t, m, e008UserA, e008ID3, "refresh-3", e008Creada.Add(2*time.Minute))

	// Ruido en la MISMA partición: perfil y movimiento no son sesiones.
	pk, _ := domainDynamo.UserKeys(e008UserA)
	m.sembrar(pk, domainDynamo.PrefixProfile, map[string]types.AttributeValue{
		"entityType": strAttr("USER_PROFILE"),
		"userId":     strAttr(e008UserA),
		"status":     strAttr(appIdentity.StatusActive),
	})
	m.sembrar(pk, "MOVEMENT#2026-11-17T09:00:00.000Z#m1", map[string]types.AttributeValue{
		"entityType": strAttr("MOVEMENT"),
		"userId":     strAttr(e008UserA),
	})

	store := dynamoAdapter.NewSessionsStore(m, e008Tabla)
	primera, nextSK, err := store.ListUserSessions(context.Background(), e008UserA, "", 2)
	if err != nil {
		t.Fatalf("primera pagina fallo: %v", err)
	}
	if len(primera) != 2 {
		t.Fatalf("se esperaban 2 sesiones, hay %d", len(primera))
	}
	if primera[0].SessionID != e008ID1 || primera[1].SessionID != e008ID2 {
		t.Fatalf("orden ascendente por SK esperado (a1, b2): %+v", primera)
	}
	if nextSK != "SESSION#"+e008ID2 {
		t.Fatalf("el cursor debe apuntar a la SK del ultimo devuelto, hay %q", nextSK)
	}
	if m.consultas != 1 || !m.lecturaConsistente {
		t.Fatalf("debe hacerse un Query con lectura consistente: consultas=%d consistente=%v", m.consultas, m.lecturaConsistente)
	}
	if !strings.Contains(m.ultimaCondicion, "begins_with(SK, :prefix)") {
		t.Fatalf("la condicion debe restringir por prefijo de SK: %q", m.ultimaCondicion)
	}
	if m.ultimoLimite != 3 {
		t.Fatalf("debe pedirse limit+1 para saber si hay mas, se pidio %d", m.ultimoLimite)
	}

	segunda, nextSK2, err := store.ListUserSessions(context.Background(), e008UserA, nextSK, 2)
	if err != nil {
		t.Fatalf("segunda pagina fallo: %v", err)
	}
	if len(segunda) != 1 || segunda[0].SessionID != e008ID3 {
		t.Fatalf("la segunda pagina debe contener solo c3: %+v", segunda)
	}
	if nextSK2 != "" {
		t.Fatalf("sin mas filas no debe emitirse cursor, hay %q", nextSK2)
	}
	if m.ultimoStart != "SESSION#"+e008ID2 {
		t.Fatalf("el cursor debe llegar como ExclusiveStartKey, se recibio %q", m.ultimoStart)
	}

	// Ninguna ruta del adaptador puede usar Scan: ni el puerto ni la interfaz del cliente lo
	// declaran, así que el compilador lo impide. Se comprueba de forma explícita.
	tipo := reflect.TypeOf((*dynamoAdapter.SessionsAPI)(nil)).Elem()
	for i := 0; i < tipo.NumMethod(); i++ {
		if strings.EqualFold(tipo.Method(i).Name, "Scan") {
			t.Fatalf("la interfaz del almacen de sesiones no debe exponer Scan")
		}
	}
}

// TestE008StoreAislaUsuarios cubre T03/T05: la consulta sólo alcanza la partición del titular.
func TestE008StoreAislaUsuarios(t *testing.T) {
	m := newMemoriaConsulta()
	e008CrearSesion(t, m, e008UserA, e008ID1, "refresh-a", e008Creada)
	e008CrearSesion(t, m, e008UserB, e008IDAjena, "refresh-b", e008Creada)

	store := dynamoAdapter.NewSessionsStore(m, e008Tabla)
	sesiones, _, err := store.ListUserSessions(context.Background(), e008UserA, "", 25)
	if err != nil {
		t.Fatalf("listado fallo: %v", err)
	}
	if len(sesiones) != 1 || sesiones[0].SessionID != e008ID1 {
		t.Fatalf("solo debe devolverse la sesion del titular: %+v", sesiones)
	}
	for _, sesion := range sesiones {
		if sesion.SessionID == e008IDAjena {
			t.Fatal("una sesion de otro usuario no puede aparecer")
		}
	}
}

// TestE008StoreMapeaLastUsedAtYEstado: `lastSeenAt` procede de `lastUsedAt` y `revoked` del
// estado real. Si `lastUsedAt` falta, se usa `createdAt`.
func TestE008StoreMapeaLastUsedAtYEstado(t *testing.T) {
	m := newMemoriaConsulta()
	e008CrearSesion(t, m, e008UserA, e008ID1, "refresh-1", e008Creada)
	e008CrearSesion(t, m, e008UserA, e008ID2, "refresh-2", e008Creada)

	ultimoUso := e008Creada.Add(90 * time.Minute)
	e008ModificarSesion(m, e008UserA, e008ID1, map[string]types.AttributeValue{
		"lastUsedAt": strAttr(ultimoUso.Format(time.RFC3339)),
		"status":     strAttr(appIdentity.StatusRevoked),
	})
	e008ModificarSesion(m, e008UserA, e008ID2, map[string]types.AttributeValue{
		"lastUsedAt": nil,
	})

	store := dynamoAdapter.NewSessionsStore(m, e008Tabla)
	sesiones, _, err := store.ListUserSessions(context.Background(), e008UserA, "", 25)
	if err != nil {
		t.Fatalf("listado fallo: %v", err)
	}
	if len(sesiones) != 2 {
		t.Fatalf("se esperaban 2 sesiones, hay %d", len(sesiones))
	}
	if !sesiones[0].LastSeenAt.Equal(ultimoUso) {
		t.Fatalf("lastSeenAt debe venir de lastUsedAt: %v", sesiones[0].LastSeenAt)
	}
	if sesiones[0].Status != appIdentity.StatusRevoked {
		t.Fatalf("el estado real debe propagarse: %q", sesiones[0].Status)
	}
	if !sesiones[1].LastSeenAt.Equal(sesiones[1].CreatedAt) {
		t.Fatalf("sin lastUsedAt, lastSeenAt debe ser createdAt: %v vs %v", sesiones[1].LastSeenAt, sesiones[1].CreatedAt)
	}
}

// TestE008StoreIgnoraEntidadAjena: un ítem con la clave de sesión pero otra entidad no se
// presenta como sesión.
func TestE008StoreIgnoraEntidadAjena(t *testing.T) {
	m := newMemoriaConsulta()
	pk, sk := domainDynamo.UserSessionKeys(e008UserA, e008ID1)
	m.sembrar(pk, sk, map[string]types.AttributeValue{
		"entityType": strAttr("OTHER_ENTITY"),
		"userId":     strAttr(e008UserA),
		"sessionId":  strAttr(e008ID1),
		"createdAt":  strAttr(e008Creada.Format(time.RFC3339)),
	})

	store := dynamoAdapter.NewSessionsStore(m, e008Tabla)
	sesiones, _, err := store.ListUserSessions(context.Background(), e008UserA, "", 25)
	if err != nil {
		t.Fatalf("listado fallo: %v", err)
	}
	if len(sesiones) != 0 {
		t.Fatalf("una entidad distinta de USER_SESSION no es una sesion: %+v", sesiones)
	}
}

// TestE008StoreFallaCerradoSinClienteNiTabla.
func TestE008StoreFallaCerradoSinClienteNiTabla(t *testing.T) {
	if _, _, err := dynamoAdapter.NewSessionsStore(nil, "").ListUserSessions(context.Background(), e008UserA, "", 25); err == nil {
		t.Fatal("sin cliente debe fallar cerrado")
	}
	m := newMemoriaConsulta()
	if _, _, err := dynamoAdapter.NewSessionsStore(m, "").ListUserSessions(context.Background(), e008UserA, "", 25); err == nil {
		t.Fatal("sin tabla debe fallar cerrado")
	}
	if _, _, err := dynamoAdapter.NewSessionsStore(m, e008Tabla).ListUserSessions(context.Background(), e008UserA, "", 0); err == nil {
		t.Fatal("un limite invalido debe fallar cerrado")
	}
}
