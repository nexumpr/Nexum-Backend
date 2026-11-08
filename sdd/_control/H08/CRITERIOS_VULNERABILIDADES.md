# I-100 — Criterios de vulnerabilidades

| Campo | Valor |
|---|---|
| Spec | `sdd/02-infraestructura/I-100-devsecops` (INF-100-02, INF-100-03, INF-100-05) |
| Estado | `IMPLEMENTADO_LOCAL` — política aplicada por CI; excepciones requieren RFC |
| Principio | Tolerancia cero a secretos expuestos y bloqueo de HIGH/CRITICAL explotables |

## 1. Severidad y decisión

| Severidad | Ejemplos | Decisión |
|---|---|---|
| **CRITICAL** | secreto real, RCE, bypass de autenticación, IAM `*` explotable | Bloquea merge y release; corrección obligatoria |
| **HIGH** | CVE explotable, IDOR demostrado, cifrado ausente en dato sensible, SG público | Bloquea merge; corrección o RFC firmada |
| **MEDIUM** | hardening, configuración débil sin explotación directa | Se registra; plan de corrección con fecha |
| **LOW/INFO** | buenas prácticas | Se registra; no bloquea |

Los umbrales aplicados en CI: gosec/govulncheck/Trivy con severidad HIGH/CRITICAL y `exit-code 1`;
gitleaks bloquea cualquier hallazgo; el escáner IaC propio bloquea cualquier regla.

## 2. Excepciones

Sólo una **RFC firmada por el responsable** puede exceptuar un HIGH/CRITICAL. Debe indicar: hallazgo
(ID y ruta), causa raíz, riesgo residual, controles compensatorios reales, condiciones, caducidad y plan
de corrección. La excepción se limita al ID y ruta concretos (sin comodines) y caduca automáticamente
cuando exista un mecanismo de comprobación (por ejemplo `expired_at` en `.trivyignore.yaml` o
`verificar_vigencia_rfc006.py`).

Estado actual: `RFC-006` (CloudFront sin WAF) aprobada temporalmente hasta 2026-12-02; no es válida para
despliegue real. `RFC-005` (HTTP interno CloudFront→ALB) ratificada. Ninguna otra excepción está activa.

## 3. Tratamiento de falsos positivos

1. Verificar la regla contra la documentación oficial.
2. Reproducir de forma determinista.
3. Documentar por qué no aplica (ruta, recurso, configuración).
4. Añadir excepción **acotada** con justificación, o corregir el código.
5. Nunca desactivar la regla global ni el escáner.

## 4. Tiempos de corrección (cuando se detecte en CI)

| Severidad | Corrección objetivo |
|---|---|
| CRITICAL | Inmediata; bloquea el PR |
| HIGH | Antes del merge o RFC con caducidad ≤ 30 días |
| MEDIUM | Siguiente release planificado |
| LOW | Backlog priorizado |

## 5. Evidencia y trazabilidad

- Cada hallazgo se registra con herramienta, versión, ID, archivo/línea, severidad y decisión.
- El resultado se asocia al SHA evaluado; no se reutiliza para otro commit.
- Los secretos detectados no se reproducen en logs ni issues: se rotan y se registra sólo el identificador.
- Un escaneo saltado o no ejecutado es `NOT_RUN`, nunca `PASS`.

## 6. Prohibiciones

- `continue-on-error`, `|| true`, `-exclude` o `--exit-code 0` para obtener verde.
- Ampliar allowlists para ocultar hallazgos.
- Escanear terceros o producción sin autorización.
- Publicar hallazgos con datos personales o credenciales.
