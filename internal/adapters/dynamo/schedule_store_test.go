package dynamo

// Pruebas del adaptador del scheduler durable (Oleada 5).
//
// Usan el mismo doble estricto de DynamoDB que E064–E067: exige claves exactas,
// prohíbe dos acciones sobre el mismo ítem en una transacción, aplica todo o nada
// y no expone `Scan`. La guarda `prohibirUpdateItem` se desactiva aquí porque el
// scheduler sí usa `UpdateItem` condicional (Claim/Release), a diferencia del CRUD
// de recordatorios.

import (
	"context"
	"errors"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	appReminders "github.com/nexumpr/Nexum-Backend/internal/application/reminders"
	domainDeletion "github.com/nexumpr/Nexum-Backend/internal/domain/deletion"
)

const (
	schedUsuarioA = "usuario-agenda-a"
	schedUsuarioB = "usuario-agenda-b"
	schedTabla    = "NEXUM_CORE_TEST"
)

var schedInstante = time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)

func schedEntorno(t *testing.T, owner string) (*ScheduleStore, *RemindersStore, *remFakeDynamo) {
	t.Helper()
	doble := nuevoRemFakeDynamo()
	doble.prohibirUpdateItem = false
	agenda := NewScheduleStore(doble, schedTabla).WithOwner(owner)
	recordatorios := NewRemindersStore(doble, schedTabla)
	return agenda, recordatorios, doble
}

func schedAlta(t *testing.T, recordatorios *RemindersStore, usuario, id, dueAt string, frecuencia *appReminders.Frequency) {
	t.Helper()
	err := recordatorios.Create(context.Background(), appReminders.CreateInput{
		UserID: usuario, ID: id, KeyHash: "clave-" + id, PayloadHash: "carga-" + id,
		Title: "Recordatorio " + id, DueAt: dueAt, Timezone: "America/Mexico_City",
		Frequency: frecuencia, Now: schedInstante,
	})
	if err != nil {
		t.Fatalf("alta de recordatorio %s: %v", id, err)
	}
}

func schedDue(t *testing.T, agenda *ScheduleStore, ahora time.Time, limite int) []appReminders.Ocurrencia {
	t.Helper()
	ocurrencias, err := agenda.Due(context.Background(), ahora, limite)
	if err != nil {
		t.Fatalf("Due: %v", err)
	}
	return ocurrencias
}

func schedReclamar(t *testing.T, agenda *ScheduleStore, oc appReminders.Ocurrencia, lease time.Duration) appReminders.Lease {
	t.Helper()
	token, err := agenda.Claim(context.Background(), oc, schedInstante, lease)
	if err != nil || token == nil {
		t.Fatalf("Claim: %v", err)
	}
	return *token
}

// schedClavesTransaccion devuelve las claves (PK|SK) de cada acción y falla si se
// repiten: DynamoDB prohíbe dos acciones sobre el mismo ítem.
func schedClavesTransaccion(t *testing.T, acciones []types.TransactWriteItem) []string {
	t.Helper()
	vistas := make(map[string]bool, len(acciones))
	claves := make([]string, 0, len(acciones))
	for i, accion := range acciones {
		var llave map[string]types.AttributeValue
		switch {
		case accion.Put != nil:
			llave = accion.Put.Item
		case accion.Delete != nil:
			llave = accion.Delete.Key
		case accion.Update != nil:
			llave = accion.Update.Key
		default:
			t.Fatalf("acción %d no soportada", i)
		}
		clave := stringAttribute(llave["PK"]) + "|" + stringAttribute(llave["SK"])
		if vistas[clave] {
			t.Fatalf("dos acciones sobre el mismo ítem: %s", clave)
		}
		vistas[clave] = true
		claves = append(claves, clave)
	}
	return claves
}

// ---------------------------------------------------------------------------
// Due
// ---------------------------------------------------------------------------

