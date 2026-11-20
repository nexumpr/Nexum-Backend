package recurring

// Registro manual del pago de una regla recurrente (E037):
// `POST /api/v1/recurring-payments/{recurringPaymentId}/payments`.
//
// Semántica aplicada (RFC-022 §5, Gate Oleada 3 D-W3-01/D-W3-02). E037 NO es un planificador:
//
//   - no genera ocurrencias futuras, no avanza `nextDueDate` y no modifica la regla;
//   - no exige que `paidAt` coincida con `nextDueDate` ni con la cadencia (un pago puede ser
//     anticipado o tardío); sólo exige que la regla exista, sea del titular y esté activa;
//   - el `amount` del cuerpo es el importe REALMENTE pagado y puede diferir del declarado en
//     la regla;
//   - el movimiento derivado usa una categoría EXISTENTE del catálogo E022 según el tipo de la
//     regla (`expense→otros`, `income→ingreso`, `saving→ahorro`); nunca `recurring`, que no
//     existe en el catálogo. `description = name` de la regla y `recurringPaymentId` conservan
//     el vínculo específico;
//   - el duplicado se controla por DÍA CIVIL en la zona IANA del titular
//     (`PREFERENCES.timezone`, `America/Mexico_City` si falta o no es válida), de modo que un
//     cambio de día UTC no permite un segundo pago de la misma ocurrencia. Un pago dividido el
//     mismo día no está soportado por el contrato vigente: requeriría una RFC con identificador
//     de ocurrencia.
//
// Invariantes de esta capa:
//
//   - La identidad llega exclusivamente del `sub` ya verificado; el cliente no elige el titular.
//   - Los importes se operan como `int64` en centavos y se serializan con el mismo DTO que
//     E028/E029 (`movements.NewMovementResponse`): nunca hay coma flotante monetaria.
//   - El cuerpo de éxito lo produce `movements.MarshalMovement`, de modo que el replay
//     idempotente devuelve bytes idénticos al original.
//   - Sin almacén, sin reloj o sin generador de identificadores, la operación falla cerrada
//     con ErrUnavailable: nunca se sustituye la persistencia por memoria local.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	appMovements "github.com/nexumpr/Nexum-Backend/internal/application/movements"
	domainIdentity "github.com/nexumpr/Nexum-Backend/internal/domain/identity"
	domainRecurring "github.com/nexumpr/Nexum-Backend/internal/domain/recurring"
)

const (
	// OperationPayRecurring identifica la operación en el registro de idempotencia por usuario.
	// Forma parte del SK (`IDEM#E037_PAY_RECURRING#<sha256>`): cambiarlo invalidaría los
	// registros ya escritos por este endpoint.
	OperationPayRecurring = "E037_PAY_RECURRING"

	// PayStatusOK es el código de éxito publicado por E037 (200, no 201).
	PayStatusOK = 200

	// DefaultTimezone se aplica cuando el titular no tiene preferencias o la zona guardada no
	// es una zona IANA utilizable (Gate Oleada 3 D-W3-08).
	DefaultTimezone = "America/Mexico_City"

	// LocalDateLayout es el formato de la fecha civil usada para el control de duplicado.
	LocalDateLayout = "2006-01-02"

	// payIDAttempts acota los reintentos ante una colisión del identificador del movimiento.
	// Una colisión de un UUIDv4 es estadísticamente imposible: el reintento existe para no
	// devolver un conflicto al cliente por un fallo que no es suyo.
	payIDAttempts = 3
)

// CategoryIDForRuleType mapea el tipo de la regla a una categoría EXISTENTE del catálogo
// público E022. El catálogo no contiene `recurring`: inventar esa categoría falsearía la
// clasificación. `expense` usa la categoría genérica `otros`; el vínculo específico se conserva
// en `recurringPaymentId` y en la descripción (nombre de la regla).
func CategoryIDForRuleType(tipo string) (string, error) {
	switch strings.TrimSpace(tipo) {
	case domainRecurring.TypeExpense:
		return "otros", nil
	case domainRecurring.TypeIncome:
		return "ingreso", nil
	case domainRecurring.TypeSaving:
		return "ahorro", nil
	default:
		return "", ErrUnavailable
	}
}

var (
	// ErrRuleInactive indica que la regla existe en la partición del titular pero está dada de
	// baja (E036). Se responde 409: el recurso existe, pero su estado impide registrar pagos.
	ErrRuleInactive = fmt.Errorf("%w: la regla recurrente esta inactiva", ErrConflict)

	// ErrDuplicatePayment indica que la regla ya tiene un pago registrado en la misma fecha
	// civil local. Es un conflicto (409) aunque la Idempotency-Key sea distinta (RFC-022 §5.2).
	ErrDuplicatePayment = fmt.Errorf("%w: ya existe un pago de la regla en la misma fecha local", ErrConflict)

	// ErrMovementIDCollision indica que la clave del movimiento generado ya existe. No es un
	// conflicto del cliente: el servicio reintenta con otro identificador.
	ErrMovementIDCollision = errors.New("pago recurrente: colision de identificador de movimiento")
)

