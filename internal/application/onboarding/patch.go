package onboarding

import (
	"context"
	"errors"
	"strings"
	"time"
)

// Escritura de E019 (`PATCH /api/v1/users/me/onboarding`).
//
// Reglas de esta capa:
//   - La identidad llega exclusivamente del claim `sub` verificado; el cliente
//     nunca elige el usuario y la escritura toca sólo `USER#<id>/ONBOARDING`.
//   - `completedSteps` es obligatorio en el contrato (el DTO de transporte lo
//     exige). Aquí se normaliza: sin duplicados, en orden canónico y rechazando
//     cualquier paso desconocido con un error de negocio (422).
//   - `microphone` y `notifications` se persisten como texto `granted|denied`
//     cuando el cliente los envía. Si no los envía, se conserva el valor
//     persistido (incluido un `pending` heredado de E001).
//   - `configured` es un booleano real; ausente conserva el valor persistido.
//   - El CAS es responsabilidad del almacén (`UpdateOnboarding` con la versión
//     leída y `ADD version :one`). El servicio reintenta UNA vez y, si el
//     conflicto persiste, devuelve ErrConflict (409): nunca sobrescribe una
//     escritura ajena ni inventa un estado.

// ErrConflict indica que la versión leída dejó de ser la vigente durante la
// escritura. El servicio reintenta una vez con una lectura nueva; si vuelve a
// ocurrir, el transporte responde 409.
var ErrConflict = errors.New("conflicto de version en onboarding")

// UnknownStepError identifica un paso de `completedSteps` fuera del catálogo
// cerrado del contrato. No incorpora el valor recibido en el mensaje: el
// transporte responde 422 sin reflejar datos del cliente.
type UnknownStepError struct {
	// Step es el valor normalizado que no pertenece al catálogo.
	Step string
}

func (e *UnknownStepError) Error() string { return "paso de onboarding no reconocido" }

// PatchCommand es la entrada de aplicación de E019, ya desacoplada del DTO HTTP.
// Los punteros distinguen "campo ausente" de "false explícito".
type PatchCommand struct {
	CompletedSteps []string
	Microphone     *bool
	Notifications  *bool
	Configured     *bool
}

// Write es la escritura calculada que el almacén debe aplicar. Contiene el valor
// final de cada atributo del ítem, no deltas: el CAS garantiza que el cálculo se
// hizo sobre la versión vigente.
type Write struct {
	// CompletedSteps ya está normalizado (sin duplicados, orden canónico).
	CompletedSteps []string

	// Done indica si están presentes los cuatro pasos canónicos.
	Done bool

	// MicrophoneRaw y NotificationsRaw son el texto a persistir
	// (`granted|denied`, o el valor heredado conservado si el campo no se envió).
	MicrophoneRaw    string
	NotificationsRaw string

	// Configured es el booleano a persistir.
	Configured bool

	// CompletedAt es la marca a persistir; nil escribe `NULL` explícito.
	CompletedAt *time.Time

	// ExpectedVersion y HasVersion describen el CAS. HasVersion=false es un ítem
	// heredado sin atributo `version`: la condición es su ausencia y `ADD`
	// materializa el 1.
	ExpectedVersion int64
	HasVersion      bool

	// Now es el instante UTC de la escritura, inyectable en pruebas.
	Now time.Time
}

// Writer es el puerto de escritura de E019. Se declara aparte de Store para que
// un almacén de sólo lectura (o un doble de E018) no pueda escribir por
// accidente: sin este puerto, el servicio falla cerrado con ErrUnavailable.
type Writer interface {
	// CreateOnboarding crea el ítem con versión 1 y falla con ErrConflict si otro
	// actor lo creó antes.
	CreateOnboarding(ctx context.Context, userID string, write Write) (*Record, error)

	// UpdateOnboarding aplica la escritura con CAS sobre `version`. Devuelve
	// ErrConflict si la versión leída ya no es la vigente y ErrUnavailable ante
	// cualquier otro fallo.
	UpdateOnboarding(ctx context.Context, userID string, write Write) (*Record, error)
}

// PermissionRaw traduce el booleano del contrato al texto persistido.
// `true` → `granted`; `false` → `denied`.
func PermissionRaw(granted bool) string {
	if granted {
		return "granted"
	}
	return "denied"
}

