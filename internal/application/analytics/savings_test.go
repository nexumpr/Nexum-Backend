package analytics

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	appContrib "github.com/nexumpr/Nexum-Backend/internal/application/contributions"
)

type e056Store struct {
	pages map[string][][]appContrib.Contribution
	zonas map[string]string
	err   error
}

func (f *e056Store) ListContributions(_ context.Context, q appContrib.ListQuery, startSK string, _ int) ([]appContrib.Contribution, string, error) {
	if f.err != nil {
		return nil, "", f.err
	}
	indice := 0
	if startSK != "" {
		var err error
		indice, err = strconv.Atoi(startSK)
		if err != nil || indice < 1 {
			return nil, "", errors.New("cursor de prueba invalido")
		}
	}
	paginas := f.pages[q.UserID]
	if indice >= len(paginas) {
		return nil, "", nil
	}
	filtradas := make([]appContrib.Contribution, 0, len(paginas[indice]))
	for _, c := range paginas[indice] {
		if q.HasFrom && c.OccurredAt.Before(q.From) {
			continue
		}
		if q.HasTo && !c.OccurredAt.Before(q.To) {
			continue
		}
		if q.Status != "" && c.Status != q.Status {
			continue
		}
		filtradas = append(filtradas, c)
	}
	siguiente := ""
	if indice+1 < len(paginas) {
		siguiente = strconv.Itoa(indice + 1)
	}
	return filtradas, siguiente, nil
}

func (f *e056Store) LoadTimezone(_ context.Context, userID string) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	return f.zonas[userID], nil
}

func e056Aportacion(usuario, id string, instante time.Time, cents int64, estado string) appContrib.Contribution {
	return appContrib.Contribution{
		ID:          id,
		UserID:      usuario,
		AmountCents: cents,
		Currency:    "MXN",
		OccurredAt:  instante,
		Status:      estado,
	}
}

func e056Servicio(store *e056Store, ahora time.Time) *Service {
	return NewService(nil).WithSavings(store, store).WithClock(func() time.Time { return ahora })
}

func e056UTC(dia, hora int) time.Time {
	return time.Date(2026, 11, dia, hora, 0, 0, 0, time.UTC)
}

func TestE056RachaCerosYConsecutivos(t *testing.T) {
	ctx := context.Background()
	ahora := e056UTC(20, 12)
	casos := []struct {
		nombre string
		dias   []int
		espera int
	}{
		{"cero", nil, 0},
		{"uno", []int{20}, 1},
		{"siete", []int{14, 15, 16, 17, 18, 19, 20}, 7},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			items := make([]appContrib.Contribution, 0, len(caso.dias))
			for _, dia := range caso.dias {
				items = append(items, e056Aportacion("user-a", "ap-"+strconv.Itoa(dia), e056UTC(dia, 12), 10000, appContrib.StatusActive))
			}
			store := &e056Store{
				pages: map[string][][]appContrib.Contribution{"user-a": {items}},
				zonas: map[string]string{"user-a": "America/Mexico_City"},
			}
			racha, err := e056Servicio(store, ahora).SavingsStreak(ctx, "user-a")
			if err != nil {
				t.Fatal(err)
			}
			if racha.Days != caso.espera || racha.AsOfDate != "2026-11-20" || racha.Timezone != "America/Mexico_City" {
				t.Fatalf("racha incorrecta: %+v", racha)
			}
		})
	}
}

func TestE056DiaSinAportacionRompeRacha(t *testing.T) {
	items := []appContrib.Contribution{
		e056Aportacion("user-a", "ap-20", e056UTC(20, 12), 10000, appContrib.StatusActive),
		e056Aportacion("user-a", "ap-19", e056UTC(19, 12), 10000, appContrib.StatusActive),
		// Falta el 18: la racha se detiene en 2 aunque el 17 exista.
		e056Aportacion("user-a", "ap-17", e056UTC(17, 12), 10000, appContrib.StatusActive),
	}
	store := &e056Store{
		pages: map[string][][]appContrib.Contribution{"user-a": {items}},
		zonas: map[string]string{"user-a": "America/Mexico_City"},
	}
	racha, err := e056Servicio(store, e056UTC(20, 12)).SavingsStreak(context.Background(), "user-a")
	if err != nil {
		t.Fatal(err)
	}
	if racha.Days != 2 {
		t.Fatalf("esperada racha de 2, obtenida %+v", racha)
	}
}

func TestE056ExcluyeRevertidasCeroYDeduplica(t *testing.T) {
	items := []appContrib.Contribution{
		e056Aportacion("user-a", "ap-20", e056UTC(20, 12), 10000, appContrib.StatusActive),
		e056Aportacion("user-a", "ap-revertida", e056UTC(19, 12), 10000, appContrib.StatusReversed),
		e056Aportacion("user-a", "ap-cero", e056UTC(18, 12), 0, appContrib.StatusActive),
		e056Aportacion("user-a", "ap-negativa", e056UTC(17, 12), -100, appContrib.StatusActive),
		e056Aportacion("user-a", "ap-20", e056UTC(16, 12), 10000, appContrib.StatusActive),
	}
	revertidaConMarca := items[1]
	revertidaConMarca.ReversedAt = e056UTC(20, 10)
	items[1] = revertidaConMarca

	store := &e056Store{
		pages: map[string][][]appContrib.Contribution{"user-a": {items}},
		zonas: map[string]string{"user-a": "America/Mexico_City"},
	}
	racha, err := e056Servicio(store, e056UTC(20, 12)).SavingsStreak(context.Background(), "user-a")
	if err != nil {
		t.Fatal(err)
	}
	if racha.Days != 1 {
		t.Fatalf("esperada solo la aportacion activa unica: %+v", racha)
	}
}

