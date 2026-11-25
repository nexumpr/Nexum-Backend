// Package content contiene el contenido editorial publico de Nexum: las preguntas
// frecuentes (E070) y los consejos educativos (E071).
//
// El contenido es estatico, se versiona en codigo y no contiene datos de usuario ni
// depende de servicios externos. Las respuestas describen comportamientos reales del
// backend (validez de tokens, un solo uso de codigos, control de version) y evitan
// marcas, productos financieros y recomendaciones de inversion personalizadas.
//
// La version forma parte de la clave de cache publica: al cambiar el texto debe
// cambiarse Version, de modo que ninguna instancia sirva contenido obsoleto.
package content

import (
	"encoding/json"
	"time"
)

// Version identifica el contenido curado. Cambia cuando cambia cualquier texto.
const Version = "2026-11-v2"

// Claves de cache publica. No llevan identidad: el mismo contenido se sirve a
// cualquier visitante y nunca se mezcla con datos de usuario.
const (
	CacheKeyFAQ  = "nexum:v1:content:faq:" + Version
	CacheKeyTips = "nexum:v1:content:tips:" + Version
)

// CacheTTL es el tiempo de vida de la copia en Redis. Coincide con el max-age
// anunciado a los clientes para no servir dos antiguedades distintas.
const CacheTTL = 300 * time.Second

// Faq es una entrada del contrato PageOfFaq (E070). El orden de los campos es el
// del contrato y no se admiten campos adicionales.
type Faq struct {
	ID       string `json:"id"`
	Question string `json:"question"`
	Answer   string `json:"answer"`
}

// FaqPage es la respuesta de E070. No incluye nextCursor porque el contrato no
// declara parametros de paginacion: el contenido completo cabe en una respuesta.
type FaqPage struct {
	Items []Faq `json:"items"`
}

// Tip es una entrada del contrato PageOfTip (E071).
type Tip struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Text  string `json:"text"`
}

// TipPage es la respuesta de E071, tampoco paginada.
type TipPage struct {
	Items []Tip `json:"items"`
}

// faqs es el contenido curado de E070: diez preguntas sobre los flujos que el
// backend ya ofrece (registro, acceso, sesion, renovacion, recuperacion, perfil,
// consentimientos, configuracion inicial, ingreso y preferencias).
var faqs = []Faq{
	{
		ID:       "faq-01",
		Question: "¿Cómo creo mi cuenta en Nexum?",
		Answer: "En el registro indicas tu nombre, tu correo, una contraseña de 12 a 128 caracteres, " +
			"tu ingreso habitual y tus compromisos fijos; puedes registrar cero compromisos. También aceptas " +
			"el aviso de privacidad vigente. El alta se guarda completa en una sola operación: si falta un dato " +
			"obligatorio no se crea una cuenta a medias. Al terminar no se abre sesión, así que después inicias " +
			"sesión con el correo y la contraseña que registraste.",
	},
	{
		ID:       "faq-02",
		Question: "¿Por qué el mensaje de error de acceso es siempre el mismo?",
		Answer: "Porque el servidor no revela si un correo está registrado. Un correo inexistente, una contraseña " +
			"incorrecta y una cuenta deshabilitada producen la misma respuesta. Además, tras varios intentos fallidos " +
			"desde la misma red o contra la misma cuenta se aplica una espera temporal antes de aceptar otro intento, " +
			"para frenar la fuerza bruta.",
	},
	{
		ID:       "faq-03",
		Question: "¿Cuánto dura mi sesión y qué ocurre si la app queda abierta?",
		Answer: "Al iniciar sesión recibes un token de acceso válido 15 minutos y un token de renovación. Mientras " +
			"la sesión siga activa, la app renueva el acceso sin volver a pedirte la contraseña. Cada solicitud " +
			"protegida se valida contra el estado real de la sesión guardada en el servidor, de modo que una sesión " +
			"cerrada o revocada deja de funcionar de inmediato aunque el token no haya caducado.",
	},
	{
		ID:       "faq-04",
		Question: "¿Qué pasa cuando renuevo el acceso o cierro sesión?",
		Answer: "Cada renovación entrega un token de renovación nuevo y consume el anterior; un token ya consumido " +
			"no sirve dos veces. Si el servidor detecta que se intentó reutilizar uno consumido, cierra la sesión " +
			"completa por seguridad. Al cerrar sesión, la sesión y su token de renovación quedan revocados y las " +
			"renovaciones posteriores se rechazan. También puedes listar tus sesiones activas y cerrar una distinta " +
			"de la actual.",
	},
	{
		ID:       "faq-05",
		Question: "Olvidé mi contraseña, ¿cómo la recupero?",
		Answer: "Solicita el restablecimiento desde la pantalla de acceso. La respuesta es la misma exista o no la " +
			"cuenta, para no revelar correos registrados. Si la cuenta existe recibirás un enlace con un código de un " +
			"solo uso que caduca a los 30 minutos. Al guardar la nueva contraseña el código queda consumido y " +
			"reutilizarlo devuelve un conflicto; las sesiones abiertas con la contraseña anterior dejan de ser válidas.",
	},
	{
		ID:       "faq-06",
		Question: "¿Cómo actualizo mi nombre sin pisar cambios de otro dispositivo?",
		Answer: "El perfil se consulta y se modifica con control de versión: cada lectura devuelve una versión y cada " +
			"cambio debe indicar la versión que leyó. Si otro dispositivo guardó antes, la actualización se rechaza con " +
			"un conflicto en lugar de sobrescribir el cambio ajeno; vuelve a leer y aplica tu ajuste. En esta versión " +
			"solo se modifica el nombre visible: el correo de registro no se edita desde la app y el perfil indica si " +
			"ese correo ya está verificado.",
	},
	{
		ID:       "faq-07",
		Question: "¿Dónde consulto y cambio mis consentimientos de privacidad?",
		Answer: "En la sección de privacidad consultas el aviso vigente y las decisiones que ya registraste. Cada " +
			"decisión conserva el documento, su versión y el instante en que la aceptaste o la retiraste. Ningún " +
			"consentimiento opcional se marca por omisión: solo queda aceptado cuando lo confirmas, y retirarlo queda " +
			"registrado igual que la aceptación.",
	},
	{
		ID:       "faq-08",
		Question: "¿Puedo dejar la configuración inicial a medias y continuar después?",
		Answer: "Sí. El avance de la configuración inicial se guarda paso a paso y puedes retomarlo más adelante. " +
			"Solo se aceptan transiciones válidas y cada actualización conserva su versión, así que la app reanuda sin " +
			"repetir lo ya capturado. Cuando cambie tu situación puedes volver a ajustar tu ingreso y tus compromisos " +
			"fijos.",
	},
	{
		ID:       "faq-09",
		Question: "¿Cómo se registra y se actualiza mi ingreso habitual?",
		Answer: "Se captura como monto con dos decimales, sin símbolo de moneda, junto con la frecuencia con la que lo " +
			"recibes: semanal, quincenal, mensual, bimestral, trimestral, semestral o anual. Se guarda en centavos para " +
			"evitar errores de redondeo y el cálculo del mes usa el valor vigente con su frecuencia, no un promedio de " +
			"meses anteriores. Actualízalo cuando cambie: el nuevo valor aplica en el siguiente cálculo.",
	},
	{
		ID:       "faq-10",
		Question: "¿Cómo ajusto el idioma y la zona horaria?",
		Answer: "En preferencias eliges el idioma y tu zona horaria IANA. Los valores se validan en el servidor y se " +
			"guardan con versión para no pisar ajustes hechos en otro dispositivo. En esta versión todavía no se envían " +
			"notificaciones ni alertas push, de presupuesto o de recordatorios: las preferencias de avisos no están " +
			"disponibles y ningún aviso se activa por omisión.",
	},
}

