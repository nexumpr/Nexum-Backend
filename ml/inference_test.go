package ml

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
	"os"
	"strings"
	"testing"
)

const (
	v1ArtifactSHA256 = "1997370f1c148df63e126e477d1f931c565d2b1f9b54dd57dc1e48b1a16d48cf"
	v1ArtifactModel  = "kmeans-v1"
	syntheticDataSHA = "8d2bf29691a08eee6a1cd1230ad81b8454712424721e5e3e21bb9e9bd99918fd"
)

type registroJSON struct {
	Category    string `json:"category"`
	AmountCents int64  `json:"amount_cents"`
	Hour        int    `json:"hour"`
	DayOfWeek   int    `json:"day_of_week"`
	Recurring   bool   `json:"recurring"`
}

type muestraReferencia struct {
	Index            int          `json:"index"`
	Record           registroJSON `json:"record"`
	ExpectedCluster  int          `json:"expected_cluster"`
	ExpectedFeatures []float64    `json:"expected_features"`
}

type referenciaV2 struct {
	ModelVersion string   `json:"model_version"`
	FeatureOrder []string `json:"feature_order"`
	Median       struct {
		Definition string  `json:"definition"`
		Cents      float64 `json:"cents"`
	} `json:"median"`
	Train struct {
		DataFile   string              `json:"data_file"`
		DataSHA256 string              `json:"data_sha256"`
		Samples    []muestraReferencia `json:"samples"`
	} `json:"train"`
	Holdout struct {
		Seed    int64               `json:"seed"`
		Records []registroJSON      `json:"records"`
		Samples []muestraReferencia `json:"samples"`
	} `json:"holdout"`
}

type metricasV2 struct {
	ModelVersion        string  `json:"model_version"`
	SchemaVersion       int     `json:"schema_version"`
	Python              string  `json:"python"`
	Sklearn             string  `json:"sklearn"`
	Numpy               string  `json:"numpy"`
	SelectedK           int     `json:"selected_k"`
	SelectedSilhouette  float64 `json:"selected_silhouette"`
	ArtifactSHA256      string  `json:"artifact_sha256"`
	ReferenceSHA256     string  `json:"reference_sha256"`
	SyntheticDataSHA256 string  `json:"synthetic_data_sha256"`
	Holdout             struct {
		Seed             int64   `json:"seed"`
		Samples          int     `json:"samples"`
		Silhouette       float64 `json:"silhouette"`
		Inertia          float64 `json:"inertia"`
		PurityProfile    float64 `json:"purity_profile"`
		PurityScaleGroup float64 `json:"purity_scale_group"`
		ClusterSizes     []int   `json:"cluster_sizes"`
		Note             string  `json:"note"`
	} `json:"holdout"`
}

func cargarReferencia(t testing.TB) referenciaV2 {
	t.Helper()
	crudo, err := ReferenciaBytes()
	if err != nil {
		t.Fatalf("referencia: %v", err)
	}
	var referencia referenciaV2
	if err := json.Unmarshal(crudo, &referencia); err != nil {
		t.Fatalf("referencia invalida: %v", err)
	}
	return referencia
}

func cargarMetricas(t testing.TB) metricasV2 {
	t.Helper()
	crudo, err := MetricasBytes()
	if err != nil {
		t.Fatalf("metricas: %v", err)
	}
	var metricas metricasV2
	if err := json.Unmarshal(crudo, &metricas); err != nil {
		t.Fatalf("metricas invalidas: %v", err)
	}
	return metricas
}

func registrosDesdeJSON(records []registroJSON) []Registro {
	salida := make([]Registro, 0, len(records))
	for _, r := range records {
		salida = append(salida, Registro(r))
	}
	return salida
}

func registrosDesdeDatos(t testing.TB) []Registro {
	t.Helper()
	crudo, err := DatosBytes()
	if err != nil {
		t.Fatalf("datos: %v", err)
	}
	var records []registroJSON
	if err := json.Unmarshal(crudo, &records); err != nil {
		t.Fatalf("datos invalidos: %v", err)
	}
	return registrosDesdeJSON(records)
}

