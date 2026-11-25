package transport_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
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
	domainIdentity "github.com/nexumpr/Nexum-Backend/internal/domain/identity"
	"github.com/nexumpr/Nexum-Backend/internal/ports"
	internalhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
	"github.com/nexumpr/Nexum-Backend/pkg/httputil"
)

// Pruebas HTTP de E020 (`GET /api/v1/users/me/export`, RFC-022 §2): autenticación con sesión
// autoritativa, `Cache-Control` de datos privados, validación de `limit` y `cursor`, envelope
// de error sin `nextCursor` y JSON con las propiedades exactas de `UserExport`.
//
// Los dobles son exclusivamente de prueba; la composición productiva usa DynamoDB y Redis
// reales y falla cerrado (503) cuando falta cualquiera de sus dependencias.

const (
	e020HTTPRuta    = "/api/v1/users/me/export"
	e020HTTPSecreto = "secreto-de-pruebas-e020-http-con-entropia-suficiente"
	e020HTTPSortKey = "e020:v1|movements|"
)

var e020HTTPAhora = time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)

// e020HTTPFila conserva la SK física junto al movimiento: el cursor se construye con
// posiciones reales, no con índices.
type e020HTTPFila struct {
	sk         string
	movimiento appMovements.Movement
}

// e020HTTPFuente implementa `export.Fuente` con paginación real por SK.
type e020HTTPFuente struct {
	mu              sync.Mutex
	perfil          *appProfile.Profile
	movimientos     []e020HTTPFila
	consentimientos []appPrivacy.ConsentRecord
	ingreso         *appIdentity.IncomeSnapshot
	onboarding      *appOnboarding.State
	preferencias    *appPreferences.Preferences
	metasRotas      bool

	// err, si no es nil, hace fallar todas las lecturas (dependencia caída).
	err error

	llamadas  int
	limites   []int
	consultas []string
}

func (f *e020HTTPFuente) registrar(nombre string, limit int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.llamadas++
	f.limites = append(f.limites, limit)
	f.consultas = append(f.consultas, nombre)
}

