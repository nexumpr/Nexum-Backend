# A-000 — Contexto y límites del sistema Nexum-Backend

| Metadato | Valor |
|---|---|
| Identificador | `A-000` |
| Bloque | `H04 / sdd/01-arquitectura-general` |
| Documento | `spec.md` — QUÉ debe cubrir el sistema |
| Versión | `0.1.1` |
| Estado | **DRAFT — requiere revisión y aprobación humana de A-000** |
| Fecha | `2026-11-05` |
| Autoridad | `constitution.md` v0.2.0, Gate H03 aprobado por el responsable el 2026-11-05 |
| Alcance | **Exclusivamente Backend**; no diseño Mobile ni infraestructura implementada |

> **Regla de precisión:** Este documento define el contexto y las obligaciones conocidas del servidor. No congela el inventario de endpoints ni diseña tablas, rutas concretas, servicios AWS o decisiones de infraestructura. Los literales del PDF docente v3 se cotejaron directamente con el contexto maestro v1.3; **el PDF docente v3 y el prototipo Android V6.0.2 `(3)` se cotejaron directamente en la consolidación H04**. Todo detalle que exija esas fuentes queda marcado como pendiente de trazabilidad, no se inventa.

## 1. Propósito, problema y valor

**Nexum** es un producto orientado a finanzas personales. **Nexum-Backend** será el sistema servidor que atienda las necesidades de identidad, sesiones, datos financieros y demás procesos compartidos que realmente requieran una API, conservando la propiedad de la información y permitiendo que clientes autorizados se comuniquen mediante contratos publicados y verificables.

El proyecto debe transformar comportamientos de servidor documentados en **capacidades reales y probables de verificar**: operaciones autorizadas, persistencia NoSQL, consistencia financiera, acceso seguro, observabilidad y despliegue reproducible. No se considera suficiente demostrar pantallas, crear endpoints falsos ni publicar contratos sin implementación posterior.

**Objetivo de A-000:** fijar quién interactúa con el Backend, qué información administra, qué límites tiene, qué obligaciones no funcionales lo condicionan y qué fuentes adicionales son necesarias antes de cerrar diseños detallados.

## 2. Límites y propiedad de responsabilidades

### 2.1 Dentro del Backend (compromiso del proyecto)

- API REST/HTTPS/JSON y contratos compatibles con consumidores autorizados, con validación de entrada y respuestas uniformes.
- Identidades, autenticación y autorización; JWT, persistencia/renovación y revocación de sesiones, logout del servidor y eliminación de cuenta donde corresponda.
- Información financiera propia del usuario, movimientos/operaciones que el producto realmente requiera, resúmenes derivados, reglas monetarias exactas, idempotencia, concurrencia y control de propiedad.
- Persistencia **Amazon DynamoDB**, modelada según operaciones y patrones de acceso finalmente identificados, con índices, expiración y controles de datos según las necesidades aprobadas.
- **Redis distribuido real y obligatorio**, compartido por las dos instancias y protegido por controles de acceso. Sus casos concretos de caché, limitación de tasa o coordinación se precisarán posteriormente; Redis no puede omitirse ni sustituir la fuente persistente DynamoDB.
- Seguridad, privacidad, protección de secretos, observabilidad, pruebas, DevSecOps, CI/CD, infraestructura reproducible y evidencias de operación.
- **Dos instancias independientes de Backend simultáneas en AWS**, balanceador real y TLS/HTTPS válido durante la verificación del requisito; evitar simulaciones de múltiples nodos.
- Publicación y compatibilidad de contratos API, con comprobación HTTP propia del Backend sin aprobación ni integración del equipo móvil.

### 2.2 Fuera del Backend

- Construcción de pantallas, navegación, permisos de Android, UI/UX, lógica exclusiva del dispositivo y almacenamiento exclusivo del cliente.
- Código Kotlin/Android, pipelines CI/CD móviles, distribución/Google Play o aceptación de integración por Mobile.
- Landing page, analítica web y publicación del aviso de privacidad en el sitio; el servidor **sí** conserva obligaciones de protección, acceso y eliminación de sus datos.
- Modificación del repositorio independiente `nexumpr/nexum-privacidad`.

### 2.3 Casos condicionados a una fuente vigente

Si el prototipo vigente justifica notificaciones push, sincronización de múltiples dispositivos, carga de contenido, moderación, IA/ML o procesamiento asíncrono, **determinar primero su componente de servidor y el flujo contractual**. No asumir su existencia, proveedor, modelo o endpoints en A-000. La verificación del inventario detallado corresponderá a fases posteriores; el levantamiento preliminar de operaciones debe estar disponible antes de cerrar A-060.

