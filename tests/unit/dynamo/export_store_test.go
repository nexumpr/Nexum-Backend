package dynamo_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	dynamoAdapter "github.com/nexumpr/Nexum-Backend/internal/adapters/dynamo"
	appExport "github.com/nexumpr/Nexum-Backend/internal/application/export"
	appGoals "github.com/nexumpr/Nexum-Backend/internal/application/goals"
	appIdentity "github.com/nexumpr/Nexum-Backend/internal/application/identity"
	appMovements "github.com/nexumpr/Nexum-Backend/internal/application/movements"
	appOnboarding "github.com/nexumpr/Nexum-Backend/internal/application/onboarding"
	appPreferences "github.com/nexumpr/Nexum-Backend/internal/application/preferences"
	appPrivacy "github.com/nexumpr/Nexum-Backend/internal/application/privacy"
	appProfile "github.com/nexumpr/Nexum-Backend/internal/application/profile"
	appRecurring "github.com/nexumpr/Nexum-Backend/internal/application/recurring"
	domainDynamo "github.com/nexumpr/Nexum-Backend/internal/domain/dynamo"
)

// Pruebas del adaptador de E020 (`GET /api/v1/users/me/export`, RFC-022 §2):
//
//   - `Query` por la partición del titular, con consistencia fuerte y orden ascendente.
//   - Paginación real por `ExclusiveStartKey`/`LastEvaluatedKey`, sin pérdidas ni duplicados.
//   - Fail-closed: un fallo de DynamoDB jamás se convierte en una colección vacía.
//   - Aislamiento de usuario: ninguna consulta sale de `USER#<sub>`.
//   - La interfaz del cliente no expone `Scan`; el compilador impide introducirlo.
//   - Reanudación de presupuestos sin arrastrar otros prefijos de la partición.
//
// El doble interpreta las MISMAS condiciones de clave que emite el adaptador real y la semántica
// de DynamoDB para `Limit` (cuenta ITEMS evaluados), `ExclusiveStartKey` y
// `LastEvaluatedKey`. Si aparece una construcción no soportada falla con error explícito en
// lugar de adivinar.

const (
	e020TablaDynamo = "NEXUM_CORE_TEST"
	e020Titular     = "11111111-2222-4333-8444-555555555555"
	e020Ajeno       = "99999999-8888-4777-8666-555555555555"
)

var e020InstanteBase = time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)

// e020Consulta registra la consulta emitida para poder afirmar sobre ella.
type e020Consulta struct {
	Tabla       string
	Condicion   string
	PK          string
	Prefijo     string
	RangoSK     string
	Limite      int32
	ExclusiveSK string
	Consistente bool
	Ascendente  bool
}

// e020Dynamo es el doble con estado del subconjunto `ExportDynamoAPI`.
type e020Dynamo struct {
	mu    sync.Mutex
	items map[string]map[string]types.AttributeValue

	// err simula un fallo de DynamoDB en toda consulta.
	err error
	// sinProgreso devuelve `LastEvaluatedKey` sin ninguna fila: el adaptador no puede avanzar.
	sinProgreso bool

	consultas []e020Consulta
	lecturas  []e020Lectura
}

// e020Lectura registra un GetItem para poder afirmar consistencia fuerte y clave exacta.
type e020Lectura struct {
	PK          string
	SK          string
	Consistente bool
}

func newE020Dynamo() *e020Dynamo {
	return &e020Dynamo{items: map[string]map[string]types.AttributeValue{}}
}

func e020ClaveItem(pk, sk string) string { return pk + "\x00" + sk }

func (m *e020Dynamo) sembrar(pk, sk string, attrs map[string]types.AttributeValue) {
	m.mu.Lock()
	defer m.mu.Unlock()
	item := map[string]types.AttributeValue{"PK": strAttr(pk), "SK": strAttr(sk)}
	for nombre, valor := range attrs {
		item[nombre] = valor
	}
	m.items[e020ClaveItem(pk, sk)] = item
}

func (m *e020Dynamo) consultasRealizadas() []e020Consulta {
	m.mu.Lock()
	defer m.mu.Unlock()
	copia := make([]e020Consulta, len(m.consultas))
	copy(copia, m.consultas)
	return copia
}

func (m *e020Dynamo) lecturasRealizadas() []e020Lectura {
	m.mu.Lock()
	defer m.mu.Unlock()
	copia := make([]e020Lectura, len(m.lecturas))
	copy(copia, m.lecturas)
	return copia
}

// GetItem resuelve el singleton `NOTIFICATION_PREFERENCES` con consistencia fuerte.
func (m *e020Dynamo) GetItem(_ context.Context, in *dynamodb.GetItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	lectura := e020Lectura{
		PK:          mapaString(in.Key, "PK"),
		SK:          mapaString(in.Key, "SK"),
		Consistente: aws.ToBool(in.ConsistentRead),
	}
	m.lecturas = append(m.lecturas, lectura)
	if m.err != nil {
		return nil, m.err
	}
	item, ok := m.items[e020ClaveItem(lectura.PK, lectura.SK)]
	if !ok {
		return &dynamodb.GetItemOutput{}, nil
	}
	copia := make(map[string]types.AttributeValue, len(item))
	for k, v := range item {
		copia[k] = v
	}
	return &dynamodb.GetItemOutput{Item: copia}, nil
}

// Query reproduce la semántica de DynamoDB usada por el adaptador.
func (m *e020Dynamo) Query(_ context.Context, in *dynamodb.QueryInput, _ ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	consulta := e020Consulta{
		Tabla:       aws.ToString(in.TableName),
		Condicion:   aws.ToString(in.KeyConditionExpression),
		Consistente: aws.ToBool(in.ConsistentRead),
		Ascendente:  in.ScanIndexForward == nil || aws.ToBool(in.ScanIndexForward),
	}
	if in.Limit != nil {
		consulta.Limite = *in.Limit
	}
	if len(in.ExclusiveStartKey) > 0 {
		consulta.ExclusiveSK = mapaString(in.ExclusiveStartKey, "SK")
	}
	if err := interpretarCondicionE020(consulta.Condicion, in.ExpressionAttributeValues, &consulta); err != nil {
		return nil, err
	}
	m.consultas = append(m.consultas, consulta)

	if m.err != nil {
		return nil, m.err
	}
	if m.sinProgreso {
		return &dynamodb.QueryOutput{
			LastEvaluatedKey: map[string]types.AttributeValue{
				"PK": strAttr(consulta.PK),
				"SK": strAttr("BUDGETSET#2026-01"),
			},
		}, nil
	}

	candidatos := make([]map[string]types.AttributeValue, 0, len(m.items))
	for _, item := range m.items {
		if mapaString(item, "PK") != consulta.PK {
			continue
		}
		sk := mapaString(item, "SK")
		if consulta.Prefijo != "" && !strings.HasPrefix(sk, consulta.Prefijo) {
			continue
		}
		if consulta.RangoSK != "" && !(sk >= consulta.RangoSK) {
			continue
		}
		candidatos = append(candidatos, item)
	}
	sort.Slice(candidatos, func(i, j int) bool {
		return mapaString(candidatos[i], "SK") < mapaString(candidatos[j], "SK")
	})

	// DynamoDB reanuda ESTRICTAMENTE después de la clave de continuación, aunque esa clave ya no
	// exista en la tabla.
	inicio := 0
	for i, item := range candidatos {
		if mapaString(item, "SK") > consulta.ExclusiveSK {
			inicio = i
			break
		}
		inicio = i + 1
	}
	restantes := candidatos[inicio:]

	limite := len(restantes)
	if consulta.Limite > 0 && int(consulta.Limite) < limite {
		limite = int(consulta.Limite)
	}
	ventana := restantes[:limite]

	salida := make([]map[string]types.AttributeValue, 0, len(ventana))
	for _, item := range ventana {
		salida = append(salida, copiarItem(item))
	}
	resultado := &dynamodb.QueryOutput{Items: salida}
	if len(restantes) > limite && limite > 0 {
		ultimo := ventana[len(ventana)-1]
		resultado.LastEvaluatedKey = map[string]types.AttributeValue{
			"PK": ultimo["PK"],
			"SK": ultimo["SK"],
		}
	}
	return resultado, nil
}

