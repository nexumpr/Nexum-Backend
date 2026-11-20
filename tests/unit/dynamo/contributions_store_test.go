package dynamo_test

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	dynamoAdapter "github.com/nexumpr/Nexum-Backend/internal/adapters/dynamo"
	appContrib "github.com/nexumpr/Nexum-Backend/internal/application/contributions"
	appMovements "github.com/nexumpr/Nexum-Backend/internal/application/movements"
	domainDynamo "github.com/nexumpr/Nexum-Backend/internal/domain/dynamo"
)

// Doble con estado para E043–E047. Interpreta las condiciones y actualizaciones EXACTAS que
// emite el adaptador; cualquier forma no soportada falla, de modo que una regresión en la
// transacción se detecta en las pruebas y no en AWS.

const (
	e04xTabla   = "NEXUM_CORE_TEST"
	e04xUsuario = "11111111-2222-4333-8444-555555555555"
	e04xMeta1   = "018f90ef-08e5-7a68-b09f-0000000000a1"
	e04xMeta2   = "018f90ef-08e5-7a68-b09f-0000000000a2"
	e04xAporte  = "018f90ef-08e5-7a68-b09f-0000000000c1"
	e04xMov     = "018f90ef-08e5-7a68-b09f-0000000000d1"
	e04xClave   = "idem-key-000000000001"
)

var e04xAhora = time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)

type memoriaAportacionesDynamo struct {
	mu            sync.Mutex
	items         map[string]map[string]types.AttributeValue
	transacciones int
	consultas     int
	fallo         error
}

func nuevaMemoriaAportaciones() *memoriaAportacionesDynamo {
	return &memoriaAportacionesDynamo{items: map[string]map[string]types.AttributeValue{}}
}

func (m *memoriaAportacionesDynamo) GetItem(_ context.Context, params *dynamodb.GetItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.fallo != nil {
		return nil, m.fallo
	}
	pk := mapaString(params.Key, "PK")
	sk := mapaString(params.Key, "SK")
	item, ok := m.items[claveDe(pk, sk)]
	if !ok {
		return &dynamodb.GetItemOutput{}, nil
	}
	return &dynamodb.GetItemOutput{Item: copiarItem(item)}, nil
}

func (m *memoriaAportacionesDynamo) Query(_ context.Context, params *dynamodb.QueryInput, _ ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.consultas++
	if m.fallo != nil {
		return nil, m.fallo
	}
	condicion := aws.ToString(params.KeyConditionExpression)
	pk := textoAporte(params.ExpressionAttributeValues[":pk"])
	type entrada struct {
		sk   string
		item map[string]types.AttributeValue
	}
	candidatos := make([]entrada, 0)
	for _, item := range m.items {
		if mapaString(item, "PK") != pk {
			continue
		}
		sk := mapaString(item, "SK")
		switch {
		case strings.Contains(condicion, "begins_with(#sk, :prefix)"):
			prefijo := textoAporte(params.ExpressionAttributeValues[":prefix"])
			if !strings.HasPrefix(sk, prefijo) {
				continue
			}
		case strings.Contains(condicion, "#sk BETWEEN :from AND :to"):
			desde := textoAporte(params.ExpressionAttributeValues[":from"])
			hasta := textoAporte(params.ExpressionAttributeValues[":to"])
			if sk < desde || sk > hasta {
				continue
			}
		default:
			return nil, errors.New("condicion de clave no soportada: " + condicion)
		}
		candidatos = append(candidatos, entrada{sk: sk, item: copiarItem(item)})
	}
	sort.Slice(candidatos, func(i, j int) bool { return candidatos[i].sk < candidatos[j].sk })
	if params.ScanIndexForward != nil && !aws.ToBool(params.ScanIndexForward) {
		for i, j := 0, len(candidatos)-1; i < j; i, j = i+1, j-1 {
			candidatos[i], candidatos[j] = candidatos[j], candidatos[i]
		}
	}
	inicio := 0
	if len(params.ExclusiveStartKey) > 0 {
		startSK := mapaString(params.ExclusiveStartKey, "SK")
		descendente := params.ScanIndexForward != nil && !aws.ToBool(params.ScanIndexForward)
		for inicio < len(candidatos) {
			if descendente {
				if candidatos[inicio].sk < startSK {
					break
				}
			} else if candidatos[inicio].sk > startSK {
				break
			}
			inicio++
		}
	}
	limite := len(candidatos) - inicio
	if params.Limit != nil && int(aws.ToInt32(params.Limit)) < limite {
		limite = int(aws.ToInt32(params.Limit))
	}
	fin := inicio + limite
	var items []map[string]types.AttributeValue
	filtro := aws.ToString(params.FilterExpression)
	for _, c := range candidatos[inicio:fin] {
		if filtro != "" && !cumpleFiltroAporte(filtro, c.item, params.ExpressionAttributeValues) {
			continue
		}
		items = append(items, c.item)
	}
	salida := &dynamodb.QueryOutput{Items: items, Count: int32(len(items)), ScannedCount: int32(fin - inicio)}
	if fin > inicio && fin < len(candidatos) {
		salida.LastEvaluatedKey = map[string]types.AttributeValue{"PK": strAttr(pk), "SK": strAttr(candidatos[fin-1].sk)}
	}
	return salida, nil
}

