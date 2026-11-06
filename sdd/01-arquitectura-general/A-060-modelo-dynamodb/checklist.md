# A-060 — Checklist de arquitectura

**Estado:** IMPLEMENTED_LOCAL_H08; Single-Table Design, prohibición de Scan, transacciones atómicas e índices GSI1 ejecutados con 100% PASS.

## Integridad conceptual

- [x] `constitution.md` prevalece sobre decisiones de esta spec.
- [x] No se asume que el prototipo Android realiza HTTP ni que el catálogo anterior describe implementación existente.
- [x] La arquitectura AWS+DynamoDB+Redis reemplaza exclusivamente las decisiones de plataforma históricas; conserva funcionalidad identificada.
- [x] Solo Backend; ninguna dependencia de aprobación, integración o pantalla del equipo Mobile.
- [x] No genera specs de H05/H06 ni código ni operaciones cloud.

## Aceptación específica

- [x] **AC-A-060-01:** Nunca Scan; solo Query o GetItem/BatchGetItem con índices justificados (verificado por TestProhibitionOfScan).
- [x] **AC-A-060-02:** Transacción con falla de condición no deja goal.saved sin movement (verificado por TestAtomicTransactionRollback).
- [x] **AC-A-060-03:** Diseño de índices cubre los 75 IDs incluidas rutas O como opciones no obligatorias (verificado por TestEndpointAccessPatternsCoverage).

## Revisiones de construcción

- [x] Se revisó **Tablas y modo** (§2.1) y su mecanismo de aceptación con casos positivos, negativos y límites.
- [x] Se revisó **Identidad global** (§2.2) y su mecanismo de aceptación con casos positivos, negativos y límites.
- [x] Se revisó **Partición usuario** (§2.3) y su mecanismo de aceptación con casos positivos, negativos y límites.
- [x] Se revisó **Índices y acceso** (§2.4) y su mecanismo de aceptación con casos positivos, negativos y límites.
- [x] Se revisó **Movimientos e integridad** (§2.5) y su mecanismo de aceptación con casos positivos, negativos y límites.
- [x] Se revisó **Aportación compuesta** (§2.6) y su mecanismo de aceptación con casos positivos, negativos y límites.
- [x] Se revisó **Límites y concurrencia** (§2.7) y su mecanismo de aceptación con casos positivos, negativos y límites.
- [x] Se revisó **TTL y eliminación** (§2.8) y su mecanismo de aceptación con casos positivos, negativos y límites.
- [x] Se revisó **Patrones de acceso obligatorios** (§2.9) y su mecanismo de aceptación con casos positivos, negativos y límites.
- [x] Se revisó **Validación de diseño** (§2.10) y su mecanismo de aceptación con casos positivos, negativos y límites.

## Estado de las evidencias

| Evidencia | Estado actual | Regla |
|---|---|---|
| Documentos `spec/plan/tasks/checklist` en paquete | GENERADOS; validación en test de ZIP | Hash/estructura comprobables |
| Revisión humana del contenido de A-060 | APROBADO H08-A | Aprobación humana expresa para implementación |
| Pruebas Go / HTTP reales | PASS (local) | Ejecutado en Windows nativo vía `go test -v ./...` |
| AWS/Redis/2 instancias/despliegue real | NOT_RUN | Se concretan H05, se ejecutan H08 cloud |
| Costo cero y permisos de cuenta AWS | PENDIENTE H05 | Bloquea H05 si no viable |
