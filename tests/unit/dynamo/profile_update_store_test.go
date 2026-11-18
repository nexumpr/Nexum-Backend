package dynamo_test

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	dynamoAdapter "github.com/nexumpr/Nexum-Backend/internal/adapters/dynamo"
	appProfile "github.com/nexumpr/Nexum-Backend/internal/application/profile"
)

// Pruebas del adaptador de escritura de E013 sobre un doble con estado que interpreta la
// MISMA condición y la MISMA expresión que emite el adaptador. El doble es exclusivamente de
// prueba: la composición productiva usa el cliente real de DynamoDB.

const (
	e013Tabla   = "NEXUM_CORE_TEST"
	e013Usuario = "33333333-4444-4555-8666-777777777777"
	e013Otro    = "77777777-6666-4555-8444-333333333333"
)

var e013Ahora = time.Date(2026, 11, 18, 11, 15, 0, 0, time.UTC)

// memoriaPerfil interpreta `UpdateItem` con condición y `ReturnValues=ALL_NEW`. Si aparece
// una condición o una expresión no soportada, el test falla en lugar de adivinar.
type memoriaPerfil struct {
	mu              sync.Mutex
	items           map[string]map[string]types.AttributeValue
	entradas        []*dynamodb.UpdateItemInput
	errores         []error
	atributosVacios bool
}

func newMemoriaPerfil() *memoriaPerfil {
	return &memoriaPerfil{items: make(map[string]map[string]types.AttributeValue)}
}

func (m *memoriaPerfil) sembrar(pk, sk string, attrs map[string]types.AttributeValue) {
	m.mu.Lock()
	defer m.mu.Unlock()
	item := map[string]types.AttributeValue{"PK": strAttr(pk), "SK": strAttr(sk)}
	for k, v := range attrs {
		item[k] = v
	}
	m.items[claveDe(pk, sk)] = item
}

func (m *memoriaPerfil) item(pk, sk string) map[string]types.AttributeValue {
	m.mu.Lock()
	defer m.mu.Unlock()
	guardado, ok := m.items[claveDe(pk, sk)]
	if !ok {
		return nil
	}
	return copiarItem(guardado)
}

func (m *memoriaPerfil) ultimaEntrada() *dynamodb.UpdateItemInput {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.entradas) == 0 {
		return nil
	}
	return m.entradas[len(m.entradas)-1]
}

func (m *memoriaPerfil) veces() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.entradas)
}

func (m *memoriaPerfil) UpdateItem(_ context.Context, params *dynamodb.UpdateItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entradas = append(m.entradas, params)

	if len(m.errores) > 0 {
		err := m.errores[0]
		m.errores = m.errores[1:]
		return nil, err
	}

	pk := mapaString(params.Key, "PK")
	sk := mapaString(params.Key, "SK")
	clave := claveDe(pk, sk)
	item := m.items[clave]

	if params.ConditionExpression != nil {
		ok, err := evaluarCondicion(aws.ToString(params.ConditionExpression), item, params.ExpressionAttributeNames, params.ExpressionAttributeValues)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, &types.ConditionalCheckFailedException{Message: aws.String("The conditional request failed")}
		}
	}

	if item == nil {
		item = map[string]types.AttributeValue{"PK": strAttr(pk), "SK": strAttr(sk)}
	}
	aplicarExpresionDeUpdate(item, params.UpdateExpression, params.ExpressionAttributeNames, params.ExpressionAttributeValues)
	m.items[clave] = item

	if m.atributosVacios || params.ReturnValues != types.ReturnValueAllNew {
		return &dynamodb.UpdateItemOutput{}, nil
	}
	return &dynamodb.UpdateItemOutput{Attributes: copiarItem(item)}, nil
}