func cumpleFiltroAporte(filtro string, item map[string]types.AttributeValue, valores map[string]types.AttributeValue) bool {
	if strings.Contains(filtro, "contains(goalIds, :goal)") {
		buscado := textoAporte(valores[":goal"])
		encontrado := false
		if ss, ok := item["goalIds"].(*types.AttributeValueMemberSS); ok {
			for _, v := range ss.Value {
				if v == buscado {
					encontrado = true
					break
				}
			}
		}
		if !encontrado {
			return false
		}
	}
	if strings.Contains(filtro, "#status = :status") {
		if mapaString(item, "status") != textoAporte(valores[":status"]) {
			return false
		}
	}
	return true
}

func (m *memoriaAportacionesDynamo) TransactWriteItems(_ context.Context, params *dynamodb.TransactWriteItemsInput, _ ...func(*dynamodb.Options)) (*dynamodb.TransactWriteItemsOutput, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.transacciones++
	if m.fallo != nil {
		return nil, m.fallo
	}
	// Fase 1: condiciones.
	razones := make([]types.CancellationReason, len(params.TransactItems))
	hayFallo := false
	for i, accion := range params.TransactItems {
		razones[i] = types.CancellationReason{Code: aws.String("None")}
		var fallo bool
		var err error
		if accion.Put != nil {
			fallo, err = !m.comprobarPut(accion.Put), nil
		} else if accion.Update != nil {
			fallo, err = m.comprobarUpdate(accion.Update)
		} else {
			err = errors.New("accion no soportada")
		}
		if err != nil {
			return nil, err
		}
		if fallo {
			razones[i] = types.CancellationReason{Code: aws.String("ConditionalCheckFailed")}
			hayFallo = true
		}
	}
	if hayFallo {
		return nil, &types.TransactionCanceledException{Message: aws.String("Transaction cancelled"), CancellationReasons: razones}
	}
	// Fase 2: aplicar.
	for _, accion := range params.TransactItems {
		if accion.Put != nil {
			pk := mapaString(accion.Put.Item, "PK")
			sk := mapaString(accion.Put.Item, "SK")
			m.items[claveDe(pk, sk)] = copiarItem(accion.Put.Item)
			continue
		}
		if accion.Update != nil {
			pk := mapaString(accion.Update.Key, "PK")
			sk := mapaString(accion.Update.Key, "SK")
			item := m.items[claveDe(pk, sk)]
			m.aplicarUpdate(item, accion.Update)
			m.items[claveDe(pk, sk)] = item
		}
	}
	return &dynamodb.TransactWriteItemsOutput{}, nil
}

func (m *memoriaAportacionesDynamo) comprobarPut(put *types.Put) bool {
	pk := mapaString(put.Item, "PK")
	sk := mapaString(put.Item, "SK")
	existente, existe := m.items[claveDe(pk, sk)]
	condicion := aws.ToString(put.ConditionExpression)
	switch {
	case condicion == "":
		return true
	case condicion == "attribute_not_exists(PK)":
		return !existe
	case strings.Contains(condicion, "expiresAt < :now"):
		if !existe {
			return true
		}
		ahora := numeroAporte(put.ExpressionAttributeValues[":now"])
		return numeroAporte(existente["expiresAt"]) < ahora
	default:
		return false
	}
}

