package application_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	appIdentity "github.com/nexumpr/Nexum-Backend/internal/application/identity"
	"github.com/nexumpr/Nexum-Backend/internal/infrastructure/security"
)

// Pruebas de aplicación de E007 (`POST /api/v1/auth/password/change`): validación del
// contrato, verificación de la contraseña vigente, derivación Argon2id real, incremento de la
// época de revocación (A6) y fallo cerrado. La transacción sobre DynamoDB se cubre en
// tests/unit/dynamo.

var e007Ahora = time.Date(2026, 11, 17, 10, 0, 0, 0, time.UTC)

// e007Store es el puerto PasswordChangeStore en memoria de prueba.
type e007Store struct {
	profile    *appIdentity.PasswordChangeProfile
	profileErr error
	applyErr   error
	aplicado   *appIdentity.PasswordChangeApplication
	lecturas   int
}

func (s *e007Store) LoadPasswordChangeProfile(context.Context, string) (*appIdentity.PasswordChangeProfile, error) {
	s.lecturas++
	return s.profile, s.profileErr
}

func (s *e007Store) ApplyPasswordChange(_ context.Context, apply appIdentity.PasswordChangeApplication) error {
	s.aplicado = &apply
	return s.applyErr
}

// e007Hasher es determinista y barato: comprueba el flujo, no Argon2id (cubierto aparte con el
// hasher real).
type e007Hasher struct {
	hashErr   error
	verifyErr error
}

func (h e007Hasher) HashPassword(password string) (string, error) {
	if h.hashErr != nil {
		return "", h.hashErr
	}
	return "hash:" + password, nil
}

func (h e007Hasher) VerifyPassword(password, hash string) (bool, error) {
	if h.verifyErr != nil {
		return false, h.verifyErr
	}
	return hash == "hash:"+password, nil
}

func e007PerfilValido() *appIdentity.PasswordChangeProfile {
	return &appIdentity.PasswordChangeProfile{
		UserID:       "usuario-e007",
		Status:       appIdentity.StatusActive,
		PasswordHash: "hash:OldPassphrase#2026",
		Version:      7,
		HasVersion:   true,
		SessionEpoch: 2,
	}
}

func e007Comando() appIdentity.PasswordChangeCommand {
	return appIdentity.PasswordChangeCommand{
		UserID:          "usuario-e007",
		CurrentPassword: "OldPassphrase#2026",
		NewPassword:     "NewPassphrase#2026",
	}
}

func e007Servicio(store *e007Store, hasher e007Hasher) *appIdentity.PasswordChangeService {
	return appIdentity.NewPasswordChangeService(store, hasher).
		WithClock(func() time.Time { return e007Ahora })
}

