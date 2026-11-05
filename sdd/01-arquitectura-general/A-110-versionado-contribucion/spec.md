# A-110 — Versionado, contribución y gates humanos

| Campo | Valor |
|---|---|
| Fase | H04 — Arquitectura general (lote consolidado) |
| Versión | 0.2.0 |
| Estado | **DRAFT — Gate H04 pendiente** |
| Trazabilidad | GOV-01..18, docente v3 |
| Proveedor vigente | AWS; DynamoDB NoSQL; Redis real; Backend Go |
| Fuente original | ZIP Android V6.0.2 `(3)`; PDF 75 rutas `(2)`; PDF docente v3 |

## 1. Objetivo y condiciones de aceptación

Esta especificación establece los **requisitos de diseño vinculantes propuestos**, no código implementado. Se rechazan decisiones sin trazabilidad, seguridad o viabilidad. Si contradice `constitution.md`, el Gate se bloquea y se tramita RFC. No crea recursos AWS, endpoints HTTP reales ni modifica cliente móvil.

## Base documental y autoridad

1. Autoridad superior: `constitution.md` v0.2.0, `AGENTS.md`, H03 humano APPROVED (2026-11-05). Ningún texto de esta entrega cambia permisos ni política.
2. Fuente académica: **Requerimientos para el desarrollo del proyecto Móvil v3(2).pdf**, cuatro páginas; requisitos de Backend y dependencias transversales. Se consultó íntegramente en esta entrega.
3. Fuente funcional: **Nexum_V6_0_2_AndroidStudio_Demo_Eliminar_Cuenta(3).zip**, 16 entradas ZIP. Se leyeron `app.js`, `index.html`, `MainActivity.java`, `AndroidManifest.xml`, `README.md` y se comprobó ausencia de `fetch(` en `app.js`. Código cliente no constituye Backend existente.
4. Catálogo: **Nexum_V6_0_2_Catalogo_Completo_Endpoints_Backend(2).pdf**, 27 páginas; 75 identificadores/rutas propuestas E001..E075. Etiquetas M/S/E/O preservadas. **No adoptar** su stack histórico Google Cloud/MongoDB; el stack vigente del proyecto es **AWS + DynamoDB + Redis**.
5. Evidencia H03: 54 decisiones del motor real (53 DENY, 1 ALLOW) y aprobación humana del Gate H03; ejecución en Windows aportada en Warp. En este lote NO se llama a OpenCode y sus permisos siguen restringidos.

**Trazabilidad:** la etiqueta M/S/E/O describe el origen de la ruta, no constituye por sí sola decisión de implementación ni reemplaza lo exigido por el docente. La formalización OpenAPI por ruta y su aprobación corresponde a H06. Las decisiones físicas de despliegue y costo cero corresponden a H05 y no están presumidas.


## 2. Contrato arquitectónico

### 1. Versionamiento

Matriz de versiones por componente independiente: backend SemVer, API path /api/v1, SDD doc version y release AWS; etiqueta `vMAJOR.MINOR.PATCH` solo en código liberado con changelog y hash verificable.

**Regla verificable:** implementar y probar este comportamiento en H08; cerrar el mecanismo de diseño, su riesgo y el contrato antes del Gate correspondiente.

### 2. Ramas

main protegida; feature/<id> -> PR -> checks -> aprobación -> merge; release/ cuando se necesite corte; no push automático a main ni credenciales personales en workflow.

**Regla verificable:** implementar y probar este comportamiento en H08; cerrar el mecanismo de diseño, su riesgo y el contrato antes del Gate correspondiente.

### 3. Orden SDD

H03 APPROVED, H04 completo DRAFT -> revisión técnica por spec -> Gate H04 humano; H05 (16 specs) solo tras H04; H06 catálogo/endpoints tras H05; H07 audita/pública SDD; H08 implementación una por una.

**Regla verificable:** implementar y probar este comportamiento en H08; cerrar el mecanismo de diseño, su riesgo y el contrato antes del Gate correspondiente.

### 4. Cambios

Cualquier alteración a elección Go/AWS/Dynamo/Redis/2 instancias/finanzas abre RFC y análisis de impacto referencias E-ID, costes y pruebas; aprobador humano antes de editar constitución o spec APPROVED.

**Regla verificable:** implementar y probar este comportamiento en H08; cerrar el mecanismo de diseño, su riesgo y el contrato antes del Gate correspondiente.

### 5. Agentes y permisos

Ocho agentes y nueve skills subordinados a constitution; opencode.json `shell/subagent/skill deny` persistente durante H04. Solo habilitar edit a directorio autorizado bajo control formal por fase, nunca ALL tools/auto.

**Regla verificable:** implementar y probar este comportamiento en H08; cerrar el mecanismo de diseño, su riesgo y el contrato antes del Gate correspondiente.

### 6. Criterio de DONE

Spec/plan/tasks/checklist con aceptación trazada; durante H08 DONE exige implementación real y HTTP, seguridad, idempotencia, QA, registros, CI, autorización humana, despliegue y evidencia.

**Regla verificable:** implementar y probar este comportamiento en H08; cerrar el mecanismo de diseño, su riesgo y el contrato antes del Gate correspondiente.

### 7. Repositorio

Documentación SDD Nexum permitida conforme decisión propia de Nexum, no heredar política de GradeX. Respaldo antes de reset o cualquier destrucción, no operar git remotes ni AWS sin autorización explícita.

**Regla verificable:** implementar y probar este comportamiento en H08; cerrar el mecanismo de diseño, su riesgo y el contrato antes del Gate correspondiente.

### 8. Transferencia

Cada entrega ZIP SHA256 manifiesto, instalador idempotente de solo carpeta autorizada, preflight, backup, rollback verificado y diagnóstico no falsos PASS. PowerShell 7 Warp Windows 11.

**Regla verificable:** implementar y probar este comportamiento en H08; cerrar el mecanismo de diseño, su riesgo y el contrato antes del Gate correspondiente.

## 3. Límites del alcance

- Solamente Backend, datos, seguridad, contratos API e infraestructura gestionada por el equipo Backend; ninguna implementación de Android, landing, Play Store o aprobación del equipo móvil.
- H04 congela **arquitectura lógica**; H05 congela viabilidad y topología física; H06 congela OpenAPI detallado de cada operación; H08 implementa conforme a esas decisiones.
- No convertir opciones propuestas de un catálogo histórico en compromisos silenciosos; no hacer optativas las obligaciones docentes.

## 4. Riesgos específicos y decisiones que deberán sustentarse

- **R-A-110-01:** No confundir prueba local documental con despliegue real.
- **R-A-110-02:** La presión de tiempo no permite saltar gates ni falsear AWS costo cero.

## 5. Condiciones de aceptación demostrables

- **AC-A-110-01:** Historial de decisiones y razones accesible.
- **AC-A-110-02:** Cada PR con CI y autorizar deploy separado.
- **AC-A-110-03:** Cambios a spec aprobada tienen RFC y revisión.

## 6. Trazabilidad y desbloqueo

- H04: aprobar este diseño en conjunto con otros once, sin ocultar discrepancias de fuentes.
- H05: seleccionar recursos AWS permitidos, demostrar costo cero y capacidad para DOS instancias simultáneas + LB/HTTPS + Redis sin simulaciones.
- H06: ratificar las 75 rutas con contratos y categorías M/S/E/O y su cobertura contra pantallas; incluir las operaciones académicas.
- H07: comprobar contradicciones, seguridad, aceptación y que nada indispensable se pospone a implementación por falta de diseño.
- H08: cada caso se implementa y prueba de forma real, aislada, con autorización de release.
