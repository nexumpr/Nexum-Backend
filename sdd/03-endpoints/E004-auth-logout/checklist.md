# Checklist de calidad — E004

**Diseño H06:** review. **Implementación local:** `IMPLEMENTED_LOCAL`. **Respuesta:** 204 sin
cuerpo. **AWS/HTTP público:** `NOT_RUN`. Gate: `sdd/_control/H08/GATE_H08E_E004.md`.

- [x] Gate local registrado por el responsable; no se confunde instalación con aprobación.
  _`AUTHORIZED_E004_LOCAL` (2026-11-15). El merge y el despliegue siguen requiriendo su propio Gate._
- [x] Método/path conservan el ID del inventario H04: `POST /api/v1/auth/logout`, `nexum_e004`.
- [x] Payload válido/negativo revisados: el contrato no admite cuerpo; cualquier contenido se rechaza con 400.
- [x] Respuesta `204` sin contenido y errores de OpenAPI revisados, sin cambios de contrato.
- [x] Semántica de propiedad y JWT: identidad derivada de `sub`/`sid` verificados; sin `sid` → 401.
- [x] Idempotencia: no exigida por el contrato; efectos del cierre idempotentes, segundo intento 401.
- [x] DynamoDB con `GetItem` consistente y `TransactWriteItems`: localizadores e índices existentes, **nunca `Scan`**.
- [x] Redis: no interviene en la autorización de revocación; ninguna caché puede autorizar una sesión revocada.
- [x] Montos decimales/cents y timestamps: no aplica a esta operación; `revokedAt`/`revokedAtISO` en UTC.
- [x] Pruebas de error 400/401/409/503 pertinentes al caso, con `ErrorEnvelope` y `traceId`.
- [x] PII y secretos ausentes en logs, respuestas y fixtures; no se persiste ni se registra el token.
- [x] Tests unitarios reales y contrato OpenAPI: `PASS` en local (`go test ./...`).
- [x] Integración Dynamo **real** y HTTP **público**: `PASS` por HTTPS el 2026-11-16
  (`GATE_H08E_E004_DESPLIEGUE_AWS.md` §5). La lectura directa de DynamoDB quedó `NOT_RUN` por
  permisos; la revocación se acredita funcionalmente (refresh 401 y logout 401).
- [x] API en dos instancias **AWS**: dos EC2 en LT **v5**, `InService`/`Healthy`, ASG 2/2/2 y dos
  targets `healthy`. TLS público operativo vía CloudFront. **No** se afirma atribución por nodo.
- [ ] `go test -race` en local: `NOT_RUN — BLOCKED_TOOLCHAIN` (sin gcc en Windows). Lo ejecuta CI:
  `PASS` en el run `43786668813`.
- [x] Gates de release aprobados antes de mutar AWS: publicación S3, `terraform apply` ligado al
  SHA-256 del plan e `instance refresh`, cada uno con su frase y su SHA.
- [x] Dependencias del cliente Mobile no constituyen Gate del Backend.

**Criterio de cierre:** ninguna casilla se marca por escribir el documento. `IMPLEMENTED_LOCAL`
no es `MERGED`, ni `DEPLOYED`, ni `AWS_VERIFIED`.
