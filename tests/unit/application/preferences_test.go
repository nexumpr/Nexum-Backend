package application_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	appPreferences "github.com/nexumpr/Nexum-Backend/internal/application/preferences"
)

// Pruebas de aplicación de E068 (`GET /api/v1/users/me/preferences`): lectura del titular,
// materialización idempotente de los valores por defecto, aislamiento entre usuarios y fallo
// cerrado ante persistencia ausente o caída. El almacén es un doble de prueba; la composición
// productiva usa DynamoDB real.

const (
	e068Usuario = "11111111-2222-4333-8444-555555555555"
	e068Otro    = "99999999-8888-4777-8666-555555555555"
)

var e068Ahora = time.Date(2026, 11, 18, 9, 30, 0, 0, time.UTC)

// e068Store es un doble con estado: emula el `PutItem` condicional del adaptador real, de modo
// que la creación perezosa concurrente se comporta igual que en DynamoDB.
type e068Store struct {
	mu            sync.Mutex
	items         map[string]appPreferences.Preferences
	lecturas      int
	creaciones    int
	falloCarga    error
	falloCreacion error
	creadoEn      []time.Time
}

func newE068Store() *e068Store {
	return &e068Store{items: map[string]appPreferences.Preferences{}}
}

func (s *e068Store) LoadPreferences(_ context.Context, userID string) (*appPreferences.Preferences, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lecturas++
	if s.falloCarga != nil {
		return nil, s.falloCarga
	}
	item, ok := s.items[userID]
	if !ok {
		return nil, nil
	}
	copia := item
	return &copia, nil
}

func (s *e068Store) CreatePreferences(_ context.Context, userID string, defaults appPreferences.Preferences, now time.Time) (*appPreferences.Preferences, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.creaciones++
	s.creadoEn = append(s.creadoEn, now)
	if s.falloCreacion != nil {
		return nil, s.falloCreacion
	}
	if existente, ok := s.items[userID]; ok {
		// La condición `attribute_not_exists(PK)` falló: el ganador es la autoridad.
		copia := existente
		return &copia, nil
	}
	s.items[userID] = defaults
	copia := defaults
	return &copia, nil
}

// TestE068DevuelvePreferenciasExistentes: si el item existe no se crea nada y se respeta el
// valor almacenado, incluida la versión que luego exige E069.
func TestE068DevuelvePreferenciasExistentes(t *testing.T) {
	store := newE068Store()
	store.items[e068Usuario] = appPreferences.Preferences{
		Locale:     "en-US",
		Timezone:   "Europe/Madrid",
		Version:    7,
		HasVersion: true,
	}

	prefs, err := appPreferences.NewService(store).Get(context.Background(), e068Usuario)
	if err != nil {
		t.Fatalf("lectura valida devolvio error: %v", err)
	}
	if prefs.Locale != "en-US" || prefs.Timezone != "Europe/Madrid" || prefs.Version != 7 {
		t.Fatalf("debe devolverse el valor almacenado: %+v", prefs)
	}
	if store.creaciones != 0 {
		t.Fatalf("no debe materializarse nada si el item existe, creaciones=%d", store.creaciones)
	}
}

// TestE068MaterializaDefaults cubre el caso central de E068: item ausente -> valores del
// contrato persistidos, no inventados en la respuesta.
func TestE068MaterializaDefaults(t *testing.T) {
	store := newE068Store()
	servicio := appPreferences.NewService(store).WithClock(func() time.Time { return e068Ahora })

	prefs, err := servicio.Get(context.Background(), e068Usuario)
	if err != nil {
		t.Fatalf("materializacion fallo: %v", err)
	}
	if prefs.Locale != "es-MX" || prefs.Timezone != "America/Mexico_City" || prefs.Version != 1 {
		t.Fatalf("defaults inesperados: %+v", prefs)
	}
	if !prefs.HasVersion {
		t.Fatal("el item creado debe tener version")
	}
	if store.creaciones != 1 {
		t.Fatalf("debe crearse exactamente una vez, creaciones=%d", store.creaciones)
	}
	if len(store.creadoEn) != 1 || !store.creadoEn[0].Equal(e068Ahora) {
		t.Fatalf("la marca de creacion debe ser la del reloj inyectado: %v", store.creadoEn)
	}
	if _, ok := store.items[e068Usuario]; !ok {
		t.Fatal("los defaults deben quedar persistidos")
	}
}