func TestE056PaginacionYDeduplicacionEntrePaginas(t *testing.T) {
	primera := []appContrib.Contribution{
		e056Aportacion("user-a", "ap-20", e056UTC(20, 12), 10000, appContrib.StatusActive),
		e056Aportacion("user-a", "ap-19", e056UTC(19, 12), 10000, appContrib.StatusActive),
	}
	segunda := []appContrib.Contribution{
		e056Aportacion("user-a", "ap-20", e056UTC(20, 12), 10000, appContrib.StatusActive),
		e056Aportacion("user-a", "ap-18", e056UTC(18, 12), 10000, appContrib.StatusActive),
	}
	store := &e056Store{
		pages: map[string][][]appContrib.Contribution{"user-a": {primera, segunda}},
		zonas: map[string]string{"user-a": "America/Mexico_City"},
	}
	racha, err := e056Servicio(store, e056UTC(20, 12)).SavingsStreak(context.Background(), "user-a")
	if err != nil {
		t.Fatal(err)
	}
	if racha.Days != 3 {
		t.Fatalf("esperada racha de 3 tras deduplicar paginas: %+v", racha)
	}
}

func TestE056CruceMedianocheZonaIANA(t *testing.T) {
	// 02:00 UTC del 21 es 20:00 local del 20; 10:00 UTC del 21 es 04:00 local del 21.
	items := []appContrib.Contribution{
		e056Aportacion("user-a", "ap-local-20", time.Date(2026, 11, 21, 2, 0, 0, 0, time.UTC), 10000, appContrib.StatusActive),
		e056Aportacion("user-a", "ap-local-21", time.Date(2026, 11, 21, 10, 0, 0, 0, time.UTC), 10000, appContrib.StatusActive),
	}
	store := &e056Store{
		pages: map[string][][]appContrib.Contribution{"user-a": {items}},
		zonas: map[string]string{"user-a": "America/Mexico_City"},
	}
	racha, err := e056Servicio(store, e056UTC(21, 12)).SavingsStreak(context.Background(), "user-a")
	if err != nil {
		t.Fatal(err)
	}
	if racha.Days != 2 || racha.AsOfDate != "2026-11-21" {
		t.Fatalf("el dia civil local no se aplico: %+v", racha)
	}
}

func TestE056CambioDST(t *testing.T) {
	// En America/New_York el 1 de noviembre de 2026 hay dos 01:30 locales con
	// offsets distintos. Ambas marcas deben caer en el MISMO dia civil.
	instante1 := time.Date(2026, 11, 1, 5, 30, 0, 0, time.UTC)
	instante2 := time.Date(2026, 11, 1, 6, 30, 0, 0, time.UTC)
	items := []appContrib.Contribution{
		e056Aportacion("user-a", "ap-dst-1", instante1, 10000, appContrib.StatusActive),
		e056Aportacion("user-a", "ap-dst-2", instante2, 10000, appContrib.StatusActive),
		e056Aportacion("user-a", "ap-dst-3", time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC), 10000, appContrib.StatusActive),
	}
	store := &e056Store{
		pages: map[string][][]appContrib.Contribution{"user-a": {items}},
		zonas: map[string]string{"user-a": "America/New_York"},
	}
	racha, err := e056Servicio(store, time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)).SavingsStreak(context.Background(), "user-a")
	if err != nil {
		t.Fatal(err)
	}
	if racha.Days != 2 || racha.Timezone != "America/New_York" || racha.AsOfDate != "2026-11-02" {
		t.Fatalf("DST mal tratado: %+v", racha)
	}
}

func TestE056ZonaInvalidaCaeAlFallback(t *testing.T) {
	items := []appContrib.Contribution{
		e056Aportacion("user-a", "ap-20", e056UTC(20, 12), 10000, appContrib.StatusActive),
	}
	store := &e056Store{
		pages: map[string][][]appContrib.Contribution{"user-a": {items}},
		zonas: map[string]string{"user-a": "Not/AZone"},
	}
	racha, err := e056Servicio(store, e056UTC(20, 12)).SavingsStreak(context.Background(), "user-a")
	if err != nil {
		t.Fatal(err)
	}
	if racha.Timezone != DefaultSavingsTimezone || racha.Days != 1 {
		t.Fatalf("fallback IANA incorrecto: %+v", racha)
	}
}

func TestE056AislamientoYNegativas(t *testing.T) {
	ctx := context.Background()
	ahora := e056UTC(20, 12)
	store := &e056Store{
		pages: map[string][][]appContrib.Contribution{
			"user-a": {{e056Aportacion("user-a", "ap-20", e056UTC(20, 12), 10000, appContrib.StatusActive)}},
		},
		zonas: map[string]string{"user-a": "America/Mexico_City", "user-b": "America/Mexico_City"},
	}
	servicio := e056Servicio(store, ahora)
	b, err := servicio.SavingsStreak(ctx, "user-b")
	if err != nil {
		t.Fatal(err)
	}
	if b.Days != 0 || strings.Contains(b.AsOfDate, "user-a") {
		t.Fatalf("usuario B recibio datos de A: %+v", b)
	}
	if _, err := servicio.SavingsStreak(ctx, ""); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("sin titular: esperado ErrUnauthorized, obtenido %v", err)
	}
	if _, err := NewService(nil).SavingsStreak(ctx, "user-a"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("sin dependencias: esperado ErrUnavailable, obtenido %v", err)
	}
	fallido := e056Servicio(&e056Store{
		pages: map[string][][]appContrib.Contribution{},
		zonas: map[string]string{"user-a": "America/Mexico_City"},
		err:   appContrib.ErrUnavailable,
	}, ahora)
	if _, err := fallido.SavingsStreak(ctx, "user-a"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("store caido: esperado ErrUnavailable, obtenido %v", err)
	}
}