func TestScheduleDueOrdenLimiteYCondicionDeRango(t *testing.T) {
	agenda, recordatorios, doble := schedEntorno(t, "nodo-a")
	for i, id := range []string{"r-temprano", "r-medio", "r-tarde"} {
		due := schedInstante.Add(time.Duration(i-3) * time.Hour).Format(time.RFC3339)
		schedAlta(t, recordatorios, schedUsuarioA, id, due, nil)
	}
	// Futuro: no debe aparecer.
	schedAlta(t, recordatorios, schedUsuarioA, "r-futuro", schedInstante.Add(2*time.Hour).Format(time.RFC3339), nil)

	ocurrencias := schedDue(t, agenda, schedInstante, 2)
	if len(ocurrencias) != 2 {
		t.Fatalf("se esperaban 2 ocurrencias vencidas, hubo %d", len(ocurrencias))
	}
	if ocurrencias[0].ReminderID != "r-temprano" || ocurrencias[1].ReminderID != "r-medio" {
		t.Fatalf("orden inesperado: %s, %s", ocurrencias[0].ReminderID, ocurrencias[1].ReminderID)
	}
	if doble.queries != 1 {
		t.Fatalf("una sola Query por Due con resultados; hubo %d", doble.queries)
	}
	entrada := doble.consultas[0]
	if condicion := aws.ToString(entrada.KeyConditionExpression); condicion != "#pk = :pk AND #sk <= :limite" {
		t.Fatalf("condición de clave inesperada: %q", condicion)
	}
	if !aws.ToBool(entrada.ConsistentRead) {
		t.Fatal("Due debe leer con consistencia fuerte")
	}
	if limite := int(aws.ToInt32(entrada.Limit)); limite != 2 {
		t.Fatalf("Limit=%d, esperado 2", limite)
	}
	if filtro := aws.ToString(entrada.FilterExpression); filtro != filtroLease {
		t.Fatalf("filtro inesperado: %q", filtro)
	}
	if pk := stringAttribute(entrada.ExpressionAttributeValues[":pk"]); pk != domainDeletion.PartitionScheduleGlobal {
		t.Fatalf("partición global inesperada: %q", pk)
	}
	if !strings.HasPrefix(stringAttribute(entrada.ExpressionAttributeValues[":limite"]), "DUE#"+schedInstante.Format("2006-01-02T15:04:05")) {
		t.Fatalf("límite inesperado: %q", stringAttribute(entrada.ExpressionAttributeValues[":limite"]))
	}
}

func TestScheduleDueIncluyeVencimientoExactoYExcluyeLeaseVigente(t *testing.T) {
	agenda, recordatorios, doble := schedEntorno(t, "nodo-a")
	exacto := schedInstante.Format(time.RFC3339)
	schedAlta(t, recordatorios, schedUsuarioA, "r-exacto", exacto, nil)
	schedAlta(t, recordatorios, schedUsuarioA, "r-reclamado", exacto, nil)
	schedAlta(t, recordatorios, schedUsuarioA, "r-libre-tras-backoff", exacto, nil)

	// Un nodo mantiene el lease vigente; otro liberó con backoff futuro.
	reclamada := schedOcurrenciaPorID(t, agenda, "r-reclamado")
	if _, err := agenda.Claim(context.Background(), reclamada, schedInstante, time.Minute); err != nil {
		t.Fatalf("Claim: %v", err)
	}
	liberada := schedOcurrenciaPorID(t, agenda, "r-libre-tras-backoff")
	token := schedReclamar(t, agenda, liberada, time.Minute)
	if err := agenda.Release(context.Background(), token, "PRODUCTOR_ERROR", schedInstante.Add(30*time.Second), schedInstante); err != nil {
		t.Fatalf("Release: %v", err)
	}

	ocurrencias := schedDue(t, agenda, schedInstante, 10)
	ids := make([]string, 0, len(ocurrencias))
	for _, oc := range ocurrencias {
		ids = append(ids, oc.ReminderID)
	}
	sort.Strings(ids)
	if len(ids) != 1 || ids[0] != "r-exacto" {
		t.Fatalf("sólo el vencimiento exacto y libre debe aparecer: %v", ids)
	}
	if doble.queries == 0 {
		t.Fatal("Due debe consultar")
	}
}

func TestScheduleDuePaginaSiLaPrimeraEstaBloqueada(t *testing.T) {
	agenda, recordatorios, doble := schedEntorno(t, "nodo-a")
	// El doble entrega páginas de 1: la primera estará arrendada y el adaptador
	// debe continuar con ExclusiveStartKey en lugar de rendirse.
	doble.tamanoPagina = 1
	schedAlta(t, recordatorios, schedUsuarioA, "r-1", schedInstante.Add(-2*time.Hour).Format(time.RFC3339), nil)
	schedAlta(t, recordatorios, schedUsuarioA, "r-2", schedInstante.Add(-time.Hour).Format(time.RFC3339), nil)
	if _, err := agenda.Claim(context.Background(), schedOcurrenciaPorID(t, agenda, "r-1"), schedInstante, time.Minute); err != nil {
		t.Fatalf("Claim del primero: %v", err)
	}
	ocurrencias := schedDue(t, agenda, schedInstante, 5)
	if len(ocurrencias) != 1 || ocurrencias[0].ReminderID != "r-2" {
		t.Fatalf("debe devolver el segundo: %v", ocurrencias)
	}
	if doble.queries < 2 {
		t.Fatalf("debe paginar con ExclusiveStartKey: %d consultas", doble.queries)
	}
	if len(doble.consultas[len(doble.consultas)-1].ExclusiveStartKey) == 0 {
		t.Fatal("la segunda consulta debe llevar ExclusiveStartKey")
	}
}

func schedOcurrenciaPorID(t *testing.T, agenda *ScheduleStore, id string) appReminders.Ocurrencia {
	t.Helper()
	for _, oc := range schedDue(t, agenda, schedInstante, 50) {
		if oc.ReminderID == id {
			return oc
		}
	}
	t.Fatalf("no se encontró la ocurrencia %s", id)
	return appReminders.Ocurrencia{}
}

