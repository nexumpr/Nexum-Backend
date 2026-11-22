// Package pushoutbox orquesta el trabajo durable de notificaciones push.
//
// Reglas de seguridad (fail-closed):
//   - Antes de cada envío se consulta notificationprefs.Service.Get. Sin item,
//     con pushEnabled=false o sin zona IANA válida para quietHours, NO se envía.
//   - La idempotencia por eventId+userID se resuelve en DynamoDB con escritura
//     condicional; el lease impide doble envío en dos nodos.
//   - La entrada en la bandeja E061 se persiste ANTES de llamar a FCM. Si no se
//     puede persistir, no se envía.
package pushoutbox

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
	_ "time/tzdata" // base IANA embebida: evita depender del SO y del paquete HTTP

	appPrefs "github.com/nexumpr/Nexum-Backend/internal/application/notificationprefs"
	"github.com/nexumpr/Nexum-Backend/internal/infrastructure/push"
)

// Estado del trabajo en el outbox.
type Estado string

const (
	EstadoPendiente Estado = "pending"
	EstadoEnviando  Estado = "sending"
	EstadoEnviado   Estado = "sent"
	EstadoFallido   Estado = "failed"
	EstadoRevocado  Estado = "revoked"
)

// Clases de decisión que se persisten como lastErrorClass (sin PII ni tokens).
const (
	ClaseSinConsentimiento   = "NO_CONSENT"
	ClaseZonaInvalida        = "ZONA_INVALIDA"
	ClaseHorarioInvalido     = "HORARIO_INVALIDO"
	ClaseQuietHours          = "QUIET_HOURS"
	ClaseSinDispositivos     = "SIN_DISPOSITIVOS"
	ClaseTokensNoDisponibles = "TOKENS_NO_DISPONIBLES"
	ClaseBandejaNoDisponible = "BANDEJA_NO_DISPONIBLE"
	ClaseEntregaDuplicada    = "ENTREGA_DUPLICADA"
	ClaseConfigFCM           = "CONFIG_FCM"
	ClaseTokenNoRegistrado   = "UNREGISTERED"
	ClaseArgumentoInvalido   = "INVALID_ARGUMENT"
	ClaseReintentable        = "REINTENTABLE"
)

// Solicitud es la entrada para encolar una notificación desde un evento de dominio.
type Solicitud struct {
	UserID         string
	Title          string
	Message        string
	Type           string
	EventID        string
	NotificationID string // opcional; se deriva de forma determinista si falta
	OcurridoEn     time.Time
}

// Registro es el ítem durable del outbox.
type Registro struct {
	ID             string // SK completa DUE#<RFC3339Nano>#<id>
	UserID         string
	NotificationID string
	Title          string
	Message        string
	Type           string
	EventID        string
	Status         Estado
	Attempts       int
	LeaseUntil     time.Time
	NextAttemptAt  time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
	UltimaClase    string
}

// RegistroBandeja es la proyección mínima del ítem E061.
type RegistroBandeja struct {
	UserID    string
	ID        string
	Title     string
	Message   string
	Type      string
	CreatedAt time.Time
}

// TokenDestino es un token FCM ya descifrado y su identificador de dispositivo.
type TokenDestino struct {
	ID    string
	Valor string
}

// TokenSource entrega los tokens activos del usuario. Nunca se loguea su valor.
type TokenSource interface {
	TokensActivos(ctx context.Context, userID string) ([]TokenDestino, error)
}

// TokenAbridor es el subconjunto del sellador AES-GCM que necesita el worker.
type TokenAbridor interface {
	Abrir(sobre []byte, aad string) (string, error)
}

// RevocadorTokens permite dar de baja un dispositivo cuando FCM devuelve
// UNREGISTERED. Es opcional: si no está disponible, se marca el trabajo como
// revocado y el propio sellor/store podrá limpiar el registro por E060.
type RevocadorTokens interface {
	RevocarTokenPush(ctx context.Context, userID, tokenID string) error
}

