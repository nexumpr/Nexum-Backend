// Package export implementa E020 (`GET /api/v1/users/me/export`).
//
// Contrato vigente (OpenAPI 3.1, schema `UserExport`) con la enmienda de RFC-022 §2.1:
//
//	profile · exportedAt · movements · contributions · recurringPayments · goals · budgets ·
//	consents · income · onboarding · preferences · nextCursor
//
// Las seis familias añadidas por RFC-022 (`income`, `recurringPayments`, `contributions`,
// `budgets`, `onboarding`, `preferences`) son propiedades OPCIONALES del schema: `required` no
// cambia. Este backend las emite siempre; las tres que el contrato declara anulables
// (`income`, `onboarding`, `preferences`) se serializan `null` cuando el titular no tiene el
// item, sin inventar valores ni materializar items durante una lectura.
//
// Invariantes (RFC-022 §2.2–§2.4):
//   - Sin topes artificiales por familia ni totales: la exportación recorre las colecciones
//     hasta agotarlas. `limit` (1..100, por defecto 25) acota filas DE LA PÁGINA, no del
//     documento completo.
//   - Orden fijo: `movements` → `contributions` → `recurringPayments` → `goals` → `budgets`.
//     Dentro de cada colección, ascendente por `SK`. `profile`, `income`, `consents`,
//     `onboarding`, `preferences` y `exportedAt` se incluyen completos en cada página.
//   - `nextCursor` se omite SOLO cuando todas las colecciones están agotadas. Un fallo
//     recuperable devuelve error (503) sin página y, por tanto, sin cursor: nunca se declara
//     terminada una exportación incompleta.
//   - El cursor es opaco y firmado con HMAC; liga el usuario, el dominio `E020_EXPORT`, la
//     colección de continuación, la `SK` de continuación y `exportedAt`. Manipulación, uso
//     por otro usuario, colección incoherente o antigüedad > 2 h producen 400.
//   - `ConsistentRead` en cada página evita réplicas retrasadas, pero NO convierte varias
//     páginas en una transacción ni en un snapshot: una exportación paginada sobre una tabla
//     viva es un recorrido ordenado y best-effort. Un elemento cuyo `SK` cambia puede
//     omitirse o leerse en su nueva posición; no se promete ausencia absoluta de duplicados
//     ni de omisiones bajo mutaciones concurrentes.
//   - No se usa `Scan`: todo se resuelve por `Query` en la partición del titular o `GetItem`.
//   - No se exportan hashes, contraseñas, tokens, `REFRESH#`, `IDEM#`, `EMAIL#`, `QUEUE#`,
//     `RESET#`, `ONE_USE#`, `DELETION#`, secretos ni datos de otros titulares.
package export

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	appGoals "github.com/nexumpr/Nexum-Backend/internal/application/goals"
	appIdentity "github.com/nexumpr/Nexum-Backend/internal/application/identity"
	appMovements "github.com/nexumpr/Nexum-Backend/internal/application/movements"
	appOnboarding "github.com/nexumpr/Nexum-Backend/internal/application/onboarding"
	appPreferences "github.com/nexumpr/Nexum-Backend/internal/application/preferences"
	appPrivacy "github.com/nexumpr/Nexum-Backend/internal/application/privacy"
	appProfile "github.com/nexumpr/Nexum-Backend/internal/application/profile"
	appRecurring "github.com/nexumpr/Nexum-Backend/internal/application/recurring"
	domainDynamo "github.com/nexumpr/Nexum-Backend/internal/domain/dynamo"
	"github.com/nexumpr/Nexum-Backend/pkg/httputil"
)

