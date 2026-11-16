# RFC-017 — Permisos permanentes de OpenCode para E005–E075

| Campo | Valor |
|---|---|
| Documento | `sdd/_control/H08/RFC-017-PERMISOS_PERMANENTES.md` |
| Estado | **PROPUESTO — NO APROBADO — NO INSTALADO** |
| Fecha | 2026-11-16 |
| Motor verificado | OpenCode **v2.2.7** (Windows 11, PowerShell 7) |
| Gate solicitado | `APRUEBO_RFC_017_PERMISOS_PERMANENTES` |
| Autoriza | Sustituir `opencode.json` por la configuración reconciliada de §4. **No** autoriza AWS, Terraform, despliegue ni merge |

---

## 1. Problema

Cada endpoint (E003, E004…) obligó a editar `opencode.json` a mano para añadir excepciones
temporales, aplicarlas, usarlas y retirarlas. Consecuencias observadas:

- `terraform apply` bloqueado por **dos** capas (permiso `deny` + política `deny`), lo que obligó a
  excepciones por nombre de plan (y falló una vez porque `-v2` rompía la cadena del glob).
- Ediciones de `spec.md`/`plan.md`/OpenAPI denegadas por completo, aunque una enmienda aprobada
  por RFC exige justamente editarlas.
- Lecturas operativas seguras (`describe-*`) pidiendo autorización en cada despliegue.
- El archivo local del usuario divergió del versionado (**161/43** frente a **151/33**), de modo
  que el manifiesto SDD nunca coincidía con el estado real.

Objetivo: una configuración **permanente**, versionada, que permita trabajo fluido sin degradar
ninguna barrera de seguridad y que deje de requerir ediciones manuales por endpoint.

## 2. Tres configuraciones comparadas

| Ref | Origen | Permissions | Policies | SHA-256 |
|---|---|---|---|---|
| **A** | `HEAD` versionado (`git show HEAD:opencode.json`) | 151 | 33 | `244d82adfec274e2e3b327a66e929f602ed4053d1c08bf0a45373b1cdc4088b6` |
| **B** | `opencode.json` local del usuario (rollouts E003/E004) | 161 | 43 | `B281E2639A9595FBB0CDFC7A882B30414ED31988D7D1F2F817452242F1163A37` |
| **C** | Reconciliada (**propuesta**) | 178 | 47 | `5579CF8A21210CB2B8FD339C2DED1FF84421B44FA30B8F2E66141F7B277E007D` |

**C no es una copia de B ni del candidato del ZIP.** Se construye desde **A** con
`herramientas/construir_opencode_permanente.py` (12 reemplazos exactos con ancla única + 2 bloques
nuevos). Es determinista: dos ejecuciones producen el mismo SHA-256.

Comparación de cobertura ejecutada con `--cobertura` (reglas de la base ausentes o con efecto
distinto en C):

| Base | Ausentes | Degradadas | Endurecidas |
|---|---|---|---|
| A (HEAD) | 1 | 11 | 0 |
| B (local) | 3 | 15 | 0 |

**Ninguna degradación es accidental: las 15 están enumeradas y justificadas en §5.**

## 3. Reglas retiradas (3, todas deliberadas)

| # | Regla | Motivo |
|---|---|---|
| 1 | `policy: tool.use shell:terraform apply* → deny` | Hacía inútil cualquier `ask`. Es **la** decisión central de esta RFC: retirar la denegación dura para que `apply` sea interactivo. Sigue exigiendo Gate humano por plan y SHA-256 |
| 2 | `permission: shell aws autoscaling describe-auto-scaling-groups * → ask` | Redundante: `...describe-auto-scaling-groups*` ya cubre el sufijo, incluido `" --args"` (el glob `*` cruza espacios, verificado por caso de prueba) |
| 3 | `permission: shell aws ec2 describe-instances * → ask` | Idem |

## 4. Reglas conservadas de B (10)

Diez `policy: tool.use ... allow` de ASG/ELBv2/EC2 añadidas durante los rollouts E003/E004.
**Se conservan** aunque `policies` no conceda permiso, porque retirarlas alteraría un
comportamiento ya observado en el motor sin prueba que lo respalde. Su limpieza queda como tarea
posterior con prueba en motor real, no en esta RFC.

## 5. Cambios de efecto (15) y reglas nuevas (32)

### 5.1 `deny → ask` (5)

