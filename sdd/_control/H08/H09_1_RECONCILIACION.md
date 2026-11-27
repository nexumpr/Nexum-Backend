# H09-1 — Reconciliación DevSecOps, gobierno y FinOps (2026-11-27)

**Estado:** `IMPLEMENTADO_LOCAL` / `CI_PENDIENTE` / `MERGE_NO_AUTORIZADO` / `AWS_NOT_RUN`
**Base:** `origin/main` `1d5778dec3f5276bcc8f7e6a5979044a83418682` · **Release:** `v0.21.4+5bef784b2d1c`
**Alcance:** corrección XFF de E001, suites Python omitidas en CI y documentación comprobable. No cambia contratos Android, endpoints, OpenAPI, Terraform ni AWS.

## 1. Seguridad de aplicación — E001 y política de proxies

**Defecto corregido:** `register_handler.go` tomaba el **primer** elemento de `X-Forwarded-For`, controlable por el cliente. Un atacante podía rotar la cabecera y evadir la cuota de registro de 5/15 min.

**Política vigente (`internal/transport/http/clientip.go`):**

1. El peer inmediato (`RemoteAddr`) debe ser loopback o privado (ALB en la VPC). Un peer público se rechaza.
2. Sin `X-Forwarded-For`, solo se acepta un peer loopback o privado.
3. La cadena debe ser parseable al 100 %: máximo 16 saltos, sin elementos vacíos, zonas ni direcciones inválidas.
4. Con peer privado se exigen **al menos dos** elementos: el último lo añade el ALB y debe ser público (borde de CloudFront); el penúltimo es la IP del visor añadida por CloudFront.
5. Con peer loopback se acepta el último elemento (pruebas o proxy local).
6. La IP elegida debe ser unicast, no especificada y no multicast.

**Montaje:** E001, E002, E003, E005, E006, E011 y los catálogos públicos usan la misma función. E001 responde **400 `INVALID_REQUEST`** (`untrusted_proxy_chain`) cuando la cadena no es verificable, antes de invocar el limitador; Redis sigue siendo la autoridad compartida de cuotas y un fallo de Redis mantiene el 503 fail-closed en autenticación.

**Lo que esta corrección no hace:** no valida criptográficamente el origen. Una cabecera secreta de CloudFront requeriría Terraform y un Gate de despliegue; queda como mejora futura, no como control actual.

**Pruebas añadidas:** cadena con prefijo falsificado, elemento vacío, cadena malformada, más de 16 saltos, IPv4/IPv6, IPv4 mapeada, último salto privado, peer público, peer privado sin cadena, loopback, concurrencia (128 goroutines) y rechazo de E001 sin invocar el limitador.

**Limitadores locales:** `middleware.NewRateLimiter` y `cache.NewRateLimiter` solo se usan en pruebas; producción usa `cacheinfra.NewAuthRateLimiter` sobre Redis. No se eliminan porque son utilidades de prueba.

**Fail-open de lecturas públicas:** `LimitarPorIP` sigue fail-open si Redis falla o la IP no es verificable. Es semántica contractual de disponibilidad para catálogos; no se cambia sin RFC.

## 2. CI — suites Python incorporadas

| Suite | Pruebas | Resultado local |
|---|---:|---|
| `tests/infra/cd` | 32 | **OK** |
| `tests/infra/certificacion` | 26 | **OK** |
| `tests/infra/devsecops` | 18 | **OK** |
| `tests/infra/systemd` | 11 | **OK** |
| **Total** | **87** | **87 OK** |

Ejecución real con `py -3 -m unittest discover -s tests/infra/<suite>`; ninguna prueba realiza llamadas mutantes ni contra producción. La suite de certificación usa un servidor local en loopback con transporte simulado. Las cuatro se añaden al job `iac` del workflow; no sustituyen los 13 checks ni la cobertura mínima de dominio del 80 %.

**DAST (no ejecutado):** la estrategia aislada consiste en levantar el binario en el runner de CI sobre `127.0.0.1`, con datos sintéticos y sin credenciales, y ejecutar el arnés existente (`herramientas/preparar_dast.py`) o un baseline ZAP contra ese loopback. Requiere un Gate `DAST` específico; no se lanza contra CloudFront ni contra producción.

## 3. Gobierno de GitHub — verificado y propuestas

