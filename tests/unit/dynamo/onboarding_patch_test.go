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

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	dynamoAdapter "github.com/nexumpr/Nexum-Backend/internal/adapters/dynamo"
	appOnboarding "github.com/nexumpr/Nexum-Backend/internal/application/onboarding"
)

// Pruebas de E019 sobre el adaptador REAL `OnboardingStore` montado sobre un doble
// con estado que interpreta EXACTAMENTE las expresiones emitidas (condición de CAS
// y `SET ... ADD version :uno`). No es un motor DynamoDB: una expresión no
// soportada falla el test en vez de adivinar, y los valores de expresión declarados
// sin uso se detectan como error, igual que el servicio real.

const e019Tabla = "NEXUM_CORE_TEST"

const (
	e019Usuario = "11111111-2222-4333-8444-555555555555"
	e019Otro    = "99999999-8888-4777-8666-555555555555"
)

var e019Ahora = time.Date(2026, 11, 18, 18, 0, 0, 0, time.UTC)

// e019Dynamo doble con estado, CAS por ítem y captura de las entradas emitidas.
type e019Dynamo struct {
	mu       sync.Mutex
	items    map[string]map[string]types.AttributeValue
	puts     []*dynamodb.PutItemInput
	updates  []*dynamodb.UpdateItemInput
	falloGet error
	falloPut error
	falloUpd error
}

func newE019Dynamo() *e019Dynamo {
	return &e019Dynamo{items: map[string]map[string]types.AttributeValue{}}
}

func copiarItemE019(item map[string]types.AttributeValue) map[string]types.AttributeValue {
	if item == nil {
		return nil
	}
	copia := make(map[string]types.AttributeValue, len(item))
	for clave, valor := range item {
		copia[clave] = valor
	}
	return copia
}

func (d *e019Dynamo) sembrar(pk, sk string, atributos map[string]types.AttributeValue) {
	d.mu.Lock()
	defer d.mu.Unlock()
	item := map[string]types.AttributeValue{"PK": strAttr(pk), "SK": strAttr(sk)}
	for clave, valor := range atributos {
		item[clave] = valor
	}
	d.items[claveDe(pk, sk)] = item
}

func (d *e019Dynamo) leer(pk, sk string) map[string]types.AttributeValue {
	d.mu.Lock()
	defer d.mu.Unlock()
	return copiarItemE019(d.items[claveDe(pk, sk)])
}

func (d *e019Dynamo) GetItem(_ context.Context, params *dynamodb.GetItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.falloGet != nil {
		return nil, d.falloGet
	}
	item := d.items[claveDe(mapaString(params.Key, "PK"), mapaString(params.Key, "SK"))]
	if item == nil {
		return &dynamodb.GetItemOutput{}, nil
	}
	return &dynamodb.GetItemOutput{Item: copiarItemE019(item)}, nil
}

func (d *e019Dynamo) PutItem(_ context.Context, params *dynamodb.PutItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.puts = append(d.puts, params)
	if d.falloPut != nil {
		return nil, d.falloPut
	}
	pk := mapaString(params.Item, "PK")
	sk := mapaString(params.Item, "SK")
	existente := d.items[claveDe(pk, sk)]
	ok, err := evaluarCondicionE019(exprDe(params.ConditionExpression), existente, params.ExpressionAttributeNames, params.ExpressionAttributeValues)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, condicionFallida()
	}
	d.items[claveDe(pk, sk)] = copiarItemE019(params.Item)
	return &dynamodb.PutItemOutput{}, nil
}

