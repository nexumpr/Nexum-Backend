package pushoutbox

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	appPrefs "github.com/nexumpr/Nexum-Backend/internal/application/notificationprefs"
	"github.com/nexumpr/Nexum-Backend/internal/infrastructure/push"
)

type storeFalso struct {
	mu            sync.Mutex
	porID         map[string]Registro
	entregado     map[string]bool
	reclamos      int
	clases        map[string]string
	proximos      map[string]time.Time
	fallarLectura bool
}

func nuevoStoreFalso() *storeFalso {
	return &storeFalso{
		porID:     make(map[string]Registro),
		entregado: make(map[string]bool),
		clases:    make(map[string]string),
		proximos:  make(map[string]time.Time),
	}
}

func claveFalsa(r Registro) string {
	due := r.NextAttemptAt
	if due.IsZero() {
		due = r.CreatedAt
	}
	return "DUE#" + due.UTC().Format(time.RFC3339Nano) + "#" + r.NotificationID
}

func (s *storeFalso) Crear(_ context.Context, registro Registro) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	clave := claveFalsa(registro)
	if _, existe := s.porID[clave]; existe {
		return false, nil
	}
	registro.ID = clave
	registro.Status = EstadoPendiente
	s.porID[clave] = registro
	return true, nil
}

func (s *storeFalso) Pendientes(_ context.Context, ahora time.Time, _ int) ([]Registro, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fallarLectura {
		return nil, errors.New("lectura no disponible")
	}
	resultado := make([]Registro, 0, len(s.porID))
	for _, registro := range s.porID {
		if (registro.Status == EstadoPendiente || registro.Status == EstadoFallido) &&
			!registro.NextAttemptAt.After(ahora) {
			resultado = append(resultado, registro)
		}
	}
	return resultado, nil
}

func (s *storeFalso) Reclamar(_ context.Context, id string, ahora time.Time, lease time.Duration) (int, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	registro, existe := s.porID[id]
	if !existe || (registro.Status != EstadoPendiente && registro.Status != EstadoFallido) {
		return 0, false, nil
	}
	if registro.LeaseUntil.After(ahora) {
		return 0, false, nil
	}
	registro.Status = EstadoEnviando
	registro.LeaseUntil = ahora.Add(lease)
	registro.Attempts++
	s.porID[id] = registro
	s.reclamos++
	return registro.Attempts, true, nil
}

func (s *storeFalso) MarcarEnviado(_ context.Context, id string, _ time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	registro := s.porID[id]
	registro.Status = EstadoEnviado
	s.porID[id] = registro
	return nil
}

func (s *storeFalso) MarcarRevocado(_ context.Context, id string, clase string, _ time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	registro := s.porID[id]
	registro.Status = EstadoRevocado
	registro.UltimaClase = clase
	s.clases[id] = clase
	s.porID[id] = registro
	return nil
}

func (s *storeFalso) Reprogramar(_ context.Context, id string, attempts int, clase string, siguiente time.Time, _ time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	registro := s.porID[id]
	registro.Status = EstadoFallido
	registro.Attempts = attempts
	registro.UltimaClase = clase
	registro.NextAttemptAt = siguiente
	registro.LeaseUntil = time.Time{}
	s.clases[id] = clase
	s.proximos[id] = siguiente
	s.porID[id] = registro
	return nil
}

func (s *storeFalso) YaEntregado(_ context.Context, userID, eventID string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.entregado[userID+"|"+eventID], nil
}

func (s *storeFalso) MarcarEntregado(_ context.Context, userID, eventID, _ string, _ time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entregado[userID+"|"+eventID] = true
	return nil
}

func (s *storeFalso) estado(notificationID string) Registro {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, registro := range s.porID {
		if registro.NotificationID == notificationID {
			return registro
		}
	}
	return Registro{}
}

func (s *storeFalso) totalReclamos() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.reclamos
}

type emisorFalso struct {
	mu       sync.Mutex
	fn       func(push.Mensaje) (push.Resultado, error)
	mensajes []push.Mensaje
}

func (e *emisorFalso) Enviar(_ context.Context, mensaje push.Mensaje) (push.Resultado, error) {
	e.mu.Lock()
	e.mensajes = append(e.mensajes, mensaje)
	fn := e.fn
	e.mu.Unlock()
	if fn == nil {
		return push.Resultado{MessageID: "sintetico"}, nil
	}
	return fn(mensaje)
}

