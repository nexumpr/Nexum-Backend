# Contrato de integración Android — Nexum V6.0.2

| Campo | Valor |
|---|---|
| Documento | `sdd/_control/H08/CONTRATO_INTEGRACION_ANDROID.md` |
| Estado | **PROPUESTO** (deriva de contratos aprobados; no los modifica) |
| Base URL | `https://d2lpmqy5w6x9es.cloudfront.net/api/v1/` |
| Operaciones | 75 (4 implementadas: E001–E004) |
| Autoridad | OpenAPI 3.1 aprobado (H06). Este documento es guía de consumo |
| Requiere | `APRUEBO_RFC_016_CONTRATOS_E005_E075` |

> Guía operativa para el equipo Android. **No** es un contrato nuevo: si discrepa del OpenAPI,
> prevalece el OpenAPI.

---

## 1. Transporte y cabeceras

| Cabecera | Cuándo | Notas |
|---|---|---|
| `Authorization: Bearer <accessToken>` | **Todas salvo las 13 públicas** | RS256, `expiresIn=900` |
| `Content-Type: application/json` | Cuerpos JSON | UTF-8 |
| `Accept-Encoding: gzip` | Siempre | el backend comprime |
| `Idempotency-Key` | E028, E037, E044 y toda mutación financiera | requerida por contrato |
| `X-Request-Id` | Opcional | se devuelve en la respuesta y en `error.traceId` |

### 1.1 Operaciones públicas (sin `Bearer`) — verificado en OpenAPI

```
E001 POST /auth/register              E002 POST /auth/login
E003 POST /auth/refresh               E005 POST /auth/password/forgot
E006 POST /auth/password/reset        E011 POST /auth/email-verification/confirm
E015 GET  /privacy/notices/current    E022 GET  /catalogs/categories
E023 GET  /catalogs/frequencies       E070 GET  /help/faq
E071 GET  /education/tips             E072 GET  /health/live
E073 GET  /health/ready
```

**E006 es pública**: el usuario que olvidó su contraseña no tiene sesión. Su `token` de
recuperación **no es un JWT**: es un secreto de un solo uso que solo sirve en E006. E003 también es
pública porque se invoca cuando el access token ya expiró.

**Nunca** enviar `userId` en el cuerpo: el servidor lo deriva del `sub` del JWT.

## 2. Envelope de error

```json
{ "error": { "code": "STRING_MAQUINA", "message": "texto humano", "details": [], "traceId": "uuid" } }
```

Reglas de cliente:

| Código | Acción Android |
|---|---|
| `400` | error de programación o validación: no reintentar |
| `401` | intentar **E003** una vez; si falla, E002 |
| `403` | recurso ajeno: no reintentar |
| `404` | recurso inexistente o ajeno (indistinguible a propósito) |
| `409` | conflicto/idempotencia: recargar estado, no reintentar a ciegas |
| `422` | regla de negocio: mostrar `message` |
| `429` | respetar `Retry-After` (segundos) |
| `500`/`503` | reintento con backoff exponencial (máx. 3, con jitter) |

## 3. Sesión

1. `accessToken` 900 s; `refreshToken` con rotación **de un solo uso** (E003).
2. Guardar `refreshToken` en `EncryptedSharedPreferences`/Keystore; nunca en logs.
3. Ante `409` en E003 (replay detectado) ⇒ **descartar la sesión y volver a login**.
4. Logout (E004) es `204` sin cuerpo y revoca en servidor; borrar tokens locales después.
5. `session.id` = `sessionId` (alias de compatibilidad).

## 4. Tipos y formatos

| Tipo | Formato | Ejemplo |
|---|---|---|
| Dinero | **string decimal** con 2 decimales | `"1234.50"` |
| Moneda | ISO-4217 | `"MXN"` |
| Fecha-hora | ISO-8601 UTC con `Z` | `"2026-11-16T20:15:00Z"` |
| Fecha | `YYYY-MM-DD` | `"2026-11-16"` |
| Periodo | `YYYY-MM` | `"2026-11"` |
| Cursor | string opaco | no interpretar ni construir |
| Enumeraciones | MAYÚSCULAS cuando aplique | `"EXPENSE"`, `"INCOME"` |

**Prohibido** enviar importes como número JSON: el servidor rechaza floats en campos monetarios.

## 5. Paginación

- Respuesta con `items[]` y `nextCursor` (o cabeceras `X-Cursor-Next` / `X-Has-More`).
- Pedir la siguiente página con `cursor=<nextCursor>`, nunca con `offset`.
- `nextCursor` ausente o `null` ⇒ fin de la lista.

## 6. Secuencias funcionales críticas

| Pantalla | Secuencia |
|---|---|
| Registro | E001 → E002 |
| Login | E002 → E021 (bootstrap) |
| Movimiento rápido | E028 (con `Idempotency-Key`) → refrescar E026/E051 |
| Pago de recurrente | E037 (con `Idempotency-Key`) → E026 |
| Aportación a metas | E043 (preview) → E044 (commit con la misma `Idempotency-Key`) → E038 |
| Reversión | E047 → E038/E051 |
| Recuperación de contraseña | E005 → correo → E006 |
| Voz | captura en Android → E075 → **confirmar** → E028 |
| Recomendaciones | E055 → si `hasEnoughData=false`, mostrar estado vacío explicativo |

## 7. Estados vacíos y errores funcionales

| Caso | Señal contractual | UI esperada |
|---|---|---|
| Sin movimientos | `items: []` | estado vacío ilustrado, no error |
| Sin datos suficientes (ML) | `hasEnoughData: false` | mensaje de «necesitamos más historial» |
| Meta completada | `completed: true` | celebración, no bloqueo |
| Sesión revocada | `401` tras rotación | volver a login |
| Cuenta en borrado | `403`/`404` según operación | pantalla de cuenta eliminada |

## 8. Rate limiting

- Cuotas por IP y por cuenta en Redis (distribuido, compartido por los 2 nodos).
- En login (E002) y reset (E005/E006) el límite es **fail-closed**: sin Redis ⇒ `503`.
- El cliente debe tratar `429` como esperado, no como fallo del backend.

## 9. Compatibilidad y versionado

- Prefijo `/api/v1`; **no** se rompe sin RFC + Gate + ventana de convivencia.
- Cambios aditivos (campos opcionales nuevos) no requieren acción del cliente.
- El cliente debe **ignorar campos desconocidos** y no fallar por ellos.
- `modelVersion` en E055 permite detectar cambios de modelo sin cambiar el contrato.

## 10. Lo que Android NO debe asumir

- Que un `202` de E005 signifique «correo entregado» (solo «solicitud aceptada»).
- Que `404` distinga «no existe» de «es de otro usuario».
- Que el orden de `items[]` sea estable sin `orderBy` explícito.
- Que los importes lleguen como números.
- Que existan endpoints de voz, medios o ML fuera de los publicados en OpenAPI.

## 11. Estado

Pendiente de la aprobación de RFC-016. Las 4 operaciones ya desplegadas (E001–E004) no cambian.
