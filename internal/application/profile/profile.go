// Package profile implementa el caso de uso del perfil propio del usuario
// (E012 `GET /api/v1/users/me` y, en su extensión, E013 `PATCH /api/v1/users/me`).
//
// Invariantes de esta capa:
//   - La identidad llega exclusivamente de `sub` del JWT verificado: el cliente no puede
//     elegir el usuario por query, cuerpo ni cabecera (E012 §1, R-031).
//   - La lectura es un `GetItem` con consistencia fuerte sobre `USER#<id>/PROFILE`; nunca
//     se usa `Scan` ni se consulta la partición de otro usuario (E012 §4, A-060 §2.5).
//   - El DTO `UserProfile` se construye en la capa de transporte. Esta capa devuelve sólo
//     los seis valores tipados del contrato, sin atributos internos del item.
//   - Ningún error de esta capa contiene correo, hash, tokens ni identificadores de sesión:
//     los mensajes son genéricos y no enumerables.
package profile

import (
	"context"
	"errors"
	"strings"
	"time"
)

// Errores de aplicación de E012/E013. El transporte los traduce a los códigos del contrato
// sin revelar el motivo exacto al cliente.
var (
	// ErrProfileInvalidRequest cubre una solicitud mal formada en esta capa (400).
	ErrProfileInvalidRequest = errors.New("solicitud de perfil invalida")

	// ErrProfileUnauthorized indica que no hay sujeto autenticado (401). El middleware ya
	// lo impide; esta guarda es defensa en profundidad.
	ErrProfileUnauthorized = errors.New("perfil no autorizado")

	// ErrProfileNotFound indica que el perfil del sujeto no existe (404).
	ErrProfileNotFound = errors.New("perfil no encontrado")

	// ErrProfileVersionConflict indica que la versión enviada en `If-Match` no es la
	// vigente (412). El cliente debe releer el recurso.
	ErrProfileVersionConflict = errors.New("version de perfil desactualizada")

	// ErrProfileConflict indica que se agotó el reintento de una carrera de versiones
	// (409). No se aplicó ninguna escritura parcial.
	ErrProfileConflict = errors.New("conflicto al actualizar el perfil")

	// ErrProfileBusinessRule agrupa las violaciones de dominio (422); el detalle concreto
	// viaja en *FieldViolation.
	ErrProfileBusinessRule = errors.New("regla de negocio de perfil incumplida")

	// ErrProfileUnavailable indica falta de persistencia o un fallo de DynamoDB (503).
	// Nunca se degrada a una respuesta con datos inventados.
	ErrProfileUnavailable = errors.New("servicio de perfil no disponible")

	// ErrProfileInternal indica un item incoherente con el contrato (500). No se rellena
	// el hueco con valores ficticios: sin `createdAt` legible no hay respuesta válida.
	ErrProfileInternal = errors.New("perfil incoherente con el contrato")
)

// FieldViolation describe una violación de dominio con el campo que la origina. El
// transporte la convierte en `details[]` del sobre de error.
type FieldViolation struct {
	Field  string
	Reason string
}

// Error implementa error sin incluir el valor recibido (podría ser dato personal).
func (v *FieldViolation) Error() string {
	return "campo invalido: " + v.Field + " (" + v.Reason + ")"
}

// Unwrap permite clasificar la violación con errors.Is(err, ErrProfileBusinessRule).
func (v *FieldViolation) Unwrap() error { return ErrProfileBusinessRule }

// Profile es el valor de dominio que corresponde al esquema `UserProfile`: exactamente los
// seis campos del contrato, sin atributos internos del item.
type Profile struct {
	ID            string
	Email         string
	DisplayName   string
	EmailVerified bool
	CreatedAt     time.Time
	Version       int64
}

// Record es la fila leída de `USER#<id>/PROFILE`. Los indicadores `Has*` distinguen un
// atributo ausente de un valor cero heredado de un item antiguo.
type Record struct {
	UserID        string
	Email         string
	DisplayName   string
	EmailVerified bool
	CreatedAt     time.Time
	HasCreatedAt  bool
	Version       int64
	HasVersion    bool
}

// Store es el puerto de persistencia de E012. El adaptador real es DynamoDB; en pruebas se
// sustituye por un doble con estado. Devuelve (nil, nil) cuando el perfil no existe.
type Store interface {
	LoadProfile(ctx context.Context, userID string) (*Record, error)
}

// Service orquesta la lectura del perfil propio.
type Service struct {
	store Store
	now   func() time.Time
}

// NewService construye el servicio. Sin almacén todas las llamadas fallan cerrado con
// ErrProfileUnavailable: no se inventa un perfil.
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

// Get devuelve el perfil del sujeto autenticado (E012).
//
// Secuencia: validar identidad → leer con consistencia fuerte → proyectar los seis campos
// del contrato. Un perfil inexistente responde 404; un fallo de persistencia, 503.
func (s *Service) Get(ctx context.Context, userID string) (*Profile, error) {
	if s == nil || s.store == nil {
		return nil, ErrProfileUnavailable
	}
	limpio := strings.TrimSpace(userID)
	if limpio == "" {
		return nil, ErrProfileUnauthorized
	}

	record, err := s.store.LoadProfile(ctx, limpio)
	if err != nil {
		return nil, ErrProfileUnavailable
	}
	if record == nil {
		return nil, ErrProfileNotFound
	}
	return projectRecord(record, limpio)
}

// projectRecord reduce la fila persistida al valor de contrato. Un item sin `createdAt`
// legible no puede satisfacer `UserProfile` (campo requerido): se informa como incoherente
// en lugar de emitir una fecha cero.
func projectRecord(record *Record, userID string) (*Profile, error) {
	if record == nil {
		return nil, ErrProfileNotFound
	}
	if !record.HasCreatedAt || record.CreatedAt.IsZero() {
		return nil, ErrProfileInternal
	}
	id := strings.TrimSpace(record.UserID)
	if id == "" {
		id = userID
	}
	version := record.Version
	if !record.HasVersion || version < 1 {
		// Los items escritos antes de que existiera `version` se exponen como v1; la
		// actualización de E013 reconoce esa forma y la inicializa con CAS propio.
		version = 1
	}
	return &Profile{
		ID:            id,
		Email:         record.Email,
		DisplayName:   record.DisplayName,
		EmailVerified: record.EmailVerified,
		CreatedAt:     record.CreatedAt.UTC(),
		Version:       version,
	}, nil
}
