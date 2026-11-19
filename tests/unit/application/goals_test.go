package application_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	appGoals "github.com/nexumpr/Nexum-Backend/internal/application/goals"
)

// Pruebas de aplicación de E039 (`POST /api/v1/goals`): validaciones de esquema y de negocio,
// ahorro de apertura, idempotencia aislada por usuario y traducción de los fallos de
// persistencia. El almacén es un doble de prueba; la composición productiva usa DynamoDB.

const (
	e039Usuario = "11111111-2222-4333-8444-555555555555"
	e039Otro    = "99999999-8888-4777-8666-555555555555"
	e039Clave   = "018f90ef-08e5-7a68-b09f-2b67a47c92f0"
	e039MetaID  = "018f90ef-08e5-7a68-b09f-0000000000a1"
	e039MetaIDB = "018f90ef-08e5-7a68-b09f-0000000000a2"
)

var e039Ahora = time.Date(2026, 11, 18, 15, 0, 0, 0, time.UTC)

// e039Store memoriza el registro de idempotencia por (usuario, operación, clave) y captura la
// escritura que la aplicación decide enviar. Dos usuarios con la misma clave nunca comparten
// entrada, de modo que la prueba de aislamiento es real y no una comprobación de firma.
type e039Store struct {
	mu       sync.Mutex
	registro map[string]appGoals.IdempotencyRecord
	creates  []appGoals.CreateWrite
	errLoad  error
	errCrea  error
}

func newE039Store() *e039Store {
	return &e039Store{registro: map[string]appGoals.IdempotencyRecord{}}
}

func e039ClaveRegistro(userID, operation, key string) string {
	return userID + "|" + operation + "|" + key
}

func (s *e039Store) LoadIdempotency(_ context.Context, userID, operation, key string) (*appGoals.IdempotencyRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.errLoad != nil {
		return nil, s.errLoad
	}
	record, ok := s.registro[e039ClaveRegistro(userID, operation, key)]
	if !ok {
		return nil, nil
	}
	copia := record
	return &copia, nil
}

func (s *e039Store) CreateGoal(_ context.Context, write appGoals.CreateWrite) (*appGoals.StoredResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.creates = append(s.creates, write)
	if s.errCrea != nil {
		return nil, s.errCrea
	}
	s.registro[e039ClaveRegistro(write.Goal.UserID, write.Operation, write.IdempotencyKey)] = appGoals.IdempotencyRecord{
		PayloadHash:  write.PayloadHash,
		Status:       "COMPLETED",
		HTTPStatus:   201,
		ResponseBody: write.ResponseBody,
		ResourceID:   write.Goal.ID,
		Location:     write.Location,
		ExpiresAt:    write.Now.Add(write.TTL),
	}
	return &appGoals.StoredResponse{
		Status:     201,
		Body:       write.ResponseBody,
		Location:   write.Location,
		ResourceID: write.Goal.ID,
	}, nil
}

func (s *e039Store) escrituras() []appGoals.CreateWrite {
	s.mu.Lock()
	defer s.mu.Unlock()
	copia := make([]appGoals.CreateWrite, len(s.creates))
	copy(copia, s.creates)
	return copia
}

func e039Servicio(store appGoals.Store) *appGoals.Service {
	secuencia := 0
	return appGoals.NewService(store).
		WithClock(func() time.Time { return e039Ahora }).
		WithIDGenerator(func() (string, error) {
			secuencia++
			if secuencia == 1 {
				return e039MetaID, nil
			}
			return e039MetaIDB, nil
		})
}

func e039Comando() appGoals.CreateCommand {
	return appGoals.CreateCommand{
		UserID:            e039Usuario,
		IdempotencyKey:    e039Clave,
		Name:              "  Viaje a Oaxaca  ",
		TargetAmountCents: 100000,
		InitialSavedCents: 25000,
		Currency:          "MXN",
		TargetDate:        "2026-12-31",
	}
}

func e039Decodificar(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var salida map[string]any
	if err := json.Unmarshal(body, &salida); err != nil {
		t.Fatalf("respuesta no es JSON: %v", err)
	}
	return salida
}

