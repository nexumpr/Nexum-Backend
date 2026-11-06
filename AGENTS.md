# Nexum-Backend — AGENTS.md operativo de infraestructura H08-I (I-000)

**Autoridad:** decisión humana expresa > constitution.md > specs aprobadas > sdd/_control/H07/ESTADO_Y_GATES.md > Gate H08-I firmado > este archivo > agentes.

## Estado y alcance activo
- A-000..A-110: implementación Go integrada en main con PR #3..#13; pruebas `go build`, `go vet`, `go test ./... -count=1` PASS según ejecución del responsable; **AWS NOT_RUN**.
- H05: APPROVED_DOCUMENTAL_CONDICIONADO. H08-I abre **SOLO I-000** mediante el Gate local y la frase explícita del instalador; H08-E permanece NO AUTORIZADO.
- RFC-002 habilita herramientas para que OpenCode implemente directamente, sin delegar archivos ni comandos rutinarios al humano. RFC-003 registra la interpretación del histórico de 24 tareas/checklists A sin falsificar firmas.
- Rama de gobierno actual: `chore/h08i-rfc002-rfc003-gate-i000`. Después del PR de apertura y merge, la implementación I-000 irá en rama `feat/h08i-i000-finops` creada por OpenCode desde main limpio.

## Operación autónoma con controles de riesgo
1. **Una spec técnica a la vez**. I-000: leer los 4 archivos (spec/plan/tasks/checklist), confirmar evidencia y plan conciso; obtener aprobación humana de alcance de esa spec UNA VEZ antes de escribir. Después crear archivos, ejecutar validaciones, actualizar registro de tareas y checklist con evidencia, crear commit/PR de la rama, revisar CI, arreglar errores; no terminar solo con planes reiterados.
2. OpenCode puede usar shell, Git, GitHub CLI, escritura y subagentes según `opencode.json`. No remitir al usuario comandos que OpenCode pueda ejecutar con sus propias herramientas. Ante permiso ASK, solicitarlo **dentro de OpenCode** una sola vez por acción; el usuario revisa y pulsa Allow once. No elegir Allow always.
3. Para **este PR de apertura**, OpenCode puede verificar, completar documentos de control RFC-002/003/Gate y actualizar hashes autorizados; ejecutar pruebas, hacer commit, `git push` de la rama, abrir PR y seguir CI. Nunca `push` directo a main. Merge requiere aprobación humana separada del PR y checks verdes.
4. Prohibido cambios de especificaciones (`spec.md`/`plan.md`) sin RFC específica; sdd A tareas/checklists históricos NO se reescriben en I-000. Resguardar `constitution.md`, `.env`, `.aws`, `.ssh`, tfstate/tfvars, secretos, `.pem`, `.key` y datos sensibles.
5. **AWS I-000: consultas de solo lectura** autorizadas una por una en OpenCode, sanitizar salidas, no revelar datos personales ni credenciales. `aws create/update/delete/put`, presupuestos mutantes, upgrade Paid, Terraform init/plan/apply/destroy, despliegue, networking real e I-010+ BLOQUEADOS hasta Gate explícito posterior. AWS Free 0 MXN personales, 45 días, dos EC2, DynamoDB, Redis OSS, ALB/CloudFront HTTPS requisitos indeclinables; si no viable, `BLOCKED_FINOPS` y ADR.
6. **No Docker ni WSL.** Backend exclusivamente; Mobile, Play Store, landing y aprobación móvil ajenos. No declarar PASS sin ejecución ni AWS_DEPLOYED si no está probado por HTTP real.
7. Si el proveedor/modelo carece realmente de herramienta shell/edit, la configuración no puede inventarla: mostrar error exacto y cambiar al agente primario `nexum-orchestrator` en sesión nueva; comprobar `opencode debug agents` y `opencode debug config`.

## Flujo por especificación
Aprobar SPEC → rama de feature → implementar exclusivamente SPEC → gofmt/pruebas/seguridad/conformidad → commit+push+PR por agente → CI y revisión humana → merge autorizado → SPEC siguiente solo tras autorización. No usar pseudoarchivos ni reportes como sustituto de recursos AWS reales.

## Salida compacta
`FASE | SPEC | GATE | CAMBIOS | PRUEBAS_REALES | GIT/PR | BLOQUEOS | SIGUIENTE_APROBACION`.
