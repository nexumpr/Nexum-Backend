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
// Topología real (Terraform I-010/I-030, verificada el 2026-11-27):
//
//	cliente -> CloudFront (origin request policy Managed-AllViewer)
//	       -> VPC Origin (ENI administrada por CloudFront en subred privada)
//	       -> ALB interno HTTP:80 (SG solo desde el prefijo CloudFront)
//	       -> EC2 Go:8080 (SG solo desde el SG del ALB)
//
// Con Managed-AllViewer, CloudFront appende la IP del visor al final de
// `X-Forwarded-For`. Después, el ALB appende la IP del salto que vio. En una VPC Origin
// ese salto es la **ENI privada** de CloudFront, no una IP pública del borde; por eso el
// último elemento de XFF puede ser privado y no se exige que sea público.
//
// **No se confía en `CloudFront-Viewer-Address`.** Managed-AllViewer reenvía cabeceras
// del visor y no garantiza que esa cabecera sea generada y sobrescrita por CloudFront;
// un cliente podría enviarla. Solo se usa la posición de XFF que añaden CloudFront y el
// ALB, protegida por la topología (ALB solo desde el prefijo CloudFront y backend solo
// desde el ALB).
//
// Fuente única: `X-Forwarded-For`.
//   - El último elemento lo añade el ALB (no controlable por el cliente).
//   - El penúltimo lo añade CloudFront con la IP del visor.
//   - El primer elemento y los anteriores son controlables y nunca se usan.
//   - Sin XFF: solo se acepta un peer loopback (desarrollo local). Un peer privado sin
//     cadena se rechaza; un peer público nunca es el proxy de confianza.
//
// Reglas de seguridad:
//   - El peer inmediato debe ser loopback o privado (ALB dentro de la VPC).
//   - Se rechazan cabeceras XFF duplicadas (ambigüedad de parseo).
//   - La cadena debe ser íntegra (≤16 saltos, sin vacíos, zonas ni valores inválidos).
//   - IPv4 mapeada en IPv6 (`::ffff:a.b.c.d`) se normaliza **antes** de validar; de lo
//     contrario una dirección privada mapeada podría superar el filtro de “público”.
//   - Con peer privado se exigen al menos dos elementos: el último es el salto que vio
//     el ALB (público o privado, pero unicast) y el penúltimo es el visor.
//   - La IP elegida debe ser unicast, no especificada y no multicast.
//
// No se codifican rangos de AWS ni IP efímeras: la confianza se deriva de la topología
// (SG del ALB restringido al prefijo CloudFront y SG del backend restringido al ALB) y
// del orden de appends que documenta AWS. Una validación criptográfica del origen
// (cabecera secreta de CloudFront) requeriría Terraform y un Gate de despliegue.
func clientIPFromTrustedChain(r *http.Request) (string, bool) {
	if r == nil {
		return "", false
	}

	peer, ok := peerAddr(r.RemoteAddr)
	if !ok {
		return "", false
	}

	values := r.Header.Values("X-Forwarded-For")
	if len(values) > 1 {
		// Varias cabeceras XFF permiten interpretaciones divergentes entre proxies.
		return "", false
	}
	forwarded := ""
	if len(values) == 1 {
		forwarded = strings.TrimSpace(values[0])
	}

	if forwarded == "" {
		if peer.IsLoopback() {
			return normalizarIP(peer), true
		}
		// Un peer interno sin cadena no acredita el paso por CloudFront.
		return "", false
	}

	if !isTrustedProxyPeer(peer) {
		return "", false
	}

	addrs, ok := parseForwardedFor(forwarded)
	if !ok {
		return "", false
	}

	// Peer loopback: proxy local que puede no añadir un salto.
	if peer.IsLoopback() {
		elegida := addrs[len(addrs)-1]
		if !isUsableClientAddr(elegida) {
			return "", false
		}
		return normalizarIP(elegida), true
	}

	// Peer privado (ALB): se exige la cadena completa visor + salto visto por ALB.
	if len(addrs) < 2 {
		return "", false
	}

	saltoALB := addrs[len(addrs)-1]
	if !isUnicastHop(saltoALB) {
		return "", false
	}

	visor := addrs[len(addrs)-2]
	if !isPublicViewer(visor) {
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
// vacío: una cadena ambigua no se interpreta a medias. Normaliza IPv4 mapeada en IPv6
// antes de que el llamador aplique los filtros de addressing.
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
		direcciones = append(direcciones, direccion.Unmap())
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

// isUnicastHop valida el salto añadido por el ALB. Con VPC Origin es la ENI privada de
// CloudFront; con un origen público sería una IP del borde. En ambos casos debe ser una
// dirección unicast y no reservada para usos locales.
func isUnicastHop(direccion netip.Addr) bool {
	if !direccion.IsValid() || !direccion.IsGlobalUnicast() {
		return false
	}
	return !direccion.IsLoopback() && !direccion.IsLinkLocalUnicast() &&
		!direccion.IsLinkLocalMulticast() && !direccion.IsMulticast() && !direccion.IsUnspecified()
}

// isPublicViewer valida la IP del visor: CloudFront la obtiene de una conexión de
// Internet, por lo que debe ser pública (no RFC1918/ULA). Se usa tanto para la cabecera
// CloudFront-Viewer-Address como para el penúltimo salto de XFF.
func isPublicViewer(direccion netip.Addr) bool {
	return isUnicastHop(direccion) && !direccion.IsPrivate()
}

// isUsableClientAddr descarta direcciones que no identifican a un cliente. Se usa solo
// en el camino loopback (pruebas o proxy local), donde una IP privada es legítima.
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
