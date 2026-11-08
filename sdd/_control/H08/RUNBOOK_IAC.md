# I-070 — Runbook de operación IaC

| Campo | Valor |
|---|---|
| Estado | `IMPLEMENTADO_LOCAL`; ninguna ejecución AWS realizada |
| Alcance | Bootstrap, backend remoto, drift, secuencia y rollback |

## 1. Activación del bootstrap (requiere Gate de infraestructura)

1. Revisar `infra/terraform/bootstrap` con `fmt -check`, `init -backend=false` y `validate`.
2. Aportar `sufijo_cuenta` real y `region = us-east-2`; no usar marcadores.
3. `plan -out` con credenciales humanas y revisión del diff por el responsable.
4. `apply` sólo tras autorización explícita; registrar fecha, operador y salida sanitizada.
5. Verificar por lectura: versionado, cifrado, Block Public Access y política TLS; `describe-table` del lock.

Sin autorización humana no se ejecuta `plan` ni `apply`. CI de PR nunca aplica infraestructura.

## 2. Activación del backend remoto

1. Copiar `backend.tf.example` a `backend.tf` sólo cuando el bootstrap esté aplicado y verificado.
2. Sustituir los placeholders por los valores reales de los outputs (bucket, tabla, key del entorno).
3. `init` con backend real requiere autorización específica; el estado local previo debe estar vacío.
4. Verificar que `terraform.tfstate` local no existe ni se versiona; `.gitignore` cubre `*.tfstate*`.

## 3. Drift

- Detectar con `plan -refresh-only` en una ventana autorizada; nunca corregir automáticamente.
- Registrar el diff y decidir: aceptar el cambio (y reflejarlo en HCL) o revertirlo con un plan revisado.
- `prevent_destroy` en bucket de state y tabla de lock: cualquier retirada exige edición explícita y Gate.

## 4. Secuencia de despliegue y rollback

1. bootstrap → 2. red → 3. IAM → 4. DynamoDB → 5. Redis → 6. cómputo → 7. balanceo → 8. observabilidad → 9. DNS.
2. Destrucción en orden inverso; el state y los datos persistentes se preservan salvo autorización separada.
3. Rollback: redeploy del artefacto anterior y, si el fallo es de configuración, plan revisado con el commit previo.

## 5. Prohibiciones

- `-auto-approve`, `destroy` masivo por ahorro, deploy desde PR no confiable.
- Secretos en `tfvars`, state versionado, logs o evidencias.
- `apply` sin plan revisado ni autorización humana.
- Cambiar Free → Paid o activar servicios no presupuestados.

## 6. Evidencia a registrar

Sólo identificadores enmascarados, fechas, comandos ejecutados y resultados. Nunca contenido de state, credenciales
ni ARN completos con ID de cuenta en documentos publicables.
