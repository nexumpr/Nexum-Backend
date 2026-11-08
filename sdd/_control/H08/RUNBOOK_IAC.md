# I-070 — Runbook de operación IaC

| Campo | Valor |
|---|---|
| Estado | `IMPLEMENTADO_LOCAL`; ninguna ejecución AWS realizada |
| Locking | S3 nativo (`use_lockfile`, Terraform >= 1.10); DynamoDB deprecado y no usado |
| States | `bootstrap/terraform.tfstate` y `envs/h08i/terraform.tfstate` (claves distintas, locks distintos) |
| Alcance | Bootstrap, custodia/migración de su state, backend remoto, drift, locks, recuperación y rollback |

> **Regla transversal:** ningún procedimiento de este runbook se ejecuta en este Gate. `terraform apply`,
> `plan` remoto, `init` con backend real, `init -migrate-state`, `state push`, `import` y `destroy` requieren
> autorización humana específica y credenciales aportadas por el operador. Nada de esto corre en CI de PR.

## 1. Activación del bootstrap (Gate AWS específico)

1. Revisar `infra/terraform/bootstrap` con `fmt -check`, `init -backend=false` y `validate`.
2. Aportar `sufijo_cuenta` real y `region = us-east-2`; no usar marcadores.
3. `plan -out` con credenciales humanas y revisión del diff por el responsable.
4. `apply` sólo tras autorización explícita; registrar fecha, operador y salida sanitizada.
5. Anotar los outputs (bucket y `configuracion_backend`) sin publicar el ID de cuenta.

El bootstrap se aplica **con state local**: no tiene bloque `backend` justamente para evitar la dependencia
circular de un backend que aún no existe. No añadir `backend` antes del primer `apply` correcto.

## 2. Custodia del state local del bootstrap

Tras el primer `apply`, en `infra/terraform/bootstrap/` existe `terraform.tfstate`. Ese archivo es **crítico**:
perderlo deja los recursos del state sin trazabilidad y multiplica el riesgo de recrearlos. Mientras la migración
no esté verificada, el state **original permanece en su ubicación operativa**: es el archivo que Terraform
necesita leer para `init -migrate-state`. Lo que se duplica es la **copia de seguridad**, no el state operativo.

1. Confirmar que el `apply` creó el state local y que `terraform state list` devuelve exactamente las direcciones
   declaradas (ver §3.1); no dar por bueno un número histórico fijo.
2. Generar una copia de seguridad **cifrada fuera del repositorio** (volumen cifrado o gestor de secretos del
   operador) manteniendo intacto el original en `infra/terraform/bootstrap/`.
3. Calcular el SHA-256 del respaldo, anotarlo y **verificar la integridad** recalculando el hash sobre la copia.
4. Registrar `lineage` y `serial` (metadatos, no contenido) junto al hash y la fecha, sin exponer información
   sensible.
5. Mantener **dos copias** del respaldo con control de acceso individual; comprobar que no queda ninguna copia en
   `%TEMP%` compartido ni en carpetas sincronizadas sin cifrar.
6. Confirmar `.gitignore`: `*.tfstate` y `*.tfstate.*` excluidos; comprobar que `git ls-files` no devuelve ningún
   `.tfstate`.
7. No adjuntar el state a GitHub Actions artifacts, releases, correos ni documentación del proyecto.
8. Etiquetar los recursos creados con `Spec = "I-070-terraform"` (ya declarado) para permitir inventario.

**Prohibiciones:** subir el state al repositorio, compartirlo por chat, editarlo a mano, guardarlo sin cifrado,
**mover o eliminar el único state operativo antes de completar la migración**, o retirar `prevent_destroy` del
bucket para facilitar una recuperación.

## 3. Migración del state de bootstrap a backend persistente

Sólo después de que el bucket exista y esté verificado (versionado, SSE, Block Public Access, política TLS).

1. Confirmar que el state local sigue en su ubicación operativa y que el respaldo cifrado de §2 está verificado.
   Sin copia íntegra no se migra.
2. Crear `infra/terraform/bootstrap/backend.tf` a partir de la plantilla, con `key` **distinta** de la del
   runtime: `bootstrap/terraform.tfstate` (runtime: `envs/h08i/terraform.tfstate`) y `use_lockfile = true`.
3. Ejecutar `terraform init -migrate-state` **exclusivamente** con autorización humana futura; responder al
   prompt de copia verificando que Terraform reporta el bucket y la clave esperados.
4. Verificar el state remoto: `terraform state list` debe devolver **las mismas direcciones** que el inventario
   declarado (§3.1) y ningún recurso ajeno.
5. Comparar `lineage`, `serial` y recursos con la evidencia previa. **No se asume que una migración incremente
   el `serial`**: el criterio es que el contenido (direcciones e identificadores) coincida, no un número
   concreto. Si `lineage` o el conjunto de recursos difieren, detenerse y revisar antes de continuar.
6. Ejecutar `plan` (autorizado) y exigir **ausencia de cambios de infraestructura inesperados** antes de dar la
   migración por terminada.
7. Sólo entonces retirar la copia operativa local, y únicamente cuando la migración **y** una lectura de
   recuperación (§4) estén comprobadas.
8. Conservar el respaldo cifrado conforme a la política de retención autorizada; no borrarlo en la misma sesión.
9. Si la migración falla: no repetir a ciegas; restaurar desde el respaldo cifrado, registrar el error y no
   borrar nunca el bucket ni el state remoto para "empezar de nuevo".

