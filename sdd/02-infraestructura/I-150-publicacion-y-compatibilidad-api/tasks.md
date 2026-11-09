# I-150-publicacion-y-compatibilidad-api — Publicación y compatibilidad de contratos API

| Campo | Valor |
|---|---|
| Etapa | H05 — Infraestructura AWS |
| Versión | 0.1.0 |
| Estado | **DRAFT / REVIEW — NO APROBADO** |
| Trazabilidad | A-040, A-110, E001..E075, I-030, I-090 |
| Implementación | H08+, tras H07 aprobado y autorización humana por release |

## Gestión de tareas H05 (solo especificación y aceptación de diseño)

Estado inicial de todos los trabajos: `TODO`, salvo evidencia local de integridad H04 documentada por el responsable. No marcar como completado por generar archivos.

### T-150-01 — Cerrar contrato

- **Estado:** TODO / validación pendiente.
- **Responsable:** Arquitectura Backend + agente especializado de la materia, aprobación humana.
- **Entrada:** H04 aprobado y A-040, A-110, E001..E075, I-030, I-090.
- **Trabajo:** OpenAPI 3.1 estable como fuente de verdad; versionar path `/api/v1`, schemas, errores con traceId, scopes JWT, page cursor, idempotency, moneda decimal/string y enums; todo EP formalizado solo en H06.
- **Salida verificable:** decisión documental con evidencia y costo/seguridad aplicables.
- **Criterio DoD:** responsable revisa trazabilidad, no hay decisión supuesta ni requisito sustituido.

### T-150-02 — Cerrar descubrimiento

- **Estado:** TODO / validación pendiente.
- **Responsable:** Arquitectura Backend + agente especializado de la materia, aprobación humana.
- **Entrada:** H04 aprobado y A-040, A-110, E001..E075, I-030, I-090.
- **Trabajo:** Definir URL de backend pública HTTPS y ruta de especificación OpenAPI o artefacto publicado con checksum, con documentación de ejemplos sin credenciales. Política CORS permite orígenes aprobados sin abrir * con credenciales.
- **Salida verificable:** decisión documental con evidencia y costo/seguridad aplicables.
- **Criterio DoD:** responsable revisa trazabilidad, no hay decisión supuesta ni requisito sustituido.

### T-150-03 — Cerrar compatibilidad

- **Estado:** TODO / validación pendiente.
- **Responsable:** Arquitectura Backend + agente especializado de la materia, aprobación humana.
- **Entrada:** H04 aprobado y A-040, A-110, E001..E075, I-030, I-090.
- **Trabajo:** Matriz E001-E075: método, ruta, status, esquema, auth, trazabilidad UI, prueba negativa, persistencia; clasificación M/S/E/O no exime requisitos obligatorios adicionales del profesor (push, ML/IA si el backend interviene).
- **Salida verificable:** decisión documental con evidencia y costo/seguridad aplicables.
- **Criterio DoD:** responsable revisa trazabilidad, no hay decisión supuesta ni requisito sustituido.

### T-150-04 — Cerrar evidencia http

- **Estado:** TODO / validación pendiente.
- **Responsable:** Arquitectura Backend + agente especializado de la materia, aprobación humana.
- **Entrada:** H04 aprobado y A-040, A-110, E001..E075, I-030, I-090.
- **Trabajo:** Desde cliente independiente: curl/HTTP GET live/ready, registro/login/logout, cuatro endpoints adicionales de la rúbrica, 401/403/404, HTTPS CA, persistencia Dynamo y paso por dos nodos. Tests contrato ejecutados por Backend.
- **Salida verificable:** decisión documental con evidencia y costo/seguridad aplicables.
- **Criterio DoD:** responsable revisa trazabilidad, no hay decisión supuesta ni requisito sustituido.

### T-150-05 — Cerrar cambio

- **Estado:** TODO / validación pendiente.
- **Responsable:** Arquitectura Backend + agente especializado de la materia, aprobación humana.
- **Entrada:** H04 aprobado y A-040, A-110, E001..E075, I-030, I-090.
- **Trabajo:** Compatibilidad semver y deprecation de `/api/v1`, revisión de breaking changes en CI; ningún cambio sin actualizar schemas/tareas/historias.
- **Salida verificable:** decisión documental con evidencia y costo/seguridad aplicables.
- **Criterio DoD:** responsable revisa trazabilidad, no hay decisión supuesta ni requisito sustituido.

### T-150-06 — Cerrar alcance

- **Estado:** TODO / validación pendiente.
- **Responsable:** Arquitectura Backend + agente especializado de la materia, aprobación humana.
- **Entrada:** H04 aprobado y A-040, A-110, E001..E075, I-030, I-090.
- **Trabajo:** La aplicación Android, Play Store, pantallas, pruebas de integración Mobile y landing se entregan por otros equipos. No bloquear una fase de Backend hasta que móvil confirme; no atribuirles gates de nuestro API.
- **Salida verificable:** decisión documental con evidencia y costo/seguridad aplicables.
- **Criterio DoD:** responsable revisa trazabilidad, no hay decisión supuesta ni requisito sustituido.

