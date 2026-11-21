# Checklist de calidad — E006

**Diseño H06:** aprobado. **Implementación local:** 2026-11-17. **CI:** PASS (PR #71 13/13; `main` run `44116124273`).
**AWS:** `DEPLOYED_VERIFIED`. **Respuesta:** 200 `Accepted`. **Aceptación:** `E006_USER_ACCEPTED = CONFIRMED`.

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
- [x] Tests unitarios, de adaptador y HTTP escritos; ejecución en CI Linux **PASS** (run `44116124273`).
- [x] Dos instancias AWS, TLS público e IaC: **VERIFIED** (LT v8, ASG 2/2/2, dos targets healthy, CloudFront con E006 400/422/400; 404 antes del release).
- [x] Gate de release y rollback: apply autorizado por SHA-256 (`90988C7C…2D63`) con 0 add/4 change/0 destroy; rollback conservado (16 objetos S3).
- [x] Dependencias del cliente Mobile no constituyen Gate del Backend.

**Cierre:** la aceptación funcional del responsable confirma el restablecimiento real de
contraseña en Postman. Replay 409, revocación de access/refresh y concurrencia AWS conservan
evidencia **pendiente**; no se declaran PASS. Limitación multi-AZ declarada: ambas instancias
en `us-east-2a` por capacidad de `t4g.micro` en `us-east-2b`.

**Criterio de cierre:** la aceptación funcional del restablecimiento real está confirmada por
el responsable. La evidencia individual de replay del token, revocación de access/refresh y
concurrencia en AWS permanece **pendiente** y no se declara PASS; no se solicita repetir el
cambio de contraseña para cerrar la documentación.

## Criterios de aceptación de la enmienda (A1)

- [x] El campo aparece en el contrato OpenAPI (YAML y JSON en paridad).
- [x] Se persiste y se devuelve en la respuesta de creación (no aplica a E006: sin creación de recurso; se conserva la paridad contractual).
- [x] Se consulta con el mismo tipo y formato (no aplica a E006).
- [x] Edición/relectura no aplica a E006.
- [x] Validación negativa devuelve 422 sin filtrar datos internos.

<!-- NOTA_VIGENCIA_AUDITORIA_SEMANTICA -->
### Nota de vigencia - auditoria semantica (2026-11-21)

- Estado: `AUDITADA_SEMANTICA_CON_OBSERVACIONES`.
- Hallazgo: Restablecimiento 200 y replay 409 verificados; los casos limite individuales en AWS no estan localizados. E006 no envia correo.
- Los bloques anteriores de este archivo conservan el estado historico del endpoint y no se reescriben.
- Evidencia: `sdd/_control/H08/GATE_H08E_E007_E011_DESPLIEGUE_AWS.md` §4 y los Gates especificos de E001-E006; `memoria.md` cierre E001/E002 y v0.15.0.
- Release inicial: ver la matriz de auditoria del PR #142. Ultima verificacion funcional agregada: `v0.15.0+8f13c86d2ca0`.
- Cualquier contradiccion entre bloques anteriores y esta nota se resuelve a favor de esta nota.