## 3. Actores y sistemas externos

| Elemento | Interacción o responsabilidad | Evidencia de alcance |
|---|---|---|
| Persona usuaria de Nexum | Gestiona sus propios datos, inicia/cierra sesión y consulta/ejecuta funcionalidades financieras autorizadas. | Identidad y capacidades generales confirmadas; comportamiento visible cotejado con ZIP Android `(3)`; los contratos HTTP permanecen pendientes de H06. |
| Cliente móvil u otro cliente autorizado | Consume la API y maneja su presentación local. **No aprueba** el Backend. | Restricción explícita de alcance. |
| Responsable de Backend | Acepta requisitos, ADR/RFC, gates, costos y despliegues de manera expresa. | Constitución/Gate H03. |
| Operador técnico Backend | Observa salud, despliega versiones autorizadas, gestiona incidentes y evidencias con mínimo privilegio. No equivale automáticamente a un rol de negocio en API. | Obligaciones de operación. |
| AWS (cómputo/red/TLS) | Aloja el runtime y permite demostrar dos nodos, balanceo y HTTPS. Topología concreta pendiente. | GOV-01..03. |
| DynamoDB | Fuente persistente NoSQL de datos del dominio, acceso autorizado. | GOV-04. |
| Redis distribuido | Caché/estado temporal compartido; política detallada pendiente. | GOV-05. |
| GitHub/automatización Backend | Controla revisiones, CI, versiones y despliegues autorizados. | GOV-11..13. |
| Servicios terceros eventuales | Solo si hay requisito funcional documentado (notificaciones, moderación, IA, etc.). | Condicional, no aprobado en A-000. |

**No se inventa un rol administrador de negocio**, proveedores terceros ni sistema de pagos: se justificarán mediante prototipo y requisitos antes de atribuirles permisos o contratos.

## 4. Contexto lógico (sin elección de servicios concretos)

```text
Persona usuaria
      │
      ▼
Cliente autorizado (p. ej. app Android; responsabilidad de Mobile)
      │ contratos API seguros, versión pública a definir
      ▼
[ Borde público HTTPS / balanceo real — diseño H04/H05 pendiente ]
      │
      ├───────────┬───────────┐
      ▼           ▼           │
[Backend Go A] [Backend Go B]  │    dos instancias distintas y simultáneas
      │           │           │
      └─────┬─────┘           │
            │                 │
       ┌────┴────────┐        │
       ▼             ▼        │
   [DynamoDB]  [Redis común] │
       │             │        │
       └── reglas de autorización, consistencia, trazas y protección de datos

Supervisión transversal: CI/CD Backend + seguridad + observabilidad + FinOps + IaC.
```

**Este diagrama es un contexto de límites, no una topología aprobada.** No implica ALB, EC2, NAT, VPC Link, número de AZ, ubicación de caché ni costos aprobados. Esas opciones requieren evaluación y ADR en su fase.

## 5. Flujos de negocio conocidos, sin diseñar endpoints

| Flujo | Responsabilidad del servidor conocida | Estado de detalle |
|---|---|---|
| Identidad inicial | Registrar una identidad válida y proteger credenciales; evitar duplicación y fuga de información. | Confirmado a nivel de obligación; reglas exactas pendientes. |
| Inicio de sesión | Autenticar y crear estado verificable/seguro de sesión, respetando autorización. | Confirmado a nivel de obligación. |
| Cierre y continuidad de sesión | Revocar/cerrar sesión real y definir renovación/persistencia segura según política posterior. | Confirmado a nivel de obligación. |
| Acceso a información personal | Restringir datos al propietario o autorización explícita; evitar IDOR y filtraciones. | Confirmado como control. |
| Operaciones financieras | Procesar únicamente operaciones permitidas por el producto con precisión monetaria e idempotencia cuando aplique. | Subconjunto histórico conocido; catálogo final pendiente. |
| Consulta/resumen financiero | Recuperar información del propietario con consistencia definida, filtros/paginación cuando correspondan. | Subconjunto histórico conocido; catálogo final pendiente. |
| Eliminación de cuenta | Aplicar reglas efectivas de supresión/retención y revocación, no solo ocultamiento en UI. | Obligación conocida; detalle legal y de flujo pendiente. |
| Capacidades complementarias | Determinar si hay verdadera responsabilidad de servidor para notificaciones, ML/IA, contenido, sincronización, etc. | **No confirmado**; requiere prototipo/fuente. |

**Restricción:** los siete endpoints históricos citados por el documento rector son un **mínimo académico parcial**, no constituyen el catálogo final ni justifican crear sus specs durante A-000.

