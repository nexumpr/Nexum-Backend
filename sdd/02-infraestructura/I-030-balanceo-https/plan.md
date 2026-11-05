# I-030-balanceo-https — Balanceador real, TLS, DNS, health y failover

| Campo | Valor |
|---|---|
| Etapa | H05 — Infraestructura AWS |
| Versión | 0.1.0 |
| Estado | **DRAFT / REVIEW — NO APROBADO** |
| Trazabilidad | A-030, I-000, I-010, I-020 |
| Implementación | H08+, tras H07 aprobado y autorización humana por release |

## 1. Estrategia de diseño documental de H05

Publicar URL HTTPS verificable y distribuir tráfico entre las dos instancias reales, con certificados válidos, checks y failover, sin confundir una CDN o proxy local con balanceo cloud.

## Autoridad y restricciones

- `constitution.md` (GOV-01..18, R-001..060) y el Gate H04 aprobado explícitamente el **2026-11-05** tienen precedencia; H05 diseña infraestructura, no la ejecuta.
- Arquitectura ratificada: Go (Clean Architecture), AWS, DynamoDB, **Redis real**, **2 instancias simultáneas**, balanceador HTTPS y CI/CD/DevSecOps. Ninguno es demostración opcional.
- El PDF del docente v3 exige Backend cloud con costo cero; el catálogo V6.0.2 enumera 75 rutas propuestas E001..E075 y utiliza stack histórico GCP/MongoDB **descartado**.
- No crear código Go, archivos Terraform `.tf`, pipelines ni endpoints H06 durante H05; `terraform apply/destroy`, `git push`, cambios AWS y facturación están prohibidos hasta fases y aprobaciones autorizadas.
- **Regla de evidencia:** documentos pueden alcanzar DRAFT completo sin acceso cloud; Gate I-GO requiere demostrar **viabilidad operacional y económica**, no asumirla. Campos pendientes no se cambian a PASS ni se solventan con créditos.

## 2. Plan secuencial y verificaciones

### P-030-01 — Alternativa candidata

**Entrada:** H04 aprobado, requisitos `A-030, I-000, I-010, I-020`, fuente AWS/cliente cuando aplique.

**Decisión a cerrar:** Application Load Balancer + target group EC2 con 2 targets y listener HTTPS:443 usando ACM público, reglas mínimas; opción concreta requiere precio/servicio permitido. No agregar API Gateway redundante sin ADR.

**Evidencia exigida:** tabla, diagrama, salida de consulta de solo lectura o análisis reproducible (según corresponda). Si requiere AWS no disponible, mantener OPEN/BLOCKED; no inventar resultado.

### P-030-02 — Certificado

**Entrada:** H04 aprobado, requisitos `A-030, I-000, I-010, I-020`, fuente AWS/cliente cuando aplique.

**Decisión a cerrar:** ACM público para nombre de dominio bajo control del proyecto, validación DNS, SNI/TLS 1.2+ y renovación; sin certificado autofirmado en URL pública. DNS o proveedor externo se costea y documenta.

**Evidencia exigida:** tabla, diagrama, salida de consulta de solo lectura o análisis reproducible (según corresponda). Si requiere AWS no disponible, mantener OPEN/BLOCKED; no inventar resultado.

### P-030-03 — Health

**Entrada:** H04 aprobado, requisitos `A-030, I-000, I-010, I-020`, fuente AWS/cliente cuando aplique.

**Decisión a cerrar:** Health interval/timeout/threshold y matcher 200 parametrizados, liveness separado de readiness. Estado healthy por instancia consultable; detectar 5xx y target unhealth.

**Evidencia exigida:** tabla, diagrama, salida de consulta de solo lectura o análisis reproducible (según corresponda). Si requiere AWS no disponible, mantener OPEN/BLOCKED; no inventar resultado.

### P-030-04 — Redirección

**Entrada:** H04 aprobado, requisitos `A-030, I-000, I-010, I-020`, fuente AWS/cliente cuando aplique.

**Decisión a cerrar:** HTTP:80 solo si hay decisión de redirección permanente a HTTPS y costo/SG permitido; de otra forma bloquear puerto 80. HSTS donde dominio/operación lo permitan, forwarded headers solo de proxies confiables.

**Evidencia exigida:** tabla, diagrama, salida de consulta de solo lectura o análisis reproducible (según corresponda). Si requiere AWS no disponible, mantener OPEN/BLOCKED; no inventar resultado.

### P-030-05 — Failover

**Entrada:** H04 aprobado, requisitos `A-030, I-000, I-010, I-020`, fuente AWS/cliente cuando aplique.

**Decisión a cerrar:** En prueba autorizada H08, impedir servicio de un nodo, health lo marca unhealthy y tráfico continúa al otro. Recuperar y comprobar retorno de ambos healthy; no demostrar solo round-robin.

**Evidencia exigida:** tabla, diagrama, salida de consulta de solo lectura o análisis reproducible (según corresponda). Si requiere AWS no disponible, mantener OPEN/BLOCKED; no inventar resultado.

### P-030-06 — Evidencias

**Entrada:** H04 aprobado, requisitos `A-030, I-000, I-010, I-020`, fuente AWS/cliente cuando aplique.

**Decisión a cerrar:** curl HTTPS con validación de certificado, cabeceras, status/latencia y requestId; listado de target health con IDs distintos; nombres de dominio/ALB reales y factura/costos de ALB.

**Evidencia exigida:** tabla, diagrama, salida de consulta de solo lectura o análisis reproducible (según corresponda). Si requiere AWS no disponible, mantener OPEN/BLOCKED; no inventar resultado.

## 3. Orden de ejecución futura H08 (no ejecutar ahora)

1. Auditar plan AWS/credenciales/precios y límites SCP sin cambios.
2. Preparar revisión de cambios, artefactos IaC y análisis económico en PR autorizado.
3. Ejecutar validaciones (fmt/validate/plan/pruebas), someter al gate humano.
4. Aprovisionar o modificar únicamente después de autorización de release y protección de datos.
5. Ejecutar pruebas reales, registrar métricas, costos y rollback, y no declarar DONE antes de las evidencias.

## 4. Dependencias cruzadas

I-000, I-010, I-020, I-060, I-120

## 5. Riesgos y tratamiento

1. `I-030-balanceo-https-R01`: ALB no habilitado en plan Free concreto. Responsable: Backend/infra; evidencia esperada: ADR, costo/caso de prueba o dictamen de servicio permitido; escalamiento humano obligatorio.
2. `I-030-balanceo-https-R02`: Certificado/dominio no disponibles. Responsable: Backend/infra; evidencia esperada: ADR, costo/caso de prueba o dictamen de servicio permitido; escalamiento humano obligatorio.
3. `I-030-balanceo-https-R03`: ALB mantiene cobro cuando los nodos están apagados. Responsable: Backend/infra; evidencia esperada: ADR, costo/caso de prueba o dictamen de servicio permitido; escalamiento humano obligatorio.

## 6. Puerta de entrada a H06

Gate H05/I-GO NO se logra con instalación de Markdown: requiere pruebas de viabilidad económica/cuenta, trazabilidad física Dynamo/Redis, controles IAM y aceptación expresa del responsable. Los contratos detallados E001..E075 se formalizan exclusivamente en H06.
