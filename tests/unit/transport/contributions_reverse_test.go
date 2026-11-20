package transport_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	appContrib "github.com/nexumpr/Nexum-Backend/internal/application/contributions"
	internalhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
)

// Pruebas HTTP de E047 (`POST /api/v1/goal-contributions/{contributionId}/reverse`):
// idempotencia obligatoria, reversión exactamente una vez, 409 en la segunda operación y
// respuesta reproducible byte a byte.

const (
	e047ClaveA = "idem-e047-reversa-000001"
	e047ClaveB = "idem-e047-reversa-000002"
	e047ClaveC = "idem-e047-reversa-000003"
	e047Cuerpo = `{"reason":"Correccion autorizada"}`
)

func peticionReversionE047(id, token, cuerpo, clave string) *http.Request {
	req := e004Peticion(http.MethodPost, "/api/v1/goal-contributions/"+id+"/reverse", token, cuerpo)
	req.SetPathValue("contributionId", id)
	if clave != "" {
		req.Header.Set("Idempotency-Key", clave)
	}
	return req
}

// TestE047HTTPReverseContratoYReplay cubre T01 y la idempotencia: la primera operación revierte,
// la repetición idéntica devuelve el mismo cuerpo con `X-Idempotent-Replay: true` y una clave
// distinta sobre la aportación ya revertida responde 409 sin alterar el estado.
func TestE047HTTPReverseContratoYReplay(t *testing.T) {
	store := nuevoStoreAportacionesE04x()
	store.sembrar(e004HTTPUsuario, e045AporteA, appContrib.StatusActive, e045Meta, 60000, e045Ahora)
	handler := internalhttp.NewContributionReverseHandler(servicioAportacionesE04x(store), e004TokensValidos(), newE004Sesiones(), nil, silencioso())

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, peticionReversionE047(e045AporteA, "jwt-valido", e047Cuerpo, e047ClaveA))
	if rec.Code != http.StatusOK {
		t.Fatalf("esperado 200, obtenido %d: %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("X-Idempotent-Replay") != "" {
		t.Fatalf("la primera operacion no es un replay: %v", rec.Header())
	}
	if rec.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatalf("Cache-Control inesperado: %q", rec.Header().Get("Cache-Control"))
	}

	var cuerpo map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &cuerpo); err != nil {
		t.Fatalf("json: %v", err)
	}
	if len(cuerpo) != 7 {
		t.Fatalf("se esperaban exactamente 7 campos, obtenidos %d: %s", len(cuerpo), rec.Body.String())
	}
	if cuerpo["status"] != appContrib.StatusReversed {
		t.Fatalf("estado inesperado: %v", cuerpo["status"])
	}
	if cuerpo["version"] != float64(2) {
		t.Fatalf("la reversion debe incrementar la version: %v", cuerpo["version"])
	}
	if cuerpo["occurredAt"] != "2026-11-19T12:00:00.000Z" {
		t.Fatalf("occurredAt no debe cambiar con la reversion: %v", cuerpo["occurredAt"])
	}
	if _, ok := cuerpo["reason"]; ok {
		t.Fatalf("el motivo de reversion no forma parte de la respuesta: %s", rec.Body.String())
	}

	persistida := store.ver(e004HTTPUsuario, e045AporteA)
	if persistida.Status != appContrib.StatusReversed || persistida.Version != 2 || persistida.ReverseReason == "" {
		t.Fatalf("estado persistido inesperado: %+v", persistida)
	}

	// Replay idéntico: mismo estado, mismo cuerpo y cabecera de replay.
	replay := httptest.NewRecorder()
	handler.ServeHTTP(replay, peticionReversionE047(e045AporteA, "jwt-valido", e047Cuerpo, e047ClaveA))
	if replay.Code != http.StatusOK {
		t.Fatalf("replay: esperado 200, obtenido %d: %s", replay.Code, replay.Body.String())
	}
	if replay.Header().Get("X-Idempotent-Replay") != "true" {
		t.Fatalf("replay sin X-Idempotent-Replay: %v", replay.Header())
	}
	if !bytes.Equal(rec.Body.Bytes(), replay.Body.Bytes()) {
		t.Fatalf("el replay debe ser byte a byte identico:\nprimera=%s\nreplay=%s", rec.Body.String(), replay.Body.String())
	}

	// Doble reversión con otra clave: 409 y sin cambios de estado.
	doble := httptest.NewRecorder()
	handler.ServeHTTP(doble, peticionReversionE047(e045AporteA, "jwt-valido", e047Cuerpo, e047ClaveB))
	if doble.Code != http.StatusConflict {
		t.Fatalf("doble reversion: esperado 409, obtenido %d: %s", doble.Code, doble.Body.String())
	}
	if final := store.ver(e004HTTPUsuario, e045AporteA); final.Version != 2 {
		t.Fatalf("la doble reversion no debe volver a incrementar la version: %+v", final)
	}
}

