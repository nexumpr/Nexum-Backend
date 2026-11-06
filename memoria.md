# Nexum-Backend — memoria vigente H08-A

**Fuente:** reportes Warp/capturas del responsable y ZIP final H07 reproducido localmente. La activación H08-A requiere ratificación explícita mediante CLI y CI verde; no se asume por presencia de archivos.

- H03 APPROVED: constitución, 8 agentes, 9 skills; configuración originalmente denegatoria conservada en historia Git; RFC-001 abre permisos limitados Go H08-A.
- H04 APPROVED: A-000..A-110 (48 docs), sin modificar.
- H05 APPROVED_DOCUMENTAL_CONDICIONADO: 16 I y 64 docs, AWS no desplegado. Free, 120 USD créditos referidos, 45 días, 0 MXN personales. Redis SKU, IVA, CloudFront VPC Origin create, TLS y egreso pendientes.
- H06 APPROVED_DOCUMENTAL_BASELINE **tras ratificación**: 75 operaciones 33+23+12+7, 300 docs, OpenAPI 3.1. Fuente docente/prototipo sin cotejo independiente reciente; futuras E076+ por RFC. Endpoints en ejecución NOT_RUN.
- H07 APPROVED_DOCUMENTAL_PUBLICADO **tras ratificación**: repositorio privado `nexumpr/Nexum-Backend`, primer commit `84be72d`, PR #1 y main `dc23e9bc9b432631ce5346adc201cd96851edecc`; CI success run `42138280237` según evidencia compartida. Fallo CRLF histórico y fix no ocultos.
- H08-A AUTHORIZED_LOCAL_ONLY **tras ratificación**: código de arquitectura Go, una spec por vez y primera A-000, permisos ASK. H08-I/E esperan aprobación y no se implementan/despliegan todavía.

**Advertencia:** no se ha realizado prueba de ejecución de configuración OpenCode en Windows 11 actual, ni Go build, ni AWS/HTTP real; pruebas estáticas del paquete no sustituyen esas evidencias. Deben validarse tras publicar configuración y antes de usar cambios automáticos.

`constitution.md` y `sdd/_control/H07/ESTADO_Y_GATES.md` son fuente normativa y estado único. No editar A/I/E para actualizar cabeceras históricas; conservar contraste con el Gate.

## Registro de Incidentes y Resoluciones H08-A

### Incidente 1 (CI GitHub Actions: Falso Positivo `UNDECLARED_FILE` / `GIT_UNDECLARED`):
- **Error observado en CI:** `herramientas/validar_sdd_integral.py --check-git` arrojó `exit code 2` reportando archivos Go (`cmd/server/main.go`, `go.mod`, `internal/...`, `pkg/...`, `tests/...`) como `UNDECLARED_FILE` y `GIT_UNDECLARED`.
- **Causa raíz:** `validar_sdd_integral.py` fue diseñado en H07 para validar exclusivamente el SDD documental frente a `BASELINE_SHA256.json`. Al ingresar archivos Go en las rutas legítimas de H08-A (`cmd/`, `internal/`, `pkg/`, `tests/`, `go.mod`, `go.sum`), el script los trataba como extras imprevistos del SDD.
- **Solución aplicada:** Se actualizó `herramientas/validar_sdd_integral.py` con `is_h08_code` para aislar las rutas de código de implementación H08-A de la validación estricta de documentos SDD, y se sincronizó el hash de control en `BASELINE_SHA256.json`.
- **Verificación:** Ejecución de `validar_sdd_integral.py --check-git` (PASS), `validar_h06.py` (PASS) y `herramientas/tests/` (10/10 tests OK). Pruebas de Go en Windows nativo (`go test -v ./...` y `go vet ./...`) al 100% PASS.
