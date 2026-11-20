package dynamo

// Pruebas del adaptador de E037 sobre un doble CON ESTADO de la tabla única.
//
// El doble interpreta las condiciones que el adaptador emite realmente (`attribute_not_exists`,
// expiración del registro de idempotencia y `ConditionCheck` de la regla activa) y aplica la
// transacción de forma atómica: si una condición falla, no escribe ninguna de las cinco acciones.
// Una expresión que el doble no sepa interpretar produce un error, no un falso positivo: el
// adaptador la traduce a ErrUnavailable y la prueba falla de forma visible.
//
// No se usa el SDK real ni red: la autoridad de la verificación en AWS es el CI Linux y, más
// adelante, la certificación HTTPS del Gate de la oleada.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	appMovements "github.com/nexumpr/Nexum-Backend/internal/application/movements"
	appRecurring "github.com/nexumpr/Nexum-Backend/internal/application/recurring"
	domainDynamo "github.com/nexumpr/Nexum-Backend/internal/domain/dynamo"
)

const (
	e037Usuario     = "usuario-e037-0001"
	e037Regla       = "018f90ef-08e5-7a68-b09f-2b67a47c92f0"
	e037Movimiento  = "018f90ef-08e5-7a68-b09f-2b67a47c92f1"
	e037MovimientoB = "018f90ef-08e5-7a68-b09f-2b67a47c92f2"
	e037Fecha       = "2026-11-05"
	e037Clave       = "clave-idempotencia-e037-0001"
	e037ClaveB      = "clave-idempotencia-e037-0002"
)

var e037Ahora = time.Date(2026, 11, 5, 18, 0, 0, 0, time.UTC)

// pagoDynamoFalso es un doble con estado y concurrencia segura de la tabla única.
type pagoDynamoFalso struct {
	mu            sync.Mutex
	items         map[string]map[string]types.AttributeValue
	transacciones int
	falloRed      error
}

func nuevoPagoDynamoFalso() *pagoDynamoFalso {
	return &pagoDynamoFalso{items: map[string]map[string]types.AttributeValue{}}
}

// claveFila compone la clave de mapa equivalente a (PK, SK).
func claveFila(pk, sk string) string { return pk + "\x00" + sk }

// poner inserta o reemplaza un ítem directamente (fixture).
func (f *pagoDynamoFalso) poner(item map[string]types.AttributeValue) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.items[claveFila(stringAttribute(item["PK"]), stringAttribute(item["SK"]))] = copiarItem(item)
}

// item devuelve una copia del ítem almacenado o nil.
func (f *pagoDynamoFalso) item(pk, sk string) map[string]types.AttributeValue {
	f.mu.Lock()
	defer f.mu.Unlock()
	return copiarItem(f.items[claveFila(pk, sk)])
}

// contarTransacciones informa cuántas transacciones se intentaron.
func (f *pagoDynamoFalso) contarTransacciones() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.transacciones
}

// GetItem devuelve una copia: el adaptador nunca puede modificar el estado del doble.
func (f *pagoDynamoFalso) GetItem(_ context.Context, in *dynamodb.GetItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.falloRed != nil {
		return nil, f.falloRed
	}
	pk := stringAttribute(in.Key["PK"])
	sk := stringAttribute(in.Key["SK"])
	return &dynamodb.GetItemOutput{Item: copiarItem(f.items[claveFila(pk, sk)])}, nil
}

