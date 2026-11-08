// Fixture deliberadamente inseguro para la prueba negativa de gosec (I-100).
// NO se usa en producción: sólo lo analiza el job de seguridad para demostrar
// que gosec detecta un patrón real (G404: math/rand en contexto sensible).
// El escaneo principal de gosec excluye este directorio.
package gosecinseguro

import "math/rand"

// NumeroInseguro usa un generador pseudoaleatorio no criptográfico.
func NumeroInseguro() int {
	return rand.Intn(100)
}
