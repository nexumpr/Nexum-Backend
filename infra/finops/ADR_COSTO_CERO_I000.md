# ADR — Costo cero y plan Free para I-000

- **ID:** `ADR-COSTO-CERO-I000`
- **Fecha:** 2026-11-06 · **Fase:** H08-I · **Spec:** I-000-cuenta-finops
- **Estado:** `PROPUESTO — BLOCKED_FINOPS` (no aprobado; no autoriza aprovisionamiento)
- **Aprobador requerido:** responsable humano del Backend

## 1. Contexto

El requisito docente exige Backend en AWS con **costo cero** y el responsable fijó una regla
irrenunciable: **cero pesos de dinero personal**, sin actualizar a AWS Paid y sin comprar dominio.
La topología seleccionable es C: CloudFront con `*.cloudfront.net`, origen VPC hacia un ALB privado,
dos instancias EC2 privadas en zonas distintas, Redis OSS administrado, DynamoDB on-demand, S3 y
DynamoDB por gateway endpoints (sin NAT), durante 45 días 24×7.

## 2. Hallazgos de esta corrida

| Hecho | Valor | Estado |
|---|---|---|
| Importe bruto 45 d del escenario C | 84.0580 USD | Calculado con precios `DOC_ONLY` |
| Costo residual con runtime apagado | 6.8440 USD | Calculado |
| Créditos declarados (H05) | 120.00 USD | No verificado |
| Créditos verificados hoy | 0.00 USD | Sesión AWS expirada |
| Plan / vigencia | Desconocidos | `NOT_VERIFIED` |
| Costo realmente facturado | Desconocido | `NOT_RUN` (Cost Explorer no ejecutado) |

## 3. Decisión

1. **Se declara `BLOCKED_FINOPS`** para I-000: no existe evidencia verificada de plan, vigencia,
   créditos, precios ni costo observado. Un importe calculado con precios históricos no es prueba de
   viabilidad económica.
2. **No se aprueba aprovisionamiento.** No se crean recursos, presupuestos ni alarmas reales; no se
   ejecuta Terraform. El plan Free no se modifica.
3. **Se mantiene el diseño C** como candidato, condicionado a: confirmación de VPC Origins, SKU Redis
   OSS, cuota de vCPU y disponibilidad de `t4g.micro` en dos zonas.
4. **Se rechaza la variante B (8 h/día)** como entorno requerido: no ofrece disponibilidad 24×7.
5. **Se conserva el comparador A** (ALB público, IPv4 públicas) sólo como referencia de costo mayor.

## 4. Alternativas evaluadas

| Alternativa | Costo bruto 45 d | Cumple 24×7 | Decisión |
|---|---|---|---|
| C — CloudFront + VPC Origin + privadas | 84.0580 (no verificado) | Sí | Candidata, bloqueada por evidencia |
| A — ALB público + IPv4 públicas | 91.8580 (piso, no techo) | Sí | Sólo comparador; mayor costo |
| B — ventanas 8 h/día | 29.0320 | **No** | Rechazada para el entorno requerido |
| Recursos simulados o mocks como producción | — | — | Prohibido por la constitución (R-036/R-037) |

## 5. Riesgos residuales

1. **Créditos insuficientes o vencidos**: el plan Free finaliza a los seis meses o al agotar créditos;
   la fecha real no está verificada.
2. **Costo variable no acotado**: LCU, egreso, solicitudes CloudFront y transferencia inter-AZ pueden
   superar el supuesto; la reserva de 3.00 USD es modelado, no tarifa.
3. **Cost Explorer facturable**: cada solicitud cuesta 0.01 USD; no se usó.
4. **TLS no extremo a extremo**: el tramo CloudFront→ALB es HTTP privado; si se exige TLS completo,
   hay que recotizar (ACM + nombre válido).
5. **Apagado no elimina el costo**: persisten 6.8440 USD por datos, backups, logs y secretos.
6. **IVA**: sin determinación fiscal; no se asume 0 %.

## 6. Consecuencias

- I-000 **no cierra** y I-010 **no inicia** hasta que el responsable:
  1. reautentique la sesión AWS y autorice la ejecución de las consultas read-only catalogadas;
  2. decida sobre el gasto de 0.01 USD por solicitud de Cost Explorer (o aporte captura de consola);
  3. acepte o rechace el riesgo de IVA y de TLS no extremo a extremo;
  4. firme la decisión final `VIABLE`/`BLOCKED_FINOPS` con la evidencia adjunta.
- Todo importe se recalcula con el mismo motor (`modelo_costos.py`) al actualizar precios; no se
  editan cifras a mano en este documento sin regenerar `resultados/costos_45d.json`.
