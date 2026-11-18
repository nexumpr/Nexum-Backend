package dynamo_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
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

// Pruebas de E017 sobre el adaptador REAL `ConsentWriteStore` montado sobre un doble con estado
// que interpreta la MISMA condición que emite (`attribute_not_exists(PK)`) y la misma consulta
// particionada de E016. Demuestran que la escritura es append-only, que la `SK` es ordenable
// cronológicamente con 9 dígitos fijos, que un conflicto de clave es 409 sin sobrescribir y
// que el consentimiento registrado se lee después con el almacén real de E016.

const (
	e017Tabla = "NEXUM_CORE_TEST_E017"

	e017UserA   = "11111111-2222-4333-8444-555555555555"
	e017UserB   = "99999999-8888-4777-8666-555555555555"
	e017ID      = "018f90ef-08e5-7a68-b09f-0000000000e7"
	e017IDB     = "018f90ef-08e5-7a68-b09f-0000000000f8"
	e017IDC     = "018f90ef-08e5-7a68-b09f-0000000000a9"
	e017IDSoloA = "018f90ef-08e5-7a68-b09f-0000000000b1"
	e017IDSoloB = "018f90ef-08e5-7a68-b09f-0000000000b2"
)

var e017Base = time.Date(2026, 11, 18, 13, 0, 0, 123456789, time.UTC)

// memoriaConsentWrite amplía el doble de E016 con la única operación de escritura que necesita
// E017. Falla si aparece una condición distinta de la autorizada: el test no adivina semántica
// que el doble no implemente.
type memoriaConsentWrite struct {
	*memoriaConsentimientos

	escrituras      int
	ultimaCondicion string
	ultimoItem      map[string]types.AttributeValue
}

func newMemoriaConsentWrite() *memoriaConsentWrite {
	return &memoriaConsentWrite{memoriaConsentimientos: newMemoriaConsentimientos()}
}

func (m *memoriaConsentWrite) PutItem(_ context.Context, params *dynamodb.PutItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error) {
	condicion := aws.ToString(params.ConditionExpression)

	m.mu.Lock()
	defer m.mu.Unlock()

	m.escrituras++
	m.ultimaCondicion = condicion
	m.ultimoItem = copiarItem(params.Item)

	if condicion != "attribute_not_exists(PK)" {
		return nil, fmt.Errorf("condicion de escritura no soportada por el doble: %q", condicion)
	}
	pk := mapaString(params.Item, "PK")
	sk := mapaString(params.Item, "SK")
	clave := claveDe(pk, sk)
	if _, existe := m.items[clave]; existe {
		return nil, &types.ConditionalCheckFailedException{Message: aws.String("The conditional request failed")}
	}
	m.items[clave] = copiarItem(params.Item)
	return &dynamodb.PutItemOutput{}, nil
}

func e017Decision(userID string, grabado time.Time, granted bool) appPrivacy.ConsentDecision {
	return appPrivacy.ConsentDecision{
		UserID:        userID,
		ID:            e017ID,
		NoticeVersion: "2026-11",
		Purpose:       appPrivacy.ConsentPurposeNotifications,
		Granted:       granted,
		RecordedAt:    grabado,
		CreatedAt:     grabado,
		Source:        appPrivacy.ConsentSourceAPI,
	}
}

func e017ClaveEsperada(userID string, grabado time.Time, id string) (string, string) {
	return domainDynamo.PrefixUser + userID,
		domainDynamo.PrefixConsent + grabado.UTC().Format("2006-01-02T15:04:05.000000000Z") + "#" + id
}

func e017LeerGuardado(t *testing.T, m *memoriaConsentWrite, pk, sk string) map[string]types.AttributeValue {
	t.Helper()
	item := m.leer(pk, sk)
	if item == nil {
		t.Fatalf("no se guardo el item %s / %s", pk, sk)
	}
	return item
}

