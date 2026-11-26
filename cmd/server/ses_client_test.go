package main

import (
	"crypto/tls"
	"net/http"
	"testing"
	"time"
)

// TestSESHTTPClientSinKeepAliveYConTimeouts fija la configuración del cliente HTTP de
// SES: sin reutilización de conexiones y con timeouts que fallan rápido dentro del
// WriteTimeout HTTP. No contacta la red.
func TestSESHTTPClientSinKeepAliveYConTimeouts(t *testing.T) {
	cliente := sesHTTPClient()
	if cliente == nil {
		t.Fatal("el cliente SES no puede ser nil")
	}
	transporte, ok := cliente.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("transporte inesperado: %T", cliente.Transport)
	}
	if !transporte.DisableKeepAlives {
		t.Fatal("el cliente SES no debe reutilizar conexiones ociosas")
	}
	if transporte.ResponseHeaderTimeout <= 0 || transporte.ResponseHeaderTimeout > 8*time.Second {
		t.Fatalf("ResponseHeaderTimeout=%s fuera del margen previsto", transporte.ResponseHeaderTimeout)
	}
	if transporte.TLSHandshakeTimeout <= 0 || transporte.TLSHandshakeTimeout > 4*time.Second {
		t.Fatalf("TLSHandshakeTimeout=%s fuera del margen previsto", transporte.TLSHandshakeTimeout)
	}
	if transporte.TLSClientConfig == nil || transporte.TLSClientConfig.MinVersion != tls.VersionTLS12 {
		t.Fatal("el cliente SES exige TLS 1.2 o superior")
	}
}
