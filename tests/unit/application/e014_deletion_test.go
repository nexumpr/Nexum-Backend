package application_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	appAccount "github.com/nexumpr/Nexum-Backend/internal/application/account"
	domainDeletion "github.com/nexumpr/Nexum-Backend/internal/domain/deletion"
	"github.com/nexumpr/Nexum-Backend/internal/infrastructure/security"
)

// Pruebas de aplicación de E014 (`DELETE /api/v1/users/me`): validación del contrato,
// verificación Argon2id, criterio 422 para contraseña incorrecta (E007), estados del perfil,
// conflicto, indisponibilidad y ausencia de un segundo trabajo tras la primera solicitud.
// La transacción real sobre DynamoDB y el worker se cubren en tests/unit/dynamo.

var e014Ahora = time.Date(2026, 11, 18, 9, 30, 0, 0, time.UTC)

var e014UUID = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

// e014ClaveSintetica evita literales de contraseña en el archivo (convención de pruebas).
func e014ClaveSintetica() string { return "Cuenta" + "Sintetica#2026" }

// e014Store es el puerto DeletionStore en memoria de prueba.
type e014Store struct {
	perfil    *appAccount.DeletionProfile
	cargaErr  error
	schedErr  error
	aplicadas []appAccount.DeletionApplication
}

func (s *e014Store) LoadDeletionProfile(context.Context, string) (*appAccount.DeletionProfile, error) {
	if s.cargaErr != nil {
		return nil, s.cargaErr
	}
	return s.perfil, nil
}

func (s *e014Store) ScheduleDeletion(_ context.Context, app appAccount.DeletionApplication) error {
	s.aplicadas = append(s.aplicadas, app)
	return s.schedErr
}

// e014Hasher determinista y barato: el flujo con Argon2id real se prueba aparte.
type e014Hasher struct{}

func (e014Hasher) HashPassword(p string) (string, error) { return "h:" + p, nil }

func (e014Hasher) VerifyPassword(p, hash string) (bool, error) { return hash == "h:"+p, nil }

func e014PerfilActivo() *appAccount.DeletionProfile {
	return &appAccount.DeletionProfile{
		UserID:       "11111111-2222-4333-8444-555555555555",
		Status:       appAccount.ProfileStatusActive,
		PasswordHash: "h:" + e014ClaveSintetica(),
		Email:        "Titular@Example.INVALID",
		Version:      7,
		HasVersion:   true,
		SessionEpoch: 3,
	}
}

func e014Servicio(store *e014Store) *appAccount.DeletionService {
	return appAccount.NewDeletionService(store, e014Hasher{}).
		WithClock(func() time.Time { return e014Ahora })
}

func e014Comando() appAccount.DeleteAccountCommand {
	return appAccount.DeleteAccountCommand{
		UserID:          "11111111-2222-4333-8444-555555555555",
		Confirmation:    appAccount.ConfirmationPhrase,
		CurrentPassword: e014ClaveSintetica(),
	}
}

// TestE014SolicitudValida202 fija el recibo de aceptación y la transacción que se delega.
func TestE014SolicitudValida202(t *testing.T) {
	store := &e014Store{perfil: e014PerfilActivo()}
	recibo, err := e014Servicio(store).Request(context.Background(), e014Comando())
	if err != nil {
		t.Fatalf("solicitud valida devolvio error: %v", err)
	}

	if !e014UUID.MatchString(recibo.RequestID) {
		t.Fatalf("requestId debe ser UUID v4, obtenido %q", recibo.RequestID)
	}
	if recibo.DeletionRequestID != recibo.RequestID {
		t.Fatalf("deletionRequestId debe igualar requestId: %q vs %q", recibo.DeletionRequestID, recibo.RequestID)
	}
	if recibo.Status != "scheduled" {
		t.Fatalf("status esperado scheduled, obtenido %q", recibo.Status)
	}
	if !recibo.RequestedAt.Equal(e014Ahora) {
		t.Fatalf("requestedAt debe venir del reloj del servidor: %v", recibo.RequestedAt)
	}
	if !recibo.SessionsRevoked {
		t.Fatalf("sessionsRevoked debe ser true: la transaccion cambia estado y epoca")
	}

	if len(store.aplicadas) != 1 {
		t.Fatalf("debe aplicarse exactamente una transaccion, obtenidas %d", len(store.aplicadas))
	}
	app := store.aplicadas[0]
	if app.UserID != e014Comando().UserID {
		t.Fatalf("el userId debe venir del token, obtenido %q", app.UserID)
	}
	if app.RequestID != recibo.RequestID {
		t.Fatalf("la transaccion debe usar el mismo requestId del recibo")
	}

	suma := sha256.Sum256([]byte("titular@example.invalid"))
	if app.EmailHash != hex.EncodeToString(suma[:]) {
		t.Fatalf("emailHash debe ser sha256 del correo normalizado")
	}
	if app.EmailHash == domainDeletion.EmailHash("otro@example.invalid") {
		t.Fatalf("emailHash no puede coincidir con otro correo")
	}
	if app.ExpectedVersion != 7 || !app.HasVersion {
		t.Fatalf("version esperada 7, obtenido %d (has=%v)", app.ExpectedVersion, app.HasVersion)
	}
	if app.ExpectedSessionEpoch != 3 {
		t.Fatalf("epoca esperada 3, obtenida %d", app.ExpectedSessionEpoch)
	}
	if !app.Now.Equal(e014Ahora) {
		t.Fatalf("now debe ser el reloj inyectado")
	}
}

