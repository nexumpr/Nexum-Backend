---
name: Trace Requirements Nexum
description: "Trazar obligaciones académicas y flujos del prototipo a GOV, arquitectura, infraestructura, endpoints y aceptación sin inventar fuentes."
metadata:
  opencode/autoinvoke: false
---

# Skill — trace-requirements

**Estado:** H03-D / REVIEW. **Invocación:** no automática; la carga sigue técnicamente denegada hasta cambio autorizado y comprobado. No crea código, specs ni servicios.

## Disparador y objetivo

En revisión de cobertura, al introducir cualquier obligación o preparar checklist y matriz de trazabilidad de un lote autorizado.

## Entradas requeridas

- Fuente identificable: PDF docente cuando se aporte; mientras tanto, resumen explícito del v1.3 con límite de procedencia.
- Requisitos del producto respaldados por prototipo vigente cuando toque, no suposiciones.
- Lista de GOV/R, decisiones ratificadas y spec de fase abierta.

## Precondiciones y fase

No atribuir al documento académico texto no cotejado. No usar 7 EP históricos como inventario final.

## Alcance y archivos autorizados

Matriz de trabajo dentro de respuesta del lote o artefacto expresamente autorizado en la fase actual; no crear archivos sdd anticipados.

## Flujo operativo detallado

1. Extraer ID del requisito y ubicación exacta de la fuente; distinguir fuente primaria, resumen e inferencia.
2. Clasificar: Backend implementable, Mobile/Landing excluido, dependencia o decisión abierta.
3. Asignar `GOV-*` y, cuando estén ratificadas, A/I/EP, tareas, checklist y prueba.
4. Identificar huérfanos, duplicados, contradicciones y condiciones de bloqueo; no rellenar vacíos.
5. Emitir cobertura comprobable y lista de lagunas que deberán revisarse en la fase que corresponda.

## Comandos y controles seguros

`Select-String -LiteralPath ".\constitution.md" -Pattern "GOV-"` (solo lectura; no copiar secretos).

## Criterios de error y escalamiento

Requisito sin fuente, una necesidad Backend sin dueño, dependencia Mobile como gate o spec inventada -> `BLOCKED`.

## Trazabilidad mínima

GOV-01..18, R-001..014, R-039..050; EP final N aún desconocido.

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
