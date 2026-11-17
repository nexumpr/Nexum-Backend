package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	appIdentity "github.com/nexumpr/Nexum-Backend/internal/application/identity"
)

// Pruebas de aplicación de E008 (`GET /api/v1/auth/sessions`): validación de límite, cursor
// opaco ligado a identidad y a filtros, reducción a los cinco campos del contrato y traducción
// de fallos de persistencia. El almacén es un doble de prueba; la composición productiva usa
// DynamoDB real.

const (
	e008Usuario = "11111111-2222-4333-8444-555555555555"
	e008Otro    = "99999999-8888-4777-8666-555555555555"

	e008SesionA = "018f90ef-08e5-7a68-b09f-0000000000a1"
	e008SesionB = "018f90ef-08e5-7a68-b09f-0000000000b2"
	e008SesionC = "018f90ef-08e5-7a68-b09f-0000000000c3"
)

var e008Ahora = time.Date(2026, 11, 17, 10, 0, 0, 0, time.UTC)

var e008Clave = []byte("clave-de-prueba-para-cursores-de-sesiones")

// e008Llamada registra exactamente qué recibió la persistencia: es la prueba de que la
// identidad sale del token y de que el cursor se traduce en `ExclusiveStartKey`.
type e008Llamada struct {
	userID  string
	startSK string
	limit   int
}

type e008Pagina struct {
	registros []appIdentity.SessionListRecord
	nextSK    string
}

type e008Store struct {
	paginas  map[string]e008Pagina
	fallo    error
	llamadas []e008Llamada
}

func newE008Store() *e008Store {
	return &e008Store{paginas: map[string]e008Pagina{}}
}

func (s *e008Store) ListUserSessions(_ context.Context, userID, startSK string, limit int) ([]appIdentity.SessionListRecord, string, error) {
	s.llamadas = append(s.llamadas, e008Llamada{userID: userID, startSK: startSK, limit: limit})
	if s.fallo != nil {
		return nil, "", s.fallo
	}
	pagina, ok := s.paginas[startSK]
	if !ok {
		return nil, "", nil
	}
	return pagina.registros, pagina.nextSK, nil
}

func e008Registro(sessionID, status string, creada time.Time) appIdentity.SessionListRecord {
	return appIdentity.SessionListRecord{
		SortKey:    "SESSION#" + sessionID,
		SessionID:  sessionID,
		CreatedAt:  creada,
		LastSeenAt: creada.Add(30 * time.Minute),
		Status:     status,
	}
}

func e008Servicio(store appIdentity.SessionsStore, clave []byte) *appIdentity.SessionsService {
	return appIdentity.NewSessionsService(store, clave).
		WithClock(func() time.Time { return e008Ahora })
}

// TestE008ListaSesionesDelTitular cubre el caso feliz: sólo los cinco campos del contrato, con
// `revoked` derivado del estado real y sin emitir cursor cuando no hay más páginas.
func TestE008ListaSesionesDelTitular(t *testing.T) {
	store := newE008Store()
	store.paginas[""] = e008Pagina{registros: []appIdentity.SessionListRecord{
		e008Registro(e008SesionA, appIdentity.StatusActive, e008Ahora.Add(-2*time.Hour)),
		e008Registro(e008SesionB, appIdentity.StatusRevoked, e008Ahora.Add(-time.Hour)),
	}}

	page, err := e008Servicio(store, e008Clave).ListSessions(context.Background(), appIdentity.SessionsQuery{
		UserID: e008Usuario,
	})
	if err != nil {
		t.Fatalf("listado valido devolvio error: %v", err)
	}
	if len(page.Items) != 2 {
		t.Fatalf("se esperaban 2 sesiones, hay %d", len(page.Items))
	}
	if page.Items[0].SessionID != e008SesionA || page.Items[0].Revoked {
		t.Fatalf("la primera sesion debe ser A y estar activa: %+v", page.Items[0])
	}
	if page.Items[1].SessionID != e008SesionB || !page.Items[1].Revoked {
		t.Fatalf("la segunda sesion debe ser B y estar revocada: %+v", page.Items[1])
	}
	if page.NextCursor != "" {
		t.Fatalf("sin pagina siguiente no debe emitirse cursor, hay %q", page.NextCursor)
	}
	if len(store.llamadas) != 1 {
		t.Fatalf("debe consultarse una sola vez, se consulto %d", len(store.llamadas))
	}
	llamada := store.llamadas[0]
	if llamada.userID != e008Usuario {
		t.Fatalf("la persistencia debe recibir el sub del token, recibio %q", llamada.userID)
	}
	if llamada.startSK != "" {
		t.Fatalf("la primera pagina no lleva ExclusiveStartKey, recibio %q", llamada.startSK)
	}
	if llamada.limit != appIdentity.SessionsDefaultLimit {
		t.Fatalf("el limite por defecto debe ser %d, recibio %d", appIdentity.SessionsDefaultLimit, llamada.limit)
	}
}

