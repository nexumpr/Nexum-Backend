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

## Regla de mantenimiento

- Nunca usar `arn:aws:s3:::bucket/*` ni el ARN del bucket: el módulo IAM exige
  objeto exacto y la validación de la variable rechaza `*` y rutas terminadas en `/`.
- Al publicar un release nuevo se **añaden** sus dos objetos; no se retiran los del
  release anterior mientras siga siendo el destino de rollback.
- La comprobación automatizada está en
  `tests/infra/terraform/test_iam_artefactos_release.py`.
