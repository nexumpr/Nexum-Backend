package main

import (
	"time"

	dynamoAdapter "github.com/nexumpr/Nexum-Backend/internal/adapters/dynamo"
	appPrefs "github.com/nexumpr/Nexum-Backend/internal/application/notificationprefs"
	cacheinfra "github.com/nexumpr/Nexum-Backend/internal/infrastructure/cache"
	nexumhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
)

// limitesCuotaOleada4 documenta el mapa de clases de la Oleada 4. POLITICAS_TRANSVERSALES_H06
// fija 60/min por sujeto para operaciones autenticadas; analítica, preferencias, push y
// notificaciones son operaciones autenticadas, no financieras mutantes (20/min). El límite
// aprobado de analítica no cambia: sólo se separan los contadores por dominio para que un
// usuario agotado en uno no consuma la cuota de los demás.
var limitesCuotaOleada4 = map[string]int64{
	"analytics":     cuotaAnalyticsPorMinuto,
	"preferences":   cuotaAnalyticsPorMinuto,
	"push":          cuotaAnalyticsPorMinuto,
	"notifications": cuotaAnalyticsPorMinuto,
}

// limitadorOleada4 construye el limitador compartido de E052–E063. Devuelve nil sin
// Redis: así los endpoints siguen fail-open y no se simula una cuota.
func limitadorOleada4(d *dependenciasOleada4) nexumhttp.LimitadorRuta {
	if d == nil || d.Redis == nil {
		return nil
	}
	return cacheinfra.NewAuthRateLimiterPorClase(d.Redis, cuotaAnalyticsPorMinuto, limitesCuotaOleada4, time.Minute)
}

// preferenciasWorkerOleada4 construye el caso de uso que el emisor FCM consulta
// antes de cada envío. Reutiliza la tabla única y el mismo adaptador de E057/E058;
// no crea recursos ni modifica las preferencias del titular.
func preferenciasWorkerOleada4(d *dependenciasOleada4) *appPrefs.Service {
	if d == nil {
		return nil
	}
	cliente, ok := oleada4DynamoCliente(d)
	if !ok || cliente == nil || d.Tabla == "" {
		return nil
	}
	store := dynamoAdapter.NewNotificationPreferencesStore(cliente, d.Tabla)
	return appPrefs.NewService(store)
}
