package main

// Pruebas del arranque del scheduler durable de recordatorios.
//
// El doble está en el borde del SDK de DynamoDB: por encima corren los adaptadores
// REALES (ScheduleStore y PushOutboxStore), el Servicio REAL de outbox y el
// Scheduler real. Así se comprueba el cableado completo sin AWS: reclamar una
// ocurrencia, encolarla en OUTBOX#GLOBAL y cerrarla desactivando el recordatorio.
//
// No se registra ninguna ruta: el scheduler no es un endpoint y este archivo no
// toca `main.go`.

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
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
	"github.com/nexumpr/Nexum-Backend/internal/application/pushoutbox"
	appReminders "github.com/nexumpr/Nexum-Backend/internal/application/reminders"
	nexumhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
)

const schedulerTablaPrueba = "NEXUM_CORE_TEST"

func schedAttrS(valor string) types.AttributeValue { return &types.AttributeValueMemberS{Value: valor} }
func schedAttrN(valor int64) types.AttributeValue {
	return &types.AttributeValueMemberN{Value: strconv.FormatInt(valor, 10)}
}
func schedAttrB(valor bool) types.AttributeValue {
	return &types.AttributeValueMemberBOOL{Value: valor}
}

// schedFakeSDK implementa ScheduleAPI y PushOutboxAPI sobre un mapa.
//
// Está endurecido a propósito: evalúa condiciones y aplica los `Update`
// (SET/REMOVE/ADD) como DynamoDB, con transacciones todo-o-nada y una razón de
// cancelación por acción. Una expresión que no reconoce NO se da por buena:
// devuelve error en lugar de fingir que la condición se cumplió. Sin esto la
// cadena «real» podría pasar por un doble permisivo y no demostraría que
// `Complete` desactiva el recordatorio ni que `Advance` aplica sus `Update`.
type schedFakeSDK struct {
	mu            sync.Mutex
	items         map[string]map[string]types.AttributeValue
	queries       int
	updates       int
	transacciones []*dynamodb.TransactWriteItemsInput
	consultas     []*dynamodb.QueryInput
	// fallarEn hace fallar la N-ésima transacción (1-based) con errorEn. Se usa
	// para provocar un fallo de `Advance` después de encolar sin duplicar.
	fallarEn int
	errorEn  error
}

func nuevoSchedFakeSDK() *schedFakeSDK {
	return &schedFakeSDK{items: map[string]map[string]types.AttributeValue{}}
}

func schedClaveSDK(pk, sk string) string { return pk + "\x00" + sk }

func schedCopiaSDK(item map[string]types.AttributeValue) map[string]types.AttributeValue {
	if item == nil {
		return nil
	}
	copia := make(map[string]types.AttributeValue, len(item))
	for clave, valor := range item {
		copia[clave] = valor
	}
	return copia
}

func (f *schedFakeSDK) poner(item map[string]types.AttributeValue) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.items[schedClaveSDK(textoScheduler(item["PK"]), textoScheduler(item["SK"]))] = schedCopiaSDK(item)
}

// item devuelve una copia del ítem exacto; nil si no existe.
func (f *schedFakeSDK) item(pk, sk string) map[string]types.AttributeValue {
	f.mu.Lock()
	defer f.mu.Unlock()
	return schedCopiaSDK(f.items[schedClaveSDK(pk, sk)])
}

// itemsConPrefijo devuelve copias de los ítems de una partición cuya SK empieza
// por el prefijo, ordenadas por SK.
func (f *schedFakeSDK) itemsConPrefijo(pk, prefijo string) []map[string]types.AttributeValue {
	f.mu.Lock()
	defer f.mu.Unlock()
	salida := make([]map[string]types.AttributeValue, 0)
	for _, item := range f.items {
		if textoScheduler(item["PK"]) == pk && strings.HasPrefix(textoScheduler(item["SK"]), prefijo) {
			salida = append(salida, schedCopiaSDK(item))
		}
	}
	sort.Slice(salida, func(i, j int) bool {
		return textoScheduler(salida[i]["SK"]) < textoScheduler(salida[j]["SK"])
	})
	return salida
}

func (f *schedFakeSDK) Query(_ context.Context, params *dynamodb.QueryInput, _ ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.queries++
	f.consultas = append(f.consultas, params)
	condicion := strings.TrimSpace(aws.ToString(params.KeyConditionExpression))
	const prefijoClave = "#pk = :pk AND "
	if !strings.HasPrefix(condicion, prefijoClave) {
		return nil, fmt.Errorf("condición de Query no soportada por el doble del scheduler: %q", condicion)
	}
	valores := params.ExpressionAttributeValues
	pk := textoScheduler(valores[":pk"])
	resto := strings.TrimPrefix(condicion, prefijoClave)
	var coincide func(sk string) bool
	switch {
	case strings.HasPrefix(resto, "#sk <= "):
		limite := textoScheduler(valores[strings.TrimSpace(strings.TrimPrefix(resto, "#sk <= "))])
		coincide = func(sk string) bool { return sk <= limite }
	case strings.HasPrefix(resto, "begins_with(#sk, "):
		marca := strings.TrimSuffix(strings.TrimPrefix(resto, "begins_with(#sk, "), ")")
		prefijo := textoScheduler(valores[strings.TrimSpace(marca)])
		coincide = func(sk string) bool { return strings.HasPrefix(sk, prefijo) }
	default:
		return nil, fmt.Errorf("condición de Query no soportada por el doble del scheduler: %q", condicion)
	}

	desde := ""
	if len(params.ExclusiveStartKey) > 0 {
		desde = textoScheduler(params.ExclusiveStartKey["SK"])
	}
	claves := make([]string, 0, len(f.items))
	for clave := range f.items {
		claves = append(claves, clave)
	}
	sort.Strings(claves)
	limite := int(aws.ToInt32(params.Limit))
	if limite <= 0 {
		limite = len(claves)
	}
	salida := &dynamodb.QueryOutput{}
	evaluados := 0
	ultima := ""
	for _, clave := range claves {
		item := f.items[clave]
		sk := textoScheduler(item["SK"])
		if textoScheduler(item["PK"]) != pk || !coincide(sk) || sk <= desde {
			continue
		}
		if evaluados >= limite {
			break
		}
		evaluados++
		ultima = sk
		cumple, err := schedEvaluaCondicion(item, aws.ToString(params.FilterExpression), params.ExpressionAttributeNames, params.ExpressionAttributeValues)
		if err != nil {
			return nil, err
		}
		if cumple {
			salida.Items = append(salida.Items, schedCopiaSDK(item))
		}
	}
	if ultima != "" && evaluados >= limite {
		for _, clave := range claves {
			item := f.items[clave]
			if textoScheduler(item["PK"]) == pk && coincide(textoScheduler(item["SK"])) && textoScheduler(item["SK"]) > ultima {
				salida.LastEvaluatedKey = map[string]types.AttributeValue{"PK": schedAttrS(pk), "SK": schedAttrS(ultima)}
				break
			}
		}
	}
	salida.Count = int32(len(salida.Items))
	salida.ScannedCount = int32(evaluados)
	return salida, nil
}

