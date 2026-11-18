package application_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	appProfile "github.com/nexumpr/Nexum-Backend/internal/application/profile"
)

// Pruebas de aplicación de E013 (`PATCH /api/v1/users/me`): CAS sobre la versión de
// `If-Match`, solicitud vacía sin escritura, validación del nombre en runes, reintento
// acotado y fallo cerrado. El almacén es un doble de prueba; la composición productiva usa
// DynamoDB real.

const (
	e013Usuario = "33333333-4444-4555-8666-777777777777"
	e013Otro    = "77777777-6666-4555-8444-333333333333"
)

var e013Ahora = time.Date(2026, 11, 18, 11, 15, 0, 0, time.UTC)

// e013Store dobla lectura y escritura. Cuenta CAS fallidos programados para probar el
// reintento interno y registra cada escritura para demostrar que un rechazo no escribe.
type e013Store struct {
	registros      map[string]*appProfile.Record
	falloLectura   error
	falloEscritura error
	casFallidos    int
	mutarAlFallar  func()
	lecturas       []string
	escrituras     []appProfile.UpdateInput
}

func (s *e013Store) LoadProfile(_ context.Context, userID string) (*appProfile.Record, error) {
	s.lecturas = append(s.lecturas, userID)
	if s.falloLectura != nil {
		return nil, s.falloLectura
	}
	return s.registros[userID], nil
}

func (s *e013Store) UpdateProfile(_ context.Context, input appProfile.UpdateInput) (*appProfile.Record, error) {
	s.escrituras = append(s.escrituras, input)
	if s.falloEscritura != nil {
		return nil, s.falloEscritura
	}
	if s.casFallidos > 0 {
		s.casFallidos--
		if s.mutarAlFallar != nil {
			s.mutarAlFallar()
		}
		return nil, appProfile.ErrProfileCASFailed
	}
	actual := s.registros[input.UserID]
	if actual == nil || e013Version(actual) != input.ExpectedVersion {
		return nil, appProfile.ErrProfileCASFailed
	}
	copia := *actual
	copia.DisplayName = input.DisplayName
	copia.Version = e013Version(actual) + 1
	copia.HasVersion = true
	s.registros[input.UserID] = &copia
	return &copia, nil
}

func e013Version(record *appProfile.Record) int64 {
	if record == nil || !record.HasVersion || record.Version < 1 {
		return 1
	}
	return record.Version
}

func e013Perfil(userID string, version int64) *appProfile.Record {
	return &appProfile.Record{
		UserID:        userID,
		Email:         "ejemplo@example.invalid",
		DisplayName:   "Usuario Ejemplo",
		EmailVerified: true,
		CreatedAt:     e013Ahora.Add(-72 * time.Hour),
		HasCreatedAt:  true,
		Version:       version,
		HasVersion:    true,
	}
}

func e013NuevoServicio(store *e013Store) *appProfile.Service {
	return appProfile.NewService(store).
		WithUpdateStore(store).
		WithClock(func() time.Time { return e013Ahora })
}

func e013Patch(nombre string) appProfile.Patch {
	return appProfile.Patch{DisplayName: &nombre}
}

// TestE013PatchVacioNoEscribe cubre `{}`: 200 con la versión vigente, sin escritura y sin
// incremento de versión.
func TestE013PatchVacioNoEscribe(t *testing.T) {
	store := &e013Store{registros: map[string]*appProfile.Record{e013Usuario: e013Perfil(e013Usuario, 7)}}
	servicio := e013NuevoServicio(store)

	perfil, err := servicio.Update(context.Background(), e013Usuario, 7, appProfile.Patch{})
	if err != nil {
		t.Fatalf("Update devolvio error: %v", err)
	}
	if perfil.Version != 7 || perfil.DisplayName != "Usuario Ejemplo" {
		t.Fatalf("el perfil vigente debe devolverse intacto: %+v", perfil)
	}
	if len(store.escrituras) != 0 {
		t.Fatalf("una solicitud vacia no debe escribir: %v", store.escrituras)
	}
	if store.registros[e013Usuario].Version != 7 {
		t.Fatalf("la version no debe incrementarse, es %d", store.registros[e013Usuario].Version)
	}
}

