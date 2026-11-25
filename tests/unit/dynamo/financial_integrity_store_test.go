package dynamo_test

// Pruebas de INYECCIÓN sobre los adaptadores de lectura de la Oleada 3, con un cliente DynamoDB
// falso que interpreta de forma estricta las condiciones y filtros EXACTOS que emite cada
// adaptador. Una forma no soportada hace fallar la prueba: si el adaptador cambia su consulta,
// la suite lo detecta y obliga a revisar la semántica en lugar de aceptarla en silencio.
//
// Reglas de integridad que se verifican contra el código de producción (no contra un doble de
// aplicación):
//   - E026: el movimiento `saving` vinculado a una aportación NO se suma dos veces y una
//     aportación `reversed` no cuenta como ahorro activo.
//   - E050: el uso presupuestal suma solo movimientos `expense` reales, ignorando `saving` e
//     `income`, y acumula a través de varias páginas de Query.
//   - E051: el listado de metas excluye tombstones de E042 y los movimientos llegan ordenados
//     del más reciente al más antiguo.
//   - E048: el conjunto se lee de `itemsJSON` y un ítem corrupto falla cerrado.
//
// No hay red ni AWS: `memoriaFinanciera` es un doble del cliente del SDK. Por eso esta suite NO
// acredita persistencia real; la verificación cloud sigue siendo NOT_RUN.

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	dynamoAdapter "github.com/nexumpr/Nexum-Backend/internal/adapters/dynamo"
	appBudget "github.com/nexumpr/Nexum-Backend/internal/application/budgetset"
	appUsage "github.com/nexumpr/Nexum-Backend/internal/application/budgetusage"
	appDashboard "github.com/nexumpr/Nexum-Backend/internal/application/dashboard"
	appSummary "github.com/nexumpr/Nexum-Backend/internal/application/financialsummary"
	appMovements "github.com/nexumpr/Nexum-Backend/internal/application/movements"
	domainDynamo "github.com/nexumpr/Nexum-Backend/internal/domain/dynamo"
)

const (
	fiTabla   = "NEXUM_CORE_TEST"
	fiUsuario = "11111111-2222-4333-8444-555555555555"
	fiZona    = "America/Mexico_City"
)

var fiAhora = time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)

func fiDia(dia int) time.Time { return time.Date(2026, 11, dia, 10, 0, 0, 0, time.UTC) }

// memoriaFinanciera es un cliente DynamoDB falso con estado para lecturas. No implementa
// escrituras: los adaptadores bajo prueba solo leen.
type memoriaFinanciera struct {
	mu           sync.Mutex
	items        map[string]map[string]types.AttributeValue
	tamanoPagina int
	consultas    int
	getItems     int
	fallo        error
}

func nuevaMemoriaFinanciera() *memoriaFinanciera {
	return &memoriaFinanciera{items: map[string]map[string]types.AttributeValue{}, tamanoPagina: 1000}
}

func (m *memoriaFinanciera) insertar(item map[string]types.AttributeValue) {
	m.items[claveDe(mapaString(item, "PK"), mapaString(item, "SK"))] = item
}

func (m *memoriaFinanciera) GetItem(_ context.Context, params *dynamodb.GetItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.getItems++
	if m.fallo != nil {
		return nil, m.fallo
	}
	item, ok := m.items[claveDe(mapaString(params.Key, "PK"), mapaString(params.Key, "SK"))]
	if !ok {
		return &dynamodb.GetItemOutput{}, nil
	}
	return &dynamodb.GetItemOutput{Item: copiarItem(item)}, nil
}

