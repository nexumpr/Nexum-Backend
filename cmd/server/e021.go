package main

import (
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	dynamoAdapter "github.com/nexumpr/Nexum-Backend/internal/adapters/dynamo"
	appBootstrap "github.com/nexumpr/Nexum-Backend/internal/application/bootstrap"
	appSummary "github.com/nexumpr/Nexum-Backend/internal/application/financialsummary"
	appProfile "github.com/nexumpr/Nexum-Backend/internal/application/profile"
	nexumhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
)

func init() {
	registrarOleada5(montajeOleada5{
		ID: "E021", Metodo: http.MethodGet, Ruta: "/bootstrap", Montar: montarE021,
	})
}

// catalogVersionOleada5 es una etiqueta de catálogo controlada por configuración.
// Por defecto se usa el mes UTC vigente; nunca incluye secretos ni datos de usuario.
func catalogVersionOleada5() string {
	if valor := strings.TrimSpace(os.Getenv("NEXUM_CATALOG_VERSION")); valor != "" {
		return valor
	}
	return time.Now().UTC().Format("2006-01")
}

func montarE021(d *dependenciasOleada5) {
	if d == nil || d.Router == nil {
		return
	}
	logger := loggerOleada5(d)

	var perfilAPI dynamoAdapter.ProfileAPI
	if d.Dynamo != nil {
		perfilAPI = d.Dynamo
	}
	perfiles := appProfile.NewService(dynamoAdapter.NewProfileStore(perfilAPI, d.Tabla))

	cliente, ok := oleada5DynamoCliente(d)
	if !ok || cliente == nil || d.Tabla == "" {
		logger.Warn("E021: persistencia no configurada; el endpoint respondera 503",
			slog.String("motivo", "dynamodb_no_configurado"))
	}
	resumenes := appSummary.NewService(dynamoAdapter.NewFinancialSummaryStore(cliente, d.Tabla))

	servicio := appBootstrap.NewService(perfiles, resumenes, catalogVersionOleada5())
	handler := nexumhttp.NewBootstrapHandler(
		servicio, d.Tokens, d.Authorizer, limitadorRecordatoriosOleada5(d), logger,
	)
	d.Router.HandleFunc(http.MethodGet, "/bootstrap", handler.ServeHTTP)
}
