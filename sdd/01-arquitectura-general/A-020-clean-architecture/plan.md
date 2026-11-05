# A-020 — Plan técnico de arquitectura

**Estado:** DRAFT H04. **Entrada obligatoria:** `spec.md` y fuentes de A-020. **Salida:** decisión lista para implementar después de H07; este plan NO ejecuta infraestructura ni código.

## 1. Estrategia de construcción

1. **Forma del sistema.** Definir interfaces, límites, precondiciones, postcondiciones, fallos, autorización y pruebas correspondientes. Monolito modular Go con un único binario Backend replicable en dos instancias simultáneas; no microservicios sin necesidad verificada. Stateless para permitir balanceo sin afinidad de sesiones.
2. **Capas y dependencia.** Definir interfaces, límites, precondiciones, postcondiciones, fallos, autorización y pruebas correspondientes. Domain (entities, value objects, reglas puras) no importa infra; Application (casos de uso y puertos) importa Domain; Infrastructure implementa puertos DynamoDB/Redis/clock/id generator/notification; Transport HTTP convierte DTO y llama Application.
3. **Módulos de negocio.** Definir interfaces, límites, precondiciones, postcondiciones, fallos, autorización y pruebas correspondientes. identity, userprivacy, onboarding, income, movements, recurring, goals, contributions, budgets, analytics, notifications, common; health y catalog como módulos de plataforma. Cada módulo exporta contratos mínimos.
4. **Topología de paquetes propuesta.** Definir interfaces, límites, precondiciones, postcondiciones, fallos, autorización y pruebas correspondientes. cmd/api, internal/domain/{money,identity,finance,...}, internal/application/{...}, internal/ports/{repository,cache,clock,events}, internal/adapters/{dynamodb,redis,mailer,push}, internal/transport/http/{handlers,middleware,dto}, internal/platform/{config,telemetry,shutdown}; internal evita que consumidores usen tipos internos.
5. **Puertos de repositorio.** Definir interfaces, límites, precondiciones, postcondiciones, fallos, autorización y pruebas correspondientes. Interfaces granulares por caso de uso: Users, Sessions, FinancialStore, IdempotencyStore, QueryRepository. La transacción multi-entidad es puerto específico TransactFinance, NO una emulación que componga Put independientes.
6. **Composition root.** Definir interfaces, límites, precondiciones, postcondiciones, fallos, autorización y pruebas correspondientes. Inicialización única inyecta config validada, SDK, caches, repositorios, políticas, handlers y ciclo de vida; fail-fast para secrets/region/identity, readiness condicionado a dependencias.
7. **Múltiples instancias.** Definir interfaces, límites, precondiciones, postcondiciones, fallos, autorización y pruebas correspondientes. Ningún estado de sesión, rate limiting, idempotencia, lock ni agregado crítico en memoria de una réplica. Redis coordina caché y límites; DynamoDB conserva la verdad. Rotación/revocación leíble por los dos nodos.
8. **Extensibilidad sin inventar.** Definir interfaces, límites, precondiciones, postcondiciones, fallos, autorización y pruebas correspondientes. ML/IA generativa del profesor exige una capacidad de producto, pero asignación técnica debe confirmarse sin trasladar toda Mobile al Backend. Definir interfaces de recomendaciones y adaptador proveedor si el flujo elegido requiere API.

## 2. Orden interno de diseño y dependencias

1. Revisar `constitution.md` y `AGENTS.md` antes de aceptar cambios; cotejar los PDF/prototipo cuyos nombres y hashes se registran en H04.
2. Identificar invariantes y entradas/salidas de cada módulo; mapear al catálogo E001..075 sin crear contratos endpoints H06.
3. Verificar acoplamiento con A-020 (capas), A-040 (DTOs), A-050 (auth), A-060 (persistencia), A-070 (consistencia), A-080 (Redis), A-090 (telemetría) y A-100 (QA).
4. Mantener como *blocker de H05* capacidad/costo de 2 instancias + balanceador + Redis, restricciones SCP/IAM, TLS y mecanismos de on/off; no inferir disponibilidad de AWS Free.
5. Registrar la decisión humana y conflictos si alguna regla altera el negocio del prototipo; no solicitar gate Mobile.

## 3. Estrategia de pruebas posteriores por capa

| Nivel | Diseño a comprobar | Evidencia futura obligatoria |
|---|---|---|
| Dominio | Invariantes y money/identidad | go test, race detector, entradas límite |
| Aplicación | Orquestación con mocks y transacciones | unit tests, casos negativos, retries |
| Infraestructura | DynamoDB/Redis reales según fase | integración, latencia, permisos mínimos, gasto |
| HTTP | Schemas OpenAPI 3.1 y respuestas | pruebas contractuales y HTTP reales |
| CI/CD | Seguridad y releases | PR checks, escaneo, autorización humana |

## 4. Riesgos y mitigación explícita

- Ciclos de dependencia entre money y finance. **Mitigación:** resolver antes de aprobar su dependencia; si requiere cambio de constitución emitir RFC.
- Acceso directo a Dynamo en handlers viola aislamiento y dificulta pruebas. **Mitigación:** resolver antes de aprobar su dependencia; si requiere cambio de constitución emitir RFC.

## 5. Criterio de cierre

Se considera listo para revisión cuando los 8 ejes técnicos de `spec.md` tienen trazabilidad a pruebas en `checklist.md`, los elementos futuros permanecen `NOT_RUN`, la matriz de alcance está intacta y las decisiones pendientes tienen dueño/fase, sin `APPROVED` automático.
