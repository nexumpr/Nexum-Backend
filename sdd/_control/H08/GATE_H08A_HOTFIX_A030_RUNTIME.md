# Gate H08-A — Hotfix A-030 (runtime HTTP)

| Campo | Valor |
|---|---|
| Documento | `sdd/_control/H08/GATE_H08A_HOTFIX_A030_RUNTIME.md` |
| Fecha | 2026-11-06 |
| Defecto | **D-01** — el ejecutable no inicia el servidor HTTP |
| Spec | A-030 (servidor HTTP); dependencia A-090 (probes) **fuera de alcance** |
| Base | `main` en `10f632799f00c998aaf648e1cb9d27c4b3e5bf52` (merge de PR #17), verificado por OpenCode antes de crear la rama |
| Rama | `fix/h08a-a030-runtime-http` |
| Estado | **APROBADO — autorización humana expresa** |

## 1. Autorización

Frase humana recibida (2026-11-06):

```
APRUEBO_GATE_HOTFIX_A030_RUNTIME
```

Alcance concedido literalmente:

- Registrar el Gate y su evidencia.
- Crear rama independiente desde `main` limpio y actualizado.
- Corregir el arranque HTTP en `cmd/server/main.go`.
- Señales, errores de arranque y graceful shutdown.
- Pruebas dentro de A-030.
- Pruebas Go, HTTP locales, integridad SDD, SHA-256 y OpenAPI.
- Actualizar `memoria.md`, tasks, checklist y evidencias **sólo** del hotfix.
- Commit, push de la rama y PR independiente.
- Revisar Actions y corregir fallos dentro del Gate.

**No autorizado:** merge, A-090, I-030, RFC-004, OpenAPI, AWS, Terraform mutante, despliegue,
cambio de plan. El merge requiere aprobación humana independiente.

## 2. Evidencia del defecto (auditoría previa, reproducida)

| Prueba | Resultado |
|---|---|
| `go build ./...` | PASS (compila) |
| Ejecución del binario | **termina en ~2 s** tras el log de inicialización |
| `Invoke-WebRequest http://127.0.0.1:8080/health` | sin listener |
| `main.go` | construye logger y `SystemContext`, registra y **sale**; no compone `Router`/`HTTPServer` |
| `HealthRegistry` | definido y nunca usado (A-090, fuera de alcance) |

Causa raíz: `main` no ejecuta `NewRouter()` ni `NewHTTPServer(...).Start()` y no bloquea a la espera de señal.

## 3. Alcance de la corrección

**Archivos:**

```
cmd/server/main.go                              (modificado)
tests/unit/transport/http_runtime_test.go       (nuevo)
cmd/server/main_test.go                         (nuevo, prueba del punto de entrada)
sdd/_control/H08/GATE_H08A_HOTFIX_A030_RUNTIME.md (este documento)
sdd/01-arquitectura-general/A-030-servidor-http/{tasks,checklist}.md (registro)
memoria.md
sdd/_control/H07/BASELINE_SHA256.json
```

**Comportamiento esperado:**

1. Listener real en el puerto de `DefaultServerConfig()` (8080).
2. Proceso permanece activo atendiendo peticiones.
3. `SIGINT`/`SIGTERM` → `Shutdown(ctx)` con timeout de 15 s.
4. Puerto ocupado o fallo de escucha → log de error y **exit 1**.
5. `http.ErrServerClosed` tratado como cierre esperado.

**Sin cambios:** `server.go`, `router.go`, middlewares, rutas `/health` y `/ready` (A-090 pendiente),
contratos OpenAPI, Terraform.

## 4. Pruebas de aceptación

| Prueba | Criterio |
|---|---|
| Arranque real | `HTTPServer` escucha en puerto efímero libre; el proceso responde |
| `GET /health` | 200 |
| `GET /api/v1/unknown` | 404 |
| Método no permitido en ruta registrada | 405 |
| Puerto ocupado | error explícito (`listen`), no éxito falso |
| Shutdown | `Shutdown` finaliza ≤ timeout y libera el puerto |
| Regresión Go | `gofmt`, `go build`, `go vet`, `go test ./... -count=1` |
| Integridad | `validar_sdd_integral --check-git` PASS; `validar_h06` 75 ops |
| Seguridad | diff sin secretos; `git diff --check` limpio |

## 4.b Correcciones de la revisión (2026-11-06, segunda pasada)

| Observación | Corrección |
|---|---|
| La prueba sólo cubría `NewHTTPServer`, no la composición de `main` | Se extrajo `run(ctx, cfg, logger) error` en `cmd/server`; `main` la invoca. `cmd/server/main_test.go` ejercita arranque, 404, apagado y error de puerto ocupado sobre ese mismo camino |
| El log afirmaba “inicializado con éxito” antes de escuchar | El mensaje pasa a “Contexto de sistema inicializado (A-000); se inicia el listener HTTP (A-030)”; no se declara disponibilidad hasta el apagado. Prueba que verifica la ausencia de la palabra “éxito” en el arranque |

## 5. Condiciones de cierre

- Commit único en la rama autorizada y PR independiente hacia `main`.
- CI documental en verde.
- **Merge no autorizado** por este Gate.

## 6. Riesgos y dependencias

| ID | Riesgo | Estado |
|---|---|---|
| A-090 | Rutas normativas `/api/v1/health/live|ready` y `HealthRegistry` no implementadas | **Pendiente, fuera de alcance**; el bootstrap EC2 seguirá fallando hasta su hotfix |
| RFC-004 | Secretos en reemplazo de instancias | Sin cambios |
| Puerto 8080 | Si está ocupado en local, la prueba usa puerto efímero | Cubierto |

## 7. Estado

`APROBADO` para implementación local; `merge` pendiente de autorización separada.
