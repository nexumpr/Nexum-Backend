package privacy

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/nexumpr/Nexum-Backend/internal/domain/identity"
)

// Errores de aplicación de E017 (`POST /api/v1/users/me/consents`). El transporte los traduce
// a los códigos del contrato sin revelar el motivo exacto al cliente.
var (
	// ErrConsentDecisionInvalid cubre una decisión que no puede registrarse: `noticeVersion`
	// ausente o fuera de rango, `purpose` fuera del enum cerrado o `granted` ausente. Se
	// responde 422 BUSINESS_RULE_VIOLATION y no se escribe nada: el contrato prohíbe marcar
	// opt-in por omisión (E017 §1).
	ErrConsentDecisionInvalid = errors.New("decision de consentimiento invalida")

	// ErrConsentUnauthorized indica que no hay sujeto autenticado (401). El middleware ya lo
	// impide; esta guarda es defensa en profundidad.
	ErrConsentUnauthorized = errors.New("consentimiento no autorizado")

	// ErrConsentConflict indica que la clave física ya estaba ocupada y la condición
	// `attribute_not_exists(PK)` impidió sobrescribir un consentimiento existente (409). El
	// historial es append-only: nunca se reemplaza una decisión anterior.
	ErrConsentConflict = errors.New("consentimiento ya registrado")

	// ErrConsentUnavailable indica falta de persistencia o un fallo de DynamoDB (503). Falla
	// cerrado: nunca se responde 201 sin que la decisión esté durablemente almacenada.
	ErrConsentUnavailable = errors.New("servicio de consentimiento no disponible")
)

const (
	// ConsentNoticeVersionMaxLength replica `maxLength: 50` del esquema `ConsentRequest` y del
	// esquema `Consent` del contrato OpenAPI.
	ConsentNoticeVersionMaxLength = 50

	// ConsentSourceAPI es la fuente que se persiste en toda decisión tomada por la API. El
	// atributo `source` es interno: no forma parte del objeto `Consent` del contrato.
	ConsentSourceAPI = "api"
)

// ConsentDecision es la decisión ya validada que se persiste. `Granted` es un valor, no un
// puntero: la ausencia del campo se rechazó antes de llegar aquí.
type ConsentDecision struct {
	UserID        string
	ID            string
	NoticeVersion string
	Purpose       string
	Granted       bool
	RecordedAt    time.Time
	CreatedAt     time.Time
	Source        string
}

// ConsentWriter es el puerto de persistencia de E017 sobre la tabla única:
// `PutItem` en `USER#<id>` / `CONSENT#<RFC3339 fijo>#<uuid>` con
// `ConditionExpression = attribute_not_exists(PK)`. No se reescribe ni se borra nada.
type ConsentWriter interface {
	AppendConsent(ctx context.Context, decision ConsentDecision) error
}

// ConsentCommand es la entrada de aplicación de E017. `UserID` proviene de `sub` verificado;
// nunca del cuerpo, de la query ni de una cabecera libre. `Granted` es un puntero para poder
// distinguir «false» (denegación explícita, válida) de «campo ausente» (422).
type ConsentCommand struct {
	UserID        string
	NoticeVersion string
	Purpose       string
	Granted       *bool
}

// ConsentsWriteService orquesta el registro append-only de decisiones de consentimiento.
type ConsentsWriteService struct {
	writer     ConsentWriter
	now        func() time.Time
	generateID func() (string, error)
}

// NewConsentsWriteService construye el servicio. Sin almacén todas las llamadas fallan cerrado
// con ErrConsentUnavailable: nunca se responde 201 sin persistencia real.
func NewConsentsWriteService(writer ConsentWriter) *ConsentsWriteService {
	return &ConsentsWriteService{
		writer:     writer,
		now:        func() time.Time { return time.Now().UTC() },
		generateID: identity.NewID,
	}
}

// WithClock permite inyectar un reloj determinista en pruebas.
func (s *ConsentsWriteService) WithClock(now func() time.Time) *ConsentsWriteService {
	if now != nil {
		s.now = now
	}
	return s
}

// WithIDGenerator permite inyectar un generador de identificadores determinista en pruebas.
func (s *ConsentsWriteService) WithIDGenerator(generate func() (string, error)) *ConsentsWriteService {
	if generate != nil {
		s.generateID = generate
	}
	return s
}

