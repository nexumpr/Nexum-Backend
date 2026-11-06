# Apertura controlada de H08 (NO habilitada automáticamente)

**Condiciones previas obligatorias:** ratificación formal Gate H06 y Gate H07, CI verde en rama principal, conformidad de fuente docente/prototipo o RFC de diferencias y aceptación explícita de comenzar H08.

Una vez autorizado, abrir RFC de permisos OpenCode con cambios mínimos en `opencode.json`: permisos por agente/operación y ruta, sin acceso a secretos ni aprobación automática de AWS. Ejecutar prueba real de motor en OpenCode; solo entonces orquestador podrá delegar skills/agents y comandos necesarios para la fase autorizada. Esta corrección de H07 **no modifica `opencode.json`**.

Secuencia de ejecución: A (Clean Architecture Go, manejo de HTTP/JWT/errores/testing) → I (IaC AWS completa, plan/apply autorizados, OIDC, VPC privada/CloudFront/ALB, 2 EC2 simultáneas, DynamoDB, Redis, observabilidad, backups, suspensión solo de auxiliares y recuperación; cero dinero personal) → E (75 operaciones por entregas pequeñas autorizadas, HTTP real, regresión/CI/DevSecOps). El equipo puede preparar tareas/PR documentales antes de H08, pero no validar despliegues inexistentes.

Antes del primer `apply`: evidencia de AWS Free, precios reales SKU, uso de crédito y vigencia, elegibilidad de CloudFront, ruta de egreso, TLS, SCP/IAM, plan de cierre y preservación de datos. Rechazar plan que arriesgue dinero personal o cierre inesperado por créditos. No cambiar AWS Free→Paid.
