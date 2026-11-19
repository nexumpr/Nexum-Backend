# RFC-021 — Frecuencias de pagos recurrentes (E032–E035) alineadas con RFC-020

| Campo | Valor |
|---|---|
| Documento | `sdd/_control/H08/RFC-021-FRECUENCIAS_RECURRENTES.md` |
| Versión | 1.0.0 |
| Fecha | 2026-11-18 |
| Estado | **PROPUESTO — no aplicado** |
| Aprobador | Responsable de Nexum-Backend |
| Frase | `APRUEBO_RFC_021_FRECUENCIAS_RECURRENTES` |
| Precedente | `RFC-020` (APROBADA en Oleada 1): 7 canónicos + 2 alias heredados |

## 1. Problema real

El contrato vigente no es coherente consigo mismo:

| Operación | Enum publicado hoy | Fuente |
|---|---|---|
| E001 `POST /auth/register` | 7 canónicos: `weekly, fortnightly, monthly, bimonthly, quarterly, semiannual, annual` | `internal/domain/identity/initial_finance.go` |
| E023 `GET /catalogs/frequencies` | enum de esquema con 9 valores; emite 7 canónicos | RFC-020 aplicada |
| E024/E025 `GET/PUT /income` | enum de 9 valores; emite 7 canónicos | RFC-020 aplicada |
| **E033 `POST /recurring-payments`** | **4 valores: `weekly, biweekly, monthly, yearly`** | `RecurringCreate.frequency` |
| **E032 lista / E034 detalle / E035 respuesta** | **4 valores** | `RecurringPayment.frequency` (mismo esquema en las tres) |

Consecuencias verificadas por inspección del OpenAPI en `main` `c9f7ec1`:

1. Un usuario registrado con `fortnightly` no puede declarar un pago recurrente quincenal:
   el único valor admitido equivalente sería `biweekly`, que es un alias que el catálogo **no
   emite** y que E001 **no** acepta.
2. Un usuario con `annual` no puede declarar un pago recurrente anual: el esquema solo admite
   `yearly`.
3. `bimonthly`, `quarterly` y `semiannual` no son expresables en absoluto, aunque E001 los
   acepta y el cálculo de equivalente mensual (`internal/domain/financial/calculation.go`) los
   soporta.
4. El cliente Android, que ya usa las 7 frecuencias canónicas en el alta, recibiría de E033 un
   valor distinto del que eligió para el mismo concepto.

No es un problema de implementación: la autoridad de transporte (OpenAPI) quedó desalineada de
la autoridad funcional (registro + catálogo). La corrección es **aditiva**, igual que RFC-020.

## 2. Cambio propuesto

Los dos esquemas publican el conjunto completo de **9 valores aceptados**:

```yaml
RecurringCreate.frequency:
  enum: [weekly, fortnightly, monthly, bimonthly, quarterly, semiannual, annual, biweekly, yearly]
RecurringPayment.frequency:
  enum: [weekly, fortnightly, monthly, bimonthly, quarterly, semiannual, annual, biweekly, yearly]
```

Reglas de aceptación y emisión:

| Operación | Acepta | Emite | Nota |
|---|---|---|---|
| E033 `POST` | **9 valores** | **7 canónicos** | `biweekly`→`fortnightly`, `yearly`→`annual` antes de persistir |
| E032 `GET` lista | n/a | **7 canónicos** | Normaliza al serializar cualquier valor histórico |
| E034 `GET` detalle | n/a | **7 canónicos** | Igual |
| E035 `PATCH` | n/a (no hay campo `frequency` en `RecurringPatch`) | **7 canónicos** | La frecuencia no es editable en este contrato; no se añade |

`RecurringPatch` **no** incorpora `frequency`: el contrato vigente no lo permite y este RFC no
amplía alcance funcional, solo el conjunto de valores admitidos en la creación y emitidos en la
respuesta.

## 3. Compatibilidad

