package dynamo

import (
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	appRecurring "github.com/nexumpr/Nexum-Backend/internal/application/recurring"
	domainRecurring "github.com/nexumpr/Nexum-Backend/internal/domain/recurring"
)

// Mapeo entre el ítem de la tabla única y el recurso `RecurringPayment` del contrato.
//
// Este archivo contiene código de PRODUCCIÓN: el adaptador de E032–E036. Los dobles de prueba
// viven en `tests/unit/dynamo`, nunca en el paquete de producción.

// recurringItem materializa la regla en un ítem de la tabla única.
func recurringItem(pk, sk, userID string, pago appRecurring.Payment, iso string) map[string]types.AttributeValue {
	item := map[string]types.AttributeValue{
		"PK":          stringAttr(pk),
		"SK":          stringAttr(sk),
		"entityType":  stringAttr(recurringEntityType),
		"userId":      stringAttr(userID),
		"recurringId": stringAttr(pago.ID),
		"name":        stringAttr(pago.Name),
		"amountCents": numberAttr(pago.AmountCents),
		"currency":    stringAttr(pago.Currency),
		"frequency":   stringAttr(pago.Frequency.String()),
		"nextDueDate": stringAttr(domainRecurring.FormatDate(pago.NextDueDate)),
		"active":      boolAttr(pago.Active),
		"type":        stringAttr(pago.Type),
		"startDate":   stringAttr(domainRecurring.FormatDate(pago.StartDate)),
		"createdAt":   stringAttr(iso),
		"updatedAt":   stringAttr(iso),
		"version":     numberAttr(pago.Version),
	}
	if pago.DueDay != nil {
		item["dueDay"] = numberAttr(int64(*pago.DueDay))
	}
	return item
}

