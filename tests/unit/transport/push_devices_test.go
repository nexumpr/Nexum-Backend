package transport_test

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	adaptersCrypto "github.com/nexumpr/Nexum-Backend/internal/adapters/crypto"
	appPush "github.com/nexumpr/Nexum-Backend/internal/application/pushdevices"
	internalhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
)

var e059HTTPAhora = time.Date(2026, 11, 22, 13, 0, 0, 0, time.UTC)

type e059HTTPStore struct {
	dispositivos map[string]*appPush.Device
	idem         map[string]*appPush.IdempotencyRecord
}

func nuevaE059HTTPStore() *e059HTTPStore {
	return &e059HTTPStore{
		dispositivos: map[string]*appPush.Device{},
		idem:         map[string]*appPush.IdempotencyRecord{},
	}
}

func (m *e059HTTPStore) LoadIdempotency(_ context.Context, userID, key string, now time.Time) (*appPush.IdempotencyRecord, error) {
	registro := m.idem[userID+"|"+key]
	if registro == nil || now.Unix() >= registro.ExpiresAt {
		return nil, nil
	}
	copia := *registro
	return &copia, nil
}

func (m *e059HTTPStore) FindActiveByLabel(_ context.Context, userID, provider, label string) (*appPush.Device, error) {
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

func (m *e059HTTPStore) Register(_ context.Context, write appPush.RegisterWrite) error {
	if m.idem[write.UserID+"|"+write.IdempotencyKey] != nil {
		return appPush.ErrIdempotencyCondition
	}
	clave := write.UserID + "|" + write.Device.ID
	if actual := m.dispositivos[clave]; actual != nil && actual.TokenFingerprint != write.PreviousFingerprint {
		return appPush.ErrDeviceCondition
	}
	copia := write.Device
	m.dispositivos[clave] = &copia
	m.idem[write.UserID+"|"+write.IdempotencyKey] = &appPush.IdempotencyRecord{
		DeviceID:     write.Device.ID,
		PayloadHash:  write.PayloadHash,
		ResponseJSON: append([]byte(nil), write.ResponseJSON...),
		ExpiresAt:    write.Now.Add(write.TTL).Unix(),
	}
	return nil
}

func (m *e059HTTPStore) GetDevice(_ context.Context, userID, deviceID string) (*appPush.Device, error) {
	dispositivo := m.dispositivos[userID+"|"+deviceID]
	if dispositivo == nil {
		return nil, nil
	}
	copia := *dispositivo
	return &copia, nil
}

func (m *e059HTTPStore) Revoke(_ context.Context, userID, deviceID string, _ time.Time) (bool, error) {
	dispositivo := m.dispositivos[userID+"|"+deviceID]
	if dispositivo == nil || !dispositivo.Active {
		return false, nil
	}
	dispositivo.Active = false
	dispositivo.TokenCipher = nil
	dispositivo.TokenFingerprint = ""
	return true, nil
}

func e059HTTPSealer(t *testing.T) *adaptersCrypto.Sealer {
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

func e059HTTPServicio(store appPush.Store, sealer appPush.TokenSealer) *appPush.Service {
	return appPush.NewService(store, sealer).WithClock(func() time.Time { return e059HTTPAhora })
}

func e059HTTPPost(store appPush.Store, sealer appPush.TokenSealer, limitador internalhttp.LimitadorRuta) *internalhttp.PushDevicesHandler {
	return internalhttp.NewPushDevicesHandler(e059HTTPServicio(store, sealer), e004TokensValidos(), newE004Sesiones(), limitador, silencioso())
}

func e060HTTPDelete(store appPush.Store, sealer appPush.TokenSealer, limitador internalhttp.LimitadorRuta) *internalhttp.PushDeviceDeleteHandler {
	return internalhttp.NewPushDeviceDeleteHandler(e059HTTPServicio(store, sealer), e004TokensValidos(), newE004Sesiones(), limitador, silencioso())
}

func e059Peticion(cuerpo, clave, token string) *http.Request {
	req := e004Peticion(http.MethodPost, "/api/v1/devices/push-tokens", token, cuerpo)
	if clave != "" {
		req.Header.Set("Idempotency-Key", clave)
	}
	return req
}

func e060Peticion(id, token string) *http.Request {
	req := e004Peticion(http.MethodDelete, "/api/v1/devices/push-tokens/"+id, token, "")
	req.SetPathValue("deviceRegistrationId", id)
	return req
}

func e059CuerpoValido() string {
	return `{"provider":"fcm","token":"token-sintetico-0001","deviceLabel":"Android prueba"}`
}

func e059Envelope(t *testing.T, rec *httptest.ResponseRecorder, code string) {
	t.Helper()
	var envelope struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
			TraceID string `json:"traceId"`
			Details []any  `json:"details"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("ErrorEnvelope invalido: %v (%s)", err, rec.Body.String())
	}
	if envelope.Error.Code != code || envelope.Error.Message == "" || envelope.Error.Details == nil {
		t.Fatalf("ErrorEnvelope fuera de contrato: %s", rec.Body.String())
	}
}

func TestE059HTTPContratoValido(t *testing.T) {
	rec := httptest.NewRecorder()
	e059HTTPPost(nuevaE059HTTPStore(), e059HTTPSealer(t), e053Limitador{permitido: true}).ServeHTTP(
		rec, e059Peticion(e059CuerpoValido(), "clave-idempotencia-0001", "jwt-valido"))

	if rec.Code != http.StatusCreated {
		t.Fatalf("esperado 201, obtenido %d: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Cache-Control"); got != "private, no-store" {
		t.Fatalf("Cache-Control incorrecto: %q", got)
	}
	var cuerpo map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &cuerpo); err != nil {
		t.Fatalf("JSON invalido: %v", err)
	}
	permitidos := map[string]bool{"id": true, "provider": true, "deviceLabel": true, "registeredAt": true}
	if len(cuerpo) != len(permitidos) {
		t.Fatalf("esquema PushDevice con campos extra: %s", rec.Body.String())
	}
	for campo := range cuerpo {
		if !permitidos[campo] {
			t.Fatalf("campo no contractual: %s", campo)
		}
	}
	var dispositivo struct {
		ID           string `json:"id"`
		Provider     string `json:"provider"`
		DeviceLabel  string `json:"deviceLabel"`
		RegisteredAt string `json:"registeredAt"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &dispositivo); err != nil {
		t.Fatal(err)
	}
	if dispositivo.ID == "" || dispositivo.Provider != "fcm" || dispositivo.DeviceLabel != "Android prueba" ||
		dispositivo.RegisteredAt != e059HTTPAhora.Format(time.RFC3339Nano) {
		t.Fatalf("PushDevice incorrecto: %+v", dispositivo)
	}
	if location := rec.Header().Get("Location"); location != "/api/v1/devices/push-tokens/"+dispositivo.ID {
		t.Fatalf("Location incorrecto: %q", location)
	}
	if strings.Contains(rec.Body.String(), "token-sintetico-0001") || strings.Contains(rec.Body.String(), "tokenCipher") ||
		strings.Contains(rec.Body.String(), e004HTTPUsuario) {
		t.Fatalf("la respuesta filtra token, cifrado o titular: %s", rec.Body.String())
	}
}

func TestE059HTTPNegativas(t *testing.T) {
	casos := []struct {
		nombre    string
		cuerpo    string
		clave     string
		token     string
		limitador internalhttp.LimitadorRuta
		esperado  int
		codigo    string
	}{
		{"sin_clave", e059CuerpoValido(), "", "jwt-valido", e053Limitador{permitido: true}, http.StatusBadRequest, "INVALID_REQUEST"},
		{"clave_corta", e059CuerpoValido(), "corta", "jwt-valido", e053Limitador{permitido: true}, http.StatusBadRequest, "INVALID_REQUEST"},
		{"cuerpo_malformado", `{`, "clave-idempotencia-0002", "jwt-valido", e053Limitador{permitido: true}, http.StatusBadRequest, "INVALID_REQUEST"},
		{"campo_extra", `{"provider":"fcm","token":"token-sintetico-0002","deviceLabel":"iPhone","extra":1}`, "clave-idempotencia-0003", "jwt-valido", e053Limitador{permitido: true}, http.StatusBadRequest, "INVALID_REQUEST"},
		{"provider_invalido", `{"provider":"apns","token":"token-sintetico-0002","deviceLabel":"iPhone"}`, "clave-idempotencia-0004", "jwt-valido", e053Limitador{permitido: true}, http.StatusBadRequest, "INVALID_REQUEST"},
		{"token_corto", `{"provider":"fcm","token":"corto","deviceLabel":"iPhone"}`, "clave-idempotencia-0005", "jwt-valido", e053Limitador{permitido: true}, http.StatusBadRequest, "INVALID_REQUEST"},
		{"etiqueta_larga", `{"provider":"fcm","token":"token-sintetico-0002","deviceLabel":"` + strings.Repeat("x", 81) + `"}`, "clave-idempotencia-0006", "jwt-valido", e053Limitador{permitido: true}, http.StatusBadRequest, "INVALID_REQUEST"},
		{"sin_cuerpo", "", "clave-idempotencia-0007", "jwt-valido", e053Limitador{permitido: true}, http.StatusBadRequest, "INVALID_REQUEST"},
		{"sin_token", e059CuerpoValido(), "clave-idempotencia-0008", "", e053Limitador{permitido: true}, http.StatusUnauthorized, "UNAUTHORIZED"},
		{"cuota", e059CuerpoValido(), "clave-idempotencia-0009", "jwt-valido", e053Limitador{permitido: false}, http.StatusTooManyRequests, "RATE_LIMITED"},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			rec := httptest.NewRecorder()
			e059HTTPPost(nuevaE059HTTPStore(), e059HTTPSealer(t), caso.limitador).ServeHTTP(
				rec, e059Peticion(caso.cuerpo, caso.clave, caso.token))
			if rec.Code != caso.esperado {
				t.Fatalf("esperado %d, obtenido %d: %s", caso.esperado, rec.Code, rec.Body.String())
			}
			if got := rec.Header().Get("Cache-Control"); got != "private, no-store" {
				t.Fatalf("Cache-Control ausente en error: %q", got)
			}
			e059Envelope(t, rec, caso.codigo)
		})
	}
}

