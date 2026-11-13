# Checklist de calidad — E003 (`POST /api/v1/auth/refresh`)

**Diseño H06:** REVIEW. **Implementación:** local en `feat/h08e-e003-auth-refresh`.
**Compilación y pruebas:** pendientes del CI Linux (`BLOCKED_POLICY_APPLICATION_CONTROL`
local). **Respuesta prevista:** 200 `AuthTokens`. **AWS/HTTP real:** `NOT_RUN`.

- [x] Método y ruta conservan el ID del inventario H04 (`E003`, `POST /api/v1/auth/refresh`).
- [x] Payload válido y negativo revisado; `additionalProperties: false` aplicado en runtime.
- [x] Respuesta 200 conforme al contrato vigente; `401`, `deviceId` opcional y `session.id`
      aprobados por **RFC-014** (`APRUEBO_RFC_014_E003_REFRESH`, 2026-11-13T18:18Z) y
      aplicados en `nexum-api-v1.yaml` y `.json` con paridad verificada.
- [x] Semántica de credencial: token opaco en el cuerpo, `security: []`; sin JWT previo.
- [x] Idempotencia no exigida por el contrato; concurrencia resuelta por compare-and-swap y
      documentada (un doble envío consume el token y revoca la familia, según el SDD).
- [x] DynamoDB: `GetItem` con consistencia fuerte, `TransactWriteItems` condicional,
      localizadores, TTL `expiresAt` Number y **cero `Scan`**.
- [x] Redis: cuotas con identificadores HMAC, TTL por ventana y fallo cerrado.
- [x] Sin montos ni timestamps de dominio financiero en esta operación.
- [x] Errores pertinentes: 400/401/409/429/500/503 cubiertos por pruebas.
- [x] Sin PII ni secretos en logs; el token nunca se registra ni se persiste en claro.
- [ ] Contrato OpenAPI actualizado: **bloqueado** hasta `APRUEBO_RFC_014_E003_REFRESH`.
- [ ] `gofmt`, `go build`, `go vet`, `staticcheck`, `govulncheck`, `gosec`, `gitleaks`,
      Semgrep y `go test -race`: **PASS** en el run `43478609688` (HEAD `04799eb`); se
      reejecutarán sobre el HEAD con el contrato RFC-014 aplicado antes de declarar el cierre
      contractual.
- [ ] API en dos instancias, TLS público, CI/CD e IaC: `NOT_RUN`.
- [ ] Gate individual de release H08 y rollback aprobados antes de cualquier `terraform apply`.
- [x] Dependencias del cliente Mobile no constituyen Gate del Backend.

**Criterio de cierre:** ninguna casilla operativa se marca por generar este documento; las
casillas de CI y despliegue sólo se marcarán con la ejecución real y su evidencia.