func (d *e019Dynamo) UpdateItem(_ context.Context, params *dynamodb.UpdateItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.updates = append(d.updates, params)
	if d.falloUpd != nil {
		return nil, d.falloUpd
	}
	pk := mapaString(params.Key, "PK")
	sk := mapaString(params.Key, "SK")
	item := d.items[claveDe(pk, sk)]
	ok, err := evaluarCondicionE019(exprDe(params.ConditionExpression), item, params.ExpressionAttributeNames, params.ExpressionAttributeValues)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, condicionFallida()
	}
	actualizado := copiarItemE019(item)
	if actualizado == nil {
		actualizado = map[string]types.AttributeValue{"PK": strAttr(pk), "SK": strAttr(sk)}
	}
	if err := aplicarUpdateE019(exprDe(params.UpdateExpression), actualizado, params.ExpressionAttributeNames, params.ExpressionAttributeValues); err != nil {
		return nil, err
	}
	d.items[claveDe(pk, sk)] = actualizado
	return &dynamodb.UpdateItemOutput{}, nil
}

func condicionFallida() error {
	return &types.ConditionalCheckFailedException{Message: stringPtr("condicion fallida")}
}

func stringPtr(valor string) *string { return &valor }

func exprDe(valor *string) string {
	if valor == nil {
		return ""
	}
	return *valor
}

// resolverE019 traduce un alias `#x` a su nombre de atributo.
func resolverE019(token string, nombres map[string]string) string {
	if alias, ok := nombres[token]; ok {
		return alias
	}
	return token
}

// evaluarCondicionE019 soporta exactamente las dos formas que emite el adaptador:
// `attribute_not_exists(#x)` y `#x = :valor`.
func evaluarCondicionE019(expr string, item map[string]types.AttributeValue, nombres map[string]string, valores map[string]types.AttributeValue) (bool, error) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return true, nil
	}
	switch {
	case strings.HasPrefix(expr, "attribute_not_exists("):
		nombre := resolverE019(strings.TrimSuffix(strings.TrimPrefix(expr, "attribute_not_exists("), ")"), nombres)
		_, existe := item[nombre]
		return !existe, nil
	default:
		partes := strings.SplitN(expr, " = ", 2)
		if len(partes) != 2 {
			return false, errors.New("condicion no soportada por el doble de E019: " + expr)
		}
		nombre := resolverE019(partes[0], nombres)
		esperado, ok := valores[partes[1]]
		if !ok {
			return false, errors.New("valor de condicion ausente: " + partes[1])
		}
		actual, existe := item[nombre]
		if !existe {
			return false, nil
		}
		return atributosIguales(actual, esperado), nil
	}
}

// aplicarUpdateE019 interpreta `SET a = :a, ... ADD #version :uno`.
func aplicarUpdateE019(expr string, item map[string]types.AttributeValue, nombres map[string]string, valores map[string]types.AttributeValue) error {
	resto := strings.TrimSpace(expr)
	if !strings.HasPrefix(resto, "SET ") {
		return errors.New("update sin SET no soportado por el doble de E019: " + expr)
	}
	resto = strings.TrimPrefix(resto, "SET ")
	indice := strings.Index(resto, " ADD ")
	if indice < 0 {
		return errors.New("update sin ADD no soportado por el doble de E019: " + expr)
	}
	for _, asignacion := range strings.Split(resto[:indice], ", ") {
		partes := strings.SplitN(asignacion, " = ", 2)
		if len(partes) != 2 {
			return errors.New("asignacion no soportada por el doble de E019: " + asignacion)
		}
		valor, ok := valores[partes[1]]
		if !ok {
			return errors.New("valor de update ausente: " + partes[1])
		}
		item[resolverE019(partes[0], nombres)] = valor
	}

	incremento := strings.TrimSpace(resto[indice+len(" ADD "):])
	partes := strings.SplitN(incremento, " ", 2)
	if len(partes) != 2 {
		return errors.New("ADD no soportado por el doble de E019: " + incremento)
	}
	delta, ok := valores[partes[1]]
	if !ok {
		return errors.New("valor de ADD ausente: " + partes[1])
	}
	deltaN, okDelta := delta.(*types.AttributeValueMemberN)
	if !okDelta {
		return errors.New("ADD no numerico")
	}
	salto, err := strconv.ParseInt(deltaN.Value, 10, 64)
	if err != nil {
		return err
	}
	nombre := resolverE019(partes[0], nombres)
	var base int64
	if actual, existe := item[nombre].(*types.AttributeValueMemberN); existe {
		base, _ = strconv.ParseInt(actual.Value, 10, 64)
	}
	item[nombre] = numAttr(base + salto)
	return nil
}

