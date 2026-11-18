package dynamo_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"regexp"
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
	appIdentity "github.com/nexumpr/Nexum-Backend/internal/application/identity"
	domainDynamo "github.com/nexumpr/Nexum-Backend/internal/domain/dynamo"
	domainIdentity "github.com/nexumpr/Nexum-Backend/internal/domain/identity"
)

// Pruebas del adaptador REAL `IncomeStore` (E024 lectura, E025 escritura condicional) sobre un
// doble con estado PROPIO.
//
// El doble compartido (`memoriaDynamo`) no implementa `UpdateItem` ni la disyunción sin
// paréntesis que emite `StoreIncome`, y NO se modifica: aquí se declara `memoriaIngreso`, que
// interpreta exactamente la gramática de las dos expresiones que emite el adaptador y aplica
// la misma restricción de DynamoDB que rechaza nombres y valores de expresión declarados y no
// usados (ValidationException). Cualquier gramática distinta falla el test en lugar de
// adivinarse.

const (
	e024Tabla   = "NEXUM_CORE_TEST"
	e024Titular = "11111111-2222-4333-8444-555555555555"
	e024Ajeno   = "99999999-8888-4777-8666-555555555555"
)

var e024Ahora = time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)

var (
	errIngresoValorNoUsado    = errors.New("valor de expresion declarado y no usado")
	errIngresoNombreNoUsado   = errors.New("nombre de expresion declarado y no usado")
	errIngresoValorAusente    = errors.New("valor de expresion referenciado y no declarado")
	errIngresoNombreAusente   = errors.New("nombre de expresion referenciado y no declarado")
	errIngresoGramatica       = errors.New("gramatica de expresion no soportada por el doble")
	errIngresoExpresionUpdate = errors.New("expresion de actualizacion no soportada por el doble")
)

var (
	ingresoTokenValor  = regexp.MustCompile(`:[A-Za-z_][A-Za-z0-9_]*`)
	ingresoTokenNombre = regexp.MustCompile(`#[A-Za-z_][A-Za-z0-9_]*`)
)

// memoriaIngreso es el doble con estado de `GetItem`/`UpdateItem`.
type memoriaIngreso struct {
	mu    sync.Mutex
	items map[string]map[string]types.AttributeValue

	lecturas   []*dynamodb.GetItemInput
	escrituras []*dynamodb.UpdateItemInput

	errLectura   error
	errEscritura error

	// alterarRespuesta permite simular una respuesta de DynamoDB incoherente con la escritura
	// (defensa en profundidad del adaptador).
	alterarRespuesta func(map[string]types.AttributeValue)
	// omitirAtributos simula una respuesta sin `Attributes` pese a pedir `ALL_NEW`.
	omitirAtributos bool
}

func newMemoriaIngreso() *memoriaIngreso {
	return &memoriaIngreso{items: make(map[string]map[string]types.AttributeValue)}
}

func (m *memoriaIngreso) sembrar(pk, sk string, attrs map[string]types.AttributeValue) {
	m.mu.Lock()
	defer m.mu.Unlock()
	item := map[string]types.AttributeValue{"PK": strAttr(pk), "SK": strAttr(sk)}
	for nombre, valor := range attrs {
		item[nombre] = valor
	}
	m.items[claveDe(pk, sk)] = item
}

func (m *memoriaIngreso) sembrarIngreso(userID string, attrs map[string]types.AttributeValue) {
	pk, sk := domainDynamo.IncomeKeys(userID)
	m.sembrar(pk, sk, attrs)
}

func (m *memoriaIngreso) leer(pk, sk string) map[string]types.AttributeValue {
	m.mu.Lock()
	defer m.mu.Unlock()
	item, ok := m.items[claveDe(pk, sk)]
	if !ok {
		return nil
	}
	return copiarItem(item)
}

func (m *memoriaIngreso) GetItem(_ context.Context, params *dynamodb.GetItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lecturas = append(m.lecturas, params)
	if m.errLectura != nil {
		return nil, m.errLectura
	}
	item, ok := m.items[claveDe(mapaString(params.Key, "PK"), mapaString(params.Key, "SK"))]
	if !ok {
		return &dynamodb.GetItemOutput{}, nil
	}
	return &dynamodb.GetItemOutput{Item: copiarItem(item)}, nil
}

func (m *memoriaIngreso) UpdateItem(_ context.Context, params *dynamodb.UpdateItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.escrituras = append(m.escrituras, params)

	// DynamoDB valida la petición ANTES de evaluar la condición: un valor de expresión
	// declarado y no usado produce ValidationException y no escribe nada.
	if err := ingresoValidarExpresiones(params); err != nil {
		return nil, err
	}
	if m.errEscritura != nil {
		return nil, m.errEscritura
	}

	pk := mapaString(params.Key, "PK")
	sk := mapaString(params.Key, "SK")
	actual := m.items[claveDe(pk, sk)]

	ok, err := ingresoEvaluarCondicion(
		aws.ToString(params.ConditionExpression),
		actual,
		params.ExpressionAttributeNames,
		params.ExpressionAttributeValues,
	)
	if err != nil {
		return nil, err
	}
	if !ok {
		// Nada se escribe: la atomicidad de la escritura condicional es el punto de la prueba.
		return nil, &types.ConditionalCheckFailedException{Message: aws.String("condicion no cumplida")}
	}

	item := actual
	if item == nil {
		item = map[string]types.AttributeValue{"PK": strAttr(pk), "SK": strAttr(sk)}
	}
	if err := ingresoAplicarActualizacion(params, item); err != nil {
		return nil, err
	}
	m.items[claveDe(pk, sk)] = item

	salida := copiarItem(item)
	if m.alterarRespuesta != nil {
		m.alterarRespuesta(salida)
	}
	if m.omitirAtributos {
		return &dynamodb.UpdateItemOutput{}, nil
	}
	return &dynamodb.UpdateItemOutput{Attributes: salida}, nil
}