func (m *memoriaFinanciera) Query(_ context.Context, params *dynamodb.QueryInput, _ ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.consultas++
	if m.fallo != nil {
		return nil, m.fallo
	}
	condicion := aws.ToString(params.KeyConditionExpression)
	pk := textoAporte(params.ExpressionAttributeValues[":pk"])
	modo := ""
	switch {
	case strings.Contains(condicion, "begins_with(SK, :prefix)"):
		modo = "prefijo"
	case strings.Contains(condicion, "#sk BETWEEN :from AND :to"):
		modo = "rango"
	case strings.Contains(condicion, "#sk >= :from AND #sk < :to"):
		modo = "rango"
	default:
		return nil, errors.New("condicion de clave no soportada: " + condicion)
	}

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
		switch modo {
		case "prefijo":
			if !strings.HasPrefix(sk, textoAporte(params.ExpressionAttributeValues[":prefix"])) {
				continue
			}
		case "rango":
			desde := textoAporte(params.ExpressionAttributeValues[":from"])
			hasta := textoAporte(params.ExpressionAttributeValues[":to"])
			if sk < desde || sk >= hasta {
				continue
			}
		}
		candidatos = append(candidatos, entrada{sk: sk, item: copiarItem(item)})
	}
	sort.Slice(candidatos, func(i, j int) bool { return candidatos[i].sk < candidatos[j].sk })
	descendente := params.ScanIndexForward != nil && !aws.ToBool(params.ScanIndexForward)
	if descendente {
		for i, j := 0, len(candidatos)-1; i < j; i, j = i+1, j-1 {
			candidatos[i], candidatos[j] = candidatos[j], candidatos[i]
		}
	}
	inicio := 0
	if len(params.ExclusiveStartKey) > 0 {
		startSK := mapaString(params.ExclusiveStartKey, "SK")
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
	restantes := candidatos[inicio:]
	if filtro := aws.ToString(params.FilterExpression); filtro != "" {
		filtrados := make([]entrada, 0, len(restantes))
		for _, c := range restantes {
			cumple, err := cumpleFiltroFinanciero(filtro, c.item, params.ExpressionAttributeValues)
			if err != nil {
				return nil, err
			}
			if cumple {
				filtrados = append(filtrados, c)
			}
		}
		restantes = filtrados
	}
	limite := m.tamanoPagina
	if params.Limit != nil && int(aws.ToInt32(params.Limit)) < limite {
		limite = int(aws.ToInt32(params.Limit))
	}
	if limite < 1 {
		limite = 1
	}
	fin := len(restantes)
	if fin > limite {
		fin = limite
	}
	salida := &dynamodb.QueryOutput{Items: make([]map[string]types.AttributeValue, 0, fin), Count: int32(fin)}
	for _, c := range restantes[:fin] {
		salida.Items = append(salida.Items, c.item)
	}
	if fin < len(restantes) {
		salida.LastEvaluatedKey = map[string]types.AttributeValue{
			"PK": strAttr(pk),
			"SK": strAttr(restantes[fin-1].sk),
		}
	}
	return salida, nil
}

// cumpleFiltroFinanciero solo admite los filtros EXACTOS de E051 y de las lecturas de movimientos
// con exclusión de tombstones de E031. Cualquier otra forma falla.
func cumpleFiltroFinanciero(filtro string, item map[string]types.AttributeValue, valores map[string]types.AttributeValue) (bool, error) {
	switch filtro {
	case "#status = :active AND (attribute_not_exists(#deleted) OR #deleted = :false)":
		esperado, ok := valores[":false"].(*types.AttributeValueMemberBOOL)
		if !ok || esperado.Value {
			return false, errors.New("valor :false invalido en el filtro de E051")
		}
		if mapaString(item, "status") != textoAporte(valores[":active"]) {
			return false, nil
		}
		borrada, existe := item["deleted"].(*types.AttributeValueMemberBOOL)
		if !existe {
			return true, nil
		}
		return !borrada.Value, nil
	case "(attribute_not_exists(#deleted) OR #deleted = :false)":
		// E031: un ítem sin `deleted` (histórico anterior a E031) cuenta como no borrado.
		esperado, ok := valores[":false"].(*types.AttributeValueMemberBOOL)
		if !ok || esperado.Value {
			return false, errors.New("valor :false invalido en el filtro de tombstones")
		}
		borrado, existe := item["deleted"].(*types.AttributeValueMemberBOOL)
		if !existe {
			return true, nil
		}
		return !borrado.Value, nil
	default:
		return false, errors.New("filtro no soportado: " + filtro)
	}
}

// ---------- Constructores de ítems (forma física de la tabla única) ----------

func itemMemoriaMovimiento(pk string, momento time.Time, tipo string, centavos int64, categoria, aportacionID string) map[string]types.AttributeValue {
	id := "mov-" + tipo + "-" + momento.Format("20060102T150405")
	item := map[string]types.AttributeValue{
		"PK":          strAttr(pk),
		"SK":          strAttr(domainDynamo.PrefixMovement + appMovements.SortableTime(momento) + "#" + id),
		"entityType":  strAttr("USER_MOVEMENT"),
		"movementId":  strAttr(id),
		"type":        strAttr(tipo),
		"amountCents": numberAttrAporte(centavos),
		"currency":    strAttr("MXN"),
		"categoryId":  strAttr(categoria),
		"description": strAttr("Movimiento de prueba"),
		"occurredAt":  strAttr(appMovements.FormatOccurredAt(momento)),
		"version":     numberAttrAporte(1),
	}
	if aportacionID != "" {
		item["contributionId"] = strAttr(aportacionID)
	}
	return item
}

