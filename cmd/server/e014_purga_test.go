package main

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	nexumhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
)

// TestMontarE014SinPurgaResponde503 fija la invariante de E014: sin worker durable no se
// acepta una eliminación. Un 202 sin ejecutor sería una promesa falsa.
func TestMontarE014SinPurgaResponde503(t *testing.T) {
	router := nexumhttp.NewRouter()
	montarE014(&dependenciasOleada2{Router: router, Logger: slog.Default()})

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/api/v1/users/me", nil))

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("sin worker de purga se esperaba 503, se obtuvo %d", rec.Code)
	}
}

// TestMontarE014WorkerInactivoResponde503 cubre la caída posterior al arranque: el estado
// se consulta por petición y el endpoint deja de aceptar eliminaciones.
func TestMontarE014WorkerInactivoResponde503(t *testing.T) {
	router := nexumhttp.NewRouter()
	montarE014(&dependenciasOleada2{
		Router:      router,
		Logger:      slog.Default(),
		PurgaLista:  true,
		PurgaActiva: func() bool { return false },
	})

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/api/v1/users/me", nil))

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("con worker inactivo se esperaba 503, se obtuvo %d", rec.Code)
	}
}