// interpretarCondicionE020 soporta las condiciones que emite el adaptador. Registra el rango de
// SK cuando se usa una comparación abierta, para que la prueba pueda rechazarla.
func interpretarCondicionE020(condicion string, valores map[string]types.AttributeValue, consulta *e020Consulta) error {
	partes := strings.Split(condicion, " AND ")
	if len(partes) == 0 || len(partes) > 2 {
		return errors.New("condicion de clave no soportada por el doble: " + condicion)
	}
	for _, parte := range partes {
		parte = strings.TrimSpace(parte)
		switch {
		case strings.HasPrefix(parte, "PK = "):
			consulta.PK = valorString(valores[strings.TrimSpace(strings.TrimPrefix(parte, "PK = "))])
		case strings.HasPrefix(parte, "begins_with(SK, "):
			alias := strings.TrimSuffix(strings.TrimSpace(strings.TrimPrefix(parte, "begins_with(SK, ")), ")")
			consulta.Prefijo = valorString(valores[alias])
		case strings.HasPrefix(parte, "SK >= "):
			consulta.RangoSK = valorString(valores[strings.TrimSpace(strings.TrimPrefix(parte, "SK >= "))])
		default:
			return errors.New("condicion de clave no soportada por el doble: " + parte)
		}
	}
	if consulta.PK == "" {
		return errors.New("condicion sin particion: " + condicion)
	}
	return nil
}

// ---------- sembrado de items ----------

func e020AportacionSK(id string, ocurrida time.Time) string {
	return appExport.PrefijoAportacion + ocurrida.UTC().Format(appMovements.OccurredAtLayout) + "#" + id
}

func e020AportacionItem(userID, id string, ocurrida time.Time, centavos int64) map[string]types.AttributeValue {
	return map[string]types.AttributeValue{
		"entityType":      strAttr("CONTRIBUTION"),
		"userId":          strAttr(userID),
		"contributionId":  strAttr(id),
		"amountCents":     numAttr(centavos),
		"currency":        strAttr("MXN"),
		"allocationsJSON": strAttr(fmt.Sprintf(`[{"goalId":"meta-1","amountCents":%d}]`, centavos)),
		"occurredAt":      strAttr(ocurrida.UTC().Format(appMovements.OccurredAtLayout)),
		"status":          strAttr("active"),
		"version":         numAttr(1),
	}
}

func e020ItemsJSON(categorias map[string]int64) string {
	nombres := make([]string, 0, len(categorias))
	for nombre := range categorias {
		nombres = append(nombres, nombre)
	}
	sort.Strings(nombres)
	partes := make([]string, 0, len(nombres))
	for _, nombre := range nombres {
		partes = append(partes, fmt.Sprintf(`{"categoryId":%q,"limitAmountCents":%d}`, nombre, categorias[nombre]))
	}
	return "[" + strings.Join(partes, ",") + "]"
}

func e020PresupuestoItem(userID, periodo string, categorias map[string]int64) map[string]types.AttributeValue {
	return map[string]types.AttributeValue{
		"entityType": strAttr("BUDGET_SET"),
		"userId":     strAttr(userID),
		"itemsJSON":  strAttr(e020ItemsJSON(categorias)),
		"currency":   strAttr("MXN"),
		"version":    numAttr(2),
	}
}

// ---------- aportaciones ----------

// TestE020ExportAportacionesConsultaLaParticionDelTitular comprueba la forma exacta de la
// consulta y que sólo se leen filas del titular autenticado.
func TestE020ExportAportacionesConsultaLaParticionDelTitular(t *testing.T) {
	dyn := newE020Dynamo()
	skUno := e020AportacionSK("ap-1", e020InstanteBase.Add(1*time.Minute))
	skDos := e020AportacionSK("ap-2", e020InstanteBase.Add(2*time.Minute))
	dyn.sembrar(domainDynamo.PrefixUser+e020Titular, skUno, e020AportacionItem(e020Titular, "ap-1", e020InstanteBase.Add(1*time.Minute), 5000))
	dyn.sembrar(domainDynamo.PrefixUser+e020Titular, skDos, e020AportacionItem(e020Titular, "ap-2", e020InstanteBase.Add(2*time.Minute), 7500))
	dyn.sembrar(domainDynamo.PrefixUser+e020Titular, e020AportacionSK("ap-3", e020InstanteBase.Add(3*time.Minute)), e020AportacionItem(e020Titular, "ap-3", e020InstanteBase.Add(3*time.Minute), 12500))
	// Fila de otra cuenta en su propia partición: no debe aparecer nunca.
	dyn.sembrar(domainDynamo.PrefixUser+e020Ajeno, e020AportacionSK("ajena", e020InstanteBase), e020AportacionItem(e020Ajeno, "ajena", e020InstanteBase, 99999))

	tienda := dynamoAdapter.NewExportStore(dynamoAdapter.ExportStoreDeps{Cliente: dyn, Tabla: e020TablaDynamo})
	filas, siguiente, err := tienda.Aportaciones(context.Background(), "  "+e020Titular+"  ", "", 2)
	if err != nil {
		t.Fatalf("aportaciones: %v", err)
	}
	if len(filas) != 2 {
		t.Fatalf("esperadas 2 filas, obtenidas %d", len(filas))
	}
	if filas[0].ID != "ap-1" || filas[1].ID != "ap-2" {
		t.Fatalf("orden ascendente por SK esperado: %+v", filas)
	}
	if siguiente != skDos {
		t.Fatalf("continuacion inesperada: %q", siguiente)
	}
	if filas[0].AmountCents != 5000 || filas[0].Currency != "MXN" || filas[0].Status != appExport.EstadoAportacionActiva {
		t.Fatalf("proyeccion inesperada: %+v", filas[0])
	}
	if len(filas[0].Allocations) != 1 || filas[0].Allocations[0].GoalID != "meta-1" || filas[0].Allocations[0].AmountCents != 5000 {
		t.Fatalf("asignaciones inesperadas: %+v", filas[0].Allocations)
	}

	consultas := dyn.consultasRealizadas()
	if len(consultas) != 1 {
		t.Fatalf("se esperaba una unica consulta, obtenidas %d", len(consultas))
	}
	consulta := consultas[0]
	if consulta.Tabla != e020TablaDynamo {
		t.Fatalf("tabla inesperada: %q", consulta.Tabla)
	}
	if consulta.PK != domainDynamo.PrefixUser+e020Titular {
		t.Fatalf("la consulta debe ir a la particion del titular: %q", consulta.PK)
	}
	if consulta.Prefijo != appExport.PrefijoAportacion {
		t.Fatalf("la consulta debe restringirse al prefijo de aportaciones: %q", consulta.Prefijo)
	}
	if consulta.RangoSK != "" {
		t.Fatalf("no se admite un rango abierto de SK: %q", consulta.RangoSK)
	}
	if !consulta.Consistente {
		t.Fatal("la lectura debe ser fuertemente consistente")
	}
	if !consulta.Ascendente {
		t.Fatal("el orden debe ser ascendente")
	}
	if consulta.Limite != 3 {
		t.Fatalf("se debe pedir una fila de mas (limit+1): %d", consulta.Limite)
	}
	if consulta.ExclusiveSK != "" {
		t.Fatalf("sin cursor no debe haber ExclusiveStartKey: %q", consulta.ExclusiveSK)
	}
}

