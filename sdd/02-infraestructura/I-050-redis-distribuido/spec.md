# I-050-redis-distribuido — Redis compartido real y seguridad

| Campo | Valor |
|---|---|
| Etapa | H05 — Infraestructura AWS |
| Versión | 0.1.0 |
| Estado | **DRAFT / REVIEW — NO APROBADO** |
| Trazabilidad | A-080, I-000, I-010, I-020 |
| Implementación | H08+, tras H07 aprobado y autorización humana por release |

## 1. Objetivo y definición de terminado

Diseñar Redis operativo y accesible desde ambos nodos Go con TTL, autenticación y límites distribuidos; nunca sustituirlo por caché de memoria local.

## Autoridad y restricciones

- `constitution.md` (GOV-01..18, R-001..060) y el Gate H04 aprobado explícitamente el **2026-11-05** tienen precedencia; H05 diseña infraestructura, no la ejecuta.
- Arquitectura ratificada: Go (Clean Architecture), AWS, DynamoDB, **Redis real**, **2 instancias simultáneas**, balanceador HTTPS y CI/CD/DevSecOps. Ninguno es demostración opcional.
- El PDF del docente v3 exige Backend cloud con costo cero; el catálogo V6.0.2 enumera 75 rutas propuestas E001..E075 y utiliza stack histórico GCP/MongoDB **descartado**.
- No crear código Go, archivos Terraform `.tf`, pipelines ni endpoints H06 durante H05; `terraform apply/destroy`, `git push`, cambios AWS y facturación están prohibidos hasta fases y aprobaciones autorizadas.
- **Regla de evidencia:** documentos pueden alcanzar DRAFT completo sin acceso cloud; Gate I-GO requiere demostrar **viabilidad operacional y económica**, no asumirla. Campos pendientes no se cambian a PASS ni se solventan con créditos.

## 2. Contrato de infraestructura propuesto

### INF-050-01 — Servicio

Evaluar ElastiCache Redis OSS compatible con plan/costo; si no está permitido, evaluar Redis OSS autogestionado en una tercera instancia aprobada, aislada y presupuestada. Valkey no debe reemplazar Redis sin ADR humano explícito si la exigencia es Redis.

**Comprobación posterior:** en H08 se verificará con evidencia no sensible de configuración/cloud/HTTP según el componente; ninguna afirmación de ejecución puede derivarse de este documento.

### INF-050-02 — Topología

Una única instancia Redis compartida representa punto único de fallo: registrar RTO y degradación. Para HA, nodos adicionales + réplica/sentinel y costo adicional; no afirmar alta disponibilidad sin prueba ni financiación.

**Comprobación posterior:** en H08 se verificará con evidencia no sensible de configuración/cloud/HTTP según el componente; ninguna afirmación de ejecución puede derivarse de este documento.

### INF-050-03 — Red

Redis privado, SG solo Go, TLS habilitado end-to-end, ACL/usuario dedicado y credenciales rotables; no puertos públicos, no claves de usuario en texto claro.

**Comprobación posterior:** en H08 se verificará con evidencia no sensible de configuración/cloud/HTTP según el componente; ninguna afirmación de ejecución puede derivarse de este documento.

### INF-050-04 — Objetivos

Cachear catálogos 1h, dashboard 20-60s, analytics 1-5min (semillas); claves con versión + HMAC usuario, TTL, invalidación after commit; atomizar rate-limits login/requests con Lua o primitivas seguras.

**Comprobación posterior:** en H08 se verificará con evidencia no sensible de configuración/cloud/HTTP según el componente; ninguna afirmación de ejecución puede derivarse de este documento.

### INF-050-05 — Fallo

DynamoDB mantiene fuente de verdad; caché no puede devolver saldo antiguo después de una mutación. Auth/rate limit sensibles fallan cerrado según A-080; lecturas públicas pueden degradarse.

**Comprobación posterior:** en H08 se verificará con evidencia no sensible de configuración/cloud/HTTP según el componente; ninguna afirmación de ejecución puede derivarse de este documento.

### INF-050-06 — Prueba compartida

