# RFC-006 — Aceptación temporal del riesgo AWS-0011 (CloudFront sin WAF)

| Campo | Valor |
|---|---|
| Documento | `sdd/_control/H08/RFC-006-EXCEPCION_WAF_I030.md` |
| Fecha | 2026-11-08 |
| Spec afectada | `sdd/02-infraestructura/I-030-balanceo-https/` (CloudFront) e I-080 (quality gate IaC) |
| Origen | Hallazgo **AWS-0011 HIGH** de Trivy 0.76.2 sobre `infra/terraform/modules/balanceo/main.tf` |
| Estado | **PROPUESTO — NO APROBADO. No surte efecto.** |
| Autoriza | nada mientras no exista aprobación humana explícita |
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

**Conclusión:** AWS-0011 **no dispone de autorización normativa válida**. La excepción que figuraba en
`.trivyignore.yaml` era improcedente y se retira en esta corrección. Mientras esta RFC no se apruebe, el
job `iac` **falla** con AWS-0011 y el PR #28 no puede fusionarse.

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

## 8. Aprobación requerida

| Control | Estado |
|---|---|
| `APRUEBO_RFC_006_WAF_TEMPORAL` | **PENDIENTE** — no solicitada aún / no concedida |
| Implementación de WAF | **NO AUTORIZADA** |
| Modificación de I-030 | **NO AUTORIZADA** (requiere su propio Gate/RFC) |
| Despliegue AWS | **BLOQUEADO** hasta resolver §5 |

Frase exacta que debe emitir el responsable para aprobar esta aceptación temporal:
`APRUEBO_RFC_006_WAF_TEMPORAL`.

## 9. Efecto inmediato mientras no se apruebe

- `.trivyignore.yaml` **no** exime AWS-0011.
- El job `iac` de I-080 falla con AWS-0011 HIGH y el check queda en rojo.
- I-080 permanece `BLOQUEADO_RFC_006`; no se declara `IAC_SCAN PASS`.
- No se modifica I-030, no se crea WAF, no se activa AWS y no se cambia el plan.