func (e *emisorFalso) total() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.mensajes)
}

type prefsFalsas struct {
	valor    *appPrefs.Preferences
	err      error
	llamadas int
}

func (p *prefsFalsas) Get(_ context.Context, _ string) (*appPrefs.Preferences, error) {
	p.llamadas++
	return p.valor, p.err
}

type tokensFalsos struct {
	mu         sync.Mutex
	porUsuario map[string][]TokenDestino
	revocados  []string
	faltan     bool
}

func (t *tokensFalsos) TokensActivos(_ context.Context, userID string) ([]TokenDestino, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.faltan {
		return nil, errors.New("store de tokens no disponible")
	}
	return append([]TokenDestino(nil), t.porUsuario[userID]...), nil
}

func (t *tokensFalsos) RevocarTokenPush(_ context.Context, userID, tokenID string) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.revocados = append(t.revocados, tokenID)
	activos := t.porUsuario[userID][:0]
	for _, token := range t.porUsuario[userID] {
		if token.ID != tokenID {
			activos = append(activos, token)
		}
	}
	t.porUsuario[userID] = activos
	return nil
}

func (t *tokensFalsos) totalRevocados() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.revocados)
}

type bandejaFalsa struct {
	mu        sync.Mutex
	guardados []RegistroBandeja
}

func (b *bandejaFalsa) GuardarBandeja(_ context.Context, registro RegistroBandeja) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.guardados = append(b.guardados, registro)
	return nil
}

func (b *bandejaFalsa) total() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.guardados)
}

var ahoraPrueba = time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)

func nuevoServicio(t *testing.T, store Store, emisor push.Emisor, prefs PreferenceReader, tokens TokenSource, bandeja NotificationSink) *Service {
	t.Helper()
	return NewService(store, emisor, prefs, tokens, bandeja, Opciones{
		Ahora: func() time.Time { return ahoraPrueba },
		Lease: time.Minute,
	})
}

func encolarPrueba(t *testing.T, servicio *Service, userID, eventID string) string {
	t.Helper()
	id, creado, err := servicio.Encolar(context.Background(), Solicitud{
		UserID: userID, EventID: eventID, Title: "Aviso", Message: "Revisa tu presupuesto", Type: "REMINDER",
	})
	if err != nil || !creado {
		t.Fatalf("encolar: id=%q creado=%v err=%v", id, creado, err)
	}
	return id
}

func preferencias(pushEnabled bool, zona string, quiet *appPrefs.QuietHours) *appPrefs.Preferences {
	p := appPrefs.Defaults()
	p.PushEnabled = pushEnabled
	p.TimeZone = zona
	p.QuietHours = quiet
	return &p
}

func TestPUSHOptOutBloqueaEnvio(t *testing.T) {
	store := nuevoStoreFalso()
	emisor := &emisorFalso{}
	prefs := &prefsFalsas{valor: preferencias(false, "", nil)}
	tokens := &tokensFalsos{porUsuario: map[string][]TokenDestino{"u1": {{ID: "d1", Valor: "token"}}}}
	bandeja := &bandejaFalsa{}
	servicio := nuevoServicio(t, store, emisor, prefs, tokens, bandeja)
	id := encolarPrueba(t, servicio, "u1", "evt-1")

	if aceptados := servicio.Ciclo(context.Background()); aceptados != 0 {
		t.Fatalf("no debió aceptar envíos: %d", aceptados)
	}
	if emisor.total() != 0 || bandeja.total() != 0 {
		t.Fatalf("opt-out envió o persistió bandeja: emisor=%d bandeja=%d", emisor.total(), bandeja.total())
	}
	if estado := store.estado(id); estado.Status != EstadoRevocado || estado.UltimaClase != ClaseSinConsentimiento {
		t.Fatalf("estado incorrecto tras opt-out: %+v", estado)
	}
}

