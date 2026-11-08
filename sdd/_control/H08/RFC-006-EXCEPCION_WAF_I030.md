# RFC-006 — Aceptación temporal del riesgo AWS-0011 (CloudFront sin WAF)

| Campo | Valor |
|---|---|
| Documento | `sdd/_control/H08/RFC-006-EXCEPCION_WAF_I030.md` |
| Fecha | 2026-11-08 |
| Spec afectada | `sdd/02-infraestructura/I-030-balanceo-https/` (CloudFront) e I-080 (quality gate IaC) |
| Origen | Hallazgo **AWS-0011 HIGH** de Trivy 0.76.2 sobre `infra/terraform/modules/balanceo/main.tf` |
| Estado | **APROBADA (2026-11-08)** — aceptación temporal del riesgo AWS-0011. No autoriza implementar WAF ni desplegar AWS |
| Autoriza | exceptuar AWS-0011 en el escáner IaC de I-080 mientras se cumplan las condiciones y vigencia de §5–§6 |
| No autoriza | implementar WAF, modificar I-030, desplegar AWS, cambiar de plan ni gastar |

## 1. Hecho y ausencia de autorización previa

Trivy `0.76.2` (`config`, severidad HIGH/CRITICAL) reporta **AWS-0011 HIGH**: la distribución de
CloudFront no tiene AWS WAF delante.

Revisión de autorizaciones existentes:

| Documento | Alcance | ¿Cubre AWS-0011? |
|---|---|---|
| `RFC-005-ENMIENDA_I030_TLS_D2.md` + `RATIFICO_D2_PARA_I030` | TLS del tramo CloudFront→ALB privado | **No**. No menciona WAF ni acepta su ausencia. |
| `GATE_H08I_I030.md` (`APRUEBO_GATE_I030_LOCAL`) | Implementación local de balanceo | **No**. No evalúa AWS-0011. |
| `I-030 tasks.md` («Sin WAF…») | Declaración de alcance del diseño local | **No**. Es una nota de alcance, no una aceptación formal de riesgo HIGH con condiciones y vigencia. |
| `GATE_H08I_I070.md` / `MATRIZ_RECURSOS_STATE.md` | Límite SSE-S3 sin CMK | **No**. Se refiere a AWS-0132, no a WAF. |

**Conclusión:** AWS-0011 **no disponía de autorización normativa válida** en el momento del hallazgo. La
excepción que figuraba en `.trivyignore.yaml` era improcedente y se retiró. Tras la aprobación humana de
esta RFC (§8), la excepción se reincorpora de forma **explícita, acotada y con vigencia**, y deja de ser
una omisión de gobierno.

## 2. Motivo económico y técnico

- **Económico:** AWS WAF tiene coste mensual no incluido en el presupuesto de I-000 (cero desembolso
  personal; plan Free). Añadirlo exige recotización y RFC de FinOps, no una decisión unilateral.
- **Técnico:** el diseño D2 expone la API sólo a través de CloudFront; el ALB es `internal` y el backend
  no tiene IPv4 pública ni NAT. WAF aportaría inspección L7 y mitigación de bots/ataques, pero no es un
  sustituto de los controles de red y autenticación ya presentes.
- **Alcance:** la implementación es **local** (`AWS_NOT_RUN`). No hay tráfico real que proteger todavía;
  el riesgo se materializa en el primer despliegue.

## 3. Riesgo HIGH aceptado (si se aprueba)

Operar CloudFront sin WAF deja la API expuesta a:
- tráfico automatizado y fuerza bruta distribuida más allá del rate limiting por IP/cuenta de la aplicación;
- payloads maliciosos L7 (XSS/SQLi) que deberían filtrarse antes del backend;
- picos de peticiones que consumen cuota y aumentan latencia/coste;
- ausencia de reglas gestionadas (OWASP) y de listas de reputación de IP.

No se mitiga el riesgo con los controles actuales; se **reduce** su probabilidad e impacto parcial.

## 4. Controles compensatorios reales (existentes, no inventados)

