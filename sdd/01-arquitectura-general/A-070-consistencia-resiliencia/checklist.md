# A-070 — Checklist de arquitectura

**Estado:** IMPLEMENTED_LOCAL_H08; split equitativo en centavos, anti-doble descuento de fijos, reversión e idempotencia ejecutadas con 100% PASS.

## Integridad conceptual

- [x] `constitution.md` prevalece sobre decisiones de esta spec.
- [x] No se asume que el prototipo Android realiza HTTP ni que el catálogo anterior describe implementación existente.
- [x] La arquitectura AWS+DynamoDB+Redis reemplaza exclusivamente las decisiones de plataforma históricas; conserva funcionalidad identificada.
- [x] Solo Backend; ninguna dependencia de aprobación, integración o pantalla del equipo Mobile.
- [x] No genera specs de H05/H06 ni código ni operaciones cloud.

## Aceptación específica

- [x] **AC-A-070-01:** Casos centavos 0.01, 0.02/3, saturación meta, retries 100 veces sin divergencia (verificado por TestEquitableSplitCentsAndSaturation).
- [x] **AC-A-070-02:** Pago recurring fixed no disminuye doble (verificado por TestAvailableCalculationNoDoubleDeduction).
- [x] **AC-A-070-03:** Reversión preserva invariante `saved=opening+allocations activas` (verificado por TestGoalReversionPreservesInvariant).

## Revisiones de construcción

- [x] Se revisó **Dinero** (§2.1) y su mecanismo de aceptación con casos positivos, negativos y límites.
- [x] Se revisó **Equivalencia mensual** (§2.2) y su mecanismo de aceptación con casos positivos, negativos y límites.
- [x] Se revisó **Disponible V6** (§2.3) y su mecanismo de aceptación con casos positivos, negativos y límites.
- [x] Se revisó **Metas** (§2.4) y su mecanismo de aceptación con casos positivos, negativos y límites.
- [x] Se revisó **Split igual y personalizado** (§2.5) y su mecanismo de aceptación con casos positivos, negativos y límites.
- [x] Se revisó **Reversión** (§2.6) y su mecanismo de aceptación con casos positivos, negativos y límites.
- [x] Se revisó **Idempotencia** (§2.7) y su mecanismo de aceptación con casos positivos, negativos y límites.
- [x] Se revisó **Resiliencia** (§2.8) y su mecanismo de aceptación con casos positivos, negativos y límites.

## Estado de las evidencias

| Evidencia | Estado actual | Regla |
|---|---|---|
| Documentos `spec/plan/tasks/checklist` en paquete | GENERADOS; validación en test de ZIP | Hash/estructura comprobables |
| Revisión humana del contenido de A-070 | APROBADO H08-A | Aprobación humana expresa para implementación |
| Pruebas Go / HTTP reales | PASS (local) | Ejecutado en Windows nativo vía `go test -v ./...` |
| AWS/Redis/2 instancias/despliegue real | NOT_RUN | Se concretan H05, se ejecutan H08 cloud |
| Costo cero y permisos de cuenta AWS | PENDIENTE H05 | Bloquea H05 si no viable |
