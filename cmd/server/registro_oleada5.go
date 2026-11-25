package main

import (
	"fmt"
	"log/slog"
	"os"
	"sort"
	"strings"
	"sync"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/redis/go-redis/v9"

	dynamoAdapter "github.com/nexumpr/Nexum-Backend/internal/adapters/dynamo"
	"github.com/nexumpr/Nexum-Backend/internal/application/demobootstrap"
	"github.com/nexumpr/Nexum-Backend/internal/ports"
	nexumhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
)

// dependenciasOleada5 agrupa las dependencias REALES de la Oleada 5. No crea
// clientes: reutiliza DynamoDB, Redis, router, material JWT y autorizador ya
// construidos en main.go. E072/E073 no se registran aquí: sus rutas ya viven en
// el HealthRegistry de Oleada 4 y duplicarlas rompería los probes del ALB.
type dependenciasOleada5 struct {
	Router     *nexumhttp.Router
	Dynamo     dynamoAdapter.DynamoAPI
	Redis      *redis.Client
	Tabla      string
	Logger     *slog.Logger
	Tokens     ports.TokenProvider
	Authorizer ports.SessionAuthorizer
	CursorKey  []byte

	// DemoEnabled se deriva de configuración controlada del entorno. Nunca se
	// fija a true en código y por defecto es false (E074 sandbox aislado).
	DemoEnabled bool
}

// montajeOleada5 declara un endpoint autorizado. La ruta es relativa a /api/v1.
type montajeOleada5 struct {
	ID     string
	Metodo string
	Ruta   string
	Montar func(*dependenciasOleada5)
}

// endpointsOleada5Autorizados es la lista CERRADA de la Oleada 5. E072/E073
// están fuera del registro porque son rutas existentes con contrato bloqueado;
// E068–E071 pertenecen a otra oleada y no aparecen aquí.
var endpointsOleada5Autorizados = map[string]struct{}{
	"E021": {}, "E064": {}, "E065": {}, "E066": {}, "E067": {},
	"E074": {}, "E075": {},
}

type registroOleada5 struct {
	mu    sync.Mutex
	porID map[string]montajeOleada5
}

func nuevoRegistroOleada5() *registroOleada5 {
	return &registroOleada5{porID: make(map[string]montajeOleada5, len(endpointsOleada5Autorizados))}
}

func (r *registroOleada5) registrar(m montajeOleada5) {
	if r == nil {
		panic("registro Oleada 5 nulo")
	}
	id := strings.TrimSpace(m.ID)
	if _, autorizado := endpointsOleada5Autorizados[id]; !autorizado {
		panic(fmt.Sprintf("endpoint no autorizado en la Oleada 5: %q", m.ID))
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
		panic(fmt.Sprintf("endpoint duplicado en la Oleada 5: %s", id))
	}
	for existenteID, existente := range r.porID {
		if existente.Metodo == metodo && existente.Ruta == ruta {
			panic(fmt.Sprintf("ruta duplicada en la Oleada 5: %s %s (%s y %s)", metodo, ruta, existenteID, id))
		}
	}
	r.porID[id] = montajeOleada5{ID: id, Metodo: metodo, Ruta: ruta, Montar: m.Montar}
}

func (r *registroOleada5) montar(d *dependenciasOleada5) {
	if r == nil || d == nil || d.Router == nil {
		return
	}
	r.mu.Lock()
	ids := make([]string, 0, len(r.porID))
	for id := range r.porID {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	pendientes := make([]montajeOleada5, 0, len(ids))
	for _, id := range ids {
		pendientes = append(pendientes, r.porID[id])
	}
	r.mu.Unlock()
	for _, m := range pendientes {
		m.Montar(d)
	}
}

var registroOleada5Global = nuevoRegistroOleada5()

func registrarOleada5(m montajeOleada5) { registroOleada5Global.registrar(m) }

func montarOleada5(d *dependenciasOleada5) { registroOleada5Global.montar(d) }

func oleada5DynamoCliente(d *dependenciasOleada5) (*dynamodb.Client, bool) {
	if d == nil || d.Dynamo == nil {
		return nil, false
	}
	cliente, ok := d.Dynamo.(*dynamodb.Client)
	return cliente, ok
}

func loggerOleada5(d *dependenciasOleada5) *slog.Logger {
	if d != nil && d.Logger != nil {
		return d.Logger
	}
	return slog.Default()
}

// demoHabilitadoOleada5 lee el interruptor explícito de E074. Por defecto es
// false: un despliegue productivo nunca expone el sandbox sin decisión expresa.
func demoHabilitadoOleada5() bool {
	return demobootstrap.EnabledFromEnvironment(os.LookupEnv)
}
