package dynamo

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	appExport "github.com/nexumpr/Nexum-Backend/internal/application/export"
	appGoals "github.com/nexumpr/Nexum-Backend/internal/application/goals"
	appIdentity "github.com/nexumpr/Nexum-Backend/internal/application/identity"
	appMovements "github.com/nexumpr/Nexum-Backend/internal/application/movements"
	appOnboarding "github.com/nexumpr/Nexum-Backend/internal/application/onboarding"
	appPreferences "github.com/nexumpr/Nexum-Backend/internal/application/preferences"
	appPrivacy "github.com/nexumpr/Nexum-Backend/internal/application/privacy"
	appProfile "github.com/nexumpr/Nexum-Backend/internal/application/profile"
	appRecurring "github.com/nexumpr/Nexum-Backend/internal/application/recurring"
	domainDynamo "github.com/nexumpr/Nexum-Backend/internal/domain/dynamo"
)

// Transporte de datos de E020 (`GET /api/v1/users/me/export`).
//
// El adaptador reutiliza los lectores ya publicados para las familias que ya tienen uno
// (movimientos, metas, perfil, ingreso, consentimientos, recurrentes, onboarding y
// preferencias) y lee directamente las dos familias cuyo adaptador aún no existe en esta rama
// (aportaciones y presupuestos), con el MISMO formato de item que fija RFC-022:
//
//   - `MovementsStore.ListMovements`      · `Query` con `begins_with(SK, "MOVEMENT#")`.
//   - `ContributionsStore` (RFC-022 §5)   · `Query` con `begins_with(SK, "CONTRIBUTION#")`.
//   - `RecurringStore.List`               · `Query` con `begins_with(SK, "RECURRING#")`.
//   - `GoalsStore.ListUserGoals`          · `Query` con `begins_with(SK, "GOAL#")`.
//   - `BudgetSetStore` (RFC-022 §3)       · `Query` con `begins_with(SK, "BUDGETSET#")`.
//   - `ProfileStore`/`profile.Service`    · `GetItem` de `USER#<id>/PROFILE`.
//   - `IncomeStore`/`identity.Service`    · `GetItem` de `USER#<id>/INCOME`.
//   - `ConsentsStore.ListUserConsents`    · `Query` con `begins_with(SK, "CONSENT#")`.
//   - `OnboardingStore`                   · `GetItem` de `USER#<id>/ONBOARDING`.
//   - `PreferencesStore`                  · `GetItem` de `USER#<id>/PREFERENCES`.
//
// Ninguno de esos puertos declara `Scan`, de modo que el compilador impide introducirlo. No se
// crean tablas, índices, particiones ni permisos: todas las claves viven en `USER#<id>`, que la
// política IAM vigente ya autoriza con `LeadingKeys` `USER#*`.

// exportConsentsMaxPaginas acota el número de páginas internas con las que se recorre la
// colección completa de consentimientos (singleton que viaja entero en cada página de la
// exportación). Agotar el tope NO trunca la lista: se falla cerrado con ErrUnavailable, de modo
// que nunca se presenta una colección incompleta como completa.
const exportConsentsMaxPaginas = 50

// exportPresupuestosMaxPaginas acota las páginas internas de una llamada de presupuestos. Un
// periodo puede contener hasta 50 filas, así que varias páginas pueden ser necesarias para
// llenar la página de la exportación. Agotar el tope con filas acumuladas devuelve la última
// clave como continuación (nunca se declara agotada una colección sin verla terminar); agotarlo
// sin ninguna fila falla cerrado.
const exportPresupuestosMaxPaginas = 50

// ExportDynamoAPI es el subconjunto del cliente DynamoDB que usa la exportación para las
// familias sin adaptador publicado. No incluye `Scan`.
type ExportDynamoAPI interface {
	Query(ctx context.Context, params *dynamodb.QueryInput, optFns ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error)
	GetItem(ctx context.Context, params *dynamodb.GetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error)
}

// ExportMovementsReader es el lector de movimientos que usa la exportación.
type ExportMovementsReader interface {
	ListMovements(ctx context.Context, query appMovements.ListQuery, startSK string, limit int) ([]appMovements.Movement, string, error)
}

// ExportRecurringReader es el lector de reglas recurrentes (E032).
type ExportRecurringReader interface {
	List(ctx context.Context, userID, startSK string, limit int, filter appRecurring.Filter) ([]appRecurring.Payment, string, error)
}

// ExportGoalsReader es el lector de metas que usa la exportación.
type ExportGoalsReader interface {
	ListUserGoals(ctx context.Context, query appGoals.ListStoreQuery) ([]appGoals.GoalRow, string, error)
}

// ExportProfileReader es el lector del perfil que usa la exportación (`profile.Service.Get`
// aplica la misma validación y proyección que E012).
type ExportProfileReader interface {
	Get(ctx context.Context, userID string) (*appProfile.Profile, error)
}

// ExportIncomeReader es el lector del ingreso base (E024).
type ExportIncomeReader interface {
	GetIncome(ctx context.Context, userID string) (*appIdentity.IncomeSnapshot, error)
}

