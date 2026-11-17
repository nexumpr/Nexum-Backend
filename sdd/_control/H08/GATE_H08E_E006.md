# Gate H08-E / E006 — Confirmar restablecimiento de contraseña

**ESTADO:** `AUTHORIZED_E006_LOCAL` (2026-11-17). Implementación local completada; PR y CI
pendientes al registrar este Gate. `AWS_NOT_RUN`: ningún despliegue, Terraform, IAM, DynamoDB
ni secreto modificado por esta unidad.

## 1. Autorización

| Campo | Valor |
|---|---|
| Frase humana | `INICIAMOS E006. PRIORIDAD ABSOLUTA: IMPLEMENTAR, FUSIONAR, DESPLEGAR Y VERIFICAR...` (instrucción de sesión 2026-11-17) |
| Alcance | Spec E006 y cambios compartidos de A6 estrictamente necesarios (E002/E003/middleware/JWT) |
| Excluido | E007..E075, H08-E general, rediseño de infraestructura, Docker, gasto personal |
| Aprobador | Responsable del proyecto |

## 2. Contrato

`POST /api/v1/auth/password/reset` — público, sin Bearer (OpenAPI `nexum_e006`).

| Elemento | Valor |
|---|---|
| Request | `PasswordResetRequest { token, newPassword }` |
| `newPassword` | 12–128 caracteres |
| Éxito | `200 Accepted { status: "accepted", requestId: UUID }` |
| Errores | 400 (token inexistente/expirado, JSON inválido), 409 (token usado, conflicto), 422 (validación), 429 (cuota), 500, 503 |
| Persistencia | `USER#<id>/PROFILE` y `RESET#<sha256(token)>/RESET`, sin `Scan` |

## 3. Transacción

Una sola `TransactWriteItems` con dos acciones sobre dos ítems distintos:

1. **PROFILE:** `SET passwordHash, passwordChangedAt, sessionEpoch ADD version :one`
   (o `SET version = :one` en perfiles legados), condición `attribute_exists(PK) AND
   status=:active AND version=:expected` (o `attribute_not_exists(version)`) **y** época
   vigente `(attribute_not_exists(sessionEpoch) OR sessionEpoch=:expectedEpoch)`.
2. **RESET:** `SET used=true, usedAt`, condición `attribute_exists(PK) AND
   (attribute_not_exists(used) OR used=false) AND expiresAt > :now`.

Los valores de expresión no utilizados se omiten por rama: DynamoDB rechaza
`ExpressionAttributeValues` con claves sobrantes. Un fallo de condición no modifica ninguno de
los dos ítems; dos solicitudes concurrentes con el mismo token producen un solo 200.

## 4. A6

Ver `RFC-018-A6-EPOCA_REVOCACION.md`. Resumen: `sessionEpoch` en perfil, claim `epo` en JWT,
persistencia en sesión, comparación en refresh (con `ConditionCheck` atómico) y en middleware
con lectura consistente; `epo` ausente = 0.

## 5. Archivos

Nuevos: `internal/application/identity/password_reset_confirm.go`,
`internal/adapters/dynamo/password_reset_confirm_store.go`,
`internal/transport/http/password_reset_handler.go`,
`internal/infrastructure/cache/password_reset_confirm_redis.go`, `cmd/server/e006.go`.

Modificados: `cmd/server/main.go`, `internal/ports/{auth,session}.go`,
`internal/infrastructure/security/{token,issuer}.go`,
`internal/application/identity/{login,refresh}.go`,
`internal/adapters/dynamo/{session_store,refresh_store,logout_store}.go`,
`internal/transport/http/middleware/auth.go` y dobles de prueba.

Pruebas: `tests/unit/dynamo/password_reset_confirm_store_test.go`,
`tests/unit/application/password_reset_confirm_test.go`,
`tests/unit/transport/password_reset_confirm_test.go`,
`tests/unit/transport/a6_epoch_test.go`.

## 6. Verificaciones

| Verificación | Estado | Evidencia |
|---|---|---|
| `go build ./...` | PASS local | 2026-11-17 |
| `go vet ./...` | PASS local | 2026-11-17 |
| `gofmt` | PASS local | aplicado |
| `go test` | NOT_RUN local — autoridad CI Linux | Windows Application Control bloquea binarios de prueba |
| CI del PR | pendiente | este Gate se actualizará con el run real |
| Despliegue AWS | NOT_RUN | requiere Gate de apply y de despliegue |

## 7. Validación dirigida del adaptador

- Tokens de E005 reales: el ítem `RESET#<sha256>/RESET` escrito por `Admitir` (con `used=false`,
  `expiresAt` numérico) es el que consume E006; prueba de extremo a extremo sobre el doble con
  estado que replica las condiciones emitidas.
- Caducidad lógica explícita: `expiresAt > now`, sin depender de la purga TTL.
- Perfiles legados: sin `version` (se inicializa a 1) y sin `sessionEpoch` (0 → 1).
- `passwordChangedAt` se escribe como RFC3339, igual que `createdAt`, `revokedAt` y `usedAt`.
- Redis: cuota de E006 con script Lua atómico (`INCR` + `PEXPIRE`), fail-closed.
- Gitleaks 8.30.1 sobre el árbol: ningún hallazgo en los archivos nuevos de E006.

## 8. Gates pendientes

```
APRUEBO_RFC_018_A6_EPOCA_REVOCACION
APRUEBO_MERGE_PR_<NUMERO_REAL>_SHA_<HEAD_REAL>
APRUEBO_APPLY_<PLAN>_SHA256_<SHA256_REAL>
```