func copiarArtefacto(a *Artifact) *Artifact {
	if a == nil {
		return nil
	}
	b := *a
	b.FeatureOrder = append([]string(nil), a.FeatureOrder...)
	b.Categories = append([]string(nil), a.Categories...)
	b.Centroids = make([][]float64, len(a.Centroids))
	for i := range a.Centroids {
		b.Centroids[i] = append([]float64(nil), a.Centroids[i]...)
	}
	return &b
}

func compararFeatures(t *testing.T, contexto string, obtenidas, esperadas []float64, tolerancia float64) {
	t.Helper()
	if len(obtenidas) != len(esperadas) {
		t.Fatalf("%s: %d features, esperadas %d", contexto, len(obtenidas), len(esperadas))
	}
	for i := range esperadas {
		if math.Abs(obtenidas[i]-esperadas[i]) > tolerancia {
			t.Fatalf("%s: feature %d = %.12f, esperada %.12f", contexto, i, obtenidas[i], esperadas[i])
		}
	}
}

func TestArtefactoV2Verificado(t *testing.T) {
	modelo, err := Load()
	if err != nil {
		t.Fatalf("cargar artefacto: %v", err)
	}
	if modelo.Version() != ActiveModelVersion || modelo.SchemaVersion != ActiveSchemaVersion {
		t.Fatalf("version/esquema: %q/%d", modelo.Version(), modelo.SchemaVersion)
	}
	if modelo.Seed != 20261116 || modelo.NInit != 10 || modelo.K < 2 || modelo.K > 6 {
		t.Fatalf("semilla/n_init/k: %d/%d/%d", modelo.Seed, modelo.NInit, modelo.K)
	}
	if !modelo.TieneEstructura() {
		t.Fatalf("silueta %.6f por debajo del umbral", modelo.SilhouetteTraining)
	}
	if modelo.MedianDefinition != MedianDefinitionTrained {
		t.Fatalf("definicion de mediana: %q", modelo.MedianDefinition)
	}
	if modelo.AmountClip.LowerPercentile != 0.5 || modelo.AmountClip.UpperPercentile != 99.5 ||
		modelo.AmountClip.LowerCents <= 0 || modelo.AmountClip.UpperCents <= modelo.AmountClip.LowerCents {
		t.Fatalf("recorte invalido: %+v", modelo.AmountClip)
	}
	if len(modelo.FeatureOrder) != 5+len(modelo.Categories)+2 {
		t.Fatalf("dimension de features: %d", len(modelo.FeatureOrder))
	}

	bytesArtefacto, err := ArtefactoBytes()
	if err != nil {
		t.Fatal(err)
	}
	sumaArtefacto := sha256.Sum256(bytesArtefacto)
	metricas := cargarMetricas(t)
	if hex.EncodeToString(sumaArtefacto[:]) != metricas.ArtifactSHA256 {
		t.Fatalf("hash del artefacto no coincide con metricas")
	}
	if metricas.ModelVersion != ActiveModelVersion || metricas.SchemaVersion != ActiveSchemaVersion {
		t.Fatalf("metricas de otra version: %q/%d", metricas.ModelVersion, metricas.SchemaVersion)
	}
	if metricas.Python != "3.13.17" || metricas.Sklearn != "1.9.1" || metricas.Numpy != "2.5.5" {
		t.Fatalf("versiones reales no reconciliadas: python=%q sklearn=%q numpy=%q",
			metricas.Python, metricas.Sklearn, metricas.Numpy)
	}
	if modelo.K != metricas.SelectedK || modelo.SilhouetteTraining != metricas.SelectedSilhouette {
		t.Fatalf("modelo y metricas divergen")
	}

	datos, err := DatosBytes()
	if err != nil {
		t.Fatal(err)
	}
	sumaDatos := sha256.Sum256(datos)
	if hex.EncodeToString(sumaDatos[:]) != modelo.SyntheticDataSHA256 ||
		modelo.SyntheticDataSHA256 != metricas.SyntheticDataSHA256 ||
		modelo.SyntheticDataSHA256 != syntheticDataSHA {
		t.Fatalf("hash de datos sinteticos divergente")
	}

	referencia, err := ReferenciaBytes()
	if err != nil {
		t.Fatal(err)
	}
	sumaReferencia := sha256.Sum256(referencia)
	if hex.EncodeToString(sumaReferencia[:]) != metricas.ReferenceSHA256 {
		t.Fatalf("hash de la referencia no coincide con metricas")
	}
}

