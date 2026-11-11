package identity

import (
	"crypto/rand"
	"fmt"
)

// NewID genera un UUID version 4 con entropía criptográfica. Se usa para el usuario y
// para cada entidad creada en el mismo registro (compromisos fijos). El identificador
// satisface `format: uuid` del contrato OpenAPI sin depender de librerías externas.
func NewID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("no se pudo generar el identificador: %w", err)
	}

	// RFC 4122: version 4 y variante 10xx.
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80

	return fmt.Sprintf(
		"%x-%x-%x-%x-%x",
		b[0:4],
		b[4:6],
		b[6:8],
		b[8:10],
		b[10:16],
	), nil
}

// NewUserID conserva el nombre historico del generador de identificadores de usuario.
func NewUserID() (string, error) {
	return NewID()
}