// ingresoExpr referencia una expresión y sus alias para validarla.
type ingresoExpr struct {
	texto   string
	nombres map[string]string
	valores map[string]types.AttributeValue
}

func ingresoExpresiones(update *dynamodb.UpdateItemInput) []ingresoExpr {
	expresiones := []ingresoExpr{}
	if condicion := aws.ToString(update.ConditionExpression); condicion != "" {
		expresiones = append(expresiones, ingresoExpr{
			texto:   condicion,
			nombres: update.ExpressionAttributeNames,
			valores: update.ExpressionAttributeValues,
		})
	}
	if actualizacion := aws.ToString(update.UpdateExpression); actualizacion != "" {
		expresiones = append(expresiones, ingresoExpr{
			texto:   actualizacion,
			nombres: update.ExpressionAttributeNames,
			valores: update.ExpressionAttributeValues,
		})
	}
	return expresiones
}

// ingresoValoresReferenciados devuelve los tokens `:x` realmente usados por las expresiones.
func ingresoValoresReferenciados(update *dynamodb.UpdateItemInput) map[string]struct{} {
	usados := map[string]struct{}{}
	for _, expresion := range ingresoExpresiones(update) {
		for _, token := range ingresoTokenValor.FindAllString(expresion.texto, -1) {
			usados[token] = struct{}{}
		}
	}
	return usados
}

func ingresoNombresReferenciados(update *dynamodb.UpdateItemInput) map[string]struct{} {
	usados := map[string]struct{}{}
	for _, expresion := range ingresoExpresiones(update) {
		for _, token := range ingresoTokenNombre.FindAllString(expresion.texto, -1) {
			usados[token] = struct{}{}
		}
	}
	return usados
}

// ingresoValidarExpresiones replica la restricción de DynamoDB: todo valor y todo nombre
// declarados deben usarse, y todo uso debe estar declarado.
func ingresoValidarExpresiones(update *dynamodb.UpdateItemInput) error {
	valoresUsados := ingresoValoresReferenciados(update)
	for declarado := range update.ExpressionAttributeValues {
		if _, ok := valoresUsados[declarado]; !ok {
			return fmt.Errorf("%w: %s", errIngresoValorNoUsado, declarado)
		}
	}
	for usado := range valoresUsados {
		if _, ok := update.ExpressionAttributeValues[usado]; !ok {
			return fmt.Errorf("%w: %s", errIngresoValorAusente, usado)
		}
	}

	nombresUsados := ingresoNombresReferenciados(update)
	for declarado := range update.ExpressionAttributeNames {
		if _, ok := nombresUsados[declarado]; !ok {
			return fmt.Errorf("%w: %s", errIngresoNombreNoUsado, declarado)
		}
	}
	for usado := range nombresUsados {
		if _, ok := update.ExpressionAttributeNames[usado]; !ok {
			return fmt.Errorf("%w: %s", errIngresoNombreAusente, usado)
		}
	}
	return nil
}

// ingresoEvaluarCondicion soporta exactamente la gramática del adaptador: átomos unidos por
// `OR` sin paréntesis (rama de versión 1) o un único átomo de igualdad (versión > 1).
func ingresoEvaluarCondicion(expr string, item map[string]types.AttributeValue, nombres map[string]string, valores map[string]types.AttributeValue) (bool, error) {
	texto := strings.TrimSpace(expr)
	if texto == "" {
		return false, fmt.Errorf("%w: condicion vacia", errIngresoGramatica)
	}
	if strings.Contains(texto, " AND ") {
		return false, fmt.Errorf("%w: %q", errIngresoGramatica, texto)
	}
	for _, alternativa := range strings.Split(texto, " OR ") {
		ok, err := ingresoEvaluarAtomo(strings.TrimSpace(alternativa), item, nombres, valores)
		if err != nil {
			return false, err
		}
		if ok {
			return true, nil
		}
	}
	return false, nil
}

func ingresoEvaluarAtomo(atomo string, item map[string]types.AttributeValue, nombres map[string]string, valores map[string]types.AttributeValue) (bool, error) {
	if strings.HasPrefix(atomo, "attribute_not_exists(") && strings.HasSuffix(atomo, ")") {
		nombre := resolverNombre(strings.TrimSuffix(strings.TrimPrefix(atomo, "attribute_not_exists("), ")"), nombres)
		_, ok := item[nombre]
		return !ok, nil
	}
	partes := strings.SplitN(atomo, " = ", 2)
	if len(partes) != 2 {
		return false, fmt.Errorf("%w: %q", errIngresoGramatica, atomo)
	}
	nombre := resolverNombre(strings.TrimSpace(partes[0]), nombres)
	esperado, ok := valores[strings.TrimSpace(partes[1])]
	if !ok {
		return false, fmt.Errorf("%w: %s", errIngresoValorAusente, partes[1])
	}
	actual, existe := item[nombre]
	if !existe {
		return false, nil
	}
	return atributosIguales(actual, esperado), nil
}