// TestE039CreaConOpeningSaved comprueba el alta completa: nombre normalizado, importes exactos
// en céntimos, `openingSaved` igual al ahorro de apertura y versión 1 en estado activo.
func TestE039CreaConOpeningSaved(t *testing.T) {
	store := newE039Store()
	respuesta, err := e039Servicio(store).Create(context.Background(), e039Comando())
	if err != nil {
		t.Fatalf("alta valida no debe fallar: %v", err)
	}
	if respuesta.Status != 201 {
		t.Fatalf("esperado 201, obtenido %d", respuesta.Status)
	}
	if respuesta.Replayed {
		t.Fatal("el primer alta no es un replay")
	}
	if respuesta.ResourceID != e039MetaID || respuesta.Location != "/api/v1/goals/"+e039MetaID {
		t.Fatalf("identificador o Location incorrectos: %q %q", respuesta.ResourceID, respuesta.Location)
	}

	cuerpo := e039Decodificar(t, respuesta.Body)
	esperado := map[string]string{
		"id":           e039MetaID,
		"name":         "Viaje a Oaxaca",
		"targetAmount": "1000.00",
		"savedAmount":  "250.00",
		"openingSaved": "250.00",
		"currency":     "MXN",
		"targetDate":   "2026-12-31",
		"status":       "active",
	}
	for campo, valor := range esperado {
		if obtenido, _ := cuerpo[campo].(string); obtenido != valor {
			t.Fatalf("%s: esperado %q, obtenido %q", campo, valor, obtenido)
		}
	}
	if version, _ := cuerpo["version"].(float64); version != 1 {
		t.Fatalf("version: esperado 1, obtenido %v", cuerpo["version"])
	}
	if len(cuerpo) != 9 {
		t.Fatalf("el DTO debe tener exactamente 9 campos, tiene %d: %v", len(cuerpo), cuerpo)
	}

	escrituras := store.escrituras()
	if len(escrituras) != 1 {
		t.Fatalf("se esperaba una escritura, hubo %d", len(escrituras))
	}
	write := escrituras[0]
	if write.Operation != appGoals.OperationCreateGoal {
		t.Fatalf("operacion inesperada: %q", write.Operation)
	}
	if write.Goal.Name != "Viaje a Oaxaca" {
		t.Fatalf("el nombre persistido debe estar normalizado: %q", write.Goal.Name)
	}
	if write.Goal.SavedAmount.Amount() != 25000 || write.Goal.OpeningSaved.Amount() != 25000 {
		t.Fatalf("savedAmount/openingSaved deben ser 25000 centimos: %d %d",
			write.Goal.SavedAmount.Amount(), write.Goal.OpeningSaved.Amount())
	}
	if write.TTL != appGoals.IdempotencyTTL {
		t.Fatalf("TTL de idempotencia inesperado: %v", write.TTL)
	}
	if len(write.PayloadHash) != 64 || strings.Trim(write.PayloadHash, "0123456789abcdef") != "" {
		t.Fatalf("payloadHash debe ser sha256 hexadecimal: %q", write.PayloadHash)
	}
}

// TestE039AperturaPorDefecto garantiza que omitir `initialSavedAmount` produce "0.00" y no un
// cero implícito ambiguo.
func TestE039AperturaPorDefecto(t *testing.T) {
	store := newE039Store()
	comando := e039Comando()
	comando.InitialSavedCents = 0
	respuesta, err := e039Servicio(store).Create(context.Background(), comando)
	if err != nil {
		t.Fatalf("alta sin apertura no debe fallar: %v", err)
	}
	cuerpo := e039Decodificar(t, respuesta.Body)
	if cuerpo["savedAmount"] != "0.00" || cuerpo["openingSaved"] != "0.00" {
		t.Fatalf("apertura por defecto incorrecta: %v", cuerpo)
	}
}