// TestE068CreacionIdempotenteConcurrente: 20 lecturas simultaneas de un item ausente convergen
// en una sola versión observable; ninguna falla ni sobrescribe a otra.
func TestE068CreacionIdempotenteConcurrente(t *testing.T) {
	store := newE068Store()
	servicio := appPreferences.NewService(store).WithClock(func() time.Time { return e068Ahora })

	const lectores = 20
	var wg sync.WaitGroup
	resultados := make([]*appPreferences.Preferences, lectores)
	errores := make([]error, lectores)
	for i := 0; i < lectores; i++ {
		wg.Add(1)
		go func(indice int) {
			defer wg.Done()
			resultados[indice], errores[indice] = servicio.Get(context.Background(), e068Usuario)
		}(i)
	}
	wg.Wait()

	for i := 0; i < lectores; i++ {
		if errores[i] != nil {
			t.Fatalf("lector %d fallo: %v", i, errores[i])
		}
		if resultados[i].Version != 1 || resultados[i].Locale != "es-MX" {
			t.Fatalf("lector %d obtuvo un estado divergente: %+v", i, resultados[i])
		}
	}
	if len(store.items) != 1 {
		t.Fatalf("solo debe existir un item por usuario, hay %d", len(store.items))
	}
}

// TestE068AislaUsuarios: la preferencia de A no se devuelve a B aunque el almacén esté
// compartido. La identidad proviene del token, no de la petición.
func TestE068AislaUsuarios(t *testing.T) {
	store := newE068Store()
	store.items[e068Otro] = appPreferences.Preferences{
		Locale:     "en-US",
		Timezone:   "America/New_York",
		Version:    4,
		HasVersion: true,
	}
	servicio := appPreferences.NewService(store).WithClock(func() time.Time { return e068Ahora })

	prefs, err := servicio.Get(context.Background(), e068Usuario)
	if err != nil {
		t.Fatalf("lectura fallo: %v", err)
	}
	if prefs.Locale != "es-MX" || prefs.Timezone != "America/Mexico_City" {
		t.Fatalf("el titular A no puede recibir preferencias de B: %+v", prefs)
	}
	if store.items[e068Otro].Locale != "en-US" {
		t.Fatal("la materializacion de A no puede modificar el item de B")
	}
}

// TestE068FallaCerradoSinAlmacen: sin persistencia no se sirven defaults efímeros.
func TestE068FallaCerradoSinAlmacen(t *testing.T) {
	_, err := appPreferences.NewService(nil).Get(context.Background(), e068Usuario)
	if !errors.Is(err, appPreferences.ErrPreferencesUnavailable) {
		t.Fatalf("sin almacen debe fallar cerrado, obtenido %v", err)
	}
	if appPreferences.NewService(nil).Disponible() {
		t.Fatal("un servicio sin almacen no esta disponible")
	}
}

// TestE068FalloDePersistenciaNoInventaValores: ni la lectura ni la creación degradan a
// valores en memoria cuando DynamoDB falla.
func TestE068FalloDePersistenciaNoInventaValores(t *testing.T) {
	store := newE068Store()
	store.falloCarga = errors.New("dynamo no disponible")
	if _, err := appPreferences.NewService(store).Get(context.Background(), e068Usuario); !errors.Is(err, appPreferences.ErrPreferencesUnavailable) {
		t.Fatalf("fallo de lectura debe ser 503 de aplicacion, obtenido %v", err)
	}

	store = newE068Store()
	store.falloCreacion = errors.New("dynamo no disponible")
	if _, err := appPreferences.NewService(store).Get(context.Background(), e068Usuario); !errors.Is(err, appPreferences.ErrPreferencesUnavailable) {
		t.Fatalf("fallo de creacion debe ser 503 de aplicacion, obtenido %v", err)
	}
	if len(store.items) != 0 {
		t.Fatal("un fallo de creacion no puede dejar un item")
	}
}

