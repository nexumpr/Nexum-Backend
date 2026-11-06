# Gate H06 — Registro histórico previo a ratificación

**Este archivo conserva la checklist original anterior a la decisión humana H08-A; no representa el estado vigente.** Estado ratificado y reservas: `sdd/_control/H08/GATE_H06_H07_H08A_RATIFICACION.md`.

# Gate H06 — Paquete de contratos para revisión y aprobación humana

**Estado:** REVIEW_READY (NO APPROVED). **Fecha de preparación:** 2026-11-05. **Quién decide:** responsable de Nexum-Backend.

## Condiciones de aceptación documental

- [ ] Los 75 métodos/rutas de H04 aparecen en la matriz y el OpenAPI 3.1.0 (33 M / 23 S / 12 E / 7 O) y su alcance se considera correcto.
- [ ] Cada ID E001–E075 tiene spec/plan/tasks/checklist completos, reglas concretas, errores y casos negativos.
- [ ] Seguridad, JWT/sesiones/consentimiento/privacidad, idempotencia y precisión de centavos están documentadas.
- [ ] DynamoDB sin Scan, Redis real compartido y dos nodos, 45 días AWS Free/IaC, compatibilidad HTTP del Backend sin aprobación móvil, están trazados.
- [ ] IA/ML genuinas y notificaciones reales conservan obligación de implementación, sin darlas por realizadas ni contratar servicios.
- [ ] El ZIP se instaló y `--verify` coincide con hashes y valida OpenAPI; el cotejo del prototipo/docente original en H07 permanece señalizado.
- [ ] Opcionales E063/E068/E069/E070/E071/E074/E075 no se activan como requisito silencioso; si la docencia los convierte en obligatorios, registrar RFC.
- [ ] No se han ejecutado AWS `apply`, creación de recursos, despliegues ni gastos.

## Decisión a emitir SOLO por humano

- **Pendiente:** `APRUEBO H06 DOCUMENTAL` / `CORREGIR H06 <ID>`.
- La aprobación **habilita preparar H07**, no GitHub push, infraestructura AWS ni código Go.
- H05 tiene aceptación documental condicionada; preflight real AWS/IaC sigue siendo obligatorio antes del deploy. H03/H04 no se reabren.

**Evidencias actuales:** scripts locales/matrices y ejemplos contractuales; HTTP, Terraform, EC2, Redis, DynamoDB, CI/CD y ML real = NOT_RUN.