// ingresoAplicarActualizacion interpreta `SET ...` con una parte opcional `ADD ...`.
func ingresoAplicarActualizacion(update *dynamodb.UpdateItemInput, item map[string]types.AttributeValue) error {
	expresion := strings.TrimSpace(aws.ToString(update.UpdateExpression))
	if !strings.HasPrefix(expresion, "SET ") {
		return fmt.Errorf("%w: %q", errIngresoExpresionUpdate, expresion)
	}
	cuerpo := strings.TrimSpace(strings.TrimPrefix(expresion, "SET "))
	var parteAdd string
	if indice := strings.Index(cuerpo, " ADD "); indice >= 0 {
		parteAdd = strings.TrimSpace(cuerpo[indice+len(" ADD "):])
		cuerpo = strings.TrimSpace(cuerpo[:indice])
	}
	for _, asignacion := range strings.Split(cuerpo, ", ") {
		lado := strings.SplitN(asignacion, " = ", 2)
		if len(lado) != 2 {
			return fmt.Errorf("%w: %q", errIngresoExpresionUpdate, asignacion)
		}
		nombre := resolverNombre(strings.TrimSpace(lado[0]), update.ExpressionAttributeNames)
		valor, ok := update.ExpressionAttributeValues[strings.TrimSpace(lado[1])]
		if !ok {
			return fmt.Errorf("%w: %s", errIngresoValorAusente, lado[1])
		}
		item[nombre] = valor
	}
	if parteAdd == "" {
		return nil
	}
	lado := strings.SplitN(parteAdd, " ", 2)
	if len(lado) != 2 {
		return fmt.Errorf("%w: %q", errIngresoExpresionUpdate, parteAdd)
	}
	nombre := resolverNombre(strings.TrimSpace(lado[0]), update.ExpressionAttributeNames)
	delta, ok := update.ExpressionAttributeValues[strings.TrimSpace(lado[1])].(*types.AttributeValueMemberN)
	if !ok {
		return fmt.Errorf("%w: incremento no numerico %q", errIngresoExpresionUpdate, parteAdd)
	}
	salto, err := strconv.ParseInt(delta.Value, 10, 64)
	if err != nil {
		return fmt.Errorf("%w: incremento invalido %q", errIngresoExpresionUpdate, delta.Value)
	}
	var base int64
	if actual, existe := item[nombre].(*types.AttributeValueMemberN); existe {
		if base, err = strconv.ParseInt(actual.Value, 10, 64); err != nil {
			return fmt.Errorf("%w: version no numerica %q", errIngresoExpresionUpdate, actual.Value)
		}
	}
	item[nombre] = numAttr(base + salto)
	return nil
}

func ingresoClaves(conjunto map[string]struct{}) []string {
	claves := make([]string, 0, len(conjunto))
	for clave := range conjunto {
		claves = append(claves, clave)
	}
	sort.Strings(claves)
	return claves
}

func ingresoAtributosDeclarados(valores map[string]types.AttributeValue) []string {
	claves := make([]string, 0, len(valores))
	for clave := range valores {
		claves = append(claves, clave)
	}
	sort.Strings(claves)
	return claves
}

// ---------------- E024: lectura ----------------

// TestE024StoreLeeIngresoDelTitularConConsistenciaFuerte cubre el camino feliz: una única
// lectura consistente sobre `USER#<id>/INCOME`, sin escrituras ni enumeración.
func TestE024StoreLeeIngresoDelTitularConConsistenciaFuerte(t *testing.T) {
	m := newMemoriaIngreso()
	m.sembrarIngreso(e024Titular, map[string]types.AttributeValue{
		"entityType":  strAttr("USER_INCOME"),
		"amountCents": numAttr(250_000),
		"frequency":   strAttr("monthly"),
		"version":     numAttr(3),
	})

	store := dynamoAdapter.NewIncomeStore(m, e024Tabla)
	snapshot, err := store.LoadIncome(context.Background(), "  "+e024Titular+"  ")
	if err != nil {
		t.Fatalf("lectura valida no debe fallar: %v", err)
	}
	if snapshot == nil {
		t.Fatal("se esperaba una instantanea de ingreso")
	}
	if snapshot.AmountCents != 250_000 || snapshot.Version != 3 || snapshot.Frequency != domainIdentity.FrequencyMonthly {
		t.Fatalf("instantanea inesperada: %+v", snapshot)
	}
	if len(m.lecturas) != 1 {
		t.Fatalf("se esperaba exactamente una lectura, hay %d", len(m.lecturas))
	}
	lectura := m.lecturas[0]
	if aws.ToString(lectura.TableName) != e024Tabla {
		t.Fatalf("tabla inesperada: %q", aws.ToString(lectura.TableName))
	}
	if !aws.ToBool(lectura.ConsistentRead) {
		t.Fatal("la lectura del ingreso debe ser consistente")
	}
	pk, sk := domainDynamo.IncomeKeys(e024Titular)
	if mapaString(lectura.Key, "PK") != pk || mapaString(lectura.Key, "SK") != sk {
		t.Fatalf("claves inesperadas: %v", lectura.Key)
	}
	if len(m.escrituras) != 0 {
		t.Fatalf("E024 es solo lectura y no debe escribir, hay %d escrituras", len(m.escrituras))
	}
}

// TestE024StoreSinIngresoNoInventaRecurso cubre el 404: item ausente, clave reutilizada por
// otra entidad y item sin importe se resuelven como «no configurado» (nil, nil).
func TestE024StoreSinIngresoNoInventaRecurso(t *testing.T) {
	casos := []struct {
		nombre string
		attrs  map[string]types.AttributeValue
	}{
		{"item ausente", nil},
		{"entidad ajena en la misma clave", map[string]types.AttributeValue{
			"entityType":  strAttr("USER_CONSENT"),
			"amountCents": numAttr(100),
			"frequency":   strAttr("monthly"),
		}},
		{"sin importe", map[string]types.AttributeValue{
			"entityType": strAttr("USER_INCOME"),
			"frequency":  strAttr("monthly"),
		}},
		{"importe no numerico", map[string]types.AttributeValue{
			"entityType":  strAttr("USER_INCOME"),
			"amountCents": strAttr("no-numerico"),
			"frequency":   strAttr("monthly"),
		}},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			m := newMemoriaIngreso()
			if caso.attrs != nil {
				m.sembrarIngreso(e024Titular, caso.attrs)
			}
			snapshot, err := dynamoAdapter.NewIncomeStore(m, e024Tabla).LoadIncome(context.Background(), e024Titular)
			if err != nil {
				t.Fatalf("no configurado no es un error de servicio: %v", err)
			}
			if snapshot != nil {
				t.Fatalf("no debe inventarse un ingreso: %+v", snapshot)
			}
		})
	}
}

