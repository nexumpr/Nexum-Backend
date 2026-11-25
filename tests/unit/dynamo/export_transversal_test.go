package dynamo_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	dynamoAdapter "github.com/nexumpr/Nexum-Backend/internal/adapters/dynamo"
	appExport "github.com/nexumpr/Nexum-Backend/internal/application/export"
	domainDynamo "github.com/nexumpr/Nexum-Backend/internal/domain/dynamo"
)

// Pruebas de las familias transversales de E020 sobre el adaptador REAL:
// bandeja de notificaciones, recordatorios, dispositivos push y el singleton de preferencias de
// notificaciones. Comprueban aislamiento A/B, paginación real, consistencia fuerte y que ningún
// token, huella, hash ni clave de idempotencia llega al contrato.

const (
	e020TA = "aaaaaaaa-1111-4222-8333-444444444444"
	e020TB = "bbbbbbbb-1111-4222-8333-444444444444"

	e020SecretoTokenA = "token-sintetico-de-A-no-exportable"
	e020SecretoTokenB = "token-sintetico-de-B-no-exportable"
)

var e020InstanteNotif = time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)

func e020PrefijoUsuario(userID string) string { return domainDynamo.PrefixUser + userID }

func e020ItemNotificacion(userID, id string, leida bool, creada time.Time) (string, map[string]types.AttributeValue) {
	sk := appExport.PrefijoNotificacion + creada.UTC().Format(time.RFC3339Nano) + "#" + id
	return sk, map[string]types.AttributeValue{
		"entityType": strAttr("NOTIFICATION"),
		"userId":     strAttr(userID),
		"id":         strAttr(id),
		"title":      strAttr("Aviso " + id),
		"message":    strAttr("Mensaje " + id),
		"isRead":     boolAttr(leida),
		"createdAt":  strAttr(creada.UTC().Format(time.RFC3339Nano)),
	}
}

func e020ItemRecordatorio(userID, id, frecuencia string, vence time.Time) (string, map[string]types.AttributeValue) {
	sk := appExport.PrefijoRecordatorio + vence.UTC().Format("2006-01-02T15:04:05.000Z") + "#" + id
	item := map[string]types.AttributeValue{
		"entityType": strAttr("USER_REMINDER"),
		"reminderID": strAttr(id),
		"userId":     strAttr(userID),
		"title":      strAttr("Recordatorio " + id),
		"dueAt":      strAttr(vence.UTC().Format(time.RFC3339)),
		"timezone":   strAttr("America/Mexico_City"),
		"active":     boolAttr(true),
		"deleted":    boolAttr(false),
		"version":    numAttr(2),
		"createdAt":  strAttr(vence.Add(-48 * time.Hour).UTC().Format(time.RFC3339Nano)),
		"updatedAt":  strAttr(vence.Add(-24 * time.Hour).UTC().Format(time.RFC3339Nano)),
	}
	if frecuencia != "" {
		item["frequency"] = strAttr(frecuencia)
	}
	return sk, item
}

func e020ItemDispositivo(userID, id, etiqueta string, activo bool, token string) (string, map[string]types.AttributeValue) {
	sk := appExport.PrefijoDispositivo + id
	return sk, map[string]types.AttributeValue{
		"entityType":       strAttr("DEVICE"),
		"userId":           strAttr(userID),
		"provider":         strAttr("fcm"),
		"deviceLabel":      strAttr(etiqueta),
		"registeredAt":     strAttr(e020InstanteNotif.Format(time.RFC3339Nano)),
		"active":           boolAttr(activo),
		"version":          numAttr(1),
		"tokenCipher":      strAttr(token),
		"tokenFingerprint": strAttr("huella-" + token),
	}
}

func e020ItemPreferenciasNotificacion(userID string, push bool, version int64) (string, map[string]types.AttributeValue) {
	return appExport.SKPreferenciasNotificacion, map[string]types.AttributeValue{
		"entityType":         strAttr("USER_NOTIFICATION_PREFERENCES"),
		"userId":             strAttr(userID),
		"pushEnabled":        boolAttr(push),
		"emailEnabled":       boolAttr(false),
		"reminderEnabled":    boolAttr(true),
		"budgetAlerts":       boolAttr(true),
		"goalUpdates":        boolAttr(false),
		"recurringReminders": boolAttr(true),
		"streakReminders":    boolAttr(false),
		"quietHoursEnabled":  boolAttr(true),
		"quietHoursStart":    strAttr("22:00"),
		"quietHoursEnd":      strAttr("07:00"),
		"timeZone":           strAttr("America/Mexico_City"),
		"version":            numAttr(version),
	}
}

