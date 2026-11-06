# A-040 — Checklist de arquitectura

**Estado:** IMPLEMENTED_LOCAL_H08; suite de contratos y validación de tipos Go ejecutadas con 100% PASS.

## Integridad conceptual

- [x] `constitution.md` prevalece sobre decisiones de esta spec.
- [x] No se asume que el prototipo Android realiza HTTP ni que el catálogo anterior describe implementación existente.
- [x] La arquitectura AWS+DynamoDB+Redis reemplaza exclusivamente las decisiones de plataforma históricas; conserva funcionalidad identificada.
- [x] Solo Backend; ninguna dependencia de aprobación, integración o pantalla del equipo Mobile.
- [x] No genera specs de H05/H06 ni código ni operaciones cloud.

## Aceptación específica

- [x] **AC-A-040-01:** Matrix 75/75 sin IDs duplicados (33 M, 23 S, 12 E, 7 O verificados por TestOpenAPIMatrixIntegrity).
- [x] **AC-A-040-02:** Errores 401/403/404 no revelan recursos ajenos ni permiten enumeración (verificado por TestErrorEnvelopeFormatAndNonEnumeration).
- [x] **AC-A-040-03:** Headers idempotencia (Idempotency-Key), versionado (/api/v1) y paginación (X-Cursor-Next, X-Has-More) implementados y probados.

## Revisiones de construcción

- [x] Se revisó **Contrato formal** (§2.1) y su mecanismo de aceptación con casos positivos, negativos y límites.
- [x] Se revisó **Campos y convenciones** (§2.2) y su mecanismo de aceptación con casos positivos, negativos y límites.
- [x] Se revisó **Paginación** (§2.3) y su mecanismo de aceptación con casos positivos, negativos y límites.
- [x] Se revisó **Errores** (§2.4) y su mecanismo de aceptación con casos positivos, negativos y límites.
- [x] Se revisó **HTTP methods** (§2.5) y su mecanismo de aceptación con casos positivos, negativos y límites.
- [x] Se revisó **Seguridad** (§2.6) y su mecanismo de aceptación con casos positivos, negativos y límites.
- [x] Se revisó **Compatibilidad** (§2.7) y su mecanismo de aceptación con casos positivos, negativos y límites.
- [x] Se revisó **Contract-tests** (§2.8) y su mecanismo de aceptación con casos positivos, negativos y límites.

## Estado de las evidencias

| Evidencia | Estado actual | Regla |
|---|---|---|
| Documentos `spec/plan/tasks/checklist` en paquete | GENERADOS; validación en test de ZIP | Hash/estructura comprobables |
| Revisión humana del contenido de A-040 | APROBADO H08-A | Aprobación humana expresa para implementación |
| Pruebas Go / HTTP reales | PASS (local) | Ejecutado en Windows nativo vía `go test -v ./...` |
| AWS/Redis/2 instancias/despliegue real | NOT_RUN | Se concretan H05, se ejecutan H08 cloud |
| Costo cero y permisos de cuenta AWS | PENDIENTE H05 | Bloquea H05 si no viable |