// tokensE019 devuelve los alias y valores referenciados por una expresión.
func tokensE019(expr string) (nombres []string, valores []string) {
	for i := 0; i < len(expr); i++ {
		if expr[i] != '#' && expr[i] != ':' {
			continue
		}
		j := i + 1
		for j < len(expr) && (esIdentificador(expr[j])) {
			j++
		}
		token := expr[i:j]
		if expr[i] == '#' {
			nombres = append(nombres, token)
		} else {
			valores = append(valores, token)
		}
		i = j - 1
	}
	return nombres, valores
}

func esIdentificador(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}

func e019Escribir() appOnboarding.Write {
	return appOnboarding.Write{
		CompletedSteps:   []string{"profile", "income"},
		Done:             false,
		MicrophoneRaw:    "denied",
		NotificationsRaw: "granted",
		Configured:       true,
		ExpectedVersion:  3,
		HasVersion:       true,
		Now:              e019Ahora,
	}
}

// TestE019AdaptadorUpdateConCas: la escritura emite `#version = :leida` sobre la
// versión leída, avanza la versión con ADD, persiste los atributos del contrato y
// no sobrescribe `createdAt`.
func TestE019AdaptadorUpdateConCas(t *testing.T) {
	mem := newE019Dynamo()
	mem.sembrar("USER#"+e019Usuario, "ONBOARDING", map[string]types.AttributeValue{
		"entityType":     strAttr("USER_ONBOARDING"),
		"userId":         strAttr(e019Usuario),
		"completedSteps": listaAttr("profile"),
		"microphone":     strAttr("granted"),
		"notifications":  strAttr("pending"),
		"configured":     boolVal(false),
		"createdAt":      strAttr("2026-11-01T00:00:00Z"),
		"updatedAt":      strAttr("2026-11-01T00:00:00Z"),
		"version":        numAttr(3),
	})

	store := dynamoAdapter.NewOnboardingStore(mem, e019Tabla)
	registro, err := store.UpdateOnboarding(context.Background(), e019Usuario, e019Escribir())
	if err != nil {
		t.Fatalf("la escritura valida no debe fallar: %v", err)
	}
	if registro.Version != 4 || !registro.HasVersion {
		t.Fatalf("la version devuelta debe ser 4, obtenida %d", registro.Version)
	}

	persistido := mem.leer("USER#"+e019Usuario, "ONBOARDING")
	if stringAttrValueMap(persistido, "version") != "4" {
		t.Fatalf("la version persistida debe ser 4, es %s", stringAttrValueMap(persistido, "version"))
	}
	if stringAttrValueMap(persistido, "microphone") != "denied" || stringAttrValueMap(persistido, "notifications") != "granted" {
		t.Fatalf("los permisos deben persistirse como texto: mic=%q notif=%q",
			stringAttrValueMap(persistido, "microphone"), stringAttrValueMap(persistido, "notifications"))
	}
	if !boolAtributoE019(persistido["configured"]) {
		t.Fatalf("configured=true debe persistirse")
	}
	if _, esNulo := persistido["completedAt"].(*types.AttributeValueMemberNULL); !esNulo {
		t.Fatalf("completedAt incompleto debe escribirse NULL, obtenido %#v", persistido["completedAt"])
	}
	if stringAttrValueMap(persistido, "createdAt") != "2026-11-01T00:00:00Z" {
		t.Fatalf("createdAt no debe sobrescribirse, es %q", stringAttrValueMap(persistido, "createdAt"))
	}
	if stringAttrValueMap(persistido, "updatedAt") != e019Ahora.Format(time.RFC3339) {
		t.Fatalf("updatedAt debe ser el instante de escritura, es %q", stringAttrValueMap(persistido, "updatedAt"))
	}
	if stringAttrValueMap(persistido, "entityType") != "USER_ONBOARDING" || stringAttrValueMap(persistido, "userId") != e019Usuario {
		t.Fatalf("la entidad y el titular deben quedar fijados")
	}

	if len(mem.updates) != 1 {
		t.Fatalf("debe emitirse una sola escritura, hay %d", len(mem.updates))
	}
	entrada := mem.updates[0]
	if condicion := exprDe(entrada.ConditionExpression); condicion != "#version = :leida" {
		t.Fatalf("condicion esperada '#version = :leida', obtenida %q", condicion)
	}
	leida, ok := entrada.ExpressionAttributeValues[":leida"].(*types.AttributeValueMemberN)
	if !ok || leida.Value != "3" {
		t.Fatalf(":leida debe ser la version leida (3), obtenido %#v", entrada.ExpressionAttributeValues[":leida"])
	}

	// Lectura posterior con E018: mismo item, misma proyeccion.
	leido, encontrado, err := store.GetOnboarding(context.Background(), e019Usuario)
	if err != nil || !encontrado {
		t.Fatalf("la lectura posterior debe encontrar el item: encontrado=%v err=%v", encontrado, err)
	}
	if want := []string{"profile", "income"}; !reflect.DeepEqual(leido.CompletedSteps, want) {
		t.Fatalf("pasos leidos %v, esperados %v", leido.CompletedSteps, want)
	}
	if appOnboarding.IsComplete(leido.CompletedSteps) || leido.CompletedAt != nil {
		t.Fatalf("un onboarding incompleto no debe llevar marca: %+v", leido)
	}
	if leido.MicrophoneRaw != "denied" || leido.NotificationsRaw != "granted" || !leido.Configured {
		t.Fatalf("la lectura posterior debe reflejar la escritura: %+v", leido)
	}
	if leido.Version != 4 || !leido.HasVersion {
		t.Fatalf("la lectura posterior debe ver la version 4, obtenida %d", leido.Version)
	}
}

