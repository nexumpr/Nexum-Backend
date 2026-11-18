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
	appPrivacy "github.com/nexumpr/Nexum-Backend/internal/application/privacy"
	domainDynamo "github.com/nexumpr/Nexum-Backend/internal/domain/dynamo"
)

// Pruebas de E016 sobre el adaptador REAL `ConsentsStore` montado sobre un doble con estado que
// interpreta la misma condición de clave que emite. Demuestran que el listado es un `Query`
// particionado descendente (nunca `Scan`), que la paginación usa `ExclusiveStartKey` y que los
// consentimientos históricos de E001 se proyectan sin reescribirse.

const e016Tabla = "NEXUM_CORE_TEST"

const (
	e016UserA   = "11111111-2222-4333-8444-555555555555"
	e016UserB   = "99999999-8888-4777-8666-555555555555"
	e016IDA     = "018f90ef-08e5-7a68-b09f-0000000000a1"
	e016IDB     = "018f90ef-08e5-7a68-b09f-0000000000b2"
	e016IDC     = "018f90ef-08e5-7a68-b09f-0000000000c3"
	e016IDAjena = "018f90ef-08e5-7a68-b09f-0000000000d4"
)

var e016Base = time.Date(2026, 11, 18, 9, 0, 0, 0, time.UTC)

// memoriaConsentimientos amplía el doble con estado existente con la única operación de lectura
// que necesita E016. Falla si aparece una condición distinta de la que emite el adaptador.
type memoriaConsentimientos struct {
	*memoriaDynamo

	consultas          int
	ultimaCondicion    string
	ultimosValores     map[string]types.AttributeValue
	ultimoStart        string
	ultimoLimite       int
	descendente        bool
	lecturaConsistente bool
}

func newMemoriaConsentimientos() *memoriaConsentimientos {
	return &memoriaConsentimientos{memoriaDynamo: newMemoriaDynamo()}
}

func (m *memoriaConsentimientos) Query(_ context.Context, params *dynamodb.QueryInput, _ ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
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
	descendente := !aws.ToBool(params.ScanIndexForward)

	m.mu.Lock()
	defer m.mu.Unlock()
	m.consultas++
	m.ultimaCondicion = condicion
	m.ultimosValores = params.ExpressionAttributeValues
	m.ultimoStart = start
	m.ultimoLimite = limite
	m.descendente = descendente
	m.lecturaConsistente = aws.ToBool(params.ConsistentRead)

	var claves []string
	for clave := range m.items {
		pkItem, skItem, _ := strings.Cut(clave, "\x00")
		if pkItem != pk || !strings.HasPrefix(skItem, prefijo) {
			continue
		}
		if start != "" {
			// En orden descendente, la consulta continúa con las claves estrictamente menores
			// que ExclusiveStartKey.
			if descendente && skItem >= start {
				continue
			}
			if !descendente && skItem <= start {
				continue
			}
		}
		claves = append(claves, skItem)
	}
	sort.Strings(claves)
	if descendente {
		for i, j := 0, len(claves)-1; i < j; i, j = i+1, j-1 {
			claves[i], claves[j] = claves[j], claves[i]
		}
	}

	salida := &dynamodb.QueryOutput{}
	for indice, sk := range claves {
		if limite > 0 && indice >= limite {
			break
		}
		salida.Items = append(salida.Items, copiarItem(m.items[claveDe(pk, sk)]))
	}
	return salida, nil
}

// e016SembrarNuevo escribe un consentimiento con el formato de E017 directamente en el doble.
func e016SembrarNuevo(m *memoriaConsentimientos, userID, id, noticeVersion, purpose string, granted bool, grabado time.Time) {
	sk := domainDynamo.PrefixConsent + grabado.UTC().Format("2006-01-02T15:04:05.000000000Z") + "#" + id
	pk := domainDynamo.PrefixUser + userID
	m.sembrar(pk, sk, map[string]types.AttributeValue{
		"id":            strAttr(id),
		"noticeVersion": strAttr(noticeVersion),
		"purpose":       strAttr(purpose),
		"granted":       &types.AttributeValueMemberBOOL{Value: granted},
		"recordedAt":    strAttr(grabado.Format(time.RFC3339Nano)),
		"source":        strAttr("api"),
		"createdAt":     strAttr(grabado.Format(time.RFC3339Nano)),
	})
}

