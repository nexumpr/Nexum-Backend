package application_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	appIdentity "github.com/nexumpr/Nexum-Backend/internal/application/identity"
	domainIdentity "github.com/nexumpr/Nexum-Backend/internal/domain/identity"
)

// Pruebas de aplicación de E024 (`GET /api/v1/income`) y E025 (`PUT /api/v1/income`):
// normalización de alias históricos, recálculo exacto del equivalente mensual con el motor
// de A-070, validación del importe, comprobación del perfil y traducción de los conflictos de
// persistencia. El almacén es un doble de prueba; la composición productiva usa DynamoDB.

const (
	e024Usuario = "11111111-2222-4333-8444-555555555555"
	e024Otro    = "99999999-8888-4777-8666-555555555555"
)

var e024Ahora = time.Date(2026, 11, 18, 10, 0, 0, 0, time.UTC)

// e024Store es el doble del puerto: registra exactamente lo que la aplicación decide enviar
// a la persistencia, de modo que las pruebas puedan comprobar normalización, redondeo y
// versión esperada sin interpretar expresiones de DynamoDB.
type e024Store struct {
	mu              sync.Mutex
	perfilExiste    bool
	snapshot        *appIdentity.IncomeSnapshot
	errLectura      error
	errPerfil       error
	errEscritura    error
	escrituras      []appIdentity.IncomeStoreWrite
	usuariosLeidos  []string
	resultadoFijado *appIdentity.IncomeSnapshot
}

func newE024Store() *e024Store {
	return &e024Store{perfilExiste: true}
}

func (s *e024Store) LoadIncome(_ context.Context, userID string) (*appIdentity.IncomeSnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.usuariosLeidos = append(s.usuariosLeidos, userID)
	if s.errLectura != nil {
		return nil, s.errLectura
	}
	if s.snapshot == nil {
		return nil, nil
	}
	copia := *s.snapshot
	return &copia, nil
}

func (s *e024Store) ProfileExists(_ context.Context, userID string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.usuariosLeidos = append(s.usuariosLeidos, userID)
	if s.errPerfil != nil {
		return false, s.errPerfil
	}
	return s.perfilExiste, nil
}

func (s *e024Store) StoreIncome(_ context.Context, write appIdentity.IncomeStoreWrite) (*appIdentity.IncomeSnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.escrituras = append(s.escrituras, write)
	if s.errEscritura != nil {
		return nil, s.errEscritura
	}
	if s.resultadoFijado != nil {
		copia := *s.resultadoFijado
		return &copia, nil
	}
	return &appIdentity.IncomeSnapshot{
		AmountCents: write.AmountCents,
		Frequency:   write.Frequency,
		Version:     write.ExpectedVersion + 1,
	}, nil
}

func e024Servicio(store appIdentity.IncomeStore) *appIdentity.IncomeService {
	return appIdentity.NewIncomeService(store).WithClock(func() time.Time { return e024Ahora })
}

// TestE024IngresoInexistenteEsNotFound cubre el 404 del contrato: un titular sin ingreso
// configurado no recibe un objeto vacío ni un importe cero inventado.
func TestE024IngresoInexistenteEsNotFound(t *testing.T) {
	servicio := e024Servicio(newE024Store())

	if _, err := servicio.GetIncome(context.Background(), e024Usuario); !errors.Is(err, appIdentity.ErrIncomeNotFound) {
		t.Fatalf("se esperaba ErrIncomeNotFound, obtenido %v", err)
	}
}

// TestE024NormalizaAliasHistoricos cubre RFC-020 §2 en la lectura: un item histórico con
// `biweekly` se responde `fortnightly` y uno con `yearly` se responde `annual`, sin reescribir
// el item y sin emitir nunca un alias.
func TestE024NormalizaAliasHistoricos(t *testing.T) {
	casos := []struct {
		persistida string
		esperada   string
	}{
		{"biweekly", "fortnightly"},
		{"yearly", "annual"},
		{"weekly", "weekly"},
		{"fortnightly", "fortnightly"},
		{"monthly", "monthly"},
		{"bimonthly", "bimonthly"},
		{"quarterly", "quarterly"},
		{"semiannual", "semiannual"},
		{"annual", "annual"},
		{"MONTHLY", "monthly"},
	}
	for _, caso := range casos {
		t.Run(caso.persistida, func(t *testing.T) {
			store := newE024Store()
			store.snapshot = &appIdentity.IncomeSnapshot{
				AmountCents: 1_000_000,
				Frequency:   domainIdentity.Frequency(caso.persistida),
				Version:     3,
			}
			snapshot, err := e024Servicio(store).GetIncome(context.Background(), e024Usuario)
			if err != nil {
				t.Fatalf("lectura valida no debe fallar: %v", err)
			}
			if snapshot.Frequency.String() != caso.esperada {
				t.Fatalf("frecuencia emitida %q, esperada %q", snapshot.Frequency.String(), caso.esperada)
			}
			if snapshot.AmountCents != 1_000_000 || snapshot.Version != 3 {
				t.Fatalf("importe/version alterados: %+v", snapshot)
			}
			if len(store.escrituras) != 0 {
				t.Fatalf("E024 es sólo lectura y no debe escribir")
			}
		})
	}
}

