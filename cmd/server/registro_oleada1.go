package main

import (
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"sync"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/redis/go-redis/v9"

	nexumhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
)

// dependenciasOleada agrupa las dependencias reales que recibe cada montaje de la
// Oleada 1. No se crea ninguna instancia nueva: se reutilizan DynamoDB, Redis,
// router y logger ya construidos en cmd/server/main.go.
type dependenciasOleada struct {
	Router *nexumhttp.Router
	Dynamo *dynamodb.Client
	Redis  *redis.Client
	Tabla  string
	Logger *slog.Logger
}

// montajeOleada declara un endpoint autorizado. La ruta es relativa a /api/v1:
// el router añade el prefijo y no se debe duplicar.
type montajeOleada struct {
	ID     string
	Metodo string
	Ruta   string
	Montar func(*dependenciasOleada)
}

// endpointsOleada1Autorizados es la lista cerrada del Gate H08-E Oleada 1.
// Un ID fuera de esta lista (por ejemplo E072 o una Oleada 2) se rechaza en el
// registro: no se pueden añadir endpoints futuros por accidente.
var endpointsOleada1Autorizados = map[string]struct{}{
	"E012": {},
	"E013": {},
	"E015": {},
	"E016": {},
	"E017": {},
	"E018": {},
	"E019": {},
	"E022": {},
	"E023": {},
	"E024": {},
	"E025": {},
	"E068": {},
	"E069": {},
	"E070": {},
	"E071": {},
}

var metodosOleada1 = map[string]struct{}{
	http.MethodGet:    {},
	http.MethodPost:   {},
	http.MethodPut:    {},
	http.MethodPatch:  {},
	http.MethodDelete: {},
}

// registroOleada1 es un registro en memoria, determinista y fail-closed.
type registroOleada1 struct {
	mu    sync.Mutex
	porID map[string]montajeOleada
}

func nuevoRegistroOleada1() *registroOleada1 {
	return &registroOleada1{porID: make(map[string]montajeOleada, len(endpointsOleada1Autorizados))}
}

// registrar valida el montaje y lo guarda. Un ID duplicado, una ruta duplicada,
// un método no admitido o un ID no autorizado provocan panic en el arranque:
// es preferible fallar antes de servir tráfico a registrar dos handlers sobre la
// misma ruta o habilitar un endpoint no aprobado.
func (r *registroOleada1) registrar(m montajeOleada) {
	if r == nil {
		panic("registro de oleada nulo")
	}
	if _, autorizado := endpointsOleada1Autorizados[m.ID]; !autorizado {
		panic(fmt.Sprintf("endpoint no autorizado en la Oleada 1: %q", m.ID))
	}
	metodo := strings.ToUpper(strings.TrimSpace(m.Metodo))
	if _, ok := metodosOleada1[metodo]; !ok {
		panic(fmt.Sprintf("%s: metodo HTTP no admitido: %q", m.ID, m.Metodo))
	}
	ruta := strings.TrimSpace(m.Ruta)
	if ruta == "" || !strings.HasPrefix(ruta, "/") || strings.HasPrefix(ruta, "/api/v1") || strings.ContainsAny(ruta, "?#") {
		panic(fmt.Sprintf("%s: ruta relativa invalida: %q", m.ID, m.Ruta))
	}
	if m.Montar == nil {
		panic(fmt.Sprintf("%s: funcion de montaje nula", m.ID))
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, existe := r.porID[m.ID]; existe {
		panic(fmt.Sprintf("endpoint duplicado en la Oleada 1: %s", m.ID))
	}
	for id, existente := range r.porID {
		if existente.Metodo == metodo && existente.Ruta == ruta {
			panic(fmt.Sprintf("ruta duplicada en la Oleada 1: %s %s (%s y %s)", metodo, ruta, id, m.ID))
		}
	}
	r.porID[m.ID] = montajeOleada{
		ID:     m.ID,
		Metodo: metodo,
		Ruta:   ruta,
		Montar: m.Montar,
	}
}

// montar ejecuta los montajes en orden determinista por ID. Sin registros no
// registra ninguna ruta: el andamiaje es válido con cero endpoints.
func (r *registroOleada1) montar(d *dependenciasOleada) {
	if r == nil || d == nil || d.Router == nil {
		return
	}

	r.mu.Lock()
	ids := make([]string, 0, len(r.porID))
	for id := range r.porID {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	pendientes := make([]montajeOleada, 0, len(ids))
	for _, id := range ids {
		pendientes = append(pendientes, r.porID[id])
	}
	r.mu.Unlock()

	for _, m := range pendientes {
		m.Montar(d)
	}
}

// registroOleada1Global es el registro que llenan los init() de cmd/server/eNNN.go.
var registroOleada1Global = nuevoRegistroOleada1()

// registrarOleada1 se invoca desde cada archivo de endpoint en su init().
// No hace I/O ni lee variables de entorno: sólo llena un mapa en memoria.
func registrarOleada1(m montajeOleada) {
	registroOleada1Global.registrar(m)
}

// montarOleada1 conecta todos los endpoints registrados. Se llama una sola vez
// desde run(), después de construir las dependencias reales.
func montarOleada1(d *dependenciasOleada) {
	registroOleada1Global.montar(d)
}