| Regla | Motivo | Riesgo que permanece |
|---|---|---|
| `permission shell terraform apply*` | Deploy autorizado por Gate de plan + SHA-256 | **Alto**: un `apply` sin revisión de plan ya no lo impide el archivo. Mitigación: Gate por plan y por SHA-256, `destroy/import/state` siguen denegados |
| `permission edit sdd/**/spec.md` | Enmiendas aprobadas por RFC | Aplicar una enmienda sin RFC deja de ser imposible por permiso. Mitigación: `DECISIONES_CONTRATO_*.json` detecta cambios fuera de decisión; `constitution.md` sigue denegado |
| `permission edit sdd/**/plan.md` | Idem | Idem |
| `permission edit sdd/03-endpoints/**/openapi/**` | Enmiendas de contrato aprobadas | `comparar_contrato_api.py` en CI detecta rupturas |
| `permission edit sdd/03-endpoints/**/openapi.*` | Idem (espejo JSON) | Idem |

### 5.2 `ask → allow` (10)

Solo lecturas **sin datos personales**: `sts get-caller-identity`, `ec2 describe-instances`,
`ec2 describe-instance-types`, `ec2 describe-instance-type-offerings`,
`ec2 describe-launch-templates`, `ec2 describe-launch-template-versions`,
`elbv2 describe-load-balancers`, `elbv2 describe-target-groups`, `elbv2 describe-target-health`,
`autoscaling describe-auto-scaling-groups|instance-refreshes|scaling-activities`.

Motivo: se ejecutan en **cada** verificación de despliegue y no revelan datos de usuarios.
Se mantienen en `ask`: `dynamodb get-item|query|describe-*`, `s3api list-objects-v2|list-buckets`,
`iam get-role|get-role-policy|list-*`, `cloudfront list-*`, `elasticache describe-*`,
`pricing get-products`, `ce get-cost-and-usage`, `budgets describe-*`, `freetier *`.

> `s3api list-objects-v2` se deja en `ask` a propósito: enumera también el bucket de secretos.

### 5.3 Reglas nuevas `ask` (22)

| Grupo | Reglas |
|---|---|
| Publicación de artefactos | `aws s3api put-object *`, `aws sts assume-role *` |
| Auto Scaling / rollout | `update-auto-scaling-group`, `start-instance-refresh`, `cancel-instance-refresh`, `set-instance-protection` |
| Launch Template | `ec2 create-launch-template`, `create-launch-template-version`, `modify-launch-template` |
| IAM de despliegue | `iam create-role`, `put-role-policy`, `get-role-policy`, `list-role-policies`, `create-instance-profile`, `add-role-to-instance-profile`, `get-instance-profile` |
| Lecturas con posible dato personal | `dynamodb get-item`, `dynamodb query` |

### 5.4 Reglas nuevas `deny` (3 permiso + 5 política)

`permission`: `aws ec2 terminate-instances *`, `aws dynamodb delete-table *`, `aws iam delete-*`
(redundantes con `aws *` y `aws * delete-*`, pero explícitas y verificadas por prueba).

`policy` (capa dura): `aws * delete-*`, `aws ec2 terminate-instances *`,
`aws dynamodb delete-table *`, `aws iam delete-*`, `git push -f *`.

### 5.5 Invariantes que **no** cambian

`terraform destroy|import|state`, `terraform init -migrate-state|-reconfigure`,
`git push origin main*`, `git push --force*`, `git reset --hard*`, `git clean *`,
`docker *`, `wsl *`, `kubectl *`, `Remove-Item *`, `rm *`, `constitution.md`,
`.env*`, `*.tfstate*`, `*.tfvars*`, `*.pem`, `*.key`, `*/.aws/*`, `*/.ssh/*`.

**`aws *` no se convierte en `allow` en ningún caso.**

## 6. Precedencia (semántica observada y verificada)

| Capa | Comportamiento | Evidencia |
|---|---|---|
| `permissions` | **Última coincidencia prevalece** | Las reglas AWS añadidas al final ganan al `aws *` deny del inicio |
| `policies` | Solo **endurece** (`deny`); **no** concede | `git status` se ejecutó con `allow` en `permissions` y **sin** entrada en `policies` |
| Glob | `*` cruza espacios | `aws ec2 describe-instances*` cubre `... --filters x` |

**NO demostrado en el motor:** que `policies ... allow` sea irrelevante para `ask`. Por eso §4 las
conserva. Tampoco se ha probado la recarga dinámica: si el motor no recarga, la configuración
aplicada surtirá efecto en sesión nueva.

## 7. Riesgos

