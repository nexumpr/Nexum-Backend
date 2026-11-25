// Package deletion contiene las claves físicas y las constantes del catálogo de purga de
// E014 (`DELETE /api/v1/users/me`). Es dominio puro: no importa el SDK de AWS, ni Redis, ni
// HTTP, y no decide nada de transporte.
//
// El diseño de referencia es `sdd/_control/H08/DISENO_E014_PURGA.md`. Ninguna función de
// este archivo crea tablas, índices ni particiones nuevas: todas las claves viven en la
// tabla única existente y dentro de prefijos ya autorizados por la política IAM vigente
// (`USER#*`, `EMAIL#*`, `REFRESH#*`, `QUEUE#*`).
package deletion

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"

	domainDynamo "github.com/nexumpr/Nexum-Backend/internal/domain/dynamo"
)

const (
	// PrefixDeletion identifica el ítem de control de progreso dentro de la partición del
	// titular: `USER#<id>` / `DELETION#<requestId>`.
	PrefixDeletion = "DELETION#"

	// PartitionQueueDeletion es la partición fija de la cola de vencimiento del worker.
	// La SK es el `requestId` (estable) para que un reintento pueda cambiar `dueAt` con un
	// UpdateItem: en DynamoDB la clave no es mutable.
	PartitionQueueDeletion = "QUEUE#DELETION"

	// PartitionQueueRecuperacion es la cola durable de E005/E006. El worker de E014 sólo
	// borra las entradas cuyo `userId` es el titular: nunca vacía la partición.
	PartitionQueueRecuperacion = "QUEUE#PWRESET"

	// EntityControl y EntityQueue distinguen los ítems propios del flujo de eliminación.
	EntityControl = "DELETION_CONTROL"
	EntityQueue   = "DELETION_QUEUE"

	// PrefixOutboxPointer y PrefixSchedulePointer son los punteros que viven en la
	// partición del titular y referencian un ítem de una partición global del servicio.
	//
	//	USER#<sub> / OUTBOXPTR#<notificationID>    -> skGlobal en OUTBOX#GLOBAL
	//	USER#<sub> / SCHEDULEPTR#<reminderID>      -> skGlobal en SCHEDULE#GLOBAL
	//
	// Existen porque la purga no puede usar `Scan`: sin el puntero, un ítem global
	// (cuya PK es del servicio, no del titular) sería inalcanzable desde la partición
	// del usuario. El puntero NO contiene datos personales: sólo la clave del ítem
	// global, su titular y la marca de creación.
	PrefixOutboxPointer   = "OUTBOXPTR#"
	PrefixSchedulePointer = "SCHEDULEPTR#"
	// PrefixReadAllPointer referencia el trabajo global de E063 (QUEUE#READALL) para
	// que la purga lo encuentre sin Scan. El ítem apunta a una partición de servicio.
	PrefixReadAllPointer = "READALLPTR#"

	// PartitionOutboxGlobal es la partición del outbox push (worker FCM, Oleada 4) y
	// PartitionScheduleGlobal la reservada al scheduler durable de recordatorios. Las
	// dos son particiones del SERVICIO: la purga sólo borra los ítems referenciados
	// por un puntero del titular y cuyo atributo `userID` coincide.
	PartitionOutboxGlobal   = "OUTBOX#GLOBAL"
	PartitionScheduleGlobal = "SCHEDULE#GLOBAL"
	// PartitionQueueReadAll es la cola global del worker durable de E063.
	PartitionQueueReadAll = "QUEUE#READALL"

	// EntityOutboxPointer y EntitySchedulePointer identifican los punteros, de modo que
	// un barrido pueda distinguirlos de otras entidades de la partición.
	EntityOutboxPointer   = "PUSH_OUTBOX_POINTER"
	EntitySchedulePointer = "REMINDER_SCHEDULE_POINTER"

	// CatalogVersion es la versión del catálogo de purga. Permite que las oleadas
	// posteriores añadan entidades sin reescribir el worker: el control persistido guarda
	// la versión con la que se creó el trabajo.
	CatalogVersion = "e014.v1"

	// Estado del perfil mientras la purga está pendiente. No existe como constante en
	// `internal/application/identity` porque es un estado nuevo de este flujo; se declara
	// aquí para no tocar paquetes ya publicados.
	StatusDeletionPending = "DELETION_PENDING"

	// Retención del tombstone mínimo: 90 días con vencimiento lógico (`expiresAt`). La
	// eliminación física la ejecuta el TTL del proveedor de forma asíncrona y no se promete
	// una fecha exacta.
	TombstoneRetention = 90 * 24 * time.Hour
)

// UserControlKeys devuelve PK y SK del control de progreso. El control vive en la
// partición del titular, que es la única que el worker recorre, y nunca se borra durante
// el barrido: `IsControlSK` protege ese prefijo.
func UserControlKeys(userID, requestID string) (pk, sk string) {
	return domainDynamo.PrefixUser + strings.TrimSpace(userID), PrefixDeletion + strings.TrimSpace(requestID)
}

// QueueKeys devuelve PK y SK de la entrada de cola. `SK = requestId` es estable por diseño.
func QueueKeys(requestID string) (pk, sk string) {
	return PartitionQueueDeletion, strings.TrimSpace(requestID)
}

