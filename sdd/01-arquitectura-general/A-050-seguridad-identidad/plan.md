# A-050 — Plan técnico de arquitectura

**Estado:** DRAFT H04. **Entrada obligatoria:** `spec.md` y fuentes de A-050. **Salida:** decisión lista para implementar después de H07; este plan NO ejecuta infraestructura ni código.

## 1. Estrategia de construcción

1. **Registro e identidad.** Definir interfaces, límites, precondiciones, postcondiciones, fallos, autorización y pruebas correspondientes. Email canónico normalizado y unicidad con item reservado vía condición; Argon2id parámetros iniciales m=64MiB t=3 p=2 (calibrar bajo dos instancias, control DoS), sal aleatoria per usuario; no almacenar contraseña reversible.
2. **JWT.** Definir interfaces, límites, precondiciones, postcondiciones, fallos, autorización y pruebas correspondientes. Access RS256 con kid/aud/iss/sub/jti/exp/iat/nbf, duración sugerida 10 min; allowlist de algoritmos, key rotation y JWKS/firma controlada; no aceptar tokens con none ni claims de userId enviados por cliente.
3. **Refresh.** Definir interfaces, límites, precondiciones, postcondiciones, fallos, autorización y pruebas correspondientes. Refresh opaco aleatorio 256-bit, solo hash/HMAC en DynamoDB; TTL sugerido 30 días y rotación cada uso mediante CAS/condición, detectar replay y revocar familia, 401 sin filtrar detalles; revocación visible en ambos nodos.
4. **Logout y revocación.** Definir interfaces, límites, precondiciones, postcondiciones, fallos, autorización y pruebas correspondientes. E004 revoca sessionId/family de forma durable y desasocia push del dispositivo; access JWT corto y mecanismo revocation/version para endpoints sensibles. La app elimina tokens locales pero no sustituye revocación en servidor.
5. **Recuperación y correo.** Definir interfaces, límites, precondiciones, postcondiciones, fallos, autorización y pruebas correspondientes. E005/E006 token de un uso hash con caducidad, respuesta neutra y rate limit, sin user enumeration; verificación de correo idempotente y firma de enlaces en proveedor aún por definir H05.
6. **Autorización de recursos.** Definir interfaces, límites, precondiciones, postcondiciones, fallos, autorización y pruebas correspondientes. userId deriva de sub; cada consulta DynamoDB usa pk por usuario derivado; 404 si no existe o es de tercero; authorization por función, jamás confiar en role/userId del payload.
7. **Cuenta y privacidad.** Definir interfaces, límites, precondiciones, postcondiciones, fallos, autorización y pruebas correspondientes. DELETE /users/me exige contraseña y confirmación ELIMINAR, revoca sesiones, programa borrado real de recursos y versiones, retención/backups documentados. No usar TTL como única garantía de borrado inmediato.
8. **Secretos y auditoría.** Definir interfaces, límites, precondiciones, postcondiciones, fallos, autorización y pruebas correspondientes. Secrets fuera de Git, logs sin hashes/refresh/access/password, PII redacted; acceso AWS de CI con OIDC y rol mínimo; audit eventos auth sin contenido financiero. Aviso de privacidad aprobado por responsable legal.

## 2. Orden interno de diseño y dependencias

1. Revisar `constitution.md` y `AGENTS.md` antes de aceptar cambios; cotejar los PDF/prototipo cuyos nombres y hashes se registran en H04.
2. Identificar invariantes y entradas/salidas de cada módulo; mapear al catálogo E001..075 sin crear contratos endpoints H06.
3. Verificar acoplamiento con A-020 (capas), A-040 (DTOs), A-050 (auth), A-060 (persistencia), A-070 (consistencia), A-080 (Redis), A-090 (telemetría) y A-100 (QA).
4. Mantener como *blocker de H05* capacidad/costo de 2 instancias + balanceador + Redis, restricciones SCP/IAM, TLS y mecanismos de on/off; no inferir disponibilidad de AWS Free.
5. Registrar la decisión humana y conflictos si alguna regla altera el negocio del prototipo; no solicitar gate Mobile.

## 3. Estrategia de pruebas posteriores por capa

| Nivel | Diseño a comprobar | Evidencia futura obligatoria |
|---|---|---|
| Dominio | Invariantes y money/identidad | go test, race detector, entradas límite |
| Aplicación | Orquestación con mocks y transacciones | unit tests, casos negativos, retries |
| Infraestructura | DynamoDB/Redis reales según fase | integración, latencia, permisos mínimos, gasto |
| HTTP | Schemas OpenAPI 3.1 y respuestas | pruebas contractuales y HTTP reales |
| CI/CD | Seguridad y releases | PR checks, escaneo, autorización humana |

## 4. Riesgos y mitigación explícita

- Exactitud del flujo de borrado y retención legal requiere aprobación responsable del dato. **Mitigación:** resolver antes de aprobar su dependencia; si requiere cambio de constitución emitir RFC.
- Recuperación requiere proveedor de correo/OTP viable bajo cuenta AWS en H05. **Mitigación:** resolver antes de aprobar su dependencia; si requiere cambio de constitución emitir RFC.

## 5. Criterio de cierre

Se considera listo para revisión cuando los 8 ejes técnicos de `spec.md` tienen trazabilidad a pruebas en `checklist.md`, los elementos futuros permanecen `NOT_RUN`, la matriz de alcance está intacta y las decisiones pendientes tienen dueño/fase, sin `APPROVED` automático.
