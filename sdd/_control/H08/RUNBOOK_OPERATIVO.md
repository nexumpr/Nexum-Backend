# Runbook operativo — I-140

| Campo | Valor |
|---|---|
| Spec | `I-140-operacion` |
| Versión | 0.1.0 |
| Estado | **IMPLEMENTADO_LOCAL — NO DESPLEGADO** |
| Ejecución | `NOT_RUN` (ningún procedimiento se ejecutó contra AWS) |

> Cada sección declara responsable, precondiciones, diagnóstico, mitigación, rollback, evidencia,
> criterio de éxito y estado. Todos los procedimientos están `DOCUMENTADO_NO_EJECUTADO`: un runbook
> escrito no acredita que se haya usado.

## deployment

- **responsable**: Responsable de release (con aprobación humana por SHA)
- **precondiciones**: CI verde, artefacto con SHA-256 verificado, entorno protegido, 2 targets previstos
- **diagnostico**: fallo en descarga del artefacto, checksum distinto o unidad systemd inactiva
- **mitigacion**: abortar el refresh, conservar la versión anterior del Launch Template
- **rollback**: restaurar la LT previa y el ASG a `min=desired=max=2`
- **evidencia**: run de CI, digest del artefacto, estado del ASG y health check
- **criterio-exito**: 2 targets sanos en `/api/v1/health/live` y ningún 5xx nuevo
- **estado**: DOCUMENTADO_NO_EJECUTADO

## health-fail

- **responsable**: Guardia de servicio
- **precondiciones**: alarmas activas y acceso a logs del entorno
- **diagnostico**: `HealthyHostCount < 2`, readiness 503 o timeouts de dependencias
- **mitigacion**: identificar el nodo degradado y forzar su reemplazo dentro del ASG
- **rollback**: volver a la LT previa si el fallo aparece tras un despliegue
- **evidencia**: métricas del ALB, logs estructurados y trazas W3C de la ventana
- **criterio-exito**: 2 targets sanos sostenidos durante 10 minutos
- **estado**: DOCUMENTADO_NO_EJECUTADO

## dynamo-throttle

- **responsable**: Backend + FinOps (consumo)
- **precondiciones**: modo on-demand de I-040 y métricas de throttling
- **diagnostico**: `ProvisionedThroughputExceeded` o latencia alta con reintentos agotados
- **mitigacion**: aplicar backoff exponencial con jitter y revisar el patrón de acceso sin `Scan`
- **rollback**: desactivar la funcionalidad que genera la carga, nunca destruir la tabla
- **evidencia**: conteo de throttles, latencias p95 y consumo estimado
- **criterio-exito**: throttles a cero y latencia dentro del objetivo durante 15 minutos
- **estado**: DOCUMENTADO_NO_EJECUTADO

## redis-unavailable

- **responsable**: Backend
- **precondiciones**: Redis OSS de I-050 y política de degradación definida
- **diagnostico**: timeouts de conexión o TLS, o `maxmemory` alcanzado
- **mitigacion**: degradar a lectura sin caché con fail-open en lecturas y fail-closed en autenticación
- **rollback**: restablecer el endpoint anterior sin regenerar secretos
- **evidencia**: tasa de aciertos, errores de conexión y estado del replication group
- **criterio-exito**: servicio operativo sin caché y sin errores 5xx sostenidos
- **estado**: DOCUMENTADO_NO_EJECUTADO

## certificados

- **responsable**: Responsable de infraestructura
- **precondiciones**: D2 vigente (CloudFront HTTPS, ALB HTTP interno) y `H-01` declarado
- **diagnostico**: error de TLS en el borde o dominio de CloudFront no resuelto
- **mitigacion**: verificar la distribución y el origen; no recrear el ALB ni cambiar DNS sin Gate
- **rollback**: mantener la distribución anterior mientras se corrige
- **evidencia**: `curl -I` contra el dominio CloudFront y estado de la distribución
- **criterio-exito**: HTTPS responde 200 en la ruta de salud pública
- **estado**: DOCUMENTADO_NO_EJECUTADO

## rotacion-secretos

- **responsable**: Seguridad
- **precondiciones**: `RFC-004` aprobado (hoy PROPUESTO) y ventana de mantenimiento
- **diagnostico**: secreto comprometido o próximo a caducar
- **mitigacion**: rotar según RFC-004; nunca congelar el secreto en el state ni en logs
- **rollback**: conservar la versión anterior hasta validar el arranque
- **evidencia**: identificador de versión (sin valor), arranque correcto y ausencia del secreto en logs
- **criterio-exito**: servicio arranca con la versión nueva y la anterior queda revocada
- **estado**: DOCUMENTADO_NO_EJECUTADO

## auth-inesperado

- **responsable**: Seguridad
- **precondiciones**: alarmas de 401/403 y rate limiting de autenticación
- **diagnostico**: pico de 401/403, intentos de enumeración o replay de refresh token
- **mitigacion**: bloquear la fuente, invalidar la familia de tokens afectada y revisar auditoría
- **rollback**: ninguna acción que reactiva sesiones revocadas
- **evidencia**: conteo por código, IPs de origen y trazas sin datos personales
- **criterio-exito**: 401 vuelve a la línea base y no aparecen sesiones reactivadas
- **estado**: DOCUMENTADO_NO_EJECUTADO

## presupuesto

- **responsable**: FinOps
- **precondiciones**: presupuesto vigente de I-000 y control diario de I-120
- **diagnostico**: saldo en aviso, bloqueo o forecast superior al crédito restante
- **mitigacion**: bloquear cambios auxiliares y escalar; **nunca** apagar el core ni pasar a Paid
- **rollback**: reanudar auxiliares solo con créditos verificados y Gate
- **evidencia**: salida de `verificar_finops_diario.py` y umbral alcanzado
- **criterio-exito**: ninguna ventana nueva abierta con saldo bajo y core intacto
- **estado**: DOCUMENTADO_NO_EJECUTADO

## incidente

- **responsable**: Responsable de incidente (rol asignado por severidad)
- **precondiciones**: severidad clasificada según `MATRIZ_INCIDENTES.md`
- **diagnostico**: causa raíz preliminar con evidencia no sensible
- **mitigacion**: contención primero, corrección después; sin cambios destructivos
- **rollback**: volver al último estado conocido bueno (LT, versión o datos)
- **evidencia**: línea de tiempo, decisiones y estados antes/después
- **criterio-exito**: severidad cerrada con evidencia y acciones preventivas registradas
- **estado**: DOCUMENTADO_NO_EJECUTADO

## eliminacion-iac

- **responsable**: Responsable de infraestructura con Gate humano
- **precondiciones**: `prevent_destroy` evaluado, copia de state, autorización explícita
- **diagnostico**: necesidad de retirar recursos no esenciales o entorno de revisión
- **mitigacion**: separar `destroy-runtime` de `destroy-data` y `destroy-state` (I-120/I-130)
- **rollback**: restaurar desde la copia de state y recrear solo lo efímero
- **evidencia**: plan revisado por dirección y proveedor, sin recursos core afectados
- **criterio-exito**: cero cambios sobre core, datos y state; ningún `destroy-all`
- **estado**: DOCUMENTADO_NO_EJECUTADO