// TestE020ExportAportacionesPaginaHastaAgotar recorre la colección completa sin perder ni
// duplicar filas.
func TestE020ExportAportacionesPaginaHastaAgotar(t *testing.T) {
	dyn := newE020Dynamo()
	total := 5
	for i := 0; i < total; i++ {
		instante := e020InstanteBase.Add(time.Duration(i) * time.Minute)
		id := fmt.Sprintf("ap-%d", i)
		dyn.sembrar(domainDynamo.PrefixUser+e020Titular, e020AportacionSK(id, instante), e020AportacionItem(e020Titular, id, instante, int64(1000*(i+1))))
	}
	for i := 0; i < 2; i++ {
		instante := e020InstanteBase.Add(time.Duration(i) * time.Minute)
		id := fmt.Sprintf("ajena-%d", i)
		dyn.sembrar(domainDynamo.PrefixUser+e020Ajeno, e020AportacionSK(id, instante), e020AportacionItem(e020Ajeno, id, instante, 1))
	}

	tienda := dynamoAdapter.NewExportStore(dynamoAdapter.ExportStoreDeps{Cliente: dyn, Tabla: e020TablaDynamo})
	vistos := map[string]bool{}
	orden := make([]string, 0, total)
	after := ""
	paginas := 0
	for {
		filas, siguiente, err := tienda.Aportaciones(context.Background(), e020Titular, after, 2)
		if err != nil {
			t.Fatalf("pagina %d: %v", paginas+1, err)
		}
		paginas++
		for _, fila := range filas {
			if vistos[fila.ID] {
				t.Fatalf("aportacion repetida entre paginas: %s", fila.ID)
			}
			if strings.HasPrefix(fila.ID, "ajena") {
				t.Fatalf("se filtro una aportacion de otra cuenta: %s", fila.ID)
			}
			vistos[fila.ID] = true
			orden = append(orden, fila.ID)
		}
		if siguiente == "" {
			break
		}
		if siguiente == after {
			t.Fatalf("sin progreso: la continuacion repite %q", after)
		}
		after = siguiente
		if paginas > 10 {
			t.Fatal("demasiadas paginas: la paginacion no avanza")
		}
	}
	if paginas != 3 {
		t.Fatalf("5 filas con pagina de 2 deben ser 3 paginas, obtenidas %d", paginas)
	}
	if len(orden) != total {
		t.Fatalf("se esperaban %d filas, obtenidas %d", total, len(orden))
	}
	for i := 0; i < total; i++ {
		esperado := fmt.Sprintf("ap-%d", i)
		if orden[i] != esperado {
			t.Fatalf("orden inesperado en la posicion %d: %q", i, orden[i])
		}
	}
}

// TestE020ExportFalloDynamoNoDevuelveColeccionVacia cubre el fail-closed: un fallo de
// persistencia nunca se presenta como colección vacía.
func TestE020ExportFalloDynamoNoDevuelveColeccionVacia(t *testing.T) {
	dyn := newE020Dynamo()
	dyn.err = errors.New("dynamodb inaccesible")
	tienda := dynamoAdapter.NewExportStore(dynamoAdapter.ExportStoreDeps{Cliente: dyn, Tabla: e020TablaDynamo})

	filas, siguiente, err := tienda.Aportaciones(context.Background(), e020Titular, "", 10)
	if !errors.Is(err, appExport.ErrUnavailable) {
		t.Fatalf("aportaciones: esperado ErrUnavailable, obtenido %v", err)
	}
	if filas != nil || siguiente != "" {
		t.Fatalf("un fallo no puede devolver filas ni continuacion: %v/%q", filas, siguiente)
	}

	filasPresupuesto, siguientePresupuesto, errPresupuesto := tienda.Presupuestos(context.Background(), e020Titular, "", 10)
	if !errors.Is(errPresupuesto, appExport.ErrUnavailable) {
		t.Fatalf("presupuestos: esperado ErrUnavailable, obtenido %v", errPresupuesto)
	}
	if filasPresupuesto != nil || siguientePresupuesto != "" {
		t.Fatalf("un fallo no puede devolver filas ni continuacion: %v/%q", filasPresupuesto, siguientePresupuesto)
	}
}

// TestE020ExportSinProgresoNoDeclaraExito comprueba que `LastEvaluatedKey` sin filas no se
// interpreta como una exportación válida.
func TestE020ExportSinProgresoNoDeclaraExito(t *testing.T) {
	dyn := newE020Dynamo()
	dyn.sinProgreso = true
	tienda := dynamoAdapter.NewExportStore(dynamoAdapter.ExportStoreDeps{Cliente: dyn, Tabla: e020TablaDynamo})

	if _, _, err := tienda.Aportaciones(context.Background(), e020Titular, "", 10); !errors.Is(err, appExport.ErrUnavailable) {
		t.Fatalf("aportaciones sin progreso deben fallar cerrado: %v", err)
	}
	if _, _, err := tienda.Presupuestos(context.Background(), e020Titular, "", 10); !errors.Is(err, appExport.ErrUnavailable) {
		t.Fatalf("presupuestos sin progreso deben fallar cerrado: %v", err)
	}
}

