package main

import (
	dynamoAdapter "github.com/nexumpr/Nexum-Backend/internal/adapters/dynamo"
	appPrefs "github.com/nexumpr/Nexum-Backend/internal/application/notificationprefs"
)

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
