# Procedimiento de recuperación del state Terraform — I-130 (INF-130-03)

| Campo | Valor |
|---|---|
| Spec | `I-130-backup-recuperacion` |
| Versión | 0.1.0 |
| Estado | **IMPLEMENTADO_LOCAL — NO DESPLEGADO** |
| Ejecución AWS | `TERRAFORM_STATE_RECOVERY_AWS_NOT_VERIFIED` |

> El state **no** es un dato de usuario: recuperar una versión anterior exige Gate, copia previa y
> escritor único. Aquí no se ejecutó ningún comando S3 ni se modificó el state real.

## 1. Propiedades declaradas del bucket (I-070)

| Propiedad | Requisito | Evidencia |
|---|---|---|
| Privacidad | Block Public Access + ownership `BucketOwnerEnforced` | `infra/terraform/bootstrap` |
| Versionado | habilitado | idem |
| Cifrado | SSE-S3 | idem |
| Protección de borrado | `prevent_destroy` | idem |
| Locking | nativo de S3 (`use_lockfile`) | `RUNBOOK_IAC.md` de I-070 |
| Auditoría | CloudTrail de la cuenta (no verificado) | `NOT_VERIFIED` |
| Secretos | el state puede contener la contraseña de Redis; nunca se publica | `RFC-004` abierto |

## 2. Procedimiento (8 pasos, todos bloqueados en local)

1. **Identificar versión exacta** del objeto de state (`version_id`); sin identificador no hay recuperación.
2. **Verificar integridad**: comparar `sha256` esperado y observado.
3. **Registrar aprobación** humana con referencia a PR/SHA.
4. **Preservar el state actual** copiándolo antes de tocar nada.
5. **Garantizar reversión** con la copia previa y la versión identificada.
6. **Rechazar escrituras concurrentes**: un token de escritura único.
7. **No exponer secretos**: prohibido volcar el state o sus valores en logs.
8. **Exigir Gate de despliegue** antes de cualquier operación real en AWS.

```powershell
py -3.13 herramientas/verificar_recuperacion.py plan-state --solicitud state.json
```

Devuelve `PLAN_LOCAL_VALIDADO` o `RECUPERACION_RECHAZADA` con la lista de requisitos faltantes y el
alcance `TERRAFORM_STATE_RECOVERY_AWS_NOT_VERIFIED`.

## 3. Concurrencia

```powershell
py -3.13 herramientas/verificar_recuperacion.py concurrencia --solicitud lock.json
```

`ESCRITOR_CONCURRENTE` bloquea la operación y `SOLICITUD_INCOMPLETA` la rechaza si falta identidad de
escritor (`token_activo` o `solicitante`). `ESCRITOR_UNICO_SIMULADO` solo indica coherencia del escenario
local: **no** acredita un lock real de S3. I-070 sigue siendo la autoridad del locking y no se implementa
ni ejecuta ningún mecanismo remoto en I-130.

## 4. Lo que este procedimiento no hace

- No recupera una versión real (requiere AWS y Gate).
- No sustituye el runbook de I-070 (`RUNBOOK_IAC.md`).
- No garantiza que un state restaurado sea correcto: se compara el hash y se valida con
  `terraform plan` en una fase posterior autorizada.
