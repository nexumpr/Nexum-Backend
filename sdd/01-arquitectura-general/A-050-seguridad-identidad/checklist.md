# A-050 — Checklist de arquitectura

**Estado:** IMPLEMENTED_LOCAL_H08; suite de autenticación, Argon2id, JWT RS256 y rotación de sesiones ejecutadas con 100% PASS.

## Integridad conceptual

- [x] `constitution.md` prevalece sobre decisiones de esta spec.
- [x] No se asume que el prototipo Android realiza HTTP ni que el catálogo anterior describe implementación existente.
- [x] La arquitectura AWS+DynamoDB+Redis reemplaza exclusivamente las decisiones de plataforma históricas; conserva funcionalidad identificada.
- [x] Solo Backend; ninguna dependencia de aprobación, integración o pantalla del equipo Mobile.
- [x] No genera specs de H05/H06 ni código ni operaciones cloud.

## Aceptación específica

- [x] **AC-A-050-01:** Reutilización refresh provoca revocación familia (verificado por TestRefreshTokenReplayFamilyRevocation).
- [x] **AC-A-050-02:** Logout en nodo A invalida refresh en nodo B (verificado por TestTwoNodesDistributedLogout).
- [x] **AC-A-050-03:** 401 homogéneo correo desconocido; 404 recurso de tercero (verificado por TestAuthHomogeneousErrorsAndNonEnumeration).

## Revisiones de construcción

- [x] Se revisó **Registro e identidad** (§2.1) y su mecanismo de aceptación con casos positivos, negativos y límites.
- [x] Se revisó **JWT** (§2.2) y su mecanismo de aceptación con casos positivos, negativos y límites.
- [x] Se revisó **Refresh** (§2.3) y su mecanismo de aceptación con casos positivos, negativos y límites.
- [x] Se revisó **Logout y revocación** (§2.4) y su mecanismo de aceptación con casos positivos, negativos y límites.
- [x] Se revisó **Recuperación y correo** (§2.5) y su mecanismo de aceptación con casos positivos, negativos y límites.
- [x] Se revisó **Autorización de recursos** (§2.6) y su mecanismo de aceptación con casos positivos, negativos y límites.
- [x] Se revisó **Cuenta y privacidad** (§2.7) y su mecanismo de aceptación con casos positivos, negativos y límites.
- [x] Se revisó **Secretos y auditoría** (§2.8) y su mecanismo de aceptación con casos positivos, negativos y límites.

## Estado de las evidencias

| Evidencia | Estado actual | Regla |
|---|---|---|
| Documentos `spec/plan/tasks/checklist` en paquete | GENERADOS; validación en test de ZIP | Hash/estructura comprobables |
| Revisión humana del contenido de A-050 | APROBADO H08-A | Aprobación humana expresa para implementación |
| Pruebas Go / HTTP reales | PASS (local) | Ejecutado en Windows nativo vía `go test -v ./...` |
| AWS/Redis/2 instancias/despliegue real | NOT_RUN | Se concretan H05, se ejecutan H08 cloud |
| Costo cero y permisos de cuenta AWS | PENDIENTE H05 | Bloquea H05 si no viable |

### Hotfix D-10

- [x] Parametros Argon2 acotados antes de derivar la clave (anti-DoS).
- [x] Longitud de digest validada contra constantes uint32 (G115 resuelto sin exclusion).
- [x] Compatibilidad con hashes canonicos existentes.
- [x] Rechazo rapido de entradas peligrosas y ausencia de panicos.
- [x] gosec y govulncheck PASS.
- [ ] CI Go en este PR — no disponible hasta integrar A-100 (bootstrap).