// TestE013PatchAplicaCAS cubre la escritura real: se envia la version leida y el adaptador
// recibe la version esperada, el nombre normalizado y el reloj de la operacion.
func TestE013PatchAplicaCAS(t *testing.T) {
	store := &e013Store{registros: map[string]*appProfile.Record{e013Usuario: e013Perfil(e013Usuario, 7)}}
	servicio := e013NuevoServicio(store)

	perfil, err := servicio.Update(context.Background(), "  "+e013Usuario+"  ", 7, e013Patch("  Nombre Nuevo  "))
	if err != nil {
		t.Fatalf("Update devolvio error: %v", err)
	}
	if perfil.DisplayName != "Nombre Nuevo" {
		t.Fatalf("el nombre debe persistirse sin espacios de los extremos: %q", perfil.DisplayName)
	}
	if perfil.Version != 8 {
		t.Fatalf("la version debe incrementarse a 8, es %d", perfil.Version)
	}
	if len(store.escrituras) != 1 {
		t.Fatalf("debe haber exactamente una escritura, hubo %d", len(store.escrituras))
	}
	entrada := store.escrituras[0]
	if entrada.UserID != e013Usuario || entrada.ExpectedVersion != 7 || entrada.DisplayName != "Nombre Nuevo" {
		t.Fatalf("escritura inesperada: %+v", entrada)
	}
	if !entrada.ProfileHasVersion {
		t.Fatal("el perfil tiene version: el CAS debe exigir igualdad")
	}
	if !entrada.Now.Equal(e013Ahora) {
		t.Fatalf("la marca de tiempo debe venir del reloj del servicio: %s", entrada.Now)
	}
}

// TestE013PatchValidaNombre cubre 422: vacio, solo espacios y mas de 100 runes. Ningun caso
// llega a escribir.
func TestE013PatchValidaNombre(t *testing.T) {
	casos := []struct {
		nombre string
		valor  string
	}{
		{"vacio", ""},
		{"solo espacios", "   "},
		{"101 runes", strings.Repeat("á", 101)},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			store := &e013Store{registros: map[string]*appProfile.Record{e013Usuario: e013Perfil(e013Usuario, 2)}}
			servicio := e013NuevoServicio(store)

			_, err := servicio.Update(context.Background(), e013Usuario, 2, e013Patch(caso.valor))
			if !errors.Is(err, appProfile.ErrProfileBusinessRule) {
				t.Fatalf("esperado 422 (ErrProfileBusinessRule), obtenido %v", err)
			}
			var violacion *appProfile.FieldViolation
			if !errors.As(err, &violacion) || violacion.Field != "displayName" {
				t.Fatalf("la violacion debe senalar displayName: %#v", err)
			}
			if len(store.escrituras) != 0 {
				t.Fatalf("un nombre invalido no debe escribir: %v", store.escrituras)
			}
		})
	}
}

// TestE013PatchLimiteDeRunes: 100 runes multibyte son validos aunque ocupen 200 bytes.
func TestE013PatchLimiteDeRunes(t *testing.T) {
	store := &e013Store{registros: map[string]*appProfile.Record{e013Usuario: e013Perfil(e013Usuario, 2)}}
	servicio := e013NuevoServicio(store)
	nombre := strings.Repeat("á", appProfile.MaxDisplayNameRunes)

	perfil, err := servicio.Update(context.Background(), e013Usuario, 2, e013Patch(nombre))
	if err != nil {
		t.Fatalf("100 runes deben aceptarse: %v", err)
	}
	if perfil.DisplayName != nombre {
		t.Fatalf("el nombre debe conservarse completo")
	}
	if len(store.escrituras) != 1 {
		t.Fatalf("debe escribirse una vez, hubo %d", len(store.escrituras))
	}
}

// TestE013PatchVersionDesactualizada cubre 412: la version de `If-Match` no es la vigente y
// no se escribe nada.
func TestE013PatchVersionDesactualizada(t *testing.T) {
	store := &e013Store{registros: map[string]*appProfile.Record{e013Usuario: e013Perfil(e013Usuario, 3)}}
	servicio := e013NuevoServicio(store)

	_, err := servicio.Update(context.Background(), e013Usuario, 2, e013Patch("Nombre Nuevo"))
	if !errors.Is(err, appProfile.ErrProfileVersionConflict) {
		t.Fatalf("esperado 412, obtenido %v", err)
	}
	if len(store.escrituras) != 0 {
		t.Fatalf("una version equivocada no debe escribir: %v", store.escrituras)
	}
}

// TestE013PatchPerfilInexistente cubre 404 sin crear el item.
func TestE013PatchPerfilInexistente(t *testing.T) {
	store := &e013Store{registros: map[string]*appProfile.Record{}}
	servicio := e013NuevoServicio(store)

	_, err := servicio.Update(context.Background(), e013Otro, 1, e013Patch("Nombre Nuevo"))
	if !errors.Is(err, appProfile.ErrProfileNotFound) {
		t.Fatalf("esperado 404, obtenido %v", err)
	}
	if len(store.escrituras) != 0 {
		t.Fatalf("un perfil inexistente no debe escribirse: %v", store.escrituras)
	}
}