// TransactWriteItems evalúa todas las condiciones antes de escribir y aplica el conjunto de forma
// atómica, como el servicio real.
func (f *pagoDynamoFalso) TransactWriteItems(_ context.Context, in *dynamodb.TransactWriteItemsInput, _ ...func(*dynamodb.Options)) (*dynamodb.TransactWriteItemsOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.falloRed != nil {
		return nil, f.falloRed
	}
	f.transacciones++

	razones := make([]types.CancellationReason, 0, len(in.TransactItems))
	fallo := false
	for _, accion := range in.TransactItems {
		cumple, err := f.condicionSeCumple(accion)
		if err != nil {
			// Expresión no soportada por el doble: fallo visible, nunca un falso positivo.
			return nil, err
		}
		if cumple {
			razones = append(razones, types.CancellationReason{Code: aws.String("None")})
			continue
		}
		razones = append(razones, types.CancellationReason{Code: aws.String("ConditionalCheckFailed")})
		fallo = true
	}
	if fallo {
		return nil, &types.TransactionCanceledException{
			Message:             aws.String("Transaction cancelled"),
			CancellationReasons: razones,
		}
	}

	for _, accion := range in.TransactItems {
		switch {
		case accion.Put != nil:
			item := copiarItem(accion.Put.Item)
			f.items[claveFila(stringAttribute(item["PK"]), stringAttribute(item["SK"]))] = item
		case accion.ConditionCheck != nil:
			// No escribe: sólo valida.
		default:
			return nil, fmt.Errorf("accion no soportada por el doble de E037")
		}
	}
	return &dynamodb.TransactWriteItemsOutput{}, nil
}

// condicionSeCumple interpreta exactamente las condiciones que emite el adaptador.
func (f *pagoDynamoFalso) condicionSeCumple(accion types.TransactWriteItem) (bool, error) {
	switch {
	case accion.Put != nil:
		existente := f.items[claveFila(stringAttribute(accion.Put.Item["PK"]), stringAttribute(accion.Put.Item["SK"]))]
		return evaluarCondicionE037(existente, aws.ToString(accion.Put.ConditionExpression), accion.Put.ExpressionAttributeNames, accion.Put.ExpressionAttributeValues)
	case accion.ConditionCheck != nil:
		existente := f.items[claveFila(stringAttribute(accion.ConditionCheck.Key["PK"]), stringAttribute(accion.ConditionCheck.Key["SK"]))]
		return evaluarCondicionE037(existente, aws.ToString(accion.ConditionCheck.ConditionExpression), accion.ConditionCheck.ExpressionAttributeNames, accion.ConditionCheck.ExpressionAttributeValues)
	default:
		return false, fmt.Errorf("accion no soportada por el doble de E037")
	}
}

// evaluarCondicionE037 soporta las tres formas usadas por el adaptador. Cualquier otra expresión
// es un error explícito.
func evaluarCondicionE037(item map[string]types.AttributeValue, expresion string, nombres map[string]string, valores map[string]types.AttributeValue) (bool, error) {
	switch expresion {
	case "attribute_not_exists(PK)":
		_, existe := item["PK"]
		return !existe, nil
	case "attribute_not_exists(PK) OR expiresAt < :now":
		if _, existe := item["PK"]; !existe {
			return true, nil
		}
		expira, ok := attributeInt(item["expiresAt"])
		if !ok {
			return false, nil
		}
		ahora, ok := attributeInt(valores[":now"])
		if !ok {
			return false, fmt.Errorf("condicion sin :now utilizable")
		}
		return expira < ahora, nil
	case "attribute_exists(PK) AND (attribute_not_exists(#activo) OR #activo = :activo)":
		if _, existe := item["PK"]; !existe {
			return false, nil
		}
		nombre := ""
		if nombres != nil {
			nombre = nombres["#activo"]
		}
		if nombre == "" {
			return false, fmt.Errorf("condicion sin alias #activo")
		}
		valor, existe := item[nombre]
		if !existe {
			return true, nil
		}
		actual, ok := valor.(*types.AttributeValueMemberBOOL)
		if !ok {
			return false, nil
		}
		esperado, ok := valores[":activo"].(*types.AttributeValueMemberBOOL)
		if !ok {
			return false, fmt.Errorf("condicion sin :activo booleano")
		}
		return actual.Value == esperado.Value, nil
	default:
		return false, fmt.Errorf("condicion no soportada por el doble de E037: %q", expresion)
	}
}

// copiarItem duplica el mapa para que nadie comparta memoria con el estado del doble.
func copiarItem(item map[string]types.AttributeValue) map[string]types.AttributeValue {
	if len(item) == 0 {
		return nil
	}
	copia := make(map[string]types.AttributeValue, len(item))
	for clave, valor := range item {
		copia[clave] = valor
	}
	return copia
}

// --- fixtures ---

