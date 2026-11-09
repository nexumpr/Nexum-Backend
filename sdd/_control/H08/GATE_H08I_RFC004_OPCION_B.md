# Gate H08-I — RFC-004 opción B (bundle de secretos en S3)

| Campo | Valor |
|---|---|
| Documento | `sdd/_control/H08/GATE_H08I_RFC004_OPCION_B.md` |
| Fecha | 2026-11-09 |
| Instrumento | `sdd/_control/H08/RFC-004-SECRETOS_BOOTSTRAP.md` |
| Specs afectadas | I-020 (bootstrap), I-060 (IAM), I-010 (bucket privado) |
| Estado | **APRUEBO_RFC004_OPCION_B REGISTRADO — IMPLEMENTADO_LOCAL (sin AWS)** |
| Base | `main` `2ebc9f2398ad344fa826ed0cf88fe07b4710fedc` |
| Autorización | `APRUEBO_RFC004_OPCION_B` (mensaje humano independiente, 2026-11-09) |

## 1. Alcance autorizado

Implementar **localmente** el mecanismo de la opción B: bucket S3 privado con Block Public
Access, ownership `BucketOwnerEnforced`, versionado, SSE-S3 y lifecycle de versiones no
actuales; permiso de instancia limitado a `s3:GetObject` sobre el prefijo
`nexum/secrets/`; y lectura del bundle por el bootstrap de I-020 con instalación atómica
de `/etc/nexum/backend.env` en `0640 root:nexum` y fallo cerrado ante cualquier anomalía.

## 2. No autoriza

- Crear el bucket ni ningún recurso AWS.
- Publicar el bundle ni generar secretos reales.
- `terraform plan/apply/destroy`.
- Desplegar, arrancar instancias ni publicar binarios.
- Cambiar D2, H-01, RFC-005, RFC-006 ni RFC-007.
- Sustituir Secrets Manager como fuente de verdad de los valores.

## 3. Entregables

| Elemento | Ruta |
|---|---|
| Módulo del bucket | `infra/terraform/modules/secretos/` |
| Permiso de lectura | `infra/terraform/modules/iam/main.tf` (`runtime_secretos`) |
| Variable IAM validada | `infra/terraform/modules/iam/variables.tf` |
| Lectura y validación del bundle | `infra/terraform/modules/computo/user-data.sh.tftpl` |
| Cableado del entorno | `infra/terraform/envs/h08i/main.tf` y `variables.tf` |
| Excepción acotada Trivy (AWS-0132) | `.trivyignore.yaml` |
| Contrato y pruebas | `tests/infra/terraform/test_secretos_contrato.py` |

## 4. Estructura del bundle (contrato)

```json
{"version": "v1", "variables": {"APP_ENV": "PRODUCTION", "...": "..."}}
```

El bootstrap rechaza: raíz que no sea objeto, ausencia de `version`, ausencia de
`variables`, clave que no cumpla `[A-Z][A-Z0-9_]{1,63}`, valor no textual o con saltos de
línea, y ausencia de `APP_ENV`. El objeto lo publica el operador fuera de banda.

## 5. Límites declarados

- **SSE-S3 sin CMK**: sin revocación criptográfica inmediata ni rotación automática. La
  rotación es operativa (nueva versión + `instance_refresh`). Aceptado por esta
  autorización y registrado como excepción acotada de Trivy `AWS-0132` sólo para
  `modules/secretos/main.tf`.
- **Auditoría**: el acceso queda cubierto por CloudTrail de eventos de datos sobre S3, que
  se configura en el Gate de despliegue; el logging de acceso del bucket no se habilita
  para no crear un segundo bucket con coste.
- **Borrado**: `force_destroy = false`; el teardown de auxiliares no destruye el bucket.
- **Coste**: sin endpoints nuevos ni NAT; almacenamiento y GET de céntimos. Se mide en el
  Gate de despliegue; no se declara «gratis».

## 6. Verificación exigida

1. `terraform fmt -check` y `terraform validate` en `envs/h08i`.
2. `python3 -m unittest discover -s tests/infra/terraform` (contrato del módulo, del
   permiso y del parser del bundle).
3. Resto de suites sin regresión y CI del PR en verde.
4. `AWS_NOT_RUN`, `DEPLOY_NOT_RUN`.

## 10. Excepción de `s3:ListBucket` para el rol gestor de IaC (2026-11-09)

- **Defecto**: `DenegarListBucket` denegaba `s3:ListBucket` a todos los principales. AWS exige
  `s3:ListBucket` para `HeadBucket`, y el recurso `aws_s3_bucket` del proveedor
  hashicorp/aws v5.72.0 llama a `HeadBucket` durante el refresh: el bucket policy se habría
  instalado y, acto seguido, el propio Terraform no podría refrescarlo ni planificar.
- **Corrección**: se separa en dos sentencias. `DenegarListadoDeVersionesYMultipart` mantiene el
  Deny global para `ListBucketVersions` y `ListBucketMultipartUploads`.
  `DenegarListBucket` mantiene el Deny para `s3:ListBucket` con condición
  `ArnNotEquals` sobre `aws:PrincipalArn` y valor único `var.gestor_iac_arn`.
- **La excepción es ausencia de Deny, no un Allow**: el rol gestor necesita además `s3:ListBucket`
  en su policy de identidad. No obtiene `GetObject`, `PutObject` ni `DeleteObject` por esta vía.
- **Alcance residual declarado**: `ListBucket` y `HeadBucket` comparten acción, por lo que el rol
  gestor puede enumerar nombres de claves del bucket (no contenidos). El bucket solo contiene el
  bundle y sus versiones.
- **Fail-closed**: `gestor_iac_arn` debe ser un ARN de **rol** IAM, distinto del runtime y del
  publicador; vacío o inválido hace fallar el plan. `terraform validate` local no se bloquea.
- **Dependencia abierta**: no se ha creado ni aprobado un rol gestor real; su ARN debe fijarse en
  el Gate de despliegue. No se deriva de `STS GetCallerIdentity` ni se habilita OIDC sin Gate.
