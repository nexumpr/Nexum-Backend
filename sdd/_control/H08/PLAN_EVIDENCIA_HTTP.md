# Plan de evidencia HTTP — I-150 (INF-150-04)

| Campo | Valor |
|---|---|
| Spec | `I-150-publicacion-y-compatibilidad-api` |
| Versión | 0.1.0 |
| Estado | **IMPLEMENTADO_LOCAL — NO DESPLEGADO** |
| Ejecución | `NOT_RUN` |
| Cliente | Independiente del Backend (curl o cliente propio) |

> Ninguna prueba se ejecutó contra AWS. Una fila `PASS` exigiría ejecución autorizada y huella de la
> respuesta; el validador rechaza `PASS` sin AWS o sin evidencia.

| ID | Método | Ruta | Esperado | AWS | Estado |
|---|---|---|---|---|---|
| `health-live` | GET | `/api/v1/health/live` | 200 | Sí | **NOT_RUN** |
| `health-ready` | GET | `/api/v1/health/ready` | 200 o 503 segun dependencias | Sí | **NOT_RUN** |
| `registro` | POST | `/api/v1/auth/register` | 201 | Sí | **NOT_RUN** |
| `login` | POST | `/api/v1/auth/login` | 200 | Sí | **NOT_RUN** |
| `logout` | POST | `/api/v1/auth/logout` | 204 | Sí | **NOT_RUN** |
| `endpoint-adicional-1` | GET | `/api/v1/movimientos` | 200 paginado | Sí | **NOT_RUN** |
| `endpoint-adicional-2` | POST | `/api/v1/movimientos` | 201 idempotente | Sí | **NOT_RUN** |
| `endpoint-adicional-3` | GET | `/api/v1/metas` | 200 | Sí | **NOT_RUN** |
| `endpoint-adicional-4` | GET | `/api/v1/estadisticas/resumen` | 200 | Sí | **NOT_RUN** |
| `errores-401-403-404` | GET | `/api/v1/usuarios/perfil` | 401/403/404 con traceId | Sí | **NOT_RUN** |
| `tls-ca-valida` | GET | `https://api.nexum.example/api/v1/health/live` | CA publica valida | Sí | **NOT_RUN** |
| `persistencia-dynamodb` | POST | `/api/v1/movimientos` | lectura posterior identica | Sí | **NOT_RUN** |
| `dos-nodos` | GET | `/api/v1/health/live` | dos IDs distintos | Sí | **NOT_RUN** |

## Condiciones

- Evidencia con huella SHA-256 de la respuesta cruda y fecha.
- Sin datos personales ni credenciales en la evidencia.
- Dos nodos: IDs de instancia distintos y AZ distintas (`TWO_NODES_AWS_NOT_VERIFIED` hoy).
- Persistencia: crear un movimiento sintético y leerlo después.
- TLS: CA pública válida; `H-01` documenta el TLSv1 del certificado por defecto.

## Validación local

```powershell
py -3.13 herramientas/verificar_publicacion_i150.py evidencia --datos tests/fixtures/publicacion/evidencia.json
```