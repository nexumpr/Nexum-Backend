#!/usr/bin/env python3
"""ML-155: entrena el K-Means de E055 con datos SINTETICOS y exporta un artefacto JSON.

No usa datos personales, no ejecuta AWS y no genera pickle. La inferencia en produccion
es Go puro: este script solo se ejecuta en local/CI para regenerar y verificar el artefacto.

Reglas de la version 2 (alineadas con ML_E055_CONTRATO_Y_DISENO.md):
  * Recorte de outliers global fijado en entrenamiento: percentiles 0,5 y 99,5 del
    conjunto sintetico. Los dos limites se guardan en el artefacto y la inferencia Go
    recorta cada importe con ellos ANTES de log1p y del ratio. La regla es global
    (no por usuario) para que train y serve apliquen exactamente la misma transformacion.
  * Mediana del ratio: la mediana FIJA calculada en entrenamiento sobre los importes
    YA RECORTADOS del conjunto sintetico (`trained_median_cents`). Es un parametro
    del artefacto y la inferencia usa exactamente ese valor, como exige el paso 2
    de la seccion 7 del diseno ("los mismos parametros del artefacto"). Usar la
    mediana de cada peticion cambiaria el espacio de features y descolocaria los
    centroides (skew train/serve); por eso NO se usa la mediana del titular.
  * La mediana se calcula como float (promedio de los dos centrales si el conteo es par),
    igual que numpy.median; nunca se trunca a entero.
  * Ventana de servicio: 90 dias moviles (o `period` explicito). No se excluye el
    trimestre en curso porque esa linea del diseno contradice la ventana de 90 dias y
    no forma parte del contrato aprobado; queda registrada para enmienda del diseno.
  * Cache Redis de 30 s: no se implementa en esta correccion. Requiere composicion,
    aislamiento por usuario e invalidacion al escribir movimientos fuera del alcance
    autorizado; se propone enmienda. El endpoint no cachea la respuesta.

Uso (entorno con Python 3.13 + scikit-learn, p. ej. dist/venv-ml):
    python ml/entrenar_kmeans.py

Salidas (v2; v1 se conserva intacta como historico):
    ml/modelo_kmeans_v2.json              centroides + recorte + normalizacion + hash
    ml/metricas_v2.json                   silueta por k, holdout, versiones reales
    ml/referencia_inferencia_v2.json      asignaciones y features de train y holdout
    ml/datos_sinteticos_v1.json           datos sinteticos (no cambian; se verifica hash)
"""
from __future__ import annotations

import hashlib
import json
import math
import platform
from collections import Counter, defaultdict
from pathlib import Path

import numpy as np
import sklearn
from sklearn.cluster import KMeans
from sklearn.metrics import silhouette_score

ROOT = Path(__file__).resolve().parents[1]
OUT = ROOT / "ml"
SEED = 20261116
HOLDOUT_SEED = 20261117
N_INIT = 10
SCHEMA_VERSION = 2
MODEL_VERSION = "kmeans-v2"
MEDIAN_DEFINITION = "trained_median_of_clipped_amounts"
LOWER_PERCENTILE = 0.5
UPPER_PERCENTILE = 99.5
MIN_SILHOUETTE = 0.35
N_POR_PERFIL = 125
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
    ("comida", 18_000, 4_000, 13.0, 1.5, 3.5, 1.6, 0.10),
    ("transporte", 8_000, 2_000, 8.0, 1.0, 2.0, 1.4, 0.60),
    ("vivienda", 280_000, 35_000, 10.0, 0.8, 1.0, 0.5, 0.95),
    ("servicios", 45_000, 10_000, 19.5, 1.3, 5.5, 0.8, 0.70),
]


