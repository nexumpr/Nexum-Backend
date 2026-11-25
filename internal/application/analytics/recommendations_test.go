package analytics

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	appMovements "github.com/nexumpr/Nexum-Backend/internal/application/movements"
	"github.com/nexumpr/Nexum-Backend/ml"
)

var uuidPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-5[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func e055Movimientos(usuario, categoria string, cantidad int, cents int64) []appMovements.Movement {
	salida := make([]appMovements.Movement, 0, cantidad)
	for i := 0; i < cantidad; i++ {
		dia := 1 + i%28
		hora := 8 + i%10
		salida = append(salida, appMovements.Movement{
			ID:                 fmt.Sprintf("e055-%s-%s-%d", usuario, categoria, i),
			Type:               appMovements.TypeExpense,
			AmountCents:        cents,
			Currency:           "MXN",
			CategoryID:         categoria,
			OccurredAt:         time.Date(2026, 11, dia, hora, 0, 0, 0, time.UTC),
			RecurringPaymentID: fmt.Sprintf("rec-%d", i%3),
		})
	}
	return salida
}

func e055Fuente(usuario string, movimientos ...[]appMovements.Movement) *e053PaginatedSource {
	todos := make([]appMovements.Movement, 0)
	for _, bloque := range movimientos {
		todos = append(todos, bloque...)
	}
	return &e053PaginatedSource{pages: map[string][][]appMovements.Movement{usuario: {todos}}}
}

func e055Servicio(fuente *e053PaginatedSource) *Service {
	return NewService(fuente).WithClock(func() time.Time {
		return time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	})
}

func TestE055HistorialInsuficiente(t *testing.T) {
	ctx := context.Background()
	casos := []struct {
		nombre      string
		movimientos []appMovements.Movement
	}{
		{"cero", nil},
		{"cinco", append(e055Movimientos("user-a", "comida", 3, 18000), e055Movimientos("user-a", "transporte", 2, 8000)...)},
		{"once", append(e055Movimientos("user-a", "comida", 6, 18000), e055Movimientos("user-a", "transporte", 5, 8000)...)},
		{"doce_una_categoria", e055Movimientos("user-a", "comida", 12, 18000)},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			servicio := e055Servicio(e055Fuente("user-a", caso.movimientos))
			pagina, err := servicio.Recommendations(ctx, "user-a", "2026-11")
			if err != nil {
				t.Fatalf("recomendaciones: %v", err)
			}
			if pagina.HasEnoughData || len(pagina.Items) != 0 || pagina.NextCursor != "" {
				t.Fatalf("esperado hasEnoughData=false e items vacios: %+v", pagina)
			}
		})
	}
}

func TestE055RecomendacionesKMeansReales(t *testing.T) {
	ctx := context.Background()
	fuente := e055Fuente("user-a",
		e055Movimientos("user-a", "comida", 6, 18000),
		e055Movimientos("user-a", "vivienda", 6, 280000),
	)
	servicio := e055Servicio(fuente)

	pagina, err := servicio.Recommendations(ctx, "user-a", "2026-11")
	if err != nil {
		t.Fatalf("recomendaciones: %v", err)
	}
	if !pagina.HasEnoughData {
		t.Fatal("historial suficiente marcado como insuficiente")
	}
	if len(pagina.Items) == 0 {
		t.Fatal("K-Means no produjo ningun cluster con masa suficiente")
	}
	if pagina.NextCursor != "" {
		t.Fatalf("E055 no declara cursor de entrada: %q", pagina.NextCursor)
	}
	visto := make(map[string]bool)
	for _, item := range pagina.Items {
		if !uuidPattern.MatchString(item.ID) {
			t.Fatalf("id no UUID v5: %q", item.ID)
		}
		if visto[item.ID] {
			t.Fatalf("id duplicado: %s", item.ID)
		}
		visto[item.ID] = true
		if item.ModelVersion != ml.ActiveModelVersion {
			t.Fatalf("modelVersion no corresponde al artefacto v2: %q", item.ModelVersion)
		}
		if item.Disclaimer != RecommendationDisclaimer {
			t.Fatalf("disclaimer ausente o alterado: %q", item.Disclaimer)
		}
		if !strings.Contains(item.Reason, "movimientos") || !strings.Contains(item.Reason, "2026-11") {
			t.Fatalf("razon sin hechos verificables: %q", item.Reason)
		}
		for _, promesa := range []string{"ahorro garantizado", "ganancia asegurada", "reducirás"} {
			if strings.Contains(strings.ToLower(item.Reason), promesa) {
				t.Fatalf("razon con promesa prohibida: %q", item.Reason)
			}
		}
	}
	if strings.Contains(fmt.Sprint(pagina), "user-a") {
		t.Fatal("la respuesta expone el identificador del titular")
	}

	repetida, err := servicio.Recommendations(ctx, "user-a", "2026-11")
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(pagina) != fmt.Sprint(repetida) {
		t.Fatal("la misma peticion produjo respuestas distintas")
	}
}

