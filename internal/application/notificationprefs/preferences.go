// Package notificationprefs contiene el caso de uso de las preferencias de
// notificación (E057 `GET /api/v1/notification-preferences` y E058 `PUT`).
//
// Modelo físico: item `USER#<userId> / NOTIFICATION_PREFERENCES` en la tabla
// única existente. Se usa una SK distinta de `PREFERENCES` (E068/E069, locale
// y zona del perfil) para que dos contratos distintos no compartan un mismo
// item ni se pisen sus atributos. No crea tablas ni índices y nunca usa `Scan`.
//
// Invariantes:
//   - El titular llega sólo desde el claim `sub` verificado por el middleware.
//   - Un item ausente se materializa con los defaults del contrato de forma
//     idempotente (creación condicional); dos lecturas concurrentes no crean
//     dos estados ni fallan la segunda.
//   - Ante un fallo real de persistencia se falla cerrado: nunca se responde con
//     valores inventados ni se devuelve un 200 con estado no persistido.
package notificationprefs

import (
	"context"
	"errors"
	"strings"
	"time"
)

// DefaultVersion es la versión del item recién materializado.
const DefaultVersion int64 = 1

// Errores de aplicación. El transporte los traduce a los códigos del contrato
// sin revelar el motivo exacto ni datos del titular.
var (
	ErrInvalidRequest  = errors.New("solicitud de preferencias de notificacion invalida")
	ErrUnauthorized    = errors.New("preferencias de notificacion no autorizadas")
	ErrVersionConflict = errors.New("version de preferencias de notificacion desactualizada")
	ErrConflict        = errors.New("conflicto al guardar preferencias de notificacion")
	ErrUnavailable     = errors.New("servicio de preferencias de notificacion no disponible")
)

// QuietHours es el bloque opcional `quietHours` del contrato. `Enabled` es el
// único campo requerido cuando el objeto está presente.
type QuietHours struct {
	Enabled bool
	Start   string
	End     string
}

// Preferences mapea 1:1 con el esquema `NotificationPreferences` del OpenAPI.
type Preferences struct {
	PushEnabled        bool
	EmailEnabled       bool
	ReminderEnabled    bool
	Version            int64
	BudgetAlerts       bool
	GoalUpdates        bool
	RecurringReminders bool
	StreakReminders    bool
	QuietHours         *QuietHours
	TimeZone           string
}

// WireQuietHours es la proyección JSON del bloque opcional.
type WireQuietHours struct {
	Enabled bool   `json:"enabled"`
	Start   string `json:"start,omitempty"`
	End     string `json:"end,omitempty"`
}

// WirePreferences es la proyección JSON estricta del esquema del contrato.
// Los campos opcionales `quietHours` y `timeZone` se omiten cuando no tienen
// valor, en lugar de emitir un objeto vacío o una zona inventada.
type WirePreferences struct {
	PushEnabled        bool            `json:"pushEnabled"`
	EmailEnabled       bool            `json:"emailEnabled"`
	ReminderEnabled    bool            `json:"reminderEnabled"`
	Version            int64           `json:"version"`
	BudgetAlerts       bool            `json:"budgetAlerts"`
	GoalUpdates        bool            `json:"goalUpdates"`
	RecurringReminders bool            `json:"recurringReminders"`
	StreakReminders    bool            `json:"streakReminders"`
	QuietHours         *WireQuietHours `json:"quietHours,omitempty"`
	TimeZone           string          `json:"timeZone,omitempty"`
}

// Wire convierte la entidad al contrato de transporte.
func (p *Preferences) Wire() WirePreferences {
	if p == nil {
		return WirePreferences{}
	}
	w := WirePreferences{
		PushEnabled:        p.PushEnabled,
		EmailEnabled:       p.EmailEnabled,
		ReminderEnabled:    p.ReminderEnabled,
		Version:            p.Version,
		BudgetAlerts:       p.BudgetAlerts,
		GoalUpdates:        p.GoalUpdates,
		RecurringReminders: p.RecurringReminders,
		StreakReminders:    p.StreakReminders,
		TimeZone:           p.TimeZone,
	}
	if p.QuietHours != nil {
		w.QuietHours = &WireQuietHours{Enabled: p.QuietHours.Enabled, Start: p.QuietHours.Start, End: p.QuietHours.End}
	}
	return w
}

// Defaults devuelve los valores admitidos por RFC-016 para un titular sin
// registro. `timeZone` no tiene default contractual: se omite hasta que E058
// lo establezca explícitamente.
func Defaults() Preferences {
	return Preferences{
		PushEnabled:        false,
		EmailEnabled:       false,
		ReminderEnabled:    true,
		Version:            DefaultVersion,
		BudgetAlerts:       true,
		GoalUpdates:        true,
		RecurringReminders: true,
		StreakReminders:    false,
	}
}

// Store es el puerto de persistencia. La implementación real es
// *dynamo.NotificationPreferencesStore; las pruebas usan dobles con estado.
type Store interface {
	Load(ctx context.Context, userID string) (*Preferences, error)
	Create(ctx context.Context, userID string, defaults Preferences, now time.Time) (*Preferences, error)
}

// Service orquesta la lectura y materialización de preferencias.
type Service struct {
	store Store
	now   func() time.Time
}

// NewService construye el servicio. Sin almacén, todas las llamadas fallan
// cerrado con ErrUnavailable: no se sirven defaults sin persistirlos.
func NewService(store Store) *Service {
	return &Service{store: store, now: func() time.Time { return time.Now().UTC() }}
}

// WithClock permite inyectar un reloj determinista en pruebas.
func (s *Service) WithClock(now func() time.Time) *Service {
	if s != nil && now != nil {
		s.now = now
	}
	return s
}

// Disponible indica si hay almacén real configurado.
func (s *Service) Disponible() bool { return s != nil && s.store != nil }

// Get devuelve las preferencias del titular. Si no existe, materializa los
// defaults con una creación condicional idempotente; si otra petición ganó la
// carrera, se devuelve el item ya creado.
func (s *Service) Get(ctx context.Context, userID string) (*Preferences, error) {
	if !s.Disponible() {
		return nil, ErrUnavailable
	}
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, ErrUnauthorized
	}

	actual, err := s.store.Load(ctx, userID)
	if err != nil {
		return nil, normalizeError(err)
	}
	if actual != nil {
		return normalize(actual), nil
	}

	creado, err := s.store.Create(ctx, userID, Defaults(), s.now().UTC())
	if err != nil {
		return nil, normalizeError(err)
	}
	if creado == nil {
		return nil, ErrUnavailable
	}
	return normalize(creado), nil
}

func normalize(p *Preferences) *Preferences {
	if p == nil {
		return nil
	}
	copia := *p
	if copia.Version < 1 {
		copia.Version = DefaultVersion
	}
	return &copia
}

func normalizeError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, ErrInvalidRequest), errors.Is(err, ErrUnauthorized),
		errors.Is(err, ErrVersionConflict), errors.Is(err, ErrConflict), errors.Is(err, ErrUnavailable):
		return err
	default:
		return ErrUnavailable
	}
}