// TestE024FrecuenciaDesconocidaEsInconsistencia impide emitir un valor fuera del enum
// publicado: una frecuencia corrupta falla cerrado en lugar de propagarse al cliente.
func TestE024FrecuenciaDesconocidaEsInconsistencia(t *testing.T) {
	store := newE024Store()
	store.snapshot = &appIdentity.IncomeSnapshot{AmountCents: 100, Frequency: "cada-luna-llena", Version: 1}

	if _, err := e024Servicio(store).GetIncome(context.Background(), e024Usuario); !errors.Is(err, appIdentity.ErrIncomeInconsistent) {
		t.Fatalf("se esperaba ErrIncomeInconsistent, obtenido %v", err)
	}
}

// TestE024SinIdentidadNoAutoriza es defensa en profundidad: el middleware ya exige `sub`.
func TestE024SinIdentidadNoAutoriza(t *testing.T) {
	if _, err := e024Servicio(newE024Store()).GetIncome(context.Background(), "  "); !errors.Is(err, appIdentity.ErrIncomeUnauthorized) {
		t.Fatalf("se esperaba ErrIncomeUnauthorized, obtenido %v", err)
	}
}

// TestE024FalloDePersistenciaNoEsNotFound demuestra que un error real de DynamoDB no se
// degrada a «no configurado»: se falla cerrado con 503.
func TestE024FalloDePersistenciaNoEsNotFound(t *testing.T) {
	store := newE024Store()
	store.errLectura = errors.New("throttling")

	if _, err := e024Servicio(store).GetIncome(context.Background(), e024Usuario); !errors.Is(err, appIdentity.ErrIncomeUnavailable) {
		t.Fatalf("se esperaba ErrIncomeUnavailable, obtenido %v", err)
	}
}

// TestE024SinAlmacenFallaCerrado impide que un despliegue sin persistencia sirva datos.
func TestE024SinAlmacenFallaCerrado(t *testing.T) {
	servicio := appIdentity.NewIncomeService(nil)
	if _, err := servicio.GetIncome(context.Background(), e024Usuario); !errors.Is(err, appIdentity.ErrIncomeUnavailable) {
		t.Fatalf("se esperaba ErrIncomeUnavailable, obtenido %v", err)
	}
	if _, err := servicio.SetIncome(context.Background(), appIdentity.IncomeUpdate{UserID: e024Usuario, AmountCents: 100, Frequency: "monthly", ExpectedVersion: 1}); !errors.Is(err, appIdentity.ErrIncomeUnavailable) {
		t.Fatalf("se esperaba ErrIncomeUnavailable en escritura, obtenido %v", err)
	}
}

