# Gate H08-E / E003 — Refresh de tokens (LOCAL)

**INSTRUCCIÓN HUMANA REGISTRADA (2026-11-13):** «APRUEBO_ALCANCE_LOCAL_E003_AUTH_REFRESH»,
acompañada de la instrucción operativa de implementar E003 de inmediato, respetando el SDD y
los Gates existentes, sin autorizar merge, mutaciones AWS, Terraform apply, publicación de
artefactos, instance refresh, cambios contractuales protegidos sin RFC/Gate ni
modificaciones de otros endpoints.

**ESTADO:** `AUTHORIZED_E003_LOCAL` — autoriza **exclusivamente** la implementación local de
E003 (`POST /api/v1/auth/refresh`), sus pruebas, documentación de evidencia y el PR técnico.

## 1. Autorizado

- Implementación de E003 en las capas existentes (`domain`, `application`, `adapters`,
  `infrastructure`, `transport`, `cmd/server`), reutilizando JWT RS256, DynamoDB, Redis y
  las sesiones reales de E002.
- Rotación de un solo uso por compare-and-swap en una única `TransactWriteItems`.
- Detección de reutilización con revocación durable de la familia.
- Validación de cuenta `ACTIVE` con lectura consistente y `ConditionCheck` en la transacción.
- Cuotas Redis con identificadores HMAC y fallo cerrado.
- Pruebas unitarias de aplicación, adaptador (con doble con estado que interpreta las
  condiciones emitidas) y handler; cadena A→B→C→D y concurrencia de dos nodos simulada.
- RFC-014 **propuesta**, tareas/checklist de E003, `memoria.md`, manifiesto SHA-256, commit,
  push de la rama y PR único.

## 2. NO autorizado aquí

- **Merge**: exige `APRUEBO_MERGE_PR_<N>_SHA_<SHA40>` sobre el HEAD exacto.
- AWS mutante, `terraform plan/apply/destroy`, publicación S3, Launch Template, ASG,
  CloudFront, ALB, IAM, secretos y presupuesto.
- E004..E075 y cualquier refactor ajeno a E003.
- Declarar `E003_DEPLOYED`, `E003_AWS_VERIFIED` o `E003_USER_ACCEPTED`.

## 2 bis. Autorización posterior recibida — contrato E003 (2026-11-13T18:18Z)

**Frase humana:** `APRUEBO_RFC_014_E003_REFRESH`.

Autoriza **exclusivamente** las tres modificaciones contractuales de E003 documentadas en
`sdd/_control/H08/RFC-014-E003-REFRESH_V602.md`:

1. `401 UNAUTHORIZED` para refresh token inválido, expirado o revocado.
2. `deviceId` opcional en `RefreshRequest`, sin cambiar `refreshToken` como obligatorio.
3. `session.id` como alias de `sessionId` en la respuesta `AuthTokens`.

Límites que se mantienen: no se altera E001, E002 ni otro endpoint; no cambia el `required`
de `AuthTokens`; `deviceId` nunca sustituye la autenticación por `refreshToken`; el merge
sigue requiriendo su propio Gate por PR + SHA40; no autoriza AWS mutante ni despliegue.

## 3. Decisiones de implementación

1. **Compatibilidad de localizadores**: E002 crea `REFRESH#<digest>/SESSION` **sin** atributo
   `used`; E003 lo crea con `used=false`. La condición de consumo acepta ambos
   (`attribute_not_exists(#used) OR #used = :false`) y rechaza `used=true`.
2. **Atomicidad**: cuatro acciones en una transacción — consumo del localizador, CAS de
   sesión (`#version` + `refreshHash` + `#status`), `ConditionCheck` de perfil `ACTIVE` y alta
   del localizador nuevo. No hay `GetItem`+`PutItem` como sustituto.
3. **Sin extensión de vida**: la expiración absoluta de la sesión no se renueva al rotar; el
   localizador nuevo hereda el mismo `expiresAt` (Number, TTL físico).
4. **Replay**: token ya consumido ⇒ revocación durable de la sesión de la familia y `409`;
   si la revocación no se confirma, `503` (nunca se informa una revocación que no ocurrió).
5. **Conflicto ≠ fallo**: `ConditionalCheckFailed` se clasifica releyendo el estado; un fallo
   de servicio se traduce en `503`.
6. **Sin memoria en producción**: `cmd/server` compone DynamoDB y Redis reales.

## 4. Estado real de la verificación

| Comprobación | Estado (2026-11-13) |
|---|---|
| Implementación local | **COMPLETADA** (7 archivos nuevos, 1 modificado) |
| CI del PR #61 | **13/13 `SUCCESS`** en run `43478609688` (HEAD `04799eb`): gofmt, vet, staticcheck, build, `go test`, `-race`, fuzzing, cobertura 91.4 %, govulncheck, gosec, gitleaks, Semgrep, integración, contratos, IaC y SBOM |
| RFC-014 | **APPROVED** (`APRUEBO_RFC_014_E003_REFRESH`, 2026-11-13T18:18Z) y aplicada en YAML/JSON |
| Validación tras el contrato | `H06_CONTRACT_PASS` · paridad 75 ops/55 rutas/102 esquemas · comparador `COMPATIBLE` (0 rupturas) |
| Merge | **BLOQUEADO** hasta Gate de merge por PR + SHA40 |
| AWS / despliegue | `AWS_NOT_RUN`, `E003_DEPLOYED=NOT_RUN` |

**No se declara** `E003_DEPLOYED` ni `AWS_VERIFIED`; la aceptación del usuario queda pendiente
de la prueba en Postman después del despliegue autorizado.
