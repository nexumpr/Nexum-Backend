package push

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
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Todas las claves y tokens de estas pruebas son sintéticos, generados en
// memoria. No hay credenciales reales ni archivos secretos.
type credencialPrueba struct {
	clave   *rsa.PrivateKey
	jsonRAW []byte
	email   string
}

func nuevaCredencialPrueba(t *testing.T) credencialPrueba {
	t.Helper()
	clave, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(clave)
	if err != nil {
		t.Fatal(err)
	}
	privadaPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
	cred := map[string]string{
		"type":                        "service_account",
		"project_id":                  "nexum-dev-sintetico",
		"private_key_id":              "sintetico-no-real",
		"private_key":                 string(privadaPEM),
		"client_email":                "emisor@nexum-dev-sintetico.iam.gserviceaccount.com",
		"client_id":                   "1234567890",
		"token_uri":                   "https://oauth2.googleapis.com/token",
		"auth_uri":                    "https://accounts.google.com/o/oauth2/auth",
		"auth_provider_x509_cert_url": "https://www.googleapis.com/oauth2/v1/certs",
		"client_x509_cert_url":        "https://www.googleapis.com/robot/v1/metadata/x509/emisor",
	}
	crudo, err := json.Marshal(cred)
	if err != nil {
		t.Fatal(err)
	}
	return credencialPrueba{clave: clave, jsonRAW: crudo, email: cred["client_email"]}
}

func verificarAssertion(t *testing.T, assertion string, clave *rsa.PublicKey, audiencia string) map[string]any {
	t.Helper()
	partes := strings.Split(assertion, ".")
	if len(partes) != 3 {
		t.Errorf("assertion JWT con %d partes", len(partes))
		return nil
	}
	cabeceraCruda, err := base64.RawURLEncoding.DecodeString(partes[0])
	if err != nil {
		t.Error(err)
		return nil
	}
	var cabecera jwtCabecera
	if err := json.Unmarshal(cabeceraCruda, &cabecera); err != nil {
		t.Error(err)
		return nil
	}
	if cabecera.Alg != "RS256" || cabecera.Typ != "JWT" {
		t.Errorf("cabecera JWT incorrecta: %+v", cabecera)
	}
	claimsCrudos, err := base64.RawURLEncoding.DecodeString(partes[1])
	if err != nil {
		t.Error(err)
		return nil
	}
	var claims map[string]any
	if err := json.Unmarshal(claimsCrudos, &claims); err != nil {
		t.Error(err)
		return nil
	}
	aud, _ := claims["aud"].(string)
	if claims["iss"] == nil || claims["scope"] != ScopeFCM || !strings.Contains(aud, audiencia) {
		t.Errorf("claims OAuth incorrectos: %+v", claims)
	}
	if claims["exp"] == nil || claims["iat"] == nil {
		t.Errorf("claims sin iat/exp: %+v", claims)
	}
	firma, err := base64.RawURLEncoding.DecodeString(partes[2])
	if err != nil {
		t.Error(err)
		return nil
	}
	hash := sha256.Sum256([]byte(partes[0] + "." + partes[1]))
	if err := rsa.VerifyPKCS1v15(clave, crypto.SHA256, hash[:], firma); err != nil {
		t.Errorf("firma RS256 inválida: %v", err)
	}
	return claims
}

