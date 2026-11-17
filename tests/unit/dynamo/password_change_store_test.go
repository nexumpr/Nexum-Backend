package dynamo_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	dynamoAdapter "github.com/nexumpr/Nexum-Backend/internal/adapters/dynamo"
	appIdentity "github.com/nexumpr/Nexum-Backend/internal/application/identity"
	domainDynamo "github.com/nexumpr/Nexum-Backend/internal/domain/dynamo"
)

// Pruebas de E007 sobre el adaptador REAL y el doble con estado compartido del paquete: el
// mismo que interpreta las condiciones que emite el adaptador. No sustituyen la verificación
// AWS, pero demuestran la transacción de una sola acción, el CAS de versión, el incremento de
// época (A6) y el fallo cerrado sin memoria como autoridad.

const (
	e007Tabla = "NEXUM_CORE_TEST"
	e007User  = "cccccccc-dddd-4eee-8fff-000000000007"
)

var e007StoreAhora = time.Date(2026, 11, 17, 11, 0, 0, 0, time.UTC)

// El puerto sólo exige GetItem y TransactWriteItems: no hay Scan en la superficie, de modo que
// el adaptador no puede enumerar la tabla ni siquiera por error.
var _ dynamoAdapter.PasswordChangeAPI = e004Fallo{}

// El adaptador cumple el puerto de aplicación.
var _ appIdentity.PasswordChangeStore = (*dynamoAdapter.PasswordChangeStore)(nil)

// e007Espia instrumenta el doble para inspeccionar exactamente qué se envía a DynamoDB.
type e007Espia struct {
	*memoriaDynamo
	mu           sync.Mutex
	llamadasTx   int
	ultimaTx     *dynamodb.TransactWriteItemsInput
	lecturas     int
	consistentes int
}

func newE007Espia() *e007Espia {
	return &e007Espia{memoriaDynamo: newMemoriaDynamo()}
}

func (e *e007Espia) GetItem(ctx context.Context, params *dynamodb.GetItemInput, opt ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
	e.mu.Lock()
	e.lecturas++
	if params.ConsistentRead != nil && *params.ConsistentRead {
		e.consistentes++
	}
	e.mu.Unlock()
	return e.memoriaDynamo.GetItem(ctx, params, opt...)
}

func (e *e007Espia) TransactWriteItems(ctx context.Context, params *dynamodb.TransactWriteItemsInput, opt ...func(*dynamodb.Options)) (*dynamodb.TransactWriteItemsOutput, error) {
	e.mu.Lock()
	e.llamadasTx++
	e.ultimaTx = params
	e.mu.Unlock()
	return e.memoriaDynamo.TransactWriteItems(ctx, params, opt...)
}

func e007SembrarPerfil(m *memoriaDynamo, attrs map[string]types.AttributeValue) {
	base := map[string]types.AttributeValue{
		"entityType":   strAttr("USER_PROFILE"),
		"userId":       strAttr(e007User),
		"status":       strAttr(appIdentity.StatusActive),
		"email":        strAttr("change@example.invalid"),
		"passwordHash": strAttr("hash:OldPassphrase#2026"),
	}
	for clave, valor := range attrs {
		base[clave] = valor
	}
	pk, sk := domainDynamo.UserKeys(e007User)
	m.sembrar(pk, sk, base)
}

func e007Aplicacion() appIdentity.PasswordChangeApplication {
	return appIdentity.PasswordChangeApplication{
		UserID:               e007User,
		PasswordHash:         "hash:NewPassphrase#2026",
		ExpectedVersion:      3,
		HasVersion:           true,
		ExpectedSessionEpoch: 2,
		NextSessionEpoch:     3,
		Now:                  e007StoreAhora,
	}
}

func e007PerfilActual(m *memoriaDynamo) map[string]types.AttributeValue {
	pk, sk := domainDynamo.UserKeys(e007User)
	return m.leer(pk, sk)
}

