package main

import (
	"log/slog"
	"net/http"
	"time"

	dynamoLocal "github.com/nexumpr/Nexum-Backend/internal/adapters/dynamo"
	appReminders "github.com/nexumpr/Nexum-Backend/internal/application/reminders"
	cacheinfra "github.com/nexumpr/Nexum-Backend/internal/infrastructure/cache"
	nexumhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
)

func init() {
	registrarOleada5(montajeOleada5{
		ID: "E064", Metodo: http.MethodGet, Ruta: "/reminders", Montar: montarE064,
	})
	registrarOleada5(montajeOleada5{
		ID: "E065", Metodo: http.MethodPost, Ruta: "/reminders", Montar: montarE065,
	})
	registrarOleada5(montajeOleada5{
		ID: "E066", Metodo: http.MethodPatch, Ruta: "/reminders/{reminderId}", Montar: montarE066,
	})
	registrarOleada5(montajeOleada5{
		ID: "E067", Metodo: http.MethodDelete, Ruta: "/reminders/{reminderId}", Montar: montarE067,
	})
}

func servicioRecordatoriosOleada5(d *dependenciasOleada5, logger *slog.Logger) *appReminders.Service {
	cliente, ok := oleada5DynamoCliente(d)
	if !ok || cliente == nil || d.Tabla == "" {
		logger.Warn("E064-E067: persistencia no configurada; los endpoints responderan 503",
			slog.String("motivo", "dynamodb_no_configurado"))
	}
	if len(d.CursorKey) == 0 {
		logger.Warn("E064-E067: clave de cursor no configurada; los endpoints responderan 503",
			slog.String("motivo", "cursor_key_no_configurado"))
	}
	store := dynamoLocal.NewRemindersStore(cliente, d.Tabla)
	return appReminders.NewService(store, d.CursorKey)
}

func limitadorRecordatoriosOleada5(d *dependenciasOleada5) nexumhttp.LimitadorRuta {
	if d == nil || d.Redis == nil {
		return nil
	}
	return cacheinfra.NewAuthRateLimiter(d.Redis, cuotaAnalyticsPorMinuto, time.Minute)
}

func montarE064(d *dependenciasOleada5) {
	if d == nil || d.Router == nil {
		return
	}
	logger := loggerOleada5(d)
	servicio := servicioRecordatoriosOleada5(d, logger)
	handler := nexumhttp.NewRemindersListHandler(servicio, d.Tokens, d.Authorizer, limitadorRecordatoriosOleada5(d), logger)
	d.Router.HandleFunc(http.MethodGet, "/reminders", handler.ServeHTTP)
}

func montarE065(d *dependenciasOleada5) {
	if d == nil || d.Router == nil {
		return
	}
	logger := loggerOleada5(d)
	servicio := servicioRecordatoriosOleada5(d, logger)
	handler := nexumhttp.NewRemindersCreateHandler(servicio, d.Tokens, d.Authorizer, limitadorRecordatoriosOleada5(d), logger)
	d.Router.HandleFunc(http.MethodPost, "/reminders", handler.ServeHTTP)
}

func montarE066(d *dependenciasOleada5) {
	if d == nil || d.Router == nil {
		return
	}
	logger := loggerOleada5(d)
	servicio := servicioRecordatoriosOleada5(d, logger)
	handler := nexumhttp.NewRemindersPatchHandler(servicio, d.Tokens, d.Authorizer, limitadorRecordatoriosOleada5(d), logger)
	d.Router.HandleFunc(http.MethodPatch, "/reminders/{reminderId}", handler.ServeHTTP)
}

func montarE067(d *dependenciasOleada5) {
	if d == nil || d.Router == nil {
		return
	}
	logger := loggerOleada5(d)
	servicio := servicioRecordatoriosOleada5(d, logger)
	handler := nexumhttp.NewRemindersDeleteHandler(servicio, d.Tokens, d.Authorizer, limitadorRecordatoriosOleada5(d), logger)
	d.Router.HandleFunc(http.MethodDelete, "/reminders/{reminderId}", handler.ServeHTTP)
}
