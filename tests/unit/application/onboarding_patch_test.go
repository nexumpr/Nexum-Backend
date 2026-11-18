package application_test

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	appOnboarding "github.com/nexumpr/Nexum-Backend/internal/application/onboarding"
)

// Pruebas de aplicación de E019 (`PATCH /api/v1/users/me/onboarding`):
// normalización del catálogo cerrado de pasos, booleanos opcionales con punteros,
// CAS con un único reintento, creación con versión 1, recálculo de `done` y
// `completedAt`, y fail-closed sin puerto de escritura. Los dobles son
// exclusivamente de prueba; la composición productiva usa DynamoDB real.

var e019Ahora = time.Date(2026, 11, 18, 15, 30, 0, 0, time.UTC)

// e019Almacen es `Store` + `Writer` en memoria con CAS real por versión. El gancho
// `antesDeEscribir` permite simular una escritura concurrente entre la lectura y
// el CAS; se ejecuta con el mutex tomado y sólo debe tocar campos internos.
type e019Almacen struct {
	mu              sync.Mutex
	registros       map[string]*appOnboarding.Record
	falloLectura    error
	falloEscritura  error
	antesDeEscribir func(a *e019Almacen, userID string)

	lecturas   int
	intentos   int
	escrituras int
	creaciones int
	conflictos int
}

func newE019Almacen() *e019Almacen {
	return &e019Almacen{registros: map[string]*appOnboarding.Record{}}
}

func copiarRegistroE019(registro *appOnboarding.Record) *appOnboarding.Record {
	if registro == nil {
		return nil
	}
	copia := *registro
	copia.CompletedSteps = append([]string{}, registro.CompletedSteps...)
	if registro.CompletedAt != nil {
		marca := *registro.CompletedAt
		copia.CompletedAt = &marca
	}
	return &copia
}

// registroDesdeWriteE019 simula la fila resultante de una escritura confirmada.
func registroDesdeWriteE019(write appOnboarding.Write, version int64) *appOnboarding.Record {
	registro := &appOnboarding.Record{
		CompletedSteps:   append([]string{}, write.CompletedSteps...),
		Version:          version,
		HasVersion:       true,
		MicrophoneRaw:    write.MicrophoneRaw,
		NotificationsRaw: write.NotificationsRaw,
		Configured:       write.Configured,
	}
	if write.CompletedAt != nil && !write.CompletedAt.IsZero() {
		marca := write.CompletedAt.UTC()
		registro.CompletedAt = &marca
	}
	return registro
}

func (a *e019Almacen) GetOnboarding(_ context.Context, userID string) (*appOnboarding.Record, bool, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.lecturas++
	if a.falloLectura != nil {
		return nil, false, a.falloLectura
	}
	registro, ok := a.registros[userID]
	if !ok {
		return nil, false, nil
	}
	return copiarRegistroE019(registro), true, nil
}

func (a *e019Almacen) CreateOnboarding(_ context.Context, userID string, write appOnboarding.Write) (*appOnboarding.Record, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.intentos++
	if a.antesDeEscribir != nil {
		a.antesDeEscribir(a, userID)
	}
	if a.falloEscritura != nil {
		return nil, a.falloEscritura
	}
	if _, existe := a.registros[userID]; existe {
		a.conflictos++
		return nil, appOnboarding.ErrConflict
	}
	registro := registroDesdeWriteE019(write, 1)
	a.registros[userID] = registro
	a.creaciones++
	return copiarRegistroE019(registro), nil
}

func (a *e019Almacen) UpdateOnboarding(_ context.Context, userID string, write appOnboarding.Write) (*appOnboarding.Record, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.intentos++
	if a.antesDeEscribir != nil {
		a.antesDeEscribir(a, userID)
	}
	if a.falloEscritura != nil {
		return nil, a.falloEscritura
	}
	actual, existe := a.registros[userID]
	if !existe {
		a.conflictos++
		return nil, appOnboarding.ErrConflict
	}
	if actual.HasVersion != write.HasVersion || (write.HasVersion && actual.Version != write.ExpectedVersion) {
		a.conflictos++
		return nil, appOnboarding.ErrConflict
	}
	version := int64(1)
	if actual.HasVersion {
		version = actual.Version + 1
	}
	registro := registroDesdeWriteE019(write, version)
	a.registros[userID] = registro
	a.escrituras++
	return copiarRegistroE019(registro), nil
}

