# Gate H08-E — OLEADA E007–E011

| Campo | Valor |
|---|---|
| Documento | `sdd/_control/H08/GATE_H08E_E007_E011_OLEADA.md` |
| Fecha | 2026-11-17 |
| Base de trabajo | `main` `4ba790a9ffa99b15bb73a6b2e93e691a2ba716b6` |
| Instrumento de gobierno | `RFC-019-OLEADA_E007_E011.md` |
| Estado | `AUTHORIZED_E007_E011_LOCAL` |

## 1. Alcance autorizado

Instrucción humana de la sesión del 2026-11-17: implementar, integrar, desplegar y verificar
**E007, E008, E009, E010 y E011**. E012 y posteriores siguen `NOT_AUTHORIZED`.

| Endpoint | Operación | Gate individual |
|---|---|---|
| E007 | `POST /api/v1/auth/password/change` | `GATE_H08E_E007.md` |
| E008 | `GET /api/v1/auth/sessions` | `GATE_H08E_E008.md` |
| E009 | `DELETE /api/v1/auth/sessions/{sessionId}` | `GATE_H08E_E009.md` |
| E010 | `POST /api/v1/auth/email-verification/request` | `GATE_H08E_E010.md` |
| E011 | `POST /api/v1/auth/email-verification/confirm` | `GATE_H08E_E011.md` |

## 2. Lo que este Gate autoriza

1. Escribir código Go de los cinco endpoints en worktrees aislados bajo `.worktrees/`.
2. Abrir **un PR por endpoint**, en el orden funcional E007 → E008 → E009 → E010 → E011.
3. Ejecutar `go build`, `go vet`, `gofmt` y las pruebas locales permitidas por el host.
4. Publicar evidencia de contrato y de CI.

## 3. Lo que este Gate NO autoriza

- Merge: exige `APRUEBO_MERGE_PR_<N>_SHA_<SHA40>` por PR.
- `terraform plan`: exige su propio permiso y Gate.
- `terraform apply`: exige `APRUEBO_APPLY_..._SHA256_<HASH>` sobre el plan exacto.
- `instance refresh`, publicación S3, cambios IAM: exigen Gate de despliegue.
- E012..E075.
- Cambios en E001–E006, en `constitution.md` o en reglas `deny`.

## 4. Estado por endpoint

Se actualiza en cada checkpoint; nunca se adelanta un estado.

| Endpoint | Código | PR | CI | Merge | Release | AWS | HTTPS | Android |
|---|---|---|---|---|---|---|---|---|
| E007 | NOT_STARTED | — | — | — | — | — | — | — |
| E008 | NOT_STARTED | — | — | — | — | — | — | — |
| E009 | NOT_STARTED | — | — | — | — | — | — | — |
| E010 | NOT_STARTED | — | — | — | — | — | — | — |
| E011 | NOT_STARTED | — | — | — | — | — | — | — |

## 5. Frases pendientes de aprobación humana

| Necesidad | Frase |
|---|---|
| Gobierno de la oleada | `APRUEBO_RFC_019_OLEADA_E007_E011` |
| Merge de cada PR | `APRUEBO_MERGE_PR_<N>_SHA_<SHA40>` |
| Apply del plan de release | `APRUEBO_APPLY_<ALCANCE>_SHA256_<HASH>` |

Ninguna de estas frases se ha concedido todavía. Este documento no las sustituye.