// TestE047HTTPMismaClaveOtroContenido cubre el conflicto de idempotencia: reutilizar la clave con
// un payload distinto responde 409 y no revierte nada nuevo.
func TestE047HTTPMismaClaveOtroContenido(t *testing.T) {
	store := nuevoStoreAportacionesE04x()
	store.sembrar(e004HTTPUsuario, e045AporteA, appContrib.StatusActive, e045Meta, 60000, e045Ahora)
	handler := internalhttp.NewContributionReverseHandler(servicioAportacionesE04x(store), e004TokensValidos(), newE004Sesiones(), nil, silencioso())

	primera := httptest.NewRecorder()
	handler.ServeHTTP(primera, peticionReversionE047(e045AporteA, "jwt-valido", e047Cuerpo, e047ClaveC))
	if primera.Code != http.StatusOK {
		t.Fatalf("primera reversion: %d %s", primera.Code, primera.Body.String())
	}

	distinta := httptest.NewRecorder()
	handler.ServeHTTP(distinta, peticionReversionE047(e045AporteA, "jwt-valido", `{"reason":"Otro motivo distinto"}`, e047ClaveC))
	if distinta.Code != http.StatusConflict {
		t.Fatalf("misma clave con otro contenido: esperado 409, obtenido %d", distinta.Code)
	}
}

// TestE047HTTPValidacion cubre T02: Idempotency-Key obligatoria y acotada, DTO cerrado y
// `reason` obligatorio de 3 a 250 caracteres.
func TestE047HTTPValidacion(t *testing.T) {
	store := nuevoStoreAportacionesE04x()
	store.sembrar(e004HTTPUsuario, e045AporteA, appContrib.StatusActive, e045Meta, 60000, e045Ahora)
	handler := internalhttp.NewContributionReverseHandler(servicioAportacionesE04x(store), e004TokensValidos(), newE004Sesiones(), nil, silencioso())

	casos := []struct {
		nombre   string
		cuerpo   string
		clave    string
		esperado int
	}{
		{"sin Idempotency-Key", e047Cuerpo, "", http.StatusBadRequest},
		{"clave demasiado corta", e047Cuerpo, "corta", http.StatusBadRequest},
		{"campo desconocido", `{"reason":"Correccion autorizada","motivo":"x"}`, e047ClaveA, http.StatusBadRequest},
		{"json malformado", `{"reason":`, e047ClaveA, http.StatusBadRequest},
		{"cuerpo vacio", "", e047ClaveA, http.StatusBadRequest},
		{"sin reason", `{}`, e047ClaveA, http.StatusUnprocessableEntity},
		{"reason demasiado corto", `{"reason":"ab"}`, e047ClaveA, http.StatusUnprocessableEntity},
		{"reason con control", `{"reason":"ab\u0007cd"}`, e047ClaveA, http.StatusUnprocessableEntity},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, peticionReversionE047(e045AporteA, "jwt-valido", caso.cuerpo, caso.clave))
			if rec.Code != caso.esperado {
				t.Fatalf("%s: esperado %d, obtenido %d: %s", caso.nombre, caso.esperado, rec.Code, rec.Body.String())
			}
		})
	}

	invalido := httptest.NewRecorder()
	handler.ServeHTTP(invalido, peticionReversionE047("no-es-uuid", "jwt-valido", e047Cuerpo, e047ClaveA))
	if invalido.Code != http.StatusBadRequest {
		t.Fatalf("identificador invalido: esperado 400, obtenido %d", invalido.Code)
	}

	if estado := store.ver(e004HTTPUsuario, e045AporteA); estado.Status != appContrib.StatusActive {
		t.Fatalf("una solicitud invalida no debe revertir la aportacion: %+v", estado)
	}
}

