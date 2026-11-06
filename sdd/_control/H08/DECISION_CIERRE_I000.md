# Decisión de cierre — H08-I / I-000 (cuenta Free y FinOps)

| Campo | Valor |
|---|---|
| Documento | `sdd/_control/H08/DECISION_CIERRE_I000.md` |
| Fecha | 2026-11-06 |
| Spec | `sdd/02-infraestructura/I-000-cuenta-finops` |
| Rama / PR | `feat/h08i-i000-finops` / PR #15 |
| Evidencia reproducible | `infra/finops/resultados/viabilidad_i000.json` (campo `matriz_gate`) |
| Estado del documento | **PROPUESTA PARA DECISIÓN HUMANA — no es una aprobación** |

> Este documento **no** declara aprobado el Gate H08-I ni cerrada I-000. Ordena la evidencia y
> enumera las decisiones que sólo el responsable puede tomar.

## 1. Estado final de I-000

| Aspecto | Resultado |
|---|---|
| Decisión del motor | **`BLOCKED_FINOPS`** con 3 motivos automáticos |
| Bloqueos reales (matriz) | **1**: determinación de impuestos/IVA |
| Condicionados a aprobación humana | 1: TLS extremo a extremo (opción A) |
| Verificaciones diferidas a Gates posteriores | 4: VPC Origins, runtime de dos instancias, costo observado, interrupción por créditos |
| Cumplidos con evidencia | 7 |
| No aplica justificado | 1 (reservas de modelado) |
| Infraestructura AWS creada | **ninguna** |
| Terraform ejecutado | **ninguno** |
| Plan AWS | `FREE` / `ACTIVE`, expira 2027-05-04 |

## 2. Evidencias que sostienen el cierre

| Requisito del Gate ("Cierre de I-000") | Evidencia | Estado |
|---|---|---|
| Saldo | 120.0 USD (`accountPlanRemainingCredits`) | `VERIFIED_AWS` |
| Vigencia | 2027-05-04 (≈180 días ≥ 45) | `VERIFIED_AWS` |
| Precios exactos | **22 SKU** Price List + 3 partidas sin cargo `DOC_OFICIAL` | `VERIFIED_AWS` / `DOC_OFICIAL` |
| SKU Redis | `cache.t4g.micro` redis 7.1 (`redis7`) a 0.016 USD/nodo-hora | `VERIFIED_AWS` |
| Cuentas (cuotas) | 16 vCPU On-Demand ≥ 4 requeridas | `VERIFIED_AWS` |
| Presupuesto | verificado 79.0430 (+10 % = 86.9473); modelado 84.0430 (+10 % = **92.4473**) | calculado |
| Cobertura | **TOTAL** en ambos escenarios (≤ 120 USD) | calculado |
| Riesgos | ADR §7 + matriz `matriz_gate` | documentado |
| Impuestos / IVA | **no determinado** | `BLOQUEO_REAL` |

**Separación financiera:** costo bruto ≠ consumo de créditos ≠ impuestos ≠ desembolso personal. El
plan Free **no admite cargos** durante su vigencia (documentación oficial), por lo que el desembolso
personal bajo el plan activo es estructuralmente nulo; el riesgo real es el **cierre automático de la
cuenta** al agotar créditos o caducar (pérdida de acceso, retención de 90 días).

## 3. Pendientes reales

| # | Pendiente | Por qué no puede cerrarlo el agente |
|---|---|---|
| 1 | Determinación fiscal del IVA | Requiere criterio fiscal responsable; la spec prohíbe asumir 0 % |
| 2 | Aceptación de seguridad de TLS opción A | La spec la declara "pendiente de aceptación de seguridad" |
| 3 | Elegibilidad de CloudFront VPC Origins | Sólo se comprueba creando un VPC origin (operación mutante) |
| 4 | Runtime de dos instancias y balanceo | Gate: obligación de Gates posteriores, no estado PASS |
| 5 | Costo observado facturado | No figura en la lista de cierre; control recurrente de operación |

## 4. Excepciones que se proponen (ninguna se aprueba aquí)