**Verificado por API el 2026-11-27:** `main` con `strict=true`, 13 checks, 1 revisor, force-push y borrado deshabilitados; `enforce_admins=false`; sin rulesets; sin environments; repositorio público; sin tags ni GitHub Releases; 170 ramas remotas.

Propuestas (no aplicadas; requieren autorización humana):

1. **Ruleset de protección**: bypass vacío o equivalente a `enforce_admins=true`; resolución de conversaciones obligatoria. No cambia checks ni revisores.
2. **Tags y Releases**: tag anotado `v0.21.4+5bef784b2d1c` sobre `5bef784b2d1c`, Release con notas, SHA-256 del binario y enlace al run de CI `45872103515`. Un tag debe apuntar al SHA productivo exacto, no al merge posterior.
3. **Trazabilidad del CD asistido**: environment `aws-deploy` con revisores, registro por despliegue de plan SHA-256, `instance refresh` ID, resultado y verificación posterior. OIDC sigue bloqueado (0 proveedores); no se simula CD.
4. **Visibilidad**: el repositorio es público por RFC-009. Riesgos: arquitectura y configuración visibles, forks que ejecutan CI sin secretos, correos de autor en el historial. No se encontraron credenciales reales. Se recomienda mantenerlo público salvo decisión expresa; no se cambia visibilidad.
5. **Ramas**: se conservan las 170 ramas históricas. Se propone archivar/etiquetar, nunca borrar sin Gate.

## 4. FinOps — evidencia read-only

| Métrica | Valor |
|---|---|
| Plan | FREE / ACTIVE |
| Créditos restantes | **124.111183 USD** |
| Vencimiento | 2027-05-04 |
| Presupuesto | 1 USD; gasto real 0.00 (sin cargo personal) |
| Consumo derivado | ≈3.6–5.7 USD/día desde 2026-11-25 (no Cost Explorer) |
| Proyección al 2026-12-18 | 21 días: ≈76–120 USD; reserva de 25 USD solo si el quemado ≤ **4.31 USD/día** |
| Recompensas Explore | 60/100 obtenidas; hasta 40 USD adicionales (DynamoDB 20, Bedrock 20) **no acreditados** |

**Decisión FinOps:** a 23 días, el presupuesto solo mantiene la reserva de 25 USD con un quemado ≤4.31 USD/día. El límite superior estimado (5.7) la incumpliría. Se requiere medición diaria de créditos durante H09-2 y un Gate si el promedio supera 4.31; no se reduce infraestructura productiva de forma unilateral. Los 40 USD de Explore no se contabilizan hasta verificar su acreditación; DynamoDB/Bedrock en consola no deben tocar la cuenta de Nexum.

## 5. Reconciliaciones documentales

- **E015:** el hash `1CEC…` corresponde a un texto de 13 662 bytes declarado en `dist/deuda/`, que **no existe**; el único preservado es `E925…` (8 123 bytes), superado y no publicable. No se aprueba ni publica v3.
- **E072/E073:** contrato `Health` conforme; GET real 200 el 2026-11-27 con `status=ok`, `service` y `timestamp`. Se cierra el hallazgo `CONTRACT_BLOCKED`.
- **E031:** se conserva el tombstone aprobado (204 idempotente, puntero conservado, exclusión en agregados).
- **E051:** se conserva sin caché de respuesta (lectura consistente; RFC-022 D-W3-07).
- **Certificación:** 74/75 PASS vigentes; E015 `CONTENT_MISMATCH`; las secciones v0.21.2/v0.21.1 son históricas.

## 6. Rúbrica DevSecOps (sin cambios por documentación)

| Dimensión | Nota |
|---|---:|
| SAST/SCA | 9 |
| Gestión de secretos | 7 |
| Seguridad IaC/cloud | 8 |
| Autenticación/sesión | 9 |
| Validación API | 9 |
| Protección de datos | 7 |
| Red/TLS | 4 |
| Pruebas/verificación | 7 |
| CI/CD/gobierno | 5 |
| Observabilidad/respuesta | 4 |
| **Total** | **69/100** |

H09-1 deja la corrección XFF y las 87 pruebas **listas para CI**, pero no se acreditan puntos productivos hasta merge y regresión autorizada. El salto a 80+ exige H09-2 (observabilidad) y H09-3 (DAST/datos); 85 exige además H09-4 (TLS/WAF/CD).
