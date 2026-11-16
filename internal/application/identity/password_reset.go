package identity

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"log/slog"
	"strings"
	"time"

	"github.com/nexumpr/Nexum-Backend/internal/ports"
)

// PasswordResetService implementa la solicitud de recuperacion (E005).
//
// Contrato (RFC-016 17.2): el 202 implica que la solicitud quedo persistida y que
// existe un camino real de envio. Sin sellador o sin correo configurado se devuelve
// ErrResetNoDisponible (503): nunca se acepta trabajo que nadie podria entregar.
type PasswordResetService struct {
	store     ports.ResetStore
	tokens    ports.ResetTokenStore
	correo    ports.Correo
	sellador  ports.Sellador
	reloj     func() time.Time
	ttl       time.Duration
	logger    *slog.Logger
	generador func(n int) ([]byte, error)
}

// NewPasswordResetService construye el servicio. Cualquier dependencia nil deja el
// servicio no disponible en lugar de degradar a un comportamiento ficticio.
func NewPasswordResetService(
	store ports.ResetStore,
	tokens ports.ResetTokenStore,
	correo ports.Correo,
	sellador ports.Sellador,
	logger *slog.Logger,
) *PasswordResetService {
	return &PasswordResetService{
		store:     store,
		tokens:    tokens,
		correo:    correo,
		sellador:  sellador,
		reloj:     time.Now,
		ttl:       30 * time.Minute,
		logger:    logger,
		generador: func(n int) ([]byte, error) { b := make([]byte, n); _, err := rand.Read(b); return b, err },
	}
}

// Disponible indica si el endpoint puede operar de verdad.
func (s *PasswordResetService) Disponible() bool {
	return s != nil && s.store != nil && s.correo != nil && s.sellador != nil
}

// BuscarCuenta resuelve el usuario a partir del correo. Se inyecta para no acoplar
// el flujo al repositorio de identidad y poder probarlo con dobles.
type BuscarCuenta func(ctx context.Context, email string) (userID string, err error)

// Solicitar ejecuta la admision y el intento de envio.
//
// Devuelve nil (202) cuando el trabajo quedo persistido, exista o no la cuenta.
// Devuelve ErrResetNoDisponible (503) cuando falta un componente real.
func (s *PasswordResetService) Solicitar(ctx context.Context, email string, buscar BuscarCuenta) error {
	if !s.Disponible() {
		return ports.ErrResetNoDisponible
	}
	email = strings.ToLower(strings.TrimSpace(email))

	tokenBytes, err := s.generador(32)
	if err != nil {
		return err
	}
	token := base64.RawURLEncoding.EncodeToString(tokenBytes)
	suma := sha256.Sum256([]byte(token))
	hash := hex.EncodeToString(suma[:])

	ahora := s.reloj()
	outboxID := hash[:16]

	userID := ""
	destino := ""
	if buscar != nil {
		if id, err := buscar(ctx, email); err == nil {
			userID, destino = id, email
		}
	}

	var sobre []byte
	if destino != "" {
		if sobre, err = s.sellador.Sellar(token, outboxID); err != nil {
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
	// Mismas dos acciones con o sin cuenta: no hay condicion que falle solo
	// para correos inexistentes.
	if err := s.store.Admitir(ctx, adm); err != nil {
		return err
	}

	if destino == "" {
		return nil
	}
	if _, err := s.correo.EnviarRecuperacion(ctx, destino, token, adm.ExpiraEn); err != nil {
		// El trabajo queda persistido; se reintentara. No se altera la respuesta
		// para no filtrar el estado del correo.
		if s.logger != nil {
			s.logger.WarnContext(ctx, "envio de recuperacion diferido", "motivo", "correo_no_aceptado")
		}
		return nil
	}
	if err := s.store.MarcarEnviado(ctx, outboxID, "", ahora); err != nil && s.logger != nil {
		s.logger.WarnContext(ctx, "no se pudo marcar el envio", "motivo", "persistencia")
	}
	return nil
}

// Reintentar procesa hasta `limite` trabajos vencidos. Barrido acotado por peticion
// mientras no exista el worker dedicado (RFC-016 17.2).
func (s *PasswordResetService) Reintentar(ctx context.Context, limite int) int {
	if !s.Disponible() || limite <= 0 {
		return 0
	}
	ahora := s.reloj()
	trabajos, err := s.store.Pendientes(ctx, ahora, limite)
	if err != nil {
		return 0
	}
	enviados := 0
	for _, t := range trabajos {
		if !t.ExpiraEn.After(ahora) || len(t.TokenSello) == 0 {
			continue
		}
		ok, err := s.store.Reclamar(ctx, t.OutboxID, ahora, 30*time.Second)
		if err != nil || !ok {
			continue
		}
		token, err := s.sellador.Abrir(t.TokenSello, t.OutboxID)
		if err != nil {
			continue
		}
		if _, err := s.correo.EnviarRecuperacion(ctx, t.Destino, token, t.ExpiraEn); err != nil {
			continue
		}
		if err := s.store.MarcarEnviado(ctx, t.OutboxID, "", ahora); err == nil {
			enviados++
		}
	}
	return enviados
}

// ValidarEmail aplica la validacion minima del contrato (formato y longitud).
func ValidarEmail(email string) bool {
	email = strings.TrimSpace(email)
	if len(email) < 3 || len(email) > 254 {
		return false
	}
	at := strings.LastIndex(email, "@")
	return at > 0 && at < len(email)-1 && strings.Contains(email[at+1:], ".")
}