// TestE007StoreTransaccionDeUnaSolaAccion inspecciona el payload real de la transacción: una
// única acción Update sobre el perfil, con CAS de estado, versión y época.
func TestE007StoreTransaccionDeUnaSolaAccion(t *testing.T) {
	espia := newE007Espia()
	e007SembrarPerfil(espia.memoriaDynamo, map[string]types.AttributeValue{
		"version":      numAttr(3),
		"sessionEpoch": numAttr(2),
	})
	store := dynamoAdapter.NewPasswordChangeStore(espia, e007Tabla)

	if err := store.ApplyPasswordChange(context.Background(), e007Aplicacion()); err != nil {
		t.Fatalf("la transaccion valida debe aplicarse: %v", err)
	}

	if espia.llamadasTx != 1 {
		t.Fatalf("debe emitirse exactamente una transaccion, obtenidas %d", espia.llamadasTx)
	}
	if espia.ultimaTx == nil {
		t.Fatalf("no se capturo la transaccion")
	}
	items := espia.ultimaTx.TransactItems
	if len(items) != 1 {
		t.Fatalf("la transaccion de E007 es de UNA sola accion, obtenidas %d", len(items))
	}
	if items[0].Update == nil {
		t.Fatalf("la accion debe ser un Update")
	}
	if items[0].Put != nil || items[0].ConditionCheck != nil || items[0].Delete != nil {
		t.Fatalf("no puede haber acciones distintas de Update")
	}

	update := items[0].Update
	if aws.ToString(update.TableName) != e007Tabla {
		t.Fatalf("tabla inesperada: %q", aws.ToString(update.TableName))
	}
	if mapaString(update.Key, "PK") != domainDynamo.PrefixUser+e007User || mapaString(update.Key, "SK") != domainDynamo.PrefixProfile {
		t.Fatalf("la clave debe ser USER#<id>/PROFILE, obtenida %q/%q",
			mapaString(update.Key, "PK"), mapaString(update.Key, "SK"))
	}

	condicion := aws.ToString(update.ConditionExpression)
	for _, atomo := range []string{
		"attribute_exists(PK)",
		"#status = :active",
		"#version = :expected",
		"attribute_not_exists(#epoch) OR #epoch = :expectedEpoch",
	} {
		if !strings.Contains(condicion, atomo) {
			t.Fatalf("la condicion debe contener %q, obtenida %q", atomo, condicion)
		}
	}

	expresion := aws.ToString(update.UpdateExpression)
	for _, fragmento := range []string{
		"passwordHash = :hash",
		"passwordChangedAt = :now",
		"sessionEpoch = :nextEpoch",
		"ADD #version :one",
	} {
		if !strings.Contains(expresion, fragmento) {
			t.Fatalf("la actualizacion debe contener %q, obtenida %q", fragmento, expresion)
		}
	}

	// Todos los valores declarados se usan: DynamoDB rechaza valores sobrantes.
	for _, valor := range []string{":hash", ":now", ":one", ":active", ":expected", ":expectedEpoch", ":nextEpoch"} {
		if _, ok := update.ExpressionAttributeValues[valor]; !ok {
			t.Fatalf("falta el valor de expresion %q", valor)
		}
	}
	if len(update.ExpressionAttributeValues) != 7 {
		t.Fatalf("no deben declararse valores sobrantes, obtenidos %d", len(update.ExpressionAttributeValues))
	}
}

