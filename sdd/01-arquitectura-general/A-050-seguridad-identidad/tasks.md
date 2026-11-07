# A-050 — Tareas auditables del diseño

**Etapa:** H04, solo documentación de arquitectura. **Estado:** DRAFT. No representan implementación Go ni despliegue AWS realizados.

## A. Revisión técnica secuencial

- [x] **A-050-T01 — Registro e identidad.** Revisar con usuario la regla de `spec.md` §2.1; cotejar fuente, identificar contratos/puertos, registrar decisión. **Evidencia:** referencia y aceptación de AC correspondiente; estado DRAFT hasta Gate H04.
- [x] **A-050-T02 — JWT.** Revisar con usuario la regla de `spec.md` §2.2; cotejar fuente, identificar contratos/puertos, registrar decisión. **Evidencia:** referencia y aceptación de AC correspondiente; estado DRAFT hasta Gate H04.
- [x] **A-050-T03 — Refresh.** Revisar con usuario la regla de `spec.md` §2.3; cotejar fuente, identificar contratos/puertos, registrar decisión. **Evidencia:** referencia y aceptación de AC correspondiente; estado DRAFT hasta Gate H04.
- [x] **A-050-T04 — Logout y revocación.** Revisar con usuario la regla de `spec.md` §2.4; cotejar fuente, identificar contratos/puertos, registrar decisión. **Evidencia:** referencia y aceptación de AC correspondiente; estado DRAFT hasta Gate H04.
- [x] **A-050-T05 — Recuperación y correo.** Revisar con usuario la regla de `spec.md` §2.5; cotejar fuente, identificar contratos/puertos, registrar decisión. **Evidencia:** referencia y aceptación de AC correspondiente; estado DRAFT hasta Gate H04.
- [x] **A-050-T06 — Autorización de recursos.** Revisar con usuario la regla de `spec.md` §2.6; cotejar fuente, identificar contratos/puertos, registrar decisión. **Evidencia:** referencia y aceptación de AC correspondiente; estado DRAFT hasta Gate H04.
- [x] **A-050-T07 — Cuenta y privacidad.** Revisar con usuario la regla de `spec.md` §2.7; cotejar fuente, identificar contratos/puertos, registrar decisión. **Evidencia:** referencia y aceptación de AC correspondiente; estado DRAFT hasta Gate H04.
- [x] **A-050-T08 — Secretos y auditoría.** Revisar con usuario la regla de `spec.md` §2.8; cotejar fuente, identificar contratos/puertos, registrar decisión. **Evidencia:** referencia y aceptación de AC correspondiente; estado DRAFT hasta Gate H04.

## B. Tareas transversales de aceptación

- [x] **A-050-T09:** Validar todas las referencias de fuente académica y contradicciones con catálogo histórico.
- [x] **A-050-T10:** Resolver riesgos de seguridad/datos de la especificación o registrar bloqueo con responsable y Gate de resolución.
- [x] **A-050-T11:** Revisar el contrato con los otros 11 documentos de H04; comprobar no introducir requisitos Mobile ni fases H05/H06.
- [x] **A-050-T12:** Recoger aprobación humana expresa del Gate H04. Hasta entonces ninguna task documental se da por `APPROVED` por instalar el ZIP.

## C. Ejecución futura después de H07

Las tareas de implementación, pruebas HTTP, CI/CD y despliegue pertenecen a H08+, nunca se marcan completadas por el documento. Cada cambio requiere release autorizado.

## Hotfix H08-A/A-050 — D-10 (2026-11-06)

- Gate `sdd/_control/H08/GATE_H08A_HOTFIX_A050_G115.md` (frase `APRUEBO_FIX_A050_GOSEC_G115`).
- `VerifyPassword` valida politica y longitudes ANTES de `argon2.IDKey`: memoria 8-256 MiB, iteraciones 1-10, paralelismo 1-16, sal 8-64 B, digest en {16,24,32,48,64} y hash codificado <= 4096 caracteres.
- La longitud del digest se toma de constantes uint32: se elimina la conversion int->uint32 (G115) sin `#nosec` ni exclusiones.
- Sin cambios de algoritmo, formato ni parametros por defecto; comparacion en tiempo constante intacta.
- Pruebas nuevas en `tests/unit/security/hasher_hardening_test.go` (7 grupos, incluido rechazo en <250 ms de parametros peligrosos).
- gosec PASS (0 hallazgos), govulncheck PASS, go build/vet/test PASS.
