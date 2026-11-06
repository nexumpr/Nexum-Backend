# Evidencia de ejecución — I-000 (revisión 2)

**Fecha:** 2026-11-06 · **Rama:** `feat/h08i-i000-finops` · **PR:** #15
**Entorno:** Windows 11 nativo, PowerShell 7, sin Docker/WSL · **Recursos AWS creados:** ninguno

## 1. Estado de partida

| Hecho | Valor |
|---|---|
| Sesión anterior | Terminó con trabajo local sin commitear (9 archivos modificados, 23 nuevos) |
| Commit base | `d19ecec` |
| PR #15 | `OPEN`, CI documental SUCCESS (run 42261696344) |
| Sesión AWS | **válida** en esta corrida (perfil `nexum`) |

Ningún cambio local se descartó: no se usó `reset --hard`, `clean` ni `checkout --`.

## 2. Defectos corregidos

| # | Defecto | Causa | Corrección | Prueba |
|---|---|---|---|---|
| 1 | `aws servicequotas get-service-quota` → `Invalid choice 'servicequotas'` | Nombre de servicio inválido en AWS CLI | `servicio_api = "service-quotas"` | `test_service_quotas_no_usa_el_nombre_invalido` |
| 2 | `NotADirectoryError: [WinError 267] '$env:TEMP'` | Se pasó la cadena literal `$env:TEMP` como ruta | `tempfile.gettempdir()` + `directorio_crudo_por_defecto()` | `test_directorio_crudo_no_es_literal_de_powershell`, `test_directorio_crudo_fuera_del_repositorio` |
| 3 | Price List con `--region` duplicado | Se emitía `us-east-2` y `us-east-1` en el mismo argv | `region_endpoint` único por consulta | `test_un_solo_region_en_todas_las_consultas` |
| 4 | `--filters` repetido | Filtros incrustados en `argumentos` | Un único `--filters` construido desde `filtros` estructurados | `test_pricing_usa_un_unico_filters`, `test_filtros_bien_formados` |
| 5 | Código de ELB inválido | Se usaba `ElasticLoadBalancing` | `AWSELB` (confirmado con `get-products`) | `test_elb_usa_awselb` |
| 6 | ElastiCache con tarifa de soporte extendido (0.026) | Selección por unidad `Hrs` sin `usagetype` | Se exige `USE2-NodeUsage:cache.t4g.micro` (0.016) | `test_elasticache_elige_nodeusage_no_extended_support` |
| 7 | DynamoDB almacenamiento en 0 USD | Se tomaba el primer tramo (0–25 GB gratuito) | Se elige el primer tramo con precio > 0 (0.25) | `test_no_toma_el_tramo_gratuito` |
| 8 | Evidencia AWS con campos vacíos | Se leía `<crudo>/<id>.json` en lugar de `evidencia/<id>.json` | Lectura desde evidencia sanitizada con respaldo al crudo | `test_lee_del_directorio_de_evidencia` |
| 9 | Campos AWS inexistentes | Se asumían nombres no publicados por la API | Nombres reales: `accountPlanType`, `accountPlanRemainingCredits`, `VpcOriginList.Items`, `freeTierUsages` | `test_creditos_tomados_de_aws`, `test_vpc_origins_no_se_deduce_de_list` |
| 10 | Sanitizador marcaba su propio marcador | Patrón que detectaba `Account":"<ACCOUNT_ID>"` | Patrón eliminado | `test_sin_patrones_sensibles` |

## 3. Consultas AWS ejecutadas (sólo lectura)

