# Cierre de Oleada 2 - evidencia funcional

| Release | `v0.18.0+9722224c15c5` |
|---|---|
| Main | `9722224c15c5a6b8f76cbc53c699b957216082e4` |
| Binario ARM64 SHA-256 | `249017ee0ef828fa9316618ecea5f930cbdd15f4325c5fcb86d73cd020353e74` |
| Launch Template | v14 |
| Instance refresh | `faae70c4-b01a-437b-9258-cbd1fce53caa` Successful 100 % |
| ASG | 2/2/2 - `i-037aa73e08855de59`, `i-0c8119d2bee0a85ec` (us-east-2a) |
| Targets | 2/2 healthy |
| Health / readiness | 200 / 200 |
| Rollback | LT v13 + `v0.17.0+87aac167fe28` |

## Matriz de endpoints

| E-ID | Contrato | PR | HTTP positivo | Persistencia | Seguridad | Resultado |
|---|---|---|---|---|---|---|
| E014 | `DELETE /users/me` | #116 | PASS | PASS | PASS | **FUNCTIONAL_PASS** |
| E027 | `GET /movements` | #103 | PASS | PASS | PASS | **FUNCTIONAL_PASS** |
| E028 | `POST /movements` | #102 | PASS | PASS | PASS | **FUNCTIONAL_PASS** |
| E029 | `GET /movements/{movementId}` | #104 | PASS | PASS | PASS | **FUNCTIONAL_PASS** |
| E030 | `PATCH /movements/{movementId}` | #105 | PASS | PASS | PASS | **FUNCTIONAL_PASS** |
| E031 | `DELETE /movements/{movementId}` | #106 | PASS | PASS | PASS | **FUNCTIONAL_PASS** |
| E032 | `GET /recurring-payments` | #109 | PASS | PASS | PASS | **FUNCTIONAL_PASS** |
| E033 | `POST /recurring-payments` | #107 | PASS | PASS | PASS | **FUNCTIONAL_PASS** |
| E034 | `GET /recurring-payments/{id}` | #108 | PASS | PASS | PASS | **FUNCTIONAL_PASS** |
| E035 | `PATCH /recurring-payments/{id}` | #110 | PASS | PASS | PASS | **FUNCTIONAL_PASS** |
| E036 | `DELETE /recurring-payments/{id}` | #111 | PASS | PASS | PASS | **FUNCTIONAL_PASS** |
| E038 | `GET /goals` | #114 | PASS | PASS | PASS | **FUNCTIONAL_PASS** |
| E039 | `POST /goals` | #112 | PASS | PASS | PASS | **FUNCTIONAL_PASS** |
| E040 | `GET /goals/{goalId}` | #113 | PASS | PASS | PASS | **FUNCTIONAL_PASS** |
| E041 | `PATCH /goals/{goalId}` | #115 | PASS | PASS | PASS | **FUNCTIONAL_PASS** |

## Evidencia y limites

- E014: 42/42 PASS (dos usuarios sinteticos; purga PURGED verificada; aislamiento bidireccional; usuario B intacto).
- E027-E031: CRUD, idempotencia (replay/409), cursor, If-Match (412), aislamiento.
- E032-E036: 9 frecuencias (7 canonicas + 2 alias normalizados), If-Match, baja logica, aislamiento.
- E038-E041: openingSaved, totals, If-Match, 422 target<saved, aislamiento.
- Regresion ejecutada en esta release: health/readiness; certificacion E014. Resto con evidencia historica.
- E014 trabajos antiguos: `NOT_VERIFIED` (requestId no conservados; Scan prohibido).
- E015: `PENDING_LEGAL`. SES: `PENDING` (E006/E011 PARTIAL).
- Multi-AZ: `LIMITED` (ambas EC2 en us-east-2a).
