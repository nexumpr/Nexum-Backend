package application_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	appRecurring "github.com/nexumpr/Nexum-Backend/internal/application/recurring"
	domainIdentity "github.com/nexumpr/Nexum-Backend/internal/domain/identity"
)

// Pruebas de aplicación de E033 (`POST /api/v1/recurring-payments`): aceptación de los nueve
// valores de frecuencia publicados con emisión de los siete canónicos (RFC-021), importe
// positivo en céntimos, coherencia de fechas, idempotencia con replay/409, aislamiento entre
// usuarios y composición fail-closed. El almacén es un doble de prueba; la composición
// productiva usa DynamoDB.

const (
	e033Usuario = "11111111-2222-4333-8444-555555555555"
	e033Otro    = "99999999-8888-4777-8666-555555555555"
	e033Clave   = "clave-idempotencia-e033-0001"
	e033Creado  = "2026-11-18T10:00:00Z"
)

var e033Ahora = time.Date(2026, 11, 18, 10, 0, 0, 0, time.UTC)

// e033RegistroIdem es el registro de idempotencia del doble.
type e033RegistroIdem struct {
	hash string
	id   string
	body []byte
}

// e033Store implementa `appRecurring.Store` con estado en memoria.
type e033Store struct {
	mu         sync.Mutex
	recursos   map[string]appRecurring.Payment
	idem       map[string]e033RegistroIdem
	escrituras []appRecurring.CreateWrite
	consultas  int
	fallo      error
}

func newE033Store() *e033Store {
	return &e033Store{
		recursos: map[string]appRecurring.Payment{},
		idem:     map[string]e033RegistroIdem{},
	}
}

func e033ClaveRecurso(userID, id string) string { return userID + "|" + id }

func (s *e033Store) Create(_ context.Context, write appRecurring.CreateWrite) (string, []byte, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fallo != nil {
		return "", nil, false, s.fallo
	}
	clave := write.UserID + "|" + write.IdempotencyKey
	if existente, ok := s.idem[clave]; ok {
		if existente.hash != write.PayloadHash {
			return "", nil, false, appRecurring.ErrConflict
		}
		return existente.id, existente.body, true, nil
	}
	s.escrituras = append(s.escrituras, write)
	s.recursos[e033ClaveRecurso(write.UserID, write.Payment.ID)] = write.Payment
	s.idem[clave] = e033RegistroIdem{hash: write.PayloadHash, id: write.Payment.ID, body: write.Body}
	return write.Payment.ID, write.Body, false, nil
}

func (s *e033Store) List(_ context.Context, userID, startSK string, limit int, filter appRecurring.Filter) ([]appRecurring.Payment, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.consultas++
	if s.fallo != nil {
		return nil, "", s.fallo
	}
	claves := make([]string, 0, len(s.recursos))
	for clave := range s.recursos {
		if strings.HasPrefix(clave, userID+"|") {
			claves = append(claves, clave)
		}
	}
	sort.Strings(claves)

	salida := make([]appRecurring.Payment, 0, limit)
	nextSK := ""
	for _, clave := range claves {
		pago := s.recursos[clave]
		if startSK != "" && pago.ID <= startSK {
			continue
		}
		if filter.Active != nil && pago.Active != *filter.Active {
			continue
		}
		if filter.Type != "" && pago.Type != filter.Type {
			continue
		}
		if len(salida) == limit {
			nextSK = salida[len(salida)-1].ID
			break
		}
		salida = append(salida, pago)
	}
	return salida, nextSK, nil
}

func (s *e033Store) Get(_ context.Context, userID, paymentID string) (*appRecurring.Payment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.consultas++
	if s.fallo != nil {
		return nil, s.fallo
	}
	pago, ok := s.recursos[e033ClaveRecurso(userID, paymentID)]
	if !ok {
		return nil, nil
	}
	copia := pago
	return &copia, nil
}

func (s *e033Store) Update(_ context.Context, write appRecurring.UpdateWrite) (*appRecurring.Payment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fallo != nil {
		return nil, s.fallo
	}
	clave := e033ClaveRecurso(write.UserID, write.PaymentID)
	actual, ok := s.recursos[clave]
	if !ok {
		return nil, appRecurring.ErrNotFound
	}
	if actual.Version != write.ExpectedVersion {
		return nil, appRecurring.ErrVersionConflict
	}
	actualizado := write.Payment
	actualizado.ID = write.PaymentID
	actualizado.Version = actual.Version + 1
	s.recursos[clave] = actualizado
	copia := actualizado
	return &copia, nil
}

