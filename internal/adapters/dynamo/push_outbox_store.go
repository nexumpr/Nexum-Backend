package dynamo

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/nexumpr/Nexum-Backend/internal/application/pushoutbox"
	domainDynamo "github.com/nexumpr/Nexum-Backend/internal/domain/dynamo"
)

// Patrón físico autocontenido para el worker FCM (sin GSI2, sin Scan):
//
//	PK = OUTBOX#GLOBAL
//	SK = DUE#<nextAttemptAt UTC, ancho fijo>#<notificationID>
//
// La SK ordena por PRÓXIMO INTENTO, no por instante de creación: un trabajo
// reprogramado al futuro se mueve de posición y deja de ocupar la página de los
// que ya vencieron. El instante se serializa con fracción de 9 dígitos fijos
// (`2006-01-02T15:04:05.000000000Z`), de modo que el orden lexicográfico de la
// SK coincide con el cronológico y la Query puede acotarse con
// `SK BETWEEN "DUE#" AND DUE#<ahora>#~` sin `FilterExpression` sobre el lease
// (el lease se descarta en memoria, de forma acotada por la paginación).
//
// Cambiar la SK exige mover el ítem: DynamoDB no permite modificar la clave con
// `UpdateItem`. `Reprogramar` usa una transacción `Put` (clave nueva) +
// `Delete` (clave vieja, condicionada al `owner` y a `attempts`) para que el
// movimiento sea atómico. Los cierres terminales (`sent`/`revoked`) no mueven la
// clave porque dejan de ser elegibles.
//
// Compatibilidad con ítems históricos: los escritos antes de este cambio tienen
// `SK = DUE#<RFC3339Nano sin relleno>#<id>`. `Pendientes` los sigue leyendo
// porque el relleno de la fracción sólo altera el orden dentro del mismo
// segundo; al reprogramarse migran al formato nuevo. Una SK antigua programada
// exactamente en el mismo segundo que `ahora` se entrega en el ciclo siguiente
// (retraso < intervalo del worker): se retrasa, nunca se pierde.
//
// La idempotencia por eventId+userID vive en la MISMA partición con
// SK IDEM#<sha256>. La bandeja E061 se escribe aparte en
// USER#<sub>/NOTIFICATION#<UTC>#<id>.
//
// Los reprocesos no reescriben la clave: `nextAttemptAt` es el atributo
// autoritativo y se filtra en la consulta. La idempotencia por
// eventId+userID vive en la MISMA partición con SK IDEM#<sha256>.
// La bandeja E061 se escribe aparte en USER#<sub>/NOTIFICATION#<UTC>#<id>.
//
// Puntero de purga (contrato en `dist/transversal/CONTRATO_PUNTEROS.md`):
//
//	PK = USER#<sub>
//	SK = OUTBOXPTR#<notificationID>
//	skGlobal = <SK del ítem en OUTBOX#GLOBAL>
//
// Se escribe en la MISMA transacción que el ítem global y se retira cuando el
// trabajo termina. Así E014 purga el outbox sin `Scan` y sin tocar datos de
// otras cuentas.
const (
	pushOutboxPartition        = "OUTBOX#GLOBAL"
	pushOutboxDuePrefix        = "DUE#"
	pushOutboxIdemPrefix       = "IDEM#"
	pushOutboxEntityType       = "PUSH_OUTBOX"
	pushOutboxIdemEntityType   = "PUSH_OUTBOX_DELIVERY"
	pushNotificationEntityType = "USER_NOTIFICATION"

	// pushOutboxLayoutInstante fija 9 decimales: ancho constante y orden
	// lexicográfico igual al cronológico.
	pushOutboxLayoutInstante = "2006-01-02T15:04:05.000000000"
	// pushOutboxSufijoCola es mayor que cualquier carácter usado en un
	// notificationID (hex y guiones), de modo que la cota superior alcanza a
	// todos los identificadores del mismo instante.
	pushOutboxSufijoCola = "~"
	// pushOutboxPaginasMax acota el barrido por ciclo. Sin este tope, un
	// backlog patológico de ítems no elegibles retendría al worker.
	pushOutboxPaginasMax = 5

	// Puntero de purga dentro de la partición del titular.
	pushOutboxPointerPrefix = "OUTBOXPTR#"
	pushOutboxPointerEntity = "PUSH_OUTBOX_POINTER"

	// estadoPerfilActivo es el único estado del perfil que autoriza encolar y enviar
	// (E002/E003). `DELETION_PENDING` o la ausencia del perfil bloquean ambas acciones:
	// es la garantía de que ni el productor ni el worker recrean datos tras el tombstone.
	estadoPerfilActivo = "ACTIVE"
)