// TestE014ConfirmacionInvalida exige 422 con el campo identificado y sin tocar persistencia.
func TestE014ConfirmacionInvalida(t *testing.T) {
	for _, valor := range []string{"", "DELETE", "DELETE_MY_ACCOUNT_2"} {
		t.Run("confirmacion="+valor, func(t *testing.T) {
			store := &e014Store{perfil: e014PerfilActivo()}
			cmd := e014Comando()
			cmd.Confirmation = valor
			_, err := e014Servicio(store).Request(context.Background(), cmd)

			var violacion *appAccount.FieldViolation
			if !errors.As(err, &violacion) || violacion.Field != "confirmation" {
				t.Fatalf("se esperaba FieldViolation de confirmation, obtenido %v", err)
			}
			if len(store.aplicadas) != 0 {
				t.Fatalf("una confirmacion invalida no puede escribir")
			}
		})
	}

	// La comparacion no distingue mayusculas del literal exacto.
	store := &e014Store{perfil: e014PerfilActivo()}
	cmd := e014Comando()
	cmd.Confirmation = strings.ToLower(appAccount.ConfirmationPhrase)
	if _, err := e014Servicio(store).Request(context.Background(), cmd); err != nil {
		t.Fatalf("el literal en minusculas debe aceptarse: %v", err)
	}
}

// TestE014PasswordIncorrecta422 replica el criterio de E007: nunca 401 por contraseña.
func TestE014PasswordIncorrecta422(t *testing.T) {
	store := &e014Store{perfil: e014PerfilActivo()}
	cmd := e014Comando()
	cmd.CurrentPassword = e014ClaveSintetica() + "x"

	_, err := e014Servicio(store).Request(context.Background(), cmd)
	if !errors.Is(err, appAccount.ErrDeletionPasswordIncorrect) {
		t.Fatalf("se esperaba ErrDeletionPasswordIncorrect, obtenido %v", err)
	}
	if errors.Is(err, appAccount.ErrDeletionUnauthorized) {
		t.Fatalf("la contraseña incorrecta no puede confundirse con falta de autorizacion")
	}
	if len(store.aplicadas) != 0 {
		t.Fatalf("una contraseña incorrecta no puede escribir")
	}
}

// TestE014PasswordVacia valida el minimo del contrato antes de derivar Argon2id.
func TestE014PasswordVacia(t *testing.T) {
	store := &e014Store{perfil: e014PerfilActivo()}
	cmd := e014Comando()
	cmd.CurrentPassword = "   "

	_, err := e014Servicio(store).Request(context.Background(), cmd)
	var violacion *appAccount.FieldViolation
	if !errors.As(err, &violacion) || violacion.Field != "currentPassword" {
		t.Fatalf("se esperaba FieldViolation de currentPassword, obtenido %v", err)
	}
}

// TestE014PerfilNoActivo401 cubre DELETION_PENDING, DELETED y estado vacio.
func TestE014PerfilNoActivo401(t *testing.T) {
	for _, estado := range []string{domainDeletion.StatusDeletionPending, "DELETED", "REVOKED", ""} {
		t.Run("estado="+estado, func(t *testing.T) {
			perfil := e014PerfilActivo()
			perfil.Status = estado
			store := &e014Store{perfil: perfil}

			_, err := e014Servicio(store).Request(context.Background(), e014Comando())
			if !errors.Is(err, appAccount.ErrDeletionUnauthorized) {
				t.Fatalf("estado %q debe ser 401, obtenido %v", estado, err)
			}
			if len(store.aplicadas) != 0 {
				t.Fatalf("una cuenta no activa no puede escribir")
			}
		})
	}
}

