# Gate H08-I / I-150 — Publicación y compatibilidad de la API (LOCAL)

**FRASE HUMANA REGISTRADA:** `APRUEBO_GATE_I150_LOCAL` (2026-11-09).

**ESTADO:** `AUTHORIZED_I150_LOCAL` — implementación **exclusivamente local**. Sin AWS, sin HTTP real,
sin despliegue y sin publicar el contrato.

## 1. Alcance autorizado

- Validador `herramientas/verificar_publicacion_i150.py` con 6 comandos.
- `MATRIZ_COMPATIBILIDAD_75.md` (E001–E075), `PLAN_PUB_OPENAPI.md`, `PLAN_EVIDENCIA_HTTP.md` y
  `MATRIZ_INF_150.md`.
- Plantilla de publicación con checksum calculado y CORS con allowlist.
- Plan de evidencia HTTP con 13 pruebas en `NOT_RUN`.
- Pruebas adversas y actualización de estado/integridad.
- Commit, push, PR y seguimiento de CI. **Merge requiere autorización humana separada.**

## 2. NO autorizado

- Publicar el contrato, abrir endpoints o ejecutar pruebas HTTP reales.
- `terraform plan/apply/destroy`, despliegue, cambios de CloudFront/ALB/DNS.
- Modificar contratos E001–E075, `path`, schemas o la clasificación M/S/E/O.
- Requerir aprobación o pruebas del equipo Mobile como gate del Backend.
- DAST, carga o AWS CLI.
- H08-E y E001+: siguen `NOT_AUTHORIZED`.

## 3. Decisiones de diseño

1. **Contrato como fuente de verdad:** se valida OpenAPI 3.1.0, prefijo `/api/v1`, `traceId` en errores,
   importes monetarios como `string` (nunca `number`) y cabeceras de idempotencia/cursores.
2. **URL documental:** `servers[0].url` es `https://api.example.invalid`; cualquier URL que afirme un
   endpoint AWS real antes del despliegue se rechaza.
3. **Checksum real:** `publicacion` recalcula el SHA-256 del artefacto OpenAPI y lo compara con el
   declarado; un hash inventado falla.
4. **CORS cerrado:** allowlist explícita; `*` prohibido, y con credenciales se exigen orígenes exactos.
5. **Compatibilidad trazable:** 75 filas con clase 33 M · 23 S · 12 E · 7 O; los 204/202 sin cuerpo y
   los PATCH sin payload no exigen esquemas.
6. **Evidencia honesta:** 13 pruebas HTTP en `NOT_RUN`; un `PASS` sin AWS o sin huella se rechaza.
7. **SemVer y deprecación:** un cambio incompatible exige nueva versión de API o `major`, RFC y
   detección en CI.
8. **Alcance separado:** Android, Play Store, pantallas, pruebas Mobile y landing nunca son gate.

## 4. Entregables y verificación

| Comprobación | Resultado |
|---|---|
| `... contrato` | `CONTRATO_VALIDO` (55 rutas, `traceId`, dinero string) |
| `... publicacion` | `PUBLICACION_VALIDA` con checksum recalculado |
| `... compatibilidad` | `MATRIZ_COMPATIBLE` (75 filas, 33/23/12/7) |
| `... evidencia` | `PLAN_VALIDO`, `ejecucion_aws = NOT_RUN` |
| `... deprecacion` | `POLITICA_VALIDA` |
| `... alcance` | `ALCANCE_SEPARADO` |
| Suite `tests/infra/publicacion` | 45 OK |
| SDD / H06 / SHA-256 / Gitleaks / Trivy / Go | PASS |

## 5. Bloqueos y dependencias

- HTTP real, TLS con CA, persistencia DynamoDB y dos nodos: `NOT_RUN` hasta desplegar.
- `H-01` (TLSv1 por certificado por defecto), RFC-004, RFC-006, RFC-007 sin cambios.
- `TWO_NODES_AWS_NOT_VERIFIED`, `AWS_SCP_NOT_VERIFIED`, branch protection `BLOQUEADO_PLAN_GITHUB`.
- INF-150-07 (CDN/moderación) no aplica: el inventario actual no expone EPs de medios.

## 6. Criterio de cierre local

I-150 podrá declararse `IMPLEMENTADO_LOCAL — NO DESPLEGADO` cuando el contrato valide, el checksum
coincida, la matriz esté trazada, la evidencia HTTP permanezca `NOT_RUN`, SemVer y alcance se validen,
Terraform/Trivy/Gitleaks/Go pasen y SDD/SHA-256 permanezcan íntegros.

**No se declara** `API_PUBLICADA`, `HTTP_VERIFIED`, `TWO_NODES_VERIFIED` ni `PRODUCTION_READY`.

## 7. Registro

| Fecha | Hecho |
|---|---|
| 2026-11-09 | `APRUEBO_GATE_I150_LOCAL` concedido; rama `feat/h08i-i150-publicacion` desde `main` `7d1418a` (incluye el cierre documental de I-140) |
| 2026-11-09 | Implementación local completada; PR y CI por el agente; merge pendiente de autorización humana ligada a PR + HEAD |

## 8. Ampliación de alcance (2026-11-09)