func (f *e020HTTPFuente) Movimientos(_ context.Context, userID, after string, limit int) ([]appMovements.Movement, string, error) {
	f.registrar("movements:"+userID+":"+after, limit)
	if f.err != nil {
		return nil, "", f.err
	}
	restantes := make([]e020HTTPFila, 0, len(f.movimientos))
	for _, fila := range f.movimientos {
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

func (f *e020HTTPFuente) Aportaciones(_ context.Context, userID, after string, limit int) ([]appExport.ContributionRow, string, error) {
	f.registrar("contributions:"+userID+":"+after, limit)
	if f.err != nil {
		return nil, "", f.err
	}
	return []appExport.ContributionRow{}, "", nil
}

func (f *e020HTTPFuente) Recurrentes(_ context.Context, userID, after string, limit int) ([]appRecurring.Payment, string, error) {
	f.registrar("recurringPayments:"+userID+":"+after, limit)
	if f.err != nil {
		return nil, "", f.err
	}
	return nil, "", nil
}

func (f *e020HTTPFuente) Metas(_ context.Context, userID, after string, limit int) ([]appGoals.GoalRow, string, error) {
	f.registrar("goals:"+userID+":"+after, limit)
	if f.err != nil {
		return nil, "", f.err
	}
	if f.metasRotas {
		// Meta sin datos: `appGoals.ContractOf` la rechaza y el servicio responde 500.
		return []appGoals.GoalRow{{}}, "", nil
	}
	return nil, "", nil
}

func (f *e020HTTPFuente) Presupuestos(_ context.Context, userID, after string, limit int) ([]appExport.BudgetRow, string, error) {
	f.registrar("budgets:"+userID+":"+after, limit)
	if f.err != nil {
		return nil, "", f.err
	}
	return nil, "", nil
}

func (f *e020HTTPFuente) Perfil(_ context.Context, userID string) (*appProfile.Profile, error) {
	f.registrar("profile:"+userID, 0)
	if f.err != nil {
		return nil, f.err
	}
	return f.perfil, nil
}

func (f *e020HTTPFuente) Ingreso(_ context.Context, userID string) (*appIdentity.IncomeSnapshot, error) {
	f.registrar("income:"+userID, 0)
	if f.err != nil {
		return nil, f.err
	}
	return f.ingreso, nil
}

func (f *e020HTTPFuente) Consentimientos(_ context.Context, userID string) ([]appPrivacy.ConsentRecord, error) {
	f.registrar("consents:"+userID, 0)
	if f.err != nil {
		return nil, f.err
	}
	return f.consentimientos, nil
}

func (f *e020HTTPFuente) Onboarding(_ context.Context, userID string) (*appOnboarding.State, error) {
	f.registrar("onboarding:"+userID, 0)
	if f.err != nil {
		return nil, f.err
	}
	return f.onboarding, nil
}

func (f *e020HTTPFuente) Preferencias(_ context.Context, userID string) (*appPreferences.Preferences, error) {
	f.registrar("preferences:"+userID, 0)
	if f.err != nil {
		return nil, f.err
	}
	return f.preferencias, nil
}

func (f *e020HTTPFuente) Notificaciones(_ context.Context, userID, after string, limit int) ([]appExport.NotificationRow, string, error) {
	f.registrar("notifications:"+userID, limit)
	if f.err != nil {
		return nil, "", f.err
	}
	return nil, "", nil
}

func (f *e020HTTPFuente) Recordatorios(_ context.Context, userID, after string, limit int) ([]appExport.ReminderRow, string, error) {
	f.registrar("reminders:"+userID, limit)
	if f.err != nil {
		return nil, "", f.err
	}
	return nil, "", nil
}

func (f *e020HTTPFuente) Dispositivos(_ context.Context, userID, after string, limit int) ([]appExport.PushDeviceRow, string, error) {
	f.registrar("devices:"+userID, limit)
	if f.err != nil {
		return nil, "", f.err
	}
	return nil, "", nil
}

func (f *e020HTTPFuente) PreferenciasNotificacion(_ context.Context, userID string) (*appExport.NotificationPreferencesRow, error) {
	f.registrar("notificationPrefs:"+userID, 0)
	if f.err != nil {
		return nil, f.err
	}
	return nil, nil
}

func (f *e020HTTPFuente) llamadasRealizadas() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.llamadas
}

func (f *e020HTTPFuente) limitesPedidos() []int {
	f.mu.Lock()
	defer f.mu.Unlock()
	copia := make([]int, len(f.limites))
	copy(copia, f.limites)
	return copia
}

// primerLimiteDeColeccion devuelve el primer `limit` > 0 registrado. Las lecturas singleton
// (profile, income, consents, onboarding, preferences) se registran con 0 porque no paginan.
func (f *e020HTTPFuente) primerLimiteDeColeccion() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, limite := range f.limites {
		if limite > 0 {
			return limite
		}
	}
	return 0
}

func e020HTTPPerfil() *appProfile.Profile {
	return &appProfile.Profile{
		ID:            e004HTTPUsuario,
		Email:         "sintetico@example.invalid",
		DisplayName:   "Usuario Sintetico",
		EmailVerified: true,
		CreatedAt:     time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC),
		Version:       3,
	}
}

