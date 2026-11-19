package application_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	appMovements "github.com/nexumpr/Nexum-Backend/internal/application/movements"
	"github.com/nexumpr/Nexum-Backend/pkg/httputil"
)

// Pruebas de la capa de aplicación de movimientos (E027/E028). No hay mocks del SDK de AWS: el
// puerto `movements.Store` se implementa con un doble de prueba y la atomicidad real de la
// transacción se prueba en `tests/unit/dynamo`.

const (
	movUsuario  = "11111111-2222-4333-8444-555555555555"
	movUsuarioB = "99999999-8888-4777-8666-555555555555"
	movID       = "018f90ef-08e5-7a68-b09f-000000000001"
	movClave    = "clave-sintetica-de-prueba-0001"
	movClaveCur = "clave-de-cursores-de-prueba-e027"
)

var movAhora = time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)

// movStore implementa movements.Store con estado en memoria del propio test.
type movStore struct {
	mu       sync.Mutex
	creates  []appMovements.CreateWrite
	createFn func(appMovements.CreateWrite) (*appMovements.CreateResult, error)
	queries  []appMovements.ListQuery
	starts   []string
	limites  []int
	listFn   func(appMovements.ListQuery, string, int) ([]appMovements.Movement, string, error)
}

func (m *movStore) CreateMovement(_ context.Context, write appMovements.CreateWrite) (*appMovements.CreateResult, error) {
	m.mu.Lock()
	m.creates = append(m.creates, write)
	fn := m.createFn
	m.mu.Unlock()
	if fn != nil {
		return fn(write)
	}
	return &appMovements.CreateResult{Status: write.Status, Body: write.ResponseBody, Location: write.Location}, nil
}

func (m *movStore) ListMovements(_ context.Context, query appMovements.ListQuery, startSK string, limit int) ([]appMovements.Movement, string, error) {
	m.mu.Lock()
	m.queries = append(m.queries, query)
	m.starts = append(m.starts, startSK)
	m.limites = append(m.limites, limit)
	fn := m.listFn
	m.mu.Unlock()
	if fn != nil {
		return fn(query, startSK, limit)
	}
	return nil, "", nil
}

func (m *movStore) ultimaEscritura(t *testing.T) appMovements.CreateWrite {
	t.Helper()
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.creates) == 0 {
		t.Fatal("no se registro ninguna escritura")
	}
	return m.creates[len(m.creates)-1]
}

func movComandoValido() appMovements.CreateCommand {
	return appMovements.CreateCommand{
		Type:           "expense",
		Amount:         "1000.00",
		Currency:       "MXN",
		CategoryID:     "food",
		Description:    "Ejemplo de concepto",
		OccurredAt:     "2026-11-05T17:00:00Z",
		IdempotencyKey: movClave,
	}
}

func movServicio(store appMovements.Store) *appMovements.Service {
	return appMovements.NewService(store).
		WithClock(func() time.Time { return movAhora }).
		WithIDGenerator(func() string { return movID }).
		WithCursorKey([]byte(movClaveCur))
}

