package dynamo

// Prueba de contrato del doble de E061/E062: comprueba que la resolución de la
// clave por id y la condición `isRead = false` que emite el adaptador real se
// evalúan igual que en DynamoDB. Si el doble diverge, esta prueba falla antes de
// que el fallo se atribuya al adaptador.

import (
	"context"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	appNotifications "github.com/nexumpr/Nexum-Backend/internal/application/notifications"
)

func TestDobleE061ResuelveClaveYCondicionDeNoLeida(t *testing.T) {
	cliente := nuevoE061Fake()
	cliente.sembrar("user-a", 2, 2, appNotifications.TypeStreak)
	store := NewNotificationsStore(cliente, "nexum-table")
	const id = "018f90ef-08e5-7a68-b09f-000000000001"

	clave, err := store.resolveNotificationKey(context.Background(), "user-a", id)
	if err != nil || clave == nil {
		t.Fatalf("resolver clave: %+v %v", clave, err)
	}
	item := cliente.items[claveItem(clave.PK, clave.SK)]
	if item == nil {
		t.Fatalf("la clave resuelta no existe en el doble: %+v", clave)
	}
	cumple, err := evaluarCondicionE063(item, "#isRead = :falso",
		map[string]string{"#isRead": "isRead"},
		map[string]types.AttributeValue{":falso": boolAttr(false)})
	t.Logf("actual=%T esperado=%T atomo=%v err=%v", item["isRead"], boolAttr(false), cumple, err)
	if err != nil || !cumple {
		t.Fatalf("la condición de no leída no se evalúa como en DynamoDB: %v %v", cumple, err)
	}

	leida, err := store.MarkRead(context.Background(), "user-a", id, time.Now().UTC())
	if err != nil || leida == nil || !leida.IsRead {
		t.Fatalf("MarkRead sobre la clave resuelta: %+v %v", leida, err)
	}
}
