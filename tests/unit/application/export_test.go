package application_test

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	appExport "github.com/nexumpr/Nexum-Backend/internal/application/export"
	appGoals "github.com/nexumpr/Nexum-Backend/internal/application/goals"
	appIdentity "github.com/nexumpr/Nexum-Backend/internal/application/identity"
	appMovements "github.com/nexumpr/Nexum-Backend/internal/application/movements"
	appOnboarding "github.com/nexumpr/Nexum-Backend/internal/application/onboarding"
	appPreferences "github.com/nexumpr/Nexum-Backend/internal/application/preferences"
	appPrivacy "github.com/nexumpr/Nexum-Backend/internal/application/privacy"
	appProfile "github.com/nexumpr/Nexum-Backend/internal/application/profile"
	appRecurring "github.com/nexumpr/Nexum-Backend/internal/application/recurring"
	domainGoal "github.com/nexumpr/Nexum-Backend/internal/domain/goal"
	"github.com/nexumpr/Nexum-Backend/pkg/httputil"
)

// Pruebas de aplicación de E020 (`GET /api/v1/users/me/export`, RFC-022 §2): orden fijo de
// colecciones, continuación real hasta agotarlas, cursor firmado ligado al usuario y al dominio
// `E020_EXPORT`, singletons completos en cada página y fail-closed sin firma, sin persistencia o
// ante un fallo de lectura (503 sin cursor).

const (
	e020Usuario = "11111111-2222-4333-8444-555555555555"
	e020Otro    = "99999999-8888-4777-8666-555555555555"
	e020Secreto = "secreto-de-pruebas-e020-con-suficiente-entropia"
)

var e020Ahora = time.Date(2026, 11, 19, 18, 30, 0, 0, time.UTC)

// e020Fila conserva la SK física junto al movimiento: el cursor se construye con posiciones
// reales, no con índices.
type e020Fila struct {
	sk         string
	movimiento appMovements.Movement
}

type e020MetaFila struct {
	sk   string
	fila appGoals.GoalRow
}

// e020Fuente es un doble con estado que respeta el contrato del puerto: devuelve hasta `limit`
// filas con SK estrictamente mayor que `after` y la SK de continuación.
type e020Fuente struct {
	movimientos     []e020Fila
	metas           []e020MetaFila
	perfil          *appProfile.Profile
	consentimientos []appPrivacy.ConsentRecord

	notificaciones []e020NotifFila
	recordatorios  []e020RemFila
	dispositivos   []e020DevFila
	prefsNotif     *appExport.NotificationPreferencesRow

	errMovimientos       error
	errMetas             error
	errPerfil            error
	errConsentimientos   error
	errNotificaciones    error
	errPreferenciasNotif error

	consultas []string
}

// Filas con SK física para que el cursor use posiciones reales también en las familias nuevas.
type e020NotifFila struct {
	sk   string
	fila appExport.NotificationRow
}

type e020RemFila struct {
	sk   string
	fila appExport.ReminderRow
}

type e020DevFila struct {
	sk   string
	fila appExport.PushDeviceRow
}

func (f *e020Fuente) Movimientos(_ context.Context, userID, after string, limit int) ([]appMovements.Movement, string, error) {
	f.consultas = append(f.consultas, "movements:"+userID+":"+after)
	if f.errMovimientos != nil {
		return nil, "", f.errMovimientos
	}
	return e020PaginarMovimientos(f.movimientos, after, limit)
}

func (f *e020Fuente) Metas(_ context.Context, userID, after string, limit int) ([]appGoals.GoalRow, string, error) {
	f.consultas = append(f.consultas, "goals:"+userID+":"+after)
	if f.errMetas != nil {
		return nil, "", f.errMetas
	}
	return e020PaginarMetas(f.metas, after, limit)
}

func (f *e020Fuente) Perfil(_ context.Context, userID string) (*appProfile.Profile, error) {
	f.consultas = append(f.consultas, "profile:"+userID)
	if f.errPerfil != nil {
		return nil, f.errPerfil
	}
	return f.perfil, nil
}

func (f *e020Fuente) Consentimientos(_ context.Context, userID string) ([]appPrivacy.ConsentRecord, error) {
	f.consultas = append(f.consultas, "consents:"+userID)
	if f.errConsentimientos != nil {
		return nil, f.errConsentimientos
	}
	return f.consentimientos, nil
}

