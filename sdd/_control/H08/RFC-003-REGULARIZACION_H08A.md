# RFC-003 — Registro de cambios históricos H08-A / 24 tasks y checklists

**Hechos verificados por responsable:** A-000..A-110 en main `76ae14c`, PR #3..#13 mergeados; `go build`, `go vet`, `go test` PASS locales; validador SDD documental y CI GitHub Actions verde. **No constituye prueba de recursos AWS ni de endpoints públicos**.

Se modificaron dos documentos por spec (`tasks.md`, `checklist.md`), total 24, con información de tareas, pruebas y algunos textos `APROBADO` sin actas humanas por spec recopiladas en este paquete. Los 12 `spec.md` y 12 `plan.md` no necesitan reescribirse para habilitar I-000.

**Decisión de regularización acotada:** conservar temporalmente los 24 documentos como evidencia histórica en Git, con la interpretación de que un PR mergeado demuestra integración y no sustituye un Gate humano individual. Las afirmaciones de aprobación por spec no respaldadas se consideran `PENDIENTE_ACTA`, no autorizan despliegue ni cambio de fase por sí solas. No falsificar, marcar ni borrar casillas automáticamente; la normalización editorial de los 24 documentos exige revisión posterior documentada por separado, no bloquea una evaluación FinOps de solo lectura. No cambiar requisitos ni generar SHAs como legitimación de firmas inexistentes.

La nueva autoridad humana para I-000 es independiente, explícita y limitada: `GATE_H08I_I000.md`. La integridad existente permanece verificada por `validar_sdd_integral.py`; no afirmar que el contenido de los 24 archivos fue rectificado.
