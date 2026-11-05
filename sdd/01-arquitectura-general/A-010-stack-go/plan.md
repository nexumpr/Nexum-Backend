# A-010 — Plan técnico de arquitectura

**Estado:** DRAFT H04. **Entrada obligatoria:** `spec.md` y fuentes de A-010. **Salida:** decisión lista para implementar después de H07; este plan NO ejecuta infraestructura ni código.

## 1. Estrategia de construcción

1. **Runtime Go.** Definir interfaces, límites, precondiciones, postcondiciones, fallos, autorización y pruebas correspondientes. Base propuesta: Go 1.27.3 estable (validar disponibilidad del binario oficial el día de H08), go.mod toolchain pin y CI en Windows/Linux; no subir actualización de versión sin PR de compatibilidad. Build reproducible -trimpath y política de checksums de módulos.
2. **HTTP y JSON.** Definir interfaces, límites, precondiciones, postcondiciones, fallos, autorización y pruebas correspondientes. net/http y ServeMux de Go (ruta registrada por método), encoding/json con DisallowUnknownFields para inputs controlados, límite bytes y validación semántica separada. Selección reduce transitive dependencies; no introducir Gin/Echo sin ADR.
3. **DynamoDB.** Definir interfaces, límites, precondiciones, postcondiciones, fallos, autorización y pruebas correspondientes. AWS SDK for Go v2, expresiones condicionadas, paginadores y context deadline. Adaptador en Infrastructure, nunca importar SDK desde Domain o Application.
4. **Redis.** Definir interfaces, límites, precondiciones, postcondiciones, fallos, autorización y pruebas correspondientes. github.com/redis/go-redis/v9 para caché distribuida, rate limit y locks solo cuando semánticamente justificables; Redis real y obligatorio en despliegue, no suplantar por mapa local.
5. **Autenticación.** Definir interfaces, límites, precondiciones, postcondiciones, fallos, autorización y pruebas correspondientes. golang.org/x/crypto/argon2 para contraseñas Argon2id; JWT firmado RS256 con biblioteca verificada (github.com/golang-jwt/jwt/v5 candidato) e interfaz de firma; claves asimétricas y rotación, KMS como opción dependiente H05 IAM/costo.
6. **Telemetría.** Definir interfaces, límites, precondiciones, postcondiciones, fallos, autorización y pruebas correspondientes. OpenTelemetry Go SDK + otelhttp; log/slog JSON sin PII ni tokens. Config de exportación desacoplada de proveedor y correlación request_id/trace_id.
7. **Tests/seguridad.** Definir interfaces, límites, precondiciones, postcondiciones, fallos, autorización y pruebas correspondientes. testing y httptest estándar, race detector, go vet, govulncheck, gosec, dependencia/secret scan y SBOM. Fijar versiones exactas en la primera PR de implementación, documentar licencias, CVEs y upgrades con límites.
8. **Construcción sin contenedores locales.** Definir interfaces, límites, precondiciones, postcondiciones, fallos, autorización y pruebas correspondientes. No exigir Docker en Windows; ejecutar go test y DynamoDB contract tests mediante mocks/fakes o servicios autorizados por pruebas. Si build AWS usa contenedores gestionados, no imponer Docker local.

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

- Go versión publicada no implica binario instalado; PR H08 bloquear si toolchain no coincide. **Mitigación:** resolver antes de aprobar su dependencia; si requiere cambio de constitución emitir RFC.
- External dependencias deben revisarse licencias y advisories antes de first commit. **Mitigación:** resolver antes de aprobar su dependencia; si requiere cambio de constitución emitir RFC.

## 5. Criterio de cierre

Se considera listo para revisión cuando los 8 ejes técnicos de `spec.md` tienen trazabilidad a pruebas en `checklist.md`, los elementos futuros permanecen `NOT_RUN`, la matriz de alcance está intacta y las decisiones pendientes tienen dueño/fase, sin `APPROVED` automático.
