# A-010 — Tareas auditables del diseño

**Etapa:** H04, solo documentación de arquitectura. **Estado:** DRAFT. No representan implementación Go ni despliegue AWS realizados.

## A. Revisión técnica secuencial

- [ ] **A-010-T01 — Runtime Go.** Revisar con usuario la regla de `spec.md` §2.1; cotejar fuente, identificar contratos/puertos, registrar decisión. **Evidencia:** referencia y aceptación de AC correspondiente; estado DRAFT hasta Gate H04.
- [ ] **A-010-T02 — HTTP y JSON.** Revisar con usuario la regla de `spec.md` §2.2; cotejar fuente, identificar contratos/puertos, registrar decisión. **Evidencia:** referencia y aceptación de AC correspondiente; estado DRAFT hasta Gate H04.
- [ ] **A-010-T03 — DynamoDB.** Revisar con usuario la regla de `spec.md` §2.3; cotejar fuente, identificar contratos/puertos, registrar decisión. **Evidencia:** referencia y aceptación de AC correspondiente; estado DRAFT hasta Gate H04.
- [ ] **A-010-T04 — Redis.** Revisar con usuario la regla de `spec.md` §2.4; cotejar fuente, identificar contratos/puertos, registrar decisión. **Evidencia:** referencia y aceptación de AC correspondiente; estado DRAFT hasta Gate H04.
- [ ] **A-010-T05 — Autenticación.** Revisar con usuario la regla de `spec.md` §2.5; cotejar fuente, identificar contratos/puertos, registrar decisión. **Evidencia:** referencia y aceptación de AC correspondiente; estado DRAFT hasta Gate H04.
- [ ] **A-010-T06 — Telemetría.** Revisar con usuario la regla de `spec.md` §2.6; cotejar fuente, identificar contratos/puertos, registrar decisión. **Evidencia:** referencia y aceptación de AC correspondiente; estado DRAFT hasta Gate H04.
- [ ] **A-010-T07 — Tests/seguridad.** Revisar con usuario la regla de `spec.md` §2.7; cotejar fuente, identificar contratos/puertos, registrar decisión. **Evidencia:** referencia y aceptación de AC correspondiente; estado DRAFT hasta Gate H04.
- [ ] **A-010-T08 — Construcción sin contenedores locales.** Revisar con usuario la regla de `spec.md` §2.8; cotejar fuente, identificar contratos/puertos, registrar decisión. **Evidencia:** referencia y aceptación de AC correspondiente; estado DRAFT hasta Gate H04.

## B. Tareas transversales de aceptación

- [ ] **A-010-T09:** Validar todas las referencias de fuente académica y contradicciones con catálogo histórico.
- [ ] **A-010-T10:** Resolver riesgos de seguridad/datos de la especificación o registrar bloqueo con responsable y Gate de resolución.
- [ ] **A-010-T11:** Revisar el contrato con los otros 11 documentos de H04; comprobar no introducir requisitos Mobile ni fases H05/H06.
- [ ] **A-010-T12:** Recoger aprobación humana expresa del Gate H04. Hasta entonces ninguna task documental se da por `APPROVED` por instalar el ZIP.

## C. Ejecución futura después de H07

Las tareas de implementación, pruebas HTTP, CI/CD y despliegue pertenecen a H08+, nunca se marcan completadas por el documento. Cada cambio requiere release autorizado.