var (
	// ErrOutboxNoDisponible indica configuración o persistencia ausente. Nunca
	// se degrada a almacenamiento en memoria.
	ErrOutboxNoDisponible = errors.New("outbox push no disponible")
	// ErrConflictoLease significa que otro nodo ganó la reclamación.
	ErrConflictoLease = errors.New("conflicto de lease del outbox push")
	// ErrCuentaNoActiva indica que el titular no está ACTIVE (eliminado o en purga).
	// El trabajo no se encola: se falla cerrado en lugar de crear datos de una cuenta
	// dada de baja. Envuelve pushoutbox.ErrCuentaNoActiva para que la señal cruce el
	// puerto de aplicación sin que ésta importe este paquete.
	ErrCuentaNoActiva = fmt.Errorf("la cuenta no esta activa: %w", pushoutbox.ErrCuentaNoActiva)
)

// PushOutboxAPI es el subconjunto de DynamoDB que usa el outbox. No incluye
// Scan: el compilador impide un barrido global. `TransactWriteItems` es
// necesario para mover la clave al reprogramar sin dejar duplicados ni huecos.
type PushOutboxAPI interface {
	Query(ctx context.Context, params *dynamodb.QueryInput, optFns ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error)
	GetItem(ctx context.Context, params *dynamodb.GetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error)
	PutItem(ctx context.Context, params *dynamodb.PutItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error)
	UpdateItem(ctx context.Context, params *dynamodb.UpdateItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error)
	DeleteItem(ctx context.Context, params *dynamodb.DeleteItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.DeleteItemOutput, error)
	TransactWriteItems(ctx context.Context, params *dynamodb.TransactWriteItemsInput, optFns ...func(*dynamodb.Options)) (*dynamodb.TransactWriteItemsOutput, error)
}

// PushOutboxStore implementa pushoutbox.Store y el sink de la bandeja E061
// sobre la tabla única existente. Cada instancia tiene un owner distinto: las
// escrituras finales exigenPoseer el lease, de modo que dos nodos no pueden
// cerrar el mismo trabajo.
type PushOutboxStore struct {
	client PushOutboxAPI
	table  string
	owner  string
	now    func() time.Time
}

// NewPushOutboxStore construye el almacén. Sin cliente o tabla, todas las
// operaciones fallan cerradas.
func NewPushOutboxStore(client PushOutboxAPI, table string) *PushOutboxStore {
	randomBytes := make([]byte, 12)
	if _, err := rand.Read(randomBytes); err != nil {
		randomBytes = []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11}
	}
	return &PushOutboxStore{
		client: client,
		table:  strings.TrimSpace(table),
		owner:  "fcm-" + hex.EncodeToString(randomBytes),
		now:    func() time.Time { return time.Now().UTC() },
	}
}

// WithOwner fija el identificador de instancia (determinista en pruebas).
func (s *PushOutboxStore) WithOwner(owner string) *PushOutboxStore {
	if s != nil && strings.TrimSpace(owner) != "" {
		s.owner = strings.TrimSpace(owner)
	}
	return s
}

// WithClock inyecta un reloj determinista en pruebas.
func (s *PushOutboxStore) WithClock(now func() time.Time) *PushOutboxStore {
	if s != nil && now != nil {
		s.now = now
	}
	return s
}

// Owner expone el identificador usado en las condiciones de lease.
func (s *PushOutboxStore) Owner() string {
	if s == nil {
		return ""
	}
	return s.owner
}

func pushOutboxInstante(t time.Time) string {
	return t.UTC().Format(pushOutboxLayoutInstante) + "Z"
}

func pushOutboxSK(due time.Time, notificationID string) string {
	return pushOutboxDuePrefix + pushOutboxInstante(due) + "#" + notificationID
}

// pushOutboxSKLimite es la cota superior inclusiva de la Query de vencimientos:
// cualquier SK con instante <= ahora queda por debajo.
func pushOutboxSKLimite(ahora time.Time) string {
	return pushOutboxSK(ahora, pushOutboxSufijoCola)
}

// pushOutboxNotificationID recupera el sufijo de una SK DUE#<instante>#<id>. Es
// la vía de compatibilidad para ítems que no guardan el atributo notificationID.
func pushOutboxNotificationID(sk string) string {
	resto, ok := strings.CutPrefix(sk, pushOutboxDuePrefix)
	if !ok {
		return ""
	}
	partes := strings.SplitN(resto, "#", 2)
	if len(partes) != 2 {
		return ""
	}
	return partes[1]
}

func pushOutboxIdemSK(userID, eventID string) string {
	suma := sha256.Sum256([]byte(strings.TrimSpace(userID) + "|" + strings.TrimSpace(eventID)))
	return pushOutboxIdemPrefix + hex.EncodeToString(suma[:])
}

// pushOutboxPointerKeys devuelve la clave del puntero de purga del titular.
func pushOutboxPointerKeys(userID, notificationID string) (string, string) {
	return domainDynamo.PrefixUser + strings.TrimSpace(userID), pushOutboxPointerPrefix + strings.TrimSpace(notificationID)
}

func pushOutboxAhora(s *PushOutboxStore) time.Time {
	if s != nil && s.now != nil {
		return s.now().UTC()
	}
	return time.Now().UTC()
}

func (s *PushOutboxStore) disponible() bool {
	return s != nil && s.client != nil && s.table != ""
}

