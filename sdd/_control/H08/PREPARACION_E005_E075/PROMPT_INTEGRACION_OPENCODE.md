# Prompt único de integración — ejecutar SOLO desde OpenCode

**MODELO: DeepSeek V4.1 Flash (High)**. Motivo: integrar la preparación contractual y de permisos con mínimo retrabajo. Consumo esperado: medio. Cambiar modelo solo por autorización explícita y a otro DeepSeek tras bloqueo reproducible.

## Misión

Prepara Nexum-Backend para implementar E005–E075. NO implementar E005 todavía, NO desplegar, NO hacer AWS mutante, NO cambiar E001–E004. Operar en Windows, Warp/PowerShell 7, sin Docker/WSL/Bash.

### Punto de partida y preservación

- Repo: `C:\Proyectos\Nexum\Nexum-Backend`.
- `main` en `8bc2548339d6fdef9211c27c157bf75568fa052d` según Scrum Master; verificar con Git local y remoto.
- ÚNICO archivo modificado local: `opencode.json`, con diez excepciones de EC2/ASG/ELBv2. **NO DESCARTAR, NO SOBRESCRIBIR, NO STASH DESTRUCTIVO, NO RESET/CLEAN.**
- PR #64 ya MERGED; no reabrir.
- Paquete ZIP de preparación provisto por el responsable, guardado por él dentro de `dist/tmp` o una ruta de lectura autorizada. Si no está localizable, detener solo la integración, no inventar ruta ni descargar código de terceros.

### Fase 0 — preflight no destructivo

1. `git status --short`, `git rev-parse HEAD`, `git fetch origin` solo bajo permisos existentes. Si cambia HEAD respecto al ZIP, comparar y reconciliar, no sobrescribir.
2. Guardar SHA256 y copia de seguridad de `opencode.json` local fuera del área de commits, sin imprimir secretos. Registrar diffs **solo reglas y políticas** contra la versión commiteada y el candidato.
3. Comprobar versión OpenCode instalada, `opencode debug config` y `.opencode/agents`, schema V2 y reglas reales con PowerShell. Nunca declarar prueba motor PASS basándose en simulación.
4. Verificar `main`/PR64 y registros E004 sin reauditar AWS. E001–E004 intocables.

### Fase 1 — reconciliación documental

5. Leer `sdd/_control/H08/PREPARACION_E005_E075/MATRIZ_RECONCILIACION_E005_E075.md`, `fichas/E005.md`...`E075.md`, RFC-016 DRAFT y PDF definitivo. Los archivos `spec.md`, `plan.md` y OpenAPI actuales están APPROVED; NO editar antes de Gate de contrato.
6. Verificar campos anidados y posibles cortes del extractor en la auditoría. Clasificar cada diferencia ESSENTIAL/COMPATIBILITY/OPTIONAL/OBSOLETE, evitando cambios especulativos. Mantener rutas, métodos y E001–E004 intactos.
7. Obtener aprobación RFC-016 y el Gate de contrato para enmiendas aprobadas. Si no hay autorización, preparar diffs concretos y detener solo los cambios protegidos; mantener propuestas para revisión.
8. No introducir `SESSIONHASH` como patrón nuevo. Para E005–E011 proponer reemplazo documental por `USER#id/SESSION#id`, `REFRESH#digest/SESSION`, solo con Gate; no crear tabla ni índice nuevo.
9. Resolver antes de implementar E005 la discrepancia de response `Accepted` vs `{accepted,message,retryAfterSeconds}` y disponibilidad real del correo; escoger proveedor autorizado y sin costes personales. Probar no enumeración, TTL, uso único y rate limit en futura implementación.

### Fase 2 — permisos permanentes

10. Comparar **tres versiones**: ZIP `opencode.json` de HEAD, configuración local modificada y `opencode.permanente.CANDIDATO.json`. Aplicar SOLO diferencias justificadas con Gate RFC-017; NO hacer copia ciega.
11. Conservar prohiciones hard-deny; configurar ASK permanente para Terraform apply, S3, STS, ASG y edición SDD protegida, sin autorización global AWS. No ejecutar acciones AWS para probar ASK.
12. Validar configuración REAL en versión instalada, simulaciones read-only, precedencias, PowerShell y agentes. Revisar approvals persistidas; no elegir `Allow always` para operaciones sensibles.
13. Si los cambios activos `opencode.json` deben commitearse, hacerlo en rama y con manifest recalculado desde blobs exactos; no mezclar cambios locales temporales por error. Si el motor o políticas corporativas bloquean, no eludir y reportar.

### Fase 3 — implementación de la preparación, NO E005

14. Preparar rama `chore/pre-e005-contratos-permisos` desde `main` limpio **sin perder el cambio local**; si checkout colisiona, detener y presentar alternativa segura, sin reset/clean. No implementar código de endpoints.
15. Integrar únicamente documentos de preparación autorizados, la configuración aprobada y documentación SDD pertinente. Actualizar `memoria.md` con hitos verdaderos. Regenerar `BASELINE_SHA256.json` con archivos versionados LF; no modificar checksum para ocultar drift local.
16. Ejecutar `validar_h06.py`, `validar_sdd_integral.py`, paridad YAML/JSON, Go build/vet/test con toolchain compatible disponible, checks seguridad pertinentes. Corregir fallos del alcance en el mismo PR.
17. Crear un solo PR de preparación conforme a gobierno, obtener CI verde, solicitar Gate merge con HEAD SHA40, NO fusionar sin respuesta humana explícita. No AWS mutante.
18. Tras merge autorizado, verificar main CI y las configuraciones desplegables de OpenCode; `READY_FOR_E005` solo si todos los requisitos quedan demostrados; en otro caso `READY_WITH_LIMITATIONS` o `BLOCKED` con exacta causa.

### Entrega y detención

- Tabla 71 diferencias con clasificación y RFC correspondientes.
- Versiones/hashes del `opencode.json` local, ZIP y resultado integrado; test real del motor.
- Archivos modificados y justificación; tests PASS/FAIL/NOT_RUN; PR/CI y Gates.
- Estado real de E001–E004 sin afirmaciones de nuevas verificaciones AWS.
- Estado `READY_FOR_E005`, `READY_WITH_LIMITATIONS` o `BLOCKED`.
- **DETENERSE antes de comenzar E005.** No crear rama E005, no publicar artefactos ni ejecutar Terraform/ASG.
