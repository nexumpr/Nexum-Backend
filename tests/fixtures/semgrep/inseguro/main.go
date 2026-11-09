// Fixture deliberadamente inseguro para la prueba negativa de Semgrep (RFC-008).
// NO se usa en produccion: solo lo analiza el paso de prueba negativa del job
// `sast-semgrep` para demostrar que las reglas reales detectan patrones reales.
// El escaneo principal excluye este directorio mediante `paths.exclude` en
// herramientas/semgrep/nexum.yml (no con banderas del CLI).
package inseguro

import (
	"crypto/md5"
	"crypto/sha1"
	"crypto/tls"
	"net/http"
)

// ConfiguracionSinVerificacion desactiva la validacion del certificado del servidor.
func ConfiguracionSinVerificacion() *tls.Config {
	return &tls.Config{InsecureSkipVerify: true}
}

// HuellaDebilMD5 usa una funcion de hash no segura.
func HuellaDebilMD5(datos []byte) [16]byte {
	return md5.Sum(datos)
}

// HuellaDebilSHA1 usa una funcion de hash no segura.
func HuellaDebilSHA1(datos []byte) [20]byte {
	return sha1.Sum(datos)
}

// DescargaSinTiempo usa el cliente HTTP por defecto, sin timeout.
func DescargaSinTiempo(url string) (*http.Response, error) {
	return http.Get(url)
}
