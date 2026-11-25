package main

// Invariante de la semilla de pruebas del scheduler: además del recordatorio y
// su puntero, debe existir el puntero de agenda (`SCHEDULEPTR#`) con la SK global
// exacta. Sin él, el `Advance`/`Complete` real no puede renovar ni retirar la
// agenda y una prueba "verde" no reproduciría la composición de producción.
//
// Este archivo sustituye al diagnóstico temporal que ocupó este nombre durante
// la corrección del scheduler; no contiene pruebas de depuración.

import (
	"testing"
	"time"
)

func TestSchedulerOleada5SemillaIncluyePunteroDeAgenda(t *testing.T) {
	base := time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)
	dueAt := base.Add(-time.Hour)
	frecuencia := "daily"
	dynamo := nuevoSchedFakeSDK()
	skAgenda := schedSembrarRecordatorio(dynamo, "usuario-semilla", "r-semilla", "Pagar la luz", dueAt, &frecuencia)

	puntero := dynamo.items["USER#usuario-semilla\x00SCHEDULEPTR#r-semilla"]
	if puntero == nil {
		t.Fatal("la semilla no escribió el puntero de agenda")
	}
	if got := schedTexto(puntero["skGlobal"]); got != skAgenda {
		t.Fatalf("skGlobal=%q, esperado %q", got, skAgenda)
	}
	if got := schedTexto(puntero["reminderID"]); got != "r-semilla" {
		t.Fatalf("reminderID=%q", got)
	}
	if got := schedTexto(puntero["entityType"]); got != "REMINDER_SCHEDULE_POINTER" {
		t.Fatalf("entityType=%q", got)
	}
}
