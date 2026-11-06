# RFC-002 — Ejecución real OpenCode V2 para H08-I / I-000

**Decisión técnica:** adaptar los perfiles de solo lectura H08-A a herramientas operativas con controles V2 y aprobación de riesgo (ASK). **Aprobación humana:** se solicita exclusivamente mediante frase expresa del instalador; archivo por sí solo no autoriza.

## Alcance
`opencode.json`, 8 agentes, AGENTS.md, memoria.md, estado/gates H07, pruebas de permisos y este registro. En I-000 se permite escribir artefactos FinOps, modificar documentación de control mediante RFC, ejecutar pruebas, Git branch/add/commit/push y GitHub CLI crear PR/consultar CI con ASK cuando corresponda. Nunca push directo a main, merge sin autorización del responsable ni publicar E endpoints.

## Restricciones esenciales
- `edit` ASK dentro de repo, excepto secretos, constitución y especificaciones `spec.md`/`plan.md` DENY; archivos de evidencia deben sanitizarse antes de Git.
- Shell ASK; comandos de lectura/test permitidos; AWS solo lista explícita de consultas read-only ASK; AWS mutante bloqueada; Terraform fmt/validate ASK, plan/init/apply/destroy DENY en I-000.
- Git/GitHub automatizados por agente con ASK para operaciones que mutan; merge requerirá aprobación humana explícita e CI verde.
- No ejecutar Docker/WSL ni comandos destructivos, ni leer `.env`, `.aws`, `.ssh`, `.tfvars`, `.tfstate`, PEM/KEY. Prohibido extraer secretos por shells indirectos.
- La autorización se extiende exclusivamente a I-000; tras su cierre la sesión debe detenerse. El perfil persistente deberá revisarse en nuevo Gate antes de cualquier I-010.

## Riesgos y validación
Los patrones de herramientas no son sandbox del sistema operativo. Prohibidas autorizaciones Always para acciones sensibles. Ejecutar `opencode debug agents`, comprobaciones de permisos y las pruebas locales/CI. La prueba local de políticas no equivale a verificación de runtime o nube.
