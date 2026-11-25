# Matriz FinOps del egreso FCM/OAuth — NAT Gateway 1 AZ

| Campo | Valor |
|---|---|
| Documento | `sdd/_control/H08/MATRIZ_FINOPS_EGRESO_FCM.md` |
| Versión | 1.0.0 |
| Fecha | 2026-11-24 |
| Motor | `infra/finops/egreso_fcm.py` (aritmética `Decimal`, sin `float`) |
| Catálogo | `infra/finops/precios-us-east-2.json` |
| Saldo verificado | **130.491975 USD** (`freetier get-account-plan-state`, 2026-11-24) |
| Estado | **`MODELADO_CON_SUPUESTOS_NO_VERIFICADOS` · `AWS_BILLING_NOT_VERIFIED`** |
| Decisión que soporta | `PROPUESTA_EGRESO_FCM.md` (NAT 1 AZ, 8 días) |

> **Regla de oro:** los créditos son **saldo**, no presupuesto, no límite de
> gasto y **no autorizan** ninguna compra. Un total por debajo del saldo no
> equivale a coste cero. La factura real es `UNKNOWN` y el IVA no está
> determinado.

## 1. Precios y estado de evidencia

| SKU | Concepto | Precio | Unidad | Estado | Nota |
|---|---|---:|---|---|---|
| `NAT_GATEWAY_HORA_SUPUESTO` | Hora de NAT Gateway | 0.045000 | gateway-hora | **`SUPUESTO`** | Precio de lista pública; `NAT_GATEWAY_HORA` original sigue `UNKNOWN` |
| `NAT_GATEWAY_PROCESAMIENTO_GB_SUPUESTO` | Datos procesados por el NAT | 0.045000 | GB-procesado | **`SUPUESTO`** | No revalidado con Price List API |
| `NAT_EGRESO_INTERNET_GB_SUPUESTO` | Salida a Internet | 0.090000 | GB-salida | **`SUPUESTO`** | **No** se aplica franquicia de 100 GB/mes (conservador) |
| `IPV4_HORA` | IPv4 pública (EIP del NAT) | 0.005000 | IP-hora | **`VERIFIED_AWS`** | Se modela como **condicional**: no se revalidó que la EIP del NAT se facture con este SKU |
| `TRANSFERENCIA_INTERAZ_GB` | Transferencia inter-AZ | 0.010000 | GB | **`VERIFIED_AWS`** | Sólo aplica si NAT y EC2 están en AZ distintas |

Ningún SKU de NAT está `VERIFIED_AWS`. Por tanto **ninguna cifra de esta matriz
es una factura**. `AWS_COST_REAL = NOT_VERIFIED`.

## 2. Supuestos de tráfico

| Supuesto | Valor | Fuente |
|---|---:|---|
| Push por día | 2 000 | Estimación de producto; no medida |
| Tamaño por push | 1 KiB | Estimación |
| OAuth por día | 1 000 | Estimación (renovación de token de cuenta de servicio) |
| Tamaño por OAuth | 2 KiB | Estimación |
| Escenario alto | ×10 | Sensibilidad, no predicción |

Fórmula (`trafico_gb`):

```
bytes_dia = 2000 × 1024 + 1000 × 2048 = 4 096 000 B/día
GB(dias, factor) = bytes_dia × dias × factor / 1024³
```

| Horizonte | Escenario | GB modelados |
|---|---|---:|
| 8 días | base | 0.030517578125 |
| 8 días | alto ×10 | 0.305175781250 |
| 45 días | base | 0.171661376953 |
| 45 días | alto ×10 | 1.716613769531 |

El tráfico es irrelevante frente a la hora del NAT: **el coste es fijo, no de
uso**. Aun con 10× el incremento es 0.04 USD en 8 días.

## 3. Escenarios (partidas y totales)

### 3.1 8 días (192 h) — ventana de certificación

| Partida | Fórmula | Base | Alto ×10 |
|---|---|---:|---:|
| NAT (1 AZ) | 192 h × 0.045 | 8.6400 | 8.6400 |
| EIP del NAT (condicional) | 192 h × 0.005 | 0.9600 | 0.9600 |
| Procesamiento | GB × 0.045 | 0.0014 | 0.0137 |
| Inter-AZ (condicional) | GB × 0.01 | 0.0003 | 0.0031 |
| Salida a Internet | GB × 0.09 | 0.0027 | 0.0275 |
| **Total modelado** | | **9.6044** | **9.6443** |
| — subtotal `VERIFIED_AWS` (EIP + inter-AZ) | | 0.9603 | 0.9631 |
| — subtotal `SUPUESTO` (NAT + proceso + salida) | | 8.6441 | 8.6812 |

**Límite de certificación:** 20.00 USD (propuesta de autorización, no un límite
de AWS). **9.6044 ≤ 20.00 → cumple**; el escenario alto también (9.6443).

Sensibilidad:

| Variante 8 días base | Total | Lectura |
|---|---:|---|
| Sin cargo de EIP | 8.6444 | Si la EIP no se facturase |
| Sin transferencia inter-AZ | 9.6041 | Caso real si ambas EC2 y el NAT están en `us-east-2a` |
| Con EIP e inter-AZ (filas de arriba) | 9.6044 | Caso conservador |

