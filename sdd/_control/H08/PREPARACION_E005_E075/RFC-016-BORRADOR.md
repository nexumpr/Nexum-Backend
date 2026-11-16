# RFC-016 — Propuesta transversal de preparación de contratos E005–E075

**Estado: DRAFT / NO APROBADO / NO APLICADO.** Esta RFC NO sustituye los 71 Gates individuales de implementación.

## Problema
Se encontraron diferencias estructuradas entre el documento de prototipo V6.0.2 (NO es OpenAPI aprobado) y los 71 contratos H06. Cinco endpoints difieren en código de respuesta HTTP (E017, E031, E037, E042 y E063). E005/E006 difieren en nombres de campos. E005–E011 contienen referencias documentales heredadas SESSIONHASH en vez del modelo operativo E003/E004. En operaciones de ahorros y pagos deben respetarse transacciones, centavos e idempotencia.

## Decisión solicitada
1. Autorizar revisión y enmiendas contractuales acotadas por matriz, sin cambiar E001–E004 ni producción.
2. Definir una única política transversal para normalización de campos, clases UI/futuro y claves de sesión; no interpretar los ejemplos del PDF como aprobación automática.
3. Resolver casos de riesgo financiero individualmente, con Gate de contrato antes de editar YAML/JSON o spec APPROVED.
4. Conservar releases incrementales (1 endpoint nuevo por release) salvo futura enmienda aprobada.
5. Mantener los estados E005–E075 NOT_IMPLEMENTED, NOT_DEPLOYED hasta evidencia real.

## Impacto
Solo cambios de SDD aprobados y OpenAPI sincronizado; ningún cambio en la API desplegada ni AWS. Los listados en `MATRIZ_RECONCILIACION_E005_E075.md` son candidatos; ninguna decisión está concedida por este DRAFT.

## Gates posteriores
Requiere autorización humana RFC-016 y Gate específico antes de modificar contratos APPROVED; Git PR por alcance; merge por HEAD SHA40; AWS mutante por operación/plan/refresh.