// TestE008PaginacionConCursorLigadoAIdentidad cubre T06: la primera página emite cursor, la
// segunda lo traduce en `ExclusiveStartKey` y no repite filas.
func TestE008PaginacionConCursorLigadoAIdentidad(t *testing.T) {
	store := newE008Store()
	store.paginas[""] = e008Pagina{
		registros: []appIdentity.SessionListRecord{
			e008Registro(e008SesionA, appIdentity.StatusActive, e008Ahora),
			e008Registro(e008SesionB, appIdentity.StatusActive, e008Ahora),
		},
		nextSK: "SESSION#" + e008SesionB,
	}
	store.paginas["SESSION#"+e008SesionB] = e008Pagina{
		registros: []appIdentity.SessionListRecord{e008Registro(e008SesionC, appIdentity.StatusActive, e008Ahora)},
	}

	servicio := e008Servicio(store, e008Clave)
	primera, err := servicio.ListSessions(context.Background(), appIdentity.SessionsQuery{UserID: e008Usuario, Limit: 2})
	if err != nil {
		t.Fatalf("primera pagina fallo: %v", err)
	}
	if primera.NextCursor == "" {
		t.Fatal("con mas filas debe emitirse nextCursor")
	}

	segunda, err := servicio.ListSessions(context.Background(), appIdentity.SessionsQuery{
		UserID: e008Usuario,
		Limit:  2,
		Cursor: primera.NextCursor,
	})
	if err != nil {
		t.Fatalf("segunda pagina fallo: %v", err)
	}
	if len(segunda.Items) != 1 || segunda.Items[0].SessionID != e008SesionC {
		t.Fatalf("la segunda pagina debe contener solo C: %+v", segunda.Items)
	}
	if segunda.NextCursor != "" {
		t.Fatalf("la ultima pagina no debe emitir cursor, hay %q", segunda.NextCursor)
	}
	if len(store.llamadas) != 2 || store.llamadas[1].startSK != "SESSION#"+e008SesionB {
		t.Fatalf("el cursor debe traducirse en ExclusiveStartKey: %+v", store.llamadas)
	}
}

// TestE008CursorDeOtroUsuarioRechazado cubre el aislamiento entre cuentas: el cursor firmado
// para otro `sub` no se acepta ni se convierte en consulta.
func TestE008CursorDeOtroUsuarioRechazado(t *testing.T) {
	store := newE008Store()
	store.paginas[""] = e008Pagina{
		registros: []appIdentity.SessionListRecord{e008Registro(e008SesionA, appIdentity.StatusActive, e008Ahora)},
		nextSK:    "SESSION#" + e008SesionA,
	}
	servicio := e008Servicio(store, e008Clave)

	primera, err := servicio.ListSessions(context.Background(), appIdentity.SessionsQuery{UserID: e008Otro, Limit: 1})
	if err != nil {
		t.Fatalf("la primera pagina del otro usuario fallo: %v", err)
	}
	if primera.NextCursor == "" {
		t.Fatal("se esperaba cursor del otro usuario")
	}

	store.llamadas = nil
	_, err = servicio.ListSessions(context.Background(), appIdentity.SessionsQuery{
		UserID: e008Usuario,
		Limit:  1,
		Cursor: primera.NextCursor,
	})
	if !errors.Is(err, appIdentity.ErrSessionsInvalidRequest) {
		t.Fatalf("un cursor de otro usuario debe ser 400, se obtuvo %v", err)
	}
	if len(store.llamadas) != 0 {
		t.Fatalf("un cursor ajeno no debe llegar a la persistencia: %+v", store.llamadas)
	}
}