- **INF-150-01:** nuevo comando `paridad` que contrasta JSON (75 operaciones, 55 rutas) con YAML
  (75/55) y con la matriz H06 (`x-nexum-catalog-id` y `operationId = nexum_eNNN`).
- **INF-150-03:** matriz ampliada a 19 columnas por operación (clase, método, ruta, operación, entrada,
  salida, status, auth, idempotencia, paginación, persistencia, prueba positiva, prueba negativa,
  requisito, dependencias, estado de contrato y estado HTTP).
- **INF-150-04:** cuatro niveles de evidencia separados (estática local, HTTP local, HTTP AWS y
  persistencia/dos nodos) y generador de comandos `curl` que **no** los ejecuta.
- **INF-150-05:** `herramientas/comparar_contrato_api.py` detecta ruta/método eliminado,
  `operationId` cambiado, parámetro o campo obligatorio nuevo, tipo y enum incompatibles, respuesta
  eliminada y cambios de seguridad; se ejecuta en GitHub Actions contra `origin/main` y falla cerrado
  si no puede comparar. Las excepciones exigen RFC en estado `APPROVED`.
- **INF-150-06:** `FRONTERA_RESPONSABILIDAD.md` con el reparto explícito y validación de que Mobile no
  es gate.
- **INF-150-07:** `contenido` escanea los 75 contratos: **0 operaciones con medios y 0 con texto libre**;
  se documenta el criterio de activación futura sin declarar moderación operativa.

`HTTP_VERIFIED` y `TWO_NODES_VERIFIED` siguen `NOT_RUN`.

## 9. Corrección de revisión (2026-11-09)

- **A — paridad semántica:** se sustituyó el conteo por un parser de bloque real del YAML que reconstruye
  la estructura y compara **por operación**: método+ruta, `operationId`, `x-nexum-catalog-id`,
  `security`, parámetros (nombre, ubicación, obligatoriedad), `requestBody` y `responses` (código y
  esquema). Un cambio de POST a PUT, un `operationId` distinto, otra seguridad, un parámetro nuevo, otra
  respuesta o un esquema distinto producen `PARIDAD_ROTA`; los `$ref` irresolubles y el YAML con
  tabulaciones fallan cerrado.
- **B — breaking changes completos:** se añaden `PROPIEDAD_RESPUESTA_ELIMINADA`,
  `TIPO_RESPUESTA_CAMBIADO`, `ENUM_RESPUESTA_REDUCIDO`, `REQUERIDO_RESPUESTA_NUEVO`,
  `ESQUEMA_RESPUESTA_CAMBIADO` y `SCOPES_CAMBIADOS`, con **seguridad efectiva** (operación sobre global,
  incluido `security: []`) y `$ref` no resoluble ⇒ falla cerrado (código 2). Una excepción solo cuenta si
  su RFC aparece ratificada en un documento de `sdd/_control/H08`.
- **C — INF-150-07:** `MODERACION_CDN_I150.md` pasa a `DISEÑO_LOCAL_COMPLETO —
  IMPLEMENTACIÓN_CLOUD_PENDIENTE` con las tres operaciones, separación validación/sanitización/
  moderación, punto de control antes de persistir, códigos de rechazo, tratamiento de PII, evaluación de
  servicio cloud (opción local, Amazon Comprehend y externo), CDN descartada por existir CloudFront D2 y
  `BLOCKED_DEPENDENCY` para moderación externa. El validador rechaza cualquier operación con contenido
  que no figure en el diseño.

## 10. Corrección final (2026-11-09)

- **A — parser YAML:** se elimina el parser artesanal y se usa **PyYAML 6.0.2** fijado en
  `herramientas/requirements.txt`, instalado en CI antes de los validadores. El cargador personalizado
  **rechaza claves duplicadas en cualquier profundidad** (ruta, operación, propiedad de schema) y falla
  cerrado ante YAML inválido. La paridad compara ahora también `components.schemas` (tipo, `required`,
  `enum`, `items`, `additionalProperties`, composición y `$ref`) y `securitySchemes`: **88 esquemas**
  contrastados. Un cambio aislado en el YAML de un tipo o de un `enum` produce `PARIDAD_ROTA`.
- **B — excepciones de breaking changes:** se elimina la aprobación por coincidencia textual. Ahora se
  exige un registro `sdd/_control/H08/EXCEPCIONES_COMPATIBILIDAD_API.json` con `alcance =
  API_COMPATIBILIDAD`, código, ruta, método, RFC, aprobador, `gate_documento` que contenga `APRUEBO` y el
  RFC, y `vigencia_hasta` no vencida. Las RFC de infraestructura/seguridad/FinOps (RFC-004, RFC-005,
  RFC-006, RFC-007) se rechazan explícitamente como autorización de API. Como **no existe ningún registro
  aprobado**, toda excepción se rechaza y el comparador queda fail-closed.

Evidencia: 110 pruebas en `tests/infra/publicacion`. `HTTP_VERIFIED` y `TWO_NODES_VERIFIED` siguen
`NOT_RUN`; INF-150-07 permanece `DISEÑO_LOCAL_COMPLETO — IMPLEMENTACIÓN_CLOUD_PENDIENTE` con
`BLOCKED_DEPENDENCY`.
