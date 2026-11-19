package dynamo_test

import (
	"errors"
	"testing"
	"time"

	dynamoAdapter "github.com/nexumpr/Nexum-Backend/internal/adapters/dynamo"
	appAccount "github.com/nexumpr/Nexum-Backend/internal/application/account"
)

// TestE014InvalidacionDenegadaPorACLCompletaPurga fija la correccion del defecto real de
// produccion: el ACL de Redis de A-080 deniega SCAN, de modo que DeletePrefix devuelve
// NOPERM. Ese permiso denegado no es una indisponibilidad: las entradas del usuario son
// inalcanzables tras el tombstone y su TTL esta acotado, por lo que el trabajo debe poder
// cerrar en PURGED conservando la verificacion durable.
func TestE014InvalidacionDenegadaPorACLCompletaPurga(t *testing.T) {
	m := newPurgaDynamo()
	e014SembrarCuenta(m, e014Usuario)
	store := dynamoAdapter.NewDeletionStore(m, e014Tabla)
	reloj := nuevoRelojPurga(e014T0)
	e014Programar(t, store, e014Usuario, e014RequestID, e014T0)

	cache := &e014Cache{falla: errors.New("NOPERM this user has no permissions to run the 'scan' command")}
	worker := e014Worker(store, cache, reloj)
	ciclos := e014CorrerHastaPurga(t, worker, reloj, m, 20)
	if ciclos < 1 {
		t.Fatalf("ciclos inesperados: %d", ciclos)
	}
	if estado := stringAttrDe(e014Control(m, e014Usuario, e014RequestID), "state"); estado != appAccount.PurgeStatePurged {
		t.Fatalf("estado esperado PURGED, obtenido %q", estado)
	}
}

// TestE014InvalidacionTransitoriaNoProducePurged garantiza que un Redis indisponible no se
// confunde con un permiso denegado: el trabajo queda en RETRY y nunca se anuncia PURGED.
func TestE014InvalidacionTransitoriaNoProducePurged(t *testing.T) {
	m := newPurgaDynamo()
	e014SembrarCuenta(m, e014Usuario)
	store := dynamoAdapter.NewDeletionStore(m, e014Tabla)
	reloj := nuevoRelojPurga(e014T0)
	e014Programar(t, store, e014Usuario, e014RequestID, e014T0)

	cache := &e014Cache{falla: errors.New("dial tcp 10.0.0.5:6379: connect: connection refused")}
	worker := e014Worker(store, cache, reloj)
	for ciclo := 0; ciclo < 5; ciclo++ {
		reloj.avanzar(time.Minute)
		worker.Ciclo(e014Contexto())
	}
	estado := stringAttrDe(e014Control(m, e014Usuario, e014RequestID), "state")
	if estado == appAccount.PurgeStatePurged {
		t.Fatalf("un fallo transitorio de Redis no puede producir PURGED")
	}
	if estado != appAccount.PurgeStateRetry && estado != appAccount.PurgeStateRunning {
		t.Fatalf("estado esperado RETRY/RUNNING, obtenido %q", estado)
	}
}