// PreferenceReader es satisfecho por *notificationprefs.Service.Get.
type PreferenceReader interface {
	Get(ctx context.Context, userID string) (*appPrefs.Preferences, error)
}

// NotificationSink persiste la notificación en la bandeja E061.
type NotificationSink interface {
	GuardarBandeja(ctx context.Context, bandeja RegistroBandeja) error
}

// Store es el puerto durable del outbox. La implementación real es
// *dynamo.PushOutboxStore sobre la tabla única existente.
type Store interface {
	Crear(ctx context.Context, registro Registro) (bool, error)
	Pendientes(ctx context.Context, ahora time.Time, limite int) ([]Registro, error)
	Reclamar(ctx context.Context, id string, ahora time.Time, lease time.Duration) (int, bool, error)
	MarcarEnviado(ctx context.Context, id string, ahora time.Time) error
	MarcarRevocado(ctx context.Context, id string, clase string, ahora time.Time) error
	Reprogramar(ctx context.Context, id string, attempts int, clase string, siguiente time.Time, ahora time.Time) error
	YaEntregado(ctx context.Context, userID, eventID string) (bool, error)
	MarcarEntregado(ctx context.Context, userID, eventID, notificationID string, ahora time.Time) error
}

// Opciones ajusta el worker sin acoplarlo al runtime HTTP.
type Opciones struct {
	Intervalo   time.Duration
	Lease       time.Duration
	Lote        int
	MaxIntentos int
	Logger      *slog.Logger
	Ahora       func() time.Time
}

func (o Opciones) conDefaults() Opciones {
	if o.Intervalo <= 0 {
		o.Intervalo = 15 * time.Second
	}
	if o.Lease <= 0 {
		o.Lease = 30 * time.Second
	}
	if o.Lote <= 0 {
		o.Lote = 25
	}
	if o.MaxIntentos <= 0 {
		o.MaxIntentos = 5
	}
	if o.Ahora == nil {
		o.Ahora = func() time.Time { return time.Now().UTC() }
	}
	return o
}

// Service es el worker idempotente. Se construye una sola vez por proceso.
type Service struct {
	store        Store
	emisor       push.Emisor
	preferencias PreferenceReader
	tokens       TokenSource
	bandeja      NotificationSink
	opciones     Opciones
}

// NewService construye el servicio. Sin store o sin emisor queda inerte; sin
// preferencias, tokens o bandeja NO envía (fail-closed).
func NewService(store Store, emisor push.Emisor, preferencias PreferenceReader, tokens TokenSource, bandeja NotificationSink, opciones Opciones) *Service {
	return &Service{
		store:        store,
		emisor:       emisor,
		preferencias: preferencias,
		tokens:       tokens,
		bandeja:      bandeja,
		opciones:     opciones.conDefaults(),
	}
}

// Disponible indica si el worker puede ejecutar.
func (s *Service) Disponible() bool {
	return s != nil && s.store != nil && s.emisor != nil
}

// Run procesa un lote inicial y continúa con un ticker hasta cancelar el contexto.
func (s *Service) Run(ctx context.Context) {
	if !s.Disponible() {
		return
	}
	s.Ciclo(ctx)
	ticker := time.NewTicker(s.opciones.Intervalo)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.Ciclo(ctx)
		}
	}
}

