package dynamo

// Scheduler durable de recordatorios: adaptador real sobre la tabla única.
//
// Patrón físico (Gate de Oleada 5 y contrato de punteros de E014):
//
//	PK = SCHEDULE#GLOBAL
//	SK = DUE#<RFC3339 UTC de ancho fijo>#<reminderID>
//	     userID, reminderID, title, dueAt, timezone, frequency (ausente = único),
//	     owner, leaseUntil, fence, attempts, version, reminderVersion
//
//	PK = USER#<sub>
//	SK = SCHEDULEPTR#<reminderID>      -> skGlobal (SK exacta del ítem global)
//
// Invariantes:
//
//   - No hay `Scan`: la interfaz no lo declara y la lectura de vencidas es una
//     `Query` con condición de rango sobre `SK` (`SK <= DUE#<ahora>#~`), donde
//     `~` es mayor que cualquier carácter de un UUID y garantiza incluir los
//     vencimientos exactamente iguales a `ahora`.
//   - La marca de tiempo usa 9 dígitos de fracción FIJOS. `time.RFC3339Nano`
//     elimina los ceros finales y eso rompe el orden lexicográfico que necesita
//     la Query («…:00.5Z» ordenaría antes de «…:00Z»). Es la misma información
//     en RFC3339, con ancho constante y orden correcto.
//   - `Claim`/`Release` son `UpdateItem` condicionales por `owner` + `fence` +
//     `version`; `Advance`/`Complete` son `TransactWriteItems` sobre recordatorio,
//     agenda y puntero. Nunca hay dos acciones sobre el mismo ítem.
//   - El ítem global lleva `userID` y `owner`/`leaseUntil` porque la purga de
//     E014 (`DeleteGlobalItem`) los necesita para comprobar titularidad y no
//     borrar un trabajo con envío en curso.

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/nexumpr/Nexum-Backend/internal/application/reminders"
	domainDeletion "github.com/nexumpr/Nexum-Backend/internal/domain/deletion"
)

const (
	scheduleDuePrefix   = "DUE#"
	scheduleEntityType  = "REMINDER_SCHEDULE"
	scheduleTimeFormat  = "2006-01-02T15:04:05.000000000Z"
	scheduleLimitSufijo = "~"
	maxPaginasDue       = 5
	scheduleOwnerVacio  = ""
)

const (
	// Una ocurrencia es reclamable si su arrendamiento venció. El `owner` no se
	// evalúa aquí: tras un `Release` el trabajo sigue retenido hasta el reintento
	// programado aunque ya no tenga dueño.
	condicionAgendaReclamable = "attribute_exists(PK) AND #version = :expected AND " +
		"(attribute_not_exists(#lease) OR #lease = :vacio OR #lease <= :ahora)"
	// El lease debe seguir siendo de este nodo y sobre la misma versión de agenda.
	condicionAgendaArrendada = "attribute_exists(PK) AND #owner = :owner AND #fence = :fence AND #version = :agendaVersion"
	// El recordatorio debe seguir existiendo, activo, sin borrar y con la versión leída.
	condicionRecordatorioMovible = "attribute_exists(PK) AND #version = :expected AND #activo = :verdadero " +
		"AND (attribute_not_exists(#borrado) OR #borrado = :falso)"
	condicionRecordatorioDesactivable = "attribute_exists(PK) AND #version = :expected " +
		"AND (attribute_not_exists(#borrado) OR #borrado = :falso)"
	// El puntero debe apuntar al recordatorio y versión esperados (CAS compartido).
	condicionPunteroRecordatorio = "attribute_exists(PK) AND #reminderSK = :recordatorioSK AND #version = :expected"
	// El puntero de agenda debe seguir apuntando a la ocurrencia que se avanza.
	condicionPunteroAgendaSK = "attribute_exists(PK) AND #skGlobal = :viejaSK"
	// El puntero de agenda sólo puede retirarse si sigue apuntando a la ocurrencia
	// que se retira. Si el titular movió el vencimiento (E066) el puntero ya apunta
	// a la ocurrencia nueva: borrarlo la dejaría inalcanzable para la purga de E014
	// y el recordatorio no volvería a dispararse.
	condicionPunteroAgendaHuerfana = "attribute_not_exists(PK) OR #skGlobal = :ocSK"
)

// ScheduleAPI es el subconjunto de DynamoDB que usa el scheduler. No incluye
// `Scan`: el compilador impide un barrido global.
type ScheduleAPI interface {
	Query(ctx context.Context, params *dynamodb.QueryInput, optFns ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error)
	GetItem(ctx context.Context, params *dynamodb.GetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error)
	UpdateItem(ctx context.Context, params *dynamodb.UpdateItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error)
	TransactWriteItems(ctx context.Context, params *dynamodb.TransactWriteItemsInput, optFns ...func(*dynamodb.Options)) (*dynamodb.TransactWriteItemsOutput, error)
}

// ScheduleStore implementa reminders.ScheduleStore.
type ScheduleStore struct {
	client ScheduleAPI
	table  string
	owner  string
}

