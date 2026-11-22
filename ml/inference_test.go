package ml

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"
)

func TestArtefactoVerificado(t *testing.T) {
	modelo, err := Load()
	if err != nil {
		t.Fatalf("cargar artefacto: %v", err)
	}
	if modelo.Version() != "kmeans-v1" || modelo.K < 2 || modelo.K > 6 || !modelo.TieneEstructura() {
		t.Fatalf("artefacto fuera de ML-155: version=%q k=%d silueta=%g", modelo.Version(), modelo.K, modelo.SilhouetteTraining)
	}
	if modelo.Seed != 20261116 || modelo.NInit != 10 {
		t.Fatalf("semilla/n_init incorrectos: %d/%d", modelo.Seed, modelo.NInit)
	}

	bytesArtefacto, err := ArtefactoBytes()
	if err != nil {
		t.Fatal(err)
	}
	suma := sha256.Sum256(bytesArtefacto)
	bytesMetricas, err := MetricasBytes()
	if err != nil {
		t.Fatal(err)
	}
	var metricas struct {
		ArtifactSHA256      string  `json:"artifact_sha256"`
		SelectedK           int     `json:"selected_k"`
		SelectedSilhouette  float64 `json:"selected_silhouette"`
		SyntheticDataSHA256 string  `json:"synthetic_data_sha256"`
	}
	if err := json.Unmarshal(bytesMetricas, &metricas); err != nil {
		t.Fatalf("metricas invalidas: %v", err)
	}
	if hex.EncodeToString(suma[:]) != metricas.ArtifactSHA256 {
		t.Fatalf("hash del artefacto no coincide con metricas: %x", suma)
	}
	if modelo.K != metricas.SelectedK || modelo.SilhouetteTraining != metricas.SelectedSilhouette {
		t.Fatalf("modelo y metricas divergen: k=%d/%d silueta=%g/%g",
			modelo.K, metricas.SelectedK, modelo.SilhouetteTraining, metricas.SelectedSilhouette)
	}
	if modelo.SyntheticDataSHA256 != metricas.SyntheticDataSHA256 {
		t.Fatal("hash de datos sinteticos divergente")
	}
}

func TestInferenciaGoReproduceReferenciaPython(t *testing.T) {
	modelo, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	crudo, err := ReferenciaBytes()
	if err != nil {
		t.Fatal(err)
	}
	var referencia []struct {
		Record struct {
			Category    string `json:"category"`
			AmountCents int64  `json:"amount_cents"`
			Hour        int    `json:"hour"`
			DayOfWeek   int    `json:"day_of_week"`
			Recurring   bool   `json:"recurring"`
		} `json:"record"`
		ExpectedCluster int `json:"expected_cluster"`
	}
	if err := json.Unmarshal(crudo, &referencia); err != nil {
		t.Fatalf("referencia invalida: %v", err)
	}
	if len(referencia) == 0 {
		t.Fatal("referencia vacia")
	}
	for i, caso := range referencia {
		registro := Registro{
			Category:    caso.Record.Category,
			AmountCents: caso.Record.AmountCents,
			Hour:        caso.Record.Hour,
			DayOfWeek:   caso.Record.DayOfWeek,
			Recurring:   caso.Record.Recurring,
		}
		vector, err := modelo.Features(registro, modelo.TrainedMedianCents)
		if err != nil {
			t.Fatalf("caso %d: features: %v", i, err)
		}
		cluster, err := modelo.NearestCluster(vector)
		if err != nil {
			t.Fatalf("caso %d: cluster: %v", i, err)
		}
		if cluster != caso.ExpectedCluster {
			t.Fatalf("caso %d: Go=%d Python=%d", i, cluster, caso.ExpectedCluster)
		}
	}
}

func TestAsignacionDeterminista(t *testing.T) {
	modelo, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	registros := []Registro{
		{Category: "comida", AmountCents: 18000, Hour: 13, DayOfWeek: 3, Recurring: false},
		{Category: "comida", AmountCents: 22000, Hour: 14, DayOfWeek: 4, Recurring: true},
		{Category: "transporte", AmountCents: 8000, Hour: 8, DayOfWeek: 1, Recurring: true},
		{Category: "vivienda", AmountCents: 280000, Hour: 10, DayOfWeek: 0, Recurring: true},
	}
	primera, err := modelo.Assign(registros, 24353.5)
	if err != nil {
		t.Fatal(err)
	}
	segunda, err := modelo.Assign(registros, 24353.5)
	if err != nil {
		t.Fatal(err)
	}
	for i := range primera {
		if primera[i].Cluster != segunda[i].Cluster {
			t.Fatalf("cluster no determinista en %d", i)
		}
	}
}
