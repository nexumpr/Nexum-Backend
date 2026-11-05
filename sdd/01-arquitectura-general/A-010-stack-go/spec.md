# A-010 — Stack Go, versiones y dependencias

| Campo | Valor |
|---|---|
| Fase | H04 — Arquitectura general (lote consolidado) |
| Versión | 0.2.0 |
| Estado | **DRAFT — Gate H04 pendiente** |
| Trazabilidad | E001-E075, GOV-03, GOV-07 |
| Proveedor vigente | AWS; DynamoDB NoSQL; Redis real; Backend Go |
| Fuente original | ZIP Android V6.0.2 `(3)`; PDF 75 rutas `(2)`; PDF docente v3 |

## 1. Objetivo y condiciones de aceptación

Esta especificación establece los **requisitos de diseño vinculantes propuestos**, no código implementado. Se rechazan decisiones sin trazabilidad, seguridad o viabilidad. Si contradice `constitution.md`, el Gate se bloquea y se tramita RFC. No crea recursos AWS, endpoints HTTP reales ni modifica cliente móvil.

## Base documental y autoridad

1. Autoridad superior: `constitution.md` v0.2.0, `AGENTS.md`, H03 humano APPROVED (2026-11-05). Ningún texto de esta entrega cambia permisos ni política.
2. Fuente académica: **Requerimientos para el desarrollo del proyecto Móvil v3(2).pdf**, cuatro páginas; requisitos de Backend y dependencias transversales. Se consultó íntegramente en esta entrega.
3. Fuente funcional: **Nexum_V6_0_2_AndroidStudio_Demo_Eliminar_Cuenta(3).zip**, 16 entradas ZIP. Se leyeron `app.js`, `index.html`, `MainActivity.java`, `AndroidManifest.xml`, `README.md` y se comprobó ausencia de `fetch(` en `app.js`. Código cliente no constituye Backend existente.
4. Catálogo: **Nexum_V6_0_2_Catalogo_Completo_Endpoints_Backend(2).pdf**, 27 páginas; 75 identificadores/rutas propuestas E001..E075. Etiquetas M/S/E/O preservadas. **No adoptar** su stack histórico Google Cloud/MongoDB; el stack vigente del proyecto es **AWS + DynamoDB + Redis**.
5. Evidencia H03: 54 decisiones del motor real (53 DENY, 1 ALLOW) y aprobación humana del Gate H03; ejecución en Windows aportada en Warp. En este lote NO se llama a OpenCode y sus permisos siguen restringidos.

**Trazabilidad:** la etiqueta M/S/E/O describe el origen de la ruta, no constituye por sí sola decisión de implementación ni reemplaza lo exigido por el docente. La formalización OpenAPI por ruta y su aprobación corresponde a H06. Las decisiones físicas de despliegue y costo cero corresponden a H05 y no están presumidas.


## 2. Contrato arquitectónico

### 1. Runtime Go

Base propuesta: Go 1.27.3 estable (validar disponibilidad del binario oficial el día de H08), go.mod toolchain pin y CI en Windows/Linux; no subir actualización de versión sin PR de compatibilidad. Build reproducible -trimpath y política de checksums de módulos.

**Regla verificable:** implementar y probar este comportamiento en H08; cerrar el mecanismo de diseño, su riesgo y el contrato antes del Gate correspondiente.

### 2. HTTP y JSON

net/http y ServeMux de Go (ruta registrada por método), encoding/json con DisallowUnknownFields para inputs controlados, límite bytes y validación semántica separada. Selección reduce transitive dependencies; no introducir Gin/Echo sin ADR.

**Regla verificable:** implementar y probar este comportamiento en H08; cerrar el mecanismo de diseño, su riesgo y el contrato antes del Gate correspondiente.

### 3. DynamoDB

AWS SDK for Go v2, expresiones condicionadas, paginadores y context deadline. Adaptador en Infrastructure, nunca importar SDK desde Domain o Application.

