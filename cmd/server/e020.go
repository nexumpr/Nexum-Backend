package main

import (
	"log/slog"
	"net/http"

	dynamoAdapter "github.com/nexumpr/Nexum-Backend/internal/adapters/dynamo"
	appExport "github.com/nexumpr/Nexum-Backend/internal/application/export"
	appIdentity "github.com/nexumpr/Nexum-Backend/internal/application/identity"
	appOnboarding "github.com/nexumpr/Nexum-Backend/internal/application/onboarding"
	appPreferences "github.com/nexumpr/Nexum-Backend/internal/application/preferences"
	appProfile "github.com/nexumpr/Nexum-Backend/internal/application/profile"
	cacheinfra "github.com/nexumpr/Nexum-Backend/internal/infrastructure/cache"
	nexumhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
	"github.com/nexumpr/Nexum-Backend/pkg/httputil"
)

func init() {
	registrarOleada3(montajeOleada3{
		ID:     "E020",
		Metodo: http.MethodGet,
		Ruta:   "/users/me/export",
		Montar: montarE020,
	})
}

// montarE020 conecta la exportación de datos propios (E020, RFC-022 §2).
//
// Fail-closed: sin persistencia, sin proveedor de tokens JWT, sin autorizador de sesiones o sin
// clave de firma de cursor, el handler responde 503 y no se exporta nada. No se registran
// dependencias simuladas ni se sirve una exportación parcial como si fuera completa.
//
// Lecturas: la exportación compone los lectores ya publicados — movimientos (E027), aportaciones
// (RFC-022 §5), reglas recurrentes (E032), metas (E038), presupuestos (RFC-022 §3), perfil
// (E012), ingreso (E024), consentimientos (E016), onboarding (E018) y preferencias (E068),
// todas con `Query` sobre `USER#<id>` o `GetItem` de una clave exacta y ninguna con `Scan`.
//
// No introduce tablas, índices, permisos ni secretos nuevos: la clave de cursor se deriva con
// separación de dominio de la clave HMAC ya existente en el bundle RFC-004 (la misma que usan
// las cuotas de login y los cursores de E008/E016/E038).
func montarE020(d *dependenciasOleada3) {
	if d == nil || d.Router == nil {
		return
	}
	logger := d.Logger
	if logger == nil {
		logger = slog.Default()
	}

	var fuente appExport.Fuente
	if cliente, ok := oleada3DynamoCliente(d); ok && d.Tabla != "" {
		tabla := d.Tabla
		fuente = dynamoAdapter.NewExportStore(dynamoAdapter.ExportStoreDeps{
			Cliente:         cliente,
			Tabla:           tabla,
			Movimientos:     dynamoAdapter.NewMovementsStore(cliente, tabla),
			Recurrentes:     dynamoAdapter.NewRecurringStore(cliente, tabla),
			Metas:           dynamoAdapter.NewGoalsStore(cliente, tabla),
			Perfil:          appProfile.NewService(dynamoAdapter.NewProfileStore(cliente, tabla)),
			Ingreso:         appIdentity.NewIncomeService(dynamoAdapter.NewIncomeStore(cliente, tabla)),
			Consentimientos: dynamoAdapter.NewConsentsStore(cliente, tabla),
			Onboarding:      appOnboarding.NewService(dynamoAdapter.NewOnboardingStore(cliente, tabla)),
			Preferencias:    dynamoAdapter.NewPreferencesStore(cliente, tabla),
		})
	} else {
		logger.Warn("E020: sin persistencia; el endpoint respondera 503",
			slog.String("motivo", "sin_cliente_dynamodb_o_tabla"))
	}

	servicio := appExport.NewService(fuente)
	claveBase, errClave := cacheinfra.LoadLoginHMACKey()
	if errClave != nil {
		logger.Warn("E020: clave HMAC base no disponible; el endpoint respondera 503",
			slog.String("motivo", "cursor_key_unavailable"))
	} else if secreto := httputil.DeriveCursorKey(claveBase, "e020"); len(secreto) > 0 {
		servicio = servicio.WithCursorSecret(secreto)
	} else {
		logger.Warn("E020: clave de cursor vacia; el endpoint respondera 503",
			slog.String("motivo", "cursor_key_unavailable"))
	}

	if d.Tokens == nil || d.Authorizer == nil {
		logger.Warn("E020: componentes de autenticacion incompletos; el endpoint respondera 503",
			slog.String("motivo", "sin_credenciales_o_autorizador"))
	}

	handler := nexumhttp.NewExportHandler(servicio, d.Tokens, d.Authorizer, logger)
	d.Router.HandleFunc(http.MethodGet, "/users/me/export", handler.ServeHTTP)
}

// Las variables de preferencias y onboarding se usan mediante sus servicios con el fin de
// mantener una única interpretación de los items heredados (versión por defecto, pasos
// canónicos y permisos). El ingreso usa el servicio de E024 para normalizar la frecuencia al
// valor canónico publicado.
var (
	_ = appIdentity.ErrIncomeNotFound
	_ = appOnboarding.ErrNotFound
	_ = appPreferences.DefaultLocale
)
