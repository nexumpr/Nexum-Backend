package pushoutbox

import (
	"testing"
	"time"

	appPrefs "github.com/nexumpr/Nexum-Backend/internal/application/notificationprefs"
)

// TestQuietHoursBordesVentana cubre la ventana normal, el cruce de medianoche, la
// madrugada, el cambio de fecha y los bordes exactos de apertura y cierre. El fin
// devuelto debe ser el próximo instante de pared local, no una suma de minutos
// sobre la medianoche (esa suma se desplaza en los cambios de horario de verano).
func TestQuietHoursBordesVentana(t *testing.T) {
	cdmx, err := time.LoadLocation("America/Mexico_City")
	if err != nil {
		t.Fatalf("cargar zona: %v", err)
	}
	casos := []struct {
		nombre   string
		ventana  appPrefs.QuietHours
		ahora    time.Time
		enFranja bool
		finLocal time.Time
	}{
		{
			nombre:   "cruce de medianoche 23:30 dentro",
			ventana:  appPrefs.QuietHours{Enabled: true, Start: "22:00", End: "07:00"},
			ahora:    time.Date(2026, 11, 23, 23, 30, 0, 0, cdmx),
			enFranja: true,
			finLocal: time.Date(2026, 11, 24, 7, 0, 0, 0, cdmx),
		},
		{
			nombre:   "cruce de medianoche 00:30 dentro y mismo dia del fin",
			ventana:  appPrefs.QuietHours{Enabled: true, Start: "22:00", End: "07:00"},
			ahora:    time.Date(2026, 11, 24, 0, 30, 0, 0, cdmx),
			enFranja: true,
			finLocal: time.Date(2026, 11, 24, 7, 0, 0, 0, cdmx),
		},
		{
			nombre:   "cruce de medianoche 06:59 dentro",
			ventana:  appPrefs.QuietHours{Enabled: true, Start: "22:00", End: "07:00"},
			ahora:    time.Date(2026, 11, 24, 6, 59, 0, 0, cdmx),
			enFranja: true,
			finLocal: time.Date(2026, 11, 24, 7, 0, 0, 0, cdmx),
		},
		{
			nombre:   "cruce de medianoche 07:00 exacto fuera",
			ventana:  appPrefs.QuietHours{Enabled: true, Start: "22:00", End: "07:00"},
			ahora:    time.Date(2026, 11, 24, 7, 0, 0, 0, cdmx),
			enFranja: false,
			finLocal: time.Date(2026, 11, 25, 7, 0, 0, 0, cdmx),
		},
		{
			nombre:   "cruce de medianoche 22:00 exacto dentro",
			ventana:  appPrefs.QuietHours{Enabled: true, Start: "22:00", End: "07:00"},
			ahora:    time.Date(2026, 11, 23, 22, 0, 0, 0, cdmx),
			enFranja: true,
			finLocal: time.Date(2026, 11, 24, 7, 0, 0, 0, cdmx),
		},
		{
			nombre:   "madrugada 03:00 fuera de ventana diurna",
			ventana:  appPrefs.QuietHours{Enabled: true, Start: "09:00", End: "17:00"},
			ahora:    time.Date(2026, 11, 24, 3, 0, 0, 0, cdmx),
			enFranja: false,
			finLocal: time.Date(2026, 11, 24, 17, 0, 0, 0, cdmx),
		},
		{
			nombre:   "ventana diurna 12:00 dentro",
			ventana:  appPrefs.QuietHours{Enabled: true, Start: "09:00", End: "17:00"},
			ahora:    time.Date(2026, 11, 24, 12, 0, 0, 0, cdmx),
			enFranja: true,
			finLocal: time.Date(2026, 11, 24, 17, 0, 0, 0, cdmx),
		},
		{
			nombre:   "ventana diurna 17:00 exacto fuera",
			ventana:  appPrefs.QuietHours{Enabled: true, Start: "09:00", End: "17:00"},
			ahora:    time.Date(2026, 11, 24, 17, 0, 0, 0, cdmx),
			enFranja: false,
			finLocal: time.Date(2026, 11, 25, 17, 0, 0, 0, cdmx),
		},
		{
			nombre:   "cambio de fecha 23:59 dentro",
			ventana:  appPrefs.QuietHours{Enabled: true, Start: "23:00", End: "00:30"},
			ahora:    time.Date(2026, 12, 31, 23, 59, 0, 0, cdmx),
			enFranja: true,
			finLocal: time.Date(2027, 1, 1, 0, 30, 0, 0, cdmx),
		},
		{
			nombre:   "cambio de fecha 00:01 dentro",
			ventana:  appPrefs.QuietHours{Enabled: true, Start: "23:00", End: "00:30"},
			ahora:    time.Date(2027, 1, 1, 0, 1, 0, 0, cdmx),
			enFranja: true,
			finLocal: time.Date(2027, 1, 1, 0, 30, 0, 0, cdmx),
		},
		{
			nombre:   "cambio de fecha 00:30 exacto fuera",
			ventana:  appPrefs.QuietHours{Enabled: true, Start: "23:00", End: "00:30"},
			ahora:    time.Date(2027, 1, 1, 0, 30, 0, 0, cdmx),
			enFranja: false,
			finLocal: time.Date(2027, 1, 2, 0, 30, 0, 0, cdmx),
		},
	}

	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			enFranja, fin, err := enVentanaSilencio(caso.ahora, &caso.ventana, cdmx)
			if err != nil {
				t.Fatalf("enVentanaSilencio: %v", err)
			}
			if enFranja != caso.enFranja {
				t.Fatalf("enFranja=%v, esperado %v (ahora=%v)", enFranja, caso.enFranja, caso.ahora)
			}
			if !fin.Equal(caso.finLocal) {
				t.Fatalf("fin=%v, esperado %v", fin.In(cdmx), caso.finLocal)
			}
			if !fin.After(caso.ahora) {
				t.Fatalf("el fin de la franja debe ser futuro: fin=%v ahora=%v", fin, caso.ahora)
			}
			// «Sin bucle»: al llegar al fin reprogramado la comprobación ya no está en
			// franja, de modo que el trabajo se envía y no se reprograma otra vez.
			if caso.enFranja {
				siguienteEnFranja, _, err := enVentanaSilencio(fin, &caso.ventana, cdmx)
				if err != nil {
					t.Fatalf("enVentanaSilencio(fin): %v", err)
				}
				if siguienteEnFranja {
					t.Fatalf("bucle: %v sigue dentro de la franja tras reprogramar", fin.In(cdmx))
				}
			}
		})
	}
}

