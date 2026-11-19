package main

import (
	"net/http"
	"sort"
	"testing"
)

// TestOleada2RutasMontadas comprueba sobre el registro REAL que las 15 operaciones de la
// Oleada 2 están montadas exactamente una vez, con su método y su ruta contractuales.
// No cuenta archivos ni carpetas: usa el mismo mecanismo que ejecuta main().
func TestOleada2RutasMontadas(t *testing.T) {
	esperadas := map[string]struct {
		metodo string
		ruta   string
	}{
		"E014": {http.MethodDelete, "/users/me"},
		"E027": {http.MethodGet, "/movements"},
		"E028": {http.MethodPost, "/movements"},
		"E029": {http.MethodGet, "/movements/{movementId}"},
		"E030": {http.MethodPatch, "/movements/{movementId}"},
		"E031": {http.MethodDelete, "/movements/{movementId}"},
		"E032": {http.MethodGet, "/recurring-payments"},
		"E033": {http.MethodPost, "/recurring-payments"},
		"E034": {http.MethodGet, "/recurring-payments/{recurringPaymentId}"},
		"E035": {http.MethodPatch, "/recurring-payments/{recurringPaymentId}"},
		"E036": {http.MethodDelete, "/recurring-payments/{recurringPaymentId}"},
		"E038": {http.MethodGet, "/goals"},
		"E039": {http.MethodPost, "/goals"},
		"E040": {http.MethodGet, "/goals/{goalId}"},
		"E041": {http.MethodPatch, "/goals/{goalId}"},
	}

	ids := registroOleada2Global.ids()
	if len(ids) != len(esperadas) {
		t.Fatalf("se esperaban %d rutas montadas, hay %d: %v", len(esperadas), len(ids), ids)
	}
	sort.Strings(ids)
	for _, id := range ids {
		esperada, ok := esperadas[id]
		if !ok {
			t.Fatalf("endpoint no autorizado montado: %s", id)
		}
		m := registroOleada2Global.porID[id]
		if m.Metodo != esperada.metodo || m.Ruta != esperada.ruta {
			t.Fatalf("%s: se esperaba %s %s, se monto %s %s", id, esperada.metodo, esperada.ruta, m.Metodo, m.Ruta)
		}
		if m.Montar == nil {
			t.Fatalf("%s: montaje nulo", id)
		}
	}
	vistas := map[string]string{}
	for _, id := range ids {
		m := registroOleada2Global.porID[id]
		clave := m.Metodo + " " + m.Ruta
		if anterior, existe := vistas[clave]; existe {
			t.Fatalf("ruta duplicada %s: %s y %s", clave, anterior, id)
		}
		vistas[clave] = id
	}
}