// reglaE037Fixture construye el ítem de regla del contrato vigente.
func reglaE037Fixture(activa bool) map[string]types.AttributeValue {
	pk, sk := domainDynamo.RecurringKeys(e037Usuario, e037Regla)
	return map[string]types.AttributeValue{
		"PK":          stringAttr(pk),
		"SK":          stringAttr(sk),
		"entityType":  stringAttr("RECURRING_PAYMENT"),
		"userId":      stringAttr(e037Usuario),
		"recurringId": stringAttr(e037Regla),
		"name":        stringAttr("Renta"),
		"amountCents": numberAttr(1_000_00),
		"currency":    stringAttr("MXN"),
		"frequency":   stringAttr("monthly"),
		"nextDueDate": stringAttr("2026-11-05"),
		"active":      boolAttr(activa),
		"type":        stringAttr("expense"),
		"startDate":   stringAttr("2026-10-05"),
		"createdAt":   stringAttr("2026-10-01T00:00:00Z"),
		"updatedAt":   stringAttr("2026-10-01T00:00:00Z"),
		"version":     numberAttr(1),
	}
}

// preferenciasE037Fixture construye el ítem de preferencias con su zona.
func preferenciasE037Fixture(zona string) map[string]types.AttributeValue {
	return map[string]types.AttributeValue{
		"PK":         stringAttr(domainDynamo.PrefixUser + e037Usuario),
		"SK":         stringAttr(preferencesSortKey),
		"entityType": stringAttr(preferencesEntityType),
		"userId":     stringAttr(e037Usuario),
		"locale":     stringAttr("es-MX"),
		"timezone":   stringAttr(zona),
		"version":    numberAttr(1),
	}
}

// movimientoE037Fixture construye el movimiento derivado que el servicio entregaría al adaptador.
func movimientoE037Fixture(id string, ocurrido time.Time) appMovements.Movement {
	return appMovements.Movement{
		ID:                 id,
		Type:               appMovements.TypeExpense,
		AmountCents:        1_000_00,
		Currency:           appMovements.CurrencyMXN,
		CategoryID:         "otros",
		Description:        "Renta",
		OccurredAt:         ocurrido.UTC().Truncate(time.Millisecond),
		Version:            1,
		RecurringPaymentID: e037Regla,
	}
}

// escrituraE037 compone la escritura completa con el cuerpo canónico del movimiento.
func escrituraE037(t *testing.T, id, clave, hash, localDate string, ocurrido time.Time) appRecurring.PayWrite {
	t.Helper()
	movimiento := movimientoE037Fixture(id, ocurrido)
	cuerpo, err := appMovements.MarshalMovement(movimiento)
	if err != nil {
		t.Fatalf("no se pudo serializar el movimiento: %v", err)
	}
	return appRecurring.PayWrite{
		UserID:         e037Usuario,
		RuleID:         e037Regla,
		Movement:       movimiento,
		IdempotencyKey: clave,
		Operation:      appRecurring.OperationPayRecurring,
		PayloadHash:    hash,
		ResponseBody:   cuerpo,
		Status:         appRecurring.PayStatusOK,
		Now:            e037Ahora,
		ExpiresAt:      e037Ahora.Add(24 * time.Hour),
		LocalDate:      localDate,
	}
}

// --- pruebas ---

