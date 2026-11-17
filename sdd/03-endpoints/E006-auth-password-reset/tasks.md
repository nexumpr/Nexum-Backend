# Tareas planificadas — E006

> Implementación local 2026-11-17 (Gate `GATE_H08E_E006.md`). `[x]` = hecho y verificable en
> el código/pruebas del repositorio; la ejecución de pruebas y la verificación AWS se anotan
> por separado. `AWS_NOT_RUN` hasta el Gate de despliegue.

- [x] `E006-T01` OpenAPI `operationId=nexum_e006` contrastado con `POST /api/v1/auth/password/reset`; contrato sin cambios.
- [x] `E006-T02` DTO estricto `PasswordResetRequest` (`token`, `newPassword`) con rechazo de campos desconocidos y tipos incorrectos.
- [x] `E006-T03` Ruta pública sin Bearer; `userId` se deriva del token persistido, nunca del cliente.
- [x] `E006-T04` Caso de uso: token de un solo uso, caducidad lógica, perfil `ACTIVE`, Argon2id y transacción atómica.
- [x] `E006-T05` Adaptador DynamoDB: `RESET#<sha256>/RESET` + `USER#<id>/PROFILE` en una `TransactWriteItems` de 2 acciones; sin `Scan`.
- [x] `E006-T06` Redis: cuota de confirmación 10/15 min por IP con Lua atómico y fail-closed; la época no se cachea.
- [x] `E006-T07` Códigos 200/400/409/422/429/500/503 y errores uniformes sin filtrar token ni contraseña.
- [x] `E006-T08` Pruebas unitarias de validación, caducidad, reutilización, concurrencia y perfiles legados.
- [x] `E006-T09` Pruebas de adaptador y HTTP escritas sobre el doble con estado; ejecución real en CI.
- [x] `E006-T10` Negativas de token usado/expirado/inexistente y dos solicitudes concurrentes.
- [x] `E006-T11` CI + seguridad + contratos: **PASS** en PR #71 (13/13; run `44110769604`) y en `main` (run `44116124273`).
- [x] `E006-T12` Release, despliegue en dos nodos y prueba HTTPS: **VERIFIED** — release `v0.13.0+578b0db3b9bb`, LT v8, ASG 2/2/2, 2 targets healthy; aceptación funcional confirmada por el responsable en Postman (`E006_USER_ACCEPTED = CONFIRMED`). Evidencia individual de replay/revocación/concurrencia: pendiente.

## A6 — cambio compartido (RFC-018)

- [x] `sessionEpoch` persistido en `PROFILE`; lectura en login y emisión del claim `epo`.
- [x] Refresh compara época de sesión y perfil y la exige en la transacción de rotación.
- [x] Middleware lee `PROFILE` con consistencia fuerte; 401 desalineado, 503 sin persistencia.
- [x] Compatibilidad: perfil/sesión/JWT sin época = 0.
- [x] E006 incrementa la época exactamente una vez dentro de su transacción.

## Casos concretos

- [x] `E006-CT01` Respuesta 200 `Accepted` con UUID v4 verificada en prueba de transporte; aceptación funcional confirmada por el responsable en Postman.
- [x] `E006-CT02` JSON malformado, campo desconocido, tipo incorrecto y longitudes fuera de rango (400/422) verificados por HTTPS real.
- [x] `E006-CT03` Sin recurso ajeno: el token resuelve su propio `userId`; una cuenta deshabilitada no cambia contraseña (prueba de aplicación; AWS individual pendiente).
- [x] `E006-CT04` Fallo de DynamoDB/Redis -> 503; ninguna escritura parcial (pruebas de aplicación/adaptador).
- [x] `E006-CT05` Dos nodos lógicos sobre el mismo almacén: una sola transacción gana (prueba local).
- [x] `E006-CT06` Token reutilizado -> 409; sesiones anteriores rechazadas en ambos nodos lógicos (pruebas locales). Evidencia AWS individual: **pendiente**.

## Enmienda A1

- [x] Persistencia corregida a `USER#<userId>/SESSION#<sessionId>` y `REFRESH#<sha256>/SESSION`; sin tabla ni índice nuevos.
- [x] Validación de entrada y mensaje 422 (`newPassword` 12–128, token no vacío).
- [x] Prueba positiva y negativa de la validación.
- [ ] Verificación HTTP real: pendiente del despliegue autorizado.

## Cierre AWS (2026-11-17)

- [x] PR #71 MERGED; `main` `578b0db3b9bb765afc981a86b23c6ade10ed80ea`; CI de `main` SUCCESS.
- [x] Release `v0.13.0+578b0db3b9bb` publicado con checksums verificados.
- [x] Apply autorizado por SHA-256 (0 add · 4 change · 0 destroy); LT v8; IAM con 16 objetos S3.
- [x] Dos instancias LT v8 `InService/Healthy`, ASG 2/2/2 y dos targets healthy; E006 servido por CloudFront (400/422/400; 404 con nodos v7 antes del release).
- [x] Regresión dirigida E001–E005 por HTTPS (negativas; sin cambios de estado).
- [x] Aceptación funcional del responsable en Postman: `E006_USER_ACCEPTED = CONFIRMED`.
- [ ] Evidencia individual AWS de replay 409, revocación de access/refresh y concurrencia: pendiente.
- **Limitación:** ambas instancias en `us-east-2a` por falta de capacidad `t4g.micro` en `us-east-2b`; subredes restauradas.
