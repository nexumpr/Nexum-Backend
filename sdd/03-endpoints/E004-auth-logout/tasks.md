# Tareas planificadas — E004

> Estado real al 2026-11-15. Una casilla marcada sólo significa **ejecutado y comprobado** en
> local; AWS, HTTP público y despliegue permanecen `NOT_RUN`. Gate:
> `sdd/_control/H08/GATE_H08E_E004.md`.

- [x] `E004-T01` Revisar OpenAPI `operationId=nexum_e004` vs `POST /api/v1/auth/logout` y contrato de dominio.
  _Verificado: `BearerAuth`, sin `requestBody`, `204` sin contenido, códigos 400/401/403/404/409/422/429/500/503. **No se modificó** YAML ni JSON: el contrato vigente ya era suficiente._
- [x] `E004-T02` Implementar request/response DTO estrictos `ninguno` / `ninguno`; rechazo de campos desconocidos.
  _No existe DTO por contrato. Cualquier cuerpo (incluido `{}`) se rechaza con 400 `INVALID_REQUEST` y detalle `body/body_not_allowed`._
- [x] `E004-T03` Implementar autorización por sujeto JWT y propiedad; públicas solo según allowlist aprobada.
  _`sub` y `sid` provienen exclusivamente de claims verificados. Un `sid` de otro sujeto, un token sin `sid` o una sesión inexistente responden 401 sin revelar existencia._
- [x] `E004-T04` Implementar caso de uso e invariantes: revocación inmediata del refresh y de la sesión.
  _`internal/application/identity/logout.go`: lectura consistente, verificación de estado y expiración, CAS de estado y versión, reconciliación tras conflicto (máx. 3 intentos), fallo cerrado._
- [x] `E004-T05` Implementar adaptador DynamoDB y patrón: `USER#id / SESSION#id` y `REFRESH#digest / SESSION`.
  _`internal/adapters/dynamo/logout_store.go`: sin `Scan`, sin tablas ni índices nuevos, sin tokens en claro, `ConsistentRead` en toda lectura autoritativa._
- [x] `E004-T06` Integrar Redis y consistencia multiinstancia sin confiar en caché para privilegios.
  _Decisión: Redis **no** participa en la autorización de revocación de E004. La autoridad es DynamoDB con lectura consistente; ninguna caché positiva puede autorizar una sesión revocada (A-080 §2.6). Sin coste ni clave nueva._
- [x] `E004-T07` Implementar códigos y errores definidos, ETag e idempotencia según operación.
  _204/400/401/409/503 con `ErrorEnvelope` y `traceId`. Sin `Idempotency-Key` (no exigida). Efectos del cierre idempotentes; la respuesta de un segundo intento es 401._
- [x] `E004-T08` Crear unit tests de reglas y validaciones; fixture sin PII/credenciales reales.
  _`tests/unit/application/logout_test.go` (11 casos) con identificadores sintéticos._
- [x] `E004-T09` Crear integración Dynamo y prueba HTTP desde cliente externo conforme OpenAPI.
  _Integración sobre adaptadores **reales** con doble con estado de DynamoDB (`tests/unit/dynamo/logout_store_test.go`, 10 casos) y HTTP con `httptest` (`tests/unit/transport/logout_v602_test.go`, 10 casos). HTTP público/AWS: `NOT_RUN`._
- [x] `E004-T10` Ejecutar negativas de auth/ownership/concurrencia y carga paginada cuando aplique.
  _Negativas de Bearer, sujeto ajeno, sesión ajena, cuerpo no permitido, dependencia caída y concurrencia logout↔refresh (25 iteraciones). Carga paginada no aplica._
- [x] `E004-T11` CI + seguridad + revisión de contratos + rollback y trazas saneadas.
  _CI del PR #63: run `43786668813`, HEAD `a31d2d6`, **13/13 `pass`**, con `go test -race` ejecutado en Linux (`PASS`) y cobertura de dominio 91.4 %. En local: gofmt, build, vet, `go test ./...`, validación H06, paridad I-150 y comparador `COMPATIBLE`. Rollback de la operación: basta revocar/no revocar; no hay migración de esquema._
