package dynamo_test

import (
	"context"
	"errors"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// purgaDynamo es un doble con estado para E014. Interpreta las MISMAS condiciones y
// actualizaciones que emite el adaptador real (AND/OR con paréntesis, comparaciones
// numéricas y de texto, attribute_exists/attribute_not_exists, SET con if_not_exists, ADD y
// REMOVE) y la semántica de paginación de DynamoDB: `Limit` se aplica ANTES del filtro y
// `LastEvaluatedKey` aparece si quedan elementos fuera de la ventana.
//
// No es un motor DynamoDB: si aparece una construcción no soportada, el test falla con un
// error explícito en lugar de adivinar. Su propósito es demostrar de extremo a extremo, a
// través del adaptador y del worker reales, el descubrimiento, el fencing, la protección del
// control y la verificación previa a PURGED.
type purgaDynamo struct {
	mu    sync.Mutex
	items map[string]map[string]types.AttributeValue

	// fallarDelete devuelve un error para la clave indicada (fallo transitorio inyectado).
	fallarDelete func(pk, sk string) error
	// ignorarDelete responde éxito pero conserva el ítem (simula un borrado que no surte
	// efecto; sirve para probar que VERIFY impide declarar PURGED).
	ignorarDelete map[string]bool
	// carreraDelete simula una transacción concurrente que muta la tabla justo entre la
	// lectura y el borrado de la clave (pk, sk). Se invoca con el mutex tomado: el hook no
	// puede llamar a los métodos públicos del doble, sólo devolver las mutaciones.
	carreraDelete func(pk, sk string) []mutacionPurga

	claims  int
	deletes int
}

// claimsContados devuelve cuántas transacciones aplicaron un `ADD fence` sobre un control.
func (m *purgaDynamo) claimsContados() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.claims
}

func newPurgaDynamo() *purgaDynamo {
	return &purgaDynamo{items: map[string]map[string]types.AttributeValue{}, ignorarDelete: map[string]bool{}}
}

func clavePurga(pk, sk string) string { return pk + "\x00" + sk }

// mutacionPurga describe el efecto de una escritura concurrente simulada: con item nil
// borra la clave; con item no nil la reemplaza.
type mutacionPurga struct {
	pk, sk string
	item   map[string]types.AttributeValue
}

// mutar aplica una escritura directa sobre la tabla del doble. Sirve para preparar estados
// que en producción sólo produce una transacción (TTL vencido, movimiento de un puntero).
func (m *purgaDynamo) mutar(mutaciones ...mutacionPurga) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, mu := range mutaciones {
		if mu.item == nil {
			delete(m.items, clavePurga(mu.pk, mu.sk))
			continue
		}
		item := map[string]types.AttributeValue{"PK": strAttr(mu.pk), "SK": strAttr(mu.sk)}
		for k, v := range mu.item {
			item[k] = v
		}
		m.items[clavePurga(mu.pk, mu.sk)] = item
	}
}

func (m *purgaDynamo) sembrar(pk, sk string, attrs map[string]types.AttributeValue) {
	m.mu.Lock()
	defer m.mu.Unlock()
	item := map[string]types.AttributeValue{
		"PK": strAttr(pk),
		"SK": strAttr(sk),
	}
	for k, v := range attrs {
		item[k] = v
	}
	m.items[clavePurga(pk, sk)] = item
}

func (m *purgaDynamo) leer(pk, sk string) map[string]types.AttributeValue {
	m.mu.Lock()
	defer m.mu.Unlock()
	item, ok := m.items[clavePurga(pk, sk)]
	if !ok {
		return nil
	}
	return copiarItem(item)
}

// instantanea devuelve una copia de todos los ítems (para aserciones globales).
func (m *purgaDynamo) instantanea() []map[string]types.AttributeValue {
	m.mu.Lock()
	defer m.mu.Unlock()
	salida := make([]map[string]types.AttributeValue, 0, len(m.items))
	for _, item := range m.items {
		salida = append(salida, copiarItem(item))
	}
	return salida
}

func (m *purgaDynamo) porParticion(pk string) []map[string]types.AttributeValue {
	var salida []map[string]types.AttributeValue
	for _, item := range m.instantanea() {
		if mapaString(item, "PK") == pk {
			salida = append(salida, item)
		}
	}
	return salida
}

