// Package push implementa el emisor real de Firebase Cloud Messaging sobre
// HTTP v1 con OAuth 2.0 JWT assertion RS256. Se usa exclusivamente stdlib.
//
// Invariantes de seguridad:
//   - Nunca se usa la legacy server key ni un secreto en claro en la URL.
//   - La credencial se interpreta en memoria; jamás se incluye en errores,
//     mensajes de log ni métricas. Sólo se observan clase de error y código HTTP.
//   - El ID de mensaje (`data.eventId`) viaja en el body en cada intento, de modo
//     que un reintento no crea una notificación distinta.
package push

import (
	"bytes"
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
	"io"
	rand2 "math/rand/v2"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	// ScopeFCM es el ámbito OAuth 2.0 mínimo para HTTP v1.
	ScopeFCM = "https://www.googleapis.com/auth/firebase.messaging"
	// GrantTypeJWTBearer es el grant OAuth 2.0 de cuenta de servicio.
	GrantTypeJWTBearer = "urn:ietf:params:oauth:grant-type:jwt-bearer"

	defaultOAuthTokenURL = "https://oauth2.googleapis.com/token"
	defaultEndpointFmt   = "https://fcm.googleapis.com/v1/projects/%s/messages:send"
	defaultTimeout       = 5 * time.Second
	defaultMaxRetries    = 2
	maxJitterRetries     = 4
)

// Errores estables para clasificar el fallo sin inspeccionar texto secreto.
var (
	ErrConfiguracion   = errors.New("fcm: configuracion incompleta")
	ErrCredencial      = fmt.Errorf("%w: credencial de servicio invalida", ErrConfiguracion)
	ErrPermanente      = errors.New("fcm: fallo permanente")
	ErrTransitorio     = errors.New("fcm: fallo transitorio")
	ErrMensajeInvalido = errors.New("fcm: mensaje invalido")
)

// Clases de error observables. No incluyen ningún dato del titular ni secreto.
const (
	ClaseConfigInvalida    = "CONFIG_INVALIDA"
	ClaseCredencial        = "CREDENCIAL_INVALIDA"
	ClaseTokenNoRegistrado = "TOKEN_NO_REGISTRADO"
	ClaseArgumentoInvalido = "ARGUMENTO_INVALIDO"
	ClaseLimiteExcedido    = "LIMITE_EXCEDIDO"
	ClaseErrorServidor     = "SERVER_ERROR"
	ClaseRed               = "RED"
	ClaseDesconocido       = "DESCONOCIDO"
)

// Observer recibe únicamente metadatos no sensibles. La implementación por
// defecto no hace nada; never se registra el body, el token ni la credencial.
type Observer interface {
	EnvioFCM(codigoHTTP int, clase string, intentos int)
}

// ObserverFunc adapta una función a Observer.
type ObserverFunc func(codigoHTTP int, clase string, intentos int)

func (f ObserverFunc) EnvioFCM(codigoHTTP int, clase string, intentos int) {
	if f != nil {
		f(codigoHTTP, clase, intentos)
	}
}

// Config es la configuración del emisor. En producción se construye desde
// variables de entorno; en pruebas se pueden inyectar Endpoint, OAuthTokenURL,
// HTTPClient, Clock y Sleep sin tocar el código de red real.
type Config struct {
	ProjectID       string
	CredentialsJSON []byte
	Endpoint        string        // opcional; por defecto HTTP v1 de Google
	OAuthTokenURL   string        // opcional; por defecto oauth2.googleapis.com
	HTTPClient      *http.Client  // opcional; se usa uno con Timeout acotado
	Timeout         time.Duration // usado sólo si HTTPClient es nil
	Clock           func() time.Time
	Sleep           func(context.Context, time.Duration) error
	Observer        Observer
	MaxRetries      int // 0 = dos reintentos por defecto
}