// PayCommand es la entrada de E037. `Amount` llega como cadena decimal del contrato y `PaidAt`
// como `date-time` RFC 3339; ambos se normalizan antes de tocar la persistencia.
type PayCommand struct {
	UserID         string
	RuleID         string
	IdempotencyKey string
	PaidAt         string
	Amount         string
	Currency       string
}

// PayResult es el resultado de E037. `Body` es la respuesta autoritativa (idéntica en creación y
// en replay) y `Replayed` indica que la transacción ya se había confirmado con la misma clave.
type PayResult struct {
	Status   int
	Body     []byte
	Replayed bool
}

// PayWrite es la escritura atómica del pago: el movimiento derivado, su puntero, el localizador
// de ocurrencia, el registro de idempotencia y el estado de la regla, todo en una sola
// transacción condicional.
type PayWrite struct {
	UserID string
	RuleID string
	// Movement es el movimiento derivado, con `RecurringPaymentID` ya fijado a la regla.
	Movement appMovements.Movement
	// IdempotencyKey es la clave original del cliente: el adaptador sólo persiste su hash.
	IdempotencyKey string
	Operation      string
	PayloadHash    string
	// ResponseBody es el cuerpo 200 canónico. Se persiste sin re-serializar para que el replay
	// sea byte a byte idéntico al original.
	ResponseBody []byte
	Status       int
	Now          time.Time
	ExpiresAt    time.Time
	// LocalDate es el día civil (`YYYY-MM-DD`) en la zona del titular que bloquea un segundo
	// pago de la misma regla ese día.
	LocalDate string
}

// PaymentStore es el puerto de persistencia de E037.
//
// La superficie es deliberadamente estrecha y no incluye `Scan`: el adaptador de DynamoDB la
// implementa con `GetItem` y `TransactWriteItems`, de modo que el compilador impide recorrer la
// tabla o escribir fuera de una transacción (A-060 §2.5, AC-A-060-01).
type PaymentStore interface {
	// LoadRule resuelve la regla por su clave exacta en la partición del titular. Devuelve
	// (nil, nil) si no existe o pertenece a otra cuenta: ambos casos son 404 sin enumeración.
	LoadRule(ctx context.Context, userID, ruleID string) (*Payment, error)
	// LoadTimezone devuelve la zona IANA declarada en `USER#<id>/PREFERENCES`, vacía si el
	// titular no tiene preferencias.
	LoadTimezone(ctx context.Context, userID string) (string, error)
	// Pay confirma el pago en UNA transacción. Si el registro de idempotencia ya existe con el
	// mismo `PayloadHash` devuelve el cuerpo original con `Replayed = true`; con otro contenido
	// devuelve ErrConflict. Si el localizador de la fecha local ya existe devuelve
	// ErrDuplicatePayment. Si la regla no existe o está inactiva devuelve ErrNotFound o
	// ErrRuleInactive. Sólo una colisión del identificador devuelve ErrMovementIDCollision.
	Pay(ctx context.Context, write PayWrite) (*PayResult, error)
}

// PaymentService orquesta el registro de pagos de reglas recurrentes (E037).
//
// Es independiente del transporte y del SDK de AWS: depende exclusivamente de `PaymentStore`.
type PaymentService struct {
	store    PaymentStore
	now      func() time.Time
	newID    func() (string, error)
	location func(name string) *time.Location
}

// NewPaymentService construye el servicio. Sin almacén todas las operaciones fallan cerradas
// con ErrUnavailable.
func NewPaymentService(store PaymentStore) *PaymentService {
	return &PaymentService{
		store:    store,
		now:      func() time.Time { return time.Now().UTC() },
		newID:    domainIdentity.NewID,
		location: loadLocation,
	}
}

// WithClock permite inyectar un reloj determinista en pruebas.
func (s *PaymentService) WithClock(now func() time.Time) *PaymentService {
	if now != nil {
		s.now = now
	}
	return s
}

// WithIDGenerator permite inyectar un generador de identificadores determinista en pruebas.
func (s *PaymentService) WithIDGenerator(generate func() (string, error)) *PaymentService {
	if generate != nil {
		s.newID = generate
	}
	return s
}

// WithLocationResolver permite inyectar un resolutor de zonas en pruebas.
func (s *PaymentService) WithLocationResolver(resolve func(name string) *time.Location) *PaymentService {
	if resolve != nil {
		s.location = resolve
	}
	return s
}