func TestE055AislamientoEntreUsuarios(t *testing.T) {
	ctx := context.Background()
	fuente := &e053PaginatedSource{pages: map[string][][]appMovements.Movement{
		"user-a": {append(e055Movimientos("user-a", "comida", 6, 18000), e055Movimientos("user-a", "vivienda", 6, 280000)...)},
		"user-b": {append(e055Movimientos("user-b", "transporte", 6, 9000), e055Movimientos("user-b", "servicios", 6, 45000)...)},
	}}
	servicio := e055Servicio(fuente)
	a, err := servicio.Recommendations(ctx, "user-a", "2026-11")
	if err != nil {
		t.Fatal(err)
	}
	b, err := servicio.Recommendations(ctx, "user-b", "2026-11")
	if err != nil {
		t.Fatal(err)
	}
	if !a.HasEnoughData || !b.HasEnoughData || len(a.Items) == 0 || len(b.Items) == 0 {
		t.Fatalf("esperados resultados para ambos: A=%+v B=%+v", a, b)
	}
	if fmt.Sprint(a) == fmt.Sprint(b) {
		t.Fatal("dos historiales distintos produjeron la misma respuesta")
	}
	if strings.Contains(fmt.Sprint(a), "transporte") || strings.Contains(fmt.Sprint(b), "comida") {
		t.Fatal("fuga de categorias entre titulares")
	}
}

func TestE055SiluetaInsuficiente(t *testing.T) {
	modelo, err := ml.Load()
	if err != nil {
		t.Fatal(err)
	}
	copia := *modelo
	copia.SilhouetteTraining = 0.34
	fuente := e055Fuente("user-a",
		e055Movimientos("user-a", "comida", 6, 18000),
		e055Movimientos("user-a", "vivienda", 6, 280000),
	)
	servicio := e055Servicio(fuente).WithModelo(&copia)
	pagina, err := servicio.Recommendations(context.Background(), "user-a", "2026-11")
	if err != nil {
		t.Fatalf("silueta baja no debe ser error: %v", err)
	}
	if pagina.HasEnoughData || len(pagina.Items) != 0 {
		t.Fatalf("silueta 0.34 emitio recomendaciones: %+v", pagina)
	}
}

func TestE055Ventana90DiasPorDefecto(t *testing.T) {
	ctx := context.Background()
	octubre := e055Movimientos("user-a", "comida", 6, 18000)
	servicios := e055Movimientos("user-a", "servicios", 6, 45000)
	for i := range octubre {
		octubre[i].OccurredAt = octubre[i].OccurredAt.AddDate(0, -1, 0)
	}
	for i := range servicios {
		servicios[i].OccurredAt = servicios[i].OccurredAt.AddDate(0, -1, 0)
	}
	servicio := e055Servicio(e055Fuente("user-a", octubre, servicios))

	// El diseño fija 90 días móviles. Esta prueba codifica que octubre (trimestre
	// en curso respecto al reloj de prueba) sí cuenta: la línea de "excluir el
	// trimestre en curso" no se aplica porque contradice la ventana de 90 días.
	porDefecto, err := servicio.Recommendations(ctx, "user-a", "")
	if err != nil {
		t.Fatal(err)
	}
	if !porDefecto.HasEnoughData || len(porDefecto.Items) == 0 {
		t.Fatalf("la ventana de 90 dias debio incluir octubre: %+v", porDefecto)
	}
	explicito, err := servicio.Recommendations(ctx, "user-a", "2026-11")
	if err != nil {
		t.Fatal(err)
	}
	if explicito.HasEnoughData || len(explicito.Items) != 0 {
		t.Fatalf("el periodo explicito no debio incluir octubre: %+v", explicito)
	}
}