// TestE047HTTPOwnershipYFalloCerrado cubre T03/T04: recurso ajeno 404, autenticación 401,
// servicio ausente 503 y almacén caído 503 sin filtrar el error.
func TestE047HTTPOwnershipYFalloCerrado(t *testing.T) {
	store := nuevoStoreAportacionesE04x()
	store.sembrar(e004HTTPUsuario, e045AporteA, appContrib.StatusActive, e045Meta, 60000, e045Ahora)
	sesiones := newE004Sesiones()
	handler := internalhttp.NewContributionReverseHandler(servicioAportacionesE04x(store), e004TokensValidos(), sesiones, nil, silencioso())

	ajena := httptest.NewRecorder()
	handler.ServeHTTP(ajena, peticionReversionE047(e045AporteA, "jwt-sujeto-ajeno", e047Cuerpo, e047ClaveA))
	if ajena.Code != http.StatusNotFound {
		t.Fatalf("recurso ajeno: esperado 404, obtenido %d: %s", ajena.Code, ajena.Body.String())
	}
	if estado := store.ver(e004HTTPUsuario, e045AporteA); estado.Status != appContrib.StatusActive {
		t.Fatalf("una peticion ajena no debe revertir la aportacion: %+v", estado)
	}

	sinBearer := httptest.NewRecorder()
	handler.ServeHTTP(sinBearer, peticionReversionE047(e045AporteA, "", e047Cuerpo, e047ClaveA))
	if sinBearer.Code != http.StatusUnauthorized {
		t.Fatalf("sin Bearer: esperado 401, obtenido %d", sinBearer.Code)
	}

	sesiones.revocar(e004HTTPSesion)
	revocada := httptest.NewRecorder()
	handler.ServeHTTP(revocada, peticionReversionE047(e045AporteA, "jwt-valido", e047Cuerpo, e047ClaveA))
	if revocada.Code != http.StatusUnauthorized {
		t.Fatalf("sesion revocada: esperado 401, obtenido %d", revocada.Code)
	}

	sinServicio := internalhttp.NewContributionReverseHandler(nil, e004TokensValidos(), newE004Sesiones(), nil, silencioso())
	recSinServicio := httptest.NewRecorder()
	sinServicio.ServeHTTP(recSinServicio, peticionReversionE047(e045AporteA, "jwt-valido", e047Cuerpo, e047ClaveA))
	if recSinServicio.Code != http.StatusServiceUnavailable {
		t.Fatalf("sin servicio: esperado 503, obtenido %d", recSinServicio.Code)
	}

	store.fallo = appContrib.ErrUnavailable
	handlerFallo := internalhttp.NewContributionReverseHandler(servicioAportacionesE04x(store), e004TokensValidos(), newE004Sesiones(), nil, silencioso())
	recFallo := httptest.NewRecorder()
	handlerFallo.ServeHTTP(recFallo, peticionReversionE047(e045AporteA, "jwt-valido", e047Cuerpo, e047ClaveB))
	if recFallo.Code != http.StatusServiceUnavailable {
		t.Fatalf("almacen caido: esperado 503, obtenido %d", recFallo.Code)
	}
	if bytes.Contains(recFallo.Body.Bytes(), []byte("Dynamo")) {
		t.Fatalf("la respuesta no debe exponer infraestructura: %s", recFallo.Body.String())
	}
}

// TestE047HTTPReversionConcurrenteUnaSolaVez comprueba que dos operaciones simultáneas con
// claves distintas sobre la misma aportación producen exactamente una reversión y un 409.
func TestE047HTTPReversionConcurrenteUnaSolaVez(t *testing.T) {
	store := nuevoStoreAportacionesE04x()
	store.sembrar(e004HTTPUsuario, e045AporteA, appContrib.StatusActive, e045Meta, 60000, e045Ahora)
	handler := internalhttp.NewContributionReverseHandler(servicioAportacionesE04x(store), e004TokensValidos(), newE004Sesiones(), nil, silencioso())

	claves := []string{e047ClaveA, e047ClaveB}
	codigos := make([]int, len(claves))
	var wg sync.WaitGroup
	for indice, clave := range claves {
		wg.Add(1)
		go func(indice int, clave string) {
			defer wg.Done()
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, peticionReversionE047(e045AporteA, "jwt-valido", e047Cuerpo, clave))
			codigos[indice] = rec.Code
		}(indice, clave)
	}
	wg.Wait()

	exitos, conflictos := 0, 0
	for _, codigo := range codigos {
		switch codigo {
		case http.StatusOK:
			exitos++
		case http.StatusConflict:
			conflictos++
		}
	}
	if exitos != 1 || conflictos != 1 {
		t.Fatalf("se esperaba 1 exito y 1 conflicto, obtenidos %v", codigos)
	}
	if final := store.ver(e004HTTPUsuario, e045AporteA); final.Version != 2 {
		t.Fatalf("la version debe incrementarse exactamente una vez: %+v", final)
	}
}
