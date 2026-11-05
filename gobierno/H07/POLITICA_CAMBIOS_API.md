# API extensible sin romper los 75 contratos actuales

**Baseline:** E001–E075, 33 M + 23 S + 12 E + 7 O, `/api/v1` y `operationId=nexum_e###`. No asumir que Android nunca pedirá nuevas operaciones. El equipo móvil puede proponer requisitos, pero no determina el Gate Backend.

**Solicitud RFC:** registrar actor, pantalla/acción/caso de uso, motivación y fuente/version; evaluar si reutilizar endpoint existente o crear nuevo E076+; marcar REQUIRED/OPTIONAL, dominio, propietario, sensibilidad y costo. Modificar `spec/plan/tasks/checklist`, `MATRIZ_OPERACIONES_H06.csv`, `TRAZABILIDAD_75_RUTAS.md` mediante RFC si el inventario rector cambia (revisar H04 aprobado), OpenAPI YAML+JSON, claves/índices DynamoDB, seguridad/CI, idempotencia, pruebas y compatibilidad. El validador H06 actual está deliberadamente anclado en 75; cambios aprobados requieren evolución conjunta de validador y baseline, **nunca desactivarlo para pasar CI**.

**Compatibilidad:** adición backward-compatible sin romper consumidores; versiones principales o periodos de transición si se elimina/renombra campo, método o ruta. Todo breaking change exige RFC y Gate. No cambiar `operationId` existente, semántica de monto ni códigos HTTP para disfrazar una regresión. No guardar PII ni accesos en ejemplos.

**Gate y presupuesto:** cambios de IA/ML, email/push, índices DynamoDB o recursos AWS deben reevaluar FinOps/seguridad antes de implementación. No se modifica AWS sin permisos y ratificación de riesgo.