// TestE037PayPersisteMovimientoYLocalizador cubre la transacción completa: movimiento, puntero,
// localizador e idempotencia en la partición del titular.
func TestE037PayPersisteMovimientoYLocalizador(t *testing.T) {
	doble := nuevoPagoDynamoFalso()
	doble.poner(reglaE037Fixture(true))
	store := NewRecurringPaymentStore(doble, "NEXUM_CORE_TEST")

	ocurrido := time.Date(2026, 11, 5, 23, 30, 0, 0, time.UTC)
	resultado, err := store.Pay(context.Background(), escrituraE037(t, e037Movimiento, e037Clave, "hash-e037-1", e037Fecha, ocurrido))
	if err != nil {
		t.Fatalf("el pago valido no debe fallar: %v", err)
	}
	if resultado.Status != appRecurring.PayStatusOK || resultado.Replayed {
		t.Fatalf("resultado inesperado: %+v", resultado)
	}

	// Movimiento: vínculo, categoría interna, importe exacto y GSI1 para la analítica.
	pk, sk, gsi1pk, gsi1sk := domainDynamo.MovementKeys(e037Usuario, e037Movimiento, appMovements.TypeExpense, ocurrido)
	item := doble.item(pk, sk)
	if item == nil {
		t.Fatal("el movimiento debe estar persistido")
	}
	if got := stringAttribute(item["recurringPaymentId"]); got != e037Regla {
		t.Fatalf("recurringPaymentId inesperado: %q", got)
	}
	if _, existe := item["contributionId"]; existe {
		t.Fatal("un pago recurrente no declara aportacion: contributionId no debe escribirse")
	}
	if got := stringAttribute(item["categoryId"]); got != "otros" {
		t.Fatalf("categoryId inesperado: %q", got)
	}
	if centavos, _ := attributeInt(item["amountCents"]); centavos != 1_000_00 {
		t.Fatalf("importe persistido inesperado: %d", centavos)
	}
	if got := stringAttribute(item["GSI1PK"]); got != gsi1pk || stringAttribute(item["GSI1SK"]) != gsi1sk {
		t.Fatalf("claves de indice inesperadas: %q / %q", stringAttribute(item["GSI1PK"]), stringAttribute(item["GSI1SK"]))
	}
	if centavos, _ := attributeInt(item["version"]); centavos != 1 {
		t.Fatalf("version inicial inesperada: %d", centavos)
	}

	// Puntero O(1): apunta a la SK real del movimiento.
	ptrPK, ptrSK := MovementPointerKeys(e037Usuario, e037Movimiento)
	puntero := doble.item(ptrPK, ptrSK)
	if puntero == nil || stringAttribute(puntero["targetSK"]) != sk {
		t.Fatalf("puntero invalido: %+v", puntero)
	}

	// Localizador de ocurrencia por fecha civil local.
	locPK, locSK := RecurringPaymentLocatorKeys(e037Usuario, e037Regla, e037Fecha)
	localizador := doble.item(locPK, locSK)
	if localizador == nil {
		t.Fatal("el localizador de la ocurrencia debe existir")
	}
	if got := stringAttribute(localizador["movementId"]); got != e037Movimiento {
		t.Fatalf("movementId del localizador inesperado: %q", got)
	}
	if got := stringAttribute(localizador["localDate"]); got != e037Fecha {
		t.Fatalf("localDate del localizador inesperado: %q", got)
	}

	// Registro de idempotencia con cuerpo, hash y vencimiento lógico.
	idemPK, idemSK := domainDynamo.UserIdempotencyKeys(e037Usuario, appRecurring.OperationPayRecurring, e037Clave)
	idem := doble.item(idemPK, idemSK)
	if idem == nil {
		t.Fatal("el registro de idempotencia debe existir")
	}
	if got := stringAttribute(idem["payloadHash"]); got != "hash-e037-1" {
		t.Fatalf("payloadHash inesperado: %q", got)
	}
	if got := stringAttribute(idem["responseBody"]); got != string(resultado.Body) {
		t.Fatalf("el cuerpo persistido debe ser identico al devuelto")
	}
	if expira, _ := attributeInt(idem["expiresAt"]); expira != e037Ahora.Add(24*time.Hour).Unix() {
		t.Fatalf("expiresAt inesperado: %d", expira)
	}
	if got := stringAttribute(idem["location"]); got != "/api/v1/movements/"+e037Movimiento {
		t.Fatalf("location inesperada: %q", got)
	}
	if !strings.HasPrefix(idemSK, domainDynamo.PrefixIdempotency+appRecurring.OperationPayRecurring+"#") {
		t.Fatalf("SK de idempotencia fuera de la operacion de E037: %q", idemSK)
	}
	if strings.Contains(idemSK, e037Clave) {
		t.Fatal("la clave cruda del cliente no puede persistirse en la SK")
	}
}

