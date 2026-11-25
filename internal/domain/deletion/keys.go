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
	// PrefixTokenPointer referencia el token de un solo uso emitido por E005/E006
	// (RESET#<sha256>/RESET) o por E010/E011 (ONE_USE#<sha256>/VERIFY). Sin el puntero,
	// el token —que contiene `userId` y, en E010, el correo— sólo sería alcanzable con
	// un Scan, que este flujo tiene prohibido.
	//
	//	USER#<sub> / TOKENPTR#<sha256(token)> -> skGlobal = RESET#<sha256> | ONE_USE#<sha256>
	PrefixTokenPointer = "TOKENPTR#"
	// PrefixDeliveryPointer referencia el registro de dedupe de entrega de E062
	// (`OUTBOX#GLOBAL/IDEM#<sha256(userID|eventId)>`), que vincula userID, eventId y
	// notificationID y sobrevive 90 días por TTL.
	//
	//	USER#<sub> / DELIVERYPTR#<sha256(eventId)> -> skGlobal = IDEM#<sha256(userID|eventId)>
	PrefixDeliveryPointer = "DELIVERYPTR#"

	// PartitionOutboxGlobal es la partición del outbox push (worker FCM, Oleada 4) y
	// PartitionScheduleGlobal la reservada al scheduler durable de recordatorios. Las
	// dos son particiones del SERVICIO: la purga sólo borra los ítems referenciados
	// por un puntero del titular y cuyo atributo `userID` coincide.
	PartitionOutboxGlobal   = "OUTBOX#GLOBAL"
	PartitionScheduleGlobal = "SCHEDULE#GLOBAL"
	// PartitionQueueReadAll es la cola global del worker durable de E063.
	PartitionQueueReadAll = "QUEUE#READALL"

	// Familias de token. La familia es el prefijo de la PK del ítem global del token y
	// también se persiste en el puntero, de modo que un puntero manipulado no pueda
	// resolver una partición de otra familia.
	TokenFamilyReset  = "RESET"
	TokenFamilyOneUse = "ONE_USE"

	// TokenResetSK y TokenVerifySK son las SK fijas del ítem global por familia
	// (coinciden con los adaptadores de E005/E006 y E010/E011).
	TokenResetSK  = "RESET"
	TokenVerifySK = "VERIFY"

	// EntityOutboxPointer y EntitySchedulePointer identifican los punteros, de modo que
	// un barrido pueda distinguirlos de otras entidades de la partición.
	EntityOutboxPointer   = "PUSH_OUTBOX_POINTER"
	EntitySchedulePointer = "REMINDER_SCHEDULE_POINTER"
	// EntityTokenPointer y EntityDeliveryPointer identifican los punteros de token y de
	// dedupe de entrega.
	EntityTokenPointer    = "TOKEN_POINTER"
	EntityDeliveryPointer = "PUSH_DELIVERY_POINTER"

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

// TokenPointerKeys devuelve PK y SK del puntero que referencia un token de un solo uso.
// El hash es el SHA-256 hexadecimal del token: el valor en claro nunca se persiste.
func TokenPointerKeys(userID, tokenHash string) (pk, sk string) {
	return UserPartition(userID), PrefixTokenPointer + strings.TrimSpace(tokenHash)
}

// IsTokenPointerSK indica si una SK referencia un token global.
func IsTokenPointerSK(sk string) bool {
	return strings.HasPrefix(strings.TrimSpace(sk), PrefixTokenPointer)
}

// DeliveryPointerKeys devuelve PK y SK del puntero que referencia el registro de dedupe
// de entrega de un evento. El SHA-256 se calcula sobre el `eventId`, no sobre datos
// financieros.
func DeliveryPointerKeys(userID, eventID string) (pk, sk string) {
	suma := sha256.Sum256([]byte(strings.TrimSpace(eventID)))
	return UserPartition(userID), PrefixDeliveryPointer + hex.EncodeToString(suma[:])
}

// IsDeliveryPointerSK indica si una SK referencia un dedupe de entrega global.
func IsDeliveryPointerSK(sk string) bool {
	return strings.HasPrefix(strings.TrimSpace(sk), PrefixDeliveryPointer)
}

// TokenPartition devuelve la PK del ítem global de un token (la «partición de la
// familia»): `RESET#<sha256>` o `ONE_USE#<sha256>`.
func TokenPartition(family, tokenHash string) (string, bool) {
	family = strings.TrimSpace(family)
	hash := strings.TrimSpace(tokenHash)
	if !esSHA256Hex(hash) {
		return "", false
	}
	switch family {
	case TokenFamilyReset, TokenFamilyOneUse:
		return family + "#" + hash, true
	default:
		return "", false
	}
}