// Validar falla cerrado si faltan datos esenciales. Los mensajes de error no
// incluyen el contenido de CredentialsJSON.
func (c Config) Validar() error {
	if strings.TrimSpace(c.ProjectID) == "" {
		return fmt.Errorf("%w: falta projectId", ErrConfiguracion)
	}
	if len(bytes.TrimSpace(c.CredentialsJSON)) == 0 {
		return fmt.Errorf("%w: falta credencial de cuenta de servicio", ErrConfiguracion)
	}
	if c.Timeout < 0 {
		return fmt.Errorf("%w: timeout negativo", ErrConfiguracion)
	}
	if c.MaxRetries < 0 || c.MaxRetries > maxJitterRetries {
		return fmt.Errorf("%w: maxRetries fuera de rango", ErrConfiguracion)
	}
	return nil
}

func (c Config) endpoint() string {
	if strings.TrimSpace(c.Endpoint) != "" {
		return strings.TrimSpace(c.Endpoint)
	}
	return fmt.Sprintf(defaultEndpointFmt, url.PathEscape(strings.TrimSpace(c.ProjectID)))
}

func (c Config) tokenURL() string {
	if strings.TrimSpace(c.OAuthTokenURL) != "" {
		return strings.TrimSpace(c.OAuthTokenURL)
	}
	return defaultOAuthTokenURL
}

func (c Config) timeout() time.Duration {
	if c.Timeout > 0 {
		return c.Timeout
	}
	return defaultTimeout
}

func (c Config) retries() int {
	if c.MaxRetries == 0 {
		return defaultMaxRetries
	}
	return c.MaxRetries
}

// Mensaje es la entrada del emisor. EventID debe ser estable por notificación
// para que los reintentos no dupliquen el envío.
type Mensaje struct {
	Token   string
	Title   string
	Body    string
	EventID string
}

func (m Mensaje) validar() error {
	if strings.TrimSpace(m.Token) == "" || strings.TrimSpace(m.Title) == "" ||
		strings.TrimSpace(m.Body) == "" || strings.TrimSpace(m.EventID) == "" {
		return ErrMensajeInvalido
	}
	return nil
}

// Resultado expone sólo metadatos de la respuesta aceptada.
type Resultado struct {
	MessageID  string
	CodigoHTTP int
	Intentos   int
}

// Emisor es la interfaz estrecha que consume el outbox.
type Emisor interface {
	Enviar(ctx context.Context, mensaje Mensaje) (Resultado, error)
}

// ErrorClasificado separa la clase del fallo del mensaje crudo del proveedor.
// Nunca transporta credenciales, tokens ni bodies.
type ErrorClasificado struct {
	Clase      string
	CodigoHTTP int
	Causa      error
}

func (e *ErrorClasificado) Error() string {
	if e == nil {
		return "fcm: error sin clasificar"
	}
	return "fcm: clase=" + e.Clase + " http=" + strconv.Itoa(e.CodigoHTTP)
}

func (e *ErrorClasificado) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Causa
}

// EsCredencialIndica que el fallo exige intervención de configuración, no un
// reintento automático de envío.
func EsCredencialInvalida(err error) bool {
	var clasificado *ErrorClasificado
	return errors.As(err, &clasificado) && clasificado.Clase == ClaseCredencial
}

func esPermanente(err error) bool {
	var clasificado *ErrorClasificado
	if !errors.As(err, &clasificado) {
		return errors.Is(err, ErrPermanente) || errors.Is(err, ErrConfiguracion)
	}
	return clasificado.Clase == ClaseTokenNoRegistrado || clasificado.Clase == ClaseArgumentoInvalido
}

// esReintentableHTTP aplica EXACTAMENTE a 429 y 5xx. Los fallos de red y de
// credencial no se reintentan dentro del emisor: el outbox se encarga del
// backoff durable para los primeros y de la intervención de configuración para
// los segundos.
func esReintentableHTTP(err error) bool {
	var clasificado *ErrorClasificado
	if !errors.As(err, &clasificado) {
		return false
	}
	return clasificado.Clase == ClaseLimiteExcedido || clasificado.Clase == ClaseErrorServidor
}

type credencialServicio struct {
	Type        string `json:"type"`
	ProjectID   string `json:"project_id"`
	ClientEmail string `json:"client_email"`
	PrivateKey  string `json:"private_key"`
	TokenURI    string `json:"token_uri"`
}

type jwtCabecera struct {
	Alg string `json:"alg"`
	Typ string `json:"typ"`
}