func TestPUSHSinServicioDePreferenciasBloqueaEnvio(t *testing.T) {
	store := nuevoStoreFalso()
	emisor := &emisorFalso{}
	tokens := &tokensFalsos{porUsuario: map[string][]TokenDestino{"u1": {{ID: "d1", Valor: "token"}}}}
	servicio := nuevoServicio(t, store, emisor, nil, tokens, &bandejaFalsa{})
	id := encolarPrueba(t, servicio, "u1", "evt-sin-prefs")

	servicio.Ciclo(context.Background())
	if emisor.total() != 0 {
		t.Fatal("sin preferencias no debe enviar")
	}
	if estado := store.estado(id); estado.Status != EstadoRevocado || estado.UltimaClase != ClaseSinConsentimiento {
		t.Fatalf("estado incorrecto: %+v", estado)
	}
}

func TestPUSHQuietHoursBloqueaYReprograma(t *testing.T) {
	store := nuevoStoreFalso()
	emisor := &emisorFalso{}
	prefs := &prefsFalsas{valor: preferencias(true, "America/Mexico_City", &appPrefs.QuietHours{
		Enabled: true, Start: "22:00", End: "06:00",
	})}
	tokens := &tokensFalsos{porUsuario: map[string][]TokenDestino{"u1": {{ID: "d1", Valor: "token"}}}}
	servicio := nuevoServicio(t, store, emisor, prefs, tokens, &bandejaFalsa{})
	id := encolarPrueba(t, servicio, "u1", "evt-quiet")

	// 23:30 en Ciudad de México (UTC-6 en noviembre) => dentro de la franja.
	servicio.opciones.Ahora = func() time.Time {
		return time.Date(2026, 11, 23, 5, 30, 0, 0, time.UTC)
	}
	servicio.Ciclo(context.Background())
	if emisor.total() != 0 {
		t.Fatal("quietHours no debió permitir el envío")
	}
	estado := store.estado(id)
	if estado.Status != EstadoFallido || estado.UltimaClase != ClaseQuietHours {
		t.Fatalf("estado incorrecto: %+v", estado)
	}
	zona, err := time.LoadLocation("America/Mexico_City")
	if err != nil {
		t.Fatal(err)
	}
	fin := time.Date(2026, 11, 23, 6, 0, 0, 0, zona)
	if !estado.NextAttemptAt.Equal(fin) {
		t.Fatalf("reprogramación incorrecta: %v != %v", estado.NextAttemptAt, fin)
	}
}

func TestPUSHZonaIANAInvalidaBloqueaEnvio(t *testing.T) {
	store := nuevoStoreFalso()
	emisor := &emisorFalso{}
	prefs := &prefsFalsas{valor: preferencias(true, "Planeta/Marte", &appPrefs.QuietHours{
		Enabled: true, Start: "22:00", End: "06:00",
	})}
	tokens := &tokensFalsos{porUsuario: map[string][]TokenDestino{"u1": {{ID: "d1", Valor: "token"}}}}
	servicio := nuevoServicio(t, store, emisor, prefs, tokens, &bandejaFalsa{})
	id := encolarPrueba(t, servicio, "u1", "evt-zona")

	servicio.Ciclo(context.Background())
	if emisor.total() != 0 {
		t.Fatal("zona inválida no debió permitir el envío")
	}
	if estado := store.estado(id); estado.UltimaClase != ClaseZonaInvalida {
		t.Fatalf("clase incorrecta: %+v", estado)
	}
}

func TestPUSHTokenRevocadoNoSeReutiliza(t *testing.T) {
	store := nuevoStoreFalso()
	emisor := &emisorFalso{fn: func(push.Mensaje) (push.Resultado, error) {
		return push.Resultado{}, &push.ErrorClasificado{Clase: push.ClaseDestinoNoRegistrado, CodigoHTTP: 404}
	}}
	prefs := &prefsFalsas{valor: preferencias(true, "", nil)}
	tokens := &tokensFalsos{porUsuario: map[string][]TokenDestino{"u1": {{ID: "d1", Valor: "token-invalido"}}}}
	bandeja := &bandejaFalsa{}
	servicio := nuevoServicio(t, store, emisor, prefs, tokens, bandeja)
	id := encolarPrueba(t, servicio, "u1", "evt-revocado")

	servicio.Ciclo(context.Background())
	servicio.Ciclo(context.Background())
	if emisor.total() != 1 {
		t.Fatalf("el token revocado se reutilizó: %d envíos", emisor.total())
	}
	if tokens.totalRevocados() != 1 {
		t.Fatalf("no se dio de baja el dispositivo: %d", tokens.totalRevocados())
	}
	if estado := store.estado(id); estado.Status != EstadoRevocado || estado.UltimaClase != ClaseDestinoNoRegistrado {
		t.Fatalf("estado incorrecto: %+v", estado)
	}
	if bandeja.total() != 1 {
		t.Fatalf("la bandeja debió persistirse una vez: %d", bandeja.total())
	}
}

