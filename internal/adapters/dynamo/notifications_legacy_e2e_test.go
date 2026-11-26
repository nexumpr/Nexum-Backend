package dynamo

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	appExport "github.com/nexumpr/Nexum-Backend/internal/application/export"
	appNotifications "github.com/nexumpr/Nexum-Backend/internal/application/notifications"
)

// registrarLegacy reproduce EXACTAMENTE el item que el worker del outbox escribió
// en producción hasta v0.21.2: `entityType=USER_NOTIFICATION` y el `type` del
// scheduler (`reminder`, fuera del enum contractual) en la SK de la bandeja.
// No es un fixture artístico: es la forma persistida que ocultaba la bandeja.
func registrarLegacy(f *e061FakeClient, userID, id, tipo string, creado time.Time, leida bool) map[string]types.AttributeValue {
	pk := notificationUserPK(userID)
	sk := notificationSortKey(creado.UTC(), id)
	item := map[string]types.AttributeValue{
		"PK":             stringAttr(pk),
		"SK":             stringAttr(sk),
		"entityType":     stringAttr(legacyNotificationEntityType),
		"id":             stringAttr(id),
		"title":          stringAttr("Recordatorio"),
		"message":        stringAttr("Pago programado"),
		"type":           stringAttr(tipo),
		"isRead":         boolAttr(leida),
		"createdAt":      stringAttr(creado.UTC().Format(time.RFC3339Nano)),
		"deliveryStatus": stringAttr("DELIVERED"),
	}
	f.items[claveItem(pk, sk)] = item
	return item
}

