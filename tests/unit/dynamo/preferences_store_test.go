package dynamo_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	dynamoAdapter "github.com/nexumpr/Nexum-Backend/internal/adapters/dynamo"
	appPreferences "github.com/nexumpr/Nexum-Backend/internal/application/preferences"
	domainDynamo "github.com/nexumpr/Nexum-Backend/internal/domain/dynamo"
)

// Pruebas de E068 sobre el adaptador REAL `PreferencesStore` montado sobre un doble con estado
// que interpreta la misma condición de `PutItem` que emite. Demuestran que la creación perezosa
// es idempotente, que no usa `Scan`, que exige lectura consistente y que la clave es la del
// diseño (`USER#<id>`/`PREFERENCES`).

const (
	e068Tabla   = "NEXUM_CORE_TEST"
	e068UserA   = "11111111-2222-4333-8444-555555555555"
	e068UserB   = "99999999-8888-4777-8666-555555555555"
	e068SKAjust = "PREFERENCES"
)

var e068Creado = time.Date(2026, 11, 18, 10, 0, 0, 0, time.UTC)

// memoriaPreferencias implementa `dynamoAdapter.PreferencesAPI` con estado. No es un motor
// DynamoDB: sólo evalúa la gramática que emite este adaptador (`attribute_not_exists(PK)`) y
// falla si aparece otra condición, en lugar de adivinar.
type memoriaPreferencias struct {
	mu                 sync.Mutex
	items              map[string]map[string]types.AttributeValue
	gets               int
	puts               int
	condiciones        []string
	ultimoPut          *dynamodb.PutItemInput
	lecturaConsistente bool
	fallo              error
	// antesDePut permite simular que otra petición escribe entre la lectura y la creación.
	antesDePut func(*memoriaPreferencias)
}

func newMemoriaPreferencias() *memoriaPreferencias {
	return &memoriaPreferencias{items: map[string]map[string]types.AttributeValue{}}
}

func (m *memoriaPreferencias) GetItem(_ context.Context, params *dynamodb.GetItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.gets++
	m.lecturaConsistente = aws.ToBool(params.ConsistentRead)
	if m.fallo != nil {
		return nil, m.fallo
	}
	item, ok := m.items[claveDe(mapaString(params.Key, "PK"), mapaString(params.Key, "SK"))]
	if !ok {
		return &dynamodb.GetItemOutput{}, nil
	}
	return &dynamodb.GetItemOutput{Item: copiarItem(item)}, nil
}

func (m *memoriaPreferencias) PutItem(_ context.Context, params *dynamodb.PutItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.puts++
	m.ultimoPut = params
	condicion := aws.ToString(params.ConditionExpression)
	m.condiciones = append(m.condiciones, condicion)
	if m.fallo != nil {
		return nil, m.fallo
	}

	pk := mapaString(params.Item, "PK")
	sk := mapaString(params.Item, "SK")
	clave := claveDe(pk, sk)
	// El gancho corre con el mutex tomado: representa de forma determinista a otro escritor que
	// ya creó el item.
	if m.antesDePut != nil {
		gancho := m.antesDePut
		m.antesDePut = nil
		gancho(m)
	}
	existente, _ := m.items[clave]
	if condicion != "" {
		ok, err := evaluarCondicion(condicion, existente, params.ExpressionAttributeNames, params.ExpressionAttributeValues)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, &types.ConditionalCheckFailedException{Message: aws.String("The conditional request failed")}
		}
	}
	m.items[clave] = copiarItem(params.Item)
	return &dynamodb.PutItemOutput{}, nil
}