func (m *purgaDynamo) GetItem(_ context.Context, in *dynamodb.GetItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	item, ok := m.items[clavePurga(mapaString(in.Key, "PK"), mapaString(in.Key, "SK"))]
	if !ok {
		return &dynamodb.GetItemOutput{}, nil
	}
	return &dynamodb.GetItemOutput{Item: copiarItem(item)}, nil
}

func (m *purgaDynamo) PutItem(_ context.Context, in *dynamodb.PutItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	pk := mapaString(in.Item, "PK")
	sk := mapaString(in.Item, "SK")
	existente := m.items[clavePurga(pk, sk)]
	if in.ConditionExpression != nil {
		ok, err := evaluarExpresion(aws.ToString(in.ConditionExpression), existente, in.ExpressionAttributeNames, in.ExpressionAttributeValues)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, &types.ConditionalCheckFailedException{Message: aws.String("condition failed")}
		}
	}
	m.items[clavePurga(pk, sk)] = copiarItem(in.Item)
	return &dynamodb.PutItemOutput{}, nil
}

func (m *purgaDynamo) DeleteItem(_ context.Context, in *dynamodb.DeleteItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.DeleteItemOutput, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	pk := mapaString(in.Key, "PK")
	sk := mapaString(in.Key, "SK")
	if m.carreraDelete != nil {
		for _, mu := range m.carreraDelete(pk, sk) {
			if mu.item == nil {
				delete(m.items, clavePurga(mu.pk, mu.sk))
				continue
			}
			item := map[string]types.AttributeValue{"PK": strAttr(mu.pk), "SK": strAttr(mu.sk)}
			for k, v := range mu.item {
				item[k] = v
			}
			m.items[clavePurga(mu.pk, mu.sk)] = item
		}
	}
	existente := m.items[clavePurga(pk, sk)]
	if in.ConditionExpression != nil {
		ok, err := evaluarExpresion(aws.ToString(in.ConditionExpression), existente, in.ExpressionAttributeNames, in.ExpressionAttributeValues)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, &types.ConditionalCheckFailedException{Message: aws.String("condition failed")}
		}
	}
	if m.fallarDelete != nil {
		if err := m.fallarDelete(pk, sk); err != nil {
			return nil, err
		}
	}
	m.deletes++
	if m.ignorarDelete[clavePurga(pk, sk)] {
		return &dynamodb.DeleteItemOutput{}, nil
	}
	delete(m.items, clavePurga(pk, sk))
	return &dynamodb.DeleteItemOutput{}, nil
}

// UpdateItem no lo usa E014 (todas las escrituras son transaccionales); se declara para
// satisfacer la interfaz y falla si un camino lo intentara.
func (m *purgaDynamo) UpdateItem(context.Context, *dynamodb.UpdateItemInput, ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error) {
	return nil, errors.New("update simple no usado por E014")
}

func (m *purgaDynamo) TransactWriteItems(_ context.Context, in *dynamodb.TransactWriteItemsInput, _ ...func(*dynamodb.Options)) (*dynamodb.TransactWriteItemsOutput, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Fase 1: evaluar TODAS las condiciones antes de escribir nada (atomicidad) y devolver
	// una razón por ítem, como DynamoDB.
	for indice, accion := range in.TransactItems {
		ok, err := m.comprobarAccion(accion)
		if err != nil {
			return nil, err
		}
		if !ok {
			razones := make([]types.CancellationReason, len(in.TransactItems))
			for i := range razones {
				razones[i] = types.CancellationReason{Code: aws.String("None")}
			}
			razones[indice] = types.CancellationReason{Code: aws.String("ConditionalCheckFailed")}
			return nil, &types.TransactionCanceledException{
				Message:             aws.String("Transaction cancelled"),
				CancellationReasons: razones,
			}
		}
	}

	// Fase 2: aplicar sobre una copia y confirmar sólo si TODO se aplica (atomicidad también
	// ante un error de interpretación del doble, no sólo ante un fallo de condición).
	nuevos := make(map[string]map[string]types.AttributeValue, len(m.items)+len(in.TransactItems))
	for k, v := range m.items {
		nuevos[k] = v
	}
	for _, accion := range in.TransactItems {
		switch {
		case accion.Put != nil:
			pk := mapaString(accion.Put.Item, "PK")
			sk := mapaString(accion.Put.Item, "SK")
			nuevos[clavePurga(pk, sk)] = copiarItem(accion.Put.Item)
		case accion.Update != nil:
			pk := mapaString(accion.Update.Key, "PK")
			sk := mapaString(accion.Update.Key, "SK")
			item := nuevos[clavePurga(pk, sk)]
			if item == nil {
				item = map[string]types.AttributeValue{"PK": strAttr(pk), "SK": strAttr(sk)}
			} else {
				item = copiarItem(item)
			}
			if err := aplicarActualizacionPurga(item, accion.Update); err != nil {
				return nil, err
			}
			if strings.HasPrefix(sk, "DELETION#") && strings.Contains(aws.ToString(accion.Update.UpdateExpression), "ADD #fence") {
				m.claims++
			}
			nuevos[clavePurga(pk, sk)] = item
		case accion.Delete != nil:
			pk := mapaString(accion.Delete.Key, "PK")
			sk := mapaString(accion.Delete.Key, "SK")
			delete(nuevos, clavePurga(pk, sk))
		case accion.ConditionCheck != nil:
			// Sin escritura: ya se evaluó.
		}
	}
	m.items = nuevos
	return &dynamodb.TransactWriteItemsOutput{}, nil
}

