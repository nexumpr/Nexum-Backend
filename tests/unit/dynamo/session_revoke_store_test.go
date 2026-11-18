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

// Pruebas de E009 sobre el adaptador REAL `SessionsStore` montado sobre el doble con estado
// (`memoriaDynamo`), que interpreta las mismas condiciones que emite el adaptador. Se verifica
// la forma exacta de la transacción y la propagación entre dos instancias lógicas.

const (
	e009Tabla    = "NEXUM_CORE_TEST"
	e009Usuario  = "11111111-2222-4333-8444-555555555555"
	e009Sesion   = "018f90ef-08e5-7a68-b09f-0000000000c1"
	e009SesionB  = "018f90ef-08e5-7a68-b09f-0000000000c2"
	e009RefreshA = "refresh-sintetico-e009-a"
	e009RefreshB = "refresh-sintetico-e009-b"
)

var e009AhoraStore = time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)

// e009Dynamo envuelve el doble con estado para registrar las lecturas y transacciones que emite
// el adaptador, sin reinterpretar su semántica.
type e009Dynamo struct {
	*memoriaDynamo
	mu            sync.Mutex
	lecturas      []*dynamodb.GetItemInput
	transacciones []*dynamodb.TransactWriteItemsInput
}

func newE009Dynamo() *e009Dynamo {
	return &e009Dynamo{memoriaDynamo: newMemoriaDynamo()}
}

func (m *e009Dynamo) GetItem(ctx context.Context, params *dynamodb.GetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
	m.mu.Lock()
	m.lecturas = append(m.lecturas, params)
	m.mu.Unlock()
	return m.memoriaDynamo.GetItem(ctx, params, optFns...)
}

func (m *e009Dynamo) TransactWriteItems(ctx context.Context, params *dynamodb.TransactWriteItemsInput, optFns ...func(*dynamodb.Options)) (*dynamodb.TransactWriteItemsOutput, error) {
	m.mu.Lock()
	m.transacciones = append(m.transacciones, params)
	m.mu.Unlock()
	return m.memoriaDynamo.TransactWriteItems(ctx, params, optFns...)
}

