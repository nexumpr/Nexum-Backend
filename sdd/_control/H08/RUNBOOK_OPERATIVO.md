# Runbook operativo — I-140

| Campo | Valor |
|---|---|
| Spec | `I-140-operacion` |
| Versión | 0.2.0 |
| Estado | **IMPLEMENTADO_LOCAL — NO DESPLEGADO** |
| Procedimientos | 12 |
| Campos por procedimiento | 13 |
| Ejecución | `NOT_RUN` (ninguno se ejecutó contra AWS) |

> Cada procedimiento declara disparador, clasificación, responsable, prerrequisitos, diagnóstico,
> impacto, mitigación, autorización, escalamiento, rollback, recuperación, evidencia y criterio de
> cierre. Todos quedan `DOCUMENTADO_NO_EJECUTADO`; ningún runbook acredita ejecución real.

## deploy-rollback

- **disparador**: Despliegue de release o rollback del Backend
- **clasificacion**: S2 (S1 si hay caida total)
- **responsable**: Infraestructura con aprobacion por SHA
- **prerrequisitos**: CI verde, artefacto con SHA-256, entorno protegido, LT previa disponible
- **diagnostico**: Artefacto distinto, checksum invalido o unidad systemd inactiva
- **impacto**: Servicio sin la version esperada o con 1 nodo sano
- **mitigacion**: Abortar el refresh y conservar la LT previa; nunca reducir el ASG por debajo de 2
- **autorizacion**: Gate de despliegue con entorno protegido y frase por SHA
- **escalamiento**: Responsable de release y guardia
- **rollback**: Restaurar la LT anterior y el ASG a min=desired=max=2
- **recuperacion**: 2 targets sanos y health check 200 durante 10 minutos
- **evidencia**: Run de CI, digest del artefacto, estado del ASG y logs sin datos sensibles
- **criterio-cierre**: 2 targets sanos y cero 5xx nuevos atribuibles al cambio
- **estado**: DOCUMENTADO_NO_EJECUTADO
- **gatillo**: CI rojo, artefacto sin checksum o release fallido

## health-fail

- **disparador**: Fallo de /health/live o /health/ready
- **clasificacion**: S2 (S1 si el servicio no responde)
- **responsable**: Guardia de servicio
- **prerrequisitos**: Alarmas activas y acceso a logs del entorno
- **diagnostico**: HealthyHostCount < 2, readiness 503 o timeouts de dependencias
- **impacto**: Trafiko parcial o total sin servicio
- **mitigacion**: Identificar el nodo degradado y forzar su reemplazo dentro del ASG
- **autorizacion**: Ninguna para diagnostico de solo lectura; Gate para reemplazar
- **escalamiento**: Responsable de guardia
- **rollback**: Volver a la LT previa si el fallo aparece tras un despliegue
- **recuperacion**: 2 targets sanos sostenidos y readiness correcto
- **evidencia**: Metricas del ALB, logs estructurados y trazas W3C de la ventana
- **criterio-cierre**: 2 targets sanos durante 10 minutos y sin 5xx nuevos
- **estado**: DOCUMENTADO_NO_EJECUTADO
- **gatillo**: Alarma de targets sanos o readiness 503

## instancia-perdida

- **disparador**: Perdida de una instancia o target
- **clasificacion**: S2
- **responsable**: Infraestructura
- **prerrequisitos**: ASG con desired=2 y LT vigente
- **diagnostico**: HealthyHostCount = 1 y ASG reponiendo capacidad
- **impacto**: Capacidad reducida sin cumplir GOV-02
- **mitigacion**: Dejar que el ASG reponga; no fijar desired=1 en ningun caso
- **autorizacion**: Gate si el reemplazo exige cambiar la LT
- **escalamiento**: Responsable de guardia
- **rollback**: Sin rollback si el reemplazo es automatico; revertir la LT si el fallo se repite
- **recuperacion**: Dos instancias sanas en zonas distintas
- **evidencia**: Actividad del ASG, IDs de instancia y estado del target group
- **criterio-cierre**: 2 targets sanos simultaneos y 2 instancias en running
- **estado**: DOCUMENTADO_NO_EJECUTADO
- **gatillo**: Una instancia termina o sale del target group

## dynamo-throttle

