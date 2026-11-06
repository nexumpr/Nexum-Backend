# Gate H08-I — AUTORIZACIÓN LIMITADA EXCLUSIVA I-000

**ESTADO:** `AUTHORIZED_I000_LOCAL_AWS_READONLY` **solo si el operador ejecuta el instalador** con la frase exacta `AUTORIZO_H08I_I000_LOCAL_Y_AWS_LECTURA`, sobre `main` a commit `76ae14c` y rama de transición `chore/h08i-rfc002-rfc003-gate-i000`, seguido de revisión y publicación del PR. Este documento por sí solo no es una firma.

**Autoriza**: implementar una spec `sdd/02-infraestructura/I-000-cuenta-finops/{spec,plan,tasks,checklist}.md`, preparar costos 45 días, scripts/controles FinOps, obtener evidencias AWS por consultas read-only con permisos solicitados en OpenCode, tests locales y PR gestionado por OpenCode. La ratificación del instalador cubre el cambio de permisos; para empezar la implementación técnica el humano aprueba el alcance I-000 en la sesión.

**NO autoriza**: Terraform init/plan/apply/destroy, creación ni destrucción de recursos, subir de Free a Paid, obligaciones financieras personales, AWS mutante, despliegue, conectividad pública, I-010..I-150, H08-E/E001+, alterar specs aprobadas ni publicar secretos. 2 EC2 reales simultáneas, Redis OSS, DynamoDB, ALB/CloudFront HTTPS, IaC completa y 45 días siguen obligaciones para Gates posteriores, no estados PASS.

**Cierre de I-000:** evidencia de saldo, vigencia, precios exactos, SKU Redis, cuentas, cuotas, impuestos/IVA, riesgos y presupuesto; si falta viabilidad, BLOCKED_FINOPS. Ejecutar pruebas; PR y CI; decisión humana sobre cierre antes de I-010. El operador no ejecuta comandos manuales que OpenCode pueda ejecutar; solo aprueba permisos/acciones de riesgo y excepciones gráficas necesarias.


## Registro de cierre — 2026-11-06

- Politica vigente: API y DynamoDB 24/7; auxiliares (secretsmanager, ssm, ssmmessages) por ventanas de mantenimiento; `logs` permanente; `monitoring` y `ec2messages` no requeridos (metric filters y ssmmessages).
- Presupuesto 45 d: bruto verificado 79.0430 + conectividad 23.7400 = 102.7830; con margen 10 % = 113.0613 sobre 120.0000 verificados (sobrante 6.9387).
- Decisiones humanas D1 (IVA condicionado), D2 (TLS opcion A), D3 (VPC Origins diferido), D4 (runtime diferido), D5 (costo observado recurrente).
- D6 `APRUEBO_CIERRE_I000_CON_CONDICIONES` declarada efectiva al cumplirse las condiciones fijadas por el responsable.
- Verificaciones: 167 pruebas FinOps, validar_sdd PASS, validar_h06 PASS, 12 pruebas de herramientas, go build/vet/test PASS, CI documental SUCCESS.
- Sin recursos AWS creados; sin Terraform; plan Free intacto. Runtime y VPC Origins siguen NOT_RUN.
