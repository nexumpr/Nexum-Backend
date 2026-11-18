package main

import (
	"net/http"
	"testing"
)

// TestE022Registro comprueba que el init() de E022 declara el endpoint autorizado con el
// método y la ruta del contrato. No se invoca el montaje: la respuesta HTTP se prueba en
// tests/unit/transport.
func TestE022Registro(t *testing.T) {
	montaje, existe := registroOleada1Global.porID["E022"]
	if !existe {
		t.Fatal("E022 no esta registrado en la Oleada 1")
	}
	if montaje.Metodo != http.MethodGet {
		t.Fatalf("metodo inesperado: %q", montaje.Metodo)
	}
	if montaje.Ruta != "/catalogs/categories" {
		t.Fatalf("ruta inesperada: %q", montaje.Ruta)
	}
	if montaje.Montar == nil {
		t.Fatal("E022 no tiene funcion de montaje")
	}
}
