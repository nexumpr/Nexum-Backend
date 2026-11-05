# Registro de gates y autoridad (corte 2026-11-05)

| Gate | Estado | Evidencia | Autoriza | NO autoriza |
|---|---|---|---|---|
| H03 | APPROVED | ratificación humana previa + pruebas OpenCode de permisos | usar constitución/agentes/skills existentes | cambiar permisos/ejecutar agentes sin gate |
| H04 | APPROVED | ratificación humana previa + revisión 12 specs | preservar arquitectura rectoría | alterar arquitectura sin RFC |
| H05 | APPROVED_DOCUMENTAL_CONDICIONADO | usuario instaló 13+11+4 archivos, V4 verificada y aprobó diseño | escribir H06 y preparar H07 | AWS `apply`/Paid/crear recursos/certificar costo |
| H06 | INSTALLED/VERIFY_PASS; **GATE PENDIENTE** | 313 archivos; 75 ops / 300 docs / OpenAPI 3.1; salida Warp | preparar evaluación H07 por solicitud actual | declarar endpoint implementado/Gate APPROVED |
| H07 | REVIEW_READY; **GATE PENDIENTE** | auditoría local y ZIP H07 | presentar Gate + plan publicación | publicación automática GitHub |
| H08+ | NOT_STARTED | ninguna | una vez Gates y autorización específica | mutaciones AWS no autorizadas |

**Los últimos dos PASS son de documentos, no producción.** El usuario solicita preparar GitHub, **no ha aprobado explícitamente el Gate H06 ni H07**, ni autorizado un push concreto. El acto futuro de aprobar puede usar: `APRUEBO GATE H06 Y GATE H07 DOCUMENTALES; AUTORIZO PUBLICAR EL SDD INICIAL EN nexumpr/Nexum-Backend` después de revisar los bloqueos y la operación exacta; no se pre-registra esa frase como recibida.

**Bloqueos técnicos pre-despliegue:** precio SKU Redis oficial, elegibilidad y permiso `CreateVpcOrigin`/IAM/SCP, verificaciones de tráfico/TLS en VPC origin, AWS Free saldo/vencimiento/IVA, topología de egreso sin NAT, estado de Terraform/seguridad/budget hard fail, pruebas 2 EC2 + DynamoDB + Redis reales, proveedor de ML/LLM, prueba funcional HTTP. La aprobación documental NO convierte estos puntos en PASS.