| Control | Evidencia | Límite |
|---|---|---|
| HTTPS obligatorio en el borde (`redirect-to-https`) | `modules/balanceo` | Security policy TLSv1 por certificado por defecto (H-01 abierto) |
| ALB `internal`, sin IPv4 pública, sin NAT ni SSH | `modules/red`, `modules/balanceo` | No expone el backend directamente |
| Ingreso al ALB sólo desde prefix list de CloudFront:80 | `modules/red` | Reduce origen no-CloudFront |
| Autenticación JWT RS256, sesiones revocables, Argon2id | A-050, `internal/application/identity` | No filtra payloads L7 |
| Rate limiting distribuido y fail-closed en auth | A-080, `internal/infrastructure/cache/rate_limiter.go` | Por sujeto; no sustituye WAF |
| Límite de tamaño de cuerpo, JSON estricto, cabeceras de seguridad, CORS allowlist | A-030/A-040 | Mitiga abuso básico |
| Sin caché de API financiera | `modules/balanceo` | Evita fuga por caché |
| Escaneo IaC y de secretos en CI | I-080 | Detecta configuración insegura, no tráfico |

## 5. Condiciones para el primer despliegue AWS

Antes de cualquier `terraform apply` autorizado:

1. Esta RFC debe estar **APROBADA** con la frase humana `APRUEBO_RFC_006_WAF_TEMPORAL`, o
2. existir una WAF desplegada y verificada (nueva RFC de coste para I-000/I-030), o
3. una decisión humana alternativa que sustituya esta aceptación.
4. Resolver además RFC-004 (secretos) y H-01 (TLSv1), que siguen abiertos.
5. Registrar en el Gate de despliegue el riesgo residual y la fecha de revisión.

Sin (1), (2) o (3), el despliegue queda **BLOQUEADO**.

## 6. Vigencia y caducidad

- Caduca el **2026-12-02** (fecha de evaluación académica) o en el **Gate de despliegue AWS**, lo que
  ocurra primero.
- La aprobación no es renovable por silencio: cada renovación exige nueva aceptación humana explícita.
- Si se implementa WAF, esta RFC queda sin efecto y la excepción de `.trivyignore.yaml` se elimina.

## 7. Alternativas y coste

| Alternativa | Efecto | Coste |
|---|---|---|
| Aceptar AWS-0011 temporalmente (esta RFC) | Mantiene D2 y presupuesto; riesgo L7 residual | 0 USD |
| AWS WAF con reglas gestionadas | Mitiga OWASP/bots; requiere recotización y cambios en I-030 | Referencia pública: ~5 USD/mes por Web ACL + ~1 USD/mes por regla + ~0,60 USD/millón de peticiones. **No revalidado** (`NOT_VERIFIED`) y sujeto a IVA |
| AWS Shield Standard | Mitigación DDoS L3/L4 incluida | 0 USD; no cubre L7 |
| Confiar sólo en rate limiting de aplicación | Ya implementado; insuficiente para L7 | 0 USD |

No se elige ninguna alternativa en esta RFC: la decisión es del responsable.

## 8. Aprobación requerida y concedida

| Control | Estado |
|---|---|
| `APRUEBO_RFC_006_WAF_TEMPORAL` | **CONCEDIDA** por el responsable el **2026-11-08** |
| Implementación de WAF | **NO AUTORIZADA** |
| Modificación de I-030 | **NO AUTORIZADA** (requiere su propio Gate/RFC) |
| Despliegue AWS | **BLOQUEADO** hasta resolver §5 |

Frase emitida por el responsable: `APRUEBO_RFC_006_WAF_TEMPORAL`.

Alcance exacto de la aprobación: aceptar temporalmente el hallazgo AWS-0011 HIGH en el escáner IaC de
I-080, con los controles compensatorios de §4, las condiciones de §5 y la caducidad de §6. El riesgo
residual sigue siendo del responsable y no se declara mitigado.

## 9. Efecto tras la aprobación

- `.trivyignore.yaml` exime AWS-0011 **solo** en `modules/balanceo/main.tf` y con referencia a esta RFC.
- El job `iac` vuelve a verde; `IAC_SCAN` puede declararse `PASS` con la excepción documentada.
- El despliegue AWS continúa **BLOQUEADO** hasta cumplir §5 (y resolver RFC-004 y H-01).
- Cualquier otro HIGH/CRITICAL sigue haciendo fallar el escáner.
- La aprobación no se renueva por silencio: al vencer §6, la excepción debe eliminarse o renovarse con
  nueva decisión humana.
