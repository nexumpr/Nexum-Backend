# I-100 — Modelo de amenazas

| Campo | Valor |
|---|---|
| Spec | `sdd/02-infraestructura/I-100-devsecops` (INF-100-01) |
| Estado | `IMPLEMENTADO_LOCAL` — modelo documental; verificación AWS/HTTP `NOT_RUN` |
| Alcance | Backend Go, API `/api/v1`, datos financieros, identidad, IaC y cadena de suministro |

## 1. Superficies y activos

| ID | Superficie | Activos | Controles actuales |
|---|---|---|---|
| S1 | Android → CloudFront/HTTPS | credenciales, JWT, datos financieros | HTTPS obligatorio, JWT RS256, rate limit, CORS, cabeceras, límite de cuerpo |
| S2 | CloudFront/VPC Origin → ALB interno → Go | tráfico de API | ALB interno, SG restringido a prefix list, sin IPv4 pública ni NAT |
| S3 | Go → DynamoDB / Redis | movimientos, metas, sesiones, caché | TLS, RBAC Redis, claves con hash, sin Scan, TTL, idempotencia |
| S4 | GitHub Actions → AWS OIDC | credenciales de despliegue | OIDC con subject de entorno, sin claves estáticas, permisos mínimos |
| S5 | Logs / backups / recuperación | trazas, evidencia, state | redacción de PII, state privado cifrado, `prevent_destroy` |
| S6 | Contraseña / sesiones / borrado de cuenta | identidad y tokens | Argon2id, refresh rotatorio, detección de replay, logout distribuido, borrado lógico |

## 2. STRIDE por superficie

| Amenaza | S1 | S2 | S3 | S4 | S5 | S6 |
|---|---|---|---|---|---|---|
| **S**poofing | JWT falsificado | origen no-CloudFront | robo de sesión Redis | rol OIDC suplantado | acceso a state | login/refresh |
| **T**ampering | payload manipulado | tráfico interno | escritura concurrente | workflow alterado | logs/state | tokens |
| **R**epudiation | falta de traza | sin logs ALB | sin auditoría de escritura | sin evidencia CI | retención | logout no registrado |
| **I**nformation disclosure | PII en respuesta | ALB público | PII en claves | secretos en logs | state/logs | enumeración de cuentas |
| **D**oS | body bombing / rate | floods | hot partition | workflow abusivo | disco de logs | fuerza bruta |
| **E**levation | IDOR / rol | SSRF interno | cross-usuario | permisos IAM amplios | lectura de secretos | refresh replay |

## 3. Riesgos priorizados

| Riesgo | Vector | Mitigación implementada | Estado |
|---|---|---|---|
| IDOR (acceso a recursos ajenos) | cambiar IDs en payload/URL | propiedad por `sub` del JWT en contexto; claves por usuario; pruebas de aislamiento | Parcial: endpoints E001–E075 pendientes |
| Token replay | reutilizar refresh token | rotación de un solo uso, detección de replay y revocación de familia | Implementado y probado |
| Ventana de access token tras replay | JWT válido hasta 10 min | revocación de refresh/familia; validación de sesión activa en casos de uso | **Riesgo residual**: el middleware valida firma/exp, no consulta sesión por request |
| Manipulación monetaria | alterar importes/roles desde cliente | importes `int64` en centavos, el cliente no declara saldo, validación servidor | Implementado |
| SSRF | URLs de terceros en backend | backend no realiza peticiones a URLs de usuario; sin proxy | Implementado por ausencia de función |
| Fuerza bruta / enumeración | login, recuperación | rate limit fail-closed, mensajes homogéneos | Implementado y probado |
| DoS por cuerpo | payload gigante | `MaxBytesReader` 1 MB, 413/error controlado | Implementado y probado |
| Secretos expuestos | commits, logs, state | gitleaks, redacción, state fuera de Git | Implementado; RFC-004 pendiente para bootstrap |
| Escaneo ofensivo accidental | DAST | sólo entorno aislado con autorización; no ejecutado | **NOT_RUN / BLOQUEADO** |

## 3.1 Matriz verificable de amenazas y controles

Leyenda de estado: **IMPLEMENTADO** (control y prueba locales), **PARCIAL** (control de componente, sin
endpoint), **PENDIENTE** (requiere endpoint o AWS), **BLOQUEADO** (decisión/Gate externo).

