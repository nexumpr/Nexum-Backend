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

// Pruebas de E011 sobre el adaptador REAL: transacción de dos acciones sobre dos ítems,
// condiciones exactas, distinción de CancellationReasons por índice y consumo atómico.

var e011Instante = time.Date(2026, 11, 17, 11, 0, 0, 0, time.UTC)

const e011TokenCrudo = "token-sintetico-de-verificacion-0123456789"

// evTxRegistro captura la transacción emitida sin mantener estado.
type evTxRegistro struct {
	tx *dynamodb.TransactWriteItemsInput
}

func (r *evTxRegistro) GetItem(context.Context, *dynamodb.GetItemInput, ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
	return &dynamodb.GetItemOutput{}, nil
}

func (r *evTxRegistro) PutItem(context.Context, *dynamodb.PutItemInput, ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error) {
	return &dynamodb.PutItemOutput{}, nil
}

func (r *evTxRegistro) TransactWriteItems(_ context.Context, params *dynamodb.TransactWriteItemsInput, _ ...func(*dynamodb.Options)) (*dynamodb.TransactWriteItemsOutput, error) {
	r.tx = params
	return &dynamodb.TransactWriteItemsOutput{}, nil
}

func e011Confirmacion() appIdentity.EmailVerificationConfirmation {
	return appIdentity.EmailVerificationConfirmation{
		UserID:          e010User,
		TokenHash:       appIdentity.HashToken(e011TokenCrudo),
		ExpectedVersion: 4,
		HasVersion:      true,
		Now:             e011Instante,
	}
}

func sembrarPerfilEV(m *almacenEV, userID string, conVersion bool, version int64, verificado bool) {
	attrs := map[string]types.AttributeValue{
		"entityType":    strAttr("USER_PROFILE"),
		"userId":        strAttr(userID),
		"email":         strAttr("verificacion@example.invalid"),
		"status":        strAttr(appIdentity.StatusActive),
		"emailVerified": &types.AttributeValueMemberBOOL{Value: verificado},
	}
	if conVersion {
		attrs["version"] = numAttr(version)
	}
	pk, sk := domainDynamo.UserKeys(userID)
	m.sembrar(pk, sk, attrs)
}

func sembrarTokenEV(m *almacenEV, userID string, expira time.Time, used bool) {
	pk := "ONE_USE#" + appIdentity.HashToken(e011TokenCrudo)
	m.sembrar(pk, "VERIFY", map[string]types.AttributeValue{
		"entityType": strAttr(appIdentity.EmailVerificationEntityType),
		"purpose":    strAttr(appIdentity.EmailVerificationPurpose),
		"userId":     strAttr(userID),
		"email":      strAttr("verificacion@example.invalid"),
		"expiresAt":  numAttr(expira.Unix()),
		"used":       &types.AttributeValueMemberBOOL{Value: used},
	})
}

func TestE011TransaccionDosAccionesYCondiciones(t *testing.T) {
	registro := &evTxRegistro{}
	store := dynamoAdapter.NewEmailVerificationStore(registro, e010Tabla)

	if err := store.ConfirmEmailVerification(context.Background(), e011Confirmacion()); err != nil {
		t.Fatalf("transaccion valida fallo: %v", err)
	}
	if registro.tx == nil || len(registro.tx.TransactItems) != 2 {
		t.Fatalf("deben emitirse exactamente dos acciones, obtenido %+v", registro.tx)
	}

	perfil := registro.tx.TransactItems[0].Update
	if perfil == nil {
		t.Fatalf("la primera accion debe actualizar el perfil")
	}
	pk, sk := domainDynamo.UserKeys(e010User)
	if mapaString(perfil.Key, "PK") != pk || mapaString(perfil.Key, "SK") != sk {
		t.Fatalf("clave de perfil inesperada: %+v", perfil.Key)
	}
	condicion := aws.ToString(perfil.ConditionExpression)
	for _, fragmento := range []string{
		"attribute_exists(PK)",
		"#status = :active",
		"#version = :expected",
		"(attribute_not_exists(#verified) OR #verified = :false)",
	} {
		if !contiene(condicion, fragmento) {
			t.Fatalf("la condicion del perfil debe incluir %q, obtenido %q", fragmento, condicion)
		}
	}
	if perfil.ExpressionAttributeNames["#verified"] != "emailVerified" {
		t.Fatalf("el alias #verified debe apuntar a emailVerified")
	}
	actualizacion := aws.ToString(perfil.UpdateExpression)
	if !contiene(actualizacion, "SET emailVerified = :true") ||
		!contiene(actualizacion, "emailVerifiedAt = :now") ||
		!contiene(actualizacion, "ADD #version :one") {
		t.Fatalf("la actualizacion del perfil es incompleta: %q", actualizacion)
	}
	if valor, ok := perfil.ExpressionAttributeValues[":expected"].(*types.AttributeValueMemberN); !ok || valor.Value != "4" {
		t.Fatalf("CAS esperado de version 4, obtenido %+v", perfil.ExpressionAttributeValues[":expected"])
	}

	token := registro.tx.TransactItems[1].Update
	if token == nil {
		t.Fatalf("la segunda accion debe consumir el token")
	}
	if mapaString(token.Key, "PK") != "ONE_USE#"+appIdentity.HashToken(e011TokenCrudo) || mapaString(token.Key, "SK") != "VERIFY" {
		t.Fatalf("clave del token inesperada: %+v", token.Key)
	}
	condicionToken := aws.ToString(token.ConditionExpression)
	for _, fragmento := range []string{
		"attribute_exists(PK)",
		"(attribute_not_exists(#used) OR #used = :false)",
		"expiresAt > :nowEpoch",
	} {
		if !contiene(condicionToken, fragmento) {
			t.Fatalf("la condicion del token debe incluir %q, obtenido %q", fragmento, condicionToken)
		}
	}
	if aws.ToString(token.UpdateExpression) != "SET #used = :true, usedAt = :now" {
		t.Fatalf("actualizacion del token inesperada: %q", aws.ToString(token.UpdateExpression))
	}
	if valor, ok := token.ExpressionAttributeValues[":nowEpoch"].(*types.AttributeValueMemberN); !ok || valor.Value != strconv.FormatInt(e011Instante.Unix(), 10) {
		t.Fatalf(":nowEpoch inesperado: %+v", token.ExpressionAttributeValues[":nowEpoch"])
	}
}