// TokenFamilyPartition reconoce una partición de token y devuelve su familia. Cualquier
// otra PK no es una partición de token: la purga nunca borra fuera del catálogo.
func TokenFamilyPartition(partition string) (string, bool) {
	pk := strings.TrimSpace(partition)
	for _, family := range []string{TokenFamilyReset, TokenFamilyOneUse} {
		prefijo := family + "#"
		if strings.HasPrefix(pk, prefijo) && esSHA256Hex(strings.TrimPrefix(pk, prefijo)) {
			return family, true
		}
	}
	return "", false
}

// TokenGlobalSK devuelve la SK fija del ítem global de token según la familia.
func TokenGlobalSK(family string) (string, bool) {
	switch strings.TrimSpace(family) {
	case TokenFamilyReset:
		return TokenResetSK, true
	case TokenFamilyOneUse:
		return TokenVerifySK, true
	default:
		return "", false
	}
}

// TokenHashFromPointer extrae el hash del token de la SK de su puntero.
func TokenHashFromPointer(sk string) string {
	if !IsTokenPointerSK(sk) {
		return ""
	}
	return strings.TrimPrefix(strings.TrimSpace(sk), PrefixTokenPointer)
}

// esSHA256Hex valida 64 dígitos hexadecimales en minúscula: la forma en que esta
// implementación persiste un digest. Evita que un puntero manipulado construya una
// partición arbitraria.
func esSHA256Hex(valor string) bool {
	if len(valor) != 64 {
		return false
	}
	for _, r := range valor {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
}

// IsGlobalPointerSK indica si una SK es un puntero a una partición global del servicio.
// El barrido de PARTITION trata estos ítems en dos pasos: primero borra el ítem global
// referenciado (con condición) y sólo después el puntero.
func IsGlobalPointerSK(sk string) bool {
	return IsOutboxPointerSK(sk) || IsSchedulePointerSK(sk) || IsReadAllPointerSK(sk) ||
		IsTokenPointerSK(sk) || IsDeliveryPointerSK(sk)
}

// GlobalPartitionForPointer devuelve la partición global a la que apunta un puntero cuya
// partición es fija (outbox, scheduler, cola READALL y dedupe de entrega). El segundo valor
// es falso si la SK no es un puntero de partición fija: los punteros de token resuelven su
// partición desde el ítem (`skGlobal`) con ResolveGlobalTarget.
func GlobalPartitionForPointer(sk string) (string, bool) {
	switch {
	case IsOutboxPointerSK(sk), IsDeliveryPointerSK(sk):
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
// frente a un puntero corrupto: incluye las particiones dinámicas de token, siempre que su
// forma sea `RESET#<sha256>` o `ONE_USE#<sha256>`.
func IsGlobalPartition(pk string) bool {
	switch strings.TrimSpace(pk) {
	case PartitionOutboxGlobal, PartitionScheduleGlobal, PartitionQueueReadAll:
		return true
	}
	_, ok := TokenFamilyPartition(pk)
	return ok
}

// ResolveGlobalTarget resuelve la clave exacta del ítem global al que apunta un puntero del
// titular. Devuelve ok=false cuando el puntero no declara una referencia utilizable o es
// incoherente (familia desconocida, partición de token con otra familia o hash distinto):
// en ese caso la purga retira el puntero sin tocar nada ajeno.
//
// `globalRef` es el atributo `skGlobal` del puntero: la SK del ítem global en las
// particiones fijas y la PK de la familia en los punteros de token.
func ResolveGlobalTarget(pointerSK, family, globalRef string) (partition, sk string, ok bool) {
	ref := strings.TrimSpace(globalRef)
	if ref == "" {
		return "", "", false
	}
	if IsTokenPointerSK(pointerSK) {
		familia, esToken := TokenFamilyPartition(ref)
		if !esToken || familia != strings.TrimSpace(family) {
			return "", "", false
		}
		// El hash de la partición debe coincidir con el del nombre del puntero: un
		// puntero manipulado no puede resolver la partición de otro token.
		if strings.TrimPrefix(ref, familia+"#") != TokenHashFromPointer(pointerSK) {
			return "", "", false
		}
		sk, ok = TokenGlobalSK(family)
		return ref, sk, ok
	}
	particionFija, okParticion := GlobalPartitionForPointer(pointerSK)
	if !okParticion {
		return "", "", false
	}
	return particionFija, ref, true
}

// IsDeletionQueueSK indica si una SK es un requestId utilizable en la cola.
func IsDeletionQueueSK(sk string) bool {
	return strings.TrimSpace(sk) != ""
}
