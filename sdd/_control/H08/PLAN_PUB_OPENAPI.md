# Plan de publicación OpenAPI — I-150 (INF-150-02)

| Campo | Valor |
|---|---|
| Spec | `I-150-publicacion-y-compatibilidad-api` |
| Versión | 0.1.0 |
| Estado | **IMPLEMENTADO_LOCAL — NO DESPLEGADO** |
| Publicación real | `NOT_RUN` |

## 1. URL pública

- Contrato: `https://api.nexum.example/api/v1/` (documental; el host real se fija por IaC al desplegar).
- El contrato **no** afirma una URL AWS: `servers[0].url` es `https://api.example.invalid` con nota de
  que la URL real se configura al desplegar.

## 2. Artefacto

| Elemento | Valor |
|---|---|
| Ruta | `sdd/03-endpoints/openapi/nexum-api-v1.json` |
| Versión | OpenAPI `3.1.0` |
| SHA-256 esperado | `0a1e41b73230c0566ec6c6e8af871731937de6ccc5637ad39dbc552d65542308` |
| Verificación | `verificar_publicacion_i150.py publicacion` recalcula el checksum |

Publicar junto a `PLAN_EVIDENCIA_HTTP.md` y la matriz de compatibilidad; el checksum del artefacto
publicado debe coincidir con el del repositorio.

## 3. CORS

- Allowlist explícita de orígenes; **prohibido** `*`, y con credenciales se exigen orígenes exactos.
- Métodos permitidos: `GET, POST, PUT, PATCH, DELETE`; cabeceras `Authorization`, `Idempotency-Key`, `If-Match`.

## 4. Ejemplos

Sin credenciales, sin tokens, sin correos reales y sin datos financieros personales.

## 5. Qué falta para publicar de verdad

1. Despliegue autorizado (I-090) y URL HTTPS real.
2. Verificación HTTP desde cliente independiente (ver `PLAN_EVIDENCIA_HTTP.md`).
3. Confirmación de dos nodos en `running` y balanceo real.
