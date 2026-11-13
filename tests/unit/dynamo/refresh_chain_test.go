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
	appIdentity "github.com/nexumpr/Nexum-Backend/internal/application/identity"
)

// memoriaDynamo es un doble con estado que interpreta las MISMAS condiciones que emite el
// adaptador (conjunción de átomos y una disyunción entre paréntesis). No es un motor
// DynamoDB: si aparece una condición no soportada, falla el test en lugar de adivinar.
//
// Su propósito es demostrar de extremo a extremo, a través del adaptador real, que la
// cadena A→B→C→D funciona con localizadores creados por E002 (sin `used`) y por E003
// (`used=false`), y que la reutilización revoca la familia.
type memoriaDynamo struct {
	mu    sync.Mutex
	items map[string]map[string]types.AttributeValue
}

func newMemoriaDynamo() *memoriaDynamo {
	return &memoriaDynamo{items: make(map[string]map[string]types.AttributeValue)}
}

func claveDe(pk, sk string) string { return pk + "\x00" + sk }

func (m *memoriaDynamo) sembrar(pk, sk string, attrs map[string]types.AttributeValue) {
	m.mu.Lock()
	defer m.mu.Unlock()
	item := map[string]types.AttributeValue{"PK": strAttr(pk), "SK": strAttr(sk)}
	for k, v := range attrs {
		item[k] = v
	}
	m.items[claveDe(pk, sk)] = item
}

func (m *memoriaDynamo) leer(pk, sk string) map[string]types.AttributeValue {
	m.mu.Lock()
	defer m.mu.Unlock()
	item, ok := m.items[claveDe(pk, sk)]
	if !ok {
		return nil
	}
	return copiarItem(item)
}

func (m *memoriaDynamo) GetItem(_ context.Context, params *dynamodb.GetItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	pk := mapaString(params.Key, "PK")
	sk := mapaString(params.Key, "SK")
	item, ok := m.items[claveDe(pk, sk)]
	if !ok {
		return &dynamodb.GetItemOutput{}, nil
	}
	return &dynamodb.GetItemOutput{Item: copiarItem(item)}, nil
}

func (m *memoriaDynamo) TransactWriteItems(_ context.Context, params *dynamodb.TransactWriteItemsInput, _ ...func(*dynamodb.Options)) (*dynamodb.TransactWriteItemsOutput, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Fase 1: evaluar TODAS las condiciones antes de escribir nada (atomicidad).
	for _, item := range params.TransactItems {
		if err := m.comprobar(item); err != nil {
			return nil, err
		}
	}
	// Fase 2: aplicar.
	for _, item := range params.TransactItems {
		switch {
		case item.Put != nil:
			pk := mapaString(item.Put.Item, "PK")
			sk := mapaString(item.Put.Item, "SK")
			m.items[claveDe(pk, sk)] = copiarItem(item.Put.Item)
		case item.Update != nil:
			m.aplicarUpdate(item.Update)
		case item.ConditionCheck != nil:
			// Sin escritura: la condición ya se evaluó.
		}
	}
	return &dynamodb.TransactWriteItemsOutput{}, nil
}

func (m *memoriaDynamo) comprobar(item types.TransactWriteItem) error {
	var (
		clave   map[string]types.AttributeValue
		expr    *string
		nombres map[string]string
		valores map[string]types.AttributeValue
	)
	switch {
	case item.Put != nil:
		pk := mapaString(item.Put.Item, "PK")
		sk := mapaString(item.Put.Item, "SK")
		clave = map[string]types.AttributeValue{"PK": strAttr(pk), "SK": strAttr(sk)}
		expr, nombres, valores = item.Put.ConditionExpression, item.Put.ExpressionAttributeNames, item.Put.ExpressionAttributeValues
	case item.Update != nil:
		clave = item.Update.Key
		expr, nombres, valores = item.Update.ConditionExpression, item.Update.ExpressionAttributeNames, item.Update.ExpressionAttributeValues
	case item.ConditionCheck != nil:
		clave = item.ConditionCheck.Key
		expr, nombres, valores = item.ConditionCheck.ConditionExpression, item.ConditionCheck.ExpressionAttributeNames, item.ConditionCheck.ExpressionAttributeValues
	}

	existente := m.items[claveDe(mapaString(clave, "PK"), mapaString(clave, "SK"))]
	if expr == nil {
		return nil
	}
	ok, err := evaluarCondicion(aws.ToString(expr), existente, nombres, valores)
	if err != nil {
		return err
	}
	if !ok {
		return &types.TransactionCanceledException{
			Message:             aws.String("Transaction cancelled"),
			CancellationReasons: []types.CancellationReason{{Code: aws.String("ConditionalCheckFailed")}},
		}
	}
	return nil
}

