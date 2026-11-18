// Package preferences contiene el caso de uso de las preferencias personales de Nexum
// (E068 `GET /api/v1/users/me/preferences` y E069 `PATCH /api/v1/users/me/preferences`).
//
// Modelo físico (DISEÑO_OLEADA_1 §4): item `USER#<userId> / PREFERENCES` con `locale`,
// `timezone`, `version` y, tras la enmienda A8, el alias exacto `timeZone`. No crea
// particiones, tablas ni índices nuevos y nunca usa `Scan`.
//
// Invariantes de esta capa:
//   - El `userId` llega exclusivamente desde el claim `sub` de un JWT verificado; el cliente
//     no elige titular ni puede observar preferencias ajenas.
//   - Un item ausente se materializa con los valores por defecto del contrato de forma
//     idempotente: dos peticiones concurrentes no crean dos versiones ni fallan la segunda.
//   - La escritura es un compare-and-swap por `version`: una versión desactualizada no pierde
//     actualizaciones ni produce escrituras parciales.
//
// La validación de forma (enum de `locale`, zona IANA) vive en el transporte, que es donde el
// contrato OpenAPI define el 422 y donde el andamiaje expone `ValidarLocale`/`ValidarZonaIANA`.
package preferences

import (
	"context"
	"errors"
	"strings"
	"time"
)

// Valores por defecto del contrato. E068 los usa para materializar un item ausente; E069 los
// usa antes de comparar la versión enviada en `If-Match`.
const (
	// DefaultLocale es el idioma inicial de la cuenta.
	DefaultLocale = "es-MX"

	// DefaultTimezone es la zona horaria inicial de la cuenta.
	DefaultTimezone = "America/Mexico_City"

	// DefaultVersion es la versión del item recién creado.
	DefaultVersion int64 = 1

	// MinTimezoneLength y MaxTimezoneLength replican `minLength: 3` y `maxLength: 64` del
	// esquema `UserPreferences`/`UserPreferencesPatch` del OpenAPI.
	MinTimezoneLength = 3
	MaxTimezoneLength = 64
)

// Errores de aplicación de preferencias. El transporte los traduce a los códigos del contrato
// sin revelar el motivo exacto ni datos del titular.
var (
	// ErrPreferencesInvalidRequest cubre una solicitud mal formada en esta capa: falta el
	// titular o una modificación no trae ningún valor aplicable (400 INVALID_REQUEST).
	ErrPreferencesInvalidRequest = errors.New("solicitud de preferencias invalida")

	// ErrPreferencesUnauthorized indica que no hay titular identificado. El middleware ya lo
	// impide; la guarda es defensa en profundidad (401 UNAUTHORIZED).
	ErrPreferencesUnauthorized = errors.New("preferencias no autorizadas")

	// ErrPreferencesVersionConflict indica que `If-Match` no coincide con la versión vigente.
	// Ninguna escritura se aplicó (412 VERSION_CONFLICT).
	ErrPreferencesVersionConflict = errors.New("version de preferencias desactualizada")

	// ErrPreferencesConflict indica que una creación perezosa concurrente dejó el item en un
	// estado no observable de forma segura (409 CONFLICT).
	ErrPreferencesConflict = errors.New("conflicto al guardar preferencias")

	// ErrPreferencesUnavailable indica falta de persistencia o un fallo de DynamoDB. Falla
	// cerrado con 503: nunca se responde con valores inventados.
	ErrPreferencesUnavailable = errors.New("servicio de preferencias no disponible")
)

// FieldViolation describe una violación de regla de negocio con el campo responsable. El
// transporte la traduce a 422 BUSINESS_RULE_VIOLATION con `field` y `reason`.
type FieldViolation struct {
	Field  string
	Reason string
}

// Error implementa error sin incluir el valor rechazado (podría ser un dato del cliente).
func (v *FieldViolation) Error() string {
	if v == nil {
		return "violacion de regla"
	}
	return v.Field + ": " + v.Reason
}

// Preferences es la representación de aplicación de `UserPreferences`. `TimeZone` no se
// almacena como estado propio de dominio: la API lo emite como alias exacto de `Timezone`
// (A8) y el item conserva ambos atributos para que la lectura y la escritura coincidan.
type Preferences struct {
	Locale     string
	Timezone   string
	Version    int64
	HasVersion bool
}

// Defaults devuelve los valores con los que se materializa un item ausente.
func Defaults() Preferences {
	return Preferences{
		Locale:     DefaultLocale,
		Timezone:   DefaultTimezone,
		Version:    DefaultVersion,
		HasVersion: true,
	}
}