func (s *e033Store) Deactivate(_ context.Context, userID, paymentID string, _ time.Time) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fallo != nil {
		return false, s.fallo
	}
	clave := e033ClaveRecurso(userID, paymentID)
	actual, ok := s.recursos[clave]
	if !ok {
		return false, nil
	}
	if actual.Active {
		actual.Active = false
		actual.Version++
		s.recursos[clave] = actual
	}
	return true, nil
}

// e033Generador produce identificadores deterministas y distintos.
func e033Generador() func() (string, error) {
	var secuencia int
	return func() (string, error) {
		secuencia++
		return fmt.Sprintf("018f90ef-08e5-7a68-b09f-2b67a4%08d", secuencia), nil
	}
}

// e033Servicio construye el servicio con reloj, generador y clave de firma de cursor
// deterministas. La clave es de prueba: la composición productiva la deriva del bundle
// RFC-004 con separación de dominio.
func e033Servicio(store appRecurring.Store) *appRecurring.Service {
	return appRecurring.NewService(store, []byte("clave-de-cursor-de-prueba-e033")).
		WithClock(func() time.Time { return e033Ahora }).
		WithIDGenerator(e033Generador())
}

// e033Comando es el comando válido de referencia: mensual, próximo vencimiento el 5.
func e033Comando() appRecurring.CreateCommand {
	return appRecurring.CreateCommand{
		UserID:         e033Usuario,
		IdempotencyKey: e033Clave,
		Name:           "Renta",
		AmountCents:    100000,
		Currency:       "MXN",
		Frequency:      "monthly",
		NextDueDate:    "2026-11-05",
		Type:           "expense",
		StartDate:      "2026-10-05",
	}
}

// TestE033NormalizaNueveFrecuencias comprueba RFC-021: se aceptan los nueve valores
// publicados y se persiste y emite SIEMPRE el canónico.
func TestE033NormalizaNueveFrecuencias(t *testing.T) {
	casos := []struct {
		entrada  string
		canonica string
		proximo  string
		inicio   string
	}{
		{"weekly", "weekly", "2026-11-08", "2026-11-01"},
		{"fortnightly", "fortnightly", "2026-11-15", "2026-11-01"},
		{"biweekly", "fortnightly", "2026-11-15", "2026-11-01"},
		{"monthly", "monthly", "2026-11-05", "2026-10-05"},
		{"bimonthly", "bimonthly", "2026-12-05", "2026-10-05"},
		{"quarterly", "quarterly", "2027-01-05", "2026-10-05"},
		{"semiannual", "semiannual", "2027-04-05", "2026-10-05"},
		{"annual", "annual", "2027-10-05", "2026-10-05"},
		{"yearly", "annual", "2027-10-05", "2026-10-05"},
	}
	for _, caso := range casos {
		t.Run(caso.entrada, func(t *testing.T) {
			store := newE033Store()
			servicio := e033Servicio(store)

			comando := e033Comando()
			comando.Frequency = caso.entrada
			comando.NextDueDate = caso.proximo
			comando.StartDate = caso.inicio

			resultado, err := servicio.Create(context.Background(), comando)
			if err != nil {
				t.Fatalf("la frecuencia %q debe aceptarse: %v", caso.entrada, err)
			}
			if len(store.escrituras) != 1 {
				t.Fatalf("debe persistirse exactamente una regla, obtenido %d", len(store.escrituras))
			}
			persistida := store.escrituras[0].Payment
			if persistida.Frequency.String() != caso.canonica {
				t.Fatalf("frecuencia persistida %q, esperada %q", persistida.Frequency, caso.canonica)
			}
			if persistida.Version != 1 || !persistida.Active {
				t.Fatalf("la regla nace activa y en version 1: %+v", persistida)
			}

			var cuerpo map[string]any
			if err := json.Unmarshal(resultado.Body, &cuerpo); err != nil {
				t.Fatalf("respuesta no es JSON: %v", err)
			}
			if cuerpo["frequency"] != caso.canonica {
				t.Fatalf("la respuesta debe emitir %q, obtenido %v", caso.canonica, cuerpo["frequency"])
			}
		})
	}
}