## 6. Información gestionada: clases, no tablas

- **Identidad y seguridad:** identidad del usuario, metadatos de cuenta, credenciales derivadas seguras, sesiones y revocaciones; **nunca** contraseñas o secretos en claro.
- **Información financiera:** movimientos y atributos necesarios, importes con precisión determinista, titularidad y posible clasificación según lo que confirme el prototipo.
- **Datos derivados:** resúmenes financieros reproducibles según reglas aprobadas; no asumir materialización ni caché permanente.
- **Datos de operación:** eventos de auditoría no sensibles, métricas, trazas, correlación de solicitudes, estados de despliegue y evidencia técnica.
- **Datos temporales:** caché, limitación de tasa y/o estado efímero compartido, sujetos a diseño y TTL.

En A-000 no se define una tabla DynamoDB, PK/SK, índice, clave Redis, schema JSON ni período exacto de retención. **Esas decisiones requieren patrones de acceso y políticas ratificadas**.

## 7. Requisitos contextuales verificables

| ID A-000 | Criterio que el diseño posterior debe satisfacer | Vinculación |
|---|---|---|
| CTX-001 | Delimitar Backend frente a Mobile y Landing sin gates cruzados. | GOV-15,16,18 |
| CTX-002 | Mantener dos instancias reales concurrentes y balanceo/HTTPS como compromisos no opcionales. | GOV-01,02,03 |
| CTX-003 | Persistencia DynamoDB y Redis compartido reales; definir fuentes de verdad/temporalidad. | GOV-04,05 |
| CTX-004 | Identidad, sesiones, JWT, logout/revocación, borrado de cuenta y propiedad de datos. | GOV-06,07,18 |
| CTX-005 | Operaciones monetarias exactas, coherentes, resistentes a duplicados y con acceso autorizado. | GOV-08 |
| CTX-006 | Rendimiento: paginación, índices, caché, limitación de tasa, JSON/compresión, parametrizados posteriormente. | GOV-09 |
| CTX-007 | Registros/métricas/trazas/health sin secretos ni PII innecesaria. | GOV-10,18 |
| CTX-008 | QA, DevSecOps, CI/CD Backend, versionamiento, despliegue humano, IaC y recuperabilidad. | GOV-11,12,13 |
| CTX-009 | Evaluar costo de uso cero sin confundir saldo de créditos con gratuidad de recursos. | GOV-14 |
| CTX-010 | Publicar contratos y verificarlos por HTTP/DB/cache sin aprobación del equipo móvil. | GOV-15 |
| CTX-011 | Inventariar cobertura de **todas** las operaciones de servidor del prototipo antes de dar por cerrado el diseño de datos y catálogo. | GOV-16,17 |
| CTX-012 | Trazabilidad y control de estado: SDD aprobado no significa funcionalidad desplegada. | Constitución R-*, GOV-11,12 |
| CTX-013 | Mantener cuenta AWS correcta, región de trabajo y límites SCP; no cambiar recursos en etapa SDD. | Contexto v1.3, GOV-01,13 |
| CTX-014 | Proteger ciclo de vida de cuenta, propiedad y privacidad de información personal/financiera. | GOV-07,18 |

### 7.1 Criterios de calidad todavía sin métricas inventadas

- **Seguridad:** autenticación/autorización verificables, privacidad por defecto, secretos fuera del repositorio; límites específicos se definirán y medirán en A-050/A-100.
- **Disponibilidad:** la verificación de dos nodos y balanceo es obligatoria; SLO/tiempos de failover se definirán y justificarán después.
- **Consistencia monetaria:** la precisión debe eliminar ambigüedad de floats, reglas/decimales/moneda pendientes de definición de producto.
- **Escalabilidad/eficiencia:** recursos con paginación/caché/rate-limits/compresión donde correspondan y con criterios medibles aprobados después.
- **Operabilidad:** salud, trazas sin PII, presupuestos, reversión y evidencia; RTO/RPO y retenciones se determinan más adelante.
- **Viabilidad económica:** los requisitos técnicos siguen obligatorios; cualquier incompatibilidad con costo cero se eleva con opciones y una decisión humana documentada, no se marca solucionada.

## 8. Dependencias de diseño y límites de las fases posteriores

