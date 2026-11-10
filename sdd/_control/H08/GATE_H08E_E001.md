# Gate H08-E / E001 — Registro de usuario (LOCAL)

**INSTRUCCIÓN HUMANA REGISTRADA (2026-11-10, sesión de implementación):**
«INICIAR IMPLEMENTACIÓN REAL H08-E / E001–E007 … Priorizar código y tests de E001 realmente
implementados … No hacer merge sin APRUEBO_MERGE_PR_N_SHA_SHA40.»

**ESTADO:** `AUTHORIZED_E001_LOCAL` — autoriza **exclusivamente** la implementación local de E001.
No autoriza E002–E007 en este PR, merge, despliegue, AWS mutante, Terraform, cambios de spec ni
publicación del contrato.

## 1. Motivo y procedimiento

H08-E figuraba `NOT_AUTHORIZED` en `sdd/_control/H07/ESTADO_Y_GATES.md`. No existía un Gate H08-E
firmado. La instrucción humana de esta sesión abre el mínimo necesario para E001 y se registra aquí
sin inventar una frase que el responsable no haya emitido. El merge sigue exigiendo una autorización
separada ligada a PR y SHA de 40 caracteres.

## 2. Alcance autorizado (E001)

- `POST /api/v1/auth/register` con `RegisterRequest` y `UserProfile` del OpenAPI 3.1 vigente.
- DTO estricto: `email`, `password`, `displayName`, `privacyNoticeVersion`, `privacyAccepted`.
- `Idempotency-Key` obligatoria, replay estable y conflicto 409 con payload distinto.
- Unicidad de email mediante **transacción atómica** de DynamoDB (`TransactWriteItems`), nunca
  `GetItem` + `PutItem` como comprobación de unicidad.
- Contraseña Argon2id; `emailVerified=false`; consentimiento y versión de aviso almacenados.
- Respuesta 201 con `UserProfile`; errores 400/409/422/429/503 y envelope `ErrorEnvelope`.
- Rate limiting de registro por IP con Redis real y fallo cerrado (503 si Redis no está disponible).
- Logs sin contraseña, hash, token ni correo.
- Pruebas unitarias, de contrato, de concurrencia, de fallo de DynamoDB, de rate limit y HTTP local
  con dos instancias compartiendo persistencia simulada.
- Commit, push, PR y seguimiento de CI. **Merge no autorizado.**

## 3. Fuera de alcance

- E002 login, E003 refresh, E004 logout, E005 forgot, E006 reset, E007 change: cada una requiere su
  propio Gate/PR y no se mezclan en este cambio.
- `terraform plan/apply/destroy`, AWS mutante, despliegue, TLS real, dos nodos AWS y DAST.
- Modificar `constitution.md`, `spec.md`, `plan.md` u OpenAPI.
- Declarar `HTTP_AWS_VERIFIED`, `TWO_NODES_AWS_VERIFIED`, `DEPLOYED_VERIFIED` o `PRODUCTION_READY`.

## 4. Decisiones de implementación

1. **Transacción única:** reserva `EMAIL#<sha256>` + perfil `USER#<id>` + registro de idempotencia
   `IDEM#<sha256(operación:key)>` en el mismo `TransactWriteItems`. Si la transacción se cancela, se
   lee el registro de idempotencia con `ConsistentRead=true` para decidir replay (mismo payload) o
   409 (payload distinto). Un fallo de condición de email produce 409 sin escritura parcial.
2. **Sin memoria en producción:** `cmd/server` compone únicamente el cliente DynamoDB real y el
   cliente Redis real. El almacén en memoria vive en `tests/support` y solo lo usan pruebas.
3. **Idempotencia compartida:** el registro vive en DynamoDB con TTL `expiresAt`; dos instancias
   comparten la misma decisión sin estado local.
4. **Rate limit fail-closed:** Redis `INCR`+`EXPIRE` atómico por script Lua; sin Redis, 503. El
   identificador de IP se hashea para no almacenar PII en claves.
5. **Contrato:** el detalle de error usa `field` y `reason`, como `ApiErrorDetail` del OpenAPI.
6. **Veracidad:** las pruebas de dos instancias son locales y usan persistencia compartida simulada;
   AWS, DynamoDB real y Redis real siguen `NOT_RUN` hasta el Gate de despliegue.

## 5. Entregables y criterio de cierre local

| Comprobación | Estado real (2026-11-10) |
|---|---|
| `go build ./...` | **PASS** (worktree y checkout integrado) |
| `go vet ./...` | **PASS** |
| `go test ./... -count=1` | **BLOCKED_POLICY_APPLICATION_CONTROL** en local: los binarios de prueba recién compilados son bloqueados por Control de aplicaciones de Windows (`fork/exec ... An Application Control policy has blocked this file`). No se declara PASS. La validación efectiva es el CI de Linux del PR. |
| `go test -race` | No ejecutado en local por el mismo bloqueo; CI lo ejecuta. |
| Pruebas de concurrencia e idempotencia | Implementadas; ejecución pendiente de CI. |
| Pruebas de contrato E001/OpenAPI | Implementadas; ejecución pendiente de CI. |
| `gofmt`, `staticcheck`, `gosec`, `govulncheck` | Pendientes de CI. |
| PR abierto con CI | Pendiente de esta ejecución. |
| Merge | **BLOQUEADO** hasta `APRUEBO_MERGE_PR_N_SHA_SHA40`. |

**No se declara** `IMPLEMENTED` hasta que el PR exista y CI esté verde; **no se declara**
`AWS_VERIFIED` en ningún caso desde este worktree. El bloqueo local se registra con su mensaje
exacto y **no** se sustituye por una afirmación de PASS.

## 6. Estado del worktree y trazabilidad

- Worktree: `C:\Proyectos\Nexum\Nexum-Backend-E001`, rama `feat/h08e-e001-register-real`.
- Base original `2958136`; integrado `main` `bf5cf8d` mediante merge normal (sin force push).
- Commit de implementación: `275f598`.
- El código vive en el repositorio; el worktree no es la fuente de verdad.