// TestE033RechazaFrecuenciaFueraDelConjunto cubre el 422 de frecuencia.
//
// La comparación es insensible a mayúsculas porque la normalización es la MISMA que usan
// E024/E025 (una sola tabla, RFC-020 §2): `MONTHLY` se acepta como `monthly`. Lo que se
// rechaza es un valor que no pertenece al conjunto publicado.
func TestE033RechazaFrecuenciaFueraDelConjunto(t *testing.T) {
	for _, valor := range []string{"daily", "once", "quincenal", "semanal", "monthlyy"} {
		t.Run(valor, func(t *testing.T) {
			store := newE033Store()
			comando := e033Comando()
			comando.Frequency = valor

			_, err := e033Servicio(store).Create(context.Background(), comando)
			var violacion *appRecurring.Violation
			if !errors.As(err, &violacion) || violacion.Field != "frequency" {
				t.Fatalf("se esperaba violacion de frequency, obtenido %v", err)
			}
			if len(store.escrituras) != 0 {
				t.Fatal("una frecuencia invalida no puede escribir nada")
			}
		})
	}
}

// TestE033ValidaImporteNombreYDivisa cubre las reglas de negocio del payload.
func TestE033ValidaImporteNombreYDivisa(t *testing.T) {
	casos := []struct {
		nombre string
		campo  string
		modif  func(*appRecurring.CreateCommand)
	}{
		{"importe cero", "amount", func(c *appRecurring.CreateCommand) { c.AmountCents = 0 }},
		{"importe negativo", "amount", func(c *appRecurring.CreateCommand) { c.AmountCents = -1 }},
		{"importe fuera del maximo", "amount", func(c *appRecurring.CreateCommand) { c.AmountCents = 100_000_000_000_000 }},
		{"nombre vacio", "name", func(c *appRecurring.CreateCommand) { c.Name = "   " }},
		{"divisa distinta", "currency", func(c *appRecurring.CreateCommand) { c.Currency = "USD" }},
		{"tipo desconocido", "type", func(c *appRecurring.CreateCommand) { c.Type = "fixed" }},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			store := newE033Store()
			comando := e033Comando()
			caso.modif(&comando)

			_, err := e033Servicio(store).Create(context.Background(), comando)
			var violacion *appRecurring.Violation
			if !errors.As(err, &violacion) || violacion.Field != caso.campo {
				t.Fatalf("se esperaba violacion de %q, obtenido %v", caso.campo, err)
			}
			if len(store.escrituras) != 0 {
				t.Fatal("una regla invalida no puede escribir nada")
			}
		})
	}
}

// TestE033ValidaCoherenciaDeFechas comprueba la cadencia anclada en `startDate`, incluido el
// recorte de fin de mes propio de las frecuencias de calendario.
func TestE033ValidaCoherenciaDeFechas(t *testing.T) {
	casos := []struct {
		nombre     string
		frecuencia string
		inicio     string
		proximo    string
		valido     bool
	}{
		{"semanal +7d", "weekly", "2026-11-01", "2026-11-08", true},
		{"semanal +14d", "weekly", "2026-11-01", "2026-11-15", true},
		{"semanal +8d", "weekly", "2026-11-01", "2026-11-09", false},
		{"quincenal +14d", "fortnightly", "2026-11-01", "2026-11-15", true},
		{"quincenal +7d", "fortnightly", "2026-11-01", "2026-11-08", false},
		{"mensual calendario", "monthly", "2026-10-05", "2026-11-05", true},
		{"mensual 31 a 28", "monthly", "2026-01-31", "2026-02-28", true},
		{"mensual 31 a 27", "monthly", "2026-01-31", "2026-02-27", false},
		{"trimestral", "quarterly", "2026-10-05", "2027-01-05", true},
		{"anual", "annual", "2026-10-05", "2027-10-05", true},
		{"anterior al inicio", "monthly", "2026-11-05", "2026-10-05", false},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			store := newE033Store()
			comando := e033Comando()
			comando.Frequency = caso.frecuencia
			comando.StartDate = caso.inicio
			comando.NextDueDate = caso.proximo

			_, err := e033Servicio(store).Create(context.Background(), comando)
			if caso.valido {
				if err != nil {
					t.Fatalf("se esperaba aceptacion: %v", err)
				}
				return
			}
			var violacion *appRecurring.Violation
			if !errors.As(err, &violacion) || violacion.Field != "nextDueDate" {
				t.Fatalf("se esperaba violacion de nextDueDate, obtenido %v", err)
			}
		})
	}
}

