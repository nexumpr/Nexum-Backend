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
perderlo deja los recursos del state sin trazabilidad y multiplica el riesgo de recrearlos.

1. Calcular su SHA-256 y anotarlo junto a `serial`/`lineage` (sin copiar el contenido en chats, issues o PR).
2. Mover el archivo a un almacén cifrado fuera del repositorio (volumen cifrado o gestor de secretos del
   operador), con **dos copias** y control de acceso individual.
3. Verificar que no queda ninguna copia en el árbol de trabajo ni en `%TEMP%` compartido.
4. Confirmar `.gitignore`: `*.tfstate` y `*.tfstate.*` excluidos; comprobar que `git ls-files` no devuelve
   ningún `.tfstate` y que no se ha añadido a ningún commit.
5. No adjuntar el state a GitHub Actions artifacts, releases, correos ni documentación del proyecto.
6. Etiquetar los recursos creados con `Spec = "I-070-terraform"` (ya declarado) para permitir inventario.

**Prohibiciones:** subir el state al repositorio, compartirlo por chat, editarlo a mano, o guardarlo en una
carpeta sincronizada sin cifrado.

## 3. Migración del state de bootstrap a backend persistente

Sólo después de que el bucket exista y esté verificado (versionado, SSE, Block Public Access, política TLS).

1. Copia de seguridad cifrada del state local + hash (§2). Sin esa copia no se migra.
2. Crear `infra/terraform/bootstrap/backend.tf` a partir de la plantilla, con `key` **distinta** de la del
   runtime: `bootstrap/terraform.tfstate` (runtime: `envs/h08i/terraform.tfstate`) y `use_lockfile = true`.
3. `terraform init -migrate-state` con autorización humana explícita; responder al prompt de copia verificando
   que Terraform reporta la migración al bucket/tabla esperados.
4. Verificar: `terraform state list` (los 7 recursos del bootstrap), `serial` incrementado, `lineage` conservado
   y `plan` (autorizado) sin cambios pendientes.
5. Conservar la copia local cifrada como respaldo durante al menos una ventana; no borrarla en la misma sesión.
6. Si la migración falla: no repetir a ciegas; restaurar la copia cifrada y registrar el error. Nunca borrar el
   bucket ni el state remoto para "empezar de nuevo".

Cada state tiene su propio objeto `*.tflock`; el bucket es compartido, las claves no. No hay tabla DynamoDB de
locking (mecanismo deprecado y no presupuestado).

## 4. Recuperación si se pierde el state del bootstrap

El bootstrap gestiona pocos recursos y todos llevan tag `Spec = "I-070-terraform"`. La recuperación es por
**importación**, nunca por recreación.

1. Inventariar por lectura autorizada: bucket, versionado, SSE, ownership, Block Public Access, política y
   lifecycle. Compararlos con el código.
2. Escribir bloques `import` para cada recurso (`aws_s3_bucket`, `aws_s3_bucket_versioning`,
   `aws_s3_bucket_server_side_encryption_configuration`, `aws_s3_bucket_public_access_block`,
   `aws_s3_bucket_ownership_controls`, `aws_s3_bucket_policy`, `aws_s3_bucket_lifecycle_configuration`) con los
   identificadores reales aportados por el operador.
3. Ejecutar `plan` (autorizado) y confirmar **cero cambios** antes de aceptar el import; si el plan intenta
   reemplazar el bucket, detenerse: la importación está mal formada.
4. `apply` del import sólo con autorización humana; jamás `-auto-approve`.
5. Registrar lineage/serial nuevos y guardar copia cifrada del state resultante.

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

## 10. Evidencia a registrar

Sólo identificadores enmascarados, fechas, comandos ejecutados, hashes de state (no contenido) y resultados.