- **disparador**: Throttling o timeouts de DynamoDB
- **clasificacion**: S2
- **responsable**: Backend / Datos
- **prerrequisitos**: Tabla on-demand de I-040 y metricas de throttling
- **diagnostico**: Reintentos agotados, latencia p95 fuera de objetivo o errores 5xx de datos
- **impacto**: Operaciones financieras lentas o rechazadas
- **mitigacion**: Backoff exponencial con jitter y revision del patron de acceso sin Scan
- **autorizacion**: Ninguna para diagnostico; Gate para cambios de capacidad
- **escalamiento**: Backend y FinOps
- **rollback**: Desactivar la funcionalidad que genera la carga; nunca destruir la tabla
- **recuperacion**: Throttles a cero y latencia dentro del objetivo
- **evidencia**: Conteo de throttles, latencias y consumo estimado sin datos personales
- **criterio-cierre**: Throttles a cero durante 15 minutos
- **estado**: DOCUMENTADO_NO_EJECUTADO
- **gatillo**: ProvisionedThroughputExceeded o latencia alta con reintentos agotados

## redis-unavailable

- **disparador**: Fallo de Redis o cache no disponible
- **clasificacion**: S2
- **responsable**: Backend
- **prerrequisitos**: Redis OSS de I-050 y politica de degradacion definida
- **diagnostico**: Errores de conexion, aciertos de cache a cero o memory pressure
- **impacto**: Latencia alta; autenticacion fail-closed
- **mitigacion**: Degradar a lectura sin cache: fail-open en lecturas y fail-closed en autenticacion
- **autorizacion**: Gate para reemplazar el nodo de cache
- **escalamiento**: Responsable de guardia
- **rollback**: Restablecer el endpoint anterior sin regenerar secretos
- **recuperacion**: Servicio operativo sin cache y sin 5xx sostenidos
- **evidencia**: Tasa de aciertos, errores de conexion y estado del replication group
- **criterio-cierre**: Operacion sin cache y credenciales que no se validan sin Redis
- **estado**: DOCUMENTADO_NO_EJECUTADO
- **gatillo**: Timeouts de conexion, TLS o maxmemory alcanzado

## https-certificados-cloudfront

- **disparador**: Problemas de HTTPS, certificados o CloudFront
- **clasificacion**: S1 si el acceso publico falla
- **responsable**: Infraestructura
- **prerrequisitos**: Decision D2 vigente y H-01 declarado
- **diagnostico**: Certificado por defecto, distribucion en error o DNS incorrecto
- **impacto**: API publica inaccesible
- **mitigacion**: Verificar distribucion y origen; no recrear el ALB ni cambiar DNS sin Gate
- **autorizacion**: Gate de infraestructura para cualquier cambio de borde
- **escalamiento**: Responsable de infraestructura
- **rollback**: Mantener la distribucion anterior mientras se corrige
- **recuperacion**: HTTPS responde 200 en la ruta de salud publica
- **evidencia**: curl -I contra el dominio y estado de la distribucion
- **criterio-cierre**: HTTPS 200 y ausencia de errores TLS en el periodo
- **estado**: DOCUMENTADO_NO_EJECUTADO
- **gatillo**: Error TLS en el borde o dominio CloudFront no resuelto

## rotacion-secretos-jwt

- **disparador**: Rotacion controlada de secretos y claves JWT
- **clasificacion**: S1 si hay compromiso
- **responsable**: Seguridad
- **prerrequisitos**: RFC-004 aprobado (hoy PROPUESTO) y ventana de mantenimiento
- **diagnostico**: Version de secreto antigua o claves JWT sin rotar
- **impacto**: Arranque fallido o sesiones invalidadas masivamente
- **mitigacion**: Rotar segun RFC-004; nunca congelar el secreto en state, logs ni repositorio
- **autorizacion**: Gate de seguridad y ventana autorizada
- **escalamiento**: Seguridad y responsable de guardia
- **rollback**: Conservar la version anterior hasta validar el arranque
- **recuperacion**: Servicio arranca con la version nueva y la anterior queda revocada
- **evidencia**: Identificador de version (sin valor), arranque correcto y ausencia del secreto en logs
- **criterio-cierre**: Arranque correcto con la version nueva y revocacion confirmada
- **estado**: DOCUMENTADO_NO_EJECUTADO
- **gatillo**: Secreto comprometido, proximo a caducar o rotacion programada

## auth-inesperado

- **disparador**: Errores 401/403 inesperados o intento de abuso
- **clasificacion**: S1 si hay indicio de intrusion
- **responsable**: Seguridad
- **prerrequisitos**: Alarmas de autenticacion y rate limiting de A-080
- **diagnostico**: Aumento de 401/403 por IP, replay detectado o familia de tokens comprometida
- **impacto**: Acceso indebido o bloqueo de usuarios legitimos
- **mitigacion**: Bloquear la fuente, invalidar la familia afectada y revisar auditoria
- **autorizacion**: Gate para bloqueos que afecten a usuarios reales
- **escalamiento**: Seguridad
- **rollback**: Ninguna accion que reactive sesiones revocadas
- **recuperacion**: 401 vuelve a la linea base y no aparecen sesiones reactivadas
- **evidencia**: Conteo por codigo, IPs de origen y trazas sin datos personales
- **criterio-cierre**: Tasa de 401 en linea base y cero sesiones revocadas activas
- **estado**: DOCUMENTADO_NO_EJECUTADO
- **gatillo**: Pico de 401/403, enumeracion o replay de refresh token