// TestE033ValidaDueDay cubre el rango publicado y la coherencia con la próxima ocurrencia.
func TestE033ValidaDueDay(t *testing.T) {
	casos := []struct {
		nombre     string
		frecuencia string
		proximo    string
		inicio     string
		dueDay     int
		valido     bool
	}{
		{"mensual coincide", "monthly", "2026-11-05", "2026-10-05", 5, true},
		{"mensual no coincide", "monthly", "2026-11-05", "2026-10-05", 6, false},
		{"semanal no aplica", "weekly", "2026-11-08", "2026-11-01", 5, false},
		{"trimestral coincide", "quarterly", "2027-01-05", "2026-10-05", 5, true},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			store := newE033Store()
			comando := e033Comando()
			comando.Frequency = caso.frecuencia
			comando.NextDueDate = caso.proximo
			comando.StartDate = caso.inicio
			dia := caso.dueDay
			comando.DueDay = &dia

			_, err := e033Servicio(store).Create(context.Background(), comando)
			if caso.valido {
				if err != nil {
					t.Fatalf("se esperaba aceptacion: %v", err)
				}
				return
			}
			var violacion *appRecurring.Violation
			if !errors.As(err, &violacion) || violacion.Field != "dueDay" {
				t.Fatalf("se esperaba violacion de dueDay, obtenido %v", err)
			}
		})
	}
}

// TestE033IdempotenciaReplayYConflicto cubre T07: misma clave y payload -> mismo cuerpo;
// misma clave con otro payload -> 409.
func TestE033IdempotenciaReplayYConflicto(t *testing.T) {
	store := newE033Store()
	servicio := e033Servicio(store)

	primero, err := servicio.Create(context.Background(), e033Comando())
	if err != nil {
		t.Fatalf("creacion inicial: %v", err)
	}
	if primero.Replayed {
		t.Fatal("la primera peticion no es un replay")
	}

	segundo, err := servicio.Create(context.Background(), e033Comando())
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if !segundo.Replayed || segundo.ID != primero.ID {
		t.Fatalf("el replay debe devolver el mismo recurso: %+v", segundo)
	}
	if string(segundo.Body) != string(primero.Body) {
		t.Fatal("el replay debe devolver bytes identicos")
	}
	if len(store.escrituras) != 1 {
		t.Fatalf("el replay no puede volver a escribir, escrituras=%d", len(store.escrituras))
	}

	distinto := e033Comando()
	distinto.Name = "Renta ajustada"
	_, err = servicio.Create(context.Background(), distinto)
	if !errors.Is(err, appRecurring.ErrConflict) {
		t.Fatalf("se esperaba conflicto de idempotencia, obtenido %v", err)
	}
}

// TestE033RequiereIdempotencyKey exige la clave y su longitud publicada.
func TestE033RequiereIdempotencyKey(t *testing.T) {
	for _, clave := range []string{"", "corta", strings.Repeat("x", 129)} {
		store := newE033Store()
		comando := e033Comando()
		comando.IdempotencyKey = clave

		_, err := e033Servicio(store).Create(context.Background(), comando)
		if !errors.Is(err, appRecurring.ErrInvalidRequest) {
			t.Fatalf("clave %q debe rechazarse, obtenido %v", clave, err)
		}
		if len(store.escrituras) != 0 {
			t.Fatal("sin clave valida no se escribe nada")
		}
	}
}