// e016SembrarHeredado escribe un consentimiento con el formato de E001 (`CONSENT#<version>`).
func e016SembrarHeredado(m *memoriaConsentimientos, userID, version string, acceptedAt types.AttributeValue) {
	pk := domainDynamo.PrefixUser + userID
	m.sembrar(pk, domainDynamo.PrefixConsent+version, map[string]types.AttributeValue{
		"acceptedAt": acceptedAt,
	})
}

func e016EsUUID(valor string) bool {
	partes := strings.Split(valor, "-")
	if len(partes) != 5 {
		return false
	}
	longitudes := []int{8, 4, 4, 4, 12}
	for i, parte := range partes {
		if len(parte) != longitudes[i] {
			return false
		}
		for _, r := range parte {
			if !strings.ContainsRune("0123456789abcdef", r) {
				return false
			}
		}
	}
	return true
}

// TestE016StoreConsultaParticionadaDescendenteSinScan cubre el requisito central: el listado se
// resuelve con `Query` sobre `PK = USER#<id>` y `begins_with(SK, "CONSENT#")`, en orden
// descendente y paginado por `ExclusiveStartKey`. Se verifica además que la interfaz no expone
// `Scan`.
func TestE016StoreConsultaParticionadaDescendenteSinScan(t *testing.T) {
	m := newMemoriaConsentimientos()
	e016SembrarNuevo(m, e016UserA, e016IDA, "2026-11", appPrivacy.ConsentPurposeNotifications, true, e016Base.Add(-3*time.Hour))
	e016SembrarNuevo(m, e016UserA, e016IDB, "2026-11", appPrivacy.ConsentPurposeAnalytics, false, e016Base.Add(-2*time.Hour))
	e016SembrarNuevo(m, e016UserA, e016IDC, "2026-12", appPrivacy.ConsentPurposePrivacyNotice, true, e016Base.Add(-time.Hour))
	e016SembrarHeredado(m, e016UserA, "2026-09", strAttr(e016Base.Add(-96*time.Hour).Format(time.RFC3339)))

	// Ruido en la MISMA partición: perfil y movimiento no son consentimientos.
	pk := domainDynamo.PrefixUser + e016UserA
	m.sembrar(pk, domainDynamo.PrefixProfile, map[string]types.AttributeValue{
		"entityType": strAttr("USER_PROFILE"),
		"userId":     strAttr(e016UserA),
	})
	m.sembrar(pk, "MOVEMENT#2026-11-18T09:00:00.000Z#m1", map[string]types.AttributeValue{
		"entityType": strAttr("MOVEMENT"),
		"userId":     strAttr(e016UserA),
	})

	store := dynamoAdapter.NewConsentsStore(m, e016Tabla)
	primera, nextSK, err := store.ListUserConsents(context.Background(), e016UserA, "", 2)
	if err != nil {
		t.Fatalf("primera pagina fallo: %v", err)
	}
	if len(primera) != 2 {
		t.Fatalf("se esperaban 2 consentimientos, hay %d", len(primera))
	}
	if primera[0].ID != e016IDC || primera[1].ID != e016IDB {
		t.Fatalf("orden descendente esperado (c3, b2): %+v", primera)
	}
	if nextSK == "" {
		t.Fatal("con mas filas debe devolverse la SK de continuacion")
	}
	if m.consultas != 1 || !m.lecturaConsistente || !m.descendente {
		t.Fatalf("debe hacerse un Query descendente con lectura consistente: consultas=%d consistente=%v descendente=%v",
			m.consultas, m.lecturaConsistente, m.descendente)
	}
	if m.ultimoLimite != 3 {
		t.Fatalf("debe pedirse limit+1 para saber si hay mas, se pidio %d", m.ultimoLimite)
	}

	segunda, nextSK2, err := store.ListUserConsents(context.Background(), e016UserA, nextSK, 2)
	if err != nil {
		t.Fatalf("segunda pagina fallo: %v", err)
	}
	if len(segunda) != 2 {
		t.Fatalf("la segunda pagina debe traer a1 y el item heredado, trae %d: %+v", len(segunda), segunda)
	}
	if segunda[0].ID != e016IDA || segunda[1].NoticeVersion != "2026-09" {
		t.Fatalf("orden descendente esperado (a1, heredado): %+v", segunda)
	}
	if nextSK2 != "" {
		t.Fatalf("sin mas filas no debe devolverse continuacion, hay %q", nextSK2)
	}
	if m.ultimoStart != nextSK {
		t.Fatalf("el cursor debe llegar como ExclusiveStartKey, se recibio %q", m.ultimoStart)
	}

	tipo := reflect.TypeOf((*dynamoAdapter.ConsentsAPI)(nil)).Elem()
	for i := 0; i < tipo.NumMethod(); i++ {
		if strings.EqualFold(tipo.Method(i).Name, "Scan") {
			t.Fatalf("la interfaz del almacen de consentimientos no debe exponer Scan")
		}
	}
}

