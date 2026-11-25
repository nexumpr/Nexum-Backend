# Artefactos de release autorizados en IAM (rollback incluido)

`iam_artifact_object_arns` se inyecta desde el tfvars de custodia del operador; no
se versiona aquí. Este documento fija los ARN exactos que **deben** estar presentes
para que el rollback no quede denegado por IAM. No son secretos: son claves de
objetos dentro de un bucket privado.

## Release vigente y release de rollback

- `v0.19.2+3098f379d7ab` estuvo desplegado en LT v17 y debe seguir autorizado.
- `v0.20.0+4bf6758d372` es el release publicado con posterioridad; si es el activo,
  el par de v0.19.2 sigue siendo necesario para volver atrás.
- Cada release usa exactamente dos objetos: binario ARM64 y unidad systemd.

```hcl
iam_artifact_object_arns = [
  "arn:aws:s3:::nexum-h08i-artefactos-713797602090/releases/v0.19.2+3098f379d7ab/nexum-backend-linux-arm64",
  "arn:aws:s3:::nexum-h08i-artefactos-713797602090/releases/v0.19.2+3098f379d7ab/nexum-backend.service",
  "arn:aws:s3:::nexum-h08i-artefactos-713797602090/releases/v0.20.0+4bf6758d372/nexum-backend-linux-arm64",
  "arn:aws:s3:::nexum-h08i-artefactos-713797602090/releases/v0.20.0+4bf6758d372/nexum-backend.service",
]
```

## Cambio exacto que debe hacer el custodio en el tfvars privado

El tfvars de custodia **no se edita desde el repositorio** (no hay valores
versionados). Para habilitar un rollout nuevo:

1. Conservar los cuatro ARN anteriores; no sustituirlos.
2. Añadir los **dos** objetos del release nuevo, con el mismo patrón:
   `arn:aws:s3:::<bucket-artefactos>/releases/<release>/nexum-backend-linux-arm64`
   y `.../nexum-backend.service`. No usar `*`, no terminar en `/`, no usar el
   ARN del bucket.
3. Fijar `release_objetivo = "<release>"` (forma `vX.Y.Z+<sha12>`) en el tfvars
   privado. La precondition bloqueante de `checks.tf` exige al menos un ARN de
   ese release, y el contrato funcional exige los dos.
4. Si no se va a desplegar un release nuevo, dejar `release_objetivo = ""`; la
   precondition de rollback sigue exigiendo los cuatro ARN de v0.19.2/v0.20.0.

## Regla de mantenimiento

- Nunca usar `arn:aws:s3:::bucket/*` ni el ARN del bucket: el módulo IAM exige
  objeto exacto y la validación de la variable rechaza `*` y rutas terminadas en `/`.
- Al publicar un release nuevo se **añaden** sus dos objetos; no se retiran los del
  release anterior mientras siga siendo el destino de rollback.
- La comprobación automatizada está en
  `tests/infra/terraform/test_iam_outbox_schedule_artefactos.py`.
- La precondition bloqueante está en
  `terraform_data.invariantes_release_rollback` (`checks.tf`); el `check`
  `artefactos_release_rollback` solo advierte y no detiene el apply.
