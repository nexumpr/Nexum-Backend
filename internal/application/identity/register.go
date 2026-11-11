package identity

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	domain "github.com/nexumpr/Nexum-Backend/internal/domain/identity"
	"github.com/nexumpr/Nexum-Backend/internal/ports"
	"github.com/nexumpr/Nexum-Backend/pkg/httputil"
)

var (
	// ErrEmailAlreadyRegistered indica que el email ya tiene una reserva activa.
	ErrEmailAlreadyRegistered = errors.New("el correo electronico ya se encuentra registrado")

	// ErrIdempotencyConflict indica que la misma Idempotency-Key se reutilizo con otro payload.
	ErrIdempotencyConflict = errors.New("la Idempotency-Key ya fue utilizada con una carga util diferente")

	// ErrStoreUnavailable indica que DynamoDB no puede atender la operacion (503).
	ErrStoreUnavailable = errors.New("persistencia no disponible")

	// ErrRegistrationInternal indica un fallo interno no clasificable.
	ErrRegistrationInternal = errors.New("fallo interno al registrar usuario")

	// ErrTransactionTooLarge indica que el alta excede los limites de una transaccion
	// atomica de DynamoDB. Se rechaza completa: nunca se persiste una parte.
	ErrTransactionTooLarge = errors.New("la operacion excede el limite de una transaccion atomica")
)

const (
	// RegisterOperation identifica la operacion en el registro de idempotencia.
	RegisterOperation = "E001_REGISTER"

	// RegisterIdempotencyTTL es la ventana de replay de la Idempotency-Key.
	RegisterIdempotencyTTL = 24 * time.Hour

	// RegisterCurrency es la divisa del alta inicial (unica admitida en E001).
	RegisterCurrency = "MXN"

	minPasswordRunes = 12
	maxPasswordRunes = 128
	minIdemKeyBytes  = 16
	maxIdemKeyBytes  = 128
	maxNameRunes     = 100
	maxPrivacyRunes  = 50
)

// FieldViolation describe un error de validacion atribuible a un campo del contrato.
// Business=true se traduce a 422; Business=false a 400.
type FieldViolation struct {
	Field    string
	Reason   string
	Business bool
}

func (v *FieldViolation) Error() string {
	return fmt.Sprintf("campo %q invalido: %s", v.Field, v.Reason)
}

// IncomeCommand es el ingreso habitual declarado, ya convertido a centavos exactos.
type IncomeCommand struct {
	AmountCents int64
	Frequency   string
}

// RecurringExpenseCommand es una fila de compromiso fijo del alta inicial.
type RecurringExpenseCommand struct {
	Concept     string
	AmountCents int64
	Frequency   string
	Category    string
	Type        string
}

// PrivacyCommand es el consentimiento declarado por el cliente.
type PrivacyCommand struct {
	Accepted      bool
	NoticeVersion string
}

// ClientCommand es el contexto no financiero declarado por el cliente.
type ClientCommand struct {
	Locale   string
	TimeZone string
}

// RegisterCommand es la entrada de aplicacion de E001. No contiene dependencias HTTP.
type RegisterCommand struct {
	Name              string
	Email             string
	Password          string
	Income            IncomeCommand
	RecurringExpenses []RecurringExpenseCommand
	Privacy           PrivacyCommand
	Client            ClientCommand
	IdempotencyKey    string
}

// UserProfile conserva el contrato de perfil usado por E012 y por el resto de
// operaciones de usuario. E001 ya no responde con este esquema.
type UserProfile struct {
	ID            string `json:"id"`
	Email         string `json:"email"`
	DisplayName   string `json:"displayName"`
	EmailVerified bool   `json:"emailVerified"`
	CreatedAt     string `json:"createdAt"`
	Version       int64  `json:"version"`
}

// RegisterUser es el bloque `user` de la respuesta 201 de E001 V6.0.2.
type RegisterUser struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Email         string `json:"email"`
	EmailVerified bool   `json:"emailVerified"`
	CreatedAt     string `json:"createdAt"`
}

