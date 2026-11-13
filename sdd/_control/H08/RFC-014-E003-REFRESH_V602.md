# RFC-014 — Alineación de E003 (refresh) con el prototipo V6.0.2

| Campo | Valor |
|---|---|
| ID | RFC-014 |
| Fecha de propuesta | 2026-11-13 |
| Fecha de aprobación | **2026-11-13T18:18Z** |
| Estado | **APPROVED** |
| Alcance | `POST /api/v1/auth/refresh` (E003) exclusivamente |
| Frase de aprobación recibida | **`APRUEBO_RFC_014_E003_REFRESH`** |
| Aprobador | Responsable de Nexum-Backend (humano) |
| Base | `main` `612530610726640eb94f0e9506029cb746e07884` |
| Rama | `feat/h08e-e003-auth-refresh` |
| PR | #61 |
| HEAD al aprobar | `04799ebe115c691649e18bee4bd6cffc29158d5f` (run `43478609688`, 13/13 pass) |

**Autorización humana registrada (texto recibido):** «APRUEBO_RFC_014_E003_REFRESH … AUTORIZO el
RFC-014 de E003 V6.0.2, exclusivamente para las tres modificaciones contractuales
documentadas … 1. HTTP 401 UNAUTHORIZED para tokens de renovación inválidos, expirados o
revocados. 2. deviceId opcional en RefreshRequest, sin cambiar el campo obligatorio
refreshToken. 3. session.id como alias de sessionId en la respuesta AuthTokens.»

**Aplicación:** las tres modificaciones se aplicaron en `nexum-api-v1.yaml` y
`nexum-api-v1.json` en el **mismo PR #61**. Restricciones respetadas: no se alteró E001,
E002 ni ningún otro endpoint; `required` de `AuthTokens` no cambió (los cinco campos
heredados siguen obligatorios) y `required` de `RefreshRequest` sigue siendo
`["refreshToken"]`; `deviceId` es una señal de consistencia que nunca autoriza por sí sola.

## 1. Motivo

El contrato H06 de E003 declara respuestas `200, 400, 409, 422, 429, 500, 503` y un
`RefreshRequest` con un único campo. La auditoría del prototipo V6.0.2 (ficha E003, página
19) y la implementación real de E002 V6.0.2 (RFC-013) dejan tres diferencias que impiden
un refresh correcto y compatible con Android:

| # | Diferencia | Efecto real sin corregir |
|---|---|---|
| A | El PDF exige `HTTP 401` para refresh inválido/revocado; H06 no lo declara | El runtime devolvería un código no declarado (401) o tendría que usar 422 «violación de regla de negocio» para una falla de autenticación |
| B | El PDF envía `deviceId` opcional en el refresh; `RefreshRequest` usa `additionalProperties: false` | Un cliente construido desde el PDF recibiría **400** y el refresh dejaría de funcionar por completo |
| C | El PDF devuelve `session.id`; `AuthTokens` sólo expone `sessionId` | E002 ya devuelve ambos (RFC-013); Android podría leer `session.id` y obtener `null` al renovar |

Ninguna de las tres elimina ni renombra algo existente: las tres son **aditivas**.

## 2. Cambios contractuales exactos

### 2.1 Respuesta `401` en E003

```yaml
'401':
  description: UNAUTHORIZED
  content:
    application/json:
      schema:
        $ref: '#/components/schemas/ErrorEnvelope'
```

Semántica: token inexistente, caducado, revocado, manipulado o incoherente con su sesión.
Mensaje único y genérico (`Refresh token invalido.`), sin enumeración de sesiones ni cuentas.

### 2.2 `RefreshRequest.deviceId` opcional

```yaml
deviceId:
  type: string
  maxLength: 128
  description: Identificador de instalación. Señal de consistencia con la sesión; nunca autoriza.
```

`required` sigue siendo únicamente `["refreshToken"]`. Un cliente que no lo envíe no cambia
de comportamiento.

### 2.3 `AuthTokens.session.id`

