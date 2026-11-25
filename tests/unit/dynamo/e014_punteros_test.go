package dynamo_test

import (
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	dynamoAdapter "github.com/nexumpr/Nexum-Backend/internal/adapters/dynamo"
	appAccount "github.com/nexumpr/Nexum-Backend/internal/application/account"
	domainDeletion "github.com/nexumpr/Nexum-Backend/internal/domain/deletion"
)

// Pruebas de la purga de E014 sobre los punteros a particiones globales del servicio
// (`OUTBOXPTR#` del outbox push y `SCHEDULEPTR#` del scheduler de recordatorios):
//   - se borra primero el ítem global y después el puntero, sin `Scan`;
//   - un trabajo con envío en curso no se borra hasta que vence el lease;
//   - un puntero corrupto o de otro titular no borra datos ajenos;
//   - tras PURGED no queda ningún puntero ni ítem global del titular, y el otro titular
//     conserva todo lo suyo.

func TestE014PurgaPunterosGlobalesSinResiduos(t *testing.T) {
	m := newPurgaDynamo()
	e014SembrarCuenta(m, e014Usuario)

	globalA := "DUE#2026-11-20T10:00:00Z#nA"
	m.sembrar(domainDeletion.PartitionOutboxGlobal, globalA, map[string]types.AttributeValue{
		"entityType":     strAttr("PUSH_OUTBOX"),
		"userID":         strAttr(e014Usuario),
		"notificationID": strAttr("nA"),
		"title":          strAttr("Contenido personal de A"),
		"message":        strAttr("Mensaje personal de A"),
		"status":         strAttr("sent"),
	})
	ptrPK, ptrSK := domainDeletion.OutboxPointerKeys(e014Usuario, "nA")
	m.sembrar(ptrPK, ptrSK, map[string]types.AttributeValue{
		"entityType": strAttr(domainDeletion.EntityOutboxPointer),
		"userID":     strAttr(e014Usuario),
		"skGlobal":   strAttr(globalA),
		"createdAt":  strAttr(e014T0.Format(time.RFC3339Nano)),
	})

	globalSA := "OCC#2026-11-25T09:00:00Z#rA"
	m.sembrar(domainDeletion.PartitionScheduleGlobal, globalSA, map[string]types.AttributeValue{
		"entityType": strAttr("REMINDER_OCCURRENCE"),
		"userID":     strAttr(e014Usuario),
		"reminderID": strAttr("rA"),
		"status":     strAttr("pending"),
	})
	ptrSPK, ptrSSK := domainDeletion.SchedulePointerKeys(e014Usuario, "rA")
	m.sembrar(ptrSPK, ptrSSK, map[string]types.AttributeValue{
		"entityType": strAttr(domainDeletion.EntitySchedulePointer),
		"userID":     strAttr(e014Usuario),
		"skGlobal":   strAttr(globalSA),
		"dueAt":      strAttr("2026-11-25T09:00:00Z"),
	})

	// Puntero corrupto: apunta a un trabajo de OTRO titular. No debe borrarlo.
	globalAjena := "DUE#2026-11-20T11:00:00Z#nB"
	m.sembrar(domainDeletion.PartitionOutboxGlobal, globalAjena, map[string]types.AttributeValue{
		"entityType":     strAttr("PUSH_OUTBOX"),
		"userID":         strAttr(e014OtroUser),
		"notificationID": strAttr("nB"),
		"title":          strAttr("Contenido de B"),
		"status":         strAttr("pending"),
	})
	ptrCruzadoPK, ptrCruzadoSK := domainDeletion.OutboxPointerKeys(e014Usuario, "nCruzado")
	m.sembrar(ptrCruzadoPK, ptrCruzadoSK, map[string]types.AttributeValue{
		"entityType": strAttr(domainDeletion.EntityOutboxPointer),
		"userID":     strAttr(e014Usuario),
		"skGlobal":   strAttr(globalAjena),
	})

	// Datos propios de B que la purga de A no puede tocar.
	globalB := "DUE#2026-11-20T12:00:00Z#nB2"
	m.sembrar(domainDeletion.PartitionOutboxGlobal, globalB, map[string]types.AttributeValue{
		"entityType":     strAttr("PUSH_OUTBOX"),
		"userID":         strAttr(e014OtroUser),
		"notificationID": strAttr("nB2"),
		"title":          strAttr("Contenido de B"),
		"status":         strAttr("pending"),
	})
	ptrBPK, ptrBSK := domainDeletion.OutboxPointerKeys(e014OtroUser, "nB2")
	m.sembrar(ptrBPK, ptrBSK, map[string]types.AttributeValue{
		"entityType": strAttr(domainDeletion.EntityOutboxPointer),
		"userID":     strAttr(e014OtroUser),
		"skGlobal":   strAttr(globalB),
	})

	store := dynamoAdapter.NewDeletionStore(m, e014Tabla)
	reloj := nuevoRelojPurga(e014T0)
	e014Programar(t, store, e014Usuario, e014RequestID, e014T0)
	worker := e014Worker(store, &e014Cache{}, reloj)
	e014CorrerHastaPurga(t, worker, reloj, m, 20)

	// Nada del titular queda: ni punteros ni los ítems globales referenciados.
	if m.leer(domainDeletion.UserPartition(e014Usuario), ptrSK) != nil {
		t.Fatalf("el puntero OUTBOXPTR del titular debe desaparecer")
	}
	if m.leer(domainDeletion.UserPartition(e014Usuario), ptrSSK) != nil {
		t.Fatalf("el puntero SCHEDULEPTR del titular debe desaparecer")
	}
	if m.leer(domainDeletion.PartitionOutboxGlobal, globalA) != nil {
		t.Fatalf("el ítem global del outbox del titular debe borrarse")
	}
	if m.leer(domainDeletion.PartitionScheduleGlobal, globalSA) != nil {
		t.Fatalf("el ítem global del scheduler del titular debe borrarse")
	}
	for _, item := range m.porParticion(domainDeletion.UserPartition(e014Usuario)) {
		sk := stringAttrDe(item, "SK")
		if sk != domainDeletion.PrefixDeletion+e014RequestID {
			t.Fatalf("residuo en la particion del titular: %q", sk)
		}
		if strings.HasPrefix(sk, domainDeletion.PrefixOutboxPointer) || strings.HasPrefix(sk, domainDeletion.PrefixSchedulePointer) {
			t.Fatalf("no puede quedar un puntero tras PURGED: %q", sk)
		}
	}

	// Lo ajeno sobrevive: el trabajo de B y su puntero, y el trabajo al que apuntaba el
	// puntero corrupto (aunque el puntero corrupto se retira del titular).
	if m.leer(domainDeletion.PartitionOutboxGlobal, globalB) == nil {
		t.Fatalf("el trabajo de B no puede borrarse")
	}
	if m.leer(ptrBPK, ptrBSK) == nil {
		t.Fatalf("el puntero de B no puede borrarse")
	}
	if m.leer(domainDeletion.PartitionOutboxGlobal, globalAjena) == nil {
		t.Fatalf("un puntero corrupto no puede borrar el trabajo de otro titular")
	}
	if m.leer(domainDeletion.UserPartition(e014Usuario), ptrCruzadoSK) != nil {
		t.Fatalf("el puntero corrupto del titular debe retirarse")
	}

	// El tombstone conserva el control y declara las claves desconocidas.
	control := e014Control(m, e014Usuario, e014RequestID)
	if estado := stringAttrDe(control, "state"); estado != appAccount.PurgeStatePurged {
		t.Fatalf("estado esperado PURGED, obtenido %q", estado)
	}
}

