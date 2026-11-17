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
	admisiones     []ports.ResetAdmission
	marcados       []string
	reintentos     []string
	trabajos       []ports.ResetTrabajo
	reclamar       bool
	pendientes     error
	reclamado      string
	owner          string
	fence          int
	marcadoFence   int
	marcadoOwner   string
	rechazarCierre bool
}

func (s *storeFake) Admitir(_ context.Context, adm ports.ResetAdmission) error {
	s.admisiones = append(s.admisiones, adm)
	return nil
}
func (s *storeFake) Pendientes(_ context.Context, _ time.Time, _ int) ([]ports.ResetTrabajo, error) {
	return s.trabajos, s.pendientes
}
func (s *storeFake) Reclamar(_ context.Context, outboxID, owner string, _ time.Time, _ time.Duration) (int, bool, error) {
	s.reclamado = outboxID
	s.owner = owner
	return s.fence, s.reclamar, nil
}
func (s *storeFake) MarcarEnviado(_ context.Context, tr ports.ResetTrabajo, _ string, _ time.Time) error {
	s.marcados = append(s.marcados, tr.OutboxID)
	s.marcadoFence = tr.Fence
	s.marcadoOwner = tr.Owner
	if s.rechazarCierre {
		return ports.ErrResetTokenInvalido
	}
	return nil
}
func (s *storeFake) Reintentar(_ context.Context, tr ports.ResetTrabajo, _ int, _ time.Time, _ time.Time, _ bool) error {
	s.reintentos = append(s.reintentos, tr.OutboxID)
	return nil
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

type selladorFake struct{ aads []string }

func (s *selladorFake) Sellar(token, aad string) ([]byte, error) {
	s.aads = append(s.aads, aad)
	return []byte("sobre:" + token), nil
}
func (s *selladorFake) Abrir(sobre []byte, aad string) (string, error) {
	s.aads = append(s.aads, aad)
	return strings.TrimPrefix(string(sobre), "sobre:"), nil
}

type limitadorFake struct {
	permitir bool
	err      error
}

func (l *limitadorFake) Permitir(context.Context, string) (bool, error) { return l.permitir, l.err }

type arrendadorFake struct{ lider bool }

func (a *arrendadorFake) Adquirir(context.Context, string, time.Duration) (bool, error) {
	return a.lider, nil
}

func servicio(store *storeFake, correo *correoFake, sellador ports.Sellador, lim *limitadorFake) *appIdentity.PasswordResetService {
	return appIdentity.NewPasswordResetService(store, correo, sellador, lim, nil)
}

func buscarSi(context.Context, string) (string, bool, error) { return "user-1", true, nil }
func buscarNo(context.Context, string) (string, bool, error) { return "", false, nil }

func TestE005SinComponentesNoAcepta(t *testing.T) {
	s := appIdentity.NewPasswordResetService(&storeFake{}, nil, nil, nil, nil)
	if err := s.Solicitar(context.Background(), "a@example.invalid", "1.2.3.4", nil); !errors.Is(err, ports.ErrResetNoDisponible) {
		t.Fatalf("esperado ErrResetNoDisponible, obtenido %v", err)
	}
}

func TestE005AdmiteSinEnviarEnLaPeticion(t *testing.T) {
	store := &storeFake{}
	correo := &correoFake{}
	s := servicio(store, correo, &selladorFake{}, &limitadorFake{permitir: true})

	if err := s.Solicitar(context.Background(), "Usuario@Example.invalid", "1.2.3.4", buscarSi); err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if len(correo.envios) != 0 {
		t.Fatal("la peticion no debe enviar correo: lo hace el worker")
	}
	adm := store.admisiones[0]
	if adm.UserID != "user-1" || adm.Destino != "usuario@example.invalid" {
		t.Fatalf("admision inesperada: %+v", adm)
	}
	if adm.ExpiraEn.Sub(adm.Ahora) != 30*time.Minute {
		t.Fatalf("TTL inesperado: %v", adm.ExpiraEn.Sub(adm.Ahora))
	}
	if len(adm.TokenHash) != 64 || len(adm.TokenSello) == 0 {
		t.Fatalf("token mal formado: hash=%q sello=%d", adm.TokenHash, len(adm.TokenSello))
	}
}

func TestE005AADEsUserIDyOutbox(t *testing.T) {
	store := &storeFake{}
	sell := &selladorFake{}
	s := servicio(store, &correoFake{}, sell, &limitadorFake{permitir: true})
	if err := s.Solicitar(context.Background(), "a@example.invalid", "1.2.3.4", buscarSi); err != nil {
		t.Fatal(err)
	}
	adm := store.admisiones[0]
	esperado := adm.UserID + "|" + adm.OutboxID
	if len(sell.aads) != 1 || sell.aads[0] != esperado {
		t.Fatalf("AAD inesperado: %v (esperado %q)", sell.aads, esperado)
	}
}

func TestE005CuentaInexistenteMismaAdmision(t *testing.T) {
	store := &storeFake{}
	s := servicio(store, &correoFake{}, &selladorFake{}, &limitadorFake{permitir: true})
	if err := s.Solicitar(context.Background(), "nadie@example.invalid", "1.2.3.4", buscarNo); err != nil {
		t.Fatal(err)
	}
	if len(store.admisiones) != 1 {
		t.Fatalf("debe admitirse igual: %d", len(store.admisiones))
	}
	if store.admisiones[0].Destino != "" || store.admisiones[0].TokenSello != nil {
		t.Fatal("sin cuenta no debe haber destino ni sobre")
	}
}

func TestE005FalloDeInfraestructuraNoSeConfundeConInexistente(t *testing.T) {
	store := &storeFake{}
	s := servicio(store, &correoFake{}, &selladorFake{}, &limitadorFake{permitir: true})
	fallo := func(context.Context, string) (string, bool, error) { return "", false, errors.New("dynamodb") }
	if err := s.Solicitar(context.Background(), "a@example.invalid", "1.2.3.4", fallo); err == nil {
		t.Fatal("un fallo de DynamoDB no puede aceptarse como cuenta inexistente")
	}
	if len(store.admisiones) != 0 {
		t.Fatal("no debe admitirse trabajo si fallo la busqueda")
	}
}

func TestE005CuotaSeAplicaAntesDeCrearTrabajo(t *testing.T) {
	store := &storeFake{}
	s := servicio(store, &correoFake{}, &selladorFake{}, &limitadorFake{permitir: false})
	if err := s.Solicitar(context.Background(), "a@example.invalid", "1.2.3.4", buscarSi); !errors.Is(err, ports.ErrResetCuota) {
		t.Fatalf("esperado ErrResetCuota, obtenido %v", err)
	}
	if len(store.admisiones) != 0 {
		t.Fatal("no debe crearse trabajo si la cuota esta agotada")
	}
}

func TestE005LimitadorCaidoFallaCerrado(t *testing.T) {
	s := servicio(&storeFake{}, &correoFake{}, &selladorFake{}, &limitadorFake{permitir: true, err: errors.New("redis")})
	if err := s.Solicitar(context.Background(), "a@example.invalid", "1.2.3.4", buscarSi); !errors.Is(err, ports.ErrResetNoDisponible) {
		t.Fatalf("esperado ErrResetNoDisponible, obtenido %v", err)
	}
}

func worker(store *storeFake, correo *correoFake, sell ports.Sellador, ar *arrendadorFake) *appIdentity.ResetWorker {
	return appIdentity.NewResetWorker(store, correo, sell, ar, nil)
}

func TestE005WorkerEnviaTrabajoVencido(t *testing.T) {
	store := &storeFake{
		reclamar: true,
		trabajos: []ports.ResetTrabajo{{
			OutboxID: "ob-1", UserID: "user-1", Destino: "a@example.invalid",
			TokenSello: []byte("sobre:token-1"), ExpiraEn: time.Now().Add(10 * time.Minute),
		}},
	}
	correo := &correoFake{}
	w := worker(store, correo, &selladorFake{}, &arrendadorFake{lider: true})
	if n := w.Ciclo(context.Background()); n != 1 {
		t.Fatalf("esperado 1 envio, obtenido %d", n)
	}
	if len(correo.envios) != 1 || !strings.HasPrefix(correo.envios[0], "a@example.invalid|token-1") {
		t.Fatalf("envio inesperado: %v", correo.envios)
	}
}

func TestE005WorkerSinLiderazgoNoTrabaja(t *testing.T) {
	store := &storeFake{reclamar: true, trabajos: []ports.ResetTrabajo{{OutboxID: "ob-1", ExpiraEn: time.Now().Add(time.Hour)}}}
	correo := &correoFake{}
	w := worker(store, correo, &selladorFake{}, &arrendadorFake{lider: false})
	if n := w.Ciclo(context.Background()); n != 0 {
		t.Fatalf("sin liderazgo no debe enviarse nada: %d", n)
	}
}

func TestE005WorkerDescartaTrabajoExpirado(t *testing.T) {
	store := &storeFake{reclamar: true, trabajos: []ports.ResetTrabajo{{
		OutboxID: "ob-1", UserID: "u", Destino: "a@example.invalid",
		TokenSello: []byte("sobre:x"), ExpiraEn: time.Now().Add(-time.Minute),
	}}}
	correo := &correoFake{}
	w := worker(store, correo, &selladorFake{}, &arrendadorFake{lider: true})
	if n := w.Ciclo(context.Background()); n != 0 || len(correo.envios) != 0 {
		t.Fatal("un trabajo expirado no debe enviarse")
	}
	if len(store.reintentos) != 1 {
		t.Fatal("debe cerrarse el trabajo expirado")
	}
}

func TestE005WorkerReintentaConBackoffAnteFalloDeCorreo(t *testing.T) {
	store := &storeFake{reclamar: true, trabajos: []ports.ResetTrabajo{{
		OutboxID: "ob-1", UserID: "u", Destino: "a@example.invalid",
		TokenSello: []byte("sobre:x"), ExpiraEn: time.Now().Add(10 * time.Minute),
	}}}
	w := worker(store, &correoFake{err: errors.New("ses")}, &selladorFake{}, &arrendadorFake{lider: true})
	if n := w.Ciclo(context.Background()); n != 0 {
		t.Fatalf("no debe contarse como enviado: %d", n)
	}
	if len(store.reintentos) != 1 {
		t.Fatal("debe reprogramarse con backoff")
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

func TestE005FenceViajaConElTrabajo(t *testing.T) {
	store := &storeFake{
		reclamar: true,
		fence:    7,
		trabajos: []ports.ResetTrabajo{{
			OutboxID: "ob-1", UserID: "u", Destino: "a@example.invalid",
			TokenSello: []byte("sobre:tok"), ExpiraEn: time.Now().Add(10 * time.Minute),
		}},
	}
	w := worker(store, &correoFake{}, &selladorFake{}, &arrendadorFake{lider: true})
	if n := w.Ciclo(context.Background()); n != 1 {
		t.Fatalf("esperado 1 envio, obtenido %d", n)
	}
	if store.owner == "" || store.owner == "worker" {
		t.Fatalf("owner debe identificar la instancia, obtenido %q", store.owner)
	}
	if store.marcadoFence != 7 || store.marcadoOwner != store.owner {
		t.Fatalf("fence/owner no viajaron al cierre: fence=%d owner=%q", store.marcadoFence, store.marcadoOwner)
	}
}

func TestE005FencingRechazaCierreConFenceAjeno(t *testing.T) {
	store := &storeFake{
		reclamar: true,
		fence:    9, // el trabajo ya fue reclamado con fence mayor
		trabajos: []ports.ResetTrabajo{{
			OutboxID: "ob-1", UserID: "u", Destino: "a@example.invalid",
			TokenSello: []byte("sobre:tok"), ExpiraEn: time.Now().Add(10 * time.Minute),
		}},
	}
	// El cierre falla porque otro worker reclamo el trabajo entre la reclamacion y el
	// cierre: la condicion por fence no se cumple y el envio no se cuenta.
	store.rechazarCierre = true
	w := worker(store, &correoFake{}, &selladorFake{}, &arrendadorFake{lider: true})
	if n := w.Ciclo(context.Background()); n != 0 {
		t.Fatalf("un fence obsoleto no puede cerrar el trabajo: %d", n)
	}
}