// Encolar registra la intención de notificar. Devuelve el notificationID y si
// el ítem fue creado (false en un replay de la misma clave).
func (s *Service) Encolar(ctx context.Context, solicitud Solicitud) (string, bool, error) {
	if s == nil || s.store == nil {
		return "", false, errors.New("pushoutbox: store no disponible")
	}
	solicitud.UserID = strings.TrimSpace(solicitud.UserID)
	solicitud.EventID = strings.TrimSpace(solicitud.EventID)
	solicitud.Title = strings.TrimSpace(solicitud.Title)
	solicitud.Message = strings.TrimSpace(solicitud.Message)
	if solicitud.UserID == "" || solicitud.EventID == "" || solicitud.Title == "" || solicitud.Message == "" {
		return "", false, errors.New("pushoutbox: solicitud incompleta")
	}
	notificationID := strings.TrimSpace(solicitud.NotificationID)
	if notificationID == "" {
		suma := sha256.Sum256([]byte(solicitud.UserID + "|" + solicitud.EventID))
		notificationID = hex.EncodeToString(suma[:16])
	}
	ahora := s.opciones.Ahora().UTC()
	due := solicitud.OcurridoEn
	if due.IsZero() {
		due = ahora
	}
	registro := Registro{
		UserID:         solicitud.UserID,
		NotificationID: notificationID,
		Title:          solicitud.Title,
		Message:        solicitud.Message,
		Type:           solicitud.Type,
		EventID:        solicitud.EventID,
		Status:         EstadoPendiente,
		NextAttemptAt:  due.UTC(),
		CreatedAt:      ahora,
		UpdatedAt:      ahora,
	}
	creado, err := s.store.Crear(ctx, registro)
	return notificationID, creado, err
}

// Ciclo hace una pasada y devuelve cuántos mensajes fueron aceptados por FCM.
func (s *Service) Ciclo(ctx context.Context) int {
	if !s.Disponible() {
		return 0
	}
	ahora := s.opciones.Ahora().UTC()
	registros, err := s.store.Pendientes(ctx, ahora, s.opciones.Lote*4)
	if err != nil {
		s.avisar(ctx, "no se pudo leer el outbox push", "clase", "OUTBOX_LECTURA")
		return 0
	}
	aceptados := 0
	for _, registro := range registros {
		if ctx.Err() != nil {
			break
		}
		if registro.Status != EstadoPendiente && registro.Status != EstadoFallido {
			continue
		}
		if registro.NextAttemptAt.After(ahora) {
			continue
		}
		intentos, ok, errReclamo := s.store.Reclamar(ctx, registro.ID, ahora, s.opciones.Lease)
		if errReclamo != nil || !ok {
			continue // otro nodo ganó el lease
		}
		registro.Attempts = intentos
		if s.procesar(ctx, registro, ahora) {
			aceptados++
		}
	}
	return aceptados
}