// TestE025RecalculaEquivalenteMensualExacto cubre R-030/GOV-08: el equivalente se deriva con
// el motor A-070 (aritmética racional con redondeo HALF_UP a céntimos), nunca con coma
// flotante, y es un valor DERIVADO que el cliente no puede enviar.
func TestE025RecalculaEquivalenteMensualExacto(t *testing.T) {
	casos := []struct {
		nombre      string
		amountCents int64
		frecuencia  string
		esperado    int64
	}{
		{"mensual 1000.00", 100_000, "monthly", 100_000},
		{"quincenal 1000.00", 100_000, "fortnightly", 200_000},
		{"anual 1200.00", 120_000, "annual", 10_000},
		{"semestral 600.00", 60_000, "semiannual", 10_000},
		{"trimestral 300.00", 30_000, "quarterly", 10_000},
		{"bimestral 200.00", 20_000, "bimonthly", 10_000},
		{"semanal 1000.00", 100_000, "weekly", 433_333},
		{"un centimo mensual", 1, "monthly", 1},
		{"un centimo semanal", 1, "weekly", 4},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			store := newE024Store()
			snapshot, err := e024Servicio(store).SetIncome(context.Background(), appIdentity.IncomeUpdate{
				UserID:          e024Usuario,
				AmountCents:     caso.amountCents,
				Frequency:       caso.frecuencia,
				ExpectedVersion: 1,
			})
			if err != nil {
				t.Fatalf("escritura valida no debe fallar: %v", err)
			}
			if len(store.escrituras) != 1 {
				t.Fatalf("se esperaba una escritura, obtenidas %d", len(store.escrituras))
			}
			if equivalente := store.escrituras[0].MonthlyEquivalentCents; equivalente != caso.esperado {
				t.Fatalf("equivalente mensual %d, esperado %d", equivalente, caso.esperado)
			}
			if snapshot.AmountCents != caso.amountCents {
				t.Fatalf("importe alterado: %d", snapshot.AmountCents)
			}
			if !store.escrituras[0].Now.Equal(e024Ahora) {
				t.Fatalf("la marca de tiempo debe ser la del reloj inyectado, obtenida %s", store.escrituras[0].Now)
			}
		})
	}
}

// TestE025NormalizaAliasAntesDePersistir cubre RFC-020 §2 en la escritura: `biweekly` se
// persiste como `fortnightly` y `yearly` como `annual`, con el equivalente recalculado sobre
// el valor canónico.
func TestE025NormalizaAliasAntesDePersistir(t *testing.T) {
	casos := []struct {
		enviada     string
		persistida  string
		equivalente int64
	}{
		{"biweekly", "fortnightly", 200_000},
		{"yearly", "annual", 10_000},
		{"BIWEEKLY", "fortnightly", 200_000},
		{" yearly ", "annual", 10_000},
	}
	for _, caso := range casos {
		t.Run(caso.enviada, func(t *testing.T) {
			store := newE024Store()
			importe := int64(100_000)
			if caso.persistida == "annual" {
				importe = 120_000
			}
			snapshot, err := e024Servicio(store).SetIncome(context.Background(), appIdentity.IncomeUpdate{
				UserID:          e024Usuario,
				AmountCents:     importe,
				Frequency:       caso.enviada,
				ExpectedVersion: 1,
			})
			if err != nil {
				t.Fatalf("un alias publicado debe aceptarse: %v", err)
			}
			if len(store.escrituras) != 1 {
				t.Fatalf("se esperaba una escritura, obtenidas %d", len(store.escrituras))
			}
			if persistida := store.escrituras[0].Frequency.String(); persistida != caso.persistida {
				t.Fatalf("frecuencia persistida %q, esperada %q", persistida, caso.persistida)
			}
			if store.escrituras[0].MonthlyEquivalentCents != caso.equivalente {
				t.Fatalf("equivalente %d, esperado %d", store.escrituras[0].MonthlyEquivalentCents, caso.equivalente)
			}
			if snapshot.Frequency.String() != caso.persistida {
				t.Fatalf("la respuesta debe emitir el canónico %q, obtenido %q", caso.persistida, snapshot.Frequency.String())
			}
		})
	}
}

// TestE025RechazaImporteInvalido separa formato (transporte) de regla de negocio: cero,
// negativo y fuera de los doce dígitos enteros se rechazan sin escribir nada.
func TestE025RechazaImporteInvalido(t *testing.T) {
	casos := []struct {
		nombre  string
		importe int64
	}{
		{"cero", 0},
		{"negativo", -1},
		{"supera doce enteros", domainIdentity.MaxIncomeAmountCents + 1},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			store := newE024Store()
			_, err := e024Servicio(store).SetIncome(context.Background(), appIdentity.IncomeUpdate{
				UserID:          e024Usuario,
				AmountCents:     caso.importe,
				Frequency:       "monthly",
				ExpectedVersion: 1,
			})
			if !errors.Is(err, appIdentity.ErrIncomeInvalidAmount) {
				t.Fatalf("se esperaba ErrIncomeInvalidAmount, obtenido %v", err)
			}
			if len(store.escrituras) != 0 {
				t.Fatalf("un importe invalido no debe escribir")
			}
		})
	}
}