func (m *memoriaDynamo) aplicarUpdate(update *types.Update) {
	pk := mapaString(update.Key, "PK")
	sk := mapaString(update.Key, "SK")
	item := m.items[claveDe(pk, sk)]
	if item == nil {
		item = map[string]types.AttributeValue{"PK": strAttr(pk), "SK": strAttr(sk)}
	}
	expresion := aws.ToString(update.UpdateExpression)
	set, add := partirExpresion(expresion)
	for _, asignacion := range set {
		partes := strings.SplitN(asignacion, " = ", 2)
		if len(partes) != 2 {
			continue
		}
		item[resolverNombre(partes[0], update.ExpressionAttributeNames)] = update.ExpressionAttributeValues[partes[1]]
	}
	for _, incremento := range add {
		partes := strings.SplitN(incremento, " ", 2)
		if len(partes) != 2 {
			continue
		}
		nombre := resolverNombre(strings.TrimSpace(partes[0]), update.ExpressionAttributeNames)
		delta := update.ExpressionAttributeValues[strings.TrimSpace(partes[1])]
		actual, okActual := item[nombre].(*types.AttributeValueMemberN)
		salto, okSalto := delta.(*types.AttributeValueMemberN)
		if !okSalto {
			continue
		}
		var base int64
		if okActual {
			base, _ = strconv.ParseInt(actual.Value, 10, 64)
		}
		incrementoValor, _ := strconv.ParseInt(salto.Value, 10, 64)
		item[nombre] = numAttr(base + incrementoValor)
	}
	m.items[claveDe(pk, sk)] = item
}

// partirExpresion separa la parte SET (lista de asignaciones) de la parte ADD.
func partirExpresion(expresion string) ([]string, []string) {
	var set, add []string
	texto := strings.TrimSpace(expresion)
	if indice := strings.Index(texto, " ADD "); indice >= 0 {
		texto, add = texto[:indice], strings.Split(strings.TrimSpace(texto[indice+len(" ADD "):]), ", ")
	}
	texto = strings.TrimPrefix(strings.TrimSpace(texto), "SET ")
	if texto != "" {
		set = strings.Split(texto, ", ")
	}
	return set, add
}

func resolverNombre(token string, nombres map[string]string) string {
	limpio := strings.TrimSpace(token)
	if alias, ok := nombres[limpio]; ok {
		return alias
	}
	return limpio
}

// evaluarCondicion soporta exactamente la gramática que emite refresh_store.go:
// átomos unidos por AND, con una disyunción entre paréntesis.
func evaluarCondicion(expr string, item map[string]types.AttributeValue, nombres map[string]string, valores map[string]types.AttributeValue) (bool, error) {
	for _, parte := range partirAND(expr) {
		if strings.HasPrefix(parte, "(") && strings.HasSuffix(parte, ")") {
			interior := parte[1 : len(parte)-1]
			alguno := false
			for _, alternativa := range strings.Split(interior, " OR ") {
				ok, err := evaluarAtomo(strings.TrimSpace(alternativa), item, nombres, valores)
				if err != nil {
					return false, err
				}
				if ok {
					alguno = true
					break
				}
			}
			if !alguno {
				return false, nil
			}
			continue
		}
		ok, err := evaluarAtomo(parte, item, nombres, valores)
		if err != nil {
			return false, err
		}
		if !ok {
			return false, nil
		}
	}
	return true, nil
}

func partirAND(expr string) []string {
	var partes []string
	profundidad := 0
	inicio := 0
	for i := 0; i < len(expr); i++ {
		switch expr[i] {
		case '(':
			profundidad++
		case ')':
			profundidad--
		}
		if profundidad == 0 && strings.HasPrefix(expr[i:], " AND ") {
			partes = append(partes, strings.TrimSpace(expr[inicio:i]))
			i += len(" AND ") - 1
			inicio = i + 1
		}
	}
	partes = append(partes, strings.TrimSpace(expr[inicio:]))
	return partes
}