func e020PaginarMovimientos(filas []e020Fila, after string, limit int) ([]appMovements.Movement, string, error) {
	restantes := make([]e020Fila, 0, len(filas))
	for _, fila := range filas {
		if fila.sk > after {
			restantes = append(restantes, fila)
		}
	}
	salida := make([]appMovements.Movement, 0, limit)
	for i := 0; i < len(restantes) && i < limit; i++ {
		salida = append(salida, restantes[i].movimiento)
	}
	if len(restantes) > limit {
		return salida, restantes[limit-1].sk, nil
	}
	return salida, "", nil
}

func e020PaginarMetas(filas []e020MetaFila, after string, limit int) ([]appGoals.GoalRow, string, error) {
	restantes := make([]e020MetaFila, 0, len(filas))
	for _, fila := range filas {
		if fila.sk > after {
			restantes = append(restantes, fila)
		}
	}
	salida := make([]appGoals.GoalRow, 0, limit)
	for i := 0; i < len(restantes) && i < limit; i++ {
		salida = append(salida, restantes[i].fila)
	}
	if len(restantes) > limit {
		return salida, restantes[limit-1].sk, nil
	}
	return salida, "", nil
}

func e020PaginarNotificaciones(filas []e020NotifFila, after string, limit int) ([]appExport.NotificationRow, string, error) {
	restantes := make([]e020NotifFila, 0, len(filas))
	for _, fila := range filas {
		if fila.sk > after {
			restantes = append(restantes, fila)
		}
	}
	salida := make([]appExport.NotificationRow, 0, limit)
	for i := 0; i < len(restantes) && i < limit; i++ {
		salida = append(salida, restantes[i].fila)
	}
	if len(restantes) > limit {
		return salida, restantes[limit-1].sk, nil
	}
	return salida, "", nil
}

func e020PaginarRecordatorios(filas []e020RemFila, after string, limit int) ([]appExport.ReminderRow, string, error) {
	restantes := make([]e020RemFila, 0, len(filas))
	for _, fila := range filas {
		if fila.sk > after {
			restantes = append(restantes, fila)
		}
	}
	salida := make([]appExport.ReminderRow, 0, limit)
	for i := 0; i < len(restantes) && i < limit; i++ {
		salida = append(salida, restantes[i].fila)
	}
	if len(restantes) > limit {
		return salida, restantes[limit-1].sk, nil
	}
	return salida, "", nil
}

func e020PaginarDispositivos(filas []e020DevFila, after string, limit int) ([]appExport.PushDeviceRow, string, error) {
	restantes := make([]e020DevFila, 0, len(filas))
	for _, fila := range filas {
		if fila.sk > after {
			restantes = append(restantes, fila)
		}
	}
	salida := make([]appExport.PushDeviceRow, 0, limit)
	for i := 0; i < len(restantes) && i < limit; i++ {
		salida = append(salida, restantes[i].fila)
	}
	if len(restantes) > limit {
		return salida, restantes[limit-1].sk, nil
	}
	return salida, "", nil
}

func e020Servicio(f *e020Fuente) *appExport.Service {
	return appExport.NewService(f).
		WithClock(func() time.Time { return e020Ahora }).
		WithCursorSecret([]byte(e020Secreto))
}

func e020Perfil() *appProfile.Profile {
	return &appProfile.Profile{
		ID:            e020Usuario,
		Email:         "sintetico@example.invalid",
		DisplayName:   "Usuario Sintetico",
		EmailVerified: true,
		CreatedAt:     time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC),
		Version:       3,
	}
}

func e020Movimiento(id string, minuto int, centavos int64) e020Fila {
	ocurrido := time.Date(2026, 11, 10, 12, minuto, 0, 0, time.UTC)
	sk := "MOVEMENT#" + ocurrido.Format("2006-01-02T15:04:05.000Z") + "#" + id
	return e020Fila{
		sk: sk,
		movimiento: appMovements.Movement{
			ID:          id,
			Type:        "expense",
			AmountCents: centavos,
			Currency:    "MXN",
			CategoryID:  "food",
			Description: "Movimiento " + id,
			OccurredAt:  ocurrido,
			Version:     1,
			CreatedAt:   ocurrido,
		},
	}
}

func e020Consent(version string, otorgado bool) appPrivacy.ConsentRecord {
	return appPrivacy.ConsentRecord{
		SortKey:       "CONSENT#" + version,
		ID:            "consent-" + version,
		NoticeVersion: version,
		Purpose:       appPrivacy.ConsentPurposePrivacyNotice,
		Granted:       otorgado,
		RecordedAt:    time.Date(2026, 11, 2, 10, 0, 0, 0, time.UTC),
	}
}