// TestE028Validacion cubre la matriz de validación: 400 para la forma inválida y 422 para el
// valor que incumple una regla financiera.
func TestE028Validacion(t *testing.T) {
	descripcionLarga := strings.Repeat("a", appMovements.MaxDescriptionLength+1)
	categoriaLarga := strings.Repeat("c", appMovements.MaxCategoryIDLength+1)

	casos := []struct {
		nombre    string
		tocar     func(*appMovements.CreateCommand)
		violacion bool
		forma     bool // true = ErrInvalidRequest (400)
	}{
		{nombre: "valido", tocar: func(*appMovements.CreateCommand) {}},
		{nombre: "tipo fuera del enum", tocar: func(c *appMovements.CreateCommand) { c.Type = "transfer" }, violacion: true},
		{nombre: "tipo con caja distinta", tocar: func(c *appMovements.CreateCommand) { c.Type = "Expense" }, violacion: true},
		{nombre: "importe cero", tocar: func(c *appMovements.CreateCommand) { c.Amount = "0.00" }, violacion: true},
		{nombre: "importe negativo", tocar: func(c *appMovements.CreateCommand) { c.Amount = "-1.00" }, forma: true},
		{nombre: "importe sin decimales", tocar: func(c *appMovements.CreateCommand) { c.Amount = "1000" }, forma: true},
		{nombre: "importe con un decimal", tocar: func(c *appMovements.CreateCommand) { c.Amount = "1000.5" }, forma: true},
		{nombre: "importe con ceros a la izquierda", tocar: func(c *appMovements.CreateCommand) { c.Amount = "0100.00" }, forma: true},
		{nombre: "importe con mas de doce enteros", tocar: func(c *appMovements.CreateCommand) { c.Amount = "1234567890123.00" }, forma: true},
		{nombre: "importe vacio", tocar: func(c *appMovements.CreateCommand) { c.Amount = "" }, violacion: true},
		{nombre: "divisa distinta de MXN", tocar: func(c *appMovements.CreateCommand) { c.Currency = "USD" }, violacion: true},
		{nombre: "divisa en minusculas se normaliza", tocar: func(c *appMovements.CreateCommand) { c.Currency = "mxn" }},
		{nombre: "descripcion vacia", tocar: func(c *appMovements.CreateCommand) { c.Description = "   " }, violacion: true},
		{nombre: "descripcion demasiado larga", tocar: func(c *appMovements.CreateCommand) { c.Description = descripcionLarga }, violacion: true},
		{nombre: "categoria vacia", tocar: func(c *appMovements.CreateCommand) { c.CategoryID = "" }, violacion: true},
		{nombre: "categoria con espacios en los extremos", tocar: func(c *appMovements.CreateCommand) { c.CategoryID = " food " }, violacion: true},
		{nombre: "categoria demasiado larga", tocar: func(c *appMovements.CreateCommand) { c.CategoryID = categoriaLarga }, violacion: true},
		{nombre: "categoria con caracter de control", tocar: func(c *appMovements.CreateCommand) { c.CategoryID = "fo\x00od" }, violacion: true},
		{nombre: "fecha no RFC3339", tocar: func(c *appMovements.CreateCommand) { c.OccurredAt = "05/11/2026" }, forma: true},
		{nombre: "fecha fuera de rango", tocar: func(c *appMovements.CreateCommand) { c.OccurredAt = "1800-01-01T00:00:00Z" }, violacion: true},
		{nombre: "fecha muy futura", tocar: func(c *appMovements.CreateCommand) { c.OccurredAt = "2026-11-25T00:00:00Z" }, violacion: true},
		{nombre: "clave de idempotencia ausente", tocar: func(c *appMovements.CreateCommand) { c.IdempotencyKey = "" }, forma: true},
		{nombre: "clave de idempotencia corta", tocar: func(c *appMovements.CreateCommand) { c.IdempotencyKey = "corta" }, forma: true},
	}

	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			comando := movComandoValido()
			caso.tocar(&comando)
			_, err := appMovements.NormalizeCreate(comando, movAhora)

			var violacion *appMovements.FieldViolation
			switch {
			case caso.violacion:
				if !errors.As(err, &violacion) {
					t.Fatalf("se esperaba FieldViolation (422) y se obtuvo %v", err)
				}
			case caso.forma:
				if !errors.Is(err, appMovements.ErrInvalidRequest) {
					t.Fatalf("se esperaba ErrInvalidRequest (400) y se obtuvo %v", err)
				}
				if errors.As(err, &violacion) {
					t.Fatalf("una forma invalida no debe reportarse como regla de negocio: %v", err)
				}
			default:
				if err != nil {
					t.Fatalf("se esperaba exito y se obtuvo %v", err)
				}
			}
		})
	}
}