// TestE017StoreEscribeItemAppendOnlyExacto cubre el requisito central: `PutItem` con
// `attribute_not_exists(PK)` en la partición del titular y exactamente los atributos
// documentados, sin claves ni índices nuevos.
func TestE017StoreEscribeItemAppendOnlyExacto(t *testing.T) {
	m := newMemoriaConsentWrite()
	store := dynamoAdapter.NewConsentWriteStore(m, e017Tabla)

	if err := store.AppendConsent(context.Background(), e017Decision(e017UserA, e017Base, true)); err != nil {
		t.Fatalf("escritura valida fallo: %v", err)
	}
	if m.escrituras != 1 {
		t.Fatalf("debe realizarse una sola escritura, se realizaron %d", m.escrituras)
	}
	if m.ultimaCondicion != "attribute_not_exists(PK)" {
		t.Fatalf("la condicion debe ser attribute_not_exists(PK), fue %q", m.ultimaCondicion)
	}
	if m.ultimoItem["PK"] == nil || m.ultimoItem["SK"] == nil {
		t.Fatalf("el item debe declarar claves primarias: %+v", m.ultimoItem)
	}

	pk, sk := e017ClaveEsperada(e017UserA, e017Base, e017ID)
	if !strings.HasSuffix(sk, "#"+e017ID) || !strings.Contains(sk, "2026-11-18T13:00:00.123456789Z") {
		t.Fatalf("la SK debe fijar 9 digitos y el UUID: %q", sk)
	}
	item := e017LeerGuardado(t, m, pk, sk)

	// Exactamente los atributos documentados: ningún extra.
	esperados := map[string]bool{
		"PK": true, "SK": true, "id": true, "noticeVersion": true, "purpose": true,
		"granted": true, "recordedAt": true, "source": true, "createdAt": true,
	}
	if len(item) != len(esperados) {
		t.Fatalf("el item debe tener %d atributos, tiene %d: %+v", len(esperados), len(item), item)
	}
	for nombre := range item {
		if !esperados[nombre] {
			t.Fatalf("atributo no documentado en el item: %q", nombre)
		}
	}

	if mapaString(item, "id") != e017ID {
		t.Fatalf("id incorrecto: %+v", item["id"])
	}
	if mapaString(item, "noticeVersion") != "2026-11" {
		t.Fatalf("noticeVersion incorrecta: %+v", item["noticeVersion"])
	}
	if mapaString(item, "purpose") != appPrivacy.ConsentPurposeNotifications {
		t.Fatalf("purpose incorrecto: %+v", item["purpose"])
	}
	if mapaString(item, "source") != appPrivacy.ConsentSourceAPI {
		t.Fatalf("source debe ser %q: %+v", appPrivacy.ConsentSourceAPI, item["source"])
	}
	granted, ok := item["granted"].(*types.AttributeValueMemberBOOL)
	if !ok || !granted.Value {
		t.Fatalf("granted debe ser BOOL true: %+v", item["granted"])
	}
	for _, campo := range []string{"recordedAt", "createdAt"} {
		grabado, err := time.Parse(time.RFC3339Nano, mapaString(item, campo))
		if err != nil {
			t.Fatalf("%s debe ser date-time ISO-8601: %q (%v)", campo, mapaString(item, campo), err)
		}
		if !grabado.Equal(e017Base) {
			t.Fatalf("%s debe ser el instante de la decision: %v vs %v", campo, grabado, e017Base)
		}
	}
}

// TestE017StoreAppendOnlyNoSobrescribe: si la clave ya existe, la condición falla, se traduce
// en conflicto 409 y el consentimiento previo queda intacto.
func TestE017StoreAppendOnlyNoSobrescribe(t *testing.T) {
	m := newMemoriaConsentWrite()
	store := dynamoAdapter.NewConsentWriteStore(m, e017Tabla)

	if err := store.AppendConsent(context.Background(), e017Decision(e017UserA, e017Base, true)); err != nil {
		t.Fatalf("primera escritura fallo: %v", err)
	}
	// Misma clave (mismo instante y mismo id) con la decision contraria: no puede reemplazar.
	if err := store.AppendConsent(context.Background(), e017Decision(e017UserA, e017Base, false)); !errors.Is(err, appPrivacy.ErrConsentConflict) {
		t.Fatalf("una clave ocupada debe ser 409, se obtuvo %v", err)
	}
	if m.escrituras != 2 {
		t.Fatalf("se esperaban dos intentos de escritura, hubo %d", m.escrituras)
	}

	pk, sk := e017ClaveEsperada(e017UserA, e017Base, e017ID)
	item := e017LeerGuardado(t, m, pk, sk)
	granted, ok := item["granted"].(*types.AttributeValueMemberBOOL)
	if !ok || !granted.Value {
		t.Fatalf("el consentimiento previo no debe sobrescribirse: %+v", item["granted"])
	}
}