func itemMemoriaAportacion(pk string, momento time.Time, estado string, centavos int64) map[string]types.AttributeValue {
	return map[string]types.AttributeValue{
		"PK":          strAttr(pk),
		"SK":          strAttr("CONTRIBUTION#" + appMovements.SortableTime(momento)),
		"entityType":  strAttr("CONTRIBUTION"),
		"status":      strAttr(estado),
		"amountCents": numberAttrAporte(centavos),
	}
}

func itemMemoriaMeta(pk, metaID, estado string, objetivo, ahorro, apertura int64, borrada *bool) map[string]types.AttributeValue {
	item := map[string]types.AttributeValue{
		"PK":                strAttr(pk),
		"SK":                strAttr(domainDynamo.PrefixGoal + metaID),
		"entityType":        strAttr("GOAL"),
		"goalId":            strAttr(metaID),
		"name":              strAttr("Meta " + metaID),
		"targetAmountCents": numberAttrAporte(objetivo),
		"savedAmountCents":  numberAttrAporte(ahorro),
		"openingSavedCents": numberAttrAporte(apertura),
		"version":           numberAttrAporte(3),
		"status":            strAttr(estado),
		"currency":          strAttr("MXN"),
		"targetDate":        strAttr("2026-12-31"),
	}
	if borrada != nil {
		item["deleted"] = &types.AttributeValueMemberBOOL{Value: *borrada}
	}
	return item
}

func itemMemoriaPreferencias(pk, zona string) map[string]types.AttributeValue {
	return map[string]types.AttributeValue{
		"PK":         strAttr(pk),
		"SK":         strAttr("PREFERENCES"),
		"entityType": strAttr("USER_PREFERENCES"),
		"timezone":   strAttr(zona),
	}
}

// ---------- E026: ahorro sin doble conteo y revertidas excluidas ----------

func TestE026StoreNoDuplicaAhorroVinculadoYExcluyeRevertidas(t *testing.T) {
	pk := domainDynamo.PrefixUser + fiUsuario
	memoria := nuevaMemoriaFinanciera()
	memoria.insertar(itemMemoriaPreferencias(pk, fiZona))
	memoria.insertar(itemMemoriaMovimiento(pk, fiDia(3), appMovements.TypeIncome, 100000, "ingreso", ""))
	memoria.insertar(itemMemoriaMovimiento(pk, fiDia(4), appMovements.TypeExpense, 20000, "comida", ""))
	memoria.insertar(itemMemoriaMovimiento(pk, fiDia(5), appMovements.TypeSaving, 30000, "ahorro", ""))
	// Movimiento vinculado: NO debe sumarse por separado; la aportación es la autoridad.
	memoria.insertar(itemMemoriaMovimiento(pk, fiDia(6), appMovements.TypeSaving, 50000, "goal", "018f90ef-08e5-7a68-b09f-0000000000c1"))
	memoria.insertar(itemMemoriaAportacion(pk, fiDia(6), "active", 50000))
	// Revertida: no cuenta como ahorro activo.
	memoria.insertar(itemMemoriaAportacion(pk, fiDia(7), "reversed", 70000))
	memoria.insertar(itemMemoriaAportacion(pk, fiDia(8), "active", 10000))
	// Varias páginas: el adaptador debe acumular sin truncar.
	memoria.tamanoPagina = 2

	elemento := dynamoAdapter.NewFinancialSummaryStore(memoria, fiTabla)
	servicio := appSummary.NewService(elemento).WithClock(func() time.Time { return fiAhora })
	resumen, err := servicio.Summary(context.Background(), fiUsuario, "2026-11")
	if err != nil {
		t.Fatalf("E026 store: %v", err)
	}
	if resumen.Income != 100000 || resumen.Expenses != 20000 {
		t.Fatalf("E026 ingresos/egresos inesperados: %+v", resumen)
	}
	// 30 000 independientes + 50 000 activa + 10 000 activa = 90 000.
	// Si el vinculado se sumara aparte serían 140 000; si la revertida contara, 160 000.
	if resumen.Savings != 90000 {
		t.Fatalf("E026 savings esperado 90000 sin doble conteo, obtenido %d", resumen.Savings)
	}
	if resumen.Net != -10000 {
		t.Fatalf("E026 net esperado -10000, obtenido %d", resumen.Net)
	}
	if memoria.consultas < 3 {
		t.Fatalf("la agregación debe paginar: consultas=%d", memoria.consultas)
	}
}

