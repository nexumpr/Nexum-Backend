package demobootstrap

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestParseEnabledFailClosed(t *testing.T) {
	casos := []struct {
		raw      string
		expected bool
		conError bool
	}{
		{"", false, false},
		{"false", false, false},
		{"FALSE", false, false},
		{"0", false, false},
		{"no", false, false},
		{"off", false, false},
		{"true", true, false},
		{"TRUE", true, false},
		{"1", true, false},
		{"yes", true, false},
		{"on", true, false},
		{"quizas", false, true},
	}
	for _, caso := range casos {
		t.Run(caso.raw, func(t *testing.T) {
			valor, err := ParseEnabled(caso.raw)
			if valor != caso.expected {
				t.Fatalf("ParseEnabled(%q)=%v, esperado %v", caso.raw, valor, caso.expected)
			}
			if (err != nil) != caso.conError {
				t.Fatalf("ParseEnabled(%q) error=%v, esperado error=%v", caso.raw, err, caso.conError)
			}
		})
	}
}

func TestEnabledFromEnvironment(t *testing.T) {
	tabla := map[string]string{EnvEnabled: "true"}
	lookup := func(clave string) (string, bool) {
		valor, ok := tabla[clave]
		return valor, ok
	}
	if !EnabledFromEnvironment(lookup) {
		t.Fatal("una configuración explícita true debe habilitar la demo")
	}

	delete(tabla, EnvEnabled)
	if EnabledFromEnvironment(lookup) {
		t.Fatal("sin variable debe prevalecer el valor por defecto false")
	}
	tabla[EnvEnabled] = "valor-invalido"
	if EnabledFromEnvironment(lookup) {
		t.Fatal("un valor no reconocido debe fallar cerrado a false")
	}
	if EnabledFromEnvironment(nil) {
		t.Fatal("un lookup nil debe fallar cerrado a false")
	}
}

func TestNewServiceFromEnvironmentUsaVariableControlada(t *testing.T) {
	t.Setenv(EnvEnabled, "true")
	if !NewServiceFromEnvironment().Bootstrap().Enabled {
		t.Fatal("el servicio debe tomar enabled del entorno")
	}
	t.Setenv(EnvEnabled, "false")
	if NewServiceFromEnvironment().Bootstrap().Enabled {
		t.Fatal("el servicio debe respetar la desactivación")
	}
}

func TestBootstrapSandboxAisladoYSinEfectos(t *testing.T) {
	var nilService *Service
	casos := []struct {
		nombre   string
		servicio *Service
		enabled  bool
	}{
		{"habilitado", NewService(true), true},
		{"desactivado", NewService(false), false},
		{"nil fail closed", nilService, false},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			respuesta := caso.servicio.Bootstrap()
			if respuesta.Enabled != caso.enabled || respuesta.Scope != ScopeSandbox {
				t.Fatalf("respuesta inesperada: %+v", respuesta)
			}
			bytes, err := json.Marshal(respuesta)
			if err != nil {
				t.Fatalf("json.Marshal: %v", err)
			}
			campos := map[string]any{}
			if err := json.Unmarshal(bytes, &campos); err != nil {
				t.Fatalf("json.Unmarshal: %v", err)
			}
			if len(campos) != 2 {
				t.Fatalf("la respuesta tiene campos extra: %s", bytes)
			}
		})
	}

	servicio := NewService(true)
	primero := servicio.Bootstrap()
	for i := 0; i < 20; i++ {
		if servicio.Bootstrap() != primero {
			t.Fatal("Bootstrap debe ser estable entre llamadas")
		}
	}

	// El servicio no puede contener almacenes ni clientes: su único estado es el
	// interruptor de configuración. Esto garantiza que un GET no escriba datos.
	tipo := reflect.TypeOf(Service{})
	for i := 0; i < tipo.NumField(); i++ {
		if tipo.Field(i).Type.Kind() != reflect.Bool {
			t.Fatalf("Service contiene una dependencia no booleana en %s", tipo.Field(i).Name)
		}
	}
}
