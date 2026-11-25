package dynamo

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/nexumpr/Nexum-Backend/internal/application/pushoutbox"
)

// Defecto corregido: la SK ordenaba por instante de creación y `Pendientes`
// aplicaba un FilterExpression sobre el lease dentro de la misma página. Un
// backlog de trabajos reprogramados al futuro ocupaba la página (DynamoDB aplica
// Limit antes del filtro) y, al ignorar `LastEvaluatedKey`, los pendientes
// nuevos no se entregaban nunca: inanición del outbox. Estas pruebas fijan el
// comportamiento correcto: orden por `nextAttemptAt`, rango por clave y
// paginación acotada.

func crearTrabajo(t *testing.T, store *PushOutboxStore, id string, vence time.Time, creado time.Time) {
	t.Helper()
	creadoOk, err := store.Crear(context.Background(), pushoutbox.Registro{
		UserID:         "u1",
		NotificationID: id,
		Title:          "Aviso",
		Message:        "Mensaje",
		Type:           "REMINDER",
		EventID:        "evt-" + id,
		NextAttemptAt:  vence,
		CreatedAt:      creado,
	})
	if err != nil || !creadoOk {
		t.Fatalf("crear %s: creado=%v err=%v", id, creadoOk, err)
	}
}

// TestOUTBOXBacklogDeReprogramadosNoProduceInanicion reproduce el escenario del
// defecto: 100 trabajos al futuro y 20 vencidos con Limit 25 y páginas cortas.
// Los 20 deben entregarse en <= 5 páginas, sin saltos ni pérdidas.
func TestOUTBOXBacklogDeReprogramadosNoProduceInanicion(t *testing.T) {
	fake := nuevoOutboxFake()
	store := NewPushOutboxStore(fake, "NEXUM_CORE_TEST")
	ahora := time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)

	// Creados antes, reprogramados al futuro: con la SK antigua (por creación)
	// ocupaban las primeras páginas para siempre.
	for i := 0; i < 100; i++ {
		crearTrabajo(t, store,
			fmt.Sprintf("futuro-%03d", i),
			ahora.Add(24*time.Hour+time.Duration(i)*time.Second),
			ahora.Add(-4*time.Hour+time.Duration(i)*time.Second))
	}
	vencidos := make(map[string]bool, 20)
	for i := 0; i < 20; i++ {
		id := fmt.Sprintf("vencido-%02d", i)
		vencidos[id] = true
		crearTrabajo(t, store, id, ahora.Add(-time.Duration(i)*time.Minute), ahora.Add(-time.Duration(i)*time.Minute))
	}

	// DynamoDB puede devolver menos de `Limit` por página (1 MB, cuotas
	// internas); forzamos 7 para ejercitar ExclusiveStartKey.
	fake.tamanoPagina = 7
	registros, err := store.Pendientes(context.Background(), ahora, 25)
	if err != nil {
		t.Fatalf("pendientes: %v", err)
	}
	if len(registros) != len(vencidos) {
		t.Fatalf("vencidos devueltos=%d esperados=%d", len(registros), len(vencidos))
	}
	for _, registro := range registros {
		if !vencidos[registro.NotificationID] {
			t.Fatalf("se coló un trabajo no vencido: %s", registro.NotificationID)
		}
		if registro.NextAttemptAt.After(ahora) {
			t.Fatalf("vencimiento posterior a ahora: %v", registro.NextAttemptAt)
		}
	}
	if len(fake.consultas) < 2 {
		t.Fatalf("la prueba debe ejercitar la paginación: %d páginas", len(fake.consultas))
	}
	if len(fake.consultas) > pushOutboxPaginasMax {
		t.Fatalf("páginas=%d supera el tope %d", len(fake.consultas), pushOutboxPaginasMax)
	}
	for i, consulta := range fake.consultas {
		if consulta.FilterExpression != nil {
			t.Fatalf("consulta %d con FilterExpression: %q", i, aws.ToString(consulta.FilterExpression))
		}
		if i > 0 && consulta.ExclusiveStartKey == nil {
			t.Fatalf("consulta %d sin ExclusiveStartKey", i)
		}
	}
}

