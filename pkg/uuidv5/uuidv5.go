// Package uuidv5 queda retirado del código activo.
//
// La generación determinista de identificadores usa UUID v8 con SHA-256 en
// `pkg/uuidv8` (decisión D-E062-01): el contrato E062 exige `format: uuid` y no
// fija la versión, y `crypto/sha1` dispara G401/G505 en gosec, que no se
// silencian con `#nosec`. Este paquete se conserva vacío para no romper el
// historial de rutas; no debe importarse.
package uuidv5