func (f *schedFakeSDK) GetItem(_ context.Context, params *dynamodb.GetItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	clave := schedClaveSDK(textoScheduler(params.Key["PK"]), textoScheduler(params.Key["SK"]))
	return &dynamodb.GetItemOutput{Item: schedCopiaSDK(f.items[clave])}, nil
}

func (f *schedFakeSDK) PutItem(_ context.Context, params *dynamodb.PutItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	clave := schedClaveSDK(textoScheduler(params.Item["PK"]), textoScheduler(params.Item["SK"]))
	cumple, err := schedEvaluaCondicion(f.items[clave], aws.ToString(params.ConditionExpression), params.ExpressionAttributeNames, params.ExpressionAttributeValues)
	if err != nil {
		return nil, err
	}
	if !cumple {
		return nil, &types.ConditionalCheckFailedException{Message: aws.String("condición no cumplida")}
	}
	f.items[clave] = schedCopiaSDK(params.Item)
	return &dynamodb.PutItemOutput{}, nil
}

func (f *schedFakeSDK) UpdateItem(_ context.Context, params *dynamodb.UpdateItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.updates++
	clave := schedClaveSDK(textoScheduler(params.Key["PK"]), textoScheduler(params.Key["SK"]))
	cumple, err := schedEvaluaCondicion(f.items[clave], aws.ToString(params.ConditionExpression), params.ExpressionAttributeNames, params.ExpressionAttributeValues)
	if err != nil {
		return nil, err
	}
	if !cumple {
		return nil, &types.ConditionalCheckFailedException{Message: aws.String("condición no cumplida")}
	}
	item := schedCopiaSDK(f.items[clave])
	if item == nil {
		item = map[string]types.AttributeValue{"PK": params.Key["PK"], "SK": params.Key["SK"]}
	}
	if err := schedAplicaUpdate(item, aws.ToString(params.UpdateExpression), params.ExpressionAttributeNames, params.ExpressionAttributeValues); err != nil {
		return nil, err
	}
	f.items[clave] = item
	salida := &dynamodb.UpdateItemOutput{}
	if params.ReturnValues == types.ReturnValueAllNew {
		salida.Attributes = schedCopiaSDK(item)
	}
	return salida, nil
}

func (f *schedFakeSDK) DeleteItem(_ context.Context, params *dynamodb.DeleteItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.DeleteItemOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	clave := schedClaveSDK(textoScheduler(params.Key["PK"]), textoScheduler(params.Key["SK"]))
	cumple, err := schedEvaluaCondicion(f.items[clave], aws.ToString(params.ConditionExpression), params.ExpressionAttributeNames, params.ExpressionAttributeValues)
	if err != nil {
		return nil, err
	}
	if !cumple {
		return nil, &types.ConditionalCheckFailedException{Message: aws.String("condición no cumplida")}
	}
	delete(f.items, clave)
	return &dynamodb.DeleteItemOutput{}, nil
}