// aplicarExpresionDeUpdate aplica SET y ADD con la gramática que emite el adaptador.
func aplicarExpresionDeUpdate(item map[string]types.AttributeValue, expresion *string, nombres map[string]string, valores map[string]types.AttributeValue) {
	set, add := partirExpresion(aws.ToString(expresion))
	for _, asignacion := range set {
		partes := strings.SplitN(asignacion, " = ", 2)
		if len(partes) != 2 {
			continue
		}
		item[resolverNombre(partes[0], nombres)] = valores[partes[1]]
	}
	for _, incremento := range add {
		partes := strings.SplitN(strings.TrimSpace(incremento), " ", 2)
		if len(partes) != 2 {
			continue
		}
		nombre := resolverNombre(partes[0], nombres)
		delta, okDelta := valores[strings.TrimSpace(partes[1])].(*types.AttributeValueMemberN)
		if !okDelta {
			continue
		}
		var base int64
		if actual, okActual := item[nombre].(*types.AttributeValueMemberN); okActual {
			base, _ = strconv.ParseInt(actual.Value, 10, 64)
		}
		salto, _ := strconv.ParseInt(delta.Value, 10, 64)
		item[nombre] = numAttr(base + salto)
	}
}

// e013SembrarPerfil escribe el item con la MISMA forma que produce E001, incluidos los
// atributos internos que E013 no debe tocar.
func e013SembrarPerfil(mem *memoriaPerfil, userID string, version int64) {
	mem.sembrar("USER#"+userID, "PROFILE", map[string]types.AttributeValue{
		"entityType":           strAttr("USER_PROFILE"),
		"userId":               strAttr(userID),
		"email":                strAttr("ejemplo@example.invalid"),
		"displayName":          strAttr("Usuario Ejemplo"),
		"passwordHash":         strAttr("$argon2id$v=19$m=65536,t=3,p=2$hash-sintetico"),
		"emailVerified":        boolAttr(true),
		"sessionEpoch":         numAttr(3),
		"status":               strAttr("ACTIVE"),
		"privacyNoticeVersion": strAttr("2026-11-v2"),
		"privacyAcceptedAt":    strAttr("2026-11-01T00:00:00Z"),
		"createdAt":            strAttr("2026-11-01T12:00:00Z"),
		"updatedAt":            strAttr("2026-11-01T12:00:00Z"),
		"version":              numAttr(version),
	})
}

func e013Entrada(userID string, version int64, conVersion bool) appProfile.UpdateInput {
	return appProfile.UpdateInput{
		UserID:            userID,
		DisplayName:       "Nombre Nuevo",
		ExpectedVersion:   version,
		ProfileHasVersion: conVersion,
		Now:               e013Ahora,
	}
}