// ---------------------------------------------------------------------------
// Claim / Release
// ---------------------------------------------------------------------------

func TestScheduleClaimDosNodosSoloUno(t *testing.T) {
	agendaA, recordatorios, doble := schedEntorno(t, "nodo-a")
	agendaB := NewScheduleStore(doble, schedTabla).WithOwner("nodo-b")
	schedAlta(t, recordatorios, schedUsuarioA, "r-carrera", schedInstante.Add(-time.Hour).Format(time.RFC3339), nil)

	// Ambos leen antes de reclamar: es la carrera real entre dos EC2.
	leidaA := schedDue(t, agendaA, schedInstante, 10)
	leidaB := schedDue(t, agendaB, schedInstante, 10)
	if len(leidaA) != 1 || len(leidaB) != 1 {
		t.Fatalf("lectura inicial: %d %d", len(leidaA), len(leidaB))
	}
	if _, err := agendaA.Claim(context.Background(), leidaA[0], schedInstante, 30*time.Second); err != nil {
		t.Fatalf("el nodo A debía reclamar: %v", err)
	}
	if _, err := agendaB.Claim(context.Background(), leidaB[0], schedInstante, 30*time.Second); !errors.Is(err, appReminders.ErrLeasePerdido) {
		t.Fatalf("el nodo B no puede reclamar: %v", err)
	}
	if len(schedDue(t, agendaB, schedInstante, 10)) != 0 {
		t.Fatal("la ocurrencia reclamada no vuelve a estar disponible")
	}
}

func TestScheduleClaimIncrementaContadoresYDevuelveElToken(t *testing.T) {
	agenda, recordatorios, doble := schedEntorno(t, "nodo-a")
	schedAlta(t, recordatorios, schedUsuarioA, "r-token", schedInstante.Add(-time.Hour).Format(time.RFC3339), nil)
	oc := schedOcurrenciaPorID(t, agenda, "r-token")

	token, err := agenda.Claim(context.Background(), oc, schedInstante, 45*time.Second)
	if err != nil || token == nil {
		t.Fatalf("Claim: %v", err)
	}
	if token.Owner != "nodo-a" || token.Fence != 1 || token.Version != 2 || token.Attempts != 1 {
		t.Fatalf("token inesperado: %+v", token)
	}
	if !token.LeaseUntil.Equal(schedInstante.Add(45 * time.Second)) {
		t.Fatalf("leaseUntil=%v", token.LeaseUntil)
	}
	item := doble.item(domainDeletion.PartitionScheduleGlobal, oc.SK)
	if stringAttribute(item["owner"]) != "nodo-a" {
		t.Fatalf("owner persistido: %q", stringAttribute(item["owner"]))
	}
	if remAtributoNumber(t, item, "version") != 2 || remAtributoNumber(t, item, "fence") != 1 || remAtributoNumber(t, item, "attempts") != 1 {
		t.Fatalf("contadores persistidos: %v", item)
	}
	if len(doble.actualizaciones) != 1 {
		t.Fatalf("Claim debe ser un único UpdateItem: %d", len(doble.actualizaciones))
	}
	if retorno := doble.actualizaciones[0].ReturnValues; retorno != types.ReturnValueAllNew {
		t.Fatalf("ReturnValues=%q", retorno)
	}

	// Un Claim con una versión obsoleta no puede prosperar.
	if _, err := agenda.Claim(context.Background(), oc, schedInstante, time.Minute); !errors.Is(err, appReminders.ErrLeasePerdido) {
		t.Fatalf("claim con versión obsoleta: %v", err)
	}
}

func TestScheduleReleasePermiteReintentoTrasElLease(t *testing.T) {
	agenda, recordatorios, doble := schedEntorno(t, "nodo-a")
	schedAlta(t, recordatorios, schedUsuarioA, "r-release", schedInstante.Add(-time.Hour).Format(time.RFC3339), nil)
	token := schedReclamar(t, agenda, schedOcurrenciaPorID(t, agenda, "r-release"), 30*time.Second)

	if err := agenda.Release(context.Background(), token, "PRODUCTOR_ERROR", schedInstante.Add(20*time.Second), schedInstante); err != nil {
		t.Fatalf("Release: %v", err)
	}
	item := doble.item(domainDeletion.PartitionScheduleGlobal, token.Ocurrencia.SK)
	if stringAttribute(item["owner"]) != "" {
		t.Fatalf("el owner debe retirarse para no bloquear la purga: %q", stringAttribute(item["owner"]))
	}
	if stringAttribute(item["lastErrorClass"]) != "PRODUCTOR_ERROR" {
		t.Fatalf("clase persistida: %q", stringAttribute(item["lastErrorClass"]))
	}
	if remAtributoNumber(t, item, "version") != 3 {
		t.Fatalf("la versión debe avanzar: %v", item["version"])
	}
	if len(schedDue(t, agenda, schedInstante, 10)) != 0 {
		t.Fatal("el backoff debe impedir el reintento inmediato")
	}
	if len(schedDue(t, agenda, schedInstante.Add(21*time.Second), 10)) != 1 {
		t.Fatal("tras el backoff la ocurrencia debe volver a estar disponible")
	}
}

