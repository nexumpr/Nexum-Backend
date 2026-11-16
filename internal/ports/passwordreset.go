package ports

import (
	"context"
	"errors"
	"time"
)

// Errores del flujo de recuperacion de contrasena (E005/E006).
var (
	// ErrResetNoDisponible: falta un componente (sellado o correo). El endpoint debe
	// responder 503: no se acepta trabajo que nadie podria entregar.
	ErrResetNoDisponible = errors.New("recuperacion de contrasena no disponible")
	// ErrResetTokenInvalido: token inexistente, expirado o ya usado. Nunca se distingue.
	ErrResetTokenInvalido = errors.New("token de recuperacion invalido")
	// ErrResetConflicto: el token fue consumido por otra peticion concurrente.
	ErrResetConflicto = errors.New("token de recuperacion ya consumido")
)

// ResetAdmission es lo que se persiste de forma atomica antes de responder 202.
type ResetAdmission struct {
	UserID     string // vacio si la cuenta no existe (no se crea perfil falso)
	Destino    string // correo real o vacio
	TokenHash  string // sha256 hex del token; el claro nunca se persiste
	TokenSello []byte // token cifrado para el envio (AES-256-GCM)
	ExpiraEn   time.Time
	OutboxID   string
	Ahora      time.Time
}

// ResetStore persiste la admision y el trabajo de envio.
type ResetStore interface {
	// Admitir ejecuta una transaccion de 2 acciones sobre 2 items distintos:
	// RESET#<hash> y QUEUE#PWRESET. Debe ser identica con o sin cuenta.
	Admitir(ctx context.Context, adm ResetAdmission) error
	// MarcarEnviado elimina el token cifrado del trabajo.
	MarcarEnviado(ctx context.Context, outboxID string, messageID string, ahora time.Time) error
	// Pendientes devuelve hasta `limite` trabajos vencidos de la particion QUEUE#PWRESET.
	Pendientes(ctx context.Context, ahora time.Time, limite int) ([]ResetTrabajo, error)
	// Reclamar toma el trabajo de forma condicional (estado PENDIENTE).
	Reclamar(ctx context.Context, outboxID string, ahora time.Time, lease time.Duration) (bool, error)
}

// ResetTrabajo es un envio pendiente reclamable.
type ResetTrabajo struct {
	OutboxID   string
	Destino    string
	TokenSello []byte
	ExpiraEn   time.Time
	Intentos   int
}

// ResetTokenStore resuelve el token que recibe E006 (publico, sin sesion).
type ResetTokenStore interface {
	// Buscar devuelve el userId y la expiracion; ErrResetTokenInvalido si no sirve.
	Buscar(ctx context.Context, tokenHash string, ahora time.Time) (string, time.Time, error)
	// Consumir marca el token como usado de forma condicional.
	Consumir(ctx context.Context, tokenHash string, ahora time.Time) error
}

// Correo envía el enlace de recuperacion.
type Correo interface {
	EnviarRecuperacion(ctx context.Context, destino, token string, expiraEn time.Time) (messageID string, err error)
}

// Sellador cifra y descifra el token para el reintento (AES-256-GCM).
type Sellador interface {
	Sellar(token string, aad string) ([]byte, error)
	Abrir(sobre []byte, aad string) (string, error)
}
