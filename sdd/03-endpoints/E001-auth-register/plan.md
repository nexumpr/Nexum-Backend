# Plan H06 → V6.0.2 — E001: registro con ingreso habitual y compromisos fijos

**Estado:** contrato V6.0.2 alineado con el prototipo Nexum (RFC-012) e implementado en
local; despliegue `NOT_RUN`. **Ruta:** `POST /api/v1/auth/register`.

## Arquitectura y secuencia

1. **Dominio** (`internal/domain/identity`): `InitialIncome`, `RecurringExpense`, `Consent`,
   `Onboarding` y `ClientContext` como objetos validados; `Registration` compone el
   agregado y fija el onboarding inicial. El equivalente mensual se delega en el motor
   exacto de A-070 (`financial.CalculateMonthlyEquivalent`), nunca en coma flotante.
2. **Aplicación** (`internal/application/identity`): `RegisterCommand` completo,
   `ValidateRegisterCommand` por campo, `payloadHash` canónico y `RegisterResponse` como
   contrato de salida. `RegisterService` hashea la contraseña con Argon2id, genera los UUID
   del servidor y comprueba en el replay la igualdad de contraseña contra el verificador
   Argon2id almacenado.
3. **Transporte** (`internal/transport/http`): DTO estricto con `httputil.DecimalMoney`
   (string, dos decimales, prohibido número JSON) y bloques obligatorios que producen `400`
   con el campo exacto. `BindJSON` mantiene `DisallowUnknownFields` y el límite de 1 MB;
   el rate limit de Redis se aplica **antes** de tocar la persistencia.
4. **Persistencia** (`internal/adapters/dynamo`): una única `TransactWriteItems` con seis
   acciones fijas más una por compromiso, condiciones `attribute_not_exists(PK)` y la
   idempotencia al final con `attribute_not_exists(PK) OR expiresAt < :now`. Los motivos de
   cancelación se traducen por tipo de acción, no por índice fijo.
5. **Respuesta:** `201` con `RegisterResponse`, `Location: /api/v1/users/me` y
   `X-Idempotent-Replay` cuando corresponde. Sin tokens: E001 no crea sesión.

## Decisiones específicas

- **Nombre del contrato:** `name` sustituye a `displayName` en el transporte; el atributo
  persistido sigue siendo `displayName` para no romper los perfiles existentes que lee E012.
- **Esquema de respuesta propio:** `RegisterResponse` es nuevo; `UserProfile` se conserva
  intacto porque lo usa E012 (`GET /api/v1/users/me`).
- **Límite de la transacción:** 100 acciones de DynamoDB menos 6 fijas = **94 compromisos**.
  El exceso se rechaza completo; no se trunca la lista.
- **Idempotencia:** el `payloadHash` cubre identidad, ingreso, compromisos en orden,
  consentimiento y cliente. La contraseña se prueba con Argon2id en el replay, de modo que
  una misma clave con otra contraseña devuelve `409` sin almacenar la contraseña ni un
  digest simple.
- **Categoría:** catálogo cerrado del prototipo; el servidor valida y persiste el valor, no
  recalcula por palabras clave ni inventa identificadores numéricos.
- **Zona horaria:** validada con la base IANA embebida en el binario, para que el resultado
  no dependa del host.
- **Sin sesión ficticia:** el bloque `session` del prototipo es condicional y queda fuera
  del alcance hasta E002/E003.

## Dependencias y riesgos

- Cambio incompatible **intencional** del contrato de E001: los campos requeridos cambian y
  el `201` pasa de `UserProfile` a `RegisterResponse`. Se registra en RFC-012 y en el
  registro de excepciones de compatibilidad de API; el comparador de contratos sigue
  bloqueando cualquier otra ruptura no autorizada.
- Errores de concurrencia `409`, reglas de negocio `422`, disponibilidad `503` y seguridad
  de recursos ajenos.
- Dependencias de despliegue: artefacto S3 versionado, CD asistido y Gate humano; AWS no se
  ha modificado en esta unidad.
- Prueba real HTTP/HTTPS y DynamoDB/Redis: **NOT_RUN**; solo el contrato y la
  implementación local son verificables ahora.
