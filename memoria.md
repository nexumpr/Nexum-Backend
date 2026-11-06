# memoria.md — estado factual vigente de Nexum-Backend

**Actualización:** 2026-11-05. **Fuente:** salidas Warp y capturas comunicadas por el responsable; baseline consolidado y revalidado localmente. No se presume acceso en tiempo real al Windows ni a AWS. El historial anterior completo permanece en Git, commit inicial `84be72d`; su redacción histórica **no constituye instrucciones vigentes**.

## Gates y publicación (verdad operativa actual)

- **H03:** APPROVED por responsable; 8 agentes y 9 skills presentes. Prueba de motor de permisos H03 indicada: 54 decisiones (53 DENY, 1 ALLOW), según evidencia compartida; las herramientas continúan denegadas en `opencode.json`.
- **H04:** APPROVED por responsable; 12 specs A-000..A-110 con 48 documentos. No modificar sin RFC.
- **H05:** APPROVED_DOCUMENTAL_CONDICIONADO; 16 specs I-000..I-150 con 64 documentos; V2, V3 y V4 instalados, coste y permisos AWS aún no certificados; **sin autorización AWS real**.
- **H06:** INSTALLED/VERIFY_PASS: 75 operaciones E001..E075 con 300 documentos y OpenAPI 3.1, 33 maqueta + 23 soporte + 12 esperadas/simuladas + 7 opcionales. **Gate humano H06 aún pendiente**.
- **H07:** el repositorio privado `nexumpr/Nexum-Backend` **sí fue creado y publicado**, primer commit `84be72d` el 2026-11-05. El workflow documental de ese commit falló con exit code 2; se identificó un CSV CRLF con SHA distinto tras checkout Linux. El usuario **autorizó explícitamente corregir estructura y publicar la corrección**. Gate formal H07 todavía pendiente de validación CI corregida y ratificación expresa.
- **H08+:** NOT_STARTED. No se ha autorizado nueva política de permisos OpenCode ni despliegue AWS, aunque la preparación del trabajo de equipo mediante revisión y PRs documentales ya puede hacerse.

## Requisitos innegociables de implementación futura

- Solo Backend Go, HTTP/OpenAPI, DynamoDB NoSQL, Redis OSS real, AWS CloudFront/ALB privado, dos EC2 Go activas simultáneamente, HTTPS público, IaC Terraform, pruebas, CI/CD, IAM OIDC, DevSecOps, logs, paginación/rate limiting y recuperación real.
- Infraestructura 100 % orquestada por IaC y pipelines, no por consola manual; acciones `plan`, `apply`, `status`, `suspend-aux`, `resume-aux`, `reconcile`, `rollback`, `destroy-aux`. Mantener esenciales operativos; solo auxiliares suspendibles.
- Plan AWS Free, cero pesos personales, 120 USD de créditos referidos y al menos 45 días estimados, sin promover a Paid; no afirmar presupuesto garantizado ni gastos $0 sin lectura real de cuenta. Redis SKU exacto, impuestos, CloudFront CreateVpcOrigin, conectividad y saldo/vencimiento están BLOQUEADOS antes de aplicar.
- No Mobile, Play Store ni frontend dentro de Backend. Equipo Mobile puede solicitar E076+ por RFC de compatibilidad, sin decidir Gates del Backend.

## Evidencias, diferencias y limitaciones

- Las verificaciones **locales** de H03/H04/H05/H06/H07 se reportaron desde Warp; no prueban ejecución de endpoints ni consumo AWS. H06 está completo documentalmente para el baseline actual, no es garantía de que nunca haya ampliaciones.
- El PDF docente original y último ZIP Android no fueron cotejados nuevamente de manera independiente para H07. Cualquier diferencia posterior debe registrarse, clasificarse y resolverse por RFC antes de declarar cumplimiento integral de fuentes.
- El commit inicial dejó `gobierno/H07/`, una carpeta que el responsable **no había autorizado como ampliación de estructura**. Se acordó trasladar los documentos auxiliares a `sdd/_control/H07/`, dejando **exactamente tres bloques de especificaciones**.
- Falla CI inicial: el CSV `FINOPS_45_DIAS_DESGLOSE_V2.csv` fue el único CRLF en el baseline, Git normalizó a LF y SHA del snapshot no coincidió. Solución: normalizar CSV, `.gitattributes` para LF, rehash de todos los archivos (sin eliminar controles), regresión en checkout Windows/Linux, informe de causa raíz.
- Historial específico de H03-D: hubo fallo en script PowerShell, corregido posteriormente; un ensayo de inferencia resultó inconcluso y fue sustituido por prueba del motor. No borrar ni reescribir commits para ocultarlo.

**Próximo paso de control:** verificar corrección H07 en un PR y checkout Linux, obtener CI `success`, integrar sin `--force`, presentar Gate H06 y H07 para ratificación formal; después autorizar H08 e implementar primero arquitectura, luego IaC 100 % y endpoints. `AGENTS.md` es la única instrucción operativa vigente de agentes, subordinada a `constitution.md`.
