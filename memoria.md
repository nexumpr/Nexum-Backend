# memoria.md — Nexum-Backend | Registro factual de H03-H05

**Fecha de actualización del contenido:** 2026-11-05.  
**Versión documental:** 0.5.0.  
**Gates:** H03=`APPROVED`, H04=`APPROVED` (ratificación humana 2026-11-05), H05=`DRAFT/REVIEW`.  
**Etapa vigente:** H05 — infraestructura documental **I-000..I-150 DRAFT/REVIEW** (64 documentos y 5 anexos). **Gate H04 APPROVED** expresamente por el responsable el 2026-11-05. H06/H07 y código siguen bloqueados hasta Gate H05 aprobado.

> Fuente de este registro: resultados de Warp/PowerShell 7 y capturas compartidos por el responsable el 5 de noviembre de 2026; contexto rector v1.3. Esta redacción no equivale a acceder al disco del usuario. El PDF docente original y el prototipo Android no fueron abiertos directamente durante H03.

## 1. Autoridad, identidades y límites

- Constitución rectora `constitution.md` v0.2.0, `AGENTS.md` v0.3.0 y ocho agentes/nueve skills subordinados a la constitución. La constitución permanece en revisión humana y su contenido **no se altera** en este cierre.
- Se construye únicamente **Backend**, contratos API, datos, seguridad, pruebas, IaC e infraestructura. No Mobile/Play Store, landing ni aprobaciones del equipo Android.
- Reglas de gobierno H03 → aprobación humana → H04 (12 especificaciones) → aprobación → H05 (16) → aprobación → H06 (catálogo completo N) → H07 → implementación H08+. Ningún documento de spec ni código antes de la etapa correspondiente.
- Entorno declarado: Windows 11 nativo, Warp Free con PowerShell 7, OpenCode CLI 2.5 confirmado el 2026-11-05 mediante `opencode --version` (2.2.5 fue una lectura historica). GitHub previsto `nexumpr`, AWS de trabajo `713797602090`/`us-east-2`, perfil `nexum`; no confundir con cuenta administrativa.
- Repositorio anterior archivado `nexumpr/Nexum-Backend-Legacy-20261105`. Repo nuevo `nexumpr/Nexum-Backend` no creado según último estado reportado. `nexum-privacidad` fuera de alcance.
- No Docker, WSL, Windows virtualizado, `git push`, terraform apply/destroy, despliegues AWS, secretos ni cambios de cobro durante H03.

## 2. Evidencia cronológica compartida por el responsable