// TestE068StoreClaveYAtributosCanonicos comprueba la clave del diseño y el contenido exacto del
// item materializado, incluido el alias A8 `timeZone` y las marcas RFC3339.
func TestE068StoreClaveYAtributosCanonicos(t *testing.T) {
	m := newMemoriaPreferencias()
	store := dynamoAdapter.NewPreferencesStore(m, e068Tabla)

	prefs, err := store.CreatePreferences(context.Background(), e068UserA, appPreferences.Defaults(), e068Creado)
	if err != nil {
		t.Fatalf("creacion fallo: %v", err)
	}
	if prefs.Locale != "es-MX" || prefs.Timezone != "America/Mexico_City" || prefs.Version != 1 || !prefs.HasVersion {
		t.Fatalf("defaults inesperados: %+v", prefs)
	}
	if m.puts != 1 {
		t.Fatalf("debe hacerse un solo PutItem, hubo %d", m.puts)
	}
	if len(m.condiciones) != 1 || m.condiciones[0] != "attribute_not_exists(PK)" {
		t.Fatalf("condicion inesperada: %v", m.condiciones)
	}

	pk, sk := domainDynamo.PrefixUser+e068UserA, e068SKAjust
	item := m.items[claveDe(pk, sk)]
	if item == nil {
		t.Fatalf("el item debe vivir en %s/%s", pk, sk)
	}
	esperados := map[string]string{
		"PK":         pk,
		"SK":         sk,
		"entityType": "USER_PREFERENCES",
		"userId":     e068UserA,
		"locale":     "es-MX",
		"timezone":   "America/Mexico_City",
		"timeZone":   "America/Mexico_City",
		"createdAt":  e068Creado.Format(time.RFC3339),
		"updatedAt":  e068Creado.Format(time.RFC3339),
	}
	for atributo, esperado := range esperados {
		if obtenido := mapaString(item, atributo); obtenido != esperado {
			t.Fatalf("%s esperado %q, obtenido %q", atributo, esperado, obtenido)
		}
	}
	if version, ok := item["version"].(*types.AttributeValueMemberN); !ok || version.Value != "1" {
		t.Fatalf("version debe ser Number 1: %v", item["version"])
	}
	// El PutItem no declara valores de expresión: no hay ninguno que pueda quedar sin usar.
	if len(m.ultimoPut.ExpressionAttributeValues) != 0 {
		t.Fatalf("PutItem no debe declarar valores de expresion: %v", m.ultimoPut.ExpressionAttributeValues)
	}
}

// TestE068StoreCreacionIdempotente: la carrera se resuelve con el item del ganador; no se
// sobrescribe una versión ya modificada.
func TestE068StoreCreacionIdempotente(t *testing.T) {
	m := newMemoriaPreferencias()
	pk, sk := domainDynamo.PrefixUser+e068UserA, e068SKAjust
	pkPrevio := strAttr(pk)
	skPrevio := strAttr(sk)
	m.items[claveDe(pk, sk)] = map[string]types.AttributeValue{
		"PK":         pkPrevio,
		"SK":         skPrevio,
		"entityType": strAttr("USER_PREFERENCES"),
		"userId":     strAttr(e068UserA),
		"locale":     strAttr("en-US"),
		"timezone":   strAttr("Europe/Madrid"),
		"timeZone":   strAttr("Europe/Madrid"),
		"version":    numAttr(5),
		"createdAt":  strAttr(e068Creado.Format(time.RFC3339)),
		"updatedAt":  strAttr(e068Creado.Format(time.RFC3339)),
	}

	store := dynamoAdapter.NewPreferencesStore(m, e068Tabla)
	prefs, err := store.CreatePreferences(context.Background(), e068UserA, appPreferences.Defaults(), e068Creado)
	if err != nil {
		t.Fatalf("la creacion perezosa no debe fallar si el item ya existe: %v", err)
	}
	if prefs.Locale != "en-US" || prefs.Version != 5 {
		t.Fatalf("debe devolverse el item existente, no los defaults: %+v", prefs)
	}
	if mapaString(m.items[claveDe(pk, sk)], "locale") != "en-US" {
		t.Fatal("el PutItem condicional no puede sobrescribir el item existente")
	}
}

// TestE068StoreCarreraEntreLecturaYCreacion simula exactamente el caso del adaptador: al ir a
// escribir, otra petición ya creó el item (condición fallida) y se devuelve esa versión.
func TestE068StoreCarreraEntreLecturaYCreacion(t *testing.T) {
	m := newMemoriaPreferencias()
	m.antesDePut = func(doble *memoriaPreferencias) {
		pk, sk := domainDynamo.PrefixUser+e068UserA, e068SKAjust
		doble.items[claveDe(pk, sk)] = map[string]types.AttributeValue{
			"PK":         strAttr(pk),
			"SK":         strAttr(sk),
			"entityType": strAttr("USER_PREFERENCES"),
			"userId":     strAttr(e068UserA),
			"locale":     strAttr("en-US"),
			"timezone":   strAttr("America/New_York"),
			"timeZone":   strAttr("America/New_York"),
			"version":    numAttr(2),
		}
	}

	store := dynamoAdapter.NewPreferencesStore(m, e068Tabla)
	prefs, err := store.CreatePreferences(context.Background(), e068UserA, appPreferences.Defaults(), e068Creado)
	if err != nil {
		t.Fatalf("una condicion perdida debe resolverse con el item del ganador: %v", err)
	}
	if prefs.Locale != "en-US" || prefs.Timezone != "America/New_York" || prefs.Version != 2 {
		t.Fatalf("debe devolverse la version ganadora: %+v", prefs)
	}
}

