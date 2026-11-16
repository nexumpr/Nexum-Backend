# Gate H08-E / E003 — Permisos temporales RFC-002 para publicar el release

**ESTADO:** `PROPUESTO — NO APLICADO`. **No se ha modificado `opencode.json`.**

| Campo | Valor |
|---|---|
| Frase solicitada | `APRUEBO_EXCEPCION_RFC002_PUBLICACION_S3_E003` |
| Fecha | 2026-11-13 |
| Alcance | asumir **un** rol y escribir **dos** objetos; nada más |
| Gate previo ya concedido | `APRUEBO_PUBLICACION_S3_E003_V0.10.0` (no se vuelve a pedir) |
| Duración | sólo hasta completar los dos `PutObject`; se retira inmediatamente después |

## 1. Bloqueo demostrado

`opencode.json` mantiene, en la lista de reglas `shell`:

- `aws *` → `deny` (línea 394)
- `aws * put-*` → `deny` (línea 666)

`aws sts assume-role` devolvió `permission.rejected` y `aws s3api put-object` está cubierto por
`aws * put-*`. No existe ninguna regla `ask` posterior que los exceptúe.

**Corrección de un informe previo:** `terraform plan*` ya está en `ask` (línea 614) y
`terraform init -backend=true*` también (línea 631), con su `tool.use: allow` en `policies`.
El plan del Launch Template **no** requiere excepción nueva.

## 2. Cambio exacto propuesto (lo aplica el responsable, no el agente)

Dos entradas al final de la lista **`permissions`** (después de todas las reglas `deny`, porque
la última coincidencia gana), con comodín final —la lección D-PERM-01 mostró que las cadenas
exactas no se aplicaban—:

```json
{ "action": "shell", "resource": "aws sts assume-role *", "effect": "ask" },
{ "action": "shell", "resource": "aws s3api put-object *", "effect": "ask" }
```

Y sus dos contrapartes al final de **`policies`**:

```json
{ "action": "tool.use", "resource": "shell:aws sts assume-role *", "effect": "allow" },
{ "action": "tool.use", "resource": "shell:aws s3api put-object *", "effect": "allow" }
```

**No se concede**: `aws s3api delete-object`, `aws s3 cp`, `aws s3 sync`, `aws iam *`,
`aws s3api put-bucket-policy`, `aws dynamodb *` de escritura, ni ningún `terraform apply`.
`aws * put-*` sigue denegado salvo la excepción literal de `s3api put-object`.

## 3. Uso previsto (exacto)

1. `aws sts assume-role --role-arn arn:aws:iam::713797602090:role/nexum-h08i-artefactos-publicador --role-session-name <ts> --profile nexum --region us-east-2`
   → credenciales **sólo en memoria del proceso**, sin escribir ficheros ni variables persistentes.
2. `aws s3api put-object` × 2 con `--if-none-match '*'`, `--server-side-encryption AES256`,
   `--checksum-algorithm SHA256`:
   - `releases/v0.10.0+1d0c2650daab/nexum-backend-linux-arm64` (19 127 165 B, `734de969…723c`)
   - `releases/v0.10.0+1d0c2650daab/nexum-backend.service` (1 357 B, `6fa4d5fb…05cc`)
3. Verificación con el operador (`s3api list-objects-v2`, ya permitido) y con el
   `ChecksumSHA256` que devuelve `PutObject`.
4. **Retirada inmediata** de las cuatro entradas, dejando `opencode.json` idéntico a `HEAD`
   (`git diff` vacío), igual que se hizo en el apply del bootstrap.

## 4. Riesgos y controles

| Riesgo | Control |
|---|---|
| Escritura fuera del prefijo | El bucket deniega al publicador fuera de `releases/` |
| Sobrescritura de un release existente | `If-None-Match: '*'`; si existe, falla y se detiene |
| Fuga de credenciales temporales | Nunca se imprimen, no se escriben en disco ni en Git; expiran en ≤1 h |
| Uso indebido de la excepción | Sólo dos patrones, sólo `put-object` y `assume-role`; retirada verificada con `git diff` |
| Escalada a `aws *` | `aws *` sigue en `deny`; la excepción es literal, no un comodín general |

## 5. Criterio de cierre

Los dos objetos existen con tamaño y `ChecksumSHA256` correctos, `memoria.md` lo registra, y
`opencode.json` vuelve a ser idéntico a `HEAD`. Sólo entonces se abre el Gate de apply del
plan de Launch Template (que ya puede generarse con los permisos vigentes).
