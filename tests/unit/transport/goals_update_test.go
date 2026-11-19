package transport_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	appGoals "github.com/nexumpr/Nexum-Backend/internal/application/goals"
	domainGoal "github.com/nexumpr/Nexum-Backend/internal/domain/goal"
	internalhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
)

// Pruebas HTTP de E041 (`PATCH /api/v1/goals/{goalId}`): 200 con ETag nuevo, 428 sin If-Match,
// 412 por validador débil u obsoleto, 400 de formato, 404 homogéneo, 409/422 de negocio, 401 y
// 503. El doble con estado y los sembrados viven en goals_queries_test.go (mismo paquete).

// UpdateGoal implementa el CAS del puerto sobre el doble de transporte.
func (s *e040HTTPStore) UpdateGoal(_ context.Context, write appGoals.UpdateWrite) (*domainGoal.Goal, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	filas := s.porUsuario[write.UserID]
	for i := range filas {
		if filas[i].Goal.ID != write.GoalID {
			continue
		}
		if filas[i].Goal.Version != write.ExpectedVersion {
			return nil, appGoals.ErrVersionConflict
		}
		if write.Patch.TargetCents != nil && filas[i].Goal.SavedAmount.Amount() > *write.Patch.TargetCents {
			return nil, appGoals.ErrTargetBelowSaved
		}
		actualizada, err := write.Patch.Apply(filas[i].Goal, write.Now)
		if err != nil {
			return nil, err
		}
		filas[i].Goal = actualizada
		copia := actualizada
		return &copia, nil
	}
	return nil, appGoals.ErrNotFound
}

// e041HTTPStoreFallo fuerza un error concreto en la escritura.
type e041HTTPStoreFallo struct {
	*e040HTTPStore
	err error
}

func (s e041HTTPStoreFallo) UpdateGoal(context.Context, appGoals.UpdateWrite) (*domainGoal.Goal, error) {
	return nil, s.err
}

func e041Handler(store appGoals.Store) http.Handler {
	return internalhttp.NewGoalPatchHandler(e038HTTPServicio(store), e004TokensValidos(), newE004Sesiones(), silencioso())
}

func e041Peticion(token, goalID, ifMatch, cuerpo string) *http.Request {
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/goals/"+goalID, strings.NewReader(cuerpo))
	req.SetPathValue("goalId", goalID)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if ifMatch != "" {
		req.Header.Set("If-Match", ifMatch)
	}
	return req
}

// TestE041HTTP200 comprueba el cuerpo completo, el ETag nuevo y que los importes históricos no
// cambian.
func TestE041HTTP200(t *testing.T) {
	store := e040Coleccion(t)
	cuerpo := `{"name":"Viaje largo","targetAmount":"400.00"}`
	rec := httptest.NewRecorder()
	e041Handler(store).ServeHTTP(rec, e041Peticion("jwt-valido", e038UUID(1), `"v1"`, cuerpo))

	if rec.Code != http.StatusOK {
		t.Fatalf("esperado 200, obtenido %d (%s)", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("ETag"); got != `"v2"` {
		t.Fatalf("ETag inesperado: %q", got)
	}
	if got := rec.Header().Get("Cache-Control"); got != "private, no-store" {
		t.Fatalf("Cache-Control inesperado: %q", got)
	}
	var salida map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &salida); err != nil {
		t.Fatalf("respuesta no es JSON: %v", err)
	}
	if len(salida) != 9 {
		t.Fatalf("el DTO debe tener 9 campos, tiene %d: %v", len(salida), salida)
	}
	if salida["name"] != "Viaje largo" || salida["targetAmount"] != "400.00" {
		t.Fatalf("campos modificados inesperados: %v", salida)
	}
	if salida["savedAmount"] != "100.00" || salida["openingSaved"] != "100.00" || salida["version"] != float64(2) {
		t.Fatalf("los importes historicos o la version no cuadran: %v", salida)
	}
}

// TestE041HTTPPrecondiciones cubre 428 sin cabecera, 412 débil y 412 obsoleto.
func TestE041HTTPPrecondiciones(t *testing.T) {
	casos := []struct {
		nombre   string
		ifMatch  string
		esperado int
	}{
		{"sin If-Match", "", http.StatusPreconditionRequired},
		{"validador debil", `W/"v1"`, http.StatusPreconditionFailed},
		{"version obsoleta", `"v9"`, http.StatusPreconditionFailed},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			rec := httptest.NewRecorder()
			handler := e041Handler(e040Coleccion(t))
			handler.ServeHTTP(rec, e041Peticion("jwt-valido", e038UUID(1), caso.ifMatch, `{"name":"x"}`))
			if rec.Code != caso.esperado {
				t.Fatalf("esperado %d, obtenido %d (%s)", caso.esperado, rec.Code, rec.Body.String())
			}
		})
	}
}

// TestE041HTTP400 cubre el formato de If-Match y del cuerpo, sin escribir nada.
func TestE041HTTP400(t *testing.T) {
	casos := []struct {
		nombre  string
		goalID  string
		ifMatch string
		cuerpo  string
	}{
		{"If-Match sin comillas", e038UUID(1), `v1`, `{"name":"x"}`},
		{"If-Match no numerico", e038UUID(1), `"vX"`, `{"name":"x"}`},
		{"json malformado", e038UUID(1), `"v1"`, `{"name":`},
		{"campo desconocido", e038UUID(1), `"v1"`, `{"savedAmount":"9.99"}`},
		{"sin cambios", e038UUID(1), `"v1"`, `{}`},
		{"nombre vacio", e038UUID(1), `"v1"`, `{"name":"   "}`},
		{"importe con un decimal", e038UUID(1), `"v1"`, `{"targetAmount":"400.0"}`},
		{"fecha imposible", e038UUID(1), `"v1"`, `{"targetDate":"2026-02-30"}`},
		{"estado fuera del enum", e038UUID(1), `"v1"`, `{"status":"pendiente"}`},
		{"goalId no uuid", "no-es-un-uuid", `"v1"`, `{"name":"x"}`},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			store := e040Coleccion(t)
			rec := httptest.NewRecorder()
			e041Handler(store).ServeHTTP(rec, e041Peticion("jwt-valido", caso.goalID, caso.ifMatch, caso.cuerpo))
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("esperado 400, obtenido %d (%s)", rec.Code, rec.Body.String())
			}
			if got := store.porUsuario[e004HTTPUsuario][0].Goal.Version; got != 1 {
				t.Fatalf("una solicitud invalida no debe escribir: version %d", got)
			}
		})
	}
}

