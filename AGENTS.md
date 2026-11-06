# AGENTS.md — instrucciones operativas vigentes de Nexum-Backend

**Corte documental:** 2026-11-05, corrección estructural H07 posterior al commit inicial `84be72d`. **Una sola fuente de estados:** `sdd/_control/H07/ESTADO_Y_GATES.md` (no reemplaza la constitución). Historial anterior conservado en Git; esta versión sustituye las tablas temporales H03/H04/H05 que ya eran obsoletas.

## 1. Autoridad y prelectura obligatorias

1. Leer íntegros `constitution.md`, `AGENTS.md` y `memoria.md` antes de proponer tareas. Si hay conflicto normativo, **BLOCKED** y pedir decisión humana; no resolver unilateralmente.
2. Leer los cuatro documentos de la especificación objetivo (`spec.md`, `plan.md`, `tasks.md`, `checklist.md`), sus fuentes y la trazabilidad correspondiente. Nunca declarar una spec aprobada por estar escrita.
3. Jerarquía: decisión humana explícita y fuente docente acreditada → `constitution.md` (GOV-01..18, R-001..060) → specs ratificadas → `AGENTS.md` → agentes/skills. El PDF docente y la maqueta no han sido cotejados otra vez directamente en H07; las diferencias se formalizan mediante RFC, no se inventan.
4. Alcance: exclusivamente Backend, contratos, datos, Go, DevSecOps e infraestructura; NO Kotlin/Mobile, Play Store, landing ni aprobación de integración por equipo móvil. **Ningún gate depende del equipo Mobile.**

## 2. Estado único y transición de fase

| Fase | Estado vigente | Qué permite |
|---|---|---|
| H03 | APPROVED | Preservar constitución, 8 agentes, 9 skills y permisos actuales |
| H04 | APPROVED | 12 specs de arquitectura ratificadas; cambios solo vía RFC |
| H05 | APPROVED_DOCUMENTAL_CONDICIONADO | 16 specs de IaC/infra, sin permiso de AWS real |
| H06 | INSTALL/VERIFY PASS; GATE PENDIENTE | Catálogo 75 ops y OpenAPI 3.1 documentados |
| H07 | Primera publicación ya hecha; GATE PENDIENTE | Corregir CI/estructura mediante PR y validar sin reescribir historia |
| H08+ | NOT_STARTED; requiere autorizaciones explícitas | Implementar una vez ratificados los gates; no hay permisos nuevos por este texto |

Una autorización expresa de **corrección de publicación** permite este PR documental, no `terraform apply` ni implementación. Las aprobaciones humanas que falten se registrarán con texto y fecha, no con inferencias. El README y `memoria.md` remiten a esta tabla; si cualquiera discrepa, bloquear.

## 3. Directorios exactos y alcance por bloque

- Arquitectura: `sdd/01-arquitectura-general/A-000..A-110` (12 × 4). No reescribir H04 aprobado sin RFC.
- Infraestructura: `sdd/02-infraestructura/I-000..I-150` (16 × 4). Terraform, dos EC2 reales en simultáneo, DynamoDB, Redis OSS compartido, CloudFront/ALB, SSL/HTTPS, IAM/OIDC, GitHub Actions, DevSecOps, FinOps y respaldo/reconstrucción **obligatorios en implementación**.
- Endpoints: `sdd/03-endpoints/E001..E075` (75 × 4), OpenAPI JSON/YAML, 33+23+12+7; cambios E076+ solo vía RFC con actualización de esquemas, DynamoDB, pruebas, índices y validadores.
- **Control no funcional:** `sdd/_control/H07/` reúne Gates, trazabilidad, manifiesto, evidencias y plan. No es una especificación ni una cuarta fase. No recrear `gobierno/H07`.
- `herramientas/` y `.github/workflows/` únicamente son validadores y CI documental hasta autorización H08; no despliegan AWS.

## 4. Permisos reales y límites

`opencode.json` mantiene en H03 las denegaciones `shell`, `subagent`, `skill`, `edit:sdd/*`, `edit:.github/*`, `edit:*.go`, `edit:*.tf`, entre otras. **Por tanto OpenCode no puede realizar todavía la implementación automatizada.** No asumir que haber escrito un agente o una skill concede capacidades. A H08 le corresponde un cambio **separado, mínimo, comprobado y autorizado** de permisos; verificar con OpenCode real, registrar evidencia y sin deshabilitar seguridad para evitar gates.

