# Servicios derivados: ML/IA, push y notificaciones (H06)

**Exigencia:** el requisito académico ML+IA generativa está registrado en constitución GOV-17 y continuidad. Los contratos E052/E054/E055 cubren salida de resultados, sin inventar rutas de proveedores o módulos frontend. **No basta una respuesta estática etiquetada IA**, ni una llamada que siempre retorne lo mismo.

## ML real y generativa: dos responsabilidades distintas

- E052 `/analytics/insights`: características basadas en datos propios (categorías, gasto, fecha), inferencia ejecutada por modelo **entrenado y versionado**; metadatos `modelVersion`, muestra, periodo, evidencia. Sin datos suficientes debe devolver lista vacía o bandera de insuficiencia; nada de inventar predicciones.
- E054 `/analytics/spending-patterns`: identificar tendencias con regla reproducible mínima (3 cargos similares en 14 días). Si se decide ML adicional, informar su modelo/versionado separadamente.
- E055 `/analytics/recommendations`: recomendación financiera educativa con texto explicativo **generado** por un modelo de lenguaje solo si se aprueba proveedor/modelo, presupuesto, residencia y tratamiento de datos. Si está temporalmente indisponible, error 503 u objeto `modelVersion` indicando modo basado en reglas **sin presentarlo como generativo**. No filtrar PII a proveedor externo por defecto.
- `IA_ML_PROVIDER`: interfaz de aplicación `Analyze(userScopedFeatures)` y `Explain(anonymizedEvidence)`. El Backend decide adaptador de inferencia por ADR de H08 antes de implementarlo, sin improvisar credenciales ni activar servicios sujetos a Paid; producto/operaciones monetarias de E052/E055 **no pueden considerarse COMPLETED** sin ML y generativa reales. Si la cuenta Free no puede sostener el modelo, bloquear Gate de implementación y presentar RFC/FinOps.
- Pruebas exigidas: batería de ejemplos no sensibles, comparación de versiones, evaluación de precisión y falsos positivos, seguridad de prompt-injection, confidencialidad, costo/token, degradación honesta y verificación de que un texto generativo provino de inferencia genuina.

## Notificaciones push

- E057/058 preferencias, E059/060 registro/revocación de token push y E061/062/063 bandeja de notificaciones, E064–E067 recordatorios. Token FCM cifrado, no logueado ni publicado; claves/servicios de envío solo por IaC/secretos autorizados, **no** se configura Play Store o app Android desde Backend.
- Opt-in explícito, zona horaria, reintentos idempotentes, deduplicación por evento, worker único por recordatorio aun con dos nodos. Si no existe canal push autorizado, mantener contrato pero marcar entrega no implementada, no afirmar envío exitoso.
- Email de recuperación/verificación E005/010: responder 202 uniforme; token un uso con expiración lógica. Proveedor/email real y elegibilidad/costo se verifican antes de activación H08; nunca asumir que SES envía desde una cuenta Free sin configuración.

## Costos y decisiones bloqueantes preimplementación

No iniciar servicio externo de ML, LLM, email, push, broker, colas pagadas o recursos auxiliares sin estimación 45 días, consentimiento y aprobación por Gate separado; ningún costo personal. IaC debe poder habilitar/deshabilitar auxiliares sin apagar API ni DynamoDB. El diseño contractual queda completo, pero la integración real requiere evidencia, no supuestos.
