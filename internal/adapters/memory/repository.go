package memory

import (
	"context"
	"fmt"
	"sync"

	"github.com/nexumpr/Nexum-Backend/internal/ports"
)

// MemoryRepository es una implementación en memoria de ports.NoSQLRepository (A-020 §2.2, AC-A-020-03).
// Permite ejecutar pruebas unitarias y de arquitectura completamente aisladas de AWS DynamoDB y sin Docker.
type MemoryRepository struct {
	mu     sync.RWMutex
	tables map[string]map[string]map[string]any
}

// NewMemoryRepository inicializa un repositorio en memoria seguro para concurrencia.
func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		tables: make(map[string]map[string]map[string]any),
	}
}

func itemKey(key map[string]any) string {
	pk := fmt.Sprintf("%v", key["PK"])
	sk := ""
	if v, ok := key["SK"]; ok {
		sk = fmt.Sprintf("#%v", v)
	}
	return pk + sk
}

// GetItem obtiene un elemento por su clave compuesta.
func (r *MemoryRepository) GetItem(ctx context.Context, tableName string, key map[string]any) (map[string]any, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	table, ok := r.tables[tableName]
	if !ok {
		return nil, ports.ErrItemNotFound
	}

	k := itemKey(key)
	item, ok := table[k]
	if !ok {
		return nil, ports.ErrItemNotFound
	}

	// Retornar copia defensiva
	copied := make(map[string]any, len(item))
	for k, v := range item {
		copied[k] = v
	}
	return copied, nil
}

// PutItem inserta o reemplaza un elemento.
func (r *MemoryRepository) PutItem(ctx context.Context, tableName string, item map[string]any, cond *ports.ConditionExpression) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.tables[tableName]; !ok {
		r.tables[tableName] = make(map[string]map[string]any)
	}

	k := itemKey(item)

	// Simulación de condición de no existencia (ej. "attribute_not_exists(PK)")
	if cond != nil && cond.Expression == "attribute_not_exists(PK)" {
		if _, exists := r.tables[tableName][k]; exists {
			return ports.ErrConditionFailed
		}
	}

	stored := make(map[string]any, len(item))
	for k, v := range item {
		stored[k] = v
	}
	r.tables[tableName][k] = stored
	return nil
}

// UpdateItem actualiza atributos de un elemento.
func (r *MemoryRepository) UpdateItem(ctx context.Context, tableName string, key map[string]any, updateExpr string, exprValues map[string]any, cond *ports.ConditionExpression) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	table, ok := r.tables[tableName]
	if !ok {
		return ports.ErrItemNotFound
	}

	k := itemKey(key)
	item, ok := table[k]
	if !ok {
		return ports.ErrItemNotFound
	}

	for field, val := range exprValues {
		item[field] = val
	}
	return nil
}

// DeleteItem elimina un elemento.
func (r *MemoryRepository) DeleteItem(ctx context.Context, tableName string, key map[string]any, cond *ports.ConditionExpression) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	table, ok := r.tables[tableName]
	if !ok {
		return nil
	}

	delete(table, itemKey(key))
	return nil
}

// Query consulta elementos particionados.
func (r *MemoryRepository) Query(ctx context.Context, tableName string, input ports.QueryInput) (*ports.QueryResult, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	table, ok := r.tables[tableName]
	if !ok {
		return &ports.QueryResult{Items: []map[string]any{}}, nil
	}

	var results []map[string]any
	targetPK := fmt.Sprintf("%v", input.PartitionKeyValue)

	for _, item := range table {
		if fmt.Sprintf("%v", item[input.PartitionKeyName]) == targetPK {
			copied := make(map[string]any, len(item))
			for k, v := range item {
				copied[k] = v
			}
			results = append(results, copied)
		}
	}

	return &ports.QueryResult{Items: results}, nil
}
