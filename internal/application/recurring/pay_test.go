package recurring

// Pruebas del caso de uso E037 con un almacén con estado en memoria de prueba.
//
// El doble reproduce el contrato del puerto `PaymentStore`: idempotencia por usuario y clave,
// conflicto si la misma clave describe otra carga, y control de duplicado por fecha civil local.
// No hay DynamoDB ni red: la verificación del adaptador real está en
// `internal/adapters/dynamo/recurring_payment_store_test.go` y la de AWS en el Gate de la oleada.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	appMovements "github.com/nexumpr/Nexum-Backend/internal/application/movements"
)

const (
	e037Usuario = "usuario-e037-0001"
	e037Regla   = "018f90ef-08e5-7a68-b09f-2b67a47c92f0"
	e037Clave   = "clave-idempotencia-e037-0001"
	e037ClaveB  = "clave-idempotencia-e037-0002"
)

var e037Ahora = time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)

// pagoIdemFalso es el registro de idempotencia del doble.
type pagoIdemFalso struct {
	hash   string
	cuerpo []byte
	status int
}

// pagoStoreFalso implementa `PaymentStore` con estado y concurrencia segura.
type pagoStoreFalso struct {
	mu         sync.Mutex
	regla      *Payment
	zona       string
	errZona    error
	errRegla   error
	errPago    error
	colisiones int
	idem       map[string]pagoIdemFalso
	fechas     map[string]bool
	escrituras []PayWrite
}

func nuevoPagoStoreFalso() *pagoStoreFalso {
	return &pagoStoreFalso{
		regla: &Payment{
			ID:          e037Regla,
			Name:        "Renta",
			AmountCents: 1_000_00,
			Currency:    "MXN",
			Type:        appMovements.TypeExpense,
			Active:      true,
			Version:     1,
			NextDueDate: time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC),
			StartDate:   time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC),
		},
		idem:   map[string]pagoIdemFalso{},
		fechas: map[string]bool{},
	}
}

func (s *pagoStoreFalso) LoadRule(_ context.Context, userID, ruleID string) (*Payment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.errRegla != nil {
		return nil, s.errRegla
	}
	if strings.TrimSpace(userID) != e037Usuario || s.regla == nil || s.regla.ID != ruleID {
		return nil, nil
	}
	copia := *s.regla
	return &copia, nil
}

func (s *pagoStoreFalso) LoadTimezone(_ context.Context, userID string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.errZona != nil {
		return "", s.errZona
	}
	if strings.TrimSpace(userID) != e037Usuario {
		return "", nil
	}
	return s.zona, nil
}

func (s *pagoStoreFalso) Pay(_ context.Context, write PayWrite) (*PayResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.errPago != nil {
		return nil, s.errPago
	}
	if s.colisiones > 0 {
		s.colisiones--
		return nil, ErrMovementIDCollision
	}

	clave := write.UserID + "|" + write.Operation + "|" + write.IdempotencyKey
	if previo, ok := s.idem[clave]; ok {
		if previo.hash != write.PayloadHash {
			return nil, ErrConflict
		}
		return &PayResult{Status: previo.status, Body: append([]byte(nil), previo.cuerpo...), Replayed: true}, nil
	}
	if strings.TrimSpace(write.Movement.RecurringPaymentID) != write.RuleID {
		return nil, ErrUnavailable
	}
	if s.regla == nil || !s.regla.Active {
		// Defensa del doble: el servicio ya rechaza antes, pero la transacción real también lo
		// verificaría con su ConditionCheck.
		return nil, ErrNotFound
	}
	llaveFecha := write.UserID + "|" + write.RuleID + "|" + write.LocalDate
	if s.fechas[llaveFecha] {
		return nil, ErrDuplicatePayment
	}
	s.fechas[llaveFecha] = true
	s.idem[clave] = pagoIdemFalso{hash: write.PayloadHash, cuerpo: append([]byte(nil), write.ResponseBody...), status: write.Status}
	s.escrituras = append(s.escrituras, write)
	return &PayResult{Status: write.Status, Body: append([]byte(nil), write.ResponseBody...)}, nil
}