func TestE059HTTPReplayYConflicto(t *testing.T) {
	store := nuevaE059HTTPStore()
	sealer := e059HTTPSealer(t)
	handler := e059HTTPPost(store, sealer, e053Limitador{permitido: true})
	primero := httptest.NewRecorder()
	handler.ServeHTTP(primero, e059Peticion(e059CuerpoValido(), "clave-idempotencia-0010", "jwt-valido"))
	if primero.Code != http.StatusCreated {
		t.Fatalf("primer registro: %d %s", primero.Code, primero.Body.String())
	}
	replay := httptest.NewRecorder()
	handler.ServeHTTP(replay, e059Peticion(e059CuerpoValido(), "clave-idempotencia-0010", "jwt-valido"))
	if replay.Code != http.StatusCreated || replay.Body.String() != primero.Body.String() {
		t.Fatalf("replay inestable: %d %s", replay.Code, replay.Body.String())
	}
	conflicto := httptest.NewRecorder()
	otroCuerpo := `{"provider":"fcm","token":"token-sintetico-9999","deviceLabel":"Android prueba"}`
	handler.ServeHTTP(conflicto, e059Peticion(otroCuerpo, "clave-idempotencia-0010", "jwt-valido"))
	if conflicto.Code != http.StatusConflict {
		t.Fatalf("esperado 409, obtenido %d: %s", conflicto.Code, conflicto.Body.String())
	}
	e059Envelope(t, conflicto, "CONFLICT")
}