func (m *purgaDynamo) comprobarAccion(accion types.TransactWriteItem) (bool, error) {
	var (
		clave   map[string]types.AttributeValue
		expr    *string
		nombres map[string]string
		valores map[string]types.AttributeValue
	)
	switch {
	case accion.Put != nil:
		clave = map[string]types.AttributeValue{
			"PK": accion.Put.Item["PK"],
			"SK": accion.Put.Item["SK"],
		}
		expr, nombres, valores = accion.Put.ConditionExpression, accion.Put.ExpressionAttributeNames, accion.Put.ExpressionAttributeValues
	case accion.Update != nil:
		clave = accion.Update.Key
		expr, nombres, valores = accion.Update.ConditionExpression, accion.Update.ExpressionAttributeNames, accion.Update.ExpressionAttributeValues
	case accion.Delete != nil:
		clave = accion.Delete.Key
		expr, nombres, valores = accion.Delete.ConditionExpression, accion.Delete.ExpressionAttributeNames, accion.Delete.ExpressionAttributeValues
	case accion.ConditionCheck != nil:
		clave = accion.ConditionCheck.Key
		expr, nombres, valores = accion.ConditionCheck.ConditionExpression, accion.ConditionCheck.ExpressionAttributeNames, accion.ConditionCheck.ExpressionAttributeValues
	}
	if expr == nil {
		return true, nil
	}
	existente := m.items[clavePurga(mapaString(clave, "PK"), mapaString(clave, "SK"))]
	return evaluarExpresion(aws.ToString(expr), existente, nombres, valores)
}

