# Gate H08-I — ARTIFACT_DELIVERY (I-020 / I-060 / I-090)

| Campo | Valor |
|---|---|
| Documento | `sdd/_control/H08/GATE_H08I_ARTIFACT_DELIVERY.md` |
| Fecha | 2026-11-09 |
| Base | `main` `17b092eae3dfb23988a191cd6375adb0e0723699` |
| Rama | `feat/h08i-artifact-delivery` |
| Estado | **APRUEBO_GATE_I020_I090_ARTIFACT_DELIVERY_LOCAL REGISTRADO — IMPLEMENTADO_LOCAL** |
| Autorización | `APRUEBO_GATE_I020_I090_ARTIFACT_DELIVERY_LOCAL` (mensaje humano independiente) |

## 1. Alcance autorizado

Implementar **localmente** el circuito de entrega de artefactos: bucket S3 privado y
versionado con SSE-S3, política que impide que terceros toquen los objetos, permiso de
escritura exclusivo para el rol publicador de CI en el prefijo `nexum/releases/`, permiso
de descarga para el rol runtime vía policy de identidad, y preconditions que fallan
cerrado si falta cualquiera de las tres identidades o si coinciden.

## 2. No autoriza

Crear el bucket ni recurso AWS alguno · publicar binarios o SBOM · `terraform plan` con
backend remoto · `apply`, bootstrap ni despliegue · crear roles IAM · fijar ARNs reales ·
iniciar H08-E.

## 3. Entregables

| Elemento | Ruta |
|---|---|
| Módulo del bucket | `infra/terraform/modules/artefactos/` |
| Cableado del entorno | `infra/terraform/envs/h08i/main.tf` y `variables.tf` |
| Permiso de descarga del runtime | `artifact_object_arns` del módulo `iam` |
| Contrato y pruebas | `tests/infra/terraform/test_artefactos_contrato.py` |

## 4. Decisión de diseño: ciclo roto con nombre determinista

El módulo necesita el ARN del rol runtime y el rol runtime necesita el ARN del prefijo:
`artefactos -> iam -> artefactos` sería un ciclo. Se rompe replicando en el entorno el
**nombre determinista** del bucket (`nexum-<entorno>-artefactos-<account>`), que el módulo
calcula con la misma fórmula. Una prueba de contrato compara ambas expresiones para
detectar cualquier deriva.

## 5. Límites declarados

- Sin ARNs reales: `artefactos_publicador_arn` y `gestor_iac_arn` van vacíos por defecto y
  el **plan** falla cerrado mientras no se fijen en el Gate de despliegue.
- La escritura exige SSE-AES256; sin ese encabezado el bucket deniega la publicación.
- `ListBucket` queda denegado salvo para el rol gestor de IaC (HeadBucket en el refresh).
- El bucket no contiene secretos: el bundle vive en `modules/secretos` (RFC-004).

## 6. Verificación

`terraform fmt -check` y `validate` en `envs/h08i` · contrato en
`tests/infra/terraform` · regresión Terraform completa · SDD y manifiesto · CI del PR.
`AWS_NOT_RUN`, `DEPLOY_NOT_RUN`.
