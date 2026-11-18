package dynamo

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	appOnboarding "github.com/nexumpr/Nexum-Backend/internal/application/onboarding"
	domainDynamo "github.com/nexumpr/Nexum-Backend/internal/domain/dynamo"
)

// OnboardingWriteAPI es el subconjunto de escritura que necesita E019. Se declara
// aparte de OnboardingAPI para que la lectura de E018 siga exigiendo sólo `GetItem`
// y para que ninguna ruta pueda usar `Scan`, `Query` ni transacciones sobre la
// partición del usuario.
type OnboardingWriteAPI interface {
	PutItem(ctx context.Context, params *dynamodb.PutItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error)
	UpdateItem(ctx context.Context, params *dynamodb.UpdateItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error)
}

// nombresOnboarding devuelve los alias de expresión del ítem. Se construye un
// mapa nuevo por llamada para que ninguna escritura pueda alterar los alias de
// otra petición concurrente.
func nombresOnboarding() map[string]string {
	return map[string]string{
		"#entityType":     "entityType",
		"#userId":         "userId",
		"#completedSteps": "completedSteps",
		"#done":           "done",
		"#microphone":     "microphone",
		"#notifications":  "notifications",
		"#configured":     "configured",
		"#completedAt":    "completedAt",
		"#updatedAt":      "updatedAt",
		"#version":        "version",
	}
}

// writer devuelve el subconjunto de escritura del cliente. Si el cliente no lo
// implementa (un doble de sólo lectura o un almacén sin persistencia), la
// escritura falla cerrada: nunca se simula el éxito sin guardar.
func (s *OnboardingStore) writer() (OnboardingWriteAPI, bool) {
	if s == nil || s.client == nil {
		return nil, false
	}
	api, ok := s.client.(OnboardingWriteAPI)
	if !ok || api == nil {
		return nil, false
	}
	return api, true
}

// UpdateOnboarding aplica la escritura de E019 con CAS sobre `version`.
//
// Condición:
//   - ítem con atributo `version`: `#version = :leida`;
//   - ítem heredado sin atributo: `attribute_not_exists(#version)`.
//
// La expresión hace `SET` de todos los atributos del onboarding y
// `ADD #version :uno`, que materializa el 1 en un ítem heredado. No se escribe
// `createdAt` (no se sobrescribe la fecha de creación) ni ningún atributo de
// `PROFILE`, sesiones o autenticación.
func (s *OnboardingStore) UpdateOnboarding(ctx context.Context, userID string, write appOnboarding.Write) (*appOnboarding.Record, error) {
	api, ok := s.writer()
	if !ok || s.table == "" {
		return nil, appOnboarding.ErrUnavailable
	}
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, appOnboarding.ErrUnavailable
	}

	ahora := write.Now.UTC()
	if write.Now.IsZero() {
		ahora = s.now().UTC()
	}

	pk, sk := domainDynamo.OnboardingKeys(userID)

	valores := map[string]types.AttributeValue{
		":entityType":     stringAttr("USER_ONBOARDING"),
		":userId":         stringAttr(userID),
		":completedSteps": onboardingStepsAttr(write.CompletedSteps),
		":done":           boolAttr(write.Done),
		":microphone":     stringAttr(write.MicrophoneRaw),
		":notifications":  stringAttr(write.NotificationsRaw),
		":configured":     boolAttr(write.Configured),
		":updatedAt":      stringAttr(ahora.Format(time.RFC3339)),
		":completedAt":    onboardingCompletedAtAttr(write.CompletedAt),
		":uno":            numberAttr(1),
	}

	condicion := "attribute_not_exists(#version)"
	if write.HasVersion {
		condicion = "#version = :leida"
		valores[":leida"] = numberAttr(write.ExpectedVersion)
	}

	const actualizacion = "SET #entityType = :entityType, #userId = :userId, " +
		"#completedSteps = :completedSteps, #done = :done, " +
		"#microphone = :microphone, #notifications = :notifications, " +
		"#configured = :configured, #updatedAt = :updatedAt, " +
		"#completedAt = :completedAt " +
		"ADD #version :uno"

	_, err := api.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(s.table),
		Key: map[string]types.AttributeValue{
			"PK": stringAttr(pk),
			"SK": stringAttr(sk),
		},
		UpdateExpression:          aws.String(actualizacion),
		ConditionExpression:       aws.String(condicion),
		ExpressionAttributeNames:  nombresOnboarding(),
		ExpressionAttributeValues: valores,
	})
	if err != nil {
		return nil, clasificarErrorEscritura(err)
	}

	version := int64(1)
	if write.HasVersion {
		version = write.ExpectedVersion + 1
	}
	return recordFromWrite(write, version), nil
}