func (s *Service) procesar(ctx context.Context, registro Registro, ahora time.Time) bool {
	permitido, clase, siguiente, err := s.autorizado(ctx, registro.UserID, ahora)
	if err != nil {
		_ = s.store.Reprogramar(ctx, registro.ID, registro.Attempts, "PREFERENCIAS_NO_DISPONIBLES", ahora.Add(5*time.Minute), ahora)
		return false
	}
	if !permitido {
		if clase == ClaseQuietHours || clase == ClaseZonaInvalida || clase == ClaseHorarioInvalido {
			if siguiente.IsZero() {
				siguiente = ahora.Add(15 * time.Minute)
			}
			_ = s.store.Reprogramar(ctx, registro.ID, registro.Attempts, clase, siguiente, ahora)
		} else {
			_ = s.store.MarcarRevocado(ctx, registro.ID, clase, ahora)
		}
		return false
	}

	if s.tokens == nil {
		_ = s.store.Reprogramar(ctx, registro.ID, registro.Attempts, ClaseTokensNoDisponibles, ahora.Add(5*time.Minute), ahora)
		return false
	}
	tokens, err := s.tokens.TokensActivos(ctx, registro.UserID)
	if err != nil {
		_ = s.store.Reprogramar(ctx, registro.ID, registro.Attempts, ClaseTokensNoDisponibles, ahora.Add(5*time.Minute), ahora)
		return false
	}
	if len(tokens) == 0 {
		_ = s.store.MarcarRevocado(ctx, registro.ID, ClaseSinDispositivos, ahora)
		return false
	}

	entregado, err := s.store.YaEntregado(ctx, registro.UserID, registro.EventID)
	if err != nil {
		_ = s.store.Reprogramar(ctx, registro.ID, registro.Attempts, "IDEMPOTENCIA_NO_DISPONIBLE", ahora.Add(5*time.Minute), ahora)
		return false
	}
	if entregado {
		_ = s.store.MarcarEnviado(ctx, registro.ID, ahora)
		return false
	}

	if s.bandeja == nil {
		_ = s.store.Reprogramar(ctx, registro.ID, registro.Attempts, ClaseBandejaNoDisponible, ahora.Add(5*time.Minute), ahora)
		return false
	}
	if err := s.bandeja.GuardarBandeja(ctx, RegistroBandeja{
		UserID: registro.UserID, ID: registro.NotificationID, Title: registro.Title,
		Message: registro.Message, Type: registro.Type, CreatedAt: ahora,
	}); err != nil {
		_ = s.store.Reprogramar(ctx, registro.ID, registro.Attempts, ClaseBandejaNoDisponible, ahora.Add(5*time.Minute), ahora)
		return false
	}

	huboExito := false
	huboConfig := false
	huboPermanente := false
	huboTransitorio := false
	clasePermanente := ""
	for _, token := range tokens {
		if token.Valor == "" {
			continue
		}
		_, errEnvio := s.emisor.Enviar(ctx, push.Mensaje{
			Token: token.Valor, Title: registro.Title, Body: registro.Message, EventID: registro.EventID,
		})
		if errEnvio == nil {
			huboExito = true
			continue
		}
		switch {
		case push.EsCredencialInvalida(errEnvio) || errors.Is(errEnvio, push.ErrConfiguracion):
			huboConfig = true
		case esErrorPermanente(errEnvio):
			huboPermanente = true
			clasePermanente = claseFalloFCM(errEnvio)
			if revocador, ok := s.tokens.(RevocadorTokens); ok {
				_ = revocador.RevocarTokenPush(ctx, registro.UserID, token.ID)
			}
		default:
			huboTransitorio = true
		}
	}

	if huboExito {
		_ = s.store.MarcarEntregado(ctx, registro.UserID, registro.EventID, registro.NotificationID, ahora)
		if err := s.store.MarcarEnviado(ctx, registro.ID, ahora); err != nil {
			s.avisar(ctx, "no se pudo cerrar el trabajo enviado", "clase", "CIERRE_ENVIO")
		}
		return true
	}
	if huboConfig {
		_ = s.store.Reprogramar(ctx, registro.ID, registro.Attempts, ClaseConfigFCM, ahora.Add(5*time.Minute), ahora)
		return false
	}
	if huboPermanente {
		if clasePermanente == "" {
			clasePermanente = ClaseArgumentoInvalido
		}
		_ = s.store.MarcarRevocado(ctx, registro.ID, clasePermanente, ahora)
		return false
	}
	if huboTransitorio {
		if registro.Attempts >= s.opciones.MaxIntentos {
			_ = s.store.Reprogramar(ctx, registro.ID, registro.Attempts, ClaseReintentable+"_AGOTADO", ahora.Add(6*time.Hour), ahora)
		} else {
			_ = s.store.Reprogramar(ctx, registro.ID, registro.Attempts, ClaseReintentable, ahora.Add(backoff(registro.Attempts)), ahora)
		}
	}
	return false
}

func esErrorPermanente(err error) bool {
	if errors.Is(err, push.ErrPermanente) || errors.Is(err, push.ErrConfiguracion) {
		return true
	}
	var clasificado *push.ErrorClasificado
	if !errors.As(err, &clasificado) {
		return false
	}
	return clasificado.Clase == push.ClaseTokenNoRegistrado || clasificado.Clase == push.ClaseArgumentoInvalido
}

func claseFalloFCM(err error) string {
	var clasificado *push.ErrorClasificado
	if errors.As(err, &clasificado) && clasificado.Clase == push.ClaseArgumentoInvalido {
		return ClaseArgumentoInvalido
	}
	return ClaseTokenNoRegistrado
}