// TestE019AdaptadorCreacion: sin ítem se crea con versión 1 y condición de
// ausencia de clave.
func TestE019AdaptadorCreacion(t *testing.T) {
	mem := newE019Dynamo()
	store := dynamoAdapter.NewOnboardingStore(mem, e019Tabla)

	escritura := appOnboarding.Write{
		CompletedSteps:   []string{"profile", "income", "privacy", "permissions"},
		Done:             true,
		MicrophoneRaw:    "granted",
		NotificationsRaw: "denied",
		Configured:       true,
		CompletedAt:      timePtrE019(e019Ahora),
		Now:              e019Ahora,
	}
	registro, err := store.CreateOnboarding(context.Background(), e019Usuario, escritura)
	if err != nil {
		t.Fatalf("la creacion valida no debe fallar: %v", err)
	}
	if registro.Version != 1 || !registro.HasVersion {
		t.Fatalf("la version inicial debe ser 1, obtenida %d", registro.Version)
	}

	persistido := mem.leer("USER#"+e019Usuario, "ONBOARDING")
	if stringAttrValueMap(persistido, "version") != "1" {
		t.Fatalf("version persistida esperada 1, es %q", stringAttrValueMap(persistido, "version"))
	}
	if stringAttrValueMap(persistido, "entityType") != "USER_ONBOARDING" {
		t.Fatalf("entityType debe ser USER_ONBOARDING")
	}
	if stringAttrValueMap(persistido, "completedAt") != e019Ahora.Format(time.RFC3339) {
		t.Fatalf("completedAt debe persistirse como RFC3339, es %q", stringAttrValueMap(persistido, "completedAt"))
	}
	if len(mem.puts) != 1 || exprDe(mem.puts[0].ConditionExpression) != "attribute_not_exists(#pk)" {
		t.Fatalf("la creacion debe usar condition attribute_not_exists(#pk)")
	}

	leido, encontrado, err := store.GetOnboarding(context.Background(), e019Usuario)
	if err != nil || !encontrado {
		t.Fatalf("la lectura posterior debe encontrar el item: encontrado=%v err=%v", encontrado, err)
	}
	if !appOnboarding.IsComplete(leido.CompletedSteps) || leido.CompletedAt == nil || !leido.CompletedAt.Equal(e019Ahora) {
		t.Fatalf("el estado completo debe conservar la marca: %+v", leido)
	}
}