| Hito | Hecho respaldado por salidas compartidas | Estado |
|---|---|---|
| H00–H02 | Limpieza operativa histórica y respaldos; ausencia física posterior SSM/bucket no comprobada independientemente | Cerrado operativamente con limitaciones documentadas en v1.3 |
| H03-A | `Preflight`, `Crear` y `Verificar` correctos; `constitution.md` SHA `172162BED16BD22285F0223C6265AE9A706ED12ACC4894B0374A9BA280AF09FB` antes de la revisión H03-B | PASS instalación |
| H03-B | Seis archivos instalados, seis SHA coincidentes; respaldo `C:\Proyectos\Nexum\Respaldos\H03-B_20261105_100945_1a37769b\`; JSON parseado en PowerShell | PASS instalación y sintaxis JSON |
| H03-C | Actualizados `AGENTS.md`, `memoria.md` y creados ocho agentes; verificación de hashes y conteo ocho; respaldo `C:\Proyectos\Nexum\Respaldos\H03-C_20261105_102358_1ebd1759` | PASS instalación/estructura |
| H03-D, primer intento | Falla real: conflicto de nombres `$Changed`/`$changed` del instalador; rollback reportado. No atribuir `PASS` a esta ejecución fallida | FAILED histórico, luego corregido |
| H03-D, instalación final | Script `InstalarSeguro` instaló `AGENTS.md`, `memoria.md` y nueve skills, con 11 hashes coincidentes. Respaldo `C:\Proyectos\Nexum\Respaldos\H03-D_20261105_105830_97334894` | PASS instalación/estructura |
| Auditoría Gate H03 / estructura | `PASS: 23 archivos íntegros (6 raíz, 8 agentes, 9 skills)`; revisadas reglas declaradas y ausencia de specs/código | PASS estructural |
| Auditoría Gate H03 / OpenCode, auditor anterior | `FAIL: TIMEOUT: opencode  excedió 20 segundos`; error del auditor por variable `$Args`, posteriormente corregido | FAILED histórico (auditor), NO es fallo del CLI |
| Diagnóstico posterior OpenCode 2.5 | `opencode --version`, `debug config` y `debug agents` completados; 8 perfiles reconocidos; 23 hashes correctos | PASS diagnósticos de carga/estructura |
| Prueba posterior con inferencia | `INCONCLUSIVE: no hubo evidencia estructurada de denegacion shell (codigo proceso 1)`; no hubo evento de herramienta ejecutada ni denegada | INCONCLUSIVE histórico, reemplazado por validación directa del motor |
| Evaluación nativa de permisos OpenCode v2.5 (2026-11-05) | Servidor local autenticado; ocho agentes reconocidos; seis `deny` por agente, cuatro bloqueos globales de `build`; **54 decisiones (53 `DENY`, 1 `ALLOW`)**; sesión eliminada; 23 hashes intactos. Archivo de evidencia `C:\Proyectos\Nexum\Auditoria\H03-GATE\permiso-nativo-20261105T182234-a2e4bc.json` | **PASS_RUNTIME**, según salida de Warp del responsable |

## 3. Diagnóstico del timeout: hallazgo en el auditor heredado

En `H03-GATE.ps1`, la función de ejecución usaba el parámetro **`$Args`**, que coincide (sin distinción de mayúsculas) con la variable automática **`$args`** de PowerShell. El texto del fallo muestra `opencode  ` sin subcomando, consistente con que se ejecutara el CLI interactivo en lugar de `opencode --version`. Además, el timeout anterior de 20 s no discrimina entre error de comando y arranque lento del CLI. **Hipótesis de causa apoyada en código y salida, no prueba runtime concluyente**. El comando de diagnóstico fue corregido posteriormente y devolvió PASS en Windows. Se conserva el fallo anterior como histórico del auditor, sin atribuirlo al CLI.

La comprobación estática de `opencode.json` mostró políticas de denegación para `shell`, `subagent`, `skill` y accesos externos; **la carga de OpenCode y sus ocho agentes está verificada; con posterioridad se comprobó la evaluación dinámica del motor mediante la API local autenticada (54 decisiones); no se ejecutaron herramientas ni modelos**. No se activan agents, skills, shell ni modos autoaccept como parte de este paquete.

## 4. Estado actual y documentos permitidos

| Entrega/fase | Estado |
|---|---|
| H03-A | Instalado y verificado según consola |
| H03-B | Instalado y verificado según consola |
| H03-C (8 agentes) | Instalado y verificado según consola |
| H03-D (9 skills) | Instalado y verificado según consola |
| Integridad H03 (23 archivos) | PASS según auditoría estructural Windows, antes de esta actualización de `memoria.md` |
| OpenCode CLI/config/agentes | PASS: versión 2.5, `debug config` y ocho agentes reconocidos en Windows |
| Evaluación efectiva del motor de permisos | **PASS_RUNTIME:** 54 decisiones devueltas por API local de OpenCode: 53 `DENY`, 1 `ALLOW`; no se ejecutaron herramientas prohibidas |
| Gate integral H03 | **APPROVED** — el responsable ratificó H03 expresamente el 2026-11-05; archivo actualizado en H04/A-000 |
| H04 (A-000..A-110) | **APPROVED** por decisión humana expresa 2026-11-05; 12 specs/48 documentos y 3 anexos verificados. |
| H05 | **DRAFT/REVIEW** autorizado, 16 specs/64 docs y 5 anexos entregados; Gate pendiente. |
| H06, H07, H08+ | NOT_STARTED / BLOQUEADOS hasta Gate H05 |

Los archivos raíz presentes son `constitution.md`, `AGENTS.md`, `memoria.md`, `opencode.json`, `README.md`, `.gitignore`. Directorios `.opencode/agents` (8) y `.opencode/skills` (9). No hay código Go, Terraform, workflows ni specs nuevos.

## 5. Decisiones y riesgos sin falsear

- AWS: dos instancias reales concurrentes con balanceador HTTPS, DynamoDB, Redis compartido, DevSecOps, CI/CD, IaC, rendimiento y observabilidad siguen **obligaciones de diseño e implementación futura**; no demos ni mocks.
- **FIN-01:** exigencia de costo de uso cero versus créditos AWS observados USD 120 el 2026-11-05. La factibilidad real no está probada; analizar en H05 y bloquear ADR económicamente inviable.
- **SEC-01:** SCP limita IAM/OIDC; no sortear controles ni inventar acceso.
- **SRC-01:** PDF profesor original no cotejado directamente en H03, solo documento v1.3, que lo resume. No atribuir requerimientos externos inexistentes.
- **PROD-01:** el prototipo Android vigente se requerirá para inventario de operaciones antes de cerrar A-060 y H06; siete endpoints históricos no son límite.
- **GOV-01:** Gate H03 aprobado expresamente por el responsable el 2026-11-05. No equivale a aprobación de A-000 ni del SDD completo.
- Sin ADR/RFC nuevos ratificados, sin PR/CI/URL/deploy/test HTTP/recursos del nuevo Backend; estado de implementación `NOT_STARTED`, despliegue `NOT_DEPLOYED`.

## 6. Evidencia verificada y límites de lectura

- Resultado de `opencode --version` en Windows: **PASS**, se identificó OpenCode 2.5; evidencia de salida compartida el 2026-11-05.
- Resultado de `opencode debug config` y `opencode debug agents`: **PASS** (fuentes listadas, ocho perfiles reconocidos); el diagnóstico no volcó configuración sensible.
- Intento de prueba de denegación con inferencia: **INCONCLUSIVE**, OpenCode retornó código 1 y no emitió evento estructurado de herramienta. No puede atribuirse a una denegación probada ni a permiso vulnerado. **No repetir inferencias automáticamente**.
- **Nueva validación del motor (2026-11-05): PASS_RUNTIME**. El responsable mostró una salida de Warp con `53 DENY`, `1 ALLOW`, 54 decisiones totales, reconocimiento de los ocho agentes, eliminación de sesión y comprobación posterior de los 23 hashes. Referencia local de evidencia: `C:\Proyectos\Nexum\Auditoria\H03-GATE\permiso-nativo-20261105T182234-a2e4bc.json`. **El archivo JSON no fue adjuntado ni leído por este documento; la evidencia directa disponible es la salida compartida.**
- **El ensayo anterior con inferencia continúa registrado como INCONCLUSIVE histórico**, pero ya no es el criterio empleado para verificar decisiones del motor. Esta validación no prueba el comportamiento futuro de todo modelo ni de controles del sistema operativo, y no autoriza ampliar permisos.
- Aprobación humana Gate H03: **APPROVED el 2026-11-05**, ratificada textualmente por el responsable tras `PASS_RUNTIME`.

## 7. Dictamen actualizado de cierre técnico (2026-11-05)

- **PASS estructural:** 23 archivos íntegros; seis archivos raíz, ocho agentes, nueve skills y ausencia de documentos de H04+ y código.
- **PASS de carga CLI:** OpenCode 2.5 reconoce ocho perfiles y responde a comandos de diagnóstico en Windows.
- **PASS_RUNTIME de evaluación del motor:** 54 decisiones de permisos consultadas directamente por API local autenticada, con 53 `DENY` y un `ALLOW`; ocho agentes y cuatro bloqueos del agente integrado `build` verificados. La sesión temporal se eliminó y los 23 archivos permanecieron intactos, según salida compartida por el responsable. No se ejecutó ninguna herramienta prohibida.
- **Histórico no ocultado:** el ensayo previo con inferencia fue `INCONCLUSIVE`; se sustituye para este criterio por evaluación directa del motor, no por un PASS inventado.
- **Seguridad operacional:** la contraseña temporal del servidor apareció en una captura previa. Detener el servidor con `Ctrl+C` y **no reutilizar esa contraseña**. Nunca incluir secretos en el SDD ni compartirlos por chat.
- **APPROVED (actualización de estado):** el responsable aprobó el Gate H03 el 2026-11-05 tras revisar la evidencia técnica; los scripts no otorgaron tal aprobación.
- **H04 autorizado parcialmente:** crear exclusivamente los cuatro documentos de **A-000**; A-010..A-110, H05/H06/H07 e implementación siguen sin iniciar. No hay código, repo nuevo ni infraestructura.

**Siguiente paso único:** solicitar aprobación humana explícita del Gate H03. Solo si se concede, iniciar la entrega **H04 / A-000** con exactamente sus cuatro documentos completos y sin adelantarse al resto de H04 o a H05.


## 8. Ratificación humana H03 e inicio controlado H04/A-000 (2026-11-05)

- **Autorización expresada por el responsable en este chat:** «APRUEBO GATE H03 de Nexum-Backend y autorizo comenzar H04 / A-000». No es aprobación del SDD completo ni de A-000.
- **Antecedente:** registro H03 v0.3.3 instalado/verificado; 54 decisiones del motor de OpenCode (53 `DENY` y 1 `ALLOW`) observadas en Warp; evidencia remota disponible solo como salida compartida y ruta local, no lectura directa del archivo JSON.
- **Estado del gobierno H03:** `APPROVED`. Los 23 archivos fundamentales fueron verificados al aprobar, antes de la sincronización de metadatos.
- **Único bloque activo:** `sdd/01-arquitectura-general/`, **única especificación de esta ronda `A-000-contexto-sistema`**, cuatro documentos y estado `DRAFT`. No se autorizan A-010..A-110 por esta aprobación.
- **Sin permisos elevados:** `opencode.json`, ocho agentes, nueve skills y los controles `DENY` se conservan **sin cambios**. La redacción de A-000 se instala mediante un proceso local autorizado separado, no mediante delegación sin permisos.
- **Alcance y fuentes:** el PDF académico y el prototipo Android no fueron adjuntados ni cotejados directamente en esta ronda. Sus requisitos se identifican como provenientes del documento de continuidad v1.3; solicitar el prototipo vigente antes de cerrar A-060.
- **Riesgos abiertos:** factibilidad de costo cero, SCP AWS, modelado de acceso de operaciones completas; ninguna decisión se presenta como resuelta.
- **Estado de implementación:** `NOT_STARTED`. **Despliegue:** `NOT_DEPLOYED`. **Nuevo repositorio remoto:** todavía no creado según último estado verificable.
- **Único siguiente gate:** revisión y aprobación humana de **A-000**. Hasta entonces A-010 no se genera.


## 9. Consolidación y revisión previa al Gate H04 (2026-11-05)

- Evidencia del responsable en Warp: `Nexum_H04_LISTO_POWERSHELL7.zip` instaló las doce specs, 48 documentos y tres anexos; Preflight/Instalar/Verificar con `PASS`, base H03 intacta.
- Corrección puntual: `Nexum_H04_REVISION_PUNTUAL_20261105.zip`, modificó solo cuatro documentos A-000 y `A-060/spec.md`; verificación posterior `PASS: 74 hashes` con cinco revisiones; otros 69 archivos sin cambios.
- El catálogo PDF V6.0.2 contiene 75 contratos propuestos, no endpoints ya publicados. El ZIP Android es demo local Java/WebView. Las decisiones de tecnología vigentes son AWS, DynamoDB y Redis, en lugar del stack histórico del catálogo.
- Riesgos siguientes (no solventados por H04): factibilidad de AWS Free/costo cero frente a dos instancias + HTTPS/balanceador + Redis; acceso IAM/SCP; aprovisionamiento físico de DynamoDB, pruebas de red y gastos efectivos. Se deben resolver en H05 con evidencia, sin declararlos `PASS` anticipado.
- **Gate H04 sigue DRAFT/REVIEW**. Solamente la ratificación humana expresa del bloque H04, con referencia a la versión y a los riesgos conocidos, habilita comenzar H05. No se creó H05.
- La documentación histórica de H03/A-000 se conserva como trazabilidad del momento en que fue escrita; este estado actualizado prevalece para el inventario vigente sin sustituir las reglas de `constitution.md`.


## 10. Gate H04 aprobado e inicio de H05 (2026-11-05)

- Responsable compartió Warp: `Nexum_H04_GATE_PREPARACION_20261105.zip` instaló y verificó exclusivamente `AGENTS.md`, `memoria.md` y 74 hashes. Mensaje humano posterior: **«APRUEBO GATE H04 de Nexum-Backend y autorizo comenzar H05 — Infraestructura»**. Esta es aprobación humana documentada de H04.
- H05 se entrega en lote documental de dieciséis specs (I-000..I-150, 64 documentos `spec/plan/tasks/checklist`) más cinco anexos de costos, cobertura y lectura de cuenta. Un `PASS` de instalación no es aprobación del Gate H05.
- Requisito docente costo cero, **dos instancias simultáneas**, balanceador HTTPS, Redis real y NoSQL DynamoDB siguen obligaciones innegociables. **Viabilidad y costo de Free/SCP NO ESTÁN VERIFICADOS EN LA CUENTA REAL**; un crédito que compensa precio no demuestra servicio sin costo. No crear recursos ni habilitar plan Paid sin aprobación.
- Fuente funcional 75 rutas propuesta, Java/WebView demo sin HTTP; fuente histórica GCP/MongoDB descartada. H06 no iniciada y contratos completos requieren su etapa posterior.
- No modificar `constitution.md`, `opencode.json`, los 8 agentes ni 9 skills; sus permisos probados en H03 permanecen restringidos. No adelantar implementación.


## Anexo factual H05/H06/H07 — 2026-11-05 (preparación de publicación)

- **H05:** el responsable instaló V2 (13 archivos), V3 (11) y V4 (4), cada uno con verificaciones SHA `PASS`; aprobó documentalmente el diseño con condicionantes (sin aprobar AWS `Create`/`apply`). Cuenta Free `READ_OK`, cuota EC2 16 vCPU, listado CloudFront VPC Origins leído; crear VPC origin, SKU Redis y costos/IVA reales continúan pendientes.
- **H06:** el responsable ejecutó `--preflight`, instalación y `--verify` de `Nexum_H06_COMPLETO_75_ENDPOINTS_SDD_OPENAPI_20261105.zip`. Reportó `313 CREATE`, `H06_CONTRACT_PASS: 75 operations; 300 specs; 33M+23S+12E+7O` y `VERIFY_PASS` por SHA. **No ha emitido todavía ratificación expresa del Gate H06.**
- **H07:** se preparan auditoría y empaquetado para publicar el SDD completo, sujetos a Gate H06, Gate H07 y autorización separada de publicación GitHub. La entrada válida actual está en `README.md` y `gobierno/H07/ESTADO_Y_GATES.md`. La historia de H03/H04/H05 de las secciones anteriores no debe usarse como estado vigente.
- **Infraestructura:** operación completamente por Terraform/CI/CD/OpenCode será futura obligación verificable, no logro actual. Cero dinero personal, sin activar Paid, dos EC2 siempre activas junto con DynamoDB/Redis/red; apagado solo auxiliar. El SDD no elimina riesgos de facturación/cierre de Free, AWS SCP/IAM, TLS de origen, ni falta de proveedor de ML/LLM.
- **Fuentes:** no hubo cotejo independiente, durante H07, del ZIP Android original o del PDF docente originales; cualquier diferencia requerirá RFC y cambios controlados antes de afirmar conformidad final.
