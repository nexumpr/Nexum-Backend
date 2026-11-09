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
