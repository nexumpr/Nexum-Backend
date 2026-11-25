# RFC-026 — Enmienda documental de E051: sin caché de respuesta (RFC-022 D-W3-07)

| Campo | Valor |
|---|---|
| Documento | `sdd/_control/H08/RFC-026-ENMIENDA_E051_CACHE.md` |
| Versión | 1.0.0 |
| Fecha del documento | 2026-11-24 |
| Estado | **APPROVED** |
| Frase de aprobación | `APRUEBO_ENMIENDA_E051_SIN_CACHE_RFC_022` |
| Fecha de concesión | 2026-11-25, en la orden del Scrum Master (según la orden recibida) |
| Aprobador | Scrum Master de Nexum-Backend |
| Alcance | **Exclusivamente documental**: alinear `sdd/03-endpoints/E051-dashboard/{spec,plan,tasks,checklist}.md` con la norma vigente. No autoriza código, OpenAPI, Terraform, AWS, release ni merge |
| Norma que se aplica | RFC-022 v1.2.1 **APPROVED** (`APRUEBO_RFC_022_CONTRATOS_OLEADA_3_V1_2_1`), §8.2 «E051 — cálculo directo»; Gate `GATE_H08E_OLEADA_3.md` §5.2, decisión **D-W3-07** |
| Instrumentos relacionados | `RFC-019-OLEADA_E007_E011.md` (reutilizable), `RECONCILIACION_CONTRACTUAL_E002_E008_E009_E051.md` §5, `MATRIZ_AUDITORIA_54_ENDPOINTS.md` (E051) |
| Base verificada | `main` `f3bccca8b2437dc154bedbef257a3cabc7620844`; rama de trabajo `fix/h08e-deuda-historica` |

> **Nota de estado.** La frase `APRUEBO_ENMIENDA_E051_SIN_CACHE_RFC_022` fue concedida por el
> responsable en la orden del 2026-11-25 y se registra aquí tal como se recibió. Este documento
> **no** la concede por sí mismo: la transcribe y fija su alcance.

---

## 1. Problema

La ficha H06 de E051 sigue exigiendo una **caché privada Redis** con TTL e invalidación ante
mutaciones financieras, mientras la norma aprobada posterior (RFC-022 v1.2.1) decidió lo
contrario y el runtime publicado ya calcula el agregado directamente desde DynamoDB.

La contradicción es **normativa**, no de comportamiento:

| Fuente | Qué dice hoy |
|---|---|
| `sdd/03-endpoints/E051-dashboard/spec.md` | Redis como persistencia (§tabla, :13), caché privada invalidada (:18), «Redis privado» en acceso físico (:88), invariante con caché (:90), T04/T05 con Redis (:97-98) |
| `sdd/03-endpoints/E051-dashboard/plan.md` | pasos 3 y 4 con Redis, TTL e invalidación (:9-10); decisiones específicas (:16, :18); prueba real con Redis (:25) |
| `sdd/03-endpoints/E051-dashboard/tasks.md` | T04, T05 y T06 exigen caché Redis (:8-10) |
| `sdd/03-endpoints/E051-dashboard/checklist.md` | «Redis invalidación/TTL/servicio real previsto; fallo seguro» (:12) |
| **RFC-022 v1.2.1 §8.2** y **Gate Oleada 3 D-W3-07** | **Sin caché de dashboard**: cálculo directo desde DynamoDB; no se usa Redis, no se usa `SCAN`, no se amplía el ACL, no se añaden hooks; la caché es optimización posterior con su propio Gate |
| Runtime (`internal/application/dashboard/dashboard.go:1-2`, `cmd/server/e051.go:26`) | Sin caché Redis; `Query`/`GetItem` por partición |

