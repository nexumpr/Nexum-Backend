package dynamo

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	domainDynamo "github.com/nexumpr/Nexum-Backend/internal/domain/dynamo"
)

var (
	// ErrScanProhibited prohíbe terminantemente la operación Scan en toda la plataforma (AC-A-060-01).
	ErrScanProhibited = errors.New("operacion Scan prohibida: todas las consultas deben usar Query o GetItem acotadas por particion (AC-A-060-01)")

	// ErrItemNotFound se retorna cuando el elemento no existe en la tabla.
	ErrItemNotFound = errors.New("elemento no encontrado")

	// ErrConditionCheckFailed se retorna cuando una condición de escritura optimista falla.
	ErrConditionCheckFailed = errors.New("fallo en la expresion condicional (ConditionCheckFailed)")

	// ErrTransactionCanceled se retorna cuando una transacción atómica es cancelada (AC-A-060-02).
	ErrTransactionCanceled = errors.New("transaccion cancelada: una o mas condiciones de guardia fallaron")
)

// TransactActionType define la operación atómica dentro de una transacción DynamoDB.
type TransactActionType string

const (
	ActionPut            TransactActionType = "PUT"
	ActionUpdate         TransactActionType = "UPDATE"
	ActionDelete         TransactActionType = "DELETE"
	ActionConditionCheck TransactActionType = "CONDITION_CHECK"
)

// TransactAction define una operación individual dentro de TransactWriteItems (A-060 §2.6).
type TransactAction struct {
	Type            TransactActionType
	PK              string
	SK              string
	Item            *domainDynamo.DynamoItem
	ExpectedVersion *int64
	MustNotExist    bool
	UpdateFn        func(existing *domainDynamo.DynamoItem) error
}

// DynamoTable emula la tabla única NEXUM_CORE en memoria con soporte estricto para
// Single-Table Design, índices secundarios GSI1 y transacciones atómicas (A-060).
type DynamoTable struct {
	mu    sync.RWMutex
	items map[string]domainDynamo.DynamoItem // clave interna: "PK#SK"
}

// NewDynamoTable inicializa una nueva instancia de tabla DynamoDB.
func NewDynamoTable() *DynamoTable {
	return &DynamoTable{
		items: make(map[string]domainDynamo.DynamoItem),
	}
}

func makeKey(pk, sk string) string {
	return pk + "##" + sk
}

// Scan está terminantemente prohibido por diseño arquitectónico (AC-A-060-01).
func (t *DynamoTable) Scan(ctx context.Context) ([]domainDynamo.DynamoItem, error) {
	return nil, ErrScanProhibited
}

// GetItem obtiene un elemento por su clave compuesta primaria PK y SK.
func (t *DynamoTable) GetItem(ctx context.Context, pk, sk string) (*domainDynamo.DynamoItem, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	item, exists := t.items[makeKey(pk, sk)]
	if !exists {
		return nil, ErrItemNotFound
	}

	// Verificar TTL si está definido
	if item.TTL > 0 && time.Now().UTC().Unix() > item.TTL {
		return nil, ErrItemNotFound
	}

	itemCopy := item
	return &itemCopy, nil
}

// PutItem guarda o reemplaza un elemento con comprobación opcional de condición.
func (t *DynamoTable) PutItem(ctx context.Context, item domainDynamo.DynamoItem, mustNotExist bool, expectedVersion *int64) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	key := makeKey(item.PK, item.SK)
	existing, exists := t.items[key]

	if mustNotExist && exists {
		return ErrConditionCheckFailed
	}

	if expectedVersion != nil {
		if !exists || existing.Version != *expectedVersion {
			return ErrConditionCheckFailed
		}
		item.Version = existing.Version + 1
	}

	item.UpdatedAt = time.Now().UTC()
	t.items[key] = item
	return nil
}

// DeleteItem elimina un elemento por su clave compuesta.
func (t *DynamoTable) DeleteItem(ctx context.Context, pk, sk string) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	delete(t.items, makeKey(pk, sk))
	return nil
}

