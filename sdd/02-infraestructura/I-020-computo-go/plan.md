# I-020-computo-go — Dos instancias Go reales, sistema y hardening

| Campo | Valor |
|---|---|
| Etapa | H05 — Infraestructura AWS |
| Versión | 0.1.0 |
| Estado | **DRAFT / REVIEW — NO APROBADO** |
| Trazabilidad | A-020, A-030, A-090, I-000, I-010 |
| Implementación | H08+, tras H07 aprobado y autorización humana por release |

## 1. Estrategia de diseño documental de H05

Definir dos procesos Go en dos instancias de cómputo independientes y simultáneas, con ejecución reproducible, observación de salud y protección ante fallos.

## Autoridad y restricciones

- `constitution.md` (GOV-01..18, R-001..060) y el Gate H04 aprobado explícitamente el **2026-11-05** tienen precedencia; H05 diseña infraestructura, no la ejecuta.
- Arquitectura ratificada: Go (Clean Architecture), AWS, DynamoDB, **Redis real**, **2 instancias simultáneas**, balanceador HTTPS y CI/CD/DevSecOps. Ninguno es demostración opcional.
- El PDF del docente v3 exige Backend cloud con costo cero; el catálogo V6.0.2 enumera 75 rutas propuestas E001..E075 y utiliza stack histórico GCP/MongoDB **descartado**.
- No crear código Go, archivos Terraform `.tf`, pipelines ni endpoints H06 durante H05; `terraform apply/destroy`, `git push`, cambios AWS y facturación están prohibidos hasta fases y aprobaciones autorizadas.
- **Regla de evidencia:** documentos pueden alcanzar DRAFT completo sin acceso cloud; Gate I-GO requiere demostrar **viabilidad operacional y económica**, no asumirla. Campos pendientes no se cambian a PASS ni se solventan con créditos.

## 2. Plan secuencial y verificaciones

### P-020-01 — Cómputo

**Entrada:** H04 aprobado, requisitos `A-020, A-030, A-090, I-000, I-010`, fuente AWS/cliente cuando aplique.

**Decisión a cerrar:** Evaluar EC2+Launch Template/Auto Scaling Group con desired=2,min=2,max=2 mientras el servicio esté encendido. Dos procesos en la misma VM o un contenedor duplicado en un host NO satisfacen el requisito. Tipo/AMI/arquitectura se eligen según precio, capacidad y disponibilidad real.

**Evidencia exigida:** tabla, diagrama, salida de consulta de solo lectura o análisis reproducible (según corresponda). Si requiere AWS no disponible, mantener OPEN/BLOCKED; no inventar resultado.

### P-020-02 — Binario

**Entrada:** H04 aprobado, requisitos `A-020, A-030, A-090, I-000, I-010`, fuente AWS/cliente cuando aplique.

**Decisión a cerrar:** Build Go con dependencias fijadas y hash SHA256 por release, servicio systemd de usuario no privilegiado, read-only filesystem en lo posible, umask, restart bounded, graceful shutdown de HTTP con timeout, límites de memoria/archivos.

**Evidencia exigida:** tabla, diagrama, salida de consulta de solo lectura o análisis reproducible (según corresponda). Si requiere AWS no disponible, mantener OPEN/BLOCKED; no inventar resultado.

### P-020-03 — Bootstrap

**Entrada:** H04 aprobado, requisitos `A-020, A-030, A-090, I-000, I-010`, fuente AWS/cliente cuando aplique.

**Decisión a cerrar:** User-data mínimo, no secreto en scripts ni logs, artefactos verificados por digest, identidad de instancia via IAM role, config por entorno y secret retrieval seguro si el servicio está permitido.

**Evidencia exigida:** tabla, diagrama, salida de consulta de solo lectura o análisis reproducible (según corresponda). Si requiere AWS no disponible, mantener OPEN/BLOCKED; no inventar resultado.