// TestE041HTTP404Y401 cubre el recurso ajeno y la falta de sesión.
func TestE041HTTP404Y401(t *testing.T) {
	t.Run("meta ajena", func(t *testing.T) {
		rec := httptest.NewRecorder()
		e041Handler(e040Coleccion(t)).ServeHTTP(rec, e041Peticion("jwt-sujeto-ajeno", e038UUID(1), `"v1"`, `{"name":"x"}`))
		if rec.Code != http.StatusNotFound {
			t.Fatalf("esperado 404, obtenido %d", rec.Code)
		}
	})
	t.Run("meta inexistente", func(t *testing.T) {
		rec := httptest.NewRecorder()
		e041Handler(e040Coleccion(t)).ServeHTTP(rec, e041Peticion("jwt-valido", e038UUID(77), `"v1"`, `{"name":"x"}`))
		if rec.Code != http.StatusNotFound {
			t.Fatalf("esperado 404, obtenido %d", rec.Code)
		}
	})
	t.Run("sin bearer", func(t *testing.T) {
		rec := httptest.NewRecorder()
		e041Handler(e040Coleccion(t)).ServeHTTP(rec, e041Peticion("", e038UUID(1), `"v1"`, `{"name":"x"}`))
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("esperado 401, obtenido %d", rec.Code)
		}
	})
}

// TestE041HTTP422Y409 cubre las reglas de negocio y el conflicto de escritura.
func TestE041HTTP422Y409(t *testing.T) {
	t.Run("objetivo bajo el ahorro", func(t *testing.T) {
		rec := httptest.NewRecorder()
		// La meta sembrada tiene 100.00 ahorrados; 99.99 queda por debajo.
		e041Handler(e040Coleccion(t)).ServeHTTP(rec, e041Peticion("jwt-valido", e038UUID(1), `"v1"`, `{"targetAmount":"99.99"}`))
		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("esperado 422, obtenido %d (%s)", rec.Code, rec.Body.String())
		}
	})
	t.Run("transicion invalida", func(t *testing.T) {
		store := e040Coleccion(t)
		rec := httptest.NewRecorder()
		e041Handler(store).ServeHTTP(rec, e041Peticion("jwt-valido", e038UUID(3), `"v1"`, `{"status":"achieved"}`))
		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("esperado 422, obtenido %d (%s)", rec.Code, rec.Body.String())
		}
	})
	t.Run("conflicto de escritura", func(t *testing.T) {
		store := e041HTTPStoreFallo{e040HTTPStore: e040Coleccion(t), err: appGoals.ErrConflict}
		rec := httptest.NewRecorder()
		e041Handler(store).ServeHTTP(rec, e041Peticion("jwt-valido", e038UUID(1), `"v1"`, `{"name":"x"}`))
		if rec.Code != http.StatusConflict {
			t.Fatalf("esperado 409, obtenido %d (%s)", rec.Code, rec.Body.String())
		}
	})
}

// TestE041HTTP503 cubre el fail-closed sin servicio y con el almacén caído.
func TestE041HTTP503(t *testing.T) {
	t.Run("sin servicio", func(t *testing.T) {
		handler := internalhttp.NewGoalPatchHandler(nil, e004TokensValidos(), newE004Sesiones(), silencioso())
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, e041Peticion("jwt-valido", e038UUID(1), `"v1"`, `{"name":"x"}`))
		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("esperado 503, obtenido %d", rec.Code)
		}
	})
	t.Run("almacen caido", func(t *testing.T) {
		store := e041HTTPStoreFallo{e040HTTPStore: e040Coleccion(t), err: appGoals.ErrUnavailable}
		rec := httptest.NewRecorder()
		e041Handler(store).ServeHTTP(rec, e041Peticion("jwt-valido", e038UUID(1), `"v1"`, `{"name":"x"}`))
		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("esperado 503, obtenido %d (%s)", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "DEPENDENCY_UNAVAILABLE") {
			t.Fatalf("codigo de error inesperado: %s", rec.Body.String())
		}
	})
}

// TestE041HTTPAislamientoEntreCuentas comprueba que la misma versión de dos cuentas no se mezcla.
func TestE041HTTPAislamientoEntreCuentas(t *testing.T) {
	store := e040Coleccion(t)
	ajena := e038MetaHTTP(t, e004HTTPOtroUser, e038UUID(1), domainGoal.StatusActive, 5000)
	store.agregar(t, ajena)

	rec := httptest.NewRecorder()
	e041Handler(store).ServeHTTP(rec, e041Peticion("jwt-sujeto-ajeno", e038UUID(1), `"v1"`, `{"name":"propia"}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("esperado 200 en la cuenta ajena, obtenido %d (%s)", rec.Code, rec.Body.String())
	}
	if got := store.porUsuario[e004HTTPUsuario][0].Goal.Name; strings.Contains(got, "propia") {
		t.Fatalf("la modificacion de otra cuenta no debe tocar la primera: %q", got)
	}
}