func (f *schedFakeSDK) TransactWriteItems(_ context.Context, params *dynamodb.TransactWriteItemsInput, _ ...func(*dynamodb.Options)) (*dynamodb.TransactWriteItemsOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.transacciones = append(f.transacciones, params)
	if f.fallarEn > 0 && len(f.transacciones) == f.fallarEn {
		if f.errorEn != nil {
			return nil, f.errorEn
		}
		return nil, errors.New("fallo inyectado en la transacción")
	}
	if len(params.TransactItems) == 0 || len(params.TransactItems) > 100 {
		return nil, fmt.Errorf("número de acciones transaccionales inválido: %d", len(params.TransactItems))
	}

	// Todo o nada: se evalúan todas las condiciones antes de aplicar nada.
	vistas := map[string]bool{}
	razones := make([]types.CancellationReason, 0, len(params.TransactItems))
	fallo := false
	for _, accion := range params.TransactItems {
		clave, condicion, nombres, valores, err := schedAccion(accion)
		if err != nil {
			return nil, err
		}
		if vistas[clave] {
			return nil, fmt.Errorf("dos acciones sobre el mismo ítem: %s", clave)
		}
		vistas[clave] = true
		cumple, err := schedEvaluaCondicion(f.items[clave], condicion, nombres, valores)
		if err != nil {
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

	for _, accion := range params.TransactItems {
		clave, _, _, _, _ := schedAccion(accion)
		switch {
		case accion.Put != nil:
			f.items[clave] = schedCopiaSDK(accion.Put.Item)
		case accion.Delete != nil:
			delete(f.items, clave)
		case accion.Update != nil:
			item := f.items[clave]
			if item == nil {
				item = map[string]types.AttributeValue{"PK": accion.Update.Key["PK"], "SK": accion.Update.Key["SK"]}
			}
			if err := schedAplicaUpdate(item, aws.ToString(accion.Update.UpdateExpression), accion.Update.ExpressionAttributeNames, accion.Update.ExpressionAttributeValues); err != nil {
				return nil, err
			}
			f.items[clave] = item
		case accion.ConditionCheck != nil:
			// Comprobación pura: ya se evaluó arriba.
		default:
			return nil, errors.New("acción transaccional no soportada")
		}
	}
	return &dynamodb.TransactWriteItemsOutput{}, nil
}

func (f *schedFakeSDK) stats() (queries, updates, transacciones int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.queries, f.updates, len(f.transacciones)
}

// consultasVistas devuelve copia de las Query emitidas, para afirmar sobre la
// forma de la lectura (condición de rango, límite y consistencia fuerte).
func (f *schedFakeSDK) consultasVistas() []*dynamodb.QueryInput {
	f.mu.Lock()
	defer f.mu.Unlock()
	copia := make([]*dynamodb.QueryInput, len(f.consultas))
	copy(copia, f.consultas)
	return copia
}

func (f *schedFakeSDK) hayPutEnParticion(particion string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, tx := range f.transacciones {
		for _, accion := range tx.TransactItems {
			if accion.Put != nil && textoScheduler(accion.Put.Item["PK"]) == particion {
				return true
			}
		}
	}
	return false
}

func textoScheduler(valor types.AttributeValue) string {
	if v, ok := valor.(*types.AttributeValueMemberS); ok {
		return v.Value
	}
	return ""
}

// schedAccion extrae la clave y la condición de cada acción transaccional.
func schedAccion(accion types.TransactWriteItem) (clave, condicion string, nombres map[string]string, valores map[string]types.AttributeValue, err error) {
	switch {
	case accion.Put != nil:
		clave = schedClaveSDK(textoScheduler(accion.Put.Item["PK"]), textoScheduler(accion.Put.Item["SK"]))
		return clave, aws.ToString(accion.Put.ConditionExpression), accion.Put.ExpressionAttributeNames, accion.Put.ExpressionAttributeValues, nil
	case accion.Delete != nil:
		clave = schedClaveSDK(textoScheduler(accion.Delete.Key["PK"]), textoScheduler(accion.Delete.Key["SK"]))
		return clave, aws.ToString(accion.Delete.ConditionExpression), accion.Delete.ExpressionAttributeNames, accion.Delete.ExpressionAttributeValues, nil
	case accion.Update != nil:
		clave = schedClaveSDK(textoScheduler(accion.Update.Key["PK"]), textoScheduler(accion.Update.Key["SK"]))
		return clave, aws.ToString(accion.Update.ConditionExpression), accion.Update.ExpressionAttributeNames, accion.Update.ExpressionAttributeValues, nil
	case accion.ConditionCheck != nil:
		clave = schedClaveSDK(textoScheduler(accion.ConditionCheck.Key["PK"]), textoScheduler(accion.ConditionCheck.Key["SK"]))
		return clave, aws.ToString(accion.ConditionCheck.ConditionExpression), accion.ConditionCheck.ExpressionAttributeNames, accion.ConditionCheck.ExpressionAttributeValues, nil
	default:
		return "", "", nil, nil, errors.New("acción transaccional vacía")
	}
}

// schedPartes separa por `separador` ignorando lo que está dentro de paréntesis.
func schedPartes(expresion, separador string) []string {
	partes := make([]string, 0, 3)
	profundidad := 0
	inicio := 0
	for i := 0; i < len(expresion); i++ {
		switch expresion[i] {
		case '(':
			profundidad++
		case ')':
			if profundidad > 0 {
				profundidad--
			}
		}
		if profundidad == 0 && strings.HasPrefix(expresion[i:], separador) {
			partes = append(partes, strings.TrimSpace(expresion[inicio:i]))
			i += len(separador) - 1
			inicio = i + 1
		}
	}
	return append(partes, strings.TrimSpace(expresion[inicio:]))
}

func schedNombre(token string, nombres map[string]string) string {
	token = strings.TrimSpace(token)
	if alias, ok := nombres[token]; ok {
		return alias
	}
	return token
}

// schedEvaluaCondicion interpreta las condiciones del camino del scheduler.
// Falla cerrado: lo que no reconoce devuelve error.
func schedEvaluaCondicion(item map[string]types.AttributeValue, expresion string, nombres map[string]string, valores map[string]types.AttributeValue) (bool, error) {
	expresion = strings.TrimSpace(expresion)
	if expresion == "" {
		return true, nil
	}
	if partes := schedPartes(expresion, " OR "); len(partes) > 1 {
		for _, parte := range partes {
			cumple, err := schedEvaluaCondicion(item, parte, nombres, valores)
			if err != nil {
				return false, err
			}
			if cumple {
				return true, nil
			}
		}
		return false, nil
	}
	if partes := schedPartes(expresion, " AND "); len(partes) > 1 {
		for _, parte := range partes {
			cumple, err := schedEvaluaCondicion(item, parte, nombres, valores)
			if err != nil {
				return false, err
			}
			if !cumple {
				return false, nil
			}
		}
		return true, nil
	}
	if strings.HasPrefix(expresion, "(") && strings.HasSuffix(expresion, ")") {
		return schedEvaluaCondicion(item, strings.TrimSpace(expresion[1:len(expresion)-1]), nombres, valores)
	}
	for _, funcion := range []struct {
		prefijo string
		existe  bool
	}{
		{"attribute_exists(", true},
		{"attribute_not_exists(", false},
	} {
		if strings.HasPrefix(expresion, funcion.prefijo) && strings.HasSuffix(expresion, ")") {
			nombre := schedNombre(expresion[len(funcion.prefijo):len(expresion)-1], nombres)
			_, presente := item[nombre]
			if funcion.existe {
				return presente, nil
			}
			return !presente, nil
		}
	}
	for _, operador := range []string{" >= ", " <= ", " = ", " > ", " < "} {
		indice := strings.Index(expresion, operador)
		if indice <= 0 {
			continue
		}
		izquierda := strings.TrimSpace(expresion[:indice])
		derecha := strings.TrimSpace(expresion[indice+len(operador):])
		if izquierda == "" || derecha == "" || strings.Contains(izquierda, " ") || strings.Contains(derecha, " ") {
			return false, fmt.Errorf("comparación no soportada por el doble del scheduler: %q", expresion)
		}
		esperado, ok := valores[derecha]
		if !ok {
			return false, fmt.Errorf("condición sin valor para %q", derecha)
		}
		return schedCompara(item, schedNombre(izquierda, nombres), operador, esperado)
	}
	return false, fmt.Errorf("condición no soportada por el doble del scheduler: %q", expresion)
}

func schedCompara(item map[string]types.AttributeValue, nombre, operador string, esperado types.AttributeValue) (bool, error) {
	actual, existe := item[nombre]
	if !existe {
		return false, nil
	}
	switch a := actual.(type) {
	case *types.AttributeValueMemberS:
		e, ok := esperado.(*types.AttributeValueMemberS)
		if !ok {
			return false, fmt.Errorf("comparación S contra %T", esperado)
		}
		return schedComparaOrden(a.Value, operador, e.Value)
	case *types.AttributeValueMemberN:
		e, ok := esperado.(*types.AttributeValueMemberN)
		if !ok {
			return false, fmt.Errorf("comparación N contra %T", esperado)
		}
		ai, err := strconv.ParseFloat(a.Value, 64)
		if err != nil {
			return false, err
		}
		ei, err := strconv.ParseFloat(e.Value, 64)
		if err != nil {
			return false, err
		}
		return schedComparaOrden(ai, operador, ei)
	case *types.AttributeValueMemberBOOL:
		e, ok := esperado.(*types.AttributeValueMemberBOOL)
		if !ok || operador != " = " {
			return false, fmt.Errorf("comparación BOOL no soportada: %q", operador)
		}
		return a.Value == e.Value, nil
	default:
		return false, fmt.Errorf("tipo de atributo no soportado en condición: %T", actual)
	}
}

func schedComparaOrden[T ~string | ~float64](actual T, operador string, esperado T) (bool, error) {
	switch operador {
	case " = ":
		return actual == esperado, nil
	case " < ":
		return actual < esperado, nil
	case " <= ":
		return actual <= esperado, nil
	case " > ":
		return actual > esperado, nil
	case " >= ":
		return actual >= esperado, nil
	default:
		return false, fmt.Errorf("operador no soportado por el doble del scheduler: %q", operador)
	}
}

// schedAplicaUpdate aplica SET/REMOVE/ADD y falla cerrado ante cualquier otra forma.
func schedAplicaUpdate(item map[string]types.AttributeValue, expresion string, nombres map[string]string, valores map[string]types.AttributeValue) error {
	resto := strings.TrimSpace(expresion)
	if resto == "" {
		return nil
	}
	for resto != "" {
		switch {
		case strings.HasPrefix(resto, "SET "):
			cuerpo, siguiente := schedSeccion(strings.TrimPrefix(resto, "SET "))
			if err := schedAplicaSet(item, cuerpo, nombres, valores); err != nil {
				return err
			}
			resto = siguiente
		case strings.HasPrefix(resto, "REMOVE "):
			cuerpo, siguiente := schedSeccion(strings.TrimPrefix(resto, "REMOVE "))
			for _, nombre := range strings.Split(cuerpo, ", ") {
				delete(item, schedNombre(nombre, nombres))
			}
			resto = siguiente
		case strings.HasPrefix(resto, "ADD "):
			cuerpo, siguiente := schedSeccion(strings.TrimPrefix(resto, "ADD "))
			if err := schedAplicaAdd(item, cuerpo, nombres, valores); err != nil {
				return err
			}
			resto = siguiente
		default:
			return fmt.Errorf("expresión de update no soportada por el doble del scheduler: %q", expresion)
		}
	}
	return nil
}

// schedSeccion corta la sección actual cuando empieza la siguiente palabra clave.
func schedSeccion(cuerpo string) (string, string) {
	corte := len(cuerpo)
	for _, marca := range []string{" REMOVE ", " ADD ", " SET "} {
		if i := strings.Index(cuerpo, marca); i >= 0 && i < corte {
			corte = i
		}
	}
	return strings.TrimSpace(cuerpo[:corte]), strings.TrimSpace(cuerpo[corte:])
}

func schedAplicaSet(item map[string]types.AttributeValue, cuerpo string, nombres map[string]string, valores map[string]types.AttributeValue) error {
	for _, asignacion := range strings.Split(cuerpo, ", ") {
		partes := strings.Split(strings.TrimSpace(asignacion), " = ")
		if len(partes) != 2 {
			return fmt.Errorf("asignación no soportada por el doble del scheduler: %q", asignacion)
		}
		destino := schedNombre(partes[0], nombres)
		fuente := strings.TrimSpace(partes[1])
		if indice := strings.Index(fuente, " + "); indice > 0 {
			base := schedNombre(fuente[:indice], nombres)
			incremento, ok := valores[strings.TrimSpace(fuente[indice+3:])]
			if !ok {
				return fmt.Errorf("SET con incremento sin valor: %q", fuente)
			}
			actual, okActual := item[base].(*types.AttributeValueMemberN)
			sumando, okSumando := incremento.(*types.AttributeValueMemberN)
			if !okActual || !okSumando {
				return fmt.Errorf("SET con incremento no numérico: %q", fuente)
			}
			a, err := strconv.ParseInt(actual.Value, 10, 64)
			if err != nil {
				return err
			}
			b, err := strconv.ParseInt(sumando.Value, 10, 64)
			if err != nil {
				return err
			}
			item[destino] = schedAttrN(a + b)
			continue
		}
		valor, ok := valores[fuente]
		if !ok {
			return fmt.Errorf("SET sin valor para %q", fuente)
		}
		item[destino] = valor
	}
	return nil
}

func schedAplicaAdd(item map[string]types.AttributeValue, cuerpo string, nombres map[string]string, valores map[string]types.AttributeValue) error {
	for _, entrada := range strings.Split(cuerpo, ", ") {
		campos := strings.Split(strings.TrimSpace(entrada), " ")
		if len(campos) != 2 {
			return fmt.Errorf("ADD no soportado por el doble del scheduler: %q", entrada)
		}
		destino := schedNombre(campos[0], nombres)
		incremento, ok := valores[campos[1]]
		if !ok {
			return fmt.Errorf("ADD sin valor para %q", campos[1])
		}
		sumando, okSumando := incremento.(*types.AttributeValueMemberN)
		if !okSumando {
			return fmt.Errorf("ADD no numérico: %q", entrada)
		}
		delta, err := strconv.ParseInt(sumando.Value, 10, 64)
		if err != nil {
			return err
		}
		actual := int64(0)
		if previo, okPrevio := item[destino].(*types.AttributeValueMemberN); okPrevio {
			if actual, err = strconv.ParseInt(previo.Value, 10, 64); err != nil {
				return err
			}
		}
		item[destino] = schedAttrN(actual + delta)
	}
	return nil
}

// schedDynamoIncompatible sólo implementa DynamoAPI (lo mínimo de otros flujos).
type schedDynamoIncompatible struct{}

func (schedDynamoIncompatible) GetItem(context.Context, *dynamodb.GetItemInput, ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
	return &dynamodb.GetItemOutput{}, nil
}

func (schedDynamoIncompatible) TransactWriteItems(context.Context, *dynamodb.TransactWriteItemsInput, ...func(*dynamodb.Options)) (*dynamodb.TransactWriteItemsOutput, error) {
	return &dynamodb.TransactWriteItemsOutput{}, nil
}

func schedDependencias(dynamo dynamoAdapter.DynamoAPI, tabla string, logger *slog.Logger) *dependenciasOleada5 {
	return &dependenciasOleada5{Dynamo: dynamo, Tabla: tabla, Logger: logger}
}

func schedOpcionesPrueba(ahora time.Time) appReminders.Opciones {
	return appReminders.Opciones{Intervalo: time.Hour, Lease: 30 * time.Second, Lote: 10, Ahora: func() time.Time { return ahora }}
}

// ---------------------------------------------------------------------------
// Fail-closed
// ---------------------------------------------------------------------------

func TestSchedulerOleada5SinDynamoFallaCerrado(t *testing.T) {
	if _, err := iniciarSchedulerOleada5(context.Background(), nil); err == nil {
		t.Fatal("sin dependencias debe fallar, no arrancar un scheduler sin persistencia")
	}
	var ctxNil context.Context
	if _, err := iniciarSchedulerOleada5(ctxNil, &dependenciasOleada5{}); err == nil {
		t.Fatal("un contexto nulo debe rechazarse")
	}
	if _, err := iniciarSchedulerOleada5(context.Background(), &dependenciasOleada5{}); err == nil {
		t.Fatal("sin DynamoDB debe fallar")
	}
	if _, err := iniciarSchedulerOleada5(context.Background(), schedDependencias(nuevoSchedFakeSDK(), "", slog.Default())); err == nil {
		t.Fatal("sin tabla debe fallar")
	}
}

func TestSchedulerOleada5ClienteIncompatibleFallaCerrado(t *testing.T) {
	_, err := iniciarSchedulerOleada5(context.Background(), schedDependencias(schedDynamoIncompatible{}, schedulerTablaPrueba, slog.Default()))
	if err == nil || !strings.Contains(err.Error(), "Query") {
		t.Fatalf("un cliente sin Query/UpdateItem debe rechazarse: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Arranque real con adaptadores reales
// ---------------------------------------------------------------------------

func TestSchedulerOleada5ReclamaEncolaYCompletaSinAWS(t *testing.T) {
	ahora := time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)
	dynamo := nuevoSchedFakeSDK()
	dynamo.poner(map[string]types.AttributeValue{
		"PK":              schedAttrS("SCHEDULE#GLOBAL"),
		"SK":              schedAttrS("DUE#2026-11-24T11:00:00.000000000Z#r-arranque"),
		"entityType":      schedAttrS("REMINDER_SCHEDULE"),
		"userID":          schedAttrS("usuario-arranque"),
		"reminderID":      schedAttrS("r-arranque"),
		"title":           schedAttrS("Pagar la luz"),
		"dueAt":           schedAttrS("2026-11-24T11:00:00Z"),
		"timezone":        schedAttrS("America/Mexico_City"),
		"owner":           schedAttrS(""),
		"leaseUntil":      schedAttrS(""),
		"version":         schedAttrN(1),
		"fence":           schedAttrN(0),
		"attempts":        schedAttrN(0),
		"reminderVersion": schedAttrN(1),
	})
	// Recordatorio y punteros reales, en la forma que escriben E065/E067, más el
	// perfil ACTIVE que exige el ConditionCheck de alta del outbox (E002/E006).
	dynamo.poner(map[string]types.AttributeValue{
		"PK":         schedAttrS("USER#usuario-arranque"),
		"SK":         schedAttrS("PROFILE"),
		"entityType": schedAttrS("USER_PROFILE"),
		"status":     schedAttrS("ACTIVE"),
		"userID":     schedAttrS("usuario-arranque"),
	})
	dynamo.poner(map[string]types.AttributeValue{
		"PK":         schedAttrS("USER#usuario-arranque"),
		"SK":         schedAttrS("REMINDER#2026-11-24T11:00:00.000Z#r-arranque"),
		"entityType": schedAttrS("USER_REMINDER"),
		"reminderID": schedAttrS("r-arranque"),
		"title":      schedAttrS("Pagar la luz"),
		"dueAt":      schedAttrS("2026-11-24T11:00:00Z"),
		"timezone":   schedAttrS("America/Mexico_City"),
		"active":     schedAttrB(true),
		"deleted":    schedAttrB(false),
		"version":    schedAttrN(1),
	})
	dynamo.poner(map[string]types.AttributeValue{
		"PK":         schedAttrS("USER#usuario-arranque"),
		"SK":         schedAttrS("REMINDERPTR#r-arranque"),
		"entityType": schedAttrS("USER_REMINDER_POINTER"),
		"reminderID": schedAttrS("r-arranque"),
		"reminderSK": schedAttrS("REMINDER#2026-11-24T11:00:00.000Z#r-arranque"),
		"deleted":    schedAttrB(false),
		"version":    schedAttrN(1),
	})

	var bitacora bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&bitacora, &slog.HandlerOptions{Level: slog.LevelDebug}))
	router := nexumhttp.NewRouter()

	deps := schedDependencias(dynamo, schedulerTablaPrueba, logger)
	deps.Router = router

	ctx, cancelar := context.WithCancel(context.Background())
	defer cancelar()

	cerrar, err := iniciarSchedulerOleada5Con(ctx, deps, schedOpcionesPrueba(ahora))
	if err != nil {
		t.Fatalf("iniciar scheduler: %v", err)
	}
	if cerrar == nil {
		t.Fatal("debe devolver una función de cierre")
	}

	// Un ciclo real: Query -> Claim -> Encolar (outbox) -> Complete.
	esperarHasta(t, 3*time.Second, func() bool {
		queries, _, _ := dynamo.stats()
		return queries > 0 && dynamo.hayPutEnParticion("OUTBOX#GLOBAL")
	})
	// El cierre es la transacción posterior al encolado: se espera su efecto para
	// afirmar sobre el estado final, no sobre una etapa intermedia.
	esperarHasta(t, 3*time.Second, func() bool {
		recordatorio := dynamo.item("USER#usuario-arranque", "REMINDER#2026-11-24T11:00:00.000Z#r-arranque")
		activo, existe := schedBool(recordatorio, "active")
		return existe && !activo
	})
	_, updates, transacciones := dynamo.stats()
	if updates < 1 {
		t.Fatal("el Claim debía usar UpdateItem condicional")
	}
	if transacciones < 1 {
		t.Fatal("el cierre debía usar una transacción")
	}

	// Efectos reales: el disparo único queda inactivo y la agenda/punteros coherentes.
	recordatorio := dynamo.item("USER#usuario-arranque", "REMINDER#2026-11-24T11:00:00.000Z#r-arranque")
	if activo, existe := schedBool(recordatorio, "active"); !existe || activo {
		t.Fatalf("el recordatorio debía quedar inactivo: %v", recordatorio)
	}
	if version := schedNumero(recordatorio, "version"); version != 2 {
		t.Fatalf("la versión del recordatorio debía avanzar a 2: %d", version)
	}
	if borrado, _ := schedBool(recordatorio, "deleted"); borrado {
		t.Fatal("el recordatorio no debía marcarse como borrado")
	}
	if doble := dynamo.item("SCHEDULE#GLOBAL", "DUE#2026-11-24T11:00:00.000000000Z#r-arranque"); doble != nil {
		t.Fatalf("la ocurrencia debía retirarse de la agenda: %v", doble)
	}
	if punteroAgenda := dynamo.item("USER#usuario-arranque", "SCHEDULEPTR#r-arranque"); punteroAgenda != nil {
		t.Fatalf("el puntero de agenda debía retirarse: %v", punteroAgenda)
	}
	// El puntero del recordatorio se conserva (E064/E066 lo resuelven) y sólo avanza.
	puntero := dynamo.item("USER#usuario-arranque", "REMINDERPTR#r-arranque")
	if puntero == nil || schedNumero(puntero, "version") != 2 {
		t.Fatalf("el puntero del recordatorio debía conservarse con versión 2: %v", puntero)
	}
	// Outbox: un único trabajo, con el eventID determinista de la ocurrencia.
	trabajos := dynamo.itemsConPrefijo("OUTBOX#GLOBAL", "DUE#")
	if len(trabajos) != 1 {
		t.Fatalf("el outbox debía tener exactamente un trabajo: %d", len(trabajos))
	}
	dueAt := time.Date(2026, 11, 24, 11, 0, 0, 0, time.UTC)
	if eventID := textoScheduler(trabajos[0]["eventId"]); eventID != appReminders.EventIDRecordatorio("r-arranque", dueAt) {
		t.Fatalf("eventID inesperado: %q", eventID)
	}
	if estado := textoScheduler(trabajos[0]["status"]); estado != string(pushoutbox.EstadoPendiente) {
		t.Fatalf("el trabajo debía quedar pendiente: %q", estado)
	}
	if notificationID := textoScheduler(trabajos[0]["notificationID"]); notificationID == "" {
		t.Fatal("el trabajo debía llevar notificationID")
	}

	// El scheduler no registra rutas: cualquier ruta nueva respondería 200/405,
	// no 404, y el router no cambia por arrancarlo.
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/scheduler", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("el scheduler no debe registrar rutas: %d %s", rec.Code, rec.Body.String())
	}

	cerrar()
	cerrar() // idempotente, sin panic

	queriesTrasCerrar, _, _ := dynamo.stats()
	time.Sleep(120 * time.Millisecond)
	queriesFinales, _, _ := dynamo.stats()
	if queriesFinales != queriesTrasCerrar {
		t.Fatalf("el scheduler siguió consultando tras cerrar: %d -> %d", queriesTrasCerrar, queriesFinales)
	}
	if !strings.Contains(bitacora.String(), "scheduler Oleada 5") {
		t.Fatalf("falta el log de arranque/cierre: %s", bitacora.String())
	}
	if strings.Contains(bitacora.String(), "password") || strings.Contains(bitacora.String(), "Pagar la luz") {
		t.Fatalf("el log del scheduler no debe contener datos sensibles ni el título: %s", bitacora.String())
	}
}

func esperarHasta(t *testing.T, limite time.Duration, condicion func() bool) {
	t.Helper()
	fin := time.Now().Add(limite)
	for time.Now().Before(fin) {
		if condicion() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("la condición no se cumplió dentro del plazo")
}

// ---------------------------------------------------------------------------
// Semillas y cadena real reutilizable
// ---------------------------------------------------------------------------

const (
	schedFormatoAgenda       = "2006-01-02T15:04:05.000000000Z"
	schedFormatoRecordatorio = "2006-01-02T15:04:05.000Z"
)

func schedTexto(valor types.AttributeValue) string {
	if texto, ok := valor.(*types.AttributeValueMemberS); ok {
		return texto.Value
	}
	return ""
}

func schedBool(item map[string]types.AttributeValue, nombre string) (bool, bool) {
	if v, ok := item[nombre].(*types.AttributeValueMemberBOOL); ok {
		return v.Value, true
	}
	return false, false
}

func schedNumero(item map[string]types.AttributeValue, nombre string) int64 {
	if v, ok := item[nombre].(*types.AttributeValueMemberN); ok {
		valor, err := strconv.ParseInt(v.Value, 10, 64)
		if err == nil {
			return valor
		}
	}
	return 0
}

func schedSembrarPerfil(dynamo *schedFakeSDK, userID string) {
	dynamo.poner(map[string]types.AttributeValue{
		"PK":         schedAttrS("USER#" + userID),
		"SK":         schedAttrS("PROFILE"),
		"entityType": schedAttrS("USER_PROFILE"),
		"status":     schedAttrS("ACTIVE"),
		"userID":     schedAttrS(userID),
	})
}

// schedSembrarRecordatorio escribe la agenda global, el recordatorio, su puntero y
// el perfil ACTIVE con las claves exactas de E065. Devuelve la SK global.
func schedSembrarRecordatorio(dynamo *schedFakeSDK, userID, reminderID, titulo string, dueAt time.Time, frecuencia *string) string {
	skAgenda := "DUE#" + dueAt.UTC().Format(schedFormatoAgenda) + "#" + reminderID
	skRecordatorio := "REMINDER#" + dueAt.UTC().Format(schedFormatoRecordatorio) + "#" + reminderID
	agenda := map[string]types.AttributeValue{
		"PK":              schedAttrS("SCHEDULE#GLOBAL"),
		"SK":              schedAttrS(skAgenda),
		"entityType":      schedAttrS("REMINDER_SCHEDULE"),
		"userID":          schedAttrS(userID),
		"reminderID":      schedAttrS(reminderID),
		"title":           schedAttrS(titulo),
		"dueAt":           schedAttrS(dueAt.UTC().Format(time.RFC3339)),
		"timezone":        schedAttrS("America/Mexico_City"),
		"owner":           schedAttrS(""),
		"leaseUntil":      schedAttrS(""),
		"version":         schedAttrN(1),
		"fence":           schedAttrN(0),
		"attempts":        schedAttrN(0),
		"reminderVersion": schedAttrN(1),
	}
	if frecuencia != nil {
		agenda["frequency"] = schedAttrS(*frecuencia)
	}
	dynamo.poner(agenda)
	dynamo.poner(map[string]types.AttributeValue{
		"PK":         schedAttrS("USER#" + userID),
		"SK":         schedAttrS(skRecordatorio),
		"entityType": schedAttrS("USER_REMINDER"),
		"reminderID": schedAttrS(reminderID),
		"title":      schedAttrS(titulo),
		"dueAt":      schedAttrS(dueAt.UTC().Format(time.RFC3339)),
		"timezone":   schedAttrS("America/Mexico_City"),
		"active":     schedAttrB(true),
		"deleted":    schedAttrB(false),
		"version":    schedAttrN(1),
	})
	dynamo.poner(map[string]types.AttributeValue{
		"PK":         schedAttrS("USER#" + userID),
		"SK":         schedAttrS("REMINDERPTR#" + reminderID),
		"entityType": schedAttrS("USER_REMINDER_POINTER"),
		"reminderID": schedAttrS(reminderID),
		"reminderSK": schedAttrS(skRecordatorio),
		"deleted":    schedAttrB(false),
		"version":    schedAttrN(1),
	})
	// Puntero de agenda: el `Advance`/`Complete` del scheduler condiciona su
	// actualización/borrado a esta SK; sin él, un trabajo reclamado no podría
	// avanzar y la semilla no reproduciría la composición real.
	dynamo.poner(map[string]types.AttributeValue{
		"PK":         schedAttrS("USER#" + userID),
		"SK":         schedAttrS("SCHEDULEPTR#" + reminderID),
		"entityType": schedAttrS("REMINDER_SCHEDULE_POINTER"),
		"reminderID": schedAttrS(reminderID),
		"skGlobal":   schedAttrS(skAgenda),
		"dueAt":      schedAttrS(dueAt.UTC().Format(time.RFC3339)),
		"userID":     schedAttrS(userID),
	})
	schedSembrarPerfil(dynamo, userID)
	return skAgenda
}

// schedCadena monta la cadena real sobre el doble: ScheduleStore + PushOutboxStore
// + Servicio de outbox + Scheduler, con owner, reloj y lote deterministas.
func schedCadena(owner string, dynamo *schedFakeSDK, reloj func() time.Time, lote int) (*appReminders.Scheduler, *dynamoAdapter.ScheduleStore) {
	agenda := dynamoAdapter.NewScheduleStore(dynamo, schedulerTablaPrueba).WithOwner(owner)
	outbox := dynamoAdapter.NewPushOutboxStore(dynamo, schedulerTablaPrueba).WithClock(reloj)
	productor := pushoutbox.NewService(outbox, nil, nil, nil, nil, pushoutbox.Opciones{Ahora: reloj})
	scheduler := appReminders.NewScheduler(agenda, productor, appReminders.Opciones{
		Intervalo: time.Hour,
		Lease:     30 * time.Second,
		Lote:      lote,
		Ahora:     reloj,
	})
	return scheduler, agenda
}

// ---------------------------------------------------------------------------
// Encolado correcto + fallo de Advance: reintento con el MISMO eventID
// ---------------------------------------------------------------------------

func TestSchedulerOleada5ReplayTrasFalloDeAdvanceNoDuplica(t *testing.T) {
	base := time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)
	ahora := base
	reloj := func() time.Time { return ahora }
	dynamo := nuevoSchedFakeSDK()
	dueAt := base.Add(-time.Hour)
	frecuencia := "daily"
	skVieja := schedSembrarRecordatorio(dynamo, "usuario-replay", "r-replay", "Pagar el agua", dueAt, &frecuencia)
	// La segunda transacción del ciclo es el `Advance`: falla una sola vez.
	dynamo.fallarEn = 2
	dynamo.errorEn = errors.New("dynamodb no disponible (simulado)")

	scheduler, _ := schedCadena("nodo-a", dynamo, reloj, 10)
	primero := scheduler.Ciclo(context.Background())
	if primero.Encoladas != 1 || primero.Avanzadas != 0 || primero.Completadas != 0 {
		t.Fatalf("primer ciclo inesperado: %+v", primero)
	}
	// El fallo de Advance no desactiva el recordatorio ni pierde la ocurrencia: el
	// lease queda vigente hasta su vencimiento.
	recordatorio := dynamo.item("USER#usuario-replay", "REMINDER#"+dueAt.UTC().Format(schedFormatoRecordatorio)+"#r-replay")
	if activo, _ := schedBool(recordatorio, "active"); !activo {
		t.Fatalf("el fallo de Advance no debía desactivar el recordatorio: %v", recordatorio)
	}
	if agenda := dynamo.item("SCHEDULE#GLOBAL", skVieja); agenda == nil || textoScheduler(agenda["leaseUntil"]) == "" {
		t.Fatalf("la ocurrencia debía conservar el lease hasta vencer: %v", agenda)
	}
	if trabajos := dynamo.itemsConPrefijo("OUTBOX#GLOBAL", "DUE#"); len(trabajos) != 1 {
		t.Fatalf("el primer ciclo debía encolar un único trabajo: %d", len(trabajos))
	}

	// Reintento cuando vence el lease: replay del mismo eventID y cierre definitivo.
	ahora = base.Add(31 * time.Second)
	segundo := scheduler.Ciclo(context.Background())
	if segundo.Replays != 1 || segundo.Avanzadas != 1 || segundo.Encoladas != 0 {
		t.Fatalf("el reintento debía ser replay y avanzar: %+v", segundo)
	}
	trabajos := dynamo.itemsConPrefijo("OUTBOX#GLOBAL", "DUE#")
	if len(trabajos) != 1 {
		t.Fatalf("el reintento duplicó el trabajo del outbox: %d", len(trabajos))
	}
	eventID := appReminders.EventIDRecordatorio("r-replay", dueAt)
	if obtenido := textoScheduler(trabajos[0]["eventId"]); obtenido != eventID {
		t.Fatalf("eventID inestable entre intentos: %q != %q", obtenido, eventID)
	}

	siguiente, err := appReminders.NextOccurrence(dueAt, "America/Mexico_City", appReminders.FrequencyDaily)
	if err != nil {
		t.Fatalf("NextOccurrence: %v", err)
	}
	skNueva := "DUE#" + siguiente.UTC().Format(schedFormatoAgenda) + "#r-replay"
	if dinamica := dynamo.item("SCHEDULE#GLOBAL", skNueva); dinamica == nil {
		t.Fatal("falta la ocurrencia siguiente tras el reintento")
	}
	if vieja := dynamo.item("SCHEDULE#GLOBAL", skVieja); vieja != nil {
		t.Fatal("la ocurrencia anterior debía retirarse al avanzar")
	}
	if puntero := dynamo.item("USER#usuario-replay", "SCHEDULEPTR#r-replay"); textoScheduler(puntero["skGlobal"]) != skNueva {
		t.Fatalf("el puntero de agenda debía apuntar a la ocurrencia siguiente: %v", puntero)
	}
}

// ---------------------------------------------------------------------------
// Más de cinco páginas de vencidos: nada se salta y no hay Scan
// ---------------------------------------------------------------------------

func TestSchedulerOleada5ProcesaMasDeCincoPaginasSinSaltos(t *testing.T) {
	base := time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)
	reloj := func() time.Time { return base }
	dynamo := nuevoSchedFakeSDK()
	const total = 130
	esperados := make(map[string]bool, total)
	for i := 0; i < total; i++ {
		id := fmt.Sprintf("r-pagina-%03d", i)
		esperados[id] = true
		schedSembrarRecordatorio(dynamo, "usuario-paginas", id, "Recordatorio "+id,
			base.Add(-time.Duration(i+1)*time.Minute), nil)
	}
	scheduler, _ := schedCadena("nodo-a", dynamo, reloj, 25)

	reclamadas, encoladas := 0, 0
	for ciclo := 0; ciclo < 20; ciclo++ {
		resultado := scheduler.Ciclo(context.Background())
		reclamadas += resultado.Reclamadas
		encoladas += resultado.Encoladas
		if resultado.Reclamadas == 0 && resultado.Encoladas == 0 {
			break
		}
	}
	if reclamadas != total || encoladas != total {
		t.Fatalf("se esperaban %d ocurrencias: reclamadas=%d encoladas=%d", total, reclamadas, encoladas)
	}

	// Sin saltos: cada recordatorio quedó inactivo y cada agenda/puntero se retiró.
	recordatorios := dynamo.itemsConPrefijo("USER#usuario-paginas", "REMINDER#")
	if len(recordatorios) != total {
		t.Fatalf("faltan recordatorios: %d", len(recordatorios))
	}
	for _, recordatorio := range recordatorios {
		if activo, _ := schedBool(recordatorio, "active"); activo {
			t.Fatalf("recordatorio sin procesar: %v", recordatorio["SK"])
		}
	}
	if restantes := dynamo.itemsConPrefijo("SCHEDULE#GLOBAL", "DUE#"); len(restantes) != 0 {
		t.Fatalf("quedaron ocurrencias sin retirar: %d", len(restantes))
	}
	if punteros := dynamo.itemsConPrefijo("USER#usuario-paginas", "SCHEDULEPTR#"); len(punteros) != 0 {
		t.Fatalf("quedaron punteros de agenda: %d", len(punteros))
	}

	// Un trabajo por ocurrencia y todos los identificadores esperados.
	trabajos := dynamo.itemsConPrefijo("OUTBOX#GLOBAL", "DUE#")
	if len(trabajos) != total {
		t.Fatalf("se esperaban %d trabajos de outbox: %d", total, len(trabajos))
	}
	vistos := make(map[string]bool, total)
	for _, trabajo := range trabajos {
		eventID := textoScheduler(trabajo["eventId"])
		if vistos[eventID] {
			t.Fatalf("eventID duplicado: %q", eventID)
		}
		vistos[eventID] = true
		resto := strings.TrimPrefix(eventID, "reminder:")
		id := strings.SplitN(resto, ":", 2)[0]
		if !esperados[id] {
			t.Fatalf("eventID de un recordatorio inesperado: %q", eventID)
		}
		delete(esperados, id)
	}
	if len(esperados) != 0 {
		t.Fatalf("recordatorios sin encolar: %v", esperados)
	}

	// Lectura acotada: siempre Query por partición con condición de rango y
	// consistencia fuerte. El doble no expone Scan (la interfaz tampoco).
	consultas := dynamo.consultasVistas()
	if len(consultas) < 6 {
		t.Fatalf("se esperaban al menos 6 consultas (6 páginas): %d", len(consultas))
	}
	for _, consulta := range consultas {
		condicion := aws.ToString(consulta.KeyConditionExpression)
		if !strings.Contains(condicion, "#sk <= :limite") {
			t.Fatalf("consulta sin condición de rango: %q", condicion)
		}
		if !aws.ToBool(consulta.ConsistentRead) {
			t.Fatal("Due debe leer con consistencia fuerte")
		}
		if limite := int(aws.ToInt32(consulta.Limit)); limite != 25 {
			t.Fatalf("límite de página inesperado: %d", limite)
		}
	}
}

