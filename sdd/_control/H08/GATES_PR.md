# I-080 — Gates de Pull Request

| Campo | Valor |
|---|---|
| Spec | `sdd/02-infraestructura/I-080-ci-backend` (INF-080-02, INF-080-03, INF-080-05) |
| Estado de branch protection | **BLOQUEADO_PLAN_GITHUB** — verificado por lectura el 2026-11-08: HTTP 403 «Upgrade to GitHub Pro or make this repository public» |
| Repositorio | Privado (`nexumpr/Nexum-Backend`, `private`) |
| Workflow | `.github/workflows/go-backend.yml` |

## 0. Verificación real de capacidades (solo lectura)

`gh api repos/nexumpr/Nexum-Backend/branches/main/protection` devuelve **HTTP 403**:
«Upgrade to GitHub Pro or make this repository public to enable this feature».

Consecuencia: en el plan actual **no es posible configurar branch protection ni required status checks**.
No se declara protección activa, no se cambia de plan y no se genera ningún cargo. Los checks de CI sí
funcionan y se exigen por revisión humana y por el proceso de merge autorizado. El procedimiento de
activación queda preparado para cuando exista un plan que lo permita o una decisión humana de
visibilidad/plan.

## 1. Checks requeridos

Estos son los nombres exactos que deben quedar como *required status checks* en `main` y `release/**`:

1. `Formato, vet, build y pruebas`
2. `Analisis estatico y vulnerabilidades`
3. `Escaneo de secretos`
4. `Validacion Terraform e IaC`
5. `Contratos OpenAPI y herramientas`
6. `SBOM de dependencias`

El check `validar` del workflow documental (`SDD documental (sin AWS ni despliegue)`) se mantiene como
requisito adicional cuando la rama lo exija.

## 2. Configuración deseada (no aplicada)

```json
{
  "required_status_checks": {
    "strict": true,
    "contexts": [
      "Formato, vet, build y pruebas",
      "Analisis estatico y vulnerabilidades",
      "Escaneo de secretos",
      "Validacion Terraform e IaC",
      "Contratos OpenAPI y herramientas",
      "SBOM de dependencias"
    ]
  },
  "enforce_admins": false,
  "required_pull_request_reviews": {
    "required_approving_review_count": 1,
    "dismiss_stale_reviews": true,
    "require_code_owner_reviews": false
  },
  "restrictions": null,
  "allow_force_pushes": false,
  "allow_deletions": false,
  "required_conversation_resolution": true
}
```

Aplicación futura, **sólo con autorización humana específica** (no ejecutada en I-080):

```bash
gh api -X PUT repos/nexumpr/Nexum-Backend/branches/main/protection \
  --input proteccion-main.json
```

Mientras no se aplique, la protección efectiva depende de la revisión humana y de no fusionar con
checks rojos. La ausencia de protección configurada **no** se declarará como cumplida.

## 3. Reglas de PR

- Un PR por especificación; sin `push` directo a `main`; sin `force push`.
- Revisión humana obligatoria y rastro del cambio (autor, diff, commit SHA).
- El merge exige autorización humana vinculada a PR + SHA completo del HEAD.
- Los checks se evalúan sobre el HEAD autorizado; si cambia, la autorización queda invalidada.
- El autor no puede forzar verde: no `--admin`, no saltarse checks.

## 4. Excepciones

Un hallazgo alto/crítico o una prueba rota bloquea el merge. La única excepción admitida es una **RFC
firmada** por el responsable que indique: hallazgo, causa, riesgo residual, vigencia y plan de
corrección. La excepción se registra en el PR y no convierte el resultado en `PASS`.

Estado actual: `AWS-0011` (CloudFront sin WAF) **no tiene excepción válida**. La propuesta está en
`RFC-006-EXCEPCION_WAF_I030.md` (**PROPUESTO — NO APROBADO**); hasta su aprobación el job `iac` falla.

## 5. Seguridad del workflow

- `permissions: contents: read`; sin `pull_request_target`; sin secretos en pasos de PR.
- PR de forks no reciben secretos del repositorio.
- OIDC/credenciales cloud quedan fuera de I-080: pertenecen a I-090 y a un Gate posterior.
- No se ejecuta Docker ni `terraform apply/destroy` en CI.