func e019Sembrar(a *e019Almacen, userID string, registro appOnboarding.Record) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.registros[userID] = copiarRegistroE019(&registro)
}

func e019Servicio(almacen appOnboarding.Store) *appOnboarding.Service {
	return appOnboarding.NewService(almacen).WithClock(func() time.Time { return e019Ahora })
}

func e019Puntero(valor bool) *bool { return &valor }

// TestE019NormalizaPasos cubre el catálogo cerrado: sin duplicados, en orden
// canónico y sin destruir el conjunto vacío.
func TestE019NormalizaPasos(t *testing.T) {
	casos := []struct {
		nombre   string
		entrada  []string
		esperado []string
	}{
		{"vacio", []string{}, []string{}},
		{"nil", nil, []string{}},
		{"desordenado y duplicado", []string{"permissions", "profile", "permissions", "income"}, []string{"profile", "income", "permissions"}},
		{"mayusculas y espacios", []string{" Privacy ", "INCOME"}, []string{"income", "privacy"}},
		{"completo", []string{"permissions", "privacy", "income", "profile"}, []string{"profile", "income", "privacy", "permissions"}},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			obtenido, err := appOnboarding.NormalizeSubmittedSteps(caso.entrada)
			if err != nil {
				t.Fatalf("entrada valida no debe fallar: %v", err)
			}
			if obtenido == nil {
				t.Fatalf("la normalizacion nunca debe devolver nil")
			}
			if !reflect.DeepEqual(obtenido, caso.esperado) {
				t.Fatalf("esperado %v, obtenido %v", caso.esperado, obtenido)
			}
		})
	}
}

// TestE019PasoDesconocido garantiza 422: cualquier valor fuera del catálogo,
// incluido el string vacío, se rechaza antes de escribir.
func TestE019PasoDesconocido(t *testing.T) {
	for _, valor := range []string{"otro", "", "  ", "perfil", "permissions#1"} {
		_, err := appOnboarding.NormalizeSubmittedSteps([]string{valor})
		var desconocido *appOnboarding.UnknownStepError
		if !errors.As(err, &desconocido) {
			t.Fatalf("el paso %q debe rechazarse como desconocido, obtenido %v", valor, err)
		}
	}

	almacen := newE019Almacen()
	e019Sembrar(almacen, e018UsuarioA, appOnboarding.Record{
		CompletedSteps: []string{"profile"},
		Version:        2,
		HasVersion:     true,
	})
	servicio := e019Servicio(almacen)
	if _, err := servicio.Patch(context.Background(), e018UsuarioA, appOnboarding.PatchCommand{
		CompletedSteps: []string{"profile", "otro"},
	}); err == nil {
		t.Fatalf("un paso desconocido no puede producir escritura")
	}
	if almacen.intentos != 0 || almacen.escrituras != 0 {
		t.Fatalf("no debe intentarse ninguna escritura, intentos=%d escrituras=%d", almacen.intentos, almacen.escrituras)
	}
}

// TestE019PatchCreaConVersionUno: sin ítem persistido se crea con versión 1 y los
// booleanos ausentes quedan `denied`/false, nunca vacíos.
func TestE019PatchCreaConVersionUno(t *testing.T) {
	almacen := newE019Almacen()
	estado, err := e019Servicio(almacen).Patch(context.Background(), e018UsuarioA, appOnboarding.PatchCommand{
		CompletedSteps: []string{"profile"},
	})
	if err != nil {
		t.Fatalf("la creacion debe funcionar: %v", err)
	}
	if almacen.creaciones != 1 {
		t.Fatalf("debe crearse exactamente un item, creaciones=%d", almacen.creaciones)
	}
	if estado.Version != 1 {
		t.Fatalf("la version inicial debe ser 1, obtenida %d", estado.Version)
	}
	if want := []string{"profile"}; !reflect.DeepEqual(estado.CompletedSteps, want) {
		t.Fatalf("pasos %v, esperados %v", estado.CompletedSteps, want)
	}
	if estado.Done || estado.CompletedAt != nil {
		t.Fatalf("con un solo paso done=false y completedAt=null: %+v", estado)
	}
	persistido := almacen.registros[e018UsuarioA]
	if persistido.MicrophoneRaw != "denied" || persistido.NotificationsRaw != "denied" || persistido.Configured {
		t.Fatalf("los opcionales ausentes deben persistir denied/false: %+v", persistido)
	}
}