func TestArtefactoV1Conservado(t *testing.T) {
	artefactoV1, err := os.ReadFile("modelo_kmeans_v1.json")
	if err != nil {
		t.Fatalf("v1 ausente: %v", err)
	}
	sumaV1 := sha256.Sum256(artefactoV1)
	if hex.EncodeToString(sumaV1[:]) != v1ArtifactSHA256 {
		t.Fatalf("v1 fue modificado: %x", sumaV1)
	}
	var v1 struct {
		ModelVersion        string `json:"model_version"`
		SyntheticDataSHA256 string `json:"synthetic_data_sha256"`
	}
	if err := json.Unmarshal(artefactoV1, &v1); err != nil {
		t.Fatalf("v1 invalido: %v", err)
	}
	if v1.ModelVersion != v1ArtifactModel || v1.SyntheticDataSHA256 != syntheticDataSHA {
		t.Fatalf("v1 fuera de su version historica: %+v", v1)
	}

	metricasV1, err := os.ReadFile("metricas_v1.json")
	if err != nil {
		t.Fatalf("metricas v1 ausentes: %v", err)
	}
	var m1 struct {
		ArtifactSHA256      string `json:"artifact_sha256"`
		SyntheticDataSHA256 string `json:"synthetic_data_sha256"`
	}
	if err := json.Unmarshal(metricasV1, &m1); err != nil {
		t.Fatalf("metricas v1 invalidas: %v", err)
	}
	if m1.ArtifactSHA256 != v1ArtifactSHA256 || m1.SyntheticDataSHA256 != syntheticDataSHA {
		t.Fatalf("metricas v1 no corresponden al artefacto historico")
	}
}

func TestReferenciaV2EquivalenciaPythonGo(t *testing.T) {
	modelo, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	referencia := cargarReferencia(t)
	if referencia.ModelVersion != ActiveModelVersion || !mismaSecuencia(referencia.FeatureOrder, modelo.FeatureOrder) {
		t.Fatalf("referencia de otra version o distinto orden de features")
	}
	if referencia.Train.DataFile != "datos_sinteticos_v1.json" ||
		referencia.Train.DataSHA256 != modelo.SyntheticDataSHA256 {
		t.Fatalf("referencia de train no apunta a los datos v1")
	}
	if referencia.Median.Definition != MedianDefinitionTrained ||
		math.Abs(referencia.Median.Cents-modelo.TrainedMedianCents) > 1e-9 {
		t.Fatalf("mediana de referencia: %q %g, artefacto %q %g",
			referencia.Median.Definition, referencia.Median.Cents,
			modelo.MedianDefinition, modelo.TrainedMedianCents)
	}

	train := registrosDesdeDatos(t)
	asignacionesTrain, err := modelo.Assign(train)
	if err != nil {
		t.Fatalf("assign train: %v", err)
	}
	for _, muestra := range referencia.Train.Samples {
		if muestra.Index < 0 || muestra.Index >= len(asignacionesTrain) {
			t.Fatalf("indice de referencia fuera de rango: %d", muestra.Index)
		}
		asignacion := asignacionesTrain[muestra.Index]
		if asignacion.Cluster != muestra.ExpectedCluster {
			t.Fatalf("train indice %d: Go=%d Python=%d",
				muestra.Index, asignacion.Cluster, muestra.ExpectedCluster)
		}
		compararFeatures(t, "train", asignacion.Features, muestra.ExpectedFeatures, 1e-9)
	}

	holdout := registrosDesdeJSON(referencia.Holdout.Records)
	asignacionesHoldout, err := modelo.Assign(holdout)
	if err != nil {
		t.Fatalf("assign holdout: %v", err)
	}
	for _, muestra := range referencia.Holdout.Samples {
		if muestra.Index < 0 || muestra.Index >= len(asignacionesHoldout) {
			t.Fatalf("indice holdout fuera de rango: %d", muestra.Index)
		}
		asignacion := asignacionesHoldout[muestra.Index]
		if asignacion.Cluster != muestra.ExpectedCluster {
			t.Fatalf("holdout indice %d: Go=%d Python=%d",
				muestra.Index, asignacion.Cluster, muestra.ExpectedCluster)
		}
		compararFeatures(t, "holdout", asignacion.Features, muestra.ExpectedFeatures, 1e-9)
	}
}