// RegisterIncome es el bloque `income` de la respuesta 201 de E001 V6.0.2.
type RegisterIncome struct {
	Amount            string `json:"amount"`
	Frequency         string `json:"frequency"`
	MonthlyEquivalent string `json:"monthlyEquivalent"`
}

// RegisterRecurringPayment es cada fila de `recurringPayments` de la respuesta 201.
type RegisterRecurringPayment struct {
	ID        string `json:"id"`
	Concept   string `json:"concept"`
	Amount    string `json:"amount"`
	Frequency string `json:"frequency"`
	Category  string `json:"category"`
	Type      string `json:"type"`
}

// RegisterConsent es el bloque `consent` de la respuesta 201. `acceptedAt` lo fija
// siempre el servidor y `acceptedByServer` declara ese hecho.
type RegisterConsent struct {
	NoticeVersion    string `json:"noticeVersion"`
	AcceptedAt       string `json:"acceptedAt"`
	AcceptedByServer bool   `json:"acceptedByServer"`
}

// RegisterOnboarding es el bloque `onboarding` de la respuesta 201.
type RegisterOnboarding struct {
	Microphone    string `json:"microphone"`
	Notifications string `json:"notifications"`
	Configured    bool   `json:"configured"`
}

// RegisterResponse es la respuesta contractual completa de E001 V6.0.2.
type RegisterResponse struct {
	User              RegisterUser               `json:"user"`
	Income            RegisterIncome             `json:"income"`
	RecurringPayments []RegisterRecurringPayment `json:"recurringPayments"`
	Consent           RegisterConsent            `json:"consent"`
	Onboarding        RegisterOnboarding         `json:"onboarding"`
}

// StoredResponse es la respuesta HTTP inmutable que se guarda con la Idempotency-Key.
// Replayed solo se marca en memoria al recuperar una respuesta ya almacenada.
//
// PasswordVerifier transporta el hash Argon2id del solicitante original para que un
// replay pueda probar que la contrasena presentada es la misma. Nunca se serializa ni
// se registra: existe solo en memoria mientras dura la comprobacion.
type StoredResponse struct {
	Status           int
	Body             []byte
	Location         string
	Replayed         bool
	PasswordVerifier string
}

// RegistrationIncome es el ingreso persistible en centavos exactos.
type RegistrationIncome struct {
	AmountCents            int64
	Frequency              string
	MonthlyEquivalentCents int64
}

// RegistrationExpense es un compromiso fijo persistible. No es un gasto ejecutado.
type RegistrationExpense struct {
	ID          string
	Concept     string
	AmountCents int64
	Frequency   string
	Category    string
	Type        string
}

// RegistrationRecord es el conjunto de filas que se persiste atomicamente con la
// reserva de email y el registro de idempotencia. No contiene la contrasena en claro.
type RegistrationRecord struct {
	UserID                  string
	Email                   string
	Name                    string
	PasswordHash            string
	Income                  RegistrationIncome
	RecurringExpenses       []RegistrationExpense
	ConsentNoticeVersion    string
	ConsentAcceptedAt       time.Time
	EmailVerified           bool
	Version                 int64
	CreatedAt               time.Time
	UpdatedAt               time.Time
	OnboardingMicrophone    string
	OnboardingNotifications string
	OnboardingConfigured    bool
	ClientLocale            string
	ClientTimeZone          string
}

// RegisterStore es el puerto atomico de E001. CreateOrReplay debe:
//   - crear reserva de email, perfil, ingreso, compromisos, consentimiento,
//     onboarding e idempotencia en una sola transaccion;
//   - devolver la respuesta guardada si la clave ya existe con el mismo payload;
//   - devolver ErrIdempotencyConflict si el payload difiere;
//   - devolver ErrEmailAlreadyRegistered si la reserva de email ya existe;
//   - devolver ErrStoreUnavailable si la persistencia no responde.
type RegisterStore interface {
	CreateOrReplay(
		ctx context.Context,
		idempotencyKey, operation, payloadHash string,
		record RegistrationRecord,
		response StoredResponse,
		ttl time.Duration,
	) (*StoredResponse, error)
}

