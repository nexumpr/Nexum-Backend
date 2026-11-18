package identity

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	domain "github.com/nexumpr/Nexum-Backend/internal/domain/identity"
)

// Casos de uso del ingreso base: E024 (`GET /api/v1/income`) y E025 (`PUT /api/v1/income`).
//
// Invariantes:
//   - La identidad llega exclusivamente de `sub` del JWT verificado por el middleware; el
//     cliente no elige el usuario y no puede leer ni escribir el ingreso de otro titular.
//   - El ingreso vive en `USER#<id>/INCOME`, exactamente el mismo item que escribe E001 en
//     el alta. No se crean particiones, tablas ni índices nuevos.
//   - Los importes son enteros de céntimos. El equivalente mensual se recalcula aquí con el
//     motor exacto de A-070 (`domain.Frequency.MonthlyEquivalent`) y nunca con coma
//     flotante (R-030, GOV-08).
//   - La frecuencia se normaliza a su valor canónico antes de persistir y de responder; los
//     alias `biweekly`/`yearly` nunca llegan a la respuesta ni al item (RFC-020 §2).
//   - E025 NO toca `PROFILE`, ni la autenticación, ni las sesiones: sólo comprueba que el
//     perfil exista y esté ACTIVE para no crear un ingreso huérfano de una cuenta eliminada.
var (
	// ErrIncomeInvalidRequest indica una solicitud mal formada a nivel de aplicación
	// (identificador vacío, versión esperada menor que 1).
	ErrIncomeInvalidRequest = errors.New("solicitud de ingreso invalida")

	// ErrIncomeUnauthorized indica ausencia de identidad verificada. El middleware ya lo
	// impide; esta guarda es defensa en profundidad.
	ErrIncomeUnauthorized = errors.New("ingreso no autorizado")

	// ErrIncomeNotFound indica que el titular no tiene ingreso configurado (E024) o que su
	// perfil ya no existe (E025).
	ErrIncomeNotFound = errors.New("ingreso base no configurado")

	// ErrIncomeInvalidAmount indica un importe fuera de la regla de negocio: cero,
	// negativo o superior a doce dígitos enteros.
	ErrIncomeInvalidAmount = errors.New("importe de ingreso invalido")

	// ErrIncomeInvalidFrequency indica una frecuencia fuera de los nueve valores publicados.
	ErrIncomeInvalidFrequency = errors.New("frecuencia de ingreso invalida")

	// ErrIncomeVersionConflict indica que la versión enviada en `If-Match` no es la vigente:
	// la escritura no se aplicó (HTTP 412).
	ErrIncomeVersionConflict = errors.New("version del ingreso desactualizada")

	// ErrIncomeConflict indica un conflicto transitorio de escritura concurrente sobre el
	// mismo item (HTTP 409); reintentar es seguro porque nada se aplicó.
	ErrIncomeConflict = errors.New("conflicto de escritura del ingreso")

	// ErrIncomeInconsistent indica un item persistido cuya forma no corresponde al contrato
	// (por ejemplo, una frecuencia desconocida). Se falla cerrado en lugar de emitir un
	// valor fuera del enum publicado.
	ErrIncomeInconsistent = errors.New("ingreso persistido incoherente")

	// ErrIncomeUnavailable indica falta de persistencia o un fallo de servicio (HTTP 503).
	// Nunca se sustituye por datos en memoria.
	ErrIncomeUnavailable = errors.New("servicio de ingreso no disponible")
)

// IncomeSnapshot es el ingreso base del titular tal como lo expone el contrato.
//
// `Frequency` ya está normalizada al valor canónico: la capa de aplicación es la única que
// decide qué frecuencia se emite.
type IncomeSnapshot struct {
	AmountCents int64
	Frequency   domain.Frequency
	Version     int64
}

// IncomeUpdate es la entrada de E025 ya validada en FORMATO por el transporte (el importe
// decimal se convierte a céntimos con `httputil.ParseDecimalToCents`). La regla de negocio
// del importe, la frecuencia y el equivalente mensual se validan en esta capa.
type IncomeUpdate struct {
	UserID          string
	AmountCents     int64
	Frequency       string
	ExpectedVersion int64
}

// IncomeStoreWrite es la escritura ya normalizada que recibe la persistencia.
type IncomeStoreWrite struct {
	UserID                 string
	AmountCents            int64
	Frequency              domain.Frequency
	MonthlyEquivalentCents int64
	ExpectedVersion        int64
	Now                    time.Time
}

// IncomeStore es el puerto de persistencia de E024/E025.
//
// Contrato del adaptador:
//   - `LoadIncome` devuelve (nil, nil) cuando el titular no tiene ingreso; un error real de
//     DynamoDB se propaga y NUNCA se convierte en «no configurado».
//   - `ProfileExists` indica si el perfil del titular existe y está ACTIVE.
//   - `StoreIncome` es una única escritura condicional por versión. Devuelve
//     ErrIncomeVersionConflict si la condición no se cumple y ErrIncomeConflict si hay un
//     conflicto transitorio de concurrencia.
type IncomeStore interface {
	LoadIncome(ctx context.Context, userID string) (*IncomeSnapshot, error)
	ProfileExists(ctx context.Context, userID string) (bool, error)
	StoreIncome(ctx context.Context, write IncomeStoreWrite) (*IncomeSnapshot, error)
}

