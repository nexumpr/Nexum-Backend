package security

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"github.com/nexumpr/Nexum-Backend/internal/ports"
	"golang.org/x/crypto/argon2"
)

// Argon2Params contiene los parámetros criptográficos recomendados (A-050 §2.1).
type Argon2Params struct {
	Memory      uint32
	Iterations  uint32
	Parallelism uint8
	SaltLength  uint32
	KeyLength   uint32
}

// DefaultArgon2Params parámetros oficiales para Nexum Backend (m=64MiB, t=3, p=2, keyLen=32).
var DefaultArgon2Params = Argon2Params{
	Memory:      64 * 1024, // 64 MiB
	Iterations:  3,
	Parallelism: 2,
	SaltLength:  16,
	KeyLength:   32,
}

// Guardas anti-DoS aplicadas a los parámetros que llegan dentro del hash
// almacenado. Los valores por defecto de A-050 (m=64 MiB, t=3, p=2, keyLen=32)
// caen dentro de estos límites; ampliarlos exige revisión de A-050 (RFC), no un
// cambio silencioso aquí.
const (
	minMemoryKiB         uint32 = 8 * 1024   // 8 MiB
	maxMemoryKiB         uint32 = 256 * 1024 // 256 MiB
	minIterations        uint32 = 1
	maxIterations        uint32 = 10
	minParallelism       uint8  = 1
	maxParallelism       uint8  = 16
	minSaltBytes                = 8
	maxSaltBytes                = 64
	maxEncodedHashLength        = 4096
)

// allowedKeyLengths evita la conversión int->uint32: la longitud del digest se
// valida contra constantes ya tipadas (corrige gosec G115 sin exclusiones).
var allowedKeyLengths = map[int]uint32{
	16: 16,
	24: 24,
	32: 32,
	48: 48,
	64: 64,
}

// validateArgon2Parameters rechaza parámetros peligrosos ANTES de invocar Argon2,
// evitando agotamiento de memoria/CPU con hashes manipulados.
func validateArgon2Parameters(memory, iterations uint32, parallelism uint8, saltLen, digestLen int) error {
	if memory < minMemoryKiB || memory > maxMemoryKiB {
		return errors.New("parametros de argon2 fuera de rango: memoria")
	}
	if iterations < minIterations || iterations > maxIterations {
		return errors.New("parametros de argon2 fuera de rango: iteraciones")
	}
	if parallelism < minParallelism || parallelism > maxParallelism {
		return errors.New("parametros de argon2 fuera de rango: paralelismo")
	}
	if saltLen < minSaltBytes || saltLen > maxSaltBytes {
		return errors.New("sal de argon2 con longitud invalida")
	}
	if _, ok := allowedKeyLengths[digestLen]; !ok {
		return errors.New("digest de argon2 con longitud invalida")
	}
	return nil
}

// Argon2Hasher implementa el puerto ports.PasswordHasher usando el algoritmo Argon2id.
type Argon2Hasher struct {
	params Argon2Params
}

// NewArgon2Hasher crea un nuevo hasher con parámetros configurables.
func NewArgon2Hasher(params *Argon2Params) *Argon2Hasher {
	p := DefaultArgon2Params
	if params != nil {
		p = *params
	}
	return &Argon2Hasher{params: p}
}

// HashPassword genera un hash seguro Argon2id con salt aleatoria per-user.
// Formato canónico: $argon2id$v=19$m=<m>,t=<t>,p=<p>$<salt>$<hash>
func (h *Argon2Hasher) HashPassword(password string) (string, error) {
	if password == "" {
		return "", errors.New("la contrasena no puede estar vacia")
	}

	salt := make([]byte, h.params.SaltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("error al generar sal criptografica: %w", err)
	}

	hash := argon2.IDKey(
		[]byte(password),
		salt,
		h.params.Iterations,
		h.params.Memory,
		h.params.Parallelism,
		h.params.KeyLength,
	)

	b64Salt := base64.RawStdEncoding.EncodeToString(salt)
	b64Hash := base64.RawStdEncoding.EncodeToString(hash)

	encoded := fmt.Sprintf(
		"$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version,
		h.params.Memory,
		h.params.Iterations,
		h.params.Parallelism,
		b64Salt,
		b64Hash,
	)

	return encoded, nil
}

// VerifyPassword compara una contraseña en texto claro contra el hash Argon2id en tiempo constante.
func (h *Argon2Hasher) VerifyPassword(password, encodedHash string) (bool, error) {
	if password == "" || encodedHash == "" {
		return false, ports.ErrInvalidPassword
	}

	// Cota de entrada antes de parsear: un hash legítimo es mucho más corto.
	if len(encodedHash) > maxEncodedHashLength {
		return false, ports.ErrInvalidPassword
	}

	parts := strings.Split(encodedHash, "$")
	if len(parts) != 6 {
		return false, ports.ErrInvalidPassword
	}

	if parts[1] != "argon2id" {
		return false, errors.New("algoritmo no soportado: se requiere argon2id")
	}

	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return false, errors.New("version de argon2 incompatible")
	}

	var memory, iterations uint32
	var parallelism uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &iterations, &parallelism); err != nil {
		return false, errors.New("parametros de hash corruptos")
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false, ports.ErrInvalidPassword
	}

	expectedHash, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false, ports.ErrInvalidPassword
	}

	// Validación de política y longitudes ANTES de derivar la clave: evita
	// agotamiento de recursos y la conversión insegura int->uint32 (G115).
	if err := validateArgon2Parameters(memory, iterations, parallelism, len(salt), len(expectedHash)); err != nil {
		return false, err
	}
	keyLen := allowedKeyLengths[len(expectedHash)]

	computedHash := argon2.IDKey(
		[]byte(password),
		salt,
		iterations,
		memory,
		parallelism,
		keyLen,
	)

	if subtle.ConstantTimeCompare(expectedHash, computedHash) == 1 {
		return true, nil
	}

	return false, ports.ErrInvalidPassword
}