// Pay registra el pago de una regla recurrente (E037).
//
// Secuencia: identidad → Idempotency-Key → importe → `paidAt` → divisa → zona del titular →
// estado de la regla → transacción atómica. La fecha civil de control se calcula ANTES de la
// transacción, de modo que dos nodos con relojes distintos pero la misma zona lleguen al mismo
// localizador y sólo uno confirme.
func (s *PaymentService) Pay(ctx context.Context, command PayCommand) (*PayResult, error) {
	if s == nil || s.store == nil || s.newID == nil || s.now == nil {
		return nil, ErrUnavailable
	}

	userID := strings.TrimSpace(command.UserID)
	if userID == "" {
		return nil, ErrUnauthorized
	}

	clave := strings.TrimSpace(command.IdempotencyKey)
	if len(clave) < idempotencyMinBytes || len(clave) > idempotencyMaxBytes {
		return nil, ErrInvalidRequest
	}

	ruleID := strings.TrimSpace(command.RuleID)
	if ruleID == "" {
		return nil, ErrInvalidRequest
	}

	ahora := s.now().UTC()

	centavos, errImporte := normalizePayAmount(command.Amount)
	if errImporte != nil {
		return nil, errImporte
	}

	ocurrido, errFecha := normalizePaidAt(command.PaidAt, ahora)
	if errFecha != nil {
		return nil, errFecha
	}

	divisa, errDivisa := normalizePayCurrency(command.Currency)
	if errDivisa != nil {
		return nil, errDivisa
	}

	zonaGuardada, errZona := s.store.LoadTimezone(ctx, userID)
	if errZona != nil {
		// Sin zona no se puede fijar el día civil de control: se falla cerrado en lugar de
		// asumir UTC y permitir un doble pago por cambio de día.
		return nil, ErrUnavailable
	}
	localDate := ocurrido.In(s.resolveLocation(zonaGuardada)).Format(LocalDateLayout)

	regla, errRegla := s.store.LoadRule(ctx, userID, ruleID)
	if errRegla != nil {
		return nil, ErrUnavailable
	}
	if regla == nil || regla.ID != ruleID {
		// Inexistente y ajena son indistinguibles: la clave vive en la partición del titular.
		return nil, ErrNotFound
	}
	if !regla.Active {
		return nil, ErrRuleInactive
	}

	tipo, errTipo := domainRecurring.ParseType(regla.Type)
	if errTipo != nil {
		return nil, ErrUnavailable
	}
	// `description` del movimiento tiene `maxLength: 120`; una regla que no pueda producir un
	// movimiento válido se rechaza en lugar de truncar el nombre en silencio.
	descripcion, errNombre := domainRecurring.ValidateName(regla.Name)
	if errNombre != nil {
		return nil, ErrUnavailable
	}

	payloadHash := payPayloadHash(ruleID, centavos, divisa, ocurrido)
	categoria, errCategoria := CategoryIDForRuleType(tipo)
	if errCategoria != nil {
		return nil, ErrUnavailable
	}

	for intento := 0; intento < payIDAttempts; intento++ {
		id, errID := s.newID()
		if errID != nil || strings.TrimSpace(id) == "" {
			return nil, ErrUnavailable
		}

		movimiento := appMovements.Movement{
			ID:                 strings.TrimSpace(id),
			Type:               tipo,
			AmountCents:        centavos,
			Currency:           divisa,
			CategoryID:         categoria,
			Description:        descripcion,
			OccurredAt:         ocurrido,
			Version:            1,
			RecurringPaymentID: ruleID,
		}
		cuerpo, errCuerpo := appMovements.MarshalMovement(movimiento)
		if errCuerpo != nil {
			return nil, ErrUnavailable
		}

		resultado, errPago := s.store.Pay(ctx, PayWrite{
			UserID:         userID,
			RuleID:         ruleID,
			Movement:       movimiento,
			IdempotencyKey: clave,
			Operation:      OperationPayRecurring,
			PayloadHash:    payloadHash,
			ResponseBody:   cuerpo,
			Status:         PayStatusOK,
			Now:            ahora,
			ExpiresAt:      ahora.Add(IdempotencyTTL),
			LocalDate:      localDate,
		})
		if errors.Is(errPago, ErrMovementIDCollision) {
			// No es un conflicto del cliente: se reintenta con otro identificador.
			continue
		}
		if errPago != nil {
			switch {
			case errors.Is(errPago, ErrConflict):
				// Conserva el centinela específico (ErrRuleInactive, ErrDuplicatePayment) para
				// que el transporte y las pruebas distingan el motivo del 409.
				return nil, errPago
			case errors.Is(errPago, ErrNotFound):
				return nil, ErrNotFound
			case errors.Is(errPago, ErrInvalidRequest):
				return nil, ErrInvalidRequest
			default:
				return nil, ErrUnavailable
			}
		}
		if resultado == nil || len(resultado.Body) == 0 || resultado.Status != PayStatusOK {
			return nil, ErrUnavailable
		}
		return resultado, nil
	}
	return nil, ErrUnavailable
}