func TestE059HTTPFailClosed(t *testing.T) {
	sinServicio := internalhttp.NewPushDevicesHandler(nil, e004TokensValidos(), newE004Sesiones(), nil, silencioso())
	rec := httptest.NewRecorder()
	sinServicio.ServeHTTP(rec, e059Peticion(e059CuerpoValido(), "clave-idempotencia-0011", "jwt-valido"))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("sin servicio: esperado 503, obtenido %d", rec.Code)
	}
	e059Envelope(t, rec, "DEPENDENCY_UNAVAILABLE")

	sinSealer := httptest.NewRecorder()
	e059HTTPPost(nuevaE059HTTPStore(), nil, nil).ServeHTTP(
		sinSealer, e059Peticion(e059CuerpoValido(), "clave-idempotencia-0012", "jwt-valido"))
	if sinSealer.Code != http.StatusServiceUnavailable {
		t.Fatalf("sin sealer: esperado 503, obtenido %d", sinSealer.Code)
	}
	sinStore := httptest.NewRecorder()
	e059HTTPPost(nil, e059HTTPSealer(t), nil).ServeHTTP(
		sinStore, e059Peticion(e059CuerpoValido(), "clave-idempotencia-0013", "jwt-valido"))
	if sinStore.Code != http.StatusServiceUnavailable {
		t.Fatalf("sin store: esperado 503, obtenido %d", sinStore.Code)
	}
}