// TestE019PatchVacioAceptado: `completedSteps: []` es válido y deja el estado sin
// pasos, con versión avanzada.
func TestE019PatchVacioAceptado(t *testing.T) {
	almacen := newE019Almacen()
	e019Sembrar(almacen, e018UsuarioA, appOnboarding.Record{
		CompletedSteps: []string{"profile", "income"},
		Version:        3,
		HasVersion:     true,
		MicrophoneRaw:  "granted",
	})
	estado, err := e019Servicio(almacen).Patch(context.Background(), e018UsuarioA, appOnboarding.PatchCommand{
		CompletedSteps: []string{},
	})
	if err != nil {
		t.Fatalf("un array vacio es valido: %v", err)
	}
	if estado.CompletedSteps == nil || len(estado.CompletedSteps) != 0 {
		t.Fatalf("completedSteps debe quedar [], obtenido %#v", estado.CompletedSteps)
	}
	if estado.Version != 4 {
		t.Fatalf("la version debe avanzar a 4, obtenida %d", estado.Version)
	}
	if !estado.Microphone {
		t.Fatalf("un permiso no enviado debe conservarse (granted)")
	}
}

// TestE019PatchCompletaYMarca: con los cuatro pasos se emite `done` y la marca del
// reloj inyectado.
func TestE019PatchCompletaYMarca(t *testing.T) {
	almacen := newE019Almacen()
	e019Sembrar(almacen, e018UsuarioA, appOnboarding.Record{
		CompletedSteps: []string{"profile"},
		Version:        1,
		HasVersion:     true,
		MicrophoneRaw:  "denied",
	})
	estado, err := e019Servicio(almacen).Patch(context.Background(), e018UsuarioA, appOnboarding.PatchCommand{
		CompletedSteps: []string{"permissions", "privacy", "income", "profile"},
	})
	if err != nil {
		t.Fatalf("la escritura valida no debe fallar: %v", err)
	}
	if !estado.Done {
		t.Fatalf("cuatro pasos deben marcar done=true")
	}
	if estado.CompletedAt == nil || !estado.CompletedAt.Equal(e019Ahora) {
		t.Fatalf("completedAt debe ser el instante de la escritura (%v), obtenido %v", e019Ahora, estado.CompletedAt)
	}
	if want := []string{"profile", "income", "privacy", "permissions"}; !reflect.DeepEqual(estado.CompletedSteps, want) {
		t.Fatalf("orden canonico esperado %v, obtenido %v", want, estado.CompletedSteps)
	}
}

// TestE019PatchConservaMarcaPrevia: si ya estaba completo, un PATCH posterior no
// reescribe la fecha de finalización.
func TestE019PatchConservaMarcaPrevia(t *testing.T) {
	marca := e019Ahora.Add(-72 * time.Hour)
	almacen := newE019Almacen()
	e019Sembrar(almacen, e018UsuarioA, appOnboarding.Record{
		CompletedSteps: []string{"profile", "income", "privacy", "permissions"},
		Version:        2,
		HasVersion:     true,
		CompletedAt:    &marca,
	})
	estado, err := e019Servicio(almacen).Patch(context.Background(), e018UsuarioA, appOnboarding.PatchCommand{
		CompletedSteps: []string{"profile", "income", "privacy", "permissions"},
		Configured:     e019Puntero(true),
	})
	if err != nil {
		t.Fatalf("la escritura valida no debe fallar: %v", err)
	}
	if estado.CompletedAt == nil || !estado.CompletedAt.Equal(marca) {
		t.Fatalf("la marca original debe conservarse (%v), obtenida %v", marca, estado.CompletedAt)
	}
	if !estado.Configured {
		t.Fatalf("configured=true explicito debe persistirse")
	}
}