def sha256_bytes(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def json_bytes(obj: object) -> bytes:
    return (json.dumps(obj, ensure_ascii=False, indent=2, sort_keys=True) + "\n").encode("utf-8")


def generar_datos(seed: int) -> tuple[list[dict[str, object]], list[str]]:
    """Genera el conjunto sintetico y devuelve registros y perfil de origen.

    Los registros NO incluyen el perfil: se conserva exactamente el mismo formato y
    orden que v1. La etiqueta de perfil solo se usa para la evaluacion holdout y para
    el muestreo de la referencia; nunca entra en las features.
    """
    rng = np.random.default_rng(seed)
    con_perfil: list[dict[str, object]] = []
    for categoria, media, desv, hora, desv_hora, dia, desv_dia, p_rec in PERFILES:
        for _ in range(N_POR_PERFIL):
            cents = int(max(1.0, rng.normal(media, desv)))
            hora_mov = int(max(0, min(23, round(rng.normal(hora, desv_hora)))))
            dia_sem = int(max(0, min(6, round(rng.normal(dia, desv_dia)))))
            con_perfil.append({
                "category": categoria,
                "amount_cents": cents,
                "hour": hora_mov,
                "day_of_week": dia_sem,
                "recurring": bool(rng.random() < p_rec),
                "_profile": categoria,
            })
    rng.shuffle(con_perfil)
    perfiles = [str(r["_profile"]) for r in con_perfil]
    registros = [{k: v for k, v in r.items() if k != "_profile"} for r in con_perfil]
    return registros, perfiles


def recorte(importes: np.ndarray) -> tuple[float, float]:
    """Percentiles 0,5 y 99,5 del conjunto de entrenamiento (regla global)."""
    bajo, alto = np.percentile(importes, [LOWER_PERCENTILE, UPPER_PERCENTILE], method="linear")
    bajo, alto = float(bajo), float(alto)
    if not math.isfinite(bajo) or not math.isfinite(alto) or bajo <= 0 or alto <= bajo:
        raise RuntimeError(f"recorte degenerado: bajo={bajo} alto={alto}")
    return bajo, alto


def importes_recortados(registros: list[dict[str, object]], bajo: float, alto: float) -> np.ndarray:
    importes = np.array([float(r["amount_cents"]) for r in registros], dtype=float)
    return np.clip(importes, bajo, alto)


def mediana_float(valores: np.ndarray) -> float:
    """Mediana float (promedio de centrales si n es par), igual que numpy.median."""
    return float(np.median(valores))


def matriz_features(
    registros: list[dict[str, object]],
    importes: np.ndarray,
    media_log: float,
    desv_log: float,
    mediana: float,
) -> np.ndarray:
    """Features en el orden del artefacto. La mediana es el parametro FIJO del artefacto."""
    if not math.isfinite(mediana) or mediana <= 0:
        raise RuntimeError("mediana no positiva")
    filas = []
    for r, importe in zip(registros, importes):
        hora = 2.0 * math.pi * float(r["hour"]) / 24.0
        dia = 2.0 * math.pi * float(r["day_of_week"]) / 7.0
        onehot = [1.0 if r["category"] == c else 0.0 for c in CATEGORIES]
        filas.append([
            (math.log1p(float(importe)) - media_log) / desv_log,
            math.sin(hora),
            math.cos(hora),
            math.sin(dia),
            math.cos(dia),
            *onehot,
            1.0 if r["recurring"] else 0.0,
            min(float(importe) / mediana, 10.0),
        ])
    X = np.array(filas, dtype=float)
    if not np.isfinite(X).all():
        raise RuntimeError("features no finitas")
    return X


def muestras_referencia(
    X: np.ndarray,
    labels: np.ndarray,
    importes: np.ndarray,
    registros: list[dict[str, object]],
    n: int = 12,
) -> list[dict[str, object]]:
    """Selecciona n indices incluyendo los extremos que activan el recorte."""
    orden = np.argsort(importes)
    indices = {int(i) for i in orden[:3]} | {int(i) for i in orden[-3:]}
    for i in range(len(registros)):
        if len(indices) >= n:
            break
        indices.add(i)
    seleccion = sorted(indices)[:n]
    return [{
        "index": int(i),
        "record": registros[i],
        "expected_cluster": int(labels[i]),
        "expected_features": [round(float(v), 12) for v in X[i]],
    } for i in seleccion]


def pureza(labels_cluster: np.ndarray, labels_perfil: list[str], grupos: dict[str, str] | None = None) -> float:
    """Mayoria por cluster frente a una etiqueta externa (metrica descriptiva, no accuracy)."""
    if grupos is not None:
        etiquetas = [grupos[p] for p in labels_perfil]
    else:
        etiquetas = labels_perfil
    por_cluster: dict[int, list[str]] = defaultdict(list)
    for cluster, etiqueta in zip(labels_cluster, etiquetas):
        por_cluster[int(cluster)].append(etiqueta)
    aciertos = 0
    for etiquetas_cluster in por_cluster.values():
        aciertos += Counter(etiquetas_cluster).most_common(1)[0][1]
    return aciertos / len(etiquetas)


def main() -> None:
    datos, _ = generar_datos(SEED)
    datos_bytes = json_bytes(datos)
    datos_sha = sha256_bytes(datos_bytes)
    ruta_datos = OUT / "datos_sinteticos_v1.json"
    if ruta_datos.exists() and ruta_datos.read_bytes() != datos_bytes:
        raise RuntimeError("datos_sinteticos_v1.json cambiaria; se conserva la version historica")
    ruta_datos.write_bytes(datos_bytes)

    importes_raw = np.array([float(r["amount_cents"]) for r in datos], dtype=float)
    bajo, alto = recorte(importes_raw)
    importes = importes_recortados(datos, bajo, alto)
    log_importe = np.log1p(importes)
    media_log = float(log_importe.mean())
    desv_log = float(log_importe.std(ddof=0))
    if not math.isfinite(media_log) or not math.isfinite(desv_log) or desv_log <= 0:
        raise RuntimeError("normalizacion log degenerada")
    mediana_entrenamiento = mediana_float(importes)
    X = matriz_features(datos, importes, media_log, desv_log, mediana_entrenamiento)

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
    if silueta < MIN_SILHOUETTE:
        raise RuntimeError(f"silueta {silueta:.4f} < {MIN_SILHOUETTE}; el diseno exige rechazar el modelo")

    artefacto = {
        "schema_version": SCHEMA_VERSION,
        "model_version": MODEL_VERSION,
        "algorithm": "KMeans",
        "seed": SEED,
        "n_init": N_INIT,
        "k": mejor_k,
        "feature_order": FEATURE_ORDER,
        "categories": CATEGORIES,
        "amount_clip": {
            "lower_percentile": LOWER_PERCENTILE,
            "upper_percentile": UPPER_PERCENTILE,
            "lower_cents": round(bajo, 6),
            "upper_cents": round(alto, 6),
        },
        "median_definition": MEDIAN_DEFINITION,
        "log_importe_mean": round(media_log, 12),
        "log_importe_std": round(desv_log, 12),
        "trained_median_cents": round(mediana_entrenamiento, 6),
        "centroids": [[round(float(v), 12) for v in fila] for fila in mejor.cluster_centers_],
        "silhouette_training": round(silueta, 6),
        "inertia": round(float(mejor.inertia_), 6),
        "synthetic_data_sha256": datos_sha,
        "generated_by": "ml/entrenar_kmeans.py",
    }
    artefacto_bytes = json_bytes(artefacto)
    (OUT / "modelo_kmeans_v2.json").write_bytes(artefacto_bytes)

    # Referencia de train: 12 casos, incluidos los extremos del recorte.
    labels_train = mejor.predict(X)
    referencia_train = muestras_referencia(X, labels_train, importes, datos)

    # Evaluacion holdout: datos generados con otra semilla, nunca usados en fit.
    holdout, perfiles_holdout = generar_datos(HOLDOUT_SEED)
    importes_holdout = importes_recortados(holdout, bajo, alto)
    # Misma mediana FIJA del entrenamiento: no se recalcula por peticion.
    X_holdout = matriz_features(holdout, importes_holdout, media_log, desv_log, mediana_entrenamiento)
    labels_holdout = mejor.predict(X_holdout)
    silueta_holdout = float(silhouette_score(X_holdout, labels_holdout))
    inercia_holdout = float(((X_holdout - mejor.cluster_centers_[labels_holdout]) ** 2).sum())
    # Agrupacion descriptiva por magnitud de los perfiles sinteticos: los tres perfiles
    # de gasto corriente (8k-45k) frente al de vivienda (280k). No es una etiqueta real
    # ni una medida de precision: solo ayuda a describir el merge que hace k=2.
    grupos_escala = {"comida": "bajo", "transporte": "bajo", "servicios": "bajo", "vivienda": "alto"}
    pureza_perfil = pureza(labels_holdout, perfiles_holdout)
    pureza_escala = pureza(labels_holdout, perfiles_holdout, grupos_escala)
    referencia_holdout = muestras_referencia(X_holdout, labels_holdout, importes_holdout, holdout)

    referencia = {
        "schema_version": SCHEMA_VERSION,
        "model_version": MODEL_VERSION,
        "feature_order": FEATURE_ORDER,
        "median": {
            "definition": MEDIAN_DEFINITION,
            "cents": round(mediana_entrenamiento, 6),
        },
        "train": {
            "data_file": "datos_sinteticos_v1.json",
            "data_sha256": datos_sha,
            "samples": referencia_train,
        },
        "holdout": {
            "seed": HOLDOUT_SEED,
            "records": holdout,
            "samples": referencia_holdout,
        },
    }
    referencia_bytes = json_bytes(referencia)
    (OUT / "referencia_inferencia_v2.json").write_bytes(referencia_bytes)

    metricas = {
        "model_version": MODEL_VERSION,
        "schema_version": SCHEMA_VERSION,
        "python": platform.python_version(),
        "python_implementation": platform.python_implementation(),
        "sklearn": sklearn.__version__,
        "numpy": np.__version__,
        "seed": SEED,
        "n_init": N_INIT,
        "samples": len(datos),
        "results": resultados,
        "selected_k": mejor_k,
        "selected_silhouette": round(silueta, 6),
        "amount_clip": artefacto["amount_clip"],
        "median_definition": MEDIAN_DEFINITION,
        "trained_median_cents": round(mediana_entrenamiento, 6),
        "synthetic_data_sha256": datos_sha,
        "artifact_sha256": sha256_bytes(artefacto_bytes),
        "reference_sha256": sha256_bytes(referencia_bytes),
        "holdout": {
            "seed": HOLDOUT_SEED,
            "samples": len(holdout),
            "silhouette": round(silueta_holdout, 6),
            "inertia": round(inercia_holdout, 6),
            "purity_profile": round(pureza_perfil, 6),
            "purity_scale_group": round(pureza_escala, 6),
            "scale_groups": {"bajo": ["comida", "transporte", "servicios"], "alto": ["vivienda"]},
            "cluster_sizes": [int(np.sum(labels_holdout == i)) for i in range(mejor_k)],
            "note": "silhouette y pureza son metricas descriptivas sobre datos sinteticos; no son precision predictiva",
        },
    }
    (OUT / "metricas_v2.json").write_bytes(json_bytes(metricas))

    print(f"ML155_V2_ARTIFACT_OK k={mejor_k} silhouette={silueta:.6f} samples={len(datos)}")
    print(f"ML155_V2_ARTIFACT_SHA256 {metricas['artifact_sha256']}")
    print(f"ML155_V2_REFERENCE_SHA256 {metricas['reference_sha256']}")
    print(
        "ML155_V2_HOLDOUT "
        f"silhouette={silueta_holdout:.6f} "
        f"purity_profile={pureza_perfil:.6f} "
        f"purity_scale_group={pureza_escala:.6f}"
    )
    print(f"ML155_V2_ENV python={platform.python_version()} sklearn={sklearn.__version__} numpy={np.__version__}")


if __name__ == "__main__":
    main()