func (m *memoriaAportacionesDynamo) comprobarUpdate(update *types.Update) (bool, error) {
	pk := mapaString(update.Key, "PK")
	sk := mapaString(update.Key, "SK")
	item, existe := m.items[claveDe(pk, sk)]
	condicion := aws.ToString(update.ConditionExpression)
	valores := update.ExpressionAttributeValues
	switch {
	case strings.Contains(condicion, "#saved <= :maxSaved"):
		if !existe || mapaString(item, "status") != textoAporte(valores[":active"]) {
			return true, nil
		}
		if deleted, ok := item["deleted"].(*types.AttributeValueMemberBOOL); ok && deleted.Value {
			return true, nil
		}
		if numeroAporte(item["savedAmountCents"]) > numeroAporte(valores[":maxSaved"]) {
			return true, nil
		}
		if numeroAporte(item["version"]) != numeroAporte(valores[":goalVersion"]) {
			return true, nil
		}
		return false, nil
	case strings.Contains(condicion, "#saved >= :alloc"):
		if !existe || mapaString(item, "status") != textoAporte(valores[":active"]) {
			return true, nil
		}
		if deleted, ok := item["deleted"].(*types.AttributeValueMemberBOOL); ok && deleted.Value {
			return true, nil
		}
		if numeroAporte(item["savedAmountCents"]) < numeroAporte(valores[":alloc"]) {
			return true, nil
		}
		if numeroAporte(item["activeContributions"]) < numeroAporte(valores[":one"]) {
			return true, nil
		}
		return false, nil
	case strings.Contains(condicion, "#status = :active AND #version = :expected"):
		if !existe {
			return true, nil
		}
		return mapaString(item, "status") != textoAporte(valores[":active"]) ||
			numeroAporte(item["version"]) != numeroAporte(valores[":expected"]), nil
	default:
		return false, errors.New("condicion de update no soportada: " + condicion)
	}
}

func (m *memoriaAportacionesDynamo) aplicarUpdate(item map[string]types.AttributeValue, update *types.Update) {
	expresion := aws.ToString(update.UpdateExpression)
	valores := update.ExpressionAttributeValues
	switch {
	case strings.Contains(expresion, "#saved = #saved + :alloc"):
		item["savedAmountCents"] = numberAttrAporte(numeroAporte(item["savedAmountCents"]) + numeroAporte(valores[":alloc"]))
		item["activeContributions"] = numberAttrAporte(numeroAporte(item["activeContributions"]) + 1)
		item["version"] = numberAttrAporte(numeroAporte(item["version"]) + 1)
		item["updatedAt"] = valores[":updatedAt"]
	case strings.Contains(expresion, "#saved = #saved - :alloc"):
		item["savedAmountCents"] = numberAttrAporte(numeroAporte(item["savedAmountCents"]) - numeroAporte(valores[":alloc"]))
		item["activeContributions"] = numberAttrAporte(numeroAporte(item["activeContributions"]) - 1)
		item["version"] = numberAttrAporte(numeroAporte(item["version"]) + 1)
		item["updatedAt"] = valores[":now"]
	case strings.Contains(expresion, "#status = :reversed"):
		item["status"] = valores[":reversed"]
		item["reversedAt"] = valores[":now"]
		item["reverseReason"] = valores[":reason"]
		item["updatedAt"] = valores[":now"]
		item["version"] = numberAttrAporte(numeroAporte(item["version"]) + 1)
	default:
		panic("update no soportado: " + expresion)
	}
}

func textoAporte(valor types.AttributeValue) string {
	if v, ok := valor.(*types.AttributeValueMemberS); ok {
		return v.Value
	}
	return ""
}

func numeroAporte(valor types.AttributeValue) int64 {
	if v, ok := valor.(*types.AttributeValueMemberN); ok {
		n, _ := strconv.ParseInt(v.Value, 10, 64)
		return n
	}
	return 0
}

func numberAttrAporte(n int64) types.AttributeValue {
	return &types.AttributeValueMemberN{Value: strconv.FormatInt(n, 10)}
}

