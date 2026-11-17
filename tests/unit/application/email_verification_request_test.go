package application_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	appIdentity "github.com/nexumpr/Nexum-Backend/internal/application/identity"
)

// Pruebas de aplicación de E010: cuota, perfil ACTIVE, rechazo si ya está verificado,
// persistencia con solo el SHA-256, envío SES síncrono y fallo cerrado sin dependencias.

const e010Usuario = "aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee"

var e010Ahora = time.Date(2026, 11, 17, 10, 0, 0, 0, time.UTC)

type e010Store struct {
	profile    *appIdentity.EmailVerificationProfile
	profileErr error
	saveErr    error
	saved      []appIdentity.EmailVerificationToken
}

func (s *e010Store) LoadEmailVerificationProfile(context.Context, string) (*appIdentity.EmailVerificationProfile, error) {
	return s.profile, s.profileErr
}

func (s *e010Store) SaveEmailVerificationToken(_ context.Context, token appIdentity.EmailVerificationToken) error {
	s.saved = append(s.saved, token)
	return s.saveErr
}

type e010Mailer struct {
	err      error
	llamadas int
	destino  string
	token    string
	expiraEn time.Time
}

func (m *e010Mailer) EnviarVerificacion(_ context.Context, destino, token string, expiraEn time.Time) (string, error) {
	m.llamadas++
	m.destino = destino
	m.token = token
	m.expiraEn = expiraEn
	if m.err != nil {
		return "", m.err
	}
	return "mensaje-sintetico", nil
}

type e010Limiter struct {
	allow    bool
	err      error
	llamadas int
	clave    string
}

func (l *e010Limiter) Permitir(_ context.Context, clave string) (bool, error) {
	l.llamadas++
	l.clave = clave
	if l.err != nil {
		return false, l.err
	}
	return l.allow, nil
}

func e010Perfil() *appIdentity.EmailVerificationProfile {
	return &appIdentity.EmailVerificationProfile{
		UserID: e010Usuario,
		Email:  "verificacion@example.invalid",
		Status: appIdentity.StatusActive,
	}
}

func e010Servicio(store *e010Store, correo *e010Mailer, limitador *e010Limiter) *appIdentity.EmailVerificationRequestService {
	var (
		storeI  appIdentity.EmailVerificationStore  = store
		correoI appIdentity.EmailVerificationMailer = correo
	)
	servicio := appIdentity.NewEmailVerificationRequestService(storeI, correoI, limitador, nil)
	return servicio.WithClock(func() time.Time { return e010Ahora })
}

func TestE010SolicitudPersisteSoloElHashYEnvia(t *testing.T) {
	store := &e010Store{profile: e010Perfil()}
	correo := &e010Mailer{}
	limitador := &e010Limiter{allow: true}
	servicio := e010Servicio(store, correo, limitador)

	if err := servicio.Solicitar(context.Background(), e010Usuario); err != nil {
		t.Fatalf("solicitud valida fallo: %v", err)
	}
	if len(store.saved) != 1 {
		t.Fatalf("debe persistirse exactamente un token, obtenido %d", len(store.saved))
	}
	guardado := store.saved[0]
	if guardado.UserID != e010Usuario || guardado.Email != "verificacion@example.invalid" {
		t.Fatalf("el token debe ligarse al usuario y correo del perfil: %+v", guardado)
	}
	if guardado.TokenHash != appIdentity.HashToken(correo.token) {
		t.Fatalf("solo puede persistirse el SHA-256 del token enviado")
	}
	if guardado.TokenHash == correo.token {
		t.Fatalf("el token en claro no puede persistirse")
	}
	if len(guardado.TokenHash) != 64 || strings.Trim(guardado.TokenHash, "0123456789abcdef") != "" {
		t.Fatalf("el hash debe ser sha256 hex de 64 caracteres")
	}
	if len(correo.token) != 43 {
		t.Fatalf("el token debe ser 32 bytes base64url sin padding (43 caracteres), obtenido %d", len(correo.token))
	}
	if strings.ContainsAny(correo.token, "+/=") {
		t.Fatalf("el token debe ser base64url sin padding")
	}
	if _, err := base64.RawURLEncoding.DecodeString(correo.token); err != nil {
		t.Fatalf("el token debe decodificar como base64url: %v", err)
	}
	if guardado.ExpiresAt != e010Ahora.Add(appIdentity.EmailVerificationTokenTTL) {
		t.Fatalf("TTL esperado 30 minutos, obtenido %v", guardado.ExpiresAt.Sub(e010Ahora))
	}
	if correo.expiraEn != guardado.ExpiresAt {
		t.Fatalf("el correo debe anunciar la misma caducidad que el token persistido")
	}
	if correo.llamadas != 1 || correo.destino != "verificacion@example.invalid" {
		t.Fatalf("debe enviarse un unico correo al destinatario del perfil")
	}
	if limitador.llamadas != 1 || limitador.clave != "user:"+e010Usuario {
		t.Fatalf("la cuota debe evaluarse una vez por usuario, obtenido %+v", limitador)
	}
}

