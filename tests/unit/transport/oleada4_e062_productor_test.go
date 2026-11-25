package transport_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"sync"
	"testing"
	"time"

	appPrefs "github.com/nexumpr/Nexum-Backend/internal/application/notificationprefs"
	appNotifications "github.com/nexumpr/Nexum-Backend/internal/application/notifications"
	"github.com/nexumpr/Nexum-Backend/internal/application/pushoutbox"
	"github.com/nexumpr/Nexum-Backend/internal/infrastructure/push"
)

// e062UUIDPattern es el patrón exacto que exige el handler de E062.
var e062UUIDPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// e062OutboxFake es el mínimo puerto durable del productor; no toca AWS.
type e062OutboxFake struct {
	mu    sync.Mutex
	porID map[string]pushoutbox.Registro
}

func nuevoE062OutboxFake() *e062OutboxFake {
	return &e062OutboxFake{porID: map[string]pushoutbox.Registro{}}
}

func (s *e062OutboxFake) Crear(_ context.Context, registro pushoutbox.Registro) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, existe := s.porID[registro.NotificationID]; existe {
		return false, nil
	}
	registro.ID = registro.NotificationID
	registro.Status = pushoutbox.EstadoPendiente
	s.porID[registro.NotificationID] = registro
	return true, nil
}

func (s *e062OutboxFake) Pendientes(_ context.Context, ahora time.Time, _ int) ([]pushoutbox.Registro, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	salida := make([]pushoutbox.Registro, 0, len(s.porID))
	for _, registro := range s.porID {
		if (registro.Status == pushoutbox.EstadoPendiente || registro.Status == pushoutbox.EstadoFallido) &&
			!registro.NextAttemptAt.After(ahora) {
			salida = append(salida, registro)
		}
	}
	return salida, nil
}

func (s *e062OutboxFake) Reclamar(_ context.Context, id string, ahora time.Time, lease time.Duration) (int, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	registro, existe := s.porID[id]
	if !existe || (registro.Status != pushoutbox.EstadoPendiente && registro.Status != pushoutbox.EstadoFallido) {
		return 0, false, nil
	}
	registro.Status = pushoutbox.EstadoEnviando
	registro.LeaseUntil = ahora.Add(lease)
	registro.Attempts++
	s.porID[id] = registro
	return registro.Attempts, true, nil
}

func (s *e062OutboxFake) MarcarEnviado(_ context.Context, id string, _ time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	registro := s.porID[id]
	registro.Status = pushoutbox.EstadoEnviado
	s.porID[id] = registro
	return nil
}

func (s *e062OutboxFake) MarcarRevocado(_ context.Context, id, clase string, _ time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	registro := s.porID[id]
	registro.Status = pushoutbox.EstadoRevocado
	registro.UltimaClase = clase
	s.porID[id] = registro
	return nil
}

func (s *e062OutboxFake) Reprogramar(_ context.Context, id string, attempts int, clase string, siguiente, _ time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	registro := s.porID[id]
	registro.Status = pushoutbox.EstadoFallido
	registro.Attempts = attempts
	registro.UltimaClase = clase
	registro.NextAttemptAt = siguiente
	s.porID[id] = registro
	return nil
}

func (s *e062OutboxFake) YaEntregado(_ context.Context, _, _ string) (bool, error) { return false, nil }

func (s *e062OutboxFake) MarcarEntregado(_ context.Context, _, _, _ string, _ time.Time) error {
	return nil
}

type e062Prefs struct{}

func (e062Prefs) Get(_ context.Context, _ string) (*appPrefs.Preferences, error) {
	p := appPrefs.Defaults()
	p.PushEnabled = true
	return &p, nil
}

type e062Tokens struct{}

func (e062Tokens) TokensActivos(_ context.Context, _ string) ([]pushoutbox.TokenDestino, error) {
	return []pushoutbox.TokenDestino{{ID: "dev-1", Valor: "token-sintetico"}}, nil
}

type e062Emisor struct{}