// NewScheduleStore construye el almacén con una identidad de nodo efímera. Sin
// cliente o tabla, todas las operaciones fallan cerrado.
func NewScheduleStore(client ScheduleAPI, table string) *ScheduleStore {
	aleatorio := make([]byte, 12)
	if _, err := rand.Read(aleatorio); err != nil {
		aleatorio = []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11}
	}
	return &ScheduleStore{
		client: client,
		table:  strings.TrimSpace(table),
		owner:  "scheduler-" + hex.EncodeToString(aleatorio),
	}
}

// WithOwner fija el identificador de nodo (determinista en pruebas).
func (s *ScheduleStore) WithOwner(owner string) *ScheduleStore {
	if s != nil && strings.TrimSpace(owner) != "" {
		s.owner = strings.TrimSpace(owner)
	}
	return s
}

// Owner expone el identificador usado en los leases.
func (s *ScheduleStore) Owner() string {
	if s == nil {
		return ""
	}
	return s.owner
}

func (s *ScheduleStore) disponible() bool {
	return s != nil && s.client != nil && s.table != ""
}

// scheduleSortKey construye la SK global de una ocurrencia.
func scheduleSortKey(dueAt time.Time, reminderID string) string {
	return scheduleDuePrefix + dueAt.UTC().Format(scheduleTimeFormat) + "#" + strings.TrimSpace(reminderID)
}

// scheduleLimitKey es el límite superior inclusivo de vencimientos: incluye todos
// los recordatorios cuya SK es `DUE#<hasta ahora>#<uuid>`.
func scheduleLimitKey(ahora time.Time) string {
	return scheduleDuePrefix + ahora.UTC().Format(scheduleTimeFormat) + "#" + scheduleLimitSufijo
}

// scheduleItemId devuelve los atributos que debe llevar el ítem global.
func scheduleItemAttrs(userID, reminderID, titulo string, dueAt time.Time, timezone string, frequency *reminders.Frequency) map[string]types.AttributeValue {
	item := map[string]types.AttributeValue{
		"entityType":      stringAttr(scheduleEntityType),
		"userID":          stringAttr(strings.TrimSpace(userID)),
		"reminderID":      stringAttr(strings.TrimSpace(reminderID)),
		"title":           stringAttr(titulo),
		"dueAt":           stringAttr(dueAt.UTC().Format(time.RFC3339Nano)),
		"timezone":        stringAttr(strings.TrimSpace(timezone)),
		"owner":           stringAttr(scheduleOwnerVacio),
		"leaseUntil":      stringAttr(""),
		"fence":           numberAttr(0),
		"attempts":        numberAttr(0),
		"version":         numberAttr(1),
		"reminderVersion": numberAttr(1),
	}
	if frequency != nil {
		item["frequency"] = stringAttr(string(*frequency))
	}
	return item
}

// Due devuelve las ocurrencias vencidas, en orden de vencimiento.
//
// La condición de clave es una única comparación de rango (`#sk <= :limite`)
// sobre la partición global. El filtro descarta lo que tiene un lease vigente; si
// una página queda vacía por el filtro se continúa con `ExclusiveStartKey` para
// que una ocurrencia reprogramada al frente no bloquee a las siguientes. El
// número de páginas por llamada está acotado y se documenta: no es un barrido.
func (s *ScheduleStore) Due(ctx context.Context, ahora time.Time, limite int) ([]reminders.Ocurrencia, error) {
	if !s.disponible() {
		return nil, reminders.ErrUnavailable
	}
	if limite < 1 {
		limite = 1
	}
	if limite > reminders.MaxSchedulerBatch {
		limite = reminders.MaxSchedulerBatch
	}
	ahora = ahora.UTC()
	entrada := &dynamodb.QueryInput{
		TableName:              aws.String(s.table),
		ConsistentRead:         aws.Bool(true),
		Limit:                  aws.Int32(int32(limite)),
		KeyConditionExpression: aws.String("#pk = :pk AND #sk <= :limite"),
		FilterExpression: aws.String("attribute_not_exists(#lease) OR #lease = :vacio OR " +
			"#lease <= :ahora"),
		ExpressionAttributeNames: map[string]string{
			"#pk":    "PK",
			"#sk":    "SK",
			"#lease": "leaseUntil",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk":     stringAttr(domainDeletion.PartitionScheduleGlobal),
			":limite": stringAttr(scheduleLimitKey(ahora)),
			":vacio":  stringAttr(scheduleOwnerVacio),
			":ahora":  stringAttr(ahora.Format(time.RFC3339Nano)),
		},
	}

	salida := make([]reminders.Ocurrencia, 0, limite)
	for pagina := 0; pagina < maxPaginasDue; pagina++ {
		out, err := s.client.Query(ctx, entrada)
		if err != nil {
			return nil, reminders.ErrUnavailable
		}
		for _, item := range out.Items {
			if stringAttribute(item["entityType"]) != scheduleEntityType {
				continue
			}
			oc, errOcurrencia := ocurrenciaDesdeItem(item)
			if errOcurrencia != nil {
				// Un ítem corrupto no puede reclamarse ni repararse sin Scan. Se
				// omite para no bloquear la partición; la ausencia de su puntero es
				// la señal de diagnóstico en E014.
				continue
			}
			if !oc.Disponible(ahora) {
				continue
			}
			salida = append(salida, oc)
			if len(salida) >= limite {
				return salida, nil
			}
		}
		if len(out.LastEvaluatedKey) == 0 || len(salida) > 0 {
			break
		}
		entrada.ExclusiveStartKey = out.LastEvaluatedKey
	}
	return salida, nil
}

