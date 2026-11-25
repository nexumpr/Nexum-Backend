package dynamo_test

import (
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	dynamoAdapter "github.com/nexumpr/Nexum-Backend/internal/adapters/dynamo"
	appAccount "github.com/nexumpr/Nexum-Backend/internal/application/account"
	domainDeletion "github.com/nexumpr/Nexum-Backend/internal/domain/deletion"
)

// TestE014TitularContradictorioNoBorraItemGlobal fija la invariante de titular de los ítems
// globales: si un ítem declara `userID` y `userId` con valores DISTINTOS, la lectura es
// contradictoria y la purga NO borra (falla cerrado). Antes, `titularDeItem` priorizaba
// `userID`, de modo que un ítem manipulado con `userID` del titular y `userId` de un tercero
// se borraba, eligiendo en silencio una de las dos formas.
//
// El puntero del titular sí debe retirarse: nunca queda apuntando a un ítem que no se borró.
func TestE014TitularContradictorioNoBorraItemGlobal(t *testing.T) {
	m := newPurgaDynamo()
	e014SembrarCuenta(m, e014Usuario)

	// (a) `userID` del titular con `userId` de otro titular.
	const skGlobalA = "IDEM#contradictorio-a"
	m.sembrar(domainDeletion.PartitionOutboxGlobal, skGlobalA, map[string]types.AttributeValue{
		"entityType": strAttr("PUSH_DELIVERY_IDEMPOTENCY"),
		"userID":     strAttr(e014Usuario),
		"userId":     strAttr(e014OtroUser),
		"eventId":    strAttr("evt-a"),
	})
	ptrAPK, ptrASK := domainDeletion.DeliveryPointerKeys(e014Usuario, "evt-a")
	m.sembrar(ptrAPK, ptrASK, map[string]types.AttributeValue{
		"entityType": strAttr(domainDeletion.EntityDeliveryPointer),
		"userID":     strAttr(e014Usuario),
		"eventId":    strAttr("evt-a"),
		"skGlobal":   strAttr(skGlobalA),
	})

	// (b) el orden inverso: `userID` de otro titular con `userId` del titular.
	const skGlobalB = "IDEM#contradictorio-b"
	m.sembrar(domainDeletion.PartitionOutboxGlobal, skGlobalB, map[string]types.AttributeValue{
		"entityType": strAttr("PUSH_DELIVERY_IDEMPOTENCY"),
		"userID":     strAttr(e014OtroUser),
		"userId":     strAttr(e014Usuario),
		"eventId":    strAttr("evt-b"),
	})
	ptrBPK, ptrBSK := domainDeletion.DeliveryPointerKeys(e014Usuario, "evt-b")
	m.sembrar(ptrBPK, ptrBSK, map[string]types.AttributeValue{
		"entityType": strAttr(domainDeletion.EntityDeliveryPointer),
		"userID":     strAttr(e014Usuario),
		"eventId":    strAttr("evt-b"),
		"skGlobal":   strAttr(skGlobalB),
	})

	store := dynamoAdapter.NewDeletionStore(m, e014Tabla)
	reloj := nuevoRelojPurga(e014T0)
	e014Programar(t, store, e014Usuario, e014RequestID, e014T0)
	worker := e014Worker(store, &e014Cache{}, reloj)
	e014CorrerHastaPurga(t, worker, reloj, m, 20)

	for _, clave := range []string{skGlobalA, skGlobalB} {
		if m.leer(domainDeletion.PartitionOutboxGlobal, clave) == nil {
			t.Fatalf("un titular contradictorio no puede borrar el item global %q", clave)
		}
	}
	for _, puntero := range []struct{ PK, SK, nombre string }{
		{ptrAPK, ptrASK, "DELIVERYPTR de (a)"},
		{ptrBPK, ptrBSK, "DELIVERYPTR de (b)"},
	} {
		if m.leer(puntero.PK, puntero.SK) != nil {
			t.Fatalf("%s debe retirarse: apunta a un item que no se borro", puntero.nombre)
		}
	}
	if estado := stringAttrDe(e014Control(m, e014Usuario, e014RequestID), "state"); estado != appAccount.PurgeStatePurged {
		t.Fatalf("estado esperado PURGED, obtenido %q", estado)
	}
}