// normalizePayAmount aplica el patrón decimal publicado y la regla de importe estrictamente
// positivo. Una forma inválida es 400; un valor con forma válida que incumple la regla financiera
// (cero) es 422, igual que en la familia de movimientos.
func normalizePayAmount(valor string) (int64, error) {
	centavos, err := appMovements.ParseAmount(valor)
	if err != nil {
		var violacion *appMovements.FieldViolation
		if errors.As(err, &violacion) {
			return 0, newViolation("amount", violacion.Reason)
		}
		return 0, ErrInvalidRequest
	}
	if centavos <= 0 {
		return 0, newViolation("amount", "out_of_range")
	}
	return centavos, nil
}

// normalizePaidAt interpreta el `date-time` del contrato y lo normaliza a UTC con milisegundos
// (la precisión de la SK del movimiento). Las cotas de cordura son las de la familia de
// movimientos —año representable y no más de 24 h en el futuro—, pero la violación se atribuye
// siempre al campo `paidAt`; la relación con `nextDueDate` y con la cadencia NO se valida: un pago
// puede ser anticipado o tardío (RFC-022 §5.3).
func normalizePaidAt(valor string, ahora time.Time) (time.Time, error) {
	instante, err := appMovements.ParseOccurredAt(valor)
	if err != nil {
		var violacion *appMovements.FieldViolation
		if errors.As(err, &violacion) {
			return time.Time{}, newViolation("paidAt", violacion.Reason)
		}
		return time.Time{}, ErrInvalidRequest
	}
	if instante.Year() < 1970 || instante.Year() > 2200 {
		return time.Time{}, newViolation("paidAt", "out_of_range")
	}
	if instante.After(ahora.UTC().Add(24 * time.Hour)) {
		return time.Time{}, newViolation("paidAt", "in_the_future")
	}
	return instante, nil
}

// normalizePayCurrency exige la única divisa publicada.
func normalizePayCurrency(valor string) (string, error) {
	divisa := strings.ToUpper(strings.TrimSpace(valor))
	if divisa != domainRecurring.CurrencyMXN {
		return "", newViolation("currency", "not_supported")
	}
	return divisa, nil
}

// payPayloadHash es el SHA-256 de la operación normalizada: regla, importe en centavos, divisa e
// instante UTC. Incluye la regla porque el registro de idempotencia es por usuario y operación:
// sin el identificador, la misma clave sobre dos reglas distintas reproduciría el movimiento de
// la primera. La longitud de cada parte se prefija para que dos cargas distintas no puedan
// producir la misma concatenación por reparto ambiguo de separadores.
func payPayloadHash(ruleID string, amountCents int64, currency string, occurredAt time.Time) string {
	partes := []string{
		ruleID,
		strconv.FormatInt(amountCents, 10),
		currency,
		appMovements.FormatOccurredAt(occurredAt),
	}
	var canonica []byte
	for _, parte := range partes {
		canonica = append(canonica, fmt.Sprintf("%d:%s;", len(parte), parte)...)
	}
	suma := sha256.Sum256(canonica)
	return hex.EncodeToString(suma[:])
}

// resolveLocation devuelve la zona del titular. Una preferencia ausente, vacía, "Local" o que no
// sea una zona IANA utilizable cae a la zona por defecto: el día civil nunca se calcula en UTC.
func (s *PaymentService) resolveLocation(nombre string) *time.Location {
	if s != nil && s.location != nil {
		if zona := s.location(nombre); zona != nil {
			return zona
		}
	}
	return defaultLocation()
}

// loadLocation resuelve una zona IANA con la base del sistema o la embebida con `time/tzdata`.
func loadLocation(nombre string) *time.Location {
	limpio := strings.TrimSpace(nombre)
	if limpio != "" && limpio != "Local" && !strings.ContainsAny(limpio, " \t\r\n") {
		if zona, err := time.LoadLocation(limpio); err == nil && zona != nil {
			return zona
		}
	}
	return defaultLocation()
}

var (
	defaultLocationOnce  sync.Once
	defaultLocationValue *time.Location
)

// defaultLocation resuelve `America/Mexico_City` una sola vez. Si la base de zonas no estuviera
// disponible, el respaldo es el desplazamiento civil vigente en México desde 2022 (UTC-06:00,
// sin horario de verano): un fallo de tzdata no puede convertir el día civil del titular en UTC.
func defaultLocation() *time.Location {
	defaultLocationOnce.Do(func() {
		if zona, err := time.LoadLocation(DefaultTimezone); err == nil && zona != nil {
			defaultLocationValue = zona
			return
		}
		defaultLocationValue = time.FixedZone(DefaultTimezone, -6*60*60)
	})
	return defaultLocationValue
}
