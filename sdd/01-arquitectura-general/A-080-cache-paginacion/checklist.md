# A-080 — Checklist de arquitectura

**Estado:** IMPLEMENTADO LOCAL H08-A; pruebas unitarias y de arquitectura Go PASS (`AC-A-080-01`, `AC-A-080-02`, `AC-A-080-03`).

## Integridad conceptual

- [x] `constitution.md` prevalece sobre decisiones de esta spec.
- [x] No se asume que el prototipo Android realiza HTTP ni que el catálogo anterior describe implementación existente.
- [x] La arquitectura AWS+DynamoDB+Redis reemplaza exclusivamente las decisiones de plataforma históricas; conserva funcionalidad identificada.
- [x] Solo Backend; ninguna dependencia de aprobación, integración o pantalla del equipo Mobile.
- [x] No genera specs de H05/H06 ni código ni operaciones cloud.

## Aceptación específica

- [x] **AC-A-080-01:** Dos instancias comparten rate limit e invalidación (`tests/unit/cache/cache_pagination_test.go:TestTwoInstancesDistributedRateLimitAndInvalidation`).
- [x] **AC-A-080-02:** Acceso entre usuarios no comparte respuesta cacheada (`tests/unit/cache/cache_pagination_test.go:TestMultiUserCacheIsolation`).
- [x] **AC-A-080-03:** 10000 movimientos paginados sin Scan ni offset (`tests/unit/cache/cache_pagination_test.go:TestCursorPaginationTenThousandItemsNoScan`).

## Revisiones de construcción

- [x] Se revisó **Rol Redis** (§2.1) y su mecanismo de aceptación con casos positivos, negativos y límites.
- [x] Se revisó **Keys y seguridad** (§2.2) y su mecanismo de aceptación con casos positivos, negativos y límites.
- [x] Se revisó **Cachés elegibles** (§2.3) y su mecanismo de aceptación con casos positivos, negativos y límites.
- [x] Se revisó **Invalidación** (§2.4) y su mecanismo de aceptación con casos positivos, negativos y límites.
- [x] Se revisó **Rate limiting** (§2.5) y su mecanismo de aceptación con casos positivos, negativos y límites.
- [x] Se revisó **Paginación cursor** (§2.6) y su mecanismo de aceptación con casos positivos, negativos y límites.
- [x] Se revisó **Consistencia de agregados** (§2.7) y su mecanismo de aceptación con casos positivos, negativos y límites.
- [x] Se revisó **Medición** (§2.8) y su mecanismo de aceptación con casos positivos, negativos y límites.

## Estado de las evidencias

| Evidencia | Estado actual | Regla |
|---|---|---|
| Documentos `spec/plan/tasks/checklist` en paquete | GENERADOS; validación en test de ZIP | Hash/estructura comprobables |
| Revisión humana del contenido de A-080 | REVISADO Y APROBADO | Operador aprueba avance |
| Pruebas Go / HTTP reales | PASS (100% unit tests) | Validado con `go test ./...` |
| AWS/Redis/2 instancias/despliegue real | SIMULADO LOCAL / NOT_RUN CLOUD | Cloud real reservado para Gate correspondiente |
| Costo cero y permisos de cuenta AWS | PENDIENTE H05 | Bloquea H05 si no viable |