type jwtClaims struct {
	Iss   string `json:"iss"`
	Scope string `json:"scope"`
	Aud   string `json:"aud"`
	Iat   int64  `json:"iat"`
	Exp   int64  `json:"exp"`
}

type tokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int64  `json:"expires_in"`
	TokenType   string `json:"token_type"`
}

type fcmNotification struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}

type fcmMessage struct {
	Token        string            `json:"token"`
	Notification fcmNotification   `json:"notification"`
	Data         map[string]string `json:"data,omitempty"`
}

type fcmRequest struct {
	Message fcmMessage `json:"message"`
}

type fcmErrorResponse struct {
	Error struct {
		Code    int    `json:"code"`
		Status  string `json:"status"`
		Message string `json:"message"`
		Details []struct {
			Type string `json:"@type"`
		} `json:"details"`
	} `json:"error"`
}

// Client es reutilizable y concurrente: cachea el access token hasta un minuto
// antes de su caducidad y serializa su renovación con un mutex.
type Client struct {
	cfg        Config
	credencial credencialServicio
	clave      *rsa.PrivateKey
	http       *http.Client
	mu         sync.Mutex
	token      string
	expiraEn   time.Time
}

// NewClient construye el cliente real. No realiza llamadas de red; Validate
// falla antes de cualquier conexión si la configuración está incompleta.
func NewClient(cfg Config) (*Client, error) {
	if err := cfg.Validar(); err != nil {
		return nil, err
	}
	c := &Client{cfg: cfg}
	if cfg.HTTPClient != nil {
		c.http = cfg.HTTPClient
	} else {
		c.http = &http.Client{Timeout: cfg.timeout()}
	}
	if err := c.cargarCredencial(); err != nil {
		return nil, err
	}
	return c, nil
}

func (c *Client) cargarCredencial() error {
	var cred credencialServicio
	if err := json.Unmarshal(c.cfg.CredentialsJSON, &cred); err != nil {
		return fmt.Errorf("%w: JSON de cuenta de servicio ilegible", ErrCredencial)
	}
	if !strings.Contains(cred.ClientEmail, "@") || strings.TrimSpace(cred.PrivateKey) == "" {
		return fmt.Errorf("%w: campos de cuenta de servicio incompletos", ErrCredencial)
	}
	if strings.TrimSpace(cred.Type) != "" && strings.TrimSpace(cred.Type) != "service_account" {
		return fmt.Errorf("%w: tipo de cuenta no soportado", ErrCredencial)
	}
	if proyecto := strings.TrimSpace(cred.ProjectID); proyecto != "" && proyecto != strings.TrimSpace(c.cfg.ProjectID) {
		return fmt.Errorf("%w: projectId no coincide con la credencial", ErrCredencial)
	}
	bloque, _ := pem.Decode([]byte(cred.PrivateKey))
	if bloque == nil {
		return fmt.Errorf("%w: clave privada no PEM", ErrCredencial)
	}
	parsed, err := x509.ParsePKCS8PrivateKey(bloque.Bytes)
	if err != nil {
		parsed, err = x509.ParsePKCS1PrivateKey(bloque.Bytes)
		if err != nil {
			return fmt.Errorf("%w: clave privada no RS256", ErrCredencial)
		}
	}
	clave, ok := parsed.(*rsa.PrivateKey)
	if !ok || clave == nil {
		return fmt.Errorf("%w: clave privada no RSA", ErrCredencial)
	}
	c.credencial = cred
	c.clave = clave
	return nil
}

func (c *Client) ahora() time.Time {
	if c.cfg.Clock != nil {
		return c.cfg.Clock().UTC()
	}
	return time.Now().UTC()
}

func codificarBase64URL(value []byte) string {
	return base64.RawURLEncoding.EncodeToString(value)
}

