# A-050 — Checklist de arquitectura

**Estado:** DRAFT; pruebas documentales del lote comprobables; pruebas reales de Backend/infra `NOT_RUN`.

## Integridad conceptual

- [ ] `constitution.md` prevalece sobre decisiones de esta spec.
- [ ] No se asume que el prototipo Android realiza HTTP ni que el catálogo anterior describe implementación existente.
- [ ] La arquitectura AWS+DynamoDB+Redis reemplaza exclusivamente las decisiones de plataforma históricas; conserva funcionalidad identificada.
- [ ] Solo Backend; ninguna dependencia de aprobación, integración o pantalla del equipo Mobile.
- [ ] No genera specs de H05/H06 ni código ni operaciones cloud.

## Aceptación específica

- [ ] **AC-A-050-01:** Reutilización refresh provoca revocación familia.
- [ ] **AC-A-050-02:** Logout en nodo A invalida refresh en nodo B.
- [ ] **AC-A-050-03:** 401 homogéneo correo desconocido; 404 recurso de tercero.

## Revisiones de construcción

- [ ] Se revisó **Registro e identidad** (§2.1) y su mecanismo de aceptación con casos positivos, negativos y límites.
- [ ] Se revisó **JWT** (§2.2) y su mecanismo de aceptación con casos positivos, negativos y límites.
- [ ] Se revisó **Refresh** (§2.3) y su mecanismo de aceptación con casos positivos, negativos y límites.
- [ ] Se revisó **Logout y revocación** (§2.4) y su mecanismo de aceptación con casos positivos, negativos y límites.
- [ ] Se revisó **Recuperación y correo** (§2.5) y su mecanismo de aceptación con casos positivos, negativos y límites.
- [ ] Se revisó **Autorización de recursos** (§2.6) y su mecanismo de aceptación con casos positivos, negativos y límites.
- [ ] Se revisó **Cuenta y privacidad** (§2.7) y su mecanismo de aceptación con casos positivos, negativos y límites.
- [ ] Se revisó **Secretos y auditoría** (§2.8) y su mecanismo de aceptación con casos positivos, negativos y límites.

## Estado de las evidencias

| Evidencia | Estado actual | Regla |
|---|---|---|
| Documentos `spec/plan/tasks/checklist` en paquete | GENERADOS; validación en test de ZIP | Hash/estructura comprobables |
| Revisión humana del contenido de A-050 | PENDIENTE | No declarar APPROVED |
| Pruebas Go / HTTP reales | NOT_RUN | Solo tras implementación H08 |
| AWS/Redis/2 instancias/despliegue real | NOT_RUN | Se concretan H05, se ejecutan H08 |
| Costo cero y permisos de cuenta AWS | PENDIENTE H05 | Bloquea H05 si no viable |
