package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	appGoals "github.com/nexumpr/Nexum-Backend/internal/application/goals"
	domainGoal "github.com/nexumpr/Nexum-Backend/internal/domain/goal"
)

// Pruebas de aplicación de E038 (`GET /api/v1/goals`): totales sobre TODA la colección y no
// sobre la página, filtro por estado, cursor firmado ligado a identidad y filtros, y fail-closed
// sin clave de firma o sin persistencia. El doble con estado vive en goals_read_test.go.

// TestE038TotalesSobreTodaLaColeccion comprueba que `totals` no depende de la página: con
// `limit=2` sólo viajan dos metas, pero el agregado cubre las cinco de la colección.
func TestE038TotalesSobreTodaLaColeccion(t *testing.T) {
	store := e038Coleccion(t)
	servicio := e038Servicio(store)

	primera, err := servicio.List(context.Background(), appGoals.ListQuery{UserID: e038Usuario, Limit: 2})
	if err != nil {
		t.Fatalf("el listado no debe fallar: %v", err)
	}
	if len(primera.Items) != 2 {
		t.Fatalf("la pagina debe traer 2 metas, trajo %d", len(primera.Items))
	}
	if primera.Totals.ActiveCount != 3 {
		t.Fatalf("activeCount debe contar las tres activas de la coleccion, obtuvo %d", primera.Totals.ActiveCount)
	}
	if primera.Totals.Saved != "355.00" {
		t.Fatalf("saved debe sumar solo las activas (355.00), obtuvo %q", primera.Totals.Saved)
	}
	if primera.NextCursor == "" {
		t.Fatal("con cinco metas y pagina de dos debe haber cursor")
	}
	if len(store.agregados) != 1 || store.agregados[0] != e038Usuario {
		t.Fatalf("el agregado debe pedirse para el titular autenticado: %v", store.agregados)
	}

	// Recorrido completo del cursor: cinco metas distintas y ninguna repetida.
	vistas := map[string]bool{}
	cursor := ""
	for pagina := 0; pagina < 5; pagina++ {
		resultado, errLista := servicio.List(context.Background(), appGoals.ListQuery{
			UserID: e038Usuario, Limit: 2, Cursor: cursor,
		})
		if errLista != nil {
			t.Fatalf("pagina %d fallo: %v", pagina, errLista)
		}
		for _, item := range resultado.Items {
			if vistas[item.ID] {
				t.Fatalf("meta repetida entre paginas: %s", item.ID)
			}
			vistas[item.ID] = true
		}
		cursor = resultado.NextCursor
		if cursor == "" {
			break
		}
	}
	if len(vistas) != 5 {
		t.Fatalf("se esperaban 5 metas distintas, se vieron %d: %v", len(vistas), vistas)
	}
}

// TestE038FiltroStatusConservaTotales comprueba que el filtro afecta a `items` pero no a
// `totals`, que siempre describe la colección completa.
func TestE038FiltroStatusConservaTotales(t *testing.T) {
	store := e038Coleccion(t)
	pagina, err := e038Servicio(store).List(context.Background(), appGoals.ListQuery{
		UserID: e038Usuario, Status: "archived", Limit: 10,
	})
	if err != nil {
		t.Fatalf("el listado filtrado no debe fallar: %v", err)
	}
	if len(pagina.Items) != 1 || pagina.Items[0].ID != "meta-03" {
		t.Fatalf("el filtro archived debe devolver solo meta-03: %+v", pagina.Items)
	}
	if pagina.NextCursor != "" {
		t.Fatalf("una sola coincidencia no debe emitir cursor: %q", pagina.NextCursor)
	}
	if pagina.Totals.ActiveCount != 3 || pagina.Totals.Saved != "355.00" {
		t.Fatalf("los totales no dependen del filtro: %+v", pagina.Totals)
	}
	if consulta := store.consultas[len(store.consultas)-1]; !consulta.HasStatus || consulta.Status != domainGoal.StatusArchived {
		t.Fatalf("el filtro debe llegar a la persistencia: %+v", consulta)
	}
}

// alterarCursor cambia un carácter del cursor conservando el alfabeto base64url, de modo que el
// caso «manipulado» falle siempre por firma y no dependa de que el último carácter sea distinto.
func alterarCursor(cursor string) string {
	alterado := []byte(cursor)
	if alterado[5] == 'A' {
		alterado[5] = 'B'
	} else {
		alterado[5] = 'A'
	}
	return string(alterado)
}

