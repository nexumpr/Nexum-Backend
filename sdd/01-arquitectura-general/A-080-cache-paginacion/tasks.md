# A-080 — Tareas auditables del diseño

**Etapa:** H04, solo documentación de arquitectura. **Estado:** DRAFT. No representan implementación Go ni despliegue AWS realizados.

## A. Revisión técnica secuencial

- [ ] **A-080-T01 — Rol Redis.** Revisar con usuario la regla de `spec.md` §2.1; cotejar fuente, identificar contratos/puertos, registrar decisión. **Evidencia:** referencia y aceptación de AC correspondiente; estado DRAFT hasta Gate H04.
- [ ] **A-080-T02 — Keys y seguridad.** Revisar con usuario la regla de `spec.md` §2.2; cotejar fuente, identificar contratos/puertos, registrar decisión. **Evidencia:** referencia y aceptación de AC correspondiente; estado DRAFT hasta Gate H04.
- [ ] **A-080-T03 — Cachés elegibles.** Revisar con usuario la regla de `spec.md` §2.3; cotejar fuente, identificar contratos/puertos, registrar decisión. **Evidencia:** referencia y aceptación de AC correspondiente; estado DRAFT hasta Gate H04.
- [ ] **A-080-T04 — Invalidación.** Revisar con usuario la regla de `spec.md` §2.4; cotejar fuente, identificar contratos/puertos, registrar decisión. **Evidencia:** referencia y aceptación de AC correspondiente; estado DRAFT hasta Gate H04.
- [ ] **A-080-T05 — Rate limiting.** Revisar con usuario la regla de `spec.md` §2.5; cotejar fuente, identificar contratos/puertos, registrar decisión. **Evidencia:** referencia y aceptación de AC correspondiente; estado DRAFT hasta Gate H04.
- [ ] **A-080-T06 — Paginación cursor.** Revisar con usuario la regla de `spec.md` §2.6; cotejar fuente, identificar contratos/puertos, registrar decisión. **Evidencia:** referencia y aceptación de AC correspondiente; estado DRAFT hasta Gate H04.
- [ ] **A-080-T07 — Consistencia de agregados.** Revisar con usuario la regla de `spec.md` §2.7; cotejar fuente, identificar contratos/puertos, registrar decisión. **Evidencia:** referencia y aceptación de AC correspondiente; estado DRAFT hasta Gate H04.
- [ ] **A-080-T08 — Medición.** Revisar con usuario la regla de `spec.md` §2.8; cotejar fuente, identificar contratos/puertos, registrar decisión. **Evidencia:** referencia y aceptación de AC correspondiente; estado DRAFT hasta Gate H04.

## B. Tareas transversales de aceptación

- [ ] **A-080-T09:** Validar todas las referencias de fuente académica y contradicciones con catálogo histórico.
- [ ] **A-080-T10:** Resolver riesgos de seguridad/datos de la especificación o registrar bloqueo con responsable y Gate de resolución.
- [ ] **A-080-T11:** Revisar el contrato con los otros 11 documentos de H04; comprobar no introducir requisitos Mobile ni fases H05/H06.
- [ ] **A-080-T12:** Recoger aprobación humana expresa del Gate H04. Hasta entonces ninguna task documental se da por `APPROVED` por instalar el ZIP.

## C. Ejecución futura después de H07

Las tareas de implementación, pruebas HTTP, CI/CD y despliegue pertenecen a H08+, nunca se marcan completadas por el documento. Cada cambio requiere release autorizado.
