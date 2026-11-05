# A-000 — Checklist de aceptación del contexto

**Versión:** `0.1.1` | **Estado documental:** `DRAFT` | **Gate anterior:** H03 aprobado por el usuario, 2026-11-05

> **Convención:** `[x]` = redactado/cotejado documentalmente dentro de este paquete; `[ ]` = pendiente de comprobar o de aprobar. No confundir preparación de contenido con prueba de instalación, aprobación humana ni ejecución del Backend.

## C0. Control de entradas y secuencia

- [x] **C0-01:** gate H03 autorizado expresamente; no depende del equipo Mobile. (CTX-012; T001)
- [x] **C0-02:** A-000 fue la primera spec instalada; la consolidación autorizada de H04 ya incorporó A-010..A-110. No se generan especificaciones H05/H06 ni código. (T001/T009; H04 consolidada)
- [x] **C0-03:** documentos base definidos: `spec.md`, `plan.md`, `tasks.md`, `checklist.md`; sin archivos vacíos. (T002..T008)
- [x] **C0-04:** hashes/estructura comprobados en **Windows real** mediante salida `PASS` aportada por el usuario el 2026-11-05. (T009; `PASS`)

## C1. Contexto funcional y límites

- [x] **C1-01:** problema/propósito del Backend explicados sin crear pantallas. (CTX-001; T002)
- [x] **C1-02:** persona usuaria, cliente consumidor, responsable y servicios externos diferenciados; no se inventan roles de negocio. (CTX-001,014; T003/T004)
- [x] **C1-03:** Mobile, Play Store, landing y repo de privacidad fuera de construcción/aprobación Backend; API independiente de aprobación Android. (CTX-001,010; T003)
- [x] **C1-04:** flujos de identidad, sesión, finanzas, cuenta y acciones condicionales descritos **sin** rutas/EP no confirmados. (CTX-004/005/011; T005)
- [x] **C1-05:** siete EP históricos tratados como subconjunto, no catálogo completo. (CTX-011; T005)

## C2. Restricciones técnicas y no funcionales

- [x] **C2-01:** dos instancias reales simultáneas + balanceo real + HTTPS/TLS son compromisos cerrados, no mocks. (CTX-002)
- [x] **C2-02:** DynamoDB persistente y Redis compartido real explícitos, sin tablas/índices inventados. (CTX-003)
- [x] **C2-03:** JWT/sesiones/revocación/logout/eliminar cuenta y propiedad del dato figuran. (CTX-004/014)
- [x] **C2-04:** moneda, importes, idempotencia y consistencia nombrados sin fijar reglas de producto inexistentes. (CTX-005)
- [x] **C2-05:** caching/paginación/rate limiting/indexación/JSON/compresión previstos y pendientes de métricas. (CTX-006)
- [x] **C2-06:** observabilidad, privacidad, QA, DevSecOps, CI/CD Backend, versiones e IaC con aprobación humana. (CTX-007/008/012/014)
- [x] **C2-07:** conflicto costo cero versus recursos y créditos reconocido, sin solución ficticia. (CTX-009)
- [x] **C2-08:** cuenta y región AWS de trabajo son contexto; SCP reconocido sin bypass. (CTX-013)

## C3. Trazabilidad y calidad factual

- [x] **C3-01:** GOV-01..GOV-18 están trazados a secciones y CTX de A-000. (CTX-012; T007)
- [x] **C3-02:** CTX-001..CTX-014 tienen criterios y están vinculados a tareas y checklist. (T006/T007)
- [x] **C3-03:** el PDF docente v3 de cuatro páginas se cotejó directamente durante H04; contexto v1.3 conserva su papel como referencia histórica secundaria. (T008)
- [x] **C3-04:** el ZIP Android V6.0.2 `(3)` y el catálogo PDF de 75 rutas fueron inspeccionados para H04; H06 debe formalizar los contratos definitivos, sin inventar HTTP existente. (CTX-011; T008)
- [x] **C3-05:** ningún ADR/topología/proveedor extra, API concreta, tabla NoSQL, contrato JSON o cifra de endpoints inventado. (T004/T005)
- [x] **C3-06:** todo requisito aceptado debe implementarse realmente tras aprobar SDD entero; existen solo las 12 especificaciones H04 en DRAFT. H05/H06/H07/H08 permanecen sin iniciar. (CTX-012)