func TestE055MuestreoDeterminista500(t *testing.T) {
	fuente := e055Fuente("user-a",
		e055Movimientos("user-a", "comida", 251, 18000),
		e055Movimientos("user-a", "vivienda", 250, 280000),
	)
	pagina, err := e055Servicio(fuente).Recommendations(context.Background(), "user-a", "2026-11")
	if err != nil {
		t.Fatal(err)
	}
	if !pagina.HasEnoughData || len(pagina.Items) == 0 {
		t.Fatalf("esperadas recomendaciones tras el muestreo: %+v", pagina)
	}
	encontrado := false
	for _, item := range pagina.Items {
		if strings.Contains(item.Reason, "La muestra se limitó a 500 movimientos") {
			encontrado = true
		}
	}
	if !encontrado {
		t.Fatalf("el muestreo no fue declarado en la razon: %+v", pagina.Items)
	}
}

func BenchmarkE055Inferencia500(b *testing.B) {
	fuente := e055Fuente("user-a",
		e055Movimientos("user-a", "comida", 251, 18000),
		e055Movimientos("user-a", "vivienda", 249, 280000),
	)
	servicio := e055Servicio(fuente)
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := servicio.Recommendations(ctx, "user-a", "2026-11"); err != nil {
			b.Fatal(err)
		}
	}
}

func TestE055Latencia500Movimientos(t *testing.T) {
	fuente := e055Fuente("user-a",
		e055Movimientos("user-a", "comida", 251, 18000),
		e055Movimientos("user-a", "vivienda", 249, 280000),
	)
	servicio := e055Servicio(fuente)
	ctx := context.Background()
	if _, err := servicio.Recommendations(ctx, "user-a", "2026-11"); err != nil {
		t.Fatal(err)
	}
	const iteraciones = 30
	duraciones := make([]time.Duration, 0, iteraciones)
	for i := 0; i < iteraciones; i++ {
		inicio := time.Now()
		if _, err := servicio.Recommendations(ctx, "user-a", "2026-11"); err != nil {
			t.Fatal(err)
		}
		duraciones = append(duraciones, time.Since(inicio))
	}
	sort.Slice(duraciones, func(i, j int) bool { return duraciones[i] < duraciones[j] })
	p95 := duraciones[(iteraciones*95/100)-1]
	if p95 > 300*time.Millisecond {
		t.Fatalf("p95 %s supera el objetivo de 300ms", p95)
	}
	t.Logf("E055_LATENCIA_LOCAL p95=%s iteraciones=%d movimientos=500", p95, iteraciones)
}

func TestE055Negativas(t *testing.T) {
	ctx := context.Background()
	fuente := e055Fuente("user-a",
		e055Movimientos("user-a", "comida", 6, 18000),
		e055Movimientos("user-a", "vivienda", 6, 280000),
	)
	servicio := e055Servicio(fuente)
	if _, err := servicio.Recommendations(ctx, "user-a", "2026-13"); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("periodo invalido: esperado ErrInvalidRequest, obtenido %v", err)
	}
	if _, err := servicio.Recommendations(ctx, "", "2026-11"); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("sin titular: esperado ErrUnauthorized, obtenido %v", err)
	}
	if _, err := NewService(nil).Recommendations(ctx, "user-a", "2026-11"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("sin fuente: esperado ErrUnavailable, obtenido %v", err)
	}
	caido := e055Servicio(&e053PaginatedSource{err: appMovements.ErrUnavailable})
	if _, err := caido.Recommendations(ctx, "user-a", "2026-11"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Dynamo caido: esperado ErrUnavailable, obtenido %v", err)
	}
}

