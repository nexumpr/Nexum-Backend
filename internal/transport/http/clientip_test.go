package http

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	appIdentity "github.com/nexumpr/Nexum-Backend/internal/application/identity"
)

// TestClientIPFromTrustedChain verifica la política documentada en clientip.go contra
// la topología real CloudFront -> ALB interno -> EC2.
func TestClientIPFromTrustedChain(t *testing.T) {
	casos := []struct {
		nombre    string
		remote    string
		forwarded string
		esperado  string
		ok        bool
	}{
		// Producción: ALB añade el borde y CloudFront añadió el visor.
		{"visor y borde", "10.42.1.10:443", "203.0.113.7, 130.176.10.20", "203.0.113.7", true},
		{"prefijo del cliente ignorado", "10.42.1.10:443", "1.2.3.4, 203.0.113.7, 130.176.10.20", "203.0.113.7", true},
		{"ipv6 visor", "10.42.1.10:443", "2001:db8::1, 130.176.10.20", "2001:db8::1", true},
		{"ipv6 borde", "10.42.1.10:443", "203.0.113.7, 2600:9000::1", "203.0.113.7", true},
		{"ipv4 mapeada", "10.42.1.10:443", "::ffff:203.0.113.7, 130.176.10.20", "203.0.113.7", true},
		{"peer privado distinto", "172.16.5.9:443", "203.0.113.7, 130.176.10.20", "203.0.113.7", true},

		// Un solo salto con peer privado no corresponde a la topología completa.
		{"un solo salto con ALB", "10.42.1.10:443", "203.0.113.7", "", false},
		// VPC Origin: el ALB appende la ENI privada de CloudFront, no una IP pública.
		{"ultimo salto privado vpc origin", "10.42.1.10:443", "203.0.113.7, 10.42.0.55", "203.0.113.7", true},
		{"ultimo salto ipv6 privado", "10.42.1.10:443", "2001:db8::1, fd00:ec2::55", "2001:db8::1", true},
		{"ultimo salto loopback", "10.42.1.10:443", "203.0.113.7, 127.0.0.1", "", false},
		// Cadenas malformadas o ambiguas.
		{"malformada", "10.42.1.10:443", "no-es-ip, 130.176.10.20", "", false},
		{"elemento vacio", "10.42.1.10:443", "203.0.113.7, , 130.176.10.20", "", false},
		{"zona ipv6", "10.42.1.10:443", "fe80::1%eth0, 130.176.10.20", "", false},
		// Un peer público nunca es el proxy de confianza.
		{"peer publico con cadena", "198.51.100.9:5555", "203.0.113.7, 130.176.10.20", "", false},
		{"peer publico sin cadena", "198.51.100.9:5555", "", "", false},
		// El peer loopback (pruebas o proxy local) acepta el último salto.
		{"loopback con un salto", "127.0.0.1:5000", "203.0.113.7", "203.0.113.7", true},
		{"loopback sin cadena", "127.0.0.1:5000", "", "127.0.0.1", true},
		// Peer privado sin cadena: no acredita el paso por CloudFront; se rechaza.
		{"privado sin cadena", "10.42.1.10:443", "", "", false},
	}

	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.RemoteAddr = caso.remote
			if caso.forwarded != "" {
				req.Header.Set("X-Forwarded-For", caso.forwarded)
			}
			ip, ok := clientIPFromTrustedChain(req)
			if ok != caso.ok {
				t.Fatalf("ok esperado %v, obtenido %v (ip=%q)", caso.ok, ok, ip)
			}
			if ip != caso.esperado {
				t.Fatalf("IP esperada %q, obtenida %q", caso.esperado, ip)
			}
		})
	}
}

