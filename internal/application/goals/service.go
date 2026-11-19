// Package goals implementa los casos de uso de metas de ahorro (E038, E039, E040, E041).
//
// Invariantes de la capa:
//   - La identidad llega exclusivamente de `sub` del JWT verificado por el middleware; el
//     cliente no elige el usuario y nunca puede leer ni escribir metas ajenas.
//   - Los importes se reciben ya convertidos a céntimos enteros por el transporte y se
//     manipulan como `int64` a través de `internal/domain/money`. No existe punto flotante.
//   - `openingSaved` conserva el ahorro de apertura declarado en el alta y no lo modifica
//     E041 (A7 de RFC-016): `contributedNet = savedAmount - openingSaved` es calculable.
//   - La idempotencia financiera está aislada por usuario (`UserIdempotencyKeys`, Gate de
//     Oleada 2 §5.2 F-02): dos cuentas con la misma clave nunca comparten resultado.
package goals

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	domainGoal "github.com/nexumpr/Nexum-Backend/internal/domain/goal"
	"github.com/nexumpr/Nexum-Backend/pkg/httputil"
)

const (
	// OperationCreateGoal identifica la operación de idempotencia de E039. Forma parte del
	// SK del registro (`IDEM#<operación>#<sha256>`), de modo que la misma clave usada en otra
	// operación no colisiona.
	OperationCreateGoal = "E039_CREATE_GOAL"

	// IdempotencyTTL es el vencimiento lógico de 24 h del Gate de Oleada 2 §5.2. No se
	// promete borrado físico: el TTL de DynamoDB lo elimina de forma asíncrona.
	IdempotencyTTL = 24 * time.Hour
)

// Errores de aplicación. El transporte los traduce a los códigos del contrato; la
// persistencia traduce sus fallos a estos mismos valores para que el handler no conozca AWS.
var (
	// ErrInvalidRequest agrupa las solicitudes mal formadas a nivel de aplicación: usuario
	// ausente, clave de idempotencia vacía, nombre fuera de rango, divisa distinta de MXN,
	// fecha no representable o paginación inválida (HTTP 400).
	ErrInvalidRequest = errors.New("solicitud de metas invalida")

	// ErrUnauthorized indica ausencia de identidad verificada. El middleware ya lo impide;
	// esta guarda es defensa en profundidad (HTTP 401).
	ErrUnauthorized = errors.New("metas no autorizadas")

	// ErrNotFound indica una meta inexistente o de otro titular. Se responde igual en ambos
	// casos para no enumerar recursos ajenos (HTTP 404).
	ErrNotFound = errors.New("meta no encontrada")

	// ErrConflict indica una reutilización de Idempotency-Key con contenido distinto o un
	// conflicto de escritura que no se resolvió tras los reintentos (HTTP 409).
	ErrConflict = errors.New("conflicto de idempotencia o de escritura")

	// ErrVersionConflict indica que el `If-Match` no corresponde a la versión vigente: no se
	// aplicó ninguna escritura (HTTP 412).
	ErrVersionConflict = errors.New("version de la meta desactualizada")

	// ErrTargetNotPositive indica un monto objetivo igual a cero (HTTP 422).
	ErrTargetNotPositive = errors.New("el monto objetivo debe ser positivo")

	// ErrOpeningExceedsTarget indica un ahorro inicial mayor que el monto objetivo (HTTP 422).
	ErrOpeningExceedsTarget = errors.New("el ahorro inicial excede el monto objetivo")

	// ErrTargetBelowSaved indica que se intenta reducir el objetivo por debajo del ahorro ya
	// acumulado (HTTP 422).
	ErrTargetBelowSaved = errors.New("el monto objetivo no puede ser menor que el ahorro acumulado")

	// ErrInvalidTransition indica un cambio de estado no admitido (HTTP 422).
	ErrInvalidTransition = errors.New("transicion de estado no admitida")

	// ErrInconsistent indica un item persistido cuya forma no corresponde al contrato. Se
	// falla cerrado en lugar de emitir un valor inventado (HTTP 500).
	ErrInconsistent = errors.New("meta persistida incoherente")

	// ErrInternal indica un fallo interno no clasificable (HTTP 500).
	ErrInternal = errors.New("error interno de metas")

	// ErrUnavailable indica falta de persistencia o un fallo de servicio (HTTP 503). Nunca se
	// sustituye por datos en memoria.
	ErrUnavailable = errors.New("servicio de metas no disponible")
)