func TestE055ImportesNoPositivosNoCuentan(t *testing.T) {
	ctx := context.Background()
	noPositivos := []appMovements.Movement{
		{ID: "cero", Type: appMovements.TypeExpense, AmountCents: 0, CategoryID: "comida", OccurredAt: time.Date(2026, 11, 2, 9, 0, 0, 0, time.UTC)},
		{ID: "negativo", Type: appMovements.TypeExpense, AmountCents: -500, CategoryID: "comida", OccurredAt: time.Date(2026, 11, 3, 9, 0, 0, 0, time.UTC)},
	}
	once := append(e055Movimientos("user-a", "comida", 6, 18000), e055Movimientos("user-a", "vivienda", 5, 280000)...)
	once = append(once, noPositivos...)
	pagina, err := e055Servicio(e055Fuente("user-a", once)).Recommendations(ctx, "user-a", "2026-11")
	if err != nil {
		t.Fatal(err)
	}
	if pagina.HasEnoughData || len(pagina.Items) != 0 {
		t.Fatalf("11 positivos + no positivos no debe alcanzar el minimo: %+v", pagina)
	}

	doce := append(e055Movimientos("user-a", "comida", 6, 18000), e055Movimientos("user-a", "vivienda", 6, 280000)...)
	doce = append(doce, noPositivos...)
	pagina, err = e055Servicio(e055Fuente("user-a", doce)).Recommendations(ctx, "user-a", "2026-11")
	if err != nil {
		t.Fatal(err)
	}
	if !pagina.HasEnoughData || len(pagina.Items) == 0 {
		t.Fatalf("12 positivos con no positivos intercalados debe calcular: %+v", pagina)
	}
}

func TestE055RecomendacionesDependenDelClustering(t *testing.T) {
	ctx := context.Background()
	modelo, err := ml.Load()
	if err != nil {
		t.Fatal(err)
	}

	bajo := e055Fuente("user-a",
		e055Movimientos("user-a", "comida", 6, 18000),
		e055Movimientos("user-a", "vivienda", 6, 280000),
	)
	unCluster := e055Fuente("user-b",
		e055Movimientos("user-b", "comida", 6, 18000),
		e055Movimientos("user-b", "transporte", 6, 8000),
	)
	servicio := e055Servicio(bajo)
	conDosGrupos, err := servicio.Recommendations(ctx, "user-a", "2026-11")
	if err != nil {
		t.Fatal(err)
	}
	conUnGrupo, err := e055Servicio(unCluster).Recommendations(ctx, "user-b", "2026-11")
	if err != nil {
		t.Fatal(err)
	}
	if !conDosGrupos.HasEnoughData || !conUnGrupo.HasEnoughData {
		t.Fatalf("historial suficiente esperado en ambos: A=%+v B=%+v", conDosGrupos, conUnGrupo)
	}
	t.Logf("E055_CLUSTERING grupos_con_dos_escalas=%d grupos_con_una_escala=%d",
		len(conDosGrupos.Items), len(conUnGrupo.Items))
	if fmt.Sprint(conDosGrupos) == fmt.Sprint(conUnGrupo) {
		t.Fatal("dos composiciones distintas produjeron la misma respuesta")
	}
	if len(conDosGrupos.Items) <= len(conUnGrupo.Items) {
		t.Fatalf("el historial con dos escalas debio producir mas grupos: A=%d B=%d",
			len(conDosGrupos.Items), len(conUnGrupo.Items))
	}
	if !strings.Contains(fmt.Sprint(conDosGrupos.Items), "vivienda") {
		t.Fatalf("la recomendacion del grupo alto no cita vivienda: %+v", conDosGrupos.Items)
	}
	if strings.Contains(fmt.Sprint(conUnGrupo.Items), "vivienda") {
		t.Fatalf("el historial de un solo grupo no debe citar vivienda: %+v", conUnGrupo.Items)
	}

	// Cambiar solo los centroides (sin tocar movimientos) debe cambiar la salida:
	// las recomendaciones dependen de la asignación, no de textos fijos.
	copia := *modelo
	copia.Centroids = make([][]float64, len(modelo.Centroids))
	for i := range modelo.Centroids {
		copia.Centroids[i] = append([]float64(nil), modelo.Centroids[i]...)
	}
	copia.Centroids[0], copia.Centroids[1] = copia.Centroids[1], copia.Centroids[0]
	invertida, err := e055Servicio(bajo).WithModelo(&copia).Recommendations(ctx, "user-a", "2026-11")
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(invertida) == fmt.Sprint(conDosGrupos) {
		t.Fatal("invertir los centroides no cambio las recomendaciones")
	}
}
