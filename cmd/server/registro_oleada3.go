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

// dependenciasOleada3 agrupa las dependencias REALES de la Oleada 3. Reutiliza DynamoDB,
// Redis, router, tokens y autorizador ya construidos en cmd/server/main.go.
type dependenciasOleada3 struct {
	Router     *nexumhttp.Router
	Dynamo     dynamoAdapter.DynamoAPI
	Redis      *redis.Client
	Tabla      string
	Logger     *slog.Logger
	Tokens     ports.TokenProvider
	Authorizer ports.SessionAuthorizer
}

// montajeOleada3 declara un endpoint autorizado. La ruta es relativa a /api/v1.
type montajeOleada3 struct {
	ID     string
	Metodo string
	Ruta   string
	Montar func(*dependenciasOleada3)
}

// endpointsOleada3Autorizados es la lista CERRADA del Gate H08-E Oleada 3
// (GATE_H08E_OLEADA_3.md §2). Un ID fuera de esta lista se rechaza en el arranque.
var endpointsOleada3Autorizados = map[string]struct{}{
	"E020": {}, "E026": {}, "E037": {}, "E042": {}, "E043": {},
	"E044": {}, "E045": {}, "E046": {}, "E047": {}, "E048": {},
	"E049": {}, "E050": {}, "E051": {},
}

type registroOleada3 struct {
	mu    sync.Mutex
	porID map[string]montajeOleada3
}

func nuevoRegistroOleada3() *registroOleada3 {
	return &registroOleada3{porID: make(map[string]montajeOleada3, len(endpointsOleada3Autorizados))}
}

func (r *registroOleada3) registrar(m montajeOleada3) {
	if r == nil {
		panic("registro Oleada 3 nulo")
	}
	id := strings.TrimSpace(m.ID)
	if _, autorizado := endpointsOleada3Autorizados[id]; !autorizado {
		panic(fmt.Sprintf("endpoint no autorizado en la Oleada 3: %q", m.ID))
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
		panic(fmt.Sprintf("endpoint duplicado en la Oleada 3: %s", id))
	}
	for existenteID, existente := range r.porID {
		if existente.Metodo == metodo && existente.Ruta == ruta {
			panic(fmt.Sprintf("ruta duplicada en la Oleada 3: %s %s (%s y %s)", metodo, ruta, existenteID, id))
		}
	}
	r.porID[id] = montajeOleada3{ID: id, Metodo: metodo, Ruta: ruta, Montar: m.Montar}
}

func (r *registroOleada3) ids() []string {
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

func (r *registroOleada3) montar(d *dependenciasOleada3) {
	if r == nil || d == nil || d.Router == nil {
		return
	}
	r.mu.Lock()
	ids := make([]string, 0, len(r.porID))
	for id := range r.porID {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	pendientes := make([]montajeOleada3, 0, len(ids))
	for _, id := range ids {
		pendientes = append(pendientes, r.porID[id])
	}
	r.mu.Unlock()
	for _, m := range pendientes {
		m.Montar(d)
	}
}

var registroOleada3Global = nuevoRegistroOleada3()

func registrarOleada3(m montajeOleada3) { registroOleada3Global.registrar(m) }

func montarOleada3(d *dependenciasOleada3) { registroOleada3Global.montar(d) }

func oleada3DynamoCliente(d *dependenciasOleada3) (*dynamodb.Client, bool) {
	if d == nil || d.Dynamo == nil {
		return nil, false
	}
	cliente, ok := d.Dynamo.(*dynamodb.Client)
	return cliente, ok
}
