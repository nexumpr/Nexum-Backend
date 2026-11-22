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
	appPrefs "github.com/nexumpr/Nexum-Backend/internal/application/notificationprefs"
	appPushDevices "github.com/nexumpr/Nexum-Backend/internal/application/pushdevices"
	"github.com/nexumpr/Nexum-Backend/internal/ports"
	nexumhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
)

// dependenciasOleada4 agrupa las dependencias REALES de la Oleada 4. No crea clientes:
// reutiliza DynamoDB, Redis, router, material JWT y autorizador ya construidos en main.go.
type dependenciasOleada4 struct {
	Router     *nexumhttp.Router
	Dynamo     dynamoAdapter.DynamoAPI
	Redis      *redis.Client
	Tabla      string
	Logger     *slog.Logger
	Tokens     ports.TokenProvider
	Authorizer ports.SessionAuthorizer

	// PushSealer cifra los tokens FCM con AES-GCM antes de persistirlos. Se
	// construye desde una clave de entorno/secret store; sin él E059/E060
	// responden 503 (fail-closed) y nunca escriben un token en claro.
	PushSealer appPushDevices.TokenSealer

	// CursorKey firma los cursores opacos de E061. Viene de una clave de entorno
	// estable (32 bytes); sin ella los handlers responden 503 en vez de generar
	// una clave efímera que invalidaría los cursores entre reinicios.
	CursorKey []byte

	// Preferencias es el mismo caso de uso expuesto por E057/E058. El worker FCM
	// lo consulta antes de enviar; sin él no envía nada (opt-out por defecto).
	Preferencias *appPrefs.Service
}

// montajeOleada4 declara un endpoint autorizado. La ruta es relativa a /api/v1.
type montajeOleada4 struct {
	ID     string
	Metodo string
	Ruta   string
	Montar func(*dependenciasOleada4)
}

// endpointsOleada4Autorizados es la lista CERRADA de la Oleada 4 (E052–E063).
// Sólo los E-ID con implementación real se registran en init(); un ID fuera de esta
// lista, un método/ruta duplicado o un montaje nulo provoca panic en arranque.
var endpointsOleada4Autorizados = map[string]struct{}{
	"E052": {}, "E053": {}, "E054": {}, "E055": {}, "E056": {}, "E057": {},
	"E058": {}, "E059": {}, "E060": {}, "E061": {}, "E062": {}, "E063": {},
}

type registroOleada4 struct {
	mu    sync.Mutex
	porID map[string]montajeOleada4
}

func nuevoRegistroOleada4() *registroOleada4 {
	return &registroOleada4{porID: make(map[string]montajeOleada4, len(endpointsOleada4Autorizados))}
}

func (r *registroOleada4) registrar(m montajeOleada4) {
	if r == nil {
		panic("registro Oleada 4 nulo")
	}
	id := strings.TrimSpace(m.ID)
	if _, autorizado := endpointsOleada4Autorizados[id]; !autorizado {
		panic(fmt.Sprintf("endpoint no autorizado en la Oleada 4: %q", m.ID))
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
		panic(fmt.Sprintf("endpoint duplicado en la Oleada 4: %s", id))
	}
	for existenteID, existente := range r.porID {
		if existente.Metodo == metodo && existente.Ruta == ruta {
			panic(fmt.Sprintf("ruta duplicada en la Oleada 4: %s %s (%s y %s)", metodo, ruta, existenteID, id))
		}
	}
	r.porID[id] = montajeOleada4{ID: id, Metodo: metodo, Ruta: ruta, Montar: m.Montar}
}

func (r *registroOleada4) ids() []string {
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

func (r *registroOleada4) montar(d *dependenciasOleada4) {
	if r == nil || d == nil || d.Router == nil {
		return
	}
	r.mu.Lock()
	ids := make([]string, 0, len(r.porID))
	for id := range r.porID {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	pendientes := make([]montajeOleada4, 0, len(ids))
	for _, id := range ids {
		pendientes = append(pendientes, r.porID[id])
	}
	r.mu.Unlock()
	for _, m := range pendientes {
		m.Montar(d)
	}
}

var registroOleada4Global = nuevoRegistroOleada4()

func registrarOleada4(m montajeOleada4) { registroOleada4Global.registrar(m) }

func montarOleada4(d *dependenciasOleada4) { registroOleada4Global.montar(d) }

func oleada4DynamoCliente(d *dependenciasOleada4) (*dynamodb.Client, bool) {
	if d == nil || d.Dynamo == nil {
		return nil, false
	}
	cliente, ok := d.Dynamo.(*dynamodb.Client)
	return cliente, ok
}