### P-020-04 — Salud

**Entrada:** H04 aprobado, requisitos `A-020, A-030, A-090, I-000, I-010`, fuente AWS/cliente cuando aplique.

**Decisión a cerrar:** GET /health/live solo proceso; /health/ready comprueba condiciones esenciales con timeout. ALB retira destino no ready; error de Redis se degrada según A-080 sin servir datos incorrectos; dependencia Dynamo crítica hace no-ready.

**Evidencia exigida:** tabla, diagrama, salida de consulta de solo lectura o análisis reproducible (según corresponda). Si requiere AWS no disponible, mantener OPEN/BLOCKED; no inventar resultado.

### P-020-05 — Simultaneidad comprobable

**Entrada:** H04 aprobado, requisitos `A-020, A-030, A-090, I-000, I-010`, fuente AWS/cliente cuando aplique.

**Decisión a cerrar:** Capturar dos instance-ids, dos IPs privadas distintas, ambos targets healthy concurrentemente, contar solicitudes a cada nodo mediante trace/nodo-ID seguro, aislar un nodo y demostrar servicio del otro sin perder estado.

**Evidencia exigida:** tabla, diagrama, salida de consulta de solo lectura o análisis reproducible (según corresponda). Si requiere AWS no disponible, mantener OPEN/BLOCKED; no inventar resultado.

### P-020-06 — Capacidad

**Entrada:** H04 aprobado, requisitos `A-020, A-030, A-090, I-000, I-010`, fuente AWS/cliente cuando aplique.

**Decisión a cerrar:** Plan de carga básico: RAM/CPU p95, conexiones Dynamo/Redis, timeout, tamaño de binario y carga a 2 nodos; no cambiar a un solo nodo por ahorro.

**Evidencia exigida:** tabla, diagrama, salida de consulta de solo lectura o análisis reproducible (según corresponda). Si requiere AWS no disponible, mantener OPEN/BLOCKED; no inventar resultado.

## 3. Orden de ejecución futura H08 (no ejecutar ahora)

1. Auditar plan AWS/credenciales/precios y límites SCP sin cambios.
2. Preparar revisión de cambios, artefactos IaC y análisis económico en PR autorizado.
3. Ejecutar validaciones (fmt/validate/plan/pruebas), someter al gate humano.
4. Aprovisionar o modificar únicamente después de autorización de release y protección de datos.
5. Ejecutar pruebas reales, registrar métricas, costos y rollback, y no declarar DONE antes de las evidencias.

## 4. Dependencias cruzadas

I-000, I-010, I-030, I-060, I-090, I-120

## 5. Riesgos y tratamiento

1. `I-020-computo-go-R01`: Plan Free limita cantidad/tipo EC2. Responsable: Backend/infra; evidencia esperada: ADR, costo/caso de prueba o dictamen de servicio permitido; escalamiento humano obligatorio.
2. `I-020-computo-go-R02`: Dos instancias agotan presupuesto. Responsable: Backend/infra; evidencia esperada: ADR, costo/caso de prueba o dictamen de servicio permitido; escalamiento humano obligatorio.
3. `I-020-computo-go-R03`: Inicio fallido por egress/descarga de artefactos. Responsable: Backend/infra; evidencia esperada: ADR, costo/caso de prueba o dictamen de servicio permitido; escalamiento humano obligatorio.
4. `I-020-computo-go-R04`: Desplegar nueva versión en un solo nodo deja incompatibilidad. Responsable: Backend/infra; evidencia esperada: ADR, costo/caso de prueba o dictamen de servicio permitido; escalamiento humano obligatorio.

## 6. Puerta de entrada a H06

Gate H05/I-GO NO se logra con instalación de Markdown: requiere pruebas de viabilidad económica/cuenta, trazabilidad física Dynamo/Redis, controles IAM y aceptación expresa del responsable. Los contratos detallados E001..E075 se formalizan exclusivamente en H06.
