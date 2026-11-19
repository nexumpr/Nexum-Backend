package cache

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/redis/go-redis/v9"

	"github.com/nexumpr/Nexum-Backend/internal/ports"
)

// maxLotesInvalidacion acota el número de iteraciones de SCAN. Si se supera, se devuelve
// error en vez de permanecer en un bucle indefinido: el worker reprograma el trabajo y no
// declara PURGED con la caché del titular todavía poblada.
const maxLotesInvalidacion = 10_000

// tamanoLoteInvalidacion es el COUNT sugerido a Redis por iteración de SCAN. Redis puede
// devolver más o menos claves; el lote real lo decide el servidor.
const tamanoLoteInvalidacion = 500

// UserCacheInvalidator invalida todas las claves de caché de un usuario usando SCAN + DEL
// por lotes sobre el cliente Redis ya existente (A-080 §2.4).
//
// Se usa en la fase TERMINAL de la purga de E014, después de verificar que no quedan datos
// localizables. No se usa `KEYS`: bloquea el servidor y Redis lo desaconseja en producción.
//
// Seguridad: el prefijo debe terminar en `:` y no puede ser vacío. `UserCachePrefix` siempre
// cumple ambas condiciones, de modo que una llamada defectuosa no puede convertirse en un
// borrado global de la instancia.
type UserCacheInvalidator struct {
	client *redis.Client
	lote   int64
}

// NewUserCacheInvalidator construye el invalidador sobre el cliente Redis compartido. Sin
// cliente, DeletePrefix falla cerrado.
func NewUserCacheInvalidator(client *redis.Client) *UserCacheInvalidator {
	return &UserCacheInvalidator{client: client, lote: tamanoLoteInvalidacion}
}

// DeletePrefix borra todas las claves que comienzan por `prefix`.
//
// El patrón se construye con `prefix + "*"`. Como el prefijo canónico no contiene caracteres
// comodín (`nexum:v1:<env>:<hash>:`), el patrón no puede ensancharse por accidente.
func (i *UserCacheInvalidator) DeletePrefix(ctx context.Context, prefix string) error {
	if i == nil || i.client == nil {
		return errors.New("invalidador de cache no configurable")
	}
	prefijo := strings.TrimSpace(prefix)
	if prefijo == "" || !strings.HasSuffix(prefijo, ":") {
		// Un prefijo vacío o sin delimitador final podría borrar claves de otros usuarios.
		return fmt.Errorf("prefijo de invalidacion no seguro")
	}

	patron := prefijo + "*"
	var cursor uint64
	for lote := 0; lote < maxLotesInvalidacion; lote++ {
		claves, siguiente, err := i.client.Scan(ctx, cursor, patron, i.lote).Result()
		if err != nil {
			if esScanDenegado(err) {
				// Única denegación tolerable: el ACL de A-080 prohíbe SCAN. Se propaga un
				// sentinel tipado para que el worker distinga este caso de un DEL denegado,
				// un timeout o una indisponibilidad, que deben reintentarse.
				return fmt.Errorf("%w: %v", ports.ErrCacheScanDenegado, err)
			}
			return fmt.Errorf("scan de invalidacion: %w", err)
		}
		if len(claves) > 0 {
			if err := i.client.Del(ctx, claves...).Err(); err != nil {
				return fmt.Errorf("borrado de invalidacion: %w", err)
			}
		}
		cursor = siguiente
		if cursor == 0 {
			return nil
		}
	}
	return fmt.Errorf("invalidacion excedio %d lotes", maxLotesInvalidacion)
}

// esScanDenegado reconoce ÚNICAMENTE el NOPERM del comando SCAN. Un NOPERM de DEL u otra
// operación no cumple esta condición y conserva el reintento seguro: el worker nunca
// declara PURGED ante una denegación distinta.
func esScanDenegado(err error) bool {
	if err == nil {
		return false
	}
	txt := strings.ToUpper(err.Error())
	return strings.Contains(txt, "NOPERM") && strings.Contains(txt, "'SCAN'")
}
