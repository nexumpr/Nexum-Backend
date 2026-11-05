---
name: Read SDD Nexum
description: "Leer fuentes aprobadas y estado de la fase activa sin modificar archivos, saltar gates ni leer secretos."
metadata:
  opencode/autoinvoke: false
---

# Skill — read-sdd

**Estado:** H03-D / REVIEW. **Invocación:** no automática; la carga sigue técnicamente denegada hasta cambio autorizado y comprobado. No crea código, specs ni servicios.

## Disparador y objetivo

Al iniciar una sesión o recibir una tarea sobre el SDD, antes de analizar especificaciones o proponer cambios.

## Entradas requeridas

- Identificador del lote activo y la solicitud humana vigente.
- `constitution.md`, `AGENTS.md`, `memoria.md` y, solo si existen y están en la fase abierta, sus spec/plan/tasks/checklist ratificados.
- Registro de aprobaciones humanas y ADR/RFC, si ya existen.

## Precondiciones y fase

Distinguir instalados/verificados de aprobados; no interpretar `APPROVED` inexistente. No abrir fuentes históricas privadas.

## Alcance y archivos autorizados

Solo lectura de gobierno y de la carpeta `sdd/` aprobada para esa fase; H03 permite únicamente los seis archivos raíz y `.opencode/` autorizados.

## Flujo operativo detallado

1. Identificar fase efectiva y gate previo con fuentes versionadas; si no hay evidencia de autorización, `BLOCKED`.
2. Leer constitución y mapear `GOV-*`, `R-*` aplicables sin copiar secretos.
3. Cargar memoria, registrar diferencias entre hechos, propuestas y pendientes.
4. Leer únicamente documentos de specs permitidas, no crear carpetas futuras para facilitar búsquedas.
5. Construir un resumen de entradas/dependencias con conflictos y un único paso permitido.

## Comandos y controles seguros

`Get-ChildItem -LiteralPath "C:\Proyectos\Nexum\Nexum-Backend" -Force` (lectura humana solo H03).

## Criterios de error y escalamiento

Falta de evidencia del gate, contradicción no resuelta o archivo privado -> `BLOCKED`.

## Trazabilidad mínima

GOV-14, GOV-16, GOV-17, R-001..014, R-040..046, R-053..060.

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