// TestE024StoreFrecuenciaAusenteEsInconsistente falla cerrado en lugar de emitir una
// frecuencia vacía fuera del enum publicado.
func TestE024StoreFrecuenciaAusenteEsInconsistente(t *testing.T) {
	m := newMemoriaIngreso()
	m.sembrarIngreso(e024Titular, map[string]types.AttributeValue{
		"entityType":  strAttr("USER_INCOME"),
		"amountCents": numAttr(120_000),
		"frequency":   strAttr("   "),
		"version":     numAttr(2),
	})
	_, err := dynamoAdapter.NewIncomeStore(m, e024Tabla).LoadIncome(context.Background(), e024Titular)
	if !errors.Is(err, appIdentity.ErrIncomeInconsistent) {
		t.Fatalf("se esperaba ErrIncomeInconsistent, obtenido %v", err)
	}
}

// TestE024StoreItemHeredadoSinVersionEsVersionInicial documenta la ruta histórica: E001 no
// escribe `version` en el ingreso, de modo que la lectura lo interpreta como la versión 1.
func TestE024StoreItemHeredadoSinVersionEsVersionInicial(t *testing.T) {
	m := newMemoriaIngreso()
	m.sembrarIngreso(e024Titular, map[string]types.AttributeValue{
		"entityType":  strAttr("USER_INCOME"),
		"amountCents": numAttr(120_000),
		"frequency":   strAttr("fortnightly"),
	})

	snapshot, err := dynamoAdapter.NewIncomeStore(m, e024Tabla).LoadIncome(context.Background(), e024Titular)
	if err != nil {
		t.Fatalf("lectura valida no debe fallar: %v", err)
	}
	if snapshot.Version != 1 {
		t.Fatalf("un item sin version debe leerse como v1, obtenido %d", snapshot.Version)
	}
	if snapshot.Frequency != domainIdentity.FrequencyFortnightly || snapshot.AmountCents != 120_000 {
		t.Fatalf("instantanea inesperada: %+v", snapshot)
	}
	if len(m.escrituras) != 0 {
		t.Fatal("la lectura no debe reescribir el item historico")
	}
}

// TestE024StoreFallaCerrado demuestra que un fallo de dependencia nunca se degrada a 404 y
// que un despliegue sin cliente o sin tabla no sirve datos.
func TestE024StoreFallaCerrado(t *testing.T) {
	m := newMemoriaIngreso()
	m.errLectura = errors.New("throttling")
	if _, err := dynamoAdapter.NewIncomeStore(m, e024Tabla).LoadIncome(context.Background(), e024Titular); !errors.Is(err, appIdentity.ErrIncomeUnavailable) {
		t.Fatalf("se esperaba ErrIncomeUnavailable, obtenido %v", err)
	}

	if _, err := dynamoAdapter.NewIncomeStore(nil, e024Tabla).LoadIncome(context.Background(), e024Titular); !errors.Is(err, appIdentity.ErrIncomeUnavailable) {
		t.Fatalf("sin cliente debe fallar cerrado, obtenido %v", err)
	}
	if _, err := dynamoAdapter.NewIncomeStore(newMemoriaIngreso(), "   ").LoadIncome(context.Background(), e024Titular); !errors.Is(err, appIdentity.ErrIncomeUnavailable) {
		t.Fatalf("sin tabla debe fallar cerrado, obtenido %v", err)
	}

	sinTitular := newMemoriaIngreso()
	snapshot, err := dynamoAdapter.NewIncomeStore(sinTitular, e024Tabla).LoadIncome(context.Background(), "  ")
	if err != nil || snapshot != nil {
		t.Fatalf("sin titular no hay recurso ni error: snapshot=%v err=%v", snapshot, err)
	}
	if len(sinTitular.lecturas) != 0 {
		t.Fatal("sin titular no debe consultarse DynamoDB")
	}
}

// TestE024StorePerfilExiste es la guarda de E025 contra ingresos huérfanos.
func TestE024StorePerfilExiste(t *testing.T) {
	casos := []struct {
		nombre   string
		attrs    map[string]types.AttributeValue
		esperado bool
	}{
		{"perfil activo", map[string]types.AttributeValue{
			"entityType": strAttr("USER_PROFILE"),
			"status":     strAttr(appIdentity.StatusActive),
		}, true},
		{"perfil revocado", map[string]types.AttributeValue{
			"entityType": strAttr("USER_PROFILE"),
			"status":     strAttr(appIdentity.StatusRevoked),
		}, false},
		{"entidad ajena en la clave del perfil", map[string]types.AttributeValue{
			"entityType": strAttr("USER_CONSENT"),
			"status":     strAttr(appIdentity.StatusActive),
		}, false},
		{"perfil ausente", nil, false},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			m := newMemoriaIngreso()
			if caso.attrs != nil {
				pk, sk := domainDynamo.UserKeys(e024Titular)
				m.sembrar(pk, sk, caso.attrs)
			}
			existe, err := dynamoAdapter.NewIncomeStore(m, e024Tabla).ProfileExists(context.Background(), e024Titular)
			if err != nil {
				t.Fatalf("consulta de perfil valida no debe fallar: %v", err)
			}
			if existe != caso.esperado {
				t.Fatalf("se esperaba existe=%v, obtenido %v", caso.esperado, existe)
			}
			if len(m.lecturas) != 1 {
				t.Fatalf("se esperaba una lectura, hay %d", len(m.lecturas))
			}
			if !aws.ToBool(m.lecturas[0].ConsistentRead) {
				t.Fatal("la comprobacion del perfil debe ser consistente")
			}
			pk, sk := domainDynamo.UserKeys(e024Titular)
			if mapaString(m.lecturas[0].Key, "PK") != pk || mapaString(m.lecturas[0].Key, "SK") != sk {
				t.Fatalf("claves de perfil inesperadas: %v", m.lecturas[0].Key)
			}
		})
	}

	m := newMemoriaIngreso()
	m.errLectura = errors.New("throttling")
	if _, err := dynamoAdapter.NewIncomeStore(m, e024Tabla).ProfileExists(context.Background(), e024Titular); !errors.Is(err, appIdentity.ErrIncomeUnavailable) {
		t.Fatalf("un fallo de DynamoDB no es «no existe»: %v", err)
	}
	if _, err := dynamoAdapter.NewIncomeStore(nil, "").ProfileExists(context.Background(), e024Titular); !errors.Is(err, appIdentity.ErrIncomeUnavailable) {
		t.Fatalf("sin composicion debe fallar cerrado: %v", err)
	}
	vacio := newMemoriaIngreso()
	existe, err := dynamoAdapter.NewIncomeStore(vacio, e024Tabla).ProfileExists(context.Background(), " ")
	if err != nil || existe {
		t.Fatalf("sin titular no hay perfil ni error: existe=%v err=%v", existe, err)
	}
	if len(vacio.lecturas) != 0 {
		t.Fatal("sin titular no debe consultarse DynamoDB")
	}
}