Sin esta enmienda, cualquier revisor o implementador que lea `spec.md` concluiría que el endpoint
**incumple** su propia especificación, cuando en realidad cumple la decisión más reciente. El
instrumento existe precisamente porque la política de edición **no permite** modificar
`spec.md`/`plan.md` sin una RFC específica (AGENTS.md §4; `opencode.json` clasifica esos archivos
como `ask`). Esta unidad **no** edita esos cuatro archivos: entrega el texto exacto.

## 2. Autoridad y precedencia

1. `constitution.md` R-002: la constitución no puede rebajarse; las specs precisan
   comportamientos, pero una contradicción sustantiva obliga a RFC/ADR y revisión humana.
2. RFC-022 v1.2.1 está **APPROVED** y es posterior a la ficha H06 (2026-11-19 frente a H06).
3. El Gate de Oleada 3 registra la decisión **D-W3-07** con este texto:
   «E051 calcula directamente desde DynamoDB, sin caché de dashboard | No existe un mecanismo
   probado que garantice consistencia tras una mutación; la caché queda como optimización
   posterior con su propio Gate» (`GATE_H08E_OLEADA_3.md:136`).
4. El Definition of Done del mismo Gate exige «E051 **sin** caché de dashboard» (`:144`).
5. El cierre funcional en AWS se certificó sobre `v0.19.2+3098f379d7ab` con ese comportamiento
   (`memoria.md`, cierre de Oleada 3; `GATE_H08E_OLEADA_3.md` §12.1).

Por tanto la norma aplicable es **D-W3-07** y la ficha H06 queda enmendada en los términos del
§3. No se cambia el contrato HTTP: ruta, método, esquema de respuesta, códigos y cabeceras
(`Cache-Control: private, no-store`) siguen intactos. **La caché no es observable por el
contrato**; solo afecta a latencia y a la documentación interna.

## 3. Texto exacto a sustituir

Referencias de línea sobre `main` `f3bccca`. Se reproduce el texto **actual** y el **propuesto**.
Las líneas no listadas no se tocan.

### 3.1 `sdd/03-endpoints/E051-dashboard/spec.md`

#### Línea 13 — tabla de propiedades, fila `Persistencia`

Actual:

```markdown
| Persistencia | AWS DynamoDB/Redis, sin Scan; ver `DYNAMODB_ACCESOS_H06.md` |
```

Propuesto:

```markdown
| Persistencia | AWS DynamoDB, sin Scan y sin caché de respuesta Redis (RFC-022 §8.2, decisión D-W3-07); ver `DYNAMODB_ACCESOS_H06.md` |
```

#### Línea 18 — §1, criterio funcional

Actual:

```markdown
Agregados consistentes y versionados, cache privada Redis invalidada ante mutación financiera.
```

Propuesto:

```markdown
Agregados consistentes calculados desde DynamoDB en cada solicitud y versionados en origen; sin caché de respuesta Redis (RFC-022 §8.2, decisión D-W3-07). La caché queda como optimización posterior y exigirá un mecanismo de consistencia y su propio Gate.
```

#### Línea 88 — §4, acceso físico

Actual:

```markdown
Query por usuario y periodo + proyecciones versionadas / Redis privado; sin Scan; ML/IA según contrato de proveedor.
```

Propuesto:

```markdown
Query por usuario y periodo + proyecciones versionadas en DynamoDB; sin Scan y sin caché de respuesta Redis (RFC-022 §8.2, decisión D-W3-07). No se usa ML/IA en este endpoint.
```

#### Línea 90 — §4, invariante

Actual:

```markdown
Invariante: Agregados consistentes y versionados, cache privada Redis invalidada ante mutación financiera. Ninguna operación puede hacer `Scan` global ni consultar PK de un usuario diferente. ETag/versionado, cancelación de transacciones, caché y TTL deben seguir H04/H05. El contrato OpenAPI es `openapi/nexum-api-v1.yaml` y su espejo JSON; ambos son la autoridad de transporte de H06.
```

Propuesto:

