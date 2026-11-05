# Contrato transversal para todas las operaciones E001–E075

**Versión:** 1.0.0-draft, H06 REVIEW. URL `https://api.example.invalid` es placeholder, **NO** una API pública. Todos los cuerpos son JSON UTF-8; respuestas financieras y de credenciales `Cache-Control: private, no-store`; compresión solo con análisis de BREACH/secretos, límites de tamaño y coste.

## HTTP y contrato

- Base `/api/v1`, OpenAPI 3.1.0; `operationId` estable `nexum_e###`; por lo general recursos propios bajo sujeto JWT. Excepciones públicas expresamente declaradas: E001, E002, E003 (refresh opaco en body), E005, E006, E011, E015, E022, E023, E070–E073.
- **JWT de acceso** vida inicial 15 min, firmante y algoritmo fijados en configuración cifrada, verificación de `iss`, `aud`, `exp`, `nbf`, `jti`, `sub`, `kid`, leeway máximo 30 s; refresh opaco de 256 bits aleatorios, digest persistido, rotación atómica por familia; TTL configurable 30 días e invalidación real en Redis/Dynamo; 1 usuario solo accede a sus recursos, ajeno 404 (no revelar existencia).
- Password `Argon2id` calibrado para nodos micro y probado en carga real antes de publicar; normalizar email (trim + lower-case dominio y reglas de normalización aprobadas); protección contra enumeración, fugas temporales, CSRF únicamente si se usa cookies (tokens Bearer en JSON por defecto, no suponer navegador con cookies).
- **Errores:** `400 INVALID_REQUEST` (malformación), `401 UNAUTHORIZED`, `403 FORBIDDEN`, `404 NOT_FOUND` (propiedad), `409 CONFLICT` (duplicado/idempotencia divergente), `412 VERSION_CONFLICT`, `422 BUSINESS_RULE_VIOLATION`, `428 PRECONDITION_REQUIRED` (If-Match faltante), `429 RATE_LIMITED`, `500 INTERNAL_ERROR`, `503 DEPENDENCY_UNAVAILABLE`; `traceId` no PII. `Retry-After` en 429/503 cuando proceda. Nunca devolver stack traces, correo existente, IDs AWS, tokens o payload sensible.
- **Precondiciones:** todos los `PATCH`/`PUT` que actualizan recurso versionado exigen `If-Match` según OpenAPI; ETag fuerte `"v<N>"` de versión persistida (no `updatedAt`); cualquier operación escrita, financiera o de cuenta, asegura unicidad/doble envío con compare-and-swap e idempotencia donde se indica.
- **Dinero:** cadena decimal de dos dígitos, moneda inicial `MXN`; convertir a centavos `int64`, evitar float; valida `amount>0` para cargos/aportes y `amount>=0` para saldo/progreso; límites de 12 enteros antes del decimal documentados; overflow 422. Fecha UTC ISO8601, zona IANA para calendario del usuario.
- **Paginación:** `limit=25` por defecto, `1..100`, `cursor` opaco firmado con HMAC, ligado a usuario, filters/version y expiración, jamás offset; retorno JSON `{items:[],nextCursor?:string}`; `nextCursor` debe ser null/ausente cuando termina. No `Scan` global, queries por owner. Para catálogos con volumen pequeño, ese mismo wrapper es válido aunque nextCursor ausente.

## Limites (valores iniciales de diseño, calibrar con pruebas y FinOps)

- Cuerpo JSON ≤256 KiB; voz texto ≤500 caracteres; transacción de ahorro ≤12 metas y límites nativos de Dynamo; exportación E020 paginada y limitada para no colapsar microinstancias.
- Prelogin: 5/min/IP en login/reset y 10/min/email; operaciones autenticadas 60/min/sujeto, financieras mutantes 20/min/sujeto, dashboard 30/min/sujeto, health balanceador sin limitaciones que rompan readiness. Definir bursts efectivos en Redis con fail-safe y headers, medir false positives en CI/QA.
- Fetch/búsquedas: timeout por request ≤15 s (diseño), 2 reintentos con jitter solo para operaciones idempotentes, circuito abierto con 503 al fallar dependencias; nunca reintentar ciegamente transacción financiera.
- Observabilidad: requestId/correlationId, métricas latency p50/p95, errores y cache hits; registro JSON minimizado por API y servicio, sin password/refresh/email/token/PII ni texto libre de finanzas. No afirmar SLO numérico probado antes de H08.

## Reglas de negocio que CI debe preservar

- Ingreso base es previsión, nunca un movimiento real por configurar E025; resumen separa estimaciones de transacciones.
- Recurrentes E033 no descuentan; registrar pago E037 genera exactamente 1 gasto; borrar regla no borra pagos históricos.
- Aportaciones E044 pueden dividirse entre metas, centavos suman exactos; E047 revierte **la misma** aportación una sola vez y conserva historia; E042 archiva metas sin destruir movimientos.
- Presupuestos usan gasto ejecutado, no promesa recurrente; límite 0 desactiva esa categoría; racha E056 días locales IANA y contribuciones confirmadas, reversa corrige días.
- Analítica E054 de gastos hormiga: al menos 3 movimientos compatibles en 14 días para afirmar patrón; si no, evidencia insuficiente. E052/E055 no pueden fingir ML ni texto generativo: documentar modelo, evidencia y costos en `IA_ML_Y_NOTIFICACIONES_H06.md`.
- Eliminación E014: reautenticación, revocación de sesiones inmediata, tombstone lógico, purga async por lotes con política documentada, retención justificable; cualquier acción tras tombstone 401/403 coherente.

## Infraestructura y operación

AWS Free, sin dinero personal ni cambio a Paid, 45 días, 2 EC2 Go en 2 AZ, DynamoDB, Redis real compartido, CloudFront+ALB privado HTTPS público; AWS costos y permisos no certificados en H06. Una sola ruta nueva por release autorizado H08 y pruebas reales independientes del cliente Android. Operar infraestructura SOLO con Terraform y pipelines (plan/apply/status/suspend-aux/resume-aux/rollback/reconcile), no operaciones manuales, salvo autorización y excepción registrada. Gates humanos para mutaciones AWS y cambios destructivos.