// TestE019PatchIncompletoBorraMarca: si el conjunto enviado deja de estar completo,
// `completedAt` vuelve a `null` (el contrato lo exige).
func TestE019PatchIncompletoBorraMarca(t *testing.T) {
	marca := e019Ahora.Add(-72 * time.Hour)
	almacen := newE019Almacen()
	e019Sembrar(almacen, e018UsuarioA, appOnboarding.Record{
		CompletedSteps: []string{"profile", "income", "privacy", "permissions"},
		Version:        6,
		HasVersion:     true,
		CompletedAt:    &marca,
	})
	estado, err := e019Servicio(almacen).Patch(context.Background(), e018UsuarioA, appOnboarding.PatchCommand{
		CompletedSteps: []string{"profile"},
	})
	if err != nil {
		t.Fatalf("la escritura valida no debe fallar: %v", err)
	}
	if estado.Done {
		t.Fatalf("un solo paso no puede marcar done")
	}
	if estado.CompletedAt != nil {
		t.Fatalf("completedAt debe ser null con done=false, obtenido %v", estado.CompletedAt)
	}
	if almacen.registros[e018UsuarioA].CompletedAt != nil {
		t.Fatalf("la marca persistida debe borrarse")
	}
}

// TestE019PatchBooleanos: `false` explícito persiste `denied`; `true` persiste
// `granted`; un campo ausente conserva el texto persistido, incluido `pending`.
func TestE019PatchBooleanos(t *testing.T) {
	almacen := newE019Almacen()
	e019Sembrar(almacen, e018UsuarioA, appOnboarding.Record{
		CompletedSteps:   []string{"profile"},
		Version:          2,
		HasVersion:       true,
		MicrophoneRaw:    "pending",
		NotificationsRaw: "granted",
		Configured:       false,
	})
	servicio := e019Servicio(almacen)

	estado, err := servicio.Patch(context.Background(), e018UsuarioA, appOnboarding.PatchCommand{
		CompletedSteps: []string{"profile"},
		Microphone:     e019Puntero(false),
		Notifications:  e019Puntero(true),
		Configured:     e019Puntero(true),
	})
	if err != nil {
		t.Fatalf("la escritura valida no debe fallar: %v", err)
	}
	persistido := almacen.registros[e018UsuarioA]
	if persistido.MicrophoneRaw != "denied" {
		t.Fatalf("microphone=false debe persistir 'denied', obtenido %q", persistido.MicrophoneRaw)
	}
	if persistido.NotificationsRaw != "granted" {
		t.Fatalf("notifications=true debe persistir 'granted', obtenido %q", persistido.NotificationsRaw)
	}
	if !persistido.Configured || !estado.Configured {
		t.Fatalf("configured=true debe persistirse")
	}
	if estado.Microphone || !estado.Notifications {
		t.Fatalf("proyeccion incorrecta: mic=%v notif=%v", estado.Microphone, estado.Notifications)
	}

	// Sin booleanos en el cuerpo, se conserva lo persistido (no se degrada a false).
	if _, err := servicio.Patch(context.Background(), e018UsuarioA, appOnboarding.PatchCommand{
		CompletedSteps: []string{"profile"},
	}); err != nil {
		t.Fatalf("la segunda escritura no debe fallar: %v", err)
	}
	conservado := almacen.registros[e018UsuarioA]
	if conservado.MicrophoneRaw != "denied" || conservado.NotificationsRaw != "granted" || !conservado.Configured {
		t.Fatalf("los opcionales ausentes no deben reescribirse: %+v", conservado)
	}
}

// TestE019PatchVersionAvanzaYReintentaUnaVez: una escritura concurrente entre la
// lectura y el CAS produce un reintento con lectura nueva, y la versión refleja el
// resultado confirmado.
func TestE019PatchVersionAvanzaYReintentaUnaVez(t *testing.T) {
	almacen := newE019Almacen()
	e019Sembrar(almacen, e018UsuarioA, appOnboarding.Record{
		CompletedSteps: []string{"profile"},
		Version:        4,
		HasVersion:     true,
	})
	almacen.antesDeEscribir = func(a *e019Almacen, userID string) {
		if a.intentos != 1 {
			return
		}
		// Otra instancia escribió entre nuestra lectura y el CAS.
		if registro, ok := a.registros[userID]; ok {
			registro.Version = 5
			registro.CompletedSteps = []string{"profile", "income"}
		}
	}

	estado, err := e019Servicio(almacen).Patch(context.Background(), e018UsuarioA, appOnboarding.PatchCommand{
		CompletedSteps: []string{"profile", "income", "privacy"},
	})
	if err != nil {
		t.Fatalf("el reintento debe resolver el conflicto: %v", err)
	}
	if almacen.lecturas != 2 || almacen.conflictos != 1 || almacen.escrituras != 1 {
		t.Fatalf("se esperaba 2 lecturas, 1 conflicto y 1 escritura: lecturas=%d conflictos=%d escrituras=%d",
			almacen.lecturas, almacen.conflictos, almacen.escrituras)
	}
	if estado.Version != 6 {
		t.Fatalf("la version final debe ser 6, obtenida %d", estado.Version)
	}
	if want := []string{"profile", "income", "privacy"}; !reflect.DeepEqual(estado.CompletedSteps, want) {
		t.Fatalf("pasos %v, esperados %v", estado.CompletedSteps, want)
	}
}