```markdown
Invariante: Agregados consistentes calculados desde DynamoDB y versionados en origen; sin caché de respuesta Redis (RFC-022 §8.2, decisión D-W3-07). Ninguna operación puede hacer `Scan` global ni consultar PK de un usuario diferente. ETag/versionado, cancelación de transacciones y TTL deben seguir H04/H05. El contrato OpenAPI es `openapi/nexum-api-v1.yaml` y su espejo JSON; ambos son la autoridad de transporte de H06.
```

#### Línea 97 — §5, criterio T04

Actual:

```markdown
- **T04:** Pérdida temporal de DynamoDB/Redis -> respuesta de fallo controlado 503; jamás confirmar escritura parcial.
```

Propuesto:

```markdown
- **T04:** Pérdida temporal de DynamoDB -> respuesta de fallo controlado 503; jamás confirmar escritura parcial. Redis no participa en E051: su indisponibilidad no altera la respuesta (RFC-022 D-W3-07).
```

#### Línea 98 — §5, criterio T05

Actual:

```markdown
- **T05:** Misma operación en dos nodos Go reales; consistencia de sesión/caché y ausencia de duplicaciones.
```

Propuesto:

```markdown
- **T05:** Misma operación en dos nodos Go reales; consistencia de los agregados leídos de DynamoDB y ausencia de duplicaciones. No hay caché compartida que pueda servir un valor obsoleto (RFC-022 D-W3-07).
```

### 3.2 `sdd/03-endpoints/E051-dashboard/plan.md`

#### Línea 9 — §«Arquitectura y secuencia», paso 3

Actual:

```markdown
3. Adaptador DynamoDB: Query por usuario y periodo + proyecciones versionadas / Redis privado; sin Scan; ML/IA según contrato de proveedor. Integrar condiciones, `Query`/`GetItem`, cero `Scan`, transacciones cuando aplique.
```

Propuesto:

```markdown
3. Adaptador DynamoDB: Query por usuario y periodo + proyecciones versionadas; sin Scan y sin caché de respuesta Redis (RFC-022 §8.2, decisión D-W3-07). Integrar condiciones, `Query`/`GetItem`, cero `Scan`, transacciones cuando aplique.
```

#### Línea 10 — §«Arquitectura y secuencia», paso 4

Actual:

```markdown
4. Redis: política por usuario, TTL/invalidation, revocación multiinstancia y fallo seguro. Un cache hit no puede saltarse autorización.
```

Propuesto:

```markdown
4. Redis: **no participa en E051** (RFC-022 §8.2, decisión D-W3-07). No se añaden claves, TTL ni hooks de invalidación para este endpoint. Una caché futura exigirá un mecanismo que garantice la consistencia con las mutaciones y su propio Gate. Redis sigue siendo obligatorio para sesiones, cuotas y otros endpoints ya publicados.
```

#### Línea 16 — §«Decisiones específicas»

Actual:

```markdown
- Agregados consistentes y versionados, cache privada Redis invalidada ante mutación financiera.
```

Propuesto:

```markdown
- Agregados consistentes calculados desde DynamoDB y versionados en origen; sin caché de respuesta Redis (RFC-022 §8.2, decisión D-W3-07).
```

#### Línea 18 — §«Decisiones específicas»

Actual:

```markdown
- Patrón de persistencia: Query por usuario y periodo + proyecciones versionadas / Redis privado; sin Scan; ML/IA según contrato de proveedor.
```

Propuesto:

```markdown
- Patrón de persistencia: Query por usuario y periodo + proyecciones versionadas en DynamoDB; sin Scan y sin caché de respuesta Redis (RFC-022 §8.2, decisión D-W3-07).
```

#### Línea 25 — §«Dependencias y riesgos»

Actual:

```markdown
- Prueba real HTTP/HTTPS y DynamoDB/Redis: **NOT_RUN**; solo el contrato/documento es verificable ahora.
```

Propuesto:

