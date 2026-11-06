# infra/finops — Implementación I-000 (cuenta AWS, plan Free y costo cero)

**Spec:** `sdd/02-infraestructura/I-000-cuenta-finops/` · **Fase:** H08-I · **Rama:** `feat/h08i-i000-finops`
**Estado de la evidencia AWS:** `NOT_VERIFIED` (sesión AWS expirada) · **Decisión vigente:** `BLOCKED_FINOPS`

Este directorio contiene la implementación reproducible y auditable de I-000. No crea, modifica
ni destruye recursos AWS; no ejecuta Terraform; no cambia el plan Free a Paid.

## Componentes

| Archivo | Función |
|---|---|
| `dinero.py` | Aritmética monetaria exacta con `Decimal`; rechaza `float` |
| `precios_us-east-2.json` | Catálogo de precios con estado de evidencia por SKU y fecha de fuente |
| `escenarios.py` | Escenarios A (24×7 ALB público), B (ventanas 8 h) y C (CloudFront + VPC Origin) |
| `modelo_costos.py` | Cálculo determinista a 45 días, costo residual, créditos y desembolso |
| `viabilidad.py` | Regla de decisión `VIABLE` / `BLOCKED_FINOPS` con motivos y evidencia faltante |
| `requisitos.py` | Valida dos instancias reales simultáneas y red privada sin NAT |
| `consultas_readonly.py` | Catálogo de 32 consultas AWS de solo lectura; un único `--region` y un único `--filters` por comando |
| `precios_api.py` | Extrae precios reales de Price List; descarta tramos gratuitos y SKU de soporte extendido |
| `evidencia_aws.py` | Construye evidencia sanitizada de cuenta, servicios y Free Tier con los campos reales de AWS |
| `sanitizar.py` | Enmascara cuenta, ARN, correos, claves, IPv4 y tokens antes de versionar |
| `evidencia/` | Instantáneas sanitizadas con estado honesto (`VERIFIED_AWS`, `NOT_RUN`, `NOT_VERIFIED`) |
| `resultados/` | Salidas deterministas del modelo y de la decisión de viabilidad |

## Uso

```powershell
py -3.13 infra/finops/consultas_readonly.py --validar     # ninguna consulta mutante
py -3.13 infra/finops/consultas_readonly.py --listar      # catálogo con costo por consulta
py -3.13 infra/finops/modelo_costos.py --texto            # totales por escenario
py -3.13 infra/finops/requisitos.py                       # invariantes de topología
py -3.13 infra/finops/viabilidad.py                       # decisión VIABLE/BLOCKED_FINOPS
py -3.13 infra/finops/precios_api.py --aplicar            # revalida precios AWS (omite los ya verificados)
py -3.13 infra/finops/evidencia_aws.py                    # evidencia read-only de cuenta y servicios
py -3.13 -m unittest discover -s tests/infra/finops       # pruebas
```

Para ejecutar una consulta concreta:

```powershell
py -3.13 infra/finops/consultas_readonly.py --ejecutar plan_free
```

La salida cruda se escribe en `%TEMP%\nexum_finops_crudo` (fuera del repositorio); sólo la versión
sanitizada se guarda en `evidencia/`.

## Reglas de evidencia

- Un precio `null` o con estado `UNKNOWN` **nunca** es 0 USD y bloquea el total bruto.
- `DOC_ONLY`, `HIPOTESIS` y `SUPUESTO` son evidencia histórica o de modelado: **no** habilitan `VIABLE`.
- Los créditos declarados (120 USD del contexto H05) **no** son créditos verificados.
- Cost Explorer cobra USD 0.01 por solicitud: no se ejecuta sin aprobación humana específica.
- La comprobación de dos instancias y red privada valida el **diseño** (`DESIGN_VALIDATED`); el
  runtime sigue `NOT_RUN`. Para pasar a `AWS_DEPLOYMENT_VERIFIED` se exige evidencia de
  `aws_ec2_describe_instances` con huella SHA-256, dos IDs reales en AZ distintas y un documento de
  control con la frase `APRUEBO_DESPLIEGUE_VERIFICADO_AWS`.
- CloudFront VPC Origins exige un **Internet Gateway adjunto** (sin rutas públicas, sin NAT).
- `DOC_OFICIAL` sólo cuenta para el bruto verificado si el precio es 0 y existe `fuente_oficial`.
- El saldo de créditos se toma de `accountPlanRemainingCredits`; el gasto liquidado sigue en Billing.