// Query soporta la condición de clave `PK = :pk` (la única que usa E014) y una
// FilterExpression opcional. Reproduce el orden por SK, `ExclusiveStartKey`, la ventana de
// `Limit` aplicada antes del filtro y `LastEvaluatedKey`.
func (m *purgaDynamo) Query(_ context.Context, in *dynamodb.QueryInput, _ ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	condicion := aws.ToString(in.KeyConditionExpression)
	if !strings.HasPrefix(condicion, "PK = ") {
		return nil, errors.New("condicion de clave no soportada por el doble: " + condicion)
	}
	aliasPK := strings.TrimSpace(strings.TrimPrefix(condicion, "PK = "))
	pk := ""
	if valor, ok := in.ExpressionAttributeValues[aliasPK]; ok {
		pk = valorString(valor)
	}

	var candidatos []map[string]types.AttributeValue
	for _, item := range m.items {
		if mapaString(item, "PK") == pk {
			candidatos = append(candidatos, item)
		}
	}
	sort.Slice(candidatos, func(i, j int) bool {
		return mapaString(candidatos[i], "SK") < mapaString(candidatos[j], "SK")
	})

	inicio := 0
	if len(in.ExclusiveStartKey) > 0 {
		sk := mapaString(in.ExclusiveStartKey, "SK")
		for i, item := range candidatos {
			if mapaString(item, "SK") == sk {
				inicio = i + 1
				break
			}
		}
	}
	restantes := candidatos[inicio:]

	limite := len(restantes)
	if in.Limit != nil && int(*in.Limit) < limite {
		limite = int(*in.Limit)
	}
	ventana := restantes[:limite]

	salida := make([]map[string]types.AttributeValue, 0, len(ventana))
	for _, item := range ventana {
		if in.FilterExpression != nil {
			ok, err := evaluarExpresion(aws.ToString(in.FilterExpression), item, in.ExpressionAttributeNames, in.ExpressionAttributeValues)
			if err != nil {
				return nil, err
			}
			if !ok {
				continue
			}
		}
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

// aplicarActualizacionPurga interpreta SET (con if_not_exists), ADD y REMOVE.
func aplicarActualizacionPurga(item map[string]types.AttributeValue, update *types.Update) error {
	expr := aws.ToString(update.UpdateExpression)
	nombres := update.ExpressionAttributeNames
	valores := update.ExpressionAttributeValues

	resto := expr
	var set, add, remove string
	if i := strings.Index(resto, " REMOVE "); i >= 0 {
		remove = resto[i+len(" REMOVE "):]
		resto = resto[:i]
	}
	if i := strings.Index(resto, " ADD "); i >= 0 {
		add = resto[i+len(" ADD "):]
		resto = resto[:i]
	}
	if strings.HasPrefix(strings.TrimSpace(resto), "SET ") {
		set = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(resto), "SET "))
	}

	for _, asignacion := range dividirLista(set) {
		partes := strings.SplitN(asignacion, " = ", 2)
		if len(partes) != 2 {
			return errors.New("SET no soportado: " + asignacion)
		}
		nombre := resolverAlias(strings.TrimSpace(partes[0]), nombres)
		valor, err := resolverValor(strings.TrimSpace(partes[1]), item, nombres, valores)
		if err != nil {
			return err
		}
		item[nombre] = valor
	}

	for _, incremento := range dividirLista(add) {
		partes := strings.Fields(incremento)
		if len(partes) != 2 {
			return errors.New("ADD no soportado: " + incremento)
		}
		nombre := resolverAlias(partes[0], nombres)
		delta, ok := valores[partes[1]].(*types.AttributeValueMemberN)
		if !ok {
			return errors.New("ADD requiere numero: " + incremento)
		}
		salto, err := strconv.ParseInt(delta.Value, 10, 64)
		if err != nil {
			return err
		}
		var base int64
		if actual, ok := item[nombre].(*types.AttributeValueMemberN); ok {
			base, _ = strconv.ParseInt(actual.Value, 10, 64)
		}
		item[nombre] = numAttr(base + salto)
	}

	for _, nombre := range dividirLista(remove) {
		delete(item, resolverAlias(strings.TrimSpace(nombre), nombres))
	}
	return nil
}

// resolverValor interpreta `:valor` y `if_not_exists(#attr, :valor)` (con o sin `+ :x`).
func resolverValor(expresion string, item map[string]types.AttributeValue, nombres map[string]string, valores map[string]types.AttributeValue) (types.AttributeValue, error) {
	if strings.HasPrefix(expresion, "if_not_exists(") {
		cierre := strings.Index(expresion, ")")
		if cierre < 0 {
			return nil, errors.New("if_not_exists sin cierre: " + expresion)
		}
		interior := expresion[len("if_not_exists("):cierre]
		partes := strings.SplitN(interior, ",", 2)
		if len(partes) != 2 {
			return nil, errors.New("if_not_exists invalido: " + expresion)
		}
		nombre := resolverAlias(strings.TrimSpace(partes[0]), nombres)
		defecto, ok := valores[strings.TrimSpace(partes[1])]
		if !ok {
			return nil, errors.New("if_not_exists sin valor por defecto: " + expresion)
		}
		base := defecto
		if existente, hay := item[nombre]; hay {
			base = existente
		}
		resto := strings.TrimSpace(expresion[cierre+1:])
		if resto == "" {
			return base, nil
		}
		if !strings.HasPrefix(resto, "+") {
			return nil, errors.New("operacion no soportada: " + expresion)
		}
		alias := strings.TrimSpace(strings.TrimPrefix(resto, "+"))
		delta, ok := valores[alias].(*types.AttributeValueMemberN)
		if !ok {
			return nil, errors.New("suma sin numero: " + expresion)
		}
		baseNum, ok := base.(*types.AttributeValueMemberN)
		if !ok {
			return nil, errors.New("suma sobre no numerico: " + expresion)
		}
		a, _ := strconv.ParseInt(baseNum.Value, 10, 64)
		b, _ := strconv.ParseInt(delta.Value, 10, 64)
		return numAttr(a + b), nil
	}
	if valor, ok := valores[expresion]; ok {
		return valor, nil
	}
	return nil, errors.New("valor no soportado: " + expresion)
}