func (s *pagoStoreFalso) escriturasRegistradas() []PayWrite {
	s.mu.Lock()
	defer s.mu.Unlock()
	copia := make([]PayWrite, len(s.escrituras))
	copy(copia, s.escrituras)
	return copia
}

// servicioE037 compone el servicio real con reloj e identificadores deterministas y una zona
// fija equivalente a `America/Mexico_City` (UTC-06:00, sin horario de verano desde 2022), de modo
// que las aserciones no dependan de la base de zonas del host.
func servicioE037(store PaymentStore) *PaymentService {
	var secuencia atomic.Int64
	return NewPaymentService(store).
		WithClock(func() time.Time { return e037Ahora }).
		WithIDGenerator(func() (string, error) {
			n := secuencia.Add(1)
			return fmt.Sprintf("018f90ef-08e5-7a68-b09f-2b67a47c%04d", n), nil
		}).
		WithLocationResolver(func(string) *time.Location {
			return time.FixedZone("America/Mexico_City", -6*60*60)
		})
}

// comandoE037 compone un comando válido.
func comandoE037() PayCommand {
	return PayCommand{
		UserID:         e037Usuario,
		RuleID:         e037Regla,
		IdempotencyKey: e037Clave,
		PaidAt:         "2026-11-05T23:30:00-06:00",
		Amount:         "1000.00",
		Currency:       "MXN",
	}
}

// TestE037PagoValido: 200 con el `Movement` exacto, vinculado a la regla y con la fecha local del
// titular.
func TestE037PagoValido(t *testing.T) {
	store := nuevoPagoStoreFalso()
	resultado, err := servicioE037(store).Pay(context.Background(), comandoE037())
	if err != nil {
		t.Fatalf("el pago valido no debe fallar: %v", err)
	}
	if resultado.Status != PayStatusOK || resultado.Replayed {
		t.Fatalf("resultado inesperado: %+v", resultado)
	}

	var cuerpo map[string]any
	if err := json.Unmarshal(resultado.Body, &cuerpo); err != nil {
		t.Fatalf("la respuesta debe ser JSON: %v", err)
	}
	esperados := map[string]any{
		"type":               appMovements.TypeExpense,
		"amount":             "1000.00",
		"currency":           "MXN",
		"categoryId":         CategoryIDRecurring,
		"description":        "Renta",
		"occurredAt":         "2026-11-06T05:30:00.000Z",
		"version":            float64(1),
		"contributionId":     nil,
		"recurringPaymentId": e037Regla,
	}
	for campo, esperado := range esperados {
		if cuerpo[campo] != esperado {
			t.Fatalf("%s esperado %v, obtenido %v", campo, esperado, cuerpo[campo])
		}
	}
	if id, _ := cuerpo["id"].(string); id == "" {
		t.Fatal("la respuesta debe incluir el identificador del movimiento")
	}
	for _, prohibido := range []string{"userId", "entityType", "PK", "SK", "payloadHash"} {
		if _, existe := cuerpo[prohibido]; existe {
			t.Fatalf("la respuesta no puede exponer %q", prohibido)
		}
	}

	escrituras := store.escriturasRegistradas()
	if len(escrituras) != 1 {
		t.Fatalf("debe persistirse un pago, escrituras=%d", len(escrituras))
	}
	write := escrituras[0]
	if write.LocalDate != "2026-11-05" {
		// 2026-11-05T23:30-06:00 es 2026-11-06T05:30Z: en la zona del titular sigue siendo día 5.
		t.Fatalf("localDate inesperada: %q", write.LocalDate)
	}
	if write.Movement.AmountCents != 1_000_00 || write.Movement.Currency != "MXN" {
		t.Fatalf("movimiento derivado inesperado: %+v", write.Movement)
	}
	if write.Movement.CategoryID != CategoryIDRecurring || write.Movement.Description != "Renta" {
		t.Fatalf("categoria o descripcion inesperadas: %+v", write.Movement)
	}
	if write.Movement.ContributionID != "" {
		t.Fatal("un pago recurrente no declara aportacion")
	}
	if write.Movement.RecurringPaymentID != e037Regla || write.Movement.Version != 1 {
		t.Fatalf("vinculo o version inesperados: %+v", write.Movement)
	}
	if write.Operation != OperationPayRecurring {
		t.Fatalf("operacion de idempotencia inesperada: %q", write.Operation)
	}
	if !write.ExpiresAt.Equal(e037Ahora.Add(IdempotencyTTL)) {
		t.Fatalf("vencimiento de idempotencia inesperado: %s", write.ExpiresAt)
	}
}

