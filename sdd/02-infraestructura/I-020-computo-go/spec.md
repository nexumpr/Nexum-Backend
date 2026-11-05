# I-020-computo-go — Dos instancias Go reales, sistema y hardening

| Campo | Valor |
|---|---|
| Etapa | H05 — Infraestructura AWS |
| Versión | 0.1.0 |
| Estado | **DRAFT / REVIEW — NO APROBADO** |
| Trazabilidad | A-020, A-030, A-090, I-000, I-010 |
| Implementación | H08+, tras H07 aprobado y autorización humana por release |

## 1. Objetivo y definición de terminado

Definir dos procesos Go en dos instancias de cómputo independientes y simultáneas, con ejecución reproducible, observación de salud y protección ante fallos.

## Autoridad y restricciones

- `constitution.md` (GOV-01..18, R-001..060) y el Gate H04 aprobado explícitamente el **2026-11-05** tienen precedencia; H05 diseña infraestructura, no la ejecuta.
- Arquitectura ratificada: Go (Clean Architecture), AWS, DynamoDB, **Redis real**, **2 instancias simultáneas**, balanceador HTTPS y CI/CD/DevSecOps. Ninguno es demostración opcional.
- El PDF del docente v3 exige Backend cloud con costo cero; el catálogo V6.0.2 enumera 75 rutas propuestas E001..E075 y utiliza stack histórico GCP/MongoDB **descartado**.
- No crear código Go, archivos Terraform `.tf`, pipelines ni endpoints H06 durante H05; `terraform apply/destroy`, `git push`, cambios AWS y facturación están prohibidos hasta fases y aprobaciones autorizadas.
- **Regla de evidencia:** documentos pueden alcanzar DRAFT completo sin acceso cloud; Gate I-GO requiere demostrar **viabilidad operacional y económica**, no asumirla. Campos pendientes no se cambian a PASS ni se solventan con créditos.

## 2. Contrato de infraestructura propuesto

### INF-020-01 — Cómputo

Evaluar EC2+Launch Template/Auto Scaling Group con desired=2,min=2,max=2 mientras el servicio esté encendido. Dos procesos en la misma VM o un contenedor duplicado en un host NO satisfacen el requisito. Tipo/AMI/arquitectura se eligen según precio, capacidad y disponibilidad real.

**Comprobación posterior:** en H08 se verificará con evidencia no sensible de configuración/cloud/HTTP según el componente; ninguna afirmación de ejecución puede derivarse de este documento.

### INF-020-02 — Binario

Build Go con dependencias fijadas y hash SHA256 por release, servicio systemd de usuario no privilegiado, read-only filesystem en lo posible, umask, restart bounded, graceful shutdown de HTTP con timeout, límites de memoria/archivos.

**Comprobación posterior:** en H08 se verificará con evidencia no sensible de configuración/cloud/HTTP según el componente; ninguna afirmación de ejecución puede derivarse de este documento.

### INF-020-03 — Bootstrap

User-data mínimo, no secreto en scripts ni logs, artefactos verificados por digest, identidad de instancia via IAM role, config por entorno y secret retrieval seguro si el servicio está permitido.

**Comprobación posterior:** en H08 se verificará con evidencia no sensible de configuración/cloud/HTTP según el componente; ninguna afirmación de ejecución puede derivarse de este documento.

### INF-020-04 — Salud

GET /health/live solo proceso; /health/ready comprueba condiciones esenciales con timeout. ALB retira destino no ready; error de Redis se degrada según A-080 sin servir datos incorrectos; dependencia Dynamo crítica hace no-ready.

**Comprobación posterior:** en H08 se verificará con evidencia no sensible de configuración/cloud/HTTP según el componente; ninguna afirmación de ejecución puede derivarse de este documento.

### INF-020-05 — Simultaneidad comprobable

Capturar dos instance-ids, dos IPs privadas distintas, ambos targets healthy concurrentemente, contar solicitudes a cada nodo mediante trace/nodo-ID seguro, aislar un nodo y demostrar servicio del otro sin perder estado.

**Comprobación posterior:** en H08 se verificará con evidencia no sensible de configuración/cloud/HTTP según el componente; ninguna afirmación de ejecución puede derivarse de este documento.

### INF-020-06 — Capacidad

Plan de carga básico: RAM/CPU p95, conexiones Dynamo/Redis, timeout, tamaño de binario y carga a 2 nodos; no cambiar a un solo nodo por ahorro.