// TestE007StoreAplicaCambioEIncrementaEpoca comprueba el efecto durable completo.
func TestE007StoreAplicaCambioEIncrementaEpoca(t *testing.T) {
	m := newMemoriaDynamo()
	e007SembrarPerfil(m, map[string]types.AttributeValue{
		"version":      numAttr(3),
		"sessionEpoch": numAttr(2),
	})
	store := dynamoAdapter.NewPasswordChangeStore(m, e007Tabla)
	ctx := context.Background()

	perfil, err := store.LoadPasswordChangeProfile(ctx, e007User)
	if err != nil || perfil == nil {
		t.Fatalf("el perfil debe leerse: perfil=%v err=%v", perfil, err)
	}
	if perfil.UserID != e007User || perfil.Status != appIdentity.StatusActive {
		t.Fatalf("perfil inesperado: %+v", perfil)
	}
	if perfil.PasswordHash != "hash:OldPassphrase#2026" {
		t.Fatalf("hash vigente inesperado: %q", perfil.PasswordHash)
	}
	if !perfil.HasVersion || perfil.Version != 3 {
		t.Fatalf("version esperada 3, obtenida %d (has=%v)", perfil.Version, perfil.HasVersion)
	}
	if perfil.SessionEpoch != 2 {
		t.Fatalf("epoca esperada 2, obtenida %d", perfil.SessionEpoch)
	}

	if err := store.ApplyPasswordChange(ctx, e007Aplicacion()); err != nil {
		t.Fatalf("la transaccion valida debe aplicarse: %v", err)
	}

	item := e007PerfilActual(m)
	if hash := stringAttrDe(item, "passwordHash"); hash != "hash:NewPassphrase#2026" {
		t.Fatalf("el passwordHash no se actualizo: %q", hash)
	}
	if epoch, ok := attributeIntDe(item, "sessionEpoch"); !ok || epoch != 3 {
		t.Fatalf("sessionEpoch debe incrementarse a 3, obtenido %v", epoch)
	}
	if version, ok := attributeIntDe(item, "version"); !ok || version != 4 {
		t.Fatalf("version debe incrementarse a 4, obtenido %v", version)
	}
	if marca := stringAttrDe(item, "passwordChangedAt"); marca != e007StoreAhora.Format(time.RFC3339) {
		t.Fatalf("passwordChangedAt inesperado: %q", marca)
	}
	// Nada más cambia: identidad y estado intactos.
	if stringAttrDe(item, "email") != "change@example.invalid" || stringAttrDe(item, "status") != appIdentity.StatusActive {
		t.Fatalf("la transaccion no debe alterar identidad ni estado")
	}
}

// TestE007StoreRechazaVersionObsoleta cubre el CAS: si el perfil avanzó, no se escribe nada.
func TestE007StoreRechazaVersionObsoleta(t *testing.T) {
	m := newMemoriaDynamo()
	e007SembrarPerfil(m, map[string]types.AttributeValue{
		"version":      numAttr(5),
		"sessionEpoch": numAttr(2),
	})
	store := dynamoAdapter.NewPasswordChangeStore(m, e007Tabla)

	err := store.ApplyPasswordChange(context.Background(), e007Aplicacion())
	if !errors.Is(err, appIdentity.ErrPasswordChangeConflict) {
		t.Fatalf("una version obsoleta debe ser conflicto, obtenido %v", err)
	}
	item := e007PerfilActual(m)
	if hash := stringAttrDe(item, "passwordHash"); hash != "hash:OldPassphrase#2026" {
		t.Fatalf("no debe escribirse el hash nuevo, obtenido %q", hash)
	}
	if version, _ := attributeIntDe(item, "version"); version != 5 {
		t.Fatalf("la version no debe cambiar, obtenida %d", version)
	}
	if epoch, _ := attributeIntDe(item, "sessionEpoch"); epoch != 2 {
		t.Fatalf("la epoca no debe cambiar, obtenida %d", epoch)
	}
}

// TestE007StoreRechazaEpocaDesalineada cubre la condición de época: una lectura obsoleta no
// puede incrementar la época dos veces.
func TestE007StoreRechazaEpocaDesalineada(t *testing.T) {
	m := newMemoriaDynamo()
	e007SembrarPerfil(m, map[string]types.AttributeValue{
		"version":      numAttr(3),
		"sessionEpoch": numAttr(4),
	})
	store := dynamoAdapter.NewPasswordChangeStore(m, e007Tabla)

	err := store.ApplyPasswordChange(context.Background(), e007Aplicacion())
	if !errors.Is(err, appIdentity.ErrPasswordChangeConflict) {
		t.Fatalf("una epoca desalineada debe ser conflicto, obtenido %v", err)
	}
	if epoch, _ := attributeIntDe(e007PerfilActual(m), "sessionEpoch"); epoch != 4 {
		t.Fatalf("la epoca no debe cambiar, obtenida %d", epoch)
	}
}