// Crear inserta el trabajo de forma condicional y, en la MISMA transacción, su puntero de
// purga y la comprobación de que el titular sigue ACTIVE. Devuelve false si ya existía la
// misma clave (replay legítimo), sin sobrescribir su estado.
//
// El `ConditionCheck` sobre `USER#<sub>/PROFILE` es la garantía de que ni un evento tardío
// ni un reintento del productor recrean datos de una cuenta eliminada: tras el tombstone el
// perfil deja de estar ACTIVE y la transacción se cancela con ErrCuentaNoActiva.
func (s *PushOutboxStore) Crear(ctx context.Context, registro pushoutbox.Registro) (bool, error) {
	if !s.disponible() {
		return false, ErrOutboxNoDisponible
	}
	ahora := pushOutboxAhora(s)
	due := registro.NextAttemptAt
	if due.IsZero() {
		due = ahora
	}
	if registro.CreatedAt.IsZero() {
		registro.CreatedAt = ahora
	}
	registro.UpdatedAt = ahora
	skGlobal := pushOutboxSK(due, registro.NotificationID)
	item := map[string]types.AttributeValue{
		"PK":             stringAttr(pushOutboxPartition),
		"SK":             stringAttr(skGlobal),
		"entityType":     stringAttr(pushOutboxEntityType),
		"userID":         stringAttr(registro.UserID),
		"notificationID": stringAttr(registro.NotificationID),
		"title":          stringAttr(registro.Title),
		"message":        stringAttr(registro.Message),
		"type":           stringAttr(registro.Type),
		"eventId":        stringAttr(registro.EventID),
		"status":         stringAttr(string(pushoutbox.EstadoPendiente)),
		"attempts":       numberAttr(0),
		"leaseUntil":     stringAttr(""),
		"nextAttemptAt":  stringAttr(pushOutboxInstante(due)),
		"createdAt":      stringAttr(pushOutboxInstante(registro.CreatedAt)),
		"updatedAt":      stringAttr(pushOutboxInstante(ahora)),
	}

	pointerPK, pointerSK := pushOutboxPointerKeys(registro.UserID, registro.NotificationID)
	pointer := map[string]types.AttributeValue{
		"PK":             stringAttr(pointerPK),
		"SK":             stringAttr(pointerSK),
		"entityType":     stringAttr(pushOutboxPointerEntity),
		"userID":         stringAttr(strings.TrimSpace(registro.UserID)),
		"notificationID": stringAttr(registro.NotificationID),
		"skGlobal":       stringAttr(skGlobal),
		"createdAt":      stringAttr(registro.CreatedAt.UTC().Format(time.RFC3339Nano)),
	}
	perfilPK, perfilSK := domainDynamo.UserKeys(strings.TrimSpace(registro.UserID))

	_, err := s.client.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{
		TransactItems: []types.TransactWriteItem{
			{Put: &types.Put{
				TableName:           aws.String(s.table),
				Item:                item,
				ConditionExpression: aws.String("attribute_not_exists(PK)"),
			}},
			{Put: &types.Put{
				TableName:           aws.String(s.table),
				Item:                pointer,
				ConditionExpression: aws.String("attribute_not_exists(PK)"),
			}},
			{ConditionCheck: &types.ConditionCheck{
				TableName:                 aws.String(s.table),
				Key:                       map[string]types.AttributeValue{"PK": stringAttr(perfilPK), "SK": stringAttr(perfilSK)},
				ConditionExpression:       aws.String("#estado = :activo"),
				ExpressionAttributeNames:  map[string]string{"#estado": "status"},
				ExpressionAttributeValues: map[string]types.AttributeValue{":activo": stringAttr(estadoPerfilActivo)},
			}},
		},
	})
	if err == nil {
		return true, nil
	}
	var cancelada *types.TransactionCanceledException
	if errors.As(err, &cancelada) {
		// El perfil no ACTIVE cancela la transacción completa: no se crea ni el ítem
		// global ni el puntero.
		if pushFailureCode(cancelada.CancellationReasons, 2) == "ConditionalCheckFailed" {
			return false, ErrCuentaNoActiva
		}
		// Un replay de la misma clave (ítem global o puntero ya presentes) es inocuo.
		return false, nil
	}
	return false, ErrOutboxNoDisponible
}