// TestE019AdaptadorCreacionConflicto: una clave ya existente no se sobrescribe con
// PutItem.
func TestE019AdaptadorCreacionConflicto(t *testing.T) {
	mem := newE019Dynamo()
	mem.sembrar("USER#"+e019Usuario, "ONBOARDING", map[string]types.AttributeValue{
		"entityType": strAttr("USER_ONBOARDING"),
		"version":    numAttr(2),
	})
	store := dynamoAdapter.NewOnboardingStore(mem, e019Tabla)
	if _, err := store.CreateOnboarding(context.Background(), e019Usuario, e019Escribir()); !errors.Is(err, appOnboarding.ErrConflict) {
		t.Fatalf("se esperaba ErrConflict, obtenido %v", err)
	}
	if stringAttrValueMap(mem.leer("USER#"+e019Usuario, "ONBOARDING"), "version") != "2" {
		t.Fatalf("el item existente no debe modificarse")
	}
}

// TestE019AdaptadorConflictoDeVersion: una versión desactualizada aborta sin
// escribir.
func TestE019AdaptadorConflictoDeVersion(t *testing.T) {
	mem := newE019Dynamo()
	mem.sembrar("USER#"+e019Usuario, "ONBOARDING", map[string]types.AttributeValue{
		"entityType": strAttr("USER_ONBOARDING"),
		"version":    numAttr(7),
	})
	store := dynamoAdapter.NewOnboardingStore(mem, e019Tabla)

	escritura := e019Escribir() // espera la version 3
	if _, err := store.UpdateOnboarding(context.Background(), e019Usuario, escritura); !errors.Is(err, appOnboarding.ErrConflict) {
		t.Fatalf("se esperaba ErrConflict, obtenido %v", err)
	}
	if stringAttrValueMap(mem.leer("USER#"+e019Usuario, "ONBOARDING"), "version") != "7" {
		t.Fatalf("una escritura en conflicto no debe avanzar la version")
	}
}

// TestE019AdaptadorLegacySinVersion: un ítem sin atributo `version` se actualiza
// con CAS por ausencia y `ADD` materializa el 1.
func TestE019AdaptadorLegacySinVersion(t *testing.T) {
	mem := newE019Dynamo()
	mem.sembrar("USER#"+e019Usuario, "ONBOARDING", map[string]types.AttributeValue{
		"entityType":    strAttr("USER_ONBOARDING"),
		"microphone":    strAttr("pending"),
		"notifications": strAttr("denied"),
	})
	store := dynamoAdapter.NewOnboardingStore(mem, e019Tabla)

	escritura := appOnboarding.Write{
		CompletedSteps:   []string{"profile"},
		MicrophoneRaw:    "denied",
		NotificationsRaw: "denied",
		Now:              e019Ahora,
	}
	registro, err := store.UpdateOnboarding(context.Background(), e019Usuario, escritura)
	if err != nil {
		t.Fatalf("un item heredado debe poder actualizarse: %v", err)
	}
	if registro.Version != 1 {
		t.Fatalf("la version materializada debe ser 1, obtenida %d", registro.Version)
	}
	if len(mem.updates) != 1 || exprDe(mem.updates[0].ConditionExpression) != "attribute_not_exists(#version)" {
		t.Fatalf("la condicion debe ser attribute_not_exists(#version)")
	}
	if _, declaraLeida := mem.updates[0].ExpressionAttributeValues[":leida"]; declaraLeida {
		t.Fatalf("sin version leida no debe declararse :leida")
	}
	if stringAttrValueMap(mem.leer("USER#"+e019Usuario, "ONBOARDING"), "version") != "1" {
		t.Fatalf("la version debe quedar materializada en el item")
	}
}

