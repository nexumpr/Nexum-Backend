package ports

import (
	"context"
	"errors"
	"time"
)

// Errores del flujo de recuperacion de contrasena (E005/E006).
var (
	// ErrResetNoDisponible: falta un componente (sellado, cola o correo). El endpoint
	// responde 503: no se acepta trabajo que nadie podria entregar.
	ErrResetNoDisponible = errors.New("recuperacion de contrasena no disponible")
	// ErrResetCuota: se supero el limite de solicitudes. El endpoint responde 429.
	ErrResetCuota = errors.New("limite de solicitudes de recuperacion superado")
	// ErrResetTokenInvalido: token inexistente, expirado o ya usado (E006).
	ErrResetTokenInvalido = errors.New("token de recuperacion invalido")
)

// ResetAdmission es lo que se persiste de forma atomica antes de responder 202.
type ResetAdmission struct {
	UserID     string // vacio si la cuenta no existe (no se crea perfil falso)
	Destino    string // correo real o vacio
	TokenHash  string // sha256 hex del token; el claro nunca se persiste
	TokenSello []byte // token cifrado para el envio (AES-256-GCM, AAD userID|outboxID)
	ExpiraEn   time.Time
	OutboxID   string
	Ahora      time.Time
}

// ResetTrabajo es un envio pendiente reclamable por el worker.
type ResetTrabajo struct {
	OutboxID   string
	UserID     string
	Destino    string
	TokenSello []byte
	ExpiraEn   time.Time
	Intentos   int
	// Fence identifica la reclamacion vigente: un worker con valor antiguo no
	// puede cerrar el trabajo que otro ya reclamo.
	Fence int
}

// ResetStore persiste la admision y la cola durable de envio.
type ResetStore interface {
	// Admitir ejecuta una transaccion con 2 acciones sobre 2 items distintos:
	// RESET#<hash> y QUEUE#PWRESET/<vencimiento>#<id>. Identica con o sin cuenta.
	Admitir(ctx context.Context, adm ResetAdmission) error
	// Pendientes devuelve hasta `limite` trabajos vencidos de QUEUE#PWRESET.
	Pendientes(ctx context.Context, ahora time.Time, limite int) ([]ResetTrabajo, error)
	// Reclamar toma el trabajo de forma condicional (PENDIENTE -> ENVIANDO + fence).
	Reclamar(ctx context.Context, outboxID string, ahora time.Time, lease time.Duration) (bool, error)
	// MarcarEnviado cierra el trabajo y elimina el token cifrado.
	MarcarEnviado(ctx context.Context, outboxID, messageID string, ahora time.Time) error
	// Reintentar reprograma el trabajo con backoff o lo cierra como fallido/expirado.
	Reintentar(ctx context.Context, outboxID string, intentos int, proximo time.Time, ahora time.Time, fallido bool) error
}

// ResetTokenStore resuelve y consume el token de E006 (publico, sin sesion).
type ResetTokenStore interface {
	Buscar(ctx context.Context, tokenHash string, ahora time.Time) (string, time.Time, error)
	Consumir(ctx context.Context, tokenHash string, ahora time.Time) error
}

// Correo envia el enlace de recuperacion (SES API v2).
type Correo interface {
	EnviarRecuperacion(ctx context.Context, destino, token string, expiraEn time.Time) (messageID string, err error)
}

// Sellador cifra y descifra el token para el reenvio (AES-256-GCM).
// El AAD aprobado es "<userID>|<outboxID>" y debe ser identico al sellar y al abrir.
type Sellador interface {
	Sellar(token, aad string) ([]byte, error)
	Abrir(sobre []byte, aad string) (string, error)
}

// Limitador aplica cuotas antes de crear trabajo persistente.
type Limitador interface {
	// Permitir devuelve false cuando se supera la cuota.
	Permitir(ctx context.Context, clave string) (bool, error)
}

// Arrendador coordina el worker entre las dos instancias (lease en Redis).
type Arrendador interface {
	Adquirir(ctx context.Context, clave string, ttl time.Duration) (bool, error)
}
