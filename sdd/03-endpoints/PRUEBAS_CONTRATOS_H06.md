# Programa de validación H06 → H08 para 75 rutas

**Hoy:** solo verificaciones estáticas y pruebas sobre datos sintéticos del contrato; pruebas HTTP, CI/CD en AWS, DynamoDB y Redis = NOT_RUN. Ningún PASS aquí significa servidor implementado.

| Suite | Qué debe comprobar | Herramienta prevista en H08 |
|---|---|---|
| C-01 | Las 75 operaciones OpenAPI tienen método/path/operationId, auth y 2xx correctos, ejemplo-schema coherente | auditor local, OpenAPI schema, contract tests Go |
| C-02 | 400/401/403/404/409/412/422/428/429/500/503, no enumeración | integración HTTP/negativos |
| C-03 | JWT expirado/alterado, revocación/rotación concurrente y borrado de cuenta en dos nodos | end-to-end y pruebas paralelas |
| C-04 | centavos exactos, 2 movimientos no duplicados por reintento, aportación multi-meta atómica y reversa única | unitarias + TransactWriteItems real |
| C-05 | cursores HMAC, paginación filtrada estable, consultas sin Scan y lecturas propias | Dynamo test/CloudWatch + HTTP |
| C-06 | Redis compartido, invalida dashboard y sesiones de nodo A/B, caída y recuperación | integración real |
| C-07 | JWT/secretos, SAST, SCA, gitleaks, IaC scan, DAST a entorno autorizado | Actions backend |
| C-08 | dos EC2 independientes y ALB/CloudFront HTTPS, salud /live /ready, failover | pruebas con evidencia AWS y HTTP |
| C-09 | presupuesto Free, 45 días, proyección, preflight FinOps, stop/recovery de auxiliares por IaC | AWS lectura+Terraform/CI autorizados |
| C-10 | ML entrenado+inferencia real, texto generativo real, PII cero, costo acotado | evaluación de modelo y seguridad |
| C-11 | consentimientos, retención/borrado, exportación aislada y notificaciones sin opt-in | seguridad/privacidad |

**Criterios de salida:** 100% de rutas documentadas, errores/cuerpos/ejemplos compatibles, 300 archivos completos, matriz de cobertura H04→H06, checksum de archivos. En H08 se evidencia por release: commit/artefacto, PR, aprobación, CI, pruebas unitarias/contrato/integración, endpoint real HTTP, datos reales de prueba, rollback, verificación de los anteriores y costos; no avanzar si falla una etapa.

**Riesgo de fuente:** el prototipo/PDF del docente no se adjuntaron de nuevo; H04 ya registra su análisis. H07 debe cotejar si se adjuntan, sin falsificar contraste actual.
