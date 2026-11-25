# Inventario de consumidores de health — verificación de compatibilidad

**Fecha:** 2026-11-25 · **Rama:** `fix/h08e-oleada5-regularizacion` (PR #145) · **Estado:** inventario verificado por búsqueda en el repositorio; sin cambios de ALB ni AWS.

## 1. Rutas y contrato

| Ruta | Contrato | Códigos | Cuerpo |
|---|---|---|---|
| `GET /api/v1/health/live` | `Health` | 200 | `{"status":"ok","service":"nexum-backend","timestamp":"<RFC3339>"}` |
| `GET /api/v1/health/ready` | `Health` / `ErrorEnvelope` | 200 / 503 | `ok|degraded` o `{"error":{"code":"DEPENDENCY_UNAVAILABLE",...,"traceId":...}}` |
| `GET /health` (legacy) | operativo, no contractual | 200 | delega en `HealthRegistry` |
| `GET /ready` (legacy) | operativo, no contractual | 200/503 | delega en `HealthRegistry` |

`dependencies` ya **no** se expone en el cuerpo público; el detalle por dependencia se registra solo en logs (nombre lógico, nunca el error del proveedor).

## 2. Consumidores reales en el repositorio

| Consumidor | Qué interpreta | ¿Depende de `status=UP` o `dependencies`? |
|---|---|---|
| Target group `nexum-h08i-tg-backend` (I-030) | `health_check_path = /api/v1/health/live`, `matcher = 200`, intervalo 30 s, umbrales 5/2, timeout 5 s | **No**: solo el código HTTP. |
| ASG (I-020) | `health_check_type = EC2` (por defecto) | No usa la ruta. |
| `CloudWatch` (I-110) | Patrones `{ $.status >= 500 }`, `{ $.status = 401 }`, `{ $.route = "/api/v1/health/ready" && $.status != 200 }` | **No**: usa `status` HTTP y `route`, no el cuerpo. |
| Bootstrap/systemd (I-020, RFC-004) | `curl` de liveness tras arrancar; falla cerrado si no responde | Solo el código. |
| Runbooks (I-140) | Comandos documentados de comprobación | Solo el código. |
| Android (handoff) | Debe basarse en el código HTTP y tolerar campos nuevos | **No** debe parsear `UP`. |
| Probes legacy `/health`, `/ready` | Sin consumidores detectados en `infra/`, `scripts/`, workflows, operación ni documentación operativa | No. |

## 3. Conclusión

- El cambio a `Health` contractual **no rompe** ningún consumidor propio: todos usan el código HTTP.
- **No** hace falta una ruta `/health/legacy` ni tocar el router: las rutas heredadas siguen existiendo y delegando en el registro.
- Liveness no consulta DynamoDB/Redis (no depende de AWS); readiness refleja si el nodo puede atender tráfico (503 en apagado, sin dependencias, con fallo o timeout).
- Sigue vigente: readiness **503 por diseño** hasta que el registro tenga dependencias reales registradas en el entorno (I-040/I-050).
