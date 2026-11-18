package dynamo_test

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	dynamoAdapter "github.com/nexumpr/Nexum-Backend/internal/adapters/dynamo"
	appProfile "github.com/nexumpr/Nexum-Backend/internal/application/profile"
)

// Pruebas del adaptador de E012 sobre un doble con estado que interpreta GetItem. El doble
// es exclusivamente de prueba: la composición productiva usa el cliente real de DynamoDB.

const (
	e012Tabla   = "NEXUM_CORE_TEST"
	e012Usuario = "11111111-2222-4333-8444-555555555555"
	e012Otro    = "99999999-8888-4777-8666-555555555555"
)

// e012SpyAPI envuelve el doble para inspeccionar exactamente qué se pidió a DynamoDB: clave,
// consistencia y ausencia de Scan.
type e012SpyAPI struct {
	inner  dynamoAdapter.ProfileAPI
	ultimo *dynamodb.GetItemInput
	veces  int
}

func (s *e012SpyAPI) GetItem(ctx context.Context, params *dynamodb.GetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
	s.veces++
	s.ultimo = params
	return s.inner.GetItem(ctx, params, optFns...)
}

// e012SembrarPerfil escribe el item con la MISMA forma que produce E001, incluidos los
// atributos internos que no deben exponerse.
func e012SembrarPerfil(mem *memoriaDynamo, userID, email string) {
	mem.sembrar("USER#"+userID, "PROFILE", map[string]types.AttributeValue{
		"entityType":           strAttr("USER_PROFILE"),
		"userId":               strAttr(userID),
		"email":                strAttr(email),
		"displayName":          strAttr("Usuario Ejemplo"),
		"passwordHash":         strAttr("$argon2id$v=19$m=65536,t=3,p=2$hash-sintetico"),
		"emailVerified":        boolAttr(true),
		"sessionEpoch":         numAttr(3),
		"status":               strAttr("ACTIVE"),
		"privacyNoticeVersion": strAttr("2026-11-v2"),
		"privacyAcceptedAt":    strAttr("2026-11-01T00:00:00Z"),
		"createdAt":            strAttr("2026-11-01T12:00:00Z"),
		"updatedAt":            strAttr("2026-11-01T12:00:00Z"),
		"version":              numAttr(4),
	})
}

// TestE012StoreLeePerfilPropio cubre la lectura con consistencia fuerte de la clave exacta.
func TestE012StoreLeePerfilPropio(t *testing.T) {
	mem := newMemoriaDynamo()
	e012SembrarPerfil(mem, e012Usuario, "ejemplo@example.invalid")
	spy := &e012SpyAPI{inner: mem}
	store := dynamoAdapter.NewProfileStore(spy, e012Tabla)

	registro, err := store.LoadProfile(context.Background(), e012Usuario)
	if err != nil {
		t.Fatalf("LoadProfile devolvio error: %v", err)
	}
	if registro == nil {
		t.Fatal("el perfil sembrado debe existir")
	}
	if registro.UserID != e012Usuario || registro.Email != "ejemplo@example.invalid" ||
		registro.DisplayName != "Usuario Ejemplo" || !registro.EmailVerified {
		t.Fatalf("proyeccion inesperada: %+v", registro)
	}
	if !registro.HasVersion || registro.Version != 4 {
		t.Fatalf("la version debe ser 4: %+v", registro)
	}
	if !registro.HasCreatedAt || registro.CreatedAt.UTC().Format("2006-01-02T15:04:05Z") != "2026-11-01T12:00:00Z" {
		t.Fatalf("createdAt inesperado: %+v", registro)
	}

	if spy.veces != 1 || spy.ultimo == nil {
		t.Fatalf("debe hacerse exactamente una lectura, hubo %d", spy.veces)
	}
	if !aws.ToBool(spy.ultimo.ConsistentRead) {
		t.Fatal("la lectura debe ser consistente: el ETag de salida no puede venir de una replica")
	}
	if got := stringAttrValueMap(spy.ultimo.Key, "PK"); got != "USER#"+e012Usuario {
		t.Fatalf("PK inesperada: %q", got)
	}
	if got := stringAttrValueMap(spy.ultimo.Key, "SK"); got != "PROFILE" {
		t.Fatalf("SK inesperada: %q", got)
	}
	if aws.ToString(spy.ultimo.TableName) != e012Tabla {
		t.Fatalf("tabla inesperada: %q", aws.ToString(spy.ultimo.TableName))
	}
}

// TestE012StorePerfilInexistenteYEntidadAjena: ni una clave ausente ni otra entidad en la
// misma clave se presentan como perfil.
func TestE012StorePerfilInexistenteYEntidadAjena(t *testing.T) {
	mem := newMemoriaDynamo()
	mem.sembrar("USER#"+e012Otro, "PROFILE", map[string]types.AttributeValue{
		"entityType": strAttr("USER_SESSION"),
		"userId":     strAttr(e012Otro),
	})
	store := dynamoAdapter.NewProfileStore(mem, e012Tabla)

	inexistente, err := store.LoadProfile(context.Background(), e012Usuario)
	if err != nil || inexistente != nil {
		t.Fatalf("una clave ausente debe ser (nil, nil): %+v / %v", inexistente, err)
	}
	ajena, err := store.LoadProfile(context.Background(), e012Otro)
	if err != nil || ajena != nil {
		t.Fatalf("otra entidad no es un perfil: %+v / %v", ajena, err)
	}
}

// TestE012StoreFallaCerradoSinConfiguracion: sin cliente o sin tabla no se lee nada.
func TestE012StoreFallaCerradoSinConfiguracion(t *testing.T) {
	if _, err := dynamoAdapter.NewProfileStore(nil, e012Tabla).LoadProfile(context.Background(), e012Usuario); !errors.Is(err, appProfile.ErrProfileUnavailable) {
		t.Fatalf("sin cliente debe fallar cerrado, obtuvo %v", err)
	}
	if _, err := dynamoAdapter.NewProfileStore(newMemoriaDynamo(), "").LoadProfile(context.Background(), e012Usuario); !errors.Is(err, appProfile.ErrProfileUnavailable) {
		t.Fatalf("sin tabla debe fallar cerrado, obtuvo %v", err)
	}
	if _, err := dynamoAdapter.NewProfileStore(newMemoriaDynamo(), e012Tabla).LoadProfile(context.Background(), "  "); !errors.Is(err, appProfile.ErrProfileUnauthorized) {
		t.Fatalf("sin sujeto debe rechazarse, obtuvo %v", err)
	}
}

// boolAttr construye un atributo booleano para los items sembrados.
func boolAttr(value bool) types.AttributeValue {
	return &types.AttributeValueMemberBOOL{Value: value}
}