// Errores de aplicación. El transporte los traduce a los códigos del contrato; la
// persistencia traduce sus fallos a estos mismos valores para que el handler no conozca AWS.
var (
	// ErrInvalidRequest agrupa las solicitudes mal formadas: identidad ausente, `limit` fuera
	// de rango o cursor manipulado, vencido, de otro usuario o de otra colección (HTTP 400).
	ErrInvalidRequest = errors.New("solicitud de exportacion invalida")

	// ErrUnauthorized indica ausencia de identidad verificada. El middleware ya lo impide;
	// esta guarda es defensa en profundidad (HTTP 401).
	ErrUnauthorized = errors.New("exportacion no autorizada")

	// ErrNotFound indica que el perfil del titular no existe y la exportación no puede
	// cumplir el campo obligatorio `profile` (HTTP 404).
	ErrNotFound = errors.New("perfil del titular no encontrado")

	// ErrUnavailable indica falta de persistencia, falta de clave de firma de cursor o un
	// fallo de lectura. Nunca se sustituye por datos en memoria ni por una exportación
	// parcial presentada como completa (HTTP 503).
	ErrUnavailable = errors.New("servicio de exportacion no disponible")

	// ErrInconsistent indica un item persistido cuya forma no corresponde al contrato o una
	// lectura que devolvió más filas de las solicitadas. Se falla cerrado en lugar de emitir
	// un DTO inventado (HTTP 500).
	ErrInconsistent = errors.New("exportacion incoherente")
)

const (
	// DefaultLimit y MaxLimit son los del parámetro `limit` del contrato (1..100, defecto 25).
	DefaultLimit = httputil.DefaultPageLimit
	MaxLimit     = httputil.MaxPageLimit

	// CursorMinLength y CursorMaxLength replican `minLength: 8` y `maxLength: 2048` del
	// parámetro `cursor`.
	CursorMinLength = 8
	CursorMaxLength = 2048

	// MonedaMXN es el único valor del enum `currency` publicado por el contrato.
	MonedaMXN = "MXN"

	// PrefijoAportacion y PrefijoPresupuesto son los prefijos físicos de las familias que no
	// tienen constante en `domain/dynamo` (su diseño vive en RFC-022 §5 y §3). Se exportan
	// para que el adaptador y el cursor usen exactamente el mismo literal.
	PrefijoAportacion  = "CONTRIBUTION#"
	PrefijoPresupuesto = "BUDGETSET#"

	// EstadoAportacionActiva y EstadoAportacionRevertida son el enum `status` de `Contribution`.
	EstadoAportacionActiva    = "active"
	EstadoAportacionRevertida = "reversed"

	// sortDomain identifica el dominio de firma del cursor (`E020_EXPORT`) y su versión. Un
	// cursor de otro endpoint, o de una versión anterior del orden, no es reutilizable.
	sortDomain = "e020:v1"

	// Nombres estables de colección que viajan en el cursor. Se codifica el NOMBRE y no un
	// índice para que añadir una colección intermedia no invalide los cursores ya emitidos.
	coleccionMovimientos  = "movements"
	coleccionAportaciones = "contributions"
	coleccionRecurrentes  = "recurringPayments"
	coleccionMetas        = "goals"
	coleccionPresupuestos = "budgets"
)

// periodPattern replica `^\d{4}-(0[1-9]|1[0-2])$` de `Budget.period`.
var periodPattern = regexp.MustCompile(`^[0-9]{4}-(0[1-9]|1[0-2])$`)

// colecciones es el orden fijo de la exportación (RFC-022 §2.2). `profile`, `income`,
// `consents`, `onboarding` y `preferences` no aparecen aquí porque son singletons completos en
// cada página.
var colecciones = []string{
	coleccionMovimientos,
	coleccionAportaciones,
	coleccionRecurrentes,
	coleccionMetas,
	coleccionPresupuestos,
}

// prefijoDe devuelve el prefijo de `SK` que identifica cada colección en la tabla única.
func prefijoDe(coleccion string) string {
	switch coleccion {
	case coleccionMovimientos:
		return domainDynamo.PrefixMovement
	case coleccionAportaciones:
		return PrefijoAportacion
	case coleccionRecurrentes:
		return domainDynamo.PrefixRecurring
	case coleccionMetas:
		return domainDynamo.PrefixGoal
	case coleccionPresupuestos:
		return PrefijoPresupuesto
	default:
		return ""
	}
}

// indiceDe devuelve la posición de la colección en el orden fijo, o -1 si no existe.
func indiceDe(coleccion string) int {
	for i, nombre := range colecciones {
		if nombre == coleccion {
			return i
		}
	}
	return -1
}

