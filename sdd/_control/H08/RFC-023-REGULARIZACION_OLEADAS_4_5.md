# RFC-023 — Regularización de Oleadas 4 y 5, concurrencia de corrección y release desde CI

| Campo | Valor |
|---|---|
| Documento | `sdd/_control/H08/RFC-023-REGULARIZACION_OLEADAS_4_5.md` |
| Versión | 1.0.0 |
| Fecha | 2026-11-24 |
| Estado | **APPROVED — `APRUEBO_RFC_023_GOBIERNO_RELEASE_OLEADAS` concedida por el Scrum Master el 2026-11-24** |
| Autoridad que la motiva | Orden ejecutiva del Scrum Master del 2026-11-24, tras la auditoría extraordinaria del mismo día |
| Aprobador | Scrum Master de Nexum-Backend |
| Frase de aprobación | `APRUEBO_RFC_023_GOBIERNO_RELEASE_OLEADAS` |
| Instrumentos previos | RFC-019 (concurrencia E-1/E-2), RFC-016, RFC-018, RFC-022 |

## 1. Problema que resuelve

La auditoría del 2026-11-24 verificó que:

1. `v0.20.0+4bf6758d372` se construyó en local, se publicó con PUT presignado y se aplicó con `terraform apply -auto-approve`, sin PR, sin CI y sin Gate formal de despliegue.
2. Las ramas `liz-oleada4` (`22d4aa9`) y `liz-oleada5` (`b57a549`) contienen trabajo funcional válido, pero no están integradas en `main` (`dbff5f8`).
3. Existen defectos de contrato y de implementación (E052, E062, E064–E067, E072/E073), bloqueos FCM (productor, IAM `OUTBOX#*`, egreso) y deudas transversales (E014/E020).
4. Los tres escáneres de CI (Staticcheck, Gosec, Gitleaks) anticipan fallos: el código no está en condiciones de pasar los checks protegidos.

La regularización exige **excepción de secuenciación** (varias unidades en paralelo) y **excepción de agrupación** (PR por oleada/unidad transversal en lugar de un PR por E-ID), sin rebajar ningún control de seguridad, contrato, CI o aprobación humana.

## 2. Alcance de la excepción

### E-1. Conservación de las ramas originales

`liz-oleada4` y `liz-oleada5` se conservan **inmutables** como evidencia de autoría y de estado auditado. No se reescriben, no se hace force-push y no se eliminan. Las correcciones se realizan en ramas nuevas que descienden de ellas o de `main`, según la topología del §3.

### E-2. PR agrupados por unidad gobernada

Se permite **un PR por unidad gobernada** en lugar de un PR por E-ID:

1. **PR-O4:** regularización de Oleada 4 (E052–E063), correcciones de contrato, DevSecOps, ML y FCM. Base `main`; incluye los commits originales de `liz-oleada4` y las correcciones.
2. **PR-O5:** regularización de Oleada 5 (E021, E064–E067, E072–E075), scheduler, health y pruebas de adaptador. **Stacked** sobre PR-O4; su delta real es Oleada 5.
3. **PR-TRANSVERSAL:** E014/E020 y contratos asociados. Stacked sobre PR-O5.
4. **PR-PLATAFORMA:** IAM, egreso FCM, recuperación y propuestas IaC. Base `main`; no incluye recursos reales.
5. **PR-HISTÓRICOS:** correcciones inequívocas de los 54 endpoints y gobierno (esta RFC/Gate). Base `main`.

Cada PR debe:

- declarar base y HEAD40;
- ejecutar los 13 checks protegidos sobre su HEAD exacto;
- corregir los fallos de CI **en el mismo PR**;
- no usar `--admin`, no modificar branch protection, no saltar checks;
- no fusionarse sin la frase humana `APRUEBO_MERGE_PR_<N>_SHA_<SHA40>`.

La excepción **no** autoriza a abandonar la trazabilidad por E-ID: cada PR enumera los E-ID que contiene y cada ficha conserva tasks/checklist con evidencia.

### E-3. Concurrencia de corrección

