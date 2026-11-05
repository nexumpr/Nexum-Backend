# A-000 — Plan de elaboración y validación del contexto

| Campo | Valor |
|---|---|
| Spec | `A-000-contexto-sistema` |
| Documento | `plan.md` — CÓMO se construye y comprueba este diseño documental |
| Versión/estado | `0.1.1` / **DRAFT** |
| Requisito de entrada | H03 **APPROVED** por el usuario el 2026-11-05; gobierno intacto, no hay otras specs |
| Salida buscada | Contexto y límites listos para revisión humana, sin arquitectura de implementación prematura |

## 1. Método y fuentes, de mayor a menor prioridad

1. Tomar `constitution.md` (GOV-01..18, R-001..060) y el Gate H03 expresamente autorizado como límites superiores.
2. Usar `Nexum_Backend_Contexto_Continuidad_SDD_v1.3_2026-11-05.md` como fuente secundaria de hechos históricos, alcance académico resumido y secuencia H03→H07. No confundirlo con el PDF del profesor original.
3. Cotejo directo realizado durante la consolidación H04: PDF académico original v3 y ZIP Android V6.0.2 `(3)`; 75 rutas propuestas extraídas del PDF catálogo. No inventar APIs existentes.
4. Separar las afirmaciones en **requisito confirmado por gobierno**, **requisito documentado por el resumen**, **pendiente de fuente primaria** y **decisión técnica no tomada**.
5. Dejar trazabilidad GOV→CTX→tarea→checklist para cada contexto declarado. Los anexos de trazabilidad detallada y ADR/RFC se crean solo cuando sean indispensables en su fase.
6. Presentar a revisión humana los cuatro documentos sin marcarlos `APPROVED` automáticamente. Registrar aprobación o correcciones; A-010..A-110 ya fueron redactadas como DRAFT en la consolidación y se revisan conjuntamente en Gate H04.

## 2. Estrategia por artefacto

| Artefacto | Trabajo especificado | Prueba documental |
|---|---|---|
| `spec.md` | Misión, límites, actores, sistemas, flujos conocidos, datos por clases, restricciones, compromisos, pendientes y matriz GOV. | Cada compromiso GOV aparece, cada CTX tiene alcance y no se inventan API/servicios. |
| `plan.md` | Método de investigación y validación de A-000, riesgos y medidas para evitar supuestos. | No incluye implementación, aprovisionamiento o diseño prematuro. |
| `tasks.md` | Actividades verificables con dependencias y responsable de aprobación. | Tareas de documentación/validación, no operaciones AWS ni Go. |
| `checklist.md` | Criterios objetivos, evidencia requerida y salvedades. | Resultado `PASS`, `FAIL`, `NOT_RUN`, `BLOCKED` solo cuando corresponda. |

## 3. Orden de revisión A-000

**Ronda 1 — Integridad:** confirmar directorio `sdd/01-arquitectura-general/A-000-contexto-sistema/`, cuatro Markdown y ninguna otra carpeta de spec. Comprobar SHA-256 desde manifiesto y que los 23 archivos H03 no cambiaron salvo sincronización **documental** explícita de estado aprobada en el paquete.

**Ronda 2 — Consistencia:** cotejar `spec.md` con `constitution.md`, límites de Mobile, carácter obligatorio de dos instancias, balanceador TLS, DynamoDB, Redis, CI/CD/seguridad, costo cero, factibilidad, datos financieros y permisos. Identificar incongruencias, no ocultarlas.

**Ronda 3 — Trazabilidad:** revisar los 18 compromisos GOV, los criterios CTX-001..014 y los 4 documentos. Verificar que dependencias específicas de diseño no se presenten como decididas.

**Ronda 4 — Revisión humana:** resolver observaciones sobre misión, actores y fronteras; aceptar explícitamente fuentes no vistas como **pendientes de cotejo** o facilitar fuentes para resolver una contradicción material. **Aprobación por el usuario**, no por agente.

**Ronda 5 — Registro:** actualizar `memoria.md` de forma separada y verificable al disponer de nueva evidencia. A-000 solo cambia a `APPROVED` con consentimiento humano; las otras once specs ya están redactadas en DRAFT y se ratifican junto con el Gate H04, antes de iniciar H05.

## 4. Matriz de responsabilidad de esta ronda

| Responsabilidad | Actor |
|---|---|
| Propiedad de alcance y aprobación | Usuario responsable de Nexum-Backend |
| Redacción/contraste documental | Asistente, sujeto a `constitution.md` |
| Revisión de gobierno | Perspectiva `sdd-guardian`, sin habilitar delegación automática |
| Coherencia del límite API | Perspectiva `api-contract-guardian`, **sin generar endpoints** |
| Consistencia de datos y finanzas | Perspectiva `go-backend-architect`, **sin programar** |
| Evaluación de requisitos cloud y costos | Perspectivas `aws-terraform-engineer`/`finops-guardian`, **sin diseñar ni aplicar infraestructura** |
| Evidencia y estados | Perspectiva `qa-evidence-verifier`, sin declarar ejecuciones inexistentes |