// Claim toma el lease con una escritura condicional y devuelve el token exacto.
func (s *ScheduleStore) Claim(ctx context.Context, oc reminders.Ocurrencia, ahora time.Time, lease time.Duration) (*reminders.Lease, error) {
	if !s.disponible() {
		return nil, reminders.ErrUnavailable
	}
	if strings.TrimSpace(oc.SK) == "" || strings.TrimSpace(oc.UserID) == "" || strings.TrimSpace(oc.ReminderID) == "" {
		return nil, reminders.ErrOcurrenciaInvalida
	}
	if lease <= 0 {
		return nil, reminders.ErrOcurrenciaInvalida
	}
	ahora = ahora.UTC()
	siguienteVersion := oc.Version + 1
	siguienteFence := oc.Fence + 1
	siguienteAttempts := oc.Attempts + 1
	limite := ahora.Add(lease)

	out, err := s.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(s.table),
		Key: map[string]types.AttributeValue{
			"PK": stringAttr(domainDeletion.PartitionScheduleGlobal),
			"SK": stringAttr(oc.SK),
		},
		UpdateExpression: aws.String("SET #owner = :owner, #lease = :lease, #version = :version, " +
			"#fence = :fence, #attempts = :attempts, #actualizado = :ahora"),
		ConditionExpression: aws.String(condicionAgendaReclamable),
		ExpressionAttributeNames: map[string]string{
			"#owner":       "owner",
			"#lease":       "leaseUntil",
			"#version":     "version",
			"#fence":       "fence",
			"#attempts":    "attempts",
			"#actualizado": "updatedAt",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":expected": numberAttr(oc.Version),
			":owner":    stringAttr(s.owner),
			":lease":    stringAttr(limite.Format(time.RFC3339Nano)),
			":version":  numberAttr(siguienteVersion),
			":fence":    numberAttr(siguienteFence),
			":attempts": numberAttr(int64(siguienteAttempts)),
			":vacio":    stringAttr(scheduleOwnerVacio),
			":ahora":    stringAttr(ahora.Format(time.RFC3339Nano)),
		},
		ReturnValues: types.ReturnValueAllNew,
	})
	if err != nil {
		var condicional *types.ConditionalCheckFailedException
		if errors.As(err, &condicional) {
			return nil, reminders.ErrLeasePerdido
		}
		return nil, reminders.ErrUnavailable
	}

	reclamada := oc
	reclamada.Owner = s.owner
	reclamada.LeaseUntil = limite
	reclamada.Version = siguienteVersion
	reclamada.Fence = siguienteFence
	reclamada.Attempts = siguienteAttempts
	if out != nil && len(out.Attributes) > 0 {
		if actualizada, errOcurrencia := ocurrenciaDesdeItem(out.Attributes); errOcurrencia == nil {
			reclamada = actualizada
		}
	}
	return &reminders.Lease{
		Ocurrencia: reclamada,
		Owner:      s.owner,
		Fence:      reclamada.Fence,
		Version:    reclamada.Version,
		Attempts:   reclamada.Attempts,
		LeaseUntil: reclamada.LeaseUntil,
	}, nil
}

// Release libera el lease y deja `siguiente` como instante mínimo de reintento.
// El `owner` se retira para que la purga de E014 no lo considere un envío en curso.
func (s *ScheduleStore) Release(ctx context.Context, lease reminders.Lease, motivo string, siguiente time.Time, ahora time.Time) error {
	if !s.disponible() {
		return reminders.ErrUnavailable
	}
	oc := lease.Ocurrencia
	if strings.TrimSpace(oc.UserID) == "" || strings.TrimSpace(oc.ReminderID) == "" || strings.TrimSpace(oc.SK) == "" {
		return reminders.ErrOcurrenciaInvalida
	}
	ahora = ahora.UTC()
	if siguiente.IsZero() || siguiente.Before(ahora) {
		siguiente = ahora
	}
	_, err := s.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(s.table),
		Key: map[string]types.AttributeValue{
			"PK": stringAttr(domainDeletion.PartitionScheduleGlobal),
			"SK": stringAttr(oc.SK),
		},
		UpdateExpression: aws.String("SET #owner = :vacio, #lease = :siguiente, #clase = :clase, " +
			"#version = :version, #actualizado = :ahora"),
		ConditionExpression: aws.String(condicionAgendaArrendada),
		ExpressionAttributeNames: map[string]string{
			"#owner":       "owner",
			"#lease":       "leaseUntil",
			"#clase":       "lastErrorClass",
			"#version":     "version",
			"#actualizado": "updatedAt",
			"#fence":       "fence",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":vacio":         stringAttr(scheduleOwnerVacio),
			":siguiente":     stringAttr(siguiente.UTC().Format(time.RFC3339Nano)),
			":clase":         stringAttr(strings.TrimSpace(motivo)),
			":owner":         stringAttr(strings.TrimSpace(lease.Owner)),
			":fence":         numberAttr(lease.Fence),
			":agendaVersion": numberAttr(lease.Version),
			":version":       numberAttr(lease.Version + 1),
			":ahora":         stringAttr(ahora.Format(time.RFC3339Nano)),
		},
	})
	if err == nil {
		return nil
	}
	var condicional *types.ConditionalCheckFailedException
	if errors.As(err, &condicional) {
		return reminders.ErrLeasePerdido
	}
	return reminders.ErrUnavailable
}