// TestE028NormalizaFechaYDivisa comprueba que la fecha queda en UTC con milisegundos (la
// precisión de la SK) y la divisa en mayúsculas.
func TestE028NormalizaFechaYDivisa(t *testing.T) {
	comando := movComandoValido()
	comando.Currency = "mxn"
	comando.OccurredAt = "2026-11-05T19:00:00.123456+02:00"

	normalizado, err := appMovements.NormalizeCreate(comando, movAhora)
	if err != nil {
		t.Fatalf("normalizacion inesperada: %v", err)
	}
	if normalizado.OccurredAt != "2026-11-05T17:00:00.123Z" {
		t.Fatalf("fecha normalizada inesperada: %q", normalizado.OccurredAt)
	}
	if normalizado.Currency != "MXN" {
		t.Fatalf("divisa normalizada inesperada: %q", normalizado.Currency)
	}
	if normalizado.Amount != "1000.00" {
		t.Fatalf("importe canonico inesperado: %q", normalizado.Amount)
	}
}

// TestE028CreaConContratoExacto cubre T01: 201 con el esquema `Movement` exacto, versión 1,
// importe en cadena decimal y campos nullable presentes.
func TestE028CreaConContratoExacto(t *testing.T) {
	store := &movStore{}
	servicio := movServicio(store)

	resultado, err := servicio.Create(context.Background(), movUsuario, movComandoValido())
	if err != nil {
		t.Fatalf("alta inesperada: %v", err)
	}
	if resultado.Status != appMovements.StatusCreated {
		t.Fatalf("estado inesperado: %d", resultado.Status)
	}
	if resultado.Replayed {
		t.Fatal("un alta nueva no puede marcarse como replay")
	}
	if resultado.Location != "/api/v1/movements/"+movID {
		t.Fatalf("Location inesperado: %q", resultado.Location)
	}

	escritura := store.ultimaEscritura(t)
	if escritura.UserID != movUsuario {
		t.Fatalf("el titular debe venir del argumento verificado, se obtuvo %q", escritura.UserID)
	}
	if escritura.Operation != appMovements.OperationCreateMovement {
		t.Fatalf("operacion persistida inesperada: %q", escritura.Operation)
	}
	if escritura.Movement.Version != 1 {
		t.Fatalf("version inicial inesperada: %d", escritura.Movement.Version)
	}
	if escritura.Movement.AmountCents != 100000 {
		t.Fatalf("centavos inesperados: %d", escritura.Movement.AmountCents)
	}
	if !escritura.ExpiresAt.Equal(movAhora.Add(appMovements.IdempotencyTTL)) {
		t.Fatalf("vencimiento de idempotencia inesperado: %v", escritura.ExpiresAt)
	}

	var cuerpo map[string]any
	if err := json.Unmarshal(resultado.Body, &cuerpo); err != nil {
		t.Fatalf("cuerpo no deserializable: %v", err)
	}
	esperadas := []string{"id", "type", "amount", "currency", "categoryId", "description", "occurredAt", "version", "contributionId", "recurringPaymentId"}
	if len(cuerpo) != len(esperadas) {
		t.Fatalf("numero de campos inesperado: %d (%v)", len(cuerpo), cuerpo)
	}
	for _, campo := range esperadas {
		if _, ok := cuerpo[campo]; !ok {
			t.Fatalf("falta el campo %q en la respuesta", campo)
		}
	}
	if cuerpo["amount"] != "1000.00" {
		t.Fatalf("importe serializado inesperado: %v", cuerpo["amount"])
	}
	if cuerpo["version"] != float64(1) {
		t.Fatalf("version serializada inesperada: %v", cuerpo["version"])
	}
	if cuerpo["contributionId"] != nil || cuerpo["recurringPaymentId"] != nil {
		t.Fatalf("los campos nullable deben emitirse como null: %v", cuerpo)
	}
	if cuerpo["occurredAt"] != "2026-11-05T17:00:00.000Z" {
		t.Fatalf("occurredAt inesperado: %v", cuerpo["occurredAt"])
	}
}

