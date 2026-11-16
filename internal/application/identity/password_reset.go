package identity

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/nexumpr/Nexum-Backend/internal/ports"
)

// PasswordResetService implementa la solicitud de recuperacion (E005).
//
// El 202 solo se emite cuando el trabajo quedo persistido. El envio NO se hace en la
// peticion: lo realiza el worker, de modo que la latencia no revela si la cuenta existe.
type PasswordResetService struct {
	store     ports.ResetStore
	correo    ports.Correo
	sellador  ports.Sellador
	limitador ports.Limitador
	reloj     func() time.Time
	ttl       time.Duration
	logger    *slog.Logger
	generador func(n int) ([]byte, error)
}

// NewPasswordResetService construye el servicio. Cualquier dependencia nil deja el
// servicio no disponible en lugar de degradar a un comportamiento ficticio.
func NewPasswordResetService(
	store ports.ResetStore,
	correo ports.Correo,
	sellador ports.Sellador,
	limitador ports.Limitador,
	logger *slog.Logger,
) *PasswordResetService {
	return &PasswordResetService{
		store:     store,
		correo:    correo,
		sellador:  sellador,
		limitador: limitador,
		reloj:     time.Now,
		ttl:       30 * time.Minute,
		logger:    logger,
		generador: func(n int) ([]byte, error) { b := make([]byte, n); _, err := rand.Read(b); return b, err },
	}
}

// Disponible indica si el endpoint puede operar de verdad.
func (s *PasswordResetService) Disponible() bool {
	return s != nil && s.store != nil && s.correo != nil && s.sellador != nil && s.limitador != nil
}

// BuscarCuenta resuelve el usuario a partir del correo.
//
// Devuelve encontrada=false solo cuando la cuenta no existe. Un fallo de DynamoDB
// debe devolverse como error: nunca se convierte en "cuenta inexistente".
type BuscarCuenta func(ctx context.Context, email string) (userID string, encontrada bool, err error)

// Solicitar ejecuta la admision del trabajo. No envia correo.
func (s *PasswordResetService) Solicitar(ctx context.Context, email string, buscar BuscarCuenta) error {
	if !s.Disponible() {
		return ports.ErrResetNoDisponible
	}
	email = strings.ToLower(strings.TrimSpace(email))

	permitido, err := s.limitador.Permitir(ctx, "pwreset:"+email)
	if err != nil {
		// Fail-closed: sin cuota fiable no se crea trabajo.
		return ports.ErrResetNoDisponible
	}
	if !permitido {
		return ports.ErrResetCuota
	}

	userID, encontrada, err := "", false, error(nil)
	if buscar != nil {
		userID, encontrada, err = buscar(ctx, email)
		if err != nil {
			// Fallo real de infraestructura: no se acepta la solicitud como si no existiera.
			return err
		}
	}
	if !encontrada {
		userID = ""
	}

	tokenBytes, err := s.generador(32)
	if err != nil {
		return err
	}
	token := base64.RawURLEncoding.EncodeToString(tokenBytes)
	suma := sha256.Sum256([]byte(token))
	hash := hex.EncodeToString(suma[:])
	ahora := s.reloj()
	outboxID := hash[:16]

	destino := ""
	var sobre []byte
	if encontrada {
		destino = email
		if sobre, err = s.sellador.Sellar(token, userID+"|"+outboxID); err != nil {
			return err
		}
	}

	adm := ports.ResetAdmission{
		UserID:     userID,
		Destino:    destino,
		TokenHash:  hash,
		TokenSello: sobre,
		ExpiraEn:   ahora.Add(s.ttl),
		OutboxID:   outboxID,
		Ahora:      ahora,
	}
	// Mismas dos acciones con o sin cuenta: ninguna condicion falla solo para
	// correos inexistentes.
	if err := s.store.Admitir(ctx, adm); err != nil {
		return err
	}
	if s.logger != nil {
		s.logger.InfoContext(ctx, "solicitud de recuperacion admitida", "outbox", outboxID)
	}
	return nil
}

// ValidarEmail aplica la validacion del contrato (formato y longitud).
func ValidarEmail(email string) bool {
	email = strings.TrimSpace(email)
	if len(email) < 3 || len(email) > 254 {
		return false
	}
	at := strings.LastIndex(email, "@")
	return at > 0 && at < len(email)-1 && strings.Contains(email[at+1:], ".")
}

// ErrResetNoDisponible se reexporta para que la capa de transporte no dependa de ports.
var ErrResetNoDisponible = ports.ErrResetNoDisponible

// ErrResetCuota se reexporta por la misma razon.
var ErrResetCuota = ports.ErrResetCuota

// EsConflicto indica si el error corresponde a un token ya consumido.
func EsConflicto(err error) bool {
	return errors.Is(err, ports.ErrResetTokenInvalido)
}
