package pushdevices_test

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	adaptersCrypto "github.com/nexumpr/Nexum-Backend/internal/adapters/crypto"
	"github.com/nexumpr/Nexum-Backend/internal/application/pushdevices"
)

var e059Ahora = time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)

// memoriaPushDevices simula las garantías del adaptador DynamoDB sin AWS:
// idempotencia por usuario/clave, rotación por huella y baja destructiva.
type memoriaPushDevices struct {
	dispositivos  map[string]*pushdevices.Device
	idem          map[string]*pushdevices.IdempotencyRecord
	fallaLoad     error
	fallaRegister error
}

func nuevaMemoriaPushDevices() *memoriaPushDevices {
	return &memoriaPushDevices{
		dispositivos: map[string]*pushdevices.Device{},
		idem:         map[string]*pushdevices.IdempotencyRecord{},
	}
}

func claveDispositivo(userID, id string) string { return userID + "|" + id }
func claveIdem(userID, key string) string       { return userID + "|" + key }

func (m *memoriaPushDevices) LoadIdempotency(_ context.Context, userID, key string, now time.Time) (*pushdevices.IdempotencyRecord, error) {
	if m.fallaLoad != nil {
		return nil, m.fallaLoad
	}
	registro := m.idem[claveIdem(userID, key)]
	if registro == nil || now.Unix() >= registro.ExpiresAt {
		return nil, nil
	}
	copia := *registro
	return &copia, nil
}

func (m *memoriaPushDevices) FindActiveByLabel(_ context.Context, userID, provider, label string) (*pushdevices.Device, error) {
	for clave, dispositivo := range m.dispositivos {
		if !strings.HasPrefix(clave, userID+"|") || !dispositivo.Active ||
			dispositivo.Provider != provider || dispositivo.DeviceLabel != label {
			continue
		}
		copia := *dispositivo
		return &copia, nil
	}
	return nil, nil
}

func (m *memoriaPushDevices) Register(_ context.Context, write pushdevices.RegisterWrite) error {
	if m.fallaRegister != nil {
		return m.fallaRegister
	}
	if m.idem[claveIdem(write.UserID, write.IdempotencyKey)] != nil {
		return pushdevices.ErrIdempotencyCondition
	}
	clave := claveDispositivo(write.UserID, write.Device.ID)
	if actual := m.dispositivos[clave]; actual != nil && actual.TokenFingerprint != write.PreviousFingerprint {
		return pushdevices.ErrDeviceCondition
	}
	copia := write.Device
	m.dispositivos[clave] = &copia
	m.idem[claveIdem(write.UserID, write.IdempotencyKey)] = &pushdevices.IdempotencyRecord{
		DeviceID:     write.Device.ID,
		PayloadHash:  write.PayloadHash,
		ResponseJSON: append([]byte(nil), write.ResponseJSON...),
		ExpiresAt:    write.Now.Add(write.TTL).Unix(),
	}
	return nil
}

func (m *memoriaPushDevices) GetDevice(_ context.Context, userID, deviceID string) (*pushdevices.Device, error) {
	dispositivo := m.dispositivos[claveDispositivo(userID, deviceID)]
	if dispositivo == nil {
		return nil, nil
	}
	copia := *dispositivo
	return &copia, nil
}

func (m *memoriaPushDevices) Revoke(_ context.Context, userID, deviceID string, _ time.Time) (bool, error) {
	dispositivo := m.dispositivos[claveDispositivo(userID, deviceID)]
	if dispositivo == nil || !dispositivo.Active {
		return false, nil
	}
	dispositivo.Active = false
	dispositivo.TokenCipher = nil
	dispositivo.TokenFingerprint = ""
	return true, nil
}

// selladorSintetico genera una clave AES de 32 bytes en memoria. La clave no se
// imprime ni se persiste; el sellador es el adaptador real de AES-GCM.
func selladorSintetico(t *testing.T) *adaptersCrypto.Sealer {
	t.Helper()
	crudo := make([]byte, 32)
	if _, err := rand.Read(crudo); err != nil {
		t.Fatalf("no se pudo generar la clave sintetica: %v", err)
	}
	sealer, err := adaptersCrypto.NuevoSealer(base64.StdEncoding.EncodeToString(crudo))
	if err != nil {
		t.Fatalf("NuevoSealer: %v", err)
	}
	return sealer
}

func servicioE059(m *memoriaPushDevices, sealer pushdevices.TokenSealer) *pushdevices.Service {
	return pushdevices.NewService(m, sealer).WithClock(func() time.Time { return e059Ahora })
}

func cmdE059(userID, key, token, label string) pushdevices.RegisterCommand {
	return pushdevices.RegisterCommand{
		UserID: userID, IdempotencyKey: key, Provider: pushdevices.ProviderFCM,
		Token: token, DeviceLabel: label,
	}
}

func huella(token string) string {
	suma := sha256.Sum256([]byte(token))
	return hex.EncodeToString(suma[:])
}