// evaluarExpresion interpreta la gramática booleana de condiciones y filtros.
func evaluarExpresion(expr string, item map[string]types.AttributeValue, nombres map[string]string, valores map[string]types.AttributeValue) (bool, error) {
	tokens := tokenizarPurga(expr)
	// DynamoDB rechaza con ValidationException un alias de nombre no declarado. El doble lo
	// replica para que un `#atributo` olvidado no se interprete como "atributo ausente".
	for _, token := range tokens {
		if strings.HasPrefix(token, "#") {
			if _, ok := nombres[token]; !ok {
				return false, errors.New("alias de nombre no declarado: " + token)
			}
		}
	}
	p := &parserPurga{tokens: tokens, item: item, nombres: nombres, valores: valores}
	resultado, err := p.parseOr()
	if err != nil {
		return false, err
	}
	if p.pos != len(p.tokens) {
		return false, errors.New("tokens sobrantes en expresion: " + expr)
	}
	return resultado, nil
}

type parserPurga struct {
	tokens  []string
	pos     int
	item    map[string]types.AttributeValue
	nombres map[string]string
	valores map[string]types.AttributeValue
}

func (p *parserPurga) mirar() string {
	if p.pos >= len(p.tokens) {
		return ""
	}
	return p.tokens[p.pos]
}

func (p *parserPurga) consumir() string {
	token := p.mirar()
	p.pos++
	return token
}

func (p *parserPurga) parseOr() (bool, error) {
	izquierda, err := p.parseAnd()
	if err != nil {
		return false, err
	}
	for strings.EqualFold(p.mirar(), "OR") {
		p.consumir()
		derecha, err := p.parseAnd()
		if err != nil {
			return false, err
		}
		izquierda = izquierda || derecha
	}
	return izquierda, nil
}

func (p *parserPurga) parseAnd() (bool, error) {
	izquierda, err := p.parseFactor()
	if err != nil {
		return false, err
	}
	for strings.EqualFold(p.mirar(), "AND") {
		p.consumir()
		derecha, err := p.parseFactor()
		if err != nil {
			return false, err
		}
		izquierda = izquierda && derecha
	}
	return izquierda, nil
}

func (p *parserPurga) parseFactor() (bool, error) {
	if p.mirar() == "(" {
		p.consumir()
		valor, err := p.parseOr()
		if err != nil {
			return false, err
		}
		if p.mirar() != ")" {
			return false, errors.New("falta parentesis de cierre")
		}
		p.consumir()
		return valor, nil
	}
	return p.parseAtomo()
}

func (p *parserPurga) parseAtomo() (bool, error) {
	nombreFuncion := p.consumir()
	switch strings.ToLower(nombreFuncion) {
	case "attribute_exists", "attribute_not_exists":
		if p.consumir() != "(" {
			return false, errors.New("falta ( en " + nombreFuncion)
		}
		nombre := resolverAlias(p.consumir(), p.nombres)
		if p.consumir() != ")" {
			return false, errors.New("falta ) en " + nombreFuncion)
		}
		_, existe := p.item[nombre]
		if strings.ToLower(nombreFuncion) == "attribute_exists" {
			return existe, nil
		}
		return !existe, nil
	case "begins_with":
		if p.consumir() != "(" {
			return false, errors.New("falta ( en begins_with")
		}
		nombre := resolverAlias(p.consumir(), p.nombres)
		if p.consumir() != "," {
			return false, errors.New("falta , en begins_with")
		}
		prefijo, ok := p.valores[p.consumir()]
		if !ok {
			return false, errors.New("valor no encontrado en begins_with")
		}
		if p.consumir() != ")" {
			return false, errors.New("falta ) en begins_with")
		}
		return strings.HasPrefix(valorString(p.item[nombre]), valorString(prefijo)), nil
	}

	nombre := resolverAlias(nombreFuncion, p.nombres)
	operador := p.consumir()
	aliasValor := p.consumir()
	esperado, ok := p.valores[aliasValor]
	if !ok {
		return false, errors.New("valor no declarado: " + aliasValor)
	}
	return compararPurga(p.item[nombre], operador, esperado)
}

