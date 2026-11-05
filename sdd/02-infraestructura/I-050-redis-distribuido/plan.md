# I-050-redis-distribuido — Redis compartido real y seguridad

| Campo | Valor |
|---|---|
| Etapa | H05 — Infraestructura AWS |
| Versión | 0.1.0 |
| Estado | **DRAFT / REVIEW — NO APROBADO** |
| Trazabilidad | A-080, I-000, I-010, I-020 |
| Implementación | H08+, tras H07 aprobado y autorización humana por release |

## 1. Estrategia de diseño documental de H05

Diseñar Redis operativo y accesible desde ambos nodos Go con TTL, autenticación y límites distribuidos; nunca sustituirlo por caché de memoria local.

## Autoridad y restricciones

- `constitution.md` (GOV-01..18, R-001..060) y el Gate H04 aprobado explícitamente el **2026-11-05** tienen precedencia; H05 diseña infraestructura, no la ejecuta.
- Arquitectura ratificada: Go (Clean Architecture), AWS, DynamoDB, **Redis real**, **2 instancias simultáneas**, balanceador HTTPS y CI/CD/DevSecOps. Ninguno es demostración opcional.
- El PDF del docente v3 exige Backend cloud con costo cero; el catálogo V6.0.2 enumera 75 rutas propuestas E001..E075 y utiliza stack histórico GCP/MongoDB **descartado**.
- No crear código Go, archivos Terraform `.tf`, pipelines ni endpoints H06 durante H05; `terraform apply/destroy`, `git push`, cambios AWS y facturación están prohibidos hasta fases y aprobaciones autorizadas.
- **Regla de evidencia:** documentos pueden alcanzar DRAFT completo sin acceso cloud; Gate I-GO requiere demostrar **viabilidad operacional y económica**, no asumirla. Campos pendientes no se cambian a PASS ni se solventan con créditos.

## 2. Plan secuencial y verificaciones

### P-050-01 — Servicio

**Entrada:** H04 aprobado, requisitos `A-080, I-000, I-010, I-020`, fuente AWS/cliente cuando aplique.

**Decisión a cerrar:** Evaluar ElastiCache Redis OSS compatible con plan/costo; si no está permitido, evaluar Redis OSS autogestionado en una tercera instancia aprobada, aislada y presupuestada. Valkey no debe reemplazar Redis sin ADR humano explícito si la exigencia es Redis.

**Evidencia exigida:** tabla, diagrama, salida de consulta de solo lectura o análisis reproducible (según corresponda). Si requiere AWS no disponible, mantener OPEN/BLOCKED; no inventar resultado.

### P-050-02 — Topología

**Entrada:** H04 aprobado, requisitos `A-080, I-000, I-010, I-020`, fuente AWS/cliente cuando aplique.

**Decisión a cerrar:** Una única instancia Redis compartida representa punto único de fallo: registrar RTO y degradación. Para HA, nodos adicionales + réplica/sentinel y costo adicional; no afirmar alta disponibilidad sin prueba ni financiación.

**Evidencia exigida:** tabla, diagrama, salida de consulta de solo lectura o análisis reproducible (según corresponda). Si requiere AWS no disponible, mantener OPEN/BLOCKED; no inventar resultado.

### P-050-03 — Red

**Entrada:** H04 aprobado, requisitos `A-080, I-000, I-010, I-020`, fuente AWS/cliente cuando aplique.

**Decisión a cerrar:** Redis privado, SG solo Go, TLS habilitado end-to-end, ACL/usuario dedicado y credenciales rotables; no puertos públicos, no claves de usuario en texto claro.

**Evidencia exigida:** tabla, diagrama, salida de consulta de solo lectura o análisis reproducible (según corresponda). Si requiere AWS no disponible, mantener OPEN/BLOCKED; no inventar resultado.

### P-050-04 — Objetivos

**Entrada:** H04 aprobado, requisitos `A-080, I-000, I-010, I-020`, fuente AWS/cliente cuando aplique.

**Decisión a cerrar:** Cachear catálogos 1h, dashboard 20-60s, analytics 1-5min (semillas); claves con versión + HMAC usuario, TTL, invalidación after commit; atomizar rate-limits login/requests con Lua o primitivas seguras.