// ExportConsentsReader es el lector de consentimientos que usa la exportación.
type ExportConsentsReader interface {
	ListUserConsents(ctx context.Context, userID, startSK string, limit int) ([]appPrivacy.ConsentRecord, string, error)
}

// ExportOnboardingReader es el lector del estado de onboarding (E018).
type ExportOnboardingReader interface {
	Get(ctx context.Context, userID string) (*appOnboarding.State, error)
}

// ExportPreferencesReader es el lector de preferencias. Se usa el almacén y no el servicio de
// E068 a propósito: una exportación NO debe materializar el item por defecto (sería una
// escritura durante una lectura); el contrato declara `preferences` anulable.
type ExportPreferencesReader interface {
	LoadPreferences(ctx context.Context, userID string) (*appPreferences.Preferences, error)
}

// ExportStoreDeps agrupa las dependencias del adaptador. Una dependencia ausente hace que la
// operación correspondiente falle cerrada con ErrUnavailable: nunca se sustituye por memoria
// local.
type ExportStoreDeps struct {
	Cliente         ExportDynamoAPI
	Tabla           string
	Movimientos     ExportMovementsReader
	Recurrentes     ExportRecurringReader
	Metas           ExportGoalsReader
	Perfil          ExportProfileReader
	Ingreso         ExportIncomeReader
	Consentimientos ExportConsentsReader
	Onboarding      ExportOnboardingReader
	Preferencias    ExportPreferencesReader
}

// ExportStore implementa `export.Fuente` componiendo los lectores reales.
type ExportStore struct {
	cliente         ExportDynamoAPI
	tabla           string
	movimientos     ExportMovementsReader
	recurrentes     ExportRecurringReader
	metas           ExportGoalsReader
	perfil          ExportProfileReader
	ingreso         ExportIncomeReader
	consentimientos ExportConsentsReader
	onboarding      ExportOnboardingReader
	preferencias    ExportPreferencesReader
}

// NewExportStore construye el adaptador.
func NewExportStore(deps ExportStoreDeps) *ExportStore {
	return &ExportStore{
		cliente:         deps.Cliente,
		tabla:           strings.TrimSpace(deps.Tabla),
		movimientos:     deps.Movimientos,
		recurrentes:     deps.Recurrentes,
		metas:           deps.Metas,
		perfil:          deps.Perfil,
		ingreso:         deps.Ingreso,
		consentimientos: deps.Consentimientos,
		onboarding:      deps.Onboarding,
		preferencias:    deps.Preferencias,
	}
}

// Movimientos devuelve hasta `limit` movimientos del titular en orden ascendente por `SK` desde
// `after` (exclusivo) y la `SK` desde la que continuar (vacía cuando la colección está agotada).
//
// El cursor del listado de E027 usa exactamente esa `SK` física, de modo que la exportación no
// reconstruye posiciones ni puede saltarse filas. Un fallo del almacén se traduce a
// ErrUnavailable y NUNCA a una colección vacía.
func (s *ExportStore) Movimientos(ctx context.Context, userID, after string, limit int) ([]appMovements.Movement, string, error) {
	if s == nil || s.movimientos == nil {
		return nil, "", appExport.ErrUnavailable
	}
	if strings.TrimSpace(userID) == "" || limit < 1 || limit > appExport.MaxLimit {
		return nil, "", appExport.ErrUnavailable
	}
	filas, siguiente, err := s.movimientos.ListMovements(ctx, appMovements.ListQuery{UserID: userID}, after, limit)
	if err != nil {
		return nil, "", appExport.ErrUnavailable
	}
	return filas, siguiente, nil
}

// Recurrentes devuelve hasta `limit` reglas recurrentes en orden ascendente por `SK`.
//
// `Filter{}` significa «sin filtro»: la exportación recorre la colección completa, incluidas
// las reglas inactivas (`active=false`), porque una exportación de datos propios no oculta
// historia.
func (s *ExportStore) Recurrentes(ctx context.Context, userID, after string, limit int) ([]appRecurring.Payment, string, error) {
	if s == nil || s.recurrentes == nil {
		return nil, "", appExport.ErrUnavailable
	}
	if strings.TrimSpace(userID) == "" || limit < 1 || limit > appExport.MaxLimit {
		return nil, "", appExport.ErrUnavailable
	}
	filas, siguiente, err := s.recurrentes.List(ctx, userID, after, limit, appRecurring.Filter{})
	if err != nil {
		return nil, "", appExport.ErrUnavailable
	}
	return filas, siguiente, nil
}

// Metas devuelve hasta `limit` metas del titular en orden ascendente por `SK` desde `after`
// (exclusivo) junto a la `SK` de continuación.
//
// `HasStatus` se deja en falso a propósito: la exportación no filtra, recorre la colección
// completa. Un item incoherente se propaga como ErrInconsistent para que el servicio responda
// 500 en lugar de omitirlo en silencio.
func (s *ExportStore) Metas(ctx context.Context, userID, after string, limit int) ([]appGoals.GoalRow, string, error) {
	if s == nil || s.metas == nil {
		return nil, "", appExport.ErrUnavailable
	}
	if strings.TrimSpace(userID) == "" || limit < 1 || limit > appExport.MaxLimit {
		return nil, "", appExport.ErrUnavailable
	}
	filas, siguiente, err := s.metas.ListUserGoals(ctx, appGoals.ListStoreQuery{
		UserID:  userID,
		StartSK: after,
		Limit:   limit,
	})
	if err != nil {
		if errors.Is(err, appGoals.ErrInconsistent) {
			return nil, "", appExport.ErrInconsistent
		}
		return nil, "", appExport.ErrUnavailable
	}
	return filas, siguiente, nil
}

