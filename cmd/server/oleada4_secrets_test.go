package main

import (
	"encoding/base64"
	"strings"
	"testing"
)

func claveDePruebaOleada4(t *testing.T, byteValue byte) string {
	t.Helper()
	cruda := make([]byte, 32)
	for i := range cruda {
		cruda[i] = byteValue
	}
	return base64.StdEncoding.EncodeToString(cruda)
}

func limpiarClavesOleada4(t *testing.T) {
	t.Helper()
	for _, nombre := range []string{envPushTokenKeyNueva, envPushTokenKeyAlias, envCursorKey} {
		t.Setenv(nombre, "")
	}
}

func TestPushSealerOleada4FallaCerradoSinClave(t *testing.T) {
	limpiarClavesOleada4(t)
	if sealer := pushSealerOleada4(); sealer != nil {
		t.Fatal("sin clave configurada no se debe construir un sellador")
	}
}

func TestPushSealerOleada4AceptaClaveValidaYAlias(t *testing.T) {
	limpiarClavesOleada4(t)
	clave := claveDePruebaOleada4(t, 7)
	t.Setenv(envPushTokenKeyAlias, clave)
	sealer := pushSealerOleada4()
	if sealer == nil {
		t.Fatal("el alias de clave válida debe construir el sellador")
	}
	sobre, err := sealer.Sellar("token-sintetico", "usuario|dispositivo")
	if err != nil || len(sobre) == 0 {
		t.Fatalf("sellar con clave válida falló: %v", err)
	}
}

func TestCursorKeyOleada4FallaCerradoYSoloAcepta32Bytes(t *testing.T) {
	limpiarClavesOleada4(t)
	if clave := claveCursorDesdeEntorno(); clave != nil {
		t.Fatal("sin clave de cursor no se debe devolver una clave efímera")
	}
	t.Setenv(envCursorKey, base64.StdEncoding.EncodeToString([]byte("corta")))
	if clave := claveCursorDesdeEntorno(); clave != nil {
		t.Fatal("una clave de cursor de longitud inválida debe rechazarse")
	}
	clave := claveDePruebaOleada4(t, 11)
	t.Setenv(envCursorKey, clave)
	obtenida := claveCursorDesdeEntorno()
	if len(obtenida) != 32 || strings.TrimSpace(base64.StdEncoding.EncodeToString(obtenida)) == "" {
		t.Fatal("la clave de cursor válida debe tener 32 bytes")
	}
}
