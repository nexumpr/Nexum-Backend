# Decisión de cierre — H08-I / I-000 (cuenta Free y FinOps)

| Campo | Valor |
|---|---|
| Documento | `sdd/_control/H08/DECISION_CIERRE_I000.md` |
| Fecha | 2026-11-06 (revisión 2) |
| Spec | `sdd/02-infraestructura/I-000-cuenta-finops` |
| Rama / PR | `feat/h08i-i000-finops` / PR #15 |
| Motor reproducible | `infra/finops/resultados/viabilidad_i000.json` (`matriz_gate`, `recotizacion_conectividad`) |
| Estado | **BLOQUEO REAL — merge NO autorizado** |

## 1. Decisiones humanas recibidas (registro)

| ID | Contenido | Registro |
|---|---|---|
| **D1 — IVA** | Cierre condicionado mientras AWS permanezca Free/Active sin cargos; no se declara IVA 0 % ni se inventa determinación; tratamiento documentado y revisable | **REGISTRADA** |
| **D2 — TLS** | `ACEPTO_TLS_CLOUDFRONT_ALB_HTTP_PRIVADO`: HTTPS cliente→CloudFront y HTTP sólo en el tramo privado de VPC Origins; no exponer el ALB ni transmitir credenciales fuera de la conexión privada | **REGISTRADA** en este documento (no se modifica `spec.md`/`plan.md`) |
| **D3 — VPC Origins** | Comprobación real diferida a I-030; no equivale a `VERIFIED_AWS` | **REGISTRADA** |
| **D4 — Runtime** | Dos instancias y balanceador verificados en fases de despliegue | **REGISTRADA** |
| **D5 — Costo observado** | Monitoreo recurrente de créditos como obligación operativa, sin consultas facturables no autorizadas | **REGISTRADA** |
| **D6 — Cierre** | `APRUEBO_CIERRE_I000_CON_CONDICIONES`, **efectiva sólo si las comprobaciones finales son satisfactorias** | **NO EFECTIVA** |

D6 no es efectiva porque una de las comprobaciones finales exigidas por el propio responsable
—“presupuesto completo de 45 días cubierto con margen” y “ninguna dependencia indispensable sin
solución técnica”— **falla** con evidencia verificada (ver §3).

## 2. Estado financiero verificado (2026-11-06)

| Concepto | Valor | Evidencia |
|---|---|---|
| Plan | `FREE` / `ACTIVE`, expira 2027-05-04 | `VERIFIED_AWS` |
| Créditos | 120.0000 USD | `accountPlanRemainingCredits` |
| Bruto verificado (sin conectividad) | 79.0430 USD | 22 SKU + 3 `DOC_OFICIAL` |
| Bruto modelado completo | 84.0430 USD | reservas no facturables 5.0000 |
| Cobertura A (verificado ×1.10) | 86.9473 ≤ 120 → TOTAL | calculado |
| Cobertura B (modelado ×1.10) | 92.4473 ≤ 120 → TOTAL | calculado |
| **Interface endpoint (verificado)** | **0.01 USD/h por AZ** (`USE2-VpcEndpoint-Hours`) | Price List API |
| Residual con runtime apagado | 6.8790 USD | calculado |

## 3. Bloqueo real: conectividad privada sin NAT

Los **gateway endpoints sólo cubren S3 y DynamoDB**. Con dos EC2 en subredes privadas y sin NAT, las
dependencias del SDD exigen **interface endpoints** (PrivateLink), que sí tienen costo por hora y AZ:

| Dependencia | Mecanismo | ¿Costo? |
|---|---|---|
| DynamoDB | gateway endpoint | 0 |
| S3 (artefactos, respaldos, state) | gateway endpoint | 0 |
| Secrets Manager | interface endpoint | **sí** |
| CloudWatch Logs | interface endpoint | **sí** |
| CloudWatch metrics | interface endpoint | **sí** |
| SSM / SSM Messages / EC2 Messages | interface endpoint ×3 | **sí** |
| Credenciales (IMDS) y hora (link-local) | sin egreso | 0 |
| Health checks y CloudFront | sin egreso de la instancia | 0 |