func TestE011LegacySinVersion(t *testing.T) {
	registro := &evTxRegistro{}
	store := dynamoAdapter.NewEmailVerificationStore(registro, e010Tabla)

	confirmacion := e011Confirmacion()
	confirmacion.HasVersion = false
	confirmacion.ExpectedVersion = 0
	if err := store.ConfirmEmailVerification(context.Background(), confirmacion); err != nil {
		t.Fatalf("un perfil legado debe poder confirmarse: %v", err)
	}
	perfil := registro.tx.TransactItems[0].Update
	if !contiene(aws.ToString(perfil.ConditionExpression), "attribute_not_exists(#version)") {
		t.Fatalf("la condicion legada debe exigir la ausencia de version: %q", aws.ToString(perfil.ConditionExpression))
	}
	if !contiene(aws.ToString(perfil.UpdateExpression), "#version = :one") {
		t.Fatalf("la transaccion legada debe inicializar version: %q", aws.ToString(perfil.UpdateExpression))
	}
	if _, existe := perfil.ExpressionAttributeValues[":expected"]; existe {
		t.Fatalf("DynamoDB rechaza valores de expresion no utilizados: :expected no debe declararse")
	}
}

func TestE011ConfirmacionRealSobreDoble(t *testing.T) {
	m := nuevoAlmacenEV()
	sembrarPerfilEV(m, e010User, true, 4, false)
	sembrarTokenEV(m, e010User, e011Instante.Add(10*time.Minute), false)
	store := dynamoAdapter.NewEmailVerificationStore(m, e010Tabla)

	if err := store.ConfirmEmailVerification(context.Background(), e011Confirmacion()); err != nil {
		t.Fatalf("confirmacion real fallo: %v", err)
	}
	pk, sk := domainDynamo.UserKeys(e010User)
	perfil := m.leer(pk, sk)
	if verificado, ok := perfil["emailVerified"].(*types.AttributeValueMemberBOOL); !ok || !verificado.Value {
		t.Fatalf("emailVerified debe quedar en true")
	}
	if iso := mapaString(perfil, "emailVerifiedAt"); iso != e011Instante.Format(time.RFC3339) {
		t.Fatalf("emailVerifiedAt inesperado: %q", iso)
	}
	if version, ok := attributeIntDe(perfil, "version"); !ok || version != 5 {
		t.Fatalf("version esperada 5, obtenida %v", version)
	}
	itemToken := m.leer("ONE_USE#"+appIdentity.HashToken(e011TokenCrudo), "VERIFY")
	if usado, ok := itemToken["used"].(*types.AttributeValueMemberBOOL); !ok || !usado.Value {
		t.Fatalf("el token debe quedar consumido")
	}
	if iso := mapaString(itemToken, "usedAt"); iso != e011Instante.Format(time.RFC3339) {
		t.Fatalf("usedAt inesperado: %q", iso)
	}

	// Replay: el token ya esta usado y el perfil ya verificado. El doble con estado reporta
	// solo el PRIMER item que falla (indice 0), mientras que DynamoDB real devuelve una razon
	// por item y el adaptador prioriza el token (indice 1). Ambos caminos son 409 en el
	// contrato; la prueba de aplicacion fija el caso de token usado con un doble que aisla la
	// condicion del token.
	errReplay := store.ConfirmEmailVerification(context.Background(), e011Confirmacion())
	if !errors.Is(errReplay, appIdentity.ErrEmailVerificationTokenUsado) &&
		!errors.Is(errReplay, appIdentity.ErrEmailVerificationConflicto) {
		t.Fatalf("el replay debe ser un conflicto 409, obtenido %v", errReplay)
	}
	if version, _ := attributeIntDe(m.leer(pk, sk), "version"); version != 5 {
		t.Fatalf("un replay no puede volver a incrementar la version")
	}
}