// TestE068ConflictoDeCreacionSePropaga: si el adaptador detecta que el item quedó en un estado
// no observable, se responde conflicto y nunca se inventan preferencias.
func TestE068ConflictoDeCreacionSePropaga(t *testing.T) {
	store := newE068Store()
	store.falloCreacion = appPreferences.ErrPreferencesConflict
	if _, err := appPreferences.NewService(store).Get(context.Background(), e068Usuario); !errors.Is(err, appPreferences.ErrPreferencesConflict) {
		t.Fatalf("el conflicto debe propagarse, obtenido %v", err)
	}
}

// TestE068TitularVacioEsNoAutorizado: sin `sub` no se consulta ni se crea nada.
func TestE068TitularVacioEsNoAutorizado(t *testing.T) {
	store := newE068Store()
	if _, err := appPreferences.NewService(store).Get(context.Background(), "   "); !errors.Is(err, appPreferences.ErrPreferencesUnauthorized) {
		t.Fatalf("titular vacio debe ser no autorizado, obtenido %v", err)
	}
	if store.lecturas != 0 || store.creaciones != 0 {
		t.Fatal("sin titular no debe tocarse la persistencia")
	}
}

// TestE068NormalizaItemIncoherente: un item historico sin `version` o sin valores no produce
// una respuesta invalida, y la lectura no escribe nada.
func TestE068NormalizaItemIncoherente(t *testing.T) {
	store := newE068Store()
	store.items[e068Usuario] = appPreferences.Preferences{}

	prefs, err := appPreferences.NewService(store).Get(context.Background(), e068Usuario)
	if err != nil {
		t.Fatalf("lectura fallo: %v", err)
	}
	if prefs.Locale != "es-MX" || prefs.Timezone != "America/Mexico_City" || prefs.Version != 1 {
		t.Fatalf("el item incoherente debe normalizarse a los valores del contrato: %+v", prefs)
	}
	if prefs.HasVersion {
		t.Fatal("un item sin version no debe declarar CAS previo")
	}
	if store.creaciones != 0 {
		t.Fatal("normalizar la lectura no debe escribir")
	}
}

// ---------- E069: modificacion de preferencias ----------

// e069Store extiende el doble de E068 con la escritura condicional por version. La comprobacion
// del CAS se hace bajo el mismo mutex que la lectura, como en el adaptador real.
type e069Store struct {
	*e068Store
	actualizaciones int
	falloUpdate     error
	ultimaUpdate    appPreferences.Update
}

func newE069Store() *e069Store {
	return &e069Store{e068Store: newE068Store()}
}

func (s *e069Store) UpdatePreferences(_ context.Context, userID string, update appPreferences.Update) (*appPreferences.Preferences, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.actualizaciones++
	s.ultimaUpdate = update
	if s.falloUpdate != nil {
		return nil, s.falloUpdate
	}
	item, ok := s.items[userID]
	if !ok {
		return nil, appPreferences.ErrPreferencesConflict
	}
	observada := item.Version
	if !item.HasVersion {
		observada = appPreferences.DefaultVersion
	}
	if observada != update.ExpectedVersion {
		return nil, appPreferences.ErrPreferencesVersionConflict
	}
	item.Locale = update.Locale
	item.Timezone = update.Timezone
	item.Version = observada + 1
	item.HasVersion = true
	s.items[userID] = item
	copia := item
	return &copia, nil
}

func servicioE069(store appPreferences.Store) *appPreferences.Service {
	return appPreferences.NewService(store).WithClock(func() time.Time { return e068Ahora })
}