// TestE014PunteroConEnvioEnCursoEsperaAlLease comprueba que un trabajo con lease vigente no se
// borra: la purga se reprograma y sólo lo retira cuando el lease vence.
func TestE014PunteroConEnvioEnCursoEsperaAlLease(t *testing.T) {
	m := newPurgaDynamo()
	e014SembrarCuenta(m, e014Usuario)

	global := "DUE#2026-11-20T10:00:00Z#nA"
	lease := e014T0.Add(10 * time.Minute)
	m.sembrar(domainDeletion.PartitionOutboxGlobal, global, map[string]types.AttributeValue{
		"entityType":     strAttr("PUSH_OUTBOX"),
		"userID":         strAttr(e014Usuario),
		"notificationID": strAttr("nA"),
		"status":         strAttr("sending"),
		"owner":          strAttr("otro-nodo"),
		"leaseUntil":     strAttr(lease.Format(time.RFC3339Nano)),
	})
	ptrPK, ptrSK := domainDeletion.OutboxPointerKeys(e014Usuario, "nA")
	m.sembrar(ptrPK, ptrSK, map[string]types.AttributeValue{
		"entityType": strAttr(domainDeletion.EntityOutboxPointer),
		"userID":     strAttr(e014Usuario),
		"skGlobal":   strAttr(global),
	})

	store := dynamoAdapter.NewDeletionStore(m, e014Tabla)
	reloj := nuevoRelojPurga(e014T0)
	e014Programar(t, store, e014Usuario, e014RequestID, e014T0)
	worker := e014Worker(store, &e014Cache{}, reloj)

	// Primer ciclo, dentro del lease: no se borra nada del trabajo en curso.
	reloj.avanzar(time.Minute)
	worker.Ciclo(e014Contexto())
	if m.leer(domainDeletion.PartitionOutboxGlobal, global) == nil {
		t.Fatalf("no se puede borrar un trabajo con lease vigente")
	}
	if m.leer(ptrPK, ptrSK) == nil {
		t.Fatalf("el puntero debe conservarse mientras el trabajo siga vivo")
	}
	if estado := stringAttrDe(e014Control(m, e014Usuario, e014RequestID), "state"); estado == appAccount.PurgeStatePurged {
		t.Fatalf("no se puede declarar PURGED con un trabajo en curso")
	}

	// Al vencer el lease, la purga lo retira y cierra el trabajo.
	e014CorrerHastaPurga(t, worker, reloj, m, 20)
	if m.leer(domainDeletion.PartitionOutboxGlobal, global) != nil {
		t.Fatalf("vencido el lease, el trabajo debe retirarse")
	}
	if m.leer(ptrPK, ptrSK) != nil {
		t.Fatalf("vencido el lease, el puntero debe retirarse")
	}
}