## incidente-seguridad-datos

- **disparador**: Incidente de seguridad o exposicion de datos
- **clasificacion**: S1
- **responsable**: Seguridad con responsable de incidente
- **prerrequisitos**: Clasificacion S1 y canal de escalamiento humano
- **diagnostico**: Indicios en logs, accesos anomalos o reporte externo
- **impacto**: Datos personales o financieros expuestos
- **mitigacion**: Contener el acceso, preservar evidencia y no borrar nada hasta autorizacion
- **autorizacion**: Gate humano inmediato; notificacion segun obligacion aplicable
- **escalamiento**: Responsable de incidente y Seguridad
- **rollback**: Ninguno que destruya evidencia; restaurar solo desde copia verificada
- **recuperacion**: Acceso contenido, causa identificada y evidencia preservada
- **evidencia**: Linea de tiempo, decisiones y estado de acceso
- **criterio-cierre**: Acceso contenido y causa raiz documentada sin datos sensibles
- **estado**: DOCUMENTADO_NO_EJECUTADO
- **gatillo**: Sospecha de acceso indebido o fuga de datos

## contencion-presupuestal

- **disparador**: Contencion de presupuesto y saldo
- **clasificacion**: S2 de FinOps
- **responsable**: FinOps
- **prerrequisitos**: Presupuesto vigente de I-000 y control diario de I-120
- **diagnostico**: Nivel BLOCKED/REVISION_URGENTE/INCIDENTE_CRITICO del control diario
- **impacto**: Riesgo de cierre de cuenta o gasto no cubierto
- **mitigacion**: Bloquear ventanas auxiliares y escalar; nunca apagar el core ni pasar a Paid
- **autorizacion**: Gate humano; ningun cambio de plan sin RFC
- **escalamiento**: FinOps y responsable
- **rollback**: Reanudar auxiliares solo con creditos verificados
- **recuperacion**: Core intacto y ninguna ventana nueva abierta
- **evidencia**: Salida de verificar_finops_diario.py y umbral alcanzado
- **criterio-cierre**: Sin ventanas nuevas con saldo bajo y core operativo
- **estado**: DOCUMENTADO_NO_EJECUTADO
- **gatillo**: Saldo en aviso, bloqueo o forecast superior al credito restante

## restauracion-continuidad

- **disparador**: Restauracion y continuidad del servicio
- **clasificacion**: S1
- **responsable**: Responsable de datos
- **prerrequisitos**: I-130 con PITR declarado, tabla aislada y Gate de despliegue
- **diagnostico**: Datos inconsistentes, borrado accidental o corrupcion
- **impacto**: Perdida de integridad financiera
- **mitigacion**: Restaurar a tabla nueva aislada y validar invariantes
- **autorizacion**: Gate de despliegue y autorizacion por PR/SHA
- **escalamiento**: Responsable de datos y Backend
- **rollback**: Descartar la tabla de verificacion; nunca restaurar sobre produccion
- **recuperacion**: Datos restaurados verificados y servicio reanudado
- **evidencia**: Informe de restore, duracion y checksums sin datos personales
- **criterio-cierre**: Invariantes financieras OK y servicio en ON_2_HEALTHY
- **estado**: DOCUMENTADO_NO_EJECUTADO
- **gatillo**: Perdida de datos o necesidad de restaurar

## eliminacion-iac

- **disparador**: Eliminacion segura de infraestructura
- **clasificacion**: S2
- **responsable**: Infraestructura con Gate humano
- **prerrequisitos**: prevent_destroy evaluado, copia de state y autorizacion explicita
- **diagnostico**: Necesidad de retirar recursos efimeros sin tocar el core
- **impacto**: Riesgo de destruir datos o state
- **mitigacion**: Separar destroy-runtime de destroy-data y destroy-state; nunca destroy-all
- **autorizacion**: Gate de infraestructura por entorno
- **escalamiento**: Responsable de infraestructura
- **rollback**: Restaurar desde la copia de state y recrear solo lo efimero
- **recuperacion**: Core, datos y state intactos
- **evidencia**: Plan revisado por direccion y proveedor, sin recursos core afectados
- **criterio-cierre**: Cero cambios sobre core, datos y state
- **estado**: DOCUMENTADO_NO_EJECUTADO
- **gatillo**: Retirada de recursos no esenciales o entorno de revision
