# Frontera de responsabilidad — I-150 (INF-150-06)

| Campo | Valor |
|---|---|
| Spec | `I-150-publicacion-y-compatibilidad-api` |
| Versión | 0.1.0 |
| Estado | **IMPLEMENTADO_LOCAL — NO DESPLEGADO** |

## 1. Responsable del Backend

| Ámbito | Evidencia |
|---|---|
| Contratos API y OpenAPI 3.1 | `sdd/03-endpoints/openapi` + `verificar_publicacion_i150.py contrato/paridad` |
| Servicios Go (Clean Architecture) | `internal/`, `pkg/`, `cmd/` con pruebas unitarias |
| Persistencia DynamoDB y caché Redis | módulos I-040 e I-050 |
| Seguridad e identidad | A-050 + módulo IAM (I-060) |
| Infraestructura AWS (diseño) | I-010, I-020, I-030, I-070 |
| DevSecOps y CI/CD Backend | I-080, I-090, I-100 |
| Evidencias HTTP independientes | `PLAN_EVIDENCIA_HTTP.md` |
| Compatibilidad y versionado | `comparar_contrato_api.py` en CI |

## 2. Fuera del gate del Backend

| Elemento | Responsable | Efecto en nuestros Gates |
|---|---|---|
| Aplicación Android | Equipo Mobile | **Ninguno** |
| Google Play Store | Equipo Mobile | **Ninguno** |
| Pantallas y navegación | Equipo Mobile | **Ninguno** |
| Landing page | Equipo de landing | **Ninguno** |
| Aprobación de integración Mobile | Equipo Mobile | **Ninguno** |

El equipo Mobile consume los contratos publicados **después**; su validación es posterior e
independiente. Ninguna fase del Backend se bloquea por ausencia de confirmación Mobile.

## 3. Verificación

`verificar_publicacion_i150.py alcance --datos tests/fixtures/publicacion/alcance.json` exige:

- `fuera_de_alcance` con `android`, `play-store`, `pantallas-mobile`, `pruebas-mobile` y `landing`;
- `bloquea_por_mobile = false`;
- `gate_mobile_requerido = false`.

Un `true` en cualquiera de los dos últimos se rechaza con `ALCANCE_INCORRECTO`.
