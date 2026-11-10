# Gate H08-I — Readiness real DynamoDB/Redis (I-040 / I-050 / A-090)

| Campo | Valor |
|---|---|
| Documento | `sdd/_control/H08/GATE_H08I_READINESS.md` |
| Fecha | 2026-11-09 |
| Base | `main` `0e3610467dbde2ca104e6114308517db3f94503e` |
| Rama | `feat/h08i-readiness-real` |
| Estado | **APRUEBO_GATE_I040_I050_READINESS_LOCAL REGISTRADO — IMPLEMENTADO_LOCAL** |
| Autorizaciones | `APRUEBO_GATE_I040_I050_READINESS_LOCAL` + `APRUEBO_DEPENDENCIAS_CLIENTES_AWS_REDIS` |

## 1. Decisión de dependencias

`A-010` mantuvo el módulo con dependencias mínimas, por lo que no existía ningún cliente
real de DynamoDB ni de Redis. El Gate de dependencias autoriza añadirlos. Versiones fijadas:

| Módulo | Versión |
|---|---|
| `github.com/aws/aws-sdk-go-v2` | `v1.47.8` |
| `github.com/aws/aws-sdk-go-v2/config` | `v1.33.17` |
| `github.com/aws/aws-sdk-go-v2/service/dynamodb` | `v1.73.2` |
| `github.com/redis/go-redis/v9` | `v9.23.0` |

## 2. Comportamiento implementado

- `internal/infrastructure/health`: comprobaciones reales. DynamoDB con `DescribeTable`
  (plano de control, bajo coste, sin leer datos de usuario); Redis con `PING` sobre el
  cliente real, TLS ≥1.2 y AUTH cuando están configurados.
- `cmd/server`: si `DYNAMODB_TABLE` o `REDIS_ADDR` están presentes, construye el cliente
  y registra la dependencia en `HealthRegistry`. Si no están, **no se registra nada** y
  `/api/v1/health/ready` responde **503** (sin evidencia no se declara disponibilidad).
- Fallo de construcción del cliente: se registra el error y la dependencia queda ausente
  → readiness 503.
- Timeouts de 2 s en el registro; `Ping` con `DialTimeout`/`ReadTimeout`/`WriteTimeout`
  de 2 s y un reintento.
- Liveness (`/api/v1/health/live`) sigue siendo independiente de las dependencias.

## 3. No autoriza

AWS, Terraform remoto, `plan`/`apply`, despliegue, publicación de artefactos ni
E072/E073. La semántica contractual de los endpoints de salud pertenece a H08-E y **no**
se modifica aquí.

## 4. Verificación

`go build`, `go vet`, `gofmt` y pruebas unitarias con dobles (servidor RESP mínimo y
`DescribeTable` contra `httptest`), incluidas respuestas inválidas, error del servicio,
configuración vacía y ausencia de cliente. **`go test` local está bloqueado por el Control
de aplicaciones de Windows**; la ejecución efectiva ocurre en CI.

`AWS_NOT_RUN`, `DEPLOY_NOT_RUN`. La verificación de dependencias reales en AWS queda
`NOT_RUN_AWS`.