Nodo A escribe clave temporal; nodo B la ve; invalidación realizada por nodo A se observa en B; reinicio de nodo verifica impacto y se detecta caída con métricas.

**Comprobación posterior:** en H08 se verificará con evidencia no sensible de configuración/cloud/HTTP según el componente; ninguna afirmación de ejecución puede derivarse de este documento.

### INF-050-07 — Costo

Comparar ElastiCache, instancia EC2 dedicada y transferencia/almacenamiento/backup; preferir servicio soportado, no dejar Redis para «después». Evidencia de capacidad real antes de aprobar I-GO.

**Comprobación posterior:** en H08 se verificará con evidencia no sensible de configuración/cloud/HTTP según el componente; ninguna afirmación de ejecución puede derivarse de este documento.

## 3. Dependencias y secuencia

I-000, I-010, I-020, I-060, I-080, I-120

## 4. Criterios de aceptación de diseño (Gate H05)

- 2 nodos acceden a mismo Redis; prueba compartida posterior.
- TLS/ACL y networking documentados.
- Lista de claves, TTL, invalidación por evento y rate limiting.
- Comportamiento ante caída: error/circuit-breaker consistente.
- Costo mensual y disponibilidad Free/SCP evidenciados.

## 5. Riesgos que impiden una aprobación sin evidencia

- **Riesgo:** ElastiCache no habilitado. Mitigación: registro explícito de causa, impacto, evidencia y ADR/RFC firmado; no asumir que ya está resuelto.
- **Riesgo:** Autogestión agrega tercera instancia con costo. Mitigación: registro explícito de causa, impacto, evidencia y ADR/RFC firmado; no asumir que ya está resuelto.
- **Riesgo:** Redis sin TLS/ACL. Mitigación: registro explícito de causa, impacto, evidencia y ADR/RFC firmado; no asumir que ya está resuelto.
- **Riesgo:** Pérdida de caché o throttle genera inconsistencia. Mitigación: registro explícito de causa, impacto, evidencia y ADR/RFC firmado; no asumir que ya está resuelto.

## 6. Entregables y referencias

Los siguientes nombres identifican evidencias/artefactos **futuros** de I-GO y de implementación; no son archivos creados en H05: `ADR_REDIS_PROVEEDOR.md`, `MATRIZ_KEYS_TTL.md`, `PLAN_PRUEBA_REDIS_COMPARTIDO.md`.

## 7. Criterio de bloqueo y aprobación

**BLOCKED** si falta una obligación técnica, contradice alguna spec A aprobada, no puede comprobarse la viabilidad Free/SCP o no hay aceptación humana del riesgo. **DRAFT** significa que el diseño está documentado; nunca equivale a infraestructura aplicada.

## 8. Cierre tecnico-financiero H05 (propuesta v2; aun no aprobada)

- Redis OSS **real administrado**, ElastiCache node-based On-Demand 1×`cache.t4g.micro`, NO Valkey ni serverless, no replica para mantener presupuesto; 0.5 GiB clase pequeña, probar saturacion / eviction. Subnet group privada en >=2 subredes/AZ disponibles (un nodo ubicado en una); SG ingress TCP6379 solo SG backend, TLS (in-transit encryption) + autenticación gestionada de forma segura, cifrado en reposo cuando soportado por el motor/clase. Verificar soporte de configuracion concreta antes de provisionar, no asumir que `cache.t4g.micro` admite todas las opciones de failover.
- Una sola cache no es alta disponibilidad. Redis = cache/session auxiliary, DynamoDB = fuente financiera; ante Redis no disponible, aplicar degrade/fail-close por operación y nunca perder estado financiero. No implementar dependencia de Redis como único almacén de revocación no recuperable; H06 precisará semántica. No apagar con Stop inexistente: se sigue cobrando hasta eliminación.
- Precio **$0.016/nodo-h** hallado en fuente secundaria de Ohio que declara usar AWS Price List; status `PRICE_SECONDARY_NOT_SKU_PROVEN` hasta consulta filtrada por motor, región, node, SKU, term OnDemand y ausencia de premium Extended Support. No reemplazar por $0.0128 Valkey.
