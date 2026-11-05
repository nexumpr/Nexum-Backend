# A-050 — Autenticación, sesiones, JWT y privacidad

| Campo | Valor |
|---|---|
| Fase | H04 — Arquitectura general (lote consolidado) |
| Versión | 0.2.0 |
| Estado | **DRAFT — Gate H04 pendiente** |
| Trazabilidad | E001-E020, E014, GOV-09 |
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

### 1. Registro e identidad

Email canónico normalizado y unicidad con item reservado vía condición; Argon2id parámetros iniciales m=64MiB t=3 p=2 (calibrar bajo dos instancias, control DoS), sal aleatoria per usuario; no almacenar contraseña reversible.

**Regla verificable:** implementar y probar este comportamiento en H08; cerrar el mecanismo de diseño, su riesgo y el contrato antes del Gate correspondiente.

### 2. JWT

Access RS256 con kid/aud/iss/sub/jti/exp/iat/nbf, duración sugerida 10 min; allowlist de algoritmos, key rotation y JWKS/firma controlada; no aceptar tokens con none ni claims de userId enviados por cliente.

**Regla verificable:** implementar y probar este comportamiento en H08; cerrar el mecanismo de diseño, su riesgo y el contrato antes del Gate correspondiente.

### 3. Refresh

Refresh opaco aleatorio 256-bit, solo hash/HMAC en DynamoDB; TTL sugerido 30 días y rotación cada uso mediante CAS/condición, detectar replay y revocar familia, 401 sin filtrar detalles; revocación visible en ambos nodos.

**Regla verificable:** implementar y probar este comportamiento en H08; cerrar el mecanismo de diseño, su riesgo y el contrato antes del Gate correspondiente.

### 4. Logout y revocación

E004 revoca sessionId/family de forma durable y desasocia push del dispositivo; access JWT corto y mecanismo revocation/version para endpoints sensibles. La app elimina tokens locales pero no sustituye revocación en servidor.

**Regla verificable:** implementar y probar este comportamiento en H08; cerrar el mecanismo de diseño, su riesgo y el contrato antes del Gate correspondiente.

### 5. Recuperación y correo

E005/E006 token de un uso hash con caducidad, respuesta neutra y rate limit, sin user enumeration; verificación de correo idempotente y firma de enlaces en proveedor aún por definir H05.

**Regla verificable:** implementar y probar este comportamiento en H08; cerrar el mecanismo de diseño, su riesgo y el contrato antes del Gate correspondiente.

### 6. Autorización de recursos

userId deriva de sub; cada consulta DynamoDB usa pk por usuario derivado; 404 si no existe o es de tercero; authorization por función, jamás confiar en role/userId del payload.

**Regla verificable:** implementar y probar este comportamiento en H08; cerrar el mecanismo de diseño, su riesgo y el contrato antes del Gate correspondiente.

### 7. Cuenta y privacidad

DELETE /users/me exige contraseña y confirmación ELIMINAR, revoca sesiones, programa borrado real de recursos y versiones, retención/backups documentados. No usar TTL como única garantía de borrado inmediato.

**Regla verificable:** implementar y probar este comportamiento en H08; cerrar el mecanismo de diseño, su riesgo y el contrato antes del Gate correspondiente.

### 8. Secretos y auditoría

Secrets fuera de Git, logs sin hashes/refresh/access/password, PII redacted; acceso AWS de CI con OIDC y rol mínimo; audit eventos auth sin contenido financiero. Aviso de privacidad aprobado por responsable legal.

**Regla verificable:** implementar y probar este comportamiento en H08; cerrar el mecanismo de diseño, su riesgo y el contrato antes del Gate correspondiente.

## 3. Límites del alcance

- Solamente Backend, datos, seguridad, contratos API e infraestructura gestionada por el equipo Backend; ninguna implementación de Android, landing, Play Store o aprobación del equipo móvil.
- H04 congela **arquitectura lógica**; H05 congela viabilidad y topología física; H06 congela OpenAPI detallado de cada operación; H08 implementa conforme a esas decisiones.
- No convertir opciones propuestas de un catálogo histórico en compromisos silenciosos; no hacer optativas las obligaciones docentes.

## 4. Riesgos específicos y decisiones que deberán sustentarse

- **R-A-050-01:** Exactitud del flujo de borrado y retención legal requiere aprobación responsable del dato.
- **R-A-050-02:** Recuperación requiere proveedor de correo/OTP viable bajo cuenta AWS en H05.

## 5. Condiciones de aceptación demostrables

- **AC-A-050-01:** Reutilización refresh provoca revocación familia.
- **AC-A-050-02:** Logout en nodo A invalida refresh en nodo B.
- **AC-A-050-03:** 401 homogéneo correo desconocido; 404 recurso de tercero.

## 6. Trazabilidad y desbloqueo

- H04: aprobar este diseño en conjunto con otros once, sin ocultar discrepancias de fuentes.
- H05: seleccionar recursos AWS permitidos, demostrar costo cero y capacidad para DOS instancias simultáneas + LB/HTTPS + Redis sin simulaciones.
- H06: ratificar las 75 rutas con contratos y categorías M/S/E/O y su cobertura contra pantallas; incluir las operaciones académicas.
- H07: comprobar contradicciones, seguridad, aceptación y que nada indispensable se pospone a implementación por falta de diseño.
- H08: cada caso se implementa y prueba de forma real, aislada, con autorización de release.
