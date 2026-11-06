# Estado y Gates — Nexum-Backend (2026-11-05)

**Único registro de estados vigentes.** La instalación documental, la autorización de publicar, la aprobación técnica y el permiso de deploy son hechos distintos.

| Lote | Estado | Evidencia verificable | Habilitado | Bloqueado |
|---|---|---|---|---|
| H03 | APPROVED | Ratificación previa, 8 agentes / 9 skills, evaluación de 54 permisos comunicada | Reutilizar gobierno aprobado | Ampliación de permisos sin gate |
| H04 | APPROVED | 12 specs / 48 docs, revisión y ratificación previa | Implementación futura conforme a spec | Reescribir specs sin RFC |
| H05 | APPROVED_DOCUMENTAL_CONDICIONADO | 16 specs / 64 docs, V2/V3/V4 PASS, aprobación condicionada | Modelo IaC rector, H06 documental | AWS real, facturación, Paid y despliegue |
| H06 | INSTALL_PASS + VERIFY_PASS, **AWAITING_HUMAN_GATE** | 75 ops, 300 docs, JSON/YAML OpenAPI | Revisión y propuesta de Gate | Certificar APIs implementadas o aprobar gate por inferencia |
| H07 | PUBLICACIÓN INICIAL `84be72d`, CI FAIL; **REMEDIACIÓN AUTORIZADA**, GATE PENDIENTE | Repo `nexumpr/Nexum-Backend` privado creado; usuario aprobó corrección organizativa y publicación del PR | Corregir estructura, validar en CI, publicar con PR | Alterar H04 o conceder H08/AWS |
| H08+ | NOT_STARTED | Ninguna implementación real | Planificación y preparación de PRs | Código/deploy sin autorizaciones de fase y permisos mínimos |

### Secuencia verificable de publicación

1. Primer commit `84be72d` en `main`; GitHub Actions job `validar` terminó exit code 2 en el run `42133317544`.
2. Se reprodujo fallo SHA `FINOPS_45_DIAS_DESGLOSE_V2.csv` por CRLF→LF; la causa raíz y reparación están en `CAUSA_RAIZ_CI_Y_MIGRACION.md`.
3. Se acordó trasladar el directorio no aprobado `gobierno/H07` a `sdd/_control/H07` para que solo existan `01`, `02`, `03` como **bloques de especificaciones**; el nuevo subdirectorio `_control` contiene documentos auxiliares, no specs.
4. Un PR correctivo deberá pasar validación integral en checkout limpio, pruebas H06, regresión de hashes y merge ordinario antes de declarar CI `PASS` de `main`.
5. **H06/H07 permanecen sin ratificación formal de Gate** hasta decisión expresa del responsable. Una autorización de publicación/corrección no es un Gate de implementación.

### Pre-deploy aún BLOCKED

Verificar con IaC y pruebas reales: AWS Free saldo/vencimiento/créditos y costos incluyendo IVA, Redis SKU oficial, permisos VPC Origins/IAM/SCP, TLS de origen, egress privado sin NAT, 2 nodos EC2 + Redis + DynamoDB reales, seguridad, observabilidad, backup y recuperación. **Cero dinero personal, nunca actualizar a Paid**. La API y la base de datos requieren disponibilidad 24/7 al desplegarse; solo auxiliares son suspendibles.

Sin cotejo independiente aún del PDF académico original y prototipo Android vigente en H07; documentar diferencias por RFC. No vender este control documental como certificado operativo.