// Query es la entrada de aplicación de E020. `UserID` proviene de `sub`; nunca del cliente.
type Query struct {
	UserID string
	Limit  int
	Cursor string
}

// ProfileContract es la proyección EXACTA del esquema `UserProfile` usada dentro de la
// exportación: seis campos, sin atributos internos del item (`passwordHash`, `sessionEpoch`,
// `privacy*`, etc.).
type ProfileContract struct {
	ID            string `json:"id"`
	Email         string `json:"email"`
	DisplayName   string `json:"displayName"`
	EmailVerified bool   `json:"emailVerified"`
	CreatedAt     string `json:"createdAt"`
	Version       int64  `json:"version"`
}

// ConsentContract es la proyección EXACTA del esquema `Consent` usada dentro de la
// exportación: cinco campos, sin `SK` ni metadatos internos.
type ConsentContract struct {
	ID            string `json:"id"`
	NoticeVersion string `json:"noticeVersion"`
	Purpose       string `json:"purpose"`
	Granted       bool   `json:"granted"`
	RecordedAt    string `json:"recordedAt"`
}

// IncomeContract es la proyección EXACTA del esquema `Income`: cuatro campos. El importe se
// serializa como cadena decimal exacta y la frecuencia ya llega canónica.
type IncomeContract struct {
	Amount    string `json:"amount"`
	Currency  string `json:"currency"`
	Frequency string `json:"frequency"`
	Version   int64  `json:"version"`
}

// AllocationContract es la proyección EXACTA del esquema `GoalAllocation` dentro de
// `Contribution`.
type AllocationContract struct {
	GoalID string `json:"goalId"`
	Amount string `json:"amount"`
}

// ContributionContract es la proyección EXACTA del esquema `Contribution`: siete campos.
type ContributionContract struct {
	ID          string               `json:"id"`
	Amount      string               `json:"amount"`
	Currency    string               `json:"currency"`
	Allocations []AllocationContract `json:"allocations"`
	OccurredAt  string               `json:"occurredAt"`
	Status      string               `json:"status"`
	Version     int64                `json:"version"`
}

// BudgetContract es la proyección EXACTA del esquema `Budget`: cinco campos.
type BudgetContract struct {
	CategoryID  string `json:"categoryId"`
	Period      string `json:"period"`
	LimitAmount string `json:"limitAmount"`
	Currency    string `json:"currency"`
	Version     int64  `json:"version"`
}

// OnboardingContract es la proyección EXACTA del esquema `Onboarding`. `completedSteps` nunca
// es `null`; `completedAt` sólo se emite cuando el onboarding está completo.
type OnboardingContract struct {
	CompletedSteps []string `json:"completedSteps"`
	Done           bool     `json:"done"`
	Version        int64    `json:"version"`
	Microphone     bool     `json:"microphone"`
	Notifications  bool     `json:"notifications"`
	Configured     bool     `json:"configured"`
	CompletedAt    *string  `json:"completedAt"`
}

// PreferencesContract es la proyección EXACTA del esquema `UserPreferences`. `timeZone` es el
// alias exacto de `timezone` que ya publica E068.
type PreferencesContract struct {
	Locale   string `json:"locale"`
	Timezone string `json:"timezone"`
	Version  int64  `json:"version"`
	TimeZone string `json:"timeZone"`
}

// ContributionRow es una aportación leída de la partición del titular, antes de la proyección
// al contrato. El adaptador no formatea importes: eso decide esta capa.
type ContributionRow struct {
	ID          string
	AmountCents int64
	Currency    string
	Allocations []AllocationRow
	OccurredAt  time.Time
	Status      string
	Version     int64
}

// AllocationRow es una asignación de una aportación a una meta.
type AllocationRow struct {
	GoalID      string
	AmountCents int64
}

// BudgetRow es una fila de `Budget` procedente de un `BUDGETSET#<period>`.
type BudgetRow struct {
	CategoryID       string
	Period           string
	LimitAmountCents int64
	Currency         string
	Version          int64
}