func evaluarAtomo(atomo string, item map[string]types.AttributeValue, nombres map[string]string, valores map[string]types.AttributeValue) (bool, error) {
	switch {
	case strings.HasPrefix(atomo, "attribute_exists("):
		nombre := resolverNombre(strings.TrimSuffix(strings.TrimPrefix(atomo, "attribute_exists("), ")"), nombres)
		_, ok := item[nombre]
		return ok, nil
	case strings.HasPrefix(atomo, "attribute_not_exists("):
		nombre := resolverNombre(strings.TrimSuffix(strings.TrimPrefix(atomo, "attribute_not_exists("), ")"), nombres)
		_, ok := item[nombre]
		return !ok, nil
	}
	partes := strings.SplitN(atomo, " = ", 2)
	if len(partes) != 2 {
		return false, errors.New("condicion no soportada por el doble de prueba: " + atomo)
	}
	nombre := resolverNombre(partes[0], nombres)
	esperado, ok := valores[strings.TrimSpace(partes[1])]
	if !ok {
		return false, errors.New("valor de condicion ausente: " + partes[1])
	}
	actual, existe := item[nombre]
	if !existe {
		return false, nil
	}
	return atributosIguales(actual, esperado), nil
}

func atributosIguales(a, b types.AttributeValue) bool {
	switch va := a.(type) {
	case *types.AttributeValueMemberS:
		vb, ok := b.(*types.AttributeValueMemberS)
		return ok && va.Value == vb.Value
	case *types.AttributeValueMemberN:
		vb, ok := b.(*types.AttributeValueMemberN)
		return ok && va.Value == vb.Value
	case *types.AttributeValueMemberBOOL:
		vb, ok := b.(*types.AttributeValueMemberBOOL)
		return ok && va.Value == vb.Value
	}
	return false
}

func copiarItem(item map[string]types.AttributeValue) map[string]types.AttributeValue {
	copia := make(map[string]types.AttributeValue, len(item))
	for k, v := range item {
		copia[k] = v
	}
	return copia
}

func mapaString(item map[string]types.AttributeValue, nombre string) string {
	valor, ok := item[nombre].(*types.AttributeValueMemberS)
	if !ok {
		return ""
	}
	return valor.Value
}

// ---------- Cadena real A→B→C→D sobre el adaptador ----------

type issuerCadena struct{}

func (issuerCadena) IssueAccessToken(context.Context, string, string, time.Time) (string, string, error) {
	return "jwt-sintetico", "jti-sintetico", nil
}

const (
	cadenaPlainA = "refresh-opaco-A"
	cadenaPlainB = "refresh-opaco-B"
	cadenaPlainC = "refresh-opaco-C"
	cadenaPlainD = "refresh-opaco-D"
)

var cadenaAhora = time.Date(2026, 11, 12, 23, 0, 0, 0, time.UTC)

// sembrarSesionE002 replica EXACTAMENTE lo que escribe E002: el localizador NO tiene el
// atributo `used`. Cualquier condición que exija `attribute_not_exists(used)` de forma
// exclusiva rompería el segundo refresh y este test lo detectaría.
func sembrarSesionE002(mem *memoriaDynamo, expira time.Time) {
	mem.sembrar("USER#"+refreshUser, "PROFILE", map[string]types.AttributeValue{
		"entityType": strAttr("USER_PROFILE"),
		"userId":     strAttr(refreshUser),
		"status":     strAttr("ACTIVE"),
	})
	mem.sembrar("USER#"+refreshUser, "SESSION#"+refreshSesion, map[string]types.AttributeValue{
		"entityType":   strAttr("USER_SESSION"),
		"userId":       strAttr(refreshUser),
		"sessionId":    strAttr(refreshSesion),
		"familyId":     strAttr(refreshFamilia),
		"refreshHash":  strAttr(appIdentity.HashToken(cadenaPlainA)),
		"status":       strAttr("ACTIVE"),
		"version":      numAttr(1),
		"expiresAt":    numAttr(expira.Unix()),
		"expiresAtISO": strAttr(expira.Format(time.RFC3339)),
		"createdAt":    strAttr(cadenaAhora.Add(-time.Hour).Format(time.RFC3339)),
	})
	mem.sembrar("REFRESH#"+appIdentity.HashToken(cadenaPlainA), "SESSION", map[string]types.AttributeValue{
		"entityType":   strAttr("REFRESH_LOOKUP"),
		"userId":       strAttr(refreshUser),
		"sessionId":    strAttr(refreshSesion),
		"familyId":     strAttr(refreshFamilia),
		"status":       strAttr("ACTIVE"),
		"expiresAt":    numAttr(expira.Unix()),
		"expiresAtISO": strAttr(expira.Format(time.RFC3339)),
	})
}