func TestRecorteOutliersAntesDeLogYRatio(t *testing.T) {
	modelo, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	casos := []struct {
		nombre string
		cents  int64
		espera float64
	}{
		{"cero", 0, 0},
		{"negativo", -1, 0},
		{"bajo", 1, modelo.AmountClip.LowerCents},
		{"dentro", 50_000, 50_000},
		{"alto", math.MaxInt64, modelo.AmountClip.UpperCents},
	}
	for _, caso := range casos {
		obtenido, err := modelo.clipAmount(caso.cents)
		if caso.espera == 0 {
			if err == nil {
				t.Fatalf("%s: esperado error", caso.nombre)
			}
			continue
		}
		if err != nil {
			t.Fatalf("%s: %v", caso.nombre, err)
		}
		if math.Abs(obtenido-caso.espera) > 1e-12 {
			t.Fatalf("%s: recorte %g, esperado %g", caso.nombre, obtenido, caso.espera)
		}
	}

	// La mediana es FIJA (`trained_median_cents`); no se recalcula con estos registros.
	registros := []Registro{
		{Category: "comida", AmountCents: 1, Hour: 13, DayOfWeek: 3},
		{Category: "vivienda", AmountCents: math.MaxInt64, Hour: 10, DayOfWeek: 0},
	}
	asignaciones, err := modelo.Assign(registros)
	if err != nil {
		t.Fatal(err)
	}
	esperadoBajoLog := (math.Log1p(modelo.AmountClip.LowerCents) - modelo.LogImporteMean) / modelo.LogImporteStd
	esperadoBajoRatio := modelo.AmountClip.LowerCents / modelo.TrainedMedianCents
	esperadoAltoLog := (math.Log1p(modelo.AmountClip.UpperCents) - modelo.LogImporteMean) / modelo.LogImporteStd
	esperadoAltoRatio := math.Min(modelo.AmountClip.UpperCents/modelo.TrainedMedianCents, 10)
	if math.Abs(asignaciones[0].Features[0]-esperadoBajoLog) > 1e-12 {
		t.Fatalf("log del outlier bajo no usa el recorte: %g vs %g", asignaciones[0].Features[0], esperadoBajoLog)
	}
	if math.Abs(asignaciones[0].Features[len(asignaciones[0].Features)-1]-esperadoBajoRatio) > 1e-12 {
		t.Fatalf("ratio del outlier bajo no usa lower/trained_median: %g vs %g",
			asignaciones[0].Features[len(asignaciones[0].Features)-1], esperadoBajoRatio)
	}
	if math.Abs(asignaciones[1].Features[0]-esperadoAltoLog) > 1e-12 {
		t.Fatalf("log del outlier alto no usa el recorte")
	}
	if math.Abs(asignaciones[1].Features[len(asignaciones[1].Features)-1]-esperadoAltoRatio) > 1e-12 {
		t.Fatalf("ratio del outlier alto no satura a 10: %g vs %g",
			asignaciones[1].Features[len(asignaciones[1].Features)-1], esperadoAltoRatio)
	}
	for i, asignacion := range asignaciones {
		for j, valor := range asignacion.Features {
			if math.IsNaN(valor) || math.IsInf(valor, 0) {
				t.Fatalf("registro %d feature %d no finita", i, j)
			}
		}
	}
}