func TestE026StoreFallaCerradoAnteItemInconsistente(t *testing.T) {
	pk := domainDynamo.PrefixUser + fiUsuario
	sinTipo := itemMemoriaMovimiento(pk, fiDia(3), appMovements.TypeExpense, 1000, "otros", "")
	delete(sinTipo, "type")
	casos := []struct {
		nombre string
		item   map[string]types.AttributeValue
	}{
		{"tipo desconocido", itemMemoriaMovimiento(pk, fiDia(3), "transfer", 1000, "otros", "")},
		{"importe negativo", itemMemoriaMovimiento(pk, fiDia(3), appMovements.TypeExpense, -1000, "otros", "")},
		{"sin tipo", sinTipo},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			memoria := nuevaMemoriaFinanciera()
			memoria.insertar(itemMemoriaPreferencias(pk, fiZona))
			memoria.insertar(caso.item)
			elemento := dynamoAdapter.NewFinancialSummaryStore(memoria, fiTabla)
			if _, err := elemento.AggregateMovements(context.Background(), fiUsuario, fiDia(1), fiDia(30)); !errors.Is(err, appSummary.ErrInconsistent) {
				t.Fatalf("esperado ErrInconsistent, obtenido %v", err)
			}
		})
	}

	// Una aportación con estado desconocido o importe negativo tampoco se agrega.
	for _, item := range []map[string]types.AttributeValue{
		itemMemoriaAportacion(pk, fiDia(6), "pending", 1000),
		itemMemoriaAportacion(pk, fiDia(6), "active", -1000),
	} {
		memoria := nuevaMemoriaFinanciera()
		memoria.insertar(item)
		elemento := dynamoAdapter.NewFinancialSummaryStore(memoria, fiTabla)
		if _, err := elemento.AggregateContributions(context.Background(), fiUsuario, fiDia(1), fiDia(30)); !errors.Is(err, appSummary.ErrInconsistent) {
			t.Fatalf("aportacion incoherente: esperado ErrInconsistent, obtenido %v", err)
		}
	}

	memoria := nuevaMemoriaFinanciera()
	memoria.fallo = errors.New("dynamodb caido")
	elemento := dynamoAdapter.NewFinancialSummaryStore(memoria, fiTabla)
	if _, err := elemento.AggregateMovements(context.Background(), fiUsuario, fiDia(1), fiDia(30)); !errors.Is(err, appSummary.ErrUnavailable) {
		t.Fatalf("un fallo de DynamoDB debe ser 503, obtenido %v", err)
	}
}

// ---------- E050: uso desde egresos reales, ignorando ahorro e ingreso ----------