// Advance mueve la ocurrencia y su recordatorio a `siguiente` en una única
// transacción de cinco acciones sobre cinco ítems distintos.
func (s *ScheduleStore) Advance(ctx context.Context, lease reminders.Lease, siguiente time.Time, ahora time.Time) error {
	if !s.disponible() {
		return reminders.ErrUnavailable
	}
	oc := lease.Ocurrencia
	if strings.TrimSpace(oc.UserID) == "" || strings.TrimSpace(oc.ReminderID) == "" {
		return reminders.ErrOcurrenciaInvalida
	}
	if siguiente.IsZero() || !siguiente.After(oc.DueAt) {
		return reminders.ErrFrecuenciaInvalida
	}
	ahora = ahora.UTC()
	siguiente = siguiente.UTC().Truncate(time.Second)
	skVieja := reminderSortKey(oc.DueAt, oc.ReminderID)
	skNueva := reminderSortKey(siguiente, oc.ReminderID)
	pkPunteroAgenda, skPunteroAgenda := domainDeletion.SchedulePointerKeys(oc.UserID, oc.ReminderID)

	raw, err := s.leerRecordatorio(ctx, oc.UserID, skVieja)
	if err != nil {
		return err
	}
	if raw == nil || boolAttribute(raw["deleted"]) {
		return s.retirarAgendaHuerfana(ctx, lease, ahora)
	}
	if version := numeroAttribute(raw["version"]); version != oc.ReminderVersion {
		return reminders.ErrLeasePerdido
	}

	nuevaVersion := oc.ReminderVersion + 1
	item := make(map[string]types.AttributeValue, len(raw)+2)
	for clave, valor := range raw {
		item[clave] = valor
	}
	item["SK"] = stringAttr(skNueva)
	item["dueAt"] = stringAttr(siguiente.Format(time.RFC3339))
	item["version"] = numberAttr(nuevaVersion)
	item["updatedAt"] = stringAttr(ahora.Format(time.RFC3339Nano))

	var frecuencia *reminders.Frequency
	if oc.Frequency != nil {
		frecuencia = oc.Frequency
	}
	nuevaAgenda := scheduleItemAttrs(oc.UserID, oc.ReminderID, oc.Titulo, siguiente, oc.Timezone, frecuencia)
	nuevaAgenda["PK"] = stringAttr(domainDeletion.PartitionScheduleGlobal)
	nuevaAgenda["SK"] = stringAttr(scheduleSortKey(siguiente, oc.ReminderID))
	nuevaAgenda["reminderVersion"] = numberAttr(nuevaVersion)
	nuevaAgenda["createdAt"] = stringAttr(ahora.Format(time.RFC3339Nano))
	nuevaAgenda["updatedAt"] = stringAttr(ahora.Format(time.RFC3339Nano))

	acciones := []types.TransactWriteItem{
		{Delete: &types.Delete{
			TableName:           aws.String(s.table),
			Key:                 map[string]types.AttributeValue{"PK": stringAttr(reminderPartition(oc.UserID)), "SK": stringAttr(skVieja)},
			ConditionExpression: aws.String(condicionRecordatorioMovible),
			ExpressionAttributeNames: map[string]string{
				"#version": "version", "#activo": "active", "#borrado": "deleted",
			},
			ExpressionAttributeValues: map[string]types.AttributeValue{
				":expected":  numberAttr(oc.ReminderVersion),
				":verdadero": boolAttr(true),
				":falso":     boolAttr(false),
			},
		}},
		{Put: &types.Put{
			TableName:           aws.String(s.table),
			Item:                item,
			ConditionExpression: aws.String("attribute_not_exists(PK)"),
		}},
		{Update: &types.Update{
			TableName:           aws.String(s.table),
			Key:                 map[string]types.AttributeValue{"PK": stringAttr(reminderPartition(oc.UserID)), "SK": stringAttr(reminderPointerSortKey(oc.ReminderID))},
			UpdateExpression:    aws.String("SET #version = :next, #reminderSK = :nuevaSK, #actualizado = :ahora"),
			ConditionExpression: aws.String(condicionPunteroRecordatorio),
			ExpressionAttributeNames: map[string]string{
				"#version": "version", "#reminderSK": "reminderSK", "#actualizado": "updatedAt",
			},
			ExpressionAttributeValues: map[string]types.AttributeValue{
				":recordatorioSK": stringAttr(skVieja),
				":expected":       numberAttr(oc.ReminderVersion),
				":next":           numberAttr(nuevaVersion),
				":nuevaSK":        stringAttr(skNueva),
				":ahora":          stringAttr(ahora.Format(time.RFC3339Nano)),
			},
		}},
		{Delete: &types.Delete{
			TableName:           aws.String(s.table),
			Key:                 map[string]types.AttributeValue{"PK": stringAttr(domainDeletion.PartitionScheduleGlobal), "SK": stringAttr(oc.SK)},
			ConditionExpression: aws.String(condicionAgendaArrendada),
			ExpressionAttributeNames: map[string]string{
				"#owner": "owner", "#fence": "fence", "#version": "version",
			},
			ExpressionAttributeValues: map[string]types.AttributeValue{
				":owner":         stringAttr(strings.TrimSpace(lease.Owner)),
				":fence":         numberAttr(lease.Fence),
				":agendaVersion": numberAttr(lease.Version),
			},
		}},
		{Put: &types.Put{
			TableName:           aws.String(s.table),
			Item:                nuevaAgenda,
			ConditionExpression: aws.String("attribute_not_exists(PK)"),
		}},
		{Update: &types.Update{
			TableName: aws.String(s.table),
			Key: map[string]types.AttributeValue{
				"PK": stringAttr(pkPunteroAgenda),
				"SK": stringAttr(skPunteroAgenda),
			},
			UpdateExpression:    aws.String("SET #skGlobal = :nuevaSK, #due = :due, #actualizado = :ahora"),
			ConditionExpression: aws.String(condicionPunteroAgendaSK),
			ExpressionAttributeNames: map[string]string{
				"#skGlobal": "skGlobal", "#due": "dueAt", "#actualizado": "updatedAt",
			},
			ExpressionAttributeValues: map[string]types.AttributeValue{
				":nuevaSK": stringAttr(scheduleSortKey(siguiente, oc.ReminderID)),
				":viejaSK": stringAttr(oc.SK),
				":due":     stringAttr(siguiente.Format(time.RFC3339Nano)),
				":ahora":   stringAttr(ahora.Format(time.RFC3339Nano)),
			},
		}},
	}
	return s.ejecutarTransaccion(ctx, acciones)
}