// TestOUTBOXNewPendienteVisibleConCienFuturos cubre el caso puntual reportado:
// un trabajo nuevo con nextAttemptAt=ahora debe verse aunque existan 100 al
// futuro, sin depender de que se drenen los primeros.
func TestOUTBOXNewPendienteVisibleConCienFuturos(t *testing.T) {
	fake := nuevoOutboxFake()
	store := NewPushOutboxStore(fake, "NEXUM_CORE_TEST")
	ahora := time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)
	for i := 0; i < 100; i++ {
		crearTrabajo(t, store,
			fmt.Sprintf("futuro-%03d", i),
			ahora.Add(6*time.Hour+time.Duration(i)*time.Second),
			ahora.Add(-48*time.Hour))
	}
	crearTrabajo(t, store, "nuevo", ahora, ahora)

	registros, err := store.Pendientes(context.Background(), ahora, 25)
	if err != nil {
		t.Fatalf("pendientes: %v", err)
	}
	if len(registros) != 1 || registros[0].NotificationID != "nuevo" {
		t.Fatalf("el trabajo nuevo no se entregó: %+v", registros)
	}
	if len(fake.consultas) != 1 {
		t.Fatalf("debía resolverse en la primera página: %d", len(fake.consultas))
	}
}

// TestOUTBOXCompatibilidadConSkHistorica fija la estrategia para ítems escritos
// antes del cambio (SK `DUE#<RFC3339Nano sin relleno>#<id>`): se siguen leyendo
// y su primera reprogramación los migra al formato con ancho fijo.
func TestOUTBOXCompatibilidadConSkHistorica(t *testing.T) {
	fake := nuevoOutboxFake()
	store := NewPushOutboxStore(fake, "NEXUM_CORE_TEST").WithOwner("nodo-a")
	ahora := time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)
	legado := "DUE#2026-11-22T11:59:00Z#legado-1"
	fake.items["OUTBOX#GLOBAL|"+legado] = map[string]types.AttributeValue{
		"PK":             stringAttr(pushOutboxPartition),
		"SK":             stringAttr(legado),
		"entityType":     stringAttr(pushOutboxEntityType),
		"userID":         stringAttr("u1"),
		"notificationID": stringAttr("legado-1"),
		"status":         stringAttr(string(pushoutbox.EstadoPendiente)),
		"attempts":       numberAttr(0),
		"nextAttemptAt":  stringAttr("2026-11-22T11:59:00Z"),
		"createdAt":      stringAttr("2026-11-22T11:59:00Z"),
	}

	registros, err := store.Pendientes(context.Background(), ahora, 25)
	if err != nil {
		t.Fatalf("pendientes: %v", err)
	}
	if len(registros) != 1 || registros[0].ID != legado {
		t.Fatalf("la SK histórica no se leyó: %+v", registros)
	}

	if _, ok, errReclamo := store.Reclamar(context.Background(), legado, ahora, 30*time.Second); errReclamo != nil || !ok {
		t.Fatalf("reclamo del trabajo histórico: ok=%v err=%v", ok, errReclamo)
	}
	siguiente := ahora.Add(5 * time.Minute)
	if err := store.Reprogramar(context.Background(), legado, 1, "REINTENTABLE", siguiente, ahora); err != nil {
		t.Fatalf("reprogramar histórico: %v", err)
	}
	if _, existe := fake.items["OUTBOX#GLOBAL|"+legado]; existe {
		t.Fatal("la SK histórica debió migrar al formato de ancho fijo")
	}
	migrada := pushOutboxSK(siguiente, "legado-1")
	if _, existe := fake.items["OUTBOX#GLOBAL|"+migrada]; !existe {
		t.Fatalf("falta la clave migrada %q", migrada)
	}
	registros, err = store.Pendientes(context.Background(), ahora, 25)
	if err != nil || len(registros) != 0 {
		t.Fatalf("el trabajo reprogramado no debe seguir vencido: %+v err=%v", registros, err)
	}
}

// TestOUTBOXReprogramarMismoInstanteNoMueveLaClave evita una transacción
// inválida (dos acciones sobre el mismo ítem) cuando el destino coincide.
func TestOUTBOXReprogramarMismoInstanteNoMueveLaClave(t *testing.T) {
	fake := nuevoOutboxFake()
	store := NewPushOutboxStore(fake, "NEXUM_CORE_TEST").WithOwner("nodo-a")
	ahora := time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)
	crearTrabajo(t, store, "n1", ahora, ahora)
	id := pushOutboxSK(ahora, "n1")
	if _, ok, err := store.Reclamar(context.Background(), id, ahora, 30*time.Second); err != nil || !ok {
		t.Fatalf("reclamo: ok=%v err=%v", ok, err)
	}
	if err := store.Reprogramar(context.Background(), id, 2, "REINTENTABLE", ahora, ahora); err != nil {
		t.Fatalf("reprogramar al mismo instante: %v", err)
	}
	if len(fake.transacciones) != 0 {
		t.Fatalf("no debía mover la clave: %d transacciones", len(fake.transacciones))
	}
	if _, existe := fake.items["OUTBOX#GLOBAL|"+id]; !existe {
		t.Fatal("el trabajo debía seguir en su clave")
	}
}