// TestE007ValidacionDelContrato cubre las reglas de longitud y obligatoriedad sin tocar el
// almacén: un cuerpo inválido nunca llega a leer ni a escribir.
func TestE007ValidacionDelContrato(t *testing.T) {
	casos := []struct {
		nombre string
		cmd    appIdentity.PasswordChangeCommand
		campo  string
	}{
		{"actual vacia", appIdentity.PasswordChangeCommand{NewPassword: "NewPassphrase#2026"}, "currentPassword"},
		{"actual en blanco", appIdentity.PasswordChangeCommand{CurrentPassword: "   ", NewPassword: "NewPassphrase#2026"}, "currentPassword"},
		{"actual desproporcionada", appIdentity.PasswordChangeCommand{CurrentPassword: strings.Repeat("a", 1025), NewPassword: "NewPassphrase#2026"}, "currentPassword"},
		{"nueva corta", appIdentity.PasswordChangeCommand{CurrentPassword: "OldPassphrase#2026", NewPassword: strings.Repeat("a", 11)}, "newPassword"},
		{"nueva larga", appIdentity.PasswordChangeCommand{CurrentPassword: "OldPassphrase#2026", NewPassword: strings.Repeat("a", 129)}, "newPassword"},
		{"nueva en blanco", appIdentity.PasswordChangeCommand{CurrentPassword: "OldPassphrase#2026", NewPassword: strings.Repeat(" ", 12)}, "newPassword"},
		{"nueva vacia", appIdentity.PasswordChangeCommand{CurrentPassword: "OldPassphrase#2026"}, "newPassword"},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			_, err := appIdentity.ValidatePasswordChangeCommand(caso.cmd)
			var violation *appIdentity.FieldViolation
			if !errors.As(err, &violation) || violation.Field != caso.campo {
				t.Fatalf("se esperaba FieldViolation en %q, obtenido %v", caso.campo, err)
			}
			if !violation.Business {
				t.Fatalf("la violacion debe clasificarse como regla de negocio (422)")
			}
		})
	}

	// Los límites exactos del contrato se aceptan: 12 y 128 runas.
	for _, longitud := range []int{appIdentity.ChangePasswordMinRunes, appIdentity.ChangePasswordMaxRunes} {
		cmd := appIdentity.PasswordChangeCommand{
			UserID:          "usuario-e007",
			CurrentPassword: "OldPassphrase#2026",
			NewPassword:     strings.Repeat("Z", longitud),
		}
		if _, err := appIdentity.ValidatePasswordChangeCommand(cmd); err != nil {
			t.Fatalf("longitud %d debe aceptarse: %v", longitud, err)
		}
	}

	// La contraseña no se normaliza: ni trim ni lowercase.
	validado, err := appIdentity.ValidatePasswordChangeCommand(appIdentity.PasswordChangeCommand{
		UserID:          "  usuario-e007  ",
		CurrentPassword: " OldPassphrase#2026 ",
		NewPassword:     "  NewPassphrase#2026  ",
	})
	if err != nil {
		t.Fatalf("un cuerpo valido no debe rechazarse: %v", err)
	}
	if validado.CurrentPassword != " OldPassphrase#2026 " || validado.NewPassword != "  NewPassphrase#2026  " {
		t.Fatalf("las contrasenas no deben normalizarse")
	}
	if validado.UserID != "usuario-e007" {
		t.Fatalf("el userId debe normalizarse por espacios: %q", validado.UserID)
	}
}

// TestE007ActualIncorrectaRechaza422 comprueba que una contraseña vigente que no verifica no
// modifica nada y devuelve el error que el handler traduce a 422.
func TestE007ActualIncorrectaRechaza422(t *testing.T) {
	store := &e007Store{profile: e007PerfilValido()}
	cmd := e007Comando()
	cmd.CurrentPassword = "OtraPassphrase#2026"

	err := e007Servicio(store, e007Hasher{}).Change(context.Background(), cmd)
	if !errors.Is(err, appIdentity.ErrPasswordChangeActualIncorrecta) {
		t.Fatalf("se esperaba ErrPasswordChangeActualIncorrecta, obtenido %v", err)
	}
	if store.aplicado != nil {
		t.Fatalf("una contrasena actual incorrecta no puede aplicar la transaccion")
	}
}

// TestE007ActualIncorrectaConArgon2idRechaza422 reproduce el comportamiento real del hasher:
// `VerifyPassword` devuelve `ports.ErrInvalidPassword` cuando la contraseña no coincide. Ese
// error es una violación de regla (422), no una caída de dependencia (503).
func TestE007ActualIncorrectaConArgon2idRechaza422(t *testing.T) {
	hasher := security.NewArgon2Hasher(nil)
	hashVigente, err := hasher.HashPassword("OldPassphrase#2026")
	if err != nil {
		t.Fatalf("no se pudo derivar el hash vigente: %v", err)
	}
	store := &e007Store{profile: e007PerfilValido()}
	store.profile.PasswordHash = hashVigente
	cmd := e007Comando()
	cmd.CurrentPassword = "OtraPassphrase#2026"

	err = appIdentity.NewPasswordChangeService(store, hasher).
		WithClock(func() time.Time { return e007Ahora }).
		Change(context.Background(), cmd)
	if !errors.Is(err, appIdentity.ErrPasswordChangeActualIncorrecta) {
		t.Fatalf("se esperaba ErrPasswordChangeActualIncorrecta, obtenido %v", err)
	}
	if store.aplicado != nil {
		t.Fatalf("una contrasena actual incorrecta no puede aplicar la transaccion")
	}
}

