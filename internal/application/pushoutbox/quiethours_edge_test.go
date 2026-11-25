package pushoutbox

import (
	"context"
	"testing"
	"time"

	appPrefs "github.com/nexumpr/Nexum-Backend/internal/application/notificationprefs"
)

// Pruebas de borde de quiet hours (E057/E058) y del cálculo de la próxima
// apertura que dispara la reprogramación del outbox.
//
// DEFECTO_DST_PENDIENTE — `enVentanaSilencio` (service.go:499) calcula el fin de
// la ventana como `medianoche.Add(minFin minutos)`, es decir suma tiempo
// transcurrido desde la medianoche local en lugar de fijar la hora de pared con
// `time.Date`. En un día con cambio de horario (23 o 25 horas) el resultado se
// desplaza una hora:
//
//   - 2026-03-08 America/New_York (23 h): devuelve 08:00 EDT en vez de 07:00.
//   - 2026-11-01 America/New_York (25 h): devuelve 06:00 EST en vez de 07:00, y
//     ese instante todavía cae dentro de la ventana: la reprogramación apunta a
//     un momento en el que el trabajo vuelve a bloquearse.
//
// Los casos afectados se marcan con t.Skip y la línea exacta. Al corregir
// El cálculo del fin de ventana usa hora de pared local, de modo que los días de 23 o 25
// horas (DST) no desplazan la apertura ni provocan reprogramaciones dentro de la propia
// ventana.
// tienen que pasar sin cambios de aserción. Ver
// dist/transversal/QUIET_HOURS_DEFECTOS.md.

type casoVentana struct {
	nombre      string
	inicio      string
	fin         string
	ahora       time.Time
	enVentana   bool
	finEsperado time.Time
}