func TestE059ServicioRegistroNuevo(t *testing.T) {
	memoria := nuevaMemoriaPushDevices()
	sealer := selladorSintetico(t)
	resultado, err := servicioE059(memoria, sealer).Register(context.Background(),
		cmdE059("user-a", "clave-idempotencia-0001", "token-sintetico-1234", "Android prueba"))
	if err != nil {
		t.Fatalf("registro: %v", err)
	}
	if resultado.Replayed || resultado.Device.ID == "" || resultado.Device.Provider != pushdevices.ProviderFCM {
		t.Fatalf("resultado fuera de contrato: %+v", resultado)
	}
	if !resultado.Device.RegisteredAt.Equal(e059Ahora) || !resultado.Device.Active {
		t.Fatalf("marca temporal o estado incorrecto: %+v", resultado.Device)
	}

	var cuerpo map[string]any
	if err := json.Unmarshal(resultado.ResponseJSON, &cuerpo); err != nil {
		t.Fatalf("responseJSON no es JSON: %v", err)
	}
	if len(cuerpo) != 4 || cuerpo["id"] != resultado.Device.ID || cuerpo["provider"] != "fcm" ||
		cuerpo["deviceLabel"] != "Android prueba" || cuerpo["registeredAt"] != e059Ahora.Format(time.RFC3339Nano) {
		t.Fatalf("esquema PushDevice incorrecto: %+v", cuerpo)
	}
	if strings.Contains(string(resultado.ResponseJSON), "token-sintetico-1234") ||
		strings.Contains(string(resultado.ResponseJSON), "tokenCipher") {
		t.Fatal("la respuesta expone el token o el cifrado")
	}

	guardado := memoria.dispositivos[claveDispositivo("user-a", resultado.Device.ID)]
	if guardado == nil || guardado.TokenFingerprint != huella("token-sintetico-1234") {
		t.Fatalf("huella no persistida: %+v", guardado)
	}
	if strings.Contains(string(guardado.TokenCipher), "token-sintetico-1234") {
		t.Fatal("el token quedó en claro en tokenCipher")
	}
	abierto, err := sealer.Abrir(guardado.TokenCipher, "user-a|"+resultado.Device.ID)
	if err != nil || abierto != "token-sintetico-1234" {
		t.Fatalf("AAD incorrecto o cifrado invalido: %q %v", abierto, err)
	}
}

func TestE059ServicioReplayMismaClave(t *testing.T) {
	memoria := nuevaMemoriaPushDevices()
	servicio := servicioE059(memoria, selladorSintetico(t))
	cmd := cmdE059("user-a", "clave-idempotencia-0002", "token-sintetico-2222", "iPhone")
	primero, err := servicio.Register(context.Background(), cmd)
	if err != nil {
		t.Fatalf("primer registro: %v", err)
	}
	segundo, err := servicio.Register(context.Background(), cmd)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if !segundo.Replayed || string(segundo.ResponseJSON) != string(primero.ResponseJSON) ||
		segundo.Device.ID != primero.Device.ID {
		t.Fatalf("replay inestable: %+v", segundo)
	}
	if len(memoria.dispositivos) != 1 || len(memoria.idem) != 1 {
		t.Fatalf("el replay creó efectos adicionales: %d dispositivos, %d idem", len(memoria.dispositivos), len(memoria.idem))
	}
}

func TestE059ServicioClaveRepetidaPayloadDistinto(t *testing.T) {
	memoria := nuevaMemoriaPushDevices()
	servicio := servicioE059(memoria, selladorSintetico(t))
	if _, err := servicio.Register(context.Background(),
		cmdE059("user-a", "clave-idempotencia-0003", "token-sintetico-3333", "iPhone")); err != nil {
		t.Fatalf("primer registro: %v", err)
	}
	_, err := servicio.Register(context.Background(),
		cmdE059("user-a", "clave-idempotencia-0003", "token-sintetico-4444", "iPhone"))
	if !errors.Is(err, pushdevices.ErrConflict) {
		t.Fatalf("esperado ErrConflict, obtenido %v", err)
	}
}

func TestE059ServicioRotacionReutilizaID(t *testing.T) {
	memoria := nuevaMemoriaPushDevices()
	sealer := selladorSintetico(t)
	servicio := servicioE059(memoria, sealer)
	primero, err := servicio.Register(context.Background(),
		cmdE059("user-a", "clave-idempotencia-0004", "token-sintetico-aaaa", "Android prueba"))
	if err != nil {
		t.Fatalf("primer registro: %v", err)
	}
	rotado, err := servicio.Register(context.Background(),
		cmdE059("user-a", "clave-idempotencia-0005", "token-sintetico-bbbb", "Android prueba"))
	if err != nil {
		t.Fatalf("rotacion: %v", err)
	}
	if rotado.Device.ID != primero.Device.ID {
		t.Fatalf("la rotacion cambió el ID: %s -> %s", primero.Device.ID, rotado.Device.ID)
	}
	if len(memoria.dispositivos) != 1 || len(memoria.idem) != 2 {
		t.Fatalf("rotación incorrecta: %d dispositivos, %d idem", len(memoria.dispositivos), len(memoria.idem))
	}
	guardado := memoria.dispositivos[claveDispositivo("user-a", primero.Device.ID)]
	if guardado.TokenFingerprint != huella("token-sintetico-bbbb") ||
		strings.Contains(string(guardado.TokenCipher), "token-sintetico-bbbb") {
		t.Fatalf("el token rotado no se reemplazó de forma segura: %+v", guardado)
	}
	abierto, err := sealer.Abrir(guardado.TokenCipher, "user-a|"+primero.Device.ID)
	if err != nil || abierto != "token-sintetico-bbbb" {
		t.Fatalf("el sobre de rotación usa un AAD incorrecto: %q %v", abierto, err)
	}
}