// TestE019AdaptadorExpresionesSinValoresNoUsados es la invariante del hotfix de
// E003: cada valor y alias declarado debe aparecer en alguna expresión, y cada
// token referenciado debe estar declarado. DynamoDB rechaza los valores sobrantes.
func TestE019AdaptadorExpresionesSinValoresNoUsados(t *testing.T) {
	mem := newE019Dynamo()
	mem.sembrar("USER#"+e019Usuario, "ONBOARDING", map[string]types.AttributeValue{
		"entityType": strAttr("USER_ONBOARDING"),
		"version":    numAttr(3),
	})
	store := dynamoAdapter.NewOnboardingStore(mem, e019Tabla)
	if _, err := store.UpdateOnboarding(context.Background(), e019Usuario, e019Escribir()); err != nil {
		t.Fatalf("escritura de preparacion fallida: %v", err)
	}
	if _, err := store.CreateOnboarding(context.Background(), e019Otro, e019Escribir()); err != nil {
		t.Fatalf("creacion de preparacion fallida: %v", err)
	}

	type entrada struct {
		nombre      string
		expresiones []string
		nombres     map[string]string
		valores     map[string]types.AttributeValue
	}
	entradas := []entrada{
		{
			nombre:      "update con version",
			expresiones: []string{exprDe(mem.updates[0].UpdateExpression), exprDe(mem.updates[0].ConditionExpression)},
			nombres:     mem.updates[0].ExpressionAttributeNames,
			valores:     mem.updates[0].ExpressionAttributeValues,
		},
		{
			nombre:      "put de creacion",
			expresiones: []string{exprDe(mem.puts[0].ConditionExpression)},
			nombres:     mem.puts[0].ExpressionAttributeNames,
			valores:     mem.puts[0].ExpressionAttributeValues,
		},
	}

	for _, caso := range entradas {
		t.Run(caso.nombre, func(t *testing.T) {
			declaradosNombres := map[string]bool{}
			declaradosValores := map[string]bool{}
			for indice, expresion := range caso.expresiones {
				usadosNombres, usadosValores := tokensE019(expresion)
				for _, token := range usadosNombres {
					if _, ok := caso.nombres[token]; !ok {
						t.Fatalf("expresion %d referencia %q sin declararlo", indice, token)
					}
					declaradosNombres[token] = true
				}
				for _, token := range usadosValores {
					if _, ok := caso.valores[token]; !ok {
						t.Fatalf("expresion %d referencia %q sin declararlo", indice, token)
					}
					declaradosValores[token] = true
				}
			}
			for token := range caso.nombres {
				if !declaradosNombres[token] {
					t.Fatalf("alias %q declarado sin uso", token)
				}
			}
			for token := range caso.valores {
				if !declaradosValores[token] {
					t.Fatalf("valor %q declarado sin uso: DynamoDB rechazaria la peticion", token)
				}
			}
		})
	}
}

