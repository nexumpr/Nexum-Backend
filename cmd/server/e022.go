package main

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	cacheinfra "github.com/nexumpr/Nexum-Backend/internal/infrastructure/cache"
	nexumhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
)

// cuotaOleada1PorMinuto es la cuota distribuida de la Oleada 1 (Gate D-429): 120
// peticiones por minuto por IP en rutas públicas y por usuario en rutas autenticadas. El
// identificador lo hashea el limitador; nunca se guarda en claro.
const cuotaOleada1PorMinuto int64 = 120

func init() {
	registrarOleada1(montajeOleada{
		ID:     "E022",
		Metodo: http.MethodGet,
		Ruta:   "/catalogs/categories",
		Montar: montarE022,
	})
}

// montarE022 conecta `GET /api/v1/catalogs/categories` (E022).
//
// El catálogo es estático y versionado en código (Gate D-E022): no hay `GetItem`, no hay
// `Scan`, no se crea la partición `PUBLIC#` y no se amplía IAM. Redis sólo se usa como
// caché de lectura con clave pública versionada y es fail-open: si no está configurado o
// falla, se sirve el catálogo desde memoria.
//
// La cuota por IP requiere Redis; sin él se omite (429 no se emite) en lugar de simularse.
func montarE022(d *dependenciasOleada) {
	if d == nil || d.Router == nil {
		return
	}
	logger := d.Logger
	if logger == nil {
		logger = slog.Default()
	}

	var limitador nexumhttp.LimitadorRuta
	if d.Redis != nil {
		limitador = cacheinfra.NewAuthRateLimiter(d.Redis, cuotaOleada1PorMinuto, time.Minute)
	}

	cargar := func(ctx context.Context, clave string, ttl time.Duration, load func() ([]byte, error)) ([]byte, error) {
		return cacheinfra.GetOrLoadJSON(ctx, d.Redis, clave, ttl, load)
	}

	handler := nexumhttp.NewCategoriesHandler(limitador, cargar, logger)
	d.Router.HandleFunc(http.MethodGet, "/catalogs/categories", handler.ServeHTTP)
}
