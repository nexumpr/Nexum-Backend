# RFC-016 — Contratos E005–E075: clasificación y decisiones pendientes

| Campo | Valor |
|---|---|
| Documento | `sdd/_control/H08/RFC-016-CONTRATOS_E005_E075.md` |
| Estado | **PROPUESTO — NO APROBADO — NO APLICADO** |
| Fecha | 2026-11-16 |
| Base | `main` `8bc2548339d6fdef9211c27c157bf75568fa052d` (PR #64 fusionado) |
| Alcance | Diferencias entre el SDD/OpenAPI aprobado (H06) y la auditoría de prototipo V6.0.2 para E005–E075 |
| Autoriza | **Nada por sí mismo**: no modifica contratos, no habilita implementación ni despliegue |

> Este documento **no** sustituye los 71 Gates individuales de implementación. Su aprobación solo
> permitiría aplicar enmiendas contractuales acotadas mediante Gates de contrato específicos.

---

## 1. Problema

La auditoría del prototipo V6.0.2 declara expresamente que **no contrastó el SDD vigente**. El
contrato aprobado es el OpenAPI 3.1 de H06 (75 operaciones), que es la autoridad de transporte.
Contrastadas ambas fuentes, las **75 rutas y métodos coinciden**; las diferencias son de:

1. **Código de éxito** en 5 operaciones.
2. **Nombre o existencia de campos** en request/response de 60 y 62 operaciones respectivamente.
3. **Referencias documentales obsoletas** a un patrón de persistencia anterior.

Ninguna de esas diferencias es un defecto del backend desplegado: el backend implementa
E001–E004 y su contrato coincide con OpenAPI.

## 2. Método de verificación (reproducible)

Herramienta nueva y versionada: `herramientas/verificar_preparacion_e005.py`.

Separa cuatro capas para no confundir hechos con decisiones:

| Capa | Qué comprueba | Efecto |
|---|---|---|
| **Integridad** | método, ruta, `operationId`, código de éxito presente en `responses`, esquema de request y coincidencia, campos declarados en `spec.md` resolubles (objetos, arrays, `$ref`, `allOf`), parámetros `path`/`query`, carpeta y 4 documentos, ficha por operación | `FAIL` si falla |
| **Detección** | diferencia SDD ↔ prototipo | informativa |
| **Decisiones** | `sdd/_control/H08/DECISIONES_CONTRATO_E005_E075.json`: mientras está `PENDIENTE` el contrato debe seguir en el valor **vigente**; si está `APROBADA`, debe estar en el **objetivo** | `FAIL` si el contrato no respeta la decisión |
| **Deuda documental** | referencias `SESSIONHASH` en E005–E011 | pendiente; `FAIL` solo si la decisión está `APROBADA` y quedan referencias |

Resultado real de la ejecución (2026-11-16):

```
PREPARACION_CONTRATO_PASS: 71 operaciones; discrepancias=5;
decisiones=12 (aprobadas=0, pendientes=12); SESSIONHASH=28 referencias
```

Propiedades del verificador:

- Funciona **antes y después** de una enmienda: una discrepancia resuelta deja de detectarse y la
  decisión pasa a exigir el valor objetivo. No codifica las diferencias conocidas como invariantes.
- **No** considera error que un campo del prototipo no exista en el contrato: 149 candidatos de
  request y 501 de response son **candidatos de auditoría**, no defectos.
- Falla cerrado ante `$ref` irresoluble, esquema inexistente, JSON inválido o decisión con estado
  desconocido.

## 3. Clasificación

Criterio aplicado:

| Clase | Significado | Efecto contractual |
|---|---|---|
| **ESENCIAL** | Sin ello la operación no cumple su función o el invariante del SDD | Enmienda obligatoria antes de implementar esa operación |
| **COMPATIBILIDAD** | Cambia un comportamiento observable ya publicado o consumible por el cliente | Enmienda + análisis de compatibilidad + Gate |
| **OPCIONAL** | Campo aditivo, derivado o de presentación; el SDD puede expresarlo de otra forma | Sin enmienda; se decide al implementar la operación |
| **OBSOLETA** | Referencia documental a un modelo sustituido | Enmienda editorial con Gate; sin efecto en runtime |

### 3.1 Diferencias de código de éxito — COMPATIBILIDAD (5)

Todas verificadas por lectura del OpenAPI vigente, no por la matriz.

| ID | Operación | SDD (vigente) | Prototipo | Propuesta | Razón |
|---|---|---|---|---|---|
| E017 | `POST /api/v1/users/me/consents` | `201` | `200` | **Conservar `201`** | Crea un consentimiento: recurso nuevo |
| E031 | `DELETE /api/v1/movements/{movementId}` | `204` | `200` | **Conservar `204`** | Borrado idempotente sin cuerpo; el prototipo añade un resumen que el SDD no exige |
| E037 | `POST /api/v1/recurring-payments/{id}/payments` | `200` | `201` | **Adoptar `201`** solo si se confirma que crea un movimiento y existe Gate de contrato | Semántica REST de creación |
| E042 | `DELETE /api/v1/goals/{goalId}` | `204` | `200` | **Conservar `204`** | Borrado idempotente sin cuerpo |
| E063 | `POST /api/v1/notifications/read-all` | `202` | `200` | **Adoptar `200`** solo si `read-all` es síncrono; con cola o trabajo diferido se conserva `202` | `202` implica procesamiento diferido |

**Ninguna de estas propuestas está aprobada.** Cambiar un código de éxito de una operación ya
publicada exige RFC + Gate + análisis de compatibilidad con el cliente.

### 3.2 E005 — `POST /api/v1/auth/password/forgot`

Contrato vigente (autoridad):

```json
// request  EmailRequest   { "email": string (required, maxLength 254) }
// response 202 Accepted   { "status": "accepted", "requestId": uuid }
```

El prototipo propone `locale` en la entrada y `{accepted, message, retryAfterSeconds}` en la salida.

| Elemento | Clasificación | Motivo |
|---|---|---|
| `locale` en request | **OPCIONAL** | Sin él la operación funciona; el idioma del correo puede derivarse del perfil |
| `accepted`/`message`/`retryAfterSeconds` | **OPCIONAL** | Redundantes con `status`/`requestId`; `retryAfterSeconds` sería útil para el cliente pero es aditivo |
| Sustituir `status`/`requestId` | **COMPATIBILIDAD** | Rompe el contrato aprobado y el patrón de `requestId` usado para trazabilidad |
| Respuesta `202` | **ESENCIAL conservar** | El SDD exige respuesta indistinguible para correo existente e inexistente (no enumeración) |

**Decisión propuesta:** conservar íntegramente el contrato aprobado. `locale` y
`retryAfterSeconds` solo podrían añadirse como campos **opcionales** mediante Gate, sin retirar
`status` ni `requestId`.

> El bloqueo real de E005 no es de campos: es la **ausencia de proveedor de correo**. Ver
> `sdd/_control/H08/BLOQUEO_E005_CORREO.md`. Una respuesta `202` sin entrega real sería una
> declaración falsa y está prohibida por `constitution.md` §2.3 y §11.

### 3.3 E006 — `POST /api/v1/auth/password/reset`

| Elemento | SDD (vigente) | Prototipo | Clasificación |
|---|---|---|---|
| Token en request | `token` | `resetToken` | **COMPATIBILIDAD** (renombrar rompe clientes) |
| Respuesta | `Accepted {status, requestId}` | `{reset, revokedSessions}` | **COMPATIBILIDAD** |
| `revokedSessions` | no existe | número de sesiones revocadas | **ESENCIAL** para el invariante de revocación (GOV-06/R-033): el cliente debe poder confirmar que las sesiones murieron |

**No se cambia nada sin autorización.** Si se decide adoptar `resetToken`, debe hacerse con
`RFC` propia y versión de API o periodo de convivencia; renombrar un campo de request es una
ruptura dura.

### 3.4 E005–E011 — persistencia obsoleta — OBSOLETA (7 operaciones, 28 referencias)

`spec.md`, `plan.md`, `tasks.md` y `checklist.md` de E005–E011 citan `SESSIONHASH#digest`.
El modelo **real y verificado en producción** por E003/E004 es:

- `USER#<userId>` / `SESSION#<sessionId>` (autoridad de sesión)
- `REFRESH#<sha256(refreshToken)>` / `SESSION` (localizador de un solo uso, TTL heredado)

**Propuesta:** una **única decisión transversal** (no siete RFC) que sustituya la referencia
documental en las siete specs. No cambia tabla, índice ni código; no toca E003/E004.

Estado en el registro de decisiones: 7 entradas `PERSISTENCIA` enlazadas a esta RFC.

### 3.5 Campos candidatos — OPCIONAL por defecto

149 candidatos de request y 501 de response repartidos en 60 y 62 operaciones. Volumen mayor:

| Operación | Campos candidatos | Observación |
|---|---|---|
| E021 `GET /bootstrap` | 59 | agregado de arranque; el SDD puede devolverlo por composición de otros endpoints |
| E074 `GET /demo/bootstrap` | 50 | datos de demostración |
| E051 `GET /dashboard` | 43 | agregado de presentación |
| E020 `GET /users/me/export` | 42 | exportación; forma del documento es decisión de producto |
| E028 `POST /movements` | 20 | incluye `summary` con 9 subcampos |
| E058 `PUT /notification-preferences` | 19 | la matriz se contradice (`quietHours` como objeto en request y como subcampos en response) |

**Regla propuesta:** ningún candidato se convierte en requisito por aparecer en la auditoría. Se
decide endpoint a endpoint, en su Gate de implementación, con la regla de que **añadir un campo
opcional no requiere enmienda** y **añadir un campo obligatorio o cambiar un tipo sí**.

### 3.6 Operaciones con riesgo financiero (atención independiente)

E028, E031, E043, E044, E047, E033, E037 y E049 concentran las decisiones de dinero. La auditoría
y el SDD coinciden en los invariantes (centavos enteros, idempotencia, transacciones atómicas,
reversión que netea), pero difieren en la forma del contrato:

- **E043/E044**: el prototipo propone `totalAmount`, `mode`, `splitMode`, `goalIds[]`; el SDD usa
  `{amount, currency, allocations}`. Son modelos distintos de reparto, no un campo suelto:
  **COMPATIBILIDAD**, requiere decisión de producto antes de implementar.
- **E044**: `TransactWriteItems` admite como máximo 100 acciones. Un reparto a N metas consume
  acciones por meta; el límite debe fijarse en el contrato (no descubrirse en runtime).
- **E031/E042/E047**: el prototipo devuelve resumen y efectos de reversión que el SDD no declara.
  Si se adoptan, dejan de ser `204` y pasan a `200` (ver §3.1).

## 4. Decisiones registradas (mecanismo)

`sdd/_control/H08/DECISIONES_CONTRATO_E005_E075.json` contiene 12 entradas: 5 de código de éxito y
7 de persistencia. Cada una fija:

```json
{"id": "E017", "tipo": "HTTP_EXITO", "valor_vigente": "201", "valor_objetivo": "200",
 "estado": "PENDIENTE", "clasificacion": "COMPATIBILIDAD", "rfc": "RFC-016", "gate": null}
```

- `PENDIENTE` → el contrato **debe** seguir en `valor_vigente`. Si alguien lo cambia sin Gate, el
  verificador falla (`CONTRATO_FUERA_DE_DECISION`).
- `APROBADA` → el contrato **debe** estar en `valor_objetivo`. Si la enmienda no se aplicó, falla.
- `RECHAZADA` → se conserva el vigente.

Este archivo **no aprueba nada**: registra valores para que la desalineación sea detectable en los
dos sentidos.

## 5. Qué NO bloquea E005

Ninguna diferencia de E006–E075 impide preparar o implementar E005, salvo dependencia técnica
demostrada. En particular **no** es requisito previo resolver E063, E074, E021 ni E051.

Lo que sí debe resolverse antes de **entregar** E005:

1. Proveedor de correo autorizado con envío real (`BLOQUEO_E005_CORREO.md`).
2. Gate de contrato si se decide añadir `locale` o `retryAfterSeconds`.
3. Decisión de persistencia de E005 (referencia `SESSIONHASH` → modelo real) — editorial.

## 6. Gates solicitados

| Gate | Alcance | Estado |
|---|---|---|
| `APRUEBO_RFC_016_CONTRATOS_E005_E075` | Aprobar la **clasificación** y el mecanismo de decisiones. No aplica ninguna enmienda | **PENDIENTE** |
| `APRUEBO_ENMIENDA_CONTRATO_<ID>` (por operación) | Aplicar una enmienda concreta a `spec.md`/OpenAPI | **PENDIENTE**, uno por operación |
| `APRUEBO_PERSISTENCIA_E005_E011` | Sustituir `SESSIONHASH#digest` por el modelo real en 7 specs | **PENDIENTE** |

Sin el primero no se aplica ninguno de los otros.

## 7. Riesgos

| Riesgo | Control |
|---|---|
| Aplicar la auditoría como si fuera contrato | El verificador exige que el contrato siga el valor vigente mientras la decisión esté `PENDIENTE` |
| Romper el cliente Android con un cambio de código | Toda enmienda de `COMPATIBILIDAD` exige RFC + Gate + análisis con `comparar_contrato_api.py` |
| Deriva entre matriz y SDD | La matriz se valida contra OpenAPI en cada ejecución de CI |
| Convertir 650 candidatos en requisitos | Se clasifican como `OPCIONAL` hasta decisión explícita por endpoint |
| Enmienda aplicada sin actualizar la matriz | `EXITO_AUSENTE_OPENAPI` / campos declarados fallan en CI |

## 8. Estado de aplicación

**No se ha modificado ningún `spec.md`, `plan.md`, OpenAPI, contrato ni código de E005–E075.**
E001–E004 permanecen intactos.