// ---------------- E025: escritura condicional ----------------

func e025Escritura(userID string, amountCents, esperada int64) appIdentity.IncomeStoreWrite {
	return appIdentity.IncomeStoreWrite{
		UserID:                 userID,
		AmountCents:            amountCents,
		Frequency:              domainIdentity.FrequencyMonthly,
		MonthlyEquivalentCents: amountCents,
		ExpectedVersion:        esperada,
		Now:                    e024Ahora,
	}
}

// TestE025StoreCreaConEtagV1 cubre el alta con `If-Match: "v1"`: se escribe UNA sola petición
// condicional, el item nace con los atributos del contrato y la versión AVANZA, de modo que el
// validador devuelto nunca repite el enviado.
func TestE025StoreCreaConEtagV1(t *testing.T) {
	m := newMemoriaIngreso()
	store := dynamoAdapter.NewIncomeStore(m, e024Tabla)

	snapshot, err := store.StoreIncome(context.Background(), e025Escritura(e024Titular, 250_000, 1))
	if err != nil {
		t.Fatalf("el alta con v1 no debe fallar: %v", err)
	}
	if snapshot.AmountCents != 250_000 || snapshot.Frequency != domainIdentity.FrequencyMonthly {
		t.Fatalf("instantanea inesperada: %+v", snapshot)
	}
	if snapshot.Version != 2 {
		t.Fatalf("toda escritura aceptada debe avanzar la version (v1 -> v2), obtenido v%d", snapshot.Version)
	}

	if len(m.escrituras) != 1 {
		t.Fatalf("se esperaba una unica escritura condicional, hay %d", len(m.escrituras))
	}
	escritura := m.escrituras[0]
	if aws.ToString(escritura.TableName) != e024Tabla {
		t.Fatalf("tabla inesperada: %q", aws.ToString(escritura.TableName))
	}
	pk, sk := domainDynamo.IncomeKeys(e024Titular)
	if mapaString(escritura.Key, "PK") != pk || mapaString(escritura.Key, "SK") != sk {
		t.Fatalf("claves inesperadas: %v", escritura.Key)
	}
	if escritura.ReturnValues != types.ReturnValueAllNew {
		t.Fatalf("debe pedirse ALL_NEW para verificar la escritura, obtenido %q", escritura.ReturnValues)
	}
	condicion := aws.ToString(escritura.ConditionExpression)
	for _, rama := range []string{"attribute_not_exists(PK)", "attribute_not_exists(#version)", "#version = :expected"} {
		if !strings.Contains(condicion, rama) {
			t.Fatalf("la condicion con v1 debe incluir %q: %q", rama, condicion)
		}
	}
	if escritura.ExpressionAttributeNames["#version"] != "version" {
		t.Fatalf("el alias #version debe resolver a version: %v", escritura.ExpressionAttributeNames)
	}

	guardado := m.leer(pk, sk)
	if guardado == nil {
		t.Fatal("el ingreso no se persistio")
	}
	if got := guardado["amountCents"].(*types.AttributeValueMemberN).Value; got != "250000" {
		t.Fatalf("amountCents persistido inesperado: %s", got)
	}
	if got := guardado["currency"].(*types.AttributeValueMemberS).Value; got != "MXN" {
		t.Fatalf("currency persistida inesperada: %s", got)
	}
	if got := guardado["frequency"].(*types.AttributeValueMemberS).Value; got != "monthly" {
		t.Fatalf("frequency persistida inesperada: %s", got)
	}
	if got := guardado["monthlyEquivalentCents"].(*types.AttributeValueMemberN).Value; got != "250000" {
		t.Fatalf("monthlyEquivalentCents persistido inesperado: %s", got)
	}
	if got := guardado["updatedAt"].(*types.AttributeValueMemberS).Value; got != e024Ahora.Format(time.RFC3339) {
		t.Fatalf("updatedAt inesperado: %s", got)
	}
	if got := guardado["version"].(*types.AttributeValueMemberN).Value; got != "2" {
		t.Fatalf("version persistida inesperada: %s", got)
	}
}

