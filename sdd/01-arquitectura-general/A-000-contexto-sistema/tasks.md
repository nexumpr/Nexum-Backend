# A-000 — Tareas del contexto de sistema

**Versión:** `0.1.1` | **Estado:** `DRAFT` | **Fecha:** 2026-11-05 | **Gate previo:** H03 `APPROVED`

> Las tareas siguientes son de **análisis y validación documental**, no de implementación. Una casilla `[x]` refleja trabajo realizado sobre los documentos de esta entrega; ninguna se interpreta como prueba de Backend, Azure/AWS, API, CI/CD o Mobile.

| ID | Acción verificable | Dependencia | Resultado esperado / evidencia | Estado de redacción |
|---|---|---|---|---|
| A000-T001 | Registrar autorización humana H03, versión de constitución y límites. | Gate H03 expreso. | `memoria.md` factual y encabezados sincronizados. | `[x]` instalado y comprobado en Warp el 2026-11-05 (salida del usuario); aprobación A-000 pendiente. |
| A000-T002 | Definir misión, problema, usuarios y valor del Backend. | GOV-01,08,16. | `spec.md` §1, §3. | `[x]` redactado, revisión pendiente. |
| A000-T003 | Determinar responsabilidades Backend vs Mobile/Landing. | GOV-15,16,18 y contexto v1.3. | `spec.md` §2, sin aprobación Android. | `[x]` redactado, revisión pendiente. |
| A000-T004 | Representar frontera lógica y dependencias externas. | GOV-01..05,13. | `spec.md` §3–4, sin proveedor/topología cerrados. | `[x]` redactado, revisión pendiente. |
| A000-T005 | Identificar flujos y datos **confirmados** y distinguir los condicionales. | GOV-06..09,16,17. | `spec.md` §5–6, sin inventario EP. | `[x]` redactado, revisión pendiente. |
| A000-T006 | Registrar seguridad, disponibilidad, precisión, costo, QA y operación como criterios. | GOV-02..14,18. | `spec.md` §7, CTX-001..014. | `[x]` redactado, revisión pendiente. |
| A000-T007 | Construir trazabilidad GOV→CTX y coherencia transversal de cuatro documentos. | T002..T006. | `spec.md` §9; `checklist.md` con IDs y evidencia. | `[x]` redactado, revisión pendiente. |
| A000-T008 | Registrar fuentes no cotejadas y decisiones no adoptadas. | Contexto v1.3; fuentes del profesor/prototipo. | `spec.md` §8, `plan.md` §6; sin atribuciones ficticias. | `[x]` identificadas las fuentes y cotejados directamente el PDF docente v3 y ZIP Android `(3)` durante la consolidación H04. |
| A000-T009 | Ejecutar validación local de hashes/estructura y aislar el paquete de A-000. | T001..T008; ZIP completo. | Salida del instalador y manifiesto SHA-256 en Warp. | `[x]` `PASS` en Windows, evidencia compartida por el usuario el 2026-11-05; no equivale a aprobación. |
| A000-T010 | Revisar y resolver hallazgos de alcance con el responsable. | T001..T009. | Respuesta humana con correcciones o aceptación. | `[ ]` PENDIENTE. |
| A000-T011 | Ratificar A-000 si cumple criterios y actualizar memoria aprobada. | T010 y `checklist.md` sin bloqueo esencial. | Aprobación explícita y registro de estado. | `[ ]` PENDIENTE. |
| A000-T012 | Autorizar H05 solo tras aprobación humana del Gate H04 (A-000..A-110). | T011 y revisión H04 integral. | A-010..A-110 ya redactadas en DRAFT; H05 aún sin archivos. | `[ ]` BLOQUEADA hasta Gate H04. |

## Criterios de terminación por tarea

- T002–T007: el contenido consta en cuatro documentos con terminología homogénea y trazabilidad; **no requieren pruebas runtime**.
- T008: identificar fuentes pendientes **es cumplimiento del control de honestidad**, no la obtención de esas fuentes. El PDF original v3 y ZIP Android `(3)` fueron cotejados durante H04; la aprobación documental sigue pendiente.
- T009: `PASS` respaldado por la salida de Warp aportada por el usuario: 23 archivos de gobierno y cuatro documentos A-000 íntegros. No es prueba de funcionalidad Backend ni aprobación documental.
- T010–T011: la única autoridad para aceptar es el responsable Backend; ninguna automatización cambia `DRAFT` a `APPROVED`.
- T012: mantener bloqueado mientras A-000 permanezca sin aprobación.

## Evidencia de esta versión

- Fuente del producto y requisitos: `constitution.md`, continuidad v1.3 y **PDF académico v3 original más ZIP Android `(3)` revisados directamente**.
- Control de fase: aprobación del usuario de H03 el 2026-11-05.
- Evidencias disponibles: manifiesto SHA y salida de instalación real de A-000 compartida en Warp el 2026-11-05. Pendientes: revisión y aprobación humana del contenido.
- **NO EJECUTADO:** build Go, pruebas unitarias/de integración/HTTP, despliegue, consultas AWS, instalación de infraestructura, operaciones GitHub, verificación del cliente Mobile.

---

## ADENDA H04 CONSOLIDADA — Fuentes contrastadas (2026-11-05; propuesta v0.2.0)

**Esta adenda complementa, NO sustituye, la versión 0.1.1 previamente instalada. El contenido histórico DRAFT se conserva para trazabilidad.** El usuario pidió preparar el bloque completo H04; se mantienen todos sus documentos en revisión, sin aprobación automática.

- Se revisó directamente el ZIP Android `(3)` de 16 entradas y sus recursos Java/WebView/JS; no contiene `fetch(` en app.js. La demo usa localStorage y datos en memoria, por lo que los endpoints descritos son propuestas pendientes, no rutas implementadas.
- Se leyó el catálogo PDF de 27 páginas y se extrajeron 75 IDs de rutas E001-E075 (33 M, 23 S, 12 E, 7 O). Anexo `../TRAZABILIDAD_75_RUTAS.md` conserva el inventario íntegro.
- Se leyó el PDF original docente v3 (4 páginas), con requisitos reales de dos instancias, balanceador y TLS, Redis, NoSQL, JWT, DevSecOps, CI/CD, observabilidad, costo cero y ML+IA generativa a nivel del proyecto Mobile. El Backend provee únicamente los contratos necesarios.
- **Precedencia inequívoca:** decisión vigente AWS/DynamoDB/Redis, NO Google Cloud/MongoDB Atlas mencionados en el catálogo histórico. No confundir esta aclaración con cambio aprobado de infraestructura.
- **Condición para Gate H04:** cotejar A000 con A010..A110 y fuentes, conservar clasificación y riesgos; bloquear H05 si hubiera contradicción arquitectónica no resuelta. Los riesgos de viabilidad física/financiera se validan de forma obligatoria durante H05, antes de implementar.