// TestE019AdaptadorNoTocaOtrosItems: PROFILE, SESSION y el estado de otro usuario
// quedan intactos tras la escritura de onboarding.
func TestE019AdaptadorNoTocaOtrosItems(t *testing.T) {
	mem := newE019Dynamo()
	mem.sembrar("USER#"+e019Usuario, "PROFILE", map[string]types.AttributeValue{
		"entityType":   strAttr("USER_PROFILE"),
		"passwordHash": strAttr("$argon2id$v=19$m=65536,t=3,p=2$c2FsdA$aGFzaA"),
		"status":       strAttr("ACTIVE"),
		"version":      numAttr(5),
	})
	mem.sembrar("USER#"+e019Usuario, "SESSION#018f90ef-08e5-7a68-b09f-0000000000e1", map[string]types.AttributeValue{
		"entityType": strAttr("USER_SESSION"),
		"status":     strAttr("ACTIVE"),
		"version":    numAttr(2),
	})
	mem.sembrar("USER#"+e019Otro, "ONBOARDING", map[string]types.AttributeValue{
		"entityType":     strAttr("USER_ONBOARDING"),
		"completedSteps": listaAttr("privacy"),
		"version":        numAttr(8),
	})
	antesPerfil := mem.leer("USER#"+e019Usuario, "PROFILE")
	antesSesion := mem.leer("USER#"+e019Usuario, "SESSION#018f90ef-08e5-7a68-b09f-0000000000e1")
	antesOtro := mem.leer("USER#"+e019Otro, "ONBOARDING")

	mem.sembrar("USER#"+e019Usuario, "ONBOARDING", map[string]types.AttributeValue{
		"entityType": strAttr("USER_ONBOARDING"),
		"version":    numAttr(3),
	})
	store := dynamoAdapter.NewOnboardingStore(mem, e019Tabla)
	if _, err := store.UpdateOnboarding(context.Background(), e019Usuario, e019Escribir()); err != nil {
		t.Fatalf("la escritura valida no debe fallar: %v", err)
	}

	if !reflect.DeepEqual(mem.leer("USER#"+e019Usuario, "PROFILE"), antesPerfil) {
		t.Fatalf("la escritura no debe tocar PROFILE")
	}
	if !reflect.DeepEqual(mem.leer("USER#"+e019Usuario, "SESSION#018f90ef-08e5-7a68-b09f-0000000000e1"), antesSesion) {
		t.Fatalf("la escritura no debe tocar la sesion")
	}
	if !reflect.DeepEqual(mem.leer("USER#"+e019Otro, "ONBOARDING"), antesOtro) {
		t.Fatalf("la escritura no debe tocar a otro usuario")
	}
}

// TestE019AdaptadorFallaCerrado: sin cliente, sin tabla, sin usuario o con un
// cliente de sólo lectura no hay escritura.
func TestE019AdaptadorFallaCerrado(t *testing.T) {
	casos := []struct {
		nombre string
		store  *dynamoAdapter.OnboardingStore
		userID string
	}{
		{"sin cliente", dynamoAdapter.NewOnboardingStore(nil, e019Tabla), e019Usuario},
		{"sin tabla", dynamoAdapter.NewOnboardingStore(newE019Dynamo(), "  "), e019Usuario},
		{"sin usuario", dynamoAdapter.NewOnboardingStore(newE019Dynamo(), e019Tabla), "  "},
		{"cliente de solo lectura", dynamoAdapter.NewOnboardingStore(lectorFallido{}, e019Tabla), e019Usuario},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			if _, err := caso.store.UpdateOnboarding(context.Background(), caso.userID, e019Escribir()); !errors.Is(err, appOnboarding.ErrUnavailable) {
				t.Fatalf("UpdateOnboarding debe fallar cerrado con ErrUnavailable, obtenido %v", err)
			}
			if _, err := caso.store.CreateOnboarding(context.Background(), caso.userID, e019Escribir()); !errors.Is(err, appOnboarding.ErrUnavailable) {
				t.Fatalf("CreateOnboarding debe fallar cerrado con ErrUnavailable, obtenido %v", err)
			}
		})
	}
}

