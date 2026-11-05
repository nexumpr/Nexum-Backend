---
name: Record Evidence Nexum
description: "Registrar pruebas y decisiones como hechos fechados, con hash y origen, sin inventar pruebas o publicar material sensible."
metadata:
  opencode/autoinvoke: false
---

# Skill — record-evidence

**Estado:** H03-D / REVIEW. **Invocación:** no automática; la carga sigue técnicamente denegada hasta cambio autorizado y comprobado. No crea código, specs ni servicios.

## Disparador y objetivo

Después de cada lote, verificación local, PR/CI, prueba HTTP o cambio de estado documental.

## Entradas requeridas

- Salida íntegra o resumen verificable del responsable con comando, fecha, entorno y fuente.
- Lista exacta de archivos autorizados, hash previo/actual, gate aplicable.
- Estado previo de memoria y documentación; ningún secreto como evidencia.

## Precondiciones y fase

No escribir `APPROVED` sin consentimiento humano; no crear evidencia futura ni afirmar que existe una URL.

## Alcance y archivos autorizados

`memoria.md` cuando la fase y el humano autoricen su actualización; evidencias sensibles permanecen fuera del proyecto. En H03 este script solo integra un registro ya respaldado por salidas del usuario.

## Flujo operativo detallado

1. Registrar hecho observado con fecha, fuente, comando y resultado; anotar evidencia reportada frente a ejecutada personalmente.
2. Calcular hashes solo de archivos permitidos, sin publicar paths/valores secretos.
3. Mantener estado `PASS/FAIL/BLOCKED/NOT_RUN` y no equiparar SHA con cumplimiento funcional.
4. Distinguir pruebas locales y despliegues; fechas y entornos de verificación se conservan aunque el runtime se apague.
5. Actualizar `memoria.md` sin inventar CI/PR/URLs/versión deploy. Solicitar firma humana para gate.

## Comandos y controles seguros

`Get-FileHash -LiteralPath ".\constitution.md" -Algorithm SHA256` (comando de lectura humana).

## Criterios de error y escalamiento

Ausencia de salida verificable, ruta sensible, estado inflado o edición fuera de fase -> `BLOCKED`.

## Trazabilidad mínima

GOV-12..18, R-003..014, R-040..046, R-053..060.

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
