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
	"unicode/utf8"
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

	// ErrProfileCASFailed indica que la escritura condicional de E013 no se aplicó porque
	// el item cambió o desapareció entre la lectura y la escritura. Es un detalle interno
	// entre esta capa y el adaptador: el transporte nunca lo recibe; el servicio decide
	// entre 412, 404 y un reintento acotado.
	ErrProfileCASFailed = errors.New("escritura condicional de perfil no aplicada")
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

// MaxDisplayNameRunes es el límite superior del contrato `ProfilePatch.displayName`
// (maxLength: 100, contado en runes y no en bytes).
const MaxDisplayNameRunes = 100

// Patch es el cuerpo `ProfilePatch` ya tipado. Un puntero nulo significa «campo ausente»,
// que es distinto de la cadena vacía: `{}` no modifica nada y `{"displayName":""}` es una
// violación de dominio (422).
type Patch struct {
	DisplayName *string
}

// Empty informa de que la solicitud no modifica ningún campo.
func (p Patch) Empty() bool { return p.DisplayName == nil }

// UpdateInput describe la única escritura condicional (CAS) de E013. `ProfileHasVersion`
// distingue un perfil con `version` de uno heredado que E012 expone como v1: el adaptador
// emite entonces `attribute_not_exists(version)` e inicializa el contador.
type UpdateInput struct {
	UserID            string
	DisplayName       string
	ExpectedVersion   int64
	ProfileHasVersion bool
	Now               time.Time
}

// UpdateStore es el puerto de escritura de E013. Devuelve ErrProfileCASFailed cuando la
// condición de versión no se cumple y ErrProfileUnavailable ante un fallo de persistencia;
// nunca escribe de forma parcial ni crea el item si no existe.
type UpdateStore interface {
	UpdateProfile(ctx context.Context, input UpdateInput) (*Record, error)
}