// TestClientIPFromTrustedChainConcurrente comprueba que la extracción es pura y no
// comparte estado entre peticiones concurrentes.
func TestClientIPFromTrustedChainConcurrente(t *testing.T) {
	const workers = 128
	var wg sync.WaitGroup
	errores := make(chan string, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(indice int) {
			defer wg.Done()
			visor := "203.0.113." + itoa(indice%250)
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.RemoteAddr = "10.42.1.10:443"
			req.Header.Set("X-Forwarded-For", visor+", 130.176.10.20")
			ip, ok := clientIPFromTrustedChain(req)
			if !ok || ip != visor {
				errores <- "ip=" + ip + " visor=" + visor
			}
		}(i)
	}
	wg.Wait()
	close(errores)
	for err := range errores {
		t.Fatalf("resultado inconsistente en concurrencia: %s", err)
	}
}

// itoa evita depender de fmt en una prueba caliente de concurrencia.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [3]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

// TestCloudFrontViewerAddress verifica la fuente preferente: la cabecera que CloudFront
// sobrescribe. Un valor inválido o un visor privado se rechazan; nunca se degrada a XFF.
func TestCloudFrontViewerAddress(t *testing.T) {
	casos := []struct {
		nombre   string
		remote   string
		viewer   string
		forward  string
		esperado string
		ok       bool
	}{
		{"visor ipv4 con puerto", "10.42.1.10:443", "198.51.100.9:1234", "", "198.51.100.9", true},
		{"visor ipv6 con puerto", "10.42.1.10:443", "[2001:db8::9]:443", "", "2001:db8::9", true},
		{"prevalece sobre xff falsificado", "10.42.1.10:443", "198.51.100.9:1", "1.2.3.4, 203.0.113.7, 130.176.10.20", "198.51.100.9", true},
		{"formato invalido se rechaza", "10.42.1.10:443", "no-es-ip", "203.0.113.7, 10.42.0.55", "", false},
		{"visor privado se rechaza", "10.42.1.10:443", "10.42.0.99:1234", "", "", false},
		{"peer publico se rechaza", "198.51.100.20:443", "198.51.100.9:1", "", "", false},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.RemoteAddr = caso.remote
			req.Header.Set("CloudFront-Viewer-Address", caso.viewer)
			if caso.forward != "" {
				req.Header.Set("X-Forwarded-For", caso.forward)
			}
			ip, ok := clientIPFromTrustedChain(req)
			if ok != caso.ok || ip != caso.esperado {
				t.Fatalf("esperado (%q,%v), obtenido (%q,%v)", caso.esperado, caso.ok, ip, ok)
			}
		})
	}
}

// TestXFFDuplicadoSeRechaza evita que dos cabeceras XFF permitan interpretaciones
// distintas entre proxies.
func TestXFFDuplicadoSeRechaza(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.42.1.10:443"
	req.Header.Add("X-Forwarded-For", "203.0.113.7")
	req.Header.Add("X-Forwarded-For", "203.0.113.8, 10.42.0.55")
	if _, ok := clientIPFromTrustedChain(req); ok {
		t.Fatal("dos cabeceras XFF deben rechazarse")
	}
}

// TestRegisterRechazaCadenaNoConfiable confirma que E001 no aplica la cuota sobre
// datos controlables y falla cerrado antes de tocar el limitador.
func TestRegisterRechazaCadenaNoConfiable(t *testing.T) {
	limiter := &contadorLimiter{}
	handler := NewRegisterHandler(
		appIdentity.NewRegisterService(nil, nil),
		limiter,
		slog.New(slog.NewJSONHandler(io.Discard, nil)),
	)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", nil)
	req.RemoteAddr = "198.51.100.9:5555"
	req.Header.Set("X-Forwarded-For", "203.0.113.7")
	req.Header.Set("Idempotency-Key", strings.Repeat("h09", 6))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status esperado 400, obtenido %d: %s", rec.Code, rec.Body.String())
	}
	if limiter.llamadas != 0 {
		t.Fatalf("el limitador no debe invocarse con una cadena no confiable")
	}
}

type contadorLimiter struct {
	llamadas int
}

func (c *contadorLimiter) Allow(context.Context, string, string) (bool, time.Duration, error) {
	c.llamadas++
	return true, 0, nil
}
