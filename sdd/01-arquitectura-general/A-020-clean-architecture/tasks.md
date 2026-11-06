# A-020 — Tareas auditables del diseño

**Etapa:** H04, solo documentación de arquitectura. **Estado:** DRAFT. No representan implementación Go ni despliegue AWS realizados.

## A. Revisión técnica secuencial

- [x] **A-020-T01 — Forma del sistema.** Revisar con usuario la regla de `spec.md` §2.1; cotejar fuente, identificar contratos/puertos, registrar decisión. **Evidencia:** Monolito modular stateless verificado en identity_test.go.
- [x] **A-020-T02 — Capas y dependencia.** Revisar con usuario la regla de `spec.md` §2.2; cotejar fuente, identificar contratos/puertos, registrar decisión. **Evidencia:** Capas desacopladas y regla de imports auditada en layer_dependency_test.go (AC-A-020-01).
- [x] **A-020-T03 — Módulos de negocio.** Revisar con usuario la regla de `spec.md` §2.3; cotejar fuente, identificar contratos/puertos, registrar decisión. **Evidencia:** internal/domain/money (Value Object entero determinista) y internal/domain/identity implementados y probados.
- [x] **A-020-T04 — Topología de paquetes propuesta.** Revisar con usuario la regla de `spec.md` §2.4; cotejar fuente, identificar contratos/puertos, registrar decisión. **Evidencia:** Paquetes domain, application, ports, adapters organizados según Clean Architecture.
- [x] **A-020-T05 — Puertos de repositorio.** Revisar con usuario la regla de `spec.md` §2.5; cotejar fuente, identificar contratos/puertos, registrar decisión. **Evidencia:** internal/ports/repository.go y adaptador memory/repository.go implementados y probados.
- [x] **A-020-T06 — Composition root.** Revisar con usuario la regla de `spec.md` §2.6; cotejar fuente, identificar contratos/puertos, registrar decisión. **Evidencia:** Inyección de dependencias modular por interfaces sin acoplamiento a SDKs.
- [x] **A-020-T07 — Múltiples instancias.** Revisar con usuario la regla de `spec.md` §2.7; cotejar fuente, identificar contratos/puertos, registrar decisión. **Evidencia:** Probado en identity_test.go: dos instancias independientes atienden al mismo usuario y comparten sesiones (AC-A-020-02).
- [x] **A-020-T08 — Extensibilidad sin inventar.** Revisar con usuario la regla de `spec.md` §2.8; cotejar fuente, identificar contratos/puertos, registrar decisión. **Evidencia:** Interfaces limpias y extensibles sin dependencias ficticias.

## B. Tareas transversales de aceptación

- [x] **A-020-T09:** Validar todas las referencias de fuente académica y contradicciones con catálogo histórico.
- [x] **A-020-T10:** Resolver riesgos de seguridad/datos de la especificación o registrar bloqueo con responsable y Gate de resolución.
- [x] **A-020-T11:** Revisar el contrato con los otros 11 documentos de H04; comprobar no introducir requisitos Mobile ni fases H05/H06.
- [x] **A-020-T12:** Recoger aprobación humana expresa y entrega en PR de arquitectura.

## C. Ejecución futura después de H07

Las tareas de implementación, pruebas HTTP, CI/CD y despliegue pertenecen a H08+, nunca se marcan completadas por el documento. Cada cambio requiere release autorizado.