```markdown
- Prueba real HTTP/HTTPS y DynamoDB: **NOT_RUN**; solo el contrato/documento es verificable ahora. Redis no interviene en E051 (RFC-022 D-W3-07).
```

> La línea 11 del `plan.md` («`Location` en 201 si corresponde») no se toca en esta RFC: es
> plantilla genérica y E051 declara `200`; su corrección, si se desea, es una limpieza editorial
> independiente.

### 3.3 `sdd/03-endpoints/E051-dashboard/tasks.md` — anotación (no sustitución)

La ficha histórica **no se reescribe**. Debajo de cada tarea afectada se añade una línea de
anotación con este texto literal:

- Bajo `E051-T04` (`tasks.md:8`):

```markdown
  > **E051-T04 — ANOTADO (RFC-026):** la caché privada Redis queda **descartada** por RFC-022 §8.2 / D-W3-07. El caso de uso calcula los agregados desde DynamoDB en cada solicitud; no se implementa TTL ni invalidación.
```

- Bajo `E051-T05` (`tasks.md:9`):

```markdown
  > **E051-T05 — ANOTADO (RFC-026):** el patrón es `Query`/`GetItem` en DynamoDB sin `Scan`; **no** se integra Redis para esta respuesta (RFC-022 D-W3-07).
```

- Bajo `E051-T06` (`tasks.md:10`):

```markdown
  > **E051-T06 — ANOTADO (RFC-026):** la consistencia entre instancias se garantiza leyendo DynamoDB con la consistencia declarada; no hay caché de respuesta que invalidar. T06 se satisface sin Redis.
```

- Bajo `E051-CT04` (`tasks.md:23`):

```markdown
  > **E051-CT04 — ANOTADO (RFC-026):** la prueba se ejecuta con DynamoDB no disponible (503) y con Redis no disponible (debe seguir 200). La caché está descartada por RFC-022 D-W3-07.
```

- Bajo `E051-CT05` (`tasks.md:24`):

```markdown
  > **E051-CT05 — ANOTADO (RFC-026):** dos nodos deben devolver el mismo agregado leyendo DynamoDB; no se prueba coherencia de caché porque no existe caché de respuesta (RFC-022 D-W3-07).
```

### 3.4 `sdd/03-endpoints/E051-dashboard/checklist.md` — casilla 12

Actual (`checklist.md:12`):

```markdown
- [ ] Redis invalidación/TTL/servicio real previsto; fallo seguro.
```

Propuesto:

```markdown
- [x] Redis **no** participa en E051: caché de respuesta descartada por RFC-022 §8.2 / D-W3-07 (ver RFC-026). Invalidación y TTL no aplican; Redis sigue siendo obligatorio para sesiones y cuotas.
```

No se marcan otras casillas: esta enmienda solo acredita que la exigencia de caché quedó
descartada, no que el endpoint tenga las pruebas de aceptación completas.

## 4. Instrucción de aplicación

1. Aplicar los reemplazos del §3.1 y §3.2 con edición literal, **sin** reformatear el resto del
   archivo (los documentos tienen una línea por párrafo; no introducir saltos nuevos).
2. Insertar las anotaciones del §3.3 **inmediatamente debajo** de la tarea correspondiente,
   conservando el estado histórico de la casilla (`[ ]`). No se cambia `[ ]` por `[x]` en T04,
   T05, T06, CT04 ni CT05: la anotación explica que la exigencia ya no aplica, no que la tarea se
   haya ejecutado.
3. Sustituir la casilla del §3.4 y dejar constancia de la referencia a esta RFC.
4. No modificar el contrato OpenAPI: la caché no aparece en `nexum_e051`.
5. No tocar `E026`, `E050`, `E051` runtime, `cmd/server/e051.go` ni Terraform.
6. Recalcular el manifiesto `sdd/_control/H07/BASELINE_SHA256.json` en el PR que aplique la
   enmienda (los cuatro archivos cambian). El PR debe pasar `validar_sdd_integral.py`,
   `validar_h06.py`, paridad YAML/JSON y el comparador de compatibilidad (no debe haber cambio
   de contrato, por lo que debe devolver `COMPATIBLE`).