// Pendientes (equivalente al `Due` del worker) devuelve los trabajos cuyo
// próximo intento ya venció, en orden de SK (es decir, de vencimiento).
//
// La Query se acota por clave, sin `FilterExpression`: `SK BETWEEN "DUE#" AND
// DUE#<ahora>#~`. Los trabajos reprogramados al futuro quedan fuera del rango y
// no consumen la página. El lease se descarta en memoria y la lectura pagina con
// `ExclusiveStartKey` hasta `pushOutboxPaginasMax` páginas, de modo que una
// página corta (límite de 1 MB de DynamoDB o ítems no elegibles) no oculta
// pendientes y no se pierde ningún vencido.
func (s *PushOutboxStore) Pendientes(ctx context.Context, ahora time.Time, limite int) ([]pushoutbox.Registro, error) {
	if !s.disponible() {
		return nil, ErrOutboxNoDisponible
	}
	if limite < 1 {
		limite = 25
	}
	ahora = ahora.UTC()
	entrada := &dynamodb.QueryInput{
		TableName:              aws.String(s.table),
		ConsistentRead:         aws.Bool(true),
		Limit:                  aws.Int32(int32(limite)),
		KeyConditionExpression: aws.String("#pk = :pk AND #sk BETWEEN :desde AND :hasta"),
		ExpressionAttributeNames: map[string]string{
			"#pk": "PK",
			"#sk": "SK",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk":    stringAttr(pushOutboxPartition),
			":desde": stringAttr(pushOutboxDuePrefix),
			":hasta": stringAttr(pushOutboxSKLimite(ahora)),
		},
	}
	registros := make([]pushoutbox.Registro, 0, limite)
	for pagina := 0; pagina < pushOutboxPaginasMax; pagina++ {
		out, err := s.client.Query(ctx, entrada)
		if err != nil {
			return nil, ErrOutboxNoDisponible
		}
		for _, item := range out.Items {
			if stringAttribute(item["entityType"]) != pushOutboxEntityType {
				continue
			}
			registro := pushOutboxDesdeItem(item)
			if !pushOutboxElegible(registro, ahora) {
				continue
			}
			registros = append(registros, registro)
			if len(registros) >= limite {
				return registros, nil
			}
		}
		if len(out.LastEvaluatedKey) == 0 {
			break
		}
		entrada.ExclusiveStartKey = out.LastEvaluatedKey
	}
	return registros, nil
}

// pushOutboxElegible aplica en memoria las condiciones que antes vivían en el
// FilterExpression: vencimiento, estado reclamable y lease no vigente.
func pushOutboxElegible(registro pushoutbox.Registro, ahora time.Time) bool {
	if !registro.NextAttemptAt.IsZero() && registro.NextAttemptAt.After(ahora) {
		return false
	}
	switch registro.Status {
	case pushoutbox.EstadoPendiente, pushoutbox.EstadoFallido:
		return true
	case pushoutbox.EstadoEnviando:
		// Un trabajo quedó en vuelo: sólo se recupera si su lease venció.
		return registro.LeaseUntil.IsZero() || !registro.LeaseUntil.After(ahora)
	default:
		return false
	}
}

// Reclamar toma el lease bajo condición: sólo si está pendiente/fallido, ya
// venció y nadie lo tiene arrendado. Devuelve el número de intento resultante.
//
// Antes de reclamar se comprueba el estado del titular (`USER#<sub>/PROFILE`). Si la cuenta
// no está ACTIVE (eliminada o en purga) NO se envía nada: el trabajo y su puntero se retiran
// del outbox para no conservar contenido de una cuenta dada de baja. Ese es el punto exacto
// que impide al worker recrear o servir datos después del tombstone.
func (s *PushOutboxStore) Reclamar(ctx context.Context, id string, ahora time.Time, lease time.Duration) (int, bool, error) {
	if !s.disponible() {
		return 0, false, ErrOutboxNoDisponible
	}
	ahora = ahora.UTC()
	item, errLectura := s.leerGlobal(ctx, id)
	if errLectura != nil {
		return 0, false, ErrOutboxNoDisponible
	}
	if len(item) == 0 {
		return 0, false, nil
	}
	userID := strings.TrimSpace(stringAttribute(item["userID"]))
	notificationID := strings.TrimSpace(stringAttribute(item["notificationID"]))
	activo, errPerfil := s.perfilActivo(ctx, userID)
	if errPerfil != nil {
		return 0, false, ErrOutboxNoDisponible
	}
	if !activo {
		// Retención por baja de cuenta: se cancela el trabajo antes de cualquier envío.
		if userID == "" {
			_, _ = s.borrarGlobal(ctx, id, ahora)
			return 0, false, nil
		}
		if err := s.descartarTrabajo(ctx, id, userID, notificationID, ahora); err != nil {
			return 0, false, ErrOutboxNoDisponible
		}
		return 0, false, nil
	}

	out, err := s.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(s.table),
		Key: map[string]types.AttributeValue{
			"PK": stringAttr(pushOutboxPartition),
			"SK": stringAttr(id),
		},
		UpdateExpression: aws.String("SET #estado = :enviando, #leaseUntil = :lease, #owner = :owner, #intentos = #intentos + :uno, #updatedAt = :ahora"),
		ConditionExpression: aws.String("((#estado = :pendiente OR #estado = :fallido) OR " +
			"(#estado = :enviando AND (attribute_not_exists(#leaseUntil) OR #leaseUntil < :ahora))) AND " +
			"(attribute_not_exists(#siguiente) OR #siguiente <= :ahora)"),
		ExpressionAttributeNames: map[string]string{
			"#estado":     "status",
			"#leaseUntil": "leaseUntil",
			"#owner":      "owner",
			"#intentos":   "attempts",
			"#updatedAt":  "updatedAt",
			"#siguiente":  "nextAttemptAt",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":enviando":  stringAttr(string(pushoutbox.EstadoEnviando)),
			":pendiente": stringAttr(string(pushoutbox.EstadoPendiente)),
			":fallido":   stringAttr(string(pushoutbox.EstadoFallido)),
			":lease":     stringAttr(pushOutboxInstante(ahora.Add(lease))),
			":owner":     stringAttr(s.owner),
			":uno":       numberAttr(1),
			":ahora":     stringAttr(pushOutboxInstante(ahora)),
		},
		ReturnValues: types.ReturnValueAllNew,
	})
	if err != nil {
		var condicional *types.ConditionalCheckFailedException
		if errors.As(err, &condicional) {
			return 0, false, nil
		}
		return 0, false, ErrOutboxNoDisponible
	}
	intentos := int64(1)
	if out != nil {
		if valor, errParse := strconv.ParseInt(stringAttribute(out.Attributes["attempts"]), 10, 64); errParse == nil {
			intentos = valor
		}
	}
	return int(intentos), true, nil
}