// TestE037PayReplayMismaClave devuelve el cuerpo original sin escribir de nuevo.
func TestE037PayReplayMismaClave(t *testing.T) {
	doble := nuevoPagoDynamoFalso()
	doble.poner(reglaE037Fixture(true))
	store := NewRecurringPaymentStore(doble, "NEXUM_CORE_TEST")
	ocurrido := time.Date(2026, 11, 5, 23, 30, 0, 0, time.UTC)

	primera, err := store.Pay(context.Background(), escrituraE037(t, e037Movimiento, e037Clave, "hash-e037-1", e037Fecha, ocurrido))
	if err != nil {
		t.Fatalf("primer pago: %v", err)
	}
	transacciones := doble.contarTransacciones()

	segunda, err := store.Pay(context.Background(), escrituraE037(t, e037MovimientoB, e037Clave, "hash-e037-1", e037Fecha, ocurrido))
	if err != nil {
		t.Fatalf("el replay no debe fallar: %v", err)
	}
	if !segunda.Replayed {
		t.Fatal("el segundo intento con la misma clave debe declararse replay")
	}
	if string(segunda.Body) != string(primera.Body) {
		t.Fatalf("el replay debe devolver bytes identicos:\n%s\n%s", primera.Body, segunda.Body)
	}
	if doble.contarTransacciones() != transacciones {
		t.Fatal("el replay no debe abrir una transaccion")
	}
	// El segundo identificador no llegó a escribirse: no hay movimiento duplicado.
	pk, sk, _, _ := domainDynamo.MovementKeys(e037Usuario, e037MovimientoB, appMovements.TypeExpense, ocurrido)
	if doble.item(pk, sk) != nil {
		t.Fatal("el replay no puede crear un segundo movimiento")
	}
}

// TestE037PayMismaClaveDistintoPayloadEsConflicto devuelve 409 sin escribir.
func TestE037PayMismaClaveDistintoPayloadEsConflicto(t *testing.T) {
	doble := nuevoPagoDynamoFalso()
	doble.poner(reglaE037Fixture(true))
	store := NewRecurringPaymentStore(doble, "NEXUM_CORE_TEST")
	ocurrido := time.Date(2026, 11, 5, 23, 30, 0, 0, time.UTC)

	if _, err := store.Pay(context.Background(), escrituraE037(t, e037Movimiento, e037Clave, "hash-e037-1", e037Fecha, ocurrido)); err != nil {
		t.Fatalf("primer pago: %v", err)
	}
	_, err := store.Pay(context.Background(), escrituraE037(t, e037MovimientoB, e037Clave, "hash-e037-2", e037Fecha, ocurrido))
	if !errors.Is(err, appRecurring.ErrConflict) {
		t.Fatalf("se esperaba ErrConflict, se obtuvo %v", err)
	}
}

// TestE037PayDuplicadoMismaFechaOtraClave bloquea un segundo pago de la misma regla ese día.
func TestE037PayDuplicadoMismaFechaOtraClave(t *testing.T) {
	doble := nuevoPagoDynamoFalso()
	doble.poner(reglaE037Fixture(true))
	store := NewRecurringPaymentStore(doble, "NEXUM_CORE_TEST")
	ocurrido := time.Date(2026, 11, 5, 23, 30, 0, 0, time.UTC)

	if _, err := store.Pay(context.Background(), escrituraE037(t, e037Movimiento, e037Clave, "hash-e037-1", e037Fecha, ocurrido)); err != nil {
		t.Fatalf("primer pago: %v", err)
	}
	_, err := store.Pay(context.Background(), escrituraE037(t, e037MovimientoB, e037ClaveB, "hash-e037-2", e037Fecha, ocurrido.Add(time.Hour)))
	if !errors.Is(err, appRecurring.ErrDuplicatePayment) {
		t.Fatalf("se esperaba ErrDuplicatePayment, se obtuvo %v", err)
	}
	pk, sk, _, _ := domainDynamo.MovementKeys(e037Usuario, e037MovimientoB, appMovements.TypeExpense, ocurrido.Add(time.Hour))
	if doble.item(pk, sk) != nil {
		t.Fatal("el duplicado no puede crear un segundo movimiento")
	}
}