## 5. Por qué no se reintroduce la caché

| Motivo | Detalle |
|---|---|
| Consistencia | No existe un mecanismo probado que invalide la caché en la misma transacción que las mutaciones de movimientos, metas, aportaciones o recurrentes. Una entrada obsoleta alteraría `summary`, `goals`, `latestMovements` o `asOf` |
| Superficie | La invalidación exigiría hooks en E024/E028/E030/E031/E033–E036 y ampliar el ACL Redis (hoy `-scan`), con riesgo de seguridad y de fuga entre usuarios |
| Beneficio no demostrado | Las consultas son `Query` acotadas (≤100 metas, 10 movimientos, un periodo). RFC-022 R-22-07 exige **medir** la latencia en AWS antes de justificar una caché |
| Contrato | La caché no es observable: no hay campo, cabecera ni código que dependa de ella. Retirarla de la documentación no rompe a Android |

La puerta queda abierta: una caché futura requerirá (a) mecanismo de consistencia demostrado,
(b) aislamiento por usuario, (c) invalidación verificable, (d) medición previa y (e) su propio
Gate. Nada de eso se autoriza aquí.

## 6. Riesgos

| ID | Riesgo | Mitigación |
|---|---|---|
| R-26-01 | Alguien lee la ficha sin la enmienda aplicada y «corrige» el runtime añadiendo caché | Este RFC fija la norma y la enmienda se aplica en el mismo PR documental; la nota de vigencia de `tasks.md`/`checklist.md` ya apunta a RFC-022 |
| R-26-02 | Latencia mayor sin caché | R-22-07: medición en AWS; si supera el objetivo acordado, se abre una optimización con Gate propio, no un cambio silencioso |
| R-26-03 | Edición parcial que deje una mención a Redis | La lista del §3 es exhaustiva sobre `spec/plan`; al aplicar, `grep -n -i redis` en la carpeta debe devolver solo las líneas propuestas que explican que **no** participa |
| R-26-04 | El PR de aplicación olvida el manifiesto | §4.6 lo exige; el job SDD falla cerrado si el hash no coincide |

## 7. Qué no hace esta RFC

- No cambia código Go, ni rutas, ni códigos, ni esquemas, ni cabeceras.
- No modifica `spec.md`/`plan.md`/`tasks.md`/`checklist.md` en esta unidad (política de edición);
  entrega el texto exacto para su aplicación con esta autorización.
- No reintroduce ni elimina Redis del resto del sistema: sesiones, cuotas y E070/E071 siguen
  usándolo.
- No autoriza AWS, release, merge ni despliegue.

## 8. Aprobación

| Frase | Estado | Alcance |
|---|---|---|
| `APRUEBO_ENMIENDA_E051_SIN_CACHE_RFC_022` | **CONCEDIDA** (orden del 2026-11-25) | Aplicar esta enmienda documental en los cuatro archivos de E051 |

## 9. Trazabilidad

- `sdd/03-endpoints/E051-dashboard/{spec,plan,tasks,checklist}.md`
- `sdd/_control/H08/RFC-022-CONTRATOS_OLEADA_3.md` §8.2
- `sdd/_control/H08/GATE_H08E_OLEADA_3.md` §5.2 D-W3-07, §6 Definition of Done, §12.1
- `sdd/_control/H08/RECONCILIACION_CONTRACTUAL_E002_E008_E009_E051.md` §5
- `sdd/_control/H08/MATRIZ_AUDITORIA_54_ENDPOINTS.md` (fila E051)
- `internal/application/dashboard/dashboard.go:1-2`, `cmd/server/e051.go:26`
- `memoria.md` — cierre de Oleada 3 (`v0.19.2+3098f379d7ab`)
