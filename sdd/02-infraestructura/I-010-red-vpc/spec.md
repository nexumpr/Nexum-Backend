# I-010-red-vpc — Red VPC, zonas, seguridad y conectividad

| Campo | Valor |
|---|---|
| Etapa | H05 — Infraestructura AWS |
| Versión | 0.1.0 |
| Estado | **DRAFT / REVIEW — NO APROBADO** |
| Trazabilidad | A-020, A-030, A-050, I-000 |
| Implementación | H08+, tras H07 aprobado y autorización humana por release |

## 1. Objetivo y definición de terminado

Diseñar conectividad verificable para dos instancias Go, ALB HTTPS y un Redis compartido, minimizando egress y exposición y sin asumir que NAT Gateway/VPC endpoints son gratuitos.

## Autoridad y restricciones

- `constitution.md` (GOV-01..18, R-001..060) y el Gate H04 aprobado explícitamente el **2026-11-05** tienen precedencia; H05 diseña infraestructura, no la ejecuta.
- Arquitectura ratificada: Go (Clean Architecture), AWS, DynamoDB, **Redis real**, **2 instancias simultáneas**, balanceador HTTPS y CI/CD/DevSecOps. Ninguno es demostración opcional.
- El PDF del docente v3 exige Backend cloud con costo cero; el catálogo V6.0.2 enumera 75 rutas propuestas E001..E075 y utiliza stack histórico GCP/MongoDB **descartado**.
- No crear código Go, archivos Terraform `.tf`, pipelines ni endpoints H06 durante H05; `terraform apply/destroy`, `git push`, cambios AWS y facturación están prohibidos hasta fases y aprobaciones autorizadas.
- **Regla de evidencia:** documentos pueden alcanzar DRAFT completo sin acceso cloud; Gate I-GO requiere demostrar **viabilidad operacional y económica**, no asumirla. Campos pendientes no se cambian a PASS ni se solventan con créditos.

## 2. Contrato de infraestructura propuesto

### INF-010-01 — Topología mínima

VPC dedicada no superpuesta; al menos dos subredes/AZ si ALB lo exige, SG de entrada ALB:443, backend solo desde SG del ALB, Redis solo desde SG del backend, egress por destino justificable. Revisar quotas por zona.

**Comprobación posterior:** en H08 se verificará con evidencia no sensible de configuración/cloud/HTTP según el componente; ninguna afirmación de ejecución puede derivarse de este documento.

### INF-010-02 — Puertos y flujos

Cliente→ALB TLS443; ALB→Go en puerto interno elegido para health + API; Go→DynamoDB vía AWS SDK con IAM, Go→Redis TCP TLS; nodos→logs/actualizaciones por ruta controlada. Documentar por matriz origen-destino.

**Comprobación posterior:** en H08 se verificará con evidencia no sensible de configuración/cloud/HTTP según el componente; ninguna afirmación de ejecución puede derivarse de este documento.

### INF-010-03 — NAT e IP

NAT Gateway, endpoints privados, IPv4 públicas y tráfico de salida tienen costos o requisitos específicos; comparar rutas sin NAT y con endpoints, sin forzar IP pública a las dos instancias ni ocultar costo de IPv4.

**Comprobación posterior:** en H08 se verificará con evidencia no sensible de configuración/cloud/HTTP según el componente; ninguna afirmación de ejecución puede derivarse de este documento.

### INF-010-04 — Identidades y datos

TLS del cliente termina en ALB; si políticas exigen cifrado tránsito ALB→Go, instrumentar TLS interno y certificados gestionados. Redis autenticado y cifrado; no exponer puertos administrativos públicamente.

**Comprobación posterior:** en H08 se verificará con evidencia no sensible de configuración/cloud/HTTP según el componente; ninguna afirmación de ejecución puede derivarse de este documento.

### INF-010-05 — Controles de seguridad

SG least privilege, no SSH público, acceso operativo temporal aprobado (SSM si autorizado) o canal compatible con SCP; registros de acceso con retención y PII mínima.

**Comprobación posterior:** en H08 se verificará con evidencia no sensible de configuración/cloud/HTTP según el componente; ninguna afirmación de ejecución puede derivarse de este documento.

### INF-010-06 — Alcance

Terraform futuro creará red/SG/rutas desde módulos; aquí únicamente definición. Evitar Default VPC como dependencia implícita o mezcla con redes de otros proyectos.

**Comprobación posterior:** en H08 se verificará con evidencia no sensible de configuración/cloud/HTTP según el componente; ninguna afirmación de ejecución puede derivarse de este documento.

## 3. Dependencias y secuencia

I-000, I-020, I-030, I-050, I-060

## 4. Criterios de aceptación de diseño (Gate H05)