func metaItem(userID, goalID string, target, saved int64, status string) map[string]types.AttributeValue {
	pk, sk := domainDynamo.GoalKeys(userID, goalID)
	return map[string]types.AttributeValue{
		"PK": strAttr(pk), "SK": strAttr(sk), "entityType": strAttr("GOAL"),
		"userId": strAttr(userID), "goalId": strAttr(goalID),
		"targetAmountCents": numberAttrAporte(target), "savedAmountCents": numberAttrAporte(saved),
		"activeContributions": numberAttrAporte(0), "status": strAttr(status),
		"version": numberAttrAporte(1), "createdAt": strAttr(e04xAhora.Format(time.RFC3339)),
		"updatedAt": strAttr(e04xAhora.Format(time.RFC3339)),
	}
}

func escrituraAporteConIDs(t *testing.T, amountCents int64, allocations []appContrib.Allocation, contributionID, movementID string) appContrib.CreateWrite {
	t.Helper()
	c := appContrib.Contribution{
		ID: contributionID, UserID: e04xUsuario, AmountCents: amountCents, Currency: "MXN",
		Allocations: allocations, OccurredAt: e04xAhora, Status: appContrib.StatusActive,
		Version: 1, CreatedAt: e04xAhora, UpdatedAt: e04xAhora,
	}
	body, err := c.Body()
	if err != nil {
		t.Fatalf("body: %v", err)
	}
	goals := map[string]appContrib.GoalSnapshot{}
	for _, a := range allocations {
		goals[a.GoalID] = appContrib.GoalSnapshot{ID: a.GoalID, TargetCents: 100000, SavedCents: 0, Status: "active", Version: 1}
	}
	return appContrib.CreateWrite{
		UserID:         e04xUsuario,
		Contribution:   c,
		Goals:          goals,
		Movement:       appMovements.Movement{ID: movementID, Type: appMovements.TypeSaving, AmountCents: amountCents, Currency: "MXN", CategoryID: "goal", Description: "Aportacion a metas", OccurredAt: e04xAhora, Version: 1, ContributionID: contributionID, CreatedAt: e04xAhora},
		IdempotencyKey: e04xClave,
		Operation:      appContrib.OperationCreate,
		PayloadHash:    appContrib.PayloadHash(amountCents, "MXN", allocations, appMovements.FormatOccurredAt(e04xAhora)),
		ResponseBody:   body,
		Location:       "/api/v1/goal-contributions/" + contributionID,
		Now:            e04xAhora,
		ExpiresAt:      e04xAhora.Add(appContrib.IdempotencyTTL),
	}
}

func escrituraAporte(t *testing.T, amountCents int64, allocations []appContrib.Allocation) appContrib.CreateWrite {
	return escrituraAporteConIDs(t, amountCents, allocations, e04xAporte, e04xMov)
}

func TestE044StoreTransaccionAtomicaConMetas(t *testing.T) {
	memoria := nuevaMemoriaAportaciones()
	memoria.items[claveDe(mustGoalKeys(e04xUsuario, e04xMeta1))] = metaItem(e04xUsuario, e04xMeta1, 100000, 0, "active")
	store := dynamoAdapter.NewContributionsStore(memoria, e04xTabla)
	escritura := escrituraAporte(t, 100000, []appContrib.Allocation{{GoalID: e04xMeta1, AmountCents: 100000}})
	respuesta, err := store.CreateContribution(context.Background(), escritura)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if respuesta.Status != 201 || respuesta.Replayed {
		t.Fatalf("respuesta inesperada: %+v", respuesta)
	}
	meta, ok := memoria.items[claveDe(mustGoalKeys(e04xUsuario, e04xMeta1))]
	if !ok {
		t.Fatal("la meta debe existir")
	}
	if numeroAporte(meta["savedAmountCents"]) != 100000 || numeroAporte(meta["activeContributions"]) != 1 || numeroAporte(meta["version"]) != 2 {
		t.Fatalf("meta no actualizada: %+v", meta)
	}
	if len(memoria.items) != 6 {
		t.Fatalf("esperados 6 items (aporte, puntero, movimiento, puntero mov, idem, meta), hay %d", len(memoria.items))
	}
	segunda, err := store.CreateContribution(context.Background(), escritura)
	if err != nil || !segunda.Replayed {
		t.Fatalf("replay: %v/%v", err, segunda != nil && segunda.Replayed)
	}
	if len(memoria.items) != 6 {
		t.Fatalf("el replay no debe duplicar: %d", len(memoria.items))
	}
	otra := escritura
	otra.PayloadHash = strings.Repeat("f", 64)
	if _, err := store.CreateContribution(context.Background(), otra); !errors.Is(err, appContrib.ErrConflict) {
		t.Fatalf("esperado conflicto, obtenido %v", err)
	}
}