// e020Coleccion siembra tres movimientos y dos metas en orden de SK.
func e020Coleccion() *e020Fuente {
	return &e020Fuente{
		movimientos: []e020Fila{
			e020Movimiento("mov-1", 1, 12345),
			e020Movimiento("mov-2", 2, 500),
			e020Movimiento("mov-3", 3, 99999),
		},
		metas: []e020MetaFila{
			{sk: "GOAL#meta-1", fila: e020Meta0("meta-1", 10000)},
			{sk: "GOAL#meta-2", fila: e020Meta0("meta-2", 25000)},
		},
		perfil:          e020Perfil(),
		consentimientos: []appPrivacy.ConsentRecord{e020Consent("2026-11-v2", true)},
	}
}

// e020Meta0 evita exigir *testing.T en los sembrados sin error esperado.
func e020Meta0(id string, guardadoCents int64) appGoals.GoalRow {
	meta, err := domainGoal.New(
		e020Usuario, id, "Meta "+id,
		guardadoCents+10000, guardadoCents,
		time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC),
		e020Ahora,
	)
	if err != nil {
		panic("meta de prueba invalida: " + err.Error())
	}
	return appGoals.GoalRow{Goal: meta, SortKey: "GOAL#" + id}
}

// TestE020PaginasYOrdenFijo comprueba el orden movements → goals, que la página respeta
// `limit`, que se recorre hasta agotar y que ningún elemento se repite ni se pierde.
func TestE020PaginasYOrdenFijo(t *testing.T) {
	servicio := e020Servicio(e020Coleccion())

	primera, err := servicio.Export(context.Background(), appExport.Query{UserID: e020Usuario, Limit: 2})
	if err != nil {
		t.Fatalf("primera pagina: %v", err)
	}
	if len(primera.Movements) != 2 || len(primera.Goals) != 0 {
		t.Fatalf("la primera pagina debe traer 2 movimientos y 0 metas: %d/%d", len(primera.Movements), len(primera.Goals))
	}
	if primera.Movements[0].Amount != "123.45" {
		t.Fatalf("el importe debe ser la cadena decimal exacta: %q", primera.Movements[0].Amount)
	}
	if primera.NextCursor == "" {
		t.Fatal("con datos pendientes debe haber nextCursor")
	}
	if primera.ExportedAt != e020Ahora.Format(time.RFC3339) {
		t.Fatalf("exportedAt inesperado: %q", primera.ExportedAt)
	}
	if len(primera.Consents) != 1 || primera.Consents[0].NoticeVersion != "2026-11-v2" {
		t.Fatalf("los consentimientos viajan completos en cada pagina: %+v", primera.Consents)
	}

	segunda, err := servicio.Export(context.Background(), appExport.Query{UserID: e020Usuario, Limit: 2, Cursor: primera.NextCursor})
	if err != nil {
		t.Fatalf("segunda pagina: %v", err)
	}
	if len(segunda.Movements) != 1 || len(segunda.Goals) != 1 {
		t.Fatalf("la segunda pagina debe traer 1 movimiento y 1 meta: %d/%d", len(segunda.Movements), len(segunda.Goals))
	}
	if segunda.NextCursor == "" {
		t.Fatal("queda una meta: debe haber nextCursor")
	}

	tercera, err := servicio.Export(context.Background(), appExport.Query{UserID: e020Usuario, Limit: 2, Cursor: segunda.NextCursor})
	if err != nil {
		t.Fatalf("tercera pagina: %v", err)
	}
	if len(tercera.Movements) != 0 || len(tercera.Goals) != 1 {
		t.Fatalf("la tercera pagina debe traer 0 movimientos y 1 meta: %d/%d", len(tercera.Movements), len(tercera.Goals))
	}
	if tercera.NextCursor != "" {
		t.Fatalf("agotadas las colecciones no debe haber cursor: %q", tercera.NextCursor)
	}

	vistos := map[string]bool{}
	for _, pagina := range []*appExport.Page{primera, segunda, tercera} {
		if pagina.ExportedAt != primera.ExportedAt {
			t.Fatalf("exportedAt debe ser el mismo en todas las paginas: %q vs %q", pagina.ExportedAt, primera.ExportedAt)
		}
		for _, movimiento := range pagina.Movements {
			if vistos[movimiento.ID] {
				t.Fatalf("movimiento repetido entre paginas: %s", movimiento.ID)
			}
			vistos[movimiento.ID] = true
		}
		for _, meta := range pagina.Goals {
			if vistos[meta.ID] {
				t.Fatalf("meta repetida entre paginas: %s", meta.ID)
			}
			vistos[meta.ID] = true
		}
	}
	for _, esperado := range []string{"mov-1", "mov-2", "mov-3", "meta-1", "meta-2"} {
		if !vistos[esperado] {
			t.Fatalf("falta %s en la exportacion", esperado)
		}
	}
	if len(vistos) != 5 {
		t.Fatalf("se esperaban 5 elementos distintos, se vieron %d", len(vistos))
	}
}

