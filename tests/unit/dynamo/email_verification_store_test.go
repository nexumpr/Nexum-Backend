package dynamo_test

import (
	"context"
	"errors"
	"strconv"
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

// Pruebas de E010 sobre el adaptador REAL y dobles con estado: verifican la condición
// attribute_not_exists, el namespace ONE_USE#, el TTL físico y que solo se persiste el hash.

const (
	e010Tabla = "NEXUM_CORE_TEST"
	e010User  = "aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee"
)

var e010Instante = time.Date(2026, 11, 17, 10, 0, 0, 0, time.UTC)

// almacenEV extiende el doble con estado de la tabla única con PutItem condicional, que es la
// operación que usa E010 para no sobrescribir un token existente.
type almacenEV struct{ *memoriaDynamo }

func nuevoAlmacenEV() *almacenEV { return &almacenEV{memoriaDynamo: newMemoriaDynamo()} }

func (a *almacenEV) PutItem(_ context.Context, params *dynamodb.PutItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error) {
	a.memoriaDynamo.mu.Lock()
	defer a.memoriaDynamo.mu.Unlock()

	pk := mapaString(params.Item, "PK")
	sk := mapaString(params.Item, "SK")
	existente := a.memoriaDynamo.items[claveDe(pk, sk)]
	if params.ConditionExpression != nil {
		ok, err := evaluarCondicion(aws.ToString(params.ConditionExpression), existente, params.ExpressionAttributeNames, params.ExpressionAttributeValues)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, &types.ConditionalCheckFailedException{Message: aws.String("conditional check failed")}
		}
	}
	a.memoriaDynamo.items[claveDe(pk, sk)] = copiarItem(params.Item)
	return &dynamodb.PutItemOutput{}, nil
}

// evRegistro captura la entrada exacta de PutItem/GetItem sin mantener estado.
type evRegistro struct {
	put      *dynamodb.PutItemInput
	putErr   error
	get      *dynamodb.GetItemInput
	getOut   *dynamodb.GetItemOutput
	getErr   error
	llamadas int
}

func (r *evRegistro) PutItem(_ context.Context, params *dynamodb.PutItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error) {
	r.llamadas++
	r.put = params
	if r.putErr != nil {
		return nil, r.putErr
	}
	return &dynamodb.PutItemOutput{}, nil
}

func (r *evRegistro) GetItem(_ context.Context, params *dynamodb.GetItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
	r.llamadas++
	r.get = params
	if r.getErr != nil {
		return nil, r.getErr
	}
	if r.getOut != nil {
		return r.getOut, nil
	}
	return &dynamodb.GetItemOutput{}, nil
}

func (r *evRegistro) TransactWriteItems(context.Context, *dynamodb.TransactWriteItemsInput, ...func(*dynamodb.Options)) (*dynamodb.TransactWriteItemsOutput, error) {
	return nil, errors.New("transaccion no esperada en E010")
}

func e010Token() appIdentity.EmailVerificationToken {
	return appIdentity.EmailVerificationToken{
		UserID:    e010User,
		Email:     "verificacion@example.invalid",
		TokenHash: appIdentity.HashToken("token-sintetico-de-verificacion-0123456789"),
		CreatedAt: e010Instante,
		ExpiresAt: e010Instante.Add(appIdentity.EmailVerificationTokenTTL),
	}
}