func TestE044StoreCapacidadExcedidaNoEscribe(t *testing.T) {
	memoria := nuevaMemoriaAportaciones()
	memoria.items[claveDe(mustGoalKeys(e04xUsuario, e04xMeta1))] = metaItem(e04xUsuario, e04xMeta1, 50000, 0, "active")
	store := dynamoAdapter.NewContributionsStore(memoria, e04xTabla)
	escritura := escrituraAporte(t, 60000, []appContrib.Allocation{{GoalID: e04xMeta1, AmountCents: 60000}})
	escritura.Goals[e04xMeta1] = appContrib.GoalSnapshot{ID: e04xMeta1, TargetCents: 50000, SavedCents: 0, Status: "active", Version: 1}
	_, err := store.CreateContribution(context.Background(), escritura)
	if !errors.Is(err, appContrib.ErrCapacityExceeded) {
		t.Fatalf("esperado capacidad, obtenido %v", err)
	}
	if len(memoria.items) != 1 {
		t.Fatalf("no debe escribir nada: %d items", len(memoria.items))
	}
}

func TestE045StoreListadoFiltrosYCursor(t *testing.T) {
	memoria := nuevaMemoriaAportaciones()
	memoria.items[claveDe(mustGoalKeys(e04xUsuario, e04xMeta1))] = metaItem(e04xUsuario, e04xMeta1, 100000, 0, "active")
	store := dynamoAdapter.NewContributionsStore(memoria, e04xTabla)
	for i, id := range []string{e04xAporte, "018f90ef-08e5-7a68-b09f-0000000000c2", "018f90ef-08e5-7a68-b09f-0000000000c3"} {
		allocations := []appContrib.Allocation{{GoalID: e04xMeta1, AmountCents: 1000}}
		escritura := escrituraAporteConIDs(t, 1000, allocations, id, fmt.Sprintf("018f90ef-08e5-7a68-b09f-0000000000d%d", i+2))
		escritura.Contribution.OccurredAt = e04xAhora.Add(time.Duration(i) * time.Hour)
		escritura.Movement.OccurredAt = escritura.Contribution.OccurredAt
		escritura.IdempotencyKey = fmt.Sprintf("idem-key-0000000000%02d", i+1)
		escritura.Goals[e04xMeta1] = appContrib.GoalSnapshot{ID: e04xMeta1, TargetCents: 100000, Status: "active", Version: int64(i + 1)}
		escritura.PayloadHash = appContrib.PayloadHash(1000, "MXN", allocations, appMovements.FormatOccurredAt(escritura.Contribution.OccurredAt))
		body, err := escritura.Contribution.Body()
		if err != nil {
			t.Fatalf("body: %v", err)
		}
		escritura.ResponseBody = body
		if _, err := store.CreateContribution(context.Background(), escritura); err != nil {
			t.Fatalf("seed %s: %v", id, err)
		}
	}
	p1, next, err := store.ListContributions(context.Background(), appContrib.ListQuery{UserID: e04xUsuario, Limit: 2}, "", 2)
	if err != nil {
		t.Fatalf("list 1: %v", err)
	}
	if len(p1) != 2 || next == "" {
		t.Fatalf("esperada pagina 2 + cursor, obtenido %d/%q", len(p1), next)
	}
	p2, next2, err := store.ListContributions(context.Background(), appContrib.ListQuery{UserID: e04xUsuario, Limit: 2}, next, 2)
	if err != nil {
		t.Fatalf("list 2: %v", err)
	}
	if len(p2) != 1 || next2 != "" {
		t.Fatalf("esperada pagina 1 sin cursor, obtenido %d/%q", len(p2), next2)
	}
	vistos := map[string]bool{}
	for _, c := range append(p1, p2...) {
		if vistos[c.ID] {
			t.Fatalf("duplicado %s", c.ID)
		}
		vistos[c.ID] = true
	}
}

