# Gate H08-E — Ventana de merges Oleada 2 (`strict` temporal)

| Campo | Valor |
|---|---|
| Documento | `sdd/_control/H08/GATE_H08E_OLEADA_2_MERGE_WINDOW.md` |
| Versión | 1.0.0 |
| Fecha | 2026-11-19 |
| Estado | **AUTORIZADO — ventana NO abierta todavía** |
| Autorización humana | `APRUEBO_RELAJAR_STRICT_TEMPORAL_OLEADA_2` (2026-11-19) |
| Alcance | Solo el campo `strict` de `required_status_checks` en `main` |
| Vigencia | Desde la apertura hasta el cierre de la ventana; máximo 24 h |

## 1. Estado original registrado (antes de cualquier cambio)

Comprobado por API el 2026-11-19:

| Dato | Valor |
|---|---|
| Rama | `main` @ `d28da66ed39b7f103fe3b3e1ae6281e918ef43d6` |
| `required_status_checks.strict` | **true** |
| Checks requeridos | **13** (`Deteccion de cambios`, `Formato, vet, build y pruebas`, `Analisis estatico y vulnerabilidades`, `Escaneo de secretos`, `Validacion Terraform e IaC`, `Contratos OpenAPI y herramientas`, `SBOM de dependencias`, `SAST Semgrep`, `Integracion HTTP y concurrencia`, `PR Quality Gate`, `Hardening Report`, `CI Success`, `validar`) |
| `allow_force_pushes.enabled` | **false** |
| `allow_deletions.enabled` | **false** |
| Rulesets | `[]` (sin merge queue configurada) |

Este bloque es la referencia de restauración. La restauración debe dejar exactamente estos valores.

## 2. Qué autoriza y qué no

**Autoriza** cambiar únicamente `required_status_checks.strict` de `true` a `false` durante la ventana de merge, para evitar que cada merge invalide los HEAD pendientes.

**No autoriza**: desactivar checks, reducir la lista de 13, permitir force push, permitir borrado de `main`, usar `--admin`, saltarse `--match-head-commit`, fusionar PR sin su frase `APRUEBO_MERGE_PR_<N>_SHA_<SHA40>`, ni cambiar ningún otro campo de protección.

## 3. Condiciones previas ya cumplidas

| Condición | Evidencia |
|---|---|
| CI Linux acumulativo PASS | Run **44466353226**, `chore/h08e-oleada2-integracion` @ `c4a64fd8b345a6f479c24430ff2d42e8e7789aa4`, 12/12 |
| 15 rutas montadas | `TestOleada2RutasMontadas`, 15/15 PASS |
| 15 PR verdes | 12/12 checks en el HEAD de cada PR |
| Sin cambios ajenos | cada PR solo toca su endpoint y, cuando aplica, los archivos compartidos del andamiaje |
| Protección original registrada | §1 |

## 4. Procedimiento exacto de la ventana

1. Verificar que el HEAD de cada PR sigue siendo el autorizado (si cambió, renovar solo esa frase).
2. Ejecutar `PATCH /repos/nexumpr/Nexum-Backend/branches/main/protection/required_status_checks` conservando `contexts` y cambiando solo `strict: false`.
3. Fusionar en orden: #102→#106, #107→#111, #112→#115, #116, y el PR documental #118, **cada uno** con `gh pr merge <N> --merge --match-head-commit <SHA40>`.
4. Tras cada merge, registrar `mergeCommit.oid` y el resultado de CI de `main`.
5. Al terminar (o ante cualquier fallo), restaurar `strict: true` con la misma lista de 13 checks y verificar `force=false`, `deletions=false`.
6. Validación posterior: CI de `main` verde y presencia de las 15 operaciones en el árbol final.

## 5. Restauración garantizada

La restauración se ejecuta en un bloque `finally` del procedimiento (se intenta aunque un merge falle). Después se comprueba por API:

```text
strict=true · contexts=13 · allow_force_pushes=false · allow_deletions=false
```

Si la restauración fallara, se detiene toda operación y se reporta como incidente; no se deja `strict=false` sin declararlo.

## 6. Estado actual

La ventana **no se ha abierto**: la autorización de `strict` no sustituye las quince frases `APRUEBO_MERGE_PR_<N>_SHA_<SHA40>`, que siguen pendientes. La protección permanece intacta en `strict=true`.

| Paso | Estado |
|---|---|
| Autorización de la excepción | CONCEDIDA |
| Protección modificada | **NO** (se hará al recibir las frases de merge) |
| Merges ejecutados | 0 |
| Restauración | Pendiente de apertura |