// TestE037ReplayMismaClave: mismo cuerpo, una sola escritura, `Replayed` verdadero.
func TestE037ReplayMismaClave(t *testing.T) {
	store := nuevoPagoStoreFalso()
	servicio := servicioE037(store)

	primera, err := servicio.Pay(context.Background(), comandoE037())
	if err != nil {
		t.Fatalf("primer pago: %v", err)
	}
	segunda, err := servicio.Pay(context.Background(), comandoE037())
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if !segunda.Replayed {
		t.Fatal("el segundo intento con la misma clave debe ser un replay")
	}
	if string(segunda.Body) != string(primera.Body) {
		t.Fatalf("el replay debe devolver bytes identicos:\n%s\n%s", primera.Body, segunda.Body)
	}
	if n := len(store.escriturasRegistradas()); n != 1 {
		t.Fatalf("el replay no puede duplicar el pago, escrituras=%d", n)
	}
}

// TestE037MismaClaveOtroPayload: 409 sin escritura.
func TestE037MismaClaveOtroPayload(t *testing.T) {
	store := nuevoPagoStoreFalso()
	servicio := servicioE037(store)

	if _, err := servicio.Pay(context.Background(), comandoE037()); err != nil {
		t.Fatalf("primer pago: %v", err)
	}

	otro := comandoE037()
	otro.Amount = "900.00"
	if _, err := servicio.Pay(context.Background(), otro); !errors.Is(err, ErrConflict) {
		t.Fatalf("se esperaba ErrConflict, se obtuvo %v", err)
	}
	if n := len(store.escriturasRegistradas()); n != 1 {
		t.Fatalf("un conflicto no puede escribir, escrituras=%d", n)
	}
}

// TestE037DuplicadoPorFechaLocal: misma regla y mismo día civil con otra clave -> 409.
func TestE037DuplicadoPorFechaLocal(t *testing.T) {
	store := nuevoPagoStoreFalso()
	servicio := servicioE037(store)

	if _, err := servicio.Pay(context.Background(), comandoE037()); err != nil {
		t.Fatalf("primer pago: %v", err)
	}

	otro := comandoE037()
	otro.IdempotencyKey = e037ClaveB
	otro.PaidAt = "2026-11-05T08:00:00-06:00"
	_, err := servicio.Pay(context.Background(), otro)
	if !errors.Is(err, ErrDuplicatePayment) {
		t.Fatalf("se esperaba ErrDuplicatePayment, se obtuvo %v", err)
	}
	if !errors.Is(err, ErrConflict) {
		t.Fatal("el duplicado debe seguir siendo un conflicto del contrato (409)")
	}
	if n := len(store.escriturasRegistradas()); n != 1 {
		t.Fatalf("un duplicado no puede escribir, escrituras=%d", n)
	}
}