// TestOUTBOXReprogramarIdempotenteYConflicto distingue el replay legítimo (el
// destino ya existe) del trabajo perdido o ajeno.
func TestOUTBOXReprogramarIdempotenteYConflicto(t *testing.T) {
	fake := nuevoOutboxFake()
	store := NewPushOutboxStore(fake, "NEXUM_CORE_TEST").WithOwner("nodo-a")
	ahora := time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)
	siguiente := ahora.Add(5 * time.Minute)

	if err := store.Reprogramar(context.Background(), pushOutboxSK(ahora, "fantasma"), 1, "REINTENTABLE", siguiente, ahora); !errors.Is(err, ErrConflictoLease) {
		t.Fatalf("un trabajo inexistente debe devolver conflicto: %v", err)
	}

	destino := pushOutboxSK(siguiente, "n-replay")
	fake.items["OUTBOX#GLOBAL|"+destino] = map[string]types.AttributeValue{
		"PK":             stringAttr(pushOutboxPartition),
		"SK":             stringAttr(destino),
		"entityType":     stringAttr(pushOutboxEntityType),
		"notificationID": stringAttr("n-replay"),
		"status":         stringAttr(string(pushoutbox.EstadoFallido)),
		"attempts":       numberAttr(1),
	}
	if err := store.Reprogramar(context.Background(), pushOutboxSK(ahora, "n-replay"), 1, "REINTENTABLE", siguiente, ahora); err != nil {
		t.Fatalf("el replay de una reprogramación aplicada debe ser inocuo: %v", err)
	}
	if len(fake.transacciones) != 0 {
		t.Fatalf("el replay no debe abrir transacción: %d", len(fake.transacciones))
	}
}

// TestOUTBOXLeaseYEstadosSeFiltranEnMemoria sustituye el FilterExpression
// eliminado: el lease vencido recupera el trabajo, el vigente y los estados
// terminales no se entregan.
func TestOUTBOXLeaseYEstadosSeFiltranEnMemoria(t *testing.T) {
	fake := nuevoOutboxFake()
	store := NewPushOutboxStore(fake, "NEXUM_CORE_TEST").WithOwner("nodo-a")
	ahora := time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)
	casos := []struct {
		id     string
		estado pushoutbox.Estado
		lease  string
		espera bool
	}{
		{"vencido", pushoutbox.EstadoEnviando, pushOutboxInstante(ahora.Add(-time.Minute)), true},
		{"vigente", pushoutbox.EstadoEnviando, pushOutboxInstante(ahora.Add(time.Minute)), false},
		{"sin-lease", pushoutbox.EstadoEnviando, "", true},
		{"pendiente", pushoutbox.EstadoPendiente, "", true},
		{"fallido", pushoutbox.EstadoFallido, "", true},
		{"enviado", pushoutbox.EstadoEnviado, "", false},
		{"revocado", pushoutbox.EstadoRevocado, "", false},
	}
	for i, caso := range casos {
		id := pushOutboxSK(ahora.Add(-time.Duration(i)*time.Second), caso.id)
		item := map[string]types.AttributeValue{
			"PK":             stringAttr(pushOutboxPartition),
			"SK":             stringAttr(id),
			"entityType":     stringAttr(pushOutboxEntityType),
			"notificationID": stringAttr(caso.id),
			"status":         stringAttr(string(caso.estado)),
			"attempts":       numberAttr(1),
			"nextAttemptAt":  stringAttr(pushOutboxInstante(ahora.Add(-time.Duration(i) * time.Second))),
		}
		if caso.lease != "" {
			item["leaseUntil"] = stringAttr(caso.lease)
		}
		fake.items["OUTBOX#GLOBAL|"+id] = item
	}
	registros, err := store.Pendientes(context.Background(), ahora, 25)
	if err != nil {
		t.Fatalf("pendientes: %v", err)
	}
	obtenidos := make(map[string]bool, len(registros))
	for _, registro := range registros {
		obtenidos[registro.NotificationID] = true
	}
	for _, caso := range casos {
		if obtenidos[caso.id] != caso.espera {
			t.Fatalf("%s: entregado=%v esperado=%v", caso.id, obtenidos[caso.id], caso.espera)
		}
	}
}
