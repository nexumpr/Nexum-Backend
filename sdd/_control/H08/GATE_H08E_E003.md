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
- **Cambios de contrato protegidos** (`OpenAPI`, `spec.md`, `plan.md`): exigen
  `APRUEBO_RFC_014_E003_REFRESH`. Sin esa frase, el OpenAPI de E003 queda intacto y el
  runtime actual (401 + `deviceId` + `session.id`) **no puede fusionarse ni publicarse**.
- AWS mutante, `terraform plan/apply/destroy`, publicación S3, Launch Template, ASG,
  CloudFront, ALB, IAM, secretos y presupuesto.
- E004..E075 y cualquier refactor ajeno a E003.
- Declarar `E003_DEPLOYED`, `E003_AWS_VERIFIED` o `E003_USER_ACCEPTED`.

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
| Alineación de formato | Verificada con comprobador de columnas propio; **gofmt no ejecutable** en este host |
| `gofmt` / `go build` / `go vet` / `go test` locales | **BLOCKED_POLICY_APPLICATION_CONTROL**: Device Guard bloquea `gofmt.exe` y el toolchain 1.27.4 que exige `go.mod`; la instalación local es 1.27.3. No se declara PASS |
| Validación autoritativa | **CI Linux del PR** |
| RFC-014 | **PROPUESTO — pendiente de `APRUEBO_RFC_014_E003_REFRESH`** |
| Merge | **BLOQUEADO** hasta Gate de merge por PR + SHA40 |
| AWS / despliegue | `AWS_NOT_RUN`, `E003_DEPLOYED=NOT_RUN` |

**No se declara** `IMPLEMENTED` como verificado, ni `AWS_VERIFIED`, ni aceptación del
usuario. El bloqueo local se registra con su mensaje exacto y no se sustituye por una
afirmación de PASS.
