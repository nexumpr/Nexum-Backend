package main

import (
	"crypto/tls"
	"net"
	"net/http"
	"time"
)

// sesHTTPClient construye el cliente HTTP del adaptador SES.
//
// El SDK de AWS reutiliza conexiones (keep-alive) y NO reintenta `SendEmail` porque
// el envio no es idempotente. En AWS se observaron solicitudes que agotaban el
// deadline completo sin respuesta y, repetidas poco despues, terminaban en ~400 ms:
// el sintoma de una conexion reutilizada que ya no responde, no de un mensaje lento.
//
// Deshabilitar el keep-alive obliga a abrir una conexion nueva en cada envio: cuesta
// un handshake TLS (~50 ms) y elimina la posibilidad de reutilizar una conexion
// ociosa rota. Los timeouts de dial/TLS/response-header fallan rapido y dejan margen
// dentro del WriteTimeout HTTP de 15 s; el deadline del contexto sigue siendo el
// limite autoritativo del envio.
func sesHTTPClient() *http.Client {
	transporte := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   3 * time.Second,
			KeepAlive: 0,
		}).DialContext,
		DisableKeepAlives:     true,
		TLSHandshakeTimeout:   3 * time.Second,
		ResponseHeaderTimeout: 6 * time.Second,
		ExpectContinueTimeout: time.Second,
		TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12},
	}
	return &http.Client{Transport: transporte}
}
