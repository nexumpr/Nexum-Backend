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

## 4. Límites declarados

- No hay endpoints E001–E075 implementados: las pruebas de autorización por recurso son de componentes
  (middleware, sesión, idempotencia), no de la API completa.
- DAST no se ejecuta: requiere entorno aislado autorizado, datos sintéticos y ventana (INF-100-04).
- WAF ausente bajo RFC-006 (temporal); H-01 (TLSv1 en el borde) abierto.
- La verificación de identidad de proceso en instancias sigue bloqueada (I-090).
- `AWS_NOT_RUN`: ningún control de este modelo se declara verificado en cloud.