// TestE009StoreTransaccionDosAcciones verifica la forma exacta: lectura consistente de la
// sesión y UNA transacción con la sesión (CAS de estado y versión) y el localizador del refresh.
func TestE009StoreTransaccionDosAcciones(t *testing.T) {
	m := newE009Dynamo()
	e004SembrarSesion(t, m.memoriaDynamo, e009Usuario, e009Sesion, "familia-e009", e009RefreshA)

	ctx := context.Background()
	store := dynamoAdapter.NewSessionsStore(m, e009Tabla)

	registro, err := store.LoadSessionForRevoke(ctx, e009Usuario, e009Sesion)
	if err != nil {
		t.Fatalf("lectura valida devolvio error: %v", err)
	}
	if registro == nil {
		t.Fatalf("la sesion sembrada debe existir")
	}
	if registro.UserID != e009Usuario || registro.SessionID != e009Sesion {
		t.Fatalf("propietario o identificador incorrectos: %+v", registro)
	}
	if registro.RefreshHash != appIdentity.HashToken(e009RefreshA) {
		t.Fatalf("debe leerse el digest del refresh vigente")
	}
	if !registro.HasVersion || registro.Version != 1 {
		t.Fatalf("debe leerse la version 1, obtenido %+v", registro)
	}
	if len(m.lecturas) != 1 || m.lecturas[0].ConsistentRead == nil || !*m.lecturas[0].ConsistentRead {
		t.Fatalf("la lectura de la sesion debe ser consistente")
	}

	if err := store.RevokeOwnedSession(ctx, appIdentity.SessionRevocation{
		UserID:          e009Usuario,
		SessionID:       e009Sesion,
		RefreshHash:     registro.RefreshHash,
		Reason:          appIdentity.RevokeReasonUserRevokedSession,
		Now:             e009AhoraStore,
		ExpectedVersion: registro.Version,
		HasVersion:      registro.HasVersion,
	}); err != nil {
		t.Fatalf("revocacion valida devolvio error: %v", err)
	}

	if len(m.transacciones) != 1 {
		t.Fatalf("debe emitirse exactamente una transaccion, obtenido %d", len(m.transacciones))
	}
	items := m.transacciones[0].TransactItems
	if len(items) != 2 {
		t.Fatalf("la transaccion debe tener dos acciones, obtenido %d", len(items))
	}

	sesion := items[0].Update
	if sesion == nil {
		t.Fatalf("la primera accion debe ser un Update de la sesion")
	}
	if pk := stringAttrDe(sesion.Key, "PK"); pk != domainDynamo.PrefixUser+e009Usuario {
		t.Fatalf("PK de sesion inesperada: %q", pk)
	}
	if sk := stringAttrDe(sesion.Key, "SK"); sk != domainDynamo.PrefixSession+e009Sesion {
		t.Fatalf("SK de sesion inesperada: %q", sk)
	}
	condicion := aws.ToString(sesion.ConditionExpression)
	for _, esperado := range []string{"attribute_exists(PK)", "#status = :active", "#version = :expected"} {
		if !strings.Contains(condicion, esperado) {
			t.Fatalf("la condicion de la sesion debe incluir %q, obtenido %q", esperado, condicion)
		}
	}
	actualizacion := aws.ToString(sesion.UpdateExpression)
	for _, esperado := range []string{"#status = :revoked", "revokedAt = :now", "revokeReason = :reason", "ADD #version :one"} {
		if !strings.Contains(actualizacion, esperado) {
			t.Fatalf("la actualizacion de la sesion debe incluir %q, obtenido %q", esperado, actualizacion)
		}
	}

	localizador := items[1].Update
	if localizador == nil {
		t.Fatalf("la segunda accion debe ser un Update del localizador de refresh")
	}
	if pk := stringAttrDe(localizador.Key, "PK"); pk != "REFRESH#"+appIdentity.HashToken(e009RefreshA) {
		t.Fatalf("PK del localizador inesperada: %q", pk)
	}
	if sk := stringAttrDe(localizador.Key, "SK"); sk != "SESSION" {
		t.Fatalf("SK del localizador inesperada: %q", sk)
	}
	if condicionLocalizador := aws.ToString(localizador.ConditionExpression); !strings.Contains(condicionLocalizador, "attribute_exists(PK)") {
		t.Fatalf("el localizador debe exigir existencia, obtenido %q", condicionLocalizador)
	}
}