// QueryPartition consulta elementos dentro de una partición PK acotados por prefijo de SK (A-060 §2.4).
func (t *DynamoTable) QueryPartition(ctx context.Context, pk string, skPrefix string, forward bool, limit int) ([]domainDynamo.DynamoItem, error) {
	if pk == "" {
		return nil, errors.New("la clave de particion (PK) es obligatoria para Query")
	}

	t.mu.RLock()
	defer t.mu.RUnlock()

	var matched []domainDynamo.DynamoItem
	for _, item := range t.items {
		if item.PK == pk {
			if skPrefix == "" || strings.HasPrefix(item.SK, skPrefix) {
				matched = append(matched, item)
			}
		}
	}

	// Ordenamiento determinista por Sort Key (SK)
	sort.Slice(matched, func(i, j int) bool {
		if forward {
			return matched[i].SK < matched[j].SK
		}
		return matched[i].SK > matched[j].SK
	})

	if limit > 0 && len(matched) > limit {
		matched = matched[:limit]
	}

	return matched, nil
}

// QueryGSI1 consulta elementos mediante el índice secundario global sparse GSI1 (A-060 §2.4).
func (t *DynamoTable) QueryGSI1(ctx context.Context, gsi1pk string, gsi1skPrefix string, forward bool, limit int) ([]domainDynamo.DynamoItem, error) {
	if gsi1pk == "" {
		return nil, errors.New("la clave de particion del indice (GSI1PK) es obligatoria")
	}

	t.mu.RLock()
	defer t.mu.RUnlock()

	var matched []domainDynamo.DynamoItem
	for _, item := range t.items {
		if item.GSI1PK == gsi1pk {
			if gsi1skPrefix == "" || strings.HasPrefix(item.GSI1SK, gsi1skPrefix) {
				matched = append(matched, item)
			}
		}
	}

	// Ordenamiento determinista por GSI1SK
	sort.Slice(matched, func(i, j int) bool {
		if forward {
			return matched[i].GSI1SK < matched[j].GSI1SK
		}
		return matched[i].GSI1SK > matched[j].GSI1SK
	})

	if limit > 0 && len(matched) > limit {
		matched = matched[:limit]
	}

	return matched, nil
}

// TransactWriteItems ejecuta un conjunto de mutaciones de forma estrictamente atómica (A-060 §2.6, AC-A-060-02).
// Si cualquiera de las condiciones falla, la transacción entera se aborta sin aplicar cambios parciales.
func (t *DynamoTable) TransactWriteItems(ctx context.Context, actions []TransactAction) error {
	if len(actions) == 0 {
		return nil
	}
	if len(actions) > 100 {
		return fmt.Errorf("limite de DynamoDB excedido: maximo 100 acciones por transaccion (solicitadas %d)", len(actions))
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	// Fase 1: Validación estricta de condiciones de todas las acciones
	type stagedMutation struct {
		key     string
		isDel   bool
		newItem domainDynamo.DynamoItem
	}
	var staged []stagedMutation

	for _, action := range actions {
		key := makeKey(action.PK, action.SK)
		existing, exists := t.items[key]

		if action.MustNotExist && exists {
			return ErrTransactionCanceled
		}

		if action.ExpectedVersion != nil {
			if !exists || existing.Version != *action.ExpectedVersion {
				return ErrTransactionCanceled
			}
		}

		switch action.Type {
		case ActionConditionCheck:
			// Solo verificación, ya evaluada arriba
			continue

		case ActionPut:
			if action.Item == nil {
				return errors.New("item requerido para operacion Put")
			}
			itemCopy := *action.Item
			if exists {
				itemCopy.Version = existing.Version + 1
			} else {
				itemCopy.Version = 1
			}
			itemCopy.UpdatedAt = time.Now().UTC()
			staged = append(staged, stagedMutation{
				key:     key,
				isDel:   false,
				newItem: itemCopy,
			})

		case ActionUpdate:
			if !exists {
				return ErrTransactionCanceled
			}
			itemCopy := existing
			if action.UpdateFn != nil {
				if err := action.UpdateFn(&itemCopy); err != nil {
					return ErrTransactionCanceled
				}
			}
			itemCopy.Version = existing.Version + 1
			itemCopy.UpdatedAt = time.Now().UTC()
			staged = append(staged, stagedMutation{
				key:     key,
				isDel:   false,
				newItem: itemCopy,
			})

		case ActionDelete:
			staged = append(staged, stagedMutation{
				key:   key,
				isDel: true,
			})
		}
	}

	// Fase 2: Aplicación atómica de todas las mutaciones preparadas
	for _, mut := range staged {
		if mut.isDel {
			delete(t.items, mut.key)
		} else {
			t.items[mut.key] = mut.newItem
		}
	}

	return nil
}