| Excepción | Alcance | Base normativa | Alternativa |
|---|---|---|---|
| **E1** — TCP sin cifrar en el tramo CloudFront→ALB (opción A) | Tráfico dentro de VPC Origins, sin tránsito público; TLS del cliente a CloudFront se mantiene | spec I-000 §9: "I-030 plantea TLS de cliente en CloudFront con tramo HTTP privado ALB: pendiente de aceptación de seguridad" | Opción B: HTTPS hasta ALB con ACM + dominio propio → coste de dominio fuera del presupuesto de cero pesos |
| **E2** — IVA no imputado hasta factura real | Presupuesto sin línea de impuestos | spec I-000 §8: "no asumir ni 0 ni un cobro sin evidencia" | Determinación fiscal explícita antes de operar |

## 5. Riesgos residuales (declarados, no mitigados)

1. **Cierre de cuenta** por agotar 120 USD antes de 45 días (~28 % de holgura sobre el bruto
   verificado) → interrupción del servicio y pérdida de datos tras 90 días.
2. **Costo variable** real (LCU, egreso, solicitudes CloudFront, inter-AZ) puede elevar el consumo de
   créditos; el modelo es una estimación, no una factura.
3. **TLS**: opción A deja el tramo interno sin cifrar (mitigado por aislamiento de VPC Origins).
4. **VPC Origins**: si la cuenta no puede crearlo, la topología C cae y habría que recotizar la
   alternativa con ALB público (+ ~7.80 USD y 4 IPv4).
5. **Costo observado** sin verificar: no se puede afirmar "0 USD facturados" desde las APIs usadas.
6. **Residual apagado** 6.8790 USD: apagar el runtime no elimina datos, backups, logs ni secretos.

## 6. Decisiones humanas exactas

Marcar y firmar (una sola vez):

- [ ] **D1 — IVA:** declarar el tratamiento fiscal aplicable.
  - `D1.a` "El plan Free no genera cargos; no hay base gravable hasta que exista factura real; se
    revisará en cada corte de facturación." → convierte IVA en `CUMPLIDA_CONDICIONADA`.
  - `D1.b` "Se determina IVA de X % sobre el bruto modelado (92.4473 + IVA)." → requiere recotizar
    cobertura con el importe resultante.
- [ ] **D2 — TLS:** `ACEPTO_TLS_CLOUDFRONT_ALB_HTTP_PRIVADO` (opción A) o exigir opción B (implica
  dominio financiado y RFC de cambio de alcance).
- [ ] **D3 — VPC Origins:** aceptar `VERIFICACION_DIFERIDA_A_FASE_POSTERIOR` con obligación de
  comprobación en I-030 antes de cualquier creación de ALB/VPC origin.
- [ ] **D4 — Runtime:** aceptar que dos instancias reales, balanceo y 45 días se verifican en
  I-020/I-030/H08-E, no en I-000 (cláusula del Gate).
- [ ] **D5 — Costo observado:** aceptar el control recurrente (captura de Billing/Bills o autorización
  de una consulta de Cost Explorer a 0.01 USD) o autorizar esa consulta ahora.
- [ ] **D6 — Cierre:** `APRUEBO_CIERRE_I000_CON_CONDICIONES` o `RECHAZO_CIERRE_I000`.

## 7. Condiciones para liberar I-010

1. D1 resuelta (a o b) y presupuesto recalculado con impuestos si aplica.
2. D2 firmada.
3. D3 y D4 aceptadas por escrito, con la obligación de verificación trasladada a I-020/I-030.
4. D6 firmada.
5. `main` con el PR de I-000 fusionado (aprobación separada del merge) y CI verde.
6. Autorización específica para la primera mutación AWS de I-010 (VPC y red), que hoy sigue prohibida.

## 8. ¿Se requiere RFC?

- **No** para cerrar I-000 con las excepciones E1/E2: la propia spec I-000 §9 prevé la aceptación de
  seguridad del tramo privado y §8 prohíbe asumir impuestos, de modo que ambas son decisiones
  previstas, no cambios de alcance.
- **Sí** (RFC mínimo de cambio de alcance/coste) si el responsable exige TLS extremo a extremo en esta
  fase o un IVA que altere el presupuesto más allá de la cobertura.

## 9. Estado propuesto

`I000_LISTO_PARA_DECISION_HUMANA` — 1 bloqueo real (IVA), 1 condicionado (TLS), 4 verificaciones
diferidas autorizables y 7 requisitos cumplidos con evidencia. Sin infraestructura creada.