func servidorOAuthPrueba(t *testing.T, cred credencialPrueba, estado int, llamadas *atomic.Int32) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		llamadas.Add(1)
		if estado != http.StatusOK {
			w.WriteHeader(estado)
			_, _ = w.Write([]byte(`{"error":"invalid_client"}`))
			return
		}
		if err := r.ParseForm(); err != nil {
			t.Errorf("formulario OAuth: %v", err)
		}
		if r.Form.Get("grant_type") != GrantTypeJWTBearer {
			t.Errorf("grant_type incorrecto: %q", r.Form.Get("grant_type"))
		}
		verificarAssertion(t, r.Form.Get("assertion"), &cred.clave.PublicKey, r.Host)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"access-token-sintetico","expires_in":3600,"token_type":"Bearer"}`))
	}))
}

func TestFCMEnvioCorrectoHTTPV1YOAuth(t *testing.T) {
	cred := nuevaCredencialPrueba(t)
	var oauthLlamadas, fcmLlamadas atomic.Int32
	var oauth *httptest.Server
	oauth = servidorOAuthPrueba(t, cred, http.StatusOK, &oauthLlamadas)
	defer oauth.Close()

	fcm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fcmLlamadas.Add(1)
		if r.Method != http.MethodPost {
			t.Errorf("método FCM: %s", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/messages:send") {
			t.Errorf("ruta FCM inesperada: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer access-token-sintetico" {
			t.Error("Authorization Bearer ausente o incorrecto")
		}
		if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
			t.Errorf("Content-Type FCM: %s", r.Header.Get("Content-Type"))
		}
		var body fcmRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if body.Message.Token != "token-sintetico" || body.Message.Notification.Title != "Aviso" ||
			body.Message.Notification.Body != "Revisa tu presupuesto" ||
			body.Message.Data["eventId"] != "evento-123" {
			t.Fatalf("body FCM incorrecto: %+v", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"name":"projects/nexum-dev-sintetico/messages/1"}`))
	}))
	defer fcm.Close()

	cfg := Config{
		ProjectID:       "nexum-dev-sintetico",
		CredentialsJSON: cred.jsonRAW,
		Endpoint:        fcm.URL + "/v1/projects/nexum-dev-sintetico/messages:send",
		OAuthTokenURL:   oauth.URL,
		HTTPClient:      &http.Client{Timeout: 2 * time.Second},
	}
	cliente, err := NewClient(cfg)
	if err != nil {
		t.Fatal(err)
	}
	resultado, err := cliente.Enviar(context.Background(), Mensaje{
		Token: "token-sintetico", Title: "Aviso", Body: "Revisa tu presupuesto", EventID: "evento-123",
	})
	if err != nil {
		t.Fatal(err)
	}
	if resultado.MessageID == "" || resultado.CodigoHTTP != http.StatusOK || resultado.Intentos != 1 {
		t.Fatalf("resultado inesperado: %+v", resultado)
	}
	if oauthLlamadas.Load() != 1 || fcmLlamadas.Load() != 1 {
		t.Fatalf("llamadas OAuth=%d FCM=%d", oauthLlamadas.Load(), fcmLlamadas.Load())
	}
}

func TestFCMCredencial401EsConfiguracionYSinReintentos(t *testing.T) {
	cred := nuevaCredencialPrueba(t)
	var oauthLlamadas, fcmLlamadas atomic.Int32
	oauth := servidorOAuthPrueba(t, cred, http.StatusUnauthorized, &oauthLlamadas)
	defer oauth.Close()
	fcm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fcmLlamadas.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer fcm.Close()

	cliente, err := NewClient(Config{
		ProjectID: "nexum-dev-sintetico", CredentialsJSON: cred.jsonRAW,
		Endpoint: fcm.URL, OAuthTokenURL: oauth.URL, HTTPClient: &http.Client{Timeout: 2 * time.Second},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = cliente.Enviar(context.Background(), Mensaje{Token: "t", Title: "x", Body: "y", EventID: "e"})
	if !errors.Is(err, ErrCredencial) {
		t.Fatalf("se esperaba ErrCredencial, se obtuvo %v", err)
	}
	if oauthLlamadas.Load() != 1 || fcmLlamadas.Load() != 0 {
		t.Fatalf("credencial inválida no debió reintentar: OAuth=%d FCM=%d", oauthLlamadas.Load(), fcmLlamadas.Load())
	}
}

func TestFCMUNREGISTEREDEsPermanente(t *testing.T) {
	casos := []struct {
		nombre string
		codigo int
		cuerpo string
	}{
		{"400_INVALID_ARGUMENT", http.StatusBadRequest,
			`{"error":{"code":400,"status":"INVALID_ARGUMENT","message":"unregistered"}}`},
		{"404_NOT_FOUND", http.StatusNotFound,
			`{"error":{"code":404,"status":"NOT_FOUND","message":"unregistered"}}`},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			cred := nuevaCredencialPrueba(t)
			var oauthLlamadas, fcmLlamadas atomic.Int32
			oauth := servidorOAuthPrueba(t, cred, http.StatusOK, &oauthLlamadas)
			defer oauth.Close()
			fcm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				fcmLlamadas.Add(1)
				w.WriteHeader(caso.codigo)
				_, _ = w.Write([]byte(caso.cuerpo))
			}))
			defer fcm.Close()

			cliente, err := NewClient(Config{
				ProjectID: "nexum-dev-sintetico", CredentialsJSON: cred.jsonRAW,
				Endpoint: fcm.URL, OAuthTokenURL: oauth.URL, HTTPClient: &http.Client{Timeout: 2 * time.Second},
			})
			if err != nil {
				t.Fatal(err)
			}
			_, err = cliente.Enviar(context.Background(), Mensaje{Token: "t", Title: "x", Body: "y", EventID: "e"})
			if !esPermanente(err) {
				t.Fatalf("UNREGISTERED debió ser permanente: %v", err)
			}
			var clasificado *ErrorClasificado
			if !errors.As(err, &clasificado) || clasificado.Clase != ClaseTokenNoRegistrado {
				t.Fatalf("clase incorrecta: %v", err)
			}
			if fcmLlamadas.Load() != 1 {
				t.Fatalf("UNREGISTERED no debió reintentarse: %d", fcmLlamadas.Load())
			}
		})
	}
}