func TestScheduleReleaseConLeaseReclamadoPorOtroNodoEsLeasePerdido(t *testing.T) {
	agendaA, recordatorios, doble := schedEntorno(t, "nodo-a")
	schedAlta(t, recordatorios, schedUsuarioA, "r-ajeno", schedInstante.Add(-time.Hour).Format(time.RFC3339), nil)
	token := schedReclamar(t, agendaA, schedOcurrenciaPorID(t, agendaA, "r-ajeno"), time.Minute)

	// Otro nodo reclamó tras vencer el lease: el token viejo ya no vale.
	item := doble.item(domainDeletion.PartitionScheduleGlobal, token.Ocurrencia.SK)
	item["owner"] = stringAttr("nodo-b")
	item["fence"] = numberAttr(token.Fence + 1)
	item["version"] = numberAttr(token.Version + 1)
	doble.poner(item)

	if err := agendaA.Release(context.Background(), token, "PRODUCTOR_ERROR", schedInstante.Add(time.Minute), schedInstante); !errors.Is(err, appReminders.ErrLeasePerdido) {
		t.Fatalf("un token superado no puede liberar: %v", err)
	}
}

func TestScheduleClaimTrasReinicioRecuperaLeaseVencido(t *testing.T) {
	agenda, recordatorios, doble := schedEntorno(t, "nodo-nuevo")
	schedAlta(t, recordatorios, schedUsuarioA, "r-huerfano", schedInstante.Add(-time.Hour).Format(time.RFC3339), nil)
	// Estado de un nodo que murió antes de cerrar: lease vencido.
	sk := scheduleSortKey(schedInstante.Add(-time.Hour), "r-huerfano")
	item := doble.item(domainDeletion.PartitionScheduleGlobal, sk)
	item["owner"] = stringAttr("nodo-muerto")
	item["leaseUntil"] = stringAttr(schedInstante.Add(-time.Minute).Format(time.RFC3339Nano))
	item["version"] = numberAttr(7)
	item["fence"] = numberAttr(3)
	doble.poner(item)

	ocurrencias := schedDue(t, agenda, schedInstante, 10)
	if len(ocurrencias) != 1 {
		t.Fatalf("el lease vencido debe ser reclamable: %d", len(ocurrencias))
	}
	token, err := agenda.Claim(context.Background(), ocurrencias[0], schedInstante, time.Minute)
	if err != nil || token == nil {
		t.Fatalf("Claim tras reinicio: %v", err)
	}
	if token.Fence != 4 || token.Version != 8 || token.Owner != "nodo-nuevo" {
		t.Fatalf("el fence debe ser monotónico: %+v", token)
	}
}

// ---------------------------------------------------------------------------
// Advance
// ---------------------------------------------------------------------------