| Consulta | Estado | Dato obtenido |
|---|---|---|
| `aws sts get-caller-identity --profile nexum --region us-east-2` | PASS | identidad válida (cuenta enmascarada) |
| `aws freetier get-account-plan-state` | PASS | `FREE` / `ACTIVE` / expira 2027-05-04 / créditos 120.0 USD |
| `aws freetier get-free-tier-usage` | PASS | `freeTierUsages: []` |
| `aws service-quotas get-service-quota --service-code ec2 --quota-code L-1216C47A` | PASS | 16 vCPU On-Demand |
| `aws ec2 describe-instance-type-offerings` (`t4g.micro`) | PASS | 2a, 2b, 2c |
| `aws elasticache describe-cache-engine-versions --engine redis` | PASS | 6 versiones, hasta 7.1 (`redis7`) |
| `aws cloudfront list-vpc-origins` | PASS | 0 orígenes existentes (elegibilidad no acreditada) |
| `aws dynamodb describe-limits` | PASS | 80 000 / 80 000 lecturas-escrituras de cuenta |
| `aws pricing get-products` (14 SKU) | PASS 14/14 | precios on-demand con `usagetype` y vigencia |
| `aws ce get-cost-and-usage` | **NO EJECUTADO** | 0.01 USD por solicitud, sin aprobación específica |

Todas las salidas crudas se escribieron en `%TEMP%\nexum_finops_crudo` (fuera del repositorio) y
sólo la versión sanitizada se versionó en `infra/finops/evidencia/`.

## 4. Artefactos

| Archivo | Cambio |
|---|---|
| `infra/finops/consultas_readonly.py` | Reescrito: filtros estructurados, `region_endpoint`, `service-quotas`, validación de códigos y de unidad esperada |
| `infra/finops/precios_api.py` | Nuevo: extrae precios reales, excluye tramos gratuitos y SKU de soporte extendido, no reconsulta evidencia válida |
| `infra/finops/evidencia_aws.py` | Nuevo: construye cuenta/servicios/free tier desde respuestas reales |
| `infra/finops/modelo_costos.py` | Bruto calculado vs verificado, cobertura, importe sin cobertura, riesgo de cobro, margen, impuestos |
| `infra/finops/viabilidad.py` | `VIABLE` exige cuenta vigente ≥ 45 días, precios verificados, cobertura TOTAL, IVA determinado y TLS resuelto |
| `infra/finops/requisitos.py` | `DESIGN_VALIDATED` separado de `AWS_DEPLOYMENT_VERIFIED`; rechazo de autocertificación |
| `infra/finops/sanitizar.py` | Eliminado el patrón que marcaba el propio marcador |
| `infra/finops/precios_us-east-2.json` | 14 SKU `VERIFIED_AWS` con `usagetype` y `fecha_fuente` |
| `infra/finops/resultados/costos_45d.json`, `viabilidad_i000.json` | Regenerados con datos reales |
| `tests/infra/finops/` | 9 archivos, **140 pruebas** |

## 5. Pruebas ejecutadas

| Comando | Resultado |
|---|---|
| `py -3.13 -m unittest discover -s tests/infra/finops` | **140 pruebas OK** |
| `py -3.13 infra/finops/consultas_readonly.py --validar` | `CATALOGO_LECTURA_OK: 24 consultas, ninguna mutante, region y filtros validados` |
| `py -3.13 infra/finops/precios_api.py --aplicar` | `verificados=14/14` |
| `py -3.13 infra/finops/evidencia_aws.py` | 8/8 consultas PASS |
| `py -3.13 infra/finops/modelo_costos.py --texto` | C 84.0080 · A 91.8080 · B 28.9820 · residual 6.8440 |
| `py -3.13 infra/finops/viabilidad.py` | `BLOCKED_FINOPS` con 7 motivos |

## 6. Límites declarados

- El bruto verificado es `UNKNOWN`: 11 partidas requeridas no tienen precio `VERIFIED_AWS`.
- No se ejecutó Cost Explorer; el costo observado sigue `NOT_RUN`.
- La elegibilidad de CloudFront VPC Origins no se deduce de `list-vpc-origins`.
- La verificación de dos instancias es de **diseño**; el runtime es `NOT_RUN` y no existe archivo
  `despliegue_runtime_*.json`.
- No se creó ningún recurso, presupuesto, alarma, certificado ni se ejecutó Terraform.
