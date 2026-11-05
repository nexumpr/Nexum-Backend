# A-100 — Tareas auditables del diseño

**Etapa:** H04, solo documentación de arquitectura. **Estado:** DRAFT. No representan implementación Go ni despliegue AWS realizados.

## A. Revisión técnica secuencial

- [ ] **A-100-T01 — Pirámide de pruebas.** Revisar con usuario la regla de `spec.md` §2.1; cotejar fuente, identificar contratos/puertos, registrar decisión. **Evidencia:** referencia y aceptación de AC correspondiente; estado DRAFT hasta Gate H04.
- [ ] **A-100-T02 — Umbrales.** Revisar con usuario la regla de `spec.md` §2.2; cotejar fuente, identificar contratos/puertos, registrar decisión. **Evidencia:** referencia y aceptación de AC correspondiente; estado DRAFT hasta Gate H04.
- [ ] **A-100-T03 — Matriz 75 rutas.** Revisar con usuario la regla de `spec.md` §2.3; cotejar fuente, identificar contratos/puertos, registrar decisión. **Evidencia:** referencia y aceptación de AC correspondiente; estado DRAFT hasta Gate H04.
- [ ] **A-100-T04 — Casos de abuso.** Revisar con usuario la regla de `spec.md` §2.4; cotejar fuente, identificar contratos/puertos, registrar decisión. **Evidencia:** referencia y aceptación de AC correspondiente; estado DRAFT hasta Gate H04.
- [ ] **A-100-T05 — Calidad estática.** Revisar con usuario la regla de `spec.md` §2.5; cotejar fuente, identificar contratos/puertos, registrar decisión. **Evidencia:** referencia y aceptación de AC correspondiente; estado DRAFT hasta Gate H04.
- [ ] **A-100-T06 — CI/CD.** Revisar con usuario la regla de `spec.md` §2.6; cotejar fuente, identificar contratos/puertos, registrar decisión. **Evidencia:** referencia y aceptación de AC correspondiente; estado DRAFT hasta Gate H04.
- [ ] **A-100-T07 — Evidencia.** Revisar con usuario la regla de `spec.md` §2.7; cotejar fuente, identificar contratos/puertos, registrar decisión. **Evidencia:** referencia y aceptación de AC correspondiente; estado DRAFT hasta Gate H04.
- [ ] **A-100-T08 — Release incremental.** Revisar con usuario la regla de `spec.md` §2.8; cotejar fuente, identificar contratos/puertos, registrar decisión. **Evidencia:** referencia y aceptación de AC correspondiente; estado DRAFT hasta Gate H04.

## B. Tareas transversales de aceptación

- [ ] **A-100-T09:** Validar todas las referencias de fuente académica y contradicciones con catálogo histórico.
- [ ] **A-100-T10:** Resolver riesgos de seguridad/datos de la especificación o registrar bloqueo con responsable y Gate de resolución.
- [ ] **A-100-T11:** Revisar el contrato con los otros 11 documentos de H04; comprobar no introducir requisitos Mobile ni fases H05/H06.
- [ ] **A-100-T12:** Recoger aprobación humana expresa del Gate H04. Hasta entonces ninguna task documental se da por `APPROVED` por instalar el ZIP.

## C. Ejecución futura después de H07

Las tareas de implementación, pruebas HTTP, CI/CD y despliegue pertenecen a H08+, nunca se marcan completadas por el documento. Cada cambio requiere release autorizado.