func TestPUSHQuietHoursBordes(t *testing.T) {
	zonaMX, err := time.LoadLocation("America/Mexico_City")
	if err != nil {
		t.Fatalf("zona America/Mexico_City: %v", err)
	}
	zonaNY, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatalf("zona America/New_York: %v", err)
	}

	casos := []struct {
		zona *time.Location
		caso casoVentana
	}{
		// Ventana nocturna 22:00–07:00 en una zona sin horario de verano.
		{zonaMX, casoVentana{
			nombre: "mx-23:30-dentro", inicio: "22:00", fin: "07:00",
			ahora:       time.Date(2026, 11, 23, 5, 30, 0, 0, time.UTC), // 2026-11-22 23:30 local
			enVentana:   true,
			finEsperado: time.Date(2026, 11, 23, 7, 0, 0, 0, zonaMX),
		}},
		{zonaMX, casoVentana{
			nombre: "mx-00:30-dentro", inicio: "22:00", fin: "07:00",
			ahora:       time.Date(2026, 11, 23, 6, 30, 0, 0, time.UTC), // 2026-11-23 00:30 local
			enVentana:   true,
			finEsperado: time.Date(2026, 11, 23, 7, 0, 0, 0, zonaMX),
		}},
		{zonaMX, casoVentana{
			nombre: "mx-06:59-dentro", inicio: "22:00", fin: "07:00",
			ahora:       time.Date(2026, 11, 23, 12, 59, 0, 0, time.UTC), // 06:59 local
			enVentana:   true,
			finEsperado: time.Date(2026, 11, 23, 7, 0, 0, 0, zonaMX),
		}},
		{zonaMX, casoVentana{
			nombre: "mx-07:00-fuera", inicio: "22:00", fin: "07:00",
			ahora:     time.Date(2026, 11, 23, 13, 0, 0, 0, time.UTC), // 07:00 local
			enVentana: false,
		}},
		{zonaMX, casoVentana{
			nombre: "mx-21:59-fuera", inicio: "22:00", fin: "07:00",
			ahora:     time.Date(2026, 11, 24, 3, 59, 0, 0, time.UTC), // 21:59 local
			enVentana: false,
		}},

		// Ventana normal 09:00–17:00 (no cruza medianoche).
		{zonaMX, casoVentana{
			nombre: "mx-normal-09:00-dentro", inicio: "09:00", fin: "17:00",
			ahora:       time.Date(2026, 11, 23, 15, 0, 0, 0, time.UTC), // 09:00 local
			enVentana:   true,
			finEsperado: time.Date(2026, 11, 23, 17, 0, 0, 0, zonaMX),
		}},
		{zonaMX, casoVentana{
			nombre: "mx-normal-12:00-dentro", inicio: "09:00", fin: "17:00",
			ahora:       time.Date(2026, 11, 23, 18, 0, 0, 0, time.UTC), // 12:00 local
			enVentana:   true,
			finEsperado: time.Date(2026, 11, 23, 17, 0, 0, 0, zonaMX),
		}},
		{zonaMX, casoVentana{
			nombre: "mx-normal-16:59-dentro", inicio: "09:00", fin: "17:00",
			ahora:       time.Date(2026, 11, 23, 22, 59, 0, 0, time.UTC), // 16:59 local
			enVentana:   true,
			finEsperado: time.Date(2026, 11, 23, 17, 0, 0, 0, zonaMX),
		}},
		{zonaMX, casoVentana{
			nombre: "mx-normal-08:59-fuera", inicio: "09:00", fin: "17:00",
			ahora:     time.Date(2026, 11, 23, 14, 59, 0, 0, time.UTC), // 08:59 local
			enVentana: false,
		}},
		{zonaMX, casoVentana{
			nombre: "mx-normal-17:00-fuera", inicio: "09:00", fin: "17:00",
			ahora:     time.Date(2026, 11, 23, 23, 0, 0, 0, time.UTC), // 17:00 local
			enVentana: false,
		}},

		// America/New_York: 2026-03-08 es de 23 h (02:00 -> 03:00).
		{zonaNY, casoVentana{
			nombre: "ny-marzo-06:30-dentro", inicio: "22:00", fin: "07:00",
			ahora:       time.Date(2026, 3, 8, 10, 30, 0, 0, time.UTC), // 06:30 EDT
			enVentana:   true,
			finEsperado: time.Date(2026, 3, 8, 7, 0, 0, 0, zonaNY),
		}},
		{zonaNY, casoVentana{
			nombre: "ny-marzo-23:30-dentro", inicio: "22:00", fin: "07:00",
			ahora:       time.Date(2026, 3, 9, 3, 30, 0, 0, time.UTC), // 2026-03-08 23:30 EDT
			enVentana:   true,
			finEsperado: time.Date(2026, 3, 9, 7, 0, 0, 0, zonaNY),
		}},
		{zonaNY, casoVentana{
			nombre: "ny-marzo-07:00-fuera", inicio: "22:00", fin: "07:00",
			ahora:     time.Date(2026, 3, 8, 11, 0, 0, 0, time.UTC), // 07:00 EDT
			enVentana: false,
		}},

		// America/New_York: 2026-11-01 es de 25 h (02:00 -> 01:00).
		{zonaNY, casoVentana{
			nombre: "ny-noviembre-06:30-dentro", inicio: "22:00", fin: "07:00",
			ahora:       time.Date(2026, 11, 1, 11, 30, 0, 0, time.UTC), // 06:30 EST
			enVentana:   true,
			finEsperado: time.Date(2026, 11, 1, 7, 0, 0, 0, zonaNY),
		}},
		{zonaNY, casoVentana{
			nombre: "ny-noviembre-00:30-dentro", inicio: "22:00", fin: "07:00",
			ahora:       time.Date(2026, 11, 1, 4, 30, 0, 0, time.UTC), // 2026-11-01 00:30 EDT
			enVentana:   true,
			finEsperado: time.Date(2026, 11, 1, 7, 0, 0, 0, zonaNY),
		}},
		{zonaNY, casoVentana{
			nombre: "ny-noviembre-07:00-fuera", inicio: "22:00", fin: "07:00",
			ahora:     time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC), // 07:00 EST
			enVentana: false,
		}},
	}

	for _, entrada := range casos {
		zona := entrada.zona
		caso := entrada.caso
		t.Run(caso.nombre, func(t *testing.T) {
			// Cálculo de hora de pared corregido: los casos DST deben pasar sin cambios.
			q := &appPrefs.QuietHours{Enabled: true, Start: caso.inicio, End: caso.fin}
			enVentana, fin, err := enVentanaSilencio(caso.ahora, q, zona)
			if err != nil {
				t.Fatalf("enVentanaSilencio: %v", err)
			}
			if enVentana != caso.enVentana {
				t.Fatalf("enVentana=%v esperado %v (local %v)",
					enVentana, caso.enVentana, caso.ahora.In(zona))
			}
			if !caso.enVentana {
				return
			}
			if !fin.Equal(caso.finEsperado) {
				t.Fatalf("fin=%v esperado %v", fin.In(zona), caso.finEsperado.In(zona))
			}
			// Invariante anti-bucle: la reprogramación avanza al futuro y en ese
			// instante la ventana ya está cerrada.
			if !fin.After(caso.ahora) {
				t.Fatalf("la reprogramación no avanza: %v <= %v", fin, caso.ahora)
			}
			dentro, _, errFin := enVentanaSilencio(fin, q, zona)
			if errFin != nil {
				t.Fatalf("enVentanaSilencio(fin): %v", errFin)
			}
			if dentro {
				t.Fatalf("el fin de ventana %v sigue dentro de quiet hours", fin.In(zona))
			}
		})
	}
}