// CreateOnboarding crea el ítem con versión 1 cuando el usuario no tiene estado
// de onboarding. La condición `attribute_not_exists(PK)` convierte una creación
// simultánea en un conflicto que el servicio resuelve con una única relectura.
func (s *OnboardingStore) CreateOnboarding(ctx context.Context, userID string, write appOnboarding.Write) (*appOnboarding.Record, error) {
	api, ok := s.writer()
	if !ok || s.table == "" {
		return nil, appOnboarding.ErrUnavailable
	}
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, appOnboarding.ErrUnavailable
	}

	ahora := write.Now.UTC()
	if write.Now.IsZero() {
		ahora = s.now().UTC()
	}
	marca := ahora.Format(time.RFC3339)

	pk, sk := domainDynamo.OnboardingKeys(userID)
	item := map[string]types.AttributeValue{
		"PK":             stringAttr(pk),
		"SK":             stringAttr(sk),
		"entityType":     stringAttr("USER_ONBOARDING"),
		"userId":         stringAttr(userID),
		"completedSteps": onboardingStepsAttr(write.CompletedSteps),
		"done":           boolAttr(write.Done),
		"microphone":     stringAttr(write.MicrophoneRaw),
		"notifications":  stringAttr(write.NotificationsRaw),
		"configured":     boolAttr(write.Configured),
		"completedAt":    onboardingCompletedAtAttr(write.CompletedAt),
		"createdAt":      stringAttr(marca),
		"updatedAt":      stringAttr(marca),
		"version":        numberAttr(1),
	}

	_, err := api.PutItem(ctx, &dynamodb.PutItemInput{
		TableName:           aws.String(s.table),
		Item:                item,
		ConditionExpression: aws.String("attribute_not_exists(#pk)"),
		ExpressionAttributeNames: map[string]string{
			"#pk": "PK",
		},
	})
	if err != nil {
		return nil, clasificarErrorEscritura(err)
	}
	return recordFromWrite(write, 1), nil
}

// clasificarErrorEscritura distingue un conflicto de versión de un fallo de
// servicio. Cualquier otro error se degrada a ErrUnavailable (503) sin exponer
// la tabla, la clave ni el mensaje del SDK.
func clasificarErrorEscritura(err error) error {
	var condicional *types.ConditionalCheckFailedException
	if errors.As(err, &condicional) {
		return appOnboarding.ErrConflict
	}
	return appOnboarding.ErrUnavailable
}

// recordFromWrite construye la fila resultante de una escritura confirmada. El
// CAS garantiza que el valor persistido es exactamente el calculado, de modo que
// la respuesta no necesita una segunda lectura.
func recordFromWrite(write appOnboarding.Write, version int64) *appOnboarding.Record {
	record := &appOnboarding.Record{
		CompletedSteps:   appOnboarding.CanonicalSteps(write.CompletedSteps),
		Version:          version,
		HasVersion:       true,
		MicrophoneRaw:    write.MicrophoneRaw,
		NotificationsRaw: write.NotificationsRaw,
		Configured:       write.Configured,
	}
	if write.CompletedAt != nil && !write.CompletedAt.IsZero() {
		marca := write.CompletedAt.UTC()
		record.CompletedAt = &marca
	}
	return record
}

// onboardingStepsAttr serializa `completedSteps` como lista de strings. Un
// conjunto vacío se persiste como `L` vacía (DynamoDB admite listas vacías, no
// conjuntos vacíos), de modo que E018 lo proyecta `[]` y nunca `null`.
func onboardingStepsAttr(steps []string) types.AttributeValue {
	items := make([]types.AttributeValue, 0, len(steps))
	for _, step := range steps {
		items = append(items, stringAttr(step))
	}
	return &types.AttributeValueMemberL{Value: items}
}

// onboardingCompletedAtAttr escribe la marca RFC3339 o un `NULL` explícito. El
// contrato exige `completedAt: null` mientras el onboarding no está completo.
func onboardingCompletedAtAttr(value *time.Time) types.AttributeValue {
	if value == nil || value.IsZero() {
		return &types.AttributeValueMemberNULL{Value: true}
	}
	return stringAttr(value.UTC().Format(time.RFC3339))
}
