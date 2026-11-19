package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"

	dynamoAdapter "github.com/nexumpr/Nexum-Backend/internal/adapters/dynamo"
	appAccount "github.com/nexumpr/Nexum-Backend/internal/application/account"
	cacheinfra "github.com/nexumpr/Nexum-Backend/internal/infrastructure/cache"
	"github.com/nexumpr/Nexum-Backend/internal/infrastructure/security"
	nexumhttp "github.com/nexumpr/Nexum-Backend/internal/transport/http"
	"github.com/nexumpr/Nexum-Backend/pkg/httputil"
)

// claveLiderPurga es la clave de liderazgo del worker en Redis. No es una autoridad de
// negocio: sólo evita que dos nodos recorran la misma cola; la exclusión real por trabajo la
// da el `fence` en DynamoDB.
const claveLiderPurga = "nexum:v1:worker:e014:leader"

func init() {
	// Se usa el registro global directamente: `registrarOleada2` puede no existir en todas
	// las revisiones del andamiaje, y `registroOleada2Global.registrar` es la superficie
	// estable. La validación (ID autorizado, método, ruta y duplicados) ocurre igualmente.
	registroOleada2Global.registrar(montajeOleada2{
		ID:     "E014",
		Metodo: http.MethodDelete,
		Ruta:   "/users/me",
		Montar: montarE014,
	})
}

// montarE014 conecta la solicitud de eliminación de cuenta (E014).
//
// Fail-closed: sin cliente DynamoDB, sin tabla, sin material JWT o sin autorizador de
// sesión, el handler responde 503. La composición productiva usa DynamoDB real; no se
// registran almacenes en memoria.
//
// `d.Dynamo` se declara como interfaz: el type-assert es explícito y su ausencia también
// falla cerrado (el adapter recibe un cliente nil y toda operación devuelve error).
func montarE014(d *dependenciasOleada2) {
	if d == nil || d.Router == nil {
		return
	}
	logger := d.Logger
	if logger == nil {
		logger = slog.Default()
	}

	// Sin worker arrancado no se acepta la solicitud: 202 sin ejecutor seria una promesa
	// falsa. El endpoint responde 503 y el resto de rutas sigue operativo.
	if !d.PurgaLista {
		logger.Error("E014: worker de purga no disponible; el endpoint respondera 503",
			slog.String("motivo", "purga_no_iniciada"))
		d.Router.HandleFunc(http.MethodDelete, "/users/me", func(w http.ResponseWriter, r *http.Request) {
			_ = httputil.WriteErrorEnvelope(w, http.StatusServiceUnavailable,
				"DEPENDENCY_UNAVAILABLE", "servicio no disponible", r.Header.Get("X-Trace-ID"), nil)
		})
		return
	}

	var cliente *dynamodb.Client
	if d.Dynamo != nil {
		cliente, _ = d.Dynamo.(*dynamodb.Client)
	}
	store := dynamoAdapter.NewDeletionStore(cliente, d.Tabla)
	servicio := appAccount.NewDeletionService(store, security.NewArgon2Hasher(nil))

	// El autorizador de sesión es obligatorio: la revocación efectiva de E014 se apoya en
	// que `AuthorizeSession` exige `PROFILE.status = ACTIVE` y la época vigente.
	autorizador := d.Authorizer
	if autorizador == nil && d.Dynamo != nil && d.Tabla != "" {
		autorizador = dynamoAdapter.NewLogoutStore(d.Dynamo, d.Tabla)
	}

	tokens := d.Tokens
	if tokens == nil {
		if proveedor, err := security.LoadTokenProvider(); err == nil {
			tokens = proveedor
		}
	}

	if cliente == nil || d.Tabla == "" || tokens == nil || autorizador == nil {
		logger.Warn("E014: componentes incompletos; el endpoint respondera 503",
			slog.String("motivo", "sin_persistencia_credenciales_o_autorizador"))
	}

	handler := nexumhttp.NewDeleteAccountHandler(servicio, tokens, autorizador, logger)
	// El estado del worker se consulta por petición: un worker caído no debe dejar el
	// endpoint aceptando trabajos que nadie ejecutará.
	protegido := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if d.PurgaActiva != nil && !d.PurgaActiva() {
			logger.ErrorContext(r.Context(), "E014: worker de purga inactivo; se rechaza la solicitud",
				slog.String("error_class", "purga_inactiva"))
			_ = httputil.WriteErrorEnvelope(w, http.StatusServiceUnavailable,
				"DEPENDENCY_UNAVAILABLE", "servicio no disponible", r.Header.Get("X-Trace-ID"), nil)
			return
		}
		handler.ServeHTTP(w, r)
	})
	d.Router.HandleFunc(http.MethodDelete, "/users/me", protegido)
}

// iniciarPurgaE014 arranca el worker durable de purga y devuelve la función de cierre.
//
// El worker vive dentro del binario Go existente (no hay servicio nuevo), se coordina con
// un lease en Redis y reclama cada trabajo con un `fence` creciente en DynamoDB. Si falta
// Redis o DynamoDB devuelve error: el llamador decide si el proceso puede servir tráfico sin
// purga (el endpoint seguiría aceptando trabajos durables, pero nadie los ejecutaría).
//
// La función de cierre cancela el contexto, espera a que la goroutine termine y es
// idempotente: se puede invocar más de una vez sin bloquear.
func iniciarPurgaE014(ctx context.Context, d *dependenciasOleada2) (func(), func() bool, error) {
	if d == nil {
		return nil, nil, errors.New("E014: dependencias nulas")
	}
	cliente, ok := d.Dynamo.(*dynamodb.Client)
	if !ok || cliente == nil || strings.TrimSpace(d.Tabla) == "" {
		return nil, nil, errors.New("E014: sin cliente DynamoDB o tabla; la purga no puede arrancar")
	}
	if d.Redis == nil {
		return nil, nil, errors.New("E014: sin Redis; la purga no puede coordinarse")
	}

	logger := d.Logger
	if logger == nil {
		logger = slog.Default()
	}

	store := dynamoAdapter.NewDeletionStore(cliente, d.Tabla)
	invalidador := cacheinfra.NewUserCacheInvalidator(d.Redis)
	lider, err := cacheinfra.NewWorkerElector(d.Redis, claveLiderPurga)
	if err != nil {
		return nil, nil, err
	}

	worker := appAccount.NewPurgeWorker(store, invalidador, lider, logger).
		WithEnvironment(os.Getenv("APP_ENV"))

	runCtx, cancel := context.WithCancel(ctx)
	terminado := make(chan struct{})
	var activa atomic.Bool

	// Supervisión mínima: si Run termina sin que el proceso se esté apagando, se reinicia
	// con espera acotada. `activa` refleja el estado real en cada petición, de modo que una
	// caída del worker no deja una bandera de disponibilidad permanentemente incorrecta.
	go func() {
		defer close(terminado)
		defer activa.Store(false)
		for {
			if runCtx.Err() != nil {
				return
			}
			activa.Store(true)
			worker.Run(runCtx)
			activa.Store(false)
			if runCtx.Err() != nil {
				return
			}
			logger.Error("E014: worker de purga termino inesperadamente; se reinicia",
				slog.String("error_class", "purga_reinicio"))
			select {
			case <-runCtx.Done():
				return
			case <-time.After(5 * time.Second):
			}
		}
	}()

	var unaVez sync.Once
	cerrar := func() {
		unaVez.Do(func() {
			cancel()
			<-terminado
		})
	}
	return cerrar, activa.Load, nil
}
