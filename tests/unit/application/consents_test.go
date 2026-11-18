package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	appPrivacy "github.com/nexumpr/Nexum-Backend/internal/application/privacy"
)

// Pruebas de aplicación de E016 (`GET /api/v1/users/me/consents`): validación de límite, cursor
// opaco ligado a identidad y a filtros, reducción a los cinco campos del contrato y traducción
// de fallos de persistencia. El almacén es un doble de prueba; la composición productiva usa
// DynamoDB real.

const (
	e016Usuario = "11111111-2222-4333-8444-555555555555"
	e016Otro    = "99999999-8888-4777-8666-555555555555"

	e016IDNuevo  = "018f90ef-08e5-7a68-b09f-0000000000a1"
	e016IDNuevo2 = "018f90ef-08e5-7a68-b09f-0000000000b2"
	e016SKLegacy = "CONSENT#2026-11"
)

var (
	e016Ahora = time.Date(2026, 11, 18, 10, 0, 0, 0, time.UTC)
	e016Clave = []byte("clave-de-prueba-para-cursores-de-consentimientos")
)

type e016Llamada struct {
	userID  string
	startSK string
	limit   int
}

type e016Pagina struct {
	registros []appPrivacy.ConsentRecord
	nextSK    string
}

type e016Store struct {
	paginas  map[string]e016Pagina
	fallo    error
	llamadas []e016Llamada
}

func newE016Store() *e016Store {
	return &e016Store{paginas: map[string]e016Pagina{}}
}

func (s *e016Store) ListUserConsents(_ context.Context, userID, startSK string, limit int) ([]appPrivacy.ConsentRecord, string, error) {
	s.llamadas = append(s.llamadas, e016Llamada{userID: userID, startSK: startSK, limit: limit})
	if s.fallo != nil {
		return nil, "", s.fallo
	}
	pagina, ok := s.paginas[startSK]
	if !ok {
		return nil, "", nil
	}
	return pagina.registros, pagina.nextSK, nil
}

func e016RegistroNuevo(id string, granted bool, grabado time.Time) appPrivacy.ConsentRecord {
	return appPrivacy.ConsentRecord{
		SortKey:       "CONSENT#" + grabado.Format("2006-01-02T15:04:05.000000000Z") + "#" + id,
		ID:            id,
		NoticeVersion: "2026-11",
		Purpose:       appPrivacy.ConsentPurposeNotifications,
		Granted:       granted,
		RecordedAt:    grabado,
	}
}

func e016RegistroLegacy() appPrivacy.ConsentRecord {
	return appPrivacy.ConsentRecord{
		SortKey:       e016SKLegacy,
		ID:            "018f90ef-08e5-7a68-b09f-00000000c0de",
		NoticeVersion: "2026-11",
		Purpose:       appPrivacy.ConsentPurposePrivacyNotice,
		Granted:       true,
		RecordedAt:    e016Ahora.Add(-48 * time.Hour),
	}
}

func e016Servicio(store appPrivacy.ConsentsStore, clave []byte) *appPrivacy.ConsentsService {
	return appPrivacy.NewConsentsService(store, clave).
		WithClock(func() time.Time { return e016Ahora })
}