func TestE010PersistenciaEmiteCondicionYAtributos(t *testing.T) {
	registro := &evRegistro{}
	store := dynamoAdapter.NewEmailVerificationStore(registro, e010Tabla)

	if err := store.SaveEmailVerificationToken(context.Background(), e010Token()); err != nil {
		t.Fatalf("persistencia valida fallo: %v", err)
	}
	if registro.put == nil {
		t.Fatalf("debe emitirse PutItem")
	}
	if aws.ToString(registro.put.TableName) != e010Tabla {
		t.Fatalf("tabla inesperada: %q", aws.ToString(registro.put.TableName))
	}
	if condicion := aws.ToString(registro.put.ConditionExpression); condicion != "attribute_not_exists(PK)" {
		t.Fatalf("la condicion debe impedir sobrescribir el token, obtenido %q", condicion)
	}

	item := registro.put.Item
	if pk := mapaString(item, "PK"); pk != "ONE_USE#"+appIdentity.HashToken("token-sintetico-de-verificacion-0123456789") {
		t.Fatalf("PK esperada ONE_USE#<sha256>, obtenida %q", pk)
	}
	if sk := mapaString(item, "SK"); sk != "VERIFY" {
		t.Fatalf("SK esperada VERIFY, obtenida %q", sk)
	}
	if entidad := mapaString(item, "entityType"); entidad != appIdentity.EmailVerificationEntityType {
		t.Fatalf("entityType esperado %q, obtenido %q", appIdentity.EmailVerificationEntityType, entidad)
	}
	if proposito := mapaString(item, "purpose"); proposito != appIdentity.EmailVerificationPurpose {
		t.Fatalf("purpose esperado %q, obtenido %q", appIdentity.EmailVerificationPurpose, proposito)
	}
	if usuario := mapaString(item, "userId"); usuario != e010User {
		t.Fatalf("userId inesperado: %q", usuario)
	}
	if correo := mapaString(item, "email"); correo != "verificacion@example.invalid" {
		t.Fatalf("email inesperado: %q", correo)
	}
	if creado := mapaString(item, "createdAt"); creado != e010Instante.Format(time.RFC3339) {
		t.Fatalf("createdAt inesperado: %q", creado)
	}
	if iso := mapaString(item, "expiresAtISO"); iso != e010Instante.Add(appIdentity.EmailVerificationTokenTTL).Format(time.RFC3339) {
		t.Fatalf("expiresAtISO inesperado: %q", iso)
	}
	ttl, ok := item["expiresAt"].(*types.AttributeValueMemberN)
	if !ok {
		t.Fatalf("expiresAt debe ser Number (TTL fisico)")
	}
	if esperado := e010Instante.Add(appIdentity.EmailVerificationTokenTTL).Unix(); ttl.Value != strconv.FormatInt(esperado, 10) {
		t.Fatalf("expiresAt esperado %d, obtenido %s", esperado, ttl.Value)
	}
	usado, ok := item["used"].(*types.AttributeValueMemberBOOL)
	if !ok || usado.Value {
		t.Fatalf("used debe nacer en false")
	}
	for nombre, valor := range item {
		if texto, ok := valor.(*types.AttributeValueMemberS); ok && strings.Contains(texto.Value, "token-sintetico-de-verificacion-0123456789") {
			t.Fatalf("el token en claro no puede persistirse en %q", nombre)
		}
	}
}

func TestE010PersistenciaNoSobrescribeTokenExistente(t *testing.T) {
	m := nuevoAlmacenEV()
	store := dynamoAdapter.NewEmailVerificationStore(m, e010Tabla)
	token := e010Token()

	if err := store.SaveEmailVerificationToken(context.Background(), token); err != nil {
		t.Fatalf("primera persistencia fallo: %v", err)
	}
	if err := store.SaveEmailVerificationToken(context.Background(), token); !errors.Is(err, appIdentity.ErrEmailVerificationTokenDuplicado) {
		t.Fatalf("un token existente no puede sobrescribirse, obtenido %v", err)
	}

	pk := "ONE_USE#" + token.TokenHash
	guardado := m.leer(pk, "VERIFY")
	if guardado == nil {
		t.Fatalf("el token debe seguir persistido")
	}
	if mapaString(guardado, "userId") != token.UserID {
		t.Fatalf("la segunda escritura no puede alterar el item")
	}
}

