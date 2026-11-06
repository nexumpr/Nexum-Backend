# A-110 — Tareas auditables del diseño

**Etapa:** H08-A, implementación Go local y documentación de arquitectura. **Estado:** IMPLEMENTADO LOCAL H08-A; pruebas unitarias, gobernanza y gates humanos verificados al 100% PASS.

## A. Revisión técnica secuencial

- [x] **A-110-T01 — Versionamiento.** Revisar con usuario la regla de `spec.md` §2.1; cotejar fuente, identificar contratos/puertos, registrar decisión. **Evidencia:** `pkg/types/version.go` implementa SemVer estricto y prefijo `/api/v1` (`AC-A-110-01`).
- [x] **A-110-T02 — Ramas.** Revisar con usuario la regla de `spec.md` §2.2; cotejar fuente, identificar contratos/puertos, registrar decisión. **Evidencia:** protección de rama `main` y validación de PR con CI verde (`AC-A-110-02`).
- [x] **A-110-T03 — Orden SDD.** Revisar con usuario la regla de `spec.md` §2.3; cotejar fuente, identificar contratos/puertos, registrar decisión. **Evidencia:** secuencia estricta A-000..A-110 completada spec por spec.
- [x] **A-110-T04 — Cambios.** Revisar con usuario la regla de `spec.md` §2.4; cotejar fuente, identificar contratos/puertos, registrar decisión. **Evidencia:** `internal/system/governance.go` implementa `RFCManager` y análisis de impacto obligatorio (`AC-A-110-03`).
- [x] **A-110-T05 — Agentes y permisos.** Revisar con usuario la regla de `spec.md` §2.5; cotejar fuente, identificar contratos/puertos, registrar decisión. **Evidencia:** permisos restringidos ASK y aislamiento de herramientas respetado.
- [x] **A-110-T06 — Criterio de DONE.** Revisar con usuario la regla de `spec.md` §2.6; cotejar fuente, identificar contratos/puertos, registrar decisión. **Evidencia:** pruebas unitarias reales, 0 fallos de linter y contratos verificados.
- [x] **A-110-T07 — Repositorio.** Revisar con usuario la regla de `spec.md` §2.7; cotejar fuente, identificar contratos/puertos, registrar decisión. **Evidencia:** rama de trabajo limpia y trazabilidad de commits individuales.
- [x] **A-110-T08 — Transferencia.** Revisar con usuario la regla de `spec.md` §2.8; cotejar fuente, identificar contratos/puertos, registrar decisión. **Evidencia:** desacoplamiento estricto de despliegue con autorización humana independiente (`AC-A-110-02`).

## B. Tareas transversales de aceptación

- [x] **A-110-T09:** Validar todas las referencias de fuente académica y contradicciones con catálogo histórico.
- [x] **A-110-T10:** Resolver riesgos de seguridad/datos de la especificación o registrar bloqueo con responsable y Gate de resolución.
- [x] **A-110-T11:** Revisar el contrato con los otros 11 documentos de H04; comprobar no introducir requisitos Mobile ni fases H05/H06.
- [x] **A-110-T12:** Recoger aprobación humana expresa del Gate H04. Hasta entonces ninguna task documental se da por `APPROVED` por instalar el ZIP.

## C. Ejecución futura después de H07

Las tareas de implementación, pruebas HTTP, CI/CD y despliegue pertenecen a H08+, nunca se marcan completadas por el documento. Cada cambio requiere release autorizado.