// TestE016ListaConsentimientosDelTitular cubre el caso feliz: sólo los cinco campos del
// contrato, `granted:false` conservado y sin emitir cursor cuando no hay más páginas.
func TestE016ListaConsentimientosDelTitular(t *testing.T) {
	store := newE016Store()
	store.paginas[""] = e016Pagina{registros: []appPrivacy.ConsentRecord{
		e016RegistroNuevo(e016IDNuevo, true, e016Ahora.Add(-time.Hour)),
		e016RegistroNuevo(e016IDNuevo2, false, e016Ahora.Add(-2*time.Hour)),
		e016RegistroLegacy(),
	}}

	page, err := e016Servicio(store, e016Clave).List(context.Background(), appPrivacy.ConsentsQuery{
		UserID: e016Usuario,
	})
	if err != nil {
		t.Fatalf("listado valido devolvio error: %v", err)
	}
	if len(page.Items) != 3 {
		t.Fatalf("se esperaban 3 consentimientos, hay %d", len(page.Items))
	}
	if page.Items[0].Granted != true || page.Items[1].Granted != false {
		t.Fatalf("granted debe conservarse tal cual: %+v", page.Items)
	}
	if page.Items[2].Purpose != appPrivacy.ConsentPurposePrivacyNotice || !page.Items[2].Granted {
		t.Fatalf("el item heredado debe proyectarse como privacyNotice concedido: %+v", page.Items[2])
	}
	if page.NextCursor != "" {
		t.Fatalf("sin pagina siguiente no debe emitirse cursor, hay %q", page.NextCursor)
	}
	if len(store.llamadas) != 1 {
		t.Fatalf("debe consultarse una sola vez, se consulto %d", len(store.llamadas))
	}
	llamada := store.llamadas[0]
	if llamada.userID != e016Usuario {
		t.Fatalf("la persistencia debe recibir el sub del token, recibio %q", llamada.userID)
	}
	if llamada.startSK != "" {
		t.Fatalf("la primera pagina no lleva ExclusiveStartKey, recibio %q", llamada.startSK)
	}
	if llamada.limit != appPrivacy.ConsentsDefaultLimit {
		t.Fatalf("el limite por defecto debe ser %d, recibio %d", appPrivacy.ConsentsDefaultLimit, llamada.limit)
	}
}

// TestE016ListaVaciaNuncaNil: la aplicación devuelve un slice vacío, no nil, para que el
// transporte emita `items: []`.
func TestE016ListaVaciaNuncaNil(t *testing.T) {
	page, err := e016Servicio(newE016Store(), e016Clave).List(context.Background(), appPrivacy.ConsentsQuery{
		UserID: e016Usuario,
	})
	if err != nil {
		t.Fatalf("listado vacio devolvio error: %v", err)
	}
	if page.Items == nil {
		t.Fatal("items debe ser un slice vacio, no nil")
	}
	if len(page.Items) != 0 {
		t.Fatalf("items debe estar vacio, hay %d", len(page.Items))
	}
}

// TestE016PaginacionConCursorLigadoAIdentidad: la primera página emite cursor, la segunda lo
// traduce en `ExclusiveStartKey` y no repite filas.
func TestE016PaginacionConCursorLigadoAIdentidad(t *testing.T) {
	store := newE016Store()
	primeraA := e016RegistroNuevo(e016IDNuevo, true, e016Ahora)
	primeraB := e016RegistroNuevo(e016IDNuevo2, false, e016Ahora.Add(-time.Hour))
	store.paginas[""] = e016Pagina{
		registros: []appPrivacy.ConsentRecord{primeraA, primeraB},
		nextSK:    primeraB.SortKey,
	}
	store.paginas[primeraB.SortKey] = e016Pagina{
		registros: []appPrivacy.ConsentRecord{e016RegistroLegacy()},
	}

	servicio := e016Servicio(store, e016Clave)
	primera, err := servicio.List(context.Background(), appPrivacy.ConsentsQuery{UserID: e016Usuario, Limit: 2})
	if err != nil {
		t.Fatalf("primera pagina fallo: %v", err)
	}
	if primera.NextCursor == "" {
		t.Fatal("con mas filas debe emitirse nextCursor")
	}

	segunda, err := servicio.List(context.Background(), appPrivacy.ConsentsQuery{
		UserID: e016Usuario,
		Limit:  2,
		Cursor: primera.NextCursor,
	})
	if err != nil {
		t.Fatalf("segunda pagina fallo: %v", err)
	}
	if len(segunda.Items) != 1 || segunda.Items[0].NoticeVersion != "2026-11" || segunda.Items[0].Purpose != appPrivacy.ConsentPurposePrivacyNotice {
		t.Fatalf("la segunda pagina debe contener solo el item heredado: %+v", segunda.Items)
	}
	if segunda.NextCursor != "" {
		t.Fatalf("la ultima pagina no debe emitir cursor, hay %q", segunda.NextCursor)
	}
	if len(store.llamadas) != 2 || store.llamadas[1].startSK != primeraB.SortKey {
		t.Fatalf("el cursor debe traducirse en ExclusiveStartKey: %+v", store.llamadas)
	}
}