**Evidencia exigida:** tabla, diagrama, salida de consulta de solo lectura o análisis reproducible (según corresponda). Si requiere AWS no disponible, mantener OPEN/BLOCKED; no inventar resultado.

### P-050-05 — Fallo

**Entrada:** H04 aprobado, requisitos `A-080, I-000, I-010, I-020`, fuente AWS/cliente cuando aplique.

**Decisión a cerrar:** DynamoDB mantiene fuente de verdad; caché no puede devolver saldo antiguo después de una mutación. Auth/rate limit sensibles fallan cerrado según A-080; lecturas públicas pueden degradarse.

**Evidencia exigida:** tabla, diagrama, salida de consulta de solo lectura o análisis reproducible (según corresponda). Si requiere AWS no disponible, mantener OPEN/BLOCKED; no inventar resultado.

### P-050-06 — Prueba compartida

**Entrada:** H04 aprobado, requisitos `A-080, I-000, I-010, I-020`, fuente AWS/cliente cuando aplique.

**Decisión a cerrar:** Nodo A escribe clave temporal; nodo B la ve; invalidación realizada por nodo A se observa en B; reinicio de nodo verifica impacto y se detecta caída con métricas.

**Evidencia exigida:** tabla, diagrama, salida de consulta de solo lectura o análisis reproducible (según corresponda). Si requiere AWS no disponible, mantener OPEN/BLOCKED; no inventar resultado.

### P-050-07 — Costo

**Entrada:** H04 aprobado, requisitos `A-080, I-000, I-010, I-020`, fuente AWS/cliente cuando aplique.

**Decisión a cerrar:** Comparar ElastiCache, instancia EC2 dedicada y transferencia/almacenamiento/backup; preferir servicio soportado, no dejar Redis para «después». Evidencia de capacidad real antes de aprobar I-GO.

**Evidencia exigida:** tabla, diagrama, salida de consulta de solo lectura o análisis reproducible (según corresponda). Si requiere AWS no disponible, mantener OPEN/BLOCKED; no inventar resultado.

## 3. Orden de ejecución futura H08 (no ejecutar ahora)

1. Auditar plan AWS/credenciales/precios y límites SCP sin cambios.
2. Preparar revisión de cambios, artefactos IaC y análisis económico en PR autorizado.
3. Ejecutar validaciones (fmt/validate/plan/pruebas), someter al gate humano.
4. Aprovisionar o modificar únicamente después de autorización de release y protección de datos.
5. Ejecutar pruebas reales, registrar métricas, costos y rollback, y no declarar DONE antes de las evidencias.

## 4. Dependencias cruzadas

I-000, I-010, I-020, I-060, I-080, I-120

## 5. Riesgos y tratamiento

1. `I-050-redis-distribuido-R01`: ElastiCache no habilitado. Responsable: Backend/infra; evidencia esperada: ADR, costo/caso de prueba o dictamen de servicio permitido; escalamiento humano obligatorio.
2. `I-050-redis-distribuido-R02`: Autogestión agrega tercera instancia con costo. Responsable: Backend/infra; evidencia esperada: ADR, costo/caso de prueba o dictamen de servicio permitido; escalamiento humano obligatorio.
3. `I-050-redis-distribuido-R03`: Redis sin TLS/ACL. Responsable: Backend/infra; evidencia esperada: ADR, costo/caso de prueba o dictamen de servicio permitido; escalamiento humano obligatorio.
4. `I-050-redis-distribuido-R04`: Pérdida de caché o throttle genera inconsistencia. Responsable: Backend/infra; evidencia esperada: ADR, costo/caso de prueba o dictamen de servicio permitido; escalamiento humano obligatorio.

## 6. Puerta de entrada a H06

Gate H05/I-GO NO se logra con instalación de Markdown: requiere pruebas de viabilidad económica/cuenta, trazabilidad física Dynamo/Redis, controles IAM y aceptación expresa del responsable. Los contratos detallados E001..E075 se formalizan exclusivamente en H06.
