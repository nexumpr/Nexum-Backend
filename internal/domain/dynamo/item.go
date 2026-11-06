package dynamo

import (
	"time"
)

// DynamoItem representa un elemento genérico dentro de la tabla única NEXUM_CORE (A-060 §2.1).
type DynamoItem struct {
	PK         string         `json:"pk"`
	SK         string         `json:"sk"`
	GSI1PK     string         `json:"gsi1pk,omitempty"`
	GSI1SK     string         `json:"gsi1sk,omitempty"`
	EntityType string         `json:"entityType"`
	Version    int64          `json:"version"`
	CreatedAt  time.Time      `json:"createdAt"`
	UpdatedAt  time.Time      `json:"updatedAt"`
	TTL        int64          `json:"ttl,omitempty"`
	Attributes map[string]any `json:"attributes"`
}

// NewDynamoItem crea un nuevo elemento con versionado optimista inicial (version=1).
func NewDynamoItem(pk, sk, entityType string, attrs map[string]any) DynamoItem {
	now := time.Now().UTC()
	if attrs == nil {
		attrs = make(map[string]any)
	}
	return DynamoItem{
		PK:         pk,
		SK:         sk,
		EntityType: entityType,
		Version:    1,
		CreatedAt:  now,
		UpdatedAt:  now,
		Attributes: attrs,
	}
}

// WithGSI1 asigna los atributos del índice secundario global sparse GSI1.
func (item DynamoItem) WithGSI1(gsi1pk, gsi1sk string) DynamoItem {
	item.GSI1PK = gsi1pk
	item.GSI1SK = gsi1sk
	return item
}

// WithTTL asigna un tiempo de expiración TTL en segundos epoch (A-060 §2.8).
func (item DynamoItem) WithTTL(ttlEpochSeconds int64) DynamoItem {
	item.TTL = ttlEpochSeconds
	return item
}