func (e062Emisor) Enviar(_ context.Context, _ push.Mensaje) (push.Resultado, error) {
	return push.Resultado{MessageID: "sintetico"}, nil
}

// GuardarBandeja convierte el sink de E061 en el almacén HTTP de pruebas: el ID
// persistido es exactamente el que generó el productor.
func (f *e061HTTPStore) GuardarBandeja(_ context.Context, bandeja pushoutbox.RegistroBandeja) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.porUser[bandeja.UserID] = append(f.porUser[bandeja.UserID], e061HTTPItem{
		n: appNotifications.Notification{
			ID:        bandeja.ID,
			Title:     bandeja.Title,
			Message:   bandeja.Message,
			Type:      appNotifications.Type(bandeja.Type),
			CreatedAt: bandeja.CreatedAt.UTC(),
		},
		sk: "NOTIFICATION#" + bandeja.CreatedAt.UTC().Format(time.RFC3339Nano) + "#" + bandeja.ID,
	})
	return nil
}

// TestE062ProductorUUIDAceptadoPorE061YE062 demuestra el corte completo con dobles:
// el productor genera el ID, E061 lo persiste y E062 lo acepta por HTTP (no 400).
func TestE062ProductorUUIDAceptadoPorE061YE062(t *testing.T) {
	ctx := context.Background()
	reloj := func() time.Time { return time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC) }
	outbox := nuevoE062OutboxFake()
	bandeja := e061NuevoStore()
	productor := pushoutbox.NewService(outbox, e062Emisor{}, e062Prefs{}, e062Tokens{}, bandeja, pushoutbox.Opciones{
		Ahora: reloj, Lease: time.Minute,
	})

	id, creado, err := productor.Encolar(ctx, pushoutbox.Solicitud{
		UserID: e004HTTPUsuario, EventID: "evt-e062-productor",
		Title: "Aviso", Message: "Revisa tu presupuesto", Type: string(appNotifications.TypeBudget),
	})
	if err != nil || !creado {
		t.Fatalf("encolar: id=%q creado=%v err=%v", id, creado, err)
	}
	if !e062UUIDPattern.MatchString(id) {
		t.Fatalf("el ID generado no cumple el patrón de E062: %q", id)
	}
	// D-E062-01: UUID v8 RFC 9562 (SHA-256), no v5 (SHA-1).
	if len(id) != 36 || id[14] != '8' {
		t.Fatalf("el ID debe ser UUID v8 (version 8): %q", id)
	}
	if id[19] != '8' && id[19] != '9' && id[19] != 'a' && id[19] != 'b' {
		t.Fatalf("variante RFC 4122 invalida en el ID: %q", id)
	}
	if productor.Ciclo(ctx) != 1 {
		t.Fatal("el worker no aceptó el envío sintético")
	}

	// E061: la bandeja contiene el mismo ID generado por el productor.
	pagina, err := appNotifications.NewService(bandeja, e061HTTPClave).List(ctx, e004HTTPUsuario, appNotifications.ListParams{Limit: 10})
	if err != nil {
		t.Fatalf("E061: %v", err)
	}
	if len(pagina.Items) != 1 || pagina.Items[0].ID != id {
		t.Fatalf("E061 no persistió el ID del productor: %+v", pagina.Items)
	}

	// E062: el handler acepta el UUID y lo marca como leído.
	req := e061HTTPPeticion(http.MethodPatch, "/api/v1/notifications/"+id+"/read", "jwt-valido")
	req.SetPathValue("notificationId", id)
	rec := httptest.NewRecorder()
	e061HTTPHandler(bandeja, nil, e061HTTPClave).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("E062 debe aceptar el UUID del productor, obtenido %d: %s", rec.Code, rec.Body.String())
	}
	var cuerpo struct {
		ID     string `json:"id"`
		IsRead bool   `json:"isRead"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &cuerpo); err != nil {
		t.Fatal(err)
	}
	if cuerpo.ID != id || !cuerpo.IsRead {
		t.Fatalf("E062 no marcó el item del productor: %+v", cuerpo)
	}
}
