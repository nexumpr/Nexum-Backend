# Gate H08-E / E006 — Confirmar restablecimiento de contraseña

**ESTADO:** `MERGED + DEPLOYED_AWS + USER_ACCEPTED_CONFIRMED` (2026-11-17). PR #71 fusionado, CI verde, release desplegado en AWS y aceptación funcional confirmada por el responsable. `E006_USER_ACCEPTED = CONFIRMED`.

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
| CI del PR | **PASS** | PR #71: 13/13; run `44110769604` |
| CI de `main` | **PASS** | run `44116124273` |
| Despliegue AWS | **VERIFIED** | release `v0.13.0+578b0db3b9bb`, LT v8, ASG 2/2/2, 2 targets healthy |

## 7. Validación dirigida del adaptador

- Tokens de E005 reales: el ítem `RESET#<sha256>/RESET` escrito por `Admitir` (con `used=false`,
  `expiresAt` numérico) es el que consume E006; prueba de extremo a extremo sobre el doble con
  estado que replica las condiciones emitidas.
- Caducidad lógica explícita: `expiresAt > now`, sin depender de la purga TTL.
- Perfiles legados: sin `version` (se inicializa a 1) y sin `sessionEpoch` (0 → 1).
- `passwordChangedAt` se escribe como RFC3339, igual que `createdAt`, `revokedAt` y `usedAt`.
- Redis: cuota de E006 con script Lua atómico (`INCR` + `PEXPIRE`), fail-closed.
- Gitleaks 8.30.1 sobre el árbol: ningún hallazgo en los archivos nuevos de E006.

## 8. Gates consumados

```
APRUEBO_RFC_018_A6_EPOCA_REVOCACION
APRUEBO_MERGE_PR_71_SHA_9c9e9855a834e5c42f49fefb59501346e1449286
APRUEBO_APPLY_LT_IAM_E006_SHA256_90988C7CB5D49F62498C6FDF00DF7AC5BA6349D034D392060880DD52598C2D63
```

No queda ningún Gate de implementación, merge o apply pendiente para E006. La evidencia
individual de replay, revocación y concurrencia en AWS sigue pendiente y no se declara PASS.

## 9. Aprobaciones humanas recibidas (2026-11-17)

```
APRUEBO_RFC_018_A6_EPOCA_REVOCACION
APRUEBO_MERGE_PR_71_SHA_9c9e9855a834e5c42f49fefb59501346e1449286
```

- HEAD verificado antes de fusionar: `9c9e9855a834e5c42f49fefb59501346e1449286`.
- CI del PR: **13/13 SUCCESS** (Go Backend CI run `44113138268`, SDD `44113138225`).
- Sin cambios de código posteriores a la aprobación. El Gate de apply se consumió con el plan
  `90988C7CB5D49F62498C6FDF00DF7AC5BA6349D034D392060880DD52598C2D63` (0 add · 4 change · 0 destroy).

## 10. Cierre de despliegue y aceptación (2026-11-17)

| Campo | Valor |
|---|---|
| Merge | PR #71 MERGED; `main` `578b0db3b9bb765afc981a86b23c6ade10ed80ea` |
| CI de `main` | Go Backend CI `44116124273` SUCCESS; SDD `44116124463` SUCCESS |
| Release | `v0.13.0+578b0db3b9bb`; binario SHA-256 `6b87752f…bb522`; unidad `6fa4d5fb…05cc` |
| Apply | `APRUEBO_APPLY_LT_IAM_E006_SHA256_90988C7C…2D63`; **0 add · 4 change · 0 destroy** |
| Launch Template | `lt-05445f32a7ee8a2f7` **v8** |
| IAM | 16 objetos S3 de artefactos (rollback conservado) |
| Flota | `i-01b1a4606bd773838` e `i-07c1c2facc51aa2a1`, LT v8, `InService/Healthy`, ASG 2/2/2, 2 targets healthy |
| HTTPS | E006 400/422/400 sobre CloudFront; 404 con nodos v7 antes del release; health 200 |
| Regresión | E001 400, E002 401, E003 401, E004 401, E005 422 (negativas dirigidas; sin cambios de estado) |
| Aceptación | `E006_USER_ACCEPTED = CONFIRMED` (declaración del responsable: restablecimiento real comprobado en Postman) |

**Evidencia individual pendiente (no declarada PASS):** reutilización del token (409),
revocación de access/refresh anteriores (401) y concurrencia real en AWS no cuentan con
evidencia independiente registrada; se conservan como pendientes. La aceptación funcional
confirmada no se extrapola a esas variantes.

**Limitación declarada:** ambas instancias quedaron en `us-east-2a` por falta de capacidad
`t4g.micro` en `us-east-2b`; el ASG conserva ambas subredes y recuperará multi-AZ cuando la
capacidad esté disponible.