func TestScheduleAdvanceMueveRecordatorioAgendaYPuntero(t *testing.T) {
	agenda, recordatorios, doble := schedEntorno(t, "nodo-a")
	frecuencia := appReminders.FrequencyDaily
	dueAt := time.Date(2026, 11, 24, 9, 0, 0, 0, time.UTC)
	schedAlta(t, recordatorios, schedUsuarioA, "r-diario", dueAt.Format(time.RFC3339), &frecuencia)
	oc := schedOcurrenciaPorID(t, agenda, "r-diario")
	token := schedReclamar(t, agenda, oc, 30*time.Second)

	siguiente, err := appReminders.NextOccurrence(dueAt, "America/Mexico_City", frecuencia)
	if err != nil {
		t.Fatalf("NextOccurrence: %v", err)
	}
	transaccionesPrevias := doble.transacciones
	if err := agenda.Advance(context.Background(), token, siguiente, schedInstante); err != nil {
		t.Fatalf("Advance: %v", err)
	}
	if doble.transacciones != transaccionesPrevias+1 {
		t.Fatalf("Advance debe ser una única transacción")
	}
	acciones := doble.transaccionesVistas[len(doble.transaccionesVistas)-1].TransactItems
	if len(acciones) != 6 {
		t.Fatalf("Advance debe tener 6 acciones, hubo %d", len(acciones))
	}
	schedClavesTransaccion(t, acciones)

	// Agenda: la ocurrencia antigua desaparece y la nueva nace libre.
	if doble.item(domainDeletion.PartitionScheduleGlobal, oc.SK) != nil {
		t.Fatal("la ocurrencia anterior debe eliminarse")
	}
	skNueva := scheduleSortKey(siguiente, "r-diario")
	nuevaAgenda := doble.item(domainDeletion.PartitionScheduleGlobal, skNueva)
	if nuevaAgenda == nil {
		t.Fatal("falta la ocurrencia nueva")
	}
	if stringAttribute(nuevaAgenda["owner"]) != "" || remAtributoNumber(t, nuevaAgenda, "attempts") != 0 {
		t.Fatalf("la ocurrencia nueva debe nacer libre: %v", nuevaAgenda)
	}
	if remAtributoNumber(t, nuevaAgenda, "reminderVersion") != 2 {
		t.Fatalf("reminderVersion debe avanzar: %v", nuevaAgenda)
	}

	// Recordatorio: la SK se mueve y la versión avanza.
	skVieja := reminderSortKey(dueAt, "r-diario")
	if doble.item(reminderPartition(schedUsuarioA), skVieja) != nil {
		t.Fatal("el recordatorio con la fecha anterior debe eliminarse")
	}
	nuevoRecordatorio := doble.item(reminderPartition(schedUsuarioA), reminderSortKey(siguiente, "r-diario"))
	if nuevoRecordatorio == nil {
		t.Fatal("falta el recordatorio con la fecha nueva")
	}
	if remAtributoNumber(t, nuevoRecordatorio, "version") != 2 || !boolAttribute(nuevoRecordatorio["active"]) {
		t.Fatalf("el recordatorio debe seguir activo y con versión 2: %v", nuevoRecordatorio)
	}
	// Puntero: apunta a la SK nueva.
	puntero := doble.item(reminderPartition(schedUsuarioA), reminderPointerSortKey("r-diario"))
	if stringAttribute(puntero["reminderSK"]) != reminderSortKey(siguiente, "r-diario") {
		t.Fatalf("puntero obsoleto: %q", stringAttribute(puntero["reminderSK"]))
	}
	if remAtributoNumber(t, puntero, "version") != 2 {
		t.Fatalf("versión del puntero: %v", puntero["version"])
	}
	// Puntero de agenda: apunta a la ocurrencia nueva.
	_, skPuntero := domainDeletion.SchedulePointerKeys(schedUsuarioA, "r-diario")
	punteroAgenda := doble.item(reminderPartition(schedUsuarioA), skPuntero)
	if stringAttribute(punteroAgenda["skGlobal"]) != skNueva {
		t.Fatalf("puntero de agenda obsoleto: %v", punteroAgenda)
	}
	// El service HTTP sigue leyendo el recordatorio tras mover la SK.
	leido, err := recordatorios.Get(context.Background(), schedUsuarioA, "r-diario")
	if err != nil || leido == nil || !leido.DueAt.Equal(siguiente) {
		t.Fatalf("lectura tras Advance: %+v err=%v", leido, err)
	}
}

func TestScheduleAdvanceUnMesFinDeMes(t *testing.T) {
	agenda, recordatorios, doble := schedEntorno(t, "nodo-a")
	frecuencia := appReminders.FrequencyMonthly
	dueAt := time.Date(2026, 1, 31, 9, 0, 0, 0, time.UTC)
	schedAlta(t, recordatorios, schedUsuarioA, "r-mensual", dueAt.Format(time.RFC3339), &frecuencia)
	token := schedReclamar(t, agenda, schedOcurrenciaPorID(t, agenda, "r-mensual"), 30*time.Second)

	siguiente, err := appReminders.NextOccurrence(dueAt, "America/Mexico_City", frecuencia)
	if err != nil {
		t.Fatalf("NextOccurrence: %v", err)
	}
	if !siguiente.Equal(time.Date(2026, 2, 28, 9, 0, 0, 0, time.UTC)) {
		t.Fatalf("31 ene debe caer al 28 feb: %v", siguiente)
	}
	if err := agenda.Advance(context.Background(), token, siguiente, schedInstante); err != nil {
		t.Fatalf("Advance: %v", err)
	}
	if doble.item(domainDeletion.PartitionScheduleGlobal, scheduleSortKey(siguiente, "r-mensual")) == nil {
		t.Fatal("falta la ocurrencia de febrero")
	}
}

func TestScheduleAdvanceRechazaSiElRecordatorioCambio(t *testing.T) {
	agenda, recordatorios, doble := schedEntorno(t, "nodo-a")
	frecuencia := appReminders.FrequencyWeekly
	dueAt := schedInstante.Add(-time.Hour)
	schedAlta(t, recordatorios, schedUsuarioA, "r-editado", dueAt.Format(time.RFC3339), &frecuencia)
	oc := schedOcurrenciaPorID(t, agenda, "r-editado")
	token := schedReclamar(t, agenda, oc, 30*time.Second)

	// El titular edita el recordatorio (E066) en paralelo: versión distinta.
	item := doble.item(reminderPartition(schedUsuarioA), reminderSortKey(dueAt, "r-editado"))
	item["version"] = numberAttr(9)
	doble.poner(item)

	if err := agenda.Advance(context.Background(), token, dueAt.AddDate(0, 0, 7), schedInstante); !errors.Is(err, appReminders.ErrLeasePerdido) {
		t.Fatalf("un cambio de versión debe invalidar el lease: %v", err)
	}
	if doble.item(domainDeletion.PartitionScheduleGlobal, oc.SK) == nil {
		t.Fatal("la transacción cancelada no debe borrar la ocurrencia")
	}
}