func TestMedianaFijaYMediaPar(t *testing.T) {
	modelo, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if modelo.MedianDefinition != MedianDefinitionTrained {
		t.Fatalf("definicion de mediana: %q", modelo.MedianDefinition)
	}

	// La mediana del artefacto es la mediana float de los importes YA RECORTADOS
	// del conjunto sintetico. El dataset v1 tiene 500 registros (conteo par) y su
	// mediana es 24353.5: un truncado a entero habria dado 24353 y era el defecto v1.
	train := registrosDesdeDatos(t)
	recortados := make([]float64, len(train))
	for i, registro := range train {
		recortados[i], err = modelo.clipAmount(registro.AmountCents)
		if err != nil {
			t.Fatal(err)
		}
	}
	mediana := medianaFloat(recortados)
	if math.Abs(mediana-modelo.TrainedMedianCents) > 1e-9 {
		t.Fatalf("trained_median_cents=%g no coincide con la mediana recortada del train=%g",
			modelo.TrainedMedianCents, mediana)
	}
	if math.Abs(mediana-24353.5) > 1e-9 {
		t.Fatalf("mediana par truncada: %g, esperada 24353.5", mediana)
	}
	if medianaFloat([]float64{10_002, 10_003}) != 10_002.5 {
		t.Fatalf("medianaFloat no promedia el conteo par")
	}

	// La mediana es FIJA: al multiplicar los importes x4 el ratio se multiplica x4.
	// Si se usara la mediana del titular, el ratio seria identico en ambas escalas.
	base := []Registro{
		{Category: "comida", AmountCents: 10_000, Hour: 13, DayOfWeek: 3},
		{Category: "comida", AmountCents: 20_000, Hour: 13, DayOfWeek: 3},
		{Category: "comida", AmountCents: 30_000, Hour: 13, DayOfWeek: 3},
	}
	escala := []Registro{
		{Category: "comida", AmountCents: 40_000, Hour: 13, DayOfWeek: 3},
		{Category: "comida", AmountCents: 80_000, Hour: 13, DayOfWeek: 3},
		{Category: "comida", AmountCents: 120_000, Hour: 13, DayOfWeek: 3},
	}
	a, err := modelo.Assign(base)
	if err != nil {
		t.Fatal(err)
	}
	b, err := modelo.Assign(escala)
	if err != nil {
		t.Fatal(err)
	}
	for i := range a {
		ratioA := a[i].Features[len(a[i].Features)-1]
		ratioB := b[i].Features[len(b[i].Features)-1]
		if math.Abs(ratioB-4*ratioA) > 1e-12 {
			t.Fatalf("ratio %d no escala x4 con mediana fija: %g vs %g", i, ratioA, ratioB)
		}
	}
	esperadoBase := 10_000.0 / modelo.TrainedMedianCents
	if math.Abs(a[0].Features[len(a[0].Features)-1]-esperadoBase) > 1e-12 {
		t.Fatalf("ratio base=%g, esperado %g",
			a[0].Features[len(a[0].Features)-1], esperadoBase)
	}

	// El parámetro se usa de verdad: duplicarlo reduce el ratio a la mitad.
	copia := copiarArtefacto(modelo)
	copia.TrainedMedianCents = 2 * modelo.TrainedMedianCents
	c, err := copia.Assign(base)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(c[0].Features[len(c[0].Features)-1]-esperadoBase/2) > 1e-12 {
		t.Fatalf("duplicar trained_median_cents no redujo el ratio a la mitad: %g",
			c[0].Features[len(c[0].Features)-1])
	}
}