// TestE017StoreSeLeeConE016 demuestra la integración real: lo escrito por E017 se proyecta en
// el historial de E016 con los cinco campos del contrato, incluido `granted:false`.
func TestE017StoreSeLeeConE016(t *testing.T) {
	m := newMemoriaConsentWrite()
	escritor := dynamoAdapter.NewConsentWriteStore(m, e017Tabla)
	lector := dynamoAdapter.NewConsentsStore(m, e017Tabla)

	denegado := e017Decision(e017UserA, e017Base, false)
	denegado.Purpose = appPrivacy.ConsentPurposeAnalytics
	if err := escritor.AppendConsent(context.Background(), denegado); err != nil {
		t.Fatalf("escritura fallo: %v", err)
	}

	pagina, nextSK, err := lector.ListUserConsents(context.Background(), e017UserA, "", appPrivacy.ConsentsDefaultLimit)
	if err != nil {
		t.Fatalf("lectura de E016 fallo: %v", err)
	}
	if nextSK != "" {
		t.Fatalf("una sola fila no debe emitir continuacion, se obtuvo %q", nextSK)
	}
	if len(pagina) != 1 {
		t.Fatalf("E016 debe leer el consentimiento escrito por E017, leyo %d", len(pagina))
	}
	record := pagina[0]
	if record.ID != e017ID || record.NoticeVersion != "2026-11" ||
		record.Purpose != appPrivacy.ConsentPurposeAnalytics || record.Granted {
		t.Fatalf("la proyeccion de E016 no coincide con la decision: %+v", record)
	}
	if !record.RecordedAt.Equal(e017Base) {
		t.Fatalf("recordedAt debe conservarse: %v vs %v", record.RecordedAt, e017Base)
	}
}

// TestE017StoreOrdenCronologicoPorClave: la `SK` de ancho fijo ordena cronológicamente aunque
// las escrituras lleguen desordenadas; E016 devuelve primero la decisión más reciente.
func TestE017StoreOrdenCronologicoPorClave(t *testing.T) {
	m := newMemoriaConsentWrite()
	escritor := dynamoAdapter.NewConsentWriteStore(m, e017Tabla)
	lector := dynamoAdapter.NewConsentsStore(m, e017Tabla)

	primero := e017Decision(e017UserA, e017Base, true)
	segundo := e017Decision(e017UserA, e017Base.Add(time.Hour), false)
	segundo.ID = e017IDB
	tercero := e017Decision(e017UserA, e017Base.Add(2*time.Hour), true)
	tercero.ID = e017IDC

	// Se escriben desordenados: el orden físico no depende del orden de llegada.
	for _, decision := range []appPrivacy.ConsentDecision{segundo, tercero, primero} {
		if err := escritor.AppendConsent(context.Background(), decision); err != nil {
			t.Fatalf("escritura fallo: %v", err)
		}
	}

	pagina, _, err := lector.ListUserConsents(context.Background(), e017UserA, "", 25)
	if err != nil {
		t.Fatalf("lectura de E016 fallo: %v", err)
	}
	if len(pagina) != 3 {
		t.Fatalf("se esperaban 3 consentimientos, hay %d", len(pagina))
	}
	orden := []string{pagina[0].ID, pagina[1].ID, pagina[2].ID}
	esperado := []string{e017IDC, e017IDB, e017ID}
	for i := range esperado {
		if orden[i] != esperado[i] {
			t.Fatalf("orden descendente esperado %v, obtenido %v", esperado, orden)
		}
	}
}