// TestE014PunteroNoSeBorraBajoEnvioActivo fija la invariante de orden del catálogo: el ítem
// global referenciado se borra ANTES que su puntero y nunca bajo un envío en curso. Si el
// outbox tiene un lease vigente (`owner` + `leaseUntil` futuro), la página se reprograma sin
// tocar el puntero: así no queda un puntero colgando de un ítem todavía en uso.
func TestE014PunteroNoSeBorraBajoEnvioActivo(t *testing.T) {
	m := newPurgaDynamo()
	e014SembrarCuenta(m, e014Usuario)

	const skGlobal = "DUE#2026-11-18T10:00:00Z#nA"
	leaseVigente := e014T0.Add(30 * time.Minute).UTC().Format(time.RFC3339Nano)
	m.sembrar(domainDeletion.PartitionOutboxGlobal, skGlobal, map[string]types.AttributeValue{
		"entityType":     strAttr("PUSH_OUTBOX"),
		"userID":         strAttr(e014Usuario),
		"notificationID": strAttr("nA"),
		"owner":          strAttr("worker-outbox-x"),
		"leaseUntil":     strAttr(leaseVigente),
	})
	ptrPK, ptrSK := domainDeletion.OutboxPointerKeys(e014Usuario, "nA")
	m.sembrar(ptrPK, ptrSK, map[string]types.AttributeValue{
		"entityType":     strAttr(domainDeletion.EntityOutboxPointer),
		"userID":         strAttr(e014Usuario),
		"notificationID": strAttr("nA"),
		"skGlobal":       strAttr(skGlobal),
	})

	store := dynamoAdapter.NewDeletionStore(m, e014Tabla)
	reloj := nuevoRelojPurga(e014T0)
	e014Programar(t, store, e014Usuario, e014RequestID, e014T0)
	worker := e014Worker(store, &e014Cache{}, reloj)

	reloj.avanzar(time.Minute)
	worker.Ciclo(e014Contexto())

	control := e014Control(m, e014Usuario, e014RequestID)
	if estado := stringAttrDe(control, "state"); estado == appAccount.PurgeStatePurged {
		t.Fatalf("no se puede declarar PURGED con un envio en curso")
	}
	if m.leer(domainDeletion.PartitionOutboxGlobal, skGlobal) == nil {
		t.Fatalf("el item global con lease vigente no puede borrarse")
	}
	if m.leer(ptrPK, ptrSK) == nil {
		t.Fatalf("el puntero no puede borrarse mientras el item global siga existiendo")
	}

	// El envío termina: el lease vence y el outbox libera la propiedad. El reintento resuelve
	// la referencia y sólo entonces retira el puntero.
	m.mutar(mutacionPurga{pk: domainDeletion.PartitionOutboxGlobal, sk: skGlobal, item: map[string]types.AttributeValue{
		"entityType":     strAttr("PUSH_OUTBOX"),
		"userID":         strAttr(e014Usuario),
		"notificationID": strAttr("nA"),
		"owner":          strAttr(""),
		"leaseUntil":     strAttr(e014T0.Add(40 * time.Minute).UTC().Format(time.RFC3339Nano)),
	}})
	e014CorrerHastaPurga(t, worker, reloj, m, 20)

	if m.leer(domainDeletion.PartitionOutboxGlobal, skGlobal) != nil {
		t.Fatalf("el item global debe borrarse cuando el lease deja de estar vigente")
	}
	if m.leer(ptrPK, ptrSK) != nil {
		t.Fatalf("el puntero debe retirarse junto al item global")
	}
}
