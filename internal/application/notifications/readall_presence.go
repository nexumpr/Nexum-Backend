package notifications

import "context"

// PresenciaReadAll publica la presencia de un worker READALL en un registro
// compartido por todos los nodos. El worker late al arrancar y en cada ciclo; la
// implementación impone un TTL corto, de modo que un nodo caído desaparece sin
// intervención manual. Retirar acelera la baja ordenada, pero el TTL es la
// autoridad: nunca se depende de un cierre limpio.
type PresenciaReadAll interface {
	// Latir renueva la presencia del propietario indicado.
	Latir(ctx context.Context, owner string) error
	// Retirar elimina la presencia del propietario indicado.
	Retirar(ctx context.Context, owner string) error
}

// CapacidadReadAll consulta cuántos workers READALL activos conoce el registro
// compartido. Es la capacidad de proceso del sistema, no la identidad del worker
// local: cualquier nodo con heartbeat vigente habilita la admisión.
type CapacidadReadAll interface {
	// WorkersActivos devuelve el número de workers con presencia vigente.
	WorkersActivos(ctx context.Context) (int64, error)
}