// TestE025StoreCasObsoletoNoEscribe cubre el 412: la version enviada no es la vigente y NADA
// se modifica (ni importe, ni versión, ni marca de tiempo).
func TestE025StoreCasObsoletoNoEscribe(t *testing.T) {
	m := newMemoriaIngreso()
	m.sembrarIngreso(e024Titular, map[string]types.AttributeValue{
		"entityType":             strAttr("USER_INCOME"),
		"amountCents":            numAttr(100_000),
		"currency":               strAttr("MXN"),
		"frequency":              strAttr("monthly"),
		"monthlyEquivalentCents": numAttr(100_000),
		"updatedAt":              strAttr("2026-11-01T00:00:00Z"),
		"version":                numAttr(3),
	})
	pk, sk := domainDynamo.IncomeKeys(e024Titular)
	antes := m.leer(pk, sk)

	_, err := dynamoAdapter.NewIncomeStore(m, e024Tabla).StoreIncome(context.Background(), e025Escritura(e024Titular, 999_999, 2))
	if !errors.Is(err, appIdentity.ErrIncomeVersionConflict) {
		t.Fatalf("se esperaba ErrIncomeVersionConflict (412), obtenido %v", err)
	}

	despues := m.leer(pk, sk)
	if !reflect.DeepEqual(antes, despues) {
		t.Fatalf("una condicion fallida no debe modificar el item:\nantes=%v\ndespues=%v", antes, despues)
	}
}

// TestE025StoreConflictoConcurrenteEsConflict cubre el 409: un conflicto transitorio no se
// confunde con una versión desactualizada ni con una caída.
func TestE025StoreConflictoConcurrenteEsConflict(t *testing.T) {
	m := newMemoriaIngreso()
	m.errEscritura = &types.TransactionConflictException{Message: aws.String("conflicto transitorio")}

	_, err := dynamoAdapter.NewIncomeStore(m, e024Tabla).StoreIncome(context.Background(), e025Escritura(e024Titular, 100, 1))
	if !errors.Is(err, appIdentity.ErrIncomeConflict) {
		t.Fatalf("se esperaba ErrIncomeConflict (409), obtenido %v", err)
	}
}

// TestE025StoreItemHeredadoSinVersionAvanzaVersion documenta la ruta normal del primer PUT:
// E001 no escribe `version`, así que el item histórico se actualiza por la rama
// `attribute_not_exists(#version)` y la versión debe quedar por encima de la leída.
func TestE025StoreItemHeredadoSinVersionAvanzaVersion(t *testing.T) {
	m := newMemoriaIngreso()
	// Forma EXACTA que escribe E001 V6.0.2 en `USER#<id>/INCOME`: sin `version`.
	m.sembrarIngreso(e024Titular, map[string]types.AttributeValue{
		"entityType":             strAttr("USER_INCOME"),
		"userId":                 strAttr(e024Titular),
		"currency":               strAttr("MXN"),
		"amountCents":            numAttr(120_000),
		"frequency":              strAttr("monthly"),
		"monthlyEquivalentCents": numAttr(120_000),
		"createdAt":              strAttr("2026-11-10T00:00:00Z"),
		"updatedAt":              strAttr("2026-11-10T00:00:00Z"),
	})

	snapshot, err := dynamoAdapter.NewIncomeStore(m, e024Tabla).StoreIncome(context.Background(), e025Escritura(e024Titular, 250_000, 1))
	if err != nil {
		t.Fatalf("el primer PUT sobre un item historico debe aceptarse: %v", err)
	}
	if snapshot.Version != 2 {
		t.Fatalf("el primer PUT debe avanzar la version (v1 -> v2), obtenido v%d", snapshot.Version)
	}
	pk, sk := domainDynamo.IncomeKeys(e024Titular)
	if got := m.leer(pk, sk)["version"].(*types.AttributeValueMemberN).Value; got != "2" {
		t.Fatalf("version persistida inesperada: %s", got)
	}
}

// TestE025StoreDosEscritoresConMismoEtagSoloUnoGana cubre E025 T07 («carrera de versiones no
// pierde actualizaciones»): dos escritores que leyeron el MISMO ETag no pueden aplicar ambos.
func TestE025StoreDosEscritoresConMismoEtagSoloUnoGana(t *testing.T) {
	m := newMemoriaIngreso()
	m.sembrarIngreso(e024Titular, map[string]types.AttributeValue{
		"entityType":             strAttr("USER_INCOME"),
		"amountCents":            numAttr(120_000),
		"currency":               strAttr("MXN"),
		"frequency":              strAttr("monthly"),
		"monthlyEquivalentCents": numAttr(120_000),
	})
	store := dynamoAdapter.NewIncomeStore(m, e024Tabla)

	if _, err := store.StoreIncome(context.Background(), e025Escritura(e024Titular, 150_000, 1)); err != nil {
		t.Fatalf("el primer escritor debe ganar: %v", err)
	}

	_, err := store.StoreIncome(context.Background(), e025Escritura(e024Titular, 999_999, 1))
	if !errors.Is(err, appIdentity.ErrIncomeVersionConflict) {
		t.Fatalf("el segundo escritor con el ETag v1 vencido debe recibir 412, obtenido %v", err)
	}

	pk, sk := domainDynamo.IncomeKeys(e024Titular)
	final := m.leer(pk, sk)
	if got := final["amountCents"].(*types.AttributeValueMemberN).Value; got != "150000" {
		t.Fatalf("la escritura perdida no debe aplicarse; amountCents final: %s", got)
	}
	if got := final["version"].(*types.AttributeValueMemberN).Value; got != "2" {
		t.Fatalf("version final inesperada: %s", got)
	}
}

