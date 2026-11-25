package uuidv8_test

import (
	"regexp"
	"strings"
	"testing"

	"github.com/nexumpr/Nexum-Backend/pkg/uuidv8"
)

var patronCanonico = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-8[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func TestUUIDv8NamespacesYFormato(t *testing.T) {
	u := uuidv8.New(uuidv8.NamespaceNexum, "usuario-1|evento-1")
	texto := u.String()
	if len(texto) != 36 {
		t.Fatalf("longitud %d: %q", len(texto), texto)
	}
	if !patronCanonico.MatchString(texto) {
		t.Fatalf("no es UUID v8 canonico: %q", texto)
	}
	if !uuidv8.EsV8(u) {
		t.Fatalf("EsV8 debe aceptar %q", texto)
	}
	if texto != strings.ToLower(texto) {
		t.Fatalf("el UUID debe ser minusculas: %q", texto)
	}
	// Indices canonicos: version en 14 y variante en 19.
	if texto[14] != '8' {
		t.Fatalf("version distinta de 8: %q", texto)
	}
	if !strings.ContainsRune("89ab", rune(texto[19])) {
		t.Fatalf("variante RFC 4122 invalida: %q", texto)
	}
}

func TestUUIDv8DeterminismoYDistincion(t *testing.T) {
	base := uuidv8.New(uuidv8.NamespaceNexum, "usuario-1|evento-1")
	if otra := uuidv8.New(uuidv8.NamespaceNexum, "usuario-1|evento-1"); otra != base {
		t.Fatalf("debe ser determinista: %s vs %s", base, otra)
	}
	if otra := uuidv8.New(uuidv8.NamespaceNexum, "usuario-1|evento-2"); otra == base {
		t.Fatalf("otro nombre debe producir otro UUID: %s", otra)
	}
	if otra := uuidv8.New(uuidv8.NamespaceNexum, "usuario-2|evento-1"); otra == base {
		t.Fatalf("otro usuario debe producir otro UUID: %s", otra)
	}
}

func TestUUIDv8NamespacesDistintosProducenIDsDistintos(t *testing.T) {
	otroNamespace := uuidv8.New(uuidv8.NamespaceNexum, "namespace-secundario")
	if otroNamespace == uuidv8.NamespaceNexum {
		t.Fatal("el namespace secundario no puede coincidir con el de Nexum")
	}
	a := uuidv8.New(uuidv8.NamespaceNexum, "mismo-nombre")
	b := uuidv8.New(otroNamespace, "mismo-nombre")
	if a == b {
		t.Fatalf("namespaces distintos no pueden compartir UUID: %s", a)
	}
	if a.String() == uuidv8.NamespaceNexum.String() {
		t.Fatal("New no debe devolver el namespace como UUID derivado")
	}
}

func TestUUIDv8NoEsSHA1(t *testing.T) {
	u := uuidv8.New(uuidv8.NamespaceNexum, "deteccion-de-sha1")
	if !uuidv8.EsV8(u) {
		t.Fatalf("New debe emitir version 8 y variante RFC 4122: %s", u)
	}
	if u[6]>>4 == 5 {
		t.Fatalf("New no puede emitir version 5 (SHA-1): %s", u)
	}
	// Un UUID v5 real (DNS/example.com, SHA-1) no debe pasar EsV8.
	v5 := uuidv8.MustParse("886313e1-3b8a-5372-9b90-0c9aee199e5d")
	if uuidv8.EsV8(v5) {
		t.Fatalf("EsV8 no debe aceptar un v5: %s", v5)
	}
}

func TestUUIDv8ParseYMustParse(t *testing.T) {
	texto := uuidv8.New(uuidv8.NamespaceNexum, "roundtrip").String()
	u, err := uuidv8.Parse("  " + texto + "  ")
	if err != nil {
		t.Fatal(err)
	}
	if u.String() != texto {
		t.Fatalf("roundtrip roto: %s vs %s", u, texto)
	}
	if _, err := uuidv8.Parse(strings.ReplaceAll(texto, "-", "")); err == nil {
		t.Fatal("la forma compacta de 32 hex debe rechazarse")
	}
	if _, err := uuidv8.Parse("no-es-un-uuid"); err == nil {
		t.Fatal("un texto arbitrario debe rechazarse")
	}
	if _, err := uuidv8.Parse("zzzzzzzz-zzzz-8zzz-9zzz-zzzzzzzzzzzz"); err == nil {
		t.Fatal("el hex invalido debe rechazarse")
	}
	defer func() {
		if recover() == nil {
			t.Fatal("MustParse debe entrar en panico con un literal invalido")
		}
	}()
	_ = uuidv8.MustParse("invalido")
}

func TestUUIDv8NamespaceNexumEsEstable(t *testing.T) {
	const esperado = "e882a8d5-8a71-8fbf-ac40-81f9cac964c7"
	if uuidv8.NamespaceNexum.String() != esperado {
		t.Fatalf("el namespace de Nexum no debe cambiar: %s", uuidv8.NamespaceNexum)
	}
	if !uuidv8.EsV8(uuidv8.NamespaceNexum) {
		t.Fatalf("el namespace debe ser un UUID v8 valido: %s", uuidv8.NamespaceNexum)
	}
}
