# ML-155 — Contrato y diseño del módulo K-Means para E055

| Campo | Valor |
|---|---|
| Documento | `sdd/_control/H08/ML_E055_CONTRATO_Y_DISENO.md` |
| Endpoint consumidor | `GET /api/v1/analytics/recommendations` (`nexum_e055`) |
| Algoritmo | **K-Means** (aprendizaje no supervisado, agrupamiento) |
| Estado del diseño | **APROBADO (2026-11-25)** — ver §14 «Decisión ratificada 2026-11-25» |
| `ML_DESIGNED` | **APPROVED** (`ML_DESIGN_APPROVED=YES`, 2026-11-25) |
| `ML_IMPLEMENTED` | `CODE_COMPLETE_LOCAL` (v2, 2026-11-22); AWS `NOT_RUN` |
| `ML_DEPLOYED` | `NOT_RUN` |
| AWS / Terraform | `NOT_RUN` (ninguna mutación; ninguna consulta) |

> Este documento **define** el módulo. No implementa, no entrena con datos reales y no
> despliega nada.

---

## 1. Contrato vigente (hecho, no propuesta)

```
GET /api/v1/analytics/recommendations
  query: period (opcional)
  200:
    items[]: { id, title, reason, disclaimer, modelVersion }
    nextCursor: string
```

El contrato **ya incluye `modelVersion`** y **ya es paginado** (`items[]` + `nextCursor`).
La auditoría propone `recommendations[]` + `hasEnoughData` + `explicacion`/`evidencia`.

## 2. Decisión contractual propuesta (RFC-016 §6)

| Elemento | Decisión | Clase |
|---|---|---|
| Contenedor | **Conservar `items[]`** + `nextCursor` | COMPATIBILIDAD (no renombrar) |
| `hasEnoughData` (boolean, top-level, requerido) | **Añadir** | ESENCIAL |
| `items[].category` (string, opcional) | Pospuesto | OPCIONAL |
| `items[].evidence` (objeto, opcional) | Pospuesto | OPCIONAL |
| `items[].confidence` (number 0..1, opcional) | Pospuesto | OPCIONAL |
| `recommendations[]` como nombre de contenedor | **Rechazado** | — |

**Por qué `hasEnoughData` es esencial:** `items: []` es ambiguo. Sin la marca, el cliente no
puede distinguir «este usuario no tiene historial suficiente» (estado vacío legítimo, mensaje de
onboarding) de «hay historial pero ningún patrón supera el umbral» (mensaje distinto). El diseño
ML de §5 exige esa distinción. Es el **único** campo ESENCIAL de las 812 diferencias clasificadas.

## 3. Qué hace y qué NO hace K-Means

**Hace:** agrupa las transacciones de un usuario en *k* grupos por similitud de rasgos
(importe relativo, momento del día, día de la semana, categoría, recurrencia). Los centroides
describen **patrones observados**.

**NO hace:** no genera consejos financieros, no predice ahorros, no garantiza resultados.
La recomendación se construye en una **capa de interpretación determinista** sobre centroides y
estadísticas verificables (§7). Prohibido inventar porcentajes o ahorros.

## 4. Features (por transacción del usuario)

| # | Feature | Cálculo | Normalización |
|---|---|---|---|
| f1 | `log_importe` | `ln(1 + importe_centavos)` | z-score |
| f2 | `hora_sin` / `hora_cos` | seno y coseno de `2π·hora/24` | ya en [-1,1] |
| f3 | `dia_sin` / `dia_cos` | seno y coseno de `2π·dia_semana/7` | ya en [-1,1] |
| f4 | `categoria` | one-hot de las categorías del catálogo (fijas, ~10) | 0/1 |
| f5 | `es_recurrente` | 1 si `recurringPaymentId` presente | 0/1 |
| f6 | `ratio_sobre_mediana` | `importe / mediana(importes del usuario)` | acotado a [0,10] |

Sin PII: **ninguna** feature usa correo, nombre, `userId` ni texto libre. Se usa únicamente
importe, marca temporal, categoría y recurrencia.

## 5. Ventana, preparación y suficiencia