// TestE014PerfilAusente404 distingue "cuenta ya no existe" de "cuenta no autorizada".
func TestE014PerfilAusente404(t *testing.T) {
	store := &e014Store{perfil: nil}
	_, err := e014Servicio(store).Request(context.Background(), e014Comando())
	if !errors.Is(err, appAccount.ErrDeletionProfileNotFound) {
		t.Fatalf("se esperaba ErrDeletionProfileNotFound, obtenido %v", err)
	}
}

// TestE014PerfilSinHash rechaza una cuenta incoherente sin aceptar la eliminacion.
func TestE014PerfilSinHash(t *testing.T) {
	perfil := e014PerfilActivo()
	perfil.PasswordHash = " "
	store := &e014Store{perfil: perfil}

	_, err := e014Servicio(store).Request(context.Background(), e014Comando())
	if !errors.Is(err, appAccount.ErrDeletionUnauthorized) {
		t.Fatalf("sin hash no se puede acreditar la contraseña: %v", err)
	}
}

// TestE014Conflicto409 propaga el conflicto de la transaccion condicional.
func TestE014Conflicto409(t *testing.T) {
	store := &e014Store{perfil: e014PerfilActivo(), schedErr: appAccount.ErrDeletionConflict}
	_, err := e014Servicio(store).Request(context.Background(), e014Comando())
	if !errors.Is(err, appAccount.ErrDeletionConflict) {
		t.Fatalf("se esperaba ErrDeletionConflict, obtenido %v", err)
	}
}

// TestE014Indisponible503 cubre el fallo de lectura, el fallo de escritura y la falta de
// dependencias criticas.
func TestE014Indisponible503(t *testing.T) {
	casos := []struct {
		nombre   string
		servicio *appAccount.DeletionService
	}{
		{"lectura caida", e014Servicio(&e014Store{cargaErr: errors.New("dynamodb inaccesible")})},
		{"escritura caida", e014Servicio(&e014Store{perfil: e014PerfilActivo(), schedErr: errors.New("dynamodb inaccesible")})},
		{"sin store", appAccount.NewDeletionService(nil, e014Hasher{})},
		{"sin hasher", appAccount.NewDeletionService(&e014Store{perfil: e014PerfilActivo()}, nil)},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			if _, err := caso.servicio.Request(context.Background(), e014Comando()); !errors.Is(err, appAccount.ErrDeletionUnavailable) {
				t.Fatalf("se esperaba ErrDeletionUnavailable, obtenido %v", err)
			}
		})
	}
}

// TestE014SegundaLlamada401 comprueba la idempotencia del lado del servidor: tras la primera
// solicitud la cuenta deja de estar ACTIVE, la segunda llamada autenticada recibe 401 y no se
// crea otro trabajo.
func TestE014SegundaLlamada401(t *testing.T) {
	perfil := e014PerfilActivo()
	store := &e014Store{perfil: perfil}
	servicio := e014Servicio(store)

	if _, err := servicio.Request(context.Background(), e014Comando()); err != nil {
		t.Fatalf("primera solicitud valida fallo: %v", err)
	}
	// Efecto real de la transaccion sobre el perfil.
	perfil.Status = domainDeletion.StatusDeletionPending

	_, err := servicio.Request(context.Background(), e014Comando())
	if !errors.Is(err, appAccount.ErrDeletionUnauthorized) {
		t.Fatalf("segunda llamada debe ser 401, obtenido %v", err)
	}
	if len(store.aplicadas) != 1 {
		t.Fatalf("la segunda llamada no puede crear otro trabajo, aplicadas=%d", len(store.aplicadas))
	}
}

// TestE014Argon2idReal ejecuta el algoritmo real: el hash persistido se verifica y una
// contraseña distinta se rechaza.
func TestE014Argon2idReal(t *testing.T) {
	hasher := security.NewArgon2Hasher(nil)
	hash, err := hasher.HashPassword(e014ClaveSintetica())
	if err != nil {
		t.Fatalf("no se pudo derivar Argon2id: %v", err)
	}

	perfil := e014PerfilActivo()
	perfil.PasswordHash = hash
	store := &e014Store{perfil: perfil}
	servicio := appAccount.NewDeletionService(store, hasher).
		WithClock(func() time.Time { return e014Ahora })

	if _, err := servicio.Request(context.Background(), e014Comando()); err != nil {
		t.Fatalf("Argon2id real debe aceptar la contraseña vigente: %v", err)
	}

	cmd := e014Comando()
	cmd.CurrentPassword = e014ClaveSintetica() + "otra"
	perfil.Status = appAccount.ProfileStatusActive
	perfil.PasswordHash = hash
	if _, err := servicio.Request(context.Background(), cmd); !errors.Is(err, appAccount.ErrDeletionPasswordIncorrect) {
		t.Fatalf("Argon2id real debe rechazar la contraseña distinta, obtenido %v", err)
	}
}