// Perfil devuelve el perfil del titular con la misma proyección de seis campos que E012.
//
// `(nil, nil)` significa que el perfil no existe: el servicio responde 404 porque la
// exportación no puede cumplir el campo obligatorio `profile`. Un fallo de lectura se traduce a
// ErrUnavailable y no se convierte en «no existe».
func (s *ExportStore) Perfil(ctx context.Context, userID string) (*appProfile.Profile, error) {
	if s == nil || s.perfil == nil {
		return nil, appExport.ErrUnavailable
	}
	perfil, err := s.perfil.Get(ctx, userID)
	if err != nil {
		switch {
		case errors.Is(err, appProfile.ErrProfileNotFound):
			return nil, nil
		case errors.Is(err, appProfile.ErrProfileUnauthorized):
			return nil, appExport.ErrUnauthorized
		default:
			return nil, appExport.ErrUnavailable
		}
	}
	return perfil, nil
}

// Ingreso devuelve el ingreso base del titular. `(nil, nil)` cuando no está configurado: el
// contrato declara `income` anulable y la exportación no inventa importes ni frecuencias.
func (s *ExportStore) Ingreso(ctx context.Context, userID string) (*appIdentity.IncomeSnapshot, error) {
	if s == nil || s.ingreso == nil {
		return nil, appExport.ErrUnavailable
	}
	if strings.TrimSpace(userID) == "" {
		return nil, appExport.ErrUnauthorized
	}
	snapshot, err := s.ingreso.GetIncome(ctx, userID)
	if err != nil {
		switch {
		case errors.Is(err, appIdentity.ErrIncomeNotFound):
			return nil, nil
		case errors.Is(err, appIdentity.ErrIncomeUnauthorized):
			return nil, appExport.ErrUnauthorized
		case errors.Is(err, appIdentity.ErrIncomeInconsistent):
			return nil, appExport.ErrInconsistent
		default:
			return nil, appExport.ErrUnavailable
		}
	}
	return snapshot, nil
}

// Onboarding devuelve el estado de onboarding. `(nil, nil)` cuando el titular no tiene item:
// el contrato declara `onboarding` anulable.
func (s *ExportStore) Onboarding(ctx context.Context, userID string) (*appOnboarding.State, error) {
	if s == nil || s.onboarding == nil {
		return nil, appExport.ErrUnavailable
	}
	if strings.TrimSpace(userID) == "" {
		return nil, appExport.ErrUnauthorized
	}
	estado, err := s.onboarding.Get(ctx, userID)
	if err != nil {
		switch {
		case errors.Is(err, appOnboarding.ErrNotFound):
			return nil, nil
		case errors.Is(err, appOnboarding.ErrUnauthorized):
			return nil, appExport.ErrUnauthorized
		default:
			return nil, appExport.ErrUnavailable
		}
	}
	return estado, nil
}

// Preferencias devuelve las preferencias sin materializar el item por defecto.
func (s *ExportStore) Preferencias(ctx context.Context, userID string) (*appPreferences.Preferences, error) {
	if s == nil || s.preferencias == nil {
		return nil, appExport.ErrUnavailable
	}
	if strings.TrimSpace(userID) == "" {
		return nil, appExport.ErrUnauthorized
	}
	prefs, err := s.preferencias.LoadPreferences(ctx, userID)
	if err != nil {
		if errors.Is(err, appPreferences.ErrPreferencesUnauthorized) {
			return nil, appExport.ErrUnauthorized
		}
		return nil, appExport.ErrUnavailable
	}
	return prefs, nil
}

// Consentimientos devuelve la colección COMPLETA de consentimientos del titular, que RFC-022
// §2.2 incluye entera en cada página de la exportación.
//
// Se recorre con `ListUserConsents` (página máxima del contrato) siguiendo `nextSK` hasta
// agotarla. El bucle exige progreso estricto: si la persistencia no avanza, o si se supera el
// tope de páginas internas, se devuelve ErrUnavailable. Nunca se devuelve una lista truncada
// como si fuera completa.
func (s *ExportStore) Consentimientos(ctx context.Context, userID string) ([]appPrivacy.ConsentRecord, error) {
	if s == nil || s.consentimientos == nil {
		return nil, appExport.ErrUnavailable
	}
	if strings.TrimSpace(userID) == "" {
		return nil, appExport.ErrUnauthorized
	}

	acumulado := make([]appPrivacy.ConsentRecord, 0)
	desde := ""
	for pagina := 0; pagina < exportConsentsMaxPaginas; pagina++ {
		filas, siguiente, err := s.consentimientos.ListUserConsents(ctx, userID, desde, appPrivacy.ConsentsMaxLimit)
		if err != nil {
			return nil, appExport.ErrUnavailable
		}
		acumulado = append(acumulado, filas...)
		if siguiente == "" {
			return acumulado, nil
		}
		if siguiente == desde {
			// Sin avance no se puede garantizar progreso: se falla cerrado en lugar de
			// repetir indefinidamente la misma página.
			return nil, appExport.ErrUnavailable
		}
		desde = siguiente
	}
	return nil, appExport.ErrUnavailable
}