// ---------------------------------------------------------------------------
// Dos nodos concurrentes: una sola reclamación por ocurrencia
// ---------------------------------------------------------------------------

func TestSchedulerOleada5DosNodosSoloUnReclamoPorOcurrencia(t *testing.T) {
	base := time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)
	reloj := func() time.Time { return base }
	dynamo := nuevoSchedFakeSDK()
	const total = 30
	for i := 0; i < total; i++ {
		schedSembrarRecordatorio(dynamo, "usuario-carrera", fmt.Sprintf("r-carrera-%02d", i), "Recordatorio",
			base.Add(-time.Duration(i+1)*time.Minute), nil)
	}
	nodoA, _ := schedCadena("nodo-a", dynamo, reloj, 25)
	nodoB, _ := schedCadena("nodo-b", dynamo, reloj, 25)

	encoladas := 0
	for ronda := 0; ronda < 4; ronda++ {
		var wg sync.WaitGroup
		var a, b appReminders.ResultadoCiclo
		wg.Add(2)
		go func() { defer wg.Done(); a = nodoA.Ciclo(context.Background()) }()
		go func() { defer wg.Done(); b = nodoB.Ciclo(context.Background()) }()
		wg.Wait()
		encoladas += a.Encoladas + b.Encoladas
		if a.Reclamadas+b.Reclamadas == 0 {
			break
		}
	}
	if encoladas != total {
		t.Fatalf("debía encolarse una vez cada ocurrencia: %d de %d", encoladas, total)
	}
	trabajos := dynamo.itemsConPrefijo("OUTBOX#GLOBAL", "DUE#")
	if len(trabajos) != total {
		t.Fatalf("trabajos duplicados en el outbox: %d", len(trabajos))
	}
	vistos := make(map[string]bool, total)
	for _, trabajo := range trabajos {
		eventID := textoScheduler(trabajo["eventId"])
		if vistos[eventID] {
			t.Fatalf("dos nodos encolaron la misma ocurrencia: %q", eventID)
		}
		vistos[eventID] = true
	}
	for _, recordatorio := range dynamo.itemsConPrefijo("USER#usuario-carrera", "REMINDER#") {
		if activo, _ := schedBool(recordatorio, "active"); activo {
			t.Fatalf("recordatorio pendiente tras la carrera: %v", recordatorio["SK"])
		}
	}
}

