# Plan — E002 V6.0.2 (implementado en local)

**Estado:** contrato V6.0.2 (RFC-013) implementado en la rama `feat/h08e-e002-login-v602`.
**Despliegue AWS:** `NOT_RUN`.

## 1. Arquitectura implementada

Clean Architecture existente, sin capas paralelas:

| Capa | Archivo | Responsabilidad |
|---|---|---|
| Aplicación | `internal/application/identity/login.go` | `LoginService`, validación canónica, flujo de autenticación, construcción de `LoginResponse`, refresh opaco CSPRNG y digest |
| Puertos | `internal/ports/auth.go` | `PasswordHasher`, `TokenProvider` y `TokenClaims` (con `JTI`) |
| Persistencia | `internal/adapters/dynamo/session_store.go` | `LoginStore`: reserva→perfil→onboarding y transacción de sesión |
| Claves | `internal/domain/dynamo/keys.go` | `UserSessionKeys`, `RefreshKeys` (además del histórico `SessionKeys`, conservado) |
| Criptografía | `internal/infrastructure/security/token.go`, `issuer.go` | RS256, validación estricta de claims, carga de PEM desde entorno |
| Caché | `internal/infrastructure/cache/login_rate_limiter.go` | Cuota dual atómica con script Lua |
| Transporte | `internal/transport/http/login_handler.go`, `binding.go` | DTO estricto, límite de 256 KiB, mapeo de errores, cabeceras no-store |
| Composición | `cmd/server/main.go` | Wiring real: DynamoDB, Redis, Argon2id, RSA y limitador |

## 2. Flujo de `LoginService`

1. `ValidateLoginCommand`: email canónico, contraseña sin normalizar, metadatos opcionales.
2. `LookupIdentity`: `GetItem ConsistentRead` de `EMAIL#<sha256>` y de `PROFILE`.
3. Coherencia reserva↔perfil; `status == ACTIVE`.
4. `VerifyPassword` Argon2id; ante correo inexistente, verificación señuelo con un hash
   derivado en construcción del propio hasher (no un literal fijo).
5. `OnboardingConfigured`: `true`/`false` real; ausencia histórica ⇒ `false`.
6. Emisión: `sessionId` y `familyId` UUID v4 CSPRNG, refresh 32 bytes, JWT RS256 900 s.
7. `CreateSession` atómica; si falla ⇒ 503 sin tokens.

Cualquier fallo de identidad/estado devuelve el **mismo** `ErrInvalidCredentials`.

## 3. Modelo físico DynamoDB

| Ítem | PK | SK |
|---|---|---|
| Sesión | `USER#<userId>` | `SESSION#<sessionId>` |
| Localizador refresh | `REFRESH#<sha256(refresh)>` | `SESSION` |

Escritura en una `TransactWriteItems` con `attribute_not_exists(PK)` en ambos ítems.
`expiresAt` es **Number (epoch seconds)** porque es el atributo TTL que
`infra/terraform/modules/dynamodb` exige; `expiresAtISO` sólo aporta legibilidad. No se crean
tablas, índices ni particiones nuevas: IAM ya autoriza `USER#*`, `SESSION#*` y `REFRESH#*`.

## 4. Criptografía y secretos

- Una sola clave RSA (≥2048 bits) para ambas instancias, cargada de
  `JWT_PRIVATE_KEY_PEM_B64` + `JWT_KEY_ID` desde el bundle privado RFC-004; decodificada en
  memoria y nunca escrita ni registrada.
- `kid` explícito; la validación rechaza `kid` desconocido. El `keyring` acepta el conjunto
  de claves públicas vigentes para permitir rotación sin invalidar tokens aún válidos.
- `iss`/`aud` fijos y aprobados; no se sustituyen por el hostname de CloudFront.
- Sin material criptográfico el endpoint responde 503 (fail-closed).

## 5. Redis

Script Lua único que incrementa y expira las dos claves y decide en la misma ejecución:
`nexum:v1:ratelimit:auth_login_ip:<sha256>` y `...:auth_login_email:<sha256>`. Clases
separadas de E001 (que mantiene 5/15 min/IP). La instancia desplegada es Redis OSS de un
nodo, no Cluster, por lo que dos claves en un script son válidas; si se migrara a Cluster
deberían compartir *hash slot*.

## 6. Pruebas

- `tests/unit/application/login_test.go`: contrato completo, `session.id = sessionId`,
  onboarding real e histórico, 401 en los tres escenarios, 400 de validación, fallo cerrado
  sin emisor y sin sesión si la persistencia falla.
- `tests/unit/security`: RS256, expiración y rechazos criptográficos.
- Regresión de E001 en `tests/unit/transport`, `tests/integration` y `tests/security`.
- Pendiente: pruebas dedicadas del adaptador DynamoDB, del limitador dual y del handler
  HTTP con dependencias simuladas.

## 7. Despliegue futuro (no ejecutado)

Release ARM64 canónico desde CI → S3 inmutable (Gate) → bundle de secretos con la clave
compartida (Gate) → despliegue gradual por ASG conservando dos targets sanos → verificación
HTTPS real de E001 y E002. Rollback: release anterior en S3 y versión previa del Launch
Template. Sin cambios de VPC, ALB, CloudFront, DynamoDB ni Redis.