// Aportaciones devuelve hasta `limit` aportaciones en orden ascendente por `SK`
// (`CONTRIBUTION#<occurredAt>#<id>`) desde `after` (exclusivo).
//
// Se pide una fila de más para distinguir «página llena con más datos» de «colección agotada»
// sin recurrir a `Scan`. Un item bajo el prefijo que no sea una aportación legible se trata como
// incoherente: la exportación no omite datos en silencio.
func (s *ExportStore) Aportaciones(ctx context.Context, userID, after string, limit int) ([]appExport.ContributionRow, string, error) {
	if s == nil || s.cliente == nil || s.tabla == "" {
		return nil, "", appExport.ErrUnavailable
	}
	userID = strings.TrimSpace(userID)
	if userID == "" || limit < 1 || limit > appExport.MaxLimit {
		return nil, "", appExport.ErrUnavailable
	}

	pk := domainDynamo.PrefixUser + userID
	entrada := &dynamodb.QueryInput{
		TableName:              aws.String(s.tabla),
		ConsistentRead:         aws.Bool(true),
		KeyConditionExpression: aws.String("PK = :pk AND begins_with(SK, :prefijo)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk":      stringAttr(pk),
			":prefijo": stringAttr(appExport.PrefijoAportacion),
		},
		ScanIndexForward: aws.Bool(true),
		Limit:            aws.Int32(int32(limit + 1)),
	}
	if after != "" {
		entrada.ExclusiveStartKey = map[string]types.AttributeValue{
			"PK": stringAttr(pk),
			"SK": stringAttr(after),
		}
	}

	salida, err := s.cliente.Query(ctx, entrada)
	if err != nil {
		return nil, "", appExport.ErrUnavailable
	}
	if salida == nil {
		return nil, "", nil
	}

	filas := make([]appExport.ContributionRow, 0, limit)
	ultimaSK := ""
	for _, item := range salida.Items {
		if len(filas) == limit {
			// El item extra confirma que queda al menos una aportación más.
			return filas, ultimaSK, nil
		}
		fila, errFila := contributionRowFromItem(item, userID)
		if errFila != nil {
			return nil, "", errFila
		}
		filas = append(filas, fila)
		ultimaSK = stringAttribute(item["SK"])
	}
	if len(salida.LastEvaluatedKey) > 0 {
		if len(filas) == 0 {
			// Sin progreso no hay clave de continuación válida.
			return nil, "", appExport.ErrUnavailable
		}
		return filas, ultimaSK, nil
	}
	return filas, "", nil
}