func TestPUSHQuietHoursConfiguracionInvalida(t *testing.T) {
	zona, err := time.LoadLocation("America/Mexico_City")
	if err != nil {
		t.Fatal(err)
	}
	ahora := time.Date(2026, 11, 23, 5, 30, 0, 0, time.UTC)
	casos := []struct {
		nombre string
		q      *appPrefs.QuietHours
	}{
		{"ausente", nil},
		{"deshabilitado", &appPrefs.QuietHours{Enabled: false, Start: "22:00", End: "07:00"}},
		{"sin-inicio", &appPrefs.QuietHours{Enabled: true, Start: "", End: "07:00"}},
		{"inicio-sin-cero", &appPrefs.QuietHours{Enabled: true, Start: "7:00", End: "17:00"}},
		{"hora-fuera-de-rango", &appPrefs.QuietHours{Enabled: true, Start: "24:00", End: "07:00"}},
		{"rango-vacio", &appPrefs.QuietHours{Enabled: true, Start: "22:00", End: "22:00"}},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			enVentana, fin, err := enVentanaSilencio(ahora, caso.q, zona)
			if caso.nombre == "ausente" || caso.nombre == "deshabilitado" {
				if err != nil || enVentana || !fin.IsZero() {
					t.Fatalf("ventana deshabilitada debe ser inocua: enVentana=%v fin=%v err=%v", enVentana, fin, err)
				}
				return
			}
			if err == nil {
				t.Fatalf("la configuración %q debía rechazarse", caso.nombre)
			}
			if enVentana {
				t.Fatalf("una configuración inválida no puede bloquear el envío")
			}
		})
	}
}

// TestPUSHQuietHoursUnaSolaReprogramacionPorVentana comprueba, sobre el worker
// real, que el bloqueo por quiet hours reprograma una única vez al cierre de la
// ventana, no vuelve a reclamar en el mismo instante y a la apertura envía.
func TestPUSHQuietHoursUnaSolaReprogramacionPorVentana(t *testing.T) {
	zona, err := time.LoadLocation("America/Mexico_City")
	if err != nil {
		t.Fatal(err)
	}
	store := nuevoStoreFalso()
	emisor := &emisorFalso{}
	prefs := &prefsFalsas{valor: preferencias(true, "America/Mexico_City", &appPrefs.QuietHours{
		Enabled: true, Start: "22:00", End: "07:00",
	})}
	tokens := &tokensFalsos{porUsuario: map[string][]TokenDestino{"u1": {{ID: "d1", Valor: "token"}}}}
	dentro := time.Date(2026, 11, 23, 5, 30, 0, 0, time.UTC) // 2026-11-22 23:30 local
	servicio := NewService(store, emisor, prefs, tokens, &bandejaFalsa{}, Opciones{
		Ahora: func() time.Time { return dentro },
		Lease: time.Minute,
	})
	id := encolarPrueba(t, servicio, "u1", "evt-ventana-unica")

	if aceptados := servicio.Ciclo(context.Background()); aceptados != 0 {
		t.Fatalf("quiet hours no debió enviar: %d", aceptados)
	}
	apertura := time.Date(2026, 11, 23, 7, 0, 0, 0, zona)
	estado := store.estado(id)
	if estado.Status != EstadoFallido || estado.UltimaClase != ClaseQuietHours {
		t.Fatalf("estado tras quiet hours: %+v", estado)
	}
	if !estado.NextAttemptAt.Equal(apertura) {
		t.Fatalf("reprogramación=%v esperada %v", estado.NextAttemptAt.In(zona), apertura.In(zona))
	}
	if !estado.NextAttemptAt.After(dentro) {
		t.Fatalf("la reprogramación no avanza al futuro: %v", estado.NextAttemptAt)
	}

	reclamos := store.totalReclamos()
	if aceptados := servicio.Ciclo(context.Background()); aceptados != 0 {
		t.Fatalf("la segunda pasada no debió enviar: %d", aceptados)
	}
	if store.totalReclamos() != reclamos {
		t.Fatal("la segunda pasada no debe reclamar un trabajo ya reprogramado")
	}
	if otraVez := store.estado(id); !otraVez.NextAttemptAt.Equal(apertura) || otraVez.Status != EstadoFallido {
		t.Fatalf("la reprogramación quedó inestable: %+v", otraVez)
	}

	servicio.opciones.Ahora = func() time.Time { return apertura }
	if aceptados := servicio.Ciclo(context.Background()); aceptados != 1 {
		t.Fatalf("a la apertura debía enviarse: %d", aceptados)
	}
	if emisor.total() != 1 || store.estado(id).Status != EstadoEnviado {
		t.Fatalf("cierre tras la ventana: envíos=%d estado=%+v", emisor.total(), store.estado(id))
	}
}
