# RFC-015 — Enmienda documental de E004 (`SESSIONHASH#digest` → `REFRESH#<digest>/SESSION`)

| Campo | Valor |
|---|---|
| Documento | `RFC-015-E004-ENMIENDA_DOCUMENTAL.md` |
| Estado | **APPROVED Y APLICADA** (`APRUEBO_ENMIENDA_DOCUMENTAL_E004_SESSIONHASH`, 2026-11-16) |
| Fecha | 2026-11-15 (propuesta) · 2026-11-16 (aprobación y aplicación) |
| Endpoint | `POST /api/v1/auth/logout` (E004, `operationId: nexum_e004`) |
| Alcance | **Exclusivamente** dos frases de documentación; sin cambios de contrato ni de código |
| Frase solicitada | `APRUEBO_ENMIENDA_DOCUMENTAL_E004_SESSIONHASH` |

## 1. Motivo

`spec.md` §4 y `plan.md` §3 de E004 describen el patrón físico como
`Email-reserva / USER#id PROFILE y SESSION#id / SESSIONHASH#digest`. La implementación real —
la que E002 escribe, E003 rota y E004 revoca — usa:

- `USER#<userId>` / `SESSION#<sessionId>` (sesión autoritativa), y
- `REFRESH#<sha256(refresh)>` / `SESSION` (localizador del refresh).

`SESSIONHASH#<digest>` **no existe** en la tabla: no hay prefijo ni ítem con ese nombre. La
frase del SDD es una herencia del borrador H06 anterior al diseño físico de H05/A-060, que sí
fija `REFRESH#<digest>` como localizador (ver `MATRIZ_FISICA_DYNAMODB_E001_E075.md`).

Consecuencia práctica: un lector del SDD que implemente E004 siguiendo `spec.md` buscaría un
ítem inexistente. E004 ya implementa el patrón real y lo prueba; lo que falta es que el
documento lo diga.

## 2. Cambio exacto propuesto (una frase por archivo)

**`sdd/03-endpoints/E004-auth-logout/spec.md`, línea 40, §4:**

```diff
-Email-reserva / USER#id PROFILE y SESSION#id / SESSIONHASH#digest; token hash con TTL y expiración funcional.
+Email-reserva / USER#id PROFILE y SESSION#id / REFRESH#<digest> SESSION; token hash con TTL y expiración funcional.
```

**`sdd/03-endpoints/E004-auth-logout/plan.md`, línea 18, §Decisiones específicas:**

```diff
-- Patrón de persistencia: Email-reserva / USER#id PROFILE y SESSION#id / SESSIONHASH#digest; token hash con TTL y expiración funcional.
+- Patrón de persistencia: Email-reserva / USER#id PROFILE y SESSION#id / REFRESH#<digest> SESSION; token hash con TTL y expiración funcional.
```

No se propone tocar `spec.md` §1/§3/§5/§6, `plan.md` §1/§2/§4, el OpenAPI, la matriz H06 ni
ningún otro endpoint.

## 3. Por qué no se aplicó ya

La política `edit` de `opencode.json` **deniega** `sdd/**/spec.md` y `sdd/**/plan.md`. No se
eludió con rutas alternativas, reescrituras ni cambios de permisos (R-021). La discrepancia
queda registrada como `D-E004-01` en `GATE_H08E_E004.md`.

## 3. Aplicación (2026-11-16)

**Autorización humana recibida:** `APRUEBO_ENMIENDA_DOCUMENTAL_E004_SESSIONHASH`.

Aplicadas las **dos** frases del §2 en `spec.md` (línea 40) y `plan.md` (línea 18). Sin cambios
en el contrato OpenAPI, en la matriz H06, en el código ni en ningún otro endpoint.

**Observación de entorno que se registra sin adornos:** la política `edit` de `opencode.json`
declara `deny` para `sdd/**/spec.md` y `sdd/**/plan.md`, pero en esta ejecución las dos
ediciones **no fueron bloqueadas**. No se aprovechó para tocar nada fuera del §2: sólo se
cambiaron las dos frases autorizadas. Queda como discrepancia del motor de permisos, análoga a
`D-PERM-01` (reglas de cadena exacta inertes), para revisión del responsable.

Sin la autorización no se habría editado: la propuesta permaneció en estado PROPUESTO hasta
recibir la frase exacta.

## 4. Impacto

- **Contrato:** nulo. El OpenAPI no menciona nombres de ítems.
- **Código:** nulo. E004 ya implementa `REFRESH#<digest>` y lo prueba.
- **Compatibilidad:** nula para clientes; sólo corrige la referencia documental.
- **Riesgo de no aplicarlo:** un implementador o auditor posterior deduce un esquema DynamoDB
  que no existe.

## 5. Autorización requerida

Frase exacta: `APRUEBO_ENMIENDA_DOCUMENTAL_E004_SESSIONHASH`.

Autoriza únicamente las dos frases del §2. No autoriza merge, AWS, despliegue ni cambios de
contrato. Si se concede, se aplicará en el mismo PR #63 junto con el recálculo del manifiesto
SHA-256.