// TestE020SinPendientesNoEmiteCursor comprueba que las colecciones vacías se emiten como arrays
// (no `null`) y que no se inventa un cursor de continuación.
func TestE020SinPendientesNoEmiteCursor(t *testing.T) {
	fuente := &e020Fuente{perfil: e020Perfil()}
	pagina, err := e020Servicio(fuente).Export(context.Background(), appExport.Query{UserID: e020Usuario})
	if err != nil {
		t.Fatalf("la exportacion vacia no debe fallar: %v", err)
	}
	if pagina.NextCursor != "" {
		t.Fatalf("sin datos no debe haber cursor: %q", pagina.NextCursor)
	}
	if pagina.Movements == nil || pagina.Goals == nil || pagina.Consents == nil {
		t.Fatal("las colecciones deben serializarse como arrays vacios, no como null")
	}
	if len(pagina.Movements) != 0 || len(pagina.Goals) != 0 || len(pagina.Consents) != 0 {
		t.Fatalf("no debe haber filas: %+v", pagina)
	}
	if pagina.Profile.ID != e020Usuario {
		t.Fatalf("el perfil debe estar siempre presente: %+v", pagina.Profile)
	}
}

// TestE020LimitDelContrato comprueba 1..100 con defecto 25 y rechazo sin recorte silencioso.
func TestE020LimitDelContrato(t *testing.T) {
	casos := []struct {
		entrada  int
		esperado int
		valido   bool
	}{
		{0, appExport.DefaultLimit, true},
		{1, 1, true},
		{100, 100, true},
		{101, 0, false},
		{-1, 0, false},
	}
	for _, caso := range casos {
		obtenido, err := appExport.NormalizeLimit(caso.entrada)
		if caso.valido && (err != nil || obtenido != caso.esperado) {
			t.Fatalf("limit %d: esperado %d, obtenido %d (%v)", caso.entrada, caso.esperado, obtenido, err)
		}
		if !caso.valido && !errors.Is(err, appExport.ErrInvalidRequest) {
			t.Fatalf("limit %d debe ser ErrInvalidRequest, obtenido %v", caso.entrada, err)
		}
	}

	if _, err := e020Servicio(e020Coleccion()).Export(context.Background(), appExport.Query{
		UserID: e020Usuario, Limit: 101,
	}); !errors.Is(err, appExport.ErrInvalidRequest) {
		t.Fatalf("limit fuera de rango debe rechazarse: %v", err)
	}
}

// TestE020CursorLigadoAlUsuario comprueba que un cursor no sirve para otra cuenta.
func TestE020CursorLigadoAlUsuario(t *testing.T) {
	servicio := e020Servicio(e020Coleccion())
	primera, err := servicio.Export(context.Background(), appExport.Query{UserID: e020Usuario, Limit: 1})
	if err != nil || primera.NextCursor == "" {
		t.Fatalf("se necesita un cursor valido: %v", err)
	}
	if _, err := servicio.Export(context.Background(), appExport.Query{
		UserID: e020Otro, Limit: 1, Cursor: primera.NextCursor,
	}); !errors.Is(err, appExport.ErrInvalidRequest) {
		t.Fatalf("el cursor de otra cuenta debe rechazarse: %v", err)
	}
}