// TestE037PayReglaInexistenteEInactiva distingue 404 de 409 sin revelar recursos ajenos.
func TestE037PayReglaInexistenteEInactiva(t *testing.T) {
	ocurrido := time.Date(2026, 11, 5, 23, 30, 0, 0, time.UTC)

	t.Run("inexistente", func(t *testing.T) {
		doble := nuevoPagoDynamoFalso()
		store := NewRecurringPaymentStore(doble, "NEXUM_CORE_TEST")
		_, err := store.Pay(context.Background(), escrituraE037(t, e037Movimiento, e037Clave, "hash-e037-1", e037Fecha, ocurrido))
		if !errors.Is(err, appRecurring.ErrNotFound) {
			t.Fatalf("se esperaba ErrNotFound, se obtuvo %v", err)
		}
	})

	t.Run("inactiva", func(t *testing.T) {
		doble := nuevoPagoDynamoFalso()
		doble.poner(reglaE037Fixture(false))
		store := NewRecurringPaymentStore(doble, "NEXUM_CORE_TEST")
		_, err := store.Pay(context.Background(), escrituraE037(t, e037Movimiento, e037Clave, "hash-e037-1", e037Fecha, ocurrido))
		if !errors.Is(err, appRecurring.ErrRuleInactive) {
			t.Fatalf("se esperaba ErrRuleInactive, se obtuvo %v", err)
		}
	})

	t.Run("regla heredada sin active", func(t *testing.T) {
		doble := nuevoPagoDynamoFalso()
		pk, sk := domainDynamo.RecurringKeys(e037Usuario, e037Regla)
		doble.poner(map[string]types.AttributeValue{
			"PK":          stringAttr(pk),
			"SK":          stringAttr(sk),
			"entityType":  stringAttr("RECURRING_PAYMENT"),
			"userId":      stringAttr(e037Usuario),
			"recurringId": stringAttr(e037Regla),
			"concept":     stringAttr("Renta"),
			"amountCents": numberAttr(1_000_00),
			"currency":    stringAttr("MXN"),
			"frequency":   stringAttr("monthly"),
			"createdAt":   stringAttr("2026-10-01T00:00:00Z"),
			"version":     numberAttr(1),
		})
		store := NewRecurringPaymentStore(doble, "NEXUM_CORE_TEST")
		if _, err := store.Pay(context.Background(), escrituraE037(t, e037Movimiento, e037Clave, "hash-e037-1", e037Fecha, ocurrido)); err != nil {
			t.Fatalf("un item heredado sin active debe poder pagarse: %v", err)
		}
	})
}

// TestE037PayColisionDeIdentificador informa la colisión para que el servicio reintente.
func TestE037PayColisionDeIdentificador(t *testing.T) {
	doble := nuevoPagoDynamoFalso()
	doble.poner(reglaE037Fixture(true))
	ptrPK, ptrSK := MovementPointerKeys(e037Usuario, e037Movimiento)
	doble.poner(map[string]types.AttributeValue{
		"PK":         stringAttr(ptrPK),
		"SK":         stringAttr(ptrSK),
		"entityType": stringAttr("MOVEMENT_LOCATOR"),
		"userId":     stringAttr(e037Usuario),
		"movementId": stringAttr(e037Movimiento),
		"targetSK":   stringAttr("MOVEMENT#2026-11-05T00:00:00.000Z#otro"),
	})
	store := NewRecurringPaymentStore(doble, "NEXUM_CORE_TEST")
	_, err := store.Pay(context.Background(), escrituraE037(t, e037Movimiento, e037Clave, "hash-e037-1", e037Fecha, time.Date(2026, 11, 5, 23, 30, 0, 0, time.UTC)))
	if !errors.Is(err, appRecurring.ErrMovementIDCollision) {
		t.Fatalf("se esperaba ErrMovementIDCollision, se obtuvo %v", err)
	}
}