func e020HTTPMovimiento(id string, minuto int, centavos int64) e020HTTPFila {
	ocurrido := time.Date(2026, 11, 10, 12, minuto, 0, 0, time.UTC)
	return e020HTTPFila{
		sk: "MOVEMENT#" + ocurrido.Format("2006-01-02T15:04:05.000Z") + "#" + id,
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

// e020HTTPFuenteCompleta siembra todas las familias para comprobar el JSON completo.
func e020HTTPFuenteCompleta() *e020HTTPFuente {
	completado := time.Date(2026, 11, 5, 8, 30, 0, 0, time.UTC)
	return &e020HTTPFuente{
		perfil: e020HTTPPerfil(),
		movimientos: []e020HTTPFila{
			e020HTTPMovimiento("mov-1", 1, 12345),
			e020HTTPMovimiento("mov-2", 2, 500),
		},
		consentimientos: []appPrivacy.ConsentRecord{{
			SortKey:       "CONSENT#2026-11-v2",
			ID:            "consent-1",
			NoticeVersion: "2026-11-v2",
			Purpose:       appPrivacy.ConsentPurposePrivacyNotice,
			Granted:       true,
			RecordedAt:    time.Date(2026, 11, 2, 10, 0, 0, 0, time.UTC),
		}},
		ingreso: &appIdentity.IncomeSnapshot{
			AmountCents: 1234500,
			Frequency:   domainIdentity.FrequencyMonthly,
			Version:     2,
		},
		onboarding: &appOnboarding.State{
			CompletedSteps: []string{"welcome", "permissions"},
			Done:           true,
			Version:        1,
			Microphone:     true,
			Notifications:  false,
			Configured:     true,
			CompletedAt:    &completado,
		},
		preferencias: &appPreferences.Preferences{
			Locale:     "es-MX",
			Timezone:   "America/Mexico_City",
			Version:    3,
			HasVersion: true,
		},
	}
}

func e020HTTPServicio(fuente *e020HTTPFuente) *appExport.Service {
	return appExport.NewService(fuente).
		WithClock(func() time.Time { return e020HTTPAhora }).
		WithCursorSecret([]byte(e020HTTPSecreto))
}

func e020HTTPHandler(fuente *e020HTTPFuente, sesiones *e004Sesiones, tokens *e004Tokens) http.Handler {
	return internalhttp.NewExportHandler(e020HTTPServicio(fuente), tokens, sesiones, silencioso())
}

func e020HTTPPeticion(ruta, token string) *http.Request {
	return e004Peticion(http.MethodGet, ruta, token, "")
}

// e020HTTPCuerpo deserializa el envelope/objeto de respuesta.
func e020HTTPCuerpo(t *testing.T, rec *httptest.ResponseRecorder) map[string]json.RawMessage {
	t.Helper()
	var cuerpo map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &cuerpo); err != nil {
		t.Fatalf("respuesta no es JSON valido: %v (%q)", err, rec.Body.String())
	}
	return cuerpo
}

// TestE020HTTP200Autenticado cubre el caso feliz: 200 con todas las propiedades de `UserExport`,
// cabecera de caché privada y ausencia de datos internos o secretos.
func TestE020HTTP200Autenticado(t *testing.T) {
	fuente := e020HTTPFuenteCompleta()
	handler := e020HTTPHandler(fuente, newE004Sesiones(), e004TokensValidos())

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, e020HTTPPeticion(e020HTTPRuta, "jwt-valido"))

	if rec.Code != http.StatusOK {
		t.Fatalf("esperado 200, obtenido %d (%s)", rec.Code, rec.Body.String())
	}
	cache := rec.Header().Get("Cache-Control")
	if !strings.Contains(cache, "private") || !strings.Contains(cache, "no-store") {
		t.Fatalf("Cache-Control debe ser privada y no almacenable, obtenido %q", cache)
	}
	if tipo := rec.Header().Get("Content-Type"); !strings.Contains(tipo, "application/json") {
		t.Fatalf("Content-Type inesperado: %q", tipo)
	}

	cuerpo := e020HTTPCuerpo(t, rec)
	requeridas := []string{
		"profile", "exportedAt", "movements", "contributions", "recurringPayments",
		"goals", "budgets", "consents", "income", "onboarding", "preferences",
	}
	for _, nombre := range requeridas {
		if _, ok := cuerpo[nombre]; !ok {
			t.Fatalf("falta la propiedad %q de UserExport: %s", nombre, rec.Body.String())
		}
	}
	if _, ok := cuerpo["nextCursor"]; ok {
		t.Fatalf("con las colecciones agotadas no debe haber nextCursor: %s", rec.Body.String())
	}
	if string(cuerpo["contributions"]) != "[]" {
		t.Fatalf("contributions debe serializarse como array vacio: %s", cuerpo["contributions"])
	}
	if string(cuerpo["recurringPayments"]) != "[]" || string(cuerpo["goals"]) != "[]" || string(cuerpo["budgets"]) != "[]" {
		t.Fatalf("las colecciones vacias deben ser arrays: %s", rec.Body.String())
	}

	var exportedAt string
	if err := json.Unmarshal(cuerpo["exportedAt"], &exportedAt); err != nil {
		t.Fatalf("exportedAt no es texto: %v", err)
	}
	if exportedAt != e020HTTPAhora.Format(time.RFC3339) {
		t.Fatalf("exportedAt inesperado: %q", exportedAt)
	}

	var perfil map[string]json.RawMessage
	if err := json.Unmarshal(cuerpo["profile"], &perfil); err != nil {
		t.Fatalf("profile no es objeto: %v", err)
	}
	clavesPerfil := []string{"id", "email", "displayName", "emailVerified", "createdAt", "version"}
	if len(perfil) != len(clavesPerfil) {
		t.Fatalf("profile debe tener exactamente %d campos, tiene %d: %s", len(clavesPerfil), len(perfil), cuerpo["profile"])
	}
	for _, clave := range clavesPerfil {
		if _, ok := perfil[clave]; !ok {
			t.Fatalf("falta profile.%s: %s", clave, cuerpo["profile"])
		}
	}

	var movimientos []map[string]json.RawMessage
	if err := json.Unmarshal(cuerpo["movements"], &movimientos); err != nil {
		t.Fatalf("movements no es un array: %v", err)
	}
	if len(movimientos) != 2 {
		t.Fatalf("se esperaban 2 movimientos, obtenidos %d", len(movimientos))
	}
	var importe string
	if err := json.Unmarshal(movimientos[0]["amount"], &importe); err != nil {
		t.Fatalf("amount debe ser una cadena decimal: %v", err)
	}
	if importe != "123.45" {
		t.Fatalf("importe inesperado: %q", importe)
	}
	if _, ok := movimientos[0]["userId"]; ok {
		t.Fatalf("movements no debe exponer atributos internos: %s", cuerpo["movements"])
	}

	var ingreso map[string]json.RawMessage
	if err := json.Unmarshal(cuerpo["income"], &ingreso); err != nil {
		t.Fatalf("income no es objeto: %v", err)
	}
	var importeIngreso string
	if err := json.Unmarshal(ingreso["amount"], &importeIngreso); err != nil || importeIngreso != "12345.00" {
		t.Fatalf("income.amount inesperado: %q (%v)", importeIngreso, err)
	}
	if string(cuerpo["onboarding"]) == "null" || string(cuerpo["preferences"]) == "null" {
		t.Fatalf("con datos, onboarding y preferences no deben ser null: %s", rec.Body.String())
	}

	completo := rec.Body.String()
	for _, prohibido := range []string{
		"jwt-valido", e004HTTPSesion, e020HTTPSecreto, "passwordHash", "sessionEpoch",
		"refreshHash", "REFRESH#", "IDEM#", "QUEUE#", "RESET#", "ONE_USE#",
	} {
		if strings.Contains(completo, prohibido) {
			t.Fatalf("la respuesta no debe contener %q: %s", prohibido, completo)
		}
	}
}