func TestE010TokensDistintosPorSolicitud(t *testing.T) {
	store := &e010Store{profile: e010Perfil()}
	correo := &e010Mailer{}
	servicio := e010Servicio(store, correo, &e010Limiter{allow: true})

	if err := servicio.Solicitar(context.Background(), e010Usuario); err != nil {
		t.Fatalf("primera solicitud fallo: %v", err)
	}
	primero := correo.token
	if err := servicio.Solicitar(context.Background(), e010Usuario); err != nil {
		t.Fatalf("segunda solicitud fallo: %v", err)
	}
	if primero == correo.token {
		t.Fatalf("cada solicitud debe generar un token nuevo")
	}
	if store.saved[0].TokenHash == store.saved[1].TokenHash {
		t.Fatalf("los hashes persistidos deben ser distintos")
	}
}

func TestE010YaVerificadoNoGeneraNiEnvia(t *testing.T) {
	perfil := e010Perfil()
	perfil.EmailVerified = true
	store := &e010Store{profile: perfil}
	correo := &e010Mailer{}
	servicio := e010Servicio(store, correo, &e010Limiter{allow: true})

	err := servicio.Solicitar(context.Background(), e010Usuario)
	if !errors.Is(err, appIdentity.ErrEmailVerificationYaVerificado) {
		t.Fatalf("se esperaba ErrEmailVerificationYaVerificado, obtenido %v", err)
	}
	if len(store.saved) != 0 || correo.llamadas != 0 {
		t.Fatalf("un correo ya verificado no genera token ni envia correo")
	}
}

func TestE010FalloDeEnvioNoRespondeExito(t *testing.T) {
	store := &e010Store{profile: e010Perfil()}
	correo := &e010Mailer{err: errors.New("ses rechazo el envio")}
	servicio := e010Servicio(store, correo, &e010Limiter{allow: true})

	err := servicio.Solicitar(context.Background(), e010Usuario)
	if !errors.Is(err, appIdentity.ErrEmailVerificationEnvio) {
		t.Fatalf("se esperaba ErrEmailVerificationEnvio, obtenido %v", err)
	}
	// El item persistido queda sin efecto y caduca por TTL; no se marca nada como verificado.
	if len(store.saved) != 1 {
		t.Fatalf("el token debe quedar persistido con TTL, obtenido %d", len(store.saved))
	}
}

func TestE010PerfilNoApto(t *testing.T) {
	casos := []struct {
		nombre string
		store  *e010Store
		espera error
	}{
		{"perfil ausente", &e010Store{}, appIdentity.ErrEmailVerificationPerfilAusente},
		{"cuenta deshabilitada", func() *e010Store {
			perfil := e010Perfil()
			perfil.Status = "DISABLED"
			return &e010Store{profile: perfil}
		}(), appIdentity.ErrEmailVerificationCuentaInactiva},
		{"perfil sin correo", func() *e010Store {
			perfil := e010Perfil()
			perfil.Email = "  "
			return &e010Store{profile: perfil}
		}(), appIdentity.ErrEmailVerificationDatos},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			correo := &e010Mailer{}
			servicio := e010Servicio(caso.store, correo, &e010Limiter{allow: true})
			err := servicio.Solicitar(context.Background(), e010Usuario)
			if !errors.Is(err, caso.espera) {
				t.Fatalf("se esperaba %v, obtenido %v", caso.espera, err)
			}
			if correo.llamadas != 0 || len(caso.store.saved) != 0 {
				t.Fatalf("un perfil no apto no puede generar ni enviar token")
			}
		})
	}
}

func TestE010CuotaYFalloCerrado(t *testing.T) {
	casos := []struct {
		nombre    string
		limitador *e010Limiter
		espera    error
	}{
		{"cuota agotada", &e010Limiter{allow: false}, appIdentity.ErrEmailVerificationCuota},
		{"redis caido", &e010Limiter{allow: true, err: errors.New("redis caido")}, appIdentity.ErrEmailVerificationNoDisponible},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			store := &e010Store{profile: e010Perfil()}
			correo := &e010Mailer{}
			servicio := e010Servicio(store, correo, caso.limitador)
			err := servicio.Solicitar(context.Background(), e010Usuario)
			if !errors.Is(err, caso.espera) {
				t.Fatalf("se esperaba %v, obtenido %v", caso.espera, err)
			}
			if correo.llamadas != 0 || len(store.saved) != 0 {
				t.Fatalf("sin cuota no se persiste ni se envia nada")
			}
		})
	}
}