// TestE017StoreAislaUsuarios: la clave vive en la partición del titular; el mismo instante y el
// mismo id en dos cuentas producen dos ítems distintos y E016 nunca mezcla historiales.
func TestE017StoreAislaUsuarios(t *testing.T) {
	m := newMemoriaConsentWrite()
	escritor := dynamoAdapter.NewConsentWriteStore(m, e017Tabla)
	lector := dynamoAdapter.NewConsentsStore(m, e017Tabla)

	propio := e017Decision(e017UserA, e017Base, true)
	propio.ID = e017IDSoloA
	ajeno := e017Decision(e017UserB, e017Base, false)
	ajeno.ID = e017IDSoloB

	if err := escritor.AppendConsent(context.Background(), propio); err != nil {
		t.Fatalf("escritura del titular fallo: %v", err)
	}
	if err := escritor.AppendConsent(context.Background(), ajeno); err != nil {
		t.Fatalf("escritura del otro usuario fallo: %v", err)
	}
	if m.escrituras != 2 {
		t.Fatalf("ambas claves son distintas por particion: %d escrituras", m.escrituras)
	}

	pagina, _, err := lector.ListUserConsents(context.Background(), e017UserA, "", 25)
	if err != nil {
		t.Fatalf("lectura de E016 fallo: %v", err)
	}
	if len(pagina) != 1 || pagina[0].ID != e017IDSoloA {
		t.Fatalf("solo debe leerse el consentimiento del titular: %+v", pagina)
	}
	for _, record := range pagina {
		if record.ID == e017IDSoloB {
			t.Fatal("un consentimiento de otra cuenta no puede aparecer en el historial")
		}
	}
	pkB, skB := e017ClaveEsperada(e017UserB, e017Base, e017IDSoloB)
	if item := m.leer(pkB, skB); item == nil || mapaString(item, "id") != e017IDSoloB {
		t.Fatalf("el consentimiento del otro usuario debe existir en su propia particion: %+v", item)
	}
}

// TestE017StoreFallaCerrado: sin cliente, sin tabla o con una decisión incompleta no se escribe
// nada.
func TestE017StoreFallaCerrado(t *testing.T) {
	ctx := context.Background()
	decision := e017Decision(e017UserA, e017Base, true)

	if err := dynamoAdapter.NewConsentWriteStore(nil, e017Tabla).AppendConsent(ctx, decision); !errors.Is(err, appPrivacy.ErrConsentUnavailable) {
		t.Fatalf("sin cliente debe fallar cerrado, se obtuvo %v", err)
	}

	m := newMemoriaConsentWrite()
	if err := dynamoAdapter.NewConsentWriteStore(m, "  ").AppendConsent(ctx, decision); !errors.Is(err, appPrivacy.ErrConsentUnavailable) {
		t.Fatalf("sin tabla debe fallar cerrado, se obtuvo %v", err)
	}

	store := dynamoAdapter.NewConsentWriteStore(m, e017Tabla)
	invalidas := []struct {
		nombre   string
		decision appPrivacy.ConsentDecision
	}{
		{"sin usuario", func() appPrivacy.ConsentDecision { d := decision; d.UserID = "  "; return d }()},
		{"sin identificador", func() appPrivacy.ConsentDecision { d := decision; d.ID = ""; return d }()},
		{"sin version", func() appPrivacy.ConsentDecision { d := decision; d.NoticeVersion = ""; return d }()},
		{"proposito invalido", func() appPrivacy.ConsentDecision { d := decision; d.Purpose = "marketing"; return d }()},
		{"sin instante", func() appPrivacy.ConsentDecision { d := decision; d.RecordedAt = time.Time{}; return d }()},
	}
	for _, caso := range invalidas {
		if err := store.AppendConsent(ctx, caso.decision); err == nil {
			t.Fatalf("%s debe fallar cerrado", caso.nombre)
		}
	}
	if m.escrituras != 0 {
		t.Fatalf("ninguna decision invalida debe llegar a DynamoDB: %d escrituras", m.escrituras)
	}
}

// TestE017StoreInterfazSoloEscrituraCondicional: la interfaz declarada no expone Scan ni
// ninguna operación de borrado o reemplazo.
func TestE017StoreInterfazSoloEscrituraCondicional(t *testing.T) {
	tipo := reflect.TypeOf((*dynamoAdapter.ConsentWriteAPI)(nil)).Elem()
	metodos := make([]string, 0, tipo.NumMethod())
	for i := 0; i < tipo.NumMethod(); i++ {
		metodos = append(metodos, tipo.Method(i).Name)
	}
	if len(metodos) != 1 || metodos[0] != "PutItem" {
		t.Fatalf("la interfaz de E017 solo debe exponer PutItem, expone %v", metodos)
	}
	for _, prohibido := range []string{"Query", "Scan", "DeleteItem", "UpdateItem", "BatchWriteItem"} {
		for _, metodo := range metodos {
			if strings.EqualFold(metodo, prohibido) {
				t.Fatalf("la interfaz de E017 no debe exponer %s", prohibido)
			}
		}
	}
}