| # | Riesgo | Severidad | Control |
|---|---|---|---|
| R1 | `terraform apply` ya no está bloqueado por archivo; un error humano puede aplicar un plan no revisado | **Alta** | Gate por plan y SHA-256, `apply <archivo.tfplan>` explícito, `destroy/import/state` denegados, `-auto-approve` no concedido |
| R2 | `spec.md`/`plan.md`/OpenAPI editables con un clic | **Alta** | RFC + Gate por enmienda; `DECISIONES_CONTRATO_*.json` detecta contrato fuera de decisión en CI; `constitution.md` sigue denegado |
| R3 | Reglas AWS añadidas al final pueden quedar antes de un `deny` si alguien reordena | Media | 63 casos de verificación en `herramientas/tests/test_permisos_opencode.py` |
| R4 | Las 10 `policy ... allow` conservadas son ruido | Baja | Documentadas; limpieza posterior con prueba en motor |
| R5 | Semántica del motor distinta de la simulada | Media | Prueba obligatoria post-instalación (§9) antes de declarar operativo |
| R6 | Permisos IAM de AWS más amplios de lo necesario | Media | Aquí solo se autoriza *solicitar*; la identidad efectiva la fija IAM, no este archivo |

## 8. Plan de reversión

1. Copia previa: `C:\Nexum-Custodia\H08E-PRE-E005-20261116\opencode.local.ORIGINAL.json`
   (SHA-256 `B281E263…3A37`).
2. Reversión en árbol: `git checkout -- opencode.json` (restaura `HEAD`) o copia del respaldo.
3. Reversión de una regla concreta: editar esa entrada y volver a ejecutar el verificador
   (`--objetivo-permanente` debe seguir en PASS).
4. **Prohibido** revertir con `git reset --hard` o `git clean`.
5. Si el motor rechazara la configuración: restaurar el respaldo, registrar `D-PERM-02` y no
   continuar hasta diagnosticar.

## 9. Pruebas

| Prueba | Estado |
|---|---|
| `verificar_permisos_opencode.py` sobre A (suelo de gobierno, 33 casos) | **PASS** |
| `verificar_permisos_opencode.py` sobre C (`--objetivo-permanente`, 63 casos) | **PASS** |
| Cobertura A→C y B→C | ejecutada; resultados en §2 |
| `herramientas/tests/test_permisos_opencode.py` (14 pruebas, incluye 2 negativas) | **PASS** |
| Determinismo del constructor (mismo SHA-256 en dos ejecuciones) | **PASS** |
| Motor real: `opencode debug config` con C instalada | **NOT_RUN** (no instalado) |
| Motor real: `opencode debug config` con A/B instalada | **PASS** (A/B: 161/43 efectivos) |

## 10. Dependencias de autorización

1. `APRUEBO_RFC_017_PERMISOS_PERMANENTES` — aprobar **este documento**.
2. Instalación: sustituir `opencode.json` por C, verificar JSON, `opencode debug config`,
   comprobar lecturas `allow`, comprobar `deny` con comandos **no mutantes**, comprobar un `ask`
   real con una operación inocua, y **no** elegir *Allow always* para operaciones sensibles.
3. Gate de merge del PR con HEAD SHA40.
4. Cada mutación AWS sigue exigiendo su Gate propio: **ASK no es autorización**.

## 11. Estado

**No se ha modificado `opencode.json`.** El archivo local del usuario permanece intacto
(SHA-256 `B281E263…3A37`) y el candidato vive fuera del árbol versionado.

## 12. Diff definitivo de tres vías (2026-11-16)

| Configuración | Permissions | Policies |
|---|---|---|
| `main` (versionada) | 151 | 33 |
| Local del usuario (intacta) | 161 | 43 |
| **Candidato reconciliado** | **178** | **47** |

**Reglas nuevas respecto a `main`: 27.** Incluyen lecturas operativas sin datos
personales (STS, EC2/ELBv2/ASG `describe-*`), ASK para `s3api put-object`, `sts assume-role`,
ASG/LT, IAM de despliegue, `dynamodb get-item|query`, y DENY explícitos de
`terminate-instances`, `delete-table`, `iam delete-*`.

**Reglas retiradas: 0.**

- ninguna

**Reglas con efecto cambiado: 11** (todas deliberadas: SDD a ASK, apply a ASK y
lecturas a ALLOW; ver §5).

**Policies retiradas: 1.** Solo la denegación dura de `terraform apply`, que es
la decisión central de esta RFC. Las 10 `policy ... allow` de la configuración local **se conservan**.

**Adiciones de este preflight** (lecturas, sin mutación): `aws ses get-account*`,
`aws ses list-email-identities*`, `aws sesv2 get-account*`, `aws logs describe-log-groups*`,
`aws ssm get-parameter*`. Con ellas el total asciende a **183 permisos**.

**No se aplica nada**: instalar el candidato exige `APRUEBO_RFC_017_PERMISOS_PERMANENTES`.
