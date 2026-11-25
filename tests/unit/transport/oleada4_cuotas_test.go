package transport_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	appAnalytics "github.com/nexumpr/Nexum-Backend/internal/application/analytics"
	appMovements "github.com/nexumpr/Nexum-Backend/internal/application/movements"
	internalhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
)

// cuotaGrabadora registra la clase y el identificador con los que el handler
// consulta el limitador, sin Redis ni estado compartido.
type cuotaGrabadora struct {
	permitido bool
	clases    []string
	ids       []string
}

func (l *cuotaGrabadora) Allow(_ context.Context, clase, identificador string) (bool, time.Duration, error) {
	l.clases = append(l.clases, clase)
	l.ids = append(l.ids, identificador)
	return l.permitido, time.Minute, nil
}

func e052CuotaHandler(fuente *e052HTTPSource, limitador internalhttp.LimitadorRuta) *internalhttp.AnalyticsInsightsHandler {
	servicio := appAnalytics.NewService(fuente).WithClock(func() time.Time {
		return time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	})
	return internalhttp.NewAnalyticsInsightsHandler(servicio, e004TokensValidos(), newE004Sesiones(), limitador, silencioso())
}

// TestOleada4CuotasPorDominio fija que cada endpoint usa su propia clase: un
// usuario agotado en analítica no consume la cuota de notificaciones, push ni
// preferencias.
func TestOleada4CuotasPorDominio(t *testing.T) {
	movimientos := func(usuario string) map[string][]appMovements.Movement {
		return map[string][]appMovements.Movement{usuario: e052HTTPMovimientos(usuario, 12)}
	}

	casos := []struct {
		nombre string
		clase  string
		build  func(*cuotaGrabadora) http.Handler
		req    func() *http.Request
	}{
		{
			nombre: "E052_insights", clase: "analytics",
			build: func(l *cuotaGrabadora) http.Handler {
				return e052CuotaHandler(&e052HTTPSource{byUser: movimientos(e004HTTPUsuario)}, l)
			},
			req: func() *http.Request {
				return e004Peticion(http.MethodGet, "/api/v1/analytics/insights?period=2026-11", "jwt-valido", "")
			},
		},
		{
			nombre: "E053_categories", clase: "analytics",
			build: func(l *cuotaGrabadora) http.Handler {
				return e053HTTPHandler(&e053HTTPSource{byUser: movimientos(e004HTTPUsuario)}, l)
			},
			req: func() *http.Request {
				return e004Peticion(http.MethodGet, "/api/v1/analytics/categories?period=2026-11", "jwt-valido", "")
			},
		},
		{
			nombre: "E054_patterns", clase: "analytics",
			build: func(l *cuotaGrabadora) http.Handler {
				return e054HTTPHandler(&e054HTTPSource{byUser: movimientos(e004HTTPUsuario)}, l)
			},
			req: func() *http.Request {
				return e004Peticion(http.MethodGet, "/api/v1/analytics/spending-patterns?period=2026-11", "jwt-valido", "")
			},
		},
		{
			nombre: "E055_recommendations", clase: "analytics",
			build: func(l *cuotaGrabadora) http.Handler {
				return e055HTTPHandler(&e055HTTPSource{byUser: movimientos(e004HTTPUsuario)}, l)
			},
			req: func() *http.Request {
				return e004Peticion(http.MethodGet, "/api/v1/analytics/recommendations?period=2026-11", "jwt-valido", "")
			},
		},
		{
			nombre: "E056_savings", clase: "analytics",
			build: func(l *cuotaGrabadora) http.Handler {
				return e056HTTPHandler(e056HTTPStoreValido(), l)
			},
			req: func() *http.Request {
				return e004Peticion(http.MethodGet, "/api/v1/analytics/savings-streak", "jwt-valido", "")
			},
		},
		{
			nombre: "E057_E058_preferences", clase: "preferences",
			build: func(l *cuotaGrabadora) http.Handler {
				return e057HTTPHandler(e057HTTPStoreValido(), l)
			},
			req: func() *http.Request {
				return e004Peticion(http.MethodGet, "/api/v1/notification-preferences", "jwt-valido", "")
			},
		},
		{
			nombre: "E059_push_register", clase: "push",
			build: func(l *cuotaGrabadora) http.Handler {
				return e059HTTPPost(nuevaE059HTTPStore(), e059HTTPSealer(t), l)
			},
			req: func() *http.Request {
				return e059Peticion(e059CuerpoValido(), "clave-idempotencia-cuota", "jwt-valido")
			},
		},
		{
			nombre: "E060_push_delete", clase: "push",
			build: func(l *cuotaGrabadora) http.Handler {
				return e060HTTPDelete(nuevaE059HTTPStore(), e059HTTPSealer(t), l)
			},
			req: func() *http.Request {
				return e060Peticion("018f90ef-08e5-7a68-b09f-2b67a47c92f0", "jwt-valido")
			},
		},
		{
			nombre: "E061_E062_E063_notifications", clase: "notifications",
			build: func(l *cuotaGrabadora) http.Handler {
				return e061HTTPHandler(e061HTTPStoreValido(), l, e061HTTPClave)
			},
			req: func() *http.Request {
				return e061HTTPPeticion(http.MethodGet, "/api/v1/notifications", "jwt-valido")
			},
		},
	}

	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			cuota := &cuotaGrabadora{permitido: true}
			caso.build(cuota).ServeHTTP(httptest.NewRecorder(), caso.req())
			if len(cuota.clases) != 1 {
				t.Fatalf("el limitador debe consultarse una vez, veces=%d", len(cuota.clases))
			}
			if cuota.clases[0] != caso.clase {
				t.Fatalf("clase de cuota incorrecta: %q, esperado %q", cuota.clases[0], caso.clase)
			}
			if cuota.ids[0] == "" {
				t.Fatal("el identificador de cuota no puede ser el sujeto vacio")
			}
		})
	}
}

// TestOleada4CuotasAislanUsuarios comprueba que el middleware traslada el `sub`
// verificado y no un valor compartido: dos usuarios producen identificadores
// distintos para la misma clase.
func TestOleada4CuotasAislanUsuarios(t *testing.T) {
	cuota := &cuotaGrabadora{permitido: true}
	handler := e052CuotaHandler(&e052HTTPSource{byUser: map[string][]appMovements.Movement{
		e004HTTPUsuario:  e052HTTPMovimientos(e004HTTPUsuario, 12),
		e004HTTPOtroUser: e052HTTPMovimientos(e004HTTPOtroUser, 12),
	}}, cuota)

	for _, token := range []string{"jwt-valido", "jwt-sujeto-ajeno"} {
		handler.ServeHTTP(httptest.NewRecorder(),
			e004Peticion(http.MethodGet, "/api/v1/analytics/insights?period=2026-11", token, ""))
	}
	if len(cuota.ids) != 2 || len(cuota.clases) != 2 {
		t.Fatalf("consultas de cuota inesperadas: ids=%v clases=%v", cuota.ids, cuota.clases)
	}
	if cuota.clases[0] != "analytics" || cuota.clases[1] != "analytics" {
		t.Fatalf("la clase debe ser analytics para ambos: %v", cuota.clases)
	}
	if cuota.ids[0] == cuota.ids[1] {
		t.Fatalf("dos usuarios no pueden compartir identificador de cuota: %q", cuota.ids[0])
	}
}