func servicioCadena(store appIdentity.RefreshStore, tokens ...string) *appIdentity.RefreshService {
	indice := 0
	var mu sync.Mutex
	return appIdentity.NewRefreshService(store, issuerCadena{}).
		WithClock(func() time.Time { return cadenaAhora }).
		WithRefreshGenerator(func() (string, error) {
			mu.Lock()
			defer mu.Unlock()
			if indice >= len(tokens) {
				return "", errors.New("sin tokens de prueba")
			}
			token := tokens[indice]
			indice++
			return token, nil
		})
}

func TestE003CadenaABCDSobreAdaptadorReal(t *testing.T) {
	mem := newMemoriaDynamo()
	expira := cadenaAhora.Add(20 * 24 * time.Hour)
	sembrarSesionE002(mem, expira)
	store := dynamoAdapter.NewRefreshStore(mem, refreshTable)
	svc := servicioCadena(store, cadenaPlainB, cadenaPlainC, cadenaPlainD)
	ctx := context.Background()

	// A → B
	primero, err := svc.Refresh(ctx, appIdentity.RefreshCommand{RefreshToken: cadenaPlainA})
	if err != nil {
		t.Fatalf("A->B fallo (localizador de E002 sin `used`): %v", err)
	}
	if primero.RefreshToken != cadenaPlainB {
		t.Fatalf("se esperaba B, hay %q", primero.RefreshToken)
	}
	if primero.SessionID != refreshSesion {
		t.Fatalf("la sesion debe conservarse: %q", primero.SessionID)
	}

	// B → C: prueba que el localizador creado por E003 (`used=false`) sí es consumible.
	segundo, err := svc.Refresh(ctx, appIdentity.RefreshCommand{RefreshToken: cadenaPlainB})
	if err != nil {
		t.Fatalf("B->C fallo (localizador de E003 con used=false): %v", err)
	}
	if segundo.RefreshToken != cadenaPlainC {
		t.Fatalf("se esperaba C, hay %q", segundo.RefreshToken)
	}

	// C → D
	tercero, err := svc.Refresh(ctx, appIdentity.RefreshCommand{RefreshToken: cadenaPlainC})
	if err != nil {
		t.Fatalf("C->D fallo: %v", err)
	}
	if tercero.RefreshToken != cadenaPlainD {
		t.Fatalf("se esperaba D, hay %q", tercero.RefreshToken)
	}
	if tercero.ExpiresIn != 900 || tercero.TokenType != "Bearer" {
		t.Fatalf("contrato inesperado: %+v", tercero)
	}
	if tercero.Session.ID != tercero.SessionID {
		t.Fatalf("session.id debe igualar sessionId: %+v", tercero)
	}

	// Estado durable: versión avanzada una vez por rotación y digest final = D.
	sesion := mem.leer("USER#"+refreshUser, "SESSION#"+refreshSesion)
	if stringAttrValueMap(sesion, "version") != "4" {
		t.Fatalf("la version debe avanzar a 4, es %s", stringAttrValueMap(sesion, "version"))
	}
	if stringAttrValueMap(sesion, "refreshHash") != appIdentity.HashToken(cadenaPlainD) {
		t.Fatal("la sesion debe apuntar al digest D")
	}
	if stringAttrValueMap(sesion, "status") != "ACTIVE" {
		t.Fatal("la sesion debe seguir ACTIVE")
	}
	if stringAttrValueMap(sesion, "expiresAt") != strconv.FormatInt(expira.Unix(), 10) {
		t.Fatal("rotar no debe extender la expiracion absoluta")
	}

	// Los tres tokens consumidos quedan durables, para detectar replay.
	for _, plain := range []string{cadenaPlainA, cadenaPlainB, cadenaPlainC} {
		locator := mem.leer("REFRESH#"+appIdentity.HashToken(plain), "SESSION")
		if stringAttrValueMap(locator, "used") != "true" {
			t.Fatalf("el localizador de %q debe quedar usado", plain)
		}
		if stringAttrValueMap(locator, "status") != "CONSUMED" {
			t.Fatalf("el localizador de %q debe quedar CONSUMED", plain)
		}
	}
	actual := mem.leer("REFRESH#"+appIdentity.HashToken(cadenaPlainD), "SESSION")
	if stringAttrValueMap(actual, "status") != "ACTIVE" || stringAttrValueMap(actual, "used") != "false" {
		t.Fatalf("el localizador vigente debe ser ACTIVE y no usado: %v", actual)
	}

	// Reutilizar A: replay + revocación durable de la familia.
	if _, err := svc.Refresh(ctx, appIdentity.RefreshCommand{RefreshToken: cadenaPlainA}); !errors.Is(err, appIdentity.ErrRefreshReplay) {
		t.Fatalf("se esperaba ErrRefreshReplay, hay %v", err)
	}
	revocada := mem.leer("USER#"+refreshUser, "SESSION#"+refreshSesion)
	if stringAttrValueMap(revocada, "status") != "REVOKED" {
		t.Fatal("la sesion debe quedar REVOKED")
	}
	if stringAttrValueMap(revocada, "revokeReason") != appIdentity.RevokeReasonReplay {
		t.Fatalf("motivo de revocacion inesperado: %s", stringAttrValueMap(revocada, "revokeReason"))
	}

	// D ya no puede renovar: la familia está revocada.
	if _, err := svc.Refresh(ctx, appIdentity.RefreshCommand{RefreshToken: cadenaPlainD}); !errors.Is(err, appIdentity.ErrInvalidRefreshToken) {
		t.Fatalf("una sesion revocada no puede renovar, hay %v", err)
	}
}