// TestE020ExportAportacionIncoherenteNoSeEmite comprueba que un item corrupto no se publica ni
// se omite en silencio.
func TestE020ExportAportacionIncoherenteNoSeEmite(t *testing.T) {
	casos := map[string]map[string]types.AttributeValue{
		"sin importe": {
			"entityType":     strAttr("CONTRIBUTION"),
			"userId":         strAttr(e020Titular),
			"contributionId": strAttr("ap-rota"),
			"occurredAt":     strAttr(e020InstanteBase.Format(appMovements.OccurredAtLayout)),
			"status":         strAttr("active"),
		},
		"estado desconocido": {
			"entityType":     strAttr("CONTRIBUTION"),
			"userId":         strAttr(e020Titular),
			"contributionId": strAttr("ap-rota"),
			"amountCents":    numAttr(100),
			"occurredAt":     strAttr(e020InstanteBase.Format(appMovements.OccurredAtLayout)),
			"status":         strAttr("otro"),
		},
		"titular ajeno": {
			"entityType":     strAttr("CONTRIBUTION"),
			"userId":         strAttr(e020Ajeno),
			"contributionId": strAttr("ap-rota"),
			"amountCents":    numAttr(100),
			"occurredAt":     strAttr(e020InstanteBase.Format(appMovements.OccurredAtLayout)),
			"status":         strAttr("active"),
		},
		"asignaciones corruptas": {
			"entityType":      strAttr("CONTRIBUTION"),
			"userId":          strAttr(e020Titular),
			"contributionId":  strAttr("ap-rota"),
			"amountCents":     numAttr(100),
			"allocationsJSON": strAttr("{no-es-json"),
			"occurredAt":      strAttr(e020InstanteBase.Format(appMovements.OccurredAtLayout)),
			"status":          strAttr("active"),
		},
	}
	for nombre, item := range casos {
		t.Run(nombre, func(t *testing.T) {
			dyn := newE020Dynamo()
			dyn.sembrar(domainDynamo.PrefixUser+e020Titular, e020AportacionSK("ap-rota", e020InstanteBase), item)
			tienda := dynamoAdapter.NewExportStore(dynamoAdapter.ExportStoreDeps{Cliente: dyn, Tabla: e020TablaDynamo})
			if _, _, err := tienda.Aportaciones(context.Background(), e020Titular, "", 10); !errors.Is(err, appExport.ErrInconsistent) {
				t.Fatalf("esperado ErrInconsistent, obtenido %v", err)
			}
		})
	}
}

// TestE020ExportAportacionSinVersionEsUno conserva el criterio de E045 para items heredados.
func TestE020ExportAportacionSinVersionEsUno(t *testing.T) {
	dyn := newE020Dynamo()
	item := e020AportacionItem(e020Titular, "ap-heredada", e020InstanteBase, 4200)
	delete(item, "version")
	dyn.sembrar(domainDynamo.PrefixUser+e020Titular, e020AportacionSK("ap-heredada", e020InstanteBase), item)
	tienda := dynamoAdapter.NewExportStore(dynamoAdapter.ExportStoreDeps{Cliente: dyn, Tabla: e020TablaDynamo})

	filas, _, err := tienda.Aportaciones(context.Background(), e020Titular, "", 10)
	if err != nil {
		t.Fatalf("aportaciones: %v", err)
	}
	if len(filas) != 1 || filas[0].Version != 1 {
		t.Fatalf("un item heredado sin version observable es la version 1: %+v", filas)
	}
}

// ---------- presupuestos ----------

// TestE020ExportPresupuestosReanudaSinArrastrarOtrosPrefijos es la prueba de regresión del
// defecto real: al reanudar una página a mitad de un periodo, la consulta no puede usar un
// rango abierto de SK (`SK >= "BUDGETSET#<periodo>"`), porque en la partición del titular
// `BUDGETSET#` es el prefijo MENOR y el rango arrastraría `CONSENT#`, `GOAL#`, `MOVEMENT#`...
// El adaptador debe restringirse al periodo de continuación y después seguir con el prefijo
// completo de presupuestos.
func TestE020ExportPresupuestosReanudaSinArrastrarOtrosPrefijos(t *testing.T) {
	dyn := newE020Dynamo()
	pk := domainDynamo.PrefixUser + e020Titular
	dyn.sembrar(pk, "BUDGETSET#2026-10", e020PresupuestoItem(e020Titular, "2026-10", map[string]int64{"food": 150000, "rent": 400000}))
	dyn.sembrar(pk, "BUDGETSET#2026-11", e020PresupuestoItem(e020Titular, "2026-11", map[string]int64{"food": 175000, "rent": 420000}))
	// Otros prefijos de la MISMA partición, lexicográficamente posteriores a BUDGETSET#.
	dyn.sembrar(pk, "CONSENT#2026-11-v2", map[string]types.AttributeValue{
		"entityType": strAttr("CONSENT"), "userId": strAttr(e020Titular), "purpose": strAttr("privacy_notice"),
	})
	dyn.sembrar(pk, "GOAL#meta-1", map[string]types.AttributeValue{
		"entityType": strAttr("GOAL"), "userId": strAttr(e020Titular),
	})
	dyn.sembrar(pk, "MOVEMENT#2026-11-10T12:00:00.000Z#mov-1", map[string]types.AttributeValue{
		"entityType": strAttr("MOVEMENT"), "userId": strAttr(e020Titular),
	})
	dyn.sembrar(pk, "PROFILE", map[string]types.AttributeValue{
		"entityType": strAttr("USER_PROFILE"), "userId": strAttr(e020Titular),
	})
	// Presupuestos de otra cuenta: no deben aparecer.
	dyn.sembrar(domainDynamo.PrefixUser+e020Ajeno, "BUDGETSET#2026-10", e020PresupuestoItem(e020Ajeno, "2026-10", map[string]int64{"food": 1}))

	tienda := dynamoAdapter.NewExportStore(dynamoAdapter.ExportStoreDeps{Cliente: dyn, Tabla: e020TablaDynamo})

	primera, siguiente, err := tienda.Presupuestos(context.Background(), e020Titular, "", 3)
	if err != nil {
		t.Fatalf("primera pagina: %v", err)
	}
	if len(primera) != 3 {
		t.Fatalf("esperadas 3 filas, obtenidas %d: %+v", len(primera), primera)
	}
	esperadas := []struct {
		periodo   string
		categoria string
		limite    int64
	}{
		{"2026-10", "food", 150000},
		{"2026-10", "rent", 400000},
		{"2026-11", "food", 175000},
	}
	for i, esperada := range esperadas {
		if primera[i].Period != esperada.periodo || primera[i].CategoryID != esperada.categoria || primera[i].LimitAmountCents != esperada.limite {
			t.Fatalf("fila %d inesperada: %+v", i, primera[i])
		}
	}
	if siguiente != "BUDGETSET#2026-11#food" {
		t.Fatalf("continuacion inesperada: %q", siguiente)
	}

	segunda, fin, err := tienda.Presupuestos(context.Background(), e020Titular, siguiente, 3)
	if err != nil {
		t.Fatalf("segunda pagina: %v", err)
	}
	if len(segunda) != 1 || segunda[0].Period != "2026-11" || segunda[0].CategoryID != "rent" || segunda[0].LimitAmountCents != 420000 {
		t.Fatalf("segunda pagina inesperada: %+v", segunda)
	}
	if fin != "" {
		t.Fatalf("agotada la coleccion no debe haber continuacion: %q", fin)
	}

	for i, consulta := range dyn.consultasRealizadas() {
		if consulta.PK != pk {
			t.Fatalf("consulta %d fuera de la particion del titular: %q", i, consulta.PK)
		}
		if consulta.RangoSK != "" {
			t.Fatalf("consulta %d uso un rango abierto de SK (%q); arrastraria prefijos ajenos", i, consulta.RangoSK)
		}
		if consulta.Prefijo == "" || !strings.HasPrefix(consulta.Prefijo, appExport.PrefijoPresupuesto) {
			t.Fatalf("consulta %d debe restringirse al prefijo de presupuestos: %q", i, consulta.Prefijo)
		}
	}
}