// leerGlobal lee el ítem del outbox con consistencia fuerte.
func (s *PushOutboxStore) leerGlobal(ctx context.Context, id string) (map[string]types.AttributeValue, error) {
	out, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(s.table),
		Key: map[string]types.AttributeValue{
			"PK": stringAttr(pushOutboxPartition),
			"SK": stringAttr(id),
		},
		ConsistentRead: aws.Bool(true),
	})
	if err != nil {
		return nil, err
	}
	if out == nil {
		return nil, nil
	}
	return out.Item, nil
}

// perfilActivo comprueba con consistencia fuerte que el titular sigue ACTIVE. Un perfil
// ausente cuenta como inactivo: tras la purga no existe y no se puede autorizar un envío.
func (s *PushOutboxStore) perfilActivo(ctx context.Context, userID string) (bool, error) {
	if strings.TrimSpace(userID) == "" {
		return false, nil
	}
	pk, sk := domainDynamo.UserKeys(strings.TrimSpace(userID))
	out, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(s.table),
		Key: map[string]types.AttributeValue{
			"PK": stringAttr(pk),
			"SK": stringAttr(sk),
		},
		ConsistentRead: aws.Bool(true),
	})
	if err != nil {
		return false, err
	}
	if out == nil || len(out.Item) == 0 {
		return false, nil
	}
	return stringAttribute(out.Item["status"]) == estadoPerfilActivo, nil
}

// descartarTrabajo retira un trabajo de una cuenta no activa: primero el ítem global (con
// condición de ausencia de lease) y sólo después el puntero. Si el global no se pudo borrar
// porque otro nodo lo reclamó, el puntero se conserva para que E014 lo resuelva al vencer
// el lease.
func (s *PushOutboxStore) descartarTrabajo(ctx context.Context, id, userID, notificationID string, ahora time.Time) error {
	borrado, err := s.borrarGlobal(ctx, id, ahora)
	if err != nil {
		return err
	}
	if !borrado {
		return nil
	}
	return s.borrarPuntero(ctx, userID, notificationID)
}

// borrarGlobal borra el ítem del outbox si no tiene un envío en curso.
func (s *PushOutboxStore) borrarGlobal(ctx context.Context, id string, ahora time.Time) (bool, error) {
	_, err := s.client.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName: aws.String(s.table),
		Key: map[string]types.AttributeValue{
			"PK": stringAttr(pushOutboxPartition),
			"SK": stringAttr(id),
		},
		ConditionExpression: aws.String("attribute_exists(PK) AND (attribute_not_exists(#owner) OR #lease < :ahora)"),
		ExpressionAttributeNames: map[string]string{
			"#owner": "owner",
			"#lease": "leaseUntil",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":ahora": stringAttr(ahora.UTC().Format(time.RFC3339Nano)),
		},
	})
	if err == nil {
		return true, nil
	}
	var condicional *types.ConditionalCheckFailedException
	if errors.As(err, &condicional) {
		return false, nil
	}
	return false, ErrOutboxNoDisponible
}

// borrarPuntero retira el puntero de purga del titular. Es idempotente: si no existe, la
// operación no tiene efecto.
func (s *PushOutboxStore) borrarPuntero(ctx context.Context, userID, notificationID string) error {
	if strings.TrimSpace(userID) == "" || strings.TrimSpace(notificationID) == "" {
		return nil
	}
	pk, sk := pushOutboxPointerKeys(userID, notificationID)
	_, err := s.client.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName: aws.String(s.table),
		Key: map[string]types.AttributeValue{
			"PK": stringAttr(pk),
			"SK": stringAttr(sk),
		},
	})
	if err != nil {
		return ErrOutboxNoDisponible
	}
	return nil
}

// MarcarEnviado cierra el trabajo sólo si este nodo sigue siendo el dueño del lease.
func (s *PushOutboxStore) MarcarEnviado(ctx context.Context, id string, ahora time.Time) error {
	return s.actualizarCierre(ctx, id, pushoutbox.EstadoEnviado, "", nil, ahora)
}