// TestQuietHoursHorarioDeVerano verifica que el fin de la franja usa la hora de
// pared local también en los cambios de horario de verano. Sumar la duración a la
// medianoche absoluta desplazaba el fin una hora (tarde en marzo, temprano en
// noviembre) y, en noviembre, podía reprogramar al mismo instante una y otra vez.
func TestQuietHoursHorarioDeVerano(t *testing.T) {
	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatalf("cargar America/New_York: %v", err)
	}
	cdmx, err := time.LoadLocation("America/Mexico_City")
	if err != nil {
		t.Fatalf("cargar America/Mexico_City: %v", err)
	}
	ventana := appPrefs.QuietHours{Enabled: true, Start: "22:00", End: "07:00"}

	casos := []struct {
		nombre string
		zona   *time.Location
		ahora  time.Time
		fin    time.Time
	}{
		{
			// 2026-03-08: a las 02:00 EST el reloj salta a 03:00 EDT.
			nombre: "Nueva York marzo 06:30 EDT",
			zona:   ny,
			ahora:  time.Date(2026, 3, 8, 6, 30, 0, 0, ny),
			fin:    time.Date(2026, 3, 8, 7, 0, 0, 0, ny),
		},
		{
			// 2026-11-01: a las 02:00 EDT el reloj retrocede a 01:00 EST.
			nombre: "Nueva York noviembre 06:30 EST",
			zona:   ny,
			ahora:  time.Date(2026, 11, 1, 6, 30, 0, 0, ny),
			fin:    time.Date(2026, 11, 1, 7, 0, 0, 0, ny),
		},
		{
			nombre: "Ciudad de Mexico marzo sin DST",
			zona:   cdmx,
			ahora:  time.Date(2026, 3, 8, 6, 30, 0, 0, cdmx),
			fin:    time.Date(2026, 3, 8, 7, 0, 0, 0, cdmx),
		},
		{
			nombre: "Ciudad de Mexico noviembre sin DST",
			zona:   cdmx,
			ahora:  time.Date(2026, 11, 1, 6, 30, 0, 0, cdmx),
			fin:    time.Date(2026, 11, 1, 7, 0, 0, 0, cdmx),
		},
	}

	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			enFranja, fin, err := enVentanaSilencio(caso.ahora, &ventana, caso.zona)
			if err != nil {
				t.Fatalf("enVentanaSilencio: %v", err)
			}
			if !enFranja {
				t.Fatalf("%v debería estar dentro de la ventana", caso.ahora.In(caso.zona))
			}
			if !fin.Equal(caso.fin) {
				t.Fatalf("fin=%v, esperado %v", fin.In(caso.zona), caso.fin)
			}
			if !fin.After(caso.ahora) {
				t.Fatalf("el fin debe ser futuro: fin=%v ahora=%v", fin, caso.ahora)
			}
			// Una sola reprogramación por ventana: al llegar al fin ya se envía.
			siguienteEnFranja, _, err := enVentanaSilencio(fin, &ventana, caso.zona)
			if err != nil {
				t.Fatalf("enVentanaSilencio(fin): %v", err)
			}
			if siguienteEnFranja {
				t.Fatalf("bucle en DST: %v seguiría en la franja", fin.In(caso.zona))
			}
		})
	}
}

// TestQuietHoursDeshabilitadoOInvalido comprueba que el servicio no bloquea envíos
// cuando la franja no está activa y que una franja imposible falla cerrada sin
// inventar un horario.
func TestQuietHoursDeshabilitadoOInvalido(t *testing.T) {
	cdmx, err := time.LoadLocation("America/Mexico_City")
	if err != nil {
		t.Fatalf("cargar zona: %v", err)
	}
	ahora := time.Date(2026, 11, 24, 12, 0, 0, 0, cdmx)

	if enFranja, fin, err := enVentanaSilencio(ahora, nil, cdmx); err != nil || enFranja || !fin.IsZero() {
		t.Fatalf("sin quietHours: enFranja=%v fin=%v err=%v", enFranja, fin, err)
	}
	deshabilitada := appPrefs.QuietHours{Enabled: false, Start: "22:00", End: "07:00"}
	if enFranja, _, err := enVentanaSilencio(ahora, &deshabilitada, cdmx); err != nil || enFranja {
		t.Fatalf("quietHours deshabilitado: enFranja=%v err=%v", enFranja, err)
	}
	for _, ventana := range []appPrefs.QuietHours{
		{Enabled: true, Start: "25:00", End: "07:00"},
		{Enabled: true, Start: "22:00", End: "7:0"},
		{Enabled: true, Start: "07:00", End: "07:00"},
		{Enabled: true, Start: "", End: ""},
	} {
		if _, _, err := enVentanaSilencio(ahora, &ventana, cdmx); err == nil {
			t.Fatalf("franja %+v debería rechazarse", ventana)
		}
	}
}