// TestE020CursorManipuladoYVencido comprueba firma y caducidad.
func TestE020CursorManipuladoYVencido(t *testing.T) {
	servicio := e020Servicio(e020Coleccion())
	primera, err := servicio.Export(context.Background(), appExport.Query{UserID: e020Usuario, Limit: 1})
	if err != nil || primera.NextCursor == "" {
		t.Fatalf("se necesita un cursor valido: %v", err)
	}

	alterado := []byte(primera.NextCursor)
	if alterado[5] == 'A' {
		alterado[5] = 'B'
	} else {
		alterado[5] = 'A'
	}
	if _, err := servicio.Export(context.Background(), appExport.Query{
		UserID: e020Usuario, Limit: 1, Cursor: string(alterado),
	}); !errors.Is(err, appExport.ErrInvalidRequest) {
		t.Fatalf("un cursor manipulado debe rechazarse: %v", err)
	}

	vencido := e020Servicio(e020Coleccion()).WithCursorTTL(-time.Minute)
	primeraVencida, err := vencido.Export(context.Background(), appExport.Query{UserID: e020Usuario, Limit: 1})
	if err != nil || primeraVencida.NextCursor == "" {
		t.Fatalf("se necesita un cursor vencido para la prueba: %v", err)
	}
	if _, err := vencido.Export(context.Background(), appExport.Query{
		UserID: e020Usuario, Limit: 1, Cursor: primeraVencida.NextCursor,
	}); !errors.Is(err, appExport.ErrInvalidRequest) {
		t.Fatalf("un cursor de mas de 2 h debe rechazarse: %v", err)
	}

	corto := "abc"
	if _, err := servicio.Export(context.Background(), appExport.Query{
		UserID: e020Usuario, Limit: 1, Cursor: corto,
	}); !errors.Is(err, appExport.ErrInvalidRequest) {
		t.Fatalf("un cursor fuera de rango debe rechazarse: %v", err)
	}
}

// TestE020CursorDeOtraColeccion comprueba que la SK del cursor debe pertenecer a la colección
// que declara y que el dominio de firma está aislado.
func TestE020CursorDeOtraColeccion(t *testing.T) {
	secreto := []byte(e020Secreto)
	unix := strconv.FormatInt(e020Ahora.Unix(), 10)

	casos := []struct {
		nombre string
		sortBy string
		lastID string
		valido bool
	}{
		{"sk de movimientos en coleccion goals", "e020:v1|goals|" + unix, "MOVEMENT#2026-11-10T12:01:00.000Z#mov-1", false},
		{"coleccion desconocida", "e020:v1|accounts|" + unix, "BUDGETSET#2026-11", false},
		{"dominio ajeno", "e008:v1|movements|" + unix, "MOVEMENT#2026-11-10T12:01:00.000Z#mov-1", false},
		{"instante invalido", "e020:v1|movements|0", "", false},
		{"inicio de la siguiente coleccion", "e020:v1|goals|" + unix, "", true},
	}
	for _, caso := range casos {
		cursor, err := httputil.GenerateCursor(secreto, e020Usuario, caso.sortBy, caso.lastID, e020Ahora, time.Hour)
		if err != nil {
			t.Fatalf("%s: no se pudo generar el cursor: %v", caso.nombre, err)
		}
		_, errExport := e020Servicio(e020Coleccion()).Export(context.Background(), appExport.Query{
			UserID: e020Usuario, Limit: 2, Cursor: cursor,
		})
		if caso.valido && errExport != nil {
			t.Fatalf("%s: el cursor deberia ser valido: %v", caso.nombre, errExport)
		}
		if !caso.valido && !errors.Is(errExport, appExport.ErrInvalidRequest) {
			t.Fatalf("%s: esperado ErrInvalidRequest, obtenido %v", caso.nombre, errExport)
		}
	}
}

// TestE020FailClosedSinSecretoNiFuente comprueba que no se emite ninguna página sin firma de
// cursor ni sin persistencia.
func TestE020FailClosedSinSecretoNiFuente(t *testing.T) {
	if _, err := appExport.NewService(e020Coleccion()).Export(context.Background(), appExport.Query{
		UserID: e020Usuario,
	}); !errors.Is(err, appExport.ErrUnavailable) {
		t.Fatalf("sin clave de cursor debe ser ErrUnavailable: %v", err)
	}
	if _, err := appExport.NewService(nil).WithCursorSecret([]byte(e020Secreto)).Export(context.Background(), appExport.Query{
		UserID: e020Usuario,
	}); !errors.Is(err, appExport.ErrUnavailable) {
		t.Fatalf("sin fuente debe ser ErrUnavailable: %v", err)
	}
	if _, err := e020Servicio(e020Coleccion()).Export(context.Background(), appExport.Query{
		UserID: "   ",
	}); !errors.Is(err, appExport.ErrUnauthorized) {
		t.Fatalf("sin identidad debe ser ErrUnauthorized: %v", err)
	}
}

// TestE020PerfilAusente comprueba el 404 sin inventar un perfil.
func TestE020PerfilAusente(t *testing.T) {
	fuente := e020Coleccion()
	fuente.perfil = nil
	if _, err := e020Servicio(fuente).Export(context.Background(), appExport.Query{
		UserID: e020Usuario,
	}); !errors.Is(err, appExport.ErrNotFound) {
		t.Fatalf("sin perfil debe ser ErrNotFound: %v", err)
	}
}