// TestE020ExportPresupuestosCortaDentroDeUnPeriodo comprueba la continuación con la clave
// sintética exacta dentro del mismo periodo y su reanudación.
func TestE020ExportPresupuestosCortaDentroDeUnPeriodo(t *testing.T) {
	dyn := newE020Dynamo()
	pk := domainDynamo.PrefixUser + e020Titular
	dyn.sembrar(pk, "BUDGETSET#2026-11", e020PresupuestoItem(e020Titular, "2026-11", map[string]int64{
		"food": 100000, "health": 200000, "rent": 300000,
	}))
	tienda := dynamoAdapter.NewExportStore(dynamoAdapter.ExportStoreDeps{Cliente: dyn, Tabla: e020TablaDynamo})

	primera, siguiente, err := tienda.Presupuestos(context.Background(), e020Titular, "", 2)
	if err != nil {
		t.Fatalf("primera pagina: %v", err)
	}
	if len(primera) != 2 || primera[0].CategoryID != "food" || primera[1].CategoryID != "health" {
		t.Fatalf("orden por categoria esperado: %+v", primera)
	}
	if siguiente != "BUDGETSET#2026-11#health" {
		t.Fatalf("continuacion inesperada: %q", siguiente)
	}

	segunda, fin, err := tienda.Presupuestos(context.Background(), e020Titular, siguiente, 2)
	if err != nil {
		t.Fatalf("segunda pagina: %v", err)
	}
	if len(segunda) != 1 || segunda[0].CategoryID != "rent" {
		t.Fatalf("segunda pagina inesperada: %+v", segunda)
	}
	if fin != "" {
		t.Fatalf("no debe quedar continuacion: %q", fin)
	}
}

// TestE020ExportPresupuestosResumePeriodoAusente comprueba que si el periodo de continuación
// desapareció, el recorrido NO declara agotada la colección: sigue por los periodos posteriores.
func TestE020ExportPresupuestosResumePeriodoAusente(t *testing.T) {
	dyn := newE020Dynamo()
	pk := domainDynamo.PrefixUser + e020Titular
	dyn.sembrar(pk, "BUDGETSET#2026-11", e020PresupuestoItem(e020Titular, "2026-11", map[string]int64{"food": 100000}))
	dyn.sembrar(pk, "BUDGETSET#2026-12", e020PresupuestoItem(e020Titular, "2026-12", map[string]int64{"food": 120000}))
	tienda := dynamoAdapter.NewExportStore(dynamoAdapter.ExportStoreDeps{Cliente: dyn, Tabla: e020TablaDynamo})

	// El cursor apunta a un periodo que ya no existe (borrado entre páginas).
	filas, siguiente, err := tienda.Presupuestos(context.Background(), e020Titular, "BUDGETSET#2026-11x#food", 5)
	if err != nil {
		t.Fatalf("reanudacion: %v", err)
	}
	if len(filas) != 1 || filas[0].Period != "2026-12" {
		t.Fatalf("debe continuar con el periodo siguiente: %+v", filas)
	}
	if siguiente != "" {
		t.Fatalf("no debe quedar continuacion: %q", siguiente)
	}
}

// TestE020ExportPresupuestoIncoherente cubre los items de presupuesto que no se pueden
// representar en el contrato.
func TestE020ExportPresupuestoIncoherente(t *testing.T) {
	casos := map[string]map[string]types.AttributeValue{
		"sin items": {
			"entityType": strAttr("BUDGET_SET"), "userId": strAttr(e020Titular), "version": numAttr(1),
		},
		"sin version": {
			"entityType": strAttr("BUDGET_SET"), "userId": strAttr(e020Titular), "itemsJSON": strAttr(e020ItemsJSON(map[string]int64{"food": 1})),
		},
		"titular ajeno": {
			"entityType": strAttr("BUDGET_SET"), "userId": strAttr(e020Ajeno), "itemsJSON": strAttr(e020ItemsJSON(map[string]int64{"food": 1})), "version": numAttr(1),
		},
		"entidad ajena": {
			"entityType": strAttr("CONSENT"), "userId": strAttr(e020Titular), "itemsJSON": strAttr(e020ItemsJSON(map[string]int64{"food": 1})), "version": numAttr(1),
		},
	}
	for nombre, item := range casos {
		t.Run(nombre, func(t *testing.T) {
			dyn := newE020Dynamo()
			dyn.sembrar(domainDynamo.PrefixUser+e020Titular, "BUDGETSET#2026-11", item)
			tienda := dynamoAdapter.NewExportStore(dynamoAdapter.ExportStoreDeps{Cliente: dyn, Tabla: e020TablaDynamo})
			if _, _, err := tienda.Presupuestos(context.Background(), e020Titular, "", 10); !errors.Is(err, appExport.ErrInconsistent) {
				t.Fatalf("esperado ErrInconsistent, obtenido %v", err)
			}
		})
	}
}

// TestE020ExportPresupuestoClaveInvalida rechaza una continuación que no tenga la forma
// `BUDGETSET#<periodo>#<categoria>`.
func TestE020ExportPresupuestoClaveInvalida(t *testing.T) {
	dyn := newE020Dynamo()
	tienda := dynamoAdapter.NewExportStore(dynamoAdapter.ExportStoreDeps{Cliente: dyn, Tabla: e020TablaDynamo})
	for _, clave := range []string{"2026-11", "BUDGETSET#", "OTRO#2026-11#food"} {
		if _, _, err := tienda.Presupuestos(context.Background(), e020Titular, clave, 10); !errors.Is(err, appExport.ErrUnavailable) {
			t.Fatalf("clave %q debe fallar cerrado, obtenido %v", clave, err)
		}
	}
	if consultas := dyn.consultasRealizadas(); len(consultas) != 0 {
		t.Fatalf("una clave invalida no debe llegar a DynamoDB: %+v", consultas)
	}
}

// TestE020ExportFamiliasVacias comprueba que una partición sin datos devuelve colecciones
// vacías (nunca `nil` con error) y ninguna continuación.
func TestE020ExportFamiliasVacias(t *testing.T) {
	dyn := newE020Dynamo()
	tienda := dynamoAdapter.NewExportStore(dynamoAdapter.ExportStoreDeps{Cliente: dyn, Tabla: e020TablaDynamo})

	aportaciones, siguienteAportaciones, err := tienda.Aportaciones(context.Background(), e020Titular, "", 25)
	if err != nil || aportaciones == nil || len(aportaciones) != 0 || siguienteAportaciones != "" {
		t.Fatalf("aportaciones vacias inesperadas: %v/%v/%q", aportaciones, err, siguienteAportaciones)
	}
	presupuestos, siguientePresupuestos, err := tienda.Presupuestos(context.Background(), e020Titular, "", 25)
	if err != nil || presupuestos == nil || len(presupuestos) != 0 || siguientePresupuestos != "" {
		t.Fatalf("presupuestos vacios inesperados: %v/%v/%q", presupuestos, err, siguientePresupuestos)
	}
}