// TestE069AplicaCambioParcialConCAS: un PATCH de locale conserva la zona y devuelve la version
// incrementada, con la version esperada recibida de If-Match.
func TestE069AplicaCambioParcialConCAS(t *testing.T) {
	store := newE069Store()
	store.items[e068Usuario] = appPreferences.Preferences{
		Locale:     "es-MX",
		Timezone:   "America/Mexico_City",
		Version:    3,
		HasVersion: true,
	}

	prefs, err := servicioE069(store).Apply(context.Background(), appPreferences.ApplyCommand{
		UserID:          e068Usuario,
		Locale:          "en-US",
		HasLocale:       true,
		ExpectedVersion: 3,
	})
	if err != nil {
		t.Fatalf("PATCH valido fallo: %v", err)
	}
	if prefs.Locale != "en-US" || prefs.Timezone != "America/Mexico_City" || prefs.Version != 4 {
		t.Fatalf("resultado inesperado: %+v", prefs)
	}
	if store.actualizaciones != 1 {
		t.Fatalf("debe escribirse una vez, hubo %d", store.actualizaciones)
	}
	if store.ultimaUpdate.ExpectedVersion != 3 || store.ultimaUpdate.Locale != "en-US" || store.ultimaUpdate.Timezone != "America/Mexico_City" {
		t.Fatalf("la escritura debe llevar el estado resultante completo: %+v", store.ultimaUpdate)
	}
	if !store.ultimaUpdate.Now.Equal(e068Ahora) {
		t.Fatalf("la marca debe venir del reloj inyectado: %v", store.ultimaUpdate.Now)
	}
}

// TestE069AceptaSoloZona: un PATCH de zona conserva el idioma vigente.
func TestE069AceptaSoloZona(t *testing.T) {
	store := newE069Store()
	store.items[e068Usuario] = appPreferences.Preferences{
		Locale:     "en-US",
		Timezone:   "America/New_York",
		Version:    2,
		HasVersion: true,
	}

	prefs, err := servicioE069(store).Apply(context.Background(), appPreferences.ApplyCommand{
		UserID:          e068Usuario,
		Timezone:        "Europe/Madrid",
		HasTimezone:     true,
		ExpectedVersion: 2,
	})
	if err != nil {
		t.Fatalf("PATCH de zona fallo: %v", err)
	}
	if prefs.Locale != "en-US" || prefs.Timezone != "Europe/Madrid" || prefs.Version != 3 {
		t.Fatalf("resultado inesperado: %+v", prefs)
	}
}

// TestE069PatchVacioNoEscribe: sin campos no hay escritura ni cambio de version (200
// idempotente), tal como declara el ejemplo `{}` del contrato.
func TestE069PatchVacioNoEscribe(t *testing.T) {
	store := newE069Store()
	store.items[e068Usuario] = appPreferences.Preferences{
		Locale:     "es-MX",
		Timezone:   "America/Mexico_City",
		Version:    2,
		HasVersion: true,
	}

	prefs, err := servicioE069(store).Apply(context.Background(), appPreferences.ApplyCommand{
		UserID:          e068Usuario,
		ExpectedVersion: 2,
	})
	if err != nil {
		t.Fatalf("PATCH vacio fallo: %v", err)
	}
	if prefs.Version != 2 || prefs.Locale != "es-MX" || prefs.Timezone != "America/Mexico_City" {
		t.Fatalf("un PATCH vacio debe devolver el estado vigente: %+v", prefs)
	}
	if store.actualizaciones != 0 {
		t.Fatalf("un PATCH vacio no debe escribir, escrituras=%d", store.actualizaciones)
	}
}

// TestE069MaterializaDefaultsAntesDeComparar: una cuenta sin item admite `If-Match: "v1"`; los
// defaults se persisten antes de aplicar el cambio y la version avanza a 2.
func TestE069MaterializaDefaultsAntesDeComparar(t *testing.T) {
	store := newE069Store()

	prefs, err := servicioE069(store).Apply(context.Background(), appPreferences.ApplyCommand{
		UserID:          e068Usuario,
		Locale:          "en-US",
		HasLocale:       true,
		ExpectedVersion: appPreferences.DefaultVersion,
	})
	if err != nil {
		t.Fatalf("materializacion con CAS fallo: %v", err)
	}
	if prefs.Locale != "en-US" || prefs.Timezone != "America/Mexico_City" || prefs.Version != 2 {
		t.Fatalf("resultado inesperado: %+v", prefs)
	}
	if store.creaciones != 1 {
		t.Fatalf("debe materializarse una vez, creaciones=%d", store.creaciones)
	}
	if store.actualizaciones != 1 || store.ultimaUpdate.ExpectedVersion != appPreferences.DefaultVersion {
		t.Fatalf("el CAS debe comparar contra la version materializada: %+v", store.ultimaUpdate)
	}
	if !store.ultimaUpdate.Now.Equal(e068Ahora) {
		t.Fatalf("la escritura debe usar el reloj inyectado: %v", store.ultimaUpdate.Now)
	}
}