// Service orquesta la lectura del perfil propio y, en E013, su modificación.
type Service struct {
	store   Store
	updates UpdateStore
	now     func() time.Time
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

// WithUpdateStore habilita la escritura de E013. Sin este puerto, `Update` falla cerrado con
// ErrProfileUnavailable: la lectura (E012) sigue funcionando y ninguna ruta escribe por
// accidente en una composición sin persistencia de escritura.
func (s *Service) WithUpdateStore(updates UpdateStore) *Service {
	if updates != nil {
		s.updates = updates
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

// Update aplica la modificación parcial de E013 (`PATCH /api/v1/users/me`).
//
// Secuencia: validar identidad y cuerpo → leer con consistencia fuerte → exigir la versión
// de `If-Match` → escribir con CAS sobre `USER#<id>/PROFILE` → proyectar los seis campos.
//
//   - Un `Patch` sin campos devuelve el perfil vigente sin escribir ni incrementar `version`.
//   - Una versión distinta de la persistida es 412 (ErrProfileVersionConflict).
//   - Si la condición CAS falla por una carrera, se relee el estado: si la versión ya no es
//     la esperada es 412, si el perfil desapareció es 404 y, si sigue siendo la misma, se
//     permite UN reintento; agotado el reintento es 409 (ErrProfileConflict).
//
// Ningún error incluye el correo, el nombre ni el identificador del usuario.
func (s *Service) Update(ctx context.Context, userID string, expectedVersion int64, patch Patch) (*Profile, error) {
	if s == nil || s.store == nil || s.updates == nil {
		return nil, ErrProfileUnavailable
	}
	limpio := strings.TrimSpace(userID)
	if limpio == "" {
		return nil, ErrProfileUnauthorized
	}
	if expectedVersion < 1 {
		return nil, ErrProfileInvalidRequest
	}
	nombre, err := normalizarDisplayName(patch.DisplayName)
	if err != nil {
		return nil, err
	}

	actual, err := s.cargarPerfil(ctx, limpio)
	if err != nil {
		return nil, err
	}
	if versionDe(actual) != expectedVersion {
		return nil, ErrProfileVersionConflict
	}
	if patch.DisplayName == nil {
		// `{}` no es una escritura: se responde el estado vigente con su versión intacta.
		return projectRecord(actual, limpio)
	}

	entrada := UpdateInput{
		UserID:            limpio,
		DisplayName:       nombre,
		ExpectedVersion:   expectedVersion,
		ProfileHasVersion: actual.HasVersion && actual.Version > 0,
		Now:               s.now(),
	}

	registro, err := s.updates.UpdateProfile(ctx, entrada)
	if errors.Is(err, ErrProfileCASFailed) {
		// Carrera entre la lectura y la escritura. Se decide con el estado vigente: si la
		// versión cambió, el cliente debe releer (412); si el perfil ya no existe, 404.
		fresco, errEstado := s.cargarConVersion(ctx, limpio, expectedVersion)
		if errEstado != nil {
			return nil, errEstado
		}
		entrada.ProfileHasVersion = fresco.HasVersion && fresco.Version > 0
		registro, err = s.updates.UpdateProfile(ctx, entrada)
		if errors.Is(err, ErrProfileCASFailed) {
			// Dos intentos con la misma versión esperada: no se insiste más.
			return nil, ErrProfileConflict
		}
	}
	switch {
	case err == nil:
	case errors.Is(err, ErrProfileNotFound):
		return nil, ErrProfileNotFound
	case errors.Is(err, ErrProfileInternal):
		return nil, ErrProfileInternal
	default:
		return nil, ErrProfileUnavailable
	}
	if registro == nil {
		return nil, ErrProfileInternal
	}
	return projectRecord(registro, limpio)
}

// cargarPerfil lee el perfil con consistencia fuerte y traduce la ausencia a 404.
func (s *Service) cargarPerfil(ctx context.Context, userID string) (*Record, error) {
	record, err := s.store.LoadProfile(ctx, userID)
	if err != nil {
		return nil, ErrProfileUnavailable
	}
	if record == nil {
		return nil, ErrProfileNotFound
	}
	return record, nil
}

// cargarConVersion relee el perfil y exige que la versión siga siendo la esperada.
func (s *Service) cargarConVersion(ctx context.Context, userID string, expected int64) (*Record, error) {
	record, err := s.cargarPerfil(ctx, userID)
	if err != nil {
		return nil, err
	}
	if versionDe(record) != expected {
		return nil, ErrProfileVersionConflict
	}
	return record, nil
}

// normalizarDisplayName valida `displayName` y devuelve el valor que se persiste.
//
// Sólo se recortan los espacios de los extremos: el valor almacenado nunca es vacío y se
// mide en runes (1..100), de modo que un nombre acentuado o con emoji no se rechaza por su
// tamaño en bytes. Un campo ausente no es un error: `{}` es una solicitud válida.
func normalizarDisplayName(valor *string) (string, error) {
	if valor == nil {
		return "", nil
	}
	limpio := strings.TrimSpace(*valor)
	if limpio == "" {
		return "", &FieldViolation{Field: "displayName", Reason: "required"}
	}
	if utf8.RuneCountInString(limpio) > MaxDisplayNameRunes {
		return "", &FieldViolation{Field: "displayName", Reason: "too_long"}
	}
	return limpio, nil
}

// versionDe aplica la misma regla que la proyección: un item heredado sin `version` se
// trata como v1, que es lo que el cliente recibió en `UserProfile` y enviará en `If-Match`.
func versionDe(record *Record) int64 {
	if record == nil || !record.HasVersion || record.Version < 1 {
		return 1
	}
	return record.Version
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
	version := versionDe(record)
	return &Profile{
		ID:            id,
		Email:         record.Email,
		DisplayName:   record.DisplayName,
		EmailVerified: record.EmailVerified,
		CreatedAt:     record.CreatedAt.UTC(),
		Version:       version,
	}, nil
}
