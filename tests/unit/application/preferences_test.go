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