func TestE010PerfilSeLeeConConsistenciaFuerte(t *testing.T) {
	registro := &evRegistro{}
	store := dynamoAdapter.NewEmailVerificationStore(registro, e010Tabla)

	if _, err := store.LoadEmailVerificationProfile(context.Background(), e010User); err != nil {
		t.Fatalf("lectura valida fallo: %v", err)
	}
	if registro.get == nil {
		t.Fatalf("debe emitirse GetItem")
	}
	if registro.get.ConsistentRead == nil || !*registro.get.ConsistentRead {
		t.Fatalf("el perfil debe leerse con consistencia fuerte")
	}
	pk, sk := domainDynamo.UserKeys(e010User)
	if mapaString(registro.get.Key, "PK") != pk || mapaString(registro.get.Key, "SK") != sk {
		t.Fatalf("clave de perfil inesperada: %+v", registro.get.Key)
	}
}

func TestE010PerfilAtributosYAusencia(t *testing.T) {
	m := nuevoAlmacenEV()
	store := dynamoAdapter.NewEmailVerificationStore(m, e010Tabla)

	pk, sk := domainDynamo.UserKeys(e010User)
	m.sembrar(pk, sk, map[string]types.AttributeValue{
		"entityType":    strAttr("USER_PROFILE"),
		"userId":        strAttr(e010User),
		"email":         strAttr("verificacion@example.invalid"),
		"status":        strAttr(appIdentity.StatusActive),
		"emailVerified": &types.AttributeValueMemberBOOL{Value: false},
		"version":       numAttr(7),
	})

	perfil, err := store.LoadEmailVerificationProfile(context.Background(), e010User)
	if err != nil || perfil == nil {
		t.Fatalf("perfil existente debe devolverse: perfil=%v err=%v", perfil, err)
	}
	if perfil.Email != "verificacion@example.invalid" || perfil.Status != appIdentity.StatusActive || perfil.EmailVerified {
		t.Fatalf("atributos inesperados: %+v", perfil)
	}
	if !perfil.HasVersion || perfil.Version != 7 {
		t.Fatalf("version esperada 7 con HasVersion=true, obtenido %+v", perfil)
	}

	ausente, err := store.LoadEmailVerificationProfile(context.Background(), "99999999-8888-4777-8666-555555555555")
	if err != nil || ausente != nil {
		t.Fatalf("un perfil ausente debe devolver (nil, nil), obtenido (%v, %v)", ausente, err)
	}

	otraEntidad, _ := domainDynamo.UserKeys("77777777-6666-4555-8444-333333333333")
	m.sembrar(otraEntidad, sk, map[string]types.AttributeValue{
		"entityType": strAttr("OTHER_ENTITY"),
		"userId":     strAttr("77777777-6666-4555-8444-333333333333"),
	})
	distinta, err := store.LoadEmailVerificationProfile(context.Background(), "77777777-6666-4555-8444-333333333333")
	if err != nil || distinta != nil {
		t.Fatalf("una entidad distinta de USER_PROFILE no es un perfil")
	}
}

func TestE010StoreSinClienteFallaCerrado(t *testing.T) {
	store := dynamoAdapter.NewEmailVerificationStore(nil, "")
	if err := store.SaveEmailVerificationToken(context.Background(), e010Token()); !errors.Is(err, appIdentity.ErrEmailVerificationNoDisponible) {
		t.Fatalf("sin cliente debe fallar cerrado, obtenido %v", err)
	}
	if _, err := store.LoadEmailVerificationProfile(context.Background(), e010User); !errors.Is(err, appIdentity.ErrEmailVerificationNoDisponible) {
		t.Fatalf("sin cliente la lectura debe fallar cerrado, obtenido %v", err)
	}

	fallo := &evRegistro{putErr: errors.New("dynamodb caido"), getErr: errors.New("dynamodb caido")}
	storeFallo := dynamoAdapter.NewEmailVerificationStore(fallo, e010Tabla)
	if err := storeFallo.SaveEmailVerificationToken(context.Background(), e010Token()); !errors.Is(err, appIdentity.ErrEmailVerificationNoDisponible) {
		t.Fatalf("un fallo de servicio debe traducirse a no disponible, obtenido %v", err)
	}
	if _, err := storeFallo.LoadEmailVerificationProfile(context.Background(), e010User); !errors.Is(err, appIdentity.ErrEmailVerificationNoDisponible) {
		t.Fatalf("un fallo de lectura debe traducirse a no disponible, obtenido %v", err)
	}
}
