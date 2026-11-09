# Matriz de contratos INF-150-01..07 — I-150

| Campo | Valor |
|---|---|
| Spec | `I-150-publicacion-y-compatibilidad-api` |
| Versión | 0.1.0 |
| Estado | **IMPLEMENTADO_LOCAL — NO DESPLEGADO** |
| Ejecución HTTP | `NOT_RUN` |

| Contrato | Implementación local | Pruebas | Estado |
|---|---|---|---|
| INF-150-01 Contrato | `contrato` (3.1.0, `/api/v1`, `traceId`, dinero string, cabeceras) + `paridad` (JSON ↔ YAML ↔ matriz: 75 operaciones) | `ContratoOpenAPI`, `Paridad` | `IMPLEMENTADO_LOCAL` |
| INF-150-02 Descubrimiento | `PLAN_PUB_OPENAPI.md` + checksum recalculado y CORS con allowlist | `Publicacion` | `IMPLEMENTADO_LOCAL` |
| INF-150-03 Compatibilidad | `MATRIZ_COMPATIBILIDAD_75.md` (33 M · 23 S · 12 E · 7 O) | `Compatibilidad` | `IMPLEMENTADO_LOCAL` |
| INF-150-04 Evidencia HTTP | `PLAN_EVIDENCIA_HTTP.md` con 13 pruebas `NOT_RUN` | `EvidenciaHTTP` | `IMPLEMENTADO_LOCAL` |
| INF-150-05 Cambio | `deprecacion` (SemVer, RFC, CI) + `comparar_contrato_api.py` ejecutado en GitHub Actions contra `origin/main` | `Deprecacion`, `Comparador` | `IMPLEMENTADO_LOCAL` |
| INF-150-06 Alcance | `alcance` + `FRONTERA_RESPONSABILIDAD.md` exigen Mobile/Play/landing fuera del gate | `Alcance` | `IMPLEMENTADO_LOCAL` |
| INF-150-07 CDN/moderación | `contenido` escanea los 75 contratos (0 medios, 3 operaciones con texto libre: E028, E030, E075) y `MODERACION_CDN_I150.md` define validación, sanitización, moderación, punto de control y evaluación de servicio | `Contenido`, `Moderacion` | `DISEÑO_LOCAL_COMPLETO — IMPLEMENTACIÓN_CLOUD_PENDIENTE` (`BLOCKED_DEPENDENCY` para moderación externa) |

## Invariantes

1. Un contrato OpenAPI no demuestra handler funcional ni HTTP real.
2. La clasificación M/S/E/O no exime requisitos obligatorios del profesor.
3. Mobile, Play Store y landing no son gate del Backend.
4. Ninguna prueba HTTP se declara `PASS` sin ejecución autorizada y huella.
5. La URL del contrato permanece documental hasta el despliegue.
6. Un cambio incompatible exige nueva versión de API o RFC aprobada; el check de CI falla cerrado.
7. La moderación solo se declara operativa con servicio desplegado y evidencia; hoy está
   `BLOCKED_DEPENDENCY` (servicio y precio sin aprobar) y `moderacion_operativa = false`.

## Dependencias y bloqueos

| Elemento | Estado |
|---|---|
| I-030 (CloudFront/ALB), I-060 (IAM), I-080 (CI), I-090 (CD) | integradas |
| H06 (75 contratos) | validado por `validar_h06.py` |
| HTTP real, TLS y dos nodos | `NOT_RUN` |
| H-01 (TLSv1 por certificado por defecto), RFC-004, RFC-006, RFC-007 | abiertos |
| Despliegue | requiere Gate independiente |