// TestE028PayloadHashEstable comprueba que el hash depende del contenido normalizado y no de la
// clave ni del formato del cuerpo.
func TestE028PayloadHashEstable(t *testing.T) {
	base := movComandoValido()
	normalizado, err := appMovements.NormalizeCreate(base, movAhora)
	if err != nil {
		t.Fatalf("normalizacion inesperada: %v", err)
	}
	huella := appMovements.PayloadHash(normalizado)

	otraClave := base
	otraClave.IdempotencyKey = "otra-clave-sintetica-de-prueba-0002"
	normalizadaOtra, err := appMovements.NormalizeCreate(otraClave, movAhora)
	if err != nil {
		t.Fatalf("normalizacion inesperada: %v", err)
	}
	if appMovements.PayloadHash(normalizadaOtra) != huella {
		t.Fatal("el hash no debe depender de la Idempotency-Key")
	}

	conEspacios := base
	conEspacios.Description = "  Ejemplo de concepto  "
	conEspacios.Currency = " mxn "
	normalizadaEspacios, err := appMovements.NormalizeCreate(conEspacios, movAhora)
	if err != nil {
		t.Fatalf("normalizacion inesperada: %v", err)
	}
	if appMovements.PayloadHash(normalizadaEspacios) != huella {
		t.Fatal("dos cuerpos equivalentes deben producir el mismo hash")
	}

	distinto := base
	distinto.Amount = "1000.01"
	normalizadaDistinto, err := appMovements.NormalizeCreate(distinto, movAhora)
	if err != nil {
		t.Fatalf("normalizacion inesperada: %v", err)
	}
	if appMovements.PayloadHash(normalizadaDistinto) == huella {
		t.Fatal("un importe distinto debe producir otro hash")
	}
}

// TestE028ReplayYConflicto comprueba que el servicio propaga el replay y el conflicto sin
// reinterpretarlos.
func TestE028ReplayYConflicto(t *testing.T) {
	cuerpo := []byte(`{"id":"` + movID + `","version":1}`)

	store := &movStore{createFn: func(appMovements.CreateWrite) (*appMovements.CreateResult, error) {
		return &appMovements.CreateResult{Status: appMovements.StatusCreated, Body: cuerpo, Location: "/api/v1/movements/" + movID, Replayed: true}, nil
	}}
	resultado, err := movServicio(store).Create(context.Background(), movUsuario, movComandoValido())
	if err != nil {
		t.Fatalf("replay inesperado: %v", err)
	}
	if !resultado.Replayed || string(resultado.Body) != string(cuerpo) {
		t.Fatalf("el replay debe reproducir el cuerpo original: %+v", resultado)
	}

	storeConflicto := &movStore{createFn: func(appMovements.CreateWrite) (*appMovements.CreateResult, error) {
		return nil, appMovements.ErrConflict
	}}
	if _, err := movServicio(storeConflicto).Create(context.Background(), movUsuario, movComandoValido()); !errors.Is(err, appMovements.ErrConflict) {
		t.Fatalf("se esperaba conflicto y se obtuvo %v", err)
	}
}

// TestE028AislamientoEntreUsuarios comprueba que la misma clave con el mismo contenido en dos
// cuentas produce dos escrituras independientes, cada una con su titular.
func TestE028AislamientoEntreUsuarios(t *testing.T) {
	store := &movStore{}
	servicio := movServicio(store)

	if _, err := servicio.Create(context.Background(), movUsuario, movComandoValido()); err != nil {
		t.Fatalf("alta A inesperada: %v", err)
	}
	if _, err := servicio.Create(context.Background(), movUsuarioB, movComandoValido()); err != nil {
		t.Fatalf("alta B inesperada: %v", err)
	}

	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.creates) != 2 {
		t.Fatalf("se esperaban dos escrituras, hay %d", len(store.creates))
	}
	if store.creates[0].UserID == store.creates[1].UserID {
		t.Fatal("las dos cuentas deben llegar al almacen con titulares distintos")
	}
	for _, escritura := range store.creates {
		if escritura.PayloadHash == "" {
			t.Fatal("el hash del payload no puede estar vacio")
		}
	}
}