func e059RegistrarHTTP(t *testing.T, handler *internalhttp.PushDevicesHandler, clave, cuerpo, token string) string {
	t.Helper()
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, e059Peticion(cuerpo, clave, token))
	if rec.Code != http.StatusCreated {
		t.Fatalf("registro previo: %d %s", rec.Code, rec.Body.String())
	}
	var dispositivo struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &dispositivo); err != nil || dispositivo.ID == "" {
		t.Fatalf("ID no obtenido: %v %s", err, rec.Body.String())
	}
	return dispositivo.ID
}

func TestE060HTTPBajaExitosaEIdempotente(t *testing.T) {
	store := nuevaE059HTTPStore()
	sealer := e059HTTPSealer(t)
	post := e059HTTPPost(store, sealer, e053Limitador{permitido: true})
	id := e059RegistrarHTTP(t, post, "clave-idempotencia-0014", e059CuerpoValido(), "jwt-valido")
	deleteHandler := e060HTTPDelete(store, sealer, e053Limitador{permitido: true})

	for i := 0; i < 2; i++ {
		rec := httptest.NewRecorder()
		deleteHandler.ServeHTTP(rec, e060Peticion(id, "jwt-valido"))
		if rec.Code != http.StatusNoContent {
			t.Fatalf("baja %d: esperado 204, obtenido %d: %s", i+1, rec.Code, rec.Body.String())
		}
		if rec.Body.Len() != 0 || rec.Header().Get("Location") != "" {
			t.Fatalf("204 con cuerpo o Location: %q", rec.Body.String())
		}
		if got := rec.Header().Get("Cache-Control"); got != "private, no-store" {
			t.Fatalf("Cache-Control incorrecto: %q", got)
		}
	}
	if dispositivo := store.dispositivos[e004HTTPUsuario+"|"+id]; dispositivo == nil || dispositivo.Active ||
		dispositivo.TokenFingerprint != "" || len(dispositivo.TokenCipher) != 0 {
		t.Fatalf("el token revocado sigue en el item: %+v", dispositivo)
	}
}