func e020TiendaTransversal(t *testing.T, dyn *e020Dynamo) *dynamoAdapter.ExportStore {
	t.Helper()
	return dynamoAdapter.NewExportStore(dynamoAdapter.ExportStoreDeps{Cliente: dyn, Tabla: e020TablaDynamo})
}

// TestE020TransversalAislamientoAB comprueba que cada titular sólo ve sus propias
// notificaciones, recordatorios, dispositivos y preferencias.
func TestE020TransversalAislamientoAB(t *testing.T) {
	dyn := newE020Dynamo()
	skN1, itemN1 := e020ItemNotificacion(e020TA, "n1", false, e020InstanteNotif)
	dyn.sembrar(e020PrefijoUsuario(e020TA), skN1, itemN1)
	skN2, itemN2 := e020ItemNotificacion(e020TA, "n2", true, e020InstanteNotif.Add(time.Hour))
	dyn.sembrar(e020PrefijoUsuario(e020TA), skN2, itemN2)
	skNB, itemNB := e020ItemNotificacion(e020TB, "nb", false, e020InstanteNotif)
	dyn.sembrar(e020PrefijoUsuario(e020TB), skNB, itemNB)

	skR1, itemR1 := e020ItemRecordatorio(e020TA, "r1", "weekly", e020InstanteNotif.Add(72*time.Hour))
	dyn.sembrar(e020PrefijoUsuario(e020TA), skR1, itemR1)
	skRB, itemRB := e020ItemRecordatorio(e020TB, "rb", "", e020InstanteNotif.Add(96*time.Hour))
	dyn.sembrar(e020PrefijoUsuario(e020TB), skRB, itemRB)

	skDA, itemDA := e020ItemDispositivo(e020TA, "d1", "Pixel A", true, e020SecretoTokenA)
	dyn.sembrar(e020PrefijoUsuario(e020TA), skDA, itemDA)
	skDAb, itemDAb := e020ItemDispositivo(e020TA, "d2", "Pixel A viejo", false, e020SecretoTokenA)
	dyn.sembrar(e020PrefijoUsuario(e020TA), skDAb, itemDAb)
	skDB, itemDB := e020ItemDispositivo(e020TB, "db", "Pixel B", true, e020SecretoTokenB)
	dyn.sembrar(e020PrefijoUsuario(e020TB), skDB, itemDB)

	skPA, itemPA := e020ItemPreferenciasNotificacion(e020TA, true, 3)
	dyn.sembrar(e020PrefijoUsuario(e020TA), skPA, itemPA)
	skPB, itemPB := e020ItemPreferenciasNotificacion(e020TB, false, 1)
	dyn.sembrar(e020PrefijoUsuario(e020TB), skPB, itemPB)

	tienda := e020TiendaTransversal(t, dyn)
	ctx := context.Background()

	notifs, siguiente, err := tienda.Notificaciones(ctx, e020TA, "", 25)
	if err != nil || siguiente != "" {
		t.Fatalf("notificaciones A: %v (siguiente=%q)", err, siguiente)
	}
	if len(notifs) != 2 || notifs[0].ID != "n1" || notifs[1].ID != "n2" {
		t.Fatalf("notificaciones A inesperadas: %+v", notifs)
	}
	if notifs[0].IsRead || !notifs[1].IsRead {
		t.Fatalf("isRead no proyectado: %+v", notifs)
	}

	remA, _, err := tienda.Recordatorios(ctx, e020TA, "", 25)
	if err != nil || len(remA) != 1 || remA[0].ID != "r1" {
		t.Fatalf("recordatorios A: %+v err=%v", remA, err)
	}
	if remA[0].Frequency == nil || *remA[0].Frequency != "weekly" || remA[0].Timezone != "America/Mexico_City" {
		t.Fatalf("recordatorio A mal proyectado: %+v", remA[0])
	}

	devA, _, err := tienda.Dispositivos(ctx, e020TA, "", 25)
	if err != nil || len(devA) != 2 {
		t.Fatalf("dispositivos A: %+v err=%v", devA, err)
	}
	if !devA[0].Active || devA[1].Active {
		t.Fatalf("active no proyectado: %+v", devA)
	}

	prefsA, err := tienda.PreferenciasNotificacion(ctx, e020TA)
	if err != nil || prefsA == nil {
		t.Fatalf("preferencias A: %+v err=%v", prefsA, err)
	}
	if !prefsA.PushEnabled || prefsA.GoalUpdates || prefsA.Version != 3 {
		t.Fatalf("preferencias A inesperadas: %+v", prefsA)
	}
	if prefsA.QuietHours == nil || !prefsA.QuietHours.Enabled || prefsA.QuietHours.Start != "22:00" {
		t.Fatalf("quietHours no proyectado: %+v", prefsA.QuietHours)
	}

	// B sólo ve lo suyo.
	notifsB, _, err := tienda.Notificaciones(ctx, e020TB, "", 25)
	if err != nil || len(notifsB) != 1 || notifsB[0].ID != "nb" {
		t.Fatalf("notificaciones B: %+v err=%v", notifsB, err)
	}
	remB, _, err := tienda.Recordatorios(ctx, e020TB, "", 25)
	if err != nil || len(remB) != 1 || remB[0].ID != "rb" {
		t.Fatalf("recordatorios B: %+v err=%v", remB, err)
	}
	if remB[0].Frequency != nil {
		t.Fatalf("un recordatorio sin frecuencia debe exportarse sin frequency: %+v", remB[0])
	}
	devB, _, err := tienda.Dispositivos(ctx, e020TB, "", 25)
	if err != nil || len(devB) != 1 || devB[0].ID != "db" {
		t.Fatalf("dispositivos B: %+v err=%v", devB, err)
	}
	prefsB, err := tienda.PreferenciasNotificacion(ctx, e020TB)
	if err != nil || prefsB == nil || prefsB.PushEnabled || prefsB.Version != 1 {
		t.Fatalf("preferencias B: %+v err=%v", prefsB, err)
	}

	// Ningún token, huella ni hash en el contrato exportado.
	contratos := make([]any, 0, len(devA)+len(notifs)+len(remA)+1)
	for _, d := range devA {
		contratos = append(contratos, appExport.PushDeviceContract{
			ID: d.ID, Provider: d.Provider, DeviceLabel: d.DeviceLabel,
			RegisteredAt: d.RegisteredAt.UTC().Format(time.RFC3339), Active: d.Active,
		})
	}
	for _, n := range notifs {
		contratos = append(contratos, appExport.NotificationContract{
			ID: n.ID, Title: n.Title, Message: n.Message, IsRead: n.IsRead,
			CreatedAt: n.CreatedAt.UTC().Format(time.RFC3339),
		})
	}
	for _, r := range remA {
		contratos = append(contratos, appExport.ReminderContract{
			ID: r.ID, Title: r.Title, DueAt: r.DueAt.UTC().Format(time.RFC3339), Timezone: r.Timezone,
			Frequency: r.Frequency, Active: r.Active,
			CreatedAt: r.CreatedAt.UTC().Format(time.RFC3339), UpdatedAt: r.UpdatedAt.UTC().Format(time.RFC3339),
		})
	}
	crudo, errJSON := json.Marshal(contratos)
	if errJSON != nil {
		t.Fatalf("serializacion: %v", errJSON)
	}
	texto := string(crudo)
	for _, prohibido := range []string{e020SecretoTokenA, e020SecretoTokenB, "token", "Cipher", "Fingerprint", "huella", "IDEM#", "leaseUntil", "passwordHash"} {
		if strings.Contains(texto, prohibido) {
			t.Fatalf("el contrato exportado no puede contener %q: %s", prohibido, texto)
		}
	}
}