// TestE009StoreRevocaYPropagaEntreInstancias cubre la consistencia entre dos nodos lógicos: la
// revocación confirmada por el nodo A es observable por el nodo B y el refresh deja de rotar.
func TestE009StoreRevocaYPropagaEntreInstancias(t *testing.T) {
	m := newE009Dynamo()
	e004SembrarPerfil(m.memoriaDynamo, e009Usuario)
	e004SembrarSesion(t, m.memoriaDynamo, e009Usuario, e009Sesion, "familia-e009", e009RefreshA)
	e004SembrarSesion(t, m.memoriaDynamo, e009Usuario, e009SesionB, "familia-e009-b", e009RefreshB)

	ctx := context.Background()
	nodoA := dynamoAdapter.NewSessionsStore(m, e009Tabla)
	nodoB := dynamoAdapter.NewSessionsStore(m, e009Tabla)

	registro, err := nodoA.LoadSessionForRevoke(ctx, e009Usuario, e009Sesion)
	if err != nil || registro == nil {
		t.Fatalf("lectura en nodo A fallo: %v", err)
	}
	if err := nodoA.RevokeOwnedSession(ctx, appIdentity.SessionRevocation{
		UserID:          e009Usuario,
		SessionID:       e009Sesion,
		RefreshHash:     registro.RefreshHash,
		Now:             e009AhoraStore,
		ExpectedVersion: registro.Version,
		HasVersion:      registro.HasVersion,
	}); err != nil {
		t.Fatalf("revocacion en nodo A fallo: %v", err)
	}

	releida, err := nodoB.LoadSessionForRevoke(ctx, e009Usuario, e009Sesion)
	if err != nil || releida == nil {
		t.Fatalf("lectura en nodo B fallo: %v", err)
	}
	if !appIdentity.SessionIsRevoked(releida.Status) {
		t.Fatalf("el nodo B debe observar la sesion revocada, obtenido %q", releida.Status)
	}
	if version, _ := attributeIntDe(e004EstadoSesion(m.memoriaDynamo, e009Usuario, e009Sesion), "version"); version != 2 {
		t.Fatalf("version esperada 2 tras el CAS, obtenida %d", version)
	}

	// El refresh vigente de la sesión revocada ya no renueva (E003 responde 401).
	if _, err := e004RefreshService(m.memoriaDynamo).Refresh(ctx, appIdentity.RefreshCommand{RefreshToken: e009RefreshA}); !errors.Is(err, appIdentity.ErrInvalidRefreshToken) {
		t.Fatalf("el refresh de la sesion revocada debe ser 401, obtenido %v", err)
	}

	// La otra sesión del mismo usuario sigue intacta: revocar una no revoca las demás.
	otra, err := nodoB.LoadSessionForRevoke(ctx, e009Usuario, e009SesionB)
	if err != nil || otra == nil {
		t.Fatalf("la segunda sesion debe seguir existiendo: %v", err)
	}
	if appIdentity.SessionIsRevoked(otra.Status) {
		t.Fatalf("la segunda sesion no debe revocarse")
	}
}

// TestE009StoreFallaCerradoSinClienteNiTabla verifica que el adaptador nunca sustituye la
// persistencia por memoria local.
func TestE009StoreFallaCerradoSinClienteNiTabla(t *testing.T) {
	ctx := context.Background()
	vacio := dynamoAdapter.NewSessionsStore(nil, e009Tabla)
	if _, err := vacio.LoadSessionForRevoke(ctx, e009Usuario, e009Sesion); !errors.Is(err, appIdentity.ErrSessionRevokeUnavailable) {
		t.Fatalf("sin cliente debe ser 503, obtenido %v", err)
	}
	if err := vacio.RevokeOwnedSession(ctx, appIdentity.SessionRevocation{UserID: e009Usuario, SessionID: e009Sesion}); !errors.Is(err, appIdentity.ErrSessionRevokeUnavailable) {
		t.Fatalf("sin cliente debe ser 503, obtenido %v", err)
	}

	m := newE009Dynamo()
	sinTabla := dynamoAdapter.NewSessionsStore(m, "")
	if _, err := sinTabla.LoadSessionForRevoke(ctx, e009Usuario, e009Sesion); !errors.Is(err, appIdentity.ErrSessionRevokeUnavailable) {
		t.Fatalf("sin tabla debe ser 503, obtenido %v", err)
	}
}

// TestE009StoreEntidadAjenaEsInexistente verifica que una clave con otra entidad no se trata
// como sesión: el llamador responde 404 y no se revoca nada.
func TestE009StoreEntidadAjenaEsInexistente(t *testing.T) {
	m := newE009Dynamo()
	pk, sk := domainDynamo.UserSessionKeys(e009Usuario, e009Sesion)
	m.sembrar(pk, sk, map[string]types.AttributeValue{
		"entityType": strAttr("USER_PROFILE"),
		"userId":     strAttr(e009Usuario),
		"status":     strAttr(appIdentity.StatusActive),
	})

	registro, err := dynamoAdapter.NewSessionsStore(m, e009Tabla).LoadSessionForRevoke(context.Background(), e009Usuario, e009Sesion)
	if err != nil {
		t.Fatalf("lectura no debe fallar: %v", err)
	}
	if registro != nil {
		t.Fatalf("una entidad ajena no es una sesion: %+v", registro)
	}
}