// TestE019AdaptadorErrorDeDynamo: un error del SDK se degrada a 503 sin filtrar
// tabla, usuario ni mensaje.
func TestE019AdaptadorErrorDeDynamo(t *testing.T) {
	mem := newE019Dynamo()
	mem.sembrar("USER#"+e019Usuario, "ONBOARDING", map[string]types.AttributeValue{
		"entityType": strAttr("USER_ONBOARDING"),
		"version":    numAttr(3),
	})
	mem.falloUpd = errors.New("dynamodb no disponible")
	store := dynamoAdapter.NewOnboardingStore(mem, e019Tabla)

	_, err := store.UpdateOnboarding(context.Background(), e019Usuario, e019Escribir())
	if !errors.Is(err, appOnboarding.ErrUnavailable) {
		t.Fatalf("un error del cliente debe ser ErrUnavailable, obtenido %v", err)
	}
	if strings.Contains(err.Error(), e019Tabla) || strings.Contains(err.Error(), e019Usuario) {
		t.Fatalf("el error no debe contener tabla ni usuario: %q", err.Error())
	}
}

// TestE019AdaptadorConcurrenciaDosNodos: dos servicios sobre el mismo almacén
// compiten por la misma versión; cada resultado es éxito o conflicto declarado y
// el estado final es el de una escritura confirmada.
func TestE019AdaptadorConcurrenciaDosNodos(t *testing.T) {
	mem := newE019Dynamo()
	mem.sembrar("USER#"+e019Usuario, "ONBOARDING", map[string]types.AttributeValue{
		"entityType": strAttr("USER_ONBOARDING"),
		"version":    numAttr(1),
	})
	nodoA := appOnboarding.NewService(dynamoAdapter.NewOnboardingStore(mem, e019Tabla)).WithClock(func() time.Time { return e019Ahora })
	nodoB := appOnboarding.NewService(dynamoAdapter.NewOnboardingStore(mem, e019Tabla)).WithClock(func() time.Time { return e019Ahora })

	const intentos = 6
	resultados := make([]error, intentos)
	var wg sync.WaitGroup
	for i := 0; i < intentos; i++ {
		wg.Add(1)
		go func(indice int) {
			defer wg.Done()
			servicio := nodoA
			if indice%2 == 1 {
				servicio = nodoB
			}
			_, err := servicio.Patch(context.Background(), e019Usuario, appOnboarding.PatchCommand{
				CompletedSteps: []string{"profile", "income"},
			})
			resultados[indice] = err
		}(i)
	}
	wg.Wait()

	exitos := 0
	for _, err := range resultados {
		switch {
		case err == nil:
			exitos++
		case errors.Is(err, appOnboarding.ErrConflict):
			// Conflicto declarado: nada se sobrescribio.
		default:
			t.Fatalf("error inesperado en concurrencia: %v", err)
		}
	}
	if exitos == 0 {
		t.Fatalf("al menos una escritura debe confirmarse")
	}

	persistido := mem.leer("USER#"+e019Usuario, "ONBOARDING")
	if stringAttrValueMap(persistido, "version") != strconv.Itoa(1+exitos) {
		t.Fatalf("la version final debe ser %d, es %q", 1+exitos, stringAttrValueMap(persistido, "version"))
	}
	store := dynamoAdapter.NewOnboardingStore(mem, e019Tabla)
	estado, err := appOnboarding.NewService(store).Get(context.Background(), e019Usuario)
	if err != nil {
		t.Fatalf("la lectura final debe funcionar: %v", err)
	}
	if want := []string{"profile", "income"}; !reflect.DeepEqual(estado.CompletedSteps, want) {
		t.Fatalf("el estado final debe ser el confirmado %v, obtenido %v", want, estado.CompletedSteps)
	}
	if estado.Done || estado.CompletedAt != nil {
		t.Fatalf("dos pasos no pueden marcar el onboarding como completo: %+v", estado)
	}
}

func timePtrE019(valor time.Time) *time.Time { return &valor }

// boolAtributoE019 lee un atributo BOOL; ausente o de otro tipo es false.
func boolAtributoE019(valor types.AttributeValue) bool {
	if tipado, ok := valor.(*types.AttributeValueMemberBOOL); ok {
		return tipado.Value
	}
	return false
}
