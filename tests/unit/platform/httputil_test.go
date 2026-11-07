package platform_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nexumpr/Nexum-Backend/pkg/httputil"
)

type samplePayload struct {
	Name   string `json:"name"`
	Amount int64  `json:"amount"`
}

func TestWriteJSON(t *testing.T) {
	rec := httptest.NewRecorder()
	payload := map[string]string{"status": "ok"}

	err := httputil.WriteJSON(rec, http.StatusOK, payload)
	if err != nil {
		t.Fatalf("WriteJSON fallo: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Errorf("codigo de estado esperado 200, obtenido %d", rec.Code)
	}

	ct := rec.Header().Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Errorf("Content-Type esperado 'application/json', obtenido '%s'", ct)
	}

	var res map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("respuesta no es JSON valido: %v", err)
	}
	if res["status"] != "ok" {
		t.Errorf("valor de respuesta inesperado: %v", res)
	}
}

func TestWriteError(t *testing.T) {
	rec := httptest.NewRecorder()
	httputil.WriteError(rec, http.StatusBadRequest, "INVALID_INPUT", "el campo 'name' es requerido")

	if rec.Code != http.StatusBadRequest {
		t.Errorf("codigo esperado 400, obtenido %d", rec.Code)
	}

	var errRes httputil.ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &errRes); err != nil {
		t.Fatalf("error al deserializar ErrorResponse: %v", err)
	}

	if errRes.Code != "INVALID_INPUT" || errRes.Message != "el campo 'name' es requerido" {
		t.Errorf("ErrorResponse incorrecta: %+v", errRes)
	}
}

func TestDecodeJSON(t *testing.T) {
	t.Run("Payload valido decodifica exitosamente", func(t *testing.T) {
		body := `{"name":"Ahorro","amount":5000}`
		req := httptest.NewRequest(http.MethodPost, "/test", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		var dst samplePayload
		err := httputil.DecodeJSON(rec, req, &dst, 1024)
		if err != nil {
			t.Fatalf("error inesperado al decodificar: %v", err)
		}
		if dst.Name != "Ahorro" || dst.Amount != 5000 {
			t.Errorf("valores decodificados incorrectos: %+v", dst)
		}
	})

	t.Run("Rechazo de campos desconocidos (DisallowUnknownFields)", func(t *testing.T) {
		body := `{"name":"Ahorro","amount":5000,"campo_fantasma":"hack"}`
		req := httptest.NewRequest(http.MethodPost, "/test", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		var dst samplePayload
		err := httputil.DecodeJSON(rec, req, &dst, 1024)
		if err == nil {
			t.Fatal("se esperaba rechazo por campo desconocido y la decodificacion paso")
		}
		if !strings.Contains(err.Error(), "campo desconocido no permitido") {
			t.Errorf("mensaje de error inesperado: %v", err)
		}
	})

	t.Run("Rechazo de payload que excede el limite de bytes (MaxBytesReader)", func(t *testing.T) {
		body := `{"name":"` + strings.Repeat("x", 200) + `","amount":100}`
		req := httptest.NewRequest(http.MethodPost, "/test", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		var dst samplePayload
		// Limite de solo 50 bytes
		err := httputil.DecodeJSON(rec, req, &dst, 50)
		if err == nil {
			t.Fatal("se esperaba error por exceder limite de bytes")
		}
		if !strings.Contains(err.Error(), "excede el limite maximo") {
			t.Errorf("error inesperado: %v", err)
		}
	})

	t.Run("Rechazo de Content-Type invalido", func(t *testing.T) {
		body := `{"name":"Ahorro","amount":100}`
		req := httptest.NewRequest(http.MethodPost, "/test", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "text/plain")
		rec := httptest.NewRecorder()

		var dst samplePayload
		err := httputil.DecodeJSON(rec, req, &dst, 1024)
		if err == nil {
			t.Fatal("se esperaba rechazo por content-type no application/json")
		}
	})

	t.Run("Rechazo de cuerpo vacio", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/test", bytes.NewBufferString(""))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		var dst samplePayload
		err := httputil.DecodeJSON(rec, req, &dst, 1024)
		if err == nil {
			t.Fatal("se esperaba error por cuerpo vacio")
		}
		if !strings.Contains(err.Error(), "vacio") {
			t.Errorf("error inesperado: %v", err)
		}
	})

	t.Run("Rechazo de datos adicionales tras el valor principal JSON", func(t *testing.T) {
		body := `{"name":"Ahorro","amount":100}{"extra":"data"}`
		req := httptest.NewRequest(http.MethodPost, "/test", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		var dst samplePayload
		err := httputil.DecodeJSON(rec, req, &dst, 1024)
		if err == nil {
			t.Fatal("se esperaba error por datos residuales tras el json")
		}
	})
}