// UserPartition devuelve la PK de la partición del titular.
func UserPartition(userID string) string {
	return domainDynamo.PrefixUser + strings.TrimSpace(userID)
}

// EmailHash normaliza el correo igual que la reserva de unicidad de A-060/E001
// (`strings.TrimSpace` + minúsculas) y devuelve su SHA-256 hexadecimal. El correo en claro
// nunca se persiste en el control ni en la cola.
func EmailHash(email string) string {
	normalizado := strings.TrimSpace(strings.ToLower(email))
	suma := sha256.Sum256([]byte(normalizado))
	return hex.EncodeToString(suma[:])
}

// EmailReservationKeys devuelve PK y SK de la reserva de unicidad a partir del hash ya
// calculado. Reutiliza la forma canónica de A-060: `EMAIL#<sha256>` / `OWNER`.
func EmailReservationKeys(emailHash string) (pk, sk string) {
	return domainDynamo.PrefixEmail + strings.TrimSpace(emailHash), domainDynamo.PrefixOwner
}

// RefreshLocatorKeys devuelve PK y SK del localizador de refresh a partir del digest
// SHA-256 del token opaco. El token en claro no se persiste ni se conoce en este flujo.
func RefreshLocatorKeys(digest string) (pk, sk string) {
	return domainDynamo.RefreshKeys(strings.TrimSpace(digest))
}

// IsControlSK indica si una SK pertenece al control de progreso. El barrido de PARTITION
// la excluye siempre: nunca se puede borrar el registro que acredita el avance.
func IsControlSK(sk string) bool {
	return strings.HasPrefix(strings.TrimSpace(sk), PrefixDeletion)
}

// OutboxPointerKeys devuelve PK y SK del puntero que referencia un trabajo del outbox push.
// El puntero se escribe en la MISMA transacción que el ítem global (contrato de
// `CONTRATO_PUNTEROS.md`); un ítem global sin puntero sólo puede ser anterior a ese contrato.
func OutboxPointerKeys(userID, notificationID string) (pk, sk string) {
	return UserPartition(userID), PrefixOutboxPointer + strings.TrimSpace(notificationID)
}

// SchedulePointerKeys devuelve PK y SK del puntero que referencia la ocurrencia programada
// de un recordatorio. Se reserva para el scheduler durable; mientras no exista, la purga
// tolera su ausencia.
func SchedulePointerKeys(userID, reminderID string) (pk, sk string) {
	return UserPartition(userID), PrefixSchedulePointer + strings.TrimSpace(reminderID)
}

// IsOutboxPointerSK indica si una SK es un puntero al outbox push.
func IsOutboxPointerSK(sk string) bool {
	return strings.HasPrefix(strings.TrimSpace(sk), PrefixOutboxPointer)
}

// IsSchedulePointerSK indica si una SK es un puntero al scheduler de recordatorios.
func IsSchedulePointerSK(sk string) bool {
	return strings.HasPrefix(strings.TrimSpace(sk), PrefixSchedulePointer)
}

// IsReadAllPointerSK indica si una SK referencia un trabajo de QUEUE#READALL.
func IsReadAllPointerSK(sk string) bool {
	return strings.HasPrefix(strings.TrimSpace(sk), PrefixReadAllPointer)
}

// ReadAllPointerKeys devuelve PK y SK del puntero de un trabajo READALL.
func ReadAllPointerKeys(userID, requestID string) (pk, sk string) {
	return UserPartition(userID), PrefixReadAllPointer + strings.TrimSpace(requestID)
}

// IsGlobalPointerSK indica si una SK es un puntero a una partición global del servicio.
// El barrido de PARTITION trata estos ítems en dos pasos: primero borra el ítem global
// referenciado (con condición) y sólo después el puntero.
func IsGlobalPointerSK(sk string) bool {
	return IsOutboxPointerSK(sk) || IsSchedulePointerSK(sk) || IsReadAllPointerSK(sk)
}

// GlobalPartitionForPointer devuelve la partición global a la que apunta un puntero. El
// segundo valor es falso si la SK no es un puntero conocido: la purga nunca borra fuera de
// las dos particiones del servicio declaradas aquí.
func GlobalPartitionForPointer(sk string) (string, bool) {
	switch {
	case IsOutboxPointerSK(sk):
		return PartitionOutboxGlobal, true
	case IsSchedulePointerSK(sk):
		return PartitionScheduleGlobal, true
	case IsReadAllPointerSK(sk):
		return PartitionQueueReadAll, true
	default:
		return "", false
	}
}

// IsGlobalPartition indica si una PK pertenece a una partición global del servicio que la
// purga puede tocar (y sólo mediante un puntero del titular). Es defensa en profundidad
// frente a un puntero corrupto.
func IsGlobalPartition(pk string) bool {
	switch strings.TrimSpace(pk) {
	case PartitionOutboxGlobal, PartitionScheduleGlobal, PartitionQueueReadAll:
		return true
	default:
		return false
	}
}

// IsDeletionQueueSK indica si una SK es un requestId utilizable en la cola.
func IsDeletionQueueSK(sk string) bool {
	return strings.TrimSpace(sk) != ""
}