// TestE020ExportSinDependenciasFallaCerrado cubre las guardas del adaptador antes de consultar.
func TestE020ExportSinDependenciasFallaCerrado(t *testing.T) {
	sinCliente := dynamoAdapter.NewExportStore(dynamoAdapter.ExportStoreDeps{Tabla: e020TablaDynamo})
	sinTabla := dynamoAdapter.NewExportStore(dynamoAdapter.ExportStoreDeps{Cliente: newE020Dynamo()})
	completa := dynamoAdapter.NewExportStore(dynamoAdapter.ExportStoreDeps{Cliente: newE020Dynamo(), Tabla: e020TablaDynamo})

	casos := []struct {
		nombre string
		tienda *dynamoAdapter.ExportStore
		userID string
		limit  int
	}{
		{"sin cliente", sinCliente, e020Titular, 10},
		{"sin tabla", sinTabla, e020Titular, 10},
		{"sin usuario", completa, "   ", 10},
		{"limit cero", completa, e020Titular, 0},
		{"limit excesivo", completa, e020Titular, appExport.MaxLimit + 1},
	}
	for _, caso := range casos {
		if _, _, err := caso.tienda.Aportaciones(context.Background(), caso.userID, "", caso.limit); !errors.Is(err, appExport.ErrUnavailable) {
			t.Fatalf("%s (aportaciones): esperado ErrUnavailable, obtenido %v", caso.nombre, err)
		}
		if _, _, err := caso.tienda.Presupuestos(context.Background(), caso.userID, "", caso.limit); !errors.Is(err, appExport.ErrUnavailable) {
			t.Fatalf("%s (presupuestos): esperado ErrUnavailable, obtenido %v", caso.nombre, err)
		}
	}
}

// TestE020ExportNingunaInterfazExponeScan impide por contrato introducir un `Scan`.
func TestE020ExportNingunaInterfazExponeScan(t *testing.T) {
	interfaces := map[string]reflect.Type{
		"ExportDynamoAPI":         reflect.TypeOf((*dynamoAdapter.ExportDynamoAPI)(nil)).Elem(),
		"ExportMovementsReader":   reflect.TypeOf((*dynamoAdapter.ExportMovementsReader)(nil)).Elem(),
		"ExportRecurringReader":   reflect.TypeOf((*dynamoAdapter.ExportRecurringReader)(nil)).Elem(),
		"ExportGoalsReader":       reflect.TypeOf((*dynamoAdapter.ExportGoalsReader)(nil)).Elem(),
		"ExportProfileReader":     reflect.TypeOf((*dynamoAdapter.ExportProfileReader)(nil)).Elem(),
		"ExportIncomeReader":      reflect.TypeOf((*dynamoAdapter.ExportIncomeReader)(nil)).Elem(),
		"ExportConsentsReader":    reflect.TypeOf((*dynamoAdapter.ExportConsentsReader)(nil)).Elem(),
		"ExportOnboardingReader":  reflect.TypeOf((*dynamoAdapter.ExportOnboardingReader)(nil)).Elem(),
		"ExportPreferencesReader": reflect.TypeOf((*dynamoAdapter.ExportPreferencesReader)(nil)).Elem(),
	}
	for nombre, tipo := range interfaces {
		if tipo.Kind() != reflect.Interface {
			t.Fatalf("%s debe ser una interfaz", nombre)
		}
		if tipo.NumMethod() == 0 {
			t.Fatalf("%s no declara metodos", nombre)
		}
		for i := 0; i < tipo.NumMethod(); i++ {
			if strings.Contains(strings.ToLower(tipo.Method(i).Name), "scan") {
				t.Fatalf("%s expone un metodo de barrido: %s", nombre, tipo.Method(i).Name)
			}
		}
	}
	api := reflect.TypeOf((*dynamoAdapter.ExportDynamoAPI)(nil)).Elem()
	if api.NumMethod() != 2 {
		t.Fatalf("el subconjunto de DynamoDB debe declarar exactamente Query y GetItem: %v", api)
	}
	permitidos := map[string]bool{"Query": true, "GetItem": true}
	for i := 0; i < api.NumMethod(); i++ {
		if !permitidos[api.Method(i).Name] {
			t.Fatalf("metodo no autorizado en el subconjunto de DynamoDB: %s", api.Method(i).Name)
		}
	}
}