| Parámetro | Valor propuesto |
|---|---|
| Ventana por defecto | últimos **90 días** (`period` puede acotarla) |
| Mínimo de transacciones | **12** |
| Mínimo de categorías distintas | **2** |
| Datos insuficientes | `hasEnoughData=false`, `items: []`, `200` (no es error) |
| Sesgo | se excluyen el trimestre en curso si está incompleto, salvo `period` explícito |
| Outliers | recorte a percentil 0,5–99,5 antes de normalizar |

## 6. Entrenamiento y selección de *k*

| Aspecto | Decisión |
|---|---|
| Entorno | **Python 3.13 + scikit-learn**, fuera de AWS, reproducible en local/CI |
| Datos de entrenamiento | **sintéticos** generados por el propio pipeline (nunca datos personales productivos sin autorización expresa) |
| *k* | se evalúa `k ∈ {2..6}`; se elige por **coeficiente de silueta** máximo; empate → menor *k* |
| Semilla | `random_state = 20261116` (fija) |
| `n_init` | 10 |
| Criterio de rechazo | si silueta < **0,35** → `hasEnoughData=false` (no hay estructura) |
| Artefacto | `modelo_kmeans_v<N>.json` (centroides + escala + version + hash) |
| Versionado | `modelVersion` en la respuesta = versión del artefacto (**ya existe en el contrato**) |

El artefacto es **JSON plano** (centroides, medias, desviaciones, etiquetas de categoría), no
pickle: se puede revisar, versionar y cargar desde Go sin ejecutar Python en producción.

## 7. Inferencia en Go y explicabilidad

1. El backend lee transacciones del usuario con `Query` sobre `USER#<id>` (**sin `Scan`**).
2. Calcula las features de §4 con los mismos parámetros del artefacto.
3. Asigna cada transacción al centroide más cercano (distancia euclídea en el espacio normalizado).
4. Genera **una recomendación por cluster con masa suficiente** (≥ 20 % de transacciones y ≥ 3 transacciones).
5. `reason` se compone de hechos verificables, no de estimaciones:
   - «N movimientos de la categoría C, mediana X, concentrados en la franja HH–HH»
   - `evidence` (si se aprueba) expone esos números para que el cliente los muestre.
6. `disclaimer` es **obligatorio** en cada ítem y no puede omitirse por configuración.

**Prohibido en `reason`/`title`:** porcentajes de ahorro, promesas de resultado, comparaciones
con otros usuarios, cualquier cifra no calculada en el paso 5.

## 8. Determinismo y latencia

| Requisito | Valor |
|---|---|
| Determinismo | mismo artefacto + mismas transacciones ⇒ misma salida (asignación por distancia, desempate por índice de cluster) |
| Latencia objetivo | p95 ≤ 300 ms con 500 transacciones |
| Cómputo | O(n·k) en memoria del request; sin servicio externo |
| Caché | Redis con TTL 30 s y clave por usuario (A-080), invalidada al escribir movimientos |
| Carga | máx. 500 transacciones por cálculo; por encima se muestrea la ventana (documentado en `reason`) |

## 9. Privacidad y multiusuario

- Cálculo **por usuario**, sin estado compartido entre usuarios.
- Ningún modelo se entrena en producción: el artefacto es global e idéntico para todos.
- No se persisten features ni centroides por usuario; solo el resultado cacheado.
- Logs sin importes, sin categorías nominales y sin `userId` en claro (A-090).
- El aislamiento se apoya en `PK = USER#<id>` y en la autorización por `sub` del JWT.

## 10. Pruebas exigidas (cuando se implemente)

| Prueba | Esperado |
|---|---|
| Usuario con 0 movimientos | `hasEnoughData=false`, `items: []`, `200` |
| Usuario con 5 movimientos | `hasEnoughData=false` |
| Usuario con 11 movimientos | `hasEnoughData=false` (borde) |
| Usuario con 12 movimientos y 1 categoría | `hasEnoughData=false` |
| Dos historiales distintos | clusters distintos y `reason` con cifras propias |
| Repetición de la misma petición | respuesta idéntica (determinismo) |
| Silueta < 0,35 | `hasEnoughData=false` |
| Fuga entre usuarios | 0 resultados de otro `userId` |
| Carga 500 movimientos | p95 ≤ 300 ms |
| `modelVersion` | igual a la versión del artefacto cargado |

