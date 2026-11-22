#!/usr/bin/env python3
"""ML-155: entrena el K-Means de E055 con datos SINTETICOS y exporta un artefacto JSON.

No usa datos personales, no ejecuta AWS y no genera pickle. La inferencia en produccion
es Go puro: este script solo se ejecuta en local/CI para regenerar y verificar el artefacto.

Uso:
    py -3 ml/entrenar_kmeans.py

Salidas:
    ml/datos_sinteticos_v1.json   conjunto sintetico (entrada reproducible)
    ml/modelo_kmeans_v1.json      centroides + normalizacion + hash de los datos
    ml/metricas_v1.json           silueta por k, k elegido, inercia y hash del artefacto
"""
from __future__ import annotations

import hashlib
import json
import math
from pathlib import Path

import numpy as np
from sklearn.cluster import KMeans
from sklearn.metrics import silhouette_score

ROOT = Path(__file__).resolve().parents[1]
OUT = ROOT / "ml"
SEED = 20261116
MODEL_VERSION = "kmeans-v1"
N_INIT = 10
CATEGORIES = ["ahorro", "comida", "ingreso", "otros", "servicios", "transporte", "vivienda"]
FEATURE_ORDER = [
    "log_importe_z",
    "hora_sin",
    "hora_cos",
    "dia_sin",
    "dia_cos",
    *[f"categoria_{c}" for c in CATEGORIES],
    "es_recurrente",
    "ratio_sobre_mediana",
]

# Perfiles sinteticos: importe en centavos, hora, dia de la semana y recurrencia.
PERFILES = [
    ("comida", 18_000, 4_000, 13.0, 1.5, 3.5, 1.6, 0.10, 125),
    ("transporte", 8_000, 2_000, 8.0, 1.0, 2.0, 1.4, 0.60, 125),
    ("vivienda", 280_000, 35_000, 10.0, 0.8, 1.0, 0.5, 0.95, 125),
    ("servicios", 45_000, 10_000, 19.5, 1.3, 5.5, 0.8, 0.70, 125),
]


def sha256_bytes(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def json_bytes(obj: object) -> bytes:
    return (json.dumps(obj, ensure_ascii=False, indent=2, sort_keys=True) + "\n").encode("utf-8")


def generar_datos() -> list[dict[str, object]]:
    rng = np.random.default_rng(SEED)
    registros: list[dict[str, object]] = []
    for categoria, media, desv, hora, desv_hora, dia, desv_dia, p_rec, n in PERFILES:
        for _ in range(n):
            cents = int(max(1.0, rng.normal(media, desv)))
            hora_mov = int(max(0, min(23, round(rng.normal(hora, desv_hora)))))
            dia_sem = int(max(0, min(6, round(rng.normal(dia, desv_dia)))))
            registros.append({
                "category": categoria,
                "amount_cents": cents,
                "hour": hora_mov,
                "day_of_week": dia_sem,
                "recurring": bool(rng.random() < p_rec),
            })
    rng.shuffle(registros)
    return registros


def features(registros: list[dict[str, object]]) -> tuple[np.ndarray, float, float, float]:
    importes = np.array([float(r["amount_cents"]) for r in registros], dtype=float)
    mediana = float(np.median(importes))
    log_importe = np.log1p(importes)
    media_log = float(log_importe.mean())
    desv_log = float(log_importe.std(ddof=0))
    if desv_log <= 0:
        raise RuntimeError("desviacion de log_importe degenerada")
    filas = []
    for r in registros:
        hora = 2.0 * math.pi * float(r["hour"]) / 24.0
        dia = 2.0 * math.pi * float(r["day_of_week"]) / 7.0
        onehot = [1.0 if r["category"] == c else 0.0 for c in CATEGORIES]
        fila = [
            (math.log1p(float(r["amount_cents"])) - media_log) / desv_log,
            math.sin(hora),
            math.cos(hora),
            math.sin(dia),
            math.cos(dia),
            *onehot,
            1.0 if r["recurring"] else 0.0,
            min(float(r["amount_cents"]) / mediana, 10.0),
        ]
        filas.append(fila)
    return np.array(filas, dtype=float), media_log, desv_log, mediana


def main() -> None:
    datos = generar_datos()
    datos_bytes = json_bytes(datos)
    datos_sha = sha256_bytes(datos_bytes)
    (OUT / "datos_sinteticos_v1.json").write_bytes(datos_bytes)

    X, media_log, desv_log, mediana = features(datos)
    resultados = []
    modelos = {}
    for k in range(2, 7):
        modelo = KMeans(n_clusters=k, n_init=N_INIT, random_state=SEED)
        etiquetas = modelo.fit_predict(X)
        silueta = float(silhouette_score(X, etiquetas))
        resultados.append({"k": k, "silueta": round(silueta, 6), "inercia": round(float(modelo.inertia_), 6)})
        modelos[k] = (modelo, silueta)

    # Empate => menor k. Orden estable por (-silueta, k).
    mejor_k = min(resultados, key=lambda r: (-r["silueta"], r["k"]))["k"]
    mejor, silueta = modelos[mejor_k]
    if silueta < 0.35:
        raise RuntimeError(f"silueta {silueta:.4f} < 0.35; el diseno exige rechazar el modelo")

    artefacto = {
        "schema_version": 1,
        "model_version": MODEL_VERSION,
        "algorithm": "KMeans",
        "seed": SEED,
        "n_init": N_INIT,
        "k": mejor_k,
        "feature_order": FEATURE_ORDER,
        "categories": CATEGORIES,
        "log_importe_mean": round(media_log, 12),
        "log_importe_std": round(desv_log, 12),
        "trained_median_cents": round(mediana, 6),
        "centroids": [[round(float(v), 12) for v in fila] for fila in mejor.cluster_centers_],
        "silhouette_training": round(silueta, 6),
        "inertia": round(float(mejor.inertia_), 6),
        "synthetic_data_sha256": datos_sha,
        "generated_by": "ml/entrenar_kmeans.py",
    }
    artefacto_bytes = json_bytes(artefacto)
    (OUT / "modelo_kmeans_v1.json").write_bytes(artefacto_bytes)

    metricas = {
        "model_version": MODEL_VERSION,
        "python": "3.14",
        "sklearn": "1.9.0",
        "seed": SEED,
        "n_init": N_INIT,
        "samples": len(datos),
        "results": resultados,
        "selected_k": mejor_k,
        "selected_silhouette": round(silueta, 6),
        "synthetic_data_sha256": datos_sha,
        "artifact_sha256": sha256_bytes(artefacto_bytes),
    }
    (OUT / "metricas_v1.json").write_bytes(json_bytes(metricas))

    # Referencia fija para verificar que la inferencia Go reproduce la asignación
    # del pipeline de Python. No forma parte del artefacto ni altera su hash.
    distancias = ((X[:, None, :] - mejor.cluster_centers_[None, :, :]) ** 2).sum(axis=2)
    asignaciones = distancias.argmin(axis=1)
    referencia = [{
        "record": registro,
        "expected_cluster": int(asignaciones[i]),
    } for i, registro in enumerate(datos[:12])]
    (OUT / "referencia_inferencia_v1.json").write_bytes(json_bytes(referencia))

    print(f"ML155_ARTIFACT_OK k={mejor_k} silhouette={silueta:.6f} samples={len(datos)}")
    print(f"ML155_ARTIFACT_SHA256 {metricas['artifact_sha256']}")
    print(f"ML155_REFERENCE_OK records={len(referencia)}")


if __name__ == "__main__":
    main()