// Complete retira la ocurrencia y deja el recordatorio inactivo. Se usa para un
// disparo único, para una cuenta no activa y para una recurrencia inválida.
func (s *ScheduleStore) Complete(ctx context.Context, lease reminders.Lease, ahora time.Time) error {
	if !s.disponible() {
		return reminders.ErrUnavailable
	}
	oc := lease.Ocurrencia
	if strings.TrimSpace(oc.UserID) == "" || strings.TrimSpace(oc.ReminderID) == "" {
		return reminders.ErrOcurrenciaInvalida
	}
	ahora = ahora.UTC()
	skRecordatorio := reminderSortKey(oc.DueAt, oc.ReminderID)
	pkPunteroAgenda, skPunteroAgenda := domainDeletion.SchedulePointerKeys(oc.UserID, oc.ReminderID)

	raw, err := s.leerRecordatorio(ctx, oc.UserID, skRecordatorio)
	if err != nil {
		return err
	}
	if raw == nil || boolAttribute(raw["deleted"]) {
		return s.retirarAgendaHuerfana(ctx, lease, ahora)
	}
	if version := numeroAttribute(raw["version"]); version != oc.ReminderVersion {
		return reminders.ErrLeasePerdido
	}
	siguienteVersion := oc.ReminderVersion + 1

	acciones := []types.TransactWriteItem{
		{Update: &types.Update{
			TableName:           aws.String(s.table),
			Key:                 map[string]types.AttributeValue{"PK": stringAttr(reminderPartition(oc.UserID)), "SK": stringAttr(skRecordatorio)},
			UpdateExpression:    aws.String("SET #activo = :falso, #version = :next, #actualizado = :ahora"),
			ConditionExpression: aws.String(condicionRecordatorioDesactivable),
			ExpressionAttributeNames: map[string]string{
				"#activo": "active", "#version": "version", "#borrado": "deleted", "#actualizado": "updatedAt",
			},
			ExpressionAttributeValues: map[string]types.AttributeValue{
				":expected": numberAttr(oc.ReminderVersion),
				":falso":    boolAttr(false),
				":next":     numberAttr(siguienteVersion),
				":ahora":    stringAttr(ahora.Format(time.RFC3339Nano)),
			},
		}},
		{Delete: &types.Delete{
			TableName:           aws.String(s.table),
			Key:                 map[string]types.AttributeValue{"PK": stringAttr(domainDeletion.PartitionScheduleGlobal), "SK": stringAttr(oc.SK)},
			ConditionExpression: aws.String(condicionAgendaArrendada),
			ExpressionAttributeNames: map[string]string{
				"#owner": "owner", "#fence": "fence", "#version": "version",
			},
			ExpressionAttributeValues: map[string]types.AttributeValue{
				":owner":         stringAttr(strings.TrimSpace(lease.Owner)),
				":fence":         numberAttr(lease.Fence),
				":agendaVersion": numberAttr(lease.Version),
			},
		}},
		{Update: &types.Update{
			TableName:           aws.String(s.table),
			Key:                 map[string]types.AttributeValue{"PK": stringAttr(reminderPartition(oc.UserID)), "SK": stringAttr(reminderPointerSortKey(oc.ReminderID))},
			UpdateExpression:    aws.String("SET #version = :next, #actualizado = :ahora"),
			ConditionExpression: aws.String(condicionPunteroRecordatorio),
			ExpressionAttributeNames: map[string]string{
				"#version": "version", "#reminderSK": "reminderSK", "#actualizado": "updatedAt",
			},
			ExpressionAttributeValues: map[string]types.AttributeValue{
				":recordatorioSK": stringAttr(skRecordatorio),
				":expected":       numberAttr(oc.ReminderVersion),
				":next":           numberAttr(siguienteVersion),
				":ahora":          stringAttr(ahora.Format(time.RFC3339Nano)),
			},
		}},
		{Delete: &types.Delete{
			TableName: aws.String(s.table),
			Key: map[string]types.AttributeValue{
				"PK": stringAttr(pkPunteroAgenda),
				"SK": stringAttr(skPunteroAgenda),
			},
		}},
	}
	return s.ejecutarTransaccion(ctx, acciones)
}

