# Nexum-Backend — SDD rector y arranque H08-A

Proyecto privado `nexumpr/Nexum-Backend`. SDD completo (12 arquitectura, 16 infraestructura, 75 endpoints) publicado; 103 specs, 412 documentos base, OpenAPI 3.1. Las 75 operaciones constituyen baseline extensible por RFC; las siete opcionales no son obligaciones automáticas.

## Estado único
Leer `sdd/_control/H07/ESTADO_Y_GATES.md` y `sdd/_control/H08/CONTRATO_INICIO_H08A.md`. H03/H04 aprobados; H05 documental condicionado; H06/H07 ratificados mediante comando humano H08-A; H08-A **local solamente** (A-000 primero). Si no se ha ejecutado aprobación del instalador y CI no está verde, este repositorio NO autoriza implementación.

## Tres bloques SDD
- `sdd/01-arquitectura-general/` A-000..A-110
- `sdd/02-infraestructura/` I-000..I-150
- `sdd/03-endpoints/` E001..E075
- `sdd/_control/H07`, `sdd/_control/H08`: auditoría, Gates y RFC, **no son cuarta carpeta de especificaciones**.

## Documentación y validación
`constitution.md` > `AGENTS.md` > `memoria.md`; 8 agentes/9 skills en `.opencode`, config v2 en `opencode.json`, manifiesto SHA-256 H07, workflow documental GitHub Actions. Validación:
```powershell
py -3.13 herramientas/validar_sdd_integral.py
py -3.13 -m unittest discover -s herramientas/tests -v
```

## Primera implementación (OpenCode)
Usar nueva sesión OpenCode v2, `nexum-orchestrator`, ver `sdd/_control/H08/PRIMERA_TAREA_OPENCODE_A000.md`. Pedir plan A-000 sin acciones y solicitar aprobación humana antes de crear archivos. **No usar agentes en main**, preparar ramas y PR. Permisos shell/edit son ASK, AWS/GitHub mutante DENY.

## Futuro obligatorio sin autorización anticipada
Terraform IaC automatizará desde creación hasta suspensión de auxiliares, 2 nodos Go simultáneos, DynamoDB, Redis OSS compartido real, balanceo CloudFront/ALB, IAM/OIDC, HTTPS, DevSecOps, CI/CD, observabilidad, respaldo y recuperación. Antes de cualquier `apply` se exigen precios, saldo/créditos y 0 MXN personales; no actualizar cuenta Free a Paid. Mobile no condiciona Gates Backend; podrá pedir E076+ mediante RFC.

**No confundir:** CI documental verde ≠ Go compilado ≠ HTTP verificado ≠ AWS desplegada. Comparación independiente con PDF docente y maqueta Android aún no está acreditada y se registra como riesgo hasta disponer de fuentes.
