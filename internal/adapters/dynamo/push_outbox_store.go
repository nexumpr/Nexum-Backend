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

// Patrón físico nuevo y autocontenido para el worker FCM (sin GSI2, sin Scan):
//
//	PK = OUTBOX#GLOBAL
//	SK = DUE#<RFC3339Nano UTC>#<notificationID>
//
// La SK fija el vencimiento inicial y permite una Query acotada por prefijo.
// Los reprocesos no reescriben la clave: `nextAttemptAt` es el atributo
// autoritativo y se filtra en la consulta. La idempotencia por
// eventId+userID vive en la MISMA partición con SK IDEM#<sha256>.
// La bandeja E061 se escribe aparte en USER#<sub>/NOTIFICATION#<UTC>#<id>.
const (
	pushOutboxPartition        = "OUTBOX#GLOBAL"
	pushOutboxDuePrefix        = "DUE#"
	pushOutboxIdemPrefix       = "IDEM#"
	pushOutboxEntityType       = "PUSH_OUTBOX"
	pushOutboxIdemEntityType   = "PUSH_OUTBOX_DELIVERY"
	pushNotificationEntityType = "USER_NOTIFICATION"
)

var (
	// ErrOutboxNoDisponible indica configuración o persistencia ausente. Nunca
	// se degrada a almacenamiento en memoria.
	ErrOutboxNoDisponible = errors.New("outbox push no disponible")
	// ErrConflictoLease significa que otro nodo ganó la reclamación.
	ErrConflictoLease = errors.New("conflicto de lease del outbox push")
)