// TestE008CursorManipuladoRechazado demuestra que el cursor está firmado: alterar un carácter
// invalida la firma.
func TestE008CursorManipuladoRechazado(t *testing.T) {
	store := newE008Store()
	store.paginas[""] = e008Pagina{
		registros: []appIdentity.SessionListRecord{e008Registro(e008SesionA, appIdentity.StatusActive, e008Ahora)},
		nextSK:    "SESSION#" + e008SesionA,
	}
	servicio := e008Servicio(store, e008Clave)
	primera, err := servicio.ListSessions(context.Background(), appIdentity.SessionsQuery{UserID: e008Usuario, Limit: 1})
	if err != nil || primera.NextCursor == "" {
		t.Fatalf("se esperaba cursor: %v", err)
	}

	// Se altera un carácter INTERMEDIO del cursor: cambiar sólo el último podría dejar intactos
	// los bits significativos de la base64 y producir la misma firma.
	posicion := len(primera.NextCursor) / 2
	sustituto := byte('A')
	if primera.NextCursor[posicion] == 'A' {
		sustituto = 'B'
	}
	manipulado := primera.NextCursor[:posicion] + string(sustituto) + primera.NextCursor[posicion+1:]
	_, err = servicio.ListSessions(context.Background(), appIdentity.SessionsQuery{
		UserID: e008Usuario,
		Limit:  1,
		Cursor: manipulado,
	})
	if !errors.Is(err, appIdentity.ErrSessionsInvalidRequest) {
		t.Fatalf("un cursor manipulado debe ser 400, se obtuvo %v", err)
	}
}

// TestE008CursorVencidoRechazado cubre la expiración del cursor.
func TestE008CursorVencidoRechazado(t *testing.T) {
	store := newE008Store()
	servicio := appIdentity.NewSessionsService(store, e008Clave).
		WithClock(func() time.Time { return e008Ahora }).
		WithCursorTTL(-time.Minute)

	store.paginas[""] = e008Pagina{
		registros: []appIdentity.SessionListRecord{e008Registro(e008SesionA, appIdentity.StatusActive, e008Ahora)},
		nextSK:    "SESSION#" + e008SesionA,
	}
	primera, err := servicio.ListSessions(context.Background(), appIdentity.SessionsQuery{UserID: e008Usuario, Limit: 1})
	if err != nil || primera.NextCursor == "" {
		t.Fatalf("se esperaba cursor: %v", err)
	}

	_, err = servicio.ListSessions(context.Background(), appIdentity.SessionsQuery{
		UserID: e008Usuario,
		Limit:  1,
		Cursor: primera.NextCursor,
	})
	if !errors.Is(err, appIdentity.ErrSessionsInvalidRequest) {
		t.Fatalf("un cursor vencido debe ser 400, se obtuvo %v", err)
	}
}

// TestE008CursorConOtroLimiteRechazado cubre T06: reutilizar un cursor emitido con otros
// filtros (`limit` distinto) es 400, no una página incoherente.
func TestE008CursorConOtroLimiteRechazado(t *testing.T) {
	store := newE008Store()
	store.paginas[""] = e008Pagina{
		registros: []appIdentity.SessionListRecord{e008Registro(e008SesionA, appIdentity.StatusActive, e008Ahora)},
		nextSK:    "SESSION#" + e008SesionA,
	}
	servicio := e008Servicio(store, e008Clave)
	primera, err := servicio.ListSessions(context.Background(), appIdentity.SessionsQuery{UserID: e008Usuario, Limit: 25})
	if err != nil || primera.NextCursor == "" {
		t.Fatalf("se esperaba cursor: %v", err)
	}

	_, err = servicio.ListSessions(context.Background(), appIdentity.SessionsQuery{
		UserID: e008Usuario,
		Limit:  50,
		Cursor: primera.NextCursor,
	})
	if !errors.Is(err, appIdentity.ErrSessionsInvalidRequest) {
		t.Fatalf("un cursor con otros filtros debe ser 400, se obtuvo %v", err)
	}
}