// Page es `UserExport`. Las cinco colecciones son la página solicitada; `profile`, `income`,
// `consents`, `onboarding` y `preferences` son completos en cada página; `nextCursor` sólo
// aparece si queda alguna colección por agotar.
type Page struct {
	Profile           ProfileContract                 `json:"profile"`
	ExportedAt        string                          `json:"exportedAt"`
	Movements         []appMovements.MovementResponse `json:"movements"`
	Contributions     []ContributionContract          `json:"contributions"`
	RecurringPayments []appRecurring.WirePayment      `json:"recurringPayments"`
	Goals             []appGoals.GoalContract         `json:"goals"`
	Budgets           []BudgetContract                `json:"budgets"`
	Consents          []ConsentContract               `json:"consents"`
	Income            *IncomeContract                 `json:"income"`
	Onboarding        *OnboardingContract             `json:"onboarding"`
	Preferences       *PreferencesContract            `json:"preferences"`
	NextCursor        string                          `json:"nextCursor,omitempty"`
}

// Fuente es el puerto de lectura de la exportación.
//
// Contrato del adaptador:
//   - Las cinco colecciones devuelven hasta `limit` filas en orden ascendente por `SK` desde
//     `after` (exclusivo) y la `SK` desde la que continuar (vacía si la colección está
//     agotada). Nunca usan `Scan` ni consultan particiones ajenas. Un fallo se traduce a
//     ErrUnavailable y NUNCA a una colección vacía.
//   - `Presupuestos` usa una clave sintética `BUDGETSET#<period>#<categoryId>`: cada
//     `BUDGETSET#<period>` contiene hasta 50 filas y la página se corta a mitad de un periodo
//     cuando `limit` lo exige.
//   - Cada lectura es FUERTEMENTE CONSISTENTE dentro de su página.
//   - `Perfil` devuelve `(nil, nil)` si el perfil no existe (el servicio responde 404).
//   - `Ingreso`, `Onboarding` y `Preferencias` devuelven `(nil, nil)` cuando el titular no
//     tiene el item: el contrato los declara anulables y la exportación no materializa items.
//   - `Consentimientos` devuelve la colección COMPLETA del titular (es un singleton que viaja
//     entero en cada página). Un fallo se propaga como error; nunca se devuelve una lista
//     truncada como si fuera completa.
type Fuente interface {
	Movimientos(ctx context.Context, userID, after string, limit int) ([]appMovements.Movement, string, error)
	Aportaciones(ctx context.Context, userID, after string, limit int) ([]ContributionRow, string, error)
	Recurrentes(ctx context.Context, userID, after string, limit int) ([]appRecurring.Payment, string, error)
	Metas(ctx context.Context, userID, after string, limit int) ([]appGoals.GoalRow, string, error)
	Presupuestos(ctx context.Context, userID, after string, limit int) ([]BudgetRow, string, error)
	Perfil(ctx context.Context, userID string) (*appProfile.Profile, error)
	Ingreso(ctx context.Context, userID string) (*appIdentity.IncomeSnapshot, error)
	Consentimientos(ctx context.Context, userID string) ([]appPrivacy.ConsentRecord, error)
	Onboarding(ctx context.Context, userID string) (*appOnboarding.State, error)
	Preferencias(ctx context.Context, userID string) (*appPreferences.Preferences, error)
}

// Service orquesta la exportación paginada.
type Service struct {
	fuente       Fuente
	cursorSecret []byte
	cursorTTL    time.Duration
	now          func() time.Time
}

// NewService construye el servicio. Sin fuente todas las llamadas fallan cerrado con
// ErrUnavailable: no existe ningún camino con datos en memoria en producción.
func NewService(fuente Fuente) *Service {
	return &Service{
		fuente:    fuente,
		cursorTTL: httputil.DefaultCursorTTL,
		now:       func() time.Time { return time.Now().UTC() },
	}
}

// WithCursorSecret configura la clave HMAC con la que se firman los cursores. Sin ella el
// servicio falla cerrado (503): nunca se emite un cursor sin firma. La clave se deriva con
// separación de dominio de un secreto ya existente (A-040 §2.3), de modo que no se introduce
// un secreto nuevo en el bundle.
func (s *Service) WithCursorSecret(secret []byte) *Service {
	if len(secret) > 0 {
		copia := make([]byte, len(secret))
		copy(copia, secret)
		s.cursorSecret = copia
	}
	return s
}

