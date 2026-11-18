package dynamo_test

import (
	"context"
	"errors"
	"reflect"
	"strconv"
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
// DynamoDB: sólo evalúa la gramática que emite este adaptador (`attribute_not_exists(PK)` en
// `PutItem` y `SET ... ADD ...` con `#version = :expected` en `UpdateItem`) y falla si aparece
// otra forma, en lugar de adivinar.
type memoriaPreferencias struct {
	mu                 sync.Mutex
	items              map[string]map[string]types.AttributeValue
	gets               int
	puts               int
	updates            int
	condiciones        []string
	ultimoPut          *dynamodb.PutItemInput
	ultimoUpdate       *dynamodb.UpdateItemInput
	lecturaConsistente bool
	fallo              error
	// antesDePut y antesDeUpdate permiten simular que otra petición escribe entre la lectura y
	// la escritura condicional.
	antesDePut    func(*memoriaPreferencias)
	antesDeUpdate func(*memoriaPreferencias)
	// fallosForzados devuelve ConditionalCheckFailedException sin evaluar la condición, para
	// verificar el bucle de reintento y el 409 por agotamiento.
	fallosForzados int
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

// UpdateItem implementa la escritura condicional que emite E069: evalúa la condición con la
// misma gramática que `evaluarCondicion` y aplica el `SET ... ADD ...` declarado.
func (m *memoriaPreferencias) UpdateItem(_ context.Context, params *dynamodb.UpdateItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.updates++
	m.ultimoUpdate = params
	m.condiciones = append(m.condiciones, aws.ToString(params.ConditionExpression))
	if m.fallo != nil {
		return nil, m.fallo
	}
	if m.antesDeUpdate != nil {
		gancho := m.antesDeUpdate
		m.antesDeUpdate = nil
		gancho(m)
	}
	clave := claveDe(mapaString(params.Key, "PK"), mapaString(params.Key, "SK"))
	existente, existe := m.items[clave]
	if !existe {
		existente = nil
	}
	if m.fallosForzados > 0 {
		m.fallosForzados--
		return nil, &types.ConditionalCheckFailedException{Message: aws.String("The conditional request failed")}
	}
	if condicion := aws.ToString(params.ConditionExpression); condicion != "" {
		cumple, err := evaluarCondicion(condicion, existente, params.ExpressionAttributeNames, params.ExpressionAttributeValues)
		if err != nil {
			return nil, err
		}
		if !cumple {
			return nil, &types.ConditionalCheckFailedException{Message: aws.String("The conditional request failed")}
		}
	}
	actualizado, err := aplicarActualizacion(params, existente)
	if err != nil {
		return nil, err
	}
	if !existe {
		actualizado["PK"] = params.Key["PK"]
		actualizado["SK"] = params.Key["SK"]
	}
	m.items[clave] = actualizado

	salida := &dynamodb.UpdateItemOutput{}
	if params.ReturnValues == types.ReturnValueAllNew {
		salida.Attributes = copiarItem(actualizado)
	}
	return salida, nil
}

// aplicarActualizacion interpreta exactamente `SET a = :b, ... ADD #version :one`. Cualquier
// otra forma produce error: el doble no adivina gramática que el adaptador no emite.
func aplicarActualizacion(params *dynamodb.UpdateItemInput, existente map[string]types.AttributeValue) (map[string]types.AttributeValue, error) {
	expresion := strings.TrimSpace(aws.ToString(params.UpdateExpression))
	partes := strings.SplitN(expresion, " ADD ", 2)
	seccionSET := strings.TrimSpace(partes[0])
	if !strings.HasPrefix(seccionSET, "SET ") {
		return nil, errors.New("UpdateExpression sin SET soportado: " + expresion)
	}
	resultado := copiarItem(existente)
	if resultado == nil {
		resultado = map[string]types.AttributeValue{}
	}
	for _, asignacion := range strings.Split(strings.TrimSpace(strings.TrimPrefix(seccionSET, "SET ")), ", ") {
		lados := strings.SplitN(asignacion, " = ", 2)
		if len(lados) != 2 {
			return nil, errors.New("asignacion SET no soportada: " + asignacion)
		}
		nombre := resolverNombre(lados[0], params.ExpressionAttributeNames)
		valor, ok := params.ExpressionAttributeValues[strings.TrimSpace(lados[1])]
		if !ok {
			return nil, errors.New("valor de SET ausente: " + lados[1])
		}
		resultado[nombre] = valor
	}
	if len(partes) == 2 {
		for _, incremento := range strings.Split(strings.TrimSpace(partes[1]), ", ") {
			campos := strings.Fields(incremento)
			if len(campos) != 2 {
				return nil, errors.New("incremento ADD no soportado: " + incremento)
			}
			nombre := resolverNombre(campos[0], params.ExpressionAttributeNames)
			delta, ok := params.ExpressionAttributeValues[campos[1]]
			if !ok {
				return nil, errors.New("valor de ADD ausente: " + campos[1])
			}
			deltaN, ok := delta.(*types.AttributeValueMemberN)
			if !ok {
				return nil, errors.New("ADD no numerico: " + campos[1])
			}
			incrementoValor, err := strconv.ParseInt(deltaN.Value, 10, 64)
			if err != nil {
				return nil, errors.New("incremento ADD invalido: " + deltaN.Value)
			}
			actual := int64(0)
			if previo, existe := resultado[nombre]; existe {
				previoN, ok := previo.(*types.AttributeValueMemberN)
				if !ok {
					return nil, errors.New("ADD sobre atributo no numerico: " + nombre)
				}
				actual, err = strconv.ParseInt(previoN.Value, 10, 64)
				if err != nil {
					return nil, errors.New("atributo ADD invalido: " + previoN.Value)
				}
			}
			resultado[nombre] = numAttr(actual + incrementoValor)
		}
	}
	return resultado, nil
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

// ---------- E069: escritura condicional ----------

var e069Actualizado = time.Date(2026, 11, 18, 12, 15, 0, 0, time.UTC)

// sembrarPreferenciasE069 deja un item con la forma exacta que escribe E068.
func sembrarPreferenciasE069(m *memoriaPreferencias, userID string, version int64, locale, timezone string) {
	pk, sk := domainDynamo.PrefixUser+userID, e068SKAjust
	m.items[claveDe(pk, sk)] = map[string]types.AttributeValue{
		"PK":         strAttr(pk),
		"SK":         strAttr(sk),
		"entityType": strAttr("USER_PREFERENCES"),
		"userId":     strAttr(userID),
		"locale":     strAttr(locale),
		"timezone":   strAttr(timezone),
		"timeZone":   strAttr(timezone),
		"version":    numAttr(version),
		"createdAt":  strAttr(e068Creado.Format(time.RFC3339)),
		"updatedAt":  strAttr(e068Creado.Format(time.RFC3339)),
	}
}

// exigirValoresUsados aplica la restriccion real de DynamoDB: todo name y todo value declarado
// en la peticion debe aparecer en la expresion. Un valor sobrante produce ValidationException
// y el adaptador lo traduciria a 503.
func exigirValoresUsados(t *testing.T, params *dynamodb.UpdateItemInput) {
	t.Helper()
	expresion := aws.ToString(params.UpdateExpression) + " " + aws.ToString(params.ConditionExpression)
	for nombre := range params.ExpressionAttributeValues {
		if !strings.Contains(expresion, nombre) {
			t.Fatalf("valor de expresion %q declarado sin uso en %q", nombre, expresion)
		}
	}
	for alias := range params.ExpressionAttributeNames {
		if !strings.Contains(expresion, alias) {
			t.Fatalf("alias %q declarado sin uso en %q", alias, expresion)
		}
	}
}

// TestE069StoreActualizaConCASYAlias: un solo UpdateItem condicional escribe locale, timezone y
// el alias A8 timeZone, conserva createdAt y devuelve la version incrementada con ALL_NEW.
func TestE069StoreActualizaConCASYAlias(t *testing.T) {
	m := newMemoriaPreferencias()
	sembrarPreferenciasE069(m, e068UserA, 3, "es-MX", "America/Mexico_City")
	store := dynamoAdapter.NewPreferencesStore(m, e068Tabla)

	prefs, err := store.UpdatePreferences(context.Background(), e068UserA, appPreferences.Update{
		Locale:          "en-US",
		Timezone:        "Europe/Madrid",
		ExpectedVersion: 3,
		HasVersion:      true,
		Now:             e069Actualizado,
	})
	if err != nil {
		t.Fatalf("actualizacion valida fallo: %v", err)
	}
	if prefs.Locale != "en-US" || prefs.Timezone != "Europe/Madrid" || prefs.Version != 4 || !prefs.HasVersion {
		t.Fatalf("resultado inesperado: %+v", prefs)
	}
	if m.updates != 1 {
		t.Fatalf("debe hacerse un solo UpdateItem, hubo %d", m.updates)
	}

	entrada := m.ultimoUpdate
	if entrada.ReturnValues != types.ReturnValueAllNew {
		t.Fatalf("ReturnValues debe ser ALL_NEW, obtenido %q", entrada.ReturnValues)
	}
	if condicion := aws.ToString(entrada.ConditionExpression); condicion != "#version = :expected" {
		t.Fatalf("condicion inesperada: %q", condicion)
	}
	expresion := aws.ToString(entrada.UpdateExpression)
	for _, fragmento := range []string{
		"SET #locale = :locale",
		"#timezone = :timezone",
		"#timeZone = :timeZone",
		"#updatedAt = :updatedAt",
		"ADD #version :one",
	} {
		if !strings.Contains(expresion, fragmento) {
			t.Fatalf("UpdateExpression debe contener %q: %q", fragmento, expresion)
		}
	}
	if esperado := entrada.ExpressionAttributeValues[":expected"]; esperado == nil {
		t.Fatal("la condicion debe declarar la version esperada")
	}
	exigirValoresUsados(t, entrada)

	pk, sk := domainDynamo.PrefixUser+e068UserA, e068SKAjust
	item := m.items[claveDe(pk, sk)]
	if mapaString(item, "locale") != "en-US" || mapaString(item, "timezone") != "Europe/Madrid" || mapaString(item, "timeZone") != "Europe/Madrid" {
		t.Fatalf("los tres atributos deben quedar alineados: %v", item)
	}
	if mapaString(item, "updatedAt") != e069Actualizado.Format(time.RFC3339) {
		t.Fatalf("updatedAt debe ser el reloj inyectado: %q", mapaString(item, "updatedAt"))
	}
	if mapaString(item, "createdAt") != e068Creado.Format(time.RFC3339) {
		t.Fatal("createdAt no puede modificarse en un PATCH")
	}
	if version, ok := item["version"].(*types.AttributeValueMemberN); !ok || version.Value != "4" {
		t.Fatalf("version debe incrementarse a 4: %v", item["version"])
	}
	if mapaString(item, "entityType") != "USER_PREFERENCES" || mapaString(item, "userId") != e068UserA {
		t.Fatal("la actualizacion no puede perder la identidad del item")
	}
}

// TestE069StoreVersionDistintaNoEscribe: If-Match desactualizado es 412 y no toca el item.
func TestE069StoreVersionDistintaNoEscribe(t *testing.T) {
	m := newMemoriaPreferencias()
	sembrarPreferenciasE069(m, e068UserA, 5, "en-US", "Europe/Madrid")
	store := dynamoAdapter.NewPreferencesStore(m, e068Tabla)

	_, err := store.UpdatePreferences(context.Background(), e068UserA, appPreferences.Update{
		Locale:          "es-MX",
		Timezone:        "America/Mexico_City",
		ExpectedVersion: 4,
		HasVersion:      true,
		Now:             e069Actualizado,
	})
	if !errors.Is(err, appPreferences.ErrPreferencesVersionConflict) {
		t.Fatalf("version distinta debe ser conflicto de version, obtenido %v", err)
	}
	pk, sk := domainDynamo.PrefixUser+e068UserA, e068SKAjust
	if mapaString(m.items[claveDe(pk, sk)], "locale") != "en-US" {
		t.Fatal("un 412 no puede modificar el item")
	}
}

// TestE069StoreCarreraRealDevuelve412: otro cliente confirma entre la lectura y el CAS; se
// relee, la version ya no coincide y no se sobrescribe su escritura.
func TestE069StoreCarreraRealDevuelve412(t *testing.T) {
	m := newMemoriaPreferencias()
	sembrarPreferenciasE069(m, e068UserA, 3, "es-MX", "America/Mexico_City")
	store := dynamoAdapter.NewPreferencesStore(m, e068Tabla)
	m.antesDeUpdate = func(doble *memoriaPreferencias) {
		sembrarPreferenciasE069(doble, e068UserA, 4, "en-US", "Europe/Madrid")
	}

	_, err := store.UpdatePreferences(context.Background(), e068UserA, appPreferences.Update{
		Locale:          "es-MX",
		Timezone:        "America/Mexico_City",
		ExpectedVersion: 3,
		HasVersion:      true,
		Now:             e069Actualizado,
	})
	if !errors.Is(err, appPreferences.ErrPreferencesVersionConflict) {
		t.Fatalf("una carrera con version nueva debe ser 412, obtenido %v", err)
	}
	pk, sk := domainDynamo.PrefixUser+e068UserA, e068SKAjust
	if mapaString(m.items[claveDe(pk, sk)], "locale") != "en-US" || mapaString(m.items[claveDe(pk, sk)], "timezone") != "Europe/Madrid" {
		t.Fatal("la escritura ganadora no puede ser sobrescrita")
	}
}

// TestE069StoreReintentaYConverge: un fallo condicional sin cambio de version observable se
// reintenta y termina aplicando el CAS.
func TestE069StoreReintentaYConverge(t *testing.T) {
	m := newMemoriaPreferencias()
	sembrarPreferenciasE069(m, e068UserA, 3, "es-MX", "America/Mexico_City")
	m.fallosForzados = 2
	store := dynamoAdapter.NewPreferencesStore(m, e068Tabla)

	prefs, err := store.UpdatePreferences(context.Background(), e068UserA, appPreferences.Update{
		Locale:          "en-US",
		Timezone:        "Europe/Madrid",
		ExpectedVersion: 3,
		HasVersion:      true,
		Now:             e069Actualizado,
	})
	if err != nil {
		t.Fatalf("el reintento debe converger: %v", err)
	}
	if prefs.Version != 4 {
		t.Fatalf("la version debe incrementarse una sola vez, obtenido %d", prefs.Version)
	}
	if m.updates != 3 {
		t.Fatalf("se esperaban 3 intentos, hubo %d", m.updates)
	}
}

// TestE069StoreAgotaReintentosCon409: si la condicion falla sin que la version observable
// cambie, tras UpdateAttempts intentos se devuelve conflicto (409) y el item queda intacto.
func TestE069StoreAgotaReintentosCon409(t *testing.T) {
	m := newMemoriaPreferencias()
	sembrarPreferenciasE069(m, e068UserA, 3, "es-MX", "America/Mexico_City")
	m.fallosForzados = dynamoAdapter.UpdateAttempts
	store := dynamoAdapter.NewPreferencesStore(m, e068Tabla)

	_, err := store.UpdatePreferences(context.Background(), e068UserA, appPreferences.Update{
		Locale:          "en-US",
		Timezone:        "Europe/Madrid",
		ExpectedVersion: 3,
		HasVersion:      true,
		Now:             e069Actualizado,
	})
	if !errors.Is(err, appPreferences.ErrPreferencesConflict) {
		t.Fatalf("agotar reintentos debe ser conflicto, obtenido %v", err)
	}
	if m.updates != dynamoAdapter.UpdateAttempts {
		t.Fatalf("se esperaban %d intentos, hubo %d", dynamoAdapter.UpdateAttempts, m.updates)
	}
	pk, sk := domainDynamo.PrefixUser+e068UserA, e068SKAjust
	if mapaString(m.items[claveDe(pk, sk)], "locale") != "es-MX" {
		t.Fatal("un 409 no puede escribir parcialmente")
	}
}

// TestE069StoreItemSinVersionUsaCondicionDeAusencia: un item historico sin `version` se
// actualiza con attribute_not_exists(#version) y sin declarar :expected (que no se usaria).
func TestE069StoreItemSinVersionUsaCondicionDeAusencia(t *testing.T) {
	m := newMemoriaPreferencias()
	pk, sk := domainDynamo.PrefixUser+e068UserA, e068SKAjust
	m.items[claveDe(pk, sk)] = map[string]types.AttributeValue{
		"PK":         strAttr(pk),
		"SK":         strAttr(sk),
		"entityType": strAttr("USER_PREFERENCES"),
		"userId":     strAttr(e068UserA),
		"locale":     strAttr("es-MX"),
		"timezone":   strAttr("America/Mexico_City"),
	}
	store := dynamoAdapter.NewPreferencesStore(m, e068Tabla)

	prefs, err := store.UpdatePreferences(context.Background(), e068UserA, appPreferences.Update{
		Locale:          "en-US",
		Timezone:        "Europe/Madrid",
		ExpectedVersion: appPreferences.DefaultVersion,
		HasVersion:      false,
		Now:             e069Actualizado,
	})
	if err != nil {
		t.Fatalf("un item sin version debe poder actualizarse: %v", err)
	}
	if prefs.Version != appPreferences.DefaultVersion || !prefs.HasVersion {
		t.Fatalf("la version debe inicializarse a 1: %+v", prefs)
	}
	entrada := m.ultimoUpdate
	if condicion := aws.ToString(entrada.ConditionExpression); condicion != "attribute_not_exists(#version)" {
		t.Fatalf("condicion inesperada: %q", condicion)
	}
	if _, declarado := entrada.ExpressionAttributeValues[":expected"]; declarado {
		t.Fatal(":expected no debe declararse si la condicion no lo usa")
	}
	exigirValoresUsados(t, entrada)
	if version, ok := m.items[claveDe(pk, sk)]["version"].(*types.AttributeValueMemberN); !ok || version.Value != "1" {
		t.Fatalf("version debe quedar en 1: %v", m.items[claveDe(pk, sk)]["version"])
	}
}

// TestE069StoreConcurrenciaSoloUnoGana: diez CAS con la misma version esperada producen una
// sola escritura; el resto recibe 412 y la version final es 2.
func TestE069StoreConcurrenciaSoloUnoGana(t *testing.T) {
	m := newMemoriaPreferencias()
	sembrarPreferenciasE069(m, e068UserA, 1, "es-MX", "America/Mexico_City")
	store := dynamoAdapter.NewPreferencesStore(m, e068Tabla)

	const escritores = 10
	var wg sync.WaitGroup
	resultados := make([]*appPreferences.Preferences, escritores)
	errores := make([]error, escritores)
	for i := 0; i < escritores; i++ {
		wg.Add(1)
		go func(indice int) {
			defer wg.Done()
			resultados[indice], errores[indice] = store.UpdatePreferences(context.Background(), e068UserA, appPreferences.Update{
				Locale:          "en-US",
				Timezone:        "Europe/Madrid",
				ExpectedVersion: 1,
				HasVersion:      true,
				Now:             e069Actualizado,
			})
		}(i)
	}
	wg.Wait()

	exitos := 0
	for i := 0; i < escritores; i++ {
		switch {
		case errores[i] == nil:
			exitos++
			if resultados[i].Version != 2 {
				t.Fatalf("el ganador debe devolver version 2: %+v", resultados[i])
			}
		case errors.Is(errores[i], appPreferences.ErrPreferencesVersionConflict):
		default:
			t.Fatalf("escritor %d: error inesperado %v", i, errores[i])
		}
	}
	if exitos != 1 {
		t.Fatalf("solo un CAS puede ganar, ganaron %d", exitos)
	}
	pk, sk := domainDynamo.PrefixUser+e068UserA, e068SKAjust
	if version, ok := m.items[claveDe(pk, sk)]["version"].(*types.AttributeValueMemberN); !ok || version.Value != "2" {
		t.Fatalf("la version final debe ser 2: %v", m.items[claveDe(pk, sk)]["version"])
	}
}

// TestE069StoreItemAusenteEsConflicto: el servicio materializa antes; si aun asi el item no
// existe al escribir, el adaptador responde conflicto en vez de inventar una version.
func TestE069StoreItemAusenteEsConflicto(t *testing.T) {
	m := newMemoriaPreferencias()
	store := dynamoAdapter.NewPreferencesStore(m, e068Tabla)

	_, err := store.UpdatePreferences(context.Background(), e068UserA, appPreferences.Update{
		Locale:          "en-US",
		Timezone:        "Europe/Madrid",
		ExpectedVersion: 1,
		HasVersion:      true,
		Now:             e069Actualizado,
	})
	if !errors.Is(err, appPreferences.ErrPreferencesConflict) {
		t.Fatalf("item ausente con CAS debe ser conflicto, obtenido %v", err)
	}
	if _, existe := m.items[claveDe(domainDynamo.PrefixUser+e068UserA, e068SKAjust)]; existe {
		t.Fatal("un 409 no puede crear el item")
	}
}

// TestE069StoreFallaCerrado: sin cliente, sin tabla o con error del servicio no hay escritura
// inventada.
func TestE069StoreFallaCerrado(t *testing.T) {
	update := appPreferences.Update{Locale: "en-US", Timezone: "Europe/Madrid", ExpectedVersion: 1, HasVersion: true, Now: e069Actualizado}
	if _, err := dynamoAdapter.NewPreferencesStore(nil, "").UpdatePreferences(context.Background(), e068UserA, update); !errors.Is(err, appPreferences.ErrPreferencesUnavailable) {
		t.Fatalf("sin cliente debe fallar cerrado, obtenido %v", err)
	}
	m := newMemoriaPreferencias()
	if _, err := dynamoAdapter.NewPreferencesStore(m, "").UpdatePreferences(context.Background(), e068UserA, update); !errors.Is(err, appPreferences.ErrPreferencesUnavailable) {
		t.Fatalf("sin tabla debe fallar cerrado, obtenido %v", err)
	}
	m = newMemoriaPreferencias()
	m.fallo = errors.New("dynamo no disponible")
	if _, err := dynamoAdapter.NewPreferencesStore(m, e068Tabla).UpdatePreferences(context.Background(), e068UserA, update); !errors.Is(err, appPreferences.ErrPreferencesUnavailable) {
		t.Fatalf("error de servicio debe ser unavailable, obtenido %v", err)
	}
}

// TestE069StoreAislaUsuarios: actualizar A no crea ni modifica la clave de B.
func TestE069StoreAislaUsuarios(t *testing.T) {
	m := newMemoriaPreferencias()
	sembrarPreferenciasE069(m, e068UserA, 1, "es-MX", "America/Mexico_City")
	sembrarPreferenciasE069(m, e068UserB, 1, "en-US", "Europe/Madrid")
	store := dynamoAdapter.NewPreferencesStore(m, e068Tabla)

	if _, err := store.UpdatePreferences(context.Background(), e068UserA, appPreferences.Update{
		Locale:          "en-US",
		Timezone:        "America/New_York",
		ExpectedVersion: 1,
		HasVersion:      true,
		Now:             e069Actualizado,
	}); err != nil {
		t.Fatalf("actualizacion de A fallo: %v", err)
	}
	pkA, skA := domainDynamo.PrefixUser+e068UserA, e068SKAjust
	pkB, skB := domainDynamo.PrefixUser+e068UserB, e068SKAjust
	if mapaString(m.items[claveDe(pkA, skA)], "timezone") != "America/New_York" {
		t.Fatal("A debe reflejar su cambio")
	}
	if mapaString(m.items[claveDe(pkB, skB)], "timezone") != "Europe/Madrid" || mapaString(m.items[claveDe(pkB, skB)], "locale") != "en-US" {
		t.Fatal("el item de B no puede cambiar")
	}
	if version, ok := m.items[claveDe(pkB, skB)]["version"].(*types.AttributeValueMemberN); !ok || version.Value != "1" {
		t.Fatal("la version de B no puede incrementarse")
	}
}

// TestE069StoreNoDeclaraValoresSinUsar: en ambas ramas de versión (item normal e histórico) todo
// name y todo value declarado se usa en la expresión. DynamoDB rechaza con ValidationException
// un valor sobrante y el adaptador lo traduciría a 503 en todas las escrituras.
func TestE069StoreNoDeclaraValoresSinUsar(t *testing.T) {
	casos := []struct {
		nombre     string
		hasVersion bool
	}{
		{"item con version", true},
		{"item historico sin version", false},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			m := newMemoriaPreferencias()
			pk, sk := domainDynamo.PrefixUser+e068UserA, e068SKAjust
			item := map[string]types.AttributeValue{
				"PK":         strAttr(pk),
				"SK":         strAttr(sk),
				"entityType": strAttr("USER_PREFERENCES"),
				"userId":     strAttr(e068UserA),
				"locale":     strAttr("es-MX"),
				"timezone":   strAttr("America/Mexico_City"),
			}
			if caso.hasVersion {
				item["version"] = numAttr(4)
			}
			m.items[claveDe(pk, sk)] = item
			store := dynamoAdapter.NewPreferencesStore(m, e068Tabla)

			esperada := int64(4)
			if !caso.hasVersion {
				// Un item histórico sin `version` se observa en la versión inicial (1).
				esperada = appPreferences.DefaultVersion
			}
			_, err := store.UpdatePreferences(context.Background(), e068UserA, appPreferences.Update{
				Locale:          "en-US",
				Timezone:        "Europe/Madrid",
				ExpectedVersion: esperada,
				HasVersion:      caso.hasVersion,
				Now:             e069Actualizado,
			})
			if err != nil {
				t.Fatalf("actualizacion fallo: %v", err)
			}
			exigirValoresUsados(t, m.ultimoUpdate)
			if _, declarado := m.ultimoUpdate.ExpressionAttributeValues[":expected"]; declarado != caso.hasVersion {
				t.Fatalf(":expected declarado=%v, esperado=%v", declarado, caso.hasVersion)
			}
		})
	}
}