// TestE020ExportDelegacionDeLectores comprueba que cada familia usa su lector, con el titular
// autenticado y el límite pedido, y que cada error se traduce al error de aplicación correcto.
func TestE020ExportDelegacionDeLectores(t *testing.T) {
	t.Run("movimientos", func(t *testing.T) {
		lector := &e020MovimientosDoble{err: errors.New("dynamodb caido")}
		tienda := dynamoAdapter.NewExportStore(dynamoAdapter.ExportStoreDeps{Movimientos: lector})
		if _, _, err := tienda.Movimientos(context.Background(), e020Titular, "MOVEMENT#x", 7); !errors.Is(err, appExport.ErrUnavailable) {
			t.Fatalf("esperado ErrUnavailable, obtenido %v", err)
		}
		if !lector.llamado || lector.userID != e020Titular || lector.after != "MOVEMENT#x" || lector.limit != 7 {
			t.Fatalf("delegacion inesperada: %+v", lector)
		}
	})

	t.Run("recurrentes", func(t *testing.T) {
		lector := &e020RecurrentesDoble{}
		tienda := dynamoAdapter.NewExportStore(dynamoAdapter.ExportStoreDeps{Recurrentes: lector})
		if _, _, err := tienda.Recurrentes(context.Background(), e020Titular, "", 5); err != nil {
			t.Fatalf("recurrentes: %v", err)
		}
		if !lector.llamado || lector.filter.Active != nil || lector.filter.Type != "" {
			t.Fatalf("la exportacion no debe filtrar reglas: %+v", lector.filter)
		}
		lector.err = errors.New("dynamodb caido")
		if _, _, err := tienda.Recurrentes(context.Background(), e020Titular, "", 5); !errors.Is(err, appExport.ErrUnavailable) {
			t.Fatalf("esperado ErrUnavailable, obtenido %v", err)
		}
	})

	t.Run("metas", func(t *testing.T) {
		lector := &e020MetasDoble{err: appGoals.ErrInconsistent}
		tienda := dynamoAdapter.NewExportStore(dynamoAdapter.ExportStoreDeps{Metas: lector})
		if _, _, err := tienda.Metas(context.Background(), e020Titular, "GOAL#x", 4); !errors.Is(err, appExport.ErrInconsistent) {
			t.Fatalf("una meta incoherente debe propagarse: %v", err)
		}
		if lector.query.UserID != e020Titular || lector.query.StartSK != "GOAL#x" || lector.query.Limit != 4 || lector.query.HasStatus {
			t.Fatalf("consulta inesperada: %+v", lector.query)
		}
		lector.err = errors.New("dynamodb caido")
		if _, _, err := tienda.Metas(context.Background(), e020Titular, "", 4); !errors.Is(err, appExport.ErrUnavailable) {
			t.Fatalf("esperado ErrUnavailable, obtenido %v", err)
		}
	})

	t.Run("perfil", func(t *testing.T) {
		lector := &e020PerfilDoble{}
		tienda := dynamoAdapter.NewExportStore(dynamoAdapter.ExportStoreDeps{Perfil: lector})

		lector.err = appProfile.ErrProfileNotFound
		if perfil, err := tienda.Perfil(context.Background(), e020Titular); perfil != nil || err != nil {
			t.Fatalf("un perfil ausente es (nil, nil): %v/%v", perfil, err)
		}
		lector.err = appProfile.ErrProfileUnauthorized
		if _, err := tienda.Perfil(context.Background(), e020Titular); !errors.Is(err, appExport.ErrUnauthorized) {
			t.Fatalf("esperado ErrUnauthorized, obtenido %v", err)
		}
		lector.err = errors.New("dynamodb caido")
		if _, err := tienda.Perfil(context.Background(), e020Titular); !errors.Is(err, appExport.ErrUnavailable) {
			t.Fatalf("esperado ErrUnavailable, obtenido %v", err)
		}
		lector.err = nil
		lector.perfil = &appProfile.Profile{ID: e020Titular}
		perfil, err := tienda.Perfil(context.Background(), e020Titular)
		if err != nil || perfil == nil || perfil.ID != e020Titular {
			t.Fatalf("perfil inesperado: %v/%v", perfil, err)
		}
		if lector.userID != e020Titular {
			t.Fatalf("el lector debe recibir el titular: %q", lector.userID)
		}
	})

	t.Run("ingreso", func(t *testing.T) {
		lector := &e020IngresoDoble{}
		tienda := dynamoAdapter.NewExportStore(dynamoAdapter.ExportStoreDeps{Ingreso: lector})

		lector.err = appIdentity.ErrIncomeNotFound
		if ingreso, err := tienda.Ingreso(context.Background(), e020Titular); ingreso != nil || err != nil {
			t.Fatalf("un ingreso ausente es (nil, nil): %v/%v", ingreso, err)
		}
		lector.err = appIdentity.ErrIncomeUnauthorized
		if _, err := tienda.Ingreso(context.Background(), e020Titular); !errors.Is(err, appExport.ErrUnauthorized) {
			t.Fatalf("esperado ErrUnauthorized, obtenido %v", err)
		}
		lector.err = appIdentity.ErrIncomeInconsistent
		if _, err := tienda.Ingreso(context.Background(), e020Titular); !errors.Is(err, appExport.ErrInconsistent) {
			t.Fatalf("esperado ErrInconsistent, obtenido %v", err)
		}
		lector.err = errors.New("dynamodb caido")
		if _, err := tienda.Ingreso(context.Background(), e020Titular); !errors.Is(err, appExport.ErrUnavailable) {
			t.Fatalf("esperado ErrUnavailable, obtenido %v", err)
		}
	})

	t.Run("onboarding", func(t *testing.T) {
		lector := &e020OnboardingDoble{}
		tienda := dynamoAdapter.NewExportStore(dynamoAdapter.ExportStoreDeps{Onboarding: lector})

		lector.err = appOnboarding.ErrNotFound
		if estado, err := tienda.Onboarding(context.Background(), e020Titular); estado != nil || err != nil {
			t.Fatalf("un onboarding ausente es (nil, nil): %v/%v", estado, err)
		}
		lector.err = appOnboarding.ErrUnauthorized
		if _, err := tienda.Onboarding(context.Background(), e020Titular); !errors.Is(err, appExport.ErrUnauthorized) {
			t.Fatalf("esperado ErrUnauthorized, obtenido %v", err)
		}
		lector.err = errors.New("dynamodb caido")
		if _, err := tienda.Onboarding(context.Background(), e020Titular); !errors.Is(err, appExport.ErrUnavailable) {
			t.Fatalf("esperado ErrUnavailable, obtenido %v", err)
		}
	})

	t.Run("preferencias", func(t *testing.T) {
		lector := &e020PreferenciasDoble{}
		tienda := dynamoAdapter.NewExportStore(dynamoAdapter.ExportStoreDeps{Preferencias: lector})

		lector.err = appPreferences.ErrPreferencesUnauthorized
		if _, err := tienda.Preferencias(context.Background(), e020Titular); !errors.Is(err, appExport.ErrUnauthorized) {
			t.Fatalf("esperado ErrUnauthorized, obtenido %v", err)
		}
		lector.err = errors.New("dynamodb caido")
		if _, err := tienda.Preferencias(context.Background(), e020Titular); !errors.Is(err, appExport.ErrUnavailable) {
			t.Fatalf("esperado ErrUnavailable, obtenido %v", err)
		}
		lector.err = nil
		lector.prefs = &appPreferences.Preferences{Locale: "es-MX", Timezone: "America/Mexico_City", Version: 2, HasVersion: true}
		prefs, err := tienda.Preferencias(context.Background(), e020Titular)
		if err != nil || prefs == nil || prefs.Locale != "es-MX" {
			t.Fatalf("preferencias inesperadas: %v/%v", prefs, err)
		}
	})

	t.Run("sin lector", func(t *testing.T) {
		vacia := dynamoAdapter.NewExportStore(dynamoAdapter.ExportStoreDeps{})
		if _, _, err := vacia.Movimientos(context.Background(), e020Titular, "", 1); !errors.Is(err, appExport.ErrUnavailable) {
			t.Fatalf("movimientos sin lector: %v", err)
		}
		if _, _, err := vacia.Recurrentes(context.Background(), e020Titular, "", 1); !errors.Is(err, appExport.ErrUnavailable) {
			t.Fatalf("recurrentes sin lector: %v", err)
		}
		if _, _, err := vacia.Metas(context.Background(), e020Titular, "", 1); !errors.Is(err, appExport.ErrUnavailable) {
			t.Fatalf("metas sin lector: %v", err)
		}
		if _, err := vacia.Perfil(context.Background(), e020Titular); !errors.Is(err, appExport.ErrUnavailable) {
			t.Fatalf("perfil sin lector: %v", err)
		}
		if _, err := vacia.Ingreso(context.Background(), e020Titular); !errors.Is(err, appExport.ErrUnavailable) {
			t.Fatalf("ingreso sin lector: %v", err)
		}
		if _, err := vacia.Onboarding(context.Background(), e020Titular); !errors.Is(err, appExport.ErrUnavailable) {
			t.Fatalf("onboarding sin lector: %v", err)
		}
		if _, err := vacia.Preferencias(context.Background(), e020Titular); !errors.Is(err, appExport.ErrUnavailable) {
			t.Fatalf("preferencias sin lector: %v", err)
		}
		if _, err := vacia.Consentimientos(context.Background(), e020Titular); !errors.Is(err, appExport.ErrUnavailable) {
			t.Fatalf("consentimientos sin lector: %v", err)
		}
	})
}

// TestE020ExportConsentimientosRecorreLaColeccionCompleta comprueba que el singleton viaja
// entero en cada página: se siguen las páginas internas y nunca se trunca en silencio.
func TestE020ExportConsentimientosRecorreLaColeccionCompleta(t *testing.T) {
	lector := &e020ConsentimientosDoble{}
	lector.pagina = func(llamada int, desde string) ([]appPrivacy.ConsentRecord, string, error) {
		switch llamada {
		case 1:
			if desde != "" {
				t.Fatalf("la primera pagina debe empezar sin cursor: %q", desde)
			}
			return []appPrivacy.ConsentRecord{{ID: "c1"}, {ID: "c2"}}, "CONSENT#2026-11-v1", nil
		case 2:
			if desde != "CONSENT#2026-11-v1" {
				t.Fatalf("la segunda pagina debe continuar desde la SK: %q", desde)
			}
			return []appPrivacy.ConsentRecord{{ID: "c3"}}, "", nil
		default:
			t.Fatalf("llamada inesperada: %d", llamada)
			return nil, "", nil
		}
	}
	tienda := dynamoAdapter.NewExportStore(dynamoAdapter.ExportStoreDeps{Consentimientos: lector})
	registros, err := tienda.Consentimientos(context.Background(), e020Titular)
	if err != nil {
		t.Fatalf("consentimientos: %v", err)
	}
	if len(registros) != 3 || registros[0].ID != "c1" || registros[2].ID != "c3" {
		t.Fatalf("coleccion incompleta: %+v", registros)
	}
	if lector.llamadas != 2 {
		t.Fatalf("se esperaban 2 paginas internas, obtenidas %d", lector.llamadas)
	}
	for _, limite := range lector.limites {
		if limite != appPrivacy.ConsentsMaxLimit {
			t.Fatalf("cada pagina interna debe pedir el maximo del contrato: %d", limite)
		}
	}
	for _, userID := range lector.usuarios {
		if userID != e020Titular {
			t.Fatalf("se consulto un titular ajeno: %q", userID)
		}
	}
}