// TestE007CambioRealConArgon2id usa el hasher de producción: el valor persistido es Argon2id
// verificable, nunca la contraseña en claro, y la época se incrementa en la misma aplicación.
func TestE007CambioRealConArgon2id(t *testing.T) {
	hasher := security.NewArgon2Hasher(nil)
	hashVigente, err := hasher.HashPassword("OldPassphrase#2026")
	if err != nil {
		t.Fatalf("no se pudo derivar el hash vigente: %v", err)
	}

	store := &e007Store{profile: e007PerfilValido()}
	store.profile.PasswordHash = hashVigente

	servicio := appIdentity.NewPasswordChangeService(store, hasher).
		WithClock(func() time.Time { return e007Ahora })
	if err := servicio.Change(context.Background(), e007Comando()); err != nil {
		t.Fatalf("un cambio valido no debe fallar: %v", err)
	}
	if store.aplicado == nil {
		t.Fatalf("debe aplicarse la transaccion")
	}

	aplicado := *store.aplicado
	if aplicado.UserID != "usuario-e007" {
		t.Fatalf("userId inesperado: %q", aplicado.UserID)
	}
	if !strings.HasPrefix(aplicado.PasswordHash, "$argon2id$") {
		t.Fatalf("el hash persistido debe ser Argon2id, obtenido %q", aplicado.PasswordHash)
	}
	if strings.Contains(aplicado.PasswordHash, "NewPassphrase#2026") {
		t.Fatalf("el hash no puede contener la contrasena en claro")
	}

	// La contraseña nueva verifica contra el hash aplicado; la anterior ya no.
	if ok, err := hasher.VerifyPassword("NewPassphrase#2026", aplicado.PasswordHash); err != nil || !ok {
		t.Fatalf("la contrasena nueva debe verificar contra el hash aplicado (ok=%v err=%v)", ok, err)
	}
	if ok, _ := hasher.VerifyPassword("OldPassphrase#2026", aplicado.PasswordHash); ok {
		t.Fatalf("la contrasena anterior no puede verificar contra el hash nuevo")
	}

	// Época A6: se incrementa exactamente una vez y conserva la versión esperada.
	if aplicado.ExpectedSessionEpoch != 2 || aplicado.NextSessionEpoch != 3 {
		t.Fatalf("epoca esperada 2->3, obtenido %d->%d", aplicado.ExpectedSessionEpoch, aplicado.NextSessionEpoch)
	}
	if !aplicado.HasVersion || aplicado.ExpectedVersion != 7 {
		t.Fatalf("version esperada 7 (has=%v), obtenido %d", aplicado.HasVersion, aplicado.ExpectedVersion)
	}
	if !aplicado.Now.Equal(e007Ahora) {
		t.Fatalf("la marca temporal debe venir del reloj inyectado: %v", aplicado.Now)
	}
}

// TestE007CuentaNoUtilizableRechaza401 agrupa los perfiles que no pueden acreditar titularidad.
func TestE007CuentaNoUtilizableRechaza401(t *testing.T) {
	casos := []struct {
		nombre string
		store  *e007Store
		cmd    appIdentity.PasswordChangeCommand
	}{
		{"perfil ausente", &e007Store{}, e007Comando()},
		{"cuenta deshabilitada", &e007Store{profile: func() *appIdentity.PasswordChangeProfile {
			p := e007PerfilValido()
			p.Status = "DISABLED"
			return p
		}()}, e007Comando()},
		{"cuenta eliminada", &e007Store{profile: func() *appIdentity.PasswordChangeProfile {
			p := e007PerfilValido()
			p.Status = "DELETED"
			return p
		}()}, e007Comando()},
		{"perfil sin hash", &e007Store{profile: func() *appIdentity.PasswordChangeProfile {
			p := e007PerfilValido()
			p.PasswordHash = ""
			return p
		}()}, e007Comando()},
		{"perfil de otro titular", &e007Store{profile: func() *appIdentity.PasswordChangeProfile {
			p := e007PerfilValido()
			p.UserID = "otro-usuario"
			return p
		}()}, e007Comando()},
		{"sin sub verificado", &e007Store{profile: e007PerfilValido()}, func() appIdentity.PasswordChangeCommand {
			c := e007Comando()
			c.UserID = "   "
			return c
		}()},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			err := e007Servicio(caso.store, e007Hasher{}).Change(context.Background(), caso.cmd)
			if !errors.Is(err, appIdentity.ErrPasswordChangeUnauthorized) {
				t.Fatalf("se esperaba ErrPasswordChangeUnauthorized, obtenido %v", err)
			}
			if caso.store.aplicado != nil {
				t.Fatalf("no puede aplicarse la transaccion")
			}
		})
	}
}

