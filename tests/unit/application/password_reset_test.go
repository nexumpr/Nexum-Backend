package application_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	appIdentity "github.com/nexumpr/Nexum-Backend/internal/application/identity"
	"github.com/nexumpr/Nexum-Backend/internal/ports"
)

type storeFake struct {
	admisiones []ports.ResetAdmission
	marcados   []string
	trabajos   []ports.ResetTrabajo
	reclamar   bool
	errAdmitir error
}

func (s *storeFake) Admitir(_ context.Context, adm ports.ResetAdmission) error {
	if s.errAdmitir != nil {
		return s.errAdmitir
	}
	s.admisiones = append(s.admisiones, adm)
	return nil
}
func (s *storeFake) MarcarEnviado(_ context.Context, outboxID, _ string, _ time.Time) error {
	s.marcados = append(s.marcados, outboxID)
	return nil
}
func (s *storeFake) Pendientes(_ context.Context, _ time.Time, _ int) ([]ports.ResetTrabajo, error) {
	return s.trabajos, nil
}
func (s *storeFake) Reclamar(_ context.Context, _ string, _ time.Time, _ time.Duration) (bool, error) {
	return s.reclamar, nil
}

type correoFake struct {
	envios []string
	err    error
}

func (c *correoFake) EnviarRecuperacion(_ context.Context, destino, token string, _ time.Time) (string, error) {
	if c.err != nil {
		return "", c.err
	}
	c.envios = append(c.envios, destino+"|"+token)
	return "msg-1", nil
}

type selladorFake struct{ err error }

func (s selladorFake) Sellar(token, _ string) ([]byte, error) {
	if s.err != nil {
		return nil, s.err
	}
	return []byte("sobre:" + token), nil
}
func (s selladorFake) Abrir(sobre []byte, _ string) (string, error) {
	return strings.TrimPrefix(string(sobre), "sobre:"), nil
}

func servicio(store *storeFake, correo *correoFake, sellador ports.Sellador) *appIdentity.PasswordResetService {
	return appIdentity.NewPasswordResetService(store, nil, correo, sellador, nil)
}

func TestE005SinSelladorNoAceptaTrabajo(t *testing.T) {
	s := servicio(&storeFake{}, &correoFake{}, nil)
	if err := s.Solicitar(context.Background(), "a@example.invalid", nil); !errors.Is(err, ports.ErrResetNoDisponible) {
		t.Fatalf("esperado ErrResetNoDisponible, obtenido %v", err)
	}
}

func TestE005CuentaExistentePersisteYEnvia(t *testing.T) {
	store := &storeFake{}
	correo := &correoFake{}
	s := servicio(store, correo, selladorFake{})
	buscar := func(context.Context, string) (string, error) { return "user-1", nil }

	if err := s.Solicitar(context.Background(), "  Usuario@Example.invalid ", buscar); err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if len(store.admisiones) != 1 {
		t.Fatalf("esperada 1 admision, obtenidas %d", len(store.admisiones))
	}
	adm := store.admisiones[0]
	if adm.UserID != "user-1" || adm.Destino != "usuario@example.invalid" {
		t.Fatalf("admision inesperada: %+v", adm)
	}
	if adm.TokenHash == "" || len(adm.TokenHash) != 64 {
		t.Fatalf("hash invalido: %q", adm.TokenHash)
	}
	if len(correo.envios) != 1 || !strings.HasSuffix(correo.envios[0], "|"+adm.TokenHash) {
		// El token enviado nunca coincide con el hash: se comprueba que no se persiste en claro.
		if len(correo.envios) != 1 {
			t.Fatalf("esperado 1 envio, obtenidos %d", len(correo.envios))
		}
	}
	tokenEnviado := strings.SplitN(correo.envios[0], "|", 2)[1]
	if tokenEnviado == adm.TokenHash {
		t.Fatal("el token enviado no puede ser el hash persistido")
	}
	if adm.ExpiraEn.Sub(adm.Ahora) != 30*time.Minute {
		t.Fatalf("TTL inesperado: %v", adm.ExpiraEn.Sub(adm.Ahora))
	}
	if len(store.marcados) != 1 {
		t.Fatalf("esperado marcar enviado, obtenidos %d", len(store.marcados))
	}
}

func TestE005CuentaInexistenteMismaAdmisionSinEnvio(t *testing.T) {
	store := &storeFake{}
	correo := &correoFake{}
	s := servicio(store, correo, selladorFake{})
	buscar := func(context.Context, string) (string, error) { return "", errors.New("no existe") }

	if err := s.Solicitar(context.Background(), "nadie@example.invalid", buscar); err != nil {
		t.Fatalf("la cuenta inexistente no debe devolver error: %v", err)
	}
	if len(store.admisiones) != 1 {
		t.Fatalf("debe admitirse igual: %d", len(store.admisiones))
	}
	if store.admisiones[0].Destino != "" || store.admisiones[0].TokenSello != nil {
		t.Fatal("no debe haber destino ni sobre para una cuenta inexistente")
	}
	if len(correo.envios) != 0 {
		t.Fatal("no debe enviarse nada")
	}
}

func TestE005FalloDeCorreoNoAlteraLaRespuesta(t *testing.T) {
	store := &storeFake{}
	correo := &correoFake{err: errors.New("ses no disponible")}
	s := servicio(store, correo, selladorFake{})
	buscar := func(context.Context, string) (string, error) { return "user-1", nil }

	if err := s.Solicitar(context.Background(), "a@example.invalid", buscar); err != nil {
		t.Fatalf("un fallo de correo no debe propagarse: %v", err)
	}
	if len(store.marcados) != 0 {
		t.Fatal("no debe marcarse como enviado")
	}
}

func TestE005FalloDePersistenciaSePropaga(t *testing.T) {
	store := &storeFake{errAdmitir: errors.New("dynamodb")}
	s := servicio(store, &correoFake{}, selladorFake{})
	if err := s.Solicitar(context.Background(), "a@example.invalid", nil); err == nil {
		t.Fatal("sin persistencia no puede haber 202")
	}
}

func TestE005ReintentoAcotado(t *testing.T) {
	store := &storeFake{
		reclamar: true,
		trabajos: []ports.ResetTrabajo{{
			OutboxID: "ob-1", Destino: "a@example.invalid",
			TokenSello: []byte("sobre:token-1"), ExpiraEn: time.Now().Add(10 * time.Minute),
		}},
	}
	correo := &correoFake{}
	s := servicio(store, correo, selladorFake{})
	if n := s.Reintentar(context.Background(), 5); n != 1 {
		t.Fatalf("esperado 1 envio, obtenido %d", n)
	}
	if len(correo.envios) != 1 || !strings.HasPrefix(correo.envios[0], "a@example.invalid|token-1") {
		t.Fatalf("envio inesperado: %v", correo.envios)
	}
}

func TestE005ValidacionDeEmail(t *testing.T) {
	for _, malo := range []string{"", "sin-arroba", "a@", "@b.c", "a@b", strings.Repeat("a", 250) + "@x.com"} {
		if appIdentity.ValidarEmail(malo) {
			t.Fatalf("deberia rechazar %q", malo)
		}
	}
	if !appIdentity.ValidarEmail("usuario@example.invalid") {
		t.Fatal("deberia aceptar un correo valido")
	}
}
