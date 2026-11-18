package privacy

import (
	_ "embed"
	"errors"
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

	// TextoResumen es el resumen legible que acompaña al contenido íntegro.
	TextoResumen = "Nexum trata tu nombre, correo y contraseña (hash Argon2id) para autenticarte; " +
		"tu ingreso base y compromisos declarados para calcular su equivalente mensual; y datos " +
		"técnicos de sesión para proteger tu cuenta. La infraestructura es AWS us-east-2; no vendemos " +
		"tus datos ni los cedemos a redes publicitarias. Algunas funciones (movimientos, presupuestos, " +
		"metas, exportación, eliminación automática, notificaciones y análisis) aún no están disponibles; " +
		"este aviso se actualizará cuando lo estén. El aviso íntegro, con finalidades, derechos, " +
		"conservación y versiones, está en el campo content y en la URL de este endpoint."
)

// ErrFechaEfectivaNoAprobada indica que aún no existe una fecha de publicación
// aprobada. No se inventa ni se calcula con time.Now(): el endpoint queda
// bloqueado hasta que el responsable fije la fecha real.
var ErrFechaEfectivaNoAprobada = errors.New("fecha efectiva del aviso no aprobada")

// effectiveAtAviso es la fecha de entrada en vigor de esta versión. Es una
// configuración inmutable por versión, no se calcula por petición ni se
// retrotrae. Se fija a la primera publicación autorizada (2026-11-18) y el
// Gate de publicación la valida antes de promover.
var effectiveAtAviso = "2026-11-18T00:00:00Z"

// Notice es el aviso vigente servido por la API.
type Notice struct {
	Version            string
	EffectiveAt        time.Time
	Text               string
	URL                string
	Content            string
	RequiresAcceptance bool
}

// AvisoVigente devuelve el aviso sólo si existe una fecha efectiva aprobada.
// No consulta servicios externos ni datos de usuario.
func AvisoVigente() (Notice, error) {
	if effectiveAtAviso == "" {
		return Notice{}, ErrFechaEfectivaNoAprobada
	}
	effectiveAt, err := time.Parse(time.RFC3339, effectiveAtAviso)
	if err != nil {
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

// Proveedor implementa el puerto que consume el handler HTTP.
type Proveedor struct{}

// Current devuelve el aviso vigente o el error de bloqueo.
func (Proveedor) Current() (Notice, error) { return AvisoVigente() }

// Contenido expone el texto íntegro para pruebas de hash.
func Contenido() string { return contenidoAviso }