// TestE020HTTPPaginacion comprueba que `nextCursor` es opaco, que la página siguiente continúa
// sin repetir ni perder filas y que el defecto de `limit` es 25.
func TestE020HTTPPaginacion(t *testing.T) {
	fuente := e020HTTPFuenteCompleta()
	fuente.movimientos = []e020HTTPFila{
		e020HTTPMovimiento("mov-1", 1, 100),
		e020HTTPMovimiento("mov-2", 2, 200),
		e020HTTPMovimiento("mov-3", 3, 300),
	}
	handler := e020HTTPHandler(fuente, newE004Sesiones(), e004TokensValidos())

	primera := httptest.NewRecorder()
	handler.ServeHTTP(primera, e020HTTPPeticion(e020HTTPRuta+"?limit=2", "jwt-valido"))
	if primera.Code != http.StatusOK {
		t.Fatalf("primera pagina: %d (%s)", primera.Code, primera.Body.String())
	}
	cuerpoPrimera := e020HTTPCuerpo(t, primera)
	var cursor string
	if err := json.Unmarshal(cuerpoPrimera["nextCursor"], &cursor); err != nil {
		t.Fatalf("con datos pendientes debe haber nextCursor: %s", primera.Body.String())
	}
	if cursor == "" || len(cursor) < appExport.CursorMinLength {
		t.Fatalf("cursor inesperado: %q", cursor)
	}
	if strings.Contains(cursor, "MOVEMENT#") || strings.Contains(cursor, e004HTTPUsuario) {
		t.Fatalf("el cursor debe ser opaco: %q", cursor)
	}

	segunda := httptest.NewRecorder()
	handler.ServeHTTP(segunda, e020HTTPPeticion(e020HTTPRuta+"?limit=2&cursor="+url.QueryEscape(cursor), "jwt-valido"))
	if segunda.Code != http.StatusOK {
		t.Fatalf("segunda pagina: %d (%s)", segunda.Code, segunda.Body.String())
	}
	cuerpoSegunda := e020HTTPCuerpo(t, segunda)
	if _, ok := cuerpoSegunda["nextCursor"]; ok {
		t.Fatalf("agotada la coleccion no debe haber nextCursor: %s", segunda.Body.String())
	}

	vistos := map[string]bool{}
	for _, cuerpo := range []map[string]json.RawMessage{cuerpoPrimera, cuerpoSegunda} {
		var movimientos []map[string]json.RawMessage
		if err := json.Unmarshal(cuerpo["movements"], &movimientos); err != nil {
			t.Fatalf("movements no es un array: %v", err)
		}
		for _, movimiento := range movimientos {
			var id string
			_ = json.Unmarshal(movimiento["id"], &id)
			if vistos[id] {
				t.Fatalf("movimiento repetido entre paginas: %s", id)
			}
			vistos[id] = true
		}
	}
	if len(vistos) != 3 {
		t.Fatalf("se esperaban 3 movimientos distintos, obtenidos %d", len(vistos))
	}

	// Sin `limit` se aplica el defecto del contrato (25).
	sinLimite := e020HTTPFuenteCompleta()
	e020HTTPHandler(sinLimite, newE004Sesiones(), e004TokensValidos()).
		ServeHTTP(httptest.NewRecorder(), e020HTTPPeticion(e020HTTPRuta, "jwt-valido"))
	limites := sinLimite.limitesPedidos()
	if len(limites) == 0 || sinLimite.primerLimiteDeColeccion() != appExport.DefaultLimit {
		t.Fatalf("sin limit la pagina debe pedir %d: %v", appExport.DefaultLimit, limites)
	}
}