## C4. Evidencia y revisión humana

- [x] **C4-01:** A-000 instalado en Windows; salida de Warp compartida con hashes, estructura y cambios exactos. (`PASS`)
- [ ] **C4-02:** responsable revisa las fronteras de sistema y confirma que el nivel A-000 es correcto. (`PENDING_HUMAN`)
- [ ] **C4-03:** tratar discrepancias materiales si aparecen al cotejar fuentes. (`PENDING_IF_APPLICABLE`)
- [ ] **C4-04:** aprobación explícita `APRUEBO A-000` y actualización del estado a `APPROVED`, sin autoaprobación. (`PENDING_HUMAN`)
- [ ] **C4-05:** A-010..A-110 ya están redactadas e instaladas en DRAFT por entrega consolidada; su aprobación y el inicio de H05 solo pueden ocurrir tras aprobación expresa del Gate H04. (`PENDING_HUMAN`)

## Resultado actual de la checklist

- **Redacción y cotejo interno:** requisitos de C1–C3 redactados conforme a las fuentes disponibles; pendiente de revisión humana de contenido.
- **Evidencia real de instalación:** el usuario compartió la salida `PASS` de Warp para A-000 el 2026-11-05. Las pruebas aisladas de construcción del ZIP no sustituyen pruebas de funcionamiento del Backend, que siguen sin ejecutarse.
- **Bloqueos de A-000 antes de aprobación:** revisión humana y, si las fuentes primarias revelan una contradicción material, su corrección. El cotejo exhaustivo del prototipo y sus operaciones es condición expresa de cierre de **A-060** y H06, no justificación para inventar contenido ahora.
- **Dictamen:** **DRAFT / NO APROBADO**. Próximo paso: revisar A-000..A-110 como bloque; **NO** crear H05 hasta ratificación del Gate H04.

---

## ADENDA H04 CONSOLIDADA — Fuentes contrastadas (2026-11-05; propuesta v0.2.0)

**Esta adenda complementa, NO sustituye, la versión 0.1.1 previamente instalada. El contenido histórico DRAFT se conserva para trazabilidad.** El usuario pidió preparar el bloque completo H04; se mantienen todos sus documentos en revisión, sin aprobación automática.

- Se revisó directamente el ZIP Android `(3)` de 16 entradas y sus recursos Java/WebView/JS; no contiene `fetch(` en app.js. La demo usa localStorage y datos en memoria, por lo que los endpoints descritos son propuestas pendientes, no rutas implementadas.
- Se leyó el catálogo PDF de 27 páginas y se extrajeron 75 IDs de rutas E001-E075 (33 M, 23 S, 12 E, 7 O). Anexo `../TRAZABILIDAD_75_RUTAS.md` conserva el inventario íntegro.
- Se leyó el PDF original docente v3 (4 páginas), con requisitos reales de dos instancias, balanceador y TLS, Redis, NoSQL, JWT, DevSecOps, CI/CD, observabilidad, costo cero y ML+IA generativa a nivel del proyecto Mobile. El Backend provee únicamente los contratos necesarios.
- **Precedencia inequívoca:** decisión vigente AWS/DynamoDB/Redis, NO Google Cloud/MongoDB Atlas mencionados en el catálogo histórico. No confundir esta aclaración con cambio aprobado de infraestructura.
- **Condición para Gate H04:** cotejar A000 con A010..A110 y fuentes, conservar clasificación y riesgos; bloquear H05 si hubiera contradicción arquitectónica no resuelta. Los riesgos de viabilidad física/financiera se validan de forma obligatoria durante H05, antes de implementar.