**Comprobación posterior:** en H08 se verificará con evidencia no sensible de configuración/cloud/HTTP según el componente; ninguna afirmación de ejecución puede derivarse de este documento.

## 3. Dependencias y secuencia

I-000, I-010, I-030, I-060, I-090, I-120

## 4. Criterios de aceptación de diseño (Gate H05)

- Diseño Launch Template/ASG deseado2 y roles sin hardcoded AMI.
- Demostración posterior de 2 instance-id simultáneos y healthy en target group.
- Prueba posterior de reinicio de un nodo y continuidad con otro.
- Endpoint health sin secretos y logs correlacionados.
- Costo real de los dos nodos incluido en 45 días y ventanas.

## 5. Riesgos que impiden una aprobación sin evidencia

- **Riesgo:** Plan Free limita cantidad/tipo EC2. Mitigación: registro explícito de causa, impacto, evidencia y ADR/RFC firmado; no asumir que ya está resuelto.
- **Riesgo:** Dos instancias agotan presupuesto. Mitigación: registro explícito de causa, impacto, evidencia y ADR/RFC firmado; no asumir que ya está resuelto.
- **Riesgo:** Inicio fallido por egress/descarga de artefactos. Mitigación: registro explícito de causa, impacto, evidencia y ADR/RFC firmado; no asumir que ya está resuelto.
- **Riesgo:** Desplegar nueva versión en un solo nodo deja incompatibilidad. Mitigación: registro explícito de causa, impacto, evidencia y ADR/RFC firmado; no asumir que ya está resuelto.

## 6. Entregables y referencias

Los siguientes nombres identifican evidencias/artefactos **futuros** de I-GO y de implementación; no son archivos creados en H05: `DISENO_DOS_INSTANCIAS.md`, `PLAN_PRUEBA_2_NODOS.md`, `HARDENING_EC2.md`.

## 7. Criterio de bloqueo y aprobación

**BLOCKED** si falta una obligación técnica, contradice alguna spec A aprobada, no puede comprobarse la viabilidad Free/SCP o no hay aceptación humana del riesgo. **DRAFT** significa que el diseño está documentado; nunca equivale a infraestructura aplicada.

## 8. Cierre tecnico-financiero H05 (propuesta v2; aun no aprobada)

- Tipos de instancia seleccionados condicionalmente `t4g.micro` arm64, 2 unidades reales en 2 AZ, EC2 estándar total 4 vCPU; quota de EC2 predeterminada 5 vCPU, verificar quota actual >=4 antes de H08. Binario Go `GOOS=linux GOARCH=arm64 CGO_ENABLED=0` cuando dependencias lo permitan; unit systemd sin Docker; AMI ARM64 soportada, EBS gp3 10 GB por host y actualización segura.
- CPU `standard` (no Unlimited) salvo RFC/cotización: performance puede limitarse; métricas CPU credit y validación de capacidad. ASG min=2 desired=2 max=2 cuando ON; al apagar por ventanas min=0 desired=0 max=2, sin ocultar que ALB/Redis siguen facturando. En H08 certificar 2 EC2 IDs y 2 ALB targets saludables al mismo tiempo.


> **PRECEDENCIA H05:** esta enmienda V3 es la propuesta vigente frente a cualquier alternativa incompatible de secciones V2 anteriores. No se considera aprobada hasta firma humana Gate H05.
## 9. Enmienda v3 de disponibilidad y cómputo

Las **dos** `t4g.micro` Go (Graviton ARM64, 1 GiB RAM cada una) se mantienen simultáneamente encendidas y sanas 24×7 durante 45 días, ubicadas en dos AZ/subredes privadas; sin IPv4 pública. CPU burst credits (`unlimited`) pueden generar costo extra si se excede el baseline: evaluar métricas y preferir configuración compatible `standard` si se acepta el posible throttling; confirmar en EC2 pricing y monitorizar. No agregar una tercera EC2 sin cuota/costeo nuevos. Redis permanece administrado en ElastiCache, no se aloja en RAM de la EC2 Go en C.

Para desplegar sin SSH/NAT, artefactos versionados Go ARM64 en bucket S3 privado via Gateway Endpoint, firma/hash reproducible, política IAM sólo lectura para ejecución, autorización de releases desde GitHub CI/CD y comprobación de dos targets. Actualización OS mediante AMIs/base imágenes y sustitución controlada, no prometer yum/apt updates online en subred privada. En H08 demostrar factibilidad de bootstrap, mantenimiento, monitoreo, y rollback **antes de abrir servicio a equipo móvil**. No se ha ejecutado despliegue.