// TestE025StoreAislaUsuarios demuestra que la escritura sólo alcanza la partición del titular.
func TestE025StoreAislaUsuarios(t *testing.T) {
	m := newMemoriaIngreso()
	m.sembrarIngreso(e024Ajeno, map[string]types.AttributeValue{
		"entityType":  strAttr("USER_INCOME"),
		"amountCents": numAttr(777_700),
		"frequency":   strAttr("annual"),
		"version":     numAttr(5),
	})

	store := dynamoAdapter.NewIncomeStore(m, e024Tabla)
	if _, err := store.StoreIncome(context.Background(), e025Escritura(e024Titular, 250_000, 1)); err != nil {
		t.Fatalf("escritura del titular fallo: %v", err)
	}

	ajeno, err := store.LoadIncome(context.Background(), e024Ajeno)
	if err != nil {
		t.Fatalf("lectura ajena fallo: %v", err)
	}
	if ajeno.AmountCents != 777_700 || ajeno.Version != 5 || ajeno.Frequency != domainIdentity.FrequencyAnnual {
		t.Fatalf("el ingreso de otro titular no debe modificarse: %+v", ajeno)
	}

	pkAjeno, skAjeno := domainDynamo.IncomeKeys(e024Ajeno)
	pkTitular, skTitular := domainDynamo.IncomeKeys(e024Titular)
	if pkAjeno == pkTitular || skAjeno != skTitular {
		t.Fatalf("las claves deben vivir en particiones distintas: %q vs %q", pkAjeno, pkTitular)
	}
	if len(m.escrituras) != 1 {
		t.Fatalf("solo debe existir una escritura, hay %d", len(m.escrituras))
	}
	if mapaString(m.escrituras[0].Key, "PK") != pkTitular {
		t.Fatalf("la escritura debe ir a la particion del titular: %v", m.escrituras[0].Key)
	}
}

// TestE025StoreNoExponeScan comprueba de forma explícita que la interfaz del adaptador no
// permite enumerar la tabla: ni Scan, ni Query, ni transacciones multiitem.
func TestE025StoreNoExponeScan(t *testing.T) {
	tipo := reflect.TypeOf((*dynamoAdapter.IncomeAPI)(nil)).Elem()
	metodos := make([]string, 0, tipo.NumMethod())
	for i := 0; i < tipo.NumMethod(); i++ {
		nombre := tipo.Method(i).Name
		metodos = append(metodos, nombre)
		switch strings.ToLower(nombre) {
		case "scan", "query", "transactwriteitems":
			t.Fatalf("la interfaz del ingreso no debe exponer %s", nombre)
		}
	}
	sort.Strings(metodos)
	esperados := []string{"GetItem", "UpdateItem"}
	if !reflect.DeepEqual(metodos, esperados) {
		t.Fatalf("superficie inesperada del adaptador: %v (esperada %v)", metodos, esperados)
	}
}