// TestE016StoreAislaUsuarios: la consulta sólo alcanza la partición del titular.
func TestE016StoreAislaUsuarios(t *testing.T) {
	m := newMemoriaConsentimientos()
	e016SembrarNuevo(m, e016UserA, e016IDA, "2026-11", appPrivacy.ConsentPurposeNotifications, true, e016Base)
	e016SembrarNuevo(m, e016UserB, e016IDAjena, "2026-11", appPrivacy.ConsentPurposeAnalytics, true, e016Base)

	store := dynamoAdapter.NewConsentsStore(m, e016Tabla)
	consentimientos, _, err := store.ListUserConsents(context.Background(), e016UserA, "", 25)
	if err != nil {
		t.Fatalf("listado fallo: %v", err)
	}
	if len(consentimientos) != 1 || consentimientos[0].ID != e016IDA {
		t.Fatalf("solo debe devolverse el consentimiento del titular: %+v", consentimientos)
	}
	for _, consentimiento := range consentimientos {
		if consentimiento.ID == e016IDAjena {
			t.Fatal("un consentimiento de otro usuario no puede aparecer")
		}
	}
}

// TestE016StoreProyectaItemHeredado: `CONSENT#<version>` de E001 se lee sin modificarse y se
// proyecta a `purpose=privacyNotice`, `granted=true`, `recordedAt=acceptedAt` e `id`
// determinista en forma de UUID.
func TestE016StoreProyectaItemHeredado(t *testing.T) {
	m := newMemoriaConsentimientos()
	aceptado := e016Base.Add(-72 * time.Hour).Truncate(time.Second)
	e016SembrarHeredado(m, e016UserA, "2026-11", strAttr(aceptado.Format(time.RFC3339)))

	store := dynamoAdapter.NewConsentsStore(m, e016Tabla)
	primera, _, err := store.ListUserConsents(context.Background(), e016UserA, "", 25)
	if err != nil {
		t.Fatalf("listado fallo: %v", err)
	}
	if len(primera) != 1 {
		t.Fatalf("se esperaba un consentimiento heredado, hay %d", len(primera))
	}
	record := primera[0]
	if record.Purpose != appPrivacy.ConsentPurposePrivacyNotice || !record.Granted {
		t.Fatalf("proyeccion heredada incorrecta: %+v", record)
	}
	if record.NoticeVersion != "2026-11" {
		t.Fatalf("la version debe salir de la SK heredada: %q", record.NoticeVersion)
	}
	if !record.RecordedAt.Equal(aceptado) {
		t.Fatalf("recordedAt debe ser acceptedAt: %v vs %v", record.RecordedAt, aceptado)
	}
	if !e016EsUUID(record.ID) {
		t.Fatalf("el id heredado debe tener forma de UUID: %q", record.ID)
	}

	segunda, _, err := store.ListUserConsents(context.Background(), e016UserA, "", 25)
	if err != nil {
		t.Fatalf("segundo listado fallo: %v", err)
	}
	if segunda[0].ID != record.ID {
		t.Fatalf("el id heredado debe ser determinista: %q vs %q", record.ID, segunda[0].ID)
	}
	if otra, _, err := store.ListUserConsents(context.Background(), e016UserB, "", 25); err != nil || len(otra) != 0 {
		t.Fatalf("otro usuario no debe ver el item heredado: %+v (%v)", otra, err)
	}
}