// MarcarRevocado guarda un fallo permanente (UNREGISTERED/opt-out) sin reintentos.
func (s *PushOutboxStore) MarcarRevocado(ctx context.Context, id string, clase string, ahora time.Time) error {
	return s.actualizarCierre(ctx, id, pushoutbox.EstadoRevocado, clase, nil, ahora)
}

// Reprogramar guarda un fallo recuperable con backoff y libera el lease.
//
// Como la SK ordena por `nextAttemptAt`, reprogramar mueve el ítem: se recrea en
// `DUE#<siguiente>#<id>` y se borra la clave anterior en una sola transacción,
// condicionada al `owner` del lease y al número de intentos leído. Si el nuevo
// instante cae en el mismo segundo que la clave vigente, la clave no cambia y se
// actualiza en sitio. Un ítem histórico con SK sin relleno migra al formato nuevo
// en su primera reprogramación.
func (s *PushOutboxStore) Reprogramar(ctx context.Context, id string, attempts int, clase string, siguiente time.Time, ahora time.Time) error {
	if !s.disponible() {
		return ErrOutboxNoDisponible
	}
	ahora = ahora.UTC()
	siguiente = siguiente.UTC()
	if siguiente.IsZero() {
		return ErrOutboxNoDisponible
	}
	item, err := s.leerItem(ctx, id)
	if err != nil {
		return err
	}
	idNotificacion := pushOutboxNotificationID(id)
	if item == nil {
		// Replay de una reprogramación ya aplicada: si el destino existe, el
		// trabajo se movió y esta llamada no debe reintentar nada.
		if destino, errDestino := s.leerItem(ctx, pushOutboxSK(siguiente, idNotificacion)); errDestino == nil && destino != nil {
			return nil
		}
		return ErrConflictoLease
	}
	if propietario := stringAttribute(item["notificationID"]); propietario != "" {
		idNotificacion = propietario
	}
	nuevaSK := pushOutboxSK(siguiente, idNotificacion)
	if nuevaSK == id {
		return s.actualizarCierre(ctx, id, pushoutbox.EstadoFallido, clase, &struct {
			intentos  int
			siguiente time.Time
		}{intentos: attempts, siguiente: siguiente}, ahora)
	}
	nuevo := make(map[string]types.AttributeValue, len(item)+4)
	for clave, valor := range item {
		nuevo[clave] = valor
	}
	nuevo["SK"] = stringAttr(nuevaSK)
	nuevo["status"] = stringAttr(string(pushoutbox.EstadoFallido))
	nuevo["attempts"] = numberAttr(int64(attempts))
	nuevo["nextAttemptAt"] = stringAttr(pushOutboxInstante(siguiente))
	nuevo["updatedAt"] = stringAttr(pushOutboxInstante(ahora))
	if clase != "" {
		nuevo["lastErrorClass"] = stringAttr(clase)
	}
	delete(nuevo, "leaseUntil")
	delete(nuevo, "owner")

	acciones := []types.TransactWriteItem{
		{Put: &types.Put{
			TableName:           aws.String(s.table),
			Item:                nuevo,
			ConditionExpression: aws.String("attribute_not_exists(PK)"),
		}},
		{Delete: &types.Delete{
			TableName:           aws.String(s.table),
			Key:                 map[string]types.AttributeValue{"PK": stringAttr(pushOutboxPartition), "SK": stringAttr(id)},
			ConditionExpression: aws.String("#owner = :owner AND #intentos = :intentos"),
			ExpressionAttributeNames: map[string]string{
				"#owner":    "owner",
				"#intentos": "attempts",
			},
			ExpressionAttributeValues: map[string]types.AttributeValue{
				":owner":    stringAttr(s.owner),
				":intentos": numberAttr(pushOutboxIntentos(item)),
			},
		}},
	}

	// El puntero de purga guarda la SK global: si el trabajo se mueve, el puntero
	// debe moverse con él en la MISMA transacción; de lo contrario E014 buscaría
	// una SK ya borrada y dejaría el global huérfano. Los ítems históricos sin
	// puntero (creados antes del contrato) no lo crean aquí: se migran al
	// reprogramarse sólo si ya existía.
	userIDItem := strings.TrimSpace(stringAttribute(item["userID"]))
	if userIDItem != "" && idNotificacion != "" {
		punteroPK, punteroSK := pushOutboxPointerKeys(userIDItem, idNotificacion)
		puntero, errPuntero := s.leerItemPorClave(ctx, punteroPK, punteroSK)
		if errPuntero != nil {
			return errPuntero
		}
		if puntero != nil {
			acciones = append(acciones, types.TransactWriteItem{Update: &types.Update{
				TableName:           aws.String(s.table),
				Key:                 map[string]types.AttributeValue{"PK": stringAttr(punteroPK), "SK": stringAttr(punteroSK)},
				UpdateExpression:    aws.String("SET #skGlobal = :nueva, #dueAt = :siguiente"),
				ConditionExpression: aws.String("#skGlobal = :vieja"),
				ExpressionAttributeNames: map[string]string{
					"#skGlobal": "skGlobal", "#dueAt": "dueAt",
				},
				ExpressionAttributeValues: map[string]types.AttributeValue{
					":nueva":     stringAttr(nuevaSK),
					":vieja":     stringAttr(id),
					":siguiente": stringAttr(pushOutboxInstante(siguiente)),
				},
			}})
		}
	}

	_, err = s.client.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{
		TransactItems: acciones,
	})
	if err == nil {
		return nil
	}
	var cancelada *types.TransactionCanceledException
	if errors.As(err, &cancelada) {
		if destino, errDestino := s.leerItem(ctx, nuevaSK); errDestino == nil && destino != nil {
			return nil
		}
		return ErrConflictoLease
	}
	return ErrOutboxNoDisponible
}