// retirarAgendaHuerfana elimina la ocurrencia cuando su recordatorio ya no existe
// o está borrado. Sin esta salida, una purga concurrente (E014) dejaría la
// ocurrencia en un bucle de reintentos que nunca podría completarse.
//
// El puntero de agenda se retira con condición: sólo si sigue apuntando a ESTA
// ocurrencia. Si el titular movió el vencimiento (E066) el puntero ya apunta a la
// ocurrencia nueva y borrarlo la dejaría inalcanzable para la purga de E014,
// además de perder el disparo. Cuando esa condición falla se reintenta retirando
// únicamente la ocurrencia antigua; el puntero nuevo nunca se toca.
func (s *ScheduleStore) retirarAgendaHuerfana(ctx context.Context, lease reminders.Lease, ahora time.Time) error {
	oc := lease.Ocurrencia
	pkPuntero, skPuntero := domainDeletion.SchedulePointerKeys(oc.UserID, oc.ReminderID)
	retirarOcurrencia := types.TransactWriteItem{Delete: &types.Delete{
		TableName:           aws.String(s.table),
		Key:                 map[string]types.AttributeValue{"PK": stringAttr(domainDeletion.PartitionScheduleGlobal), "SK": stringAttr(oc.SK)},
		ConditionExpression: aws.String(condicionAgendaArrendada),
		ExpressionAttributeNames: map[string]string{
			"#owner": "owner", "#fence": "fence", "#version": "version",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":owner":         stringAttr(strings.TrimSpace(lease.Owner)),
			":fence":         numberAttr(lease.Fence),
			":agendaVersion": numberAttr(lease.Version),
		},
	}}
	err := s.ejecutarTransaccion(ctx, []types.TransactWriteItem{
		retirarOcurrencia,
		{Delete: &types.Delete{
			TableName:           aws.String(s.table),
			Key:                 map[string]types.AttributeValue{"PK": stringAttr(pkPuntero), "SK": stringAttr(skPuntero)},
			ConditionExpression: aws.String(condicionPunteroAgendaHuerfana),
			ExpressionAttributeNames: map[string]string{
				"#skGlobal": "skGlobal",
			},
			ExpressionAttributeValues: map[string]types.AttributeValue{
				":ocSK": stringAttr(oc.SK),
			},
		}},
	})
	if err == nil {
		return nil
	}
	if !errors.Is(err, reminders.ErrLeasePerdido) {
		return err
	}
	// Una condición canceló la transacción. Si la que falló fue la del puntero (ya
	// apunta a otra ocurrencia) basta con retirar la ocurrencia antigua; si fue la
	// de la ocurrencia, el reintento vuelve a fallar y se propaga ErrLeasePerdido.
	if errRetirar := s.ejecutarTransaccion(ctx, []types.TransactWriteItem{retirarOcurrencia}); errRetirar != nil {
		return errRetirar
	}
	return nil
}

// leerRecordatorio lee el ítem exacto del recordatorio con consistencia fuerte.
func (s *ScheduleStore) leerRecordatorio(ctx context.Context, userID, sk string) (map[string]types.AttributeValue, error) {
	out, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName:      aws.String(s.table),
		ConsistentRead: aws.Bool(true),
		Key: map[string]types.AttributeValue{
			"PK": stringAttr(reminderPartition(userID)),
			"SK": stringAttr(sk),
		},
	})
	if err != nil {
		return nil, reminders.ErrUnavailable
	}
	if out == nil || len(out.Item) == 0 {
		return nil, nil
	}
	if stringAttribute(out.Item["entityType"]) != reminderEntity {
		return nil, nil
	}
	return out.Item, nil
}

func (s *ScheduleStore) ejecutarTransaccion(ctx context.Context, acciones []types.TransactWriteItem) error {
	if _, err := s.client.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{TransactItems: acciones}); err != nil {
		if esCancelacionPorCondicion(err) {
			return reminders.ErrLeasePerdido
		}
		return reminders.ErrUnavailable
	}
	return nil
}