// RegisterService orquesta el registro E001 sin conocer HTTP ni AWS.
type RegisterService struct {
	store    RegisterStore
	hasher   ports.PasswordHasher
	now      func() time.Time
	newID    func() (string, error)
	location string
}

// NewRegisterService construye el servicio con reloj y generador de UUID reales.
func NewRegisterService(store RegisterStore, hasher ports.PasswordHasher) *RegisterService {
	return &RegisterService{
		store:    store,
		hasher:   hasher,
		now:      func() time.Time { return time.Now().UTC() },
		newID:    domain.NewID,
		location: "/api/v1/users/me",
	}
}

// WithClock permite inyectar un reloj determinista en pruebas.
func (s *RegisterService) WithClock(now func() time.Time) *RegisterService {
	if now != nil {
		s.now = now
	}
	return s
}

// WithIDGenerator permite inyectar un generador determinista en pruebas. Se usa tanto
// para el usuario como para cada compromiso fijo, por lo que un generador de prueba
// debe ser secuencial cuando la peticion incluye gastos recurrentes.
func (s *RegisterService) WithIDGenerator(gen func() (string, error)) *RegisterService {
	if gen != nil {
		s.newID = gen
	}
	return s
}

// validatedRegister contiene las entradas ya validadas listas para el agregado.
type validatedRegister struct {
	name     string
	email    string
	income   domain.InitialIncome
	expenses []domain.RecurringExpense
	privacy  PrivacyCommand
	client   domain.ClientContext
	idemKey  string
}

// ValidateRegisterCommand normaliza y valida la entrada completa de E001 V6.0.2.
// Devuelve las entradas validadas o un *FieldViolation con el campo responsable.
func ValidateRegisterCommand(in RegisterCommand) (validatedRegister, error) {
	var out validatedRegister

	cleanEmail, err := domain.ValidateEmail(in.Email)
	if err != nil {
		return out, &FieldViolation{Field: "email", Reason: "invalid_email"}
	}
	out.email = cleanEmail

	passwordRunes := utf8.RuneCountInString(in.Password)
	if passwordRunes < minPasswordRunes || passwordRunes > maxPasswordRunes {
		return out, &FieldViolation{Field: "password", Reason: "invalid_length"}
	}
	if strings.TrimSpace(in.Password) == "" {
		return out, &FieldViolation{Field: "password", Reason: "blank", Business: true}
	}

	name := strings.TrimSpace(in.Name)
	if name == "" {
		return out, &FieldViolation{Field: "name", Reason: "blank", Business: true}
	}
	if utf8.RuneCountInString(name) > maxNameRunes {
		return out, &FieldViolation{Field: "name", Reason: "too_long"}
	}
	if hasControlRune(name) {
		return out, &FieldViolation{Field: "name", Reason: "control_characters", Business: true}
	}
	out.name = name

	// Ingreso habitual: importe exacto en centavos y frecuencia del enum cerrado.
	if in.Income.AmountCents <= 0 {
		return out, &FieldViolation{Field: "income.amount", Reason: "must_be_positive", Business: true}
	}
	income, err := domain.NewInitialIncome(in.Income.AmountCents, in.Income.Frequency)
	if err != nil {
		if errors.Is(err, domain.ErrInvalidFrequency) {
			return out, &FieldViolation{Field: "income.frequency", Reason: "unsupported_frequency"}
		}
		return out, &FieldViolation{Field: "income.amount", Reason: "invalid_amount", Business: true}
	}
	out.income = income

	// Gastos fijos iniciales: lista completa, sin truncar y sin filas vacias.
	if len(in.RecurringExpenses) > domain.MaxRecurringExpenses {
		return out, &FieldViolation{
			Field:    "recurringExpenses",
			Reason:   "too_many_items",
			Business: true,
		}
	}
	out.expenses = make([]domain.RecurringExpense, 0, len(in.RecurringExpenses))
	for i, item := range in.RecurringExpenses {
		expense, err := domain.NewRecurringExpense(item.Concept, item.AmountCents, item.Frequency, item.Category, item.Type)
		if err != nil {
			return out, &FieldViolation{
				Field:    expenseField(i, err),
				Reason:   expenseReason(err),
				Business: errors.Is(err, domain.ErrInvalidAmount),
			}
		}
		out.expenses = append(out.expenses, expense)
	}

	privacyVersion := strings.TrimSpace(in.Privacy.NoticeVersion)
	if privacyVersion == "" {
		return out, &FieldViolation{Field: "privacy.noticeVersion", Reason: "required"}
	}
	if utf8.RuneCountInString(privacyVersion) > maxPrivacyRunes {
		return out, &FieldViolation{Field: "privacy.noticeVersion", Reason: "too_long"}
	}
	if hasControlRune(privacyVersion) {
		return out, &FieldViolation{Field: "privacy.noticeVersion", Reason: "control_characters", Business: true}
	}
	if !in.Privacy.Accepted {
		return out, &FieldViolation{Field: "privacy.accepted", Reason: "must_be_true", Business: true}
	}
	out.privacy = PrivacyCommand{Accepted: true, NoticeVersion: privacyVersion}

	client, err := domain.NewClientContext(in.Client.Locale, in.Client.TimeZone)
	if err != nil {
		field := "client.timeZone"
		if strings.Contains(err.Error(), "locale") {
			field = "client.locale"
		}
		return out, &FieldViolation{Field: field, Reason: "invalid_value"}
	}
	out.client = client

	if !ValidIdempotencyKey(in.IdempotencyKey) {
		return out, &FieldViolation{Field: "Idempotency-Key", Reason: "invalid"}
	}
	out.idemKey = strings.TrimSpace(in.IdempotencyKey)

	return out, nil
}

