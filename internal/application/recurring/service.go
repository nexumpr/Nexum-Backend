package recurring

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	domainDynamo "github.com/nexumpr/Nexum-Backend/internal/domain/dynamo"
	domainIdentity "github.com/nexumpr/Nexum-Backend/internal/domain/identity"
	domainRecurring "github.com/nexumpr/Nexum-Backend/internal/domain/recurring"
	"github.com/nexumpr/Nexum-Backend/pkg/httputil"
)

// Service orquesta los cinco casos de uso de pagos recurrentes.
//
// No conoce HTTP ni el SDK de AWS: depende del puerto `Store`. Sin almacén todas las
// operaciones fallan cerrado con ErrUnavailable; la lista falla cerrado además sin clave de
// firma de cursor, porque un cursor sin firmar permitiría manipular la paginación.
type Service struct {
	store        Store
	cursorSecret []byte
	now          func() time.Time
	newID        func() (string, error)
	cursorTTL    time.Duration
}

// NewService construye el servicio. `cursorSecret` sólo lo necesita E032; si está vacío,
// la lista responde 503 y el resto de operaciones sigue funcionando con normalidad.
func NewService(store Store, cursorSecret []byte) *Service {
	return &Service{
		store:        store,
		cursorSecret: cursorSecret,
		now:          func() time.Time { return time.Now().UTC() },
		newID:        domainIdentity.NewID,
		cursorTTL:    httputil.DefaultCursorTTL,
	}
}

// WithClock permite inyectar un reloj determinista en pruebas.
func (s *Service) WithClock(now func() time.Time) *Service {
	if now != nil {
		s.now = now
	}
	return s
}

// WithIDGenerator permite inyectar un generador de identificadores determinista en pruebas.
func (s *Service) WithIDGenerator(generate func() (string, error)) *Service {
	if generate != nil {
		s.newID = generate
	}
	return s
}

// WithCursorTTL permite acortar la vigencia del cursor en pruebas.
func (s *Service) WithCursorTTL(ttl time.Duration) *Service {
	if ttl != 0 {
		s.cursorTTL = ttl
	}
	return s
}