// TestE020ExportConsentimientosFallaCerrado cubre error, falta de progreso y tope de páginas.
func TestE020ExportConsentimientosFallaCerrado(t *testing.T) {
	t.Run("error de lectura", func(t *testing.T) {
		lector := &e020ConsentimientosDoble{}
		lector.pagina = func(int, string) ([]appPrivacy.ConsentRecord, string, error) {
			return nil, "", errors.New("dynamodb caido")
		}
		tienda := dynamoAdapter.NewExportStore(dynamoAdapter.ExportStoreDeps{Consentimientos: lector})
		if _, err := tienda.Consentimientos(context.Background(), e020Titular); !errors.Is(err, appExport.ErrUnavailable) {
			t.Fatalf("esperado ErrUnavailable, obtenido %v", err)
		}
	})

	t.Run("sin progreso", func(t *testing.T) {
		lector := &e020ConsentimientosDoble{}
		lector.pagina = func(_ int, desde string) ([]appPrivacy.ConsentRecord, string, error) {
			// Devuelve siempre la misma clave de continuación: el adaptador debe detectar que
			// no hay avance y fallar cerrado en lugar de repetir la página indefinidamente.
			return []appPrivacy.ConsentRecord{{ID: "c"}}, "CONSENT#2026-11", nil
		}
		tienda := dynamoAdapter.NewExportStore(dynamoAdapter.ExportStoreDeps{Consentimientos: lector})
		if _, err := tienda.Consentimientos(context.Background(), e020Titular); !errors.Is(err, appExport.ErrUnavailable) {
			t.Fatalf("sin avance debe fallar cerrado: %v", err)
		}
		if lector.llamadas != 2 {
			t.Fatalf("debe detenerse en la segunda llamada, obtenidas %d", lector.llamadas)
		}
	})

	t.Run("tope de paginas", func(t *testing.T) {
		lector := &e020ConsentimientosDoble{}
		lector.pagina = func(llamada int, _ string) ([]appPrivacy.ConsentRecord, string, error) {
			return []appPrivacy.ConsentRecord{{ID: fmt.Sprintf("c-%d", llamada)}}, fmt.Sprintf("CONSENT#%03d", llamada), nil
		}
		tienda := dynamoAdapter.NewExportStore(dynamoAdapter.ExportStoreDeps{Consentimientos: lector})
		if _, err := tienda.Consentimientos(context.Background(), e020Titular); !errors.Is(err, appExport.ErrUnavailable) {
			t.Fatalf("agotar el tope debe fallar cerrado: %v", err)
		}
		if lector.llamadas != 50 {
			t.Fatalf("se esperaban 50 paginas internas, obtenidas %d", lector.llamadas)
		}
	})

	t.Run("usuario vacio", func(t *testing.T) {
		lector := &e020ConsentimientosDoble{}
		tienda := dynamoAdapter.NewExportStore(dynamoAdapter.ExportStoreDeps{Consentimientos: lector})
		if _, err := tienda.Consentimientos(context.Background(), "  "); !errors.Is(err, appExport.ErrUnauthorized) {
			t.Fatalf("esperado ErrUnauthorized, obtenido %v", err)
		}
		if lector.llamadas != 0 {
			t.Fatalf("sin titular no se debe consultar: %d", lector.llamadas)
		}
	})
}

// ---------- dobles de lectores ----------

type e020MovimientosDoble struct {
	llamado bool
	userID  string
	after   string
	limit   int
	filas   []appMovements.Movement
	resto   string
	err     error
}

func (d *e020MovimientosDoble) ListMovements(_ context.Context, query appMovements.ListQuery, startSK string, limit int) ([]appMovements.Movement, string, error) {
	d.llamado = true
	d.userID, d.after, d.limit = query.UserID, startSK, limit
	return d.filas, d.resto, d.err
}

type e020RecurrentesDoble struct {
	llamado bool
	userID  string
	after   string
	limit   int
	filter  appRecurring.Filter
	err     error
}

func (d *e020RecurrentesDoble) List(_ context.Context, userID, startSK string, limit int, filter appRecurring.Filter) ([]appRecurring.Payment, string, error) {
	d.llamado = true
	d.userID, d.after, d.limit, d.filter = userID, startSK, limit, filter
	return nil, "", d.err
}

type e020MetasDoble struct {
	query appGoals.ListStoreQuery
	err   error
}

func (d *e020MetasDoble) ListUserGoals(_ context.Context, query appGoals.ListStoreQuery) ([]appGoals.GoalRow, string, error) {
	d.query = query
	return nil, "", d.err
}

type e020PerfilDoble struct {
	userID string
	perfil *appProfile.Profile
	err    error
}

func (d *e020PerfilDoble) Get(_ context.Context, userID string) (*appProfile.Profile, error) {
	d.userID = userID
	return d.perfil, d.err
}

type e020IngresoDoble struct {
	userID string
	err    error
}

func (d *e020IngresoDoble) GetIncome(_ context.Context, userID string) (*appIdentity.IncomeSnapshot, error) {
	d.userID = userID
	return nil, d.err
}

type e020OnboardingDoble struct {
	userID string
	err    error
}

func (d *e020OnboardingDoble) Get(_ context.Context, userID string) (*appOnboarding.State, error) {
	d.userID = userID
	return nil, d.err
}

type e020PreferenciasDoble struct {
	userID string
	prefs  *appPreferences.Preferences
	err    error
}

func (d *e020PreferenciasDoble) LoadPreferences(_ context.Context, userID string) (*appPreferences.Preferences, error) {
	d.userID = userID
	return d.prefs, d.err
}

type e020ConsentimientosDoble struct {
	mu       sync.Mutex
	llamadas int
	usuarios []string
	limites  []int
	pagina   func(llamada int, desde string) ([]appPrivacy.ConsentRecord, string, error)
}

func (d *e020ConsentimientosDoble) ListUserConsents(_ context.Context, userID, startSK string, limit int) ([]appPrivacy.ConsentRecord, string, error) {
	d.mu.Lock()
	d.llamadas++
	llamada := d.llamadas
	d.usuarios = append(d.usuarios, userID)
	d.limites = append(d.limites, limit)
	d.mu.Unlock()
	if d.pagina == nil {
		return nil, "", nil
	}
	return d.pagina(llamada, startSK)
}

// Compilación: el cliente real de DynamoDB satisface el subconjunto de la exportación.
var _ dynamoAdapter.ExportDynamoAPI = (*dynamodb.Client)(nil)