// leerItemPorClave devuelve un ítem por PK/SK con lectura consistente.
func (s *PushOutboxStore) leerItemPorClave(ctx context.Context, pk, sk string) (map[string]types.AttributeValue, error) {
	out, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName:      aws.String(s.table),
		ConsistentRead: aws.Bool(true),
		Key:            map[string]types.AttributeValue{"PK": stringAttr(pk), "SK": stringAttr(sk)},
	})
	if err != nil {
		return nil, ErrOutboxNoDisponible
	}
	if out == nil || len(out.Item) == 0 {
		return nil, nil
	}
	return out.Item, nil
}

// leerItem devuelve el ítem completo con lectura consistente. Un ítem ausente se
// representa como (nil, nil): no se degrada a un error de dependencia.
func (s *PushOutboxStore) leerItem(ctx context.Context, sk string) (map[string]types.AttributeValue, error) {
	out, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName:      aws.String(s.table),
		ConsistentRead: aws.Bool(true),
		Key: map[string]types.AttributeValue{
			"PK": stringAttr(pushOutboxPartition),
			"SK": stringAttr(sk),
		},
	})
	if err != nil {
		return nil, ErrOutboxNoDisponible
	}
	if out == nil || len(out.Item) == 0 {
		return nil, nil
	}
	return out.Item, nil
}

func pushOutboxIntentos(item map[string]types.AttributeValue) int64 {
	intentos, err := strconv.ParseInt(stringAttribute(item["attempts"]), 10, 64)
	if err != nil || intentos < 0 {
		return 0
	}
	return intentos
}

func (s *PushOutboxStore) actualizarCierre(ctx context.Context, id string, estado pushoutbox.Estado, clase string, reprogramacion *struct {
	intentos  int
	siguiente time.Time
}, ahora time.Time) error {
	if !s.disponible() {
		return ErrOutboxNoDisponible
	}
	ahora = ahora.UTC()
	expresion := "SET #estado = :estado, #updatedAt = :ahora REMOVE #leaseUntil, #owner"
	nombres := map[string]string{
		"#estado":     "status",
		"#updatedAt":  "updatedAt",
		"#leaseUntil": "leaseUntil",
		"#owner":      "owner",
	}
	valores := map[string]types.AttributeValue{
		":estado": stringAttr(string(estado)),
		":ahora":  stringAttr(pushOutboxInstante(ahora)),
		":owner":  stringAttr(s.owner),
	}
	if clase != "" {
		expresion = "SET #estado = :estado, #clase = :clase, #updatedAt = :ahora REMOVE #leaseUntil, #owner"
		nombres["#clase"] = "lastErrorClass"
		valores[":clase"] = stringAttr(clase)
	}
	if reprogramacion != nil {
		expresion = "SET #estado = :estado, #intentos = :intentos, #siguiente = :siguiente, #clase = :clase, #updatedAt = :ahora REMOVE #leaseUntil, #owner"
		nombres["#intentos"] = "attempts"
		nombres["#siguiente"] = "nextAttemptAt"
		valores[":intentos"] = numberAttr(int64(reprogramacion.intentos))
		valores[":siguiente"] = stringAttr(pushOutboxInstante(reprogramacion.siguiente))
	}
	salida, err := s.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(s.table),
		Key: map[string]types.AttributeValue{
			"PK": stringAttr(pushOutboxPartition),
			"SK": stringAttr(id),
		},
		UpdateExpression:          aws.String(expresion),
		ConditionExpression:       aws.String("#owner = :owner"),
		ExpressionAttributeNames:  nombres,
		ExpressionAttributeValues: valores,
		ReturnValues:              types.ReturnValueAllOld,
	})
	if err != nil {
		var condicional *types.ConditionalCheckFailedException
		if errors.As(err, &condicional) {
			return ErrConflictoLease
		}
		return ErrOutboxNoDisponible
	}
	if estado == pushoutbox.EstadoEnviado || estado == pushoutbox.EstadoRevocado {
		// Trabajo terminado: el ítem global ya no debe conservar título ni mensaje y su
		// puntero dejaría de tener sentido. Se retiran ambos (idempotente). El registro de
		// idempotencia `IDEM#` (sin contenido personal) permanece.
		_, _ = s.borrarGlobal(ctx, id, ahora)
		if salida != nil {
			userID := strings.TrimSpace(stringAttribute(salida.Attributes["userID"]))
			notificationID := strings.TrimSpace(stringAttribute(salida.Attributes["notificationID"]))
			_ = s.borrarPuntero(ctx, userID, notificationID)
		}
	}
	return nil
}