// TestE013PatchSinIdentidad: sin sujeto no se lee ni se escribe.
func TestE013PatchSinIdentidad(t *testing.T) {
	store := &e013Store{registros: map[string]*appProfile.Record{e013Usuario: e013Perfil(e013Usuario, 1)}}
	servicio := e013NuevoServicio(store)

	for _, userID := range []string{"", "   "} {
		if _, err := servicio.Update(context.Background(), userID, 1, appProfile.Patch{}); !errors.Is(err, appProfile.ErrProfileUnauthorized) {
			t.Fatalf("userID %q debe producir 401, obtuvo %v", userID, err)
		}
	}
	if len(store.lecturas) != 0 || len(store.escrituras) != 0 {
		t.Fatalf("sin identidad no debe tocarse la persistencia: %v / %v", store.lecturas, store.escrituras)
	}
}

// TestE013PatchVersionInvalida: una version menor que 1 no es una precondicion valida.
func TestE013PatchVersionInvalida(t *testing.T) {
	store := &e013Store{registros: map[string]*appProfile.Record{e013Usuario: e013Perfil(e013Usuario, 1)}}
	servicio := e013NuevoServicio(store)

	for _, version := range []int64{0, -1} {
		if _, err := servicio.Update(context.Background(), e013Usuario, version, appProfile.Patch{}); !errors.Is(err, appProfile.ErrProfileInvalidRequest) {
			t.Fatalf("version %d debe ser una solicitud invalida, obtuvo %v", version, err)
		}
	}
	if len(store.lecturas) != 0 {
		t.Fatalf("no debe leerse con una version invalida: %v", store.lecturas)
	}
}

// TestE013PatchReintentaUnaVezConExito: un CAS fallido por carrera no es un fallo del
// cliente; si la version sigue siendo la esperada, el reintento interno unico lo resuelve.
func TestE013PatchReintentaUnaVezConExito(t *testing.T) {
	store := &e013Store{
		registros:   map[string]*appProfile.Record{e013Usuario: e013Perfil(e013Usuario, 4)},
		casFallidos: 1,
	}
	servicio := e013NuevoServicio(store)

	perfil, err := servicio.Update(context.Background(), e013Usuario, 4, e013Patch("Nombre Nuevo"))
	if err != nil {
		t.Fatalf("el reintento debe resolver la carrera: %v", err)
	}
	if perfil.Version != 5 {
		t.Fatalf("la version final debe ser 5, es %d", perfil.Version)
	}
	if len(store.escrituras) != 2 {
		t.Fatalf("debe intentarse dos veces (original + reintento), hubo %d", len(store.escrituras))
	}
}

// TestE013PatchReintentoAgotadoEs409: dos fallos CAS con la misma version agotan el
// reintento y responden 409, no un bucle indefinido.
func TestE013PatchReintentoAgotadoEs409(t *testing.T) {
	store := &e013Store{
		registros:   map[string]*appProfile.Record{e013Usuario: e013Perfil(e013Usuario, 4)},
		casFallidos: 5,
	}
	servicio := e013NuevoServicio(store)

	_, err := servicio.Update(context.Background(), e013Usuario, 4, e013Patch("Nombre Nuevo"))
	if !errors.Is(err, appProfile.ErrProfileConflict) {
		t.Fatalf("esperado 409, obtenido %v", err)
	}
	if len(store.escrituras) != 2 {
		t.Fatalf("no debe reintentarse mas de una vez, hubo %d escrituras", len(store.escrituras))
	}
}

// TestE013PatchCarreraQueCambiaLaVersion: si entre la lectura y la escritura otro cliente
// incrementa la version, el resultado es 412 y no se sobrescribe su cambio.
func TestE013PatchCarreraQueCambiaLaVersion(t *testing.T) {
	store := &e013Store{registros: map[string]*appProfile.Record{e013Usuario: e013Perfil(e013Usuario, 4)}}
	servicio := e013NuevoServicio(store)

	// El primer CAS falla y, antes de la relectura, el item avanza a v5 (otro cliente).
	store.casFallidos = 1
	store.mutarAlFallar = func() {
		avanzado := e013Perfil(e013Usuario, 5)
		avanzado.DisplayName = "Cambio De Otro Cliente"
		store.registros[e013Usuario] = avanzado
	}

	_, err := servicio.Update(context.Background(), e013Usuario, 4, e013Patch("Nombre Nuevo"))
	if !errors.Is(err, appProfile.ErrProfileVersionConflict) {
		t.Fatalf("esperado 412, obtenido %v", err)
	}
	if store.registros[e013Usuario].DisplayName != "Cambio De Otro Cliente" {
		t.Fatal("no debe sobrescribirse el cambio de otro cliente")
	}
	if len(store.escrituras) != 1 {
		t.Fatalf("sin la version esperada no debe reintentarse: %d escrituras", len(store.escrituras))
	}
}

