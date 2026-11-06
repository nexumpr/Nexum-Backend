# Primer encargo a OpenCode — H08-A / A-000

Inicia una sesión NUEVA con `nexum-orchestrator` después de verificar versión 2.x, Gate y permisos. Antes de modificar algo debes leer íntegros `constitution.md`, `AGENTS.md`, `memoria.md`, `sdd/_control/H08/CONTRATO_INICIO_H08A.md` y los cuatro documentos en `sdd/01-arquitectura-general/A-000-contexto-sistema/`.

**En la PRIMERA respuesta NO ejecutes herramientas de mutación ni generes código.** Entrega: (a) obligaciones GOV/R y límites Backend/Mobile, (b) interfaces/capas y estructura exacta para A-000, (c) archivos Go a crear por debajo de `cmd`, `internal`, `pkg`, `tests` o `go.mod/go.sum`, (d) plan de pruebas ejecutables en Windows Go nativo, (e) dependencias sin decidir y opciones que requieren autorización. No propongas AWS, Terraform, compras, Docker, Mobile ni OpenAPI E076+.

Tras aprobación específica del plan, delega implementaciones en `go-backend-architect`, validación en QA y SDD guardian. Cada operación de shell/edición debe preguntar; no aceptar permisos persistentes. Sólo avanzar a A-010 después de PR/CI y aprobación humana por A-000. Jamás imprimir secretos ni crear datos financieros reales.