// ocurrenciaDesdeItem materializa una fila de `SCHEDULE#GLOBAL`.
func ocurrenciaDesdeItem(item map[string]types.AttributeValue) (reminders.Ocurrencia, error) {
	userID := strings.TrimSpace(stringAttribute(item["userID"]))
	reminderID := strings.TrimSpace(stringAttribute(item["reminderID"]))
	sk := strings.TrimSpace(stringAttribute(item["SK"]))
	if userID == "" || reminderID == "" || sk == "" {
		return reminders.Ocurrencia{}, reminders.ErrOcurrenciaInvalida
	}
	dueAt, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(stringAttribute(item["dueAt"])))
	if err != nil {
		return reminders.Ocurrencia{}, reminders.ErrOcurrenciaInvalida
	}
	oc := reminders.Ocurrencia{
		SK:              sk,
		UserID:          userID,
		ReminderID:      reminderID,
		Titulo:          stringAttribute(item["title"]),
		DueAt:           dueAt.UTC(),
		Timezone:        strings.TrimSpace(stringAttribute(item["timezone"])),
		ReminderVersion: numeroAttribute(item["reminderVersion"]),
		Version:         numeroAttribute(item["version"]),
		Fence:           numeroAttribute(item["fence"]),
		Attempts:        int(numeroAttribute(item["attempts"])),
		Owner:           strings.TrimSpace(stringAttribute(item["owner"])),
	}
	if oc.ReminderVersion < 1 || oc.Version < 1 {
		return reminders.Ocurrencia{}, reminders.ErrOcurrenciaInvalida
	}
	if lease := strings.TrimSpace(stringAttribute(item["leaseUntil"])); lease != "" {
		if parsed, errLease := time.Parse(time.RFC3339Nano, lease); errLease == nil {
			oc.LeaseUntil = parsed.UTC()
		}
	}
	if raw := strings.TrimSpace(stringAttribute(item["frequency"])); raw != "" {
		frecuencia, ok := reminders.ParseFrequency(raw)
		if !ok {
			return reminders.Ocurrencia{}, reminders.ErrFrecuenciaInvalida
		}
		oc.Frequency = &frecuencia
	}
	return oc, nil
}

// accionesAgenda devuelve las acciones que mantienen el ítem global de agenda y
// su puntero alineados con el recordatorio. Se emiten en la MISMA transacción que
// el recordatorio (Create/Update/Delete) para que no exista estado parcial:
//
//   - recordatorio activo: Put del ítem global (crear o reescribir) y Put del
//     puntero. Si el vencimiento cambió, Delete del ítem global anterior.
//   - recordatorio inactivo: Delete del ítem global y del puntero.
//
// Nunca hay dos acciones sobre el mismo ítem. Los contadores monotónicos
// (`version`, `fence`, `attempts`) se conservan cuando la SK no cambia; así un
// `Advance` en vuelo pierde por versión en lugar de reutilizar un fence antiguo.
// El `owner`/`leaseUntil` se limpian: la edición del titular prevalece y el
// scheduler tendrá que reclamar de nuevo.
func (s *RemindersStore) accionesAgenda(
	ctx context.Context,
	userID, reminderID, titulo string,
	dueAt time.Time,
	timezone string,
	frequency *reminders.Frequency,
	activo bool,
	version int64,
	condicionAlta bool,
	ahora time.Time,
) ([]types.TransactWriteItem, error) {
	skGlobalVieja, err := s.leerPunteroAgenda(ctx, userID, reminderID)
	if err != nil {
		return nil, err
	}
	pkPuntero, skPuntero := domainDeletion.SchedulePointerKeys(userID, reminderID)
	acciones := make([]types.TransactWriteItem, 0, 3)

	if !activo {
		if skGlobalVieja != "" {
			acciones = append(acciones, types.TransactWriteItem{Delete: &types.Delete{
				TableName: aws.String(s.table),
				Key: map[string]types.AttributeValue{
					"PK": stringAttr(domainDeletion.PartitionScheduleGlobal),
					"SK": stringAttr(skGlobalVieja),
				},
			}})
		}
		acciones = append(acciones, types.TransactWriteItem{Delete: &types.Delete{
			TableName: aws.String(s.table),
			Key:       map[string]types.AttributeValue{"PK": stringAttr(pkPuntero), "SK": stringAttr(skPuntero)},
		}})
		return acciones, nil
	}

	skGlobalNueva := scheduleSortKey(dueAt, reminderID)
	versionAgenda, fence, intentos := int64(1), int64(0), int64(0)
	if skGlobalVieja == skGlobalNueva {
		conservada, errLeer := s.leerAgenda(ctx, skGlobalVieja)
		if errLeer != nil {
			return nil, errLeer
		}
		if conservada != nil {
			versionAgenda = conservada.Version + 1
			fence = conservada.Fence
			intentos = conservada.Attempts
		}
	} else if skGlobalVieja != "" {
		acciones = append(acciones, types.TransactWriteItem{Delete: &types.Delete{
			TableName: aws.String(s.table),
			Key: map[string]types.AttributeValue{
				"PK": stringAttr(domainDeletion.PartitionScheduleGlobal),
				"SK": stringAttr(skGlobalVieja),
			},
		}})
	}

	agenda := scheduleItemAttrs(userID, reminderID, titulo, dueAt, timezone, frequency)
	agenda["PK"] = stringAttr(domainDeletion.PartitionScheduleGlobal)
	agenda["SK"] = stringAttr(skGlobalNueva)
	agenda["version"] = numberAttr(versionAgenda)
	agenda["fence"] = numberAttr(fence)
	agenda["attempts"] = numberAttr(intentos)
	agenda["reminderVersion"] = numberAttr(version)
	agenda["createdAt"] = stringAttr(ahora.UTC().Format(time.RFC3339Nano))
	agenda["updatedAt"] = stringAttr(ahora.UTC().Format(time.RFC3339Nano))
	putAgenda := types.TransactWriteItem{Put: &types.Put{TableName: aws.String(s.table), Item: agenda}}
	if condicionAlta {
		putAgenda.Put.ConditionExpression = aws.String(condicionAltaNueva)
	}
	acciones = append(acciones, putAgenda)

	punteroAgenda := schedulePointerItem(userID, reminderID, skGlobalNueva, dueAt, ahora)
	putPuntero := types.TransactWriteItem{Put: &types.Put{TableName: aws.String(s.table), Item: punteroAgenda}}
	if condicionAlta {
		putPuntero.Put.ConditionExpression = aws.String(condicionAltaNueva)
	}
	acciones = append(acciones, putPuntero)
	return acciones, nil
}

