---
name: Validate CI Security Nexum
description: "Revisar quality gates de Backend: SAST, deps, secretos, SBOM, IaC, contratos y PR/CD autorizado, sin iniciar pipelines."
metadata:
  opencode/autoinvoke: false
---

# Skill — validate-ci-security

**Estado:** H03-D / REVIEW. **Invocación:** no automática; la carga sigue técnicamente denegada hasta cambio autorizado y comprobado. No crea código, specs ni servicios.

## Disparador y objetivo

Al especificar QA/DevSecOps en H04/H05 o al revisar resultados reales de PR/releases desde H08.

## Entradas requeridas

- GOV/R, threat model, contratos, política de ramas y gates aprobados.
- Workflows solo cuando hayan sido creados legítimamente desde H08.
- Logs sanitizados, commit SHA, artifact y resultado verificable.

## Precondiciones y fase

GitHub nuevo no existe en H03; no crear workflows en fase SDD; CI skipped no es PASS.

## Alcance y archivos autorizados

Revisión de reglas documentales H03, especificación en H04/H05, ejecución de PR/CI solo H08+ tras publicación H07.

## Flujo operativo detallado

1. Identificar QG-0..6 y evidencias previstas por lote sin confundir aprobaciones con ejecución.
2. Comprobar principio de mínimo privilegio, riesgos SAST/SCA/secret scan/DAST/SSR, SBOM, cadena de suministro, CI/CD.
3. Evaluar segregación PR→build→security→artifact SHA→autorización humana→deploy→comprobación HTTPS.
4. Garantizar que el runner no tenga permisos cloud amplios, ni secretos en logs, ni despliegue automático por push.
5. Reportar estado real de cada gate (`passed`, `failed`, `skipped`, `not_run`) y fallos críticos bloqueantes.

## Comandos y controles seguros

En H03 solo revisar texto; desde H08 `git diff --check` local y comprobaciones CI aprobadas, sin `gh workflow run`, `gh pr merge` ni pushes no autorizados.

## Criterios de error y escalamiento

SAST omitido, pipeline skipped tratado como éxito, credenciales expuestas, sin revisión de despliegue -> `BLOCKED`.

## Trazabilidad mínima

GOV-11..18, R-015..029, R-040..060.

## Contrato transversal de seguridad y gobierno

- **Autoridad:** leer `constitution.md` (`GOV-01..18`, `R-001..060`), `AGENTS.md` y `memoria.md` antes de usar la skill. Si falta una fuente, detener la acción y registrar `BLOCKED`.
- **Fase única:** H03-D solo crea instrucciones y verifica estructura; H04/H05/H06 son especificación por fases; H07 publicación autorizada; H08+ implementación aprobada. Nunca crear documentos de fases no abiertas ni código o infraestructura por invocar una skill.
- **Estado inicial:** la skill es **documentación** y no se ejecuta automáticamente. `opencode.json` y los ocho agentes mantienen `skill: deny`, `shell: deny`, `subagent: deny` y prohibiciones de operaciones mutantes. Ningún paso de esta skill eleva permisos o autoriza romper esas denegaciones.
- **Privacidad:** nunca abrir, copiar o imprimir `.env`, claves/token JWT, credenciales AWS/SSH, `.tfstate`, parámetros secretos, dump financieros ni respaldos. Nunca invocar proveedores de pago ni servicios externos por defecto.
- **Decisiones:** un agente no autoaprueba; toda excepción, costo, cambio de alcance o mutación requiere autorización humana específica. Distinguir `DRAFT`, `REVIEW`, `APPROVED` y `NOT_RUN` de resultados realmente verificados.
- **Comandos seguros:** los citados son ejemplos de validación para el **responsable humano** en PowerShell 7, únicamente en la fase en que estén permitidos. No autorizan ejecutar shell desde OpenCode.

## Formato obligatorio del resultado

```text
SKILL / FASE / ESTADO DOCUMENTAL:
OBJETIVO, FUENTES Y VERSIONES:
REQUISITOS (GOV/R y fuente del producto/docente):
ENTRADAS / RUTAS PERMITIDAS / RUTAS EXCLUIDAS:
VALIDACIONES: comando | alcance | PASS/FAIL/BLOCKED/NOT_RUN | evidencia
RIESGOS / DECISIONES ABIERTAS:
AUTORIZACIÓN HUMANA REQUERIDA:
SIGUIENTE PASO ÚNICO:
```
