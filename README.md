# Nexum-Backend — SDD rector (solo Backend)

**Fuente contractual para implementar, no prueba de código ejecutado.** Alcance: Backend Go, API HTTP, modelo DynamoDB, Redis OSS, infraestructura AWS, CI/CD, pruebas, DevSecOps y FinOps. No incluye aprobación ni implementación de Mobile, Play Store o frontend.

## Estado vigente (5 de noviembre de 2026)

| Gate | Estado actual comprobable | Consecuencia |
|---|---|---|
| H03 | APPROVED por el responsable | Constitución, agentes y skills preservados |
| H04 | APPROVED por el responsable | 12 especificaciones de arquitectura ratificadas |
| H05 | APPROVED_DOCUMENTAL_CONDICIONADO | 16 specs definidas; despliegue AWS **NO autorizado** |
| H06 | INSTALLED/VERIFY_PASS; **GATE_HUMANO_PENDIENTE** | 75 operaciones documentadas; todavía no certificadas en ejecución |
| H07 | PUBLICACIÓN_INICIAL_REALIZADA; **GATE_HUMANO_PENDIENTE** | Commit inicial `84be72d`; fallo CI por CRLF en proceso de corrección |
| H08+ | NOT_STARTED; NO AUTORIZADO automáticamente | Requiere gates y autorización explícita para implementar / elevar permisos |

Una autorización para publicar/corregir el repositorio **no** equivale a aprobar H06, H07 o acciones sobre AWS. Estado único rector y evidencias: [`sdd/_control/H07/ESTADO_Y_GATES.md`](sdd/_control/H07/ESTADO_Y_GATES.md). La historia de errores y su reparación es pública en el historial Git y en [`CAUSA_RAIZ_CI_Y_MIGRACION.md`](sdd/_control/H07/CAUSA_RAIZ_CI_Y_MIGRACION.md).

## Estructura normativa (solo tres bloques de especificaciones)

```text
Nexum-Backend/
├── constitution.md           # autoridad normativa; no modificar sin RFC
├── AGENTS.md                 # instrucciones vigentes de agentes
├── memoria.md               # hechos actuales y antecedentes comprobables
├── opencode.json             # permisos H03 todavía restrictivos
├── .opencode/agents/          # 8 agentes
├── .opencode/skills/          # 9 skills
├── .github/workflows/        # CI documental, no deploy
├── herramientas/             # validación SDD y sus pruebas
└── sdd/
    ├── 01-arquitectura-general/  # A-000..A-110, 12 especificaciones
    ├── 02-infraestructura/       # I-000..I-150, 16 especificaciones
    ├── 03-endpoints/            # E001..E075, 75 operaciones
    └── _control/H07/           # evidencias y gates (NO es un cuarto bloque de specs)
```

Cada especificación tiene `spec.md`, `plan.md`, `tasks.md`, `checklist.md`: 103 × 4 = **412 documentos**. Catálogo inicial de **75 operaciones** (33 maqueta, 23 soporte, 12 esperadas/simuladas, 7 opcionales), extensible por RFC; OpenAPI 3.1 en `sdd/03-endpoints/openapi/`. La clasificación y obligatoriedad individual están en H06; las siete opcionales no son compromisos obligatorios por mera existencia.

## Validación reproducible (Python 3.13, sin dependencias de terceros)

```powershell
py -3.13 herramientas/validar_sdd_integral.py
py -3.13 sdd/03-endpoints/herramientas/validar_h06.py
py -3.13 -m unittest discover -s herramientas/tests -v
```

El manifiesto SHA-256 está en `sdd/_control/H07/BASELINE_SHA256.json`; `.gitattributes` fija finales de línea LF para Windows y Linux. No desactivar la validación para hacer pasar CI. Archivos `sdd/_control/H07/*` son **gobierno y evidencia**, no instrucciones que puedan sustituir `constitution.md` o conceder aprobación.

## Implementación planificada, NO ejecutada

Orden de trabajo: H08 arquitectura Go, luego IaC AWS integral y posteriormente operaciones E001–E075 en releases controlados. Obligatorio: dos instancias Go simultáneas, CloudFront/ALB HTTPS conforme al diseño, DynamoDB, Redis real, Terraform, AWS Free con **cero dinero personal**, automatización de crear/estado/apagar solo auxiliares/reactivar/reconciliar/recuperar, verificación HTTP y pruebas de seguridad. No hay permisos concedidos para `terraform apply`, activar Paid, crear recursos AWS ni desplegar. Las incertidumbres reales de costos/Red/TLS/Redis/SCP siguen expresamente **BLOCKED** hasta evidencia.

El equipo Mobile puede proponer cambios o endpoints adicionales mediante RFC; ningún gate del Backend depende de aprobación del equipo Mobile. Ver [`POLITICA_CAMBIOS_API.md`](sdd/_control/H07/POLITICA_CAMBIOS_API.md) y [`PLAN_ENTREGA_EQUIPO.md`](sdd/_control/H07/PLAN_ENTREGA_EQUIPO.md).