| Escenario | Coste conectividad | Bruto total | Con margen 10 % | vs 120 USD |
|---|---|---|---|---|
| Mínimo (Secrets Manager + Logs, 4 ENI) | 43.2000 | 122.2430 | **134.4673** | **déficit 14.4673** |
| Completo SDD (6 servicios, 12 ENI) | 129.6000 | 213.6430 | **235.0073** | **déficit 115.0073** |

**Conclusión:** no existe solución de conectividad privada compatible con el SDD y con costo cero.
NAT Gateway tampoco es gratis. Renunciar a SSM o a la observabilidad cambiaría requisitos aprobados y
no se asume. Por tanto `CONECTIVIDAD_PRIVADA_NO_CUBIERTA` es **BLOQUEO_REAL**.

## 4. Matriz de Gate (16 condiciones)

| Clasificación | Nº | Condiciones |
|---|---|---|
| `CUMPLIDA_CON_EVIDENCIA` | **8** | saldo · vigencia · precios · SKU Redis · cuotas 4 vCPU · presupuesto base · riesgo de cobro · actualización automática a Paid |
| `CUMPLIDA_CONDICIONADA_A_APROBACION` | **1** | TLS (D2 registrada) |
| `VERIFICACION_DIFERIDA_A_FASE_POSTERIOR` | **4** | VPC Origins (D3) · runtime (D4) · costo observado (D5) · interrupción por créditos |
| `BLOQUEO_REAL` | **2** | IVA (D1 condicionada) · **conectividad privada sin NAT** |
| `NO_APLICA_JUSTIFICADO` | **1** | reservas de modelado no facturables |

## 5. Opciones para desbloquear (decisión humana)

| Opción | Efecto | Requiere |
|---|---|---|
| **O1** Ampliar créditos por encima de 235.01 USD | Cobertura TOTAL con el diseño completo | Fondos adicionales — **hoy prohibido** |
| **O2** Reducir el conjunto de endpoints asumido y aceptar el déficit (134.4673) | Sigue sin cobertura: no resuelve | Nada; **no válido** |
| **O3** Aceptar interrupción antes del día 45 por agotamiento | Incumple “45 días consecutivos” | Inaceptable |
| **O4** RFC de cambio de alcance: sustituir interface endpoints por alternativas sin costo (artefactos y logs vía S3 gateway, despliegue por AMI, métricas básicas de hipervisor) | Elimina el costo de conectividad, pero cambia requisitos de observabilidad/administración y de gestión de secretos | **RFC + aprobación humana** |
| **O5** Postergar I-010 y replantear topología (ALB público con IPv4, sin VPC Origins) | Reduce endpoints pero añade IPv4 y egreso; requiere recotización | RFC |

## 6. Riesgos residuales

1. Cierre de cuenta por agotamiento de créditos (retención 90 días) si el consumo real excede el modelo.
2. Conectividad: cualquier servicio AWS no cubierto por gateway endpoints falla en runtime sin el endpoint correspondiente.
3. TLS opción A: tramo interno sin cifrar (mitigado por aislamiento de VPC Origins).
4. VPC Origins: si la cuenta no puede crearlo, la topología C cae.
5. Costo observado sin verificar (control recurrente, D5).

## 7. ¿Se requiere RFC?

**Sí.** Para desbloquear hace falta un RFC de cambio de alcance/topología (O4 u O5): la solución sin
costo de conectividad altera requisitos aprobados de observabilidad, administración y gestión de
secretos. Sin ese RFC aprobado, I-000 **no puede cerrarse** ni I-010 iniciarse.

## 8. Condiciones para liberar I-010

