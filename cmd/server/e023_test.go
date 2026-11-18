package main

import (
	"net/http"
	"testing"
)

// TestE023Registro comprueba que el init() de E023 declara el endpoint autorizado con el
// método y la ruta del contrato. No se invoca el montaje: la respuesta HTTP se prueba en
// tests/unit/transport.
func TestE023Registro(t *testing.T) {
	montaje, existe := registroOleada1Global.porID["E023"]
	if !existe {
		t.Fatal("E023 no esta registrado en la Oleada 1")
	}
	if montaje.Metodo != http.MethodGet {
		t.Fatalf("metodo inesperado: %q", montaje.Metodo)
	}
	if montaje.Ruta != "/catalogs/frequencies" {
		t.Fatalf("ruta inesperada: %q", montaje.Ruta)
	}
	if montaje.Montar == nil {
		t.Fatal("E023 no tiene funcion de montaje")
	}
}

// TestE023RutaNoColisionaConE022 comprueba que el catálogo de frecuencias no comparte
// método ni ruta con otro endpoint registrado: el registro lo rechazaría en el arranque,
// y esta prueba lo detecta antes.
func TestE023RutaNoColisionaConE022(t *testing.T) {
	for id, montaje := range registroOleada1Global.porID {
		if id == "E023" {
			continue
		}
		if montaje.Metodo == http.MethodGet && montaje.Ruta == "/catalogs/frequencies" {
			t.Fatalf("la ruta de E023 colisiona con %s", id)
		}
	}
}