func TestE046StoreLoadYAislamiento(t *testing.T) {
	memoria := nuevaMemoriaAportaciones()
	memoria.items[claveDe(mustGoalKeys(e04xUsuario, e04xMeta1))] = metaItem(e04xUsuario, e04xMeta1, 100000, 0, "active")
	store := dynamoAdapter.NewContributionsStore(memoria, e04xTabla)
	escritura := escrituraAporte(t, 100000, []appContrib.Allocation{{GoalID: e04xMeta1, AmountCents: 100000}})
	if _, err := store.CreateContribution(context.Background(), escritura); err != nil {
		t.Fatalf("create: %v", err)
	}
	c, err := store.LoadContribution(context.Background(), e04xUsuario, e04xAporte)
	if err != nil || c == nil {
		t.Fatalf("load: %v", err)
	}
	if c.ID != e04xAporte || c.AmountCents != 100000 || len(c.Allocations) != 1 {
		t.Fatalf("aportacion inesperada: %+v", c)
	}
	ajena, err := store.LoadContribution(context.Background(), "otro-usuario", e04xAporte)
	if err != nil || ajena != nil {
		t.Fatalf("otro usuario no debe ver la aportacion: %v/%v", err, ajena)
	}
}

func TestE047StoreReversionYReplay(t *testing.T) {
	memoria := nuevaMemoriaAportaciones()
	memoria.items[claveDe(mustGoalKeys(e04xUsuario, e04xMeta1))] = metaItem(e04xUsuario, e04xMeta1, 100000, 0, "active")
	store := dynamoAdapter.NewContributionsStore(memoria, e04xTabla)
	escritura := escrituraAporte(t, 100000, []appContrib.Allocation{{GoalID: e04xMeta1, AmountCents: 100000}})
	if _, err := store.CreateContribution(context.Background(), escritura); err != nil {
		t.Fatalf("create: %v", err)
	}
	c, _ := store.LoadContribution(context.Background(), e04xUsuario, e04xAporte)
	siguiente := *c
	siguiente.Status = appContrib.StatusReversed
	siguiente.Version = c.Version + 1
	siguiente.ReversedAt = e04xAhora
	siguiente.ReverseReason = "Correccion autorizada"
	body, _ := siguiente.Body()
	write := appContrib.ReverseWrite{
		UserID: e04xUsuario, Previous: *c, Next: siguiente,
		IdempotencyKey: "idem-rev-00000000001", Operation: appContrib.OperationReverse,
		PayloadHash:  appContrib.ReversePayloadHash(c.ID, siguiente.ReverseReason),
		ResponseBody: body, Location: "/api/v1/goal-contributions/" + c.ID,
		Now: e04xAhora, ExpiresAt: e04xAhora.Add(appContrib.IdempotencyTTL),
	}
	r1, err := store.ReverseContribution(context.Background(), write)
	if err != nil || r1.Status != 200 || r1.Replayed {
		t.Fatalf("reverse: %v/%+v", err, r1)
	}
	meta := memoria.items[claveDe(mustGoalKeys(e04xUsuario, e04xMeta1))]
	if numeroAporte(meta["savedAmountCents"]) != 0 || numeroAporte(meta["activeContributions"]) != 0 {
		t.Fatalf("meta no revertida: %+v", meta)
	}
	r2, err := store.ReverseContribution(context.Background(), write)
	if err != nil || !r2.Replayed {
		t.Fatalf("replay: %v/%v", err, r2 != nil && r2.Replayed)
	}
	write.IdempotencyKey = "idem-rev-00000000002"
	write.PayloadHash = appContrib.ReversePayloadHash(c.ID, "Otro motivo")
	if _, err := store.ReverseContribution(context.Background(), write); !errors.Is(err, appContrib.ErrAlreadyReversed) {
		t.Fatalf("doble reversion debe 409, obtenido %v", err)
	}
}

func TestE044StoreSinDynamoFallaCerrado(t *testing.T) {
	store := dynamoAdapter.NewContributionsStore(nil, "")
	if _, err := store.CreateContribution(context.Background(), appContrib.CreateWrite{}); !errors.Is(err, appContrib.ErrUnavailable) {
		t.Fatalf("esperado ErrUnavailable, obtenido %v", err)
	}
	if _, err := store.LoadContribution(context.Background(), "u", "018f90ef-08e5-7a68-b09f-2b67a47c92f0"); !errors.Is(err, appContrib.ErrUnavailable) {
		t.Fatalf("esperado ErrUnavailable, obtenido %v", err)
	}
}

func mustGoalKeys(userID, goalID string) (string, string) {
	return domainDynamo.GoalKeys(userID, goalID)
}