// NormalizeSubmittedSteps valida y normaliza `completedSteps` de una escritura.
//
// Acepta `nil`/vacío como conjunto vacío (el contrato exige que el array esté
// presente; eso lo impone el DTO). Recorta espacios y unifica mayúsculas, igual
// que la proyección de E018. Cualquier valor fuera del catálogo produce
// *UnknownStepError, incluido el string vacío.
func NormalizeSubmittedSteps(raw []string) ([]string, error) {
	admitidos := make(map[string]struct{}, len(canonicalSteps))
	for _, step := range canonicalSteps {
		admitidos[step] = struct{}{}
	}

	vistos := make(map[string]struct{}, len(raw))
	for _, value := range raw {
		limpio := strings.ToLower(strings.TrimSpace(value))
		if _, ok := admitidos[limpio]; !ok {
			return nil, &UnknownStepError{Step: limpio}
		}
		vistos[limpio] = struct{}{}
	}

	presentes := make([]string, 0, len(vistos))
	for step := range vistos {
		presentes = append(presentes, step)
	}
	// CanonicalSteps deduplica y ordena según el contrato; nunca devuelve nil.
	return CanonicalSteps(presentes), nil
}

// Patch aplica la transición de onboarding del titular.
//
// Flujo: valida el usuario y los pasos; lee la versión vigente con consistencia
// fuerte; calcula la escritura completa; delega el CAS en el almacén. Un
// conflicto se reintenta UNA sola vez con una lectura nueva; si vuelve a fallar,
// devuelve ErrConflict y nada se ha sobrescrito.
func (s *Service) Patch(ctx context.Context, userID string, command PatchCommand) (*State, error) {
	if s == nil || s.store == nil {
		return nil, ErrUnavailable
	}
	writer, ok := s.store.(Writer)
	if !ok || writer == nil {
		// Almacén de sólo lectura: no se degrada a éxito sin persistencia.
		return nil, ErrUnavailable
	}

	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, ErrUnauthorized
	}

	steps, err := NormalizeSubmittedSteps(command.CompletedSteps)
	if err != nil {
		return nil, err
	}

	for intento := 0; intento < 2; intento++ {
		record, found, errLectura := s.store.GetOnboarding(ctx, userID)
		if errLectura != nil {
			return nil, ErrUnavailable
		}

		write := s.planWrite(record, found, command, steps)

		var (
			resultado    *Record
			errEscritura error
		)
		if found && record != nil {
			resultado, errEscritura = writer.UpdateOnboarding(ctx, userID, write)
		} else {
			resultado, errEscritura = writer.CreateOnboarding(ctx, userID, write)
		}

		switch {
		case errEscritura == nil:
			if resultado == nil {
				return nil, ErrUnavailable
			}
			return stateFromRecord(resultado), nil
		case errors.Is(errEscritura, ErrConflict):
			// Otra instancia escribió entre la lectura y el CAS: se reintenta con
			// una lectura nueva. No se reutiliza la versión anterior.
			continue
		default:
			return nil, ErrUnavailable
		}
	}

	return nil, ErrConflict
}

// planWrite calcula el valor final de cada atributo a partir de la fila leída.
// `record` puede ser nil cuando el ítem no existe: en ese caso los campos
// opcionales toman su valor por defecto (`denied`/false), no un vacío ambiguo.
func (s *Service) planWrite(record *Record, found bool, command PatchCommand, steps []string) Write {
	now := s.now().UTC()

	var leido *Record
	if found && record != nil {
		leido = record
	}

	write := Write{
		CompletedSteps: steps,
		Done:           IsComplete(steps),
		Now:            now,
	}

	// completedAt: se marca la primera vez que el onboarding queda completo y se
	// conserva mientras siga completo. Sin los cuatro pasos se escribe NULL.
	switch {
	case !write.Done:
		write.CompletedAt = nil
	case leido != nil && leido.CompletedAt != nil && !leido.CompletedAt.IsZero():
		marca := leido.CompletedAt.UTC()
		write.CompletedAt = &marca
	default:
		marca := now
		write.CompletedAt = &marca
	}

	// microphone / notifications: `true` → granted, `false` → denied. Ausente
	// conserva el texto persistido para no destruir un `pending` heredado.
	switch {
	case command.Microphone != nil:
		write.MicrophoneRaw = PermissionRaw(*command.Microphone)
	case leido != nil && leido.MicrophoneRaw != "":
		write.MicrophoneRaw = leido.MicrophoneRaw
	default:
		write.MicrophoneRaw = PermissionRaw(false)
	}
	switch {
	case command.Notifications != nil:
		write.NotificationsRaw = PermissionRaw(*command.Notifications)
	case leido != nil && leido.NotificationsRaw != "":
		write.NotificationsRaw = leido.NotificationsRaw
	default:
		write.NotificationsRaw = PermissionRaw(false)
	}

	switch {
	case command.Configured != nil:
		write.Configured = *command.Configured
	case leido != nil:
		write.Configured = leido.Configured
	default:
		write.Configured = false
	}

	if leido != nil {
		write.ExpectedVersion = leido.Version
		write.HasVersion = leido.HasVersion
	}
	return write
}
