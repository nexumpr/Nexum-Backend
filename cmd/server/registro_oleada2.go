package main

import (
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/redis/go-redis/v9"

	dynamoAdapter "github.com/nexumpr/Nexum-Backend/internal/adapters/dynamo"
	"github.com/nexumpr/Nexum-Backend/internal/ports"
	nexumhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
)

// dependenciasOleada2 agrupa las dependencias REALES de la Oleada 2. No crea clientes:
// reutiliza DynamoDB, Redis, router, material JWT y autorizador de sesión ya construidos
// en cmd/server/main.go.
type dependenciasOleada2 struct {
	Router     *nexumhttp.Router
	Dynamo     dynamoAdapter.DynamoAPI
	Redis      *redis.Client
	Tabla      string
	Logger     *slog.Logger
	Tokens     ports.TokenProvider
	Authorizer ports.SessionAuthorizer
	// PurgaLista indica que el worker durable de E014 arrancó. Sin él, el endpoint
	// responde 503: nunca se acepta una eliminación sin ejecutor.
	PurgaLista bool
	// PurgaActiva consulta el estado vivo del worker (puede caer y reiniciarse). Si
	// devuelve false, el endpoint responde 503 en esa petición.
	PurgaActiva func() bool
}

// montajeOleada2 declara un endpoint autorizado. La ruta es relativa a /api/v1.
type montajeOleada2 struct {
	ID     string
	Metodo string
	Ruta   string
	Montar func(*dependenciasOleada2)
}

// endpointsOleada2Autorizados es la lista CERRADA del Gate H08-E Oleada 2
// (GATE_H08E_OLEADA_2.md §2). Un ID fuera de esta lista se rechaza en el arranque:
// no se pueden habilitar endpoints de otras oleadas por accidente.
var endpointsOleada2Autorizados = map[string]struct{}{
	"E014": {},
	"E027": {},
	"E028": {},
	"E029": {},
	"E030": {},
	"E031": {},
	"E032": {},
	"E033": {},
	"E034": {},
	"E035": {},
	"E036": {},
	"E038": {},
	"E039": {},
	"E040": {},
	"E041": {},
}

// registroOleada2 valida y conserva los montajes antes de conectarlos al router.
type registroOleada2 struct {
	mu    sync.Mutex
	porID map[string]montajeOleada2
}

func nuevoRegistroOleada2() *registroOleada2 {
	return &registroOleada2{porID: make(map[string]montajeOleada2, len(endpointsOleada2Autorizados))}
}

// registrar valida el montaje y lo guarda. Un ID no autorizado o duplicado, una ruta
// duplicada, un método no admitido o una función nula provocan panic en el arranque:
// es preferible fallar antes de servir tráfico.
func (r *registroOleada2) registrar(m montajeOleada2) {
	if r == nil {
		panic("registro Oleada 2 nulo")
	}
	id := strings.TrimSpace(m.ID)
	if _, autorizado := endpointsOleada2Autorizados[id]; !autorizado {
		panic(fmt.Sprintf("endpoint no autorizado en la Oleada 2: %q", m.ID))
	}
	metodo := strings.ToUpper(strings.TrimSpace(m.Metodo))
	if _, ok := metodosOleada1[metodo]; !ok {
		panic(fmt.Sprintf("%s: metodo HTTP no admitido: %q", id, m.Metodo))
	}
	ruta := strings.TrimSpace(m.Ruta)
	if ruta == "" || !strings.HasPrefix(ruta, "/") || strings.HasPrefix(ruta, "/api/") {
		panic(fmt.Sprintf("%s: ruta invalida (relativa a /api/v1): %q", id, m.Ruta))
	}
	if m.Montar == nil {
		panic(fmt.Sprintf("%s: funcion de montaje nula", id))
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, existe := r.porID[id]; existe {
		panic(fmt.Sprintf("endpoint duplicado en la Oleada 2: %s", id))
	}
	for existenteID, existente := range r.porID {
		if existente.Metodo == metodo && existente.Ruta == ruta {
			panic(fmt.Sprintf("ruta duplicada en la Oleada 2: %s %s (%s y %s)", metodo, ruta, existenteID, id))
		}
	}
	r.porID[id] = montajeOleada2{ID: id, Metodo: metodo, Ruta: ruta, Montar: m.Montar}
}

// ids devuelve los identificadores registrados en orden determinista.
func (r *registroOleada2) ids() []string {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	ids := make([]string, 0, len(r.porID))
	for id := range r.porID {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// montar ejecuta los montajes en orden determinista por ID. Sin registros no
// registra ninguna ruta: el andamiaje es válido con cero endpoints.
func (r *registroOleada2) montar(d *dependenciasOleada2) {
	if r == nil || d == nil || d.Router == nil {
		return
	}

	r.mu.Lock()
	ids := make([]string, 0, len(r.porID))
	for id := range r.porID {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	pendientes := make([]montajeOleada2, 0, len(ids))
	for _, id := range ids {
		pendientes = append(pendientes, r.porID[id])
	}
	r.mu.Unlock()

	for _, m := range pendientes {
		m.Montar(d)
	}
}

// registroOleada2Global es el registro que llenan los init() de cmd/server/e0NN.go.
var registroOleada2Global = nuevoRegistroOleada2()

// registrarOleada2 se invoca desde cada archivo de endpoint en su init().
// No hace I/O ni lee variables de entorno: sólo llena un mapa en memoria.
func registrarOleada2(m montajeOleada2) {
	registroOleada2Global.registrar(m)
}

// montarOleada2 conecta todos los endpoints registrados. Se llama una sola vez
// desde run(), después de construir las dependencias reales.
func montarOleada2(d *dependenciasOleada2) {
	registroOleada2Global.montar(d)
}

// oleada2DynamoCliente devuelve el cliente DynamoDB sólo si es utilizable. Los
// endpoints deben fallar cerrado (503) cuando falta, nunca operar sobre nil.
func oleada2DynamoCliente(d *dependenciasOleada2) (*dynamodb.Client, bool) {
	if d == nil || d.Dynamo == nil {
		return nil, false
	}
	cliente, ok := d.Dynamo.(*dynamodb.Client)
	return cliente, ok
}