// Presupuestos devuelve hasta `limit` filas `Budget` en orden ascendente por la clave sintética
// `BUDGETSET#<period>#<categoryId>`.
//
// Cada `BUDGETSET#<period>` es un item único con hasta 50 categorías (RFC-022 §3.1), de modo
// que la página puede cortarse a mitad de un periodo: la continuación es la clave sintética
// exacta y la siguiente llamada reanuda dentro del periodo correspondiente.
//
// La reanudación NO usa un rango abierto sobre la `SK`. DynamoDB admite una sola condición de
// rango por consulta y no permite combinar `begins_with` con `>=` sobre la misma clave; además,
// `SK >= "BUDGETSET#<periodo>"` no se limita al prefijo de presupuestos y arrastraría todos los
// prefijos posteriores de la partición (`CONSENT#`, `GOAL#`, `MOVEMENT#`...), que no son
// presupuestos. Por eso la primera consulta se restringe al periodo de continuación con
// `begins_with(SK, "BUDGETSET#<periodo>")` y, agotado ese periodo, el recorrido continúa por la
// colección completa desde la clave del periodo siguiente.
func (s *ExportStore) Presupuestos(ctx context.Context, userID, after string, limit int) ([]appExport.BudgetRow, string, error) {
	if s == nil || s.cliente == nil || s.tabla == "" {
		return nil, "", appExport.ErrUnavailable
	}
	userID = strings.TrimSpace(userID)
	if userID == "" || limit < 1 || limit > appExport.MaxLimit {
		return nil, "", appExport.ErrUnavailable
	}

	pk := domainDynamo.PrefixUser + userID
	prefijo := appExport.PrefijoPresupuesto
	periodoContinuacion := ""
	var inicio map[string]types.AttributeValue
	if after != "" {
		periodoContinuacion = periodoDeClave(after)
		if periodoContinuacion == "" {
			return nil, "", appExport.ErrUnavailable
		}
		// Primera consulta: sólo las filas restantes del periodo donde se cortó la página.
		prefijo = periodoContinuacion
	}

	filas := make([]appExport.BudgetRow, 0, limit+1)
	for pagina := 0; pagina < exportPresupuestosMaxPaginas; pagina++ {
		entrada := &dynamodb.QueryInput{
			TableName:              aws.String(s.tabla),
			ConsistentRead:         aws.Bool(true),
			KeyConditionExpression: aws.String("PK = :pk AND begins_with(SK, :prefijo)"),
			ExpressionAttributeValues: map[string]types.AttributeValue{
				":pk":      stringAttr(pk),
				":prefijo": stringAttr(prefijo),
			},
			ScanIndexForward: aws.Bool(true),
			Limit:            aws.Int32(int32(limit + 1)),
		}
		if len(inicio) > 0 {
			entrada.ExclusiveStartKey = inicio
		}
		salida, err := s.cliente.Query(ctx, entrada)
		if err != nil {
			return nil, "", appExport.ErrUnavailable
		}
		if salida == nil {
			return filas, "", nil
		}
		for _, item := range salida.Items {
			periodo := strings.TrimPrefix(stringAttribute(item["SK"]), appExport.PrefijoPresupuesto)
			conjunto, errConjunto := presupuestoRowsFromItem(item, userID, periodo)
			if errConjunto != nil {
				return nil, "", errConjunto
			}
			for _, fila := range conjunto {
				clave := claveDePresupuesto(fila)
				if after != "" && clave <= after {
					continue
				}
				if len(filas) == limit {
					return filas, claveDePresupuesto(filas[len(filas)-1]), nil
				}
				filas = append(filas, fila)
			}
		}
		if len(salida.LastEvaluatedKey) == 0 {
			if periodoContinuacion != "" {
				// El periodo de continuación se agotó (o ya no existe). El recorrido sigue por
				// la colección completa a partir del periodo siguiente; terminar aquí
				// declararía completa una exportación que aún tiene presupuestos.
				inicio = map[string]types.AttributeValue{
					"PK": stringAttr(pk),
					"SK": stringAttr(periodoContinuacion),
				}
				periodoContinuacion = ""
				prefijo = appExport.PrefijoPresupuesto
				continue
			}
			return filas, "", nil
		}
		periodoContinuacion = ""
		prefijo = appExport.PrefijoPresupuesto
		inicio = salida.LastEvaluatedKey
	}
	if len(filas) > 0 {
		return filas, claveDePresupuesto(filas[len(filas)-1]), nil
	}
	return nil, "", appExport.ErrUnavailable
}

// claveDePresupuesto compone la clave de continuación sintética de una fila.
func claveDePresupuesto(fila appExport.BudgetRow) string {
	return appExport.PrefijoPresupuesto + fila.Period + "#" + fila.CategoryID
}

// periodoDeClave extrae `BUDGETSET#<period>` de una clave sintética. Devuelve "" si la clave no
// tiene la forma esperada.
func periodoDeClave(clave string) string {
	partes := strings.Split(clave, "#")
	if len(partes) < 2 || appExport.PrefijoPresupuesto != partes[0]+"#" {
		return ""
	}
	periodo := strings.TrimSpace(partes[1])
	if periodo == "" {
		return ""
	}
	return appExport.PrefijoPresupuesto + periodo
}

type exportAllocationJSON struct {
	GoalID      string `json:"goalId"`
	AmountCents int64  `json:"amountCents"`
}

// contributionRowFromItem interpreta el item `CONTRIBUTION#` con el MISMO formato que fija
// RFC-022 §5 (y que usa E045): `amountCents`, `currency`, `allocationsJSON`, `occurredAt`
// ordenable, `status` y `version`.
func contributionRowFromItem(item map[string]types.AttributeValue, userID string) (appExport.ContributionRow, error) {
	if entidad := stringAttribute(item["entityType"]); entidad != "" && entidad != "CONTRIBUTION" {
		return appExport.ContributionRow{}, appExport.ErrInconsistent
	}
	if titular := stringAttribute(item["userId"]); titular != "" && titular != userID {
		return appExport.ContributionRow{}, appExport.ErrInconsistent
	}
	id := strings.TrimSpace(stringAttribute(item["contributionId"]))
	importe, okImporte := attributeInt(item["amountCents"])
	if id == "" || !okImporte {
		return appExport.ContributionRow{}, appExport.ErrInconsistent
	}
	ocurrida, okOcurrida := appMovements.ParseStoredOccurredAt(stringAttribute(item["occurredAt"]))
	if !okOcurrida {
		return appExport.ContributionRow{}, appExport.ErrInconsistent
	}
	estado := strings.TrimSpace(stringAttribute(item["status"]))
	if estado != appExport.EstadoAportacionActiva && estado != appExport.EstadoAportacionRevertida {
		return appExport.ContributionRow{}, appExport.ErrInconsistent
	}
	version, okVersion := attributeInt(item["version"])
	if !okVersion || version < 1 {
		// Mismo criterio que E045: un item heredado sin versión observable es la versión 1.
		version = 1
	}

	asignaciones := make([]appExport.AllocationRow, 0)
	if crudo := stringAttribute(item["allocationsJSON"]); crudo != "" {
		var leidas []exportAllocationJSON
		if err := json.Unmarshal([]byte(crudo), &leidas); err != nil {
			return appExport.ContributionRow{}, appExport.ErrInconsistent
		}
		for _, leida := range leidas {
			if strings.TrimSpace(leida.GoalID) == "" || leida.AmountCents <= 0 {
				return appExport.ContributionRow{}, appExport.ErrInconsistent
			}
			asignaciones = append(asignaciones, appExport.AllocationRow{
				GoalID:      leida.GoalID,
				AmountCents: leida.AmountCents,
			})
		}
	}

	return appExport.ContributionRow{
		ID:          id,
		AmountCents: importe,
		Currency:    strings.TrimSpace(stringAttribute(item["currency"])),
		Allocations: asignaciones,
		OccurredAt:  ocurrida,
		Status:      estado,
		Version:     version,
	}, nil
}