// TestE008LimiteFueraDeRangoRechazado: el contrato admite 1..100 y no recorta en silencio.
func TestE008LimiteFueraDeRangoRechazado(t *testing.T) {
	for _, limite := range []int{-1, 101, 1000} {
		store := newE008Store()
		_, err := e008Servicio(store, e008Clave).ListSessions(context.Background(), appIdentity.SessionsQuery{
			UserID: e008Usuario,
			Limit:  limite,
		})
		if !errors.Is(err, appIdentity.ErrSessionsInvalidRequest) {
			t.Fatalf("limite %d debe rechazarse con 400, se obtuvo %v", limite, err)
		}
		if len(store.llamadas) != 0 {
			t.Fatalf("un limite invalido no debe llegar a la persistencia: %+v", store.llamadas)
		}
	}

	// Cero significa "parametro ausente": se aplica el valor por defecto del contrato.
	store := newE008Store()
	if _, err := e008Servicio(store, e008Clave).ListSessions(context.Background(), appIdentity.SessionsQuery{UserID: e008Usuario}); err != nil {
		t.Fatalf("limite ausente debe usar el valor por defecto: %v", err)
	}
	if len(store.llamadas) != 1 || store.llamadas[0].limit != appIdentity.SessionsDefaultLimit {
		t.Fatalf("el limite por defecto debe aplicarse: %+v", store.llamadas)
	}
}

// TestE008FallaCerradoSinAlmacenOSinClave: nunca se lista sin persistencia ni sin firma.
func TestE008FallaCerradoSinAlmacenOSinClave(t *testing.T) {
	ctx := context.Background()
	if _, err := appIdentity.NewSessionsService(nil, e008Clave).ListSessions(ctx, appIdentity.SessionsQuery{UserID: e008Usuario}); !errors.Is(err, appIdentity.ErrSessionsUnavailable) {
		t.Fatalf("sin almacen debe ser 503, se obtuvo %v", err)
	}
	if _, err := appIdentity.NewSessionsService(newE008Store(), nil).ListSessions(ctx, appIdentity.SessionsQuery{UserID: e008Usuario}); !errors.Is(err, appIdentity.ErrSessionsUnavailable) {
		t.Fatalf("sin clave de firma debe ser 503, se obtuvo %v", err)
	}
	if _, err := e008Servicio(newE008Store(), e008Clave).ListSessions(ctx, appIdentity.SessionsQuery{}); !errors.Is(err, appIdentity.ErrSessionsUnauthorized) {
		t.Fatalf("sin sub debe ser 401, se obtuvo %v", err)
	}
}

// TestE008StoreCaidoEsIndisponible cubre T04: un fallo de DynamoDB se traduce en 503 y nunca en
// una lista vacía presentada como éxito.
func TestE008StoreCaidoEsIndisponible(t *testing.T) {
	store := newE008Store()
	store.fallo = errors.New("dynamodb inaccesible")

	page, err := e008Servicio(store, e008Clave).ListSessions(context.Background(), appIdentity.SessionsQuery{UserID: e008Usuario})
	if !errors.Is(err, appIdentity.ErrSessionsUnavailable) {
		t.Fatalf("un fallo de persistencia debe ser 503, se obtuvo %v", err)
	}
	if page != nil {
		t.Fatalf("no debe devolverse pagina parcial: %+v", page)
	}
}

// TestE008EstadoRevocado: regla del contrato `revoked = status != ACTIVE`, con la
// compatibilidad heredada de que un ítem sin `status` se considera activo.
func TestE008EstadoRevocado(t *testing.T) {
	casos := []struct {
		status   string
		revocado bool
	}{
		{"", false},
		{appIdentity.StatusActive, false},
		{"active", false},
		{"  ACTIVE  ", false},
		{appIdentity.StatusRevoked, true},
		{appIdentity.StatusConsumed, true},
		{"DISABLED", true},
	}
	for _, caso := range casos {
		if obtenido := appIdentity.SessionIsRevoked(caso.status); obtenido != caso.revocado {
			t.Fatalf("status %q: se esperaba revocado=%v, se obtuvo %v", caso.status, caso.revocado, obtenido)
		}
	}
}