// TestE020FalloDeLecturaNoDevuelvePagina comprueba que un fallo recuperable se propaga como
// ErrUnavailable y que no existe página ni cursor parcial.
func TestE020FalloDeLecturaNoDevuelvePagina(t *testing.T) {
	fallo := appExport.ErrUnavailable
	casos := map[string]func(*e020Fuente){
		"movimientos":     func(f *e020Fuente) { f.errMovimientos = fallo },
		"metas":           func(f *e020Fuente) { f.errMetas = fallo },
		"perfil":          func(f *e020Fuente) { f.errPerfil = fallo },
		"consentimientos": func(f *e020Fuente) { f.errConsentimientos = fallo },
	}
	for nombre, romper := range casos {
		fuente := e020Coleccion()
		romper(fuente)
		pagina, err := e020Servicio(fuente).Export(context.Background(), appExport.Query{UserID: e020Usuario})
		if !errors.Is(err, appExport.ErrUnavailable) {
			t.Fatalf("%s: esperado ErrUnavailable, obtenido %v", nombre, err)
		}
		if pagina != nil {
			t.Fatalf("%s: un fallo no puede devolver pagina parcial: %+v", nombre, pagina)
		}
	}
}

// TestE020ConsentimientosCompletosEnCadaPagina comprueba el singleton de RFC-022 §2.2.
func TestE020ConsentimientosCompletosEnCadaPagina(t *testing.T) {
	fuente := e020Coleccion()
	fuente.consentimientos = []appPrivacy.ConsentRecord{
		e020Consent("2026-11-v1", false),
		e020Consent("2026-11-v2", true),
	}
	servicio := e020Servicio(fuente)

	primera, err := servicio.Export(context.Background(), appExport.Query{UserID: e020Usuario, Limit: 1})
	if err != nil {
		t.Fatalf("primera pagina: %v", err)
	}
	segunda, err := servicio.Export(context.Background(), appExport.Query{UserID: e020Usuario, Limit: 1, Cursor: primera.NextCursor})
	if err != nil {
		t.Fatalf("segunda pagina: %v", err)
	}
	if len(primera.Consents) != 2 || len(segunda.Consents) != 2 {
		t.Fatalf("cada pagina debe incluir los dos consentimientos: %d/%d", len(primera.Consents), len(segunda.Consents))
	}
	if primera.Consents[1].Granted != true || primera.Consents[0].RecordedAt == "" {
		t.Fatalf("proyeccion de consentimiento inesperada: %+v", primera.Consents)
	}
}

// TestE020MetaIncoherente comprueba que un item persistido inválido no se emite.
func TestE020MetaIncoherente(t *testing.T) {
	fuente := e020Coleccion()
	fuente.metas = []e020MetaFila{{
		sk:   "GOAL#meta-rota",
		fila: appGoals.GoalRow{Goal: domainGoal.Goal{ID: "meta-rota", UserID: e020Usuario}, SortKey: "GOAL#meta-rota"},
	}}
	if _, err := e020Servicio(fuente).Export(context.Background(), appExport.Query{
		UserID: e020Usuario,
	}); !errors.Is(err, appExport.ErrInconsistent) {
		t.Fatalf("una meta incoherente debe ser ErrInconsistent: %v", err)
	}
}

// TestE020ExportedAtViajaEnElCursor comprueba que la marca se fija en la primera página y no se
// recalcula en las siguientes (RFC-022 §2.3.1).
func TestE020ExportedAtViajaEnElCursor(t *testing.T) {
	reloj := e020Ahora
	fuente := e020Coleccion()
	servicio := appExport.NewService(fuente).
		WithCursorSecret([]byte(e020Secreto)).
		WithClock(func() time.Time { return reloj })

	primera, err := servicio.Export(context.Background(), appExport.Query{UserID: e020Usuario, Limit: 1})
	if err != nil || primera.NextCursor == "" {
		t.Fatalf("se necesita un cursor valido: %v", err)
	}
	reloj = e020Ahora.Add(3 * time.Hour)

	segunda, err := servicio.Export(context.Background(), appExport.Query{UserID: e020Usuario, Limit: 1, Cursor: primera.NextCursor})
	if err != nil {
		t.Fatalf("segunda pagina: %v", err)
	}
	if segunda.ExportedAt != e020Ahora.Format(time.RFC3339) {
		t.Fatalf("exportedAt debe conservarse desde el cursor: %q", segunda.ExportedAt)
	}
}

