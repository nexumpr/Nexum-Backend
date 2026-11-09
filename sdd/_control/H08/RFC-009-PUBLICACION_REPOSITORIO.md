# RFC-009 — Publicación del repositorio `nexumpr/Nexum-Backend`

| Campo | Valor |
|---|---|
| Documento | `sdd/_control/H08/RFC-009-PUBLICACION_REPOSITORIO.md` |
| Fecha | 2026-11-09 |
| Base | `main` `1aff5cef2a6964b117eb53dd35aab2cdc619abde` |
| Estado | **APROBADA (2026-11-09)** por `APRUEBO_REPO_PUBLICO_CON_RFC_009` |
| Autoriza | cambiar la visibilidad de **privado a público** y habilitar las protecciones que eso desbloquea |
| No autoriza | AWS, Terraform remoto, despliegue, cambios de plan ni de facturación |

## 1. Problema

El CI completo de `main` no se ejecuta: en el run `42670707068` (intentos 1 y 2) GitHub deja de
crear jobs a mitad de ejecución, con subconjuntos distintos en cada intento y reintento denegado.
La facturación no es consultable con el token actual: causa **`NO_VERIFIED`**.

Además, la protección de ramas está `BLOQUEADO_PLAN_GITHUB` (HTTP 403) porque el repositorio es
privado con plan Free.

## 2. Decisión

Publicar el repositorio. En repositorios **públicos**, GitHub no consume minutos del cupo privado en
runners estándar y la protección de ramas con checks obligatorios es gratuita.

## 3. Evidencia previa (barrido de secretos)

Gitleaks `v8.30.1` sobre los 114 commits, **sin baseline**: 8 coincidencias, **todas falsas** —
hashes SHA-256, el propio `.gitleaks-baseline.json`, `secretos_uri = var.secretos_s3_uri` y dos
cadenas sintéticas de pruebas de telemetría (una de ellas un JWT de 108 caracteres usado para
probar el redactor).

**Nunca existió en el historial** ningún `.env`, `.pem`, `.key`, `.tfstate`, `.tfvars`, `id_rsa` ni
`credentials`. No hay credenciales que rotar.

## 4. Qué pasa a ser público

Código Go y pruebas · 103 especificaciones SDD, RFC y Gates · Terraform completo de AWS (incluido
IAM) · workflows y políticas de CI · `memoria.md` con su historial de incidentes · IDs de cuenta
`713797602090`, nombres de bucket y ARNs · **emails de autor de los 157 commits** · **12 ramas
remotas**, incluida `Arquitectura_Lizeth` · logs y artefactos de Actions (SBOM, cobertura,
dependencias, Hardening Report).

## 5. Riesgos aceptados y controles

| Riesgo | Control |
|---|---|
| Reconocimiento de la arquitectura AWS | Aceptado: no hay secretos ni credenciales; los identificadores no autentican |
| PR desde forks ejecutando código en runners | `GITHUB_TOKEN` de fork en solo lectura; el repo **no usa `secrets.`** en ningún workflow; se exige aprobación para colaboradores externos |
| `cd-backend.yml` disparado por terceros | Es `workflow_dispatch` sobre `main`; un fork no puede lanzarlo |
| Exposición de identidades de autor | Irreversible una vez publicado; aceptado por el responsable al aprobar |
| Integridad académica | Aceptado por el responsable; el material ya es del proyecto |

## 6. Consecuencias irreversibles

El historial y las ramas publicadas no se pueden «despublicar» por completo: clones y cachés de
terceros pueden conservarlos. La aprobación asume esta irreversibilidad.

## 7. Post-condiciones obligatorias

1. Proteger `main` con los checks obligatorios del grafo RFC-008 más `validar`.
2. Exigir aprobación para workflows de colaboradores externos.
3. Reejecutar `Go Backend CI` en `main` y verificar **12/12** más SDD.
4. Registrar el resultado en `ESTADO_Y_GATES.md` y `memoria.md`.

## 8. Aprobación

`APRUEBO_REPO_PUBLICO_CON_RFC_009` (2026-11-09). Es una autorización de **visibilidad**: no
autoriza AWS, despliegue, cambio de plan ni gasto.