func TestE050StoreUsoDesdeExpenseRealIgnoraAhorroEIngreso(t *testing.T) {
	pk := domainDynamo.PrefixUser + fiUsuario
	memoria := nuevaMemoriaFinanciera()
	memoria.insertar(itemMemoriaPreferencias(pk, "UTC"))
	memoria.insertar(itemMemoriaMovimiento(pk, fiDia(10), appMovements.TypeExpense, 25000, "comida", ""))
	memoria.insertar(itemMemoriaMovimiento(pk, fiDia(11), appMovements.TypeExpense, 10000, "transporte", ""))
	// Mismo tamaño que un egreso, pero ahorro: no puede consumir presupuesto.
	memoria.insertar(itemMemoriaMovimiento(pk, fiDia(9), appMovements.TypeSaving, 5000, "comida", ""))
	// Ingreso: tampoco.
	memoria.insertar(itemMemoriaMovimiento(pk, fiDia(12), appMovements.TypeIncome, 99999, "comida", ""))
	memoria.tamanoPagina = 2

	conjunto, ok := dynamoAdapter.BudgetSetItem(fiUsuario, "2026-11", "MXN", 3, []appBudget.Budget{
		{CategoryID: "comida", LimitAmountCents: 100000},
		{CategoryID: "transporte", LimitAmountCents: 50000},
	}, fiAhora)
	if !ok {
		t.Fatal("BudgetSetItem debe aceptar el conjunto valido")
	}
	memoria.insertar(conjunto)

	elemento := dynamoAdapter.NewBudgetUsageStore(memoria, fiTabla)
	uso, err := appUsage.NewService(elemento).WithClock(func() time.Time { return fiAhora }).Usage(context.Background(), fiUsuario, "2026-11")
	if err != nil {
		t.Fatalf("E050 store: %v", err)
	}
	if len(uso) != 2 {
		t.Fatalf("E050: esperadas 2 categorias, obtenidas %d", len(uso))
	}
	if uso[0].CategoryID != "comida" || uso[0].UsedCents != 25000 || uso[0].LimitCents != 100000 {
		t.Fatalf("E050 comida inesperada: %+v", uso[0])
	}
	if uso[1].CategoryID != "transporte" || uso[1].UsedCents != 10000 || uso[1].LimitCents != 50000 {
		t.Fatalf("E050 transporte inesperada: %+v", uso[1])
	}
	if memoria.consultas < 2 {
		t.Fatalf("la agregación debe paginar: consultas=%d", memoria.consultas)
	}

	// Sin conjunto no hay filas y no se inventan límites.
	if _, err := elemento.LoadSet(context.Background(), fiUsuario, "2026-12"); err != nil {
		t.Fatalf("LoadSet sin conjunto: %v", err)
	}
	vacio, err := appUsage.NewService(elemento).WithClock(func() time.Time { return fiAhora }).Usage(context.Background(), fiUsuario, "2026-12")
	if err != nil || len(vacio) != 0 {
		t.Fatalf("E050 sin presupuesto: %v/%d", err, len(vacio))
	}
}

// ---------- E051: exclusión de tombstones y orden de recientes ----------

func TestE051StoreExcluyeTombstoneYOrdenaRecientes(t *testing.T) {
	pk := domainDynamo.PrefixUser + fiUsuario
	borrada := true
	falsa := false
	memoria := nuevaMemoriaFinanciera()
	memoria.insertar(itemMemoriaMeta(pk, "018f90ef-08e5-7a68-b09f-0000000000a1", "active", 100000, 25000, 5000, nil))
	memoria.insertar(itemMemoriaMeta(pk, "018f90ef-08e5-7a68-b09f-0000000000a2", "archived", 100000, 0, 0, &borrada))
	memoria.insertar(itemMemoriaMeta(pk, "018f90ef-08e5-7a68-b09f-0000000000a3", "archived", 100000, 0, 0, nil))
	memoria.insertar(itemMemoriaMeta(pk, "018f90ef-08e5-7a68-b09f-0000000000a4", "active", 100000, 0, 0, &falsa))
	memoria.insertar(itemMemoriaMovimiento(pk, fiDia(3), appMovements.TypeExpense, 1000, "comida", ""))
	memoria.insertar(itemMemoriaMovimiento(pk, fiDia(5), appMovements.TypeIncome, 2000, "ingreso", ""))
	memoria.insertar(itemMemoriaMovimiento(pk, fiDia(4), appMovements.TypeSaving, 3000, "ahorro", ""))

	elemento := dynamoAdapter.NewDashboardStore(memoria, fiTabla)
	metas, err := elemento.ListActiveGoals(context.Background(), fiUsuario, 100)
	if err != nil {
		t.Fatalf("E051 metas: %v", err)
	}
	// A (sin atributo deleted) y D (deleted=false) son visibles; B (tombstone) y C (archived) no.
	if len(metas) != 2 {
		t.Fatalf("E051: esperadas 2 metas visibles, obtenidas %d", len(metas))
	}
	for _, meta := range metas {
		if meta.ID == "018f90ef-08e5-7a68-b09f-0000000000a2" || meta.ID == "018f90ef-08e5-7a68-b09f-0000000000a3" {
			t.Fatalf("E051 listo una meta borrada o archivada: %s", meta.ID)
		}
	}

	movimientos, err := elemento.ListLatestMovements(context.Background(), fiUsuario, 10)
	if err != nil {
		t.Fatalf("E051 movimientos: %v", err)
	}
	if len(movimientos) != 3 {
		t.Fatalf("E051: esperados 3 movimientos, obtenidos %d", len(movimientos))
	}
	if !movimientos[0].OccurredAt.Equal(fiDia(5)) || !movimientos[1].OccurredAt.Equal(fiDia(4)) || !movimientos[2].OccurredAt.Equal(fiDia(3)) {
		t.Fatalf("E051: orden inesperado: %s/%s/%s", movimientos[0].OccurredAt, movimientos[1].OccurredAt, movimientos[2].OccurredAt)
	}
	recientes, err := elemento.ListLatestMovements(context.Background(), fiUsuario, 2)
	if err != nil || len(recientes) != 2 {
		t.Fatalf("E051 limite: %v/%d", err, len(recientes))
	}
	if !recientes[0].OccurredAt.Equal(fiDia(5)) || !recientes[1].OccurredAt.Equal(fiDia(4)) {
		t.Fatalf("E051 limite devolvio un orden inesperado")
	}

	// Un movimiento incoherente no se convierte en un valor inventado.
	memoria.insertar(map[string]types.AttributeValue{
		"PK": strAttr(pk), "SK": strAttr("MOVEMENT#2026-11-06T10:00:00.000Z"),
		"type": strAttr("expense"),
	})
	if _, err := elemento.ListLatestMovements(context.Background(), fiUsuario, 10); !errors.Is(err, appDashboard.ErrInvalid) {
		t.Fatalf("un movimiento sin importe debe fallar cerrado, obtenido %v", err)
	}
}

