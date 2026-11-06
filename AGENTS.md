# Nexum-Backend — AGENTS.md operativo H08-A

**Autoridad:** decisión humana expresa > `constitution.md` > 103 specs aprobadas/documentadas > registro `sdd/_control/H07/ESTADO_Y_GATES.md` > este archivo > perfiles/skills. Si hay contradicción bloquear y preguntar; no ocultarla ni editar SDD para forzar PASS.

## Fase única
H08-A: implementación **local** de arquitectura Go de A-000 a A-110, una spec a la vez, iniciando A-000. H03/H04 aprobados; H05 aprobado documentalmente con bloqueos AWS; H06 baseline aprobado con fuente docente/prototipo pendiente de cotejo independiente; H07 publicado y CI verde. La ratificación solo es válida si el usuario ejecutó el instalador con frase exacta y fue integrado el PR con GitHub Actions verde.

## Restricciones absolutas
- Solo Backend Go y sus contratos. Mobile, Play Store, landing y aprobación móvil fuera de alcance.
- No modificar sin RFC/decisión `constitution.md`, `sdd/**`, `.github/**`, `.opencode/**`, `opencode.json`, contratos OpenAPI ni el repositorio de terceros.
- No AWS ni Terraform (incluidos `plan`/`apply`), no actualizaciones AWS Paid, no recursos cloud, no dinero personal, no Docker ni WSL, no `git push` o `gh` desde OpenCode; operador publica PRs.
- No leer/escribir/copiar secretos `.env`, tokens, `.aws`, `.ssh`, `*.pem`, `*.tfstate`; solo datos ficticios locales para unit tests.
- El permiso shell ASK se revisa **cada vez** y no se permite «Allow always»; una configuración no es aislamiento de OS. Un comando indirecto que permita AWS o borrar datos sigue prohibido.

## Responsabilidades y secuencia
1. Antes de cada acción: leer constitución, AGENTS, memoria, `sdd/_control/H08/CONTRATO_INICIO_H08A.md`, cuatro documentos de la spec A objetivo y dependencias.
2. Primera respuesta por spec: **solo plan**, rutas exactas `cmd/**`, `internal/**`, `pkg/**`, `tests/**`, `go.mod`, `go.sum`, riesgos y comandos de prueba; esperar aprobación.
3. Después de autorización concreta: delegar Go al arquitecto, revisión de conformidad a SDD guardian y QA; ninguna ejecución por sorpresa.
4. Validar con Go nativo en Windows: compilación, `go test`, `go vet`, `go test -race` si la plataforma lo soporta, sin afirmar pruebas ejecutadas cuando no lo están.
5. Equipo crea PR manual; CI verde y aprobación humana por spec antes de cambiar de A-000 a A-010 etc. Implementación I-000..I-150 y E001..E075 esperan sus propios Gates.

## Estado técnico
103 specs/412 docs/75 rutas H06; GitHub CI SDD H07 success (evidencia compartida). Go backend, API HTTP real, DynamoDB/Redis reales, AWS FinOps y despliegue = **NOT_RUN**. Los encabezados DRAFT de las specs reflejan fecha de redacción, no sustituyen la aprobación formal H04 del control vigente. Cualquier diferencia de fuente docente o prototipo se registra RFC; no declarar cobertura absoluta de fuentes.

## 8 agentes y 9 skills
- Orquestador principal `nexum-orchestrator` coordina sin escribir; Go architect implementa solo rutas de código y con ASK.
- `sdd-guardian`, `qa-evidence-verifier`, `devsecops-security`, `api-contract-guardian`, `finops-guardian` revisan dentro de su competencia.
- `aws-terraform-engineer` conserva rol pero no puede operar AWS en esta fase.
- Las skills se consultan con permiso ASK según `opencode.json`; las skills no conceden acceso por sí mismas.

## Salida obligatoria de cada propuesta
`FASE | SPEC | GATE | REQUISITOS/FUENTES | ARCHIVOS EXACTOS | COMANDOS PROPUESTOS | PERMISOS | PRUEBAS/ESTADO | BLOQUEOS | APROBACIÓN REQUERIDA`.

## Inicio
Leer `sdd/_control/H08/PRIMERA_TAREA_OPENCODE_A000.md` y producir plan para A-000 sin cambios. Detener ante divergencia de permisos reales, errores o petición de AWS. Nunca marcar `DEPLOYED_VERIFIED` sin ejecutar cloud/HTTP real.