// TestE020HTTPAutenticacion cubre 401 por JWT ausente, formato inválido, token desconocido,
// token expirado, token sin sesión y sesión revocada, más 503 si el estado no es determinable.
func TestE020HTTPAutenticacion(t *testing.T) {
	sesiones := newE004Sesiones()
	handler := e020HTTPHandler(e020HTTPFuenteCompleta(), sesiones, e004TokensValidos())

	casos := []struct {
		nombre   string
		cabecera string
	}{
		{"sin cabecera", ""},
		{"esquema basico", "Basic abc"},
		{"token desconocido", "Bearer desconocido"},
		{"token expirado", "Bearer jwt-expirado"},
		{"token sin sesion", "Bearer jwt-sin-sesion"},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			req := e020HTTPPeticion(e020HTTPRuta, "")
			if caso.cabecera != "" {
				req.Header.Set("Authorization", caso.cabecera)
			}
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("esperado 401, obtenido %d (%s)", rec.Code, rec.Body.String())
			}
			cuerpo := e020HTTPCuerpo(t, rec)
			if _, ok := cuerpo["error"]; !ok {
				t.Fatalf("debe devolver ErrorEnvelope: %s", rec.Body.String())
			}
			if cache := rec.Header().Get("Cache-Control"); !strings.Contains(cache, "no-store") {
				t.Fatalf("incluso los errores deben ser no-store: %q", cache)
			}
		})
	}

	t.Run("sesion revocada", func(t *testing.T) {
		revo := newE004Sesiones()
		revo.revocar(e004HTTPSesion)
		rec := httptest.NewRecorder()
		e020HTTPHandler(e020HTTPFuenteCompleta(), revo, e004TokensValidos()).
			ServeHTTP(rec, e020HTTPPeticion(e020HTTPRuta, "jwt-valido"))
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("una sesion revocada debe ser 401, obtenido %d", rec.Code)
		}
	})

	t.Run("estado de sesion indeterminable", func(t *testing.T) {
		roto := newE004Sesiones()
		roto.fallo = ports.ErrSessionUnavailable
		rec := httptest.NewRecorder()
		e020HTTPHandler(e020HTTPFuenteCompleta(), roto, e004TokensValidos()).
			ServeHTTP(rec, e020HTTPPeticion(e020HTTPRuta, "jwt-valido"))
		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("sin poder verificar la sesion debe ser 503, obtenido %d", rec.Code)
		}
	})
}

