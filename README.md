# Nexum-Backend — estado vigente H09-1

Backend Go de Nexum (finanzas personales). Repositorio público `nexumpr/Nexum-Backend`; SDD completo con 103 especificaciones (12 arquitectura, 16 infraestructura, 75 endpoints) y contrato OpenAPI 3.1 de 75 operaciones.

## Estado operativo (2026-11-27)

| Elemento | Valor |
|---|---|
| `main` | `1d5778dec3f5276bcc8f7e6a5979044a83418682` |
| Release productiva | `v0.21.4+5bef784b2d1c` |
| Cómputo | ASG `2/2/2`, Launch Template v26, `us-east-2a` + `us-east-2b` |
| Entrada | CloudFront HTTPS → VPC Origin → ALB interno HTTP:80 → 2 EC2 Go:8080 |
| Datos | DynamoDB `NEXUM_CORE_H08I` (SSE KMS) + Redis OSS `cache.t4g.micro` con TLS y RBAC |
| Certificación funcional | **74/75 PASS**; E015 pendiente de aprobación legal |
| Plan AWS | FREE/ACTIVE; sin cargos personales; créditos y vigencia se verifican en cada Gate |
| Gobierno | H08E cerrado; H09-1 en curso; H09-2..H09-4 no autorizadas |

La certificación funcional es independiente de la madurez DevSecOps. Los estados `LOCAL`, `MERGED` y `PRODUCTIVO` se registran por separado y nunca se extrapolan entre sí.

## Bloques SDD

- `sdd/01-arquitectura-general/` A-000..A-110
- `sdd/02-infraestructura/` I-000..I-150
- `sdd/03-endpoints/` E001..E075
- `sdd/_control/H07` y `sdd/_control/H08`: auditoría, Gates, RFC y matrices; **no** son una cuarta carpeta de especificaciones.

## Validación local

```powershell
py -3 herramientas/validar_sdd_integral.py --check-git
py -3 sdd/03-endpoints/herramientas/validar_h06.py
py -3 -m unittest discover -s herramientas/tests
py -3 -m unittest discover -s tests/infra/terraform
go build ./...; go vet ./...; go test ./... -count=1
```

El CI ejecuta doce jobs más `validar`: formato/vet/build/pruebas, análisis estático, secretos, IaC, contratos, SBOM, Semgrep, integración, quality gate, hardening report y éxito final. `main` exige `strict=true`, 13 checks y una revisión; force-push y borrado están deshabilitados.

## Límites y bloqueos vigentes

- `E015` sirve `2026-11-v2`; la propuesta `2026-11-v3` **no está aprobada ni publicada** y su artefacto de 13 662 bytes no está preservado en Git.
- CloudFront con certificado por defecto impone TLSv1; no se declara TLS 1.2.
- WAF ausente; la excepción `RFC-006` vence el **2026-12-02**.
- Observabilidad: log group definido con retención 7 días, pero **sin ingesta** (`storedBytes=0`) y alarmas sin acciones (`RFC-007` inexistente).
- Redis y NAT operan en una sola AZ: hay punto único de fallo declarado.
- Sin environment `aws-deploy`, sin OIDC y sin CD ejecutado; el despliegue es asistido y requiere Gate por SHA de plan.
- Sin tags ni GitHub Releases; el identificador de release es la convención `vX.Y.Z+<sha12>` del prefijo S3.

## Reglas de trabajo

`constitution.md` > `AGENTS.md` > `memoria.md`. Ningún `apply`, despliegue, merge, rotación de secretos o gasto se ejecuta sin autorización humana específica. No se usa Docker ni WSL. Mobile, Play Store y landing quedan fuera del alcance del Backend.

**No confundir:** CI documental verde ≠ Go compilado ≠ HTTP verificado ≠ AWS desplegado. Toda afirmación de producción exige evidencia HTTP/AWS de la release exacta.
