package main

import "testing"

// TestOleada3TreceRutasMontadas comprueba que, con todos los endpoints integrados, el registro
// contiene exactamente los 13 E-ID con su método y ruta reales. Es una prueba de integración del
// lote completo; no se ejecuta en ramas individuales donde aún faltan endpoints.
func TestOleada3TreceRutasMontadas(t *testing.T) {
	esperado := map[string][2]string{
		"E020": {"GET", "/users/me/export"},
		"E026": {"GET", "/financial-summary"},
		"E037": {"POST", "/recurring-payments/{recurringPaymentId}/payments"},
		"E042": {"DELETE", "/goals/{goalId}"},
		"E043": {"POST", "/goal-contributions/preview"},
		"E044": {"POST", "/goal-contributions"},
		"E045": {"GET", "/goal-contributions"},
		"E046": {"GET", "/goal-contributions/{contributionId}"},
		"E047": {"POST", "/goal-contributions/{contributionId}/reverse"},
		"E048": {"GET", "/budgets"},
		"E049": {"PUT", "/budgets"},
		"E050": {"GET", "/budgets/usage"},
		"E051": {"GET", "/dashboard"},
	}
	registroOleada3Global.mu.Lock()
	defer registroOleada3Global.mu.Unlock()
	if len(registroOleada3Global.porID) != len(esperado) {
		t.Fatalf("se esperaban %d endpoints montados, hay %d: %v", len(esperado), len(registroOleada3Global.porID), registroOleada3Global.ids())
	}
	for id, par := range esperado {
		m, ok := registroOleada3Global.porID[id]
		if !ok {
			t.Fatalf("falta el endpoint %s", id)
		}
		if m.Metodo != par[0] || m.Ruta != par[1] {
			t.Fatalf("%s: esperado %s %s, obtenido %s %s", id, par[0], par[1], m.Metodo, m.Ruta)
		}
	}
}