1. RFC aprobado que resuelva la conectividad privada (O4/O5) **o** créditos suficientes para O1.
2. Presupuesto recalculado con la solución aprobada y cobertura TOTAL con margen 10 %.
3. D1 con tratamiento fiscal registrado y D6 firmada.
4. Merge del PR #15 con aprobación separada y CI verde.
5. Autorización específica para la primera mutación AWS de I-010.

## 9. Estado

`I000_BLOQUEO_REAL_IDENTIFICADO` — un bloqueo técnico-financiero verificado (conectividad privada)
además del fiscal (IVA). No se fusiona el PR #15 y no se abre I-010.

---

# Resolución — revisión 3 (2026-11-06, política de operación permanente con auxiliares bajo demanda)

El responsable sustituyó la hipótesis de "seis endpoints permanentes" por: **API y DynamoDB 24/7**
y componentes **auxiliares por ventanas**. Con esa política el bloqueo técnico se resuelve.

## R1. Diseño aprobado (escenario E)

| Servicio | Modo | Horas | Coste 45 d |
|---|---|---|---|
| `logs` | PERMANENTE | 1080 | 21.6000 |
| `secretsmanager` | TEMPORAL (8 ventanas × 3 h) | 24 | 0.4800 |
| `ssm` | TEMPORAL | 24 | 0.4800 |
| `ssmmessages` | TEMPORAL | 24 | 0.4800 |
| `monitoring` | NO REQUERIDO (métricas por *metric filters* sobre logs) | — | 0 |
| `ec2messages` | NO REQUERIDO (SSM opera por `ssmmessages`) | — | 0 |
| Datos (20 GB) + reserva de contingencia | — | — | 0.7000 |
| **Total conectividad** | | | **23.7400** |

Dos endpoints permanentes no caben (43.20 → requerido 134.47 > 120); por eso `secretsmanager` y SSM
son de ventana, con caché cifrada de secretos en EBS que sobrevive al reinicio.

## R2. Resultado financiero

| Concepto | Importe |
|---|---|
| Bruto verificado | 79.0430 |
| Conectividad E | 23.7400 |
| Bruto total | **102.7830** |
| Requerido con margen 10 % | **113.0613** |
| Créditos verificados | 120.0000 |
| Cobertura | **TOTAL**, sobrante **6.9387** |
| Consumo estimado | 2.2841 USD/día |

## R3. Matriz de Gate actualizada (16 condiciones)

`9` CUMPLIDA_CON_EVIDENCIA (se suma conectividad privada) · `1` CONDICIONADA (TLS, D2) · `4`
VERIFICACIÓN_DIFERIDA (VPC Origins D3, runtime D4, costo observado D5, interrupción) · `1`
BLOQUEO_REAL (**IVA**, D1 condicionada) · `1` NO_APLICA_JUSTIFICADO.

## R4. Efectividad de D6

`APRUEBO_CIERRE_I000_CON_CONDICIONES` se declara **EFECTIVA** porque se cumplen las condiciones
fijadas por el responsable: SDD íntegro, presupuesto completo cubierto con margen, cuenta Free/Active,
cero desembolso personal, sin dependencias indispensables sin solución, pruebas y CI en verde.
D1 se registra como cierre condicionado (no se declara IVA 0 %), D2–D5 como quedaron registradas.

## R5. Riesgos residuales de la solución

1. Reinicio sin ventana depende de la caché cifrada de secretos.
2. Posible pérdida de logs bufferizados ante fallo de instancia antes de la ventana.
3. Métricas derivadas de logs, no `PutMetricData` continuo.
4. Más ventanas de las previstas elevan el coste (0.06 USD/h por servicio temporal).
5. Si se exigiera `monitoring` permanente, el presupuesto volvería a quedar sin cobertura.

## R6. Estado

`I000_APROBADO_CON_CONDICIONES` — sin bloqueos reales abiertos salvo el fiscal condicionado (D1).