// YaEntregado consulta el registro de idempotencia por eventId+userID.
func (s *PushOutboxStore) YaEntregado(ctx context.Context, userID, eventID string) (bool, error) {
	if !s.disponible() {
		return false, ErrOutboxNoDisponible
	}
	out, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(s.table),
		Key: map[string]types.AttributeValue{
			"PK": stringAttr(pushOutboxPartition),
			"SK": stringAttr(pushOutboxIdemSK(userID, eventID)),
		},
	})
	if err != nil {
		return false, ErrOutboxNoDisponible
	}
	return out != nil && len(out.Item) > 0, nil
}

// MarcarEntregado graba el hecho de entrega con escritura condicional. Un replay
// no altera el registro existente.
func (s *PushOutboxStore) MarcarEntregado(ctx context.Context, userID, eventID, notificationID string, ahora time.Time) error {
	if !s.disponible() {
		return ErrOutboxNoDisponible
	}
	if ahora.IsZero() {
		ahora = pushOutboxAhora(s)
	}
	_, err := s.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(s.table),
		Item: map[string]types.AttributeValue{
			"PK":             stringAttr(pushOutboxPartition),
			"SK":             stringAttr(pushOutboxIdemSK(userID, eventID)),
			"entityType":     stringAttr(pushOutboxIdemEntityType),
			"userID":         stringAttr(userID),
			"eventId":        stringAttr(eventID),
			"notificationID": stringAttr(notificationID),
			"createdAt":      stringAttr(ahora.UTC().Format(time.RFC3339Nano)),
			// El dedupe de entrega contiene identificadores vinculables
			// (userID/eventId/notificationID). TTL de 90 días: evita el residuo
			// permanente; DynamoDB lo purga de forma best-effort, no exacta.
			"expiresAt": numberAttr(ahora.UTC().Add(90 * 24 * time.Hour).Unix()),
		},
		ConditionExpression: aws.String("attribute_not_exists(PK)"),
	})
	if err == nil {
		return nil
	}
	var condicional *types.ConditionalCheckFailedException
	if errors.As(err, &condicional) {
		return nil
	}
	return ErrOutboxNoDisponible
}

// GuardarBandeja materializa la notificación E061 en la partición del titular.
// La escritura es condicional: si el worker se ejecuta dos veces no duplica la
// entrada de la bandeja.
func (s *PushOutboxStore) GuardarBandeja(ctx context.Context, bandeja pushoutbox.RegistroBandeja) error {
	if !s.disponible() {
		return ErrOutboxNoDisponible
	}
	creado := bandeja.CreatedAt
	if creado.IsZero() {
		creado = pushOutboxAhora(s)
	}
	_, err := s.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(s.table),
		Item: map[string]types.AttributeValue{
			"PK":         stringAttr(domainDynamo.PrefixUser + strings.TrimSpace(bandeja.UserID)),
			"SK":         stringAttr("NOTIFICATION#" + creado.UTC().Format(time.RFC3339Nano) + "#" + bandeja.ID),
			"entityType": stringAttr(pushNotificationEntityType),
			"id":         stringAttr(bandeja.ID),
			"title":      stringAttr(bandeja.Title),
			"message":    stringAttr(bandeja.Message),
			"type":       stringAttr(bandeja.Type),
			"isRead":     boolAttr(false),
			"createdAt":  stringAttr(creado.UTC().Format(time.RFC3339Nano)),
		},
		ConditionExpression: aws.String("attribute_not_exists(PK)"),
	})
	if err == nil {
		return nil
	}
	var condicional *types.ConditionalCheckFailedException
	if errors.As(err, &condicional) {
		return nil
	}
	return ErrOutboxNoDisponible
}

func pushOutboxDesdeItem(item map[string]types.AttributeValue) pushoutbox.Registro {
	parsear := func(valor types.AttributeValue) time.Time {
		texto := stringAttribute(valor)
		if texto == "" {
			return time.Time{}
		}
		parsed, err := time.Parse(time.RFC3339Nano, texto)
		if err != nil {
			return time.Time{}
		}
		return parsed
	}
	intentos, _ := strconv.Atoi(stringAttribute(item["attempts"]))
	return pushoutbox.Registro{
		ID:             stringAttribute(item["SK"]),
		UserID:         stringAttribute(item["userID"]),
		NotificationID: stringAttribute(item["notificationID"]),
		Title:          stringAttribute(item["title"]),
		Message:        stringAttribute(item["message"]),
		Type:           stringAttribute(item["type"]),
		EventID:        stringAttribute(item["eventId"]),
		Status:         pushoutbox.Estado(stringAttribute(item["status"])),
		Attempts:       intentos,
		LeaseUntil:     parsear(item["leaseUntil"]),
		NextAttemptAt:  parsear(item["nextAttemptAt"]),
		CreatedAt:      parsear(item["createdAt"]),
		UpdatedAt:      parsear(item["updatedAt"]),
		UltimaClase:    stringAttribute(item["lastErrorClass"]),
	}
}