### 3.2 45 días (1 080 h) — convención FinOps

| Partida | Fórmula | Base | Alto ×10 |
|---|---|---:|---:|
| NAT (1 AZ) | 1 080 h × 0.045 | 48.6000 | 48.6000 |
| EIP del NAT (condicional) | 1 080 h × 0.005 | 5.4000 | 5.4000 |
| Procesamiento | GB × 0.045 | 0.0077 | 0.0772 |
| Inter-AZ (condicional) | GB × 0.01 | 0.0017 | 0.0172 |
| Salida a Internet | GB × 0.09 | 0.0154 | 0.1545 |
| **Total modelado** | | **54.0248** | **54.2489** |
| — subtotal `VERIFIED_AWS` | | 5.4017 | 5.4172 |
| — subtotal `SUPUESTO` | | 48.6231 | 48.8317 |

## 4. Total del entorno y cobertura de créditos

Referencia de infraestructura **ya existente** (documental, no factura
observada; excluye I-110 y consumo variable):

| Componente | 45 días | Fuente |
|---|---:|---|
| I-000 (base) | 102.7830 | Gate I-000 |
| Endpoint SES 2 AZ | 21.6000 | `GATE_H08E_E005_SES_ENDPOINT` |
| **Referencia 45 d** | **124.3830** | suma |
| Prorrateo 8 d | **22.1125** | 124.3830 × 8 / 45 |

| Concepto | 8 días | 45 días |
|---|---:|---:|
| Infraestructura existente (prorrateo / total) | 22.1125 | 124.3830 |
| NAT 1 AZ base | 9.6044 | 54.0248 |
| **Total entorno (base)** | **31.7169** | **178.4078** |
| Margen 10 % | **34.8886** | **196.2486** |
| Créditos verificados | 130.491975 | 130.491975 |
| ¿Cubierto? | **Sí** | **No** |
| Holgura / déficit vs total | +98.775075 | **−47.915825** |
| Holgura / déficit vs margen 10 % | +95.603375 | **−65.756625** |

Lectura correcta:

- **8 días:** el total con margen (34.8886) cabe en el saldo. Es la ventana que
  la propuesta declara.
- **45 días:** **no** cabe. El NAT por sí solo (54.0248) ya consume el 41.4 % del
  saldo y el entorno completo lo supera en 47.915825 USD. Por tanto no se puede
  sostener la convención de 45 días sin recortar otras partidas, sin más
  créditos o sin apagar el NAT.
- **161 días (hasta expiración del plan Free):** sólo la hora del NAT son
  193.20 USD (3 864 h), por encima del saldo. Inviable.

## 5. Reglas de uso de esta matriz

1. **El saldo no autoriza gasto.** `creditos_no_son_autorizacion = true`.
2. **El límite de 20 USD es una propuesta de autorización** para la ventana de
   8 días; no existe como control de AWS. Un presupuesto/alarma no impide el
   consumo por sí solo.
3. **No se declara coste cero.** El coste incremental mínimo es 1.20 USD/día
   (NAT 1.08 + EIP 0.12) más un residuo de tráfico.
4. **Toda cifra `SUPUESTO` debe revalidarse** contra Price List antes del Gate
   de `apply`; si el precio real sube, se repite el cálculo, no se ajusta la
   conclusión.
5. **IVA e impuestos: `UNKNOWN`.** No se presenta un total "final".
6. **La matriz no sustituye la factura.** `AWS_COST_REAL = NOT_VERIFIED` y
   `AWS_BILLING_NOT_VERIFIED`.
7. **Horizonte explícito.** Cualquier Gate de `apply` debe declarar fecha de
   reversión; sin fecha, no se aprueba.

## 6. Comparación de horizontes (resumen ejecutivo)

| Horizonte | Coste NAT base | Total entorno | Cabe en saldo | Decisión |
|---|---:|---:|---|---|
| 8 días | 9.6044 | 31.7169 | Sí | **Aprobable** con Gate de plan |
| 45 días | 54.0248 | 178.4078 | **No** | Requiere recorte o rechazo |
| 161 días | ≈ 193.20 (sólo NAT) | > 300 | **No** | Inviable |

## 7. Estado de evidencia por afirmación

| Afirmación | Estado |
|---|---|
| Precio hora NAT = 0.045 USD | `SUPUESTO` / `NOT_VERIFIED` |
| Precio procesamiento = 0.045 USD/GB | `SUPUESTO` / `NOT_VERIFIED` |
| Precio salida = 0.09 USD/GB | `SUPUESTO` / `NOT_VERIFIED` |
| Precio IPv4 = 0.005 USD/h | `VERIFIED_AWS` (aplicación al NAT no revalidada) |
| Inter-AZ = 0.01 USD/GB | `VERIFIED_AWS` |
| Saldo 130.491975 USD | Verificado por lectura el 2026-11-24 |
| Tráfico diario | `SUPUESTO` de producto |
| Cobertura a 8 días | Calculada; depende de precios `SUPUESTO` |
| Cobertura a 45 días | Calculada; **no cubierta** con el saldo actual |
| Factura real | `UNKNOWN` |