// ---------------------------------------------------------------------------
// Reinicio: lease vencido recuperado por un nodo nuevo
// ---------------------------------------------------------------------------

func TestSchedulerOleada5ReinicioRecuperaLeaseVencido(t *testing.T) {
	base := time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)
	ahora := base
	reloj := func() time.Time { return ahora }
	dynamo := nuevoSchedFakeSDK()
	dueAt := base.Add(-time.Hour)
	skVieja := schedSembrarRecordatorio(dynamo, "usuario-reinicio", "r-reinicio", "Llamar al banco", dueAt, nil)

	// El nodo A reclama y muere antes de cerrar.
	_, agendaA := schedCadena("nodo-a", dynamo, reloj, 10)
	ocurrencias, err := agendaA.Due(context.Background(), base, 10)
	if err != nil || len(ocurrencias) != 1 {
		t.Fatalf("Due: %v %d", err, len(ocurrencias))
	}
	if _, err := agendaA.Claim(context.Background(), ocurrencias[0], base, 30*time.Second); err != nil {
		t.Fatalf("Claim del nodo A: %v", err)
	}

	// Con el lease vigente nadie más puede reclamar.
	nodoB, _ := schedCadena("nodo-b", dynamo, reloj, 10)
	if conLease := nodoB.Ciclo(context.Background()); conLease.Reclamadas != 0 || conLease.Encoladas != 0 {
		t.Fatalf("el lease vigente no debía permitir reclamar: %+v", conLease)
	}

	// Tras el vencimiento, el nodo nuevo recupera el trabajo huérfano.
	ahora = base.Add(31 * time.Second)
	resultado := nodoB.Ciclo(context.Background())
	if resultado.Reclamadas != 1 || resultado.Encoladas != 1 || resultado.Completadas != 1 {
		t.Fatalf("el nodo nuevo debía recuperar el trabajo: %+v", resultado)
	}
	trabajos := dynamo.itemsConPrefijo("OUTBOX#GLOBAL", "DUE#")
	if len(trabajos) != 1 {
		t.Fatalf("la recuperación debía encolar un único trabajo: %d", len(trabajos))
	}
	if eventID := textoScheduler(trabajos[0]["eventId"]); eventID != appReminders.EventIDRecordatorio("r-reinicio", dueAt) {
		t.Fatalf("eventID inesperado: %q", eventID)
	}
	if vieja := dynamo.item("SCHEDULE#GLOBAL", skVieja); vieja != nil {
		t.Fatal("la ocurrencia recuperada debía retirarse al completarse")
	}
	recordatorio := dynamo.item("USER#usuario-reinicio", "REMINDER#"+dueAt.UTC().Format(schedFormatoRecordatorio)+"#r-reinicio")
	if activo, _ := schedBool(recordatorio, "active"); activo {
		t.Fatalf("el recordatorio debía quedar inactivo: %v", recordatorio)
	}
}