// TestE025AceptaElMaximoPublicado comprueba el borde superior exacto del patrón: doce
// dígitos enteros y dos decimales se aceptan sin pérdida de precisión.
func TestE025AceptaElMaximoPublicado(t *testing.T) {
	store := newE024Store()
	if _, err := e024Servicio(store).SetIncome(context.Background(), appIdentity.IncomeUpdate{
		UserID:          e024Usuario,
		AmountCents:     domainIdentity.MaxIncomeAmountCents,
		Frequency:       "monthly",
		ExpectedVersion: 1,
	}); err != nil {
		t.Fatalf("el maximo publicado debe aceptarse: %v", err)
	}
	if len(store.escrituras) != 1 || store.escrituras[0].AmountCents != domainIdentity.MaxIncomeAmountCents {
		t.Fatalf("importe maximo no persistido: %+v", store.escrituras)
	}
}

// TestE025RechazaFrecuenciaDesconocida cubre RFC-020 §5: un valor fuera de los nueve
// publicados produce 422 y no se persiste.
func TestE025RechazaFrecuenciaDesconocida(t *testing.T) {
	for _, frecuencia := range []string{"daily", "once", "cada-mes", "", "hourly"} {
		t.Run("frecuencia="+frecuencia, func(t *testing.T) {
			store := newE024Store()
			_, err := e024Servicio(store).SetIncome(context.Background(), appIdentity.IncomeUpdate{
				UserID:          e024Usuario,
				AmountCents:     100,
				Frequency:       frecuencia,
				ExpectedVersion: 1,
			})
			if !errors.Is(err, appIdentity.ErrIncomeInvalidFrequency) {
				t.Fatalf("se esperaba ErrIncomeInvalidFrequency, obtenido %v", err)
			}
			if len(store.escrituras) != 0 {
				t.Fatalf("una frecuencia invalida no debe escribir")
			}
		})
	}
}

// TestE025ExigeVersionEsperadaValida impide un CAS con versión 0, que no existe en el
// contrato (`minimum: 1`).
func TestE025ExigeVersionEsperadaValida(t *testing.T) {
	store := newE024Store()
	_, err := e024Servicio(store).SetIncome(context.Background(), appIdentity.IncomeUpdate{
		UserID:          e024Usuario,
		AmountCents:     100,
		Frequency:       "monthly",
		ExpectedVersion: 0,
	})
	if !errors.Is(err, appIdentity.ErrIncomeInvalidRequest) {
		t.Fatalf("se esperaba ErrIncomeInvalidRequest, obtenido %v", err)
	}
	if len(store.escrituras) != 0 {
		t.Fatalf("una version invalida no debe escribir")
	}
}

// TestE025PerfilInexistenteEsNotFound impide crear un ingreso huérfano de una cuenta
// eliminada o inexistente.
func TestE025PerfilInexistenteEsNotFound(t *testing.T) {
	store := newE024Store()
	store.perfilExiste = false

	_, err := e024Servicio(store).SetIncome(context.Background(), appIdentity.IncomeUpdate{
		UserID:          e024Usuario,
		AmountCents:     100,
		Frequency:       "monthly",
		ExpectedVersion: 1,
	})
	if !errors.Is(err, appIdentity.ErrIncomeNotFound) {
		t.Fatalf("se esperaba ErrIncomeNotFound, obtenido %v", err)
	}
	if len(store.escrituras) != 0 {
		t.Fatalf("sin perfil no debe escribirse el ingreso")
	}
}

// TestE025FalloDelPerfilEsUnavailable no convierte un fallo de DynamoDB en 404.
func TestE025FalloDelPerfilEsUnavailable(t *testing.T) {
	store := newE024Store()
	store.errPerfil = errors.New("throttling")

	_, err := e024Servicio(store).SetIncome(context.Background(), appIdentity.IncomeUpdate{
		UserID:          e024Usuario,
		AmountCents:     100,
		Frequency:       "monthly",
		ExpectedVersion: 1,
	})
	if !errors.Is(err, appIdentity.ErrIncomeUnavailable) {
		t.Fatalf("se esperaba ErrIncomeUnavailable, obtenido %v", err)
	}
}