// WithCursorTTL permite acortar o invertir la vigencia del cursor en pruebas.
func (s *Service) WithCursorTTL(ttl time.Duration) *Service {
	if ttl != 0 {
		s.cursorTTL = ttl
	}
	return s
}

// WithClock permite inyectar un reloj determinista en pruebas.
func (s *Service) WithClock(now func() time.Time) *Service {
	if now != nil {
		s.now = now
	}
	return s
}

// Export devuelve una página de la exportación del titular.
//
// Secuencia: validar identidad y límite → verificar el cursor (firma, expiración, pertenencia,
// dominio, colección y coherencia de `SK`) → leer el perfil, el ingreso, los consentimientos,
// el onboarding y las preferencias completos → recorrer las colecciones en el orden fijo hasta
// completar la página o agotarlas → emitir el cursor siguiente sólo si queda algo por leer.
func (s *Service) Export(ctx context.Context, query Query) (*Page, error) {
	if s == nil || s.fuente == nil {
		return nil, ErrUnavailable
	}
	userID := strings.TrimSpace(query.UserID)
	if userID == "" {
		return nil, ErrUnauthorized
	}
	if len(s.cursorSecret) == 0 {
		// Sin clave de firma no se puede emitir ni verificar un cursor: se falla cerrado en
		// lugar de paginar de forma insegura.
		return nil, ErrUnavailable
	}

	limit, errLimite := NormalizeLimit(query.Limit)
	if errLimite != nil {
		return nil, errLimite
	}

	exportedAt := s.now().UTC().Truncate(time.Second)
	coleccion := colecciones[0]
	after := ""

	if raw := strings.TrimSpace(query.Cursor); raw != "" {
		if len(raw) < CursorMinLength || len(raw) > CursorMaxLength {
			return nil, fmt.Errorf("%w: cursor fuera de rango", ErrInvalidRequest)
		}
		payload, errCursor := httputil.ValidateCursor(s.cursorSecret, raw, userID)
		if errCursor != nil {
			// Manipulado, corrupto, vencido o de otro usuario: un único 400 sin distinguir.
			return nil, fmt.Errorf("%w: cursor no valido", ErrInvalidRequest)
		}
		coleccionCursor, instante, errSort := parseSortKey(payload.SortBy)
		if errSort != nil {
			return nil, fmt.Errorf("%w: cursor no valido", ErrInvalidRequest)
		}
		// Una `SK` de continuación vacía significa «empezar la colección por el principio»;
		// cualquier otra debe pertenecer a la colección declarada. Así un cursor no puede
		// reutilizarse para saltar a otra familia ni para inyectar una clave arbitraria.
		if payload.LastID != "" && !strings.HasPrefix(payload.LastID, prefijoDe(coleccionCursor)) {
			return nil, fmt.Errorf("%w: cursor no apunta a la coleccion declarada", ErrInvalidRequest)
		}
		coleccion, after, exportedAt = coleccionCursor, payload.LastID, instante
	}

	perfil, errPerfil := s.fuente.Perfil(ctx, userID)
	if errPerfil != nil {
		return nil, errPerfil
	}
	if perfil == nil {
		return nil, ErrNotFound
	}
	consentimientos, errConsents := s.fuente.Consentimientos(ctx, userID)
	if errConsents != nil {
		return nil, errConsents
	}
	ingreso, errIngreso := s.fuente.Ingreso(ctx, userID)
	if errIngreso != nil {
		return nil, errIngreso
	}
	onboarding, errOnboarding := s.fuente.Onboarding(ctx, userID)
	if errOnboarding != nil {
		return nil, errOnboarding
	}
	preferencias, errPreferencias := s.fuente.Preferencias(ctx, userID)
	if errPreferencias != nil {
		return nil, errPreferencias
	}

	page := &Page{
		Profile:           profileContract(perfil),
		ExportedAt:        exportedAt.Format(time.RFC3339),
		Movements:         make([]appMovements.MovementResponse, 0, limit),
		Contributions:     make([]ContributionContract, 0, limit),
		RecurringPayments: make([]appRecurring.WirePayment, 0, limit),
		Goals:             make([]appGoals.GoalContract, 0, limit),
		Budgets:           make([]BudgetContract, 0, limit),
		Consents:          consentContracts(consentimientos),
		Income:            incomeContract(ingreso),
		Onboarding:        onboardingContract(onboarding),
		Preferences:       preferencesContract(preferencias),
	}

	// Recorrido en el orden fijo. `restantes` es lo que falta para llenar LA PÁGINA; al
	// llegar a cero la continuación apunta a la colección y posición actuales.
	restantes := limit
	continuacion := -1
	continuacionSK := ""
	for actual := indiceDe(coleccion); actual < len(colecciones); actual++ {
		if restantes == 0 {
			continuacion, continuacionSK = actual, after
			break
		}
		switch colecciones[actual] {
		case coleccionMovimientos:
			filas, siguiente, err := s.fuente.Movimientos(ctx, userID, after, restantes)
			if err != nil {
				return nil, err
			}
			if len(filas) > restantes {
				return nil, ErrInconsistent
			}
			for i := range filas {
				page.Movements = append(page.Movements, appMovements.NewMovementResponse(filas[i]))
			}
			restantes -= len(filas)
			if siguiente != "" {
				continuacion, continuacionSK = actual, siguiente
			}
		case coleccionAportaciones:
			filas, siguiente, err := s.fuente.Aportaciones(ctx, userID, after, restantes)
			if err != nil {
				return nil, err
			}
			if len(filas) > restantes {
				return nil, ErrInconsistent
			}
			for i := range filas {
				contrato, errContrato := contributionContract(filas[i])
				if errContrato != nil {
					// Un item incoherente no se emite ni se omite en silencio.
					return nil, ErrInconsistent
				}
				page.Contributions = append(page.Contributions, contrato)
			}
			restantes -= len(filas)
			if siguiente != "" {
				continuacion, continuacionSK = actual, siguiente
			}
		case coleccionRecurrentes:
			filas, siguiente, err := s.fuente.Recurrentes(ctx, userID, after, restantes)
			if err != nil {
				return nil, err
			}
			if len(filas) > restantes {
				return nil, ErrInconsistent
			}
			for i := range filas {
				if strings.TrimSpace(filas[i].ID) == "" {
					return nil, ErrInconsistent
				}
				page.RecurringPayments = append(page.RecurringPayments, filas[i].Wire())
			}
			restantes -= len(filas)
			if siguiente != "" {
				continuacion, continuacionSK = actual, siguiente
			}
		case coleccionMetas:
			filas, siguiente, err := s.fuente.Metas(ctx, userID, after, restantes)
			if err != nil {
				return nil, err
			}
			if len(filas) > restantes {
				return nil, ErrInconsistent
			}
			for i := range filas {
				contrato, errContrato := appGoals.ContractOf(&filas[i].Goal)
				if errContrato != nil {
					// Un item incoherente no se emite ni se omite en silencio.
					return nil, ErrInconsistent
				}
				page.Goals = append(page.Goals, contrato)
			}
			restantes -= len(filas)
			if siguiente != "" {
				continuacion, continuacionSK = actual, siguiente
			}
		case coleccionPresupuestos:
			filas, siguiente, err := s.fuente.Presupuestos(ctx, userID, after, restantes)
			if err != nil {
				return nil, err
			}
			if len(filas) > restantes {
				return nil, ErrInconsistent
			}
			for i := range filas {
				contrato, errContrato := budgetContract(filas[i])
				if errContrato != nil {
					return nil, ErrInconsistent
				}
				page.Budgets = append(page.Budgets, contrato)
			}
			restantes -= len(filas)
			if siguiente != "" {
				continuacion, continuacionSK = actual, siguiente
			}
		}
		if continuacion >= 0 {
			break
		}
		// Sólo la colección de arranque puede tener una posición inicial distinta de cero.
		after = ""
	}

	if continuacion >= 0 {
		cursor, errGen := httputil.GenerateCursor(
			s.cursorSecret,
			userID,
			formatSortKey(colecciones[continuacion], exportedAt),
			continuacionSK,
			exportedAt,
			s.cursorTTL,
		)
		if errGen != nil {
			return nil, ErrUnavailable
		}
		page.NextCursor = cursor
	}
	return page, nil
}

