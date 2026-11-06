# Evidencia de ejecución — I-000

**Fecha:** 2026-11-06 · **Rama:** `feat/h08i-i000-finops` · **Entorno:** Windows 11 nativo, PowerShell 7, sin Docker/WSL
**Alcance:** H08-I / I-000 exclusivamente. Sin mutaciones AWS, sin Terraform, sin despliegue.

## 1. Integración de la transición H08-I

| Hecho | Valor |
|---|---|
| PR #14 | `MERGED` (merge commit `e4c4167`) |
| CI del PR #14 | `validar` SUCCESS (run 42258192807) |
| `main` local | fast-forward a `e4c4167` |
| Rama I-000 | `feat/h08i-i000-finops` creada desde `main` limpio |

## 2. Consultas AWS ejecutadas

| Consulta | Resultado | Evidencia |
|---|---|---|
| `aws sts get-caller-identity --profile nexum --region us-east-2` | **FAIL de credenciales**: *Your session has expired. Please reauthenticate using 'aws login'* | `evidencia/cuenta_2026-11-06.json` |
| `aws freetier get-account-plan-state` | `NOT_RUN` (sin sesión válida) | `evidencia/cuenta_2026-11-06.json` |
| `aws freetier get-free-tier-usage` | `NOT_RUN` | `evidencia/free_tier_2026-11-06.json` |
| `aws elasticache describe-cache-engine-versions`, `cloudfront list-vpc-origins`, `servicequotas get-service-quota`, `ec2 describe-instance-type-offerings`, `dynamodb describe-limits` | `NOT_RUN` | `evidencia/servicios_2026-11-06.json` |
| `aws budgets describe-budgets` | `NOT_RUN` | sin presupuestos que describir; crear está prohibido |
| `aws pricing get-products` (EC2, ElastiCache, ALB, DynamoDB, CloudFront, S3) | `NOT_RUN` | requieren sesión válida |
| `aws ce get-cost-and-usage` | **NO EJECUTADO por costo** | 0.01 USD por solicitud; sin aprobación específica |

La sesión AWS caducada es un **bloqueo real**, no un resultado: no se sustituyó por supuestos ni se
marcó como PASS.

## 3. Artefactos implementados

| Archivo | Verificación |
|---|---|
| `infra/finops/dinero.py` | Aritmética exacta `Decimal`; rechaza `float` |
| `infra/finops/precios_us-east-2.json` | 28 SKU con estado de evidencia y fecha de fuente |
| `infra/finops/escenarios.py` | Escenarios A, B y C con cantidades exactas |
| `infra/finops/modelo_costos.py` | Reproduce 84.0580 USD de V3; residual 6.8440 USD |
| `infra/finops/viabilidad.py` | Decisión `BLOCKED_FINOPS` con 8 motivos |
| `infra/finops/requisitos.py` | Dos instancias y red sin NAT validados en diseño |
| `infra/finops/consultas_readonly.py` | 16 consultas, 0 mutantes, Cost Explorer marcado facturable |
| `infra/finops/sanitizar.py` | Enmascara cuenta, ARN, correo, claves, IPv4, JWT |
| `infra/finops/evidencia/` | 5 instantáneas sanitizadas |
| `infra/finops/resultados/` | `costos_45d.json`, `viabilidad_i000.json` deterministas |

## 4. Pruebas ejecutadas

| Comando | Resultado |
|---|---|
| `py -3.13 -m unittest discover -s tests/infra/finops` | **63 pruebas OK** |
| `py -3.13 infra/finops/consultas_readonly.py --validar` | `CATALOGO_LECTURA_OK: 16 consultas, ninguna mutante` |
| `py -3.13 infra/finops/requisitos.py` | `diseno_valido: true`, runtime `NOT_RUN` |
| `py -3.13 infra/finops/modelo_costos.py --texto` | C 84.0580 / A 91.8580 / B 29.0320 |
| `py -3.13 infra/finops/viabilidad.py` | `BLOCKED_FINOPS` |

## 5. Trazabilidad de requisitos

| Código | Cobertura | Estado |
|---|---|---|
| INF-000-01 Plan y contexto | Catálogo de consultas + evidencia de cuenta | `NOT_VERIFIED` |
| INF-000-02 Matriz por recurso | Catálogo de precios + matriz de costos | `PASS` (documental) |
| INF-000-03 Dos escenarios | Escenarios A/B/C en código | `PASS` (modelo) |
| INF-000-04 Costeo | Motor exacto + residual + créditos | `PASS` (cálculo) |
| INF-000-05 Bloqueo de decisión | `viabilidad.py` produce `BLOCKED_FINOPS` | `PASS` (regla aplicada) |
| INF-000-06 Controles | Consultas de presupuestos catalogadas; creación prohibida | `NOT_RUN` |
| INF-000-07 Investigación con fecha | Catálogo con `fecha_fuente` por SKU | `PASS` (parcial: sin revalidar) |
| INF-000-08 Separación de responsabilidades | Documentos marcan diseño vs runtime | `PASS` |

## 6. Limitaciones declaradas

- `VERIFIED_AWS` no aparece en ninguna partida: no hubo Price List API.
- Los 120 USD de créditos son un dato histórico del operador, no un saldo verificado.
- El costo residual supone que DynamoDB lecturas/escrituras cesan; se mantienen por prudencia.
- La validación de dos instancias es de diseño; **no** acredita instancias ni tráfico reales.
- No se creó ningún recurso AWS, presupuesto, alarma ni certificado.
