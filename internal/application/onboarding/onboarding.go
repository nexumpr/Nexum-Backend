// Package onboarding implementa el estado de onboarding del propio usuario
// (E018 `GET /api/v1/users/me/onboarding`).
//
// Reglas de esta capa:
//   - La identidad llega exclusivamente del claim `sub` verificado; el cliente
//     nunca elige el usuario.
//   - La lectura es un `GetItem` consistente de `USER#<id>/ONBOARDING`. No hay
//     `Scan` ni acceso a la partición de otro usuario.
//   - La proyección al contrato sólo emite los siete campos de `Onboarding`:
//     `completedSteps` (nunca nulo), `done`, `version`, `microphone`,
//     `notifications`, `configured` y `completedAt` (nullable explícito).
//   - Los permisos se persisten como texto (`granted|pending|denied`) desde
//     E001; la API los proyecta a booleano: `granted` → `true`, el resto →
//     `false`. No se reescribe el valor persistido al leer.
package onboarding

import (
	"context"
	"errors"
	"strings"
	"time"
)

// Pasos canónicos del onboarding. El orden de esta lista es el orden canónico
// de `completedSteps` en la respuesta y de la normalización de E019.
const (
	StepProfile     = "profile"
	StepIncome      = "income"
	StepPrivacy     = "privacy"
	StepPermissions = "permissions"
)

// canonicalSteps define el orden contractual. No se exporta el slice para que
// ningún llamador pueda mutarlo.
var canonicalSteps = [...]string{StepProfile, StepIncome, StepPrivacy, StepPermissions}

// Errores de aplicación. El transporte los traduce a códigos del contrato sin
// revelar el motivo exacto.
var (
	// ErrUnauthorized indica que la identidad autenticada no identifica usuario.
	// El middleware ya lo impide; la guarda es defensa en profundidad (401).
	ErrUnauthorized = errors.New("onboarding no autorizado")

	// ErrNotFound indica que el usuario no tiene estado de onboarding (404).
	ErrNotFound = errors.New("estado de onboarding no encontrado")

	// ErrUnavailable indica falta de persistencia o un fallo de DynamoDB. Falla
	// cerrado con 503: nunca se inventa un estado ni se asume vacío.
	ErrUnavailable = errors.New("servicio de onboarding no disponible")
)

// Record es la fila leída de `USER#<id>/ONBOARDING`, sin proyectar al contrato.
// Conserva los valores persistidos para poder preservarlos en una escritura
// posterior (E019) sin inventar permisos.
type Record struct {
	// CompletedSteps son los pasos persistidos (puede faltar el atributo).
	CompletedSteps []string

	// Version es la versión leída. HasVersion distingue `0` de un atributo
	// ausente en un ítem heredado.
	Version    int64
	HasVersion bool

	// MicrophoneRaw y NotificationsRaw conservan el texto persistido
	// (`granted|pending|denied`). Vacío significa atributo ausente.
	MicrophoneRaw    string
	NotificationsRaw string

	// Configured es el valor del atributo booleano homónimo.
	Configured bool

	// CompletedAt es la marca persistida, si existe.
	CompletedAt *time.Time
}

// State es la proyección de `Onboarding` que consume el transporte.
type State struct {
	CompletedSteps []string
	Done           bool
	Version        int64
	Microphone     bool
	Notifications  bool
	Configured     bool
	CompletedAt    *time.Time
}

// Store es el puerto de persistencia de E018. `found=false` significa que la
// clave no existe; el adaptador nunca sustituye la ausencia por un estado vacío.
type Store interface {
	GetOnboarding(ctx context.Context, userID string) (record *Record, found bool, err error)
}

// Service orquesta la lectura del estado de onboarding.
type Service struct {
	store Store
	now   func() time.Time
}

// NewService construye el servicio. Sin almacén, todas las llamadas fallan
// cerradas con ErrUnavailable: no se sirve un estado ficticio.
func NewService(store Store) *Service {
	return &Service{
		store: store,
		now:   func() time.Time { return time.Now().UTC() },
	}
}

// WithClock permite inyectar un reloj determinista en pruebas.
func (s *Service) WithClock(now func() time.Time) *Service {
	if now != nil {
		s.now = now
	}
	return s
}

// Get devuelve el estado de onboarding del titular.
func (s *Service) Get(ctx context.Context, userID string) (*State, error) {
	if s == nil || s.store == nil {
		return nil, ErrUnavailable
	}
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, ErrUnauthorized
	}

	record, found, err := s.store.GetOnboarding(ctx, userID)
	if err != nil {
		return nil, ErrUnavailable
	}
	if !found || record == nil {
		return nil, ErrNotFound
	}
	return stateFromRecord(record), nil
}

// CanonicalSteps devuelve sólo los pasos admitidos, sin duplicados y en el
// orden canónico. Nunca devuelve `nil`: un estado sin pasos se serializa `[]`.
// Los valores desconocidos de un ítem heredado se ignoran en la proyección,
// porque no forman parte del contrato y no pueden escribirse por E019.
func CanonicalSteps(raw []string) []string {
	vistos := make(map[string]struct{}, len(raw))
	for _, value := range raw {
		vistos[strings.ToLower(strings.TrimSpace(value))] = struct{}{}
	}
	steps := make([]string, 0, len(canonicalSteps))
	for _, step := range canonicalSteps {
		if _, ok := vistos[step]; ok {
			steps = append(steps, step)
		}
	}
	return steps
}

// IsComplete indica si están presentes los cuatro pasos canónicos.
func IsComplete(steps []string) bool {
	if len(steps) != len(canonicalSteps) {
		return false
	}
	presentes := make(map[string]struct{}, len(steps))
	for _, step := range steps {
		presentes[step] = struct{}{}
	}
	for _, step := range canonicalSteps {
		if _, ok := presentes[step]; !ok {
			return false
		}
	}
	return true
}

// PermissionGranted proyecta el texto persistido al booleano del contrato.
func PermissionGranted(raw string) bool {
	return strings.EqualFold(strings.TrimSpace(raw), "granted")
}

// stateFromRecord reduce la fila al contrato. `completedAt` sólo se emite
// cuando `done=true`: mientras el onboarding no está completo el contrato exige
// `null`, aunque exista una marca heredada.
func stateFromRecord(record *Record) *State {
	steps := CanonicalSteps(record.CompletedSteps)
	done := IsComplete(steps)

	state := &State{
		CompletedSteps: steps,
		Done:           done,
		Version:        record.Version,
		Microphone:     PermissionGranted(record.MicrophoneRaw),
		Notifications:  PermissionGranted(record.NotificationsRaw),
		Configured:     record.Configured,
	}
	if !record.HasVersion {
		// Ítem heredado sin atributo `version`: la versión lógica es 1 y la
		// primera escritura de E019 lo materializa con su CAS por ausencia.
		state.Version = 1
	}
	if done && record.CompletedAt != nil && !record.CompletedAt.IsZero() {
		marca := record.CompletedAt.UTC()
		state.CompletedAt = &marca
	}
	return state
}