// autorizado aplica E057/E058. Un item ausente nunca se inventa aquí: si el
// servicio de preferencias no está disponible, se bloquea el envío.
func (s *Service) autorizado(ctx context.Context, userID string, ahora time.Time) (bool, string, time.Time, error) {
	if s.preferencias == nil {
		return false, ClaseSinConsentimiento, time.Time{}, nil
	}
	preferencias, err := s.preferencias.Get(ctx, userID)
	if err != nil {
		return false, "", time.Time{}, err
	}
	if preferencias == nil || !preferencias.PushEnabled {
		return false, ClaseSinConsentimiento, time.Time{}, nil
	}
	q := preferencias.QuietHours
	if q == nil || !q.Enabled {
		return true, "", time.Time{}, nil
	}
	zona, err := cargarZonaIANA(preferencias.TimeZone)
	if err != nil {
		return false, ClaseZonaInvalida, time.Time{}, nil
	}
	enVentana, fin, err := enVentanaSilencio(ahora, q, zona)
	if err != nil {
		return false, ClaseHorarioInvalido, time.Time{}, nil
	}
	if enVentana {
		return false, ClaseQuietHours, fin, nil
	}
	return true, "", time.Time{}, nil
}

// cargarZonaIANA valida un nombre IANA con la base embebida (time/tzdata).
// Se replica la comprobación de internal/transport/http para evitar el ciclo
// de importación application -> transport -> application.
func cargarZonaIANA(valor string) (*time.Location, error) {
	limpio := strings.TrimSpace(valor)
	if len(limpio) < 3 || len(limpio) > 64 || limpio == "Local" || strings.ContainsAny(limpio, " \t\r\n") {
		return nil, fmt.Errorf("zona IANA inválida")
	}
	zona, err := time.LoadLocation(limpio)
	if err != nil || zona == nil {
		return nil, fmt.Errorf("zona IANA no disponible")
	}
	return zona, nil
}

// enVentanaSilencio calcula si ahora cae en quietHours y cuándo termina la
// franja. Soporta rangos que cruzan la medianoche.
func enVentanaSilencio(ahora time.Time, q *appPrefs.QuietHours, zona *time.Location) (bool, time.Time, error) {
	if q == nil || !q.Enabled || zona == nil {
		return false, time.Time{}, nil
	}
	inicio, err := time.Parse("15:04", q.Start)
	if err != nil {
		return false, time.Time{}, fmt.Errorf("inicio de quietHours inválido")
	}
	fin, err := time.Parse("15:04", q.End)
	if err != nil {
		return false, time.Time{}, fmt.Errorf("fin de quietHours inválido")
	}
	minInicio := inicio.Hour()*60 + inicio.Minute()
	minFin := fin.Hour()*60 + fin.Minute()
	if minInicio == minFin {
		return false, time.Time{}, fmt.Errorf("rango de quietHours vacío")
	}
	local := ahora.In(zona)
	minActual := local.Hour()*60 + local.Minute()
	medianoche := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, zona)
	finAbsoluto := medianoche.Add(time.Duration(minFin) * time.Minute)
	if minFin < minInicio { // cruza medianoche
		if minActual >= minInicio {
			finAbsoluto = finAbsoluto.AddDate(0, 0, 1)
		}
		return minActual >= minInicio || minActual < minFin, finAbsoluto, nil
	}
	if minActual >= minFin {
		finAbsoluto = finAbsoluto.AddDate(0, 0, 1)
	}
	return minActual >= minInicio && minActual < minFin, finAbsoluto, nil
}

func backoff(intentos int) time.Duration {
	if intentos < 1 {
		intentos = 1
	}
	if intentos > 7 {
		intentos = 7
	}
	return 30 * time.Second * time.Duration(1<<uint(intentos-1))
}

func (s *Service) avisar(ctx context.Context, mensaje string, pares ...any) {
	if s != nil && s.opciones.Logger != nil {
		s.opciones.Logger.WarnContext(ctx, mensaje, pares...)
	}
}