func TestE003CuentaDeshabilitadaBloqueaLaRotacion(t *testing.T) {
	mem := newMemoriaDynamo()
	sembrarSesionE002(mem, cadenaAhora.Add(20*24*time.Hour))
	mem.sembrar("USER#"+refreshUser, "PROFILE", map[string]types.AttributeValue{
		"entityType": strAttr("USER_PROFILE"),
		"userId":     strAttr(refreshUser),
		"status":     strAttr("DISABLED"),
	})
	store := dynamoAdapter.NewRefreshStore(mem, refreshTable)
	svc := servicioCadena(store, cadenaPlainB)

	if _, err := svc.Refresh(context.Background(), appIdentity.RefreshCommand{RefreshToken: cadenaPlainA}); !errors.Is(err, appIdentity.ErrInvalidRefreshToken) {
		t.Fatalf("una cuenta deshabilitada no renueva, hay %v", err)
	}
	sesion := mem.leer("USER#"+refreshUser, "SESSION#"+refreshSesion)
	if stringAttrValueMap(sesion, "refreshHash") != appIdentity.HashToken(cadenaPlainA) {
		t.Fatal("la transaccion no debe haber escrito nada")
	}
}

func TestE003ConcurrenteSobreAdaptadorUnaSolaRotacion(t *testing.T) {
	mem := newMemoriaDynamo()
	sembrarSesionE002(mem, cadenaAhora.Add(20*24*time.Hour))
	store := dynamoAdapter.NewRefreshStore(mem, refreshTable)

	// Dos servicios independientes comparten persistencia: es la situación de dos nodos.
	resultados := make([]error, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(indice int) {
			defer wg.Done()
			svc := servicioCadena(store, cadenaPlainB)
			_, err := svc.Refresh(context.Background(), appIdentity.RefreshCommand{RefreshToken: cadenaPlainA})
			resultados[indice] = err
		}(i)
	}
	wg.Wait()

	exitos, replays := 0, 0
	for _, err := range resultados {
		switch {
		case err == nil:
			exitos++
		case errors.Is(err, appIdentity.ErrRefreshReplay):
			replays++
		default:
			t.Fatalf("error inesperado en concurrencia: %v", err)
		}
	}
	if exitos != 1 {
		t.Fatalf("debe haber exactamente una rotacion exitosa, hay %d", exitos)
	}
	if replays != 1 {
		t.Fatalf("el perdedor debe detectar reutilizacion, hay %d", replays)
	}
	if stringAttrValueMap(mem.leer("USER#"+refreshUser, "SESSION#"+refreshSesion), "status") != "REVOKED" {
		t.Fatal("la familia debe quedar revocada tras la carrera")
	}
}

