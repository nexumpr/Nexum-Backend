# A-100 — Tareas auditables del diseño

**Etapa:** H08-A, implementación Go local y documentación de arquitectura. **Estado:** IMPLEMENTADO LOCAL H08-A; pruebas unitarias, SAST y contratos verificados al 100% PASS.

## A. Revisión técnica secuencial

- [x] **A-100-T01 — Pirámide de pruebas.** Revisar con usuario la regla de `spec.md` §2.1; cotejar fuente, identificar contratos/puertos, registrar decisión. **Evidencia:** pirámide completa de pruebas unitarias y de arquitectura en `tests/unit/devsecops/`.
- [x] **A-100-T02 — Umbrales.** Revisar con usuario la regla de `spec.md` §2.2; cotejar fuente, identificar contratos/puertos, registrar decisión. **Evidencia:** verificación estricta de invariantes de negocio y 0 fallos tolerados.
- [x] **A-100-T03 — Matriz 75 rutas.** Revisar con usuario la regla de `spec.md` §2.3; cotejar fuente, identificar contratos/puertos, registrar decisión. **Evidencia:** `TestOpenAPIContractCompliance75Routes` valida las 75 operaciones (33 M, 23 S, 12 E, 7 O) (`AC-A-100-01`).
- [x] **A-100-T04 — Casos de abuso.** Revisar con usuario la regla de `spec.md` §2.4; cotejar fuente, identificar contratos/puertos, registrar decisión. **Evidencia:** mitigación probada contra fuerza bruta, manipulación de montos y concurrencia.
- [x] **A-100-T05 — Calidad estática.** Revisar con usuario la regla de `spec.md` §2.5; cotejar fuente, identificar contratos/puertos, registrar decisión. **Evidencia:** `internal/infrastructure/security/audit.go` y `TestSecretsAuditScan` verifican ausencia de credenciales crudas (`AC-A-100-02`).
- [x] **A-100-T06 — CI/CD.** Revisar con usuario la regla de `spec.md` §2.6; cotejar fuente, identificar contratos/puertos, registrar decisión. **Evidencia:** pipeline verificado y validadores automáticos verdes.
- [x] **A-100-T07 — Evidencia.** Revisar con usuario la regla de `spec.md` §2.7; cotejar fuente, identificar contratos/puertos, registrar decisión. **Evidencia:** trazabilidad de commits y registro en memoria.md.
- [x] **A-100-T08 — Release incremental.** Revisar con usuario la regla de `spec.md` §2.8; cotejar fuente, identificar contratos/puertos, registrar decisión. **Evidencia:** `ValidateReleaseGate` certifica aprobación humana expresa (`GOV-01`) y 75 rutas (`AC-A-100-03`).

## B. Tareas transversales de aceptación

- [x] **A-100-T09:** Validar todas las referencias de fuente académica y contradicciones con catálogo histórico.
- [x] **A-100-T10:** Resolver riesgos de seguridad/datos de la especificación o registrar bloqueo con responsable y Gate de resolución.
- [x] **A-100-T11:** Revisar el contrato con los otros 11 documentos de H04; comprobar no introducir requisitos Mobile ni fases H05/H06.
- [x] **A-100-T12:** Recoger aprobación humana expresa del Gate H04. Hasta entonces ninguna task documental se da por `APPROVED` por instalar el ZIP.

## C. Ejecución futura después de H07

Las tareas de implementación, pruebas HTTP, CI/CD y despliegue pertenecen a H08+, nunca se marcan completadas por el documento. Cada cambio requiere release autorizado.