func TestFeaturesOrdenYDominio(t *testing.T) {
	modelo, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	esperado := nombresFeatures(modelo.Categories)
	if !mismaSecuencia(modelo.FeatureOrder, esperado) {
		t.Fatalf("orden de features: %v", modelo.FeatureOrder)
	}
	registro := Registro{Category: "comida", AmountCents: 18_000, Hour: 13, DayOfWeek: 3, Recurring: true}
	asignaciones, err := modelo.Assign([]Registro{registro})
	if err != nil {
		t.Fatal(err)
	}
	if len(asignaciones) != 1 || len(asignaciones[0].Features) != len(modelo.FeatureOrder) {
		t.Fatalf("dimension de features incorrecta")
	}
	for i, categoria := range modelo.Categories {
		esperado := 0.0
		if categoria == "comida" {
			esperado = 1
		}
		if asignaciones[0].Features[5+i] != esperado {
			t.Fatalf("one-hot de %s = %g", categoria, asignaciones[0].Features[5+i])
		}
	}
	if asignaciones[0].Features[5+len(modelo.Categories)] != 1 {
		t.Fatalf("es_recurrente no esta en la posicion esperada")
	}

	desconocida := registro
	desconocida.Category = "categoria-inexistente"
	otros := registro
	otros.Category = "otros"
	a, err := modelo.Assign([]Registro{desconocida})
	if err != nil {
		t.Fatal(err)
	}
	b, err := modelo.Assign([]Registro{otros})
	if err != nil {
		t.Fatal(err)
	}
	compararFeatures(t, "fallback-otros", a[0].Features, b[0].Features, 0)

	invalidos := []Registro{
		{Category: "comida", AmountCents: 0, Hour: 13, DayOfWeek: 3},
		{Category: "comida", AmountCents: -10, Hour: 13, DayOfWeek: 3},
		{Category: "comida", AmountCents: 100, Hour: 24, DayOfWeek: 3},
		{Category: "comida", AmountCents: 100, Hour: -1, DayOfWeek: 3},
		{Category: "comida", AmountCents: 100, Hour: 13, DayOfWeek: 7},
		{Category: "comida", AmountCents: 100, Hour: 13, DayOfWeek: -1},
	}
	for i, registro := range invalidos {
		if _, err := modelo.Assign([]Registro{registro}); err == nil {
			t.Fatalf("registro invalido %d aceptado", i)
		}
	}
	if asignaciones, err := modelo.Assign(nil); err != nil || len(asignaciones) != 0 {
		t.Fatalf("ausencia de datos: %v / %d", err, len(asignaciones))
	}
	if asignaciones, err := modelo.Assign([]Registro{}); err != nil || len(asignaciones) != 0 {
		t.Fatalf("lista vacia: %v / %d", err, len(asignaciones))
	}
}

func TestEntradasNoFinitasYArtefactoInvalido(t *testing.T) {
	modelo, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	dimension := len(modelo.FeatureOrder)
	conNaN := make([]float64, dimension)
	conNaN[0] = math.NaN()
	if _, err := modelo.NearestCluster(conNaN); err == nil {
		t.Fatal("NaN aceptado en NearestCluster")
	}
	conInf := make([]float64, dimension)
	conInf[0] = math.Inf(1)
	if _, err := modelo.NearestCluster(conInf); err == nil {
		t.Fatal("Inf aceptado en NearestCluster")
	}
	if _, err := modelo.NearestCluster([]float64{1}); err == nil {
		t.Fatal("dimension incorrecta aceptada")
	}

	casos := []struct {
		nombre string
		mutar  func(*Artifact)
	}{
		{"std-nan", func(a *Artifact) { a.LogImporteStd = math.NaN() }},
		{"media-inf", func(a *Artifact) { a.LogImporteMean = math.Inf(1) }},
		{"clip-inf", func(a *Artifact) { a.AmountClip.UpperCents = math.Inf(1) }},
		{"clip-nan", func(a *Artifact) { a.AmountClip.LowerCents = math.NaN() }},
		{"mediana-nan", func(a *Artifact) { a.TrainedMedianCents = math.NaN() }},
		{"centroide-nan", func(a *Artifact) { a.Centroids[0][0] = math.NaN() }},
		{"version", func(a *Artifact) { a.ModelVersion = "kmeans-v1" }},
		{"esquema", func(a *Artifact) { a.SchemaVersion = 1 }},
		{"mediana-def", func(a *Artifact) { a.MedianDefinition = "global" }},
		{"orden", func(a *Artifact) { a.FeatureOrder[0], a.FeatureOrder[1] = a.FeatureOrder[1], a.FeatureOrder[0] }},
	}
	for _, caso := range casos {
		copia := copiarArtefacto(modelo)
		caso.mutar(copia)
		if err := copia.validate(); err == nil {
			t.Fatalf("%s: artefacto invalido aceptado", caso.nombre)
		}
	}

	// Un artefacto con std=0 produce una division no finita que debe fallar cerrada.
	copia := copiarArtefacto(modelo)
	copia.LogImporteStd = 0
	if _, err := copia.Assign([]Registro{{Category: "comida", AmountCents: 100, Hour: 1, DayOfWeek: 1}}); err == nil {
		t.Fatal("std=0 aceptado")
	}
}