func (c *Client) jwtAssertion() (string, error) {
	cabecera, err := json.Marshal(jwtCabecera{Alg: "RS256", Typ: "JWT"})
	if err != nil {
		return "", fmt.Errorf("%w: no se pudo codificar la cabecera JWT", ErrCredencial)
	}
	ahora := c.ahora()
	claims, err := json.Marshal(jwtClaims{
		Iss:   c.credencial.ClientEmail,
		Scope: ScopeFCM,
		Aud:   c.cfg.tokenURL(),
		Iat:   ahora.Unix(),
		Exp:   ahora.Add(time.Hour).Unix(),
	})
	if err != nil {
		return "", fmt.Errorf("%w: no se pudo codificar el claim JWT", ErrCredencial)
	}
	entrada := codificarBase64URL(cabecera) + "." + codificarBase64URL(claims)
	hash := sha256.Sum256([]byte(entrada))
	firma, err := rsa.SignPKCS1v15(rand.Reader, c.clave, crypto.SHA256, hash[:])
	if err != nil {
		return "", fmt.Errorf("%w: no se pudo firmar el assertion", ErrCredencial)
	}
	return entrada + "." + codificarBase64URL(firma), nil
}

func (c *Client) accessToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.token != "" && c.ahora().Before(c.expiraEn.Add(-time.Minute)) {
		return c.token, nil
	}
	assertion, err := c.jwtAssertion()
	if err != nil {
		return "", err
	}
	form := url.Values{}
	form.Set("grant_type", GrantTypeJWTBearer)
	form.Set("assertion", assertion)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.tokenURL(), strings.NewReader(form.Encode()))
	if err != nil {
		return "", &ErrorClasificado{Clase: ClaseConfigInvalida, Causa: err}
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.http.Do(req)
	if err != nil {
		return "", &ErrorClasificado{Clase: ClaseRed, Causa: err}
	}
	defer resp.Body.Close()
	cuerpo, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", &ErrorClasificado{Clase: ClaseRed, Causa: err}
	}
	if resp.StatusCode != http.StatusOK {
		clase := ClaseDesconocido
		switch {
		case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden ||
			resp.StatusCode == http.StatusBadRequest:
			clase = ClaseCredencial
		case resp.StatusCode == http.StatusTooManyRequests:
			clase = ClaseLimiteExcedido
		case resp.StatusCode >= 500:
			clase = ClaseErrorServidor
		}
		return "", &ErrorClasificado{Clase: clase, CodigoHTTP: resp.StatusCode}
	}
	var parsed tokenResponse
	if err := json.Unmarshal(cuerpo, &parsed); err != nil || strings.TrimSpace(parsed.AccessToken) == "" {
		return "", &ErrorClasificado{Clase: ClaseCredencial, CodigoHTTP: resp.StatusCode}
	}
	segundos := parsed.ExpiresIn
	if segundos <= 0 {
		segundos = 3600
	}
	c.token = parsed.AccessToken
	c.expiraEn = c.ahora().Add(time.Duration(segundos) * time.Second)
	return c.token, nil
}