func TestE003ConditionCheckDeCuentaBloqueaEnLaTransaccion(t *testing.T) {
	mem := newMemoriaDynamo()
	sembrarSesionE002(mem, cadenaAhora.Add(20*24*time.Hour))
	store := dynamoAdapter.NewRefreshStore(mem, refreshTable)

	// El perfil pasa a DISABLED después de la lectura previa: sólo la ConditionCheck de
	// la transacción impide escribir.
	mem.sembrar("USER#"+refreshUser, "PROFILE", map[string]types.AttributeValue{
		"entityType": strAttr("USER_PROFILE"),
		"userId":     strAttr(refreshUser),
		"status":     strAttr("DISABLED"),
	})

	err := store.RotateRefresh(context.Background(), appIdentity.RefreshRotation{
		UserID:          refreshUser,
		SessionID:       refreshSesion,
		FamilyID:        refreshFamilia,
		OldDigest:       appIdentity.HashToken(cadenaPlainA),
		NewDigest:       appIdentity.HashToken(cadenaPlainB),
		NewAccessJTI:    "jti-nuevo",
		ExpectedVersion: 1,
		NewVersion:      2,
		ExpiresAt:       cadenaAhora.Add(20 * 24 * time.Hour),
		Now:             cadenaAhora,
	})
	if !errors.Is(err, appIdentity.ErrRefreshConflict) {
		t.Fatalf("el perfil no ACTIVE debe abortar la transaccion, hay %v", err)
	}
	if stringAttrValueMap(mem.leer("USER#"+refreshUser, "SESSION#"+refreshSesion), "version") != "1" {
		t.Fatal("no debe escribirse nada si la cuenta no esta ACTIVE")
	}
}

func TestE003LocalizadorDeOtraFamiliaNoSeAfecta(t *testing.T) {
	mem := newMemoriaDynamo()
	sembrarSesionE002(mem, cadenaAhora.Add(20*24*time.Hour))
	// Segunda sesión, familia distinta, del mismo usuario.
	otraSesion := "018f90ef-08e5-7a68-b09f-0000000000c3"
	otraFamilia := "018f90ef-08e5-7a68-b09f-0000000000d4"
	otraFamiliaRefresh := "refresh-opaco-otra-familia"
	mem.sembrar("USER#"+refreshUser, "SESSION#"+otraSesion, map[string]types.AttributeValue{
		"entityType":  strAttr("USER_SESSION"),
		"userId":      strAttr(refreshUser),
		"sessionId":   strAttr(otraSesion),
		"familyId":    strAttr(otraFamilia),
		"refreshHash": strAttr(appIdentity.HashToken(otraFamiliaRefresh)),
		"status":      strAttr("ACTIVE"),
		"version":     numAttr(1),
		"expiresAt":   numAttr(cadenaAhora.Add(20 * 24 * time.Hour).Unix()),
	})
	mem.sembrar("REFRESH#"+appIdentity.HashToken(otraFamiliaRefresh), "SESSION", map[string]types.AttributeValue{
		"entityType": strAttr("REFRESH_LOOKUP"),
		"userId":     strAttr(refreshUser),
		"sessionId":  strAttr(otraSesion),
		"familyId":   strAttr(otraFamilia),
		"status":     strAttr("ACTIVE"),
		"expiresAt":  numAttr(cadenaAhora.Add(20 * 24 * time.Hour).Unix()),
	})

	store := dynamoAdapter.NewRefreshStore(mem, refreshTable)
	svc := servicioCadena(store, cadenaPlainB, "refresh-opaco-otra-familia-rotada")
	ctx := context.Background()

	if _, err := svc.Refresh(ctx, appIdentity.RefreshCommand{RefreshToken: cadenaPlainA}); err != nil {
		t.Fatalf("la primera rotacion debe funcionar: %v", err)
	}
	if _, err := svc.Refresh(ctx, appIdentity.RefreshCommand{RefreshToken: cadenaPlainA}); !errors.Is(err, appIdentity.ErrRefreshReplay) {
		t.Fatalf("se esperaba replay, hay %v", err)
	}
	// La otra familia sigue intacta y puede renovar.
	if _, err := svc.Refresh(ctx, appIdentity.RefreshCommand{RefreshToken: otraFamiliaRefresh}); err != nil {
		t.Fatalf("la familia ajena no debe verse afectada: %v", err)
	}
	if stringAttrValueMap(mem.leer("USER#"+refreshUser, "SESSION#"+otraSesion), "status") != "ACTIVE" {
		t.Fatal("la otra sesion debe seguir ACTIVE")
	}
}