// ---------------------------------------------------------------------------
// Complete
// ---------------------------------------------------------------------------

func TestScheduleCompleteUnicoDesactivaYRetiraAgenda(t *testing.T) {
	agenda, recordatorios, doble := schedEntorno(t, "nodo-a")
	dueAt := schedInstante.Add(-time.Hour)
	schedAlta(t, recordatorios, schedUsuarioA, "r-unico", dueAt.Format(time.RFC3339), nil)
	oc := schedOcurrenciaPorID(t, agenda, "r-unico")
	token := schedReclamar(t, agenda, oc, 30*time.Second)

	if err := agenda.Complete(context.Background(), token, schedInstante); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	acciones := doble.transaccionesVistas[len(doble.transaccionesVistas)-1].TransactItems
	if len(acciones) != 4 {
		t.Fatalf("Complete debe tener 4 acciones, hubo %d", len(acciones))
	}
	schedClavesTransaccion(t, acciones)

	recordatorio := doble.item(reminderPartition(schedUsuarioA), reminderSortKey(dueAt, "r-unico"))
	if recordatorio == nil || boolAttribute(recordatorio["active"]) {
		t.Fatalf("el recordatorio debe quedar inactivo: %v", recordatorio)
	}
	if remAtributoNumber(t, recordatorio, "version") != 2 {
		t.Fatalf("versión del recordatorio: %v", recordatorio["version"])
	}
	if doble.item(domainDeletion.PartitionScheduleGlobal, oc.SK) != nil {
		t.Fatal("la ocurrencia debe retirarse")
	}
	_, skPuntero := domainDeletion.SchedulePointerKeys(schedUsuarioA, "r-unico")
	if doble.item(reminderPartition(schedUsuarioA), skPuntero) != nil {
		t.Fatal("el puntero de agenda debe retirarse")
	}
	// El puntero del recordatorio se conserva: E064/E066 siguen resolviéndolo.
	puntero := doble.item(reminderPartition(schedUsuarioA), reminderPointerSortKey("r-unico"))
	if puntero == nil || boolAttribute(puntero["deleted"]) {
		t.Fatalf("el puntero del recordatorio no debe borrarse: %v", puntero)
	}

	// Un Complete repetido con el mismo token no puede reaplicarse.
	if err := agenda.Complete(context.Background(), token, schedInstante); !errors.Is(err, appReminders.ErrLeasePerdido) && !errors.Is(err, appReminders.ErrOcurrenciaAusente) {
		t.Fatalf("segundo Complete: %v", err)
	}
}

func TestScheduleCompleteRetiraAgendaHuerfana(t *testing.T) {
	agenda, recordatorios, doble := schedEntorno(t, "nodo-a")
	dueAt := schedInstante.Add(-time.Hour)
	schedAlta(t, recordatorios, schedUsuarioA, "r-purgado", dueAt.Format(time.RFC3339), nil)
	oc := schedOcurrenciaPorID(t, agenda, "r-purgado")
	token := schedReclamar(t, agenda, oc, 30*time.Second)

	// E014 purga el recordatorio mientras el trabajo estaba reclamado.
	doble.eliminar(reminderPartition(schedUsuarioA), reminderSortKey(dueAt, "r-purgado"))

	if err := agenda.Complete(context.Background(), token, schedInstante); err != nil {
		t.Fatalf("Complete de una ocurrencia huérfana: %v", err)
	}
	if doble.item(domainDeletion.PartitionScheduleGlobal, oc.SK) != nil {
		t.Fatal("la ocurrencia huérfana debe retirarse, no reintentarse para siempre")
	}
	_, skPuntero := domainDeletion.SchedulePointerKeys(schedUsuarioA, "r-purgado")
	if doble.item(reminderPartition(schedUsuarioA), skPuntero) != nil {
		t.Fatal("el puntero huérfano debe retirarse")
	}
}