// TestE061BandejaLegacyYCanonicaEsLaRegresionDeProduccion cubre el defecto real de
// v0.21.2 con registros con la MISMA forma que DynamoDB contiene: el listado, el
// conteo, los filtros (antes de paginar) y la marca de lectura deben tratar igual
// el `entityType` histórico y el canónico, y el `type=reminder` debe responder al
// filtro contractual `system`.
func TestE061BandejaLegacyYCanonicaEsLaRegresionDeProduccion(t *testing.T) {
	cliente := nuevoE061Fake()
	base := time.Date(2026, 11, 25, 8, 0, 0, 0, time.UTC)

	// Histórico escrito por el worker (lo que hoy existe en la tabla).
	registrarLegacy(cliente, "user-a", "018f90ef-08e5-7a68-b09f-000000000101", "reminder", base, false)
	registrarLegacy(cliente, "user-a", "018f90ef-08e5-7a68-b09f-000000000102", "reminder", base.Add(time.Minute), true)
	// Escritura nueva canónica tras el fix.
	cliente.sembrar("user-a", 1, 1, appNotifications.TypeBudget)
	// Otro titular no debe aparecer jamás en la bandeja de A.
	registrarLegacy(cliente, "user-b", "018f90ef-08e5-7a68-b09f-000000000201", "reminder", base, false)

	store := NewNotificationsStore(cliente, "nexum-table")
	ctx := context.Background()

	lista, err := store.List(ctx, appNotifications.ListQuery{UserID: "user-a", Limit: 50})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(lista.Items) != 3 {
		t.Fatalf("la bandeja debe incluir 2 legacy + 1 canónica, obtenido %d", len(lista.Items))
	}
	for _, item := range lista.Items {
		if item.Type == appNotifications.LegacyTypeReminder {
			t.Fatalf("un item legacy no puede proyectarse con type=%q", item.Type)
		}
	}

	// Filtro contractual `system`: debe alcanzar los dos avisos históricos con
	// type=reminder y excluir budget. Si el filtro no incluyera el alias, la
	// respuesta sería vacía aunque las notificaciones existan.
	sistema := appNotifications.TypeSystem
	filtradas, err := store.List(ctx, appNotifications.ListQuery{UserID: "user-a", Limit: 50, Type: sistema})
	if err != nil {
		t.Fatalf("List type=system: %v", err)
	}
	if len(filtradas.Items) != 2 {
		t.Fatalf("type=system debe devolver los 2 avisos legacy, obtenido %d", len(filtradas.Items))
	}
	for _, item := range filtradas.Items {
		if item.Type != appNotifications.TypeSystem {
			t.Fatalf("type=%q, esperado system", item.Type)
		}
	}

	presupuesto := appNotifications.TypeBudget
	otras, err := store.List(ctx, appNotifications.ListQuery{UserID: "user-a", Limit: 50, Type: presupuesto})
	if err != nil || len(otras.Items) != 1 {
		t.Fatalf("type=budget no debe absorber avisos: %d %v", len(otras.Items), err)
	}

	// El filtro se aplica antes de paginar: con limit=1 y type=system deben salir
	// dos páginas sin duplicados aunque haya items de otro tipo intercalados.
	vistas := map[string]bool{}
	var cursor *appNotifications.NotificationKey
	for i := 0; i < 3; i++ {
		pagina, err := store.List(ctx, appNotifications.ListQuery{UserID: "user-a", Limit: 1, Type: sistema, After: cursor})
		if err != nil {
			t.Fatalf("paginación type=system: %v", err)
		}
		for _, item := range pagina.Items {
			if vistas[item.ID] {
				t.Fatalf("id duplicado entre páginas: %s", item.ID)
			}
			vistas[item.ID] = true
		}
		cursor = pagina.NextKey
		if cursor == nil {
			break
		}
	}
	if len(vistas) != 2 {
		t.Fatalf("paginación type=system recorrió %d avisos, esperado 2", len(vistas))
	}

	total, err := store.CountUnread(ctx, "user-a")
	if err != nil {
		t.Fatalf("CountUnread: %v", err)
	}
	if total != 2 {
		t.Fatalf("unreadCount=%d, esperado 2 (legacy no leída + budget)", total)
	}

	// E062 sobre un aviso legacy: la lectura debe persistir y descontar del total.
	leida, err := store.MarkRead(ctx, "user-a", "018f90ef-08e5-7a68-b09f-000000000101", base.Add(time.Hour))
	if err != nil || leida == nil || !leida.IsRead {
		t.Fatalf("MarkRead legacy: %+v %v", leida, err)
	}
	if leida.Type != appNotifications.TypeSystem {
		t.Fatalf("MarkRead legacy type=%q, esperado system", leida.Type)
	}
	restantes, err := store.CountUnread(ctx, "user-a")
	if err != nil || restantes != 1 {
		t.Fatalf("unreadCount tras E062=%d, esperado 1 (%v)", restantes, err)
	}

	// E063 depende del mismo List+MarkRead del adaptador: la admisión debe contar
	// el aviso legacy como no leído y poder marcarlo.
	estado, err := store.BeginReadAll(ctx, "user-a", "req-legacy-1", base.Add(2*time.Hour))
	if err != nil || estado == nil {
		t.Fatalf("BeginReadAll: %+v %v", estado, err)
	}
	if estado.InitialUnread != 1 {
		t.Fatalf("initialUnread=%d, esperado 1", estado.InitialUnread)
	}
	pendientes, err := store.List(ctx, appNotifications.ListQuery{UserID: "user-a", Limit: 50, IsRead: boolPtr(false)})
	if err != nil {
		t.Fatalf("List pendientes: %v", err)
	}
	for _, item := range pendientes.Items {
		if _, err := store.MarkRead(ctx, "user-a", item.ID, base.Add(2*time.Hour)); err != nil {
			t.Fatalf("MarkRead en lote E063: %v", err)
		}
	}
	if final, err := store.CountUnread(ctx, "user-a"); err != nil || final != 0 {
		t.Fatalf("unreadCount final=%d, esperado 0 (%v)", final, err)
	}

	// Aislamiento: el registro de B sigue intacto y no aparece en A.
	if b, err := store.CountUnread(ctx, "user-b"); err != nil || b != 1 {
		t.Fatalf("el titular B debe conservar su aviso: %d %v", b, err)
	}
}