func compararPurga(actual types.AttributeValue, operador string, esperado types.AttributeValue) (bool, error) {
	if actual == nil {
		// Un atributo ausente incumple cualquier comparación, incluido `<>`.
		return false, nil
	}
	switch operador {
	case "=":
		return igualesPurga(actual, esperado), nil
	case "<>":
		return !igualesPurga(actual, esperado), nil
	case "<", "<=", ">", ">=":
		if a, ok := actual.(*types.AttributeValueMemberN); ok {
			b, ok := esperado.(*types.AttributeValueMemberN)
			if !ok {
				return false, errors.New("comparacion numerica con no numerico")
			}
			x, err := strconv.ParseInt(a.Value, 10, 64)
			if err != nil {
				return false, err
			}
			y, err := strconv.ParseInt(b.Value, 10, 64)
			if err != nil {
				return false, err
			}
			return compararOrden(x < y, x == y, x > y, operador), nil
		}
		x, y := valorString(actual), valorString(esperado)
		return compararOrden(x < y, x == y, x > y, operador), nil
	default:
		return false, errors.New("operador no soportado: " + operador)
	}
}

func compararOrden(menor, igual, mayor bool, operador string) bool {
	switch operador {
	case "<":
		return menor
	case "<=":
		return menor || igual
	case ">":
		return mayor
	case ">=":
		return mayor || igual
	}
	return false
}

func igualesPurga(a, b types.AttributeValue) bool {
	switch valor := a.(type) {
	case *types.AttributeValueMemberS:
		otro, ok := b.(*types.AttributeValueMemberS)
		return ok && valor.Value == otro.Value
	case *types.AttributeValueMemberN:
		otro, ok := b.(*types.AttributeValueMemberN)
		return ok && valor.Value == otro.Value
	case *types.AttributeValueMemberBOOL:
		otro, ok := b.(*types.AttributeValueMemberBOOL)
		return ok && valor.Value == otro.Value
	}
	return false
}

func resolverAlias(token string, nombres map[string]string) string {
	limpio := strings.TrimSpace(token)
	if alias, ok := nombres[limpio]; ok {
		return alias
	}
	return limpio
}

func valorString(valor types.AttributeValue) string {
	if texto, ok := valor.(*types.AttributeValueMemberS); ok {
		return texto.Value
	}
	return ""
}

func dividirLista(texto string) []string {
	texto = strings.TrimSpace(texto)
	if texto == "" {
		return nil
	}
	// Divide por comas de NIVEL SUPERIOR: `if_not_exists(#phase, :collect)` contiene una coma
	// interna que no separa asignaciones.
	var partes []string
	profundidad := 0
	var actual strings.Builder
	for _, r := range texto {
		switch r {
		case '(':
			profundidad++
			actual.WriteRune(r)
		case ')':
			profundidad--
			actual.WriteRune(r)
		case ',':
			if profundidad > 0 {
				actual.WriteRune(r)
				continue
			}
			partes = append(partes, actual.String())
			actual.Reset()
		default:
			actual.WriteRune(r)
		}
	}
	partes = append(partes, actual.String())

	salida := make([]string, 0, len(partes))
	for _, parte := range partes {
		if limpio := strings.TrimSpace(parte); limpio != "" {
			salida = append(salida, limpio)
		}
	}
	return salida
}

// tokenizarPurga separa paréntesis, comas, operadores y palabras.
func tokenizarPurga(expr string) []string {
	var tokens []string
	var actual strings.Builder
	cerrar := func() {
		if actual.Len() > 0 {
			tokens = append(tokens, actual.String())
			actual.Reset()
		}
	}
	for _, r := range expr {
		switch r {
		case ' ', '\t', '\n', '\r':
			cerrar()
		case '(', ')', ',':
			cerrar()
			tokens = append(tokens, string(r))
		case '=', '<', '>':
			cerrar()
			tokens = append(tokens, string(r))
		default:
			actual.WriteRune(r)
		}
	}
	cerrar()

	// Une operadores de dos caracteres (`<=`, `>=`, `<>`).
	unidos := make([]string, 0, len(tokens))
	for i := 0; i < len(tokens); i++ {
		if (tokens[i] == "<" || tokens[i] == ">") && i+1 < len(tokens) && (tokens[i+1] == "=" || tokens[i+1] == ">") {
			unidos = append(unidos, tokens[i]+tokens[i+1])
			i++
			continue
		}
		unidos = append(unidos, tokens[i])
	}
	return unidos
}
