# RFC-010 — Promoción de releases sin proveedor OIDC (temporal)

| Campo | Valor |
|---|---|
| Documento | `sdd/_control/H08/RFC-010-PROMOCION_SIN_OIDC.md` |
| Fecha | 2026-11-10 |
| Base | `main` `e579a2d2701b8b17c6781bc29a4d24151960ead6` |
| Estado | **PROPUESTO — requiere aprobación humana** |
| Bloqueo que resuelve | `iam:CreateOpenIDConnectProvider` = `explicitDeny`; `iam:ListOpenIDConnectProviders` = `AccessDenied` por SCP `p-ys0pa0u8` |

## 1. Hecho demostrado

La cuenta `713797602090` está bajo la organización `o-w0uoh8jcnh` (gestora `478815938944`).
La SCP `p-ys0pa0u8` deniega `iam:*Provider*`, compatible con el `iam:*Provider*` que
documenta el plan Free. **No se puede crear el proveedor OIDC de GitHub ni verificar si ya
existe**, porque también está denegado listarlo.

Consecuencia: `.github/workflows/cd-backend.yml` no puede autenticarse hoy. El diseño de
I-090 sigue siendo válido; lo que falla es el mecanismo de credenciales.

## 2. Procedimiento propuesto: promoción asistida

1. **CI (GitHub, sin credenciales AWS)**: construye `binario-<sha>` linux/arm64, su SHA-256,
   la SBOM y las pruebas. Sin secretos y sin AWS. Ya está implementado en `go-backend.yml`.
2. **Promoción (operador, sesión temporal)**: el responsable ejecuta el paso de publicación
   en su estación con una sesión de corta duración (`aws login`/SSO del perfil `nexum`),
   asumiendo el rol dedicado de publicación definido por I-060 — **no**
   `AccountFullAccessRole`.
3. **Publicación**: descarga el artefacto del run autorizado (`run_id` + SHA exacto), verifica
   el SHA-256, y lo sube a S3 con `--server-side-encryption AES256` y `--if-none-match "*"`.
   Un 412 es fallo cerrado: sin identidad verificadora no se declara idempotencia.
4. **Despliegue**: `instance_refresh` sobre el ASG con `MinHealthyPercentage=100`, abortando
   si no hay 2 objetivos sanos antes y después. Rollback a la LT previa.
5. **Verificación**: HTTP real contra CloudFront, readiness 200 con ambas dependencias,
   logs en CloudWatch.

## 3. Prohibiciones que se conservan

- Sin `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY` ni `AWS_SESSION_TOKEN` en GitHub, en el
  repositorio, en ficheros versionados ni en logs.
- Sin claves de acceso permanentes de usuario IAM.
- Sin despliegue automático desde PR o merge.
- El rol de publicación es de mínimo privilegio: `s3:PutObject` sobre el prefijo de releases,
  sin lectura, sin borrado y sin escritura fuera del prefijo (ya implementado en
  `modules/artefactos`).

## 4. Límite declarado

**Esto no es CD automático.** La promoción requiere intervención humana con sesión temporal.
Se documenta como `CD_ASISTIDO`, no como `CD_CONTINUO`. Cuando la SCP permita OIDC (o exista
un proveedor ya creado), se sustituye únicamente el paso de credenciales; el resto del
circuito no cambia.

## 5. Aprobación requerida

`APRUEBO_CD_ASISTIDO_SIN_OIDC` — autoriza documentar y operar la promoción asistida. No
autoriza AWS mutante, ni `apply`, ni despliegue por sí sola.
