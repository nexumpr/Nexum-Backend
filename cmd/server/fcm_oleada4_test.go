package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	cryptoAdapter "github.com/nexumpr/Nexum-Backend/internal/adapters/crypto"
	appPrefs "github.com/nexumpr/Nexum-Backend/internal/application/notificationprefs"
	"github.com/nexumpr/Nexum-Backend/internal/application/pushoutbox"
	"github.com/nexumpr/Nexum-Backend/internal/infrastructure/push"
)

// Todos los valores de esta prueba son sintéticos. La credencial de cifrado es
// una clave de 32 bytes generada en memoria; no hay secretos reales.
const (
	fcmTestTabla   = "NEXUM_CORE_TEST"
	fcmTestUsuario = "u1"
	fcmTestToken   = "token-fcm-sintetico-123456"
)

func fcmAttrS(valor string) types.AttributeValue { return &types.AttributeValueMemberS{Value: valor} }
func fcmAttrB(valor bool) types.AttributeValue   { return &types.AttributeValueMemberBOOL{Value: valor} }
func fcmAttrN(valor int64) types.AttributeValue {
	return &types.AttributeValueMemberN{Value: strconv.FormatInt(valor, 10)}
}

func fcmTexto(valor types.AttributeValue) string {
	if v, ok := valor.(*types.AttributeValueMemberS); ok {
		return v.Value
	}
	return ""
}

func fcmNumero(valor types.AttributeValue) int64 {
	if v, ok := valor.(*types.AttributeValueMemberN); ok {
		if n, err := strconv.ParseInt(v.Value, 10, 64); err == nil {
			return n
		}
	}
	return 0
}

// fcmDispositivosFake simula la tabla única de E059/E060 con los atributos
// reales: tokenCipher en base64, active booleano, version y revokedAt.
type fcmDispositivosFake struct {
	mu              sync.Mutex
	items           map[string]map[string]types.AttributeValue
	actualizaciones []*dynamodb.UpdateItemInput
}

func (f *fcmDispositivosFake) Query(ctx context.Context, params *dynamodb.QueryInput, _ ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	pk := fcmTexto(params.ExpressionAttributeValues[":pk"])
	prefijo := fcmTexto(params.ExpressionAttributeValues[":prefijo"])
	salida := &dynamodb.QueryOutput{Items: []map[string]types.AttributeValue{}}
	for clave, item := range f.items {
		if !strings.HasPrefix(clave, pk+"|") {
			continue
		}
		if prefijo != "" && !strings.HasPrefix(fcmTexto(item["SK"]), prefijo) {
			continue
		}
		copia := make(map[string]types.AttributeValue, len(item))
		for k, v := range item {
			copia[k] = v
		}
		salida.Items = append(salida.Items, copia)
	}
	return salida, nil
}

func (f *fcmDispositivosFake) GetItem(ctx context.Context, params *dynamodb.GetItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	clave := fcmTexto(params.Key["PK"]) + "|" + fcmTexto(params.Key["SK"])
	return &dynamodb.GetItemOutput{Item: f.items[clave]}, nil
}

func (f *fcmDispositivosFake) PutItem(ctx context.Context, params *dynamodb.PutItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	clave := fcmTexto(params.Item["PK"]) + "|" + fcmTexto(params.Item["SK"])
	f.items[clave] = params.Item
	return &dynamodb.PutItemOutput{}, nil
}