Cada state tiene su propio objeto `*.tflock`; el bucket es compartido, las claves no. No hay tabla DynamoDB de
locking (mecanismo deprecado y no presupuestado).

### 3.1 Inventario declarativo (no un número fijo)

El inventario se obtiene de los recursos **realmente declarados** en `infra/terraform/bootstrap`, no de una cifra
histórica. Con la configuración actual son **8 direcciones Terraform**: 7 recursos AWS y 1 recurso local
`terraform_data` (validación de precondiciones, no crea nada en AWS).

```
terraform_data.validacion_bootstrap
aws_s3_bucket.state
aws_s3_bucket_versioning.state
aws_s3_bucket_server_side_encryption_configuration.state
aws_s3_bucket_public_access_block.state
aws_s3_bucket_ownership_controls.state
aws_s3_bucket_policy.state
aws_s3_bucket_lifecycle_configuration.state
```

Si se añade o retira un recurso en el código, el inventario cambia con él: la comprobación es
`terraform state list` **contra el código**, nunca contra un 7/8 memorizado. Además se distingue expresamente
entre recursos AWS y `terraform_data`, que no debe confundirse con un recurso en la nube.

## 4. Recuperación si se pierde el state del bootstrap

El bootstrap gestiona pocos recursos y todos llevan tag `Spec = "I-070-terraform"`. La recuperación es por
**importación**, nunca por recreación.

1. Inventariar por lectura autorizada: bucket, versionado, SSE, ownership, Block Public Access, política y
   lifecycle. Compararlos con el código y con el inventario de §3.1.
2. Escribir bloques `import` **sólo** para los recursos AWS previstos, usando la dirección exacta del código:
   `aws_s3_bucket.state`, `aws_s3_bucket_versioning.state`,
   `aws_s3_bucket_server_side_encryption_configuration.state`, `aws_s3_bucket_public_access_block.state`,
   `aws_s3_bucket_ownership_controls.state`, `aws_s3_bucket_policy.state` y
   `aws_s3_bucket_lifecycle_configuration.state`, con los identificadores reales aportados por el operador.
   `terraform_data.validacion_bootstrap` no se importa desde AWS.
3. Confirmar que cada recurso se importa **en una única dirección** y que no hay dos `import` apuntando al mismo
   identificador real.
4. Ejecutar `plan` (autorizado). El plan de una importación **no es un plan normal sin cambios**: se acepta
   exactamente `N to import` (el número de direcciones previstas) junto con:

   - **0 to add** — no se crea ningún recurso nuevo;
   - **0 to change** — no se modifica ningún atributo no autorizado;
   - **0 to replace** — ningún recurso se reemplaza;
   - **0 to destroy** — no se destruye nada.

   Si el plan propone **reemplazar o recrear el bucket**, detenerse: la importación está mal formada o los
   identificadores no son los correctos.
5. `apply` del import sólo con **Gate humano independiente**; jamás `-auto-approve`.
6. Registrar `lineage`/`serial` resultantes y guardar copia cifrada del state nuevo; verificar después que
   `plan` ya no propone importaciones ni cambios.

Si el bucket se creó fuera de Terraform o no hay certeza de sus atributos, **no** se importa a ciegas: se
documenta el estado real y se decide con el responsable.

## 5. Drift

- Detectar con `plan -refresh-only` en ventana autorizada; nunca corregir automáticamente.
- Clasificar: cambio no gestionado, deriva de consola, recurso eliminado.
- Registrar el diff y decidir: aceptar (reflejar en HCL) o revertir con plan revisado.

## 6. Lock huérfano (`terraform.tfstate.tflock`)

1. Confirmar que no hay ejecución de Terraform en curso (persona o CI).
2. Revisar el objeto de lock: fecha, principal y contenido acotado.
3. Si es huérfano, `terraform force-unlock <LOCK_ID>` con autorización humana y registro.
4. Nunca borrar el objeto de lock a mano, ni el state, ni el bucket.

## 7. Recuperación de state corrupto o sobrescrito

1. Elegir la versión anterior con `list-object-versions` (versionado habilitado).
2. Restaurar a una clave temporal y revisarla sin promoverla automáticamente.
3. Promover sólo con el lock libre y autorización humana; registrar `versionId`.
4. No editar el JSON del state a mano ni usar `state push`.

## 8. Secuencia de despliegue y rollback

1. bootstrap → 2. red → 3. IAM → 4. DynamoDB → 5. Redis → 6. cómputo → 7. balanceo → 8. observabilidad → 9. DNS.
2. Destrucción en orden inverso; state y datos persistentes se preservan salvo autorización separada.
3. Rollback: redeploy del artefacto anterior y, si el fallo es de configuración, plan revisado con el commit previo.

## 9. Prohibiciones

- `-auto-approve`, `destroy` masivo por ahorro, deploy desde PR no confiable.
- `init` con backend real, `migrate-state`, `force-unlock`, `state push`, `import` o `apply` sin autorización.
- Publicar state, credenciales, ARN completos o identificadores de cuenta.
- Cambiar Free → Paid o activar servicios no presupuestados.
- Retirar `prevent_destroy` del bucket de state para "facilitar" una migración o recuperación.
- Mover o borrar el state local operativo antes de que la migración esté verificada; o borrar el bucket/state
  para corregir un lock huérfano.

## 10. Evidencia a registrar

Sólo identificadores enmascarados, fechas, comandos ejecutados, hashes de state (no contenido) y resultados.