// TestE019PatchConflictoPersistente: si la versión cambia en cada intento, se
// devuelve ErrConflict (409) y no se sobrescribe nada.
func TestE019PatchConflictoPersistente(t *testing.T) {
	almacen := newE019Almacen()
	e019Sembrar(almacen, e018UsuarioA, appOnboarding.Record{
		CompletedSteps: []string{"profile"},
		Version:        4,
		HasVersion:     true,
	})
	almacen.antesDeEscribir = func(a *e019Almacen, userID string) {
		if registro, ok := a.registros[userID]; ok {
			registro.Version++
		}
	}

	_, err := e019Servicio(almacen).Patch(context.Background(), e018UsuarioA, appOnboarding.PatchCommand{
		CompletedSteps: []string{"profile", "income"},
	})
	if !errors.Is(err, appOnboarding.ErrConflict) {
		t.Fatalf("se esperaba ErrConflict, obtenido %v", err)
	}
	if almacen.intentos != 2 {
		t.Fatalf("el servicio debe reintentar exactamente una vez, intentos=%d", almacen.intentos)
	}
	if almacen.escrituras != 0 {
		t.Fatalf("ningun intento puede haber escrito, escrituras=%d", almacen.escrituras)
	}
	if want := []string{"profile"}; !reflect.DeepEqual(almacen.registros[e018UsuarioA].CompletedSteps, want) {
		t.Fatalf("el estado previo no debe modificarse: %v", almacen.registros[e018UsuarioA].CompletedSteps)
	}
}

// TestE019PatchLegacySinVersion: un ítem heredado sin atributo `version` se
// actualiza con CAS por ausencia y se materializa la versión 1.
func TestE019PatchLegacySinVersion(t *testing.T) {
	almacen := newE019Almacen()
	e019Sembrar(almacen, e018UsuarioA, appOnboarding.Record{
		CompletedSteps: []string{"profile"},
		MicrophoneRaw:  "denied",
	})
	if _, err := e019Servicio(almacen).Patch(context.Background(), e018UsuarioA, appOnboarding.PatchCommand{
		CompletedSteps: []string{"profile", "privacy"},
	}); err != nil {
		t.Fatalf("un item heredado debe poder actualizarse: %v", err)
	}
	persistido := almacen.registros[e018UsuarioA]
	if !persistido.HasVersion || persistido.Version != 1 {
		t.Fatalf("la version debe materializarse en 1: version=%d hasVersion=%v", persistido.Version, persistido.HasVersion)
	}
}

// TestE019PatchAislamiento: la escritura de A no altera el estado de B.
func TestE019PatchAislamiento(t *testing.T) {
	almacen := newE019Almacen()
	e019Sembrar(almacen, e018UsuarioA, appOnboarding.Record{CompletedSteps: []string{"profile"}, Version: 1, HasVersion: true})
	e019Sembrar(almacen, e018UsuarioB, appOnboarding.Record{CompletedSteps: []string{"income"}, Version: 9, HasVersion: true})

	if _, err := e019Servicio(almacen).Patch(context.Background(), e018UsuarioA, appOnboarding.PatchCommand{
		CompletedSteps: []string{"profile", "income", "privacy", "permissions"},
	}); err != nil {
		t.Fatalf("la escritura de A no debe fallar: %v", err)
	}
	b := almacen.registros[e018UsuarioB]
	if b.Version != 9 || !reflect.DeepEqual(b.CompletedSteps, []string{"income"}) {
		t.Fatalf("el estado de B no debe cambiar: version=%d pasos=%v", b.Version, b.CompletedSteps)
	}
}