// TestE016StoreProyectaAcceptedAtNumerico: el ítem heredado puede haberse escrito con epoch en
// segundos (misma convención que otros atributos temporales del backend).
func TestE016StoreProyectaAcceptedAtNumerico(t *testing.T) {
	m := newMemoriaConsentimientos()
	aceptado := e016Base.Add(-48 * time.Hour).Truncate(time.Second)
	e016SembrarHeredado(m, e016UserA, "2026-10", numAttr(aceptado.Unix()))

	store := dynamoAdapter.NewConsentsStore(m, e016Tabla)
	primera, _, err := store.ListUserConsents(context.Background(), e016UserA, "", 25)
	if err != nil {
		t.Fatalf("listado fallo: %v", err)
	}
	if len(primera) != 1 || !primera[0].RecordedAt.Equal(aceptado) {
		t.Fatalf("acceptedAt numerico debe leerse como epoch: %+v", primera)
	}
}

// TestE016StoreOmiteItemsSinDatosMinimos: un ítem incompleto no se presenta como consentimiento
// ni rellena huecos con valores inventados.
func TestE016StoreOmiteItemsSinDatosMinimos(t *testing.T) {
	m := newMemoriaConsentimientos()
	pk := domainDynamo.PrefixUser + e016UserA
	// Nuevo sin `granted`.
	m.sembrar(pk, "CONSENT#2026-11-18T09:00:00.000000000Z#"+e016IDA, map[string]types.AttributeValue{
		"id":            strAttr(e016IDA),
		"noticeVersion": strAttr("2026-11"),
		"purpose":       strAttr(appPrivacy.ConsentPurposeNotifications),
		"recordedAt":    strAttr(e016Base.Format(time.RFC3339Nano)),
	})
	// Heredado sin `acceptedAt`.
	e016SembrarHeredado(m, e016UserA, "2026-09", nil)
	// Nuevo con propósito fuera del enum.
	e016SembrarNuevo(m, e016UserA, e016IDB, "2026-11", "marketing", true, e016Base)

	store := dynamoAdapter.NewConsentsStore(m, e016Tabla)
	primera, _, err := store.ListUserConsents(context.Background(), e016UserA, "", 25)
	if err != nil {
		t.Fatalf("listado fallo: %v", err)
	}
	if len(primera) != 0 {
		t.Fatalf("los items incompletos deben omitirse: %+v", primera)
	}
}

// TestE016StoreExpresionesSinValoresNoUsados: todo valor declarado aparece en la expresión de la
// consulta (misma regla que se aplica a las transacciones).
func TestE016StoreExpresionesSinValoresNoUsados(t *testing.T) {
	m := newMemoriaConsentimientos()
	store := dynamoAdapter.NewConsentsStore(m, e016Tabla)
	if _, _, err := store.ListUserConsents(context.Background(), e016UserA, "", 25); err != nil {
		t.Fatalf("consulta fallo: %v", err)
	}
	for placeholder := range m.ultimosValores {
		if !strings.Contains(m.ultimaCondicion, placeholder) {
			t.Fatalf("el valor %q no aparece en la condicion %q", placeholder, m.ultimaCondicion)
		}
	}
}

// TestE016StoreFallaCerrado: sin cliente, sin tabla, sin usuario o con límite inválido no se
// consulta nada.
func TestE016StoreFallaCerrado(t *testing.T) {
	ctx := context.Background()
	if _, _, err := dynamoAdapter.NewConsentsStore(nil, "").ListUserConsents(ctx, e016UserA, "", 25); err == nil {
		t.Fatal("sin cliente debe fallar cerrado")
	}
	m := newMemoriaConsentimientos()
	if _, _, err := dynamoAdapter.NewConsentsStore(m, "").ListUserConsents(ctx, e016UserA, "", 25); err == nil {
		t.Fatal("sin tabla debe fallar cerrado")
	}
	if _, _, err := dynamoAdapter.NewConsentsStore(m, e016Tabla).ListUserConsents(ctx, "  ", "", 25); err == nil {
		t.Fatal("sin usuario debe fallar cerrado")
	}
	for _, limite := range []int{-1, 0, 101} {
		if _, _, err := dynamoAdapter.NewConsentsStore(m, e016Tabla).ListUserConsents(ctx, e016UserA, "", limite); err == nil {
			t.Fatalf("limite %d debe fallar cerrado", limite)
		}
	}
	if m.consultas != 0 {
		t.Fatalf("ninguna llamada invalida debe llegar a DynamoDB: %d", m.consultas)
	}
}