func TestE060HTTPAjenoInexistenteYAislamientoAB(t *testing.T) {
	store := nuevaE059HTTPStore()
	sealer := e059HTTPSealer(t)
	post := e059HTTPPost(store, sealer, e053Limitador{permitido: true})
	idA := e059RegistrarHTTP(t, post, "clave-idempotencia-0015", e059CuerpoValido(), "jwt-valido")
	idB := e059RegistrarHTTP(t, post, "clave-idempotencia-0016", e059CuerpoValido(), "jwt-sujeto-ajeno")
	if idA == idB {
		t.Fatal("dos titulares compartieron identificador de dispositivo")
	}
	deleteHandler := e060HTTPDelete(store, sealer, e053Limitador{permitido: true})

	recAjeno := httptest.NewRecorder()
	deleteHandler.ServeHTTP(recAjeno, e060Peticion(idA, "jwt-sujeto-ajeno"))
	if recAjeno.Code != http.StatusNotFound {
		t.Fatalf("usuario B sobre dispositivo de A: esperado 404, obtenido %d: %s", recAjeno.Code, recAjeno.Body.String())
	}
	e059Envelope(t, recAjeno, "NOT_FOUND")
	if !store.dispositivos[e004HTTPUsuario+"|"+idA].Active {
		t.Fatal("la baja ajena afectó al dispositivo de A")
	}

	recInexistente := httptest.NewRecorder()
	deleteHandler.ServeHTTP(recInexistente, e060Peticion("018f90ef-08e5-7a68-b09f-2b67a47c92f0", "jwt-valido"))
	if recInexistente.Code != http.StatusNotFound {
		t.Fatalf("inexistente: esperado 404, obtenido %d: %s", recInexistente.Code, recInexistente.Body.String())
	}
	e059Envelope(t, recInexistente, "NOT_FOUND")

	recPropio := httptest.NewRecorder()
	deleteHandler.ServeHTTP(recPropio, e060Peticion(idA, "jwt-valido"))
	if recPropio.Code != http.StatusNoContent {
		t.Fatalf("baja propia: esperado 204, obtenido %d: %s", recPropio.Code, recPropio.Body.String())
	}
	if !store.dispositivos[e004HTTPOtroUser+"|"+idB].Active {
		t.Fatal("la baja de A afectó al dispositivo de B")
	}
}

func TestE060HTTPNegativas(t *testing.T) {
	casos := []struct {
		nombre    string
		id        string
		token     string
		limitador internalhttp.LimitadorRuta
		esperado  int
		codigo    string
	}{
		{"sin_token", "018f90ef-08e5-7a68-b09f-2b67a47c92f0", "", e053Limitador{permitido: true}, http.StatusUnauthorized, "UNAUTHORIZED"},
		{"id_invalido", "no-es-uuid", "jwt-valido", e053Limitador{permitido: true}, http.StatusBadRequest, "INVALID_REQUEST"},
		{"cuota", "018f90ef-08e5-7a68-b09f-2b67a47c92f0", "jwt-valido", e053Limitador{permitido: false}, http.StatusTooManyRequests, "RATE_LIMITED"},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			rec := httptest.NewRecorder()
			e060HTTPDelete(nuevaE059HTTPStore(), e059HTTPSealer(t), caso.limitador).ServeHTTP(
				rec, e060Peticion(caso.id, caso.token))
			if rec.Code != caso.esperado {
				t.Fatalf("esperado %d, obtenido %d: %s", caso.esperado, rec.Code, rec.Body.String())
			}
			e059Envelope(t, rec, caso.codigo)
		})
	}
}

func TestE060HTTPFailClosed(t *testing.T) {
	sinServicio := internalhttp.NewPushDeviceDeleteHandler(nil, e004TokensValidos(), newE004Sesiones(), nil, silencioso())
	rec := httptest.NewRecorder()
	sinServicio.ServeHTTP(rec, e060Peticion("018f90ef-08e5-7a68-b09f-2b67a47c92f0", "jwt-valido"))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("sin servicio: esperado 503, obtenido %d", rec.Code)
	}
	e059Envelope(t, rec, "DEPENDENCY_UNAVAILABLE")
}
