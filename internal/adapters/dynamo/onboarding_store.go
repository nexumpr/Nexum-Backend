package dynamo

import (
	"context"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	appOnboarding "github.com/nexumpr/Nexum-Backend/internal/application/onboarding"
	domainDynamo "github.com/nexumpr/Nexum-Backend/internal/domain/dynamo"
)

// OnboardingAPI es el subconjunto del cliente DynamoDB que necesita el estado de
// onboarding. Se declara como interfaz propia para que el compilador garantice que
// ninguna ruta usa `Scan`, `Query` ni transacciones: sólo el ítem del titular.
type OnboardingAPI interface {
	GetItem(ctx context.Context, params *dynamodb.GetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error)
}

// OnboardingStore implementa `appOnboarding.Store` sobre la tabla única existente.
// No crea tablas, índices ni particiones: usa `USER#<userId>/ONBOARDING`, que E001 ya
// escribe en el alta. La lectura es consistente para que una escritura confirmada por
// otro nodo se observe en el acto.
type OnboardingStore struct {
	client OnboardingAPI
	table  string

	// now sólo se usa en E019; se declara aquí para compartir construcción y pruebas.
	now func() time.Time
}

// NewOnboardingStore construye el almacén real. Si el cliente o la tabla faltan,
// todas las operaciones fallan cerradas: nunca se sustituyen por memoria local.
func NewOnboardingStore(client OnboardingAPI, table string) *OnboardingStore {
	return &OnboardingStore{
		client: client,
		table:  strings.TrimSpace(table),
		now:    func() time.Time { return time.Now().UTC() },
	}
}

// WithClock permite inyectar un reloj determinista en pruebas.
func (s *OnboardingStore) WithClock(now func() time.Time) *OnboardingStore {
	if now != nil {
		s.now = now
	}
	return s
}

// GetOnboarding lee `USER#<id>/ONBOARDING` con `ConsistentRead`. Devuelve
// (`nil`, false, nil) cuando la clave no existe: el llamador responde 404 y no un
// estado vacío inventado.
func (s *OnboardingStore) GetOnboarding(ctx context.Context, userID string) (*appOnboarding.Record, bool, error) {
	if s == nil || s.client == nil || s.table == "" {
		return nil, false, appOnboarding.ErrUnavailable
	}
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, false, appOnboarding.ErrUnavailable
	}

	pk, sk := domainDynamo.OnboardingKeys(userID)
	out, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(s.table),
		Key: map[string]types.AttributeValue{
			"PK": stringAttr(pk),
			"SK": stringAttr(sk),
		},
		// Lectura consistente: la versión y los pasos no pueden venir de una réplica
		// retrasada, porque E019 hace CAS sobre esa versión.
		ConsistentRead: aws.Bool(true),
	})
	if err != nil {
		return nil, false, appOnboarding.ErrUnavailable
	}
	if out == nil || out.Item == nil {
		return nil, false, nil
	}

	item := out.Item
	// La clave `USER#<id>/ONBOARDING` es exclusiva, pero un ítem de otra entidad en la
	// misma clave no se interpreta como onboarding (falla cerrado como ausencia).
	if entity := strings.TrimSpace(stringAttribute(item["entityType"])); entity != "" && entity != "USER_ONBOARDING" {
		return nil, false, nil
	}

	record := &appOnboarding.Record{
		CompletedSteps:   onboardingSteps(item["completedSteps"]),
		MicrophoneRaw:    strings.TrimSpace(stringAttribute(item["microphone"])),
		NotificationsRaw: strings.TrimSpace(stringAttribute(item["notifications"])),
		Configured:       onboardingBool(item["configured"]),
	}
	if version, ok := attributeInt(item["version"]); ok {
		record.Version = version
		record.HasVersion = true
	}
	if completedAt, ok := onboardingTime(item["completedAt"]); ok {
		record.CompletedAt = &completedAt
	}
	return record, true, nil
}

// onboardingSteps lee la lista persistida. Acepta la forma canónica `L` de strings y,
// por compatibilidad con datos heredados, un `SS`. Cualquier otro tipo se ignora y la
// proyección devuelve `[]`.
func onboardingSteps(value types.AttributeValue) []string {
	var raw []string
	switch typed := value.(type) {
	case *types.AttributeValueMemberL:
		for _, item := range typed.Value {
			if text, ok := item.(*types.AttributeValueMemberS); ok {
				raw = append(raw, text.Value)
			}
		}
	case *types.AttributeValueMemberSS:
		raw = append(raw, typed.Value...)
	}
	return appOnboarding.CanonicalSteps(raw)
}

// onboardingBool lee un atributo booleano; ausente o de otro tipo es `false`.
func onboardingBool(value types.AttributeValue) bool {
	if typed, ok := value.(*types.AttributeValueMemberBOOL); ok {
		return typed.Value
	}
	return false
}

// onboardingTime interpreta `completedAt`. El `NULL` explícito y el atributo ausente
// producen `false`, que el contrato proyecta como `null`.
func onboardingTime(value types.AttributeValue) (time.Time, bool) {
	if _, ok := value.(*types.AttributeValueMemberNULL); ok {
		return time.Time{}, false
	}
	texto := strings.TrimSpace(stringAttribute(value))
	if texto == "" {
		return time.Time{}, false
	}
	if parsed, err := time.Parse(time.RFC3339Nano, texto); err == nil {
		return parsed.UTC(), true
	}
	if parsed, err := time.Parse(time.RFC3339, texto); err == nil {
		return parsed.UTC(), true
	}
	return time.Time{}, false
}
