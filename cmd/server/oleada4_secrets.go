package main

import (
	"encoding/base64"
	"errors"
	"os"
	"strings"

	cryptoAdapter "github.com/nexumpr/Nexum-Backend/internal/adapters/crypto"
	appPushDevices "github.com/nexumpr/Nexum-Backend/internal/application/pushdevices"
)

const (
	envPushSelladorKeyNueva = "NEXUM_PUSH_TOKEN_KEY_B64"
	envPushSelladorKeyAlias = "NEXUM_PUSH_TOKEN_KEY"
	envCursorKey            = "NEXUM_NOTIFICATION_CURSOR_KEY_B64"
)

// claveBase64Oleada4 devuelve la primera variable configurada, sin incluir
// su valor en errores ni logs.
func claveBase64Oleada4(nombres ...string) ([]byte, error) {
	for _, nombre := range nombres {
		codificada := strings.TrimSpace(os.Getenv(nombre))
		if codificada == "" {
			continue
		}
		cruda, err := base64.StdEncoding.DecodeString(codificada)
		if err != nil || len(cruda) != 32 {
			return nil, errors.New("clave inválida: se requieren 32 bytes codificados en base64 estándar")
		}
		return cruda, nil
	}
	return nil, errors.New("clave no configurada")
}

// pushSealerOleada4 construye el sellador AES-GCM usado por E059/E060 y por el
// worker FCM. Sin clave válida devuelve nil y los endpoints fallan cerrados.
func pushSealerOleada4() appPushDevices.TokenSealer {
	cruda, err := claveBase64Oleada4(envPushSelladorKeyNueva, envPushSelladorKeyAlias)
	if err != nil {
		return nil
	}
	sealer, err := cryptoAdapter.NuevoSealer(base64.StdEncoding.EncodeToString(cruda))
	if err != nil {
		return nil
	}
	return sealer
}

// claveCursorDesdeEntorno carga la clave HMAC estable de los cursores de E061.
// La devuelve como copia para que ningún handler pueda mutar la configuración.
func claveCursorDesdeEntorno() []byte {
	cruda, err := claveBase64Oleada4(envCursorKey)
	if err != nil {
		return nil
	}
	return cruda
}