func (f *fcmDispositivosFake) UpdateItem(ctx context.Context, params *dynamodb.UpdateItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	clave := fcmTexto(params.Key["PK"]) + "|" + fcmTexto(params.Key["SK"])
	item := f.items[clave]
	if item == nil {
		return nil, &types.ConditionalCheckFailedException{}
	}
	if strings.Contains(aws.ToString(params.ConditionExpression), "#active = :active") {
		esperado, okEsperado := params.ExpressionAttributeValues[":active"].(*types.AttributeValueMemberBOOL)
		actual, okActual := item["active"].(*types.AttributeValueMemberBOOL)
		if !okEsperado || !okActual || actual.Value != esperado.Value {
			return nil, &types.ConditionalCheckFailedException{}
		}
	}
	actualizado := make(map[string]types.AttributeValue, len(item))
	for k, v := range item {
		actualizado[k] = v
	}
	actualizado["active"] = params.ExpressionAttributeValues[":inactive"]
	actualizado["revokedAt"] = params.ExpressionAttributeValues[":revokedAt"]
	actualizado["version"] = fcmAttrN(fcmNumero(item["version"]) + 1)
	delete(actualizado, "tokenCipher")
	delete(actualizado, "tokenFingerprint")
	f.items[clave] = actualizado
	f.actualizaciones = append(f.actualizaciones, params)
	return &dynamodb.UpdateItemOutput{Attributes: actualizado}, nil
}

func (f *fcmDispositivosFake) item(userID, deviceID string) map[string]types.AttributeValue {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.items["USER#"+userID+"|DEVICE#"+deviceID]
}

// fcmOutboxFake es el mínimo storage del worker; la prueba se centra en la
// integración con el item real de E059.
// DeleteItem completa la interfaz PushOutboxAPI tras añadir la retirada del ítem global
// y de su puntero al cerrar un trabajo. Es un doble de pruebas sin efectos externos.
func (f *fcmDispositivosFake) DeleteItem(_ context.Context, params *dynamodb.DeleteItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.DeleteItemOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	clave := fcmTexto(params.Key["PK"]) + "|" + fcmTexto(params.Key["SK"])
	delete(f.items, clave)
	return &dynamodb.DeleteItemOutput{}, nil
}

// TransactWriteItems completa la interfaz PushOutboxAPI; la fuente de tokens no la usa.
func (f *fcmDispositivosFake) TransactWriteItems(_ context.Context, _ *dynamodb.TransactWriteItemsInput, _ ...func(*dynamodb.Options)) (*dynamodb.TransactWriteItemsOutput, error) {
	return &dynamodb.TransactWriteItemsOutput{}, nil
}

type fcmOutboxFake struct {
	porID     map[string]pushoutbox.Registro
	entregado map[string]bool
}

func (s *fcmOutboxFake) Crear(_ context.Context, r pushoutbox.Registro) (bool, error) {
	if _, existe := s.porID[r.NotificationID]; existe {
		return false, nil
	}
	r.ID = r.NotificationID
	r.Status = pushoutbox.EstadoPendiente
	s.porID[r.ID] = r
	return true, nil
}

func (s *fcmOutboxFake) Pendientes(_ context.Context, ahora time.Time, _ int) ([]pushoutbox.Registro, error) {
	resultado := []pushoutbox.Registro{}
	for _, r := range s.porID {
		if (r.Status == pushoutbox.EstadoPendiente || r.Status == pushoutbox.EstadoFallido) && !r.NextAttemptAt.After(ahora) {
			resultado = append(resultado, r)
		}
	}
	return resultado, nil
}

func (s *fcmOutboxFake) Reclamar(_ context.Context, id string, ahora time.Time, lease time.Duration) (int, bool, error) {
	r := s.porID[id]
	if r.ID == "" || (r.Status != pushoutbox.EstadoPendiente && r.Status != pushoutbox.EstadoFallido) {
		return 0, false, nil
	}
	r.Status = pushoutbox.EstadoEnviando
	r.LeaseUntil = ahora.Add(lease)
	r.Attempts++
	s.porID[id] = r
	return r.Attempts, true, nil
}

func (s *fcmOutboxFake) MarcarEnviado(_ context.Context, id string, _ time.Time) error {
	r := s.porID[id]
	r.Status = pushoutbox.EstadoEnviado
	s.porID[id] = r
	return nil
}

