package dynamo

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"

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

// ExportDynamoAPI es el subconjunto del cliente DynamoDB que usa la exportación para las dos
// familias sin adaptador publicado. No incluye `Scan`.
type ExportDynamoAPI interface {
	Query(ctx context.Context, params *dynamodb.QueryInput, optFns ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error)
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
		TableName:      aws.String(s.tabla),
		ConsistentRead: aws.Bool(true),
		KeyConditionExpression: aws.String("PK = :pk AND begins_with(SK, :prefijo)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk":     stringAttr(pk),
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
// exacta y la siguiente llamada reanuda desde el periodo correspondiente con `SK >=`.
func (s *ExportStore) Presupuestos(ctx context.Context, userID, after string, limit int) ([]appExport.BudgetRow, string, error) {
	if s == nil || s.cliente == nil || s.tabla == "" {
		return nil, "", appExport.ErrUnavailable
	}
	userID = strings.TrimSpace(userID)
	if userID == "" || limit < 1 || limit > appExport.MaxLimit {
		return nil, "", appExport.ErrUnavailable
	}

	pk := domainDynamo.PrefixUser + userID
	condicion := "PK = :pk AND begins_with(SK, :prefijo)"
	valores := map[string]types.AttributeValue{
		":pk":      stringAttr(pk),
		":prefijo": stringAttr(appExport.PrefijoPresupuesto),
	}
	if after != "" {
		periodo := periodoDeClave(after)
		if periodo == "" {
			return nil, "", appExport.ErrUnavailable
		}
		condicion = "PK = :pk AND SK >= :desde"
		valores = map[string]types.AttributeValue{
			":pk":    stringAttr(pk),
			":desde": stringAttr(periodo),
		}
	}

	filas := make([]appExport.BudgetRow, 0, limit+1)
	var inicio map[string]types.AttributeValue
	for pagina := 0; pagina < exportPresupuestosMaxPaginas; pagina++ {
		entrada := &dynamodb.QueryInput{
			TableName:                 aws.String(s.tabla),
			ConsistentRead:            aws.Bool(true),
			KeyConditionExpression:    aws.String(condicion),
			ExpressionAttributeValues: valores,
			ScanIndexForward:          aws.Bool(true),
			Limit:                     aws.Int32(int32(limit + 1)),
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
			return filas, "", nil
		}
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

type allocationJSON struct {
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
		var leidas []allocationJSON
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

type budgetJSON struct {
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
	var crudos []budgetJSON
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