// NormalizeLimit valida `limit` del contrato (1..100, por defecto 25). Un valor cero se
// interpreta como ausencia del parámetro; cualquier otro fuera de rango es 400, nunca se
// recorta en silencio.
func NormalizeLimit(limit int) (int, error) {
	if limit == 0 {
		return DefaultLimit, nil
	}
	if limit < 1 || limit > MaxLimit {
		return 0, fmt.Errorf("%w: limit fuera de rango", ErrInvalidRequest)
	}
	return limit, nil
}

// formatSortKey serializa la colección y `exportedAt` en el campo `SortBy` del cursor.
func formatSortKey(coleccion string, exportedAt time.Time) string {
	return sortDomain + "|" + coleccion + "|" + strconv.FormatInt(exportedAt.Unix(), 10)
}

// parseSortKey interpreta `SortBy` y rechaza cualquier valor que no corresponda al dominio,
// a una colección conocida o a un instante válido.
func parseSortKey(sortBy string) (string, time.Time, error) {
	partes := strings.Split(sortBy, "|")
	if len(partes) != 3 || partes[0] != sortDomain {
		return "", time.Time{}, errors.New("cursor de otro dominio")
	}
	coleccion := partes[1]
	if prefijoDe(coleccion) == "" {
		return "", time.Time{}, errors.New("coleccion desconocida")
	}
	unix, err := strconv.ParseInt(partes[2], 10, 64)
	if err != nil || unix <= 0 {
		return "", time.Time{}, errors.New("instante de exportacion invalido")
	}
	return coleccion, time.Unix(unix, 0).UTC(), nil
}

