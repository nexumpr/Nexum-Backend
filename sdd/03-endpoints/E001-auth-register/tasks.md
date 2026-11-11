# Tareas — E001 V6.0.2

> Evidencia local únicamente. Ninguna casilla implica prueba HTTP real ni despliegue AWS;
> esos estados siguen `NOT_RUN` hasta el Gate correspondiente.

## Contrato y gobierno

- [x] `E001-T01` RFC-012 de alineación E001 V6.0.2 registrada con el alcance autorizado.
- [x] `E001-T02` OpenAPI YAML y JSON sincronizados: `RegisterRequest`, `RegisterIncomeInput`,
      `RegisterRecurringExpenseInput`, `RegisterPrivacyInput`, `RegisterClientInput`,
      `RegisterResponse` y sus cinco bloques.
- [x] `E001-T03` `UserProfile` conservado sin cambios para E012; E001 deja de usarlo.
- [x] `E001-T04` Excepción de compatibilidad de API registrada solo para la ruptura
      intencional de E001 (campos requeridos nuevos y cambio del esquema 201).
- [x] `E001-T05` Matriz H06, matriz de compatibilidad y matriz de accesos DynamoDB
      actualizadas en su entrada E001.

## Dominio y aplicación

- [x] `E001-T06` Enum cerrado de siete frecuencias (sin `daily` ni `once`) y catálogo de
      cinco categorías derivadas.
- [x] `E001-T07` `InitialIncome` con importe positivo en centavos y equivalente mensual
      exacto (A-070, HALF_UP).
- [x] `E001-T08` `RecurringExpense` con concepto, importe, frecuencia, categoría y tipo
      `fixed`; identificador asignado por el servidor.
- [x] `E001-T09` `Consent` con versión declarada y `acceptedAt` del servidor; `Onboarding`
      inicial `pending/pending/false`; `ClientContext` con locale BCP-47 y zona IANA.
- [x] `E001-T10` Validación por campo de todo el comando, incluidos los bloques obligatorios
      ausentes y el límite de 94 compromisos.

## Transporte

- [x] `E001-T11` DTO estricto con `DecimalMoney` (string de dos decimales; número JSON
      rechazado) y rechazo de campos heredados y de `signupConfirm`.
- [x] `E001-T12` Respuesta 201 con `RegisterResponse`, `Location` y `X-Idempotent-Replay`.
- [x] `E001-T13` Rate limit de Redis aplicado antes de la persistencia, con `Retry-After`.

## Persistencia

- [x] `E001-T14` `TransactWriteItems` única: reserva de correo, perfil, ingreso,
      consentimiento, onboarding, compromisos e idempotencia.
- [x] `E001-T15` Traducción de motivos de cancelación por tipo de acción; conflicto de
      correo, conflicto de idempotencia y no disponibilidad diferenciados.
- [x] `E001-T16` Guardas de 100 acciones y de tamaño; el exceso se rechaza sin escribir.
- [x] `E001-T17` Replay estable con verificación Argon2id de la contraseña y `409` ante
      payload o contraseña distintos.

## Verificación local

- [x] `E001-T18` `gofmt`, `go build ./...`, `go vet ./...` y `go test ./... -count=1`.
- [x] `E001-T19` Pruebas de dominio: siete frecuencias, equivalencias mensuales, categorías,
      tipo, límite y contexto de cliente.
- [x] `E001-T20` Pruebas de aplicación: 0/1/varias filas, validaciones, límite 94/95,
      idempotencia, conflicto de contraseña, concurrencia y contenido de la respuesta.
- [x] `E001-T21` Pruebas del adaptador DynamoDB: seis acciones fijas, compromisos, límite de
      la transacción, atomicidad ante fallo de una fila y particiones autorizadas por IAM.
- [x] `E001-T22` Pruebas de transporte: 201 completo, `[]`, replay, 409 por contraseña,
      campos desconocidos y heredados, bloques ausentes y rate limit.

## Pendiente de Gate (no ejecutado)

- [ ] `E001-T23` Publicar el artefacto inmutable en S3 y desplegar con el CD existente
      (requiere autorización humana específica).
- [ ] `E001-T24` Verificar `POST /api/v1/auth/register` por HTTPS real y comprobar en
      DynamoDB los items `USER#`, `INCOME`, `CONSENT#`, `ONBOARDING`, `RECURRING#` e `IDEM#`.

## Casos concretos

- [x] `E001-CT01` Solicitud válida produce 201 y el esquema exacto de OpenAPI.
- [x] `E001-CT02` JSON malformado, campos desconocidos y tipos incorrectos → 400/422.
- [x] `E001-CT03` No aplica recurso ajeno: el alta siempre crea el sujeto desde cero.
- [x] `E001-CT04` Fallo de persistencia → 503 sin escritura parcial.
- [x] `E001-CT05` Concurrencia multiinstancia simulada sin duplicaciones en el doble.
- [x] `E001-CT06` Replay idéntico estable; payload o contraseña distintos → 409.
