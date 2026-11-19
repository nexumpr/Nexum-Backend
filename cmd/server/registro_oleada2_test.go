package main

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	nexumhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
)

// idsOleada2Aprobados es la lista del Gate H08E_OLEADA_2 §2. La prueba falla si el
// código y el Gate divergen: no se pueden colar endpoints de otras oleadas.
var idsOleada2Aprobados = []string{
	"E014", "E027", "E028", "E029", "E030", "E031", "E032", "E033", "E034", "E035",
	"E036", "E038", "E039", "E040", "E041",
}

func TestListaCerradaOleada2CoincideConElGate(t *testing.T) {
	if len(endpointsOleada2Autorizados) != len(idsOleada2Aprobados) {
		t.Fatalf("la lista cerrada tiene %d IDs y el Gate %d", len(endpointsOleada2Autorizados), len(idsOleada2Aprobados))
	}
	for _, id := range idsOleada2Aprobados {
		if _, ok := endpointsOleada2Autorizados[id]; !ok {
			t.Fatalf("falta el endpoint aprobado %s en la lista cerrada", id)
		}
	}
	// Endpoints de otras oleadas no deben estar autorizados aquí.
	for _, ajeno := range []string{"E020", "E026", "E037", "E042", "E072", "E075"} {
		if _, ok := endpointsOleada2Autorizados[ajeno]; ok {
			t.Fatalf("endpoint ajeno a la Oleada 2 autorizado por error: %s", ajeno)
		}
	}
}

func TestRegistroOleada2RechazaIDNoAutorizado(t *testing.T) {
	exigirPanic(t, func() {
		nuevoRegistroOleada2().registrar(montajeOleada2{
			ID: "E037", Metodo: http.MethodPost, Ruta: "/x", Montar: func(*dependenciasOleada2) {},
		})
	})
}

func TestRegistroOleada2RechazaDuplicados(t *testing.T) {
	r := nuevoRegistroOleada2()
	ok := func(*dependenciasOleada2) {}
	r.registrar(montajeOleada2{ID: "E027", Metodo: http.MethodGet, Ruta: "/movements", Montar: ok})
	exigirPanic(t, func() {
		r.registrar(montajeOleada2{ID: "E027", Metodo: http.MethodGet, Ruta: "/movements", Montar: ok})
	})
	exigirPanic(t, func() {
		r.registrar(montajeOleada2{ID: "E029", Metodo: http.MethodGet, Ruta: "/movements", Montar: ok})
	})
}

func TestRegistroOleada2RechazaRutaConPrefijoApiYMetodoInvalido(t *testing.T) {
	ok := func(*dependenciasOleada2) {}
	exigirPanic(t, func() {
		nuevoRegistroOleada2().registrar(montajeOleada2{ID: "E027", Metodo: http.MethodGet, Ruta: "/api/v1/movements", Montar: ok})
	})
	exigirPanic(t, func() {
		nuevoRegistroOleada2().registrar(montajeOleada2{ID: "E027", Metodo: "FETCH", Ruta: "/movements", Montar: ok})
	})
	exigirPanic(t, func() {
		nuevoRegistroOleada2().registrar(montajeOleada2{ID: "E027", Metodo: http.MethodGet, Ruta: "/movements"})
	})
}

// TestMontarOleada2SinRegistrosNoHaceNada garantiza que el andamiaje es válido con
// cero endpoints: no se registran rutas ficticias.
func TestMontarOleada2SinRegistrosNoHaceNada(t *testing.T) {
	router := nexumhttp.NewRouter()
	nuevoRegistroOleada2().montar(&dependenciasOleada2{Router: router, Logger: slog.Default()})

	for _, ruta := range []string{"/api/v1/movements", "/api/v1/goals", "/api/v1/recurring-payments"} {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, ruta, nil))
		if rec.Code != http.StatusNotFound {
			t.Fatalf("%s deberia responder 404 con cero registros, obtuvo %d", ruta, rec.Code)
		}
	}
}

// TestMontarOleada2OrdenDeterminista comprueba que el montaje sigue el orden por ID
// y no el orden de registro (el orden de init() no es contractual).
func TestMontarOleada2OrdenDeterminista(t *testing.T) {
	r := nuevoRegistroOleada2()
	var orden []string
	reg := func(id, ruta string) {
		r.registrar(montajeOleada2{
			ID: id, Metodo: http.MethodGet, Ruta: ruta,
			Montar: func(*dependenciasOleada2) { orden = append(orden, id) },
		})
	}
	reg("E040", "/goals/id40")
	reg("E027", "/movements")
	reg("E014", "/users/me")

	r.montar(&dependenciasOleada2{Router: nexumhttp.NewRouter(), Logger: slog.Default()})

	esperado := []string{"E014", "E027", "E040"}
	if !reflect.DeepEqual(orden, esperado) {
		t.Fatalf("orden de montaje inesperado: %v", orden)
	}
}

func TestRegistroOleada2IdsOrdenados(t *testing.T) {
	r := nuevoRegistroOleada2()
	ok := func(*dependenciasOleada2) {}
	r.registrar(montajeOleada2{ID: "E041", Metodo: http.MethodPatch, Ruta: "/goals/{goalId}", Montar: ok})
	r.registrar(montajeOleada2{ID: "E014", Metodo: http.MethodDelete, Ruta: "/users/me", Montar: ok})

	ids := r.ids()
	if len(ids) != 2 || ids[0] != "E014" || ids[1] != "E041" {
		t.Fatalf("ids inesperados: %v", ids)
	}
}

func exigirPanic(t *testing.T, fn func()) {
	t.Helper()
	defer func() {
		if rec := recover(); rec == nil {
			t.Fatal("se esperaba panic de validacion y no ocurrio")
		} else if strings.TrimSpace(strings.ToLower(rec.(string))) == "" {
			t.Fatal("el panic debe explicar el motivo")
		}
	}()
	fn()
}

// TestRegistrarOleada2GlobalValida cubre el punto de entrada que usan los init() de
// cada endpoint: un ID fuera del Gate debe abortar el arranque.
func TestRegistrarOleada2GlobalValida(t *testing.T) {
	exigirPanic(t, func() {
		registrarOleada2(montajeOleada2{
			ID: "E037", Metodo: http.MethodPost, Ruta: "/no-autorizado",
			Montar: func(*dependenciasOleada2) {},
		})
	})
}

// TestOleada2DynamoClienteFallaCerrado comprueba que los endpoints reciben ok=false
// cuando no hay cliente, para responder 503 en vez de operar sobre nil.
func TestOleada2DynamoClienteFallaCerrado(t *testing.T) {
	if _, ok := oleada2DynamoCliente(nil); ok {
		t.Fatal("con dependencias nulas no debe haber cliente")
	}
	if _, ok := oleada2DynamoCliente(&dependenciasOleada2{}); ok {
		t.Fatal("con Dynamo nulo no debe haber cliente")
	}
}