// TestE039Validaciones cubre los límites de importe y las entradas mal formadas.
func TestE039Validaciones(t *testing.T) {
	casos := []struct {
		nombre   string
		modifica func(*appGoals.CreateCommand)
		esperado error
	}{
		{"sin usuario", func(c *appGoals.CreateCommand) { c.UserID = "  " }, appGoals.ErrUnauthorized},
		{"sin clave idempotente", func(c *appGoals.CreateCommand) { c.IdempotencyKey = "" }, appGoals.ErrInvalidRequest},
		{"nombre vacio", func(c *appGoals.CreateCommand) { c.Name = "   " }, appGoals.ErrInvalidRequest},
		{"nombre demasiado largo", func(c *appGoals.CreateCommand) { c.Name = strings.Repeat("a", 121) }, appGoals.ErrInvalidRequest},
		{"divisa distinta", func(c *appGoals.CreateCommand) { c.Currency = "USD" }, appGoals.ErrInvalidRequest},
		{"fecha invalida", func(c *appGoals.CreateCommand) { c.TargetDate = "2026-02-30" }, appGoals.ErrInvalidRequest},
		{"objetivo cero", func(c *appGoals.CreateCommand) { c.TargetAmountCents = 0 }, appGoals.ErrTargetNotPositive},
		{"objetivo negativo", func(c *appGoals.CreateCommand) { c.TargetAmountCents = -1 }, appGoals.ErrTargetNotPositive},
		{"objetivo sobre el maximo", func(c *appGoals.CreateCommand) { c.TargetAmountCents = 100_000_000_000_000 }, appGoals.ErrInvalidRequest},
		{"apertura negativa", func(c *appGoals.CreateCommand) { c.InitialSavedCents = -1 }, appGoals.ErrInvalidRequest},
		{"apertura mayor que el objetivo", func(c *appGoals.CreateCommand) { c.InitialSavedCents = 100001 }, appGoals.ErrOpeningExceedsTarget},
		{"apertura igual al objetivo", func(c *appGoals.CreateCommand) { c.InitialSavedCents = 100000 }, nil},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			store := newE039Store()
			comando := e039Comando()
			caso.modifica(&comando)
			_, err := e039Servicio(store).Create(context.Background(), comando)
			if caso.esperado == nil {
				if err != nil {
					t.Fatalf("no debia fallar: %v", err)
				}
				return
			}
			if !errors.Is(err, caso.esperado) {
				t.Fatalf("esperado %v, obtenido %v", caso.esperado, err)
			}
			if len(store.escrituras()) != 0 {
				t.Fatal("una solicitud invalida no debe escribir nada")
			}
		})
	}
}

// TestE039ReplayIdempotente comprueba que repetir la misma petición devuelve el mismo cuerpo y
// no vuelve a escribir.
func TestE039ReplayIdempotente(t *testing.T) {
	store := newE039Store()
	servicio := e039Servicio(store)
	primera, err := servicio.Create(context.Background(), e039Comando())
	if err != nil {
		t.Fatalf("primera alta fallo: %v", err)
	}
	segunda, err := servicio.Create(context.Background(), e039Comando())
	if err != nil {
		t.Fatalf("el replay no debe fallar: %v", err)
	}
	if !segunda.Replayed {
		t.Fatal("la segunda respuesta debe marcarse como replay")
	}
	if string(segunda.Body) != string(primera.Body) {
		t.Fatalf("el replay debe devolver el mismo cuerpo:\n%s\n%s", primera.Body, segunda.Body)
	}
	if segunda.ResourceID != primera.ResourceID || segunda.Status != primera.Status {
		t.Fatalf("el replay debe conservar identificador y codigo: %+v vs %+v", segunda, primera)
	}
	if len(store.escrituras()) != 1 {
		t.Fatalf("el replay no debe escribir de nuevo: %d escrituras", len(store.escrituras()))
	}
}

// TestE039NormalizacionDelHash demuestra que los espacios sobrantes del nombre no convierten
// un reintento idéntico en un 409.
func TestE039NormalizacionDelHash(t *testing.T) {
	store := newE039Store()
	servicio := e039Servicio(store)
	if _, err := servicio.Create(context.Background(), e039Comando()); err != nil {
		t.Fatalf("primera alta fallo: %v", err)
	}
	comando := e039Comando()
	comando.Name = "Viaje a Oaxaca"
	replay, err := servicio.Create(context.Background(), comando)
	if err != nil {
		t.Fatalf("el nombre normalizado debe producir el mismo hash: %v", err)
	}
	if !replay.Replayed {
		t.Fatal("se esperaba replay con el nombre ya canonico")
	}
}

// TestE039ConflictoDePayload comprueba que la misma clave con otro contenido es un 409 y no
// crea una segunda meta.
func TestE039ConflictoDePayload(t *testing.T) {
	store := newE039Store()
	servicio := e039Servicio(store)
	if _, err := servicio.Create(context.Background(), e039Comando()); err != nil {
		t.Fatalf("primera alta fallo: %v", err)
	}
	comando := e039Comando()
	comando.TargetAmountCents = 200000
	_, err := servicio.Create(context.Background(), comando)
	if !errors.Is(err, appGoals.ErrConflict) {
		t.Fatalf("esperado ErrConflict, obtenido %v", err)
	}
	if len(store.escrituras()) != 1 {
		t.Fatalf("el conflicto no debe escribir: %d escrituras", len(store.escrituras()))
	}
}