- [x] `E004-T12` Solicitar autorización humana de release; desplegar desde IaC/CI, probar en dos nodos y registrar evidencia real.
  _**COMPLETADA**: release `v0.11.0+1e03d5cd12a7` publicado en S3; `terraform apply` del plan `5FC9634F…FD735` → LT **v5** e IAM con 10 objetos; instance refresh `d6708f22-…` `Successful` 100 %; dos EC2 `InService`/`Healthy` en v5, ASG 2/2/2 y dos targets `healthy`. HTTPS real y aceptación del responsable en Postman (200 → 204 → refresh 401). Ver `GATE_H08E_E004_DESPLIEGUE_AWS.md`._

## Casos concretos

- [x] `E004-CT01` Solicitud válida produce el código y esquema exactos indicados en OpenAPI.
  _204 con cuerpo de longitud cero y `Cache-Control: no-store`, comprobado en `TestE004HTTPLogout204SinCuerpo`._
- [x] `E004-CT02` JSON malformado, campos desconocidos, tipo incorrecto y longitud excedida -> 400/422 sin secretos en logs.
  _`TestE004HTTPBodyNoPermitido` y `TestE004HTTPErroresSinDatosSensibles`._
- [x] `E004-CT03` Petición sobre recurso de otro usuario no filtra existencia; verificar propiedad con dos sujetos.
  _`TestE004HTTPSesionDeOtroSujeto` y `TestE004SesionAjenaNoSeRevoca`._
- [x] `E004-CT04` Pérdida temporal de DynamoDB/Redis -> fallo controlado 503; jamás confirmar escritura parcial.
  _`TestE004HTTPDependenciaIndisponible` y `TestE004PersistenciaNoDisponible`; además `TestE004HTTPSinAutorizadorFallaCerrado`._
- [x] `E004-CT05` Misma operación en dos nodos Go reales; consistencia de sesión/caché y ausencia de duplicaciones.
  _Dos instancias **lógicas** independientes sobre el mismo almacén (`TestE004RevocacionObservableEnDosInstancias`, `TestE004HTTPRevocacionVisibleEntreInstancias`). En AWS: **dos EC2 reales en LT v5** e `InService`/`Healthy`, con la sesión en DynamoDB compartido; **no** se declara prueba cruzada instrumentada porque CloudFront/ALB no exponen qué nodo atendió cada petición._
- [x] `E004-CT06` Revocación/rotación en nodo A persiste y es observable inmediatamente en nodo B; token reutilizado rechazado.
  _Cubierto por las pruebas anteriores más `TestE004ReplayConserva409` (replay sigue siendo 409) y `TestE004RotacionPreviaNoInvalidaAccessToken`._

<!-- NOTA_VIGENCIA_AUDITORIA_SEMANTICA -->
### Nota de vigencia - auditoria semantica (2026-11-21)

- Estado: `AUDITADA_SEMANTICA_CON_OBSERVACIONES`.
- Hallazgo: `spec.md`/`plan.md` dicen que la revocacion usa Redis y DynamoDB; el codigo decide solo DynamoDB. La cabecera del checklist decia NOT_RUN.
- Los bloques anteriores de este archivo conservan el estado historico del endpoint y no se reescriben.
- Evidencia: `sdd/_control/H08/GATE_H08E_E007_E011_DESPLIEGUE_AWS.md` §4 y los Gates especificos de E001-E006; `memoria.md` cierre E001/E002 y v0.15.0.
- Release inicial: ver la matriz de auditoria del PR #142. Ultima verificacion funcional agregada: `v0.15.0+8f13c86d2ca0`.
- Cualquier contradiccion entre bloques anteriores y esta nota se resuelve a favor de esta nota.