```yaml
session:
  type: object
  additionalProperties: false
  required: [id]
  properties:
    id: {type: string, format: uuid}
```

Alias exacto de `sessionId`, igual que en `LoginResponse` de E002. `required` de
`AuthTokens` no cambia: los cinco campos heredados siguen siendo obligatorios.

Los espejos `nexum-api-v1.yaml` y `nexum-api-v1.json` deben quedar idénticos; el validador
`validar_h06.py` y la paridad semántica de I-150 lo comprueban.

## 3. Compatibilidad

- **E002**: intacto. `LoginResponse` y `AuthTokens` no se modifican más allá de añadir el
  objeto opcional `session` a `AuthTokens` (que sólo usa E003).
- **E001**: intacto.
- **Android**: las tres adiciones eliminan rupturas con el PDF; ninguna obliga a cambiar
  el cliente. Si el cliente no envía `deviceId`, no se aplica ninguna comprobación de
  dispositivo.
- **Comparador de compatibilidad (I-150)**: añadir respuestas y propiedades opcionales no
  es ruptura; no requiere entrada en `EXCEPCIONES_COMPATIBILIDAD_API.json`.

## 4. Riesgos y mitigaciones

| Riesgo | Mitigación |
|---|---|
| `401` se confunde con `403` | Se documenta: 401 = credencial de refresh no válida; 403 se reserva a autorización de recursos |
| `deviceId` se interpreta como factor de autenticación | Validación explícita: sólo compara con la sesión; una discrepancia devuelve 401 sin revocar |
| Duplicidad `sessionId` + `session.id` | `session.id` se deriva del mismo valor; no crea sesión nueva |
| Cliente antiguo envía `deviceId` con otro valor | 401 (fallo cerrado) y mensaje genérico; se documenta en el handoff a Android |

## 5. Pruebas que cubren el cambio

- Handler: `session.id == sessionId`, `expiresIn=900`, `tokenType=Bearer`, propiedades
  exactas, 401 genérico, `deviceId` aceptado.
- Aplicación: `deviceId` incoherente ⇒ 401 sin revocar; coherente ⇒ rotación.
- Adaptador: cadena A→B→C→D sobre un doble con estado que interpreta las condiciones
  emitidas; replay de A ⇒ 409 + familia revocada.

## 6. Rollback contractual

Revertir este RFC en el mismo PR: retirar el `401`, `deviceId` y `session.id` del YAML/JSON,
y volver a mapear el token inválido a `422 BUSINESS_RULE_VIOLATION` en el handler. No hay
migración de datos ni cambio de esquema persistido: el rollback es sólo de contrato.

## 7. Consecuencia de no aprobarlo

*(Histórico: ya no aplica.)* Si no se hubiera aprobado, el PR habría quedado abierto con el
runtime actual (401 + `deviceId` + `session.id`) sin fusionarse, o bien ajustado a `422` y sin
las dos adiciones.

## 8. Registro de aplicación (2026-11-13T18:18Z)

- Gate recibido: `APRUEBO_RFC_014_E003_REFRESH`, registrado en `GATE_H08E_E003.md`.
- Cambios aplicados en `sdd/03-endpoints/openapi/nexum-api-v1.yaml` y su espejo `.json`:
  respuesta `401` en la operación E003, propiedad `deviceId` (opcional, `maxLength: 128`) en
  `RefreshRequest`, y objeto `session` con `id` en `AuthTokens`.
- Validaciones ejecutadas tras el cambio: `validar_h06.py` → `H06_CONTRACT_PASS` (75
  operaciones, 300 specs, 33M+23S+12E+7O, refs válidos); paridad I-150 → 75 operaciones y
  55 rutas en YAML y JSON, 102 esquemas comparados, 0 problemas; comparador de
  compatibilidad contra `origin/main` → `COMPATIBLE`, 0 rupturas, sin excepciones.
- Sin cambio de runtime: el handler ya devolvía `401`, aceptaba `deviceId` y emitía
  `session.id`, por lo que el contrato y el código quedan alineados sin tocar Go.
