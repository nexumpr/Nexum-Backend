# Decisión de fuente normativa — Oleada 5

**Fecha:** 2026-11-23 · **Rama:** `liz-oleada5` (base `liz-oleada4`/`22d4aa9`).

El responsable confirmó que la **única fuente de verdad** para la Oleada 5 es:

`Nexum_Guia_Liz_Oleada5_FINAL_E021_E064_E075.md`

## Alcance aceptado

- 9 E-ID: **E021, E064, E065, E066, E067, E072, E073, E074, E075**.
- **E068–E071 quedan fuera**: la guía, el Plan Detallado y el Contexto Maestro los sitúan en Oleada 1 u opcionales, no en Oleada 5.
- No se modificarán `spec.md`, `plan.md`, `tasks.md` ni OpenAPI sin RFC/Gate.

## PDF V6.0.2

`Nexum_V6_0_2_Auditoria_Definitiva_Pantallas_75_Contratos_Verificados.pdf` **no es contrato**. Se excluye como fuente de implementación porque él mismo se declara «documento propuesto / no contrato SDD aprobado» y «pendiente de contraste SDD/OPENAPI». Sus divergencias quedan documentadas como contexto, no como requisitos.

## Gates que la propia guía exige

- **E072/E073:** no cambiar la respuesta observable de health sin Gate del propietario; la guía los registra como `CONTRACT_BLOCKED` por el desajuste entre el runtime (`status=UP`) y el esquema OpenAPI (`ok|degraded`, `service`, `timestamp`). No duplicar rutas ni quitar probes.
- **E064–E067:** scheduler durable, outbox/FCM y cambios en E014/E020 requieren Gate puntual. No tocar archivos de Oleada 4 sin autorización.
- **CI Linux, AWS y merge:** exclusivos del responsable; este documento no los autoriza.

Cero secretos, claves B64 o URLs presignadas.
