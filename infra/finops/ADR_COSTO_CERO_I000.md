# ADR — Costo cero y plan Free para I-000

- **ID:** `ADR-COSTO-CERO-I000`
- **Fecha:** 2026-11-06 · **Fase:** H08-I · **Spec:** I-000-cuenta-finops
- **Estado:** `PROPUESTO — BLOCKED_FINOPS` (no aprobado; no autoriza aprovisionamiento)
- **Aprobador requerido:** responsable humano del Backend

## 1. Contexto

El requisito exige Backend en AWS con **costo cero** y el responsable fijó una regla irrenunciable:
**cero pesos de dinero personal**, sin pasar a AWS Paid y sin comprar dominio. La topología
seleccionable es C: CloudFront con `*.cloudfront.net`, origen VPC hacia un ALB privado, dos
instancias EC2 privadas en zonas distintas, Redis OSS administrado, DynamoDB on-demand y gateway
endpoints para S3 y DynamoDB (sin NAT), durante 45 días 24×7.

## 2. Evidencia verificada en AWS (2026-11-06)

| Hecho | Valor | Estado |
|---|---|---|
| Plan de cuenta | `FREE`, `ACTIVE` | `VERIFIED_AWS` |
| Expiración del plan | 2027-05-04 | `VERIFIED_AWS` |
| Créditos restantes | 120.0 USD (`accountPlanRemainingCredits`) | `VERIFIED_AWS` |
| Cuota de vCPU On-Demand | 16 | `VERIFIED_AWS` |
| `t4g.micro` disponible | us-east-2a / 2b / 2c | `VERIFIED_AWS` |
| ElastiCache Redis OSS | `cache.t4g.micro` redis 7.1 a 0.016 USD/nodo-hora | `VERIFIED_AWS` |
| ALB | 0.0225 USD/h + 0.008 USD/LCU-h (`AWSELB`) | `VERIFIED_AWS` |
| CloudFront VPC Origins | 0 orígenes existentes; elegibilidad **no** acreditada | `NOT_VERIFIED` |
| Costo bruto verificado 45 d | `UNKNOWN` (11 partidas sin precio verificado) | `NOT_VERIFIED` |
| Costo observado facturado | Desconocido; Cost Explorer no ejecutado | `NOT_RUN` |
| IVA / impuestos | `UNKNOWN` | `NOT_VERIFIED` |

Cifras modeladas: C = 84.0080 USD (bruto calculado, tarifas mixtas), residual 6.8440 USD,
A = 91.8080 USD, B = 28.9820 USD (no cumple 24×7).

## 3. Decisión

1. **Se declara `BLOCKED_FINOPS`.** Hay cuenta Free vigente con 120 USD de créditos, pero el bruto
   verificado y la cobertura son indeterminables mientras 11 partidas requeridas no tengan precio
   `VERIFIED_AWS`; además el IVA no está determinado y el TLS extremo a extremo no está resuelto.
2. **No se aprueba aprovisionamiento.** Ni recursos, ni presupuestos, ni alarmas, ni Terraform.
3. **Se mantiene el diseño C** como candidato, condicionado a confirmar VPC Origins y completar la
   cotización verificada.
4. **Se rechaza la variante B (8 h/día)** como entorno requerido: no ofrece 24×7.
5. **Se conserva A** sólo como comparador de costo mayor.
6. **La política de gasto personal cero se mantiene**, pero se declara explícitamente que **no**
   prueba ausencia de cobro: `riesgo_cobro_personal` es `True` mientras la cobertura sea `UNKNOWN`.

## 4. Alternativas evaluadas

| Alternativa | Bruto calculado 45 d | 24×7 | Decisión |
|---|---|---|---|
| C — CloudFront + VPC Origin + privadas | 84.0080 (no verificado) | Sí | Candidata, bloqueada por evidencia |
| A — ALB público + IPv4 | 91.8080 (piso, no techo) | Sí | Sólo comparador |
| B — ventanas 8 h/día | 28.9820 | **No** | Rechazada para el entorno requerido |
| Mocks o dobles como producción | — | — | Prohibido (R-036/R-037) |

## 5. Riesgos residuales

1. **Cobertura indeterminada**: 120 USD verificados no garantizan cubrir un bruto aún desconocido;
   el margen exigido es 10 % y no puede aplicarse hasta tener el bruto verificado.
2. **Costo variable**: LCU, egreso, solicitudes CloudFront y transferencia inter-AZ dependen del
   tráfico real; las reservas del modelo son supuestos, no tarifas.
3. **Cost Explorer facturable**: 0.01 USD por solicitud; no se usó, por lo que no se puede afirmar
   que el gasto observado sea cero.
4. **TLS no extremo a extremo**: el tramo CloudFront→ALB es HTTP privado; exige aceptación formal o
   recotización con ACM y nombre válido.
5. **IVA**: sin determinación fiscal; no se asume 0 %.
6. **Apagado no elimina el costo**: persisten 6.8440 USD por datos, backups, logs y secretos.
7. **Créditos con expiración no verificada individualmente**: la API expone saldo y fecha de plan,
   no el vencimiento de cada lote de crédito.

## 6. Consecuencias

- I-000 **no cierra** e I-010 **no inicia** hasta que el responsable:
  1. acepte o rechace el costo de completar los precios pendientes (nuevas consultas sin cargo);
  2. decida sobre el gasto de 0.01 USD por solicitud de Cost Explorer o aporte captura de consola;
  3. resuelva IVA y TLS extremo a extremo;
  4. firme la decisión final con la evidencia adjunta.
- Todo importe se recalcula con `modelo_costos.py`; no se editan cifras a mano sin regenerar
  `resultados/costos_45d.json`.
