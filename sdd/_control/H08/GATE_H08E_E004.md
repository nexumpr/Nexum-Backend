# Gate H08-E / E004 — Cierre de sesión actual (LOCAL)

**INSTRUCCIÓN HUMANA REGISTRADA (2026-11-15):** el responsable ordenó, en dos mensajes
sucesivos, (1) «Implementar E004 completamente, con revocación real, pruebas, CI y PR técnico
listo para merge» y (2) «Continuar código E004 ya iniciado, corregir defectos concretos y
concluir pruebas y PR sin reanalizar», con la advertencia de que si faltaba Gate se presentara
el alcance exacto sin asumir aprobación implícita y sin descartar lo ya escrito.

**ESTADO:** `AUTHORIZED_E004_LOCAL` — autoriza **exclusivamente** la implementación local de
E004 (`POST /api/v1/auth/logout`), sus pruebas, la documentación de evidencia y el PR técnico.

## 1. Autorizado

- Implementación de E004 en las capas existentes (`ports`, `application`, `adapters`,
  `transport`, `cmd/server`), reutilizando JWT RS256, DynamoDB y las sesiones reales de
  E002/E003. Sin tablas, índices, claves ni servicios nuevos.
- Caso de uso `Logout` con identidad derivada **sólo** de claims verificados (`sub`, `sid`).
- Revocación durable `ACTIVE -> REVOKED` con compare-and-swap de estado y versión, y cierre del
  localizador de refresh vigente en la misma transacción.
- Verificación de revocación en el middleware para rutas protegidas, con fallo cerrado 503.
- Handler `POST /auth/logout`: 204 sin cuerpo, rechazo de cualquier cuerpo, errores uniformes.
- Pruebas unitarias de aplicación, adaptador (con doble con estado que interpreta las
  condiciones emitidas) y HTTP; concurrencia aislamiento y reconciliación tras conflicto.
- Actualización de `tasks.md`, `checklist.md`, `memoria.md`, este Gate, el manifiesto SHA-256,
  commit, push de la rama y **un** PR técnico.

## 2. NO autorizado aquí

- **Merge**: exige `APRUEBO_MERGE_PR_<N>_SHA_<SHA40>` sobre el HEAD exacto.
- AWS mutante, `terraform plan/apply/destroy`, publicación S3, Launch Template, ASG,
  CloudFront, ALB, IAM, secrets y presupuesto. Nada de la FASE 5 se ejecutó.
- E005..E075 y cualquier refactor ajeno a E004.
- Declarar `E004_DEPLOYED`, `E004_AWS_VERIFIED` ni `E004_USER_ACCEPTED`.
- Modificar `sdd/**/spec.md` o `sdd/**/plan.md`: la política `edit` los **deniega**
  (`opencode.json`). No se eludió (R-021); las discrepancias se registran en §4 para decisión
  humana.

## 3. Decisiones de implementación

1. **Contrato suficiente, sin cambios**: el OpenAPI vigente de `nexum_e004` ya declara
   `POST /api/v1/auth/logout`, `BearerAuth`, **sin `requestBody`**, `204` sin contenido y los
   códigos `400/401/403/404/409/422/429/500/503`. No se modificó YAML ni JSON. Paridad
   verificada: 75 operaciones, 55 rutas, 102 esquemas; comparador de compatibilidad
   `COMPATIBLE` con 0 rupturas.
2. **Identidad**: `sub` y `sid` salen del JWT validado. Un token sin `sid` o sin `sub` es 401.
   No se aceptan `userId`, `sessionId`, `refreshToken` ni `allDevices` del cliente.
3. **Sin comparar `jti`**: E003 rota `accessJti` en cada refresh; compararlo invalidaría access
   tokens todavía válidos de una sesión activa. La autorización se decide por estado de sesión,
   no por token (T19).
4. **CAS de estado y versión**: condición
   `attribute_exists(PK) AND #status = :active AND #version = :expected`, con `ADD #version :one`.
   Un conflicto relee el estado vigente y revoca la versión y el digest nuevos (máximo 3
   intentos). Un ítem heredado **sin** `version` (anterior a E002 actual) se revoca con CAS de
   estado y la versión se inicializa a 1: no tiene rotador posible porque E003 exige el atributo.
5. **Localizador de refresh**: se revoca `REFRESH#<refreshHash>/SESSION` en la misma
   transacción. Si el TTL lo purgó, la transacción se cancela completa y se reintenta **solo** la
   sesión (la sesión es la autoridad). Nunca se confirma 204 sin revocación durable.
6. **Rutas protegidas**: `AuthenticateBearerWithSession` es la forma obligatoria; sin
   autorizador la ruta responde 503 en lugar de autorizar solo con la firma. `NewLogoutHandler`
   exige servicio, tokens y autorizador; si falta cualquiera, 503.
7. **Segundo cierre**: la sesión ya revocada produce 401 sin nuevas mutaciones (efectos
   idempotentes, respuesta no idempotente), y el refresh posterior es 401.
8. **Sin memoria en producción**: `cmd/server` compone DynamoDB real; si falta el cliente se
   pasa un interface nil para fallar cerrado con 503 en lugar de operar sobre un puntero nil.

## 4. Discrepancias documentales registradas (no corregidas por política)

| # | Discrepancia | Estado |
|---|---|---|
| D-E004-01 | `spec.md` §4 y `plan.md` §3 citan `SESSIONHASH#digest`; la implementación real de E002/E003 usa `REFRESH#<digest>/SESSION` | **RESUELTA**: RFC-015 **aprobada y aplicada** (2026-11-16) con `APRUEBO_ENMIENDA_DOCUMENTAL_E004_SESSIONHASH`; sólo las dos frases autorizadas |
| D-E004-02 | El PDF del prototipo encabeza la respuesta como HTTP 201 | **No aplicado**: el contrato OpenAPI y la instrucción humana fijan **204 sin cuerpo**; se trata como error editorial del PDF |
| D-E004-03 | El SDD menciona `allDevices` y `refreshToken` en el cuerpo | **No implementado**: `allDevices` es opcional y no se habilita; el `refreshToken` no es necesario porque la sesión se deduce del `sid` |

## 5. Estado real de la verificación (2026-11-15)

| Comprobación | Estado |
|---|---|
| Implementación local | **COMPLETADA** (4 archivos nuevos de producción, 2 modificados, 3 suites de prueba) |
| `gofmt` / `go build` / `go vet` | **PASS** en local |
| `go test ./... -count=1` | **PASS** (14 paquetes) |
| `go test -race` local | **NOT_RUN — BLOCKED_TOOLCHAIN**: `-race` exige cgo y este host Windows no tiene `gcc`. Autoridad: el job de CI en Linux |
| Contrato OpenAPI | `H06_CONTRACT_PASS` (75 operaciones) · paridad 75/55/102 · comparador `COMPATIBLE` con 0 rupturas |
| Merge | **BLOQUEADO** hasta Gate de merge por PR + SHA40 |
| AWS / despliegue | `AWS_NOT_RUN`, `E004_DEPLOYED=NOT_RUN`, `E004_AWS_VERIFIED=NOT_RUN` |

**No se declara** `E004_DEPLOYED` ni `AWS_VERIFIED`; tampoco se declaran verificados dos nodos
AWS: la prueba de revocación entre instancias usa dos objetos independientes sobre el mismo
almacén, no dos máquinas en AWS.
