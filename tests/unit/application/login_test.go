package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	appIdentity "github.com/nexumpr/Nexum-Backend/internal/application/identity"
)

type fakeLoginStore struct {
	identity   *appIdentity.StoredIdentity
	lookupErr  error
	onboarding bool
	onbErr     error
	saved      []appIdentity.SessionRecord
	saveErr    error
}

func (f *fakeLoginStore) LookupIdentity(context.Context, string) (*appIdentity.StoredIdentity, error) {
	return f.identity, f.lookupErr
}

func (f *fakeLoginStore) OnboardingConfigured(context.Context, string) (bool, error) {
	return f.onboarding, f.onbErr
}

func (f *fakeLoginStore) CreateSession(_ context.Context, record appIdentity.SessionRecord) error {
	if f.saveErr != nil {
		return f.saveErr
	}
	f.saved = append(f.saved, record)
	return nil
}

type fakeIssuer struct {
	token string
	jti   string
	err   error
}

func (f *fakeIssuer) IssueAccessToken(context.Context, string, string, time.Time) (string, string, error) {
	return f.token, f.jti, f.err
}

var fixedLoginNow = time.Date(2026, 11, 11, 18, 0, 0, 0, time.UTC)

func newLoginService(store appIdentity.LoginStore, issuer appIdentity.AccessTokenIssuer) *appIdentity.LoginService {
	seq := 0
	return appIdentity.NewLoginService(store, &mockSimpleHasher{}, issuer).
		WithClock(func() time.Time { return fixedLoginNow }).
		WithIDGenerator(func() (string, error) {
			seq++
			if seq == 1 {
				return "018f90ef-08e5-4a68-b09f-0000000000a1", nil
			}
			return "018f90ef-08e5-4a68-b09f-0000000000b2", nil
		}).
		WithRefreshGenerator(func() (string, error) { return "refresh-opaco-de-prueba", nil })
}

func storedIdentity() *appIdentity.StoredIdentity {
	return &appIdentity.StoredIdentity{
		UserID:        "47622cbc-36ec-4b30-9b8a-d9c26833a395",
		Email:         "usuario.prueba@example.com",
		DisplayName:   "Usuario de Prueba Nexum",
		PasswordHash:  "hashed:" + syntheticPhrase,
		EmailVerified: false,
		Status:        "ACTIVE",
	}
}

func TestLoginExitosoDevuelveContratoCompleto(t *testing.T) {
	store := &fakeLoginStore{identity: storedIdentity(), onboarding: true}
	svc := newLoginService(store, &fakeIssuer{token: "jwt-de-prueba", jti: "jti-1"})

	response, err := svc.Login(context.Background(), appIdentity.LoginCommand{
		Email:      "  Usuario.Prueba@Example.com ",
		Password:   syntheticPhrase,
		DeviceID:   "installation-6a4f1d8c",
		DeviceName: "Android",
		Platform:   "android",
	})
	if err != nil {
		t.Fatalf("login fallo: %v", err)
	}

	if response.TokenType != "Bearer" || response.ExpiresIn != 900 {
		t.Fatalf("tokens inesperados: %+v", response)
	}
	if response.SessionID == "" || response.Session.ID != response.SessionID {
		t.Fatalf("session.id debe igualar sessionId: %+v", response)
	}
	if response.User.ID != storedIdentity().UserID || response.User.Name != "Usuario de Prueba Nexum" {
		t.Fatalf("identidad inesperada: %+v", response.User)
	}
	if response.User.EmailVerified {
		t.Fatal("emailVerified debe reflejar el perfil (false)")
	}
	if !response.Onboarding.Configured {
		t.Fatal("onboarding.configured debe reflejar el item real")
	}
	if len(store.saved) != 1 {
		t.Fatalf("se esperaba una sesion persistida, hay %d", len(store.saved))
	}
	record := store.saved[0]
	if record.RefreshHash == "refresh-opaco-de-prueba" || record.RefreshHash == "" {
		t.Fatal("solo debe persistirse el digest del refresh, nunca el token")
	}
	if record.ExpiresAt.Before(fixedLoginNow) {
		t.Fatalf("expiracion invalida: %v", record.ExpiresAt)
	}
	if record.DeviceID == "" || record.Platform != "android" {
		t.Fatalf("metadatos del dispositivo no persistidos: %+v", record)
	}
}