| ID | Superficie | Amenaza | Spec/ruta afectada | Control | Prueba (P/N) | Evidencia | Sev. | Responsable | Estado |
|---|---|---|---|---|---|---|---|---|---|
| T-01 | S1 | IDOR entre usuarios | E001–E075 (M/S/E/O) | `sub` del JWT en contexto; claves por usuario; sin confianza en IDs del cliente | P: contexto verificado; N: token ajeno/ausente | `tests/security`, middleware | Alta | Backend/Sec | PARCIAL |
| T-02 | S1 | JWT manipulado / `alg=none` | A-050, `/api/v1/auth/*` | RS256, rechazo de algoritmo y firma | P: token válido; N: payload/firma alterados | `TestJWTRechaza...` | Crítica | Sec | IMPLEMENTADO |
| T-03 | S1 | JWT expirado | A-050 | `exp` validado | N: token expirado → 401 | `TestJWTExpiradoSeRechaza` | Alta | Sec | IMPLEMENTADO |
| T-04 | S6 | Refresh replay | A-050 | rotación de un solo uso, revocación de familia | N: replay → revocación | `TestSesionLogoutYReplayRevocan` | Crítica | Sec | IMPLEMENTADO |
| T-05 | S1 | Acceso sin autenticación | A-050 | middleware exige Bearer válido | N: sin token/formato → 401 | `TestAuthMiddleware...` | Alta | Sec | IMPLEMENTADO |
| T-06 | S3 | Manipulación financiera | A-020/A-070, E001–E075 | importes `int64` en centavos; servidor calcula; idempotencia | P: aritmética exacta; N: clave repetida con otro payload → 409 | `tests/unit/financial` | Crítica | Backend | PARCIAL |
| T-07 | S3 | Carrera de idempotencia | A-070 | lock atómico por clave/usuario | N: 20 concurrentes → 1 procede | `TestIdempotenciaConcurrenteYConflictos` | Alta | Backend | IMPLEMENTADO |
| T-08 | S3 | Acceso a recursos de terceros | E001–E075 | propiedad por usuario en claves y casos de uso | P: aislamiento de idempotencia por usuario | `TestIdempotencia...` | Crítica | Backend | PARCIAL |
| T-09 | S1 | DoS por body | A-010/A-040 | `MaxBytesReader` 1 MB, JSON estricto | N: cuerpo > 1 MB rechazado | `TestBodyGrandeSeRechaza` | Media | Backend | IMPLEMENTADO |
| T-10 | S1 | Fuerza bruta / rate | A-080 | rate limit distribuido, 429 + Retry-After | N: tercera petición → 429 | `TestRateLimiter...` | Alta | Backend | IMPLEMENTADO |
| T-11 | S1 | CORS abierto | A-030 | allowlist explícita, sin wildcard | N: origen no autorizado sin ACAO | `TestCORS...` | Media | Backend | IMPLEMENTADO |
| T-12 | S1 | Cabeceras débiles | A-030 | nosniff, DENY, HSTS, no-store | P: cabeceras presentes | `TestCabecerasDeSeguridad` | Baja | Backend | IMPLEMENTADO |
| T-13 | S2 | Origen no-CloudFront | I-030/I-010 | ALB interno, SG a prefix list, sin IPv4 pública | P: contrato Terraform; N: SG público (escáner IaC) | `tests/infra/terraform` | Alta | Infra | IMPLEMENTADO (diseño) |
| T-14 | S2 | SSRF | Backend | no hay proxy ni fetch a URLs de usuario | P: ausencia de función; N: n/a | modelo de amenazas | Media | Backend | IMPLEMENTADO por ausencia |
| T-15 | S4 | Suplantación OIDC | I-060/I-090 | subject por entorno, audience exacta, sin claves estáticas | P: contrato IAM/OIDC; N: workflow sin OIDC salvo CD | `tests/infra/terraform`, `tests/infra/cd` | Crítica | Infra/Sec | PARCIAL (AWS_NOT_RUN) |
| T-16 | S4 | Sustitución de artefacto | I-080/I-090/I-100 | SHA-256, SBOM del mismo run, S3 `If-None-Match`, digest en LT | P: integridad verificada; N: digest distinto → fail | `tests/infra/cd`, helpers | Crítica | Sec/Infra | IMPLEMENTADO (sin AWS) |
| T-17 | S5 | Secreto en logs/artefactos | A-010/A-100 | redacción de PII, gitleaks, sin state en artefactos | N: reglas de gitleaks y baseline acotado | CI + `tests/infra/ci` | Crítica | Sec | IMPLEMENTADO |
| T-18 | S5 | State/backup expuesto | I-070 | bucket privado cifrado, `prevent_destroy`, state fuera de Git | P: contrato; N: `tfstate` rastreado → fail | escáner IaC | Alta | Infra | IMPLEMENTADO (diseño) |
| T-19 | S6 | Enumeración de cuentas | A-050 | mensajes homogéneos | P: mismo error para usuario/clave inválidos | `tests/unit/security` | Media | Sec | IMPLEMENTADO |
| T-20 | S6 | Borrado de cuenta incompleto | GOV-07/E-specs | borrado lógico y revocación; cascada pendiente de endpoints | P: estado DELETED; N: no implementado a nivel datos | `tests/unit/domain` | Alta | Backend | PENDIENTE |
| T-21 | S5 | Escaneo ofensivo accidental | I-100 | DAST sólo con Gate, objetivo aislado, datos sintéticos | N: harness sin Gate → BLOCKED | `test_dast_y_gosec` | Alta | Sec | IMPLEMENTADO (sin ejecutar) |
| T-22 | S4 | Dependencia comprometida | I-080/I-100 | govulncheck, SBOM, acciones por SHA | P: CI; N: CVE simulada en pruebas de política | CI + criterios | Alta | Sec | IMPLEMENTADO |

### 3.2 Relación con E001–E075

Los 75 contratos están **documentados** en `sdd/03-endpoints` y validados por `validar_h06.py` (33 M,
23 S, 12 E, 7 O). **No hay endpoints HTTP implementados**: las pruebas T-01, T-06 y T-08 son de
componentes (middleware, dominio, idempotencia), no de la API completa. Cualquier caso que requiera un
endpoint se registra como `NOT_IMPLEMENTED / NOT_RUN` y no se declara probado.

## 4. Límites declarados

- No hay endpoints E001–E075 implementados: las pruebas de autorización por recurso son de componentes
  (middleware, sesión, idempotencia), no de la API completa.
- DAST no se ejecuta: requiere entorno aislado autorizado, datos sintéticos y ventana (INF-100-04).
- WAF ausente bajo RFC-006 (temporal); H-01 (TLSv1 en el borde) abierto.
- La verificación de identidad de proceso en instancias sigue bloqueada (I-090).
- `AWS_NOT_RUN`: ningún control de este modelo se declara verificado en cloud.
