# H05 — Matriz de cobertura del proyecto y bloqueos de aprobación

## Pruebas verificables en futuro H08 (cada una obligatoria)

| Req | Fases y owners Backend | Prueba específica | Evidencia esperada | H05 estado |
|---|---|---|---|---|
| Dos instancias reales | I-020/I-030 | 2 instanceIDs healthy simultáneos | AWS describe targets + requests | DESIGN / NO EJECUTADO |
| Balanceo/HTTPS | I-030 | TLS CA, tráfico ambos, failover | curl -v filtrado, 2 targets | DESIGN |
| NoSQL DynamoDB | I-040 | CRUD real, consultas Index y transacción | API tests, describe tables/GSIs | DESIGN |
| Redis distribuido | I-050 | Nodo A escribe, Nodo B lee/invalida | test compartido, TLS | DESIGN |
| Costo cero | I-000/I-110/I-120 | Cotización Ohio Free/SCP + facturación real posterior | matriz firmada | **BLOCKED_OPEN_COST** |
| Terraform IaC + destroy | I-070/I-120/I-130 | plan revisado, apply y destroy-runtime separado | logs controlados, state preservado | DESIGN |
| CI/CD + DevSecOps | I-080/I-090/I-100 | PR tests/scan/protected deploy | checks verdes, review y release | DESIGN |
| Logs y observabilidad | I-110 | traces, alarms, fail scenario | CloudWatch/OTEL | DESIGN |
| Backups/DR | I-130 | restore en tabla aislada | RTO/RPO medidos | DESIGN |
| 7 endpoints evaluación parcial | I-150/H06 | registro, login, logout, +4 reales | HTTP 2xx/4xx y Dynamo persisted | DESIGN |
| Todas las rutas propuestas | I-040/I-150/H06 | E001..E075 diferenciadas M/S/E/O | OpenAPI+test por contrato | DESIGN |
| Push/ML/IA contenidos | I-150/H06 | contratos Backend aplicables al prototipo y profesor | evidencia APIs y nube | SPEC DEPENDENCY |
| Alcance no-Mobile | I-150 | No depender de visto bueno móvil | CI HTTP independiente | DESIGN |

**Estado I-GO:** aún no aprobado. La existencia de la matriz no cubre precios reales, servicios bloqueados por SCP o rutas de despliegue; deben ser demostrables mediante evidencia y aceptación. No se autoriza avanzar a H06 antes de I-GO expreso.