// lecturaAgenda es la proyección de los contadores monotónicos del ítem global.
type lecturaAgenda struct {
	Version  int64
	Fence    int64
	Attempts int64
}

// leerPunteroAgenda devuelve la SK global apuntada, si el puntero existe.
func (s *RemindersStore) leerPunteroAgenda(ctx context.Context, userID, reminderID string) (string, error) {
	out, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName:      aws.String(s.table),
		ConsistentRead: aws.Bool(true),
		Key: map[string]types.AttributeValue{
			"PK": stringAttr(reminderPartition(userID)),
			"SK": stringAttr(domainDeletion.PrefixSchedulePointer + strings.TrimSpace(reminderID)),
		},
	})
	if err != nil {
		return "", reminders.ErrUnavailable
	}
	if out == nil || len(out.Item) == 0 {
		return "", nil
	}
	if stringAttribute(out.Item["entityType"]) != domainDeletion.EntitySchedulePointer {
		return "", nil
	}
	return strings.TrimSpace(stringAttribute(out.Item["skGlobal"])), nil
}

// leerAgenda devuelve los contadores del ítem global indicado; nil si no existe.
func (s *RemindersStore) leerAgenda(ctx context.Context, skGlobal string) (*lecturaAgenda, error) {
	if strings.TrimSpace(skGlobal) == "" {
		return nil, nil
	}
	out, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName:      aws.String(s.table),
		ConsistentRead: aws.Bool(true),
		Key: map[string]types.AttributeValue{
			"PK": stringAttr(domainDeletion.PartitionScheduleGlobal),
			"SK": stringAttr(skGlobal),
		},
	})
	if err != nil {
		return nil, reminders.ErrUnavailable
	}
	if out == nil || len(out.Item) == 0 {
		return nil, nil
	}
	if stringAttribute(out.Item["entityType"]) != scheduleEntityType {
		return nil, nil
	}
	return &lecturaAgenda{
		Version:  numeroAttribute(out.Item["version"]),
		Fence:    numeroAttribute(out.Item["fence"]),
		Attempts: numeroAttribute(out.Item["attempts"]),
	}, nil
}

// numeroAttribute lee un número de DynamoDB; ausente o inválido devuelve 0.
func numeroAttribute(value types.AttributeValue) int64 {
	switch v := value.(type) {
	case *types.AttributeValueMemberN:
		numero, err := strconv.ParseInt(v.Value, 10, 64)
		if err != nil {
			return 0
		}
		return numero
	default:
		return 0
	}
}

// schedulePointerItem construye el puntero de purga del titular. Se escribe en la
// MISMA transacción que el ítem global; sin él, E014 no podría alcanzar la
// partición del servicio sin `Scan`.
func schedulePointerItem(userID, reminderID, skGlobal string, dueAt time.Time, ahora time.Time) map[string]types.AttributeValue {
	pk, sk := domainDeletion.SchedulePointerKeys(userID, reminderID)
	return map[string]types.AttributeValue{
		"PK":         stringAttr(pk),
		"SK":         stringAttr(sk),
		"entityType": stringAttr(domainDeletion.EntitySchedulePointer),
		"userID":     stringAttr(strings.TrimSpace(userID)),
		"reminderID": stringAttr(strings.TrimSpace(reminderID)),
		"skGlobal":   stringAttr(skGlobal),
		"dueAt":      stringAttr(dueAt.UTC().Format(time.RFC3339Nano)),
		"createdAt":  stringAttr(ahora.Format(time.RFC3339Nano)),
		"updatedAt":  stringAttr(ahora.Format(time.RFC3339Nano)),
	}
}