// expenseField traduce el error de dominio de una fila al nombre del campo del contrato.
func expenseField(index int, err error) string {
	prefix := fmt.Sprintf("recurringExpenses[%d]", index)
	switch {
	case errors.Is(err, domain.ErrInvalidExpenseConcept):
		return prefix + ".concept"
	case errors.Is(err, domain.ErrInvalidAmount):
		return prefix + ".amount"
	case errors.Is(err, domain.ErrInvalidFrequency):
		return prefix + ".frequency"
	case errors.Is(err, domain.ErrInvalidCategory):
		return prefix + ".category"
	case errors.Is(err, domain.ErrInvalidExpenseType):
		return prefix + ".type"
	default:
		return prefix
	}
}

func expenseReason(err error) string {
	switch {
	case errors.Is(err, domain.ErrInvalidExpenseConcept):
		return "invalid_concept"
	case errors.Is(err, domain.ErrInvalidAmount):
		return "must_be_positive"
	case errors.Is(err, domain.ErrInvalidFrequency):
		return "unsupported_frequency"
	case errors.Is(err, domain.ErrInvalidCategory):
		return "unsupported_category"
	case errors.Is(err, domain.ErrInvalidExpenseType):
		return "must_be_fixed"
	default:
		return "invalid_value"
	}
}

// ValidIdempotencyKey aplica el contrato 16..128 caracteres imprimibles ASCII.
func ValidIdempotencyKey(key string) bool {
	trimmed := strings.TrimSpace(key)
	if len(trimmed) < minIdemKeyBytes || len(trimmed) > maxIdemKeyBytes {
		return false
	}
	for _, r := range trimmed {
		if r < 0x21 || r > 0x7e {
			return false
		}
	}
	return true
}

func hasControlRune(value string) bool {
	for _, r := range value {
		if unicode.IsControl(r) {
			return true
		}
	}
	return false
}