### T-150-07 — Cerrar cdn/moderación

- **Estado:** TODO / validación pendiente.
- **Responsable:** Arquitectura Backend + agente especializado de la materia, aprobación humana.
- **Entrada:** H04 aprobado y A-040, A-110, E001..E075, I-030, I-090.
- **Trabajo:** Si algún EP recibe texto/imagen/video, diseñar moderación pre-storage mediante servicio cloud permitido y CDN de AWS según requisito, con costos y manejo de contenidos inapropiados; no crear EP artificiosos para archivos no presentes.
- **Salida verificable:** decisión documental con evidencia y costo/seguridad aplicables.
- **Criterio DoD:** responsable revisa trazabilidad, no hay decisión supuesta ni requisito sustituido.

### T-150-90 — Revisión de riesgos y dependencias

- **Estado:** TODO.
- **Trabajo:** resolver o escalar: Rutas opcionales asumidas obligatorias sin evidencia, Equipo Mobile convertido indebidamente en gate, Función de upload futura sin moderación/CDN.
- **Criterio DoD:** cada riesgo clasificado RESOLVED/BLOCKED/ACCEPTED con firma/fecha; no usar ACCEPTED para incumplir requisito docente sin aclaración.

### T-150-99 — Ratificación de la especificación

- **Estado:** REVIEW pendiente.
- **Entrada:** `spec.md`, `plan.md`, `tasks.md`, `checklist.md` actualizados.
- **Salida:** decisión explícita del responsable y vínculo a evidencia en informe I-GO.
- **Bloqueo:** no crear Terraform/endpoints ni ejecutar AWS.

## Implementacion H08-I (2026-11-09) — LOCAL, sin AWS

Gate `APRUEBO_GATE_I150_LOCAL`. Rama `feat/h08i-i150-publicacion`. Ninguna prueba HTTP se ejecuto.

- **T-150-01 (Contrato):** IMPLEMENTADO_LOCAL. OpenAPI 3.1.0, `/api/v1`, `traceId`, dinero como string y cabeceras verificadas por validador.
- **T-150-02 (Descubrimiento):** IMPLEMENTADO_LOCAL. Plan de publicacion con checksum recalculado y CORS con allowlist; publicacion real `NOT_RUN`.
- **T-150-03 (Compatibilidad):** IMPLEMENTADO_LOCAL. Matriz de 75 operaciones con clases 33/23/12/7 y campos obligatorios.
- **T-150-04 (Evidencia HTTP):** IMPLEMENTADO_LOCAL. 13 pruebas planificadas en `NOT_RUN`; un `PASS` sin AWS o sin huella se rechaza.
- **T-150-05 (Cambio):** IMPLEMENTADO_LOCAL. SemVer, ventana de deprecacion, RFC y deteccion en CI exigidos.
- **T-150-06 (Alcance):** IMPLEMENTADO_LOCAL. Mobile, Play Store y landing fuera del gate.
- **T-150-07 (CDN/moderacion):** NO_APLICA_HOY; el inventario no expone EPs de medios.
- **T-150-99 (Ratificacion):** REVIEW pendiente; merge requiere autorizacion humana ligada a PR + HEAD.

### Ampliacion de alcance (2026-11-09)

- **Paridad:** comando `paridad` contrasta JSON (75/55), YAML (75/55) y matriz H06 con `operationId`.
- **Matriz:** 19 columnas por operacion, incluidas pruebas positiva/negativa y estado HTTP.
- **Evidencia HTTP:** cuatro niveles separados y generador de `curl` que no ejecuta nada.
- **Cambio:** `comparar_contrato_api.py` en CI contra `origin/main`, con excepciones solo por RFC aprobada.
- **Frontera:** `FRONTERA_RESPONSABILIDAD.md` y validacion de que Mobile no es gate.
- **Contenido:** `contenido` detecta texto libre en E028, E030 y E075; `MODERACION_CDN_I150.md` fija el criterio.

### Correccion de revision (2026-11-09)

- **Paridad semantica:** parser de bloque del YAML que reconstruye la estructura y compara por operacion (metodo+ruta, operationId, catalogo, seguridad, parametros, requestBody y respuestas); rechaza tabulaciones, duplicados y `$ref` irresolubles.
- **Breaking changes:** comparador ampliado a respuestas (propiedad, tipo, enum, requerido, esquema) y seguridad efectiva con scopes; `$ref` roto falla cerrado; excepcion solo con RFC ratificada en `sdd/_control/H08`.
- **INF-150-07:** `MODERACION_CDN_I150.md` en `DISEÑO_LOCAL_COMPLETO — IMPLEMENTACIÓN_CLOUD_PENDIENTE` con las 3 operaciones y `BLOCKED_DEPENDENCY` para moderacion externa; el validador rechaza contenido sin tratamiento.

### Correccion final (2026-11-09)

- **A:** PyYAML 6.0.2 fijado y usado con cargador que rechaza duplicados; paridad semantica de `components.schemas`.
- **B:** excepciones de breaking changes solo con autorizacion humana especifica de API, vigente y acotada; RFC de otras materias rechazadas.