// TestE013StoreAplicaCASYConservaCamposInternos es la prueba central: un único UpdateItem
// condicionado incrementa la versión y cambia el nombre, sin tocar correo, hash, estado de
// verificación, época, estado de cuenta ni privacidad.
func TestE013StoreAplicaCASYConservaCamposInternos(t *testing.T) {
	mem := newMemoriaPerfil()
	e013SembrarPerfil(mem, e013Usuario, 4)
	store := dynamoAdapter.NewProfileUpdateStore(mem, e013Tabla)

	registro, err := store.UpdateProfile(context.Background(), e013Entrada(e013Usuario, 4, true))
	if err != nil {
		t.Fatalf("UpdateProfile devolvio error: %v", err)
	}
	if registro == nil {
		t.Fatal("el item escrito debe devolverse con ALL_NEW")
	}
	if registro.DisplayName != "Nombre Nuevo" || !registro.HasVersion || registro.Version != 5 {
		t.Fatalf("proyeccion inesperada: %+v", registro)
	}
	if registro.Email != "ejemplo@example.invalid" || !registro.EmailVerified || !registro.HasCreatedAt {
		t.Fatalf("los campos no modificados deben conservarse: %+v", registro)
	}

	item := mem.item("USER#"+e013Usuario, "PROFILE")
	if stringAttrDe(item, "displayName") != "Nombre Nuevo" {
		t.Fatalf("displayName no persistido: %v", stringAttrDe(item, "displayName"))
	}
	if stringAttrDe(item, "updatedAt") != "2026-11-18T11:15:00Z" {
		t.Fatalf("updatedAt inesperado: %v", stringAttrDe(item, "updatedAt"))
	}
	if version, _ := attributeIntDe(item, "version"); version != 5 {
		t.Fatalf("version debe incrementarse a 5, es %d", version)
	}
	// Ningún atributo interno cambia por la escritura.
	if stringAttrDe(item, "email") != "ejemplo@example.invalid" ||
		stringAttrDe(item, "passwordHash") != "$argon2id$v=19$m=65536,t=3,p=2$hash-sintetico" ||
		stringAttrDe(item, "status") != "ACTIVE" ||
		stringAttrDe(item, "privacyNoticeVersion") != "2026-11-v2" {
		t.Fatalf("un atributo interno fue alterado: %+v", item)
	}
	if epoch, _ := attributeIntDe(item, "sessionEpoch"); epoch != 3 {
		t.Fatalf("sessionEpoch no debe cambiar, es %d", epoch)
	}
	if verificado, ok := item["emailVerified"].(*types.AttributeValueMemberBOOL); !ok || !verificado.Value {
		t.Fatal("emailVerified no debe cambiar")
	}

	entrada := mem.ultimaEntrada()
	if aws.ToString(entrada.TableName) != e013Tabla {
		t.Fatalf("tabla inesperada: %q", aws.ToString(entrada.TableName))
	}
	if mapaString(entrada.Key, "PK") != "USER#"+e013Usuario || mapaString(entrada.Key, "SK") != "PROFILE" {
		t.Fatalf("clave inesperada: %v", entrada.Key)
	}
	if entrada.ReturnValues != types.ReturnValueAllNew {
		t.Fatalf("ReturnValues debe ser ALL_NEW, es %q", entrada.ReturnValues)
	}
	if got := aws.ToString(entrada.ConditionExpression); got != "attribute_exists(PK) AND #version = :expected" {
		t.Fatalf("condicion inesperada: %q", got)
	}
	if got := aws.ToString(entrada.UpdateExpression); got != "SET displayName = :name, updatedAt = :now ADD #version :one" {
		t.Fatalf("expresion inesperada: %q", got)
	}
	if entrada.ExpressionAttributeNames["#version"] != "version" {
		t.Fatalf("el alias #version debe resolver a version: %v", entrada.ExpressionAttributeNames)
	}
}

// TestE013StoreVersionDistintaNoEscribe cubre 412: la condición falla y el item queda igual.
func TestE013StoreVersionDistintaNoEscribe(t *testing.T) {
	mem := newMemoriaPerfil()
	e013SembrarPerfil(mem, e013Usuario, 4)
	store := dynamoAdapter.NewProfileUpdateStore(mem, e013Tabla)

	_, err := store.UpdateProfile(context.Background(), e013Entrada(e013Usuario, 3, true))
	if !errors.Is(err, appProfile.ErrProfileCASFailed) {
		t.Fatalf("esperado ErrProfileCASFailed, obtenido %v", err)
	}
	item := mem.item("USER#"+e013Usuario, "PROFILE")
	if stringAttrDe(item, "displayName") != "Usuario Ejemplo" {
		t.Fatal("una condicion fallida no debe escribir")
	}
	if version, _ := attributeIntDe(item, "version"); version != 4 {
		t.Fatalf("la version no debe cambiar, es %d", version)
	}
	if mem.veces() != 1 {
		t.Fatalf("debe hacerse un unico intento, hubo %d", mem.veces())
	}
}

// TestE013StorePerfilInexistenteNoCrea: la condición `attribute_exists(PK)` impide crear el
// item y no se inventa un perfil.
func TestE013StorePerfilInexistenteNoCrea(t *testing.T) {
	mem := newMemoriaPerfil()
	store := dynamoAdapter.NewProfileUpdateStore(mem, e013Tabla)

	_, err := store.UpdateProfile(context.Background(), e013Entrada(e013Otro, 1, true))
	if !errors.Is(err, appProfile.ErrProfileCASFailed) {
		t.Fatalf("esperado ErrProfileCASFailed, obtenido %v", err)
	}
	if item := mem.item("USER#"+e013Otro, "PROFILE"); item != nil {
		t.Fatalf("no debe crearse el perfil: %v", item)
	}
}