## 11. Artefacto y reproducibilidad

- Script de entrenamiento versionado en el repo (`ml/entrenar_kmeans.py`) cuando se implemente.
- Salida: `ml/modelo_kmeans_v1.json` + `ml/metricas_v1.json` (silueta, inercia, k elegido, hash de datos sintéticos).
- El CI verifica que el artefacto versionado coincide con el hash registrado y que la inferencia
  en Go reproduce las asignaciones del script de referencia para un conjunto fijo.

## 12. Estados

| Estado | Valor |
|---|---|
| `ML_DESIGNED` | **APPROVED** (2026-11-25; ver §14) |
| `ML_IMPLEMENTED` | `CODE_COMPLETE_LOCAL` (v2, 2026-11-22); AWS `NOT_RUN` |
| `ML_DEPLOYED` | `NOT_RUN` |
| Enmienda de contrato requerida | `hasEnoughData` (ESENCIAL) + 3 opcionales pospuestos |
| Gate del diseño ML | `APRUEBO_GATE_ML_155` (2026-11-25) |
| Enmiendas del diseño | D-ML-01 (trimestre en curso) y D-ML-02 (sin caché), §15 |

## 13. Evidencia local de implementación — NO constituye Gate

| Campo | Valor observado |
|---|---|
| Fecha | 2026-11-22 |
| Rama | `liz-oleada4` |
| Artefacto | `ml/modelo_kmeans_v1.json`, SHA256 `1997370f1c148df63e126e477d1f931c565d2b1f9b54dd57dc1e48b1a16d48cf` |
| Datos sintéticos | 500 registros, SHA256 `8d2bf29691a08eee6a1cd1230ad81b8454712424721e5e3e21bb9e9bd99918fd` |
| Selección | `k=2`, silueta de entrenamiento `0.780658`, semilla `20261116`, `n_init=10` |
| Entorno real de entrenamiento | Python `3.14.5` + scikit-learn `1.9.0`; Python `3.13` no está instalado en el equipo. El script usa sintaxis compatible con 3.13 y debe regenerarse en 3.13 cuando el responsable lo exija como Gate. |
| Inferencia | Go puro, sin Python/AWS/proveedor en la ruta de petición; 12 asignaciones de referencia Python reproducidas por `ml/inference_test.go`. |
| Latencia local | p95 `1.724 ms` con 500 movimientos (30 iteraciones); objetivo `<= 300 ms`. |
| Gates | `ML_GATE_APPROVED=NO`; `ML_DESIGNED=PENDIENTE_APROBACION`; `AWS_DEPLOYED=NO`. |

Esta sección documenta ejecución local verificable. No firma, no aprueba el RFC y no autoriza despliegue.

---

## 14. Decisión ratificada 2026-11-25

**Autoridad:** decisión humana expresa del responsable con las frases
`APRUEBO_GATE_ML_155` y `APRUEBO_ENMIENDA_ML_VENTANA_90_DIAS_SIN_CACHE_RESPUESTA`.
Esta sección **fija** el diseño aprobado y prevalece sobre cualquier línea anterior de este
documento que diga lo contrario; las enmiendas D-ML-01 y D-ML-02 de §15 la formalizan.

### 14.1 Diseño aprobado (valores exactos)

| Elemento | Decisión ratificada |
|---|---|
| Algoritmo | **K-Means real** (no reglas fijas, no umbrales presentados como modelo) |
| Artefacto activo | `ml/modelo_kmeans_v2.json`, `modelVersion = kmeans-v2` |
| SHA-256 del modelo | `9670c91be3480057361adefa297e6f248a7e83f80af0b0c9d55c6e112bff168c` |
| Ventana de cálculo | **móvil de 90 días**, con el **trimestre en curso incluido** |
| Recorte de outliers | percentiles **P0,5 / P99,5** globales del entrenamiento, aplicados antes de normalizar |
| Mediana | **fija del entrenamiento** (`trained_median_of_clipped_amounts`); **no** se recalcula por titular |
| Caché de respuestas ML | **sin caché Redis** de respuestas; cada petición calcula con el artefacto y los datos del titular |
| Datos de entrenamiento | **sintéticos**, versionados y con SHA-256 verificado |
| Evaluación | **independiente** (holdout separado del conjunto de entrenamiento) |
| Proveedores de IA de pago | **ninguno**; la inferencia es Go nativo y no llama a servicios externos |
| Silueta | métrica **descriptiva** del agrupamiento; **no** es precisión predictiva ni garantía de acierto |
| `modelVersion` | valor del artefacto cargado (`kmeans-v2`) en cada ítem |