**Regla verificable:** implementar y probar este comportamiento en H08; cerrar el mecanismo de diseño, su riesgo y el contrato antes del Gate correspondiente.

### 4. Redis

github.com/redis/go-redis/v9 para caché distribuida, rate limit y locks solo cuando semánticamente justificables; Redis real y obligatorio en despliegue, no suplantar por mapa local.

**Regla verificable:** implementar y probar este comportamiento en H08; cerrar el mecanismo de diseño, su riesgo y el contrato antes del Gate correspondiente.

### 5. Autenticación

golang.org/x/crypto/argon2 para contraseñas Argon2id; JWT firmado RS256 con biblioteca verificada (github.com/golang-jwt/jwt/v5 candidato) e interfaz de firma; claves asimétricas y rotación, KMS como opción dependiente H05 IAM/costo.

**Regla verificable:** implementar y probar este comportamiento en H08; cerrar el mecanismo de diseño, su riesgo y el contrato antes del Gate correspondiente.

### 6. Telemetría

OpenTelemetry Go SDK + otelhttp; log/slog JSON sin PII ni tokens. Config de exportación desacoplada de proveedor y correlación request_id/trace_id.

**Regla verificable:** implementar y probar este comportamiento en H08; cerrar el mecanismo de diseño, su riesgo y el contrato antes del Gate correspondiente.

### 7. Tests/seguridad

testing y httptest estándar, race detector, go vet, govulncheck, gosec, dependencia/secret scan y SBOM. Fijar versiones exactas en la primera PR de implementación, documentar licencias, CVEs y upgrades con límites.

**Regla verificable:** implementar y probar este comportamiento en H08; cerrar el mecanismo de diseño, su riesgo y el contrato antes del Gate correspondiente.

### 8. Construcción sin contenedores locales

No exigir Docker en Windows; ejecutar go test y DynamoDB contract tests mediante mocks/fakes o servicios autorizados por pruebas. Si build AWS usa contenedores gestionados, no imponer Docker local.

**Regla verificable:** implementar y probar este comportamiento en H08; cerrar el mecanismo de diseño, su riesgo y el contrato antes del Gate correspondiente.

## 3. Límites del alcance

- Solamente Backend, datos, seguridad, contratos API e infraestructura gestionada por el equipo Backend; ninguna implementación de Android, landing, Play Store o aprobación del equipo móvil.
- H04 congela **arquitectura lógica**; H05 congela viabilidad y topología física; H06 congela OpenAPI detallado de cada operación; H08 implementa conforme a esas decisiones.
- No convertir opciones propuestas de un catálogo histórico en compromisos silenciosos; no hacer optativas las obligaciones docentes.

## 4. Riesgos específicos y decisiones que deberán sustentarse

- **R-A-010-01:** Go versión publicada no implica binario instalado; PR H08 bloquear si toolchain no coincide.
- **R-A-010-02:** External dependencias deben revisarse licencias y advisories antes de first commit.

## 5. Condiciones de aceptación demostrables

- **AC-A-010-01:** go version objetivo y matriz Windows/Linux 64 bits definida.
- **AC-A-010-02:** Toda dependencia de infraestructura detrás de puertos y sometida a SCA.
- **AC-A-010-03:** Build de implementación debe funcionar sin Docker local.

## 6. Trazabilidad y desbloqueo

- H04: aprobar este diseño en conjunto con otros once, sin ocultar discrepancias de fuentes.
- H05: seleccionar recursos AWS permitidos, demostrar costo cero y capacidad para DOS instancias simultáneas + LB/HTTPS + Redis sin simulaciones.
- H06: ratificar las 75 rutas con contratos y categorías M/S/E/O y su cobertura contra pantallas; incluir las operaciones académicas.
- H07: comprobar contradicciones, seguridad, aceptación y que nada indispensable se pospone a implementación por falta de diseño.
- H08: cada caso se implementa y prueba de forma real, aislada, con autorización de release.
