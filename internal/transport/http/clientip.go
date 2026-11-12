package http

import (
	"net"
	"net/http"
	"net/netip"
	"strings"
)

// maxForwardedHops acota la cadena procesada para evitar entradas manipuladas enormes.
const maxForwardedHops = 16

// ClientIPPolicy documenta la política aplicada para obtener la IP efectiva de E002.
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
// Por tanto el primer elemento es controlable por el cliente y **no** es de confianza.
// La IP efectiva verificable es la que queda inmediatamente antes del último salto, que es
// la que escribió CloudFront. Si solo hay un elemento, se acepta como IP del visor porque
// CloudFront sustituyó la cabecera.
//
// Esta política no usa rangos de AWS codificados a mano y no hace DNS inverso.
func clientIPFromTrustedChain(r *http.Request) (string, bool) {
	if r == nil {
		return "", false
	}

	forwarded := r.Header.Get("X-Forwarded-For")
	if forwarded != "" {
		// La cabecera existe: es la única fuente aceptable. Si la cadena es inválida o
		// ambigua NO se cae al peer, porque agruparía a todos los clientes afectados en la
		// misma cuota compartida y permitiría evadirse manipulando la cabecera.
		return viewerIPFromForwardedFor(forwarded)
	}

	// Sin cadena de proxies: el peer inmediato es la única fuente disponible.
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err != nil {
		host = strings.TrimSpace(r.RemoteAddr)
	}
	valor, err := netip.ParseAddr(strings.Trim(host, "[]"))
	if err != nil {
		return "", false
	}
	return normalizarIP(valor), true
}

// viewerIPFromForwardedFor extrae la IP del visor de la cadena X-Forwarded-For aplicando
// la política documentada arriba. Devuelve false si la cadena es inválida o ambigua.
func viewerIPFromForwardedFor(cadena string) (string, bool) {
	partes := strings.Split(cadena, ",")
	if len(partes) == 0 || len(partes) > maxForwardedHops {
		return "", false
	}

	limpias := make([]netip.Addr, 0, len(partes))
	for _, parte := range partes {
		valor := strings.TrimSpace(parte)
		if valor == "" {
			continue
		}
		direccion, err := netip.ParseAddr(valor)
		if err != nil {
			// Un elemento malformado invalida la cadena completa: no se adivina.
			return "", false
		}
		limpias = append(limpias, direccion)
	}
	if len(limpias) == 0 {
		return "", false
	}

	// El último elemento lo escribió el ALB (IP del borde de CloudFront); el anterior es
	// la IP del visor escrita por CloudFront. Con un solo elemento, es el visor.
	elegida := limpias[len(limpias)-1]
	if len(limpias) >= 2 {
		elegida = limpias[len(limpias)-2]
	}
	if elegida.IsUnspecified() || elegida.IsMulticast() {
		return "", false
	}
	return normalizarIP(elegida), true
}

// normalizarIP devuelve la forma canónica: IPv4 mapeada en IPv6 se reduce a IPv4.
func normalizarIP(direccion netip.Addr) string {
	if direccion.Is4In6() {
		return direccion.Unmap().String()
	}
	return direccion.String()
}
