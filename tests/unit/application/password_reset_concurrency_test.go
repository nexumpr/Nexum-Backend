package application_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	appIdentity "github.com/nexumpr/Nexum-Backend/internal/application/identity"
	"github.com/nexumpr/Nexum-Backend/internal/ports"
)

// colaAtomica modela la semantica de reclamacion de DynamoDB: solo un worker puede
// tomar un trabajo, y el cierre exige owner y fence vigentes. Sirve para probar la
// competencia real entre dos instancias bajo -race.
type colaAtomica struct {
	mu        sync.Mutex
	trabajos  map[string]*ports.ResetTrabajo
	fence     map[string]int
	enviados  int
	cierresOK int
	cierresKO int
}

func nuevaCola(items ...ports.ResetTrabajo) *colaAtomica {
	c := &colaAtomica{trabajos: map[string]*ports.ResetTrabajo{}, fence: map[string]int{}}
	for i := range items {
		it := items[i]
		c.trabajos[it.OutboxID] = &it
	}
	return c
}

func (c *colaAtomica) Admitir(context.Context, ports.ResetAdmission) error { return nil }

func (c *colaAtomica) Pendientes(context.Context, time.Time, int) ([]ports.ResetTrabajo, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]ports.ResetTrabajo, 0, len(c.trabajos))
	for _, t := range c.trabajos {
		out = append(out, *t)
	}
	return out, nil
}

func (c *colaAtomica) Reclamar(_ context.Context, outboxID, owner string, _ time.Time, _ time.Duration) (int, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	t, ok := c.trabajos[outboxID]
	if !ok || t.Owner != "" && t.Owner != owner {
		return 0, false, nil // ya reclamado por otro
	}
	if t.Owner == owner && c.fence[outboxID] > 0 {
		return 0, false, nil // el mismo worker no re-reclama en esta prueba
	}
	c.fence[outboxID]++
	t.Owner = owner
	t.Fence = c.fence[outboxID]
	return t.Fence, true, nil
}

func (c *colaAtomica) MarcarEnviado(_ context.Context, t ports.ResetTrabajo, _ string, _ time.Time) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	actual, ok := c.trabajos[t.OutboxID]
	if !ok || actual.Owner != t.Owner || c.fence[t.OutboxID] != t.Fence {
		c.cierresKO++
		return errors.New("fence u owner obsoleto")
	}
	c.enviados++
	c.cierresOK++
	delete(c.trabajos, t.OutboxID)
	return nil
}

func (c *colaAtomica) Reintentar(context.Context, ports.ResetTrabajo, int, time.Time, time.Time, bool) error {
	return nil
}

type correoLento struct {
	mu     sync.Mutex
	envios int
}

func (c *correoLento) EnviarRecuperacion(context.Context, string, string, time.Time) (string, error) {
	time.Sleep(5 * time.Millisecond) // fuerza solapamiento entre workers
	c.mu.Lock()
	c.envios++
	c.mu.Unlock()
	return "msg", nil
}

func TestE005DosWorkersSoloUnoEnvia(t *testing.T) {
	cola := nuevaCola(ports.ResetTrabajo{
		OutboxID: "ob-1", UserID: "u", Destino: "a@example.invalid",
		TokenSello: []byte("sobre:tok"), ExpiraEn: time.Now().Add(10 * time.Minute),
	})
	correo := &correoLento{}

	workerA := appIdentity.NewResetWorker(cola, correo, &selladorFake{}, &arrendadorFake{lider: true}, nil)
	workerB := appIdentity.NewResetWorker(cola, correo, &selladorFake{}, &arrendadorFake{lider: true}, nil)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); workerA.Ciclo(context.Background()) }()
	go func() { defer wg.Done(); workerB.Ciclo(context.Background()) }()
	wg.Wait()

	if correo.envios != 1 {
		t.Fatalf("solo un worker puede enviar el trabajo, envios=%d", correo.envios)
	}
	if cola.cierresOK != 1 || cola.cierresKO != 0 {
		t.Fatalf("cierres ok=%d ko=%d", cola.cierresOK, cola.cierresKO)
	}
}

func TestE005FenceObsoletoNoCierra(t *testing.T) {
	cola := nuevaCola(ports.ResetTrabajo{
		OutboxID: "ob-2", UserID: "u", Destino: "a@example.invalid",
		TokenSello: []byte("sobre:tok"), ExpiraEn: time.Now().Add(10 * time.Minute),
	})
	// Simula que otro nodo reclamo despues: el fence sube a 2.
	cola.trabajos["ob-2"].Owner = "otro"
	cola.fence["ob-2"] = 2

	w := appIdentity.NewResetWorker(cola, &correoLento{}, &selladorFake{}, &arrendadorFake{lider: true}, nil)
	if n := w.Ciclo(context.Background()); n != 0 {
		t.Fatalf("un trabajo ajeno no debe enviarse: %d", n)
	}
}

func TestE005AtLeastOnceReenviaMismoToken(t *testing.T) {
	// Si SES falla, el trabajo se reprograma y se reenvia el MISMO enlace (no se rota).
	cola := &storeFake{
		reclamar: true,
		trabajos: []ports.ResetTrabajo{{
			OutboxID: "ob-3", UserID: "u", Destino: "a@example.invalid",
			TokenSello: []byte("sobre:token-fijo"), ExpiraEn: time.Now().Add(10 * time.Minute),
		}},
	}
	correo := &correoFake{err: errors.New("ses temporal")}
	w := appIdentity.NewResetWorker(cola, correo, &selladorFake{}, &arrendadorFake{lider: true}, nil)
	if n := w.Ciclo(context.Background()); n != 0 {
		t.Fatalf("no debe contarse como enviado: %d", n)
	}
	if len(cola.reintentos) != 1 {
		t.Fatalf("debe reprogramarse: %v", cola.reintentos)
	}
}
