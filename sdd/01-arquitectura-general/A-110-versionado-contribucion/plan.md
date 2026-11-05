# A-110 — Plan técnico de arquitectura

**Estado:** DRAFT H04. **Entrada obligatoria:** `spec.md` y fuentes de A-110. **Salida:** decisión lista para implementar después de H07; este plan NO ejecuta infraestructura ni código.

## 1. Estrategia de construcción

1. **Versionamiento.** Definir interfaces, límites, precondiciones, postcondiciones, fallos, autorización y pruebas correspondientes. Matriz de versiones por componente independiente: backend SemVer, API path /api/v1, SDD doc version y release AWS; etiqueta `vMAJOR.MINOR.PATCH` solo en código liberado con changelog y hash verificable.
2. **Ramas.** Definir interfaces, límites, precondiciones, postcondiciones, fallos, autorización y pruebas correspondientes. main protegida; feature/<id> -> PR -> checks -> aprobación -> merge; release/ cuando se necesite corte; no push automático a main ni credenciales personales en workflow.
3. **Orden SDD.** Definir interfaces, límites, precondiciones, postcondiciones, fallos, autorización y pruebas correspondientes. H03 APPROVED, H04 completo DRAFT -> revisión técnica por spec -> Gate H04 humano; H05 (16 specs) solo tras H04; H06 catálogo/endpoints tras H05; H07 audita/pública SDD; H08 implementación una por una.
4. **Cambios.** Definir interfaces, límites, precondiciones, postcondiciones, fallos, autorización y pruebas correspondientes. Cualquier alteración a elección Go/AWS/Dynamo/Redis/2 instancias/finanzas abre RFC y análisis de impacto referencias E-ID, costes y pruebas; aprobador humano antes de editar constitución o spec APPROVED.
5. **Agentes y permisos.** Definir interfaces, límites, precondiciones, postcondiciones, fallos, autorización y pruebas correspondientes. Ocho agentes y nueve skills subordinados a constitution; opencode.json `shell/subagent/skill deny` persistente durante H04. Solo habilitar edit a directorio autorizado bajo control formal por fase, nunca ALL tools/auto.
6. **Criterio de DONE.** Definir interfaces, límites, precondiciones, postcondiciones, fallos, autorización y pruebas correspondientes. Spec/plan/tasks/checklist con aceptación trazada; durante H08 DONE exige implementación real y HTTP, seguridad, idempotencia, QA, registros, CI, autorización humana, despliegue y evidencia.
7. **Repositorio.** Definir interfaces, límites, precondiciones, postcondiciones, fallos, autorización y pruebas correspondientes. Documentación SDD Nexum permitida conforme decisión propia de Nexum, no heredar política de GradeX. Respaldo antes de reset o cualquier destrucción, no operar git remotes ni AWS sin autorización explícita.
8. **Transferencia.** Definir interfaces, límites, precondiciones, postcondiciones, fallos, autorización y pruebas correspondientes. Cada entrega ZIP SHA256 manifiesto, instalador idempotente de solo carpeta autorizada, preflight, backup, rollback verificado y diagnóstico no falsos PASS. PowerShell 7 Warp Windows 11.

## 2. Orden interno de diseño y dependencias

1. Revisar `constitution.md` y `AGENTS.md` antes de aceptar cambios; cotejar los PDF/prototipo cuyos nombres y hashes se registran en H04.
2. Identificar invariantes y entradas/salidas de cada módulo; mapear al catálogo E001..075 sin crear contratos endpoints H06.
3. Verificar acoplamiento con A-020 (capas), A-040 (DTOs), A-050 (auth), A-060 (persistencia), A-070 (consistencia), A-080 (Redis), A-090 (telemetría) y A-100 (QA).
4. Mantener como *blocker de H05* capacidad/costo de 2 instancias + balanceador + Redis, restricciones SCP/IAM, TLS y mecanismos de on/off; no inferir disponibilidad de AWS Free.
5. Registrar la decisión humana y conflictos si alguna regla altera el negocio del prototipo; no solicitar gate Mobile.

## 3. Estrategia de pruebas posteriores por capa

| Nivel | Diseño a comprobar | Evidencia futura obligatoria |
|---|---|---|
| Dominio | Invariantes y money/identidad | go test, race detector, entradas límite |
| Aplicación | Orquestación con mocks y transacciones | unit tests, casos negativos, retries |
| Infraestructura | DynamoDB/Redis reales según fase | integración, latencia, permisos mínimos, gasto |
| HTTP | Schemas OpenAPI 3.1 y respuestas | pruebas contractuales y HTTP reales |
| CI/CD | Seguridad y releases | PR checks, escaneo, autorización humana |

## 4. Riesgos y mitigación explícita

- No confundir prueba local documental con despliegue real. **Mitigación:** resolver antes de aprobar su dependencia; si requiere cambio de constitución emitir RFC.
- La presión de tiempo no permite saltar gates ni falsear AWS costo cero. **Mitigación:** resolver antes de aprobar su dependencia; si requiere cambio de constitución emitir RFC.

## 5. Criterio de cierre

Se considera listo para revisión cuando los 8 ejes técnicos de `spec.md` tienen trazabilidad a pruebas en `checklist.md`, los elementos futuros permanecen `NOT_RUN`, la matriz de alcance está intacta y las decisiones pendientes tienen dueño/fase, sin `APPROVED` automático.
