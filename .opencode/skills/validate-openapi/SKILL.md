---
name: Validate OpenAPI Nexum
description: "Comprobar en diseño o implementación contratos OpenAPI, cobertura total, consistencia, seguridad, errores y compatibilidad versionada."
metadata:
  opencode/autoinvoke: false
---

# Skill — validate-openapi

**Estado:** H03-D / REVIEW. **Invocación:** no automática; la carga sigue técnicamente denegada hasta cambio autorizado y comprobado. No crea código, specs ni servicios.

## Disparador y objetivo

Al revisar contratos API en H04/H06 o sus pruebas HTTP una vez implementados y autorizados.

## Entradas requeridas

- Inventario de operaciones proveniente del prototipo vigente (cuando corresponda).
- Especificaciones A/I/EP aprobadas, convenciones de errores y OpenAPI contractual solo después de su fase.
- Evidencias HTTP/contrato versionadas cuando existan.

## Precondiciones y fase

No inventar rutas, payloads, schemas ni respuestas reales; no fijar N=7 ni depender de aprobación Android.

## Alcance y archivos autorizados

En H03 la skill es instrucción; H04 especifica lineamientos; H06 examina el catálogo y artefactos de contrato autorizados; H08 valida implementaciones.

## Flujo operativo detallado

1. Separar flujos Android locales de necesidades Backend y mapear cada necesidad a operaciones reales.
2. Verificar método/ruta, versionado, autorización, propiedad, errores, límites, paginación e idempotencia.
3. Comprobar schemas, referencias, ejemplos etiquetados y cambios incompatibles que requieran RFC.
4. Asociar EP→A/I→DynamoDB/Redis/seguridad, cobertura de pruebas y compatibilidad clientes.
5. En H08+, comparar OpenAPI publicado con respuesta HTTPS real y estado de datos autorizado; nunca reportar contract-only como deploy verificado.

## Comandos y controles seguros

Revisión documental manual de `spec.md` y OpenAPI únicamente cuando exista en su fase. En H03 **no ejecutar** linter ni crear especificaciones.

## Criterios de error y escalamiento

EP sin caso de uso, endpoint necesario omitido, `2xx` ficticio, prueba Mobile como gate -> `BLOCKED`/`FAIL`.

## Trazabilidad mínima

GOV-06..10, GOV-13, GOV-17, R-030..039, R-040..051.

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