// TestE013StorePerfilHeredadoInicializaVersion: un item sin `version` (expuesto como v1 por
// E012) se actualiza con CAS de ausencia y sale con version 1.
func TestE013StorePerfilHeredadoInicializaVersion(t *testing.T) {
	mem := newMemoriaPerfil()
	mem.sembrar("USER#"+e013Usuario, "PROFILE", map[string]types.AttributeValue{
		"entityType":  strAttr("USER_PROFILE"),
		"userId":      strAttr(e013Usuario),
		"email":       strAttr("ejemplo@example.invalid"),
		"displayName": strAttr("Usuario Ejemplo"),
		"createdAt":   strAttr("2026-11-01T12:00:00Z"),
	})
	store := dynamoAdapter.NewProfileUpdateStore(mem, e013Tabla)

	registro, err := store.UpdateProfile(context.Background(), e013Entrada(e013Usuario, 1, false))
	if err != nil {
		t.Fatalf("UpdateProfile devolvio error: %v", err)
	}
	if !registro.HasVersion || registro.Version != 1 || registro.DisplayName != "Nombre Nuevo" {
		t.Fatalf("el item heredado debe quedar en v1 con el nombre nuevo: %+v", registro)
	}
	entrada := mem.ultimaEntrada()
	if got := aws.ToString(entrada.ConditionExpression); got != "attribute_exists(PK) AND attribute_not_exists(#version)" {
		t.Fatalf("condicion heredada inesperada: %q", got)
	}
	if got := aws.ToString(entrada.UpdateExpression); got != "SET displayName = :name, updatedAt = :now, #version = :one" {
		t.Fatalf("expresion heredada inesperada: %q", got)
	}
	if _, declarado := entrada.ExpressionAttributeValues[":expected"]; declarado {
		t.Fatal(":expected no debe declararse si la condicion no lo usa")
	}
}

// TestE013StoreValoresDeExpresionTodosUsados reproduce la clase de defecto de E003: DynamoDB
// rechaza cualquier valor de expresión declarado que no aparezca en la condición o en la
// actualización. Se comprueba en las dos formas (con y sin `version`).
func TestE013StoreValoresDeExpresionTodosUsados(t *testing.T) {
	casos := []struct {
		nombre     string
		conVersion bool
	}{
		{"perfil con version", true},
		{"perfil heredado", false},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			mem := newMemoriaPerfil()
			if caso.conVersion {
				e013SembrarPerfil(mem, e013Usuario, 4)
			} else {
				mem.sembrar("USER#"+e013Usuario, "PROFILE", map[string]types.AttributeValue{
					"entityType":  strAttr("USER_PROFILE"),
					"userId":      strAttr(e013Usuario),
					"displayName": strAttr("Usuario Ejemplo"),
					"createdAt":   strAttr("2026-11-01T12:00:00Z"),
				})
			}
			store := dynamoAdapter.NewProfileUpdateStore(mem, e013Tabla)
			if _, err := store.UpdateProfile(context.Background(), e013Entrada(e013Usuario, 4, caso.conVersion)); err != nil {
				t.Fatalf("UpdateProfile devolvio error: %v", err)
			}
			entrada := mem.ultimaEntrada()
			expresion := aws.ToString(entrada.ConditionExpression) + " " + aws.ToString(entrada.UpdateExpression)
			for valor := range entrada.ExpressionAttributeValues {
				if !strings.Contains(expresion, valor) {
					t.Fatalf("valor de expresion declarado y no usado: %q en %q", valor, expresion)
				}
			}
			for _, token := range tokensDeExpresion(expresion) {
				if _, ok := entrada.ExpressionAttributeValues[token]; !ok {
					t.Fatalf("token %q sin valor declarado", token)
				}
			}
		})
	}
}

