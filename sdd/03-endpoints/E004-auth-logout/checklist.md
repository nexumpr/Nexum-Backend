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
- [ ] Integración Dynamo **real** y HTTP **público**: `NOT_RUN` (requiere despliegue autorizado).
- [ ] API en dos instancias **AWS**, TLS público, CI/CD e IaC: `NOT_RUN`. Las pruebas de dos
  instancias son lógicas, sobre el mismo almacén; no se declaran dos nodos AWS verificados.
- [ ] `go test -race` en local: `NOT_RUN — BLOCKED_TOOLCHAIN` (sin gcc en Windows); lo ejecuta CI.
- [ ] Gate individual de release H08 y rollback aprobados antes de `terraform apply`.
- [x] Dependencias del cliente Mobile no constituyen Gate del Backend.

**Criterio de cierre:** ninguna casilla se marca por escribir el documento. `IMPLEMENTED_LOCAL`
no es `MERGED`, ni `DEPLOYED`, ni `AWS_VERIFIED`.