// Register ejecuta E001. La contrasena se hashea con Argon2id antes de persistir y
// nunca se devuelve ni se registra. La operacion completa (identidad, ingreso,
// compromisos, consentimiento, onboarding e idempotencia) es una sola transaccion.
func (s *RegisterService) Register(ctx context.Context, in RegisterCommand) (*StoredResponse, error) {
	if s.store == nil || s.hasher == nil {
		return nil, ErrStoreUnavailable
	}

	validated, err := ValidateRegisterCommand(in)
	if err != nil {
		return nil, err
	}

	passwordHash, err := s.hasher.HashPassword(in.Password)
	if err != nil {
		return nil, ErrRegistrationInternal
	}

	now := s.now().UTC()

	userID, err := s.newID()
	if err != nil {
		return nil, ErrRegistrationInternal
	}

	expenses := make([]domain.RecurringExpense, 0, len(validated.expenses))
	for _, expense := range validated.expenses {
		expenseID, idErr := s.newID()
		if idErr != nil {
			return nil, ErrRegistrationInternal
		}
		withID, idErr := expense.WithID(expenseID)
		if idErr != nil {
			return nil, ErrRegistrationInternal
		}
		expenses = append(expenses, withID)
	}

	consent, err := domain.NewConsent(validated.privacy.NoticeVersion, now)
	if err != nil {
		return nil, ErrRegistrationInternal
	}

	registration, err := domain.NewRegistration(domain.RegistrationParams{
		UserID:            userID,
		Email:             validated.email,
		Name:              validated.name,
		PasswordHash:      passwordHash,
		Income:            validated.income,
		RecurringExpenses: expenses,
		Consent:           consent,
		Client:            validated.client,
	}, now)
	if err != nil {
		return nil, ErrRegistrationInternal
	}

	response := buildResponse(registration)

	body, err := json.Marshal(response)
	if err != nil {
		return nil, ErrRegistrationInternal
	}

	record := buildRecord(registration)

	stored := StoredResponse{
		Status:   201,
		Body:     body,
		Location: s.location,
		// El verificador Argon2id permite distinguir "mismo payload" de "misma clave
		// con otra contrasena" sin almacenar ni hashear la contrasena en claro ni
		// usar un SHA-256 simple. Es el mismo hash que ya persiste PROFILE.
		PasswordVerifier: passwordHash,
	}

	replayed, err := s.store.CreateOrReplay(
		ctx,
		validated.idemKey,
		RegisterOperation,
		payloadHash(validated),
		record,
		stored,
		RegisterIdempotencyTTL,
	)
	if err != nil {
		return nil, err
	}
	if replayed != nil {
		// La clave ya existia con el mismo payload. La contrasena NO forma parte del
		// payloadHash (un hash de contrasena no es determinista por el sal Argon2id),
		// asi que se comprueba aqui contra el verificador Argon2id guardado: una
		// contrasena distinta con la misma clave es un conflicto, no un replay.
		if replayed.PasswordVerifier == "" {
			return nil, ErrIdempotencyConflict
		}
		same, verifyErr := s.hasher.VerifyPassword(in.Password, replayed.PasswordVerifier)
		if verifyErr != nil || !same {
			return nil, ErrIdempotencyConflict
		}
		replayed.PasswordVerifier = ""
		return replayed, nil
	}
	return &stored, nil
}

