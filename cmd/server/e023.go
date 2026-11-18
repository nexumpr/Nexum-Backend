package main

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	cacheinfra "github.com/nexumpr/Nexum-Backend/internal/infrastructure/cache"
	nexumhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
)

func init() {
	registrarOleada1(montajeOleada{
		ID:     "E023",
		Metodo: http.MethodGet,
		Ruta:   "/catalogs/frequencies",
		Montar: montarE023,
	})
}

// montarE023 conecta `GET /api/v1/catalogs/frequencies` (E023).
//
// El catálogo es estático y versionado en código (Gate D-E023): no hay `GetItem`, no hay
// `Scan`, no se crea la partición `PUBLIC#` y no se amplía IAM. Emite sólo los siete
// valores canónicos; los alias heredados `biweekly`/`yearly` no se listan (RFC-020 §2).
//
// Redis se usa únicamente como caché de lectura con clave pública versionada y es
// fail-open: si no está configurado o falla, el catálogo se sirve desde memoria. La cuota
// es la de la Oleada 1 (120/minuto por IP) y requiere Redis; sin él se omite (429 no se
// emite) en lugar de simularse.
func montarE023(d *dependenciasOleada) {
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

	handler := nexumhttp.NewFrequenciesHandler(limitador, cargar, logger)
	d.Router.HandleFunc(http.MethodGet, "/catalogs/frequencies", handler.ServeHTTP)
}
