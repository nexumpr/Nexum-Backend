# Registro de evidencias H05 — 2026-11-05

| Comprobación | Resultado observado | Límite |
|---|---|---|
| V3 preflight / install / verify | PASS 7 UPDATE + 4 CREATE; verify 11 | Evidencia del terminal del usuario |
| AWS plan Free | READ_OK YES | No incluye saldo ni vencimiento en texto mostrado |
| Cuota EC2 | 16 vCPU | No demuestra capacidad asignable en las AZ al desplegar |
| t4g.micro | READ_OK | Contenido JSON no adjunto; no asumir dos AZ de manera concluyente |
| VPC origins list | READ_OK y pantalla de 0 orígenes | No equivale a permiso CreateVpcOrigin ni elegibilidad de alta |
| Redis | 0.013 / 0.016 / 0.026 | Requiere SKU inequívoco de Redis OSS, tipo y región |
| Saldo USD 120 | Afirmación anterior del responsable | No está confirmado como crédito restante actual |
| IaC | Requisito explícito del responsable | Implementación diferida a H08+, no clicks manuales |

**Salida recomendada:** H05 listo para decisión DOCUMENTAL condicionada al diseño, con **despliegue bloqueado** hasta evidencias AWS definitivas. No declarar AWS verificado ni Gate aprobado.