// TestE033AislamientoEntreUsuarios: dos cuentas con la MISMA Idempotency-Key crean recursos
// distintos y ninguna puede leer el del otro.
func TestE033AislamientoEntreUsuarios(t *testing.T) {
	store := newE033Store()
	servicio := e033Servicio(store)

	comandoA := e033Comando()
	resultadoA, err := servicio.Create(context.Background(), comandoA)
	if err != nil {
		t.Fatalf("alta de A: %v", err)
	}

	comandoB := e033Comando()
	comandoB.UserID = e033Otro
	resultadoB, err := servicio.Create(context.Background(), comandoB)
	if err != nil {
		t.Fatalf("alta de B: %v", err)
	}
	if resultadoA.ID == resultadoB.ID {
		t.Fatal("dos cuentas no pueden compartir el registro de idempotencia")
	}
	if len(store.escrituras) != 2 {
		t.Fatalf("cada cuenta debe tener su propia regla, escrituras=%d", len(store.escrituras))
	}

	if _, err := servicio.Get(context.Background(), e033Otro, resultadoA.ID); !errors.Is(err, appRecurring.ErrNotFound) {
		t.Fatalf("B no puede leer la regla de A, obtenido %v", err)
	}
	pagina, err := servicio.List(context.Background(), appRecurring.ListQuery{UserID: e033Otro, Limit: 25})
	if err != nil {
		t.Fatalf("listado de B: %v", err)
	}
	if len(pagina.Items) != 1 || pagina.Items[0].ID != resultadoB.ID {
		t.Fatalf("el listado de B solo debe contener su regla: %+v", pagina.Items)
	}
}

// TestE033NoGeneraPago: la creación registra la regla y nada más.
func TestE033NoGeneraPago(t *testing.T) {
	store := newE033Store()
	if _, err := e033Servicio(store).Create(context.Background(), e033Comando()); err != nil {
		t.Fatalf("alta: %v", err)
	}
	if len(store.escrituras) != 1 {
		t.Fatalf("una sola transaccion, obtenido %d", len(store.escrituras))
	}
	if len(store.recursos) != 1 {
		t.Fatalf("una sola regla persistida, obtenido %d", len(store.recursos))
	}
}

// TestE033FallaCerrado comprueba que sin almacén o con un fallo de persistencia no se
// confirma nada.
func TestE033FallaCerrado(t *testing.T) {
	if _, err := appRecurring.NewService(nil, nil).Create(context.Background(), e033Comando()); !errors.Is(err, appRecurring.ErrUnavailable) {
		t.Fatalf("sin almacen se esperaba ErrUnavailable, obtenido %v", err)
	}

	store := newE033Store()
	store.fallo = errors.New("dynamodb no disponible")
	if _, err := e033Servicio(store).Create(context.Background(), e033Comando()); !errors.Is(err, appRecurring.ErrUnavailable) {
		t.Fatalf("con fallo de persistencia se esperaba ErrUnavailable, obtenido %v", err)
	}
}

// TestE033SerializaContratoExacto valida que el cuerpo tenga exactamente los once campos del
// contrato y ninguno interno.
func TestE033SerializaContratoExacto(t *testing.T) {
	store := newE033Store()
	resultado, err := e033Servicio(store).Create(context.Background(), e033Comando())
	if err != nil {
		t.Fatalf("alta: %v", err)
	}

	var cuerpo map[string]any
	if err := json.Unmarshal(resultado.Body, &cuerpo); err != nil {
		t.Fatalf("respuesta no es JSON: %v", err)
	}
	esperados := []string{"id", "name", "amount", "currency", "frequency", "nextDueDate", "active", "version", "type", "startDate", "dueDay"}
	if len(cuerpo) != len(esperados) {
		t.Fatalf("se esperaban %d campos, obtenidos %d: %v", len(esperados), len(cuerpo), cuerpo)
	}
	for _, campo := range esperados {
		if _, ok := cuerpo[campo]; !ok {
			t.Fatalf("falta el campo %q", campo)
		}
	}
	for _, prohibido := range []string{"userId", "entityType", "PK", "SK", "createdAt", "concept"} {
		if _, ok := cuerpo[prohibido]; ok {
			t.Fatalf("la respuesta no puede exponer %q", prohibido)
		}
	}
	if cuerpo["amount"] != "1000.00" {
		t.Fatalf("el importe debe serializarse como cadena decimal: %v", cuerpo["amount"])
	}
	if cuerpo["dueDay"] != nil {
		t.Fatalf("dueDay ausente debe emitirse como null: %v", cuerpo["dueDay"])
	}
}