// TestE020HTTPLimitInvalido comprueba 1..100 y que un valor inválido no consulta la fuente.
func TestE020HTTPLimitInvalido(t *testing.T) {
	for _, limite := range []string{"0", "101", "-1", "abc", "1.5"} {
		t.Run("limit="+limite, func(t *testing.T) {
			fuente := e020HTTPFuenteCompleta()
			handler := e020HTTPHandler(fuente, newE004Sesiones(), e004TokensValidos())
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, e020HTTPPeticion(e020HTTPRuta+"?limit="+url.QueryEscape(limite), "jwt-valido"))
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("limit=%q debe ser 400, obtenido %d (%s)", limite, rec.Code, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), "INVALID_REQUEST") {
				t.Fatalf("codigo inesperado: %s", rec.Body.String())
			}
			if fuente.llamadasRealizadas() != 0 {
				t.Fatalf("una solicitud invalida no debe consultar la fuente (%d llamadas)", fuente.llamadasRealizadas())
			}
		})
	}

	for _, limite := range []string{"1", "100"} {
		t.Run("limit valido "+limite, func(t *testing.T) {
			rec := httptest.NewRecorder()
			e020HTTPHandler(e020HTTPFuenteCompleta(), newE004Sesiones(), e004TokensValidos()).
				ServeHTTP(rec, e020HTTPPeticion(e020HTTPRuta+"?limit="+limite, "jwt-valido"))
			if rec.Code != http.StatusOK {
				t.Fatalf("limit=%s debe aceptarse, obtenido %d (%s)", limite, rec.Code, rec.Body.String())
			}
		})
	}

	// Convencion del proyecto (E027, E008 y E020): un parametro presente pero en blanco se
	// trata como ausente y aplica el defecto; no se inventa un 400 por espacios.
	t.Run("limit en blanco aplica el defecto", func(t *testing.T) {
		fuente := e020HTTPFuenteCompleta()
		rec := httptest.NewRecorder()
		e020HTTPHandler(fuente, newE004Sesiones(), e004TokensValidos()).
			ServeHTTP(rec, e020HTTPPeticion(e020HTTPRuta+"?limit="+url.QueryEscape(" "), "jwt-valido"))
		if rec.Code != http.StatusOK {
			t.Fatalf("limit en blanco debe aplicar el defecto, obtenido %d (%s)", rec.Code, rec.Body.String())
		}
		if limites := fuente.limitesPedidos(); len(limites) == 0 || fuente.primerLimiteDeColeccion() != appExport.DefaultLimit {
			t.Fatalf("se esperaba DefaultLimit=%d como primer limite de coleccion, obtenido %v", appExport.DefaultLimit, limites)
		}
	})
}

