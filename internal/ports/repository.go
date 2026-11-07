package ports

import (
	"context"
	"errors"
)

var (
	// ErrItemNotFound se retorna cuando una entidad no existe en el repositorio NoSQL.
	ErrItemNotFound = errors.New("elemento no encontrado en repositorio NoSQL")

	// ErrConditionFailed se retorna cuando una expresión condicional falla (concurrencia optimista/idempotencia).
	ErrConditionFailed = errors.New("fallo en condicion de escritura en repositorio NoSQL")
)

// ConditionExpression define una expresión condicional para escrituras atómicas en DynamoDB.
type ConditionExpression struct {
	Expression string
	Values     map[string]any
}

// QueryInput especifica los parámetros para consultas particionadas en DynamoDB.
type QueryInput struct {
	PartitionKeyName  string
	PartitionKeyValue any
	SortKeyCondition  string
	SortKeyValue      any
	Limit             int32
	ExclusiveStartKey map[string]any
	ScanIndexForward  bool
}

// QueryResult encapsula el resultado paginado de una consulta NoSQL.
type QueryResult struct {
	Items            []map[string]any
	LastEvaluatedKey map[string]any
}

// NoSQLRepository define el puerto independiente para persistencia en Amazon DynamoDB (A-010 §2.3, GOV-04).
// El dominio y la aplicación solo interactúan con esta abstracción, nunca con el SDK de AWS directamente.
type NoSQLRepository interface {
	// GetItem obtiene un elemento por su clave primaria compuesta (PK, SK).
	GetItem(ctx context.Context, tableName string, key map[string]any) (map[string]any, error)

	// PutItem inserta o reemplaza un elemento, soportando expresiones condicionales de consistencia.
	PutItem(ctx context.Context, tableName string, item map[string]any, cond *ConditionExpression) error

	// UpdateItem actualiza atributos de un elemento existente sujeto a una condición de guardia.
	UpdateItem(ctx context.Context, tableName string, key map[string]any, updateExpr string, exprValues map[string]any, cond *ConditionExpression) error

	// DeleteItem elimina un elemento por clave primaria, opcionalmente con expresión condicional.
	DeleteItem(ctx context.Context, tableName string, key map[string]any, cond *ConditionExpression) error

	// Query ejecuta una consulta particionada paginada.
	Query(ctx context.Context, tableName string, input QueryInput) (*QueryResult, error)
}