func TestFCMReintento429Y500ConJitterAcotadoSinDuplicarEventID(t *testing.T) {
	cred := nuevaCredencialPrueba(t)
	var oauthLlamadas, fcmLlamadas atomic.Int32
	oauth := servidorOAuthPrueba(t, cred, http.StatusOK, &oauthLlamadas)
	defer oauth.Close()
	var muEventos sync.Mutex
	eventos := make([]string, 0, 3)
	fcm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		llamada := fcmLlamadas.Add(1)
		var body fcmRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		muEventos.Lock()
		eventos = append(eventos, body.Message.Data["eventId"])
		muEventos.Unlock()
		switch llamada {
		case 1:
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":{"status":"RESOURCE_EXHAUSTED"}}`))
		case 2:
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":{"status":"INTERNAL"}}`))
		default:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"name":"projects/x/messages/ok"}`))
		}
	}))
	defer fcm.Close()

	var pausas []time.Duration
	cliente, err := NewClient(Config{
		ProjectID: "nexum-dev-sintetico", CredentialsJSON: cred.jsonRAW,
		Endpoint: fcm.URL, OAuthTokenURL: oauth.URL, HTTPClient: &http.Client{Timeout: 2 * time.Second},
		Sleep: func(_ context.Context, d time.Duration) error { pausas = append(pausas, d); return nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	resultado, err := cliente.Enviar(context.Background(), Mensaje{
		Token: "t", Title: "x", Body: "y", EventID: "evento-unico",
	})
	if err != nil {
		t.Fatal(err)
	}
	if resultado.Intentos != 3 || fcmLlamadas.Load() != 3 {
		t.Fatalf("reintentos incorrectos: %+v llamadas=%d", resultado, fcmLlamadas.Load())
	}
	if len(pausas) != 2 {
		t.Fatalf("pausas capturadas: %v", pausas)
	}
	for _, p := range pausas {
		if p < 150*time.Millisecond || p > 2500*time.Millisecond {
			t.Fatalf("jitter fuera de rango: %v", p)
		}
	}
	muEventos.Lock()
	defer muEventos.Unlock()
	for _, evento := range eventos {
		if evento != "evento-unico" {
			t.Fatalf("eventId cambiado entre intentos: %v", eventos)
		}
	}
	if oauthLlamadas.Load() != 1 {
		t.Fatalf("el access token debió reutilizarse: %d", oauthLlamadas.Load())
	}
}

func TestFCMConfigIncompletaYCredencialMalformadaSinSecretos(t *testing.T) {
	if err := (Config{}).Validar(); !errors.Is(err, ErrConfiguracion) {
		t.Fatalf("config vacía debió fallar: %v", err)
	}
	secreto := "PRIVATE-KEY-SINTETICO-NO-REAL"
	_, err := NewClient(Config{
		ProjectID:       "nexum-dev-sintetico",
		CredentialsJSON: []byte(`{"type":"service_account","client_email":"a@b.c","private_key":"` + secreto + `"}`),
	})
	if !errors.Is(err, ErrCredencial) {
		t.Fatalf("credencial malformada debió fallar: %v", err)
	}
	if strings.Contains(err.Error(), secreto) {
		t.Fatal("el error reveló contenido de la credencial")
	}
}

func TestFCMMensajeInvalidoNoLlamaRed(t *testing.T) {
	cliente, err := NewClient(Config{ProjectID: "nexum-dev-sintetico", CredentialsJSON: nuevaCredencialPrueba(t).jsonRAW})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cliente.Enviar(context.Background(), Mensaje{Token: "t"}); !errors.Is(err, ErrMensajeInvalido) {
		t.Fatalf("mensaje incompleto debió rechazarse: %v", err)
	}
}