// TestE037PayConcurrenciaSoloUnoGana: dos pagos simultáneos con claves distintas y la misma fecha
// local; la transacción condicional deja ganar sólo a uno.
func TestE037PayConcurrenciaSoloUnoGana(t *testing.T) {
	doble := nuevoPagoDynamoFalso()
	doble.poner(reglaE037Fixture(true))
	store := NewRecurringPaymentStore(doble, "NEXUM_CORE_TEST")
	ocurrido := time.Date(2026, 11, 5, 23, 30, 0, 0, time.UTC)

	type salida struct {
		id  string
		err error
	}
	escrituras := []appRecurring.PayWrite{
		escrituraE037(t, e037Movimiento, e037Clave, "hash-e037-1", e037Fecha, ocurrido),
		escrituraE037(t, e037MovimientoB, e037ClaveB, "hash-e037-2", e037Fecha, ocurrido),
	}
	canal := make(chan salida, 2)
	inicio := make(chan struct{})
	for _, escritura := range escrituras {
		escritura := escritura
		go func() {
			<-inicio
			_, err := store.Pay(context.Background(), escritura)
			canal <- salida{id: escritura.Movement.ID, err: err}
		}()
	}
	close(inicio)

	exitos, duplicados := 0, 0
	for i := 0; i < 2; i++ {
		resultado := <-canal
		switch {
		case resultado.err == nil:
			exitos++
		case errors.Is(resultado.err, appRecurring.ErrDuplicatePayment):
			duplicados++
		default:
			t.Fatalf("error inesperado en la carrera: %v", resultado.err)
		}
	}
	if exitos != 1 || duplicados != 1 {
		t.Fatalf("exactamente un pago debe ganar: exitos=%d duplicados=%d", exitos, duplicados)
	}

	// Sólo existe un movimiento para la fecha local.
	coincidencias := 0
	for _, id := range []string{e037Movimiento, e037MovimientoB} {
		pk, sk, _, _ := domainDynamo.MovementKeys(e037Usuario, id, appMovements.TypeExpense, ocurrido)
		if doble.item(pk, sk) != nil {
			coincidencias++
		}
	}
	if coincidencias != 1 {
		t.Fatalf("debe persistirse exactamente un movimiento, se encontraron %d", coincidencias)
	}
}

// TestE037LoadRuleYLoadTimezone comprueba las dos lecturas previas del puerto.
func TestE037LoadRuleYLoadTimezone(t *testing.T) {
	doble := nuevoPagoDynamoFalso()
	doble.poner(reglaE037Fixture(true))
	doble.poner(preferenciasE037Fixture("America/Mexico_City"))
	store := NewRecurringPaymentStore(doble, "NEXUM_CORE_TEST").WithClock(func() time.Time { return e037Ahora })

	regla, err := store.LoadRule(context.Background(), e037Usuario, e037Regla)
	if err != nil || regla == nil {
		t.Fatalf("la regla debe existir: regla=%v err=%v", regla, err)
	}
	if regla.Name != "Renta" || !regla.Active || regla.Type != appMovements.TypeExpense {
		t.Fatalf("proyeccion inesperada: %+v", regla)
	}

	zona, err := store.LoadTimezone(context.Background(), e037Usuario)
	if err != nil || zona != "America/Mexico_City" {
		t.Fatalf("zona inesperada: %q err=%v", zona, err)
	}

	ajena, err := store.LoadRule(context.Background(), "otro-usuario", e037Regla)
	if err != nil || ajena != nil {
		t.Fatalf("una regla de otra particion no existe: regla=%v err=%v", ajena, err)
	}

	sinPreferencias, err := store.LoadTimezone(context.Background(), "otro-usuario")
	if err != nil || sinPreferencias != "" {
		t.Fatalf("sin preferencias la zona debe ser vacia: %q err=%v", sinPreferencias, err)
	}
}

// TestE037PaySinPersistenciaFallaCerrado: sin cliente o sin tabla no se confirma nada.
func TestE037PaySinPersistenciaFallaCerrado(t *testing.T) {
	for _, store := range []*RecurringPaymentStore{
		NewRecurringPaymentStore(nil, ""),
		NewRecurringPaymentStore(nuevoPagoDynamoFalso(), ""),
	} {
		if _, err := store.Pay(context.Background(), escrituraE037(t, e037Movimiento, e037Clave, "hash", e037Fecha, e037Ahora)); !errors.Is(err, appRecurring.ErrUnavailable) {
			t.Fatalf("se esperaba ErrUnavailable, se obtuvo %v", err)
		}
		if _, err := store.LoadRule(context.Background(), e037Usuario, e037Regla); !errors.Is(err, appRecurring.ErrUnavailable) {
			t.Fatalf("LoadRule sin persistencia debe fallar cerrado: %v", err)
		}
	}
}
