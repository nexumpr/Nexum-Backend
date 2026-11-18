package dynamo_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	dynamoAdapter "github.com/nexumpr/Nexum-Backend/internal/adapters/dynamo"
	appOnboarding "github.com/nexumpr/Nexum-Backend/internal/application/onboarding"
)

// Pruebas de E018 sobre el adaptador REAL `OnboardingStore` montado sobre el doble
// con estado compartido de la suite. Demuestran lectura consistente de
// `USER#<id>/ONBOARDING`, proyección de permisos legacy y ausencia como `found=false`.

const e018Tabla = "NEXUM_CORE_TEST"

const (
	e018Usuario = "11111111-2222-4333-8444-555555555555"
	e018Otro    = "99999999-8888-4777-8666-555555555555"
)

// e018Lector envuelve el doble con estado y registra las opciones de la lectura:
// `ConsistentRead` no puede quedar en falso porque E019 hace CAS sobre `version`.
type e018Lector struct {
	*memoriaDynamo

	lecturas        int
	consistente     bool
	tablaSolicitada string
}

func newE018Lector() *e018Lector {
	return &e018Lector{memoriaDynamo: newMemoriaDynamo()}
}

func (l *e018Lector) GetItem(ctx context.Context, params *dynamodb.GetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
	l.lecturas++
	l.consistente = aws.ToBool(params.ConsistentRead)
	l.tablaSolicitada = aws.ToString(params.TableName)
	return l.memoriaDynamo.GetItem(ctx, params, optFns...)
}

// TestE018AdaptadorLecturaConsistente: la lectura es `GetItem` con
// `ConsistentRead=true` sobre la tabla configurada y la clave del titular.
func TestE018AdaptadorLecturaConsistente(t *testing.T) {
	lector := newE018Lector()
	lector.sembrar("USER#"+e018Usuario, "ONBOARDING", map[string]types.AttributeValue{
		"entityType":     strAttr("USER_ONBOARDING"),
		"userId":         strAttr(e018Usuario),
		"completedSteps": listaAttr("income", "profile", "profile"),
		"microphone":     strAttr("granted"),
		"notifications":  strAttr("denied"),
		"configured":     boolVal(true),
		"version":        numAttr(5),
	})

	store := dynamoAdapter.NewOnboardingStore(lector, e018Tabla)
	registro, encontrado, err := store.GetOnboarding(context.Background(), e018Usuario)
	if err != nil {
		t.Fatalf("lectura valida no debe fallar: %v", err)
	}
	if !encontrado || registro == nil {
		t.Fatalf("la clave sembrada debe encontrarse")
	}
	if !lector.consistente {
		t.Fatalf("la lectura debe usar ConsistentRead=true")
	}
	if lector.tablaSolicitada != e018Tabla {
		t.Fatalf("tabla solicitada %q, esperada %q", lector.tablaSolicitada, e018Tabla)
	}
	if want := []string{"profile", "income"}; !reflect.DeepEqual(registro.CompletedSteps, want) {
		t.Fatalf("pasos proyectados %v, esperados %v", registro.CompletedSteps, want)
	}
	if registro.MicrophoneRaw != "granted" || registro.NotificationsRaw != "denied" {
		t.Fatalf("los valores persistidos deben conservarse: mic=%q notif=%q", registro.MicrophoneRaw, registro.NotificationsRaw)
	}
	if !registro.Configured {
		t.Fatalf("configured debe leerse del atributo booleano")
	}
	if !registro.HasVersion || registro.Version != 5 {
		t.Fatalf("version esperada 5 con HasVersion=true, obtenido %d/%v", registro.Version, registro.HasVersion)
	}
}

// TestE018AdaptadorSinItem: una clave ausente no se convierte en un registro vacío.
func TestE018AdaptadorSinItem(t *testing.T) {
	store := dynamoAdapter.NewOnboardingStore(newE018Lector(), e018Tabla)
	registro, encontrado, err := store.GetOnboarding(context.Background(), e018Usuario)
	if err != nil || encontrado || registro != nil {
		t.Fatalf("clave ausente debe devolver (nil,false,nil), obtenido (%v,%v,%v)", registro, encontrado, err)
	}
}

// TestE018AdaptadorAislamiento: el almacén sólo puede resolver la clave del usuario
// solicitado; el estado de otro usuario no aparece.
func TestE018AdaptadorAislamiento(t *testing.T) {
	lector := newE018Lector()
	lector.sembrar("USER#"+e018Usuario, "ONBOARDING", map[string]types.AttributeValue{
		"entityType": strAttr("USER_ONBOARDING"),
		"version":    numAttr(2),
	})
	store := dynamoAdapter.NewOnboardingStore(lector, e018Tabla)

	if _, encontrado, err := store.GetOnboarding(context.Background(), e018Otro); err != nil || encontrado {
		t.Fatalf("el usuario B no debe observar el item de A: encontrado=%v err=%v", encontrado, err)
	}
}