// PushOutboxAPI es el subconjunto de DynamoDB que usa el outbox. No incluye
// Scan: el compilador impide un barrido global.
type PushOutboxAPI interface {
	Query(ctx context.Context, params *dynamodb.QueryInput, optFns ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error)
	GetItem(ctx context.Context, params *dynamodb.GetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error)
	PutItem(ctx context.Context, params *dynamodb.PutItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error)
	UpdateItem(ctx context.Context, params *dynamodb.UpdateItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error)
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

func pushOutboxSK(due time.Time, notificationID string) string {
	return fmt.Sprintf("%s%s#%s", pushOutboxDuePrefix, due.UTC().Format(time.RFC3339Nano), notificationID)
}

func pushOutboxIdemSK(userID, eventID string) string {
	suma := sha256.Sum256([]byte(strings.TrimSpace(userID) + "|" + strings.TrimSpace(eventID)))
	return pushOutboxIdemPrefix + hex.EncodeToString(suma[:])
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

// Crear inserta el trabajo de forma condicional. Devuelve false si ya existía
// la misma clave (replay legítimo), sin sobrescribir su estado.
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
	item := map[string]types.AttributeValue{
		"PK":             stringAttr(pushOutboxPartition),
		"SK":             stringAttr(pushOutboxSK(due, registro.NotificationID)),
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
		"nextAttemptAt":  stringAttr(due.UTC().Format(time.RFC3339Nano)),
		"createdAt":      stringAttr(registro.CreatedAt.UTC().Format(time.RFC3339Nano)),
		"updatedAt":      stringAttr(ahora.UTC().Format(time.RFC3339Nano)),
	}
	_, err := s.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName:           aws.String(s.table),
		Item:                item,
		ConditionExpression: aws.String("attribute_not_exists(PK)"),
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

// Pendientes hace una Query por PK con begins_with(SK,"DUE#"). El filtro
// acota estado y vencimiento; la paginación la controla el límite del lote.
func (s *PushOutboxStore) Pendientes(ctx context.Context, ahora time.Time, limite int) ([]pushoutbox.Registro, error) {
	if !s.disponible() {
		return nil, ErrOutboxNoDisponible
	}
	if limite < 1 {
		limite = 25
	}
	out, err := s.client.Query(ctx, &dynamodb.QueryInput{
		TableName:              aws.String(s.table),
		ConsistentRead:         aws.Bool(true),
		Limit:                  aws.Int32(int32(limite)),
		KeyConditionExpression: aws.String("#pk = :pk AND begins_with(#sk, :prefijo)"),
		FilterExpression: aws.String("((#estado = :pendiente OR #estado = :fallido) OR " +
			"(#estado = :enviando AND (attribute_not_exists(#lease) OR #lease <= :ahora))) AND #siguiente <= :ahora"),
		ExpressionAttributeNames: map[string]string{
			"#pk":        "PK",
			"#sk":        "SK",
			"#estado":    "status",
			"#siguiente": "nextAttemptAt",
			"#lease":     "leaseUntil",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk":        stringAttr(pushOutboxPartition),
			":prefijo":   stringAttr(pushOutboxDuePrefix),
			":pendiente": stringAttr(string(pushoutbox.EstadoPendiente)),
			":fallido":   stringAttr(string(pushoutbox.EstadoFallido)),
			":enviando":  stringAttr(string(pushoutbox.EstadoEnviando)),
			":ahora":     stringAttr(ahora.UTC().Format(time.RFC3339Nano)),
		},
	})
	if err != nil {
		return nil, ErrOutboxNoDisponible
	}
	registros := make([]pushoutbox.Registro, 0, len(out.Items))
	for _, item := range out.Items {
		if stringAttribute(item["entityType"]) != pushOutboxEntityType {
			continue
		}
		registros = append(registros, pushOutboxDesdeItem(item))
	}
	return registros, nil
}

// Reclamar toma el lease bajo condición: sólo si está pendiente/fallido, ya
// venció y nadie lo tiene arrendado. Devuelve el número de intento resultante.
func (s *PushOutboxStore) Reclamar(ctx context.Context, id string, ahora time.Time, lease time.Duration) (int, bool, error) {
	if !s.disponible() {
		return 0, false, ErrOutboxNoDisponible
	}
	ahora = ahora.UTC()
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
			":lease":     stringAttr(ahora.Add(lease).UTC().Format(time.RFC3339Nano)),
			":owner":     stringAttr(s.owner),
			":uno":       numberAttr(1),
			":ahora":     stringAttr(ahora.Format(time.RFC3339Nano)),
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

// MarcarEnviado cierra el trabajo sólo si este nodo sigue siendo el dueño del lease.
func (s *PushOutboxStore) MarcarEnviado(ctx context.Context, id string, ahora time.Time) error {
	return s.actualizarCierre(ctx, id, pushoutbox.EstadoEnviado, "", nil, ahora)
}

// MarcarRevocado guarda un fallo permanente (UNREGISTERED/opt-out) sin reintentos.
func (s *PushOutboxStore) MarcarRevocado(ctx context.Context, id string, clase string, ahora time.Time) error {
	return s.actualizarCierre(ctx, id, pushoutbox.EstadoRevocado, clase, nil, ahora)
}

// Reprogramar guarda un fallo recuperable con backoff y libera el lease.
func (s *PushOutboxStore) Reprogramar(ctx context.Context, id string, attempts int, clase string, siguiente time.Time, ahora time.Time) error {
	return s.actualizarCierre(ctx, id, pushoutbox.EstadoFallido, clase, &struct {
		intentos  int
		siguiente time.Time
	}{intentos: attempts, siguiente: siguiente}, ahora)
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
		":ahora":  stringAttr(ahora.Format(time.RFC3339Nano)),
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
		valores[":siguiente"] = stringAttr(reprogramacion.siguiente.UTC().Format(time.RFC3339Nano))
	}
	_, err := s.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(s.table),
		Key: map[string]types.AttributeValue{
			"PK": stringAttr(pushOutboxPartition),
			"SK": stringAttr(id),
		},
		UpdateExpression:          aws.String(expresion),
		ConditionExpression:       aws.String("#owner = :owner"),
		ExpressionAttributeNames:  nombres,
		ExpressionAttributeValues: valores,
	})
	if err != nil {
		var condicional *types.ConditionalCheckFailedException
		if errors.As(err, &condicional) {
			return ErrConflictoLease
		}
		return ErrOutboxNoDisponible
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