// ---------- E048: lectura del conjunto y fallo cerrado del ítem corrupto ----------

func TestE048StoreLeeConjuntoYFallaCerrado(t *testing.T) {
	pk := domainDynamo.PrefixUser + fiUsuario
	memoria := nuevaMemoriaFinanciera()
	periodo, ok := dynamoAdapter.BudgetSetItem(fiUsuario, "2026-11", "MXN", 3, []appBudget.Budget{
		{CategoryID: "comida", LimitAmountCents: 100000},
	}, fiAhora)
	if !ok {
		t.Fatal("BudgetSetItem debe aceptar el conjunto valido")
	}
	memoria.insertar(periodo)
	anterior, _ := dynamoAdapter.BudgetSetItem(fiUsuario, "2026-10", "MXN", 1, []appBudget.Budget{
		{CategoryID: "transporte", LimitAmountCents: 50000},
	}, fiAhora)
	memoria.insertar(anterior)

	elemento := dynamoAdapter.NewBudgetSetStore(memoria, fiTabla)
	conjunto, err := elemento.LoadSet(context.Background(), fiUsuario, "2026-11")
	if err != nil || conjunto == nil {
		t.Fatalf("E048 LoadSet: %v/%v", err, conjunto)
	}
	if conjunto.Version != 3 || conjunto.Currency != "MXN" || len(conjunto.Items) != 1 || conjunto.Items[0].LimitAmountCents != 100000 {
		t.Fatalf("E048 conjunto inesperado: %+v", conjunto)
	}
	if faltante, err := elemento.LoadSet(context.Background(), fiUsuario, "2026-12"); err != nil || faltante != nil {
		t.Fatalf("E048 sin conjunto: %v/%v", err, faltante)
	}
	conjuntos, err := elemento.ListAllSets(context.Background(), fiUsuario)
	if err != nil || len(conjuntos) != 2 {
		t.Fatalf("E048 ListAllSets: %v/%d", err, len(conjuntos))
	}
	if conjuntos[0].Period != "2026-10" || conjuntos[1].Period != "2026-11" {
		t.Fatalf("E048 periodos inesperados: %+v", conjuntos)
	}

	memoria.insertar(map[string]types.AttributeValue{
		"PK": strAttr(pk), "SK": strAttr("BUDGETSET#2026-12"),
		"entityType": strAttr("BUDGET_SET"), "version": numberAttrAporte(1), "itemsJSON": strAttr("{"),
	})
	if _, err := elemento.LoadSet(context.Background(), fiUsuario, "2026-12"); !errors.Is(err, appBudget.ErrInconsistent) {
		t.Fatalf("un itemsJSON corrupto debe fallar cerrado, obtenido %v", err)
	}
	for _, invalido := range [][]appBudget.Budget{
		{{CategoryID: "", LimitAmountCents: 1}},
		{{CategoryID: "comida", LimitAmountCents: -1}},
	} {
		if _, ok := dynamoAdapter.BudgetSetItem(fiUsuario, "2026-12", "MXN", 1, invalido, fiAhora); ok {
			t.Fatalf("BudgetSetItem debe rechazar %+v", invalido)
		}
	}
}
