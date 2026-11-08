# I-070 — Runbook de operación IaC

| Campo | Valor |
|---|---|
| Estado | `IMPLEMENTADO_LOCAL`; ninguna ejecución AWS realizada |
| Locking | S3 nativo (`use_lockfile`, Terraform >= 1.10); DynamoDB deprecado y no usado |
| Alcance | Bootstrap, backend remoto, drift, locks, recuperación y rollback |

## 1. Activación del bootstrap (requiere Gate de infraestructura)

1. Revisar `infra/terraform/bootstrap` con `fmt -check`, `init -backend=false` y `validate`.
2. Aportar `sufijo_cuenta` real y `region = us-east-2`; no usar marcadores.
3. `plan -out` con credenciales humanas y revisión del diff por el responsable.
4. `apply` sólo tras autorización explícita; registrar fecha, operador y salida sanitizada.
5. Verificar por lectura: versionado, cifrado, Block Public Access y política TLS.

Sin autorización humana no se ejecuta `plan` ni `apply`. CI de PR nunca aplica infraestructura.

## 2. Activación del backend remoto

1. Copiar `backend.tf.example` a `backend.tf` sólo cuando el bootstrap esté aplicado y verificado.
2. Sustituir los placeholders por los valores reales de los outputs.
3. `use_lockfile = true` (Terraform >= 1.10). No añadir `dynamodb_table`: el locking DynamoDB
   está deprecado y no se ha presupuestado una tabla adicional.
4. `init` con backend real requiere autorización específica; el state local previo debe estar vacío.
5. Verificar que `terraform.tfstate` local no existe ni se versiona (`.gitignore` cubre `*.tfstate*`).

## 3. Drift

- Detectar con `plan -refresh-only` en una ventana autorizada; nunca corregir automáticamente.
- Registrar el diff y decidir: aceptar el cambio (y reflejarlo en HCL) o revertirlo con un plan revisado.
- `prevent_destroy` en el bucket de state: cualquier retirada exige edición explícita y Gate.

## 4. Lock huérfano (`terraform.tfstate.tflock`)

1. Confirmar que no hay ninguna ejecución de Terraform en curso (persona o CI).
2. Revisar el objeto `envs/h08i/terraform.tfstate.tflock` en el bucket: contenido, fecha y principal.
3. Si el lock es huérfano, `terraform force-unlock <LOCK_ID>` requiere autorización humana y registro;
   **no** se borra el objeto a mano ni el state.
4. Nunca eliminar el bucket de state ni su versionado para "arreglar" un lock.

## 5. Recuperación de state corrupto o sobrescrito

1. Identificar la versión anterior del objeto con el versionado de S3 (`list-object-versions`).
2. Restaurar la versión elegida a una clave temporal y revisarla antes de promoverla.
3. Promover sólo con autorización humana y con el lock libre; registrar `versionId` usado.
4. No editar el JSON del state a mano ni usar `terraform state` sobre recursos remotos en este Gate.

## 6. Secuencia de despliegue y rollback

1. bootstrap → 2. red → 3. IAM → 4. DynamoDB → 5. Redis → 6. cómputo → 7. balanceo → 8. observabilidad → 9. DNS.
2. Destrucción en orden inverso; state y datos persistentes se preservan salvo autorización separada.
3. Rollback: redeploy del artefacto anterior y, si el fallo es de configuración, plan revisado con el commit previo.

## 7. Prohibiciones

- `-auto-approve`, `destroy` masivo por ahorro, deploy desde PR no confiable.
- `init` con backend real, `migrate-state` o `force-unlock` sin autorización humana.
- Secretos en `tfvars`, state versionado, logs o evidencias.
- Cambiar Free → Paid o activar servicios no presupuestados.

## 8. Evidencia a registrar

Sólo identificadores enmascarados, fechas, comandos ejecutados y resultados. Nunca contenido de state,
credenciales ni ARN completos con ID de cuenta en documentos publicables.