Prohibidos sin autorización específica: ampliar permisos, lanzar subagentes/shell, crear/modificar AWS, activar Paid, saltar límites financieros, manipular secretos, `git push --force`, borrar repositorios, desplegar APIs o pruebas destructivas. Prohibido guardar en repo `.env`, `.aws`, llaves, estados Terraform, tokens, facturas identificables, backups o trazas sensibles.

**AWS:** 120 USD en créditos fue el saldo referido, no prueba de disponibilidad futura; dinero personal = **0 MXN**. Cuenta Free debe mantenerse Free, con fallos cerrados de costos y permisos; una alerta sola no es límite duro. Pre-deploy pendientes: permisos VPC Origins, SKU/importe Redis, IVA/créditos, conectividad sin NAT, TLS de origen, saldo/vencimiento/consumos, permisos IAM/SCP. La API (2 instancias) y DynamoDB deben ser 24/7 cuando el sistema esté desplegado; solo recursos auxiliares se suspenden mediante IaC. No declarar `PASS` AWS sin ejecución real.

## 5. Protocolo de trabajo (toda tarea)

Antes de modificar presentar:

```text
FASE Y GATE VIGENTES:
REQUISITO / SPEC ID:
FUENTES LEÍDAS Y LÍMITES:
ARCHIVOS EXACTOS A CAMBIAR:
DEPENDENCIAS / RFC NECESARIO:
VALIDACIÓN REPRODUCIBLE:
AUTORIZACIÓN HUMANA DEL PASO:
```

Separar `DRAFT/REVIEW/APPROVED` (documento), `NOT_STARTED/IMPLEMENTED_LOCAL/VERIFIED` (código), `NOT_DEPLOYED/DEPLOYED_VERIFIED` (nube), y `PASS/FAIL/BLOCKED/NOT_RUN` (prueba). Un archivo generado no es código funcionando ni una aceptación del usuario.

Una vez autorizado H08: PRs pequeños, CI/DevSecOps, tests unitarios/contrato/integración/HTTP real, release **de una nueva operación por vez** con regresión y aprobación humana para despliegue. IAM mínimo y no credenciales en Git; IaC automatizará plan/apply/estado/suspend-aux/resume-aux/reconcile/rollback/destroy-aux. No operar infraestructura manualmente como sustituto de Terraform, y nunca eliminar esenciales para ahorrar crédito sin aprobación específica.

## 6. Los ocho agentes (solo perfiles documentados hasta cambio de permisos)

- `nexum-orchestrator`: coordina dependencias, no concede aprobación ni usa delegación mientras esté DENY.
- `sdd-guardian`: GOV/R, trazabilidad y control de cambios; no omite requisitos.
- `go-backend-architect`: Go, capas, invariantes, seguridad de dominio; no programa antes de H08.
- `aws-terraform-engineer`: IaC completa, doble nodo, datos, red, rollback; no ejecuta AWS sin gates.
- `devsecops-security`: IAM, sesión/JWT, secretos, pruebas y threat model; no bypass.
- `api-contract-guardian`: OpenAPI y compatibilidad; no inventa endpoints fuera de RFC.
- `qa-evidence-verifier`: evidencia, QA y bloqueos; no concede PASS sin ejecución.
- `finops-guardian`: AWS Free, 45 días y cero gasto personal; no asume gratuidad de un servicio por existir créditos.

Las nueve skills instaladas tampoco están habilitadas para su invocación mientras la configuración de H03 lo deniegue. Los perfiles y skills no se modifican en esta corrección de H07.

## 7. Verificar antes de un PR

```powershell
py -3.13 herramientas/validar_sdd_integral.py
py -3.13 sdd/03-endpoints/herramientas/validar_h06.py
py -3.13 -m unittest discover -s herramientas/tests -v
```

Se exige CI verde en rama de PR antes de `main`, salvo corrección de CI documentada por la misma PR y validada en checkout limpio. **No** desactivar la comprobación SHA ni sustituir por un PASS simulado. Cualquier actualización futura de specs requiere nuevo manifiesto, RFC y Gate aplicable. La primera publicación con CI fallido está registrada sin ocultarla en `sdd/_control/H07/CAUSA_RAIZ_CI_Y_MIGRACION.md`.
