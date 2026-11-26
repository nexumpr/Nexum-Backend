package ses

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aws/smithy-go"

	appIdentity "github.com/nexumpr/Nexum-Backend/internal/application/identity"
)

type errorTimeout struct{}

func (errorTimeout) Error() string   { return "i/o timeout" }
func (errorTimeout) Timeout() bool   { return true }
func (errorTimeout) Temporary() bool { return true }

// TestClasificarErrorVerificacion fija la separacion entre fallo ambiguo, transitorio
// y definitivo. La clase solo se usa en el log; ningun caso produce 202.
func TestClasificarErrorVerificacion(t *testing.T) {
	casos := []struct {
		nombre string
		ctx    context.Context
		err    error
		clase  error
	}{
		{"deadline", context.Background(), context.DeadlineExceeded, appIdentity.ErrEnvioAmbiguo},
		{"cancelado", context.Background(), context.Canceled, appIdentity.ErrEnvioAmbiguo},
		{"timeout de red", context.Background(), errorTimeout{}, appIdentity.ErrEnvioAmbiguo},
		{"throttling", context.Background(), &smithy.GenericAPIError{Code: "ThrottlingException"}, appIdentity.ErrEnvioTransitorio},
		{"limite de peticiones", context.Background(), &smithy.GenericAPIError{Code: "TooManyRequestsException"}, appIdentity.ErrEnvioTransitorio},
		{"permiso", context.Background(), &smithy.GenericAPIError{Code: "AccessDeniedException"}, appIdentity.ErrEnvioDefinitivo},
		{"mensaje rechazado", context.Background(), &smithy.GenericAPIError{Code: "MessageRejected"}, appIdentity.ErrEnvioDefinitivo},
		{"validacion", context.Background(), &smithy.GenericAPIError{Code: "ValidationException"}, appIdentity.ErrEnvioDefinitivo},
		{"desconocido", context.Background(), errors.New("otra cosa"), appIdentity.ErrEnvioDefinitivo},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			err := clasificarErrorVerificacion(caso.ctx, caso.err)
			if !errors.Is(err, caso.clase) {
				t.Fatalf("clase=%v, esperado %v", err, caso.clase)
			}
			// El mensaje original nunca se registra por el servicio; el error se conserva
			// envuelto solo para diagnostico local acotado.
			if err == nil {
				t.Fatal("el error no puede desaparecer")
			}
		})
	}
	if err := clasificarErrorVerificacion(context.Background(), nil); err != nil {
		t.Fatalf("nil debe seguir siendo nil: %v", err)
	}
	_ = time.Second
}
