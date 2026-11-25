// Package uuidv8 implementa identificadores UUID versión 8 (RFC 9562 §5.8)
// deterministas para Nexum.
//
// Un UUID v8 es un UUID de contenido definido por la aplicación. Nexum lo
// deriva como SHA-256(namespace || nombre): toma los 16 primeros bytes del
// digest, fija la versión 8 en el nibble alto del byte 6 y la variante RFC 4122
// (10xx) en el byte 8. El resultado es determinista, estable entre procesos,
// independiente del nodo y no contiene secretos.
//
// Se usa SHA-256 y no SHA-1 (UUID v5) de forma deliberada: el contrato de E062
// solo exige `format: uuid` y no impone versión, así que no hay razón para
// introducir un algoritmo débil en el árbol (gosec G401/G505) ni para ocultarlo
// con `#nosec`.
//
// La representación es la canónica 8-4-4-4-12 en minúsculas.
package uuidv8

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

// UUID es el valor binario de 16 bytes de un UUID.
type UUID [16]byte

// ErrFormato indica que la cadena no es un UUID canónico 8-4-4-4-12.
var ErrFormato = errors.New("uuid: formato canonico 8-4-4-4-12 requerido")

// namespaceNexumTexto es el UUID reservado por Nexum como namespace de sus
// identificadores deterministas.
//
// Se calculó una sola vez como los 16 primeros bytes de
// SHA-256("nexum:uuidv8:namespace:v1"), con los bits de versión 8 y variante
// RFC 4122 fijados. Queda congelado: cambiarlo cambiaría todos los
// identificadores derivados. No es un namespace de DNS, URL ni OID de la IANA;
// es propio de este backend.
const namespaceNexumTexto = "e882a8d5-8a71-8fbf-ac40-81f9cac964c7"

// NamespaceNexum es el namespace estable de los identificadores deterministas
// de Nexum. No debe reasignarse: las pruebas de contrato comprueban que el
// valor emitido por New con este namespace es reproducible.
var NamespaceNexum = MustParse(namespaceNexumTexto)

// Parse acepta un UUID canónico con guiones (cualquier versión y variante).
// No acepta la forma compacta de 32 hex: los identificadores nuevos siempre se
// emiten con guiones y el formato estricto evita ambigüedades.
func Parse(texto string) (UUID, error) {
	var u UUID
	limpio := strings.TrimSpace(texto)
	if len(limpio) != 36 ||
		limpio[8] != '-' || limpio[13] != '-' || limpio[18] != '-' || limpio[23] != '-' {
		return u, ErrFormato
	}
	compacto := strings.ReplaceAll(limpio, "-", "")
	if len(compacto) != 32 {
		return u, ErrFormato
	}
	bytes, err := hex.DecodeString(compacto)
	if err != nil || len(bytes) != len(u) {
		return u, ErrFormato
	}
	copy(u[:], bytes)
	return u, nil
}

// MustParse es Parse para constantes de paquete: entra en pánico si el literal
// está mal formado, de modo que un error de programación falla en el arranque y
// no se convierte en identificadores silenciosamente distintos.
func MustParse(texto string) UUID {
	u, err := Parse(texto)
	if err != nil {
		panic(fmt.Sprintf("uuidv8: identificador invalido %q: %v", texto, err))
	}
	return u
}

// String devuelve la representación canónica en minúsculas.
func (u UUID) String() string {
	return fmt.Sprintf("%x-%x-%x-%x-%x", u[0:4], u[4:6], u[6:8], u[8:10], u[10:16])
}

// New deriva un UUID v8 determinista de namespace y nombre.
//
// El mismo par (namespace, nombre) produce siempre el mismo UUID; un byte
// distinto en cualquiera de los dos produce, con probabilidad abrumadora, un
// UUID distinto. New no guarda estado ni consulta relojes.
func New(namespace UUID, nombre string) UUID {
	h := sha256.New()
	h.Write(namespace[:])
	h.Write([]byte(nombre))
	suma := h.Sum(nil)

	var u UUID
	copy(u[:], suma[:16])
	u[6] = (u[6] & 0x0f) | 0x80 // versión 8 (RFC 9562)
	u[8] = (u[8] & 0x3f) | 0x80 // variante RFC 4122 (10xx)
	return u
}

// EsV8 indica si el UUID declara versión 8 y variante RFC 4122. Se usa en las
// pruebas de contrato para no confundir un hash truncado con un UUID real y
// para demostrar que lo emitido no es un UUID v5 basado en SHA-1.
func EsV8(u UUID) bool {
	return u[6]>>4 == 8 && u[8]>>6 == 0b10
}