func TestPUSHDosConsumidoresConLeaseNoDuplican(t *testing.T) {
	store := nuevoStoreFalso()
	var totalEnvios atomic.Int32
	emisor := &emisorFalso{fn: func(push.Mensaje) (push.Resultado, error) {
		totalEnvios.Add(1)
		time.Sleep(5 * time.Millisecond)
		return push.Resultado{MessageID: "ok"}, nil
	}}
	prefs := &prefsFalsas{valor: preferencias(true, "", nil)}
	tokens := &tokensFalsos{porUsuario: map[string][]TokenDestino{"u1": {{ID: "d1", Valor: "token"}}}}
	bandeja := &bandejaFalsa{}
	servicio := nuevoServicio(t, store, emisor, prefs, tokens, bandeja)
	id := encolarPrueba(t, servicio, "u1", "evt-lease")

	var wg sync.WaitGroup
	wg.Add(2)
	for i := 0; i < 2; i++ {
		go func() { defer wg.Done(); servicio.Ciclo(context.Background()) }()
	}
	wg.Wait()
	if totalEnvios.Load() != 1 {
		t.Fatalf("doble envío por carrera de consumidores: %d", totalEnvios.Load())
	}
	if store.totalReclamos() != 1 {
		t.Fatalf("más de un reclamo exitoso: %d", store.totalReclamos())
	}
	if estado := store.estado(id); estado.Status != EstadoEnviado {
		t.Fatalf("estado final incorrecto: %+v", estado)
	}
}

func TestPUSHIdempotenciaPorEventIdYUserID(t *testing.T) {
	store := nuevoStoreFalso()
	emisor := &emisorFalso{}
	prefs := &prefsFalsas{valor: preferencias(true, "", nil)}
	tokens := &tokensFalsos{porUsuario: map[string][]TokenDestino{"u1": {{ID: "d1", Valor: "token"}}}}
	bandeja := &bandejaFalsa{}
	servicio := nuevoServicio(t, store, emisor, prefs, tokens, bandeja)
	primero := encolarPrueba(t, servicio, "u1", "evento-unico")
	idSegundo, creadoSegundo, err := servicio.Encolar(context.Background(), Solicitud{
		UserID: "u1", EventID: "evento-unico", Title: "Otro título", Message: "Otro mensaje",
	})
	if err != nil {
		t.Fatal(err)
	}
	// El notificationID es determinista para (userID, eventID): un replay no crea
	// un segundo trabajo ni un segundo envío.
	if primero != idSegundo || creadoSegundo {
		t.Fatalf("replay no idempotente: id=%q vs %q creado=%v", primero, idSegundo, creadoSegundo)
	}

	servicio.Ciclo(context.Background())
	if emisor.total() != 1 {
		t.Fatalf("eventId duplicado generó %d envíos", emisor.total())
	}
	if bandeja.total() != 1 {
		t.Fatalf("bandeja duplicada: %d", bandeja.total())
	}
	if estado := store.estado(primero); estado.Status != EstadoEnviado {
		t.Fatalf("trabajo %s no cerrado: %+v", primero, estado)
	}
}

func TestPUSHCredencialFCMInvalidaNoMarcaEnviado(t *testing.T) {
	store := nuevoStoreFalso()
	emisor := &emisorFalso{fn: func(push.Mensaje) (push.Resultado, error) {
		return push.Resultado{}, push.ErrCredencial
	}}
	prefs := &prefsFalsas{valor: preferencias(true, "", nil)}
	tokens := &tokensFalsos{porUsuario: map[string][]TokenDestino{"u1": {{ID: "d1", Valor: "token"}}}}
	servicio := nuevoServicio(t, store, emisor, prefs, tokens, &bandejaFalsa{})
	id := encolarPrueba(t, servicio, "u1", "evt-config")

	if aceptados := servicio.Ciclo(context.Background()); aceptados != 0 {
		t.Fatalf("config inválida no debe contar envío: %d", aceptados)
	}
	estado := store.estado(id)
	if estado.Status != EstadoFallido || estado.UltimaClase != ClaseConfigFCM {
		t.Fatalf("estado incorrecto: %+v", estado)
	}
}