// TestE020HTTPCursorInvalido cubre cursor corto, demasiado largo, manipulado, de otra cuenta,
// vencido, de otra colección y de otro dominio. Todos son 400 sin filtrar el cursor.
func TestE020HTTPCursorInvalido(t *testing.T) {
	secreto := []byte(e020HTTPSecreto)
	unix := strconv.FormatInt(e020HTTPAhora.Unix(), 10)
	skMovimiento := e020HTTPMovimiento("mov-1", 1, 100).sk

	valido, err := httputil.GenerateCursor(secreto, e004HTTPUsuario, e020HTTPSortKey+unix, skMovimiento, e020HTTPAhora, time.Hour)
	if err != nil {
		t.Fatalf("no se pudo generar el cursor: %v", err)
	}
	ajeno, err := httputil.GenerateCursor(secreto, e004HTTPOtroUser, e020HTTPSortKey+unix, skMovimiento, e020HTTPAhora, time.Hour)
	if err != nil {
		t.Fatalf("no se pudo generar el cursor ajeno: %v", err)
	}
	vencido, err := httputil.GenerateCursor(secreto, e004HTTPUsuario, e020HTTPSortKey+unix, skMovimiento, e020HTTPAhora.Add(-3*time.Hour), -time.Minute)
	if err != nil {
		t.Fatalf("no se pudo generar el cursor vencido: %v", err)
	}
	otraColeccion, err := httputil.GenerateCursor(secreto, e004HTTPUsuario, "e020:v1|goals|"+unix, skMovimiento, e020HTTPAhora, time.Hour)
	if err != nil {
		t.Fatalf("no se pudo generar el cursor de otra coleccion: %v", err)
	}
	otroDominio, err := httputil.GenerateCursor(secreto, e004HTTPUsuario, "e008:v1|movements|"+unix, skMovimiento, e020HTTPAhora, time.Hour)
	if err != nil {
		t.Fatalf("no se pudo generar el cursor de otro dominio: %v", err)
	}
	manipulado := []byte(valido)
	if manipulado[5] == 'A' {
		manipulado[5] = 'B'
	} else {
		manipulado[5] = 'A'
	}

	casos := []struct {
		nombre string
		token  string
		cursor string
	}{
		{"corto", "jwt-valido", "abc"},
		{"demasiado largo", "jwt-valido", strings.Repeat("A", appExport.CursorMaxLength+1)},
		{"manipulado", "jwt-valido", string(manipulado)},
		{"de otra cuenta", "jwt-sujeto-ajeno", valido},
		{"cursor de otra cuenta presentado por el titular", "jwt-valido", ajeno},
		{"vencido", "jwt-valido", vencido},
		{"de otra coleccion", "jwt-valido", otraColeccion},
		{"de otro dominio", "jwt-valido", otroDominio},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			fuente := e020HTTPFuenteCompleta()
			handler := e020HTTPHandler(fuente, newE004Sesiones(), e004TokensValidos())
			ruta := e020HTTPRuta + "?limit=2&cursor=" + url.QueryEscape(caso.cursor)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, e020HTTPPeticion(ruta, caso.token))
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("esperado 400, obtenido %d (%s)", rec.Code, rec.Body.String())
			}
			cuerpo := rec.Body.String()
			if !strings.Contains(cuerpo, "INVALID_REQUEST") {
				t.Fatalf("codigo inesperado: %s", cuerpo)
			}
			if strings.Contains(cuerpo, caso.cursor) {
				t.Fatalf("la respuesta no debe repetir el cursor: %s", cuerpo)
			}
			if strings.Contains(cuerpo, "nextCursor") {
				t.Fatalf("un 400 no puede sugerir continuacion: %s", cuerpo)
			}
			if fuente.llamadasRealizadas() != 0 {
				t.Fatalf("un cursor invalido no debe consultar la fuente (%d llamadas)", fuente.llamadasRealizadas())
			}
		})
	}

	t.Run("cursor valido del titular", func(t *testing.T) {
		rec := httptest.NewRecorder()
		e020HTTPHandler(e020HTTPFuenteCompleta(), newE004Sesiones(), e004TokensValidos()).
			ServeHTTP(rec, e020HTTPPeticion(e020HTTPRuta+"?limit=2&cursor="+url.QueryEscape(valido), "jwt-valido"))
		if rec.Code != http.StatusOK {
			t.Fatalf("un cursor del titular debe aceptarse, obtenido %d (%s)", rec.Code, rec.Body.String())
		}
	})
}

// TestE020HTTP503SinFuente cubre el fail-closed de la composición: sin servicio, sin fuente, sin
// clave de cursor, sin tokens o sin autorizador de sesión el endpoint responde 503.
func TestE020HTTP503SinFuente(t *testing.T) {
	servicioCompleto := e020HTTPServicio(e020HTTPFuenteCompleta())
	servicioSinSecreto := appExport.NewService(e020HTTPFuenteCompleta())
	servicioSinFuente := appExport.NewService(nil).WithCursorSecret([]byte(e020HTTPSecreto))
	fuenteCaida := e020HTTPFuenteCompleta()
	// El contrato del puerto exige que el adaptador traduzca cualquier fallo de persistencia a
	// ErrUnavailable; el handler no conoce AWS. Un error no traducido es un fallo interno (500)
	// y se prueba en su propio caso.
	fuenteCaida.err = appExport.ErrUnavailable

	casos := []struct {
		nombre  string
		handler http.Handler
	}{
		{"sin servicio", internalhttp.NewExportHandler(nil, e004TokensValidos(), newE004Sesiones(), silencioso())},
		{"servicio sin fuente", internalhttp.NewExportHandler(servicioSinFuente, e004TokensValidos(), newE004Sesiones(), silencioso())},
		{"servicio sin clave de cursor", internalhttp.NewExportHandler(servicioSinSecreto, e004TokensValidos(), newE004Sesiones(), silencioso())},
		{"sin proveedor de tokens", internalhttp.NewExportHandler(servicioCompleto, nil, newE004Sesiones(), silencioso())},
		{"sin autorizador de sesion", internalhttp.NewExportHandler(servicioCompleto, e004TokensValidos(), nil, silencioso())},
		{"fuente caida", e020HTTPHandler(fuenteCaida, newE004Sesiones(), e004TokensValidos())},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			rec := httptest.NewRecorder()
			caso.handler.ServeHTTP(rec, e020HTTPPeticion(e020HTTPRuta, "jwt-valido"))
			if rec.Code != http.StatusServiceUnavailable {
				t.Fatalf("esperado 503, obtenido %d (%s)", rec.Code, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), "DEPENDENCY_UNAVAILABLE") {
				t.Fatalf("codigo inesperado: %s", rec.Body.String())
			}
			if strings.Contains(rec.Body.String(), "nextCursor") {
				t.Fatalf("un 503 no puede declarar el fin de la exportacion: %s", rec.Body.String())
			}
			if cache := rec.Header().Get("Cache-Control"); !strings.Contains(cache, "no-store") {
				t.Fatalf("los errores tambien deben ser no-store: %q", cache)
			}
		})
	}
}

