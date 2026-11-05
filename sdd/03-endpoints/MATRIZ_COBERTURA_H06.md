# Cobertura y riesgos H06

**Documentación:** 75 operaciones definidas con sus 4 artefactos, modelos tipados y OpenAPI; fuentes: 33 M, 23 S, 12 E, 7 O. Los 75 IDs del catálogo H04 están presentes, sin eliminación silenciosa.

## Matriz de capacidades

| Capacidad | IDs | Decisión |
|---|---|---|
| Registro, login, refresh, logout, recuperación, verificación, sesiones | E001–E011 | obligaciones de identidad, antibot, revocación cross-node |
| Cuenta, perfil, consentimiento, privacidad, onboarding, exportación | E012–E021 | eliminación real, protección de datos; sin Mobile Gate |
| Catálogos, ingreso, resumen y movimientos | E022–E031 | centavos, idempotencia, índices y versiones |
| Recurrentes | E032–E037 | no doble descuento, pago real exactamente una vez |
| Metas y aportaciones | E038–E047 | split atómico de hasta 12 metas; reversión sin borrar historial |
| Presupuestos | E048–E050 | consumo por operaciones ejecutadas, reemplazo versionado |
| Dashboard y análisis | E051–E056 | caché correcta; ML y texto generativo genuinos pendientes de implementación |
| Push, notificaciones y recordatorios | E057–E067 | preferencias, consentimiento, entrega real verificada |
| Preferencias/ayuda/demo/voz/health | E068–E075 | siete O opcionales, endpoints health E072/73 obligatorios |

## Puntos a comprobar en Gate H07 / H08 (no ocultar)

1. Cotejo independiente con ZIP de prototipo y PDF docente original: H04 contiene las 75 referencias pero no se recibieron otra vez ahora. No afirmar inspección actual de esas fuentes.
2. Contratación/selección del **modelo real de ML y LLM generativa**, su costo y privacidad: interfaces definidas, proveedor de inferencia no autorizado. Debe bloquear la implementación de IA si no cabe en Free. Nunca responder con un mock presentado como resultado de inferencia.
3. Envío de emails/push: requieren autorización de proveedor, credenciales seguras y evidencia de entrega, con IaC y créditos cero pesos.
4. AWS CloudFront VPC Origin permiso `Create` no probado; Redis tarifa oficial SKU inequívoca no probada; TLS privado y viabilidad financiera por 45 días todavía sujetos al preflight H08. No cerrar estos rubros por aprobar H06.
5. Revisión de límites I/O de EC2 micro y costos de Dynamo/Redis/CloudWatch con carga de 2 nodos antes de release.

**Se permite aprobar H06 documentalmente** si acepta el diseño y los bloqueos de ejecución anteriores, pero **NO** declarar funcionalidades reales completadas ni autorizar despliegues. Una aprobación H06 no convierte O opcionales en obligaciones sin ratificación.