func (s *fcmOutboxFake) MarcarRevocado(_ context.Context, id, clase string, _ time.Time) error {
	r := s.porID[id]
	r.Status = pushoutbox.EstadoRevocado
	r.UltimaClase = clase
	s.porID[id] = r
	return nil
}

func (s *fcmOutboxFake) Reprogramar(_ context.Context, id string, attempts int, clase string, siguiente, _ time.Time) error {
	r := s.porID[id]
	r.Status = pushoutbox.EstadoFallido
	r.Attempts = attempts
	r.UltimaClase = clase
	r.NextAttemptAt = siguiente
	s.porID[id] = r
	return nil
}

func (s *fcmOutboxFake) YaEntregado(_ context.Context, userID, eventID string) (bool, error) {
	return s.entregado[userID+"|"+eventID], nil
}

func (s *fcmOutboxFake) MarcarEntregado(_ context.Context, userID, eventID, _ string, _ time.Time) error {
	s.entregado[userID+"|"+eventID] = true
	return nil
}

type fcmPrefs struct{}

func (fcmPrefs) Get(_ context.Context, _ string) (*appPrefs.Preferences, error) {
	p := appPrefs.Defaults()
	p.PushEnabled = true
	return &p, nil
}

type fcmBandeja struct{}

func (fcmBandeja) GuardarBandeja(_ context.Context, _ pushoutbox.RegistroBandeja) error { return nil }

type fcmEmisor struct {
	ultimo push.Mensaje
}

func (e *fcmEmisor) Enviar(_ context.Context, mensaje push.Mensaje) (push.Resultado, error) {
	e.ultimo = mensaje
	return push.Resultado{}, &push.ErrorClasificado{Clase: push.ClaseDestinoNoRegistrado, CodigoHTTP: 404}
}

