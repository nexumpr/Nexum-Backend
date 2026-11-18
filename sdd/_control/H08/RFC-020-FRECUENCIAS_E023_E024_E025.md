# RFC-020 — Frecuencias canónicas y alias heredados en E023, E024 y E025

| Campo | Valor |
|---|---|
| Documento | `sdd/_control/H08/RFC-020-FRECUENCIAS_E023_E024_E025.md` |
| Versión | 1.1.0 (corrige la v1.0.0: no se elimina ningún valor publicado) |
| Fecha | 2026-11-18 |
| Estado | **PROPUESTO — aprobación conjunta en `GATE_H08E_OLEADA_1.md` §14** |
| Aprobador | Responsable de Nexum-Backend |
| Frase | `APRUEBO_GATE_OLEADA_1_RFC019_E012_E013_E015_E016_E017_E018_E019_E022_E023_E024_E025_E068_E069_E070_E071` |

## 1. Problema real

El contrato vigente no es coherente consigo mismo:

| Operación | Enum publicado hoy | Fuente |
|---|---|---|
| E001 `POST /auth/register` | `weekly`, `fortnightly`, `monthly`, `bimonthly`, `quarterly`, `semiannual`, `annual` (7 canónicos) | `internal/domain/identity/initial_finance.go` y `RegisterRequest` |
| E023 `GET /catalogs/frequencies` | `weekly`, `biweekly`, `monthly`, `yearly` | `Frequency.id` (OpenAPI) |
| E024 `GET /income` | `weekly`, `biweekly`, `monthly`, `yearly` | `Income.frequency` (OpenAPI) |
| E025 `PUT /income` | `weekly`, `biweekly`, `monthly`, `yearly` | `IncomeUpdate.frequency` (OpenAPI) |

Consecuencias verificadas por inspección:

1. Un usuario registrado con `fortnightly`, `bimonthly`, `quarterly`, `semiannual` o `annual`
   tendría en `USER#<id>/INCOME` un valor que E024 no puede devolver sin violar su enum.
2. E023 no ofrecería al cliente las frecuencias que E001 ya acepta y que los pagos recurrentes de
   la Oleada 2 (E032–E035) necesitan.
3. Los alias `biweekly`/`yearly` no existen en E001: el usuario que eligió `fortnightly` o
   `annual` no puede expresar su frecuencia con el enum actual de E025.

No es un problema de implementación: es una divergencia entre la autoridad de transporte
(OpenAPI) y el contrato realmente aceptado por el registro. La corrección debe ser **aditiva**:
añadir los 7 valores canónicos **sin retirar** `biweekly` ni `yearly` del esquema publicado.

## 2. Cambio propuesto

Los tres esquemas publican el **conjunto completo de 9 valores aceptados**:

```yaml
Frequency.id:
  enum: [weekly, fortnightly, monthly, bimonthly, quarterly, semiannual, annual, biweekly, yearly]
Income.frequency:
  enum: [weekly, fortnightly, monthly, bimonthly, quarterly, semiannual, annual, biweekly, yearly]
IncomeUpdate.frequency:
  enum: [weekly, fortnightly, monthly, bimonthly, quarterly, semiannual, annual, biweekly, yearly]
```

Reglas de emisión y aceptación:

| Operación | Acepta | Emite | Nota |
|---|---|---|---|
| E023 | n/a (GET) | **7 canónicos** (`items`) | `biweekly`/`yearly` permanecen en el enum del esquema por compatibilidad, pero **no** se emiten como ítems del catálogo |
| E024 | n/a (GET) | **7 canónicos** | Si un item histórico contuviera `biweekly`/`yearly`, se normaliza a `fortnightly`/`annual` al serializar; no se reescribe el item |
| E025 | **9 valores** | **7 canónicos** | `biweekly`→`fortnightly`, `yearly`→`annual` antes de persistir y antes de responder |

Etiquetas de E023 (sólo los 7 canónicos):

| id | label |
|---|---|
| `weekly` | Semanal |
| `fortnightly` | Quincenal |
| `monthly` | Mensual |
| `bimonthly` | Bimestral |
| `quarterly` | Trimestral |
| `semiannual` | Semestral |
| `annual` | Anual |