func buildResponse(registration *domain.Registration) RegisterResponse {
	income := registration.Income()
	expenses := registration.RecurringExpenses()

	payments := make([]RegisterRecurringPayment, 0, len(expenses))
	for _, expense := range expenses {
		payments = append(payments, RegisterRecurringPayment{
			ID:        expense.ID(),
			Concept:   expense.Concept(),
			Amount:    httputil.FormatCentsToDecimal(expense.AmountCents()),
			Frequency: expense.Frequency().String(),
			Category:  expense.Category(),
			Type:      expense.Type(),
		})
	}

	onboarding := registration.Onboarding()
	consent := registration.Consent()

	return RegisterResponse{
		User: RegisterUser{
			ID:            registration.UserID(),
			Name:          registration.Name(),
			Email:         registration.Email(),
			EmailVerified: registration.EmailVerified(),
			CreatedAt:     registration.CreatedAt().UTC().Format(time.RFC3339),
		},
		Income: RegisterIncome{
			Amount:            httputil.FormatCentsToDecimal(income.AmountCents()),
			Frequency:         income.Frequency().String(),
			MonthlyEquivalent: httputil.FormatCentsToDecimal(income.MonthlyEquivalentCents()),
		},
		RecurringPayments: payments,
		Consent: RegisterConsent{
			NoticeVersion:    consent.NoticeVersion(),
			AcceptedAt:       consent.AcceptedAt().UTC().Format(time.RFC3339),
			AcceptedByServer: true,
		},
		Onboarding: RegisterOnboarding{
			Microphone:    onboarding.Microphone(),
			Notifications: onboarding.Notifications(),
			Configured:    onboarding.Configured(),
		},
	}
}

func buildRecord(registration *domain.Registration) RegistrationRecord {
	income := registration.Income()
	expenses := registration.RecurringExpenses()

	persisted := make([]RegistrationExpense, 0, len(expenses))
	for _, expense := range expenses {
		persisted = append(persisted, RegistrationExpense{
			ID:          expense.ID(),
			Concept:     expense.Concept(),
			AmountCents: expense.AmountCents(),
			Frequency:   expense.Frequency().String(),
			Category:    expense.Category(),
			Type:        expense.Type(),
		})
	}

	onboarding := registration.Onboarding()

	return RegistrationRecord{
		UserID:       registration.UserID(),
		Email:        registration.Email(),
		Name:         registration.Name(),
		PasswordHash: registration.PasswordHash(),
		Income: RegistrationIncome{
			AmountCents:            income.AmountCents(),
			Frequency:              income.Frequency().String(),
			MonthlyEquivalentCents: income.MonthlyEquivalentCents(),
		},
		RecurringExpenses:       persisted,
		ConsentNoticeVersion:    registration.PrivacyNoticeVersion(),
		ConsentAcceptedAt:       registration.PrivacyAcceptedAt(),
		EmailVerified:           registration.EmailVerified(),
		Version:                 registration.Version(),
		CreatedAt:               registration.CreatedAt(),
		UpdatedAt:               registration.UpdatedAt(),
		OnboardingMicrophone:    onboarding.Microphone(),
		OnboardingNotifications: onboarding.Notifications(),
		OnboardingConfigured:    onboarding.Configured(),
		ClientLocale:            registration.Client().Locale(),
		ClientTimeZone:          registration.Client().TimeZone(),
	}
}

// payloadHash calcula un hash canonico y determinista de TODO el payload normalizado
// que determina la operacion: identidad, ingreso, compromisos fijos en su orden de
// envio, consentimiento y contexto de cliente. No incluye la clave de idempotencia ni
// los identificadores que genera el servidor.
func payloadHash(in validatedRegister) string {
	h := sha256.New()
	writePart := func(value string) {
		h.Write([]byte(strconv.Itoa(len(value))))
		h.Write([]byte(":"))
		h.Write([]byte(value))
	}

	writePart(in.name)
	writePart(in.email)
	writePart(strconv.FormatInt(in.income.AmountCents(), 10))
	writePart(in.income.Frequency().String())

	writePart(strconv.Itoa(len(in.expenses)))
	for _, expense := range in.expenses {
		writePart(expense.Concept())
		writePart(strconv.FormatInt(expense.AmountCents(), 10))
		writePart(expense.Frequency().String())
		writePart(expense.Category())
		writePart(expense.Type())
	}

	writePart(in.privacy.NoticeVersion)
	if in.privacy.Accepted {
		writePart("1")
	} else {
		writePart("0")
	}
	writePart(in.client.Locale())
	writePart(in.client.TimeZone())

	return hex.EncodeToString(h.Sum(nil))
}