// TestE033FrecuenciaCanonicaCoincideConElCatalogo garantiza que la normalización de E033 y el
// catálogo público E023 no derivan (RFC-021 §6).
func TestE033FrecuenciaCanonicaCoincideConElCatalogo(t *testing.T) {
	canonicos := map[string]bool{}
	for _, frecuencia := range domainIdentity.AllowedFrequencies() {
		canonicos[frecuencia.String()] = true
	}
	for _, entrada := range []string{"weekly", "fortnightly", "biweekly", "monthly", "bimonthly", "quarterly", "semiannual", "annual", "yearly"} {
		comando := e033Comando()
		comando.Frequency = entrada
		switch entrada {
		case "weekly":
			comando.StartDate = "2026-11-01"
			comando.NextDueDate = "2026-11-08"
		case "fortnightly", "biweekly":
			comando.StartDate = "2026-11-01"
			comando.NextDueDate = "2026-11-15"
		case "monthly":
			comando.StartDate = "2026-10-05"
			comando.NextDueDate = "2026-11-05"
		case "bimonthly":
			comando.StartDate = "2026-10-05"
			comando.NextDueDate = "2026-12-05"
		case "quarterly":
			comando.StartDate = "2026-10-05"
			comando.NextDueDate = "2027-01-05"
		case "semiannual":
			comando.StartDate = "2026-10-05"
			comando.NextDueDate = "2027-04-05"
		default:
			comando.StartDate = "2026-10-05"
			comando.NextDueDate = "2027-10-05"
		}
		store := newE033Store()
		if _, err := e033Servicio(store).Create(context.Background(), comando); err != nil {
			t.Fatalf("%q debe aceptarse: %v", entrada, err)
		}
		obtenida := store.escrituras[0].Payment.Frequency.String()
		if !canonicos[obtenida] {
			t.Fatalf("%q se normalizo a %q, que el catalogo E023 no emite", entrada, obtenida)
		}
	}
}

// TestE034DetallePropiedadYErrores cubre E034 en la capa de aplicación: el detalle propio se
// devuelve, el de otra cuenta y el inexistente son el mismo ErrNotFound, y la falta de
// identidad o de persistencia falla cerrada.
func TestE034DetallePropiedadYErrores(t *testing.T) {
	store := newE033Store()
	servicio := e033Servicio(store)

	creado, err := servicio.Create(context.Background(), e033Comando())
	if err != nil {
		t.Fatalf("alta: %v", err)
	}

	propio, err := servicio.Get(context.Background(), e033Usuario, creado.ID)
	if err != nil {
		t.Fatalf("detalle propio: %v", err)
	}
	if propio.ID != creado.ID || propio.Name != "Renta" || propio.Version != 1 {
		t.Fatalf("detalle inesperado: %+v", propio)
	}

	if _, err := servicio.Get(context.Background(), e033Otro, creado.ID); !errors.Is(err, appRecurring.ErrNotFound) {
		t.Fatalf("el recurso ajeno debe ser ErrNotFound, obtenido %v", err)
	}
	inexistente := "018f90ef-08e5-7a68-b09f-2b67a4ffffffffff"
	if _, err := servicio.Get(context.Background(), e033Usuario, inexistente); !errors.Is(err, appRecurring.ErrNotFound) {
		t.Fatalf("el recurso inexistente debe ser ErrNotFound, obtenido %v", err)
	}
	if _, err := servicio.Get(context.Background(), "   ", creado.ID); !errors.Is(err, appRecurring.ErrUnauthorized) {
		t.Fatalf("sin identidad se esperaba ErrUnauthorized, obtenido %v", err)
	}
	if _, err := servicio.Get(context.Background(), e033Usuario, "  "); !errors.Is(err, appRecurring.ErrInvalidRequest) {
		t.Fatalf("sin identificador se esperaba ErrInvalidRequest, obtenido %v", err)
	}

	store.fallo = errors.New("dynamodb no disponible")
	if _, err := servicio.Get(context.Background(), e033Usuario, creado.ID); !errors.Is(err, appRecurring.ErrUnavailable) {
		t.Fatalf("con fallo de persistencia se esperaba ErrUnavailable, obtenido %v", err)
	}
	if _, err := appRecurring.NewService(nil, nil).Get(context.Background(), e033Usuario, creado.ID); !errors.Is(err, appRecurring.ErrUnavailable) {
		t.Fatalf("sin almacen se esperaba ErrUnavailable, obtenido %v", err)
	}
}