// IncomeService orquesta la lectura y la configuración del ingreso base.
type IncomeService struct {
	store IncomeStore
	now   func() time.Time
}

// NewIncomeService construye el servicio. Sin almacén todas las llamadas fallan cerrado con
// ErrIncomeUnavailable: no existe ningún camino con datos en memoria en producción.
func NewIncomeService(store IncomeStore) *IncomeService {
	return &IncomeService{
		store: store,
		now:   func() time.Time { return time.Now().UTC() },
	}
}

// WithClock permite inyectar un reloj determinista en pruebas.
func (s *IncomeService) WithClock(now func() time.Time) *IncomeService {
	if now != nil {
		s.now = now
	}
	return s
}

// GetIncome devuelve el ingreso base del titular (E024).
//
// La frecuencia leída se normaliza al valor canónico: un item histórico con `biweekly` o
// `yearly` se responde como `fortnightly`/`annual` sin reescribirlo (RFC-020 §2). Una
// frecuencia desconocida no se emite: se falla cerrado con ErrIncomeInconsistent.
func (s *IncomeService) GetIncome(ctx context.Context, userID string) (*IncomeSnapshot, error) {
	if s == nil || s.store == nil {
		return nil, ErrIncomeUnavailable
	}
	clean := strings.TrimSpace(userID)
	if clean == "" {
		return nil, ErrIncomeUnauthorized
	}

	snapshot, err := s.store.LoadIncome(ctx, clean)
	if err != nil {
		if errors.Is(err, ErrIncomeInconsistent) {
			return nil, ErrIncomeInconsistent
		}
		return nil, ErrIncomeUnavailable
	}
	if snapshot == nil {
		return nil, ErrIncomeNotFound
	}
	if snapshot.AmountCents < domain.MinIncomeAmountCents || snapshot.Version < 1 {
		return nil, ErrIncomeInconsistent
	}

	canonical, errFrecuencia := domain.ParseIncomeFrequency(snapshot.Frequency.String())
	if errFrecuencia != nil {
		return nil, ErrIncomeInconsistent
	}

	return &IncomeSnapshot{
		AmountCents: snapshot.AmountCents,
		Frequency:   canonical,
		Version:     snapshot.Version,
	}, nil
}

// SetIncome configura el ingreso base del titular (E025).
//
// Secuencia: validar importe y frecuencia → derivar el equivalente mensual exacto →
// comprobar que el perfil existe y está ACTIVE (para no crear un ingreso huérfano) →
// ejecutar la escritura condicional por versión. Si la versión esperada no es la vigente,
// no se aplica ninguna escritura y se devuelve ErrIncomeVersionConflict.
func (s *IncomeService) SetIncome(ctx context.Context, command IncomeUpdate) (*IncomeSnapshot, error) {
	if s == nil || s.store == nil {
		return nil, ErrIncomeUnavailable
	}
	userID := strings.TrimSpace(command.UserID)
	if userID == "" {
		return nil, ErrIncomeUnauthorized
	}
	if command.ExpectedVersion < 1 {
		return nil, ErrIncomeInvalidRequest
	}
	if err := domain.ValidateIncomeAmount(command.AmountCents); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrIncomeInvalidAmount, err)
	}

	frequency, errFrecuencia := domain.ParseIncomeFrequency(command.Frequency)
	if errFrecuencia != nil {
		return nil, fmt.Errorf("%w: %v", ErrIncomeInvalidFrequency, errFrecuencia)
	}

	equivalente, errEquivalente := frequency.MonthlyEquivalent(command.AmountCents)
	if errEquivalente != nil {
		return nil, fmt.Errorf("%w: %v", ErrIncomeInvalidAmount, errEquivalente)
	}

	existe, errPerfil := s.store.ProfileExists(ctx, userID)
	if errPerfil != nil {
		return nil, ErrIncomeUnavailable
	}
	if !existe {
		return nil, ErrIncomeNotFound
	}

	write := IncomeStoreWrite{
		UserID:                 userID,
		AmountCents:            command.AmountCents,
		Frequency:              frequency,
		MonthlyEquivalentCents: equivalente,
		ExpectedVersion:        command.ExpectedVersion,
		Now:                    s.now().UTC(),
	}

	resultado, err := s.store.StoreIncome(ctx, write)
	if err != nil {
		switch {
		case errors.Is(err, ErrIncomeVersionConflict):
			return nil, ErrIncomeVersionConflict
		case errors.Is(err, ErrIncomeConflict):
			return nil, ErrIncomeConflict
		case errors.Is(err, ErrIncomeNotFound):
			return nil, ErrIncomeNotFound
		case errors.Is(err, ErrIncomeInconsistent):
			return nil, ErrIncomeInconsistent
		case errors.Is(err, ErrIncomeInvalidRequest):
			return nil, ErrIncomeInvalidRequest
		default:
			return nil, ErrIncomeUnavailable
		}
	}
	if resultado == nil {
		return nil, ErrIncomeUnavailable
	}
	return resultado, nil
}
