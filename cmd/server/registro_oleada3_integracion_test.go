package main

import (
	"sort"
	"testing"
)

// TestOleada3TreceRutasMontadas comprueba que, con todos los endpoints integrados, el registro
// contiene exactamente los 13 E-ID con su método y ruta reales. Es una prueba de integración del
// lote completo; no se ejecuta en ramas individuales donde aún faltan endpoints.
//
// La instantánea se toma y se libera el mutex ANTES de evaluar aserciones: `ids()` (y cualquier
// otro método del registro) adquiere el mismo `sync.Mutex` no reentrante, por lo que invocarlo
// dentro del bloqueo produciría un deadlock del propio test.
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
	obtenido := make(map[string]montajeOleada3, len(registroOleada3Global.porID))
	for id, m := range registroOleada3Global.porID {
		obtenido[id] = montajeOleada3{ID: m.ID, Metodo: m.Metodo, Ruta: m.Ruta}
	}
	registroOleada3Global.mu.Unlock()

	if len(obtenido) != len(esperado) {
		t.Fatalf("se esperaban %d endpoints montados, hay %d: %v", len(esperado), len(obtenido), idsOrdenados(obtenido))
	}
	for id, par := range esperado {
		m, ok := obtenido[id]
		if !ok {
			t.Fatalf("falta el endpoint %s; montados: %v", id, idsOrdenados(obtenido))
		}
		if m.Metodo != par[0] || m.Ruta != par[1] {
			t.Fatalf("%s: esperado %s %s, obtenido %s %s", id, par[0], par[1], m.Metodo, m.Ruta)
		}
	}
}

// idsOrdenados devuelve los E-ID de una instantánea del registro en orden determinista, sin
// tocar el mutex global (la instantánea ya no se comparte).
func idsOrdenados(instantanea map[string]montajeOleada3) []string {
	ids := make([]string, 0, len(instantanea))
	for id := range instantanea {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}
