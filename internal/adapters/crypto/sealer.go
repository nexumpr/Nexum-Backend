package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
)

// Sealer cifra el token de recuperacion para el reenvio (AES-256-GCM).
//
// La clave llega en base64 (32 bytes) desde el bundle RFC-004 (A9,
// PASSWORD_RESET_SEAL_KEY_B64). El AAD aprobado es "<userID>|<outboxID>".
type Sealer struct {
	aead cipher.AEAD
}

// ErrClaveInvalida indica una clave ausente o con longitud incorrecta.
var ErrClaveInvalida = errors.New("clave de sellado invalida")

// NuevoSealer construye el sellador. Falla cerrado si la clave no es de 32 bytes.
func NuevoSealer(claveB64 string) (*Sealer, error) {
	crudo, err := base64.StdEncoding.DecodeString(strings.TrimSpace(claveB64))
	if err != nil || len(crudo) != 32 {
		return nil, ErrClaveInvalida
	}
	bloque, err := aes.NewCipher(crudo)
	if err != nil {
		return nil, ErrClaveInvalida
	}
	aead, err := cipher.NewGCM(bloque)
	if err != nil {
		return nil, ErrClaveInvalida
	}
	return &Sealer{aead: aead}, nil
}

// Sellar devuelve nonce||ciphertext. El AAD debe ser identico al de Abrir.
func (s *Sealer) Sellar(token, aad string) ([]byte, error) {
	if s == nil || s.aead == nil {
		return nil, ErrClaveInvalida
	}
	nonce := make([]byte, s.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return s.aead.Seal(nonce, nonce, []byte(token), []byte(aad)), nil
}

// Abrir descifra y verifica el AAD. Un sobre de otro usuario falla.
func (s *Sealer) Abrir(sobre []byte, aad string) (string, error) {
	if s == nil || s.aead == nil {
		return "", ErrClaveInvalida
	}
	n := s.aead.NonceSize()
	if len(sobre) <= n {
		return "", fmt.Errorf("sobre de token invalido")
	}
	claro, err := s.aead.Open(nil, sobre[:n], sobre[n:], []byte(aad))
	if err != nil {
		return "", fmt.Errorf("no se pudo abrir el token: %w", err)
	}
	return string(claro), nil
}