// TestE025StoreTodaExpresionDeclaraSoloValoresUsados cubre la restricción de DynamoDB que este
// proyecto ya incumplió una vez: un valor declarado y no usado produce ValidationException.
// El doble valida CADA petición del adaptador; aquí además se compara el conjunto real y se
// demuestra que el validador rechaza un valor sobrante, un valor ausente y un nombre sobrante.
func TestE025StoreTodaExpresionDeclaraSoloValoresUsados(t *testing.T) {
	m := newMemoriaIngreso()
	if _, err := dynamoAdapter.NewIncomeStore(m, e024Tabla).StoreIncome(context.Background(), e025Escritura(e024Titular, 250_000, 1)); err != nil {
		t.Fatalf("escritura valida no debe fallar: %v", err)
	}
	if len(m.escrituras) != 1 {
		t.Fatalf("se esperaba una escritura, hay %d", len(m.escrituras))
	}
	escritura := m.escrituras[0]
	usados := ingresoClaves(ingresoValoresReferenciados(escritura))
	declarados := ingresoAtributosDeclarados(escritura.ExpressionAttributeValues)
	if !reflect.DeepEqual(usados, declarados) {
		t.Fatalf("valores declarados y usados deben coincidir exactamente:\nusados=%v\ndeclarados=%v", usados, declarados)
	}
	for _, obligatorio := range []string{":amount", ":currency", ":frequency", ":monthly", ":updated", ":expected"} {
		if _, ok := escritura.ExpressionAttributeValues[obligatorio]; !ok {
			t.Fatalf("falta el valor de expresion %s: %v", obligatorio, declarados)
		}
	}
	for _, nombre := range ingresoClaves(ingresoNombresReferenciados(escritura)) {
		if _, ok := escritura.ExpressionAttributeNames[nombre]; !ok {
			t.Fatalf("el nombre %s se usa y no esta declarado", nombre)
		}
	}

	// Valor declarado y no usado: se rechaza sin escribir.
	conSobrante := &dynamodb.UpdateItemInput{
		TableName:           aws.String(e024Tabla),
		Key:                 escritura.Key,
		UpdateExpression:    escritura.UpdateExpression,
		ConditionExpression: escritura.ConditionExpression,
		ExpressionAttributeNames: map[string]string{
			"#version": "version",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{},
		ReturnValues:              types.ReturnValueAllNew,
	}
	for nombre, valor := range escritura.ExpressionAttributeValues {
		conSobrante.ExpressionAttributeValues[nombre] = valor
	}
	conSobrante.ExpressionAttributeValues[":sobrante"] = numAttr(42)
	if _, err := m.UpdateItem(context.Background(), conSobrante); !errors.Is(err, errIngresoValorNoUsado) {
		t.Fatalf("un valor declarado y no usado debe rechazarse, obtenido %v", err)
	}

	// Valor usado y no declarado.
	sinValor := &dynamodb.UpdateItemInput{
		TableName:           aws.String(e024Tabla),
		Key:                 escritura.Key,
		UpdateExpression:    escritura.UpdateExpression,
		ConditionExpression: escritura.ConditionExpression,
		ExpressionAttributeNames: map[string]string{
			"#version": "version",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{},
		ReturnValues:              types.ReturnValueAllNew,
	}
	for nombre, valor := range escritura.ExpressionAttributeValues {
		if nombre == ":currency" {
			continue
		}
		sinValor.ExpressionAttributeValues[nombre] = valor
	}
	if _, err := m.UpdateItem(context.Background(), sinValor); !errors.Is(err, errIngresoValorAusente) {
		t.Fatalf("un valor usado y no declarado debe rechazarse, obtenido %v", err)
	}

	// Nombre declarado y no usado.
	conNombreSobrante := &dynamodb.UpdateItemInput{
		TableName:           aws.String(e024Tabla),
		Key:                 escritura.Key,
		UpdateExpression:    escritura.UpdateExpression,
		ConditionExpression: escritura.ConditionExpression,
		ExpressionAttributeNames: map[string]string{
			"#version": "version",
			"#extra":   "extra",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{},
		ReturnValues:              types.ReturnValueAllNew,
	}
	for nombre, valor := range escritura.ExpressionAttributeValues {
		conNombreSobrante.ExpressionAttributeValues[nombre] = valor
	}
	if _, err := m.UpdateItem(context.Background(), conNombreSobrante); !errors.Is(err, errIngresoNombreNoUsado) {
		t.Fatalf("un nombre declarado y no usado debe rechazarse, obtenido %v", err)
	}
}

// TestE025StoreFallaCerrado cubre argumentos inválidos, dependencia caída y una respuesta
// incoherente con lo escrito (defensa en profundidad del adaptador).
func TestE025StoreFallaCerrado(t *testing.T) {
	casosInvalidos := []struct {
		nombre string
		write  appIdentity.IncomeStoreWrite
	}{
		{"sin titular", appIdentity.IncomeStoreWrite{AmountCents: 100, Frequency: domainIdentity.FrequencyMonthly, ExpectedVersion: 1}},
		{"importe cero", e025Escritura(e024Titular, 0, 1)},
		{"importe negativo", e025Escritura(e024Titular, -1, 1)},
		{"sin frecuencia", appIdentity.IncomeStoreWrite{UserID: e024Titular, AmountCents: 100, ExpectedVersion: 1}},
		{"version cero", e025Escritura(e024Titular, 100, 0)},
	}
	for _, caso := range casosInvalidos {
		t.Run("arg/"+caso.nombre, func(t *testing.T) {
			m := newMemoriaIngreso()
			_, err := dynamoAdapter.NewIncomeStore(m, e024Tabla).StoreIncome(context.Background(), caso.write)
			if !errors.Is(err, appIdentity.ErrIncomeInvalidRequest) {
				t.Fatalf("se esperaba ErrIncomeInvalidRequest, obtenido %v", err)
			}
			if len(m.escrituras) != 0 {
				t.Fatal("una solicitud invalida no debe llegar a DynamoDB")
			}
		})
	}

	if _, err := dynamoAdapter.NewIncomeStore(nil, e024Tabla).StoreIncome(context.Background(), e025Escritura(e024Titular, 100, 1)); !errors.Is(err, appIdentity.ErrIncomeUnavailable) {
		t.Fatalf("sin cliente debe fallar cerrado, obtenido %v", err)
	}
	if _, err := dynamoAdapter.NewIncomeStore(newMemoriaIngreso(), "").StoreIncome(context.Background(), e025Escritura(e024Titular, 100, 1)); !errors.Is(err, appIdentity.ErrIncomeUnavailable) {
		t.Fatalf("sin tabla debe fallar cerrado, obtenido %v", err)
	}

	caido := newMemoriaIngreso()
	caido.errEscritura = errors.New("connection refused")
	if _, err := dynamoAdapter.NewIncomeStore(caido, e024Tabla).StoreIncome(context.Background(), e025Escritura(e024Titular, 100, 1)); !errors.Is(err, appIdentity.ErrIncomeUnavailable) {
		t.Fatalf("una caida de dependencia debe ser ErrIncomeUnavailable, obtenido %v", err)
	}

	incoherente := newMemoriaIngreso()
	incoherente.alterarRespuesta = func(attrs map[string]types.AttributeValue) {
		attrs["amountCents"] = numAttr(1)
	}
	if _, err := dynamoAdapter.NewIncomeStore(incoherente, e024Tabla).StoreIncome(context.Background(), e025Escritura(e024Titular, 250_000, 1)); !errors.Is(err, appIdentity.ErrIncomeInconsistent) {
		t.Fatalf("un importe devuelto distinto del escrito debe ser ErrIncomeInconsistent, obtenido %v", err)
	}

	frecuenciaRara := newMemoriaIngreso()
	frecuenciaRara.alterarRespuesta = func(attrs map[string]types.AttributeValue) {
		attrs["frequency"] = strAttr("yearly")
	}
	if _, err := dynamoAdapter.NewIncomeStore(frecuenciaRara, e024Tabla).StoreIncome(context.Background(), e025Escritura(e024Titular, 250_000, 1)); !errors.Is(err, appIdentity.ErrIncomeInconsistent) {
		t.Fatalf("una frecuencia devuelta no canonica debe ser ErrIncomeInconsistent, obtenido %v", err)
	}

	sinVersion := newMemoriaIngreso()
	sinVersion.alterarRespuesta = func(attrs map[string]types.AttributeValue) {
		attrs["version"] = numAttr(0)
	}
	if _, err := dynamoAdapter.NewIncomeStore(sinVersion, e024Tabla).StoreIncome(context.Background(), e025Escritura(e024Titular, 250_000, 1)); !errors.Is(err, appIdentity.ErrIncomeUnavailable) {
		t.Fatalf("una version devuelta invalida debe fallar cerrado, obtenido %v", err)
	}

	vacio := newMemoriaIngreso()
	vacio.omitirAtributos = true
	if _, err := dynamoAdapter.NewIncomeStore(vacio, e024Tabla).StoreIncome(context.Background(), e025Escritura(e024Titular, 250_000, 1)); !errors.Is(err, appIdentity.ErrIncomeUnavailable) {
		t.Fatalf("una respuesta sin atributos debe fallar cerrado, obtenido %v", err)
	}
}

// El adaptador satisface el puerto que consume la aplicación.
var _ appIdentity.IncomeStore = (*dynamoAdapter.IncomeStore)(nil)

// El doble satisface la superficie mínima que declara el adaptador.
var _ dynamoAdapter.IncomeAPI = (*memoriaIngreso)(nil)