// TestE013PatchPerfilHeredadoSinVersion: E012 expone v1 para un item sin `version`; el CAS
// debe reconocer esa forma y pedir al adaptador que inicialice el contador.
func TestE013PatchPerfilHeredadoSinVersion(t *testing.T) {
	heredado := e013Perfil(e013Usuario, 1)
	heredado.Version = 0
	heredado.HasVersion = false
	store := &e013Store{registros: map[string]*appProfile.Record{e013Usuario: heredado}}
	servicio := e013NuevoServicio(store)

	perfil, err := servicio.Update(context.Background(), e013Usuario, 1, e013Patch("Nombre Nuevo"))
	if err != nil {
		t.Fatalf("un item heredado debe poder actualizarse con If-Match v1: %v", err)
	}
	if perfil.Version != 2 || perfil.DisplayName != "Nombre Nuevo" {
		t.Fatalf("proyeccion inesperada: %+v", perfil)
	}
	if store.escrituras[0].ProfileHasVersion {
		t.Fatal("el adaptador debe emitir la condicion attribute_not_exists(version)")
	}
}

// TestE013PatchFallaCerrado: sin puerto de escritura, sin almacén, con lectura o escritura
// caídas, el resultado es 503 (nunca una escritura simulada).
func TestE013PatchFallaCerrado(t *testing.T) {
	if _, err := appProfile.NewService(nil).Update(context.Background(), e013Usuario, 1, e013Patch("Nombre")); !errors.Is(err, appProfile.ErrProfileUnavailable) {
		t.Fatalf("sin almacen debe ser 503, obtuvo %v", err)
	}

	soloLectura := &e013Store{registros: map[string]*appProfile.Record{e013Usuario: e013Perfil(e013Usuario, 1)}}
	if _, err := appProfile.NewService(soloLectura).Update(context.Background(), e013Usuario, 1, e013Patch("Nombre")); !errors.Is(err, appProfile.ErrProfileUnavailable) {
		t.Fatalf("sin puerto de escritura debe ser 503, obtuvo %v", err)
	}

	caidaLectura := &e013Store{falloLectura: errors.New("dynamodb no disponible")}
	if _, err := e013NuevoServicio(caidaLectura).Update(context.Background(), e013Usuario, 1, e013Patch("Nombre")); !errors.Is(err, appProfile.ErrProfileUnavailable) {
		t.Fatalf("una lectura caida debe ser 503, obtuvo %v", err)
	}

	caidaEscritura := &e013Store{
		registros:      map[string]*appProfile.Record{e013Usuario: e013Perfil(e013Usuario, 1)},
		falloEscritura: errors.New("dynamodb no disponible"),
	}
	if _, err := e013NuevoServicio(caidaEscritura).Update(context.Background(), e013Usuario, 1, e013Patch("Nombre")); !errors.Is(err, appProfile.ErrProfileUnavailable) {
		t.Fatalf("una escritura caida debe ser 503, obtuvo %v", err)
	}
}

// TestE013PatchIncoherenciaEs500: el adaptador no puede devolver un item sin atributos y la
// aplicación no inventa una respuesta.
func TestE013PatchIncoherenciaEs500(t *testing.T) {
	store := &e013Store{
		registros:      map[string]*appProfile.Record{e013Usuario: e013Perfil(e013Usuario, 1)},
		falloEscritura: appProfile.ErrProfileInternal,
	}
	if _, err := e013NuevoServicio(store).Update(context.Background(), e013Usuario, 1, e013Patch("Nombre")); !errors.Is(err, appProfile.ErrProfileInternal) {
		t.Fatalf("la incoherencia debe ser 500, obtuvo %v", err)
	}
}

// TestE013PatchSinDatosSensiblesEnErrores: los errores no repiten correo, nombre ni id.
func TestE013PatchSinDatosSensiblesEnErrores(t *testing.T) {
	store := &e013Store{registros: map[string]*appProfile.Record{}}
	_, err := e013NuevoServicio(store).Update(context.Background(), e013Usuario, 9, e013Patch("Nombre Privado"))
	if err == nil {
		t.Fatal("se esperaba error")
	}
	for _, prohibido := range []string{"ejemplo@example.invalid", "Nombre Privado", e013Usuario} {
		if strings.Contains(err.Error(), prohibido) {
			t.Fatalf("el error no debe contener %q: %v", prohibido, err)
		}
	}
}

// TestE013PatchVacioInformaEmpty documenta la distinción entre campo ausente y vacío.
func TestE013PatchVacioInformaEmpty(t *testing.T) {
	if !(appProfile.Patch{}).Empty() {
		t.Fatal("un Patch sin campos debe informar Empty")
	}
	vacio := ""
	if (appProfile.Patch{DisplayName: &vacio}).Empty() {
		t.Fatal("displayName vacio no es un patch vacio: es una violacion de dominio")
	}
}