type exportBudgetJSON struct {
	CategoryID       string `json:"categoryId"`
	LimitAmountCents int64  `json:"limitAmountCents"`
}

// presupuestoRowsFromItem interpreta el item `BUDGETSET#<period>` con el MISMO formato que fija
// RFC-022 §3.1 y que usa E048/E049: `itemsJSON`, `currency` y `version` del conjunto.
//
// Las filas se ordenan por `categoryId` para que la clave sintética del cursor sea monótona.
func presupuestoRowsFromItem(item map[string]types.AttributeValue, userID, periodo string) ([]appExport.BudgetRow, error) {
	if entidad := stringAttribute(item["entityType"]); entidad != "" && entidad != "BUDGET_SET" {
		return nil, appExport.ErrInconsistent
	}
	if titular := stringAttribute(item["userId"]); titular != "" && titular != userID {
		return nil, appExport.ErrInconsistent
	}
	version, okVersion := attributeInt(item["version"])
	if !okVersion || version < 1 {
		return nil, appExport.ErrInconsistent
	}
	var crudos []exportBudgetJSON
	if err := json.Unmarshal([]byte(stringAttribute(item["itemsJSON"])), &crudos); err != nil {
		return nil, appExport.ErrInconsistent
	}
	sort.Slice(crudos, func(i, j int) bool { return crudos[i].CategoryID < crudos[j].CategoryID })
	moneda := strings.TrimSpace(stringAttribute(item["currency"]))
	if moneda == "" {
		moneda = appExport.MonedaMXN
	}
	filas := make([]appExport.BudgetRow, 0, len(crudos))
	for _, crudo := range crudos {
		if strings.TrimSpace(crudo.CategoryID) == "" || crudo.LimitAmountCents < 0 {
			return nil, appExport.ErrInconsistent
		}
		filas = append(filas, appExport.BudgetRow{
			CategoryID:       crudo.CategoryID,
			Period:           periodo,
			LimitAmountCents: crudo.LimitAmountCents,
			Currency:         moneda,
			Version:          version,
		})
	}
	return filas, nil
}

// ---------------------------------------------------------------------------------------
// Familias transversales: bandeja de notificaciones, recordatorios, dispositivos push y
// preferencias de notificaciones. Todas viven en `USER#<id>` y se leen con `Query` por
// prefijo de SK o `GetItem` exacto. Ninguna usa `Scan`.
// ---------------------------------------------------------------------------------------

// exportPaginaParticion recorre una colección de la partición del titular con `begins_with`
// sobre la SK, orden ascendente y consistencia fuerte. Pide una fila de más para distinguir
// «página llena con más datos» de «colección agotada» sin `Scan`.
func (s *ExportStore) exportPaginaParticion(ctx context.Context, userID, prefijo, after string, limit int) ([]map[string]types.AttributeValue, string, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" || limit < 1 || limit > appExport.MaxLimit || prefijo == "" {
		return nil, "", appExport.ErrUnavailable
	}
	pk := domainDynamo.PrefixUser + userID
	entrada := &dynamodb.QueryInput{
		TableName:              aws.String(s.tabla),
		ConsistentRead:         aws.Bool(true),
		KeyConditionExpression: aws.String("PK = :pk AND begins_with(SK, :prefijo)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk":      stringAttr(pk),
			":prefijo": stringAttr(prefijo),
		},
		ScanIndexForward: aws.Bool(true),
		Limit:            aws.Int32(int32(limit + 1)),
	}
	if after != "" {
		if !strings.HasPrefix(after, prefijo) {
			return nil, "", appExport.ErrUnavailable
		}
		entrada.ExclusiveStartKey = map[string]types.AttributeValue{
			"PK": stringAttr(pk),
			"SK": stringAttr(after),
		}
	}
	salida, err := s.cliente.Query(ctx, entrada)
	if err != nil {
		return nil, "", appExport.ErrUnavailable
	}
	if salida == nil {
		return nil, "", nil
	}
	if len(salida.Items) > limit {
		// La fila extra confirma que queda al menos un elemento más.
		return salida.Items[:limit], stringAttribute(salida.Items[limit-1]["SK"]), nil
	}
	if len(salida.LastEvaluatedKey) > 0 {
		if len(salida.Items) == 0 {
			// Sin progreso no hay clave de continuación válida.
			return nil, "", appExport.ErrUnavailable
		}
		return salida.Items, stringAttribute(salida.Items[len(salida.Items)-1]["SK"]), nil
	}
	return salida.Items, "", nil
}

