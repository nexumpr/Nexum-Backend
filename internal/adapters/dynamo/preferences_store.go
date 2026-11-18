package dynamo

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	appPreferences "github.com/nexumpr/Nexum-Backend/internal/application/preferences"
	domainDynamo "github.com/nexumpr/Nexum-Backend/internal/domain/dynamo"
)

// PreferencesAPI es el subconjunto del cliente DynamoDB que usan E068/E069. Se declara aquí (y
// no se reutiliza una interfaz más amplia) para que el compilador impida `Scan`: las
// preferencias se resuelven siempre por clave exacta en la partición del titular.
type PreferencesAPI interface {
	GetItem(ctx context.Context, params *dynamodb.GetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error)
	PutItem(ctx context.Context, params *dynamodb.PutItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error)
}

const (
	// preferencesSortKey es la SK canónica del item de preferencias dentro de `USER#<id>`
	// (DISEÑO_OLEADA_1 §4). No se añade a `internal/domain/dynamo` para no tocar archivos
	// compartidos durante la oleada en paralelo; la clave es la del diseño aprobado.
	preferencesSortKey = "PREFERENCES"

	// preferencesEntityType identifica el item para no confundirlo con cualquier otra entidad
	// que ocupara la misma clave.
	preferencesEntityType = "USER_PREFERENCES"
)

// PreferencesStore implementa `preferences.Store` sobre la tabla única existente.
//
// Patrón físico (DISEÑO_OLEADA_1 §4):
//
//	PK = USER#<userId>   SK = PREFERENCES
//
// Atributos: `entityType`, `userId`, `locale`, `timezone`, `timeZone`, `version`, `createdAt`
// y `updatedAt`. Las claves ya están autorizadas por la política IAM vigente (`USER#*`): esta
// unidad no requiere cambios de IAM, tabla ni índices.
type PreferencesStore struct {
	client PreferencesAPI
	table  string
}

// NewPreferencesStore construye el adaptador. Sin cliente o tabla todas las operaciones fallan
// cerradas (ErrPreferencesUnavailable): nunca se sustituyen por memoria local.
func NewPreferencesStore(client PreferencesAPI, table string) *PreferencesStore {
	return &PreferencesStore{client: client, table: strings.TrimSpace(table)}
}

// preferencesKeys devuelve la clave compuesta del item de preferencias del titular.
func preferencesKeys(userID string) (pk, sk string) {
	return domainDynamo.PrefixUser + userID, preferencesSortKey
}

// LoadPreferences lee `USER#<id>/PREFERENCES` con consistencia fuerte. Devuelve (nil, nil) si
// el item no existe o si la clave la ocupa otra entidad: preferencias inexistentes no son un
// error, y un item ajeno nunca se interpreta como preferencias.
func (s *PreferencesStore) LoadPreferences(ctx context.Context, userID string) (*appPreferences.Preferences, error) {
	if s == nil || s.client == nil || s.table == "" {
		return nil, appPreferences.ErrPreferencesUnavailable
	}
	if strings.TrimSpace(userID) == "" {
		return nil, nil
	}

	pk, sk := preferencesKeys(userID)
	out, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(s.table),
		Key: map[string]types.AttributeValue{
			"PK": stringAttr(pk),
			"SK": stringAttr(sk),
		},
		// La versión leída decide el CAS de E069: una réplica retrasada provocaría 412
		// espurios o, peor, una escritura sobre una versión ya superada.
		ConsistentRead: aws.Bool(true),
	})
	if err != nil {
		return nil, appPreferences.ErrPreferencesUnavailable
	}
	if out == nil || len(out.Item) == 0 {
		return nil, nil
	}
	if entity := stringAttribute(out.Item["entityType"]); entity != "" && entity != preferencesEntityType {
		return nil, nil
	}

	prefs := &appPreferences.Preferences{
		Locale:   stringAttribute(out.Item["locale"]),
		Timezone: stringAttribute(out.Item["timezone"]),
	}
	if prefs.Timezone == "" {
		// Compatibilidad con un item que sólo guardara el alias A8.
		prefs.Timezone = stringAttribute(out.Item["timeZone"])
	}
	if version, ok := attributeInt(out.Item["version"]); ok && version >= 1 {
		prefs.Version = version
		prefs.HasVersion = true
	}
	return prefs, nil
}

// CreatePreferences materializa los valores por defecto con un `PutItem` condicional
// `attribute_not_exists(PK)`.
//
// Si otra petición ganó la carrera, la condición falla y se devuelve el item ya existente: la
// creación perezosa es idempotente y nunca sobrescribe preferencias modificadas por E069 entre
// la lectura y la escritura.
func (s *PreferencesStore) CreatePreferences(ctx context.Context, userID string, defaults appPreferences.Preferences, now time.Time) (*appPreferences.Preferences, error) {
	if s == nil || s.client == nil || s.table == "" {
		return nil, appPreferences.ErrPreferencesUnavailable
	}
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, appPreferences.ErrPreferencesUnavailable
	}

	locale := strings.TrimSpace(defaults.Locale)
	if locale == "" {
		locale = appPreferences.DefaultLocale
	}
	timezone := strings.TrimSpace(defaults.Timezone)
	if timezone == "" {
		timezone = appPreferences.DefaultTimezone
	}
	version := defaults.Version
	if version < 1 {
		version = appPreferences.DefaultVersion
	}
	nowISO := now.UTC().Format(time.RFC3339)

	pk, sk := preferencesKeys(userID)
	_, err := s.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(s.table),
		Item: map[string]types.AttributeValue{
			"PK":         stringAttr(pk),
			"SK":         stringAttr(sk),
			"entityType": stringAttr(preferencesEntityType),
			"userId":     stringAttr(userID),
			"locale":     stringAttr(locale),
			"timezone":   stringAttr(timezone),
			// A8: el alias se persiste junto al valor canónico para que cualquier lector vea
			// el mismo dato sin traducir en vuelo.
			"timeZone":  stringAttr(timezone),
			"version":   numberAttr(version),
			"createdAt": stringAttr(nowISO),
			"updatedAt": stringAttr(nowISO),
		},
		ConditionExpression: aws.String("attribute_not_exists(PK)"),
	})
	if err == nil {
		return &appPreferences.Preferences{
			Locale:     locale,
			Timezone:   timezone,
			Version:    version,
			HasVersion: true,
		}, nil
	}

	var condicional *types.ConditionalCheckFailedException
	if !errors.As(err, &condicional) {
		return nil, appPreferences.ErrPreferencesUnavailable
	}

	// Otra petición creó el item entre la lectura y el PutItem: el ganador es la autoridad.
	existente, errLectura := s.LoadPreferences(ctx, userID)
	if errLectura != nil || existente == nil || !existente.HasVersion {
		return nil, appPreferences.ErrPreferencesConflict
	}
	return existente, nil
}