Los nombres de agentes expresan responsabilidades de revisión futuras; en esta entrega no se invocan modelos/agents ni se amplían sus permisos.

## 5. Controles de cambios y seguridad de instalación

- El paquete se aplica **localmente** mediante un instalador de biblioteca estándar Python invocado desde Warp/PowerShell 7, con verificaciones de hashes del gobierno H03 anterior y del contenido ZIP.
- Se crean **solo cuatro** archivos de A-000. La sincronización de `constitution.md`, `AGENTS.md` y `memoria.md` ajusta estado de Gate H03 ya aprobado, sin alterar GOV, R, agentes, skills u `opencode.json`.
- El instalador comprueba ausencia de `sdd/` antes de instalar, respalda los tres documentos raíz en una carpeta **fuera del proyecto**, verifica después y ofrece reejecución idempotente.
- Si detecta archivos previos imprevistos, hash modificado, enlace simbólico o estado parcial, bloquea. **No borra contenido del usuario** para forzar una instalación.
- El cierre de la instalación solo representa archivos presentes e íntegros, no aprobación de su contenido.
- Se prohíben `git init`, `git push`, `gh repo create`, comandos AWS, Terraform, Go, descargas remotas y ejecución de agentes en este lote.

## 6. Riesgos y cómo se gestionan

| ID | Riesgo | Tratamiento / etapa responsable |
|---|---|---|
| SRC-01 | PDF académico v3 cotejado directamente durante consolidación H04. | Se revisaron sus 4 páginas; mantener matriz de requisitos y resolver discrepancias antes de Gate H04. |
| PROD-01 | Prototipo Android V6.0.2 `(3)` confirmado y fuente JS/Java inspeccionada; API remota inexistente. | Catálogo de 75 rutas incorporado a la matriz H04; no fabricar implementaciones; H06 formaliza cada contrato. |
| FIN-01 | Costo cero versus dos nodos, balanceador y Redis. | Mantener requisitos, evaluar viabilidad en fase económica y elevar ADR incompatible. |
| SEC-01 | SCP limita algunas operaciones AWS. | Reconocer la restricción, no inferir permisos ni intentar bypass. |
| GOVR-01 | El CLI puede seguir bloqueando edición/delegación de agentes. | Mantener `opencode.json` restrictivo; documento se instala con autorización humana externa, sin levantar permisos. |
| DATA-01 | Precisión, monedas, fórmulas y retención todavía no definidas. | No inventar reglas financieras; asignarlas al diseño y contratos cuando haya fuentes. |

## 7. Resultado y condición de término

La entrega se considera **instalada** con los cuatro archivos y hashes correctos, y **apta para revisión** cuando `spec.md` cubra cada GOV, `tasks.md` tenga criterios ejecutables de documentación y `checklist.md` identifique pendientes reales.

El estado `APPROVED` de A-000 **no surge de `PASS` de instalación**. Solo una confirmación humana puede aprobar el bloque H04 y autorizar H05. La redacción previa de A-010..A-110 no constituye aprobación; ningún paso de H05/H06/H07/H08 se abre por completar este plan.

---

## ADENDA H04 CONSOLIDADA — Fuentes contrastadas (2026-11-05; propuesta v0.2.0)

**Esta adenda complementa, NO sustituye, la versión 0.1.1 previamente instalada. El contenido histórico DRAFT se conserva para trazabilidad.** El usuario pidió preparar el bloque completo H04; se mantienen todos sus documentos en revisión, sin aprobación automática.

- Se revisó directamente el ZIP Android `(3)` de 16 entradas y sus recursos Java/WebView/JS; no contiene `fetch(` en app.js. La demo usa localStorage y datos en memoria, por lo que los endpoints descritos son propuestas pendientes, no rutas implementadas.
- Se leyó el catálogo PDF de 27 páginas y se extrajeron 75 IDs de rutas E001-E075 (33 M, 23 S, 12 E, 7 O). Anexo `../TRAZABILIDAD_75_RUTAS.md` conserva el inventario íntegro.
- Se leyó el PDF original docente v3 (4 páginas), con requisitos reales de dos instancias, balanceador y TLS, Redis, NoSQL, JWT, DevSecOps, CI/CD, observabilidad, costo cero y ML+IA generativa a nivel del proyecto Mobile. El Backend provee únicamente los contratos necesarios.
- **Precedencia inequívoca:** decisión vigente AWS/DynamoDB/Redis, NO Google Cloud/MongoDB Atlas mencionados en el catálogo histórico. No confundir esta aclaración con cambio aprobado de infraestructura.
- **Condición para Gate H04:** cotejar A000 con A010..A110 y fuentes, conservar clasificación y riesgos; bloquear H05 si hubiera contradicción arquitectónica no resuelta. Los riesgos de viabilidad física/financiera se validan de forma obligatoria durante H05, antes de implementar.