// GoalContract es la proyección EXACTA del esquema `Goal` del contrato: nueve campos, sin
// extras. Se construye campo a campo, de modo que atributos internos (`PK`, `SK`,
// `entityType`, `userId`, `createdAt`) no tienen ningún camino de serialización.
type GoalContract struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	TargetAmount string `json:"targetAmount"`
	SavedAmount  string `json:"savedAmount"`
	Currency     string `json:"currency"`
	TargetDate   string `json:"targetDate"`
	Status       string `json:"status"`
	Version      int64  `json:"version"`
	// OpeningSaved es la enmienda A7: sin él `contributedNet` no es calculable por el
	// cliente. El contrato lo declara opcional; este backend lo emite siempre.
	OpeningSaved string `json:"openingSaved"`
}

// ContractOf proyecta el agregado al DTO del contrato. Valida antes de formatear: una meta
// incoherente no se emite nunca.
func ContractOf(goal *domainGoal.Goal) (GoalContract, error) {
	if goal == nil {
		return GoalContract{}, ErrInconsistent
	}
	if err := goal.Validate(); err != nil {
		return GoalContract{}, ErrInconsistent
	}
	return GoalContract{
		ID:           goal.ID,
		Name:         goal.Name,
		TargetAmount: httputil.FormatCentsToDecimal(goal.TargetAmount.Amount()),
		SavedAmount:  httputil.FormatCentsToDecimal(goal.SavedAmount.Amount()),
		Currency:     goal.TargetAmount.Currency(),
		TargetDate:   goal.FormatTargetDate(),
		Status:       goal.Status.String(),
		Version:      goal.Version,
		OpeningSaved: httputil.FormatCentsToDecimal(goal.OpeningSaved.Amount()),
	}, nil
}

// marshalContract serializa la proyección. El orden de campos es estable (estructura Go), de
// modo que el cuerpo guardado en el registro de idempotencia es reproducible byte a byte.
func marshalContract(goal *domainGoal.Goal) ([]byte, error) {
	contract, err := ContractOf(goal)
	if err != nil {
		return nil, err
	}
	body, err := json.Marshal(contract)
	if err != nil {
		return nil, ErrInternal
	}
	return body, nil
}

// createFingerprint es la representación canónica del payload de E039 que se hashea. Incluye
// la versión del esquema para que un cambio futuro de significado no reutilice hashes.
type createFingerprint struct {
	Schema     int    `json:"schema"`
	Name       string `json:"name"`
	Target     int64  `json:"targetAmountCents"`
	Currency   string `json:"currency"`
	TargetDate string `json:"targetDate"`
	Opening    int64  `json:"openingSavedCents"`
}

// hashCreatePayload calcula el SHA-256 del DTO ya normalizado (nombre recortado e importes en
// céntimos). Un nombre con espacios sobrantes produce el mismo hash que su forma canónica, de
// modo que un reintento idéntico del cliente es un replay y no un 409.
func hashCreatePayload(name string, targetCents, openingCents int64, currency, targetDate string) (string, error) {
	raw, err := json.Marshal(createFingerprint{
		Schema:     1,
		Name:       name,
		Target:     targetCents,
		Currency:   currency,
		TargetDate: targetDate,
		Opening:    openingCents,
	})
	if err != nil {
		return "", ErrInternal
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

// Service orquesta los casos de uso de metas.
type Service struct {
	store    Store
	now      func() time.Time
	newID    func() (string, error)
	location func(goalID string) string
}

// NewService construye el servicio. Sin almacén todas las llamadas fallan cerrado con
// ErrUnavailable: no existe ningún camino con datos en memoria en producción.
func NewService(store Store) *Service {
	return &Service{
		store: store,
		now:   func() time.Time { return time.Now().UTC() },
		newID: defaultNewID,
		location: func(goalID string) string {
			return "/api/v1/goals/" + strings.TrimSpace(goalID)
		},
	}
}

// WithClock permite inyectar un reloj determinista en pruebas.
func (s *Service) WithClock(now func() time.Time) *Service {
	if now != nil {
		s.now = now
	}
	return s
}

// WithIDGenerator permite inyectar un generador determinista en pruebas.
func (s *Service) WithIDGenerator(gen func() (string, error)) *Service {
	if gen != nil {
		s.newID = gen
	}
	return s
}