// TestE038CursorLigadoAFiltrosYUsuario cubre manipulación, cambio de límite, cambio de filtro y
// cursor de otra cuenta: todos se rechazan con 400 sin revelar el motivo exacto.
func TestE038CursorLigadoAFiltrosYUsuario(t *testing.T) {
	store := e038Coleccion(t)
	servicio := e038Servicio(store)

	primera, err := servicio.List(context.Background(), appGoals.ListQuery{UserID: e038Usuario, Limit: 2})
	if err != nil || primera.NextCursor == "" {
		t.Fatalf("se necesita un cursor valido para la prueba: %v", err)
	}
	cursor := primera.NextCursor

	casos := []struct {
		nombre string
		query  appGoals.ListQuery
	}{
		{"otro limite", appGoals.ListQuery{UserID: e038Usuario, Limit: 3, Cursor: cursor}},
		{"otro filtro", appGoals.ListQuery{UserID: e038Usuario, Limit: 2, Status: "active", Cursor: cursor}},
		{"otro usuario", appGoals.ListQuery{UserID: e038Otro, Limit: 2, Cursor: cursor}},
		{"manipulado", appGoals.ListQuery{UserID: e038Usuario, Limit: 2, Cursor: alterarCursor(cursor)}},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			if _, err := servicio.List(context.Background(), caso.query); !errors.Is(err, appGoals.ErrInvalidRequest) {
				t.Fatalf("esperado ErrInvalidRequest, obtenido %v", err)
			}
		})
	}

	// Cursor vencido: el TTL negativo lo emite ya caducado.
	caducado, err := e038Servicio(store).WithCursorTTL(-time.Hour).List(context.Background(), appGoals.ListQuery{UserID: e038Usuario, Limit: 2})
	if err != nil || caducado.NextCursor == "" {
		t.Fatalf("se necesita un cursor caducado: %v", err)
	}
	if _, err := servicio.List(context.Background(), appGoals.ListQuery{UserID: e038Usuario, Limit: 2, Cursor: caducado.NextCursor}); !errors.Is(err, appGoals.ErrInvalidRequest) {
		t.Fatalf("un cursor vencido debe rechazarse con 400, obtenido %v", err)
	}
}

// TestE038Validaciones cubre límite y estado fuera de rango y el valor por defecto.
func TestE038Validaciones(t *testing.T) {
	store := e038Coleccion(t)
	servicio := e038Servicio(store)

	casos := []struct {
		nombre   string
		query    appGoals.ListQuery
		esperado error
	}{
		{"limite cero aplica el defecto", appGoals.ListQuery{UserID: e038Usuario, Limit: 0}, nil},
		{"limite excesivo", appGoals.ListQuery{UserID: e038Usuario, Limit: 101}, appGoals.ErrInvalidRequest},
		{"limite negativo", appGoals.ListQuery{UserID: e038Usuario, Limit: -1}, appGoals.ErrInvalidRequest},
		{"estado fuera del enum", appGoals.ListQuery{UserID: e038Usuario, Status: "pendiente"}, appGoals.ErrInvalidRequest},
		{"cursor demasiado corto", appGoals.ListQuery{UserID: e038Usuario, Cursor: "corto"}, appGoals.ErrInvalidRequest},
		{"sin usuario", appGoals.ListQuery{UserID: "  "}, appGoals.ErrUnauthorized},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			pagina, err := servicio.List(context.Background(), caso.query)
			if caso.esperado != nil {
				if !errors.Is(err, caso.esperado) {
					t.Fatalf("esperado %v, obtenido %v", caso.esperado, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("no debia fallar: %v", err)
			}
			if pagina.Totals.ActiveCount != 3 {
				t.Fatalf("los totales deben seguir presentes: %+v", pagina.Totals)
			}
		})
	}
}

// TestE038SinSecretoFallaCerrado garantiza que sin clave de firma no se pagina.
func TestE038SinSecretoFallaCerrado(t *testing.T) {
	servicio := appGoals.NewService(e038Coleccion(t)).WithClock(func() time.Time { return e038Ahora })
	if _, err := servicio.List(context.Background(), appGoals.ListQuery{UserID: e038Usuario}); !errors.Is(err, appGoals.ErrUnavailable) {
		t.Fatalf("esperado ErrUnavailable, obtenido %v", err)
	}
}

// TestE038FallosDePersistencia cubre los 503 de lista y agregado.
func TestE038FallosDePersistencia(t *testing.T) {
	t.Run("fallo de lista", func(t *testing.T) {
		store := e038Coleccion(t)
		store.errLista = errors.New("dynamodb inaccesible")
		if _, err := e038Servicio(store).List(context.Background(), appGoals.ListQuery{UserID: e038Usuario}); !errors.Is(err, appGoals.ErrUnavailable) {
			t.Fatalf("esperado ErrUnavailable, obtenido %v", err)
		}
	})
	t.Run("fallo de totales", func(t *testing.T) {
		store := e038Coleccion(t)
		store.errTotal = errors.New("dynamodb inaccesible")
		if _, err := e038Servicio(store).List(context.Background(), appGoals.ListQuery{UserID: e038Usuario}); !errors.Is(err, appGoals.ErrUnavailable) {
			t.Fatalf("esperado ErrUnavailable, obtenido %v", err)
		}
	})
}

// TestE038SinAlmacen falla cerrado antes de tocar el agregado.
func TestE038SinAlmacen(t *testing.T) {
	servicio := appGoals.NewService(nil).WithCursorSecret([]byte(e038Secreto))
	if _, err := servicio.List(context.Background(), appGoals.ListQuery{UserID: e038Usuario}); !errors.Is(err, appGoals.ErrUnavailable) {
		t.Fatalf("esperado ErrUnavailable, obtenido %v", err)
	}
}
