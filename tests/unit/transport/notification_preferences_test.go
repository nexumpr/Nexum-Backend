package transport_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	appPrefs "github.com/nexumpr/Nexum-Backend/internal/application/notificationprefs"
	internalhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
)

type e057HTTPStore struct {
	items     map[string]*appPrefs.Preferences
	err       error
	updateErr error
}

func (f *e057HTTPStore) Load(_ context.Context, userID string) (*appPrefs.Preferences, error) {
	if f.err != nil {
		return nil, f.err
	}
	if p := f.items[userID]; p != nil {
		copia := *p
		return &copia, nil
	}
	return nil, nil
}

func (f *e057HTTPStore) Create(_ context.Context, userID string, defaults appPrefs.Preferences, _ time.Time) (*appPrefs.Preferences, error) {
	if f.err != nil {
		return nil, f.err
	}
	if p := f.items[userID]; p != nil {
		copia := *p
		return &copia, nil
	}
	copia := defaults
	f.items[userID] = &copia
	return &copia, nil
}

func (f *e057HTTPStore) Update(_ context.Context, userID string, update appPrefs.Update) (*appPrefs.Preferences, error) {
	if f.updateErr != nil {
		return nil, f.updateErr
	}
	resultado := update.Value
	resultado.Version = update.ExpectedVersion + 1
	f.items[userID] = &resultado
	return &resultado, nil
}

func e057HTTPHandler(store appPrefs.Store, limitador internalhttp.LimitadorRuta) *internalhttp.NotificationPreferencesHandler {
	servicio := appPrefs.NewService(store)
	return internalhttp.NewNotificationPreferencesHandler(servicio, e004TokensValidos(), newE004Sesiones(), limitador, silencioso())
}

func e057HTTPStoreValido() *e057HTTPStore {
	return &e057HTTPStore{items: map[string]*appPrefs.Preferences{
		e004HTTPUsuario: {
			PushEnabled:        true,
			EmailEnabled:       false,
			ReminderEnabled:    true,
			Version:            3,
			BudgetAlerts:       false,
			GoalUpdates:        true,
			RecurringReminders: false,
			StreakReminders:    true,
			QuietHours:         &appPrefs.QuietHours{Enabled: true, Start: "23:00", End: "06:30"},
			TimeZone:           "America/Mexico_City",
		},
	}}
}

