---
name: record-evidence
description: Registrar evidencia reproducible sin datos sensibles.
metadata:
  opencode/autoinvoke: false
---

# Skill record-evidence — Nexum-Backend

**Vigencia:** transición H08-A, ratificada con comando humano y Gate documentado. El acceso de skill sólo se concede cuando `opencode.json` y el perfil agente lo permiten; cargar una skill no concede automáticamente shell ni edit.

## Alcance
Documentar comandos, resultados y SHA en texto propuesto para PR humano; no exponer credenciales.

## Procedimiento obligatorio
1. Verificar estado único `sdd/_control/H07/ESTADO_Y_GATES.md` y contrato `sdd/_control/H08/CONTRATO_INICIO_H08A.md`.
2. Leer constitución, AGENTS, memoria y spec A activa completa.
3. Solicitar aprobación humana para toda edición/comando, documentando rutas y pruebas esperadas.
4. Bloquear si falta evidencia, hay secrets o se intenta editar specs, GitHub, IaC, despliegues, Mobile o funciones de otras fases.
5. No usar resultados simulados como PASS, ni actualizar los criterios aprobados para hacer que las pruebas pasen.

## Resultado de la skill
`FASE | SPEC | FUENTE | ACCIÓN PROPUESTA | PERMISO | VALIDACIÓN Y EVIDENCIA | RIESGOS | SIGUIENTE PASO`.

**Restricciones:** solo H08-A local; sin Terraform/AWS/gastos/servicios de pago, sin comandos globales, sin cambios de `main` por agente. Registrar cambios en Git mediante revisión humana y PR.