// TestE016PaginacionSinDuplicadosNiSaltos recorre tres páginas de un elemento sobre cinco
// registros y comprueba que la unión es exactamente el historial, sin repetir ninguno.
func TestE016PaginacionSinDuplicadosNiSaltos(t *testing.T) {
	store := newE016Store()
	ids := []string{
		"018f90ef-08e5-7a68-b09f-0000000000a1",
		"018f90ef-08e5-7a68-b09f-0000000000b2",
		"018f90ef-08e5-7a68-b09f-0000000000c3",
		"018f90ef-08e5-7a68-b09f-0000000000d4",
		"018f90ef-08e5-7a68-b09f-0000000000e5",
	}
	var registros []appPrivacy.ConsentRecord
	for i, id := range ids {
		registros = append(registros, e016RegistroNuevo(id, i%2 == 0, e016Ahora.Add(-time.Duration(i)*time.Minute)))
	}
	// Cada página se sirve desde la SK del último elemento de la página anterior.
	store.paginas[""] = e016Pagina{registros: registros[:1], nextSK: registros[0].SortKey}
	for i := 1; i < len(registros); i++ {
		pagina := e016Pagina{registros: registros[i : i+1]}
		if i < len(registros)-1 {
			pagina.nextSK = registros[i].SortKey
		}
		store.paginas[registros[i-1].SortKey] = pagina
	}

	servicio := e016Servicio(store, e016Clave)
	vistos := map[string]bool{}
	cursor := ""
	for paginas := 0; paginas < 10; paginas++ {
		page, err := servicio.List(context.Background(), appPrivacy.ConsentsQuery{
			UserID: e016Usuario,
			Limit:  1,
			Cursor: cursor,
		})
		if err != nil {
			t.Fatalf("pagina %d fallo: %v", paginas, err)
		}
		for _, item := range page.Items {
			if vistos[item.ID] {
				t.Fatalf("el consentimiento %q se repitio entre paginas", item.ID)
			}
			vistos[item.ID] = true
		}
		if page.NextCursor == "" {
			break
		}
		cursor = page.NextCursor
	}
	if len(vistos) != len(registros) {
		t.Fatalf("la paginacion debe recorrer los %d registros, recorrio %d", len(registros), len(vistos))
	}
}

// TestE016CursorDeOtroUsuarioRechazado cubre el aislamiento entre cuentas: el cursor firmado
// para otro `sub` no se acepta ni se convierte en consulta.
func TestE016CursorDeOtroUsuarioRechazado(t *testing.T) {
	store := newE016Store()
	registro := e016RegistroNuevo(e016IDNuevo, true, e016Ahora)
	store.paginas[""] = e016Pagina{registros: []appPrivacy.ConsentRecord{registro}, nextSK: registro.SortKey}
	servicio := e016Servicio(store, e016Clave)

	primera, err := servicio.List(context.Background(), appPrivacy.ConsentsQuery{UserID: e016Otro, Limit: 1})
	if err != nil {
		t.Fatalf("la primera pagina del otro usuario fallo: %v", err)
	}
	if primera.NextCursor == "" {
		t.Fatal("se esperaba cursor del otro usuario")
	}

	store.llamadas = nil
	_, err = servicio.List(context.Background(), appPrivacy.ConsentsQuery{
		UserID: e016Usuario,
		Limit:  1,
		Cursor: primera.NextCursor,
	})
	if !errors.Is(err, appPrivacy.ErrConsentsInvalidRequest) {
		t.Fatalf("un cursor de otro usuario debe ser 400, se obtuvo %v", err)
	}
	if len(store.llamadas) != 0 {
		t.Fatalf("un cursor ajeno no debe llegar a la persistencia: %+v", store.llamadas)
	}
}