// TestE020TransversalExportaFamiliasYNoFiltraSecretos comprueba que las cuatro familias
// transversales viajan en la página y que el JSON resultante no contiene tokens, huellas,
// hashes, claves de idempotencia ni leases.
func TestE020TransversalExportaFamiliasYNoFiltraSecretos(t *testing.T) {
	fuente := e020Coleccion()
	fuente.notificaciones = []e020NotifFila{{
		sk: appExport.PrefijoNotificacion + "2026-11-20T10:00:00.000000001Z#n1",
		fila: appExport.NotificationRow{
			ID: "n1", Title: "Aviso", Message: "Mensaje", IsRead: true,
			CreatedAt: e020Ahora.Add(-2 * time.Hour),
		},
	}}
	fuente.recordatorios = []e020RemFila{{
		sk: appExport.PrefijoRecordatorio + "2026-11-25T09:00:00.000Z#r1",
		fila: appExport.ReminderRow{
			ID: "r1", Title: "Pagar renta", DueAt: e020Ahora.Add(24 * time.Hour),
			Timezone: "America/Mexico_City", Active: true,
			CreatedAt: e020Ahora.Add(-72 * time.Hour), UpdatedAt: e020Ahora.Add(-48 * time.Hour),
		},
	}}
	fuente.dispositivos = []e020DevFila{{
		sk: appExport.PrefijoDispositivo + "d1",
		fila: appExport.PushDeviceRow{
			ID: "d1", Provider: "fcm", DeviceLabel: "Pixel", RegisteredAt: e020Ahora.Add(-96 * time.Hour),
			Active: true,
		},
	}}
	fuente.prefsNotif = &appExport.NotificationPreferencesRow{
		PushEnabled: true, ReminderEnabled: true, BudgetAlerts: true, RecurringReminders: true,
		Version: 4, TimeZone: "America/Mexico_City",
		QuietHours: &appExport.QuietHoursRow{Enabled: true, Start: "22:00", End: "07:00"},
	}

	pagina, err := e020Servicio(fuente).Export(context.Background(), appExport.Query{
		UserID: e020Usuario, Limit: 100,
	})
	if err != nil {
		t.Fatalf("exportacion: %v", err)
	}
	if len(pagina.Notifications) != 1 || len(pagina.Reminders) != 1 || len(pagina.PushDevices) != 1 {
		t.Fatalf("familias transversales incompletas: %d/%d/%d",
			len(pagina.Notifications), len(pagina.Reminders), len(pagina.PushDevices))
	}
	if pagina.Notifications[0].ID != "n1" || !pagina.Notifications[0].IsRead {
		t.Fatalf("notificacion mal proyectada: %+v", pagina.Notifications[0])
	}
	if pagina.Reminders[0].Frequency != nil || pagina.Reminders[0].CreatedAt == "" || pagina.Reminders[0].UpdatedAt == "" {
		t.Fatalf("recordatorio mal proyectado: %+v", pagina.Reminders[0])
	}
	prefs := pagina.NotificationPreferences
	if prefs == nil || !prefs.PushEnabled || prefs.Version != 4 {
		t.Fatalf("preferencias de notificacion mal proyectadas: %+v", prefs)
	}
	if prefs.QuietHours == nil || prefs.QuietHours.Start != "22:00" {
		t.Fatalf("quietHours mal proyectado: %+v", prefs.QuietHours)
	}

	crudo, errJSON := json.Marshal(pagina)
	if errJSON != nil {
		t.Fatalf("serializacion: %v", errJSON)
	}
	texto := strings.ToLower(string(crudo))
	for _, prohibido := range []string{
		"token", "cipher", "fingerprint", "idem#", "passwordhash", "refreshhash",
		"leaseuntil", "skglobal", "schedulesptr", "outboxptr",
	} {
		if strings.Contains(texto, prohibido) {
			t.Fatalf("la exportacion no puede contener %q: %s", prohibido, texto)
		}
	}
	for _, presente := range []string{"notifications", "reminders", "pushdevices", "notificationpreferences"} {
		if !strings.Contains(texto, presente) {
			t.Fatalf("falta %q en la exportacion: %s", presente, texto)
		}
	}
}