// TestE013StoreFallaCerrado: sin cliente, sin tabla o con datos incompletos no se escribe.
func TestE013StoreFallaCerrado(t *testing.T) {
	ctx := context.Background()
	if _, err := dynamoAdapter.NewProfileUpdateStore(nil, e013Tabla).UpdateProfile(ctx, e013Entrada(e013Usuario, 1, true)); !errors.Is(err, appProfile.ErrProfileUnavailable) {
		t.Fatalf("sin cliente debe fallar cerrado, obtuvo %v", err)
	}
	mem := newMemoriaPerfil()
	if _, err := dynamoAdapter.NewProfileUpdateStore(mem, "  ").UpdateProfile(ctx, e013Entrada(e013Usuario, 1, true)); !errors.Is(err, appProfile.ErrProfileUnavailable) {
		t.Fatalf("sin tabla debe fallar cerrado, obtuvo %v", err)
	}
	store := dynamoAdapter.NewProfileUpdateStore(mem, e013Tabla)
	if _, err := store.UpdateProfile(ctx, e013Entrada("   ", 1, true)); !errors.Is(err, appProfile.ErrProfileUnavailable) {
		t.Fatalf("sin usuario debe fallar cerrado, obtuvo %v", err)
	}
	sinNombre := e013Entrada(e013Usuario, 1, true)
	sinNombre.DisplayName = "   "
	if _, err := store.UpdateProfile(ctx, sinNombre); !errors.Is(err, appProfile.ErrProfileUnavailable) {
		t.Fatalf("sin nombre debe fallar cerrado, obtuvo %v", err)
	}
	sinVersion := e013Entrada(e013Usuario, 0, true)
	if _, err := store.UpdateProfile(ctx, sinVersion); !errors.Is(err, appProfile.ErrProfileUnavailable) {
		t.Fatalf("sin version esperada debe fallar cerrado, obtuvo %v", err)
	}
	if mem.veces() != 0 {
		t.Fatalf("ningun caso debe llegar a DynamoDB: %d llamadas", mem.veces())
	}
}

// TestE013StoreErroresDeDynamo: un fallo generic es 503 y una respuesta sin atributos es
// incoherente (500); nunca se emite un perfil inventado.
func TestE013StoreErroresDeDynamo(t *testing.T) {
	mem := newMemoriaPerfil()
	e013SembrarPerfil(mem, e013Usuario, 4)
	mem.errores = []error{errors.New("dynamodb no disponible")}
	store := dynamoAdapter.NewProfileUpdateStore(mem, e013Tabla)

	if _, err := store.UpdateProfile(context.Background(), e013Entrada(e013Usuario, 4, true)); !errors.Is(err, appProfile.ErrProfileUnavailable) {
		t.Fatalf("un fallo de DynamoDB debe ser 503, obtuvo %v", err)
	}

	vacio := newMemoriaPerfil()
	e013SembrarPerfil(vacio, e013Usuario, 4)
	vacio.atributosVacios = true
	if _, err := dynamoAdapter.NewProfileUpdateStore(vacio, e013Tabla).UpdateProfile(context.Background(), e013Entrada(e013Usuario, 4, true)); !errors.Is(err, appProfile.ErrProfileInternal) {
		t.Fatalf("ALL_NEW sin atributos debe ser incoherente, obtuvo %v", err)
	}
}

// TestE013StoreEntidadAjenaEsIncoherente: otra entidad en la misma clave no se presenta como
// perfil aunque la condición se cumpla.
func TestE013StoreEntidadAjenaEsIncoherente(t *testing.T) {
	mem := newMemoriaPerfil()
	mem.sembrar("USER#"+e013Otro, "PROFILE", map[string]types.AttributeValue{
		"entityType": strAttr("USER_SESSION"),
		"version":    numAttr(2),
	})
	store := dynamoAdapter.NewProfileUpdateStore(mem, e013Tabla)

	if _, err := store.UpdateProfile(context.Background(), e013Entrada(e013Otro, 2, true)); !errors.Is(err, appProfile.ErrProfileInternal) {
		t.Fatalf("una entidad ajena debe ser incoherente, obtuvo %v", err)
	}
}

// tokensDeExpresion extrae los marcadores `:valor` de una expresión.
func tokensDeExpresion(expresion string) []string {
	var tokens []string
	vistos := map[string]bool{}
	campos := strings.FieldsFunc(expresion, func(r rune) bool {
		return r == ' ' || r == ',' || r == '(' || r == ')'
	})
	for _, campo := range campos {
		if len(campo) < 2 || campo[0] != ':' || vistos[campo] {
			continue
		}
		vistos[campo] = true
		tokens = append(tokens, campo)
	}
	return tokens
}
