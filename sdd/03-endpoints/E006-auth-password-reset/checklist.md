# Checklist de calidad — E006

**Diseño H06:** aprobado. **Implementación local:** 2026-11-17. **CI:** pendiente del PR.
**AWS:** `NOT_RUN`. **Respuesta:** 200 `Accepted`.

- [x] Gate local registrado en `sdd/_control/H08/GATE_H08E_E006.md`; no confundir instalación con aprobación.
- [x] Método/path conservan el ID del inventario H04 (`nexum_e006`).
- [x] Payload válido/negativo y JSON Schema `PasswordResetRequest` verificados (12–128, campos desconocidos rechazados).
- [x] Respuesta `Accepted` y errores de OpenAPI implementados (400/409/422/429/500/503).
- [x] Ruta pública justificada: el token de un solo uso es la credencial; `userId` sale del registro persistido.
- [x] Concurrencia resuelta con CAS de `version` + consumo condicional del token en una transacción; sin idempotencia adicional exigida por el contrato.
- [x] DynamoDB usa `GetItem` consistente y `TransactWriteItems`; **cero `Scan`**.
- [x] Redis: cuota 10/15 min por IP con Lua atómico; fallo de Redis -> 503.
- [x] Timestamps UTC RFC3339 (`passwordChangedAt`, `usedAt`); importes no aplican.
- [x] Pruebas de error pertinentes al caso (400/409/422/429/503) y de éxito 200.
- [x] Sin PII ni secretos en logs, cuerpos de error, fixtures ni evidencias.
- [x] Tests unitarios, de adaptador y HTTP escritos; ejecución en CI Linux (Windows bloquea binarios de prueba).
- [ ] Dos instancias AWS, TLS público e IaC: **NOT_RUN**; requieren Gate de despliegue.
- [ ] Gate de release y rollback: pendientes de autorización humana.
- [x] Dependencias del cliente Mobile no constituyen Gate del Backend.

**Criterio de cierre:** la spec solo podrá declararse `AWS_VERIFIED` tras el cambio real de
contraseña por HTTPS, el rechazo de la contraseña anterior, el 409 del token reutilizado y la
revocación efectiva de access/refresh tokens anteriores.

## Criterios de aceptación de la enmienda (A1)

- [x] El campo aparece en el contrato OpenAPI (YAML y JSON en paridad).
- [x] Se persiste y se devuelve en la respuesta de creación (no aplica a E006: sin creación de recurso; se conserva la paridad contractual).
- [x] Se consulta con el mismo tipo y formato (no aplica a E006).
- [x] Edición/relectura no aplica a E006.
- [x] Validación negativa devuelve 422 sin filtrar datos internos.