// Record valida la decisión, genera el identificador y la persiste de forma append-only.
//
// Secuencia: sujeto autenticado → normalización y validación (`noticeVersion`, `purpose`,
// `granted`) → identificador UUID → escritura condicional → valor de contrato de cinco
// campos. Cualquier fallo de persistencia se traduce en 409 o 503, nunca en un éxito ficticio.
func (s *ConsentsWriteService) Record(ctx context.Context, command ConsentCommand) (*Consent, error) {
	if s == nil || s.writer == nil {
		return nil, ErrConsentUnavailable
	}

	userID := strings.TrimSpace(command.UserID)
	if userID == "" {
		return nil, ErrConsentUnauthorized
	}

	noticeVersion, purpose, granted, err := NormalizeConsentDecision(
		command.NoticeVersion, command.Purpose, command.Granted)
	if err != nil {
		return nil, err
	}

	now := s.now().UTC()
	if now.IsZero() {
		// Un reloj no determinable no puede fechar una decisión: falla cerrado.
		return nil, ErrConsentUnavailable
	}

	id, errID := s.generateID()
	id = strings.TrimSpace(id)
	if errID != nil || !isConsentID(id) {
		return nil, ErrConsentUnavailable
	}

	decision := ConsentDecision{
		UserID:        userID,
		ID:            id,
		NoticeVersion: noticeVersion,
		Purpose:       purpose,
		Granted:       granted,
		RecordedAt:    now,
		CreatedAt:     now,
		Source:        ConsentSourceAPI,
	}
	if err := s.writer.AppendConsent(ctx, decision); err != nil {
		switch {
		case errors.Is(err, ErrConsentConflict):
			return nil, ErrConsentConflict
		case errors.Is(err, ErrConsentUnauthorized):
			return nil, ErrConsentUnauthorized
		case errors.Is(err, ErrConsentDecisionInvalid):
			return nil, ErrConsentDecisionInvalid
		default:
			return nil, ErrConsentUnavailable
		}
	}

	return &Consent{
		ID:            decision.ID,
		NoticeVersion: decision.NoticeVersion,
		Purpose:       decision.Purpose,
		Granted:       decision.Granted,
		RecordedAt:    decision.RecordedAt.UTC(),
	}, nil
}

// NormalizeConsentDecision valida y normaliza los tres campos del `ConsentRequest`.
//
// Reglas del contrato:
//   - `noticeVersion` no puede quedar vacío (ni sólo espacios) y admite como máximo 50
//     caracteres Unicode; se rechazan caracteres de control.
//   - `purpose` debe pertenecer al enum cerrado `privacyNotice|notifications|analytics`.
//   - `granted` es obligatorio: su ausencia es 422 porque el contrato prohíbe interpretar el
//     silencio como consentimiento. `false` es una decisión válida y se conserva.
//
// Devuelve los valores normalizados (recortados) listos para persistirse.
func NormalizeConsentDecision(noticeVersion, purpose string, granted *bool) (string, string, bool, error) {
	version := strings.TrimSpace(noticeVersion)
	if version == "" || utf8.RuneCountInString(version) > ConsentNoticeVersionMaxLength || containsControlRune(version) {
		return "", "", false, ErrConsentDecisionInvalid
	}

	limpio := strings.TrimSpace(purpose)
	if !IsConsentPurpose(limpio) {
		return "", "", false, ErrConsentDecisionInvalid
	}

	if granted == nil {
		// Ausente o `null`: no se marca opt-in por omisión.
		return "", "", false, ErrConsentDecisionInvalid
	}

	return version, limpio, *granted, nil
}

// containsControlRune rechaza caracteres de control en un texto que se persiste y se devuelve
// al cliente: evita valores que rompan logs, cabeceras o la representación del contrato.
func containsControlRune(valor string) bool {
	for _, r := range valor {
		if r < 0x20 || r == 0x7f {
			return true
		}
	}
	return false
}

// isConsentID valida la forma UUID 8-4-4-4-12 en hexadecimal minúscula que exige
// `Consent.id` (`format: uuid`). No impone una versión concreta: el contrato admite cualquier
// UUID, y la comprobación existe para que un generador defectuoso no pueda emitir un
// identificador fuera del contrato ni introducir `#` en la clave física del historial.
func isConsentID(valor string) bool {
	if len(valor) != 36 {
		return false
	}
	for i, r := range valor {
		switch i {
		case 8, 13, 18, 23:
			if r != '-' {
				return false
			}
		default:
			if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')) {
				return false
			}
		}
	}
	return true
}