// TestE016CursorManipuladoRechazado demuestra que el cursor está firmado: alterar un carácter
// intermedio invalida la firma HMAC.
func TestE016CursorManipuladoRechazado(t *testing.T) {
	store := newE016Store()
	registro := e016RegistroNuevo(e016IDNuevo, true, e016Ahora)
	store.paginas[""] = e016Pagina{registros: []appPrivacy.ConsentRecord{registro}, nextSK: registro.SortKey}
	servicio := e016Servicio(store, e016Clave)
	primera, err := servicio.List(context.Background(), appPrivacy.ConsentsQuery{UserID: e016Usuario, Limit: 1})
	if err != nil || primera.NextCursor == "" {
		t.Fatalf("se esperaba cursor: %v", err)
	}

	posicion := len(primera.NextCursor) / 2
	sustituto := byte('A')
	if primera.NextCursor[posicion] == 'A' {
		sustituto = 'B'
	}
	manipulado := primera.NextCursor[:posicion] + string(sustituto) + primera.NextCursor[posicion+1:]
	_, err = servicio.List(context.Background(), appPrivacy.ConsentsQuery{
		UserID: e016Usuario,
		Limit:  1,
		Cursor: manipulado,
	})
	if !errors.Is(err, appPrivacy.ErrConsentsInvalidRequest) {
		t.Fatalf("un cursor manipulado debe ser 400, se obtuvo %v", err)
	}
}

// TestE016CursorVencidoRechazado cubre la expiración del cursor.
func TestE016CursorVencidoRechazado(t *testing.T) {
	store := newE016Store()
	registro := e016RegistroNuevo(e016IDNuevo, true, e016Ahora)
	store.paginas[""] = e016Pagina{registros: []appPrivacy.ConsentRecord{registro}, nextSK: registro.SortKey}
	servicio := appPrivacy.NewConsentsService(store, e016Clave).
		WithClock(func() time.Time { return e016Ahora }).
		WithCursorTTL(-time.Minute)

	primera, err := servicio.List(context.Background(), appPrivacy.ConsentsQuery{UserID: e016Usuario, Limit: 1})
	if err != nil || primera.NextCursor == "" {
		t.Fatalf("se esperaba cursor: %v", err)
	}
	_, err = servicio.List(context.Background(), appPrivacy.ConsentsQuery{
		UserID: e016Usuario,
		Limit:  1,
		Cursor: primera.NextCursor,
	})
	if !errors.Is(err, appPrivacy.ErrConsentsInvalidRequest) {
		t.Fatalf("un cursor vencido debe ser 400, se obtuvo %v", err)
	}
}

// TestE016CursorConOtroLimiteRechazado: reutilizar un cursor emitido con otros filtros
// (`limit` distinto) es 400, no una página incoherente.
func TestE016CursorConOtroLimiteRechazado(t *testing.T) {
	store := newE016Store()
	registro := e016RegistroNuevo(e016IDNuevo, true, e016Ahora)
	store.paginas[""] = e016Pagina{registros: []appPrivacy.ConsentRecord{registro}, nextSK: registro.SortKey}
	servicio := e016Servicio(store, e016Clave)
	primera, err := servicio.List(context.Background(), appPrivacy.ConsentsQuery{UserID: e016Usuario, Limit: 25})
	if err != nil || primera.NextCursor == "" {
		t.Fatalf("se esperaba cursor: %v", err)
	}
	_, err = servicio.List(context.Background(), appPrivacy.ConsentsQuery{
		UserID: e016Usuario,
		Limit:  50,
		Cursor: primera.NextCursor,
	})
	if !errors.Is(err, appPrivacy.ErrConsentsInvalidRequest) {
		t.Fatalf("un cursor con otros filtros debe ser 400, se obtuvo %v", err)
	}
}

// TestE016CursorDeOtroListadoRechazado demuestra la separación de dominio de la clave: un
// cursor firmado con otra clave derivada no se acepta.
func TestE016CursorDeOtroListadoRechazado(t *testing.T) {
	store := newE016Store()
	registro := e016RegistroNuevo(e016IDNuevo, true, e016Ahora)
	store.paginas[""] = e016Pagina{registros: []appPrivacy.ConsentRecord{registro}, nextSK: registro.SortKey}
	emisor := e016Servicio(store, []byte("clave-de-otro-listado"))
	primera, err := emisor.List(context.Background(), appPrivacy.ConsentsQuery{UserID: e016Usuario, Limit: 1})
	if err != nil || primera.NextCursor == "" {
		t.Fatalf("se esperaba cursor: %v", err)
	}
	_, err = e016Servicio(store, e016Clave).List(context.Background(), appPrivacy.ConsentsQuery{
		UserID: e016Usuario,
		Limit:  1,
		Cursor: primera.NextCursor,
	})
	if !errors.Is(err, appPrivacy.ErrConsentsInvalidRequest) {
		t.Fatalf("un cursor de otro listado debe ser 400, se obtuvo %v", err)
	}
}

