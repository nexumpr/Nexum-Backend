package main

import (
	"net/http"
	"testing"
)

// TestRegistroOleada3ListaCerrada verifica que el registro solo admite los 13 E-ID del Gate y
// que E037 está presente. Un ID fuera de la lista debe fallar en el arranque.
func TestRegistroOleada3ListaCerrada(t *testing.T) {
	if len(endpointsOleada3Autorizados) != 13 {
		t.Fatalf("se esperaban 13 E-ID autorizados, hay %d", len(endpointsOleada3Autorizados))
	}
	for _, id := range []string{
		"E020", "E026", "E037", "E042", "E043", "E044", "E045",
		"E046", "E047", "E048", "E049", "E050", "E051",
	} {
		if _, ok := endpointsOleada3Autorizados[id]; !ok {
			t.Fatalf("falta el E-ID autorizado %s", id)
		}
	}
}

// TestRegistroOleada3IdsYDuplicados usa `ids()` (evita U1000) y comprueba que el registro
// rechaza un endpoint duplicado antes de servir tráfico.
func TestRegistroOleada3IdsYDuplicados(t *testing.T) {
	r := nuevoRegistroOleada3()
	montaje := montajeOleada3{
		ID:     "E037",
		Metodo: http.MethodPost,
		Ruta:   "/recurring-payments/{recurringPaymentId}/payments",
		Montar: func(*dependenciasOleada3) {},
	}
	r.registrar(montaje)
	ids := r.ids()
	if len(ids) != 1 || ids[0] != "E037" {
		t.Fatalf("ids inesperados: %v", ids)
	}
	defer func() {
		if recover() == nil {
			t.Fatal("un endpoint duplicado debe provocar panic en el arranque")
		}
	}()
	r.registrar(montaje)
}

// TestRegistroOleada3GlobalContieneE037 comprueba que el init de E037 llegó al registro global.
func TestRegistroOleada3GlobalContieneE037(t *testing.T) {
	encontrado := false
	for _, id := range registroOleada3Global.ids() {
		if id == "E037" {
			encontrado = true
			break
		}
	}
	if !encontrado {
		t.Fatalf("E037 no está registrado en el registro global: %v", registroOleada3Global.ids())
	}
}

// TestMontarOleada3SinRouterNoPanica asegura que el montaje es seguro sin dependencias.
func TestMontarOleada3SinRouterNoPanica(t *testing.T) {
	montarOleada3(nil)
	montarOleada3(&dependenciasOleada3{})
}