Se permite trabajar con **hasta cuatro worktrees escritores simultáneos** y **hasta ocho unidades de trabajo activas**, con estas condiciones:

1. **Propiedad exclusiva de archivos compartidos** por el orquestador: `cmd/server/main.go`, `internal/transport/http/router.go`, `sdd/03-endpoints/openapi/*.yaml|json`, `memoria.md`, `sdd/_control/H07/BASELINE_SHA256.json`, `.github/workflows/*` y configuración común.
2. Ningún archivo puede ser modificado por dos unidades a la vez. Si una unidad necesita un cambio en un archivo compartido, lo solicita al orquestador y continúa con trabajo independiente.
3. Las interfaces entre unidades se acuerdan antes de implementar; se reconcilian al integrar.
4. Cada worktree tiene su propia rama y su propio estado; no se comparte el índice de Git.
5. Para que los PR apilados ejecuten los mismos checks que un PR contra `main`, se permite añadir `fix/**` y `feat/**` a los bases de `pull_request` en `go-backend.yml`. No se alteran jobs, permisos, umbrales, herramientas ni required checks.

### E-4. Release exclusivamente desde CI de `main`

Se prohíbe construir el binario productivo en local y publicarlo con PUT presignado. La release debe:

1. partir de `main` con CI verde;
2. usar el artefacto ARM64 generado por el CI de ese `main`, con SHA-256 verificado;
3. incluir SBOM del mismo run;
4. publicarse de forma inmutable bajo Gate;
5. aplicarse con un plan Terraform guardado, SHA-256 verificado y Gate humano por SHA;
6. ejecutar un único `instance refresh` controlado;
7. certificarse funcionalmente.

`terraform apply -auto-approve` queda prohibido para releases productivas.

### E-5. Autorizaciones de esta sesión

Se registran en `GATE_H08E_REGULARIZACION_OLEADAS_4_5.md` las frases concedidas por el Scrum Master. Ninguna de ellas autoriza merge, publicación, apply, despliegue, rotación de credenciales, borrado de objetos ni creación de recursos de pago.

## 3. Topología Git autorizada

```
main (dbff5f8)
├── fix/h08e-regularizacion-54-gobierno      → PR-HISTÓRICOS
├── feat/h08i-iam-egreso-propuesta           → PR-PLATAFORMA
└── fix/h08e-oleada4-regularizacion (desde 22d4aa9) → PR-O4
    └── fix/h08e-oleada5-regularizacion (desde b57a549) → PR-O5 (stacked)
        └── fix/h08e-transversal-e014-e020 (desde PR-O5) → PR-TRANSVERSAL (stacked)
```

- `liz-oleada4` y `liz-oleada5` permanecen intactas.
- PR-O5 se abre contra la rama de PR-O4 mientras PR-O4 no esté fusionado; al fusionarse PR-O4, se retargetea a `main`.
- PR-TRANSVERSAL se abre contra la rama de PR-O5.

## 4. Condiciones de validez

La excepción deja de ser válida si:

1. se modifica una rama original de Liz;
2. se fusiona sin frase humana por PR+HEAD40;
3. se publica o despliega sin CI de `main` y Gate por SHA;
4. se relaja un check, se amplía una allowlist de secretos sin justificación o se añade `#nosec` indiscriminado;
5. se modifican contratos aprobados sin RFC específica;
6. se ejecuta una mutación AWS no autorizada.

## 5. Relación con RFC-019

RFC-019 sigue vigente para concurrencia y release agrupado. RFC-023 la extiende para la **regularización de dos oleadas ya implementadas fuera de proceso** y para la **corrección paralela**. En caso de conflicto en esta fase, prevalece RFC-023; en lo no previsto, RFC-019.

## 6. Aprobación

`APRUEBO_RFC_023_GOBIERNO_RELEASE_OLEADAS` — Scrum Master, 2026-11-24.

Esta aprobación es de **gobierno de desarrollo y release**. No sustituye las aprobaciones de merge, publicación, apply, despliegue ni las decisiones de producto, legales o FinOps.