func TestE059ServicioSinTitular(t *testing.T) {
	servicio := servicioE059(nuevaMemoriaPushDevices(), selladorSintetico(t))
	_, err := servicio.Register(context.Background(), cmdE059("", "clave-idempotencia-0006", "token-sintetico-cccc", "iPhone"))
	if !errors.Is(err, pushdevices.ErrUnauthorized) {
		t.Fatalf("esperado ErrUnauthorized, obtenido %v", err)
	}
}

func TestE059ServicioSinSealerOSinStore(t *testing.T) {
	memoria := nuevaMemoriaPushDevices()
	if _, err := pushdevices.NewService(memoria, nil).Register(context.Background(),
		cmdE059("user-a", "clave-idempotencia-0007", "token-sintetico-dddd", "iPhone")); !errors.Is(err, pushdevices.ErrUnavailable) {
		t.Fatalf("sin sellador: %v", err)
	}
	if _, err := pushdevices.NewService(nil, selladorSintetico(t)).Register(context.Background(),
		cmdE059("user-a", "clave-idempotencia-0007", "token-sintetico-dddd", "iPhone")); !errors.Is(err, pushdevices.ErrUnavailable) {
		t.Fatalf("sin store: %v", err)
	}
	if err := pushdevices.NewService(nil, nil).Revoke(context.Background(), "user-a", "018f90ef-08e5-7a68-b09f-2b67a47c92f0"); !errors.Is(err, pushdevices.ErrUnavailable) {
		t.Fatalf("baja sin dependencias: %v", err)
	}
}

func TestE060ServicioBajaIdempotenteYTokenRevocado(t *testing.T) {
	memoria := nuevaMemoriaPushDevices()
	servicio := servicioE059(memoria, selladorSintetico(t))
	registro, err := servicio.Register(context.Background(),
		cmdE059("user-a", "clave-idempotencia-0008", "token-sintetico-eeee", "iPhone"))
	if err != nil {
		t.Fatalf("registro: %v", err)
	}
	if err := servicio.Revoke(context.Background(), "user-a", registro.Device.ID); err != nil {
		t.Fatalf("baja: %v", err)
	}
	if err := servicio.Revoke(context.Background(), "user-a", registro.Device.ID); err != nil {
		t.Fatalf("baja repetida debe ser idempotente: %v", err)
	}
	guardado := memoria.dispositivos[claveDispositivo("user-a", registro.Device.ID)]
	if guardado.Active || guardado.TokenFingerprint != "" || len(guardado.TokenCipher) != 0 {
		t.Fatalf("el token revocado sigue utilizable: %+v", guardado)
	}

	nuevo, err := servicio.Register(context.Background(),
		cmdE059("user-a", "clave-idempotencia-0009", "token-sintetico-ffff", "iPhone"))
	if err != nil {
		t.Fatalf("registro tras la baja: %v", err)
	}
	if nuevo.Device.ID == registro.Device.ID {
		t.Fatal("una instalación revocada fue reutilizada")
	}
}

func TestE060ServicioAjenoInexistente(t *testing.T) {
	memoria := nuevaMemoriaPushDevices()
	servicio := servicioE059(memoria, selladorSintetico(t))
	registro, err := servicio.Register(context.Background(),
		cmdE059("user-a", "clave-idempotencia-0010", "token-sintetico-1111", "iPhone"))
	if err != nil {
		t.Fatalf("registro: %v", err)
	}
	if err := servicio.Revoke(context.Background(), "user-b", registro.Device.ID); !errors.Is(err, pushdevices.ErrNotFound) {
		t.Fatalf("dispositivo ajeno: esperado ErrNotFound, obtenido %v", err)
	}
	if err := servicio.Revoke(context.Background(), "user-a", "018f90ef-08e5-7a68-b09f-2b67a47c92f0"); !errors.Is(err, pushdevices.ErrNotFound) {
		t.Fatalf("inexistente: esperado ErrNotFound, obtenido %v", err)
	}
	if !memoria.dispositivos[claveDispositivo("user-a", registro.Device.ID)].Active {
		t.Fatal("la baja ajena desactivó el dispositivo del titular")
	}
}