func TestE057HTTPContratoValido(t *testing.T) {
	rec := httptest.NewRecorder()
	e057HTTPHandler(e057HTTPStoreValido(), e053Limitador{permitido: true}).ServeHTTP(
		rec, e004Peticion(http.MethodGet, "/api/v1/notification-preferences", "jwt-valido", ""))

	if rec.Code != http.StatusOK {
		t.Fatalf("esperado 200, obtenido %d: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Cache-Control"); got != "private, no-store" {
		t.Fatalf("Cache-Control incorrecto: %q", got)
	}
	var cuerpo struct {
		PushEnabled        bool `json:"pushEnabled"`
		EmailEnabled       bool `json:"emailEnabled"`
		ReminderEnabled    bool `json:"reminderEnabled"`
		Version            int  `json:"version"`
		BudgetAlerts       bool `json:"budgetAlerts"`
		GoalUpdates        bool `json:"goalUpdates"`
		RecurringReminders bool `json:"recurringReminders"`
		StreakReminders    bool `json:"streakReminders"`
		QuietHours         *struct {
			Enabled bool   `json:"enabled"`
			Start   string `json:"start"`
			End     string `json:"end"`
		} `json:"quietHours"`
		TimeZone string `json:"timeZone"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &cuerpo); err != nil {
		t.Fatalf("JSON invalido: %v (%s)", err, rec.Body.String())
	}
	if !cuerpo.PushEnabled || cuerpo.EmailEnabled || !cuerpo.ReminderEnabled || cuerpo.Version != 3 ||
		cuerpo.BudgetAlerts || !cuerpo.GoalUpdates || cuerpo.RecurringReminders || !cuerpo.StreakReminders {
		t.Fatalf("cuerpo fuera de contrato: %+v", cuerpo)
	}
	if cuerpo.QuietHours == nil || !cuerpo.QuietHours.Enabled || cuerpo.QuietHours.Start != "23:00" || cuerpo.QuietHours.End != "06:30" {
		t.Fatalf("quietHours incorrecto: %+v", cuerpo.QuietHours)
	}
	if cuerpo.TimeZone != "America/Mexico_City" {
		t.Fatalf("timeZone incorrecto: %q", cuerpo.TimeZone)
	}
	if strings.Contains(rec.Body.String(), e004HTTPUsuario) {
		t.Fatal("la respuesta expone el identificador del titular")
	}
	// additionalProperties: false — ningún campo fuera del esquema.
	var generic map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &generic); err != nil {
		t.Fatal(err)
	}
	permitidos := map[string]bool{
		"pushEnabled": true, "emailEnabled": true, "reminderEnabled": true, "version": true,
		"budgetAlerts": true, "goalUpdates": true, "recurringReminders": true,
		"streakReminders": true, "quietHours": true, "timeZone": true,
	}
	for k := range generic {
		if !permitidos[k] {
			t.Fatalf("campo no contractual en la respuesta: %s", k)
		}
	}
}

func TestE057HTTPDefaultsYAislamiento(t *testing.T) {
	store := &e057HTTPStore{items: map[string]*appPrefs.Preferences{}}
	rec := httptest.NewRecorder()
	e057HTTPHandler(store, e053Limitador{permitido: true}).ServeHTTP(
		rec, e004Peticion(http.MethodGet, "/api/v1/notification-preferences", "jwt-sujeto-ajeno", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("usuario B: esperado 200, obtenido %d: %s", rec.Code, rec.Body.String())
	}
	var cuerpo struct {
		PushEnabled     bool `json:"pushEnabled"`
		Version         int  `json:"version"`
		StreakReminders bool `json:"streakReminders"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &cuerpo); err != nil {
		t.Fatal(err)
	}
	if cuerpo.PushEnabled || cuerpo.Version != 1 || cuerpo.StreakReminders {
		t.Fatalf("defaults incorrectos para B: %+v", cuerpo)
	}
	if store.items[e004HTTPOtroUser] == nil {
		t.Fatal("los defaults no se materializaron para B")
	}
}

func TestE057HTTPNegativas(t *testing.T) {
	casos := []struct {
		nombre    string
		metodo    string
		token     string
		limitador internalhttp.LimitadorRuta
		espera    int
		codigo    string
	}{
		{"sin_token", http.MethodGet, "", e053Limitador{permitido: true}, http.StatusUnauthorized, "UNAUTHORIZED"},
		{"metodo", http.MethodPost, "jwt-valido", e053Limitador{permitido: true}, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED"},
		{"cuota", http.MethodGet, "jwt-valido", e053Limitador{permitido: false}, http.StatusTooManyRequests, "RATE_LIMITED"},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			rec := httptest.NewRecorder()
			e057HTTPHandler(e057HTTPStoreValido(), caso.limitador).ServeHTTP(
				rec, e004Peticion(caso.metodo, "/api/v1/notification-preferences", caso.token, ""))
			if rec.Code != caso.espera {
				t.Fatalf("esperado %d, obtenido %d: %s", caso.espera, rec.Code, rec.Body.String())
			}
			if got := rec.Header().Get("Cache-Control"); got != "private, no-store" {
				t.Fatalf("Cache-Control ausente en error: %q", got)
			}
			var envelope struct {
				Error struct {
					Code string `json:"code"`
				} `json:"error"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil || envelope.Error.Code != caso.codigo {
				t.Fatalf("ErrorEnvelope incorrecto: %s", rec.Body.String())
			}
		})
	}
}

func TestE057HTTPFailClosed(t *testing.T) {
	sinServicio := internalhttp.NewNotificationPreferencesHandler(nil, e004TokensValidos(), newE004Sesiones(), nil, silencioso())
	rec := httptest.NewRecorder()
	sinServicio.ServeHTTP(rec, e004Peticion(http.MethodGet, "/api/v1/notification-preferences", "jwt-valido", ""))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("sin servicio: esperado 503, obtenido %d", rec.Code)
	}
	recCaido := httptest.NewRecorder()
	e057HTTPHandler(&e057HTTPStore{err: context.DeadlineExceeded}, nil).ServeHTTP(
		recCaido, e004Peticion(http.MethodGet, "/api/v1/notification-preferences", "jwt-valido", ""))
	if recCaido.Code != http.StatusServiceUnavailable {
		t.Fatalf("store caído: esperado 503, obtenido %d: %s", recCaido.Code, recCaido.Body.String())
	}
}

func TestE058HTTPActualizaContrato(t *testing.T) {
	store := e057HTTPStoreValido()
	rec := httptest.NewRecorder()
	cuerpo := `{"pushEnabled":false,"emailEnabled":true,"reminderEnabled":false,` +
		`"budgetAlerts":false,"streakReminders":true,` +
		`"quietHours":{"enabled":true,"start":"22:00","end":"06:30"},` +
		`"timeZone":"America/New_York"}`
	e057HTTPHandler(store, e053Limitador{permitido: true}).ServeHTTP(
		rec, e004Peticion(http.MethodPut, "/api/v1/notification-preferences", "jwt-valido", cuerpo))

	if rec.Code != http.StatusOK {
		t.Fatalf("esperado 200, obtenido %d: %s", rec.Code, rec.Body.String())
	}
	var respuesta struct {
		PushEnabled        bool `json:"pushEnabled"`
		EmailEnabled       bool `json:"emailEnabled"`
		ReminderEnabled    bool `json:"reminderEnabled"`
		Version            int  `json:"version"`
		BudgetAlerts       bool `json:"budgetAlerts"`
		GoalUpdates        bool `json:"goalUpdates"`
		RecurringReminders bool `json:"recurringReminders"`
		StreakReminders    bool `json:"streakReminders"`
		QuietHours         *struct {
			Enabled bool   `json:"enabled"`
			Start   string `json:"start"`
			End     string `json:"end"`
		} `json:"quietHours"`
		TimeZone string `json:"timeZone"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &respuesta); err != nil {
		t.Fatal(err)
	}
	if respuesta.PushEnabled || !respuesta.EmailEnabled || respuesta.ReminderEnabled || respuesta.Version != 4 ||
		respuesta.BudgetAlerts || !respuesta.GoalUpdates || !respuesta.RecurringReminders || !respuesta.StreakReminders {
		t.Fatalf("respuesta incorrecta: %+v", respuesta)
	}
	if respuesta.QuietHours == nil || !respuesta.QuietHours.Enabled || respuesta.QuietHours.Start != "22:00" || respuesta.TimeZone != "America/New_York" {
		t.Fatalf("quietHours/timeZone incorrectos: %+v", respuesta)
	}
	// Persistencia real del estado escrito.
	if guardado := store.items[e004HTTPUsuario]; guardado == nil || guardado.Version != 4 || guardado.TimeZone != "America/New_York" {
		t.Fatalf("no se persistió el estado: %+v", guardado)
	}
	// Relectura por GET: el mismo estado, sin incrementar versión.
	recLeida := httptest.NewRecorder()
	e057HTTPHandler(store, e053Limitador{permitido: true}).ServeHTTP(
		recLeida, e004Peticion(http.MethodGet, "/api/v1/notification-preferences", "jwt-valido", ""))
	if recLeida.Code != http.StatusOK || !strings.Contains(recLeida.Body.String(), `"version":4`) {
		t.Fatalf("relectura incorrecta: %d %s", recLeida.Code, recLeida.Body.String())
	}
}

func TestE058HTTPDefaultsOpcionales(t *testing.T) {
	store := &e057HTTPStore{items: map[string]*appPrefs.Preferences{}}
	rec := httptest.NewRecorder()
	e057HTTPHandler(store, e053Limitador{permitido: true}).ServeHTTP(
		rec, e004Peticion(http.MethodPut, "/api/v1/notification-preferences", "jwt-valido",
			`{"pushEnabled":true,"emailEnabled":false,"reminderEnabled":true}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("esperado 200, obtenido %d: %s", rec.Code, rec.Body.String())
	}
	var respuesta struct {
		BudgetAlerts       bool `json:"budgetAlerts"`
		GoalUpdates        bool `json:"goalUpdates"`
		RecurringReminders bool `json:"recurringReminders"`
		StreakReminders    bool `json:"streakReminders"`
		Version            int  `json:"version"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &respuesta); err != nil {
		t.Fatal(err)
	}
	if !respuesta.BudgetAlerts || !respuesta.GoalUpdates || !respuesta.RecurringReminders || respuesta.StreakReminders || respuesta.Version != 2 {
		t.Fatalf("defaults opcionales incorrectos: %+v", respuesta)
	}
}

func TestE058HTTPValidaciones422(t *testing.T) {
	casos := []struct {
		nombre string
		cuerpo string
		campo  string
	}{
		{"quietHours_sin_enabled", `{"pushEnabled":true,"emailEnabled":true,"reminderEnabled":true,"quietHours":{"start":"22:00","end":"06:00"}}`, "quietHours.enabled"},
		{"quietHours_formato", `{"pushEnabled":true,"emailEnabled":true,"reminderEnabled":true,"quietHours":{"enabled":true,"start":"25:00","end":"06:00"}}`, "quietHours"},
		{"quietHours_iguales", `{"pushEnabled":true,"emailEnabled":true,"reminderEnabled":true,"quietHours":{"enabled":true,"start":"22:00","end":"22:00"}}`, "quietHours"},
		{"timezone_invalida", `{"pushEnabled":true,"emailEnabled":true,"reminderEnabled":true,"timeZone":"No/Existe"}`, "timeZone"},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			rec := httptest.NewRecorder()
			e057HTTPHandler(e057HTTPStoreValido(), e053Limitador{permitido: true}).ServeHTTP(
				rec, e004Peticion(http.MethodPut, "/api/v1/notification-preferences", "jwt-valido", caso.cuerpo))
			if rec.Code != http.StatusUnprocessableEntity {
				t.Fatalf("esperado 422, obtenido %d: %s", rec.Code, rec.Body.String())
			}
			var envelope struct {
				Error struct {
					Code    string `json:"code"`
					Details []struct {
						Field string `json:"field"`
					} `json:"details"`
				} `json:"error"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil ||
				envelope.Error.Code != "BUSINESS_RULE_VIOLATION" || len(envelope.Error.Details) == 0 ||
				envelope.Error.Details[0].Field != caso.campo {
				t.Fatalf("422 incorrecto: %s", rec.Body.String())
			}
		})
	}
}

func TestE058HTTPCuerpoInvalido400(t *testing.T) {
	casos := []struct {
		nombre string
		cuerpo string
	}{
		{"falta_requerido", `{"pushEnabled":true,"emailEnabled":true}`},
		{"campo_desconocido", `{"pushEnabled":true,"emailEnabled":true,"reminderEnabled":true,"inexistente":1}`},
		{"json_malformado", `{"pushEnabled":`},
		{"tipo_incorrecto", `{"pushEnabled":"si","emailEnabled":true,"reminderEnabled":true}`},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			rec := httptest.NewRecorder()
			e057HTTPHandler(e057HTTPStoreValido(), e053Limitador{permitido: true}).ServeHTTP(
				rec, e004Peticion(http.MethodPut, "/api/v1/notification-preferences", "jwt-valido", caso.cuerpo))
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("esperado 400, obtenido %d: %s", rec.Code, rec.Body.String())
			}
			if got := rec.Header().Get("Cache-Control"); got != "private, no-store" {
				t.Fatalf("Cache-Control ausente: %q", got)
			}
		})
	}
}

func TestE058HTTPNegativasYAislamiento(t *testing.T) {
	sinToken := httptest.NewRecorder()
	e057HTTPHandler(e057HTTPStoreValido(), e053Limitador{permitido: true}).ServeHTTP(
		sinToken, e004Peticion(http.MethodPut, "/api/v1/notification-preferences", "", `{"pushEnabled":true,"emailEnabled":true,"reminderEnabled":true}`))
	if sinToken.Code != http.StatusUnauthorized {
		t.Fatalf("sin token: %d", sinToken.Code)
	}
	sinCuota := httptest.NewRecorder()
	e057HTTPHandler(e057HTTPStoreValido(), e053Limitador{permitido: false}).ServeHTTP(
		sinCuota, e004Peticion(http.MethodPut, "/api/v1/notification-preferences", "jwt-valido", `{"pushEnabled":true,"emailEnabled":true,"reminderEnabled":true}`))
	if sinCuota.Code != http.StatusTooManyRequests {
		t.Fatalf("sin cuota: %d", sinCuota.Code)
	}
	conflicto := httptest.NewRecorder()
	e057HTTPHandler(&e057HTTPStore{items: map[string]*appPrefs.Preferences{}, updateErr: appPrefs.ErrConflict}, e053Limitador{permitido: true}).ServeHTTP(
		conflicto, e004Peticion(http.MethodPut, "/api/v1/notification-preferences", "jwt-valido", `{"pushEnabled":true,"emailEnabled":true,"reminderEnabled":true}`))
	if conflicto.Code != http.StatusConflict {
		t.Fatalf("conflicto: %d %s", conflicto.Code, conflicto.Body.String())
	}

	store := e057HTTPStoreValido()
	recB := httptest.NewRecorder()
	e057HTTPHandler(store, nil).ServeHTTP(recB, e004Peticion(http.MethodPut, "/api/v1/notification-preferences", "jwt-sujeto-ajeno",
		`{"pushEnabled":true,"emailEnabled":true,"reminderEnabled":true}`))
	if recB.Code != http.StatusOK {
		t.Fatalf("usuario B: %d %s", recB.Code, recB.Body.String())
	}
	if a := store.items[e004HTTPUsuario]; a == nil || !a.PushEnabled || a.Version != 3 {
		t.Fatalf("la escritura de B modificó a A: %+v", a)
	}
}