// recurringFromItem interpreta un ítem como regla recurrente del contrato vigente.
//
// Conviven dos formas en la misma clave `USER#<id>/RECURRING#<id>`:
//
//  1. **Ítem del contrato (E033)**: guarda `name`, `amountCents`, `currency`, `frequency`,
//     `nextDueDate`, `active`, `type`, `startDate` y `version`. Se emite tal cual, con la
//     frecuencia NORMALIZADA (un valor histórico `biweekly`/`yearly` se responde como
//     `fortnightly`/`annual` sin reescribir el ítem, RFC-021 §3).
//
//  2. **Ítem heredado del alta de E001** (`D-E032-01`, decisión del orquestador): el alta
//     escribe `concept`, `amountCents`, `frequency`, `category`, `expenseType`, `createdAt`
//     y `version`, pero NO guarda programación. No se excluye ni se inventa una fecha: se
//     proyecta de forma DETERMINISTA:
//
//     - `id`         <- `recurringId`
//     - `name`       <- `concept` (o `name` si convivieran)
//     - `amount`     <- `amountCents`
//     - `currency`   <- `currency` o `MXN` (el alta sólo admite MXN)
//     - `type`       <- `type` si ya existe; si no, `expenseType` (`fixed` en E001) se
//     interpreta como gasto, que es la única naturaleza que el alta admite.
//     - `version`    <- `version`
//     - `startDate`  <- fecha UTC de `createdAt`
//     - `nextDueDate`<- primera ocurrencia >= hoy calculada desde `startDate` con la
//     frecuencia normalizada (`domainRecurring.NextOccurrenceOnOrAfter`). Es una
//     PROYECCIÓN de lectura: no se escribe en el ítem.
//     - `active`     <- `active` si existe; si no, `true` (el alta no cancela reglas)
//     - `dueDay`     <- día de `nextDueDate` si cae en 1..28 y la frecuencia es de
//     calendario mensual; en otro caso `null`
//
//     E034 devuelve el ítem igual; E035 lo actualiza (la escritura AÑADE los atributos del
//     contrato y conserva `concept`/`expenseType`/`category`); E036 lo desactiva.
//
// Devuelve `false` —sin inventar nada— sólo cuando faltan los mínimos irrenunciables: tipo
// de entidad distinto, identificador, importe positivo, versión válida, frecuencia dentro de
// los nueve valores publicados o una fecha de inicio derivable.
func recurringFromItem(item map[string]types.AttributeValue, today time.Time) (*appRecurring.Payment, bool) {
	if len(item) == 0 {
		return nil, false
	}
	if entity := stringAttribute(item["entityType"]); entity != recurringEntityType {
		return nil, false
	}

	id := strings.TrimSpace(stringAttribute(item["recurringId"]))
	if id == "" {
		return nil, false
	}

	nombre := strings.TrimSpace(stringAttribute(item["name"]))
	if nombre == "" {
		// Alias de lectura del alta de E001.
		nombre = strings.TrimSpace(stringAttribute(item["concept"]))
	}
	if nombre == "" {
		return nil, false
	}

	importe, ok := attributeInt(item["amountCents"])
	if !ok || importe <= 0 {
		return nil, false
	}
	version, ok := attributeInt(item["version"])
	if !ok || version < 1 {
		return nil, false
	}
	frecuencia, errFrecuencia := domainRecurring.ParseFrequency(stringAttribute(item["frequency"]))
	if errFrecuencia != nil {
		return nil, false
	}

	divisa := strings.TrimSpace(stringAttribute(item["currency"]))
	if divisa == "" {
		divisa = domainRecurring.CurrencyMXN
	}

	proximo, errProximo := domainRecurring.ParseDate(stringAttribute(item["nextDueDate"]))
	inicio, errInicio := domainRecurring.ParseDate(stringAttribute(item["startDate"]))
	tipo, errTipo := domainRecurring.ParseType(stringAttribute(item["type"]))
	activo, tieneActivo := boolAttributeOK(item["active"])

	if errProximo == nil && errInicio == nil && errTipo == nil && tieneActivo {
		var dueDay *int
		if valor, ok := attributeInt(item["dueDay"]); ok {
			dia := int(valor)
			dueDay = &dia
		}
		return &appRecurring.Payment{
			ID:          id,
			Name:        nombre,
			AmountCents: importe,
			Currency:    divisa,
			Frequency:   frecuencia,
			NextDueDate: proximo,
			Active:      activo,
			Version:     version,
			Type:        tipo,
			StartDate:   inicio,
			DueDay:      dueDay,
		}, true
	}

	// Forma heredada del alta (D-E032-01).
	inicio, ok = recurringLegacyStartDate(item)
	if !ok {
		return nil, false
	}
	proximo, errProximo = domainRecurring.NextOccurrenceOnOrAfter(inicio, frecuencia, today)
	if errProximo != nil {
		return nil, false
	}

	tipoHeredado := tipo
	if errTipo != nil {
		tipoHeredado, errTipo = domainRecurring.ParseType(stringAttribute(item["expenseType"]))
		if errTipo != nil {
			// El alta sólo admite compromisos fijos: su naturaleza es gasto.
			tipoHeredado = domainRecurring.TypeExpense
		}
	}

	activoHeredado := true
	if tieneActivo {
		activoHeredado = activo
	}

	var dueDay *int
	if _, errMes := domainRecurring.PeriodsMonths(frecuencia); errMes == nil {
		if dia := proximo.Day(); dia >= domainRecurring.MinDueDay && dia <= domainRecurring.MaxDueDay {
			valor := dia
			dueDay = &valor
		}
	}

	return &appRecurring.Payment{
		ID:          id,
		Name:        nombre,
		AmountCents: importe,
		Currency:    divisa,
		Frequency:   frecuencia,
		NextDueDate: proximo,
		Active:      activoHeredado,
		Version:     version,
		Type:        tipoHeredado,
		StartDate:   inicio,
		DueDay:      dueDay,
	}, true
}

// recurringLegacyStartDate deriva la fecha de inicio de un ítem heredado a partir de
// `createdAt` (RFC3339). Sin esa marca no hay ancla de calendario y el ítem no se expone.
func recurringLegacyStartDate(item map[string]types.AttributeValue) (time.Time, bool) {
	creado, ok := parseSessionTime(stringAttribute(item["createdAt"]))
	if !ok {
		return time.Time{}, false
	}
	return domainRecurring.DateOnly(creado), true
}

// boolAttributeOK lee un atributo BOOL e indica si estaba presente. Un atributo ausente no se
// interpreta como `false`: `active` es obligatorio en el contrato y su ausencia distingue el
// ítem heredado del ítem ya migrado.
func boolAttributeOK(value types.AttributeValue) (bool, bool) {
	booleano, ok := value.(*types.AttributeValueMemberBOOL)
	if !ok {
		return false, false
	}
	return booleano.Value, true
}