// profileContract proyecta el perfil al DTO del contrato. No hay ningún camino de
// serialización para atributos internos del item.
func profileContract(perfil *appProfile.Profile) ProfileContract {
	return ProfileContract{
		ID:            perfil.ID,
		Email:         perfil.Email,
		DisplayName:   perfil.DisplayName,
		EmailVerified: perfil.EmailVerified,
		CreatedAt:     perfil.CreatedAt.UTC().Format(time.RFC3339),
		Version:       perfil.Version,
	}
}

// consentContracts proyecta los consentimientos al DTO del contrato. La `SK` de la fila no se
// copia: el DTO sólo tiene los cinco campos publicados.
func consentContracts(registros []appPrivacy.ConsentRecord) []ConsentContract {
	salida := make([]ConsentContract, 0, len(registros))
	for _, registro := range registros {
		salida = append(salida, ConsentContract{
			ID:            registro.ID,
			NoticeVersion: registro.NoticeVersion,
			Purpose:       registro.Purpose,
			Granted:       registro.Granted,
			RecordedAt:    registro.RecordedAt.UTC().Format(time.RFC3339),
		})
	}
	return salida
}

// incomeContract proyecta el ingreso base. `nil` se serializa `null` porque el contrato
// declara la propiedad anulable; no se inventa un importe ni una frecuencia.
func incomeContract(ingreso *appIdentity.IncomeSnapshot) *IncomeContract {
	if ingreso == nil {
		return nil
	}
	return &IncomeContract{
		Amount:    httputil.FormatCentsToDecimal(ingreso.AmountCents),
		Currency:  MonedaMXN,
		Frequency: ingreso.Frequency.String(),
		Version:   ingreso.Version,
	}
}

// onboardingContract proyecta el estado de onboarding. Sin item, `null`.
func onboardingContract(estado *appOnboarding.State) *OnboardingContract {
	if estado == nil {
		return nil
	}
	pasos := estado.CompletedSteps
	if pasos == nil {
		pasos = []string{}
	}
	contrato := &OnboardingContract{
		CompletedSteps: pasos,
		Done:           estado.Done,
		Version:        estado.Version,
		Microphone:     estado.Microphone,
		Notifications:  estado.Notifications,
		Configured:     estado.Configured,
	}
	if estado.CompletedAt != nil && !estado.CompletedAt.IsZero() {
		marca := estado.CompletedAt.UTC().Format(time.RFC3339)
		contrato.CompletedAt = &marca
	}
	return contrato
}