// TestScheduleE066MueveFechaDuranteLeaseNoPierdePuntero reproduce el defecto real
// del retiro de ocurrencias huérfanas: el puntero de agenda NO puede borrarse sin
// comprobar a qué ocurrencia apunta.
func TestScheduleE066MueveFechaDuranteLeaseNoPierdePuntero(t *testing.T) {
	// Secuencia pedida: Claim -> E066 mueve dueAt -> cierre del scheduler.
	// El puntero ya apunta a la ocurrencia NUEVA y debe conservarse para que E014
	// la alcance; la ocurrencia antigua ya no existe, así que el cierre se resuelve
	// como lease perdido y no como fallo de servicio.
	t.Run("secuencia_real", func(t *testing.T) {
		agenda, recordatorios, doble := schedEntorno(t, "nodo-a")
		dueAt := time.Date(2026, 11, 24, 9, 0, 0, 0, time.UTC)
		schedAlta(t, recordatorios, schedUsuarioA, "r-movido", dueAt.Format(time.RFC3339), nil)
		oc := schedOcurrenciaPorID(t, agenda, "r-movido")
		token := schedReclamar(t, agenda, oc, 30*time.Second)

		nuevo := dueAt.Add(24 * time.Hour)
		texto := nuevo.Format(time.RFC3339)
		if _, err := recordatorios.Update(context.Background(), appReminders.UpdateInput{
			UserID: schedUsuarioA, ID: "r-movido", ExpectedVersion: 1,
			DueAt: &texto, Now: schedInstante,
		}); err != nil {
			t.Fatalf("E066 debía mover el vencimiento: %v", err)
		}
		nuevaSK := scheduleSortKey(nuevo, "r-movido")
		pkPuntero, skPuntero := domainDeletion.SchedulePointerKeys(schedUsuarioA, "r-movido")
		if stringAttribute(doble.item(pkPuntero, skPuntero)["skGlobal"]) != nuevaSK {
			t.Fatal("E066 debía reapuntar el puntero a la ocurrencia nueva")
		}

		// El recordatorio de la fecha anterior ya no existe: la ocurrencia retirada
		// por el scheduler es huérfana.
		if err := agenda.Complete(context.Background(), token, schedInstante); !errors.Is(err, appReminders.ErrLeasePerdido) {
			t.Fatalf("el cierre de una ocurrencia ya movida debe resolverse como lease perdido: %v", err)
		}
		if puntero := doble.item(pkPuntero, skPuntero); stringAttribute(puntero["skGlobal"]) != nuevaSK {
			t.Fatalf("el puntero de la ocurrencia nueva se perdió: %v", puntero)
		}
		// El borrado del puntero no puede ser incondicional: el de la transacción
		// emitida debe exigir que siga apuntando a la ocurrencia retirada.
		if !schedBorraPunteroConCondicion(t, doble, pkPuntero, skPuntero, oc.SK) {
			t.Fatal("el puntero de agenda se borraba sin condición: E066 podía dejarlo apuntando a otra ocurrencia")
		}
		ocurrencias := schedDue(t, agenda, nuevo, 10)
		if len(ocurrencias) != 1 || ocurrencias[0].SK != nuevaSK {
			t.Fatalf("la agenda nueva debe seguir siendo reclamable: %+v", ocurrencias)
		}
	})

	// Estado que el guard protege: la ocurrencia antigua sobrevive al movimiento de
	// fecha (retiro incompleto, reparación o índice parcial). Aquí el borrado
	// incondicional sí destruía el puntero nuevo y dejaba la agenda inalcanzable
	// para E014.
	t.Run("ocurrencia_superviviente", func(t *testing.T) {
		agenda, recordatorios, doble := schedEntorno(t, "nodo-a")
		dueAt := time.Date(2026, 11, 24, 9, 0, 0, 0, time.UTC)
		schedAlta(t, recordatorios, schedUsuarioA, "r-superviviente", dueAt.Format(time.RFC3339), nil)
		oc := schedOcurrenciaPorID(t, agenda, "r-superviviente")
		token := schedReclamar(t, agenda, oc, 30*time.Second)
		arrendada := doble.item(domainDeletion.PartitionScheduleGlobal, oc.SK)
		if arrendada == nil {
			t.Fatal("falta la ocurrencia arrendada")
		}

		nuevo := dueAt.Add(48 * time.Hour)
		texto := nuevo.Format(time.RFC3339)
		if _, err := recordatorios.Update(context.Background(), appReminders.UpdateInput{
			UserID: schedUsuarioA, ID: "r-superviviente", ExpectedVersion: 1,
			DueAt: &texto, Now: schedInstante,
		}); err != nil {
			t.Fatalf("E066 debía mover el vencimiento: %v", err)
		}
		nuevaSK := scheduleSortKey(nuevo, "r-superviviente")
		// Se repone la ocurrencia antigua tal como la dejó el Claim: el puntero y el
		// recordatorio ya se movieron, pero la ocurrencia no se retiró.
		doble.poner(arrendada)

		if err := agenda.Complete(context.Background(), token, schedInstante); err != nil {
			t.Fatalf("el cierre huérfano debía retirar la ocurrencia antigua: %v", err)
		}
		if doble.item(domainDeletion.PartitionScheduleGlobal, oc.SK) != nil {
			t.Fatal("la ocurrencia antigua debía retirarse para no reintentarse para siempre")
		}
		// El reintento retiró SÓLO la ocurrencia: una acción, sin tocar el puntero.
		ultima := doble.transaccionesVistas[len(doble.transaccionesVistas)-1].TransactItems
		if len(ultima) != 1 || ultima[0].Delete == nil || stringAttribute(ultima[0].Delete.Key["SK"]) != oc.SK {
			t.Fatalf("el reintento debía retirar sólo la ocurrencia antigua: %+v", ultima)
		}
		// Puntero -> ocurrencia nueva, y la ocurrencia existe: E014 la resuelve.
		pkPuntero, skPuntero := domainDeletion.SchedulePointerKeys(schedUsuarioA, "r-superviviente")
		puntero := doble.item(pkPuntero, skPuntero)
		if stringAttribute(puntero["skGlobal"]) != nuevaSK {
			t.Fatalf("el puntero nuevo se perdió: %v", puntero)
		}
		if doble.item(domainDeletion.PartitionScheduleGlobal, nuevaSK) == nil {
			t.Fatal("el puntero quedó huérfano: su ocurrencia no existe")
		}
		if ocurrencias := schedDue(t, agenda, nuevo, 10); len(ocurrencias) != 1 || ocurrencias[0].SK != nuevaSK {
			t.Fatalf("la agenda nueva debía seguir siendo reclamable: %+v", ocurrencias)
		}
	})
}

