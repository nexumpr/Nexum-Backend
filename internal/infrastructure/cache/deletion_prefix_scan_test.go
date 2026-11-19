package cache

import (
	"errors"
	"testing"
)

// TestEsScanDenegadoDistinguenComando fija que la excepcion de E014 se limita a SCAN.
// Un NOPERM de DEL u otra operacion, o un error desconocido, no la activan.
func TestEsScanDenegadoDistinguenComando(t *testing.T) {
	casos := []struct {
		nombre string
		err    error
		want   bool
	}{
		{"scan noperm", errors.New("NOPERM this user has no permissions to run the 'scan' command"), true},
		{"del noperm", errors.New("NOPERM this user has no permissions to run the 'del' command"), false},
		{"keys noperm", errors.New("NOPERM this user has no permissions to run the 'keys' command"), false},
		{"conexion", errors.New("dial tcp 10.0.0.5:6379: connect: connection refused"), false},
		{"timeout", errors.New("i/o timeout"), false},
		{"nil", nil, false},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			if got := esScanDenegado(caso.err); got != caso.want {
				t.Fatalf("esScanDenegado(%v) = %v, esperado %v", caso.err, got, caso.want)
			}
		})
	}
}