// TestE028Concurrencia comprueba que el servicio es seguro en concurrencia y que sólo una de las
// peticiones simultáneas crea el recurso: el resto reproduce la respuesta.
func TestE028Concurrencia(t *testing.T) {
	const peticiones = 24

	var mu sync.Mutex
	creados := 0
	replays := 0
	var primero *appMovements.CreateResult

	store := &movStore{createFn: func(write appMovements.CreateWrite) (*appMovements.CreateResult, error) {
		mu.Lock()
		defer mu.Unlock()
		if primero == nil {
			creados++
			primero = &appMovements.CreateResult{Status: write.Status, Body: write.ResponseBody, Location: write.Location}
			return primero, nil
		}
		replays++
		return &appMovements.CreateResult{Status: primero.Status, Body: primero.Body, Location: primero.Location, Replayed: true}, nil
	}}
	servicio := movServicio(store)

	var wg sync.WaitGroup
	cuerpos := make([][]byte, peticiones)
	for i := 0; i < peticiones; i++ {
		wg.Add(1)
		go func(indice int) {
			defer wg.Done()
			resultado, err := servicio.Create(context.Background(), movUsuario, movComandoValido())
			if err != nil {
				t.Errorf("alta concurrente %d: %v", indice, err)
				return
			}
			cuerpos[indice] = resultado.Body
		}(i)
	}
	wg.Wait()

	if creados != 1 {
		t.Fatalf("se esperaba un unico alta y hubo %d", creados)
	}
	if replays != peticiones-1 {
		t.Fatalf("se esperaban %d replays y hubo %d", peticiones-1, replays)
	}
	for i, cuerpo := range cuerpos {
		if len(cuerpo) == 0 || string(cuerpo) != string(cuerpos[0]) {
			t.Fatalf("el cuerpo %d difiere del primero", i)
		}
	}
}

// TestE027ListadoYCursores cubre la paginación firmada de E027.
func TestE027ListadoYCursores(t *testing.T) {
	movimiento := appMovements.Movement{
		ID: movID, Type: "expense", AmountCents: 100000, Currency: "MXN",
		CategoryID: "food", Description: "Ejemplo", OccurredAt: movAhora, Version: 1,
	}

	store := &movStore{listFn: func(_ appMovements.ListQuery, _ string, _ int) ([]appMovements.Movement, string, error) {
		return []appMovements.Movement{movimiento}, "MOVEMENT#2026-11-18T11:00:00.000Z#" + movID, nil
	}}
	servicio := movServicio(store)

	pagina, err := servicio.List(context.Background(), appMovements.ListQuery{UserID: movUsuario})
	if err != nil {
		t.Fatalf("listado inesperado: %v", err)
	}
	if len(pagina.Items) != 1 || pagina.NextCursor == "" {
		t.Fatalf("pagina inesperada: %+v", pagina)
	}

	sigma := appMovements.FilterSignature(appMovements.ListQuery{Limit: appMovements.DefaultLimit})
	payload, err := httputil.ValidateCursor([]byte(movClaveCur), pagina.NextCursor, movUsuario)
	if err != nil {
		t.Fatalf("el cursor emitido debe ser verificable: %v", err)
	}
	if payload.LastID != "MOVEMENT#2026-11-18T11:00:00.000Z#"+movID || payload.SortBy != sigma {
		t.Fatalf("contenido del cursor inesperado: %+v", payload)
	}
	if store.limites[0] != appMovements.DefaultLimit {
		t.Fatalf("limite por defecto inesperado: %d", store.limites[0])
	}

	// Reanudar con el cursor emitido.
	if _, err := servicio.List(context.Background(), appMovements.ListQuery{UserID: movUsuario, Cursor: pagina.NextCursor}); err != nil {
		t.Fatalf("reanudacion inesperada: %v", err)
	}
	store.mu.Lock()
	ultimoInicio := store.starts[len(store.starts)-1]
	store.mu.Unlock()
	if ultimoInicio != payload.LastID {
		t.Fatalf("la reanudacion debe empezar en la clave del cursor: %q", ultimoInicio)
	}

	// Un cursor emitido con otros filtros no puede reutilizarse.
	if _, err := servicio.List(context.Background(), appMovements.ListQuery{UserID: movUsuario, Cursor: pagina.NextCursor, Type: "income"}); !errors.Is(err, appMovements.ErrInvalidRequest) {
		t.Fatalf("filtro distinto con cursor antiguo debe ser 400, se obtuvo %v", err)
	}
	// Un cursor de otra cuenta no se acepta.
	if _, err := servicio.List(context.Background(), appMovements.ListQuery{UserID: movUsuarioB, Cursor: pagina.NextCursor}); !errors.Is(err, appMovements.ErrInvalidRequest) {
		t.Fatalf("cursor ajeno debe ser 400, se obtuvo %v", err)
	}
	// Un cursor manipulado no se acepta.
	manipulado := pagina.NextCursor[:len(pagina.NextCursor)-2] + "AA"
	if _, err := servicio.List(context.Background(), appMovements.ListQuery{UserID: movUsuario, Cursor: manipulado}); !errors.Is(err, appMovements.ErrInvalidRequest) {
		t.Fatalf("cursor manipulado debe ser 400, se obtuvo %v", err)
	}
}

