package cache

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestGetOrLoadJSONSinRedis(t *testing.T) {
	esperado := []byte(`{"items":[]}`)
	datos, err := GetOrLoadJSON(context.Background(), nil, "nexum:v1:test", time.Minute, func() ([]byte, error) {
		return esperado, nil
	})
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if string(datos) != string(esperado) {
		t.Fatalf("datos inesperados: %s", datos)
	}
}

func TestGetOrLoadJSONPropagaErrorDeCarga(t *testing.T) {
	_, err := GetOrLoadJSON(context.Background(), nil, "nexum:v1:test", time.Minute, func() ([]byte, error) {
		return nil, errors.New("fallo de carga")
	})
	if err == nil {
		t.Fatal("se esperaba error de carga")
	}
}

func TestGetOrLoadJSONRechazaCargaNula(t *testing.T) {
	if _, err := GetOrLoadJSON(context.Background(), nil, "nexum:v1:test", time.Minute, nil); err == nil {
		t.Fatal("se esperaba error por carga nula")
	}
}
