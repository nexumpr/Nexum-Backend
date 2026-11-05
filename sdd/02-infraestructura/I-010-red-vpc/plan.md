# I-010-red-vpc — Red VPC, zonas, seguridad y conectividad

| Campo | Valor |
|---|---|
| Etapa | H05 — Infraestructura AWS |
| Versión | 0.1.0 |
| Estado | **DRAFT / REVIEW — NO APROBADO** |
| Trazabilidad | A-020, A-030, A-050, I-000 |
| Implementación | H08+, tras H07 aprobado y autorización humana por release |

## 1. Estrategia de diseño documental de H05

Diseñar conectividad verificable para dos instancias Go, ALB HTTPS y un Redis compartido, minimizando egress y exposición y sin asumir que NAT Gateway/VPC endpoints son gratuitos.

## Autoridad y restricciones

- `constitution.md` (GOV-01..18, R-001..060) y el Gate H04 aprobado explícitamente el **2026-11-05** tienen precedencia; H05 diseña infraestructura, no la ejecuta.
- Arquitectura ratificada: Go (Clean Architecture), AWS, DynamoDB, **Redis real**, **2 instancias simultáneas**, balanceador HTTPS y CI/CD/DevSecOps. Ninguno es demostración opcional.
- El PDF del docente v3 exige Backend cloud con costo cero; el catálogo V6.0.2 enumera 75 rutas propuestas E001..E075 y utiliza stack histórico GCP/MongoDB **descartado**.
- No crear código Go, archivos Terraform `.tf`, pipelines ni endpoints H06 durante H05; `terraform apply/destroy`, `git push`, cambios AWS y facturación están prohibidos hasta fases y aprobaciones autorizadas.
- **Regla de evidencia:** documentos pueden alcanzar DRAFT completo sin acceso cloud; Gate I-GO requiere demostrar **viabilidad operacional y económica**, no asumirla. Campos pendientes no se cambian a PASS ni se solventan con créditos.

## 2. Plan secuencial y verificaciones

### P-010-01 — Topología mínima

**Entrada:** H04 aprobado, requisitos `A-020, A-030, A-050, I-000`, fuente AWS/cliente cuando aplique.

**Decisión a cerrar:** VPC dedicada no superpuesta; al menos dos subredes/AZ si ALB lo exige, SG de entrada ALB:443, backend solo desde SG del ALB, Redis solo desde SG del backend, egress por destino justificable. Revisar quotas por zona.

**Evidencia exigida:** tabla, diagrama, salida de consulta de solo lectura o análisis reproducible (según corresponda). Si requiere AWS no disponible, mantener OPEN/BLOCKED; no inventar resultado.

### P-010-02 — Puertos y flujos

**Entrada:** H04 aprobado, requisitos `A-020, A-030, A-050, I-000`, fuente AWS/cliente cuando aplique.

**Decisión a cerrar:** Cliente→ALB TLS443; ALB→Go en puerto interno elegido para health + API; Go→DynamoDB vía AWS SDK con IAM, Go→Redis TCP TLS; nodos→logs/actualizaciones por ruta controlada. Documentar por matriz origen-destino.

**Evidencia exigida:** tabla, diagrama, salida de consulta de solo lectura o análisis reproducible (según corresponda). Si requiere AWS no disponible, mantener OPEN/BLOCKED; no inventar resultado.

### P-010-03 — NAT e IP

**Entrada:** H04 aprobado, requisitos `A-020, A-030, A-050, I-000`, fuente AWS/cliente cuando aplique.

**Decisión a cerrar:** NAT Gateway, endpoints privados, IPv4 públicas y tráfico de salida tienen costos o requisitos específicos; comparar rutas sin NAT y con endpoints, sin forzar IP pública a las dos instancias ni ocultar costo de IPv4.

**Evidencia exigida:** tabla, diagrama, salida de consulta de solo lectura o análisis reproducible (según corresponda). Si requiere AWS no disponible, mantener OPEN/BLOCKED; no inventar resultado.

### P-010-04 — Identidades y datos

**Entrada:** H04 aprobado, requisitos `A-020, A-030, A-050, I-000`, fuente AWS/cliente cuando aplique.

**Decisión a cerrar:** TLS del cliente termina en ALB; si políticas exigen cifrado tránsito ALB→Go, instrumentar TLS interno y certificados gestionados. Redis autenticado y cifrado; no exponer puertos administrativos públicamente.

**Evidencia exigida:** tabla, diagrama, salida de consulta de solo lectura o análisis reproducible (según corresponda). Si requiere AWS no disponible, mantener OPEN/BLOCKED; no inventar resultado.

### P-010-05 — Controles de seguridad

**Entrada:** H04 aprobado, requisitos `A-020, A-030, A-050, I-000`, fuente AWS/cliente cuando aplique.

**Decisión a cerrar:** SG least privilege, no SSH público, acceso operativo temporal aprobado (SSM si autorizado) o canal compatible con SCP; registros de acceso con retención y PII mínima.

**Evidencia exigida:** tabla, diagrama, salida de consulta de solo lectura o análisis reproducible (según corresponda). Si requiere AWS no disponible, mantener OPEN/BLOCKED; no inventar resultado.

### P-010-06 — Alcance

**Entrada:** H04 aprobado, requisitos `A-020, A-030, A-050, I-000`, fuente AWS/cliente cuando aplique.

**Decisión a cerrar:** Terraform futuro creará red/SG/rutas desde módulos; aquí únicamente definición. Evitar Default VPC como dependencia implícita o mezcla con redes de otros proyectos.

**Evidencia exigida:** tabla, diagrama, salida de consulta de solo lectura o análisis reproducible (según corresponda). Si requiere AWS no disponible, mantener OPEN/BLOCKED; no inventar resultado.

## 3. Orden de ejecución futura H08 (no ejecutar ahora)

1. Auditar plan AWS/credenciales/precios y límites SCP sin cambios.
2. Preparar revisión de cambios, artefactos IaC y análisis económico en PR autorizado.
3. Ejecutar validaciones (fmt/validate/plan/pruebas), someter al gate humano.
4. Aprovisionar o modificar únicamente después de autorización de release y protección de datos.
5. Ejecutar pruebas reales, registrar métricas, costos y rollback, y no declarar DONE antes de las evidencias.

## 4. Dependencias cruzadas

I-000, I-020, I-030, I-050, I-060

## 5. Riesgos y tratamiento

1. `I-010-red-vpc-R01`: Costo NAT/endpoints e IPv4 inadvertido. Responsable: Backend/infra; evidencia esperada: ADR, costo/caso de prueba o dictamen de servicio permitido; escalamiento humano obligatorio.
2. `I-010-red-vpc-R02`: AZ insuficientes en una cuenta Free. Responsable: Backend/infra; evidencia esperada: ADR, costo/caso de prueba o dictamen de servicio permitido; escalamiento humano obligatorio.
3. `I-010-red-vpc-R03`: Rutas sin salida que dejan nodos irrecuperables. Responsable: Backend/infra; evidencia esperada: ADR, costo/caso de prueba o dictamen de servicio permitido; escalamiento humano obligatorio.

## 6. Puerta de entrada a H06

Gate H05/I-GO NO se logra con instalación de Markdown: requiere pruebas de viabilidad económica/cuenta, trazabilidad física Dynamo/Redis, controles IAM y aceptación expresa del responsable. Los contratos detallados E001..E075 se formalizan exclusivamente en H06.