// TestE018AdaptadorLegacy cubre datos heredados: `completedSteps` ausente se
// proyecta `[]`, permisos `pending`/`denied` conservan su texto, `version` ausente
// deja `HasVersion=false` y `completedAt` NULL no produce marca.
func TestE018AdaptadorLegacy(t *testing.T) {
	lector := newE018Lector()
	lector.sembrar("USER#"+e018Usuario, "ONBOARDING", map[string]types.AttributeValue{
		"entityType":    strAttr("USER_ONBOARDING"),
		"microphone":    strAttr("pending"),
		"notifications": strAttr("denied"),
		"completedAt":   &types.AttributeValueMemberNULL{Value: true},
	})
	store := dynamoAdapter.NewOnboardingStore(lector, e018Tabla)

	registro, encontrado, err := store.GetOnboarding(context.Background(), e018Usuario)
	if err != nil || !encontrado {
		t.Fatalf("el item heredado debe encontrarse: encontrado=%v err=%v", encontrado, err)
	}
	if registro.CompletedSteps == nil || len(registro.CompletedSteps) != 0 {
		t.Fatalf("completedSteps ausente debe proyectarse [], obtenido %#v", registro.CompletedSteps)
	}
	if registro.MicrophoneRaw != "pending" || registro.NotificationsRaw != "denied" {
		t.Fatalf("los textos legacy deben conservarse: mic=%q notif=%q", registro.MicrophoneRaw, registro.NotificationsRaw)
	}
	if registro.HasVersion {
		t.Fatalf("sin atributo version, HasVersion debe ser false")
	}
	if registro.CompletedAt != nil {
		t.Fatalf("completedAt NULL no debe producir marca")
	}
}

// TestE018AdaptadorStringSet: un `SS` heredado se acepta como lista de pasos.
func TestE018AdaptadorStringSet(t *testing.T) {
	lector := newE018Lector()
	lector.sembrar("USER#"+e018Usuario, "ONBOARDING", map[string]types.AttributeValue{
		"entityType":     strAttr("USER_ONBOARDING"),
		"completedSteps": &types.AttributeValueMemberSS{Value: []string{"privacy", "profile"}},
	})
	store := dynamoAdapter.NewOnboardingStore(lector, e018Tabla)

	registro, encontrado, err := store.GetOnboarding(context.Background(), e018Usuario)
	if err != nil || !encontrado {
		t.Fatalf("el item debe encontrarse: encontrado=%v err=%v", encontrado, err)
	}
	if want := []string{"profile", "privacy"}; !reflect.DeepEqual(registro.CompletedSteps, want) {
		t.Fatalf("pasos desde SS %v, esperados %v", registro.CompletedSteps, want)
	}
}

// TestE018AdaptadorEntidadAjena: un item de otra entidad en la misma clave no se
// interpreta como onboarding (falla cerrado como ausencia).
func TestE018AdaptadorEntidadAjena(t *testing.T) {
	lector := newE018Lector()
	lector.sembrar("USER#"+e018Usuario, "ONBOARDING", map[string]types.AttributeValue{
		"entityType": strAttr("USER_CONSENT"),
		"version":    numAttr(1),
	})
	store := dynamoAdapter.NewOnboardingStore(lector, e018Tabla)

	if _, encontrado, err := store.GetOnboarding(context.Background(), e018Usuario); err != nil || encontrado {
		t.Fatalf("una entidad ajena no debe leerse como onboarding: encontrado=%v err=%v", encontrado, err)
	}
}

// TestE018AdaptadorFallaCerrado: sin cliente, tabla o usuario no hay lectura.
func TestE018AdaptadorFallaCerrado(t *testing.T) {
	casos := []struct {
		nombre string
		store  *dynamoAdapter.OnboardingStore
		userID string
	}{
		{"sin cliente", dynamoAdapter.NewOnboardingStore(nil, e018Tabla), e018Usuario},
		{"sin tabla", dynamoAdapter.NewOnboardingStore(newE018Lector(), "  "), e018Usuario},
		{"sin usuario", dynamoAdapter.NewOnboardingStore(newE018Lector(), e018Tabla), "  "},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			if _, _, err := caso.store.GetOnboarding(context.Background(), caso.userID); !errors.Is(err, appOnboarding.ErrUnavailable) {
				t.Fatalf("debe fallar cerrado con ErrUnavailable, obtenido %v", err)
			}
		})
	}
}

// TestE018AdaptadorErrorDeDynamo: un error del cliente nunca se traduce en ausencia.
func TestE018AdaptadorErrorDeDynamo(t *testing.T) {
	store := dynamoAdapter.NewOnboardingStore(lectorFallido{}, e018Tabla)
	if _, _, err := store.GetOnboarding(context.Background(), e018Usuario); !errors.Is(err, appOnboarding.ErrUnavailable) {
		t.Fatalf("un error de DynamoDB debe ser ErrUnavailable, obtenido %v", err)
	}
}

// lectorFallido simula un fallo de red del cliente DynamoDB.
type lectorFallido struct{}

func (lectorFallido) GetItem(context.Context, *dynamodb.GetItemInput, ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
	return nil, errors.New("dynamodb no disponible")
}

// listaAttr construye el atributo `L` de strings canónico.
func listaAttr(valores ...string) types.AttributeValue {
	items := make([]types.AttributeValue, 0, len(valores))
	for _, valor := range valores {
		items = append(items, strAttr(valor))
	}
	return &types.AttributeValueMemberL{Value: items}
}

// boolVal construye el atributo booleano.
func boolVal(valor bool) types.AttributeValue {
	return &types.AttributeValueMemberBOOL{Value: valor}
}

// TestE018AdaptadorErrorNoFiltra: el error devuelto no contiene la tabla ni la clave.
func TestE018AdaptadorErrorNoFiltra(t *testing.T) {
	store := dynamoAdapter.NewOnboardingStore(lectorFallido{}, e018Tabla)
	_, _, err := store.GetOnboarding(context.Background(), e018Usuario)
	if err == nil {
		t.Fatalf("se esperaba error")
	}
	if strings.Contains(err.Error(), e018Tabla) || strings.Contains(err.Error(), e018Usuario) {
		t.Fatalf("el error no debe contener tabla ni usuario: %q", err.Error())
	}
}