func TestE010FalloDePersistenciaFallaCerrado(t *testing.T) {
	store := &e010Store{profile: e010Perfil(), saveErr: errors.New("dynamodb caido")}
	correo := &e010Mailer{}
	servicio := e010Servicio(store, correo, &e010Limiter{allow: true})

	err := servicio.Solicitar(context.Background(), e010Usuario)
	if !errors.Is(err, appIdentity.ErrEmailVerificationNoDisponible) {
		t.Fatalf("se esperaba ErrEmailVerificationNoDisponible, obtenido %v", err)
	}
	if correo.llamadas != 0 {
		t.Fatalf("sin persistencia no puede enviarse un token que nadie podria consumir")
	}

	storeLectura := &e010Store{profileErr: errors.New("dynamodb caido")}
	servicioLectura := e010Servicio(storeLectura, &e010Mailer{}, &e010Limiter{allow: true})
	if err := servicioLectura.Solicitar(context.Background(), e010Usuario); !errors.Is(err, appIdentity.ErrEmailVerificationNoDisponible) {
		t.Fatalf("un fallo de lectura debe fallar cerrado, obtenido %v", err)
	}
}

func TestE010SinDependenciasRespondeNoDisponible(t *testing.T) {
	casos := map[string]*appIdentity.EmailVerificationRequestService{
		"sin store": appIdentity.NewEmailVerificationRequestService(
			nil, &e010Mailer{}, &e010Limiter{allow: true}, nil),
		"sin correo": appIdentity.NewEmailVerificationRequestService(
			&e010Store{profile: e010Perfil()}, nil, &e010Limiter{allow: true}, nil),
		"sin limitador": appIdentity.NewEmailVerificationRequestService(
			&e010Store{profile: e010Perfil()}, &e010Mailer{}, nil, nil),
	}
	for nombre, servicio := range casos {
		t.Run(nombre, func(t *testing.T) {
			if servicio.Disponible() {
				t.Fatalf("sin una dependencia critica el servicio no esta disponible")
			}
			if err := servicio.Solicitar(context.Background(), e010Usuario); !errors.Is(err, appIdentity.ErrEmailVerificationNoDisponible) {
				t.Fatalf("se esperaba ErrEmailVerificationNoDisponible, obtenido %v", err)
			}
		})
	}
}

func TestE010NoRegistraElTokenEnLogs(t *testing.T) {
	var salida bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&salida, nil))

	store := &e010Store{profile: e010Perfil()}
	correo := &e010Mailer{}
	servicio := appIdentity.NewEmailVerificationRequestService(store, correo, &e010Limiter{allow: true}, logger).
		WithClock(func() time.Time { return e010Ahora })

	if err := servicio.Solicitar(context.Background(), e010Usuario); err != nil {
		t.Fatalf("solicitud valida fallo: %v", err)
	}
	if correo.token == "" {
		t.Fatalf("el correo debe haberse enviado")
	}
	registro := salida.String()
	if strings.Contains(registro, correo.token) {
		t.Fatalf("el token en claro no puede aparecer en los logs")
	}
	if strings.Contains(registro, appIdentity.HashToken(correo.token)) {
		t.Fatalf("el digest del token no debe registrarse")
	}
	if strings.Contains(registro, "verificacion@example.invalid") {
		t.Fatalf("el correo del usuario no debe registrarse")
	}
	if !strings.Contains(registro, "verificacion de correo solicitada") {
		t.Fatalf("se esperaba el registro funcional de la solicitud, obtenido %q", registro)
	}
}

func TestE010UsuarioVacioNoConsultaPersistencia(t *testing.T) {
	store := &e010Store{profile: e010Perfil()}
	correo := &e010Mailer{}
	servicio := e010Servicio(store, correo, &e010Limiter{allow: true})
	if err := servicio.Solicitar(context.Background(), "  "); !errors.Is(err, appIdentity.ErrEmailVerificationPerfilAusente) {
		t.Fatalf("sin identidad no puede solicitarse la verificacion, obtenido %v", err)
	}
	if correo.llamadas != 0 || len(store.saved) != 0 {
		t.Fatalf("sin identidad no se persiste ni se envia nada")
	}
}
