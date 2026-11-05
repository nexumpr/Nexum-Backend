# I-030-balanceo-https — Balanceador real, TLS, DNS, health y failover

| Campo | Valor |
|---|---|
| Etapa | H05 — Infraestructura AWS |
| Versión | 0.1.0 |
| Estado | **DRAFT / REVIEW — NO APROBADO** |
| Trazabilidad | A-030, I-000, I-010, I-020 |
| Implementación | H08+, tras H07 aprobado y autorización humana por release |

## 1. Objetivo y definición de terminado

Publicar URL HTTPS verificable y distribuir tráfico entre las dos instancias reales, con certificados válidos, checks y failover, sin confundir una CDN o proxy local con balanceo cloud.

## Autoridad y restricciones

- `constitution.md` (GOV-01..18, R-001..060) y el Gate H04 aprobado explícitamente el **2026-11-05** tienen precedencia; H05 diseña infraestructura, no la ejecuta.
- Arquitectura ratificada: Go (Clean Architecture), AWS, DynamoDB, **Redis real**, **2 instancias simultáneas**, balanceador HTTPS y CI/CD/DevSecOps. Ninguno es demostración opcional.
- El PDF del docente v3 exige Backend cloud con costo cero; el catálogo V6.0.2 enumera 75 rutas propuestas E001..E075 y utiliza stack histórico GCP/MongoDB **descartado**.
- No crear código Go, archivos Terraform `.tf`, pipelines ni endpoints H06 durante H05; `terraform apply/destroy`, `git push`, cambios AWS y facturación están prohibidos hasta fases y aprobaciones autorizadas.
- **Regla de evidencia:** documentos pueden alcanzar DRAFT completo sin acceso cloud; Gate I-GO requiere demostrar **viabilidad operacional y económica**, no asumirla. Campos pendientes no se cambian a PASS ni se solventan con créditos.

## 2. Contrato de infraestructura propuesto

### INF-030-01 — Alternativa candidata

Application Load Balancer + target group EC2 con 2 targets y listener HTTPS:443 usando ACM público, reglas mínimas; opción concreta requiere precio/servicio permitido. No agregar API Gateway redundante sin ADR.

**Comprobación posterior:** en H08 se verificará con evidencia no sensible de configuración/cloud/HTTP según el componente; ninguna afirmación de ejecución puede derivarse de este documento.

### INF-030-02 — Certificado

ACM público para nombre de dominio bajo control del proyecto, validación DNS, SNI/TLS 1.2+ y renovación; sin certificado autofirmado en URL pública. DNS o proveedor externo se costea y documenta.

**Comprobación posterior:** en H08 se verificará con evidencia no sensible de configuración/cloud/HTTP según el componente; ninguna afirmación de ejecución puede derivarse de este documento.

### INF-030-03 — Health

Health interval/timeout/threshold y matcher 200 parametrizados, liveness separado de readiness. Estado healthy por instancia consultable; detectar 5xx y target unhealth.

**Comprobación posterior:** en H08 se verificará con evidencia no sensible de configuración/cloud/HTTP según el componente; ninguna afirmación de ejecución puede derivarse de este documento.

### INF-030-04 — Redirección

HTTP:80 solo si hay decisión de redirección permanente a HTTPS y costo/SG permitido; de otra forma bloquear puerto 80. HSTS donde dominio/operación lo permitan, forwarded headers solo de proxies confiables.

**Comprobación posterior:** en H08 se verificará con evidencia no sensible de configuración/cloud/HTTP según el componente; ninguna afirmación de ejecución puede derivarse de este documento.

### INF-030-05 — Failover

En prueba autorizada H08, impedir servicio de un nodo, health lo marca unhealthy y tráfico continúa al otro. Recuperar y comprobar retorno de ambos healthy; no demostrar solo round-robin.

**Comprobación posterior:** en H08 se verificará con evidencia no sensible de configuración/cloud/HTTP según el componente; ninguna afirmación de ejecución puede derivarse de este documento.

### INF-030-06 — Evidencias

curl HTTPS con validación de certificado, cabeceras, status/latencia y requestId; listado de target health con IDs distintos; nombres de dominio/ALB reales y factura/costos de ALB.

**Comprobación posterior:** en H08 se verificará con evidencia no sensible de configuración/cloud/HTTP según el componente; ninguna afirmación de ejecución puede derivarse de este documento.

## 3. Dependencias y secuencia

I-000, I-010, I-020, I-060, I-120

## 4. Criterios de aceptación de diseño (Gate H05)

