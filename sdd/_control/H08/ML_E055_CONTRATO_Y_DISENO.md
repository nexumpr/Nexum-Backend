# ML-155 — Contrato y diseño del módulo K-Means para E055

| Campo | Valor |
|---|---|
| Documento | `sdd/_control/H08/ML_E055_CONTRATO_Y_DISENO.md` |
| Endpoint consumidor | `GET /api/v1/analytics/recommendations` (`nexum_e055`) |
| Algoritmo | **K-Means** (aprendizaje no supervisado, agrupamiento) |
| Estado del diseño | **DISEÑADO — pendiente de aprobación (RFC-016 §6)** |
| `ML_DESIGNED` | **PENDIENTE_APROBACION** (no `VERIFIED` hasta firma humana) |
| `ML_IMPLEMENTED` | `NOT_RUN` |
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
| `ML_DESIGNED` | **PENDIENTE_APROBACION** (este documento es propuesta) |
| `ML_IMPLEMENTED` | `NOT_RUN` |
| `ML_DEPLOYED` | `NOT_RUN` |
| Enmienda de contrato requerida | `hasEnoughData` (ESENCIAL) + 3 opcionales pospuestos |
| Gate | `APRUEBO_RFC_016_CONTRATOS_E005_E075` |