// Notificaciones devuelve la bandeja E061 del titular en orden ascendente por SK.
func (s *ExportStore) Notificaciones(ctx context.Context, userID, after string, limit int) ([]appExport.NotificationRow, string, error) {
	if s == nil || s.cliente == nil || s.tabla == "" {
		return nil, "", appExport.ErrUnavailable
	}
	items, siguiente, err := s.exportPaginaParticion(ctx, userID, appExport.PrefijoNotificacion, after, limit)
	if err != nil {
		return nil, "", err
	}
	filas := make([]appExport.NotificationRow, 0, len(items))
	for _, item := range items {
		fila, errFila := exportNotificationRow(item, strings.TrimSpace(userID))
		if errFila != nil {
			return nil, "", errFila
		}
		filas = append(filas, fila)
	}
	return filas, siguiente, nil
}

// Recordatorios devuelve E064–E067 en orden ascendente por SK (vencimiento).
func (s *ExportStore) Recordatorios(ctx context.Context, userID, after string, limit int) ([]appExport.ReminderRow, string, error) {
	if s == nil || s.cliente == nil || s.tabla == "" {
		return nil, "", appExport.ErrUnavailable
	}
	items, siguiente, err := s.exportPaginaParticion(ctx, userID, appExport.PrefijoRecordatorio, after, limit)
	if err != nil {
		return nil, "", err
	}
	filas := make([]appExport.ReminderRow, 0, len(items))
	for _, item := range items {
		fila, errFila := exportReminderRow(item, strings.TrimSpace(userID))
		if errFila != nil {
			return nil, "", errFila
		}
		filas = append(filas, fila)
	}
	return filas, siguiente, nil
}

// Dispositivos devuelve los dispositivos push del titular. La proyección descarta cualquier
// atributo de token: no se leen `token`, `tokenCipher` ni `tokenFingerprint`.
func (s *ExportStore) Dispositivos(ctx context.Context, userID, after string, limit int) ([]appExport.PushDeviceRow, string, error) {
	if s == nil || s.cliente == nil || s.tabla == "" {
		return nil, "", appExport.ErrUnavailable
	}
	items, siguiente, err := s.exportPaginaParticion(ctx, userID, appExport.PrefijoDispositivo, after, limit)
	if err != nil {
		return nil, "", err
	}
	filas := make([]appExport.PushDeviceRow, 0, len(items))
	for _, item := range items {
		fila, errFila := exportPushDeviceRow(item, strings.TrimSpace(userID))
		if errFila != nil {
			return nil, "", errFila
		}
		filas = append(filas, fila)
	}
	return filas, siguiente, nil
}

// PreferenciasNotificacion lee el singleton `USER#<id>/NOTIFICATION_PREFERENCES` con
// consistencia fuerte. `(nil, nil)` si no existe o si la clave la ocupa otra entidad: el
// contrato la declara anulable y la exportación no materializa defaults.
func (s *ExportStore) PreferenciasNotificacion(ctx context.Context, userID string) (*appExport.NotificationPreferencesRow, error) {
	if s == nil || s.cliente == nil || s.tabla == "" {
		return nil, appExport.ErrUnavailable
	}
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, appExport.ErrUnauthorized
	}
	salida, err := s.cliente.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(s.tabla),
		Key: map[string]types.AttributeValue{
			"PK": stringAttr(domainDynamo.PrefixUser + userID),
			"SK": stringAttr(appExport.SKPreferenciasNotificacion),
		},
		ConsistentRead: aws.Bool(true),
	})
	if err != nil {
		return nil, appExport.ErrUnavailable
	}
	if salida == nil || len(salida.Item) == 0 {
		return nil, nil
	}
	entidad := stringAttribute(salida.Item["entityType"])
	if entidad != "" && entidad != "USER_NOTIFICATION_PREFERENCES" {
		return nil, nil
	}
	fila := &appExport.NotificationPreferencesRow{
		PushEnabled:        boolAttribute(salida.Item["pushEnabled"]),
		EmailEnabled:       boolAttribute(salida.Item["emailEnabled"]),
		ReminderEnabled:    boolAttribute(salida.Item["reminderEnabled"]),
		BudgetAlerts:       boolAttribute(salida.Item["budgetAlerts"]),
		GoalUpdates:        boolAttribute(salida.Item["goalUpdates"]),
		RecurringReminders: boolAttribute(salida.Item["recurringReminders"]),
		StreakReminders:    boolAttribute(salida.Item["streakReminders"]),
		TimeZone:           strings.TrimSpace(stringAttribute(salida.Item["timeZone"])),
		Version:            1,
	}
	if version, ok := attributeInt(salida.Item["version"]); ok && version >= 1 {
		fila.Version = version
	}
	if habilitado, ok := notificationBoolOptional(salida.Item["quietHoursEnabled"]); ok {
		fila.QuietHours = &appExport.QuietHoursRow{
			Enabled: habilitado,
			Start:   stringAttribute(salida.Item["quietHoursStart"]),
			End:     stringAttribute(salida.Item["quietHoursEnd"]),
		}
	}
	return fila, nil
}

