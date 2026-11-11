package identity

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	// tzdata embebido: la validacion de la zona IANA de `client.timeZone` debe dar
	// el mismo resultado en el binario linux/arm64 de las instancias, en los runners
	// de CI y en Windows. Sin esto dependeria de que el host tenga la base de zonas
	// instalada y una peticion valida podria rechazarse por el entorno.
	_ "time/tzdata"
)

// ErrInvalidClientContext indica un locale o una zona horaria no validos.
var ErrInvalidClientContext = errors.New("contexto de cliente invalido")

const maxLocaleRunes = 35
const maxTimeZoneRunes = 64

// localePattern acepta BCP-47 simplificado: idioma de 2-3 letras y hasta tres
// subtags alfanumericos de 2-8 caracteres (es-MX, en-US, es-419, zh-Hant-TW).
var localePattern = regexp.MustCompile(`^[A-Za-z]{2,3}(-[A-Za-z0-9]{2,8}){0,3}$`)

// ClientContext agrupa los datos no financieros que el cliente declara al registrarse.
// Ambos son opcionales. La zona horaria se valida contra la base IANA y se conserva
// para calculos futuros de meses, rachas y visualizacion; el servidor NO deriva de
// ella fechas de vencimiento.
type ClientContext struct {
	locale   string
	timeZone string
}

// NewClientContext valida y normaliza el contexto del cliente. Cadena vacia es valida.
func NewClientContext(locale, timeZone string) (ClientContext, error) {
	cleanLocale := strings.TrimSpace(locale)
	if cleanLocale != "" {
		if len(cleanLocale) > maxLocaleRunes || !localePattern.MatchString(cleanLocale) {
			return ClientContext{}, fmt.Errorf("%w: locale %q", ErrInvalidClientContext, locale)
		}
	}

	cleanZone := strings.TrimSpace(timeZone)
	if cleanZone != "" {
		if len(cleanZone) > maxTimeZoneRunes {
			return ClientContext{}, fmt.Errorf("%w: zona horaria demasiado larga", ErrInvalidClientContext)
		}
		if _, err := time.LoadLocation(cleanZone); err != nil {
			return ClientContext{}, fmt.Errorf("%w: zona horaria %q no es IANA valida", ErrInvalidClientContext, timeZone)
		}
	}

	return ClientContext{locale: cleanLocale, timeZone: cleanZone}, nil
}

// Locale retorna el locale normalizado (puede ser vacio).
func (c ClientContext) Locale() string { return c.locale }

// TimeZone retorna la zona IANA validada (puede ser vacia).
func (c ClientContext) TimeZone() string { return c.timeZone }
