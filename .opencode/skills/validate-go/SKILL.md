---
name: Validate Go Nexum
description: "Definir y, solo en H08 autorizado, verificar Go build, unit/integration, race, vet, cobertura y calidad con evidencia real."
metadata:
  opencode/autoinvoke: false
---

# Skill — validate-go

**Estado:** H03-D / REVIEW. **Invocación:** no automática; la carga sigue técnicamente denegada hasta cambio autorizado y comprobado. No crea código, specs ni servicios.

## Disparador y objetivo

Diseñar criterios de calidad Go en H04/A-100 o validar implementación real aprobada desde H08.

## Entradas requeridas

- Estado del lote y SDD aprobado; módulo Go y dependencias solo si ya existen legítimamente en H08.
- Cobertura, invariantes financieros, límites de seguridad y contratos relevantes.
- Permiso explícito de ejecutar comandos locales en fase implementativa.

## Precondiciones y fase

Antes de H08 jamás generar `go.mod`, código, descarga de dependencias o pruebas simuladas.

## Alcance y archivos autorizados

Durante H03 se escribe solo esta instrucción; H04/H05/H06 definen pruebas documentales; H08+ pruebas en fuentes Go autorizadas; fuera de alcance infra/secretos.

## Flujo operativo detallado

1. Comprobar aprobación QG-0, rama/código existente y ausencia de secretos en la carpeta de pruebas.
2. Definir controles futuros: `gofmt` (solo lectura en verificación), `go vet`, `go test`, `go test -race`, cobertura, tests seguridad.
3. En H08+ y tras permiso específico, ejecutar validaciones una por una con versiones y evidencia.
4. Separar tests unitarios, integración DynamoDB/Redis y HTTP real; no convertir mocks en verificación final.
5. Bloquear release si falla cualquiera de los gates obligatorios; no ocultar warnings.

## Comandos y controles seguros

`go version`; desde H08 con autorización: `go test ./...`, `go test -race ./...`, `go vet ./...` (pueden generar cachés/descargar módulos; no ejecutar ahora).

## Criterios de error y escalamiento

H03–H07 con solicitud de ejecutar Go -> `BLOCKED`; errores, race, cobertura insuficiente o dependencias faltantes -> `FAIL`/`BLOCKED` según evidencia.

## Trazabilidad mínima

GOV-01, GOV-08, GOV-09, GOV-11..13, R-007..009, R-030..034, R-040..045.

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