| Dependencia | Por qué es necesaria | Estado / condición |
|---|---|---|
| PDF académico original | Cotejar literales, alcance y fecha frente al resumen del contexto v1.3. | **COTEJADO DIRECTAMENTE en H04 consolidada**; fuente primaria de cuatro páginas y requisitos 1..20. |
| Prototipo Android **vigente** | Inventario preliminar de funcionalidades, separación local/servidor y patrones de acceso. | **COTEJADO EN H04**: ZIP V6.0.2 `(3)` inspeccionado; catálogo de 75 rutas incorporado. Los contratos HTTP definitivos se aprueban en H06. |
| Viabilidad Free/costo cero | Contradicción potencial entre dos nodos reales, balanceador/Redis y costos de uso. | **ABIERTA**; debe resolverse en el bloque de costos y ADR apropiados, sin quitar requisitos. |
| Capacidades AWS/SCP | Restricciones de IAM/OIDC y despliegue; no asumir que son superables. | **ABIERTA**, revisar puntualmente al diseñar infraestructura. |
| Decisiones concretas de Go/HTTP/modelo | Requieren specs especializadas y aprobación secuencial. | **NO DECIDIDAS** en A-000. |

No se crea RFC o ADR por adelantado. Cuando una contradicción esencial afecte una decisión posterior, se tramitará allí y se revisarán las specs dependientes.

## 9. Trazabilidad de compromisos GOV de H03 a A-000

| GOV | Referencia contextual | GOV | Referencia contextual | GOV | Referencia contextual |
|---|---|---|---|---|---|
| GOV-01 | §2, §4, CTX-002 | GOV-07 | §5–6, CTX-004/014 | GOV-13 | §2, CTX-008/013 |
| GOV-02 | §2, §4, CTX-002 | GOV-08 | §5–7, CTX-005 | GOV-14 | §7–8, CTX-009 |
| GOV-03 | §2, §4, CTX-002 | GOV-09 | §7, CTX-006 | GOV-15 | §2, §5, CTX-001/010 |
| GOV-04 | §2, §6, CTX-003 | GOV-10 | §7, CTX-007 | GOV-16 | §5, §8, CTX-011 |
| GOV-05 | §2, §6, CTX-003 | GOV-11 | §7, CTX-008/012 | GOV-17 | §2.3, CTX-011 |
| GOV-06 | §5–6, CTX-004 | GOV-12 | §7, CTX-008/012 | GOV-18 | §2, §6–7, CTX-014 |

## 10. Aceptación de A-000

Se podrá aprobar **solo** cuando el responsable revise y acepte este contexto, valide que ninguna obligación conocida falta, confirme los límites con Mobile, se reconozcan como abiertos los insumos no cotejados y las decisiones pendientes estén asignadas a su etapa, sin presentarlas como hechos resueltos. La ausencia de fuentes directas no permite falsificar requisitos; un conflicto material descubierto durante el cotejo puede requerir corrección de A-000 antes de ratificarlo.

**No se han ejecutado pruebas de Backend, HTTP, AWS, CI ni despliegues; no existe código nuevo.** Estado documental `DRAFT`, implementación `NOT_STARTED`, despliegue `NOT_DEPLOYED`.

---

## ADENDA H04 CONSOLIDADA — Fuentes contrastadas (2026-11-05; propuesta v0.2.0)

**Esta adenda complementa, NO sustituye, la versión 0.1.1 previamente instalada. El contenido histórico DRAFT se conserva para trazabilidad.** El usuario pidió preparar el bloque completo H04; se mantienen todos sus documentos en revisión, sin aprobación automática.

- Se revisó directamente el ZIP Android `(3)` de 16 entradas y sus recursos Java/WebView/JS; no contiene `fetch(` en app.js. La demo usa localStorage y datos en memoria, por lo que los endpoints descritos son propuestas pendientes, no rutas implementadas.
- Se leyó el catálogo PDF de 27 páginas y se extrajeron 75 IDs de rutas E001-E075 (33 M, 23 S, 12 E, 7 O). Anexo `../TRAZABILIDAD_75_RUTAS.md` conserva el inventario íntegro.
- Se leyó el PDF original docente v3 (4 páginas), con requisitos reales de dos instancias, balanceador y TLS, Redis, NoSQL, JWT, DevSecOps, CI/CD, observabilidad, costo cero y ML+IA generativa a nivel del proyecto Mobile. El Backend provee únicamente los contratos necesarios.
- **Precedencia inequívoca:** decisión vigente AWS/DynamoDB/Redis, NO Google Cloud/MongoDB Atlas mencionados en el catálogo histórico. No confundir esta aclaración con cambio aprobado de infraestructura.
- **Condición para Gate H04:** cotejar A000 con A010..A110 y fuentes, conservar clasificación y riesgos; bloquear H05 si hubiera contradicción arquitectónica no resuelta. Los riesgos de viabilidad física/financiera se validan de forma obligatoria durante H05, antes de implementar.