- URL pública con certificado de CA confiable, TLS 1.2+.
- Distribución a dos targets simultáneos demostrada después del deploy.
- Failover de un nodo sin pérdida de persistencia.
- Cálculo ALB hourly+LCU y uso IPv4 / DNS verificado.
- No publicar credenciales ni origen interno en headers.

## 5. Riesgos que impiden una aprobación sin evidencia

- **Riesgo:** ALB no habilitado en plan Free concreto. Mitigación: registro explícito de causa, impacto, evidencia y ADR/RFC firmado; no asumir que ya está resuelto.
- **Riesgo:** Certificado/dominio no disponibles. Mitigación: registro explícito de causa, impacto, evidencia y ADR/RFC firmado; no asumir que ya está resuelto.
- **Riesgo:** ALB mantiene cobro cuando los nodos están apagados. Mitigación: registro explícito de causa, impacto, evidencia y ADR/RFC firmado; no asumir que ya está resuelto.

## 6. Entregables y referencias

Los siguientes nombres identifican evidencias/artefactos **futuros** de I-GO y de implementación; no son archivos creados en H05: `CONTRATO_TLS_Y_HEALTH.md`, `MATRIZ_COSTOS_LB.md`, `PLAN_FAILOVER.md`.

## 7. Criterio de bloqueo y aprobación

**BLOCKED** si falta una obligación técnica, contradice alguna spec A aprobada, no puede comprobarse la viabilidad Free/SCP o no hay aceptación humana del riesgo. **DRAFT** significa que el diseño está documentado; nunca equivale a infraestructura aplicada.

## 8. Cierre tecnico-financiero H05 (propuesta v2; aun no aprobada)

- Seleccionar ALB Application, internet-facing, al menos 2 AZ y dos IPv4 públicas facturables, HTTP/2 y TLS:443 ACM público **no exportable** con validación DNS de dominio controlado, healthcheck /health/ready y deregistration delay. Requiere dominio propio real antes de HTTPS validado: nombre `*.elb.amazonaws.com` no es un dominio del que podamos emitir certificados ACM propios. Listener 80 opcional solo redirect 301 a 443, sin TLS falso.
- Presupuesto: $0.0225/h fijo + $0.008 por LCU-h de consumo con supuesto 1 LCU promedio + $0.005/IP-h por 2 IP; monitorear LCUs, escalamiento de IP, errores 5xx y solicitud a cada target. No prometer tarifa plana: superar 1 LCU excederá el presupuesto de referencia.


> **PRECEDENCIA H05:** esta enmienda V3 es la propuesta vigente frente a cualquier alternativa incompatible de secciones V2 anteriores. No se considera aprobada hasta firma humana Gate H05.
## 9. Alternativa concreta v3: CloudFront TLS público con ALB privado (pendiente de ADR)

El requisito GOV-03 exige balanceo real con acceso API público HTTPS válido. Propuesta C: `https://dXXXXXXXX.cloudfront.net` (certificado AWS por defecto) → CloudFront VPC origin → **ALB interno real** (listener HTTP privado) → 2 targets EC2 separados sanos. CloudFront NO es el balanceador de los dos targets: ALB cumple esa función. No hay dominio propio, ACM extra, Route 53 ni IP públicas. El componente de origen es un recurso VPC de CloudFront; no se reemplaza por exposición directa de ALB. Políticas de CloudFront: ViewerProtocolPolicy=HTTPS-only/RedirectToHTTPS (si procede); métodos REST completos incluidos POST/PUT/PATCH/DELETE/OPTIONS; deshabilitar caching de respuestas de API autenticadas/personalizadas, reenvío de Authorization/cookies/headers requeridos, CORS definido, preservar semántica JWT, errores y cache-control; no Cache Everything, WAF con cargo no habilitado por defecto. TTL 0 para APIs de estado financiero; seguridad/firma y rate-limit backend.

**Riesgo informado:** TLS cliente→CloudFront; el trayecto CloudFront→ALB por VPC es privado pero se plantea **HTTP sin TLS de aplicación**. Es una desviación respecto a la alternativa previa `ALB listener HTTPS:443`; no es cifrado extremo a extremo a nivel HTTP. El Gate sólo puede aprobar C si el autorizador acepta este límite y el requisito docente no exige TLS en cada salto. Si exige HTTPS hasta el ALB, bloquear C y elegir nombre DNS/certificado bajo control SIN pagos personales y recotizar; nunca presentar HTTP privado como TLS extremo a extremo. Evidenciar el esquema en OpenAPI `servers`, test TLS desde móvil, y distribución de dos targets en H08. Ningún listener ni CDN se ha creado en H05.