// TestE020HTTPErroresDeAplicacion cubre 404, 500 y 400 con el envelope del contrato.
func TestE020HTTPErroresDeAplicacion(t *testing.T) {
	casos := []struct {
		nombre   string
		fuente   func() *e020HTTPFuente
		esperado int
		codigo   string
	}{
		{
			nombre: "perfil ausente",
			fuente: func() *e020HTTPFuente {
				fuente := e020HTTPFuenteCompleta()
				fuente.perfil = nil
				return fuente
			},
			esperado: http.StatusNotFound,
			codigo:   "NOT_FOUND",
		},
		{
			nombre: "item incoherente",
			fuente: func() *e020HTTPFuente {
				fuente := e020HTTPFuenteCompleta()
				fuente.metasRotas = true
				return fuente
			},
			esperado: http.StatusInternalServerError,
			codigo:   "INTERNAL_ERROR",
		},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			rec := httptest.NewRecorder()
			e020HTTPHandler(caso.fuente(), newE004Sesiones(), e004TokensValidos()).
				ServeHTTP(rec, e020HTTPPeticion(e020HTTPRuta, "jwt-valido"))
			if rec.Code != caso.esperado {
				t.Fatalf("esperado %d, obtenido %d (%s)", caso.esperado, rec.Code, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), caso.codigo) {
				t.Fatalf("codigo inesperado: %s", rec.Body.String())
			}
			for _, prohibido := range []string{e004HTTPSesion, e004HTTPUsuario, e020HTTPSecreto, "jwt-valido"} {
				if strings.Contains(rec.Body.String(), prohibido) {
					t.Fatalf("la respuesta no debe contener %q: %s", prohibido, rec.Body.String())
				}
			}
		})
	}
}

// TestE020HTTPNuncaEmiteCursorEnError comprueba de forma transversal que ningún error declara
// una continuación, de modo que un cliente no puede confundir un fallo con el final.
func TestE020HTTPNuncaEmiteCursorEnError(t *testing.T) {
	fuenteCaida := e020HTTPFuenteCompleta()
	fuenteCaida.err = appExport.ErrUnavailable
	handler := e020HTTPHandler(fuenteCaida, newE004Sesiones(), e004TokensValidos())

	for _, ruta := range []string{
		e020HTTPRuta,
		e020HTTPRuta + "?limit=0",
		e020HTTPRuta + "?cursor=abc",
	} {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, e020HTTPPeticion(ruta, "jwt-valido"))
		if rec.Code == http.StatusOK {
			t.Fatalf("la fuente esta caida: %s no puede responder 200", ruta)
		}
		if strings.Contains(rec.Body.String(), "nextCursor") {
			t.Fatalf("%s: un error no puede incluir nextCursor: %s", ruta, rec.Body.String())
		}
	}
}

// Compilación: los dobles satisfacen los puertos reales.
var (
	_ appExport.Fuente = (*e020HTTPFuente)(nil)
	_ http.Handler     = (*internalhttp.ExportHandler)(nil)
)