func TestE011TokenExpiradoEnAdaptador(t *testing.T) {
	m := nuevoAlmacenEV()
	sembrarPerfilEV(m, e010User, true, 4, false)
	sembrarTokenEV(m, e010User, e011Instante.Add(-time.Minute), false)
	store := dynamoAdapter.NewEmailVerificationStore(m, e010Tabla)

	if err := store.ConfirmEmailVerification(context.Background(), e011Confirmacion()); !errors.Is(err, appIdentity.ErrEmailVerificationTokenUsado) {
		t.Fatalf("un token expirado debe rechazarse, obtenido %v", err)
	}
	pk, sk := domainDynamo.UserKeys(e010User)
	if verificado, _ := m.leer(pk, sk)["emailVerified"].(*types.AttributeValueMemberBOOL); verificado != nil && verificado.Value {
		t.Fatalf("un token expirado no puede verificar el correo")
	}
}

func TestE011PerfilYaVerificadoOVersionDesactualizada(t *testing.T) {
	casos := []struct {
		nombre string
		ajuste func(*almacenEV)
	}{
		{"ya verificado", func(m *almacenEV) {
			sembrarPerfilEV(m, e010User, true, 4, true)
		}},
		{"version distinta", func(m *almacenEV) {
			sembrarPerfilEV(m, e010User, true, 9, false)
		}},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			m := nuevoAlmacenEV()
			caso.ajuste(m)
			sembrarTokenEV(m, e010User, e011Instante.Add(10*time.Minute), false)
			store := dynamoAdapter.NewEmailVerificationStore(m, e010Tabla)

			err := store.ConfirmEmailVerification(context.Background(), e011Confirmacion())
			if !errors.Is(err, appIdentity.ErrEmailVerificationConflicto) {
				t.Fatalf("se esperaba conflicto de perfil, obtenido %v", err)
			}
			itemToken := m.leer("ONE_USE#"+appIdentity.HashToken(e011TokenCrudo), "VERIFY")
			if usado, _ := itemToken["used"].(*types.AttributeValueMemberBOOL); usado != nil && usado.Value {
				t.Fatalf("si el perfil falla, el token no puede consumirse (atomicidad)")
			}
		})
	}
}

func TestE011PerfilLegadoSeInicializa(t *testing.T) {
	m := nuevoAlmacenEV()
	sembrarPerfilEV(m, e010User, false, 0, false)
	sembrarTokenEV(m, e010User, e011Instante.Add(10*time.Minute), false)
	store := dynamoAdapter.NewEmailVerificationStore(m, e010Tabla)

	confirmacion := e011Confirmacion()
	confirmacion.HasVersion = false
	if err := store.ConfirmEmailVerification(context.Background(), confirmacion); err != nil {
		t.Fatalf("un perfil legado debe confirmarse: %v", err)
	}
	pk, sk := domainDynamo.UserKeys(e010User)
	perfil := m.leer(pk, sk)
	if version, ok := attributeIntDe(perfil, "version"); !ok || version != 1 {
		t.Fatalf("version debe inicializarse a 1, obtenido %v", version)
	}
	if verificado, _ := perfil["emailVerified"].(*types.AttributeValueMemberBOOL); verificado == nil || !verificado.Value {
		t.Fatalf("emailVerified debe quedar en true")
	}
}

func TestE011StoreSinClienteFallaCerrado(t *testing.T) {
	store := dynamoAdapter.NewEmailVerificationStore(nil, "")
	if err := store.ConfirmEmailVerification(context.Background(), e011Confirmacion()); !errors.Is(err, appIdentity.ErrEmailVerificationConfirmNoDisponible) {
		t.Fatalf("sin cliente debe fallar cerrado, obtenido %v", err)
	}
	if _, err := store.LoadEmailVerificationToken(context.Background(), "hash"); !errors.Is(err, appIdentity.ErrEmailVerificationConfirmNoDisponible) {
		t.Fatalf("sin cliente la lectura de token debe fallar cerrado, obtenido %v", err)
	}
}

// contiene evita depender del orden exacto de los fragmentos de una expresion DynamoDB.
func contiene(texto, fragmento string) bool {
	return strings.Contains(texto, fragmento)
}
