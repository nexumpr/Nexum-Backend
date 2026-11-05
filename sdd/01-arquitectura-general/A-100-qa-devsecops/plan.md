# A-100 — Plan técnico de arquitectura

**Estado:** DRAFT H04. **Entrada obligatoria:** `spec.md` y fuentes de A-100. **Salida:** decisión lista para implementar después de H07; este plan NO ejecuta infraestructura ni código.

## 1. Estrategia de construcción

1. **Pirámide de pruebas.** Definir interfaces, límites, precondiciones, postcondiciones, fallos, autorización y pruebas correspondientes. Unitarias Domain/Application sin SDK, integración HTTP+repositorio, contratos OpenAPI, E2E mínima funcional, carga de listados, seguridad auth/replay y prueba destrucción/restauración de entornos por IaC futura.
2. **Umbrales.** Definir interfaces, límites, precondiciones, postcondiciones, fallos, autorización y pruebas correspondientes. Objetivo orientativo >=80% líneas de lógica negocio y >=90% casos críticos de money/auth, sin tomar cobertura como sustituto de invariantes; gates por pruebas y hallazgos críticos.
3. **Matriz 75 rutas.** Definir interfaces, límites, precondiciones, postcondiciones, fallos, autorización y pruebas correspondientes. Cada ID Nexum-E001..075 incluye clasificación, responsable backend/no HTTP local, ACL, repos, tests y estado de implementación H06/H08; no se pierde extensión prometida por presentarla como opcional en PDF antiguo.
4. **Casos de abuso.** Definir interfaces, límites, precondiciones, postcondiciones, fallos, autorización y pruebas correspondientes. Acceso a recursos de otro usuario, IDOR, JWT alg confusion, fuerza bruta, refresh replay, race doble pago, tampering monto, overposting, JSON masivo, paginación cruzada, DNS/SSRF en enlaces dinámicos, secreto en logs.
5. **Calidad estática.** Definir interfaces, límites, precondiciones, postcondiciones, fallos, autorización y pruebas correspondientes. gofmt, go vet, govulncheck, gosec, SCA, secret scanning, SBOM, license policy, lint con versión fijada y pruebas de dependencias; no secretos/credenciales en repo.
6. **CI/CD.** Definir interfaces, límites, precondiciones, postcondiciones, fallos, autorización y pruebas correspondientes. GitHub Actions Backend únicamente: PR verifica tests/lint/seguridad/build, firmar/verificar artefactos cuando aplique; deployment con environment protection y aprobación humana, OIDC cloud mínimo privilegio, release y rollback. No responsabilidad Mobile.
7. **Evidencia.** Definir interfaces, límites, precondiciones, postcondiciones, fallos, autorización y pruebas correspondientes. Capturas y logs resumidos con fecha, commit, hash build, región, release, HTTP status/body saneado, pruebas negativas, costos y autorización; guardar fuera del repo si governance lo exige.
8. **Release incremental.** Definir interfaces, límites, precondiciones, postcondiciones, fallos, autorización y pruebas correspondientes. Solo endpoint/funcionalidad autorizada por release; primero health/bootstrap técnico, luego E001 registro, E002 login, E004 logout y cuatro adicionales del profesor; no marcar endpoints como implementados por contrato escrito.

## 2. Orden interno de diseño y dependencias

1. Revisar `constitution.md` y `AGENTS.md` antes de aceptar cambios; cotejar los PDF/prototipo cuyos nombres y hashes se registran en H04.
2. Identificar invariantes y entradas/salidas de cada módulo; mapear al catálogo E001..075 sin crear contratos endpoints H06.
3. Verificar acoplamiento con A-020 (capas), A-040 (DTOs), A-050 (auth), A-060 (persistencia), A-070 (consistencia), A-080 (Redis), A-090 (telemetría) y A-100 (QA).
4. Mantener como *blocker de H05* capacidad/costo de 2 instancias + balanceador + Redis, restricciones SCP/IAM, TLS y mecanismos de on/off; no inferir disponibilidad de AWS Free.
5. Registrar la decisión humana y conflictos si alguna regla altera el negocio del prototipo; no solicitar gate Mobile.

## 3. Estrategia de pruebas posteriores por capa

| Nivel | Diseño a comprobar | Evidencia futura obligatoria |
|---|---|---|
| Dominio | Invariantes y money/identidad | go test, race detector, entradas límite |
| Aplicación | Orquestación con mocks y transacciones | unit tests, casos negativos, retries |
| Infraestructura | DynamoDB/Redis reales según fase | integración, latencia, permisos mínimos, gasto |
| HTTP | Schemas OpenAPI 3.1 y respuestas | pruebas contractuales y HTTP reales |
| CI/CD | Seguridad y releases | PR checks, escaneo, autorización humana |

## 4. Riesgos y mitigación explícita

- No abrir Cloud deploy hasta evaluar permisos/FinOps en H05. **Mitigación:** resolver antes de aprobar su dependencia; si requiere cambio de constitución emitir RFC.
- El proyecto escolar pide 7 operaciones tempranas, pero H08 inicia tras H07 aprobado por decisión humana. **Mitigación:** resolver antes de aprobar su dependencia; si requiere cambio de constitución emitir RFC.

## 5. Criterio de cierre

Se considera listo para revisión cuando los 8 ejes técnicos de `spec.md` tienen trazabilidad a pruebas en `checklist.md`, los elementos futuros permanecen `NOT_RUN`, la matriz de alcance está intacta y las decisiones pendientes tienen dueño/fase, sin `APPROVED` automático.
