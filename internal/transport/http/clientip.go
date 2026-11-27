package http

import (
	"net"
	"net/http"
	"net/netip"
	"strings"
)

// maxForwardedHops acota la cadena procesada para evitar entradas manipuladas enormes.
const maxForwardedHops = 16

// ClientIPPolicy documenta la política aplicada para obtener la IP efectiva de los
// endpoints de identidad (E001, E002, E003, E005, E006, E011).
//
// Topología real del proyecto (Terraform I-030):
//
//	cliente -> CloudFront (origin request policy Managed-AllViewer) -> ALB interno -> EC2
//
// Con Managed-AllViewer, CloudFront reenvía las cabeceras del visor y **añade** la IP del
// visor al final de `X-Forwarded-For`; después el ALB añade la IP del salto que vio
// (la del borde de CloudFront). El resultado en el backend es:
//
//	[valores enviados por el cliente..., ip-del-visor, ip-del-borde-cloudfront]
//
// El primer elemento es controlable por el cliente y **no** es de confianza. Tampoco se
// acepta el penúltimo salto de forma ciega: la política exige que la petición provenga
// realmente del proxy interno y que la cadena tenga la forma que produce esa topología.
//
// Reglas (fail-closed cuando la cadena no es verificable):
//  1. El peer inmediato (`RemoteAddr`) debe ser loopback o una dirección privada
//     (RFC1918/ULA). El ALB interno conecta desde la VPC; un peer público no puede
//     ser el proxy de confianza.
//  2. Sin `X-Forwarded-For`, solo se acepta un peer loopback o privado (desarrollo
//     local o llamada interna). Un peer público sin cadena se rechaza.
//  3. Con `X-Forwarded-For`: la lista debe ser íntegramente parseable (máximo 16 saltos),
//     sin elementos vacíos, zonas ni direcciones inválidas.
//  4. Si el peer es loopback (pruebas o proxy local que no añade salto), se toma el
//     último elemento. Si el peer es privado (ALB), se exigen al menos dos elementos:
//     el último lo escribió el ALB y debe ser una dirección pública del borde de
//     CloudFront; el penúltimo es la IP del visor escrita por CloudFront.
//  5. La IP elegida debe ser unicast y no especificada ni multicast.
//
// Esta política no usa rangos de AWS codificados a mano y no hace DNS inverso. Una
// validación criptográfica del origen (cabecera secreta de CloudFront) requeriría un
// cambio de infraestructura y se registra como mejora futura, no como control actual.
func clientIPFromTrustedChain(r *http.Request) (string, bool) {
	if r == nil {
		return "", false
	}

	peer, ok := peerAddr(r.RemoteAddr)
	if !ok {
		return "", false
	}

	forwarded := strings.TrimSpace(r.Header.Get("X-Forwarded-For"))
	if forwarded == "" {
		// Sin cadena de proxies: solo un peer local o interno es aceptable.
		if isTrustedProxyPeer(peer) {
			return normalizarIP(peer), true
		}
		return "", false
	}

	// La cadena solo se evalúa si el peer inmediato es el proxy de confianza.
	if !isTrustedProxyPeer(peer) {
		return "", false
	}

	addrs, ok := parseForwardedFor(forwarded)
	if !ok {
		return "", false
	}

	// Peer loopback: proxy local que puede no añadir un salto. Se acepta el último
	// elemento, que es el cliente inmediato.
	if peer.IsLoopback() {
		elegida := addrs[len(addrs)-1]
		if !isUsableClientAddr(elegida) {
			return "", false
		}
		return normalizarIP(elegida), true
	}

	// Peer privado (ALB interno): la topología CloudFront -> ALB -> EC2 exige al menos
	// dos entradas: la del visor (añadida por CloudFront) y la del borde (añadida por
	// ALB). Un único valor indicaría que la petición no pasó por la cadena completa.
	if len(addrs) < 2 {
		return "", false
	}

	borde := addrs[len(addrs)-1]
	if !isPublicUnicast(borde) {
		return "", false
	}

	visor := addrs[len(addrs)-2]
	if !isUsableClientAddr(visor) {
		return "", false
	}
	return normalizarIP(visor), true
}

// peerAddr extrae la dirección del peer inmediato de `RemoteAddr`.
func peerAddr(remoteAddr string) (netip.Addr, bool) {
	host, _, err := net.SplitHostPort(strings.TrimSpace(remoteAddr))
	if err != nil {
		host = strings.TrimSpace(remoteAddr)
	}
	valor, err := netip.ParseAddr(strings.Trim(host, "[]"))
	if err != nil || valor.Zone() != "" {
		return netip.Addr{}, false
	}
	return valor, true
}

// parseForwardedFor parsea la lista completa y rechaza cualquier elemento inválido o
// vacío: una cadena ambigua no se interpreta a medias.
func parseForwardedFor(cadena string) ([]netip.Addr, bool) {
	partes := strings.Split(cadena, ",")
	if len(partes) == 0 || len(partes) > maxForwardedHops {
		return nil, false
	}

	direcciones := make([]netip.Addr, 0, len(partes))
	for _, parte := range partes {
		valor := strings.TrimSpace(parte)
		if valor == "" {
			return nil, false
		}
		direccion, err := netip.ParseAddr(valor)
		if err != nil || direccion.Zone() != "" {
			return nil, false
		}
		direcciones = append(direcciones, direccion)
	}
	if len(direcciones) == 0 {
		return nil, false
	}
	return direcciones, true
}

// isTrustedProxyPeer indica si la dirección puede ser el proxy interno de confianza:
// loopback (pruebas/proxy local) o privada (ALB dentro de la VPC).
func isTrustedProxyPeer(direccion netip.Addr) bool {
	if !direccion.IsValid() {
		return false
	}
	return direccion.IsLoopback() || direccion.IsPrivate()
}

// isPublicUnicast valida la IP del borde de CloudFront añadida por el ALB: debe ser
// pública, unicast y no reservada para usos locales.
func isPublicUnicast(direccion netip.Addr) bool {
	if !direccion.IsValid() || !direccion.IsGlobalUnicast() {
		return false
	}
	if direccion.IsPrivate() || direccion.IsLoopback() || direccion.IsLinkLocalUnicast() ||
		direccion.IsLinkLocalMulticast() || direccion.IsMulticast() || direccion.IsUnspecified() {
		return false
	}
	return true
}

// isUsableClientAddr descarta direcciones que no identifican a un cliente.
func isUsableClientAddr(direccion netip.Addr) bool {
	if !direccion.IsValid() || !direccion.IsGlobalUnicast() {
		return false
	}
	return !direccion.IsUnspecified() && !direccion.IsMulticast()
}

// normalizarIP devuelve la forma canónica: IPv4 mapeada en IPv6 se reduce a IPv4.
func normalizarIP(direccion netip.Addr) string {
	if direccion.Is4In6() {
		return direccion.Unmap().String()
	}
	return direccion.String()
}
