package system

import "errors"

var (
	// ErrBoundaryViolation se produce cuando una operación intenta violar las fronteras de A-000.
	ErrBoundaryViolation = errors.New("boundary violation: operacion fuera de los limites del Backend")

	// ErrInvalidActor se produce cuando el actor no está autorizado o reconocido.
	ErrInvalidActor = errors.New("actor invalido o no reconocido")

	// ErrClientBalanceAuthority se produce cuando el cliente intenta declarar o fijar un balance/saldo unilateralmente.
	ErrClientBalanceAuthority = errors.New("violacion de autoridad: el cliente no es autoridad de saldo, balance o calculo monetario")

	// ErrMissingPersistence se produce cuando no se define el backend de persistencia DynamoDB.
	ErrMissingPersistence = errors.New("persistencia DynamoDB requerida segun GOV-04/CTX-003")

	// ErrMissingCache se produce cuando no se define el backend de cache distribuida Redis.
	ErrMissingCache = errors.New("cache distribuida Redis requerida segun GOV-05/CTX-003")

	// ErrInvalidNodeTopology se produce cuando no se respeta la exigencia de 2 nodos independientes en produccion.
	ErrInvalidNodeTopology = errors.New("topologia invalida: se requieren al menos dos nodos simultaneos en produccion")
)