// TestE037ReglaNoDisponible: inexistente -> 404; inactiva -> 409; error de lectura -> 503.
func TestE037ReglaNoDisponible(t *testing.T) {
	casos := []struct {
		nombre   string
		preparar func(*pagoStoreFalso)
		esperado error
	}{
		{"inexistente", func(s *pagoStoreFalso) { s.regla = nil }, ErrNotFound},
		{"ajena", func(s *pagoStoreFalso) { s.regla.ID = "018f90ef-08e5-7a68-b09f-2b67a47c9fff" }, ErrNotFound},
		{"inactiva", func(s *pagoStoreFalso) { s.regla.Active = false }, ErrRuleInactive},
		{"lectura fallida", func(s *pagoStoreFalso) { s.errRegla = errors.New("dynamodb caido") }, ErrUnavailable},
		{"tipo invalido", func(s *pagoStoreFalso) { s.regla.Type = "fixed" }, ErrUnavailable},
		{"zona no disponible", func(s *pagoStoreFalso) { s.errZona = errors.New("dynamodb caido") }, ErrUnavailable},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			store := nuevoPagoStoreFalso()
			caso.preparar(store)
			_, err := servicioE037(store).Pay(context.Background(), comandoE037())
			if !errors.Is(err, caso.esperado) {
				t.Fatalf("se esperaba %v, se obtuvo %v", caso.esperado, err)
			}
			if n := len(store.escriturasRegistradas()); n != 0 {
				t.Fatalf("una regla no pagable no puede escribir, escrituras=%d", n)
			}
		})
	}
}

// TestE037FechaYZona comprueba el día civil del titular y el respaldo de zona.
func TestE037FechaYZona(t *testing.T) {
	casos := []struct {
		nombre    string
		zona      string
		paidAt    string
		localDate string
		ocurrido  string
	}{
		{"23:30 del dia 5", "", "2026-11-05T23:30:00-06:00", "2026-11-05", "2026-11-06T05:30:00.000Z"},
		{"00:30 del dia 6", "", "2026-11-06T00:30:00-06:00", "2026-11-06", "2026-11-06T06:30:00.000Z"},
		{"zona invalida cae a la de defecto", "Not/AZone", "2026-11-05T23:30:00-06:00", "2026-11-05", "2026-11-06T05:30:00.000Z"},
		{"zona local no es IANA", "Local", "2026-11-05T23:30:00-06:00", "2026-11-05", "2026-11-06T05:30:00.000Z"},
		{"zona del titular", "America/Mexico_City", "2026-11-05T23:30:00-06:00", "2026-11-05", "2026-11-06T05:30:00.000Z"},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			store := nuevoPagoStoreFalso()
			store.zona = caso.zona
			comando := comandoE037()
			comando.PaidAt = caso.paidAt
			// Resolutor real: valida que una zona ausente o invalida use el respaldo del Gate.
			servicio := NewPaymentService(store).
				WithClock(func() time.Time { return e037Ahora }).
				WithIDGenerator(func() (string, error) { return "018f90ef-08e5-7a68-b09f-2b67a47c92f1", nil })

			if _, err := servicio.Pay(context.Background(), comando); err != nil {
				t.Fatalf("pago: %v", err)
			}
			escrituras := store.escriturasRegistradas()
			if len(escrituras) != 1 {
				t.Fatalf("escrituras=%d", len(escrituras))
			}
			if escrituras[0].LocalDate != caso.localDate {
				t.Fatalf("localDate esperada %q, obtenida %q", caso.localDate, escrituras[0].LocalDate)
			}
			if got := appMovements.FormatOccurredAt(escrituras[0].Movement.OccurredAt); got != caso.ocurrido {
				t.Fatalf("occurredAt esperado %q, obtenido %q", caso.ocurrido, got)
			}
		})
	}
}

// TestE037DosFechasLocalesDistintasNoSonDuplicado: el cambio de día UTC no bloquea el pago del
// día siguiente, y un instante UTC del día 6 puede pertenecer al día 5 local.
func TestE037DosFechasLocalesDistintasNoSonDuplicado(t *testing.T) {
	store := nuevoPagoStoreFalso()
	servicio := servicioE037(store)

	if _, err := servicio.Pay(context.Background(), comandoE037()); err != nil {
		t.Fatalf("pago del dia 5 local: %v", err)
	}

	siguiente := comandoE037()
	siguiente.IdempotencyKey = e037ClaveB
	siguiente.PaidAt = "2026-11-06T00:30:00-06:00"
	if _, err := servicio.Pay(context.Background(), siguiente); err != nil {
		t.Fatalf("el dia local 6 no es duplicado del 5: %v", err)
	}

	escrituras := store.escriturasRegistradas()
	if len(escrituras) != 2 || escrituras[0].LocalDate == escrituras[1].LocalDate {
		t.Fatalf("se esperaban dos fechas locales distintas: %+v", escrituras)
	}
}

