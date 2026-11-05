# A-020 — Tareas auditables del diseño

**Etapa:** H04, solo documentación de arquitectura. **Estado:** DRAFT. No representan implementación Go ni despliegue AWS realizados.

## A. Revisión técnica secuencial

- [ ] **A-020-T01 — Forma del sistema.** Revisar con usuario la regla de `spec.md` §2.1; cotejar fuente, identificar contratos/puertos, registrar decisión. **Evidencia:** referencia y aceptación de AC correspondiente; estado DRAFT hasta Gate H04.
- [ ] **A-020-T02 — Capas y dependencia.** Revisar con usuario la regla de `spec.md` §2.2; cotejar fuente, identificar contratos/puertos, registrar decisión. **Evidencia:** referencia y aceptación de AC correspondiente; estado DRAFT hasta Gate H04.
- [ ] **A-020-T03 — Módulos de negocio.** Revisar con usuario la regla de `spec.md` §2.3; cotejar fuente, identificar contratos/puertos, registrar decisión. **Evidencia:** referencia y aceptación de AC correspondiente; estado DRAFT hasta Gate H04.
- [ ] **A-020-T04 — Topología de paquetes propuesta.** Revisar con usuario la regla de `spec.md` §2.4; cotejar fuente, identificar contratos/puertos, registrar decisión. **Evidencia:** referencia y aceptación de AC correspondiente; estado DRAFT hasta Gate H04.
- [ ] **A-020-T05 — Puertos de repositorio.** Revisar con usuario la regla de `spec.md` §2.5; cotejar fuente, identificar contratos/puertos, registrar decisión. **Evidencia:** referencia y aceptación de AC correspondiente; estado DRAFT hasta Gate H04.
- [ ] **A-020-T06 — Composition root.** Revisar con usuario la regla de `spec.md` §2.6; cotejar fuente, identificar contratos/puertos, registrar decisión. **Evidencia:** referencia y aceptación de AC correspondiente; estado DRAFT hasta Gate H04.
- [ ] **A-020-T07 — Múltiples instancias.** Revisar con usuario la regla de `spec.md` §2.7; cotejar fuente, identificar contratos/puertos, registrar decisión. **Evidencia:** referencia y aceptación de AC correspondiente; estado DRAFT hasta Gate H04.
- [ ] **A-020-T08 — Extensibilidad sin inventar.** Revisar con usuario la regla de `spec.md` §2.8; cotejar fuente, identificar contratos/puertos, registrar decisión. **Evidencia:** referencia y aceptación de AC correspondiente; estado DRAFT hasta Gate H04.

## B. Tareas transversales de aceptación

- [ ] **A-020-T09:** Validar todas las referencias de fuente académica y contradicciones con catálogo histórico.
- [ ] **A-020-T10:** Resolver riesgos de seguridad/datos de la especificación o registrar bloqueo con responsable y Gate de resolución.
- [ ] **A-020-T11:** Revisar el contrato con los otros 11 documentos de H04; comprobar no introducir requisitos Mobile ni fases H05/H06.
- [ ] **A-020-T12:** Recoger aprobación humana expresa del Gate H04. Hasta entonces ninguna task documental se da por `APPROVED` por instalar el ZIP.

## C. Ejecución futura después de H07

Las tareas de implementación, pruebas HTTP, CI/CD y despliegue pertenecen a H08+, nunca se marcan completadas por el documento. Cada cambio requiere release autorizado.
