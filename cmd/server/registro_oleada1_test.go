package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	nexumhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
)

func TestRegistroOleada1ListaAutorizada(t *testing.T) {
	if len(endpointsOleada1Autorizados) != 15 {
		t.Fatalf("la lista autorizada debe tener 15 IDs, tiene %d", len(endpointsOleada1Autorizados))
	}
	for _, id := range []string{"E012", "E013", "E015", "E016", "E017", "E018", "E019", "E022", "E023", "E024", "E025", "E068", "E069", "E070", "E071"} {
		if _, ok := endpointsOleada1Autorizados[id]; !ok {
			t.Fatalf("falta el ID autorizado %s", id)
		}
	}
	for _, id := range []string{"E014", "E020", "E072", "E075"} {
		if _, ok := endpointsOleada1Autorizados[id]; ok {
			t.Fatalf("el ID %s no pertenece a la Oleada 1 y no debe estar autorizado", id)
		}
	}
}

func TestRegistroOleada1CeroEndpoints(t *testing.T) {
	registro := nuevoRegistroOleada1()
	router := nexumhttp.NewRouter()
	registro.montar(&dependenciasOleada{Router: router})

	peticion := httptest.NewRequest(http.MethodGet, "/api/v1/users/me", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, peticion)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("con cero endpoints el router debe responder 404, respondio %d", rec.Code)
	}
}

func TestRegistroOleada1OrdenDeterminista(t *testing.T) {
	registro := nuevoRegistroOleada1()
	var orden []string
	registro.registrar(montajeOleada{ID: "E013", Metodo: http.MethodPatch, Ruta: "/users/me", Montar: func(*dependenciasOleada) { orden = append(orden, "E013") }})
	registro.registrar(montajeOleada{ID: "E012", Metodo: http.MethodGet, Ruta: "/users/me", Montar: func(*dependenciasOleada) { orden = append(orden, "E012") }})
	registro.montar(&dependenciasOleada{Router: nexumhttp.NewRouter()})

	if len(orden) != 2 || orden[0] != "E012" || orden[1] != "E013" {
		t.Fatalf("el orden de montaje debe ser determinista por ID, obtenido %v", orden)
	}
}

func TestRegistroOleada1MontaRouter(t *testing.T) {
	registro := nuevoRegistroOleada1()
	registro.registrar(montajeOleada{
		ID:     "E012",
		Metodo: http.MethodGet,
		Ruta:   "/users/me",
		Montar: func(d *dependenciasOleada) {
			d.Router.HandleFunc(http.MethodGet, "/users/me", func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			})
		},
	})
	router := nexumhttp.NewRouter()
	registro.montar(&dependenciasOleada{Router: router})

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/users/me", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("la ruta registrada debe responder 200, respondio %d", rec.Code)
	}
}

func TestRegistrarOleada1Global(t *testing.T) {
	anterior := registroOleada1Global
	t.Cleanup(func() { registroOleada1Global = anterior })
	registroOleada1Global = nuevoRegistroOleada1()

	registrarOleada1(montajeOleada{
		ID:     "E012",
		Metodo: http.MethodGet,
		Ruta:   "/users/me",
		Montar: func(*dependenciasOleada) {},
	})
	if len(registroOleada1Global.porID) != 1 {
		t.Fatalf("el registro global debe contener 1 endpoint, contiene %d", len(registroOleada1Global.porID))
	}
}

func TestRegistroOleada1RechazaDuplicadosYNoAutorizados(t *testing.T) {
	casos := []struct {
		nombre  string
		primero montajeOleada
		segundo montajeOleada
	}{
		{
			nombre:  "ID duplicado",
			primero: montajeOleada{ID: "E012", Metodo: http.MethodGet, Ruta: "/users/me", Montar: func(*dependenciasOleada) {}},
			segundo: montajeOleada{ID: "E012", Metodo: http.MethodPatch, Ruta: "/users/me", Montar: func(*dependenciasOleada) {}},
		},
		{
			nombre:  "metodo y ruta duplicados",
			primero: montajeOleada{ID: "E012", Metodo: http.MethodGet, Ruta: "/users/me", Montar: func(*dependenciasOleada) {}},
			segundo: montajeOleada{ID: "E013", Metodo: http.MethodGet, Ruta: "/users/me", Montar: func(*dependenciasOleada) {}},
		},
		{
			nombre:  "ID fuera de la oleada",
			primero: montajeOleada{ID: "E072", Metodo: http.MethodGet, Ruta: "/health/extra", Montar: func(*dependenciasOleada) {}},
		},
		{
			nombre:  "ruta con prefijo api",
			primero: montajeOleada{ID: "E012", Metodo: http.MethodGet, Ruta: "/api/v1/users/me", Montar: func(*dependenciasOleada) {}},
		},
		{
			nombre:  "metodo no admitido",
			primero: montajeOleada{ID: "E012", Metodo: "TRACE", Ruta: "/users/me", Montar: func(*dependenciasOleada) {}},
		},
		{
			nombre:  "montaje nulo",
			primero: montajeOleada{ID: "E012", Metodo: http.MethodGet, Ruta: "/users/me"},
		},
	}

	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			registro := nuevoRegistroOleada1()
			if caso.segundo.ID != "" {
				registro.registrar(caso.primero)
			}
			esperarPanic(t, func() {
				if caso.segundo.ID != "" {
					registro.registrar(caso.segundo)
					return
				}
				registro.registrar(caso.primero)
			})
		})
	}
}

func esperarPanic(t *testing.T, fn func()) {
	t.Helper()
	defer func() {
		if recuperado := recover(); recuperado == nil {
			t.Fatal("se esperaba panic y no ocurrio")
		}
	}()
	fn()
}