// TestE037ImporteYFechaInvalidos separa 400 (forma) de 422 (regla de negocio).
func TestE037ImporteYFechaInvalidos(t *testing.T) {
	casos := []struct {
		nombre    string
		modificar func(*PayCommand)
		esperado  error
		campo     string
	}{
		{"importe no decimal", func(c *PayCommand) { c.Amount = "1000" }, ErrInvalidRequest, ""},
		{"importe con tres decimales", func(c *PayCommand) { c.Amount = "1000.000" }, ErrInvalidRequest, ""},
		{"importe negativo", func(c *PayCommand) { c.Amount = "-1000.00" }, ErrInvalidRequest, ""},
		{"importe con ceros a la izquierda", func(c *PayCommand) { c.Amount = "01000.00" }, ErrInvalidRequest, ""},
		{"importe de trece digitos", func(c *PayCommand) { c.Amount = "1000000000000.00" }, ErrInvalidRequest, ""},
		{"importe cero", func(c *PayCommand) { c.Amount = "0.00" }, &Violation{}, "amount"},
		{"importe ausente", func(c *PayCommand) { c.Amount = "" }, &Violation{}, "amount"},
		{"fecha sin hora", func(c *PayCommand) { c.PaidAt = "2026-11-05" }, ErrInvalidRequest, ""},
		{"fecha sin zona", func(c *PayCommand) { c.PaidAt = "2026-11-05T23:30:00" }, ErrInvalidRequest, ""},
		{"fecha muy futura", func(c *PayCommand) { c.PaidAt = "2026-11-08T00:00:00Z" }, &Violation{}, "paidAt"},
		{"fecha fuera de rango", func(c *PayCommand) { c.PaidAt = "1969-12-31T23:59:59Z" }, &Violation{}, "paidAt"},
		{"divisa no publicada", func(c *PayCommand) { c.Currency = "USD" }, &Violation{}, "currency"},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			store := nuevoPagoStoreFalso()
			comando := comandoE037()
			caso.modificar(&comando)
			_, err := servicioE037(store).Pay(context.Background(), comando)
			if err == nil {
				t.Fatal("la solicitud debia rechazarse")
			}
			if _, esViolacion := caso.esperado.(*Violation); esViolacion {
				var violacion *Violation
				if !errors.As(err, &violacion) {
					t.Fatalf("se esperaba una violacion de negocio, se obtuvo %v", err)
				}
				if violacion.Field != caso.campo {
					t.Fatalf("campo esperado %q, obtenido %q", caso.campo, violacion.Field)
				}
			} else if !errors.Is(err, caso.esperado) {
				t.Fatalf("se esperaba %v, se obtuvo %v", caso.esperado, err)
			}
			if n := len(store.escriturasRegistradas()); n != 0 {
				t.Fatalf("una solicitud invalida no puede escribir, escrituras=%d", n)
			}
		})
	}
}

// TestE037SolicitudNoAutorizada: sin identidad, sin clave o sin regla no se toca la persistencia.
func TestE037SolicitudNoAutorizada(t *testing.T) {
	casos := []struct {
		nombre    string
		modificar func(*PayCommand)
		esperado  error
	}{
		{"sin usuario", func(c *PayCommand) { c.UserID = "  " }, ErrUnauthorized},
		{"sin clave", func(c *PayCommand) { c.IdempotencyKey = "" }, ErrInvalidRequest},
		{"clave corta", func(c *PayCommand) { c.IdempotencyKey = "corta" }, ErrInvalidRequest},
		{"clave larga", func(c *PayCommand) { c.IdempotencyKey = strings.Repeat("x", 129) }, ErrInvalidRequest},
		{"sin regla", func(c *PayCommand) { c.RuleID = "" }, ErrInvalidRequest},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			store := nuevoPagoStoreFalso()
			comando := comandoE037()
			caso.modificar(&comando)
			if _, err := servicioE037(store).Pay(context.Background(), comando); !errors.Is(err, caso.esperado) {
				t.Fatalf("se esperaba %v, se obtuvo %v", caso.esperado, err)
			}
			if n := len(store.escriturasRegistradas()); n != 0 {
				t.Fatalf("no puede escribir, escrituras=%d", n)
			}
		})
	}
}