// TestE007StoreRechazaPerfilNoActivo: la condición de estado impide escribir sobre una cuenta
// deshabilitada, aunque la lectura previa la hubiera visto ACTIVE.
func TestE007StoreRechazaPerfilNoActivo(t *testing.T) {
	m := newMemoriaDynamo()
	e007SembrarPerfil(m, map[string]types.AttributeValue{
		"status":       strAttr("DISABLED"),
		"version":      numAttr(3),
		"sessionEpoch": numAttr(2),
	})
	store := dynamoAdapter.NewPasswordChangeStore(m, e007Tabla)

	err := store.ApplyPasswordChange(context.Background(), e007Aplicacion())
	if !errors.Is(err, appIdentity.ErrPasswordChangeConflict) {
		t.Fatalf("una cuenta no ACTIVE debe abortar la transaccion, obtenido %v", err)
	}
	if hash := stringAttrDe(e007PerfilActual(m), "passwordHash"); hash != "hash:OldPassphrase#2026" {
		t.Fatalf("no debe escribirse el hash nuevo, obtenido %q", hash)
	}
}

// TestE007StorePerfilLegadoSinVersion: sin `version` la condición exige su ausencia, no declara
// `:expected` (DynamoDB rechaza valores no usados) e inicializa versión y época.
func TestE007StorePerfilLegadoSinVersion(t *testing.T) {
	espia := newE007Espia()
	e007SembrarPerfil(espia.memoriaDynamo, nil)
	store := dynamoAdapter.NewPasswordChangeStore(espia, e007Tabla)
	ctx := context.Background()

	perfil, err := store.LoadPasswordChangeProfile(ctx, e007User)
	if err != nil || perfil == nil {
		t.Fatalf("el perfil legado debe leerse: perfil=%v err=%v", perfil, err)
	}
	if perfil.HasVersion {
		t.Fatalf("un perfil sin `version` no debe declarar HasVersion")
	}
	if perfil.SessionEpoch != 0 {
		t.Fatalf("la epoca ausente equivale a 0, obtenida %d", perfil.SessionEpoch)
	}

	apply := e007Aplicacion()
	apply.HasVersion = false
	apply.ExpectedVersion = 0
	apply.ExpectedSessionEpoch = 0
	apply.NextSessionEpoch = 1
	if err := store.ApplyPasswordChange(ctx, apply); err != nil {
		t.Fatalf("un perfil legado debe poder cambiar su contrasena: %v", err)
	}

	update := espia.ultimaTx.TransactItems[0].Update
	condicion := aws.ToString(update.ConditionExpression)
	if !strings.Contains(condicion, "attribute_not_exists(#version)") {
		t.Fatalf("la condicion legada debe exigir la ausencia de version: %q", condicion)
	}
	if strings.Contains(condicion, "#version = :expected") {
		t.Fatalf("la condicion legada no puede exigir igualdad de version: %q", condicion)
	}
	if _, ok := update.ExpressionAttributeValues[":expected"]; ok {
		t.Fatalf("un valor de expresion no utilizado haria fallar DynamoDB")
	}
	if expresion := aws.ToString(update.UpdateExpression); !strings.Contains(expresion, "#version = :one") {
		t.Fatalf("la actualizacion legada debe inicializar la version: %q", expresion)
	}

	item := e007PerfilActual(espia.memoriaDynamo)
	if version, ok := attributeIntDe(item, "version"); !ok || version != 1 {
		t.Fatalf("la version debe inicializarse a 1, obtenido %v", version)
	}
	if epoch, ok := attributeIntDe(item, "sessionEpoch"); !ok || epoch != 1 {
		t.Fatalf("la epoca debe pasar de 0 a 1, obtenido %v", epoch)
	}
}