func TestDeterminismoV2(t *testing.T) {
	modelo, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	registros := registrosDesdeDatos(t)[:30]
	primera, err := modelo.Assign(registros)
	if err != nil {
		t.Fatal(err)
	}
	segunda, err := modelo.Assign(registros)
	if err != nil {
		t.Fatal(err)
	}
	for i := range primera {
		if primera[i].Cluster != segunda[i].Cluster {
			t.Fatalf("cluster no determinista en %d", i)
		}
		compararFeatures(t, "determinismo", segunda[i].Features, primera[i].Features, 0)
	}

	invertidos := make([]Registro, len(registros))
	for i := range registros {
		invertidos[len(registros)-1-i] = registros[i]
	}
	tercera, err := modelo.Assign(invertidos)
	if err != nil {
		t.Fatal(err)
	}
	for i := range registros {
		if tercera[len(registros)-1-i].Cluster != primera[i].Cluster {
			t.Fatalf("el orden altero el cluster en %d", i)
		}
	}
}

func TestUmbralEstructura(t *testing.T) {
	modelo, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	justo := copiarArtefacto(modelo)
	justo.SilhouetteTraining = 0.35
	if !justo.TieneEstructura() {
		t.Fatal("0.35 debe aceptarse")
	}
	debajo := copiarArtefacto(modelo)
	debajo.SilhouetteTraining = 0.349999
	if debajo.TieneEstructura() {
		t.Fatal("0.349999 debe rechazarse")
	}
	var nulo *Artifact
	if nulo.TieneEstructura() {
		t.Fatal("artefacto nil no tiene estructura")
	}
}

func TestEvaluacionHoldoutV2(t *testing.T) {
	metricas := cargarMetricas(t)
	h := metricas.Holdout
	if h.Samples <= 0 {
		t.Fatalf("holdout invalido: %+v", h)
	}
	if h.Seed == 20261116 {
		t.Fatalf("holdout con la semilla de entrenamiento")
	}
	for nombre, valor := range map[string]float64{
		"silhouette":         h.Silhouette,
		"purity_profile":     h.PurityProfile,
		"purity_scale_group": h.PurityScaleGroup,
	} {
		if math.IsNaN(valor) || math.IsInf(valor, 0) || valor < 0 || valor > 1 {
			t.Fatalf("%s fuera de rango: %g", nombre, valor)
		}
	}
	if len(h.ClusterSizes) != 2 {
		t.Fatalf("tamanos de cluster inesperados: %v", h.ClusterSizes)
	}
	if !strings.Contains(strings.ToLower(h.Note), "no son precision predictiva") {
		t.Fatalf("la nota debe aclarar que no es precision predictiva: %q", h.Note)
	}
	t.Logf("HOLDOUT_V2 samples=%d silhouette=%.6f purity_profile=%.6f purity_scale_group=%.6f clusters=%v",
		h.Samples, h.Silhouette, h.PurityProfile, h.PurityScaleGroup, h.ClusterSizes)
}

func BenchmarkAssign500V2(b *testing.B) {
	modelo, err := Load()
	if err != nil {
		b.Fatal(err)
	}
	registros := registrosDesdeDatos(b)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := modelo.Assign(registros); err != nil {
			b.Fatal(err)
		}
	}
}
