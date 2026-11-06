# Programa de ejecución por equipo Backend (H08+ después de gates)

## Orden de integración recomendado (sin dependencia de aprobaciones del equipo móvil)

**Bloque A — arquitectura general antes que Cloud:** responsable backend + revisor de seguridad/QA. Implementar skeleton real Go en Clean Architecture: módulos, rutas HTTP/contratos, middlewares, inyección de puertos, error envelope, JWT/refresh seguro, adaptadores mock **solo para test**, pruebas unitarias/contrato/estática. A-000..A-110 en tickets independientes, PRs pequeños y CI; sin exigir nube para compilar.

**Bloque I — IaC completa y automática:** responsable infraestructura + revisor FinOps/seguridad. Terraform state independiente y cifrado, IAM/OIDC, redes sin NAT (validar egress de build/SSM/observabilidad), CloudFront VPC origin+ALB privado, 2 EC2 en AZ diferentes, Redis OSS real compartido, DynamoDB y respaldos, CI/CD de Backend, seguridad, plan de costos y kill-switch auxiliar. Pipeline plan/checks → aprobación humana por ambiente → apply → health/verificación y rollback; no consola manual en operación. **Antes del primer apply**, resolver todos los R1 y aprobación de gasto dentro de crédito sin pasar a Paid.

**Bloque E — endpoints por entregas controladas:** empezar identidad/usuario (E001–E023), datos financieros (E024–E051), analítica/ML (E052–E056), notificaciones/recordatorios (E057–E067), opcionales (E063/E068+ cuando se autoricen). Una nueva operación por release autorizado, PR independiente con pruebas de regresión y evidencia HTTP real de cada release, no un mega-merge. El orden puede adaptarse por dependencias expresas sin cambiar contrato silenciosamente.

**Bloque R — hardening/aceptación:** pruebas de carrera multi-nodo, precisión centavos, DAST autorizado, backup/restore, fallo de nodo, controles finops 45 días, rehidratación automática de auxiliares y desmantelamiento seguro; informe de resultados con PASS/FAIL/BLOCKED/NOT_RUN. Certificar únicamente lo observado.

## Roles, trabajo colaborativo y PR

Asignar nombres reales posteriormente; roles no implican privilegios: Arquitecto Backend (A), Infra/Terraform (I), API/Domain (E), QA/Seguridad (cruza todos), FinOps/DevOps (I). El equipo puede preparar PRs en paralelo, pero los merges a `main` respetan dependencias y gates. Contrato OpenAPI es fuente compartida; cualquier cambio de ruta/schema requiere RFC y regresión contra móvil sin pedirle aprobación.

**Definición de done (por PR):** requisito traceable, código implementado, pruebas reales, evidencia local reproducible y CI PASS, revisión de seguridad y costo si aplica, aprobación humana; sin mocks en producción, no reemplazar Redis/DynamoDB y no prometer HA sin dos nodos reales.