// TestE069MaterializaYRechazaVersionDistinta: aunque el item se materialice, una version que no
// es la inicial se rechaza con 412 y no se aplica ningun cambio.
func TestE069MaterializaYRechazaVersionDistinta(t *testing.T) {
	store := newE069Store()

	_, err := servicioE069(store).Apply(context.Background(), appPreferences.ApplyCommand{
		UserID:          e068Usuario,
		Locale:          "en-US",
		HasLocale:       true,
		ExpectedVersion: 7,
	})
	if !errors.Is(err, appPreferences.ErrPreferencesVersionConflict) {
		t.Fatalf("version distinta debe ser conflicto, obtenido %v", err)
	}
	if store.actualizaciones != 0 {
		t.Fatal("un 412 no puede escribir")
	}
	if store.items[e068Usuario].Version != appPreferences.DefaultVersion {
		t.Fatalf("el item materializado debe conservar la version inicial: %+v", store.items[e068Usuario])
	}
}

// TestE069VersionDesactualizadaSinEscritura: una version observada distinta de If-Match no
// llega al almacen.
func TestE069VersionDesactualizadaSinEscritura(t *testing.T) {
	store := newE069Store()
	store.items[e068Usuario] = appPreferences.Preferences{
		Locale:     "es-MX",
		Timezone:   "America/Mexico_City",
		Version:    5,
		HasVersion: true,
	}

	_, err := servicioE069(store).Apply(context.Background(), appPreferences.ApplyCommand{
		UserID:          e068Usuario,
		Timezone:        "Europe/Madrid",
		HasTimezone:     true,
		ExpectedVersion: 4,
	})
	if !errors.Is(err, appPreferences.ErrPreferencesVersionConflict) {
		t.Fatalf("esperado conflicto de version, obtenido %v", err)
	}
	if store.actualizaciones != 0 {
		t.Fatal("no debe intentarse la escritura con una version desactualizada")
	}
}

// TestE069ConflictoDeReintentoSePropaga: cuando el adaptador agota el CAS, el 409 llega intacto.
func TestE069ConflictoDeReintentoSePropaga(t *testing.T) {
	store := newE069Store()
	store.items[e068Usuario] = appPreferences.Preferences{
		Locale:     "es-MX",
		Timezone:   "America/Mexico_City",
		Version:    1,
		HasVersion: true,
	}
	store.falloUpdate = appPreferences.ErrPreferencesConflict

	_, err := servicioE069(store).Apply(context.Background(), appPreferences.ApplyCommand{
		UserID:          e068Usuario,
		Locale:          "en-US",
		HasLocale:       true,
		ExpectedVersion: 1,
	})
	if !errors.Is(err, appPreferences.ErrPreferencesConflict) {
		t.Fatalf("el conflicto debe propagarse, obtenido %v", err)
	}
}

// TestE069SinUpdateStoreFallaCerrado: con un almacen de solo lectura no se finge una escritura.
func TestE069SinUpdateStoreFallaCerrado(t *testing.T) {
	store := newE068Store()
	store.items[e068Usuario] = appPreferences.Preferences{
		Locale:     "es-MX",
		Timezone:   "America/Mexico_City",
		Version:    1,
		HasVersion: true,
	}

	_, err := servicioE069(store).Apply(context.Background(), appPreferences.ApplyCommand{
		UserID:          e068Usuario,
		Locale:          "en-US",
		HasLocale:       true,
		ExpectedVersion: 1,
	})
	if !errors.Is(err, appPreferences.ErrPreferencesUnavailable) {
		t.Fatalf("sin escritura condicional debe fallar cerrado, obtenido %v", err)
	}
}

