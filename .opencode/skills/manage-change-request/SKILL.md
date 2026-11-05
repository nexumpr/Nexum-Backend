---
name: Manage Change Request Nexum
description: "Preparar RFC/ADR por conflicto de requisitos y evaluar impacto de un cambio sin alterar decisiones o fases posteriores."
metadata:
  opencode/autoinvoke: false
---

# Skill — manage-change-request

**Estado:** H03-D / REVIEW. **Invocación:** no automática; la carga sigue técnicamente denegada hasta cambio autorizado y comprobado. No crea código, specs ni servicios.

## Disparador y objetivo

Cuando se detecte discrepancia entre Constitución, PDF docente, prototipo, costos, API, seguridad o specs previamente aprobadas.

## Entradas requeridas

- Conflicto exacto y documentos/decisiones afectados con versión y aprobaciones.
- Consecuencias de mantener vs modificar, restricciones de costo/seguridad/compatibilidad.
- Fase actual y autorizador humano.

## Precondiciones y fase

No crear ADR/RFC fuera de su fase como entregable formal; en H03 registrar propuesta únicamente en conversación o memoria si autorizada.

## Alcance y archivos autorizados

Análisis en texto durante H03; anexos ADR/RFC solo en fase de especificación autorizada; modificaciones a docs ratificados requieren re-aprobación.

## Flujo operativo detallado

1. Detallar problema con evidencia, requisito fuente y norma `GOV/R` aplicable.
2. Comparar al menos opción de conservar, opción de cambiar y consecuencias/riesgos; no decidir a escondidas.
3. Identificar alcance: A/I/EP, gates, costo, seguridad, dependencias y regresión; no abrir carpetas futuras.
4. Proponer decisión al responsable y estado PENDIENTE/BLOCKED, sin tocar código ni cloud.
5. Solo tras aprobación específica de versión/impacto, registrar cambio con historial, validar dependencias y nuevo gate.

## Comandos y controles seguros

En H03 únicamente `Get-FileHash` de archivo de gobierno permitido y revisión de texto; sin Git push, Terraform, AWS ni cambio de estado automático.

## Criterios de error y escalamiento

Cambio sin fuente, requisito eliminado sin consulta, costo oculto o ADR aprobado por agente -> `BLOCKED`.

## Trazabilidad mínima

GOV-14..18, R-001..014, R-023..029, R-039..060.

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