// TestE039AislamientoEntreUsuarios verifica que dos cuentas con la misma Idempotency-Key no
// comparten resultado: cada una crea su propia meta y el replay nunca cruza identidades.
func TestE039AislamientoEntreUsuarios(t *testing.T) {
	store := newE039Store()
	servicio := e039Servicio(store)

	altaA, err := servicio.Create(context.Background(), e039Comando())
	if err != nil {
		t.Fatalf("alta del usuario A fallo: %v", err)
	}
	comandoB := e039Comando()
	comandoB.UserID = e039Otro
	altaB, err := servicio.Create(context.Background(), comandoB)
	if err != nil {
		t.Fatalf("alta del usuario B fallo: %v", err)
	}
	if altaB.Replayed {
		t.Fatal("la misma clave de otro usuario no puede ser un replay")
	}
	if altaB.ResourceID == altaA.ResourceID {
		t.Fatal("cada usuario debe tener su propio recurso")
	}
	if len(store.escrituras()) != 2 {
		t.Fatalf("se esperaban dos altas, hubo %d", len(store.escrituras()))
	}

	replayB, err := servicio.Create(context.Background(), comandoB)
	if err != nil {
		t.Fatalf("replay de B fallo: %v", err)
	}
	if !replayB.Replayed || replayB.ResourceID != altaB.ResourceID {
		t.Fatalf("el replay de B debe devolver su propio recurso: %+v", replayB)
	}
	if string(replayB.Body) == string(altaA.Body) {
		t.Fatal("el replay de B no puede devolver el cuerpo de A")
	}
}

// TestE039RegistroVencidoSeReescribe documenta el vencimiento lógico de 24 h: un registro
// caducado no se reutiliza y la operación se escribe de nuevo.
func TestE039RegistroVencidoSeReescribe(t *testing.T) {
	store := newE039Store()
	servicio := e039Servicio(store)
	if _, err := servicio.Create(context.Background(), e039Comando()); err != nil {
		t.Fatalf("primera alta fallo: %v", err)
	}
	store.mu.Lock()
	for clave, record := range store.registro {
		record.ExpiresAt = e039Ahora.Add(-time.Minute)
		store.registro[clave] = record
	}
	store.mu.Unlock()

	segunda, err := servicio.Create(context.Background(), e039Comando())
	if err != nil {
		t.Fatalf("tras vencer el registro debe poder escribirse: %v", err)
	}
	if segunda.Replayed {
		t.Fatal("un registro vencido no es un replay")
	}
	if len(store.escrituras()) != 2 {
		t.Fatalf("se esperaban dos escrituras, hubo %d", len(store.escrituras()))
	}
}

// TestE039FallosDePersistencia comprueba el fail-closed: un fallo real de DynamoDB se propaga
// como 503 y nunca se convierte en un 404 o en una respuesta inventada.
func TestE039FallosDePersistencia(t *testing.T) {
	t.Run("sin almacen", func(t *testing.T) {
		if _, err := appGoals.NewService(nil).Create(context.Background(), e039Comando()); !errors.Is(err, appGoals.ErrUnavailable) {
			t.Fatalf("esperado ErrUnavailable, obtenido %v", err)
		}
	})
	t.Run("fallo de lectura", func(t *testing.T) {
		store := newE039Store()
		store.errLoad = errors.New("dynamodb inaccesible")
		if _, err := e039Servicio(store).Create(context.Background(), e039Comando()); !errors.Is(err, appGoals.ErrUnavailable) {
			t.Fatalf("esperado ErrUnavailable, obtenido %v", err)
		}
	})
	t.Run("fallo de escritura", func(t *testing.T) {
		store := newE039Store()
		store.errCrea = appGoals.ErrUnavailable
		if _, err := e039Servicio(store).Create(context.Background(), e039Comando()); !errors.Is(err, appGoals.ErrUnavailable) {
			t.Fatalf("esperado ErrUnavailable, obtenido %v", err)
		}
	})
}
