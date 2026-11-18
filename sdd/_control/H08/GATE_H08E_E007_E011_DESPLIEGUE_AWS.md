# Gate H08-E — Despliegue E007–E011 y hotfix E003

| Campo | Valor |
|---|---|
| Documento | `sdd/_control/H08/GATE_H08E_E007_E011_DESPLIEGUE_AWS.md` |
| Fecha | 2026-11-18 |
| Baseline funcional | **`v0.15.0+8f13c86d2ca0`** |
| `main` | `8f13c86d2ca04159e32b734a6f96aa47c82d79ec` |
| Cuenta / región | `713797602090` · `us-east-2` |
| Estado | `DEPLOYED` · `FUNCTIONAL_VERIFIED` |

## 1. Releases

| Release | Binario SHA-256 | Plan aplicado | LT |
|---|---|---|---|
| `v0.14.0+7869b1b96648` | `4baadf0b032259a2fbbd0f2ae8eebc3d73229c21146640960752fb3a7895e7bc` | `612E6C13…CB79` (0/4/0) | v9 |
| `v0.15.0+8f13c86d2ca0` | `0b66779e5394a454ba588963cfe634009bb04251a9ada773e1ba0d501a8dbf19` | `05C2116A…FC97` (0/4/0) | v10 |

Unidad systemd en ambos: `6fa4d5fb051914102b1bc3b6cd2814e881bf02bb26df1d938f4f0d46e15705cc`.

Publicación con rol `nexum-h08i-artefactos-publicador`, `If-None-Match: *`, SSE `AES256` y `--checksum-algorithm SHA256`; los checksums devueltos por S3 coinciden con los digests locales.

## 2. Gates de apply utilizados

```
APRUEBO_APPLY_LT_IAM_E007_E011_SHA256_612E6C139C0ED85F5929FB6B745C4363F98840BAB3BC5F4318A35714DA21CB79
APRUEBO_APPLY_LT_IAM_E003_SHA256_05C2116A4BEE24AC6DDA921FD6E26AE4EF576C68BDDC51934A363CC77485FC97
```

Ambos planes: **0 add · 4 change · 0 destroy · 0 replace**, limitados a Launch Template, política IAM de artefactos y los dos `terraform_data` de invariantes. Sin cambios en DynamoDB, Redis, SES, VPC Endpoint, CloudFront, VPC, subredes, security groups, ASG, instance profile ni secretos. Coste incremental 0.0000 USD.

## 3. Flota final

| Recurso | Valor |
|---|---|
| ASG `nexum-h08i-asg-backend` | **Min 2 / Desired 2 / Max 2** |
| Subredes | `us-east-2a` y `us-east-2b` restauradas |
| Instancias | `i-0665404e779ef7902` · `i-0a5dd963809ed3c2c` — **LT v10**, `InService`/`Healthy` |
| Target group `nexum-h08i-tg-backend` | **2 targets healthy** |
| `GET /health/live` | **200** |

Instance refresh del hotfix: `62ec1db0-3d44-400a-966e-45a950820fe9` → **Successful 100 %**.

## 4. Verificación funcional sobre v0.15.0

| Endpoint | HTTP real | Efecto comprobado |
|---|---|---|
| E001 | 201 | cuenta creada |
| E002 | 200 | sesión emitida |
| E003 | 200 | rotación; tokens nuevos distintos; access nuevo usable; replay del anterior **409**; refresh posterior **401** |
| E004 | 204 sin cuerpo | access posterior 401 · refresh posterior 401 |
| E005 | 202 | SES aceptó el envío (`MessageId` registrado, estado `ENVIADO`) |
| E006 | 200 | replay del código **409** |
| E007 | 200 | token previo **401** · login nueva 200 · login anterior **401** |
| E008 | 200 | cinco campos exactos · `current` correcto |
| E009 | 204 | `revoked=true` · token revocado 401 · inexistente 404 |
| E010 | 202 | envío real a identidad verificada en SES |
| E011 | 200 | `emailVerified=true` persistido · replay **409** · login refleja la verificación |

## 5. Incidencia resuelta — E003

`POST /auth/refresh` devolvía **503** con token válido y **401** con token inválido. Causa: `ExpressionAttributeValues[":zero"]` declarado sin uso en el `ConditionCheck` del perfil; DynamoDB responde `ValidationException` y el adaptador lo traducía a `ErrRefreshUnavailable`. Afectaba al 100 % de las rotaciones. Regresión introducida por A6/RFC-018 (E006). Corregido en PR #79 con prueba de regresión.

## 6. Incidencia operativa — capacidad EC2

`t4g.micro` sin capacidad en `us-east-2b`. El primer refresh quedó en 0 % durante 12 minutos reintentando, sin caída de servicio. Recuperación aplicada: cancelar, restringir el ASG al subnet de `us-east-2a`, nuevo refresh, restaurar subredes y 2/2/2. En el hotfix se aplicó la restricción de zona de forma preventiva y completó al primer intento.

## 7. Rollback

`v0.14.0+7869b1b96648` y todos los releases anteriores permanecen publicados, y sus ARN siguen en la política IAM del runtime (20 objetos). Revertir exige su propio plan y Gate.

## 8. Límites declarados

- La aceptación por parte del equipo Android **no** está confirmada; `ANDROID_READY` es técnico, no una aceptación.
- E012 y posteriores: `NOT_AUTHORIZED`.
- La política de permisos de Terraform aplicada localmente está pendiente de versionar mediante RFC/PR propio.
