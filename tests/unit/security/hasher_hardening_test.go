package security_test

import (
	"encoding/base64"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/nexumpr/Nexum-Backend/internal/infrastructure/security"
)

// Pruebas de endurecimiento de A-050 (D-10 / gosec G115):
// las entradas malformadas o con parámetros peligrosos deben rechazarse ANTES
// de invocar Argon2, sin pánicos y sin consumir recursos desproporcionados.

func hasherPrueba() *security.Argon2Hasher {
	return security.NewArgon2Hasher(nil) // parámetros oficiales (m=64MiB, t=3, p=2)
}

func TestHashYVerificacionValidos(t *testing.T) {
	h := hasherPrueba()

	encoded, err := h.HashPassword("ClaveSegura-2026!")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if !strings.HasPrefix(encoded, "$argon2id$v=19$m=65536,t=3,p=2$") {
		t.Fatalf("formato inesperado: %s", encabezado(encoded))
	}

	ok, err := h.VerifyPassword("ClaveSegura-2026!", encoded)
	if err != nil || !ok {
		t.Fatalf("hash válido rechazado: ok=%v err=%v", ok, err)
	}

	ok, err = h.VerifyPassword("ClaveIncorrecta", encoded)
	if ok {
		t.Fatal("contraseña incorrecta aceptada")
	}
	if err == nil {
		t.Fatal("contraseña incorrecta no reportó error de credenciales")
	}
}

func encabezado(encoded string) string {
	partes := strings.Split(encoded, "$")
	if len(partes) < 4 {
		return encoded
	}
	return strings.Join(partes[:4], "$")
}

func hashCon(memory, iterations uint32, parallelism uint8, saltB64, digestB64 string) string {
	return fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s", memory, iterations, parallelism, saltB64, digestB64)
}

func salValida() string {
	return base64.RawStdEncoding.EncodeToString([]byte("0123456789abcdef"))
}

func digestValido() string {
	return base64.RawStdEncoding.EncodeToString(make([]byte, 32))
}

func TestRechazoDeEstructura(t *testing.T) {
	h := hasherPrueba()

	casos := map[string]string{
		"vacio":                "",
		"sin separadores":      "argon2id",
		"partes insuficientes": "$argon2id$v=19$m=65536,t=3,p=2$abc",
		"algoritmo erroneo":    strings.Replace(hashCon(65536, 3, 2, salValida(), digestValido()), "argon2id", "sha256", 1),
		"version erronea":      strings.Replace(hashCon(65536, 3, 2, salValida(), digestValido()), "v=19", "v=16", 1),
		"parametros corruptos": "$argon2id$v=19$m=x,t=y,p=z$" + salValida() + "$" + digestValido(),
		"hash gigantesco":      "$argon2id$v=19$m=65536,t=3,p=2$" + strings.Repeat("A", 5000),
	}
	for nombre, encoded := range casos {
		if _, err := h.VerifyPassword("clave", encoded); err == nil {
			t.Errorf("%s: se esperaba error y se aceptó", nombre)
		}
	}
}

func TestRechazoDeParametrosPeligrososAntesDeArgon2(t *testing.T) {
	h := hasherPrueba()

	casos := []struct {
		nombre      string
		memory      uint32
		iterations  uint32
		parallelism uint8
	}{
		{"memoria excesiva", 4294967295, 3, 2},
		{"memoria insuficiente", 1024, 3, 2},
		{"iteraciones excesivas", 65536, 4294967295, 2},
		{"iteraciones cero", 65536, 0, 2},
		{"paralelismo excesivo", 65536, 3, 255},
		{"paralelismo cero", 65536, 3, 0},
	}

	for _, caso := range casos {
		encoded := hashCon(caso.memory, caso.iterations, caso.parallelism, salValida(), digestValido())
		inicio := time.Now()
		_, err := h.VerifyPassword("clave", encoded)
		transcurrido := time.Since(inicio)

		if err == nil {
			t.Errorf("%s: se esperaba rechazo", caso.nombre)
			continue
		}
		// Debe rechazarse por validación (rápido), no tras derivar la clave.
		if transcurrido > 250*time.Millisecond {
			t.Errorf("%s: tardó %v; Argon2 se ejecutó antes de validar", caso.nombre, transcurrido)
		}
	}
}

func TestRechazoDeLongitudesDeSalYDigest(t *testing.T) {
	h := hasherPrueba()

	salCorta := base64.RawStdEncoding.EncodeToString([]byte("corta"))
	digestCorto := base64.RawStdEncoding.EncodeToString(make([]byte, 8))
	digestLargo := base64.RawStdEncoding.EncodeToString(make([]byte, 128))

	casos := map[string]string{
		"sal demasiado corta":    hashCon(65536, 3, 2, salCorta, digestValido()),
		"digest demasiado corto": hashCon(65536, 3, 2, salValida(), digestCorto),
		"digest demasiado largo": hashCon(65536, 3, 2, salValida(), digestLargo),
	}
	for nombre, encoded := range casos {
		if _, err := h.VerifyPassword("clave", encoded); err == nil {
			t.Errorf("%s: se esperaba rechazo", nombre)
		}
	}
}

func TestCompatibilidadConHashExistenteFormatoCanonico(t *testing.T) {
	// Hash generado con parámetros oficiales: debe seguir verificándose tras el
	// endurecimiento (no se cambió formato ni parámetros por defecto).
	h := hasherPrueba()
	encoded, err := h.HashPassword("Compatibilidad-2026")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}

	// Se reconstruye explícitamente el formato canónico con los valores por defecto.
	partes := strings.Split(encoded, "$")
	if len(partes) != 6 {
		t.Fatalf("formato no canónico: %d partes", len(partes))
	}
	reconstruido := hashCon(65536, 3, 2, partes[4], partes[5])

	ok, err := h.VerifyPassword("Compatibilidad-2026", reconstruido)
	if err != nil || !ok {
		t.Fatalf("hash canónico rechazado: ok=%v err=%v", ok, err)
	}
}

func TestEntradasNoProvocanPanico(t *testing.T) {
	h := hasherPrueba()
	entradas := []string{
		"$", "$$$$$", "$argon2id$", "$argon2id$v=19$",
		"$argon2id$v=19$m=65536,t=3,p=2$$",
		"$argon2id$v=19$m=65536,t=3,p=2$!!!$???",
		strings.Repeat("$", 200),
	}
	for _, entrada := range entradas {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("pánico con entrada %q: %v", entrada, r)
				}
			}()
			_, _ = h.VerifyPassword("clave", entrada)
		}()
	}
}