// FAQs devuelve una copia del contenido de E070. La copia evita que un llamador
// modifique el contenido compartido del proceso.
func FAQs() []Faq {
	copia := make([]Faq, len(faqs))
	copy(copia, faqs)
	return copia
}

// FAQPayload serializa la respuesta completa de E070. Se recalcula solo cuando la
// cache publica no tiene la entrada; el resultado es determinista.
func FAQPayload() ([]byte, error) {
	return json.Marshal(FaqPage{Items: FAQs()})
}

// tips es el contenido curado de E071: ocho consejos generales de finanzas
// personales. Son educativos, no personalizados y no recomiendan productos ni
// inversiones concretas.
var tips = []Tip{
	{
		ID:    "tip-01",
		Title: "Anota el gasto el mismo día",
		Text: "Registrar el movimiento cuando ocurre toma segundos y evita reconstruir el mes de memoria. " +
			"Los gastos pequeños que se olvidan son los que más distorsionan el disponible.",
	},
	{
		ID:    "tip-02",
		Title: "Separa lo fijo de lo variable",
		Text: "Clasifica primero los compromisos fijos: renta, servicios, transporte y pagos recurrentes. " +
			"Saber cuánto está comprometido antes de empezar el mes deja claro qué margen real queda para el resto.",
	},
	{
		ID:    "tip-03",
		Title: "Anticipa las fechas de cobro",
		Text: "Revisa en qué día del mes se cobran tus compromisos y comprueba que el saldo alcance para todos. " +
			"Adelantar un pago unos días cuando hay margen evita cargos por falta de fondos.",
	},
	{
		ID:    "tip-04",
		Title: "Construye un colchón para imprevistos",
		Text: "Antes de metas grandes, separa un fondo para emergencias. Un punto de partida frecuente es cubrir " +
			"entre tres y seis meses de gastos fijos; ajusta la cifra a tu situación y revísala cada trimestre.",
	},
	{
		ID:    "tip-05",
		Title: "Pon límites por categoría",
		Text: "Un límite mensual por categoría convierte un deseo difuso en una cifra comprobable. Revisa el avance " +
			"a mitad de mes: corregir a tiempo es más fácil que justificar el exceso al final.",
	},
	{
		ID:    "tip-06",
		Title: "Define metas con monto y fecha",
		Text: "Una meta sirve cuando tiene monto objetivo y fecha. Divide el total entre las aportaciones que puedes " +
			"sostener y, si un mes no llegas, ajusta la fecha en lugar de cubrir el hueco con deuda cara.",
	},
	{
		ID:    "tip-07",
		Title: "Revisa suscripciones y cargos recurrentes",
		Text: "Los cobros automáticos pasan desapercibidos. Una vez al mes revisa tus suscripciones y cancela las que " +
			"no usas: liberan dinero cada mes sin cambiar tu día a día.",
	},
	{
		ID:    "tip-08",
		Title: "Mide tu avance cada mes",
		Text: "Compara lo gastado con lo que ingresó y con lo presupuestado. Un mes cerrado con números claros es el " +
			"mejor punto de partida para el siguiente. Estos consejos son generales y educativos: no sustituyen " +
			"asesoría financiera personalizada ni constituyen una recomendación de inversión.",
	},
}

// Tips devuelve una copia del contenido de E071, por la misma razón que FAQs: el
// contenido compartido del proceso no debe ser modificable por un llamador.
func Tips() []Tip {
	copia := make([]Tip, len(tips))
	copy(copia, tips)
	return copia
}

// TipsPayload serializa la respuesta completa de E071.
func TipsPayload() ([]byte, error) {
	return json.Marshal(TipPage{Items: Tips()})
}
