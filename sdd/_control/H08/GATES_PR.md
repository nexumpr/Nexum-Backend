# I-080 — Gates de Pull Request

| Campo | Valor |
|---|---|
| Spec | `sdd/02-infraestructura/I-080-ci-backend` (INF-080-02, INF-080-03, INF-080-05) |
| Estado de branch protection | **ACTIVA** — verificada por API el 2026-11-27 (`strict=true`, 13 checks, 1 revisor) |
| Repositorio | Público (`nexumpr/Nexum-Backend`, `visibility=public`, RFC-009) |
| Workflow | `.github/workflows/go-backend.yml` |

## 0. Verificación real de capacidades (solo lectura)

`gh api repos/nexumpr/Nexum-Backend/branches/main/protection` devuelve **HTTP 200** con:

- `required_status_checks.strict = true` y **13 contextos** (los doce jobs del grafo RFC-008 más `validar`);
- `required_pull_request_reviews.required_approving_review_count = 1`;
- `allow_force_pushes = false` y `allow_deletions = false`;
- `enforce_admins = false`, `required_signatures = false`, `required_linear_history = false` y
  `required_conversation_resolution = false`;
- **sin rulesets** (`/rulesets` devuelve `[]`) y **sin environments** (`aws-deploy` no existe).

Historia: el 2026-11-08 la API devolvía HTTP 403 en repositorio privado. La publicación del repositorio
(RFC-009) eliminó esa limitación de plan. La protección se configura y se verifica; no se declara nada
que la API no devuelva.

**Propuesta pendiente de aprobación (no aplicada):** crear un ruleset que impida el bypass de
administradores (`enforce_admins` equivalente) y exigir resolución de conversaciones. No se modifica
ningún setting de GitHub sin autorización específica.

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

## 2. Configuración aplicada y propuesta pendiente

La configuración vigente es la verificada en §0 (13 checks, `strict=true`, 1 revisor,
force-push y borrado deshabilitados). El JSON siguiente se conserva como referencia
histórica de la solicitud original de I-080; **no** describe el estado actual.

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

Estado actual: `AWS-0011` (CloudFront sin WAF) está **autorizado temporalmente** por `RFC-006`
(`APRUEBO_RFC_006_WAF_TEMPORAL`, 2026-11-08), con caducidad 2026-12-02 y condiciones de despliegue.
Cualquier otro HIGH/CRITICAL sin RFC firmada sigue bloqueando.

## 5. Seguridad del workflow

- `permissions: contents: read`; sin `pull_request_target`; sin secretos en pasos de PR.
- PR de forks no reciben secretos del repositorio.
- OIDC/credenciales cloud quedan fuera de I-080: pertenecen a I-090 y a un Gate posterior.
- No se ejecuta Docker ni `terraform apply/destroy` en CI.

## Grafo modernizado (RFC-008, 2026-11-09)

Los checks exigidos para el merge son los doce del grafo vigente:

| Check | Job |
|---|---|
| Deteccion de cambios | `detectar` |
| Formato, vet, build y pruebas | `calidad` |
| Analisis estatico y vulnerabilidades | `seguridad` |
| Escaneo de secretos | `secretos` |
| Validacion Terraform e IaC | `iac` |
| Contratos OpenAPI y herramientas | `contratos` |
| SBOM de dependencias | `cadena-suministro` |
| SAST Semgrep | `sast-semgrep` |
| Integracion HTTP y concurrencia | `integracion` |
| PR Quality Gate | `quality-gate` |
| Hardening Report | `hardening-report` |
| CI Success | `ci-success` |
| validar (SDD documental) | `sdd-validacion.yml` |

`quality-gate` acepta `skipped` unicamente si el perfil de cambios lo justifica y lo
registra; cualquier `failure`, `cancelled`, resultado ausente o artefacto exigido no
publicado bloquea. `ci-success` es el unico nodo que declara el resultado final.

**La proteccion de ramas esta ACTIVA** desde la publicacion del repositorio: `strict=true`,
13 checks y una revision. `enforce_admins=false` permite que un administrador omita la
proteccion; mientras no se apruebe un ruleset equivalente, el merge sigue exigiendo
autorizacion humana vinculada a PR + SHA40 y la prohibicion expresa de `--admin`/bypass.

La excepcion temporal de WAF (RFC-006, AWS-0011) mantiene su vigencia y su verificacion
automatica dentro del job `iac`; ninguna otra excepcion esta autorizada.