// schedBorraPunteroConCondicion comprueba que toda transacción emitida que borre el
// puntero de agenda lo hace con una condición que lo ata a la ocurrencia retirada.
func schedBorraPunteroConCondicion(t *testing.T, doble *remFakeDynamo, pk, sk, ocSK string) bool {
	t.Helper()
	visto := false
	for _, tx := range doble.transaccionesVistas {
		for _, accion := range tx.TransactItems {
			if accion.Delete == nil {
				continue
			}
			if stringAttribute(accion.Delete.Key["PK"]) != pk || stringAttribute(accion.Delete.Key["SK"]) != sk {
				continue
			}
			visto = true
			condicion := aws.ToString(accion.Delete.ConditionExpression)
			if !strings.Contains(condicion, "skGlobal") {
				return false
			}
			if stringAttribute(accion.Delete.ExpressionAttributeValues[":ocSK"]) != ocSK {
				return false
			}
		}
	}
	return visto
}

func TestScheduleCompleteConFenceObsoletoEsLeasePerdido(t *testing.T) {
	agenda, recordatorios, doble := schedEntorno(t, "nodo-a")
	dueAt := schedInstante.Add(-time.Hour)
	schedAlta(t, recordatorios, schedUsuarioA, "r-complete-ajeno", dueAt.Format(time.RFC3339), nil)
	token := schedReclamar(t, agenda, schedOcurrenciaPorID(t, agenda, "r-complete-ajeno"), time.Minute)

	// El lease fue reclamado por otro nodo: owner y fence cambiaron.
	item := doble.item(domainDeletion.PartitionScheduleGlobal, token.Ocurrencia.SK)
	item["owner"] = stringAttr("nodo-b")
	item["fence"] = numberAttr(token.Fence + 1)
	item["version"] = numberAttr(token.Version + 1)
	doble.poner(item)

	if err := agenda.Complete(context.Background(), token, schedInstante); !errors.Is(err, appReminders.ErrLeasePerdido) {
		t.Fatalf("un token superado no puede completar: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Aislamiento entre titulares y ausencia de Scan
// ---------------------------------------------------------------------------

func TestScheduleAislamientoEntreTitulares(t *testing.T) {
	agenda, recordatorios, _ := schedEntorno(t, "nodo-a")
	dueAt := schedInstante.Add(-time.Hour).Format(time.RFC3339)
	schedAlta(t, recordatorios, schedUsuarioA, "r-a", dueAt, nil)
	schedAlta(t, recordatorios, schedUsuarioB, "r-b", dueAt, nil)

	ocurrencias := schedDue(t, agenda, schedInstante, 10)
	if len(ocurrencias) != 2 {
		t.Fatalf("la partición global es compartida: %d", len(ocurrencias))
	}
	titulares := map[string]bool{}
	for _, oc := range ocurrencias {
		titulares[oc.UserID] = true
	}
	if !titulares[schedUsuarioA] || !titulares[schedUsuarioB] {
		t.Fatalf("cada ocurrencia debe llevar su titular: %v", titulares)
	}
	// Completar una no afecta a la otra.
	token := schedReclamar(t, agenda, schedOcurrenciaPorID(t, agenda, "r-a"), time.Minute)
	if err := agenda.Complete(context.Background(), token, schedInstante); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	restantes := schedDue(t, agenda, schedInstante, 10)
	if len(restantes) != 1 || restantes[0].ReminderID != "r-b" {
		t.Fatalf("el otro titular debe conservar su ocurrencia: %v", restantes)
	}
}

func TestScheduleAPIYRemindersAPINoExponenScan(t *testing.T) {
	for nombre, tipo := range map[string]reflect.Type{
		"ScheduleAPI":  reflect.TypeOf((*ScheduleAPI)(nil)).Elem(),
		"RemindersAPI": reflect.TypeOf((*RemindersAPI)(nil)).Elem(),
	} {
		for i := 0; i < tipo.NumMethod(); i++ {
			if strings.EqualFold(tipo.Method(i).Name, "Scan") {
				t.Fatalf("%s no puede exponer Scan", nombre)
			}
		}
	}
}
