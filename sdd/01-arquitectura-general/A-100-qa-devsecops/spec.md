# A-100 — Pruebas, calidad y DevSecOps

| Campo | Valor |
|---|---|
| Fase | H04 — Arquitectura general (lote consolidado) |
| Versión | 0.2.0 |
| Estado | **DRAFT — Gate H04 pendiente** |
| Trazabilidad | GOV-07, E001-E075 |
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

### 1. Pirámide de pruebas

Unitarias Domain/Application sin SDK, integración HTTP+repositorio, contratos OpenAPI, E2E mínima funcional, carga de listados, seguridad auth/replay y prueba destrucción/restauración de entornos por IaC futura.

**Regla verificable:** implementar y probar este comportamiento en H08; cerrar el mecanismo de diseño, su riesgo y el contrato antes del Gate correspondiente.

### 2. Umbrales

Objetivo orientativo >=80% líneas de lógica negocio y >=90% casos críticos de money/auth, sin tomar cobertura como sustituto de invariantes; gates por pruebas y hallazgos críticos.

**Regla verificable:** implementar y probar este comportamiento en H08; cerrar el mecanismo de diseño, su riesgo y el contrato antes del Gate correspondiente.

### 3. Matriz 75 rutas

Cada ID Nexum-E001..075 incluye clasificación, responsable backend/no HTTP local, ACL, repos, tests y estado de implementación H06/H08; no se pierde extensión prometida por presentarla como opcional en PDF antiguo.

**Regla verificable:** implementar y probar este comportamiento en H08; cerrar el mecanismo de diseño, su riesgo y el contrato antes del Gate correspondiente.

### 4. Casos de abuso

Acceso a recursos de otro usuario, IDOR, JWT alg confusion, fuerza bruta, refresh replay, race doble pago, tampering monto, overposting, JSON masivo, paginación cruzada, DNS/SSRF en enlaces dinámicos, secreto en logs.

**Regla verificable:** implementar y probar este comportamiento en H08; cerrar el mecanismo de diseño, su riesgo y el contrato antes del Gate correspondiente.

### 5. Calidad estática

gofmt, go vet, govulncheck, gosec, SCA, secret scanning, SBOM, license policy, lint con versión fijada y pruebas de dependencias; no secretos/credenciales en repo.

**Regla verificable:** implementar y probar este comportamiento en H08; cerrar el mecanismo de diseño, su riesgo y el contrato antes del Gate correspondiente.

### 6. CI/CD

GitHub Actions Backend únicamente: PR verifica tests/lint/seguridad/build, firmar/verificar artefactos cuando aplique; deployment con environment protection y aprobación humana, OIDC cloud mínimo privilegio, release y rollback. No responsabilidad Mobile.

**Regla verificable:** implementar y probar este comportamiento en H08; cerrar el mecanismo de diseño, su riesgo y el contrato antes del Gate correspondiente.

### 7. Evidencia

Capturas y logs resumidos con fecha, commit, hash build, región, release, HTTP status/body saneado, pruebas negativas, costos y autorización; guardar fuera del repo si governance lo exige.

**Regla verificable:** implementar y probar este comportamiento en H08; cerrar el mecanismo de diseño, su riesgo y el contrato antes del Gate correspondiente.

### 8. Release incremental

Solo endpoint/funcionalidad autorizada por release; primero health/bootstrap técnico, luego E001 registro, E002 login, E004 logout y cuatro adicionales del profesor; no marcar endpoints como implementados por contrato escrito.

**Regla verificable:** implementar y probar este comportamiento en H08; cerrar el mecanismo de diseño, su riesgo y el contrato antes del Gate correspondiente.

## 3. Límites del alcance

- Solamente Backend, datos, seguridad, contratos API e infraestructura gestionada por el equipo Backend; ninguna implementación de Android, landing, Play Store o aprobación del equipo móvil.
- H04 congela **arquitectura lógica**; H05 congela viabilidad y topología física; H06 congela OpenAPI detallado de cada operación; H08 implementa conforme a esas decisiones.
- No convertir opciones propuestas de un catálogo histórico en compromisos silenciosos; no hacer optativas las obligaciones docentes.

## 4. Riesgos específicos y decisiones que deberán sustentarse

- **R-A-100-01:** No abrir Cloud deploy hasta evaluar permisos/FinOps en H05.
- **R-A-100-02:** El proyecto escolar pide 7 operaciones tempranas, pero H08 inicia tras H07 aprobado por decisión humana.

## 5. Condiciones de aceptación demostrables

- **AC-A-100-01:** Contrato OpenAPI pasa lint+examples+HTTP real en implementación.
- **AC-A-100-02:** Race detector detecta acceso inseguro; control SAST/DAST en CI.
- **AC-A-100-03:** Firma de release y aprobación humana registradas.

## 6. Trazabilidad y desbloqueo

- H04: aprobar este diseño en conjunto con otros once, sin ocultar discrepancias de fuentes.
- H05: seleccionar recursos AWS permitidos, demostrar costo cero y capacidad para DOS instancias simultáneas + LB/HTTPS + Redis sin simulaciones.
- H06: ratificar las 75 rutas con contratos y categorías M/S/E/O y su cobertura contra pantallas; incluir las operaciones académicas.
- H07: comprobar contradicciones, seguridad, aceptación y que nada indispensable se pospone a implementación por falta de diseño.
- H08: cada caso se implementa y prueba de forma real, aislada, con autorización de release.