// TestE068StoreLeeConConsistenciaFuerte: la versión que habilita el CAS de E069 no puede venir
// de una réplica retrasada.
func TestE068StoreLeeConConsistenciaFuerte(t *testing.T) {
	m := newMemoriaPreferencias()
	store := dynamoAdapter.NewPreferencesStore(m, e068Tabla)

	if prefs, err := store.LoadPreferences(context.Background(), e068UserA); err != nil || prefs != nil {
		t.Fatalf("un item ausente debe ser (nil, nil): %+v %v", prefs, err)
	}
	if !m.lecturaConsistente {
		t.Fatal("LoadPreferences debe usar ConsistentRead")
	}
}

// TestE068StoreAislaUsuarios: cada titular tiene su propia clave; no existe lectura cruzada.
func TestE068StoreAislaUsuarios(t *testing.T) {
	m := newMemoriaPreferencias()
	store := dynamoAdapter.NewPreferencesStore(m, e068Tabla)

	if _, err := store.CreatePreferences(context.Background(), e068UserA, appPreferences.Defaults(), e068Creado); err != nil {
		t.Fatalf("creacion de A fallo: %v", err)
	}
	ajenas, err := store.LoadPreferences(context.Background(), e068UserB)
	if err != nil {
		t.Fatalf("lectura de B fallo: %v", err)
	}
	if ajenas != nil {
		t.Fatalf("B no puede ver las preferencias de A: %+v", ajenas)
	}
}

// TestE068StoreIgnoraEntidadAjena: una clave reutilizada por otra entidad no se interpreta como
// preferencias ni se sobrescribe.
func TestE068StoreIgnoraEntidadAjena(t *testing.T) {
	m := newMemoriaPreferencias()
	pk, sk := domainDynamo.PrefixUser+e068UserA, e068SKAjust
	m.items[claveDe(pk, sk)] = map[string]types.AttributeValue{
		"PK":         strAttr(pk),
		"SK":         strAttr(sk),
		"entityType": strAttr("OTHER_ENTITY"),
		"locale":     strAttr("en-US"),
	}

	store := dynamoAdapter.NewPreferencesStore(m, e068Tabla)
	if prefs, err := store.LoadPreferences(context.Background(), e068UserA); err != nil || prefs != nil {
		t.Fatalf("otra entidad no es preferencias: %+v %v", prefs, err)
	}
}

// TestE068StoreFallosCerrados: sin cliente, sin tabla o con error del servicio no hay valores
// inventados.
func TestE068StoreFallosCerrados(t *testing.T) {
	if _, err := dynamoAdapter.NewPreferencesStore(nil, "").LoadPreferences(context.Background(), e068UserA); !errors.Is(err, appPreferences.ErrPreferencesUnavailable) {
		t.Fatalf("sin cliente debe fallar cerrado, obtenido %v", err)
	}
	m := newMemoriaPreferencias()
	if _, err := dynamoAdapter.NewPreferencesStore(m, "").CreatePreferences(context.Background(), e068UserA, appPreferences.Defaults(), e068Creado); !errors.Is(err, appPreferences.ErrPreferencesUnavailable) {
		t.Fatalf("sin tabla debe fallar cerrado, obtenido %v", err)
	}

	m = newMemoriaPreferencias()
	m.fallo = errors.New("dynamo no disponible")
	store := dynamoAdapter.NewPreferencesStore(m, e068Tabla)
	if _, err := store.LoadPreferences(context.Background(), e068UserA); !errors.Is(err, appPreferences.ErrPreferencesUnavailable) {
		t.Fatalf("error de lectura debe ser unavailable, obtenido %v", err)
	}
	if _, err := store.CreatePreferences(context.Background(), e068UserA, appPreferences.Defaults(), e068Creado); !errors.Is(err, appPreferences.ErrPreferencesUnavailable) {
		t.Fatalf("error de escritura debe ser unavailable, obtenido %v", err)
	}
}

// TestE068StoreNoExponeScan: el compilador y la reflexión confirman que el puerto no declara
// `Scan`, de modo que ninguna ruta puede degradar a un barrido global.
func TestE068StoreNoExponeScan(t *testing.T) {
	tipo := reflect.TypeOf((*dynamoAdapter.PreferencesAPI)(nil)).Elem()
	for i := 0; i < tipo.NumMethod(); i++ {
		if strings.EqualFold(tipo.Method(i).Name, "Scan") {
			t.Fatal("la interfaz de preferencias no debe exponer Scan")
		}
	}
}