// TestE020TransversalPaginacionYConsistencia comprueba el patrón limit+1, la reanudación por SK
// y que toda lectura (Query y GetItem) es fuertemente consistente y acotada a la partición.
func TestE020TransversalPaginacionYConsistencia(t *testing.T) {
	dyn := newE020Dynamo()
	for i, id := range []string{"n1", "n2", "n3"} {
		sk, item := e020ItemNotificacion(e020TA, id, false, e020InstanteNotif.Add(time.Duration(i)*time.Minute))
		dyn.sembrar(e020PrefijoUsuario(e020TA), sk, item)
	}
	skP, itemP := e020ItemPreferenciasNotificacion(e020TA, true, 2)
	dyn.sembrar(e020PrefijoUsuario(e020TA), skP, itemP)

	tienda := e020TiendaTransversal(t, dyn)
	ctx := context.Background()

	primera, siguiente, err := tienda.Notificaciones(ctx, e020TA, "", 2)
	if err != nil || len(primera) != 2 {
		t.Fatalf("primera pagina: %+v err=%v", primera, err)
	}
	if !strings.HasSuffix(siguiente, "#n2") {
		t.Fatalf("continuacion inesperada: %q", siguiente)
	}
	segunda, fin, err := tienda.Notificaciones(ctx, e020TA, siguiente, 2)
	if err != nil || len(segunda) != 1 || segunda[0].ID != "n3" || fin != "" {
		t.Fatalf("segunda pagina: %+v siguiente=%q err=%v", segunda, fin, err)
	}

	if _, err := tienda.PreferenciasNotificacion(ctx, e020TA); err != nil {
		t.Fatalf("preferencias: %v", err)
	}

	consultas := dyn.consultasRealizadas()
	if len(consultas) != 2 {
		t.Fatalf("consultas esperadas 2, obtenidas %d", len(consultas))
	}
	for _, consulta := range consultas {
		if !consulta.Consistente {
			t.Fatalf("toda Query debe ser fuertemente consistente: %+v", consulta)
		}
		if !consulta.Ascendente {
			t.Fatalf("la exportacion exige orden ascendente: %+v", consulta)
		}
		if consulta.PK != e020PrefijoUsuario(e020TA) {
			t.Fatalf("la Query salio de la particion del titular: %+v", consulta)
		}
		if consulta.Prefijo != appExport.PrefijoNotificacion {
			t.Fatalf("prefijo inesperado: %+v", consulta)
		}
		if consulta.Limite != 3 {
			t.Fatalf("el adaptador debe pedir limit+1: %+v", consulta)
		}
		if consulta.RangoSK != "" {
			t.Fatalf("no se admite un rango abierto de SK: %+v", consulta)
		}
	}
	lecturas := dyn.lecturasRealizadas()
	if len(lecturas) != 1 || lecturas[0].SK != appExport.SKPreferenciasNotificacion ||
		lecturas[0].PK != e020PrefijoUsuario(e020TA) || !lecturas[0].Consistente {
		t.Fatalf("lectura del singleton inesperada: %+v", lecturas)
	}
}