func exportNotificationRow(item map[string]types.AttributeValue, userID string) (appExport.NotificationRow, error) {
	if entidad := stringAttribute(item["entityType"]); entidad != "NOTIFICATION" {
		return appExport.NotificationRow{}, appExport.ErrInconsistent
	}
	if titular := stringAttribute(item["userId"]); titular != "" && titular != userID {
		return appExport.NotificationRow{}, appExport.ErrInconsistent
	}
	id := strings.TrimSpace(stringAttribute(item["id"]))
	if id == "" {
		sk := strings.TrimPrefix(stringAttribute(item["SK"]), appExport.PrefijoNotificacion)
		if i := strings.LastIndex(sk, "#"); i >= 0 {
			id = sk[i+1:]
		}
	}
	creado, ok := exportInstante(stringAttribute(item["createdAt"]))
	if id == "" || !ok {
		return appExport.NotificationRow{}, appExport.ErrInconsistent
	}
	return appExport.NotificationRow{
		ID:        id,
		Title:     stringAttribute(item["title"]),
		Message:   stringAttribute(item["message"]),
		IsRead:    boolAttribute(item["isRead"]),
		CreatedAt: creado,
	}, nil
}

func exportReminderRow(item map[string]types.AttributeValue, userID string) (appExport.ReminderRow, error) {
	if entidad := stringAttribute(item["entityType"]); entidad != "USER_REMINDER" {
		return appExport.ReminderRow{}, appExport.ErrInconsistent
	}
	if titular := stringAttribute(item["userId"]); titular != "" && titular != userID {
		return appExport.ReminderRow{}, appExport.ErrInconsistent
	}
	id := strings.TrimSpace(stringAttribute(item["reminderID"]))
	dueAt, okDue := exportInstante(stringAttribute(item["dueAt"]))
	creado, okCreado := exportInstante(stringAttribute(item["createdAt"]))
	actualizado, okActualizado := exportInstante(stringAttribute(item["updatedAt"]))
	if id == "" || !okDue || !okCreado || !okActualizado {
		return appExport.ReminderRow{}, appExport.ErrInconsistent
	}
	fila := appExport.ReminderRow{
		ID:        id,
		Title:     stringAttribute(item["title"]),
		DueAt:     dueAt,
		Timezone:  strings.TrimSpace(stringAttribute(item["timezone"])),
		Active:    boolAttribute(item["active"]),
		CreatedAt: creado,
		UpdatedAt: actualizado,
	}
	// Frecuencia: ausente o vacía significa «sin recurrencia»; un valor fuera del enum
	// es un item incoherente y se rechaza aquí, antes de proyectarlo al contrato.
	frecuencia, valida := appExport.NormalizeReminderFrequency(stringAttribute(item["frequency"]))
	if !valida {
		return appExport.ReminderRow{}, appExport.ErrInconsistent
	}
	fila.Frequency = frecuencia
	return fila, nil
}

// exportPushDeviceRow proyecta SÓLO los cinco campos publicables. Los atributos de token se
// ignoran por completo: no se copian ni se validan.
func exportPushDeviceRow(item map[string]types.AttributeValue, userID string) (appExport.PushDeviceRow, error) {
	if entidad := stringAttribute(item["entityType"]); entidad != pushDeviceEntityType {
		return appExport.PushDeviceRow{}, appExport.ErrInconsistent
	}
	if titular := stringAttribute(item["userId"]); titular != "" && titular != userID {
		return appExport.PushDeviceRow{}, appExport.ErrInconsistent
	}
	id := strings.TrimPrefix(stringAttribute(item["SK"]), appExport.PrefijoDispositivo)
	registrado, ok := exportInstante(stringAttribute(item["registeredAt"]))
	if strings.TrimSpace(id) == "" || !ok {
		return appExport.PushDeviceRow{}, appExport.ErrInconsistent
	}
	return appExport.PushDeviceRow{
		ID:           strings.TrimSpace(id),
		Provider:     stringAttribute(item["provider"]),
		DeviceLabel:  stringAttribute(item["deviceLabel"]),
		RegisteredAt: registrado,
		Active:       boolAttribute(item["active"]),
	}, nil
}

// exportInstante interpreta las dos formas de fecha que escriben los adaptadores
// (`RFC3339Nano` de los workers y `RFC3339` de los endpoints HTTP).
func exportInstante(valor string) (time.Time, bool) {
	crudo := strings.TrimSpace(valor)
	if crudo == "" {
		return time.Time{}, false
	}
	if marca, err := time.Parse(time.RFC3339Nano, crudo); err == nil {
		return marca.UTC(), true
	}
	if marca, err := time.Parse(time.RFC3339, crudo); err == nil {
		return marca.UTC(), true
	}
	return time.Time{}, false
}
