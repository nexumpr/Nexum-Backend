package privacy

import (
	_ "embed"
	"time"
)

// aviso_2026_11_v2.txt es el contenido íntegro aprobado por el responsable.
// El SHA-256 se verifica en las pruebas y debe coincidir con el valor del Gate.
//
//go:embed aviso_2026_11_v2.txt
var contenidoAviso string

const (
	// VersionAviso es el identificador contractual de la versión publicada.
	VersionAviso = "2026-11-v2"

	// URLAnuncio es la URL pública del propio endpoint. No es una web separada:
	// Android consume este JSON y muestra el aviso en su interfaz.
	URLAnuncio = "https://d2lpmqy5w6x9es.cloudfront.net/api/v1/privacy/notices/current"

	// EffectiveAtAviso es una constante fijada en el release que publica la
	// versión. No se calcula con time.Now() por petición ni es retroactiva.
	EffectiveAtAviso = "2026-11-18T00:00:00Z"

	// TextoResumen es el resumen legible que acompaña al contenido íntegro.
	TextoResumen = "Nexum trata tu nombre, correo y contraseña (hash Argon2id) para autenticarte; " +
		"tu ingreso base y compromisos declarados para calcular su equivalente mensual; y datos " +
		"técnicos de sesión para proteger tu cuenta. La infraestructura es AWS us-east-2; no vendemos " +
		"tus datos ni los cedemos a redes publicitarias. Algunas funciones (movimientos, presupuestos, " +
		"metas, exportación, eliminación automática, notificaciones y análisis) aún no están disponibles; " +
		"este aviso se actualizará cuando lo estén. El aviso íntegro, con finalidades, derechos, " +
		"conservación y versiones, está en el campo content y en la URL de este endpoint."
)

// Notice es el aviso vigente servido por la API.
type Notice struct {
	Version            string
	EffectiveAt        time.Time
	Text               string
	URL                string
	Content            string
	RequiresAcceptance bool
}

// AvisoVigente devuelve una copia del aviso publicado. Es determinista y no
// consulta servicios externos ni datos de usuario.
func AvisoVigente() Notice {
	effectiveAt, err := time.Parse(time.RFC3339, EffectiveAtAviso)
	if err != nil {
		// La constante es interna y está cubierta por pruebas; si se corrompe,
		// se devuelve la fecha cero y la prueba de contrato falla cerrada.
		effectiveAt = time.Time{}
	}
	return Notice{
		Version:            VersionAviso,
		EffectiveAt:        effectiveAt.UTC(),
		Text:               TextoResumen,
		URL:                URLAnuncio,
		Content:            contenidoAviso,
		RequiresAcceptance: true,
	}
}

// Contenido expone el texto íntegro para pruebas de hash.
func Contenido() string { return contenidoAviso }