// TestE069SinAlmacenYTitularVacio: sin persistencia es 503 y sin `sub` es 401; ninguno toca el
// almacen.
func TestE069SinAlmacenYTitularVacio(t *testing.T) {
	cmd := appPreferences.ApplyCommand{UserID: e068Usuario, Locale: "en-US", HasLocale: true, ExpectedVersion: 1}
	if _, err := appPreferences.NewService(nil).Apply(context.Background(), cmd); !errors.Is(err, appPreferences.ErrPreferencesUnavailable) {
		t.Fatalf("sin almacen debe fallar cerrado, obtenido %v", err)
	}

	store := newE069Store()
	cmd.UserID = "   "
	if _, err := servicioE069(store).Apply(context.Background(), cmd); !errors.Is(err, appPreferences.ErrPreferencesUnauthorized) {
		t.Fatalf("titular vacio debe ser no autorizado, obtenido %v", err)
	}
	if store.lecturas != 0 || store.actualizaciones != 0 {
		t.Fatal("sin titular no debe tocarse la persistencia")
	}

	cmd.UserID = e068Usuario
	cmd.ExpectedVersion = 0
	if _, err := servicioE069(store).Apply(context.Background(), cmd); !errors.Is(err, appPreferences.ErrPreferencesInvalidRequest) {
		t.Fatalf("version no valida debe ser solicitud invalida, obtenido %v", err)
	}
}

// TestE069ConcurrenciaSoloUnCASGana: escrituras simultaneas con la misma version esperada
// producen un solo exito y el resto 412; la version final es unica.
func TestE069ConcurrenciaSoloUnCASGana(t *testing.T) {
	store := newE069Store()
	store.items[e068Usuario] = appPreferences.Preferences{
		Locale:     "es-MX",
		Timezone:   "America/Mexico_City",
		Version:    1,
		HasVersion: true,
	}
	servicio := servicioE069(store)

	const escritores = 10
	var wg sync.WaitGroup
	errores := make([]error, escritores)
	for i := 0; i < escritores; i++ {
		wg.Add(1)
		go func(indice int) {
			defer wg.Done()
			_, errores[indice] = servicio.Apply(context.Background(), appPreferences.ApplyCommand{
				UserID:          e068Usuario,
				Locale:          "en-US",
				HasLocale:       true,
				ExpectedVersion: 1,
			})
		}(i)
	}
	wg.Wait()

	exitos := 0
	for i := 0; i < escritores; i++ {
		switch {
		case errores[i] == nil:
			exitos++
		case errors.Is(errores[i], appPreferences.ErrPreferencesVersionConflict):
		default:
			t.Fatalf("escritor %d: error inesperado %v", i, errores[i])
		}
	}
	if exitos != 1 {
		t.Fatalf("solo un CAS puede ganar, ganaron %d", exitos)
	}
	if store.items[e068Usuario].Version != 2 {
		t.Fatalf("la version final debe ser 2: %+v", store.items[e068Usuario])
	}
}

// TestE069PatchVacioMaterializa: un PATCH vacio sobre una cuenta sin item persiste los defaults
// (version 1) y los devuelve, sin una segunda escritura.
func TestE069PatchVacioMaterializa(t *testing.T) {
	store := newE069Store()

	prefs, err := servicioE069(store).Apply(context.Background(), appPreferences.ApplyCommand{
		UserID:          e068Usuario,
		ExpectedVersion: appPreferences.DefaultVersion,
	})
	if err != nil {
		t.Fatalf("PATCH vacio sobre item ausente fallo: %v", err)
	}
	if prefs.Locale != appPreferences.DefaultLocale || prefs.Timezone != appPreferences.DefaultTimezone || prefs.Version != appPreferences.DefaultVersion {
		t.Fatalf("deben devolverse los defaults materializados: %+v", prefs)
	}
	if store.creaciones != 1 {
		t.Fatalf("debe materializarse una vez, creaciones=%d", store.creaciones)
	}
	if store.actualizaciones != 0 {
		t.Fatalf("un PATCH vacio no debe actualizar, actualizaciones=%d", store.actualizaciones)
	}
}