Los alias no llevan etiqueta porque no son ítems del catálogo; se documentan en la descripción
del campo `frequency` de E025 y en la prueba de normalización.

## 3. Compatibilidad

- **Ningún valor publicado se elimina.** El conjunto anterior
  `{weekly, biweekly, monthly, yearly}` es subconjunto del nuevo conjunto de 9. El comparador
  `herramientas/comparar_contrato_api.py` sólo bloquea `ENUM_REDUCIDO` y
  `ENUM_RESPUESTA_REDUCIDO`; una ampliación produce `COMPATIBLE`.
- **Cliente Android**: el prototipo V6.0.2 ya usa las 7 frecuencias canónicas en el alta; el enum
  ampliado elimina un valor no representable, no introduce uno nuevo para el cliente.
- **Datos existentes**: no se migran. E024 devuelve el valor canónico ya persistido; si un item
  histórico contuviera un alias, se normaliza al serializar.
- **E001**: no se toca. Su enum ya es el canónico.
- **E023**: el catálogo devuelve 7 ítems; el enum del esquema conserva 9 valores para no reducir
  el contrato publicado. Un cliente que valide contra el esquema no rechaza la respuesta.

## 4. Alternativas descartadas

| Alternativa | Motivo del descarte |
|---|---|
| Sustituir 4 por 7 eliminando `biweekly`/`yearly` | Reducción de enum publicada: rompe compatibilidad y exige una decisión incompatible |
| Mantener 4 valores y mapear `bimonthly/quarterly/semianual` a otro valor | Falsifica la frecuencia declarada y rompe la exactitud financiera (R-030) |
| Devolver 404 en E024 para frecuencias no representables | Rompe el perfil de usuarios válidos y oculta datos propios |
| Cambiar E001 a 4 valores | Rompe E001 desplegado y el prototipo; cambio destructivo |
| Dejar E023 con 4 y que Oleada 2 use otro catálogo | Duplica la fuente de verdad y garantiza incoherencia |

## 5. Impacto y verificación

- OpenAPI YAML y JSON se regeneran desde el mismo objeto (paridad I-150).
- `sdd/03-endpoints/DICCIONARIO_DATOS_H06.md` (o el documento equivalente de campos) se actualiza
  con el conjunto de 9 valores, la regla de emisión de 7 y la normalización de alias.
- Pruebas exigidas:
  - E023 devuelve exactamente **7** ítems canónicos y ningún alias.
  - E024 serializa los 7 valores canónicos y normaliza `biweekly`/`yearly` históricos.
  - E025 acepta los 9 valores; `biweekly` produce `fortnightly` y `yearly` produce `annual`;
    un valor fuera del conjunto → 422.
  - Los tres esquemas publicados contienen los 9 valores y el comparador queda `COMPATIBLE`.
- No cambia la persistencia (`frequency` sigue siendo un string), ni el cálculo de equivalente
  mensual, ni los importes.

## 6. Riesgos

| Riesgo | Mitigación |
|---|---|
| Cliente con enum cerrado de 4 valores rechaza la respuesta ampliada | El único cliente es Nexum Android, que usa 7; se documenta en la entrega |
| Confusión entre alias y canónicos | Los alias sólo se aceptan en entrada de E025 y se normalizan antes de persistir; E023/E024 sólo emiten canónicos |
| Deriva documental | La RFC y el Gate citan los esquemas exactos; el PR actualiza YAML/JSON y pruebas |

## 7. Registro

| Fecha | Cambio | Estado |
|---|---|---|
| 2026-11-18 | Creación; aprobación conjunta solicitada en el Gate de Oleada 1 §14 | PROPUESTO |
| 2026-11-18 | **v1.1.0:** corrige la incoherencia de la v1.0.0. Se conservan `biweekly` y `yearly` en los tres esquemas (9 valores publicados), se emiten sólo los 7 canónicos y se normalizan los alias en E025. Sin reducción de enum. | **PROPUESTO** |
