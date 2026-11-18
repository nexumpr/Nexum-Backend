package privacy

import (
	_ "embed"
	"errors"
	"strings"
	"time"
)

// aviso_2026_11_v2.txt es el contenido íntegro propuesto. El SHA-256 se verifica
// en las pruebas y debe coincidir con el valor del Gate y de la autorización.
//
//go:embed aviso_2026_11_v2.txt
var contenidoAviso string

const (
	// VersionAviso es el identificador contractual de la versión.
	VersionAviso = "2026-11-v2"

	// URLAnuncio es la URL pública de este endpoint. Mientras la página externa
	// del equipo de privacidad no tenga contenido consistente, se usa la API para
	// no dirigir al usuario a información contradictoria. El handler no consulta
	// la página externa en cada petición.
	URLAnuncio = "https://d2lpmqy5w6x9es.cloudfront.net/api/v1/privacy/notices/current"

	// EffectiveAtEnvVar es la variable de entorno inmutable del release. Se
	// configura en el bundle RFC-004 durante la promoción; no requiere cambiar
	// código ni HEAD después de aprobar los merges.
	EffectiveAtEnvVar = "E015_EFFECTIVE_AT"

	// EffectiveAtMinimo impide fechas históricas anteriores a la primera
	// publicación autorizada. No se usa para calcular la fecha.
	EffectiveAtMinimo = "2026-11-18T00:00:00Z"

	// TextoResumen es el resumen legible que acompaña al contenido íntegro.
	TextoResumen = "Nexum trata tu nombre, correo y contraseña (hash Argon2id) para autenticarte; " +
		"tu ingreso base y compromisos declarados para calcular su equivalente mensual; y datos " +
		"técnicos de sesión para proteger tu cuenta. La infraestructura es AWS us-east-2; no vendemos " +
		"tus datos ni los cedemos a redes publicitarias. Algunas funciones (movimientos, presupuestos, " +
		"metas, exportación, eliminación automática, notificaciones y análisis) aún no están disponibles; " +
		"este aviso se actualizará cuando lo estén. El aviso íntegro, con finalidades, derechos, " +
		"conservación y versiones, está en el campo content y en la URL de este endpoint."
)

// ErrFechaEfectivaNoAprobada indica que falta una fecha de publicación válida.
// No se inventa ni se calcula con time.Now(): la release no se promueve sin la
// configuración del bundle.
var ErrFechaEfectivaNoAprobada = errors.New("fecha efectiva del aviso no configurada")

// ParseEffectiveAt valida RFC3339, normaliza a UTC y rechaza fechas anteriores
// a la primera publicación autorizada. La fecha puede ser futura; no se
// retrotrae.
func ParseEffectiveAt(raw string) (time.Time, error) {
	limpio := strings.TrimSpace(raw)
	if limpio == "" {
		return time.Time{}, ErrFechaEfectivaNoAprobada
	}
	fecha, err := time.Parse(time.RFC3339, limpio)
	if err != nil {
		return time.Time{}, ErrFechaEfectivaNoAprobada
	}
	fecha = fecha.UTC()
	minimo, _ := time.Parse(time.RFC3339, EffectiveAtMinimo)
	if fecha.Before(minimo) {
		return time.Time{}, ErrFechaEfectivaNoAprobada
	}
	return fecha, nil
}

// Proveedor es el aviso de una versión con su fecha efectiva inmutable.
type Proveedor struct {
	EffectiveAt time.Time
}

// ProveedorDesdeEntorno lee la fecha una sola vez del entorno del release. Es
// el mecanismo que usa cmd/server; no consulta la fecha por petición.
func ProveedorDesdeEntorno(getenv func(string) string) (Proveedor, error) {
	if getenv == nil {
		return Proveedor{}, ErrFechaEfectivaNoAprobada
	}
	fecha, err := ParseEffectiveAt(getenv(EffectiveAtEnvVar))
	if err != nil {
		return Proveedor{}, err
	}
	return Proveedor{EffectiveAt: fecha}, nil
}

// Notice es el aviso vigente servido por la API.
type Notice struct {
	Version            string
	EffectiveAt        time.Time
	Text               string
	URL                string
	Content            string
	RequiresAcceptance bool
}

// AvisoVigente construye el aviso con la fecha ya validada del release.
func AvisoVigente(effectiveAt time.Time) (Notice, error) {
	if effectiveAt.IsZero() {
		return Notice{}, ErrFechaEfectivaNoAprobada
	}
	return Notice{
		Version:            VersionAviso,
		EffectiveAt:        effectiveAt.UTC(),
		Text:               TextoResumen,
		URL:                URLAnuncio,
		Content:            contenidoAviso,
		RequiresAcceptance: true,
	}, nil
}

// Current implementa el puerto que consume el handler HTTP.
func (p Proveedor) Current() (Notice, error) { return AvisoVigente(p.EffectiveAt) }

// Contenido expone el texto íntegro para pruebas de hash.
func Contenido() string { return contenidoAviso }