// TestE016LimiteFueraDeRangoRechazado: el contrato admite 1..100 y no recorta en silencio.
func TestE016LimiteFueraDeRangoRechazado(t *testing.T) {
	for _, limite := range []int{-1, 101, 1000} {
		store := newE016Store()
		_, err := e016Servicio(store, e016Clave).List(context.Background(), appPrivacy.ConsentsQuery{
			UserID: e016Usuario,
			Limit:  limite,
		})
		if !errors.Is(err, appPrivacy.ErrConsentsInvalidRequest) {
			t.Fatalf("limite %d debe rechazarse con 400, se obtuvo %v", limite, err)
		}
		if len(store.llamadas) != 0 {
			t.Fatalf("un limite invalido no debe llegar a la persistencia: %+v", store.llamadas)
		}
	}

	store := newE016Store()
	if _, err := e016Servicio(store, e016Clave).List(context.Background(), appPrivacy.ConsentsQuery{UserID: e016Usuario}); err != nil {
		t.Fatalf("limite ausente debe usar el valor por defecto: %v", err)
	}
	if len(store.llamadas) != 1 || store.llamadas[0].limit != appPrivacy.ConsentsDefaultLimit {
		t.Fatalf("el limite por defecto debe aplicarse: %+v", store.llamadas)
	}
}

// TestE016FallaCerradoSinAlmacenOSinClave: nunca se lista sin persistencia ni sin firma.
func TestE016FallaCerradoSinAlmacenOSinClave(t *testing.T) {
	ctx := context.Background()
	if _, err := appPrivacy.NewConsentsService(nil, e016Clave).List(ctx, appPrivacy.ConsentsQuery{UserID: e016Usuario}); !errors.Is(err, appPrivacy.ErrConsentsUnavailable) {
		t.Fatalf("sin almacen debe ser 503, se obtuvo %v", err)
	}
	if _, err := appPrivacy.NewConsentsService(newE016Store(), nil).List(ctx, appPrivacy.ConsentsQuery{UserID: e016Usuario}); !errors.Is(err, appPrivacy.ErrConsentsUnavailable) {
		t.Fatalf("sin clave de firma debe ser 503, se obtuvo %v", err)
	}
	if _, err := e016Servicio(newE016Store(), e016Clave).List(ctx, appPrivacy.ConsentsQuery{}); !errors.Is(err, appPrivacy.ErrConsentsUnauthorized) {
		t.Fatalf("sin sub debe ser 401, se obtuvo %v", err)
	}
}

// TestE016StoreCaidoEsIndisponible: un fallo de DynamoDB se traduce en 503 y nunca en una lista
// vacía presentada como éxito.
func TestE016StoreCaidoEsIndisponible(t *testing.T) {
	store := newE016Store()
	store.fallo = errors.New("dynamodb inaccesible")

	page, err := e016Servicio(store, e016Clave).List(context.Background(), appPrivacy.ConsentsQuery{UserID: e016Usuario})
	if !errors.Is(err, appPrivacy.ErrConsentsUnavailable) {
		t.Fatalf("un fallo de persistencia debe ser 503, se obtuvo %v", err)
	}
	if page != nil {
		t.Fatalf("no debe devolverse pagina parcial: %+v", page)
	}
}

// TestE016EnumeracionCerradaDePropositos: el enum del contrato es cerrado.
func TestE016EnumeracionCerradaDePropositos(t *testing.T) {
	for _, valido := range []string{"privacyNotice", "notifications", "analytics"} {
		if !appPrivacy.IsConsentPurpose(valido) {
			t.Fatalf("%q debe ser un proposito valido", valido)
		}
	}
	for _, invalido := range []string{"", "PrivacyNotice", "marketing", "privacy_notice"} {
		if appPrivacy.IsConsentPurpose(invalido) {
			t.Fatalf("%q no debe ser un proposito valido", invalido)
		}
	}
}