func TestFCMFuenteE059TokenCipherYRevocacionUNREGISTERED(t *testing.T) {
	claveB64 := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x2a}, 32))
	sellador, err := cryptoAdapter.NuevoSealer(claveB64)
	if err != nil {
		t.Fatal(err)
	}
	fake := &fcmDispositivosFake{items: make(map[string]map[string]types.AttributeValue)}
	registrar := func(deviceID, token string, activo bool) string {
		sobre, errSellar := sellador.Sellar(token, fcmTestUsuario+"|"+deviceID)
		if errSellar != nil {
			t.Fatal(errSellar)
		}
		cipherB64 := base64.StdEncoding.EncodeToString(sobre)
		clave := "USER#" + fcmTestUsuario + "|DEVICE#" + deviceID
		fake.items[clave] = map[string]types.AttributeValue{
			"PK":               fcmAttrS("USER#" + fcmTestUsuario),
			"SK":               fcmAttrS("DEVICE#" + deviceID),
			"entityType":       fcmAttrS("DEVICE"),
			"userId":           fcmAttrS(fcmTestUsuario),
			"provider":         fcmAttrS("fcm"),
			"deviceLabel":      fcmAttrS("Pixel sintético"),
			"registeredAt":     fcmAttrS("2026-11-22T12:00:00Z"),
			"tokenCipher":      fcmAttrS(cipherB64),
			"tokenFingerprint": fcmAttrS("huella-" + deviceID),
			"active":           fcmAttrB(activo),
			"version":          fcmAttrN(1),
		}
		return clave
	}
	registrar("dev-1", fcmTestToken, true)
	registrar("dev-2", "token-inactivo-sintetico", false)

	fuente := nuevaFuenteTokensDynamo(fake, fcmTestTabla, sellador, nil)
	tokens, err := fuente.TokensActivos(context.Background(), fcmTestUsuario)
	if err != nil {
		t.Fatal(err)
	}
	if len(tokens) != 1 {
		t.Fatalf("sólo el dispositivo activo debe entregar token; se obtuvieron %d: %+v", len(tokens), tokens)
	}
	if tokens[0].ID != "dev-1" || tokens[0].Valor != fcmTestToken {
		t.Fatalf("token E059 incorrecto: %+v", tokens[0])
	}

	ahora := time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)
	store := &fcmOutboxFake{porID: map[string]pushoutbox.Registro{}, entregado: map[string]bool{}}
	emisor := &fcmEmisor{}
	servicio := pushoutbox.NewService(store, emisor, fcmPrefs{}, fuente, fcmBandeja{}, pushoutbox.Opciones{
		Ahora: func() time.Time { return ahora },
		Lease: time.Minute,
	})
	if _, _, err := servicio.Encolar(context.Background(), pushoutbox.Solicitud{
		UserID: fcmTestUsuario, EventID: "evt-e059", Title: "Aviso", Message: "Mensaje", Type: "REMINDER",
	}); err != nil {
		t.Fatal(err)
	}
	servicio.Ciclo(context.Background())

	if emisor.ultimo.Token != fcmTestToken {
		t.Fatalf("el worker no entregó el token abierto de E059: %q", emisor.ultimo.Token)
	}
	if emisor.ultimo.EventID != "evt-e059" {
		t.Fatalf("eventId alterado: %q", emisor.ultimo.EventID)
	}
	for _, id := range store.porID {
		if id.Status != pushoutbox.EstadoRevocado || id.UltimaClase != pushoutbox.ClaseDestinoNoRegistrado {
			t.Fatalf("trabajo no marcado como revocado tras UNREGISTERED: %+v", id)
		}
	}

	item := fake.item(fcmTestUsuario, "dev-1")
	activo, ok := item["active"].(*types.AttributeValueMemberBOOL)
	if !ok || activo.Value {
		t.Fatalf("el dispositivo no quedó inactivo: %+v", item["active"])
	}
	if fcmTexto(item["revokedAt"]) == "" {
		t.Fatal("falta revokedAt tras la revocación")
	}
	if version := fcmNumero(item["version"]); version != 2 {
		t.Fatalf("version incorrecta tras la revocación: %d", version)
	}
	if _, existe := item["tokenCipher"]; existe {
		t.Fatal("tokenCipher no fue eliminado")
	}
	if _, existe := item["tokenFingerprint"]; existe {
		t.Fatal("tokenFingerprint no fue eliminado")
	}
	if len(fake.actualizaciones) != 1 {
		t.Fatalf("actualizaciones de baja: %d", len(fake.actualizaciones))
	}
	update := aws.ToString(fake.actualizaciones[0].UpdateExpression)
	for _, fragmento := range []string{"#active = :inactive", "#revokedAt = :revokedAt", "#version = #version + :one", "REMOVE #tokenCipher, #tokenFingerprint"} {
		if !strings.Contains(update, fragmento) {
			t.Fatalf("expresión de baja incompleta (%q): %s", fragmento, update)
		}
	}
}

// TestFCMCredencialArchivoAcotado fija la frontera G304: solo rutas absolutas
// dentro del directorio permitido. La credencial es sintetica ({}), sin secretos.
func TestFCMCredencialArchivoAcotado(t *testing.T) {
	dir := t.TempDir()
	ruta := filepath.Join(dir, "credencial-sintetica.json")
	if err := os.WriteFile(ruta, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	leido, err := leerCredencialFCM(ruta, dir)
	if err != nil || string(leido) != "{}" {
		t.Fatalf("lectura valida: %q err=%v", string(leido), err)
	}

	casos := []struct {
		nombre string
		ruta   string
	}{
		{"relativa", "credencial-sintetica.json"},
		{"fuera_del_arbol", filepath.Join(dir, "..", "fuera.json")},
		{"directorio", dir},
		{"vacia", ""},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			if _, err := leerCredencialFCM(caso.ruta, dir); err == nil {
				t.Fatalf("ruta %q debio rechazarse", caso.ruta)
			}
		})
	}
}