// TestE020ExportaBandejaLegacy fija la frontera de la exportación: un aviso con
// entityType histórico no puede tumbar la exportación con ErrInconsistent, y un
// item realmente incoherente sigue rechazándose (no se ignora en silencio).
func TestE020ExportaBandejaLegacy(t *testing.T) {
	cliente := nuevoE061Fake()
	base := time.Date(2026, 11, 25, 9, 0, 0, 0, time.UTC)
	registrarLegacy(cliente, "user-a", "018f90ef-08e5-7a68-b09f-000000000301", "reminder", base, false)
	cliente.sembrar("user-a", 1, 1, appNotifications.TypeSystem)

	fuente := NewExportStore(ExportStoreDeps{Cliente: cliente, Tabla: "nexum-table"})
	filas, siguiente, err := fuente.Notificaciones(context.Background(), "user-a", "", 50)
	if err != nil {
		t.Fatalf("Notificaciones con legacy: %v", err)
	}
	if siguiente != "" || len(filas) != 2 {
		t.Fatalf("filas=%d siguiente=%q, esperado 2 y fin de página", len(filas), siguiente)
	}

	// Un item con una entidad distinta sigue siendo incoherente: no se exporta ni
	// se descarta en silencio, la exportación falla cerrada.
	pk := notificationUserPK("user-a")
	sk := notificationSortKey(base.Add(time.Minute), "018f90ef-08e5-7a68-b09f-000000000302")
	cliente.items[claveItem(pk, sk)] = map[string]types.AttributeValue{
		"PK": stringAttr(pk), "SK": stringAttr(sk), "entityType": stringAttr("OTRA_COSA"),
		"id":        stringAttr("018f90ef-08e5-7a68-b09f-000000000302"),
		"createdAt": stringAttr(base.Add(time.Minute).Format(time.RFC3339Nano)),
	}
	if _, _, err := fuente.Notificaciones(context.Background(), "user-a", "", 50); !errors.Is(err, appExport.ErrInconsistent) {
		t.Fatalf("item incoherente: esperado ErrInconsistent, obtenido %v", err)
	}
}

// TestE061FiltroTipoNoTraduceValoresDesconocidos protege contra una normalización
// indiscriminada: sólo `system` absorbe el alias histórico. Un tipo desconocido
// se conserva tal cual y no se convierte en system.
func TestE061FiltroTipoNoTraduceValoresDesconocidos(t *testing.T) {
	if got := appNotifications.CanonicalType(appNotifications.Type("budget")); got != appNotifications.TypeBudget {
		t.Fatalf("budget no debe cambiar: %q", got)
	}
	if got := appNotifications.CanonicalType(appNotifications.Type("desconocido")); got != appNotifications.Type("desconocido") {
		t.Fatalf("un tipo desconocido no se reescribe: %q", got)
	}
	if got := appNotifications.CanonicalType(appNotifications.LegacyTypeReminder); got != appNotifications.TypeSystem {
		t.Fatalf("reminder debe normalizarse a system: %q", got)
	}
	if alias := appNotifications.TypeFilterAliases(appNotifications.TypeBudget); len(alias) != 0 {
		t.Fatalf("budget no admite alias: %v", alias)
	}
	if alias := appNotifications.TypeFilterAliases(appNotifications.TypeSystem); len(alias) != 1 ||
		alias[0] != appNotifications.LegacyTypeReminder {
		t.Fatalf("system debe admitir el alias reminder: %v", alias)
	}
}

func boolPtr(v bool) *bool { return &v }

// guardia de compilación: el filtro emitido usa expresiones con OR y no debe
// contener AND anidado que rompa la precedencia.
func TestE061FiltroTipoSeEmiteConParentesis(t *testing.T) {
	cliente := nuevoE061Fake()
	store := NewNotificationsStore(cliente, "nexum-table")
	if _, err := store.List(context.Background(), appNotifications.ListQuery{
		UserID: "user-a", Limit: 5, Type: appNotifications.TypeSystem,
	}); err != nil {
		t.Fatal(err)
	}
	if len(cliente.queries) == 0 {
		t.Fatal("no se emitió Query")
	}
	expresion := strings.Join([]string{
		stringAttrValue(cliente.queries[0].FilterExpression),
	}, "")
	if !strings.Contains(expresion, "(#type = :type OR #typeAlias0 = :typeAlias0)") {
		t.Fatalf("filtro type sin alias entre paréntesis: %q", expresion)
	}
}

func stringAttrValue(valor *string) string {
	if valor == nil {
		return ""
	}
	return *valor
}
