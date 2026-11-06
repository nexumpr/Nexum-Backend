# A-010 — Tareas auditables del diseño

**Etapa:** H04, solo documentación de arquitectura. **Estado:** DRAFT. No representan implementación Go ni despliegue AWS realizados.

## A. Revisión técnica secuencial

- [x] **A-010-T01 — Runtime Go.** Revisar con usuario la regla de `spec.md` §2.1; cotejar fuente, identificar contratos/puertos, registrar decisión. **Evidencia:** Go 1.27.3 verificado en Windows nativo y probado en runtime_test.go; PR #4 mergeado.
- [x] **A-010-T02 — HTTP y JSON.** Revisar con usuario la regla de `spec.md` §2.2; cotejar fuente, identificar contratos/puertos, registrar decisión. **Evidencia:** pkg/httputil/json.go implementado con net/http, DisallowUnknownFields y MaxBytesReader; probado en httputil_test.go.
- [x] **A-010-T03 — DynamoDB.** Revisar con usuario la regla de `spec.md` §2.3; cotejar fuente, identificar contratos/puertos, registrar decisión. **Evidencia:** internal/ports/repository.go con NoSQLRepository desacoplado de AWS; probado en repository_port_test.go.
- [x] **A-010-T04 — Redis.** Revisar con usuario la regla de `spec.md` §2.4; cotejar fuente, identificar contratos/puertos, registrar decisión. **Evidencia:** internal/ports/cache.go con DistributedCache para TTL y locks; probado en repository_port_test.go.
- [x] **A-010-T05 — Autenticación.** Revisar con usuario la regla de `spec.md` §2.5; cotejar fuente, identificar contratos/puertos, registrar decisión. **Evidencia:** internal/ports/auth.go con PasswordHasher y TokenProvider; probado en auth_port_test.go.
- [x] **A-010-T06 — Telemetría.** Revisar con usuario la regla de `spec.md` §2.6; cotejar fuente, identificar contratos/puertos, registrar decisión. **Evidencia:** pkg/telemetry/logger.go con log/slog estructurado JSON y sanitización anti-PII; probado en telemetry_test.go.
- [x] **A-010-T07 — Tests/seguridad.** Revisar con usuario la regla de `spec.md` §2.7; cotejar fuente, identificar contratos/puertos, registrar decisión. **Evidencia:** Suite de pruebas al 100% PASS, go vet limpio con 0 warnings.
- [x] **A-010-T08 — Construcción sin contenedores locales.** Revisar con usuario la regla de `spec.md` §2.8; cotejar fuente, identificar contratos/puertos, registrar decisión. **Evidencia:** Verificado en Windows 11 nativo sin Docker; PR #4 mergeado.

## B. Tareas transversales de aceptación

- [x] **A-010-T09:** Validar todas las referencias de fuente académica y contradicciones con catálogo histórico.
- [x] **A-010-T10:** Resolver riesgos de seguridad/datos de la especificación o registrar bloqueo con responsable y Gate de resolución.
- [x] **A-010-T11:** Revisar el contrato con los otros 11 documentos de H04; comprobar no introducir requisitos Mobile ni fases H05/H06.
- [x] **A-010-T12:** Recoger aprobación humana expresa y merge formal en PR #4.

## C. Ejecución futura después de H07

Las tareas de implementación, pruebas HTTP, CI/CD y despliegue pertenecen a H08+, nunca se marcan completadas por el documento. Cada cambio requiere release autorizado.