// TestE037ColisionDeIDReintentaAcotado: una colisión del identificador no es un 409 del cliente.
func TestE037ColisionDeIDReintentaAcotado(t *testing.T) {
	store := nuevoPagoStoreFalso()
	store.colisiones = 2
	if _, err := servicioE037(store).Pay(context.Background(), comandoE037()); err != nil {
		t.Fatalf("tras dos colisiones el tercer intento debe confirmar: %v", err)
	}

	store = nuevoPagoStoreFalso()
	store.colisiones = payIDAttempts
	if _, err := servicioE037(store).Pay(context.Background(), comandoE037()); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("agotados los intentos debe fallar cerrado, se obtuvo %v", err)
	}
	if n := len(store.escriturasRegistradas()); n != 0 {
		t.Fatalf("sin confirmacion no puede haber escritura, escrituras=%d", n)
	}
}

// TestE037ConcurrenciaSoloUnGana: dos pagos simultáneos con claves distintas y la misma fecha
// local; sólo uno se persiste.
func TestE037ConcurrenciaSoloUnGana(t *testing.T) {
	store := nuevoPagoStoreFalso()
	servicio := servicioE037(store)

	comandos := []PayCommand{comandoE037(), comandoE037()}
	comandos[1].IdempotencyKey = e037ClaveB

	type salida struct{ err error }
	canal := make(chan salida, len(comandos))
	inicio := make(chan struct{})
	for _, comando := range comandos {
		comando := comando
		go func() {
			<-inicio
			_, err := servicio.Pay(context.Background(), comando)
			canal <- salida{err: err}
		}()
	}
	close(inicio)

	exitos, duplicados := 0, 0
	for i := 0; i < len(comandos); i++ {
		switch resultado := <-canal; {
		case resultado.err == nil:
			exitos++
		case errors.Is(resultado.err, ErrDuplicatePayment):
			duplicados++
		default:
			t.Fatalf("error inesperado en la carrera: %v", resultado.err)
		}
	}
	if exitos != 1 || duplicados != 1 {
		t.Fatalf("exactamente un pago debe ganar: exitos=%d duplicados=%d", exitos, duplicados)
	}
	if n := len(store.escriturasRegistradas()); n != 1 {
		t.Fatalf("debe persistirse un solo pago, escrituras=%d", n)
	}
}

// TestE037FallaCerradoSinAlmacen: sin puerto no se confirma nada.
func TestE037FallaCerradoSinAlmacen(t *testing.T) {
	if _, err := NewPaymentService(nil).Pay(context.Background(), comandoE037()); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("sin almacen debe devolver ErrUnavailable, se obtuvo %v", err)
	}
}

// TestE037ResolucionDeZonaDelRespaldo comprueba que la zona por defecto no depende de tzdata: el
// respaldo es UTC-06:00, el desplazamiento civil de México desde 2022.
func TestE037ResolucionDeZonaDelRespaldo(t *testing.T) {
	instante := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
	for _, nombre := range []string{"", "   ", "Local", "Not/AZone", DefaultTimezone} {
		zona := loadLocation(nombre)
		if zona == nil {
			t.Fatalf("la zona %q no puede ser nula", nombre)
		}
		if _, offset := instante.In(zona).Zone(); offset != -6*60*60 {
			t.Fatalf("zona %q: offset esperado -21600, obtenido %d", nombre, offset)
		}
	}
	if got := instante.In(loadLocation(DefaultTimezone)).Format(LocalDateLayout); got != "2026-11-05" {
		t.Fatalf("fecha civil inesperada: %q", got)
	}
}