- **Ningún valor publicado se elimina.** `{weekly, biweekly, monthly, yearly}` es subconjunto
  del conjunto de 9. `comparar_contrato_api.py` solo bloquea `ENUM_REDUCIDO` /
  `ENUM_RESPUESTA_REDUCIDO`; una ampliación resulta `COMPATIBLE`.
- **Datos existentes:** no se migran. Un `frequency` histórico `biweekly`/`yearly` se normaliza
  al serializar; no se reescribe el ítem.
- **E001, E023, E024, E025:** no se tocan.
- **Cálculo financiero:** `calculation.go` ya contempla `fortnightly` (×26/12), `bimonthly`,
  `quarterly`, `semiannual` y `annual`. No cambia ninguna fórmula ni ningún importe.

### 3.1 Comprobación de no-ruptura

| Verificación | Resultado |
|---|---|
| Los 7 canónicos de RFC-020 están presentes | Sí: `weekly, fortnightly, monthly, bimonthly, quarterly, semiannual, annual` |
| Los 2 alias heredados se conservan | Sí: `biweekly`, `yearly` (nunca se eliminan del enum publicado) |
| Los 4 valores previamente publicados siguen admitidos | Sí: `weekly`, `biweekly`, `monthly`, `yearly` (subconjunto del nuevo conjunto) |
| `ENUM_REDUCIDO` / `ENUM_RESPUESTA_REDUCIDO` | No se produce: solo ampliación |
| `RecurringPatch` | Sin cambios: no incorpora `frequency` |
| Valores inventados | Ninguno: no se añade ninguna frecuencia fuera de RFC-020 |
| Comparador `comparar_contrato_api.py` | `COMPATIBLE`, 0 rupturas |

## 4. Alternativas descartadas

| Alternativa | Motivo del descarte |
|---|---|
| Traducir `fortnightly`→`biweekly` de forma transparente en el servidor | Cambia el valor que el usuario declaró y rompe la exactitud de la frecuencia (R-030) |
| Dejar 4 valores y devolver 422 a las demás frecuencias | Un usuario válido de E001 no puede crear su pago recurrente; funcionalidad incompleta |
| Mantener 4 valores y que el cliente elija un alias | El alias no lo emite E023 y el cliente no puede descubrirlo |
| Cambiar E001 o E023 a 4 valores | Reducción de enum publicada y regresión de endpoints ya desplegados |

## 5. Impacto técnico

- `sdd/03-endpoints/openapi/nexum-api-v1.yaml` y `.json` regenerados desde el mismo objeto.
- Normalización en el dominio de recurrentes (nuevo `internal/domain/financial` o
  `internal/domain/recurring`), reutilizando la tabla de RFC-020 para no duplicar la fuente.
- Pruebas exigidas:
  - E033 acepta los 9 valores; `biweekly` persiste y emite `fortnightly`; `yearly` → `annual`.
  - Valor fuera del conjunto → 422.
  - E032/E034 devuelven exactamente un valor canónico, incluso para ítems históricos con alias.
  - Los dos esquemas publicados contienen los 9 valores y el comparador queda `COMPATIBLE`.
- Persistencia: `frequency` sigue siendo string; sin cambio de claves ni de índices.

## 6. Riesgos

| Riesgo | Mitigación |
|---|---|
| Cliente con enum cerrado de 4 valores rechaza la respuesta ampliada | El único cliente es Nexum Android, que ya usa los 7 canónicos; se documenta en la entrega |
| Deriva entre RFC-020 y RFC-021 | Se reutiliza la misma tabla de alias y una prueba de contrato que compara ambos conjuntos |
| Duplicar la normalización en tres sitios | Una sola función de normalización, usada por E024, E025 y E032–E035 |

## 7. Registro

| Fecha | Cambio | Estado |
|---|---|---|
| 2026-11-18 | Versión 1.0.0 creada a partir del hallazgo F-01 del Gate de Oleada 2 | PROPUESTO |