// TestE007CASPerdidoEsConflicto cubre la carrera de versión/época: 409 y sin cambios.
func TestE007CASPerdidoEsConflicto(t *testing.T) {
	store := &e007Store{profile: e007PerfilValido(), applyErr: appIdentity.ErrPasswordChangeConflict}

	err := e007Servicio(store, e007Hasher{}).Change(context.Background(), e007Comando())
	if !errors.Is(err, appIdentity.ErrPasswordChangeConflict) {
		t.Fatalf("se esperaba ErrPasswordChangeConflict, obtenido %v", err)
	}
}

// TestE007FallaCerrado comprueba que ningún fallo de dependencia confirma el cambio.
func TestE007FallaCerrado(t *testing.T) {
	casos := []struct {
		nombre string
		store  *e007Store
		hasher e007Hasher
	}{
		{"lectura del perfil caida", &e007Store{profileErr: errors.New("dynamodb inaccesible")}, e007Hasher{}},
		{"escritura caida", &e007Store{profile: e007PerfilValido(), applyErr: errors.New("dynamodb inaccesible")}, e007Hasher{}},
		{"hasher no disponible al verificar", &e007Store{profile: e007PerfilValido()}, e007Hasher{verifyErr: errors.New("argon2 no disponible")}},
		{"hasher no disponible al derivar", &e007Store{profile: e007PerfilValido()}, e007Hasher{hashErr: errors.New("argon2 no disponible")}},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			err := e007Servicio(caso.store, caso.hasher).Change(context.Background(), e007Comando())
			if !errors.Is(err, appIdentity.ErrPasswordChangeUnavailable) {
				t.Fatalf("se esperaba ErrPasswordChangeUnavailable, obtenido %v", err)
			}
		})
	}
}

// TestE007PerfilLegadoSinVersion propaga HasVersion=false para que el adaptador use
// attribute_not_exists(version) e inicialice la versión.
func TestE007PerfilLegadoSinVersion(t *testing.T) {
	store := &e007Store{profile: e007PerfilValido()}
	store.profile.HasVersion = false
	store.profile.Version = 0
	store.profile.SessionEpoch = 0

	if err := e007Servicio(store, e007Hasher{}).Change(context.Background(), e007Comando()); err != nil {
		t.Fatalf("un perfil legado debe poder cambiar su contrasena: %v", err)
	}
	if store.aplicado == nil {
		t.Fatalf("debe aplicarse la transaccion")
	}
	if store.aplicado.HasVersion {
		t.Fatalf("HasVersion debe propagarse como false")
	}
	if store.aplicado.ExpectedSessionEpoch != 0 || store.aplicado.NextSessionEpoch != 1 {
		t.Fatalf("epoca ausente equivale a 0->1, obtenido %d->%d",
			store.aplicado.ExpectedSessionEpoch, store.aplicado.NextSessionEpoch)
	}
}

// TestE007SinDependenciasRespondeNoDisponible cubre la composición incompleta.
func TestE007SinDependenciasRespondeNoDisponible(t *testing.T) {
	servicio := appIdentity.NewPasswordChangeService(nil, nil)
	if servicio.Disponible() {
		t.Fatalf("sin store ni hasher el servicio no esta disponible")
	}
	if err := servicio.Change(context.Background(), e007Comando()); !errors.Is(err, appIdentity.ErrPasswordChangeUnavailable) {
		t.Fatalf("se esperaba ErrPasswordChangeUnavailable, obtenido %v", err)
	}

	// Sin store pero con hasher tampoco opera.
	soloHasher := appIdentity.NewPasswordChangeService(nil, e007Hasher{})
	if soloHasher.Disponible() {
		t.Fatalf("sin store el servicio no esta disponible")
	}
	if err := soloHasher.Change(context.Background(), e007Comando()); !errors.Is(err, appIdentity.ErrPasswordChangeUnavailable) {
		t.Fatalf("se esperaba ErrPasswordChangeUnavailable, obtenido %v", err)
	}
}
