# H04 — Dictamen consolidado para revisión humana

**Estado:** DRAFT, NO APPROVED. Incluye 12 specs x 4 archivos (48 documentos) y esta matriz. La autorización del usuario de preparar arquitectura completa **no implica aprobación de su contenido**. Las especificaciones H05/H06 no se generan.

## Resultado del análisis de las fuentes reales

- ZIP Android actual `(3)`: 16 archivos; Java/WebView/HTML/JS; `app.js` sin llamadas `fetch(`, usa `nexumV6Account` y `nexumV6Profile` en localStorage; datos demo y memoria. Mismo comportamiento funcional que el PDF de rutas de V6.0.2; no considerar HTTP existente.
- Catálogo PDF `(2)`: exactamente 75 ID E001..E075, niveles **33 M / 23 S / 12 E / 7 O**; contract draft, no API operativa. Mantenerlo como referencia funcional.
- Requerimientos docente v3: 4 páginas; se leyeron los apartados 1..20, incluidos AWS/Azure/GCP como opciones docentes, >=2 instancias Backend, costo cero, NoSQL, balanceador/HTTPS, Redis, JWT, IaC, observabilidad, CI/CD, ML/IA en Mobile, push y privacidad.
- Decisión de proyecto más reciente y prioritaria: AWS + Go + DynamoDB + Redis, no MongoDB/Google Cloud del PDF histórico.

## Auditoría de diseño por especificación

- `A-000` — Contexto del sistema y frontera de responsabilidad; 8 decisiones; 3 criterios de aceptación; 3 riesgos explícitos.
- `A-010` — Stack Go, versiones y dependencias; 8 decisiones; 3 criterios de aceptación; 2 riesgos explícitos.
- `A-020` — Clean Architecture y modularidad; 8 decisiones; 3 criterios de aceptación; 2 riesgos explícitos.
- `A-030` — Servidor HTTP, middleware y políticas de transporte; 8 decisiones; 3 criterios de aceptación; 2 riesgos explícitos.
- `A-040` — Contratos REST/OpenAPI y versionamiento; 8 decisiones; 3 criterios de aceptación; 2 riesgos explícitos.
- `A-050` — Autenticación, sesiones, JWT y privacidad; 8 decisiones; 3 criterios de aceptación; 2 riesgos explícitos.
- `A-060` — Modelo DynamoDB, consultas, índices y transacciones; 10 decisiones; 3 criterios de aceptación; 3 riesgos explícitos.
- `A-070` — Dinero exacto, idempotencia, reglas y resiliencia; 8 decisiones; 3 criterios de aceptación; 2 riesgos explícitos.
- `A-080` — Redis compartido, caché, rate limiting y paginación; 8 decisiones; 3 criterios de aceptación; 2 riesgos explícitos.
- `A-090` — Observabilidad, alertas y salud; 8 decisiones; 3 criterios de aceptación; 2 riesgos explícitos.
- `A-100` — Pruebas, calidad y DevSecOps; 8 decisiones; 3 criterios de aceptación; 2 riesgos explícitos.
- `A-110` — Versionado, contribución y gates humanos; 8 decisiones; 3 criterios de aceptación; 2 riesgos explícitos.

## Bloqueos que NO deben posponerse a H08

1. H05 debe demostrar disponibilidad real de servicios AWS en cuenta Free, dos nodos simultáneos, balanceador HTTPS y Redis real, con costo de desembolso cero según criterio del profesor; créditos temporales o topologías simuladas NO prueban esta condición. Si inviable, detener el Gate, ofrecer evidencia y pedir decisión formal de usuario/docente antes de ejecutar.
2. El diseño final de DynamoDB A-060 necesita el plan físico de particiones, tabla(s), GSI y costos (H05), y un catálogo 75/75 con payloads concretos (H06). H04 fija patrones lógicos e invariantes, no presupone recursos existentes.
3. H06 debe comprobar si el catálogo fue exhaustivo respecto a ML/IA del profesor y push real, y decidir técnicamente qué interacción requiere Backend; no responsabilizar de implementar Mobile.
4. Seguridad: no abrir permisos shell/subagent/skill de OpenCode por el solo hecho de aprobar H04; una transición controlada pertenece a plan aprobado posterior.
5. SDD tras aprobar H07 es punto de partida de implementación, pero H08 aún exige autorización por release, CI/seguridad, pruebas HTTP y despliegue real. Ninguna de estas pruebas puede declararse hecha por existir documentos.

## Gate H04

Solicitar aprobación humana expresa tras revisar los doce `spec.md`, planes, listas y decisiones. No marcar las specs individuales como completadas automáticamente tras instalar. El archivo `A-000` previamente instalado era `DRAFT`; el lote permite su revisión integral, sin asumir que el Gate A-000 se aprobó antes.