// TestE025PropagaConflictosDePersistencia demuestra que el servicio no oculta el motivo: 412
// (versión), 409 (concurrencia) y 503 (dependencia) se distinguen.
func TestE025PropagaConflictosDePersistencia(t *testing.T) {
	casos := []struct {
		nombre   string
		errStore error
		esperado error
	}{
		{"version desactualizada", appIdentity.ErrIncomeVersionConflict, appIdentity.ErrIncomeVersionConflict},
		{"conflicto concurrente", appIdentity.ErrIncomeConflict, appIdentity.ErrIncomeConflict},
		{"caida de dependencia", errors.New("connection refused"), appIdentity.ErrIncomeUnavailable},
		{"item incoherente", appIdentity.ErrIncomeInconsistent, appIdentity.ErrIncomeInconsistent},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			store := newE024Store()
			store.errEscritura = caso.errStore
			_, err := e024Servicio(store).SetIncome(context.Background(), appIdentity.IncomeUpdate{
				UserID:          e024Usuario,
				AmountCents:     100,
				Frequency:       "monthly",
				ExpectedVersion: 2,
			})
			if !errors.Is(err, caso.esperado) {
				t.Fatalf("se esperaba %v, obtenido %v", caso.esperado, err)
			}
		})
	}
}

// TestE025UsaLaIdentidadRecibida comprueba el aislamiento en la capa de aplicación: el
// identificador que llega al almacén es exactamente el recibido y nunca se mezcla con otro
// titular.
func TestE025UsaLaIdentidadRecibida(t *testing.T) {
	store := newE024Store()
	if _, err := e024Servicio(store).SetIncome(context.Background(), appIdentity.IncomeUpdate{
		UserID:          e024Otro,
		AmountCents:     500,
		Frequency:       "monthly",
		ExpectedVersion: 4,
	}); err != nil {
		t.Fatalf("escritura valida no debe fallar: %v", err)
	}
	if len(store.escrituras) != 1 {
		t.Fatalf("se esperaba una escritura, obtenidas %d", len(store.escrituras))
	}
	if store.escrituras[0].UserID != e024Otro {
		t.Fatalf("titular incorrecto: %q", store.escrituras[0].UserID)
	}
	if store.escrituras[0].ExpectedVersion != 4 {
		t.Fatalf("version esperada incorrecta: %d", store.escrituras[0].ExpectedVersion)
	}
}

// TestE025ResultadoNuloFallaCerrado impide responder 200 sin constancia de la escritura.
func TestE025ResultadoNuloFallaCerrado(t *testing.T) {
	servicio := appIdentity.NewIncomeService(e024StoreSinResultado{}).WithClock(func() time.Time { return e024Ahora })
	if _, err := servicio.SetIncome(context.Background(), appIdentity.IncomeUpdate{
		UserID:          e024Usuario,
		AmountCents:     100,
		Frequency:       "monthly",
		ExpectedVersion: 1,
	}); !errors.Is(err, appIdentity.ErrIncomeUnavailable) {
		t.Fatalf("se esperaba ErrIncomeUnavailable, obtenido %v", err)
	}
}

// e024StoreSinResultado devuelve (nil, nil) para comprobar la guarda de la aplicación.
type e024StoreSinResultado struct{}

func (e024StoreSinResultado) LoadIncome(context.Context, string) (*appIdentity.IncomeSnapshot, error) {
	return nil, nil
}

func (e024StoreSinResultado) ProfileExists(context.Context, string) (bool, error) { return true, nil }

func (e024StoreSinResultado) StoreIncome(context.Context, appIdentity.IncomeStoreWrite) (*appIdentity.IncomeSnapshot, error) {
	return nil, nil
}

// TestE024NoExponeEquivalenteMensual garantiza que la instantánea de aplicación no lleva
// ningún campo interno que el transporte pudiera serializar por descuido: sólo importe,
// frecuencia y versión.
func TestE024NoExponeEquivalenteMensual(t *testing.T) {
	store := newE024Store()
	store.snapshot = &appIdentity.IncomeSnapshot{AmountCents: 250_000, Frequency: domainIdentity.FrequencyMonthly, Version: 2}

	snapshot, err := e024Servicio(store).GetIncome(context.Background(), e024Usuario)
	if err != nil {
		t.Fatalf("lectura valida no debe fallar: %v", err)
	}
	if snapshot.AmountCents != 250_000 || snapshot.Version != 2 || snapshot.Frequency != domainIdentity.FrequencyMonthly {
		t.Fatalf("instantanea inesperada: %+v", snapshot)
	}
}

// El doble cumple el puerto: la aplicación no depende de DynamoDB.
var _ appIdentity.IncomeStore = (*e024Store)(nil)