// TestE020TransversalFailClosed comprueba que un item incoherente no se emite y que un fallo de
// persistencia nunca se convierte en una colección vacía.
func TestE020TransversalFailClosed(t *testing.T) {
	dyn := newE020Dynamo()
	sk, item := e020ItemNotificacion(e020TA, "n1", false, e020InstanteNotif)
	item["entityType"] = strAttr("OTRA_COSA")
	dyn.sembrar(e020PrefijoUsuario(e020TA), sk, item)
	tienda := e020TiendaTransversal(t, dyn)
	if _, _, err := tienda.Notificaciones(context.Background(), e020TA, "", 25); !errors.Is(err, appExport.ErrInconsistent) {
		t.Fatalf("un item incoherente debe ser ErrInconsistent: %v", err)
	}

	dyn2 := newE020Dynamo()
	dyn2.err = errors.New("dynamodb caido")
	tienda2 := e020TiendaTransversal(t, dyn2)
	if _, _, err := tienda2.Recordatorios(context.Background(), e020TA, "", 25); !errors.Is(err, appExport.ErrUnavailable) {
		t.Fatalf("un fallo de lectura debe ser ErrUnavailable: %v", err)
	}
	if _, err := tienda2.PreferenciasNotificacion(context.Background(), e020TA); !errors.Is(err, appExport.ErrUnavailable) {
		t.Fatalf("un fallo del singleton debe ser ErrUnavailable: %v", err)
	}

	// Sin item de preferencias: (nil, nil), sin inventar defaults.
	dyn3 := newE020Dynamo()
	tienda3 := e020TiendaTransversal(t, dyn3)
	prefs, err := tienda3.PreferenciasNotificacion(context.Background(), e020TA)
	if err != nil || prefs != nil {
		t.Fatalf("sin item debe devolver (nil, nil): %+v err=%v", prefs, err)
	}

	// Un titular vacío no puede exportar: no se consulta otra partición.
	if _, _, err := tienda3.Notificaciones(context.Background(), "  ", "", 25); !errors.Is(err, appExport.ErrUnavailable) {
		t.Fatalf("sin titular debe ser ErrUnavailable: %v", err)
	}
}