func TestPUSHReintentoTransitorioReprograma(t *testing.T) {
	store := nuevoStoreFalso()
	llamada := 0
	emisor := &emisorFalso{fn: func(push.Mensaje) (push.Resultado, error) {
		llamada++
		if llamada == 1 {
			return push.Resultado{}, &push.ErrorClasificado{Clase: push.ClaseErrorServidor, CodigoHTTP: 500}
		}
		return push.Resultado{MessageID: "ok"}, nil
	}}
	prefs := &prefsFalsas{valor: preferencias(true, "", nil)}
	tokens := &tokensFalsos{porUsuario: map[string][]TokenDestino{"u1": {{ID: "d1", Valor: "token"}}}}
	servicio := nuevoServicio(t, store, emisor, prefs, tokens, &bandejaFalsa{})
	id := encolarPrueba(t, servicio, "u1", "evt-retry")

	servicio.Ciclo(context.Background())
	estado := store.estado(id)
	if estado.Status != EstadoFallido || estado.UltimaClase != ClaseReintentable || estado.Attempts != 1 {
		t.Fatalf("reprogramación incorrecta: %+v", estado)
	}
	reloj := ahoraPrueba.Add(time.Hour)
	servicio.opciones.Ahora = func() time.Time { return reloj }
	servicio.Ciclo(context.Background())
	if estado := store.estado(id); estado.Status != EstadoEnviado {
		t.Fatalf("no se envió tras la reprogramación: %+v", estado)
	}
	if emisor.total() != 2 {
		t.Fatalf("llamadas al emisor: %d", emisor.total())
	}
}

func credencialSintetica(t *testing.T) []byte {
	t.Helper()
	clave, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(clave)
	if err != nil {
		t.Fatal(err)
	}
	cred := map[string]string{
		"type": "service_account", "project_id": "nexum-dev-sintetico",
		"private_key":  string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})),
		"client_email": "emisor@nexum-dev-sintetico.iam.gserviceaccount.com",
	}
	crudo, err := json.Marshal(cred)
	if err != nil {
		t.Fatal(err)
	}
	return crudo
}

func TestPUSHWorkerConFCMHTTPV1YHTTPTest(t *testing.T) {
	var oauthLlamadas, fcmLlamadas atomic.Int32
	oauth := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		oauthLlamadas.Add(1)
		_, _ = w.Write([]byte(`{"access_token":"token-sintetico","expires_in":3600}`))
	}))
	defer oauth.Close()
	fcm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fcmLlamadas.Add(1)
		var cuerpo struct {
			Message struct {
				Token        string                       `json:"token"`
				Notification struct{ Title, Body string } `json:"notification"`
				Data         map[string]string            `json:"data"`
			} `json:"message"`
		}
		if err := json.NewDecoder(r.Body).Decode(&cuerpo); err != nil {
			t.Error(err)
		}
		if cuerpo.Message.Token != "token-real-sintetico" || cuerpo.Message.Data["eventId"] != "evt-e2e-local" {
			t.Errorf("body FCM inesperado: %+v", cuerpo)
		}
		if r.Header.Get("Authorization") != "Bearer token-sintetico" {
			t.Error("falta Authorization Bearer")
		}
		_, _ = w.Write([]byte(`{"name":"projects/nexum-dev-sintetico/messages/1"}`))
	}))
	defer fcm.Close()

	cliente, err := push.NewClient(push.Config{
		ProjectID: "nexum-dev-sintetico", CredentialsJSON: credencialSintetica(t),
		Endpoint: fcm.URL, OAuthTokenURL: oauth.URL, HTTPClient: &http.Client{Timeout: 2 * time.Second},
	})
	if err != nil {
		t.Fatal(err)
	}
	store := nuevoStoreFalso()
	prefs := &prefsFalsas{valor: preferencias(true, "", nil)}
	tokens := &tokensFalsos{porUsuario: map[string][]TokenDestino{"u1": {{ID: "d1", Valor: "token-real-sintetico"}}}}
	bandeja := &bandejaFalsa{}
	servicio := nuevoServicio(t, store, cliente, prefs, tokens, bandeja)
	id := encolarPrueba(t, servicio, "u1", "evt-e2e-local")

	if aceptados := servicio.Ciclo(context.Background()); aceptados != 1 {
		t.Fatalf("aceptados=%d", aceptados)
	}
	if fcmLlamadas.Load() != 1 || oauthLlamadas.Load() != 1 {
		t.Fatalf("llamadas OAuth=%d FCM=%d", oauthLlamadas.Load(), fcmLlamadas.Load())
	}
	if bandeja.total() != 1 {
		t.Fatalf("bandeja=%d", bandeja.total())
	}
	if estado := store.estado(id); estado.Status != EstadoEnviado {
		t.Fatalf("estado=%+v", estado)
	}
}