// Update es una modificación ya validada que se aplica con compare-and-swap. `Locale` y
// `Timezone` son los valores resultantes completos: el adaptador escribe siempre los tres
// atributos (`locale`, `timezone`, `timeZone`) para que el alias no pueda desalinearse.
type Update struct {
	Locale          string
	Timezone        string
	ExpectedVersion int64
	HasVersion      bool
	Now             time.Time
}

// ApplyCommand es la entrada de aplicación de E069. `UserID` proviene de `sub`; nunca del
// cuerpo, de la ruta ni de una cabecera libre. `HasLocale`/`HasTimezone` distinguen "ausente"
// de "presente" porque el PATCH es parcial.
type ApplyCommand struct {
	UserID      string
	Locale      string
	Timezone    string
	HasLocale   bool
	HasTimezone bool
	// ExpectedVersion es la versión exigida por `If-Match`; el transporte la valida antes.
	ExpectedVersion int64
}

// Store es el puerto de persistencia de E068. Devuelve (nil, nil) cuando el item no existe y
// ErrPreferencesUnavailable ante un fallo de servicio.
type Store interface {
	LoadPreferences(ctx context.Context, userID string) (*Preferences, error)
	CreatePreferences(ctx context.Context, userID string, defaults Preferences, now time.Time) (*Preferences, error)
}

// UpdateStore es la extensión de persistencia que necesita E069: escritura condicional por
// versión. Un item creado por E068 siempre tiene `version`, pero el adaptador tolera items
// históricos sin el atributo.
type UpdateStore interface {
	Store
	UpdatePreferences(ctx context.Context, userID string, update Update) (*Preferences, error)
}

// Service orquesta la lectura perezosa y la modificación de preferencias.
type Service struct {
	store Store
	now   func() time.Time
}

// NewService construye el servicio. Sin almacén, todas las llamadas fallan cerrado con
// ErrPreferencesUnavailable: no se sirven valores por defecto sin persistirlos.
func NewService(store Store) *Service {
	return &Service{
		store: store,
		now:   func() time.Time { return time.Now().UTC() },
	}
}

// WithClock permite inyectar un reloj determinista en pruebas.
func (s *Service) WithClock(now func() time.Time) *Service {
	if s != nil && now != nil {
		s.now = now
	}
	return s
}

// Disponible indica si el servicio puede atender: hay almacén real configurado.
func (s *Service) Disponible() bool {
	return s != nil && s.store != nil
}

// Get devuelve las preferencias del titular. Si el item no existe, materializa los valores por
// defecto con una creación condicional (`attribute_not_exists(PK)`) e idempotente: si otra
// petición ganó la carrera, se devuelve el item ya creado, no un error.
func (s *Service) Get(ctx context.Context, userID string) (*Preferences, error) {
	if !s.Disponible() {
		return nil, ErrPreferencesUnavailable
	}
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, ErrPreferencesUnauthorized
	}

	actual, err := s.store.LoadPreferences(ctx, userID)
	if err != nil {
		return nil, ErrPreferencesUnavailable
	}
	if actual != nil {
		return normalize(actual), nil
	}

	creado, err := s.store.CreatePreferences(ctx, userID, Defaults(), s.now().UTC())
	if err != nil {
		if errors.Is(err, ErrPreferencesUnavailable) || errors.Is(err, ErrPreferencesConflict) {
			return nil, err
		}
		return nil, ErrPreferencesUnavailable
	}
	if creado == nil {
		return nil, ErrPreferencesUnavailable
	}
	return normalize(creado), nil
}

// normalize garantiza que la respuesta cumple el contrato (`locale` y `timezone` no vacíos y
// `version >= 1`) ante un item histórico incoherente, sin inventar escrituras: la corrección
// es sólo de lectura y el siguiente PATCH persiste el estado con el CAS correspondiente.
func normalize(p *Preferences) *Preferences {
	if p == nil {
		return nil
	}
	copia := *p
	copia.Locale = strings.TrimSpace(copia.Locale)
	if copia.Locale == "" {
		copia.Locale = DefaultLocale
	}
	copia.Timezone = strings.TrimSpace(copia.Timezone)
	if copia.Timezone == "" {
		copia.Timezone = DefaultTimezone
	}
	if !copia.HasVersion || copia.Version < 1 {
		// `version` ausente o inválida: la versión observable es la inicial y no existe CAS
		// previo que pueda perderse (el adaptador exige `attribute_not_exists(#version)`).
		copia.Version = DefaultVersion
		copia.HasVersion = false
	}
	return &copia
}