// TestE007StoreLecturaConsistenteYPerfilAusente: la lectura es fuerte y la ausencia no es error.
func TestE007StoreLecturaConsistenteYPerfilAusente(t *testing.T) {
	espia := newE007Espia()
	store := dynamoAdapter.NewPasswordChangeStore(espia, e007Tabla)

	perfil, err := store.LoadPasswordChangeProfile(context.Background(), "usuario-sin-perfil")
	if err != nil {
		t.Fatalf("un perfil ausente no es un error de servicio: %v", err)
	}
	if perfil != nil {
		t.Fatalf("un perfil ausente debe devolver nil")
	}
	if espia.lecturas != 1 || espia.consistentes != 1 {
		t.Fatalf("la lectura debe ser unica y consistente: lecturas=%d consistentes=%d", espia.lecturas, espia.consistentes)
	}

	// Un userId vacío no consulta DynamoDB.
	if perfil, err := store.LoadPasswordChangeProfile(context.Background(), "   "); err != nil || perfil != nil {
		t.Fatalf("un userId vacio no debe consultar: perfil=%v err=%v", perfil, err)
	}
	if espia.lecturas != 1 {
		t.Fatalf("no debe emitirse una segunda lectura, obtenidas %d", espia.lecturas)
	}
}

// TestE007StoreEntidadDistintaNoEsPerfil: una clave reutilizada por otra entidad no se trata
// como perfil, de modo que no se escribe sobre ella.
func TestE007StoreEntidadDistintaNoEsPerfil(t *testing.T) {
	m := newMemoriaDynamo()
	e007SembrarPerfil(m, map[string]types.AttributeValue{
		"entityType": strAttr("USER_SESSION"),
		"version":    numAttr(3),
	})
	store := dynamoAdapter.NewPasswordChangeStore(m, e007Tabla)

	perfil, err := store.LoadPasswordChangeProfile(context.Background(), e007User)
	if err != nil || perfil != nil {
		t.Fatalf("una entidad distinta no es un perfil: perfil=%v err=%v", perfil, err)
	}
}

// TestE007StoreFallaCerrado: sin cliente, sin tabla o con DynamoDB caído nunca se confirma.
func TestE007StoreFallaCerrado(t *testing.T) {
	ctx := context.Background()

	casos := []struct {
		nombre string
		store  *dynamoAdapter.PasswordChangeStore
	}{
		{"sin cliente", dynamoAdapter.NewPasswordChangeStore(nil, e007Tabla)},
		{"sin tabla", dynamoAdapter.NewPasswordChangeStore(newMemoriaDynamo(), "")},
		{"cliente caido", dynamoAdapter.NewPasswordChangeStore(e004Fallo{}, e007Tabla)},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			if _, err := caso.store.LoadPasswordChangeProfile(ctx, e007User); !errors.Is(err, appIdentity.ErrPasswordChangeUnavailable) {
				t.Fatalf("la lectura debe fallar cerrada, obtenido %v", err)
			}
			if err := caso.store.ApplyPasswordChange(ctx, e007Aplicacion()); !errors.Is(err, appIdentity.ErrPasswordChangeUnavailable) {
				t.Fatalf("la escritura debe fallar cerrada, obtenido %v", err)
			}
		})
	}

	// Payload incompleto: tampoco se emite la transacción.
	m := newMemoriaDynamo()
	e007SembrarPerfil(m, map[string]types.AttributeValue{"version": numAttr(3)})
	store := dynamoAdapter.NewPasswordChangeStore(m, e007Tabla)
	incompleta := e007Aplicacion()
	incompleta.UserID = "  "
	if err := store.ApplyPasswordChange(ctx, incompleta); !errors.Is(err, appIdentity.ErrPasswordChangeUnavailable) {
		t.Fatalf("un payload sin titularidad debe fallar cerrada, obtenido %v", err)
	}
	incompleta = e007Aplicacion()
	incompleta.PasswordHash = ""
	if err := store.ApplyPasswordChange(ctx, incompleta); !errors.Is(err, appIdentity.ErrPasswordChangeUnavailable) {
		t.Fatalf("un payload sin hash debe fallar cerrada, obtenido %v", err)
	}
	if hash := stringAttrDe(e007PerfilActual(m), "passwordHash"); hash != "hash:OldPassphrase#2026" {
		t.Fatalf("nada debe escribirse con payload incompleto, obtenido %q", hash)
	}
}
