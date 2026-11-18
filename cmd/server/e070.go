package main

import (
	"net/http"
	"time"

	"github.com/redis/go-redis/v9"

	cacheinfra "github.com/nexumpr/Nexum-Backend/internal/infrastructure/cache"
	nexumhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
)

// Cuota de las rutas de contenido publico. Las dos rutas (E070 y E071) comparten
// una sola ventana por IP para que no se pueda duplicar el presupuesto alternando
// entre ellas. Es un control de abuso, no una dependencia: si Redis falla o la IP
// no se puede determinar, la lectura se sirve igualmente (fail-open).
const (
	claseCuotaContenido   = "public_content"
	maximoCuotaContenido  = 120
	ventanaCuotaContenido = time.Minute
)

func init() {
	registrarOleada1(montajeOleada{
		ID:     "E070",
		Metodo: http.MethodGet,
		Ruta:   "/help/faq",
		Montar: montarE070,
	})
}

// montarE070 conecta las preguntas frecuentes publicas. No requiere DynamoDB,
// tokens ni el bundle de secretos: contenido estatico versionado servido con cache
// publica y cuota opcional si Redis esta disponible.
func montarE070(d *dependenciasOleada) {
	if d == nil || d.Router == nil {
		return
	}
	handler := nexumhttp.NewFAQHandler(d.Redis, d.Logger)
	d.Router.HandleFunc(http.MethodGet, "/help/faq",
		nexumhttp.LimitarPorIP(limitadorContenido(d.Redis), claseCuotaContenido, handler.ServeHTTP))
}

// limitadorContenido devuelve el limitador real o nil. Con nil, LimitarPorIP
// devuelve el handler sin envolver: no se simula una cuota inexistente.
func limitadorContenido(clienteRedis *redis.Client) nexumhttp.LimitadorRuta {
	if clienteRedis == nil {
		return nil
	}
	return cacheinfra.NewAuthRateLimiter(clienteRedis, maximoCuotaContenido, ventanaCuotaContenido)
}