// Comprobación adicional de la firma JWT usada por el worker: el assertion
// debe verificar con la clave pública sintética.
func TestPUSHAssertionFirmadoConClavePublica(t *testing.T) {
	clave, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(clave)
	if err != nil {
		t.Fatal(err)
	}
	cred, err := json.Marshal(map[string]string{
		"type": "service_account", "project_id": "nexum-dev-sintetico",
		"private_key":  string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})),
		"client_email": "emisor@nexum-dev-sintetico.iam.gserviceaccount.com",
	})
	if err != nil {
		t.Fatal(err)
	}
	var assertion string
	oauth := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Error(err)
		}
		assertion = r.Form.Get("assertion")
		_, _ = w.Write([]byte(`{"access_token":"t","expires_in":3600}`))
	}))
	defer oauth.Close()
	fcm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer fcm.Close()
	cliente, err := push.NewClient(push.Config{
		ProjectID: "nexum-dev-sintetico", CredentialsJSON: cred,
		Endpoint: fcm.URL, OAuthTokenURL: oauth.URL,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cliente.Enviar(context.Background(), push.Mensaje{Token: "t", Title: "x", Body: "y", EventID: "e"}); err != nil {
		t.Fatal(err)
	}
	partes := strings.Split(assertion, ".")
	if len(partes) != 3 {
		t.Fatalf("assertion con %d partes", len(partes))
	}
	firma, err := base64.RawURLEncoding.DecodeString(partes[2])
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256([]byte(partes[0] + "." + partes[1]))
	if err := rsa.VerifyPKCS1v15(&clave.PublicKey, crypto.SHA256, hash[:], firma); err != nil {
		t.Fatal(fmt.Errorf("firma inválida: %w", err))
	}
}

// TestPUSHNotificationIDCumplePatronE062 fija el contrato: E062 exige UUID con
// guiones. La forma hex de 32 caracteres que se generaba antes era rechazada con
// 400 invalid_uuid. No existen datos productivos con la forma antigua porque no
// hay productor en producción; el patrón se prueba sobre el valor generado.
func TestPUSHNotificationIDCumplePatronE062(t *testing.T) {
	patron := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-8[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

	store := nuevoStoreFalso()
	servicio := nuevoServicio(t, store, &emisorFalso{}, &prefsFalsas{valor: preferencias(true, "", nil)},
		&tokensFalsos{}, &bandejaFalsa{})
	id, creado, err := servicio.Encolar(context.Background(), Solicitud{
		UserID: "u1", EventID: "evt-uuid", Title: "Aviso", Message: "Mensaje",
	})
	if err != nil || !creado {
		t.Fatalf("encolar: id=%q creado=%v err=%v", id, creado, err)
	}
	if !patron.MatchString(id) {
		t.Fatalf("notificationID no es UUID v8 valido para E062: %q", id)
	}
	if len(id) != 36 {
		t.Fatalf("longitud inesperada del UUID: %d", len(id))
	}
	if id[14] != '8' {
		t.Fatalf("la version debe ser 8 (RFC 9562), obtenido %q en %q", id[14], id)
	}
	if !strings.ContainsRune("89ab", rune(id[19])) {
		t.Fatalf("la variante debe ser RFC 4122 (8|9|a|b), obtenido %q en %q", id[19], id)
	}
	if strings.Contains(id, "u1") {
		t.Fatalf("el ID no debe exponer el userID: %q", id)
	}

	// Determinista para el mismo (userID, eventID): se repite el cálculo interno
	// para no depender de que el store acepte un segundo Crear.
	if otra := notificationUUID("u1", "evt-uuid"); otra != id {
		t.Fatalf("el UUID debe ser determinista: %q vs %q", id, otra)
	}
	if otra := notificationUUID("u1", "evt-otro"); otra == id {
		t.Fatalf("otro evento no puede compartir UUID: %q", otra)
	}
	if otra := notificationUUID("u2", "evt-uuid"); otra == id {
		t.Fatalf("otro usuario no puede compartir UUID: %q", otra)
	}
	if recortado := notificationUUID(" u1 ", " evt-uuid "); recortado != id {
		t.Fatalf("los espacios no deben cambiar el UUID: %q", recortado)
	}
}

// TestPUSHNotificationIDNoEsSHA1 demuestra de forma estructural que el ID no es
// un UUID v5 (SHA-1): la version es 8 y la variante es RFC 4122. Un v5 llevaria
// 5 en el nibble de version y habria requerido SHA-1, que gosec marca G401/G505.
func TestPUSHNotificationIDNoEsSHA1(t *testing.T) {
	id := notificationUUID("u1", "evt-sha1")
	if len(id) != 36 {
		t.Fatalf("longitud inesperada: %q", id)
	}
	if id[14] == '5' {
		t.Fatalf("el ID no puede ser UUID v5 (SHA-1): %q", id)
	}
	if id[14] != '8' {
		t.Fatalf("el ID debe ser UUID v8: %q", id)
	}
	if !strings.ContainsRune("89ab", rune(id[19])) {
		t.Fatalf("variante RFC 4122 invalida: %q", id)
	}
	v5 := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-5[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	if v5.MatchString(id) {
		t.Fatalf("el ID coincide con el patron de UUID v5: %q", id)
	}
}

// El worker debe operar aunque falte el emisor FCM: un trabajo sin dispositivos se
// cierra como revocado (SIN_DISPOSITIVOS) en vez de quedar pendiente con cero intentos.
func TestPUSHWorkerSinEmisorCierraTrabajoSinDispositivos(t *testing.T) {
	store := nuevoStoreFalso()
	tokens := &tokensFalsos{porUsuario: map[string][]TokenDestino{}}
	servicio := nuevoServicio(t, store, nil, &prefsFalsas{valor: preferencias(true, "America/Mexico_City", nil)}, tokens, &bandejaFalsa{})
	if !servicio.Disponible() {
		t.Fatal("el worker debe estar disponible solo con el store del outbox")
	}
	id := encolarPrueba(t, servicio, "u-sin-fcm", "evt-sin-fcm")
	servicio.Ciclo(context.Background())

	registro := store.estado(id)
	if registro.NotificationID == "" {
		t.Fatalf("no se encontro el trabajo %q", id)
	}
	if registro.Status != EstadoRevocado || registro.UltimaClase != ClaseSinDispositivos {
		t.Fatalf("se esperaba revocado/SIN_DISPOSITIVOS, obtenido estado=%s clase=%q", registro.Status, registro.UltimaClase)
	}
	if registro.Attempts != 1 {
		t.Fatalf("el trabajo debio reclamarse una vez, intentos=%d", registro.Attempts)
	}
}

// Con un dispositivo registrado pero sin emisor FCM, el trabajo no se pierde ni provoca
// un panic: queda reprogramado con CONFIG_FCM para reintentarse cuando la configuracion
// este disponible.
func TestPUSHWorkerSinEmisorConDispositivoReprogramaConfigFCM(t *testing.T) {
	store := nuevoStoreFalso()
	tokens := &tokensFalsos{porUsuario: map[string][]TokenDestino{
		"u-con-fcm": {{ID: "dev-1", Valor: "token-valido"}},
	}}
	servicio := nuevoServicio(t, store, nil, &prefsFalsas{valor: preferencias(true, "America/Mexico_City", nil)}, tokens, &bandejaFalsa{})
	id := encolarPrueba(t, servicio, "u-con-fcm", "evt-con-fcm")
	servicio.Ciclo(context.Background())

	registro := store.estado(id)
	if registro.NotificationID == "" {
		t.Fatalf("no se encontro el trabajo %q", id)
	}
	if registro.Status != EstadoFallido || registro.UltimaClase != ClaseConfigFCM {
		t.Fatalf("se esperaba fallido/CONFIG_FCM, obtenido estado=%s clase=%q", registro.Status, registro.UltimaClase)
	}
	if registro.Attempts < 1 {
		t.Fatalf("el trabajo debio reclamarse, intentos=%d", registro.Attempts)
	}
}