// TestE027LimitesYFiltros cubre los límites del contrato y la firma de filtros.
func TestE027LimitesYFiltros(t *testing.T) {
	store := &movStore{}
	servicio := movServicio(store)

	for _, caso := range []appMovements.ListQuery{
		{UserID: movUsuario, Limit: -1},
		{UserID: movUsuario, Limit: appMovements.MaxLimit + 1},
		{UserID: movUsuario, Cursor: "corto"},
		{UserID: movUsuario, Type: "transfer"},
		{UserID: movUsuario, HasFrom: true, From: movAhora, HasTo: true, To: movAhora.Add(-time.Hour)},
	} {
		if _, err := servicio.List(context.Background(), caso); !errors.Is(err, appMovements.ErrInvalidRequest) {
			t.Fatalf("se esperaba 400 para %+v y se obtuvo %v", caso, err)
		}
	}

	// Sin titular no se lista nada.
	if _, err := servicio.List(context.Background(), appMovements.ListQuery{}); !errors.Is(err, appMovements.ErrUnauthorized) {
		t.Fatalf("se esperaba 401 sin titular y se obtuvo %v", err)
	}

	// Sin clave de cursor el listado falla cerrado.
	sinClave := appMovements.NewService(store)
	if _, err := sinClave.List(context.Background(), appMovements.ListQuery{UserID: movUsuario}); !errors.Is(err, appMovements.ErrUnavailable) {
		t.Fatalf("se esperaba 503 sin clave de cursor y se obtuvo %v", err)
	}

	// La firma distingue los conjuntos que un cursor no puede mezclar.
	firmaBase := appMovements.FilterSignature(appMovements.ListQuery{})
	if firmaBase == appMovements.FilterSignature(appMovements.ListQuery{Type: "expense"}) {
		t.Fatal("la firma debe distinguir por tipo")
	}
	if firmaBase == appMovements.FilterSignature(appMovements.ListQuery{HasFrom: true, From: movAhora}) {
		t.Fatal("la firma debe distinguir por desde")
	}
	if firmaBase != appMovements.FilterSignature(appMovements.ListQuery{Limit: 7}) {
		t.Fatal("la firma no debe depender del limite")
	}
}

// TestE028FailClosedSinAlmacen comprueba que sin almacén el servicio no inventa una respuesta.
func TestE028FailClosedSinAlmacen(t *testing.T) {
	servicio := appMovements.NewService(nil)
	if _, err := servicio.Create(context.Background(), movUsuario, movComandoValido()); !errors.Is(err, appMovements.ErrUnavailable) {
		t.Fatalf("se esperaba 503 sin almacen y se obtuvo %v", err)
	}
	if _, err := servicio.List(context.Background(), appMovements.ListQuery{UserID: movUsuario}); !errors.Is(err, appMovements.ErrUnavailable) {
		t.Fatalf("se esperaba 503 sin almacen y se obtuvo %v", err)
	}
}