func (c *Client) dormir(ctx context.Context, intento int) error {
	base := 200 * time.Millisecond
	for i := 0; i < intento && i < 3; i++ {
		base *= 2
	}
	if base > 2*time.Second {
		base = 2 * time.Second
	}
	mitad := base / 4
	espera := base - mitad + time.Duration(rand2.Int64N(int64(2*mitad+1)))
	if c.cfg.Sleep != nil {
		return c.cfg.Sleep(ctx, espera)
	}
	temporizador := time.NewTimer(espera)
	defer temporizador.Stop()
	select {
	case <-temporizador.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Enviar publica un mensaje HTTP v1. Sólo reintenta 429/5xx, con un máximo de
// MaxRetries (dos por defecto) y jitter acotado. Los errores de red y de
// credencial se devuelven de inmediato; el outbox aplica su propio backoff.
func (c *Client) Enviar(ctx context.Context, mensaje Mensaje) (Resultado, error) {
	if err := mensaje.validar(); err != nil {
		return Resultado{}, err
	}
	var ultimaClase string
	var ultimoCodigo int
	for intento := 0; intento <= c.cfg.retries(); intento++ {
		token, err := c.accessToken(ctx)
		if err != nil {
			var clasificado *ErrorClasificado
			if errors.As(err, &clasificado) {
				ultimaClase, ultimoCodigo = clasificado.Clase, clasificado.CodigoHTTP
			} else {
				ultimaClase = ClaseCredencial
			}
			c.observar(ultimoCodigo, ultimaClase, intento+1)
			if esReintentableHTTP(err) && intento < c.cfg.retries() {
				if errDormir := c.dormir(ctx, intento); errDormir != nil {
					return Resultado{Intentos: intento + 1}, errDormir
				}
				continue
			}
			if ultimaClase == ClaseCredencial {
				return Resultado{Intentos: intento + 1, CodigoHTTP: ultimoCodigo}, ErrCredencial
			}
			return Resultado{Intentos: intento + 1, CodigoHTTP: ultimoCodigo}, err
		}

		cuerpo, err := json.Marshal(fcmRequest{Message: fcmMessage{
			Token:        mensaje.Token,
			Notification: fcmNotification{Title: mensaje.Title, Body: mensaje.Body},
			Data:         map[string]string{"eventId": mensaje.EventID},
		}})
		if err != nil {
			return Resultado{}, fmt.Errorf("%w: no se pudo codificar el mensaje", ErrMensajeInvalido)
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.endpoint(), bytes.NewReader(cuerpo))
		if err != nil {
			return Resultado{}, &ErrorClasificado{Clase: ClaseConfigInvalida, Causa: err}
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		resp, err := c.http.Do(req)
		if err != nil {
			ultimaClase = ClaseRed
			c.observar(0, ultimaClase, intento+1)
			return Resultado{Intentos: intento + 1}, &ErrorClasificado{Clase: ClaseRed, Causa: err}
		}
		respuesta, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		_ = resp.Body.Close()

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			var aceptado struct {
				Name string `json:"name"`
			}
			_ = json.Unmarshal(respuesta, &aceptado)
			c.observar(resp.StatusCode, "OK", intento+1)
			return Resultado{MessageID: aceptado.Name, CodigoHTTP: resp.StatusCode, Intentos: intento + 1}, nil
		}

		clase := clasificarRespuestaFCM(resp.StatusCode, respuesta)
		ultimaClase, ultimoCodigo = clase, resp.StatusCode
		c.observar(resp.StatusCode, clase, intento+1)
		errEnvio := &ErrorClasificado{Clase: clase, CodigoHTTP: resp.StatusCode}
		if esReintentableHTTP(errEnvio) && intento < c.cfg.retries() {
			if errDormir := c.dormir(ctx, intento); errDormir != nil {
				return Resultado{Intentos: intento + 1, CodigoHTTP: ultimoCodigo}, errDormir
			}
			continue
		}
		if clase == ClaseCredencial {
			errEnvio.Causa = ErrCredencial
		}
		return Resultado{Intentos: intento + 1, CodigoHTTP: ultimoCodigo}, errEnvio
	}
	return Resultado{Intentos: c.cfg.retries() + 1, CodigoHTTP: ultimoCodigo},
		&ErrorClasificado{Clase: ultimaClase, CodigoHTTP: ultimoCodigo}
}

func clasificarRespuestaFCM(codigo int, cuerpo []byte) string {
	var parsed fcmErrorResponse
	_ = json.Unmarshal(cuerpo, &parsed)
	detalle := strings.ToUpper(parsed.Error.Status)
	for _, d := range parsed.Error.Details {
		detalle += " " + strings.ToUpper(d.Type)
	}
	detalle += " " + strings.ToUpper(parsed.Error.Message)

	if codigo == http.StatusUnauthorized || codigo == http.StatusForbidden {
		return ClaseCredencial
	}
	if codigo == http.StatusTooManyRequests || codigo == http.StatusRequestTimeout {
		return ClaseLimiteExcedido
	}
	if codigo >= 500 {
		return ClaseErrorServidor
	}
	if strings.Contains(detalle, "UNREGISTERED") || strings.Contains(detalle, "NOT REGISTERED") ||
		strings.Contains(detalle, "INVALID TOKEN") || strings.Contains(detalle, "INVALID_ARGUMENT") {
		return ClaseTokenNoRegistrado
	}
	return ClaseArgumentoInvalido
}

func (c *Client) observar(codigo int, clase string, intentos int) {
	if c.cfg.Observer != nil {
		c.cfg.Observer.EnvioFCM(codigo, clase, intentos)
	}
}