### 14.2 Estados registrados

| Estado | Valor |
|---|---|
| `ML_DESIGN_APPROVED` | **YES** (2026-11-25) |
| `ML_FUNCTIONAL_VERIFIED_AWS` | **NO** — no se ejecutó una consulta autenticada a `GET /api/v1/analytics/recommendations` en AWS |
| `ML_DEPLOYED` | **NOT_RUN** — el binario desplegado es `v0.20.0+4bf6758d372`, que contiene `kmeans-v1`; v2 no se ha desplegado |
| `ML_IMPLEMENTED` | `CODE_COMPLETE_LOCAL` (v2) |
| Entrenamiento en esta sesión | **NO** — no se reentrenó; se verificaron los artefactos existentes |

### 14.3 Verificación de artefactos (sin reentrenar)

| Archivo | SHA-256 verificado |
|---|---|
| `ml/modelo_kmeans_v2.json` | `9670c91be3480057361adefa297e6f248a7e83f80af0b0c9d55c6e112bff168c` |
| `ml/metricas_v2.json` | `dd40de5ef28f3c91fdd751a7b7708372485f8da8764f227dd82216193d367e2e` |
| `ml/referencia_inferencia_v2.json` | `322f2d3787474e77313f1034e0afc7bdd1aa06b08a7c8e9df759b810ee788930` |

`ml/metricas_v2.json` declara `selected_silhouette = 0.780848` (selección de `k` en
entrenamiento) y `holdout.silhouette = 0.775027` (evaluación independiente). Ambas son
**descriptivas**; no se presentan como precisión predictiva.

### 14.4 Explicabilidad: la mediana del entrenamiento no es del titular

La inferencia usa `trained_median_cents` como **parámetro fijo** para construir la feature
`ratio_sobre_mediana`; no es una estadística del titular. El texto de `reason` sólo puede citar
la **mediana de los importes del grupo del propio titular**, calculada en la capa determinista
de explicación. Queda prohibido presentar la mediana del entrenamiento como «tu mediana» o como
una estadística personal. La prueba
`TestE055ExplicacionNoAtribuyeLaMedianaDeEntrenamientoAlTitular` lo verifica con un modelo
instrumentado y datos donde ambas medianas difieren.

## 15. Enmiendas formales D-ML-01 y D-ML-02 (2026-11-25)

**Frase que las autoriza:** `APRUEBO_ENMIENDA_ML_VENTANA_90_DIAS_SIN_CACHE_RESPUESTA`.

| ID | Texto derogado o eliminado | Decisión vigente |
|---|---|---|
| **D-ML-01** | §5, fila «Sesgo: se excluyen el trimestre en curso si está incompleto, salvo `period` explícito» | **DEROGADO**. La ventana es móvil de 90 días y **el trimestre en curso cuenta**. La implementación ya lo aplica en `ventanaRecomendaciones` y la prueba `TestE055Ventana90DiasPorDefecto` lo fija. |
| **D-ML-02** | §8, fila «Caché: Redis con TTL 30 s y clave por usuario (A-080), invalidada al escribir movimientos» | **ELIMINADO**. No hay caché Redis de respuestas ML. Cada petición lee los movimientos del titular con `Query` y ejecuta la inferencia; no existe clave de caché que invalidar. |

No se modifican `spec.md`, `plan.md`, el OpenAPI, el artefacto ni las métricas por estas
enmiendas: son correcciones del documento de diseño y del comportamiento ya implementado.