// TestE020TransversalPaginacionHastaAgotar comprueba que las familias nuevas usan el mismo
// cursor firmado y que la última página no emite continuación.
func TestE020TransversalPaginacionHastaAgotar(t *testing.T) {
	fuente := &e020Fuente{perfil: e020Perfil()}
	fuente.notificaciones = []e020NotifFila{
		{sk: appExport.PrefijoNotificacion + "2026-11-20T10:00:00.000000001Z#n1", fila: appExport.NotificationRow{ID: "n1", CreatedAt: e020Ahora}},
		{sk: appExport.PrefijoNotificacion + "2026-11-20T10:00:00.000000002Z#n2", fila: appExport.NotificationRow{ID: "n2", CreatedAt: e020Ahora}},
	}
	servicio := e020Servicio(fuente)

	primera, err := servicio.Export(context.Background(), appExport.Query{UserID: e020Usuario, Limit: 1})
	if err != nil || len(primera.Notifications) != 1 || primera.NextCursor == "" {
		t.Fatalf("primera pagina: %+v err=%v", primera, err)
	}
	segunda, err := servicio.Export(context.Background(), appExport.Query{
		UserID: e020Usuario, Limit: 1, Cursor: primera.NextCursor,
	})
	if err != nil || len(segunda.Notifications) != 1 || segunda.Notifications[0].ID != "n2" {
		t.Fatalf("segunda pagina: %+v err=%v", segunda, err)
	}
	// La página se llenó exactamente en la última notificación: la continuación apunta a la
	// colección siguiente con posición vacía, no declara agotada la exportación.
	if segunda.NextCursor == "" {
		t.Fatal("quedan colecciones por recorrer: debe haber continuación")
	}
	tercera, err := servicio.Export(context.Background(), appExport.Query{
		UserID: e020Usuario, Limit: 1, Cursor: segunda.NextCursor,
	})
	if err != nil {
		t.Fatalf("tercera pagina: %v", err)
	}
	if len(tercera.Notifications) != 0 || tercera.NextCursor != "" {
		t.Fatalf("agotadas todas las colecciones no debe haber cursor: %+v", tercera)
	}
	// Un cursor de otro titular se sigue rechazando.
	if _, err := servicio.Export(context.Background(), appExport.Query{
		UserID: e020Otro, Limit: 1, Cursor: primera.NextCursor,
	}); !errors.Is(err, appExport.ErrInvalidRequest) {
		t.Fatalf("el cursor debe estar ligado al titular: %v", err)
	}
}

func (f *e020Fuente) Aportaciones(context.Context, string, string, int) ([]appExport.ContributionRow, string, error) {
	return nil, "", nil
}
func (f *e020Fuente) Recurrentes(context.Context, string, string, int) ([]appRecurring.Payment, string, error) {
	return nil, "", nil
}
func (f *e020Fuente) Presupuestos(context.Context, string, string, int) ([]appExport.BudgetRow, string, error) {
	return nil, "", nil
}
func (f *e020Fuente) Ingreso(context.Context, string) (*appIdentity.IncomeSnapshot, error) {
	return nil, nil
}
func (f *e020Fuente) Onboarding(context.Context, string) (*appOnboarding.State, error) {
	return nil, nil
}
func (f *e020Fuente) Preferencias(context.Context, string) (*appPreferences.Preferences, error) {
	return nil, nil
}

// Familias transversales: los dobles devuelven colecciones vacías salvo que la prueba las
// siembre (ver `notifications`, `reminders`, `devices` y `notificationPrefs` más abajo).
func (f *e020Fuente) Notificaciones(_ context.Context, userID, after string, limit int) ([]appExport.NotificationRow, string, error) {
	f.consultas = append(f.consultas, "notifications:"+userID+":"+after)
	if f.errNotificaciones != nil {
		return nil, "", f.errNotificaciones
	}
	return e020PaginarNotificaciones(f.notificaciones, after, limit)
}
func (f *e020Fuente) Recordatorios(_ context.Context, userID, after string, limit int) ([]appExport.ReminderRow, string, error) {
	f.consultas = append(f.consultas, "reminders:"+userID+":"+after)
	return e020PaginarRecordatorios(f.recordatorios, after, limit)
}
func (f *e020Fuente) Dispositivos(_ context.Context, userID, after string, limit int) ([]appExport.PushDeviceRow, string, error) {
	f.consultas = append(f.consultas, "devices:"+userID+":"+after)
	return e020PaginarDispositivos(f.dispositivos, after, limit)
}
func (f *e020Fuente) PreferenciasNotificacion(_ context.Context, userID string) (*appExport.NotificationPreferencesRow, error) {
	f.consultas = append(f.consultas, "notificationPrefs:"+userID)
	if f.errPreferenciasNotif != nil {
		return nil, f.errPreferenciasNotif
	}
	return f.prefsNotif, nil
}