- Diagrama red con CIDR, subredes, SG e interfaces incluidas.
- Matriz de flujos con pruebas positivas/negativas documentadas.
- Ruta de salida de backend compatible con DynamoDB, logs y actualización segura.
- Sin exposición de Redis ni puertos administrativos al público.
- Estimación de NAT/endpoints/IP en matriz I-000, incluso cuando se elija no usarlos.

## 5. Riesgos que impiden una aprobación sin evidencia

- **Riesgo:** Costo NAT/endpoints e IPv4 inadvertido. Mitigación: registro explícito de causa, impacto, evidencia y ADR/RFC firmado; no asumir que ya está resuelto.
- **Riesgo:** AZ insuficientes en una cuenta Free. Mitigación: registro explícito de causa, impacto, evidencia y ADR/RFC firmado; no asumir que ya está resuelto.
- **Riesgo:** Rutas sin salida que dejan nodos irrecuperables. Mitigación: registro explícito de causa, impacto, evidencia y ADR/RFC firmado; no asumir que ya está resuelto.

## 6. Entregables y referencias

Los siguientes nombres identifican evidencias/artefactos **futuros** de I-GO y de implementación; no son archivos creados en H05: `DIAGRAMA_RED.md`, `MATRIZ_FLUJOS.md`, `TABLA_SG_RUTAS.md`.

## 7. Criterio de bloqueo y aprobación

**BLOCKED** si falta una obligación técnica, contradice alguna spec A aprobada, no puede comprobarse la viabilidad Free/SCP o no hay aceptación humana del riesgo. **DRAFT** significa que el diseño está documentado; nunca equivale a infraestructura aplicada.

## 8. Cierre tecnico-financiero H05 (propuesta v2; aun no aprobada)

- Diseño económico concreto: VPC /20 dedicada, dos subredes públicas por AZ para ALB + dos Go EC2 (rutas 0.0.0.0/0 al IGW) y al menos dos subredes privadas para ElastiCache (sin 0.0.0.0/0) con SG exclusivo; asignar a cada EC2 IPv4 pública (cobro intencional). No crear NAT Gateway ni endpoints privados de interfaz en esta modalidad. Gateway endpoints DynamoDB y S3 asociados a tablas de rutas de EC2.
- Diagrama de flujo: internet -> ALB:443 (ACM DNS-validado) -> SG-Go puerto interno /health/ready y API; EC2 Go -> SG-Redis TCP TLS:6379; EC2 -> DynamoDB gateway, S3 gateway, endpoints AWS externos por egress 443. Entradas EC2: solo SG del ALB, **no** 22/3389 de Internet, Redis privado, IMDSv2. Control egress restrictivo por puertos y rutas, controles IAM para API; si no se acepta EC2 con IPv4 pública aunque SG cerrado, pedir rediseño/recotización (no omitir NAT gratis).
- Se paga minimo 4 IP públicas: 2 ALB+2 EC2; el ALB puede usar más, comprobar factura. NAT=0 porque no se crea, no porque su servicio sea gratuito. Separar SG entre front/back/cache y acceso de administración vía SSM sobre TLS/443; prueba real futura en H08.


> **PRECEDENCIA H05:** esta enmienda V3 es la propuesta vigente frente a cualquier alternativa incompatible de secciones V2 anteriores. No se considera aprobada hasta firma humana Gate H05.
## 9. Topología v3 candidata sin IPv4 públicas — revisión antes del Gate

Preferir VPC `us-east-2` con dos subredes privadas en AZ diferentes: ALB **interno** con target group de dos Go EC2 privadas, Redis ElastiCache en redes privadas (1 nodo, sin atribuir HA) y CloudFront VPC origin asociado al ALB. CloudFront presenta HTTPS público en `*.cloudfront.net`; **0 IPv4 públicas de ALB o EC2**, sin NAT ni IPv4 elásticas. No inferir que «privado» elimina el cobro del ALB por hora/LCU. Sólo el par de Gateway Endpoints a DynamoDB y S3 es sin cargo adicional según AWS; asegurar rutas de cada tabla privada.

Precondiciones para H08: SG de VPC origin administrado por AWS + ingress específico al ALB, Go sólo desde SG ALB, Redis sólo desde SG Go, sin SSH/pública en instancias. El par Go obtiene artefactos ARM64 de S3 cifrado con IAM mínimo y endpoint gateway; despliegue/rotación de AMI se diseñará con mecanismo seguro que **no dependa de NAT, SSH ni endpoints de interfaz por defecto**. No usar CloudWatch Agent/SSM en privados sin cotizar Interface Endpoint; registros Go pueden archivarse cifrados a S3 gateway y métricas de ALB/CloudFront son de servicio. Establecer plan de mantenimiento de SO sin Internet antes de H08; si no resulta viable, recotizar alternativa antes de crear recursos. AWS CloudFront VPC origins no admite gRPC; Nexum usa REST/JSON. Confirmar service quota del SG/prefix list y restricciones de origen.