// TestE014PunteroSinReferenciaNoBloquea comprueba que un puntero sin `skGlobal` no impide la
// purga: se elimina y se contabiliza como clave desconocida.
func TestE014PunteroSinReferenciaNoBloquea(t *testing.T) {
	m := newPurgaDynamo()
	e014SembrarCuenta(m, e014Usuario)
	ptrPK, ptrSK := domainDeletion.OutboxPointerKeys(e014Usuario, "sin-referencia")
	m.sembrar(ptrPK, ptrSK, map[string]types.AttributeValue{
		"entityType": strAttr(domainDeletion.EntityOutboxPointer),
		"userID":     strAttr(e014Usuario),
	})

	store := dynamoAdapter.NewDeletionStore(m, e014Tabla)
	reloj := nuevoRelojPurga(e014T0)
	e014Programar(t, store, e014Usuario, e014RequestID, e014T0)
	worker := e014Worker(store, &e014Cache{}, reloj)
	e014CorrerHastaPurga(t, worker, reloj, m, 20)

	if m.leer(ptrPK, ptrSK) != nil {
		t.Fatalf("un puntero sin referencia debe retirarse")
	}
	control := e014Control(m, e014Usuario, e014RequestID)
	if estado := stringAttrDe(control, "state"); estado != appAccount.PurgeStatePurged {
		t.Fatalf("estado esperado PURGED, obtenido %q", estado)
	}
	if desconocidos, ok := attributeIntDe(control, "unknownKeys"); !ok || desconocidos < 1 {
		// `unknownKeys` se elimina del tombstone final (minimizacion); se acepta su ausencia.
		if ok {
			t.Fatalf("el puntero sin referencia debe contarse como clave desconocida: %d", desconocidos)
		}
	}
}

// TestE014PurgaPunteroReadAll elimina el trabajo global de E063 (QUEUE#READALL)
// mediante su puntero READALLPTR# del titular, sin Scan y sin tocar la cola de B.
func TestE014PurgaPunteroReadAll(t *testing.T) {
	m := newPurgaDynamo()
	e014SembrarCuenta(m, e014Usuario)

	global := "REQ#2026-11-20T10:00:00.000000000Z#req-a"
	m.sembrar(domainDeletion.PartitionQueueReadAll, global, map[string]types.AttributeValue{
		"entityType": strAttr("READALL"),
		"userID":     strAttr(e014Usuario),
		"requestId":  strAttr("req-a"),
		"leaseOwner": strAttr(""),
		"leaseUntil": &types.AttributeValueMemberN{Value: "0"},
		"createdAt":  &types.AttributeValueMemberN{Value: "1763632800"},
	})
	ptrPK, ptrSK := domainDeletion.ReadAllPointerKeys(e014Usuario, "req-a")
	m.sembrar(ptrPK, ptrSK, map[string]types.AttributeValue{
		"entityType": strAttr("READALL_POINTER"),
		"userID":     strAttr(e014Usuario),
		"requestId":  strAttr("req-a"),
		"skGlobal":   strAttr(global),
	})

	// Cola de otro titular: no debe tocarse.
	globalB := "REQ#2026-11-20T11:00:00.000000000Z#req-b"
	m.sembrar(domainDeletion.PartitionQueueReadAll, globalB, map[string]types.AttributeValue{
		"entityType": strAttr("READALL"),
		"userID":     strAttr(e014OtroUser),
		"requestId":  strAttr("req-b"),
	})
	ptrBPK, ptrBSK := domainDeletion.ReadAllPointerKeys(e014OtroUser, "req-b")
	m.sembrar(ptrBPK, ptrBSK, map[string]types.AttributeValue{
		"entityType": strAttr("READALL_POINTER"),
		"userID":     strAttr(e014OtroUser),
		"skGlobal":   strAttr(globalB),
	})

	store := dynamoAdapter.NewDeletionStore(m, e014Tabla)
	reloj := nuevoRelojPurga(e014T0)
	e014Programar(t, store, e014Usuario, e014RequestID, e014T0)
	worker := e014Worker(store, &e014Cache{}, reloj)
	e014CorrerHastaPurga(t, worker, reloj, m, 20)

	if _, existe := m.items[domainDeletion.PartitionQueueReadAll+"\x00"+global]; existe {
		t.Fatal("el trabajo READALL del titular no se eliminó")
	}
	if _, existe := m.items[ptrPK+"\x00"+ptrSK]; existe {
		t.Fatal("el puntero READALLPTR# del titular no se eliminó")
	}
	if _, existe := m.items[domainDeletion.PartitionQueueReadAll+"\x00"+globalB]; !existe {
		t.Fatal("la cola READALL de B se eliminó")
	}
	if _, existe := m.items[ptrBPK+"\x00"+ptrBSK]; !existe {
		t.Fatal("el puntero READALLPTR# de B se eliminó")
	}
}