func TestLoginSinOnboardingDevuelveFalse(t *testing.T) {
	store := &fakeLoginStore{identity: storedIdentity(), onboarding: false}
	svc := newLoginService(store, &fakeIssuer{token: "jwt", jti: "jti"})

	response, err := svc.Login(context.Background(), appIdentity.LoginCommand{
		Email: "usuario.prueba@example.com", Password: syntheticPhrase,
	})
	if err != nil {
		t.Fatalf("login fallo: %v", err)
	}
	if response.Onboarding.Configured {
		t.Fatal("un usuario historico sin onboarding debe devolver false, nunca true")
	}
}

func TestLoginCredencialesInvalidas(t *testing.T) {
	casos := []struct {
		nombre string
		store  *fakeLoginStore
		cmd    appIdentity.LoginCommand
	}{
		{"password incorrecta", &fakeLoginStore{identity: storedIdentity()}, appIdentity.LoginCommand{Email: "usuario.prueba@example.com", Password: "OtraPassphrase#2026"}},
		{"usuario inexistente", &fakeLoginStore{identity: nil}, appIdentity.LoginCommand{Email: "nadie@example.com", Password: syntheticPhrase}},
		{"cuenta deshabilitada", &fakeLoginStore{identity: func() *appIdentity.StoredIdentity {
			i := storedIdentity()
			i.Status = "DELETED"
			return i
		}()}, appIdentity.LoginCommand{Email: "usuario.prueba@example.com", Password: syntheticPhrase}},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			svc := newLoginService(caso.store, &fakeIssuer{token: "jwt", jti: "jti"})
			_, err := svc.Login(context.Background(), caso.cmd)
			if !errors.Is(err, appIdentity.ErrInvalidCredentials) {
				t.Fatalf("se esperaba ErrInvalidCredentials, se obtuvo %v", err)
			}
			if len(caso.store.saved) != 0 {
				t.Fatal("no debe crearse sesion ante credenciales invalidas")
			}
		})
	}
}

func TestLoginValidacion(t *testing.T) {
	svc := newLoginService(&fakeLoginStore{identity: storedIdentity()}, &fakeIssuer{token: "jwt", jti: "jti"})

	casos := []struct {
		nombre string
		cmd    appIdentity.LoginCommand
		campo  string
	}{
		{"email invalido", appIdentity.LoginCommand{Email: "sin-arroba", Password: syntheticPhrase}, "email"},
		{"password vacia", appIdentity.LoginCommand{Email: "usuario.prueba@example.com", Password: ""}, "password"},
		{"platform invalida", appIdentity.LoginCommand{Email: "usuario.prueba@example.com", Password: syntheticPhrase, Platform: "ios"}, "platform"},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			_, err := svc.Login(context.Background(), caso.cmd)
			var violation *appIdentity.FieldViolation
			if !errors.As(err, &violation) {
				t.Fatalf("se esperaba FieldViolation, se obtuvo %v", err)
			}
			if violation.Field != caso.campo {
				t.Fatalf("campo esperado %q, obtenido %q", caso.campo, violation.Field)
			}
		})
	}
}

func TestLoginFallaCerradoSinDependencias(t *testing.T) {
	svc := appIdentity.NewLoginService(&fakeLoginStore{identity: storedIdentity()}, &mockSimpleHasher{}, nil)
	_, err := svc.Login(context.Background(), appIdentity.LoginCommand{
		Email: "usuario.prueba@example.com", Password: syntheticPhrase,
	})
	if !errors.Is(err, appIdentity.ErrLoginUnavailable) {
		t.Fatalf("sin emisor de tokens se esperaba ErrLoginUnavailable, se obtuvo %v", err)
	}
}

func TestLoginNoDevuelveSesionSiFallaLaPersistencia(t *testing.T) {
	store := &fakeLoginStore{identity: storedIdentity(), saveErr: errors.New("dynamodb caido")}
	svc := newLoginService(store, &fakeIssuer{token: "jwt", jti: "jti"})

	_, err := svc.Login(context.Background(), appIdentity.LoginCommand{
		Email: "usuario.prueba@example.com", Password: syntheticPhrase,
	})
	if !errors.Is(err, appIdentity.ErrLoginUnavailable) {
		t.Fatalf("se esperaba ErrLoginUnavailable, se obtuvo %v", err)
	}
}