// Create registra una regla recurrente (E033).
//
// Secuencia: identidad → Idempotency-Key → validación y normalización del DTO → coherencia
// de la programación → transacción atómica (recurso + registro de idempotencia). No genera
// ningún pago ni movimiento: sólo la regla.
func (s *Service) Create(ctx context.Context, command CreateCommand) (*CreateResult, error) {
	if s == nil || s.store == nil {
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

	pago, err := buildPayment(command)
	if err != nil {
		return nil, err
	}

	id, errID := s.newID()
	if errID != nil || strings.TrimSpace(id) == "" {
		return nil, ErrUnavailable
	}
	pago.ID = id
	pago.Version = 1
	pago.Active = true

	cuerpo, errCuerpo := pago.Body()
	if errCuerpo != nil {
		return nil, ErrUnavailable
	}

	write := CreateWrite{
		UserID:         userID,
		IdempotencyKey: clave,
		Operation:      OperationCreate,
		PayloadHash:    payloadHash(pago),
		Body:           cuerpo,
		TTL:            IdempotencyTTL,
		Payment:        pago,
		Now:            s.now().UTC(),
	}

	idCreado, cuerpoAutoritativo, replayed, errStore := s.store.Create(ctx, write)
	if errStore != nil {
		switch {
		case errors.Is(errStore, ErrConflict):
			return nil, ErrConflict
		case errors.Is(errStore, ErrInvalidRequest):
			return nil, ErrInvalidRequest
		case errors.Is(errStore, ErrNotFound):
			return nil, ErrNotFound
		default:
			return nil, ErrUnavailable
		}
	}
	if strings.TrimSpace(idCreado) == "" || len(cuerpoAutoritativo) == 0 {
		return nil, ErrUnavailable
	}
	return &CreateResult{ID: idCreado, Body: cuerpoAutoritativo, Replayed: replayed}, nil
}

// buildPayment valida y normaliza la carga útil de E033. Devuelve una violación de negocio
// (422) por cada regla incumplida, nunca un valor corregido en silencio.
func buildPayment(command CreateCommand) (Payment, error) {
	nombre, err := domainRecurring.ValidateName(command.Name)
	if err != nil {
		return Payment{}, newViolation("name", "invalid_value")
	}
	if err := domainRecurring.ValidateAmount(command.AmountCents); err != nil {
		return Payment{}, newViolation("amount", "out_of_range")
	}
	divisa := strings.TrimSpace(command.Currency)
	if divisa != domainRecurring.CurrencyMXN {
		return Payment{}, newViolation("currency", "not_supported")
	}
	frecuencia, err := domainRecurring.ParseFrequency(command.Frequency)
	if err != nil {
		return Payment{}, newViolation("frequency", "out_of_enum")
	}
	tipo, err := domainRecurring.ParseType(command.Type)
	if err != nil {
		return Payment{}, newViolation("type", "out_of_enum")
	}
	inicio, err := domainRecurring.ParseDate(command.StartDate)
	if err != nil {
		return Payment{}, newViolation("startDate", "invalid_format")
	}
	proximo, err := domainRecurring.ParseDate(command.NextDueDate)
	if err != nil {
		return Payment{}, newViolation("nextDueDate", "invalid_format")
	}
	dueDay := copyDueDay(command.DueDay)
	if err := domainRecurring.ValidateDueDay(dueDay, frecuencia, proximo); err != nil {
		return Payment{}, dueDayViolation(err)
	}
	if err := domainRecurring.ValidateSchedule(inicio, proximo, frecuencia); err != nil {
		return Payment{}, newViolation("nextDueDate", "incoherent_schedule")
	}
	return Payment{
		Name:        nombre,
		AmountCents: command.AmountCents,
		Currency:    divisa,
		Frequency:   frecuencia,
		NextDueDate: proximo,
		Type:        tipo,
		StartDate:   inicio,
		DueDay:      dueDay,
	}, nil
}

// dueDayViolation traduce el error de dominio de `dueDay` a una violación de campo.
func dueDayViolation(err error) *Violation {
	if errors.Is(err, domainRecurring.ErrIncoherentSchedule) {
		return newViolation("dueDay", "incoherent_schedule")
	}
	return newViolation("dueDay", "out_of_range")
}

// copyDueDay copia el valor para que el recurso no comparta memoria con la petición.
func copyDueDay(value *int) *int {
	if value == nil {
		return nil
	}
	copia := *value
	return &copia
}

// List devuelve una página de reglas del titular (E032).
//
// Secuencia: identidad → límite → filtros → verificación criptográfica del cursor (firma,
// expiración, pertenencia y filtros) → consulta por partición → cursor siguiente sólo si la
// persistencia confirmó que hay más filas.
func (s *Service) List(ctx context.Context, query ListQuery) (*ListPage, error) {
	if s == nil || s.store == nil || len(s.cursorSecret) == 0 {
		return nil, ErrUnavailable
	}
	userID := strings.TrimSpace(query.UserID)
	if userID == "" {
		return nil, ErrUnauthorized
	}

	limit, errLimite := normalizeLimit(query.Limit)
	if errLimite != nil {
		return nil, errLimite
	}

	filtro := Filter{}
	if query.Active != nil {
		activo := *query.Active
		filtro.Active = &activo
	}
	if strings.TrimSpace(query.Type) != "" {
		tipo, errTipo := domainRecurring.ParseType(query.Type)
		if errTipo != nil {
			return nil, ErrInvalidRequest
		}
		filtro.Type = tipo
	}

	startSK := ""
	if raw := strings.TrimSpace(query.Cursor); raw != "" {
		if len(raw) < CursorMinLength || len(raw) > CursorMaxLength {
			return nil, ErrInvalidRequest
		}
		payload, errCursor := httputil.ValidateCursor(s.cursorSecret, raw, userID)
		if errCursor != nil {
			// Manipulado, corrupto, vencido o de otro usuario: un único 400 sin distinguir.
			return nil, ErrInvalidRequest
		}
		if payload.SortBy != listSortKey(limit, filtro) {
			// Cursor emitido con otros filtros o con otro tamaño de página.
			return nil, ErrInvalidRequest
		}
		if !strings.HasPrefix(payload.LastID, domainDynamo.PrefixRecurring) {
			return nil, ErrInvalidRequest
		}
		startSK = payload.LastID
	}

	items, nextSK, errStore := s.store.List(ctx, userID, startSK, limit, filtro)
	if errStore != nil {
		return nil, ErrUnavailable
	}

	pagina := &ListPage{Items: items}
	if nextSK != "" {
		cursor, errGen := httputil.GenerateCursor(s.cursorSecret, userID, listSortKey(limit, filtro), nextSK, s.now().UTC(), s.cursorTTL)
		if errGen != nil {
			return nil, ErrUnavailable
		}
		pagina.NextCursor = cursor
	}
	return pagina, nil
}

// normalizeLimit valida el parámetro `limit` del contrato (1..100, por defecto 25). Un
// valor cero se interpreta como ausencia del parámetro; cualquier otro fuera de rango es
// 400: nunca se recorta en silencio.
func normalizeLimit(limit int) (int, error) {
	if limit == 0 {
		return DefaultLimit, nil
	}
	if limit < 1 || limit > MaxLimit {
		return 0, ErrInvalidRequest
	}
	return limit, nil
}

// listSortKey liga el cursor al conjunto de filtros y al tamaño de página con el que se
// emitió, de modo que reutilizarlo con otros filtros sea un 400 y no una página incoherente.
func listSortKey(limit int, filter Filter) string {
	activo := "all"
	if filter.Active != nil {
		if *filter.Active {
			activo = "true"
		} else {
			activo = "false"
		}
	}
	tipo := "all"
	if filter.Type != "" {
		tipo = filter.Type
	}
	return "recurring:v1:limit=" + strconv.Itoa(limit) + ":active=" + activo + ":type=" + tipo
}

// Get devuelve una regla del titular (E034). Un identificador de otra cuenta no existe en
// su partición y por tanto produce el mismo 404 que uno inexistente.
func (s *Service) Get(ctx context.Context, userID, paymentID string) (*Payment, error) {
	if s == nil || s.store == nil {
		return nil, ErrUnavailable
	}
	limpioUsuario := strings.TrimSpace(userID)
	if limpioUsuario == "" {
		return nil, ErrUnauthorized
	}
	limpioID := strings.TrimSpace(paymentID)
	if limpioID == "" {
		return nil, ErrInvalidRequest
	}

	pago, err := s.store.Get(ctx, limpioUsuario, limpioID)
	if err != nil {
		return nil, ErrUnavailable
	}
	if pago == nil {
		return nil, ErrNotFound
	}
	return pago, nil
}

// Update aplica una modificación parcial con compare-and-swap por versión (E035).
//
// Orden: identidad → versión esperada → lectura consistente → 404 si no existe → 412 si la
// versión no es la vigente → fusión y validación de la coherencia resultante → CAS. Si la
// fusión no cambia nada, se devuelve el recurso actual sin incrementar la versión.
//
// `type` es inmutable: el contrato admite el campo en `RecurringPatch` (por eso el DTO lo
// acepta) pero una regla no puede cambiar de naturaleza sin invalidar el historial de pagos
// ya ejecutados.
func (s *Service) Update(ctx context.Context, command PatchCommand) (*Payment, error) {
	if s == nil || s.store == nil {
		return nil, ErrUnavailable
	}
	userID := strings.TrimSpace(command.UserID)
	if userID == "" {
		return nil, ErrUnauthorized
	}
	paymentID := strings.TrimSpace(command.PaymentID)
	if paymentID == "" {
		return nil, ErrInvalidRequest
	}
	if command.ExpectedVersion < 1 {
		return nil, ErrInvalidRequest
	}

	actual, err := s.store.Get(ctx, userID, paymentID)
	if err != nil {
		return nil, ErrUnavailable
	}
	if actual == nil {
		return nil, ErrNotFound
	}
	if actual.Version != command.ExpectedVersion {
		return nil, ErrVersionConflict
	}

	propuesto := *actual
	propuesto.DueDay = copyDueDay(actual.DueDay)

	if command.Name != nil {
		nombre, errNombre := domainRecurring.ValidateName(*command.Name)
		if errNombre != nil {
			return nil, newViolation("name", "invalid_value")
		}
		propuesto.Name = nombre
	}
	if command.AmountCents != nil {
		if errImporte := domainRecurring.ValidateAmount(*command.AmountCents); errImporte != nil {
			return nil, newViolation("amount", "out_of_range")
		}
		propuesto.AmountCents = *command.AmountCents
	}
	if command.Active != nil {
		propuesto.Active = *command.Active
	}
	if command.StartDate != nil {
		inicio, errFecha := domainRecurring.ParseDate(*command.StartDate)
		if errFecha != nil {
			return nil, newViolation("startDate", "invalid_format")
		}
		propuesto.StartDate = inicio
	}
	if command.NextDueDate != nil {
		proximo, errFecha := domainRecurring.ParseDate(*command.NextDueDate)
		if errFecha != nil {
			return nil, newViolation("nextDueDate", "invalid_format")
		}
		propuesto.NextDueDate = proximo
	}
	if command.Type != nil {
		tipo, errTipo := domainRecurring.ParseType(*command.Type)
		if errTipo != nil {
			return nil, newViolation("type", "out_of_enum")
		}
		if errInmutable := domainRecurring.ValidateTypeChange(actual.Type, tipo); errInmutable != nil {
			return nil, newViolation("type", "immutable")
		}
		propuesto.Type = tipo
	}
	if command.DueDay.Present {
		propuesto.DueDay = copyDueDay(command.DueDay.Value)
	}

	// La frecuencia no cambia en E035, así que la coherencia se revalida contra la vigente.
	if errDueDay := domainRecurring.ValidateDueDay(propuesto.DueDay, actual.Frequency, propuesto.NextDueDate); errDueDay != nil {
		return nil, dueDayViolation(errDueDay)
	}
	if errCalendario := domainRecurring.ValidateSchedule(propuesto.StartDate, propuesto.NextDueDate, actual.Frequency); errCalendario != nil {
		return nil, newViolation("nextDueDate", "incoherent_schedule")
	}

	if !cambia(*actual, propuesto) {
		return actual, nil
	}

	write := UpdateWrite{
		UserID:          userID,
		PaymentID:       paymentID,
		ExpectedVersion: command.ExpectedVersion,
		Payment:         propuesto,
		Now:             s.now().UTC(),
	}
	resultado, errStore := s.store.Update(ctx, write)
	if errStore != nil {
		switch {
		case errors.Is(errStore, ErrVersionConflict):
			return nil, ErrVersionConflict
		case errors.Is(errStore, ErrConflict):
			return nil, ErrConflict
		case errors.Is(errStore, ErrNotFound):
			return nil, ErrNotFound
		default:
			return nil, ErrUnavailable
		}
	}
	if resultado == nil {
		return nil, ErrUnavailable
	}
	return resultado, nil
}

// cambia indica si la fusión modifica algún campo mutable. Evita incrementar la versión en
// una petición que no cambia nada.
func cambia(actual, propuesto Payment) bool {
	if actual.Name != propuesto.Name ||
		actual.AmountCents != propuesto.AmountCents ||
		actual.Active != propuesto.Active ||
		actual.Type != propuesto.Type ||
		!actual.NextDueDate.Equal(propuesto.NextDueDate) ||
		!actual.StartDate.Equal(propuesto.StartDate) {
		return true
	}
	return !sameDueDay(actual.DueDay, propuesto.DueDay)
}

// sameDueDay compara dos días de vencimiento opcionales.
func sameDueDay(a, b *int) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

// Deactivate marca la regla como inactiva (E036). No borra el ítem ni ningún pago: el
// historial de pagos anteriores debe seguir siendo consultable. Es idempotente: repetir la
// operación sobre una regla ya inactiva devuelve el mismo 204 sin incrementar la versión.
func (s *Service) Deactivate(ctx context.Context, userID, paymentID string) error {
	if s == nil || s.store == nil {
		return ErrUnavailable
	}
	limpioUsuario := strings.TrimSpace(userID)
	if limpioUsuario == "" {
		return ErrUnauthorized
	}
	limpioID := strings.TrimSpace(paymentID)
	if limpioID == "" {
		return ErrInvalidRequest
	}

	existia, err := s.store.Deactivate(ctx, limpioUsuario, limpioID, s.now().UTC())
	if err != nil {
		return ErrUnavailable
	}
	if !existia {
		return ErrNotFound
	}
	return nil
}