// TestE019PatchFallaCerrado cubre 503: sin almacén, sin puerto de escritura, sin
// identidad, con lectura caída o con escritura caída no hay éxito ficticio.
func TestE019PatchFallaCerrado(t *testing.T) {
	if _, err := appOnboarding.NewService(nil).Patch(context.Background(), e018UsuarioA, appOnboarding.PatchCommand{CompletedSteps: []string{}}); !errors.Is(err, appOnboarding.ErrUnavailable) {
		t.Fatalf("sin almacen debe ser ErrUnavailable, obtenido %v", err)
	}

	soloLectura := newE018Almacen() // implementa Store, no Writer
	if _, err := e019Servicio(soloLectura).Patch(context.Background(), e018UsuarioA, appOnboarding.PatchCommand{CompletedSteps: []string{}}); !errors.Is(err, appOnboarding.ErrUnavailable) {
		t.Fatalf("un almacen sin escritura debe ser ErrUnavailable, obtenido %v", err)
	}

	lecturaCaida := newE019Almacen()
	lecturaCaida.falloLectura = errors.New("dynamodb no disponible")
	if _, err := e019Servicio(lecturaCaida).Patch(context.Background(), e018UsuarioA, appOnboarding.PatchCommand{CompletedSteps: []string{}}); !errors.Is(err, appOnboarding.ErrUnavailable) {
		t.Fatalf("una lectura caida debe ser ErrUnavailable, obtenido %v", err)
	}

	escrituraCaida := newE019Almacen()
	e019Sembrar(escrituraCaida, e018UsuarioA, appOnboarding.Record{Version: 1, HasVersion: true})
	escrituraCaida.falloEscritura = errors.New("dynamodb no disponible")
	if _, err := e019Servicio(escrituraCaida).Patch(context.Background(), e018UsuarioA, appOnboarding.PatchCommand{CompletedSteps: []string{}}); !errors.Is(err, appOnboarding.ErrUnavailable) {
		t.Fatalf("una escritura caida debe ser ErrUnavailable, obtenido %v", err)
	}

	vacio := newE019Almacen()
	if _, err := e019Servicio(vacio).Patch(context.Background(), "   ", appOnboarding.PatchCommand{CompletedSteps: []string{}}); !errors.Is(err, appOnboarding.ErrUnauthorized) {
		t.Fatalf("sin identidad debe ser ErrUnauthorized, obtenido %v", err)
	}
}

// TestE019PatchConcurrenteNoPierdeEstado: con varias instancias escribiendo a la
// vez, cada resultado es éxito o conflicto declarado, la versión avanza una vez por
// escritura confirmada y el estado final es coherente.
func TestE019PatchConcurrenteNoPierdeEstado(t *testing.T) {
	almacen := newE019Almacen()
	e019Sembrar(almacen, e018UsuarioA, appOnboarding.Record{
		CompletedSteps: []string{"profile"},
		Version:        1,
		HasVersion:     true,
	})
	servicio := e019Servicio(almacen)

	const intentos = 6
	resultados := make([]error, intentos)
	var wg sync.WaitGroup
	for i := 0; i < intentos; i++ {
		wg.Add(1)
		go func(indice int) {
			defer wg.Done()
			_, err := servicio.Patch(context.Background(), e018UsuarioA, appOnboarding.PatchCommand{
				CompletedSteps: []string{"profile", "income"},
			})
			resultados[indice] = err
		}(i)
	}
	wg.Wait()

	exitos := 0
	for _, err := range resultados {
		switch {
		case err == nil:
			exitos++
		case errors.Is(err, appOnboarding.ErrConflict):
			// Conflicto declarado: el cliente debe releer con E018.
		default:
			t.Fatalf("error inesperado en concurrencia: %v", err)
		}
	}
	if exitos == 0 {
		t.Fatalf("al menos una escritura debe confirmarse")
	}
	persistido := almacen.registros[e018UsuarioA]
	if persistido.Version != int64(1+exitos) {
		t.Fatalf("la version debe ser 1+exitos=%d, obtenida %d", 1+exitos, persistido.Version)
	}
	if want := []string{"profile", "income"}; !reflect.DeepEqual(persistido.CompletedSteps, want) {
		t.Fatalf("el estado final debe ser el confirmado, obtenido %v", persistido.CompletedSteps)
	}
}