// preferencesContract proyecta las preferencias. Sin item, `null`: la exportación no crea el
// item por defecto (a diferencia de E068, que materializa en su propia lectura).
func preferencesContract(prefs *appPreferences.Preferences) *PreferencesContract {
	if prefs == nil {
		return nil
	}
	locale := strings.TrimSpace(prefs.Locale)
	if locale == "" {
		locale = appPreferences.DefaultLocale
	}
	zona := strings.TrimSpace(prefs.Timezone)
	if zona == "" {
		zona = appPreferences.DefaultTimezone
	}
	version := prefs.Version
	if !prefs.HasVersion || version < 1 {
		version = appPreferences.DefaultVersion
	}
	return &PreferencesContract{
		Locale:   locale,
		Timezone: zona,
		Version:  version,
		TimeZone: zona,
	}
}

// contributionContract valida la fila y la proyecta al DTO. Un item que no puede representarse
// en el contrato (importe negativo, estado desconocido, moneda fuera del enum, versión no
// positiva o asignación inválida) no se emite.
func contributionContract(fila ContributionRow) (ContributionContract, error) {
	id := strings.TrimSpace(fila.ID)
	if id == "" || fila.AmountCents < 0 || fila.Version < 1 || fila.OccurredAt.IsZero() {
		return ContributionContract{}, fmt.Errorf("%w: aportacion invalida", ErrInconsistent)
	}
	switch fila.Status {
	case EstadoAportacionActiva, EstadoAportacionRevertida:
	default:
		return ContributionContract{}, fmt.Errorf("%w: estado de aportacion invalido", ErrInconsistent)
	}
	currency := strings.TrimSpace(fila.Currency)
	if currency == "" {
		currency = MonedaMXN
	}
	if currency != MonedaMXN {
		return ContributionContract{}, fmt.Errorf("%w: divisa de aportacion invalida", ErrInconsistent)
	}
	asignaciones := make([]AllocationContract, 0, len(fila.Allocations))
	for _, asignacion := range fila.Allocations {
		if strings.TrimSpace(asignacion.GoalID) == "" || asignacion.AmountCents <= 0 {
			return ContributionContract{}, fmt.Errorf("%w: asignacion invalida", ErrInconsistent)
		}
		asignaciones = append(asignaciones, AllocationContract{
			GoalID: asignacion.GoalID,
			Amount: httputil.FormatCentsToDecimal(asignacion.AmountCents),
		})
	}
	return ContributionContract{
		ID:          id,
		Amount:      httputil.FormatCentsToDecimal(fila.AmountCents),
		Currency:    currency,
		Allocations: asignaciones,
		OccurredAt:  fila.OccurredAt.UTC().Format(time.RFC3339),
		Status:      fila.Status,
		Version:     fila.Version,
	}, nil
}

// budgetContract valida la fila y la proyecta al DTO.
func budgetContract(fila BudgetRow) (BudgetContract, error) {
	categoria := strings.TrimSpace(fila.CategoryID)
	periodo := strings.TrimSpace(fila.Period)
	if categoria == "" || !periodPattern.MatchString(periodo) {
		return BudgetContract{}, fmt.Errorf("%w: presupuesto invalido", ErrInconsistent)
	}
	if fila.LimitAmountCents < 0 || fila.Version < 1 {
		return BudgetContract{}, fmt.Errorf("%w: presupuesto invalido", ErrInconsistent)
	}
	currency := strings.TrimSpace(fila.Currency)
	if currency == "" {
		currency = MonedaMXN
	}
	if currency != MonedaMXN {
		return BudgetContract{}, fmt.Errorf("%w: divisa de presupuesto invalida", ErrInconsistent)
	}
	return BudgetContract{
		CategoryID:  categoria,
		Period:      periodo,
		LimitAmount: httputil.FormatCentsToDecimal(fila.LimitAmountCents),
		Currency:    currency,
		Version:     fila.Version,
	}, nil
}
