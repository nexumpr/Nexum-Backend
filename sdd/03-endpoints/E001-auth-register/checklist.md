# Checklist de calidad — E001 V6.0.2

**Contrato:** V6.0.2 alineado con el prototipo Nexum (RFC-012). **Implementación local:**
verificada con `go build`, `go vet` y `go test`. **HTTP/DynamoDB/Redis reales:** `NOT_RUN`.
**Respuesta prevista:** 201 (`RegisterResponse`).

## Contrato

- [x] Método y path conservan el identificador y la ruta del inventario H04.
- [x] `RegisterRequest` documenta los nueve campos de entrada y sus reglas exactas.
- [x] `RegisterResponse` documenta los cinco bloques de salida y no expone contraseña ni hash.
- [x] OpenAPI YAML y JSON sincronizados (`paridad` = `PARIDAD_OK`).
- [x] `UserProfile` intacto para E012; el cambio de esquema de E001 está aislado.
- [x] Ruptura intencional registrada en RFC-012 y en el registro de excepciones de API.
- [x] Los otros 74 contratos no cambian de comportamiento.

## Permisos y seguridad

- [x] Endpoint público sin `Authorization`; HTTPS obligatorio en el borde (CloudFront).
- [x] `Idempotency-Key` obligatoria con validación de longitud y caracteres.
- [x] Contraseña Argon2id de 12–128 caracteres; nunca en claro ni en logs.
- [x] Replay con contraseña distinta devuelve `409` sin almacenar la contraseña.
- [x] PII y secretos ausentes en logs, capturas y fixtures.
- [x] Rate limit de Redis antes de la persistencia; `503` si Redis no responde.

## Dinero y datos

- [x] Importes como cadena decimal de dos decimales en el transporte y `int64` centavos en
      dominio y persistencia; ningún `float`.
- [x] Equivalente mensual calculado con el motor exacto de A-070 y probado en las siete
      frecuencias.
- [x] Marcas de tiempo UTC; zona horaria del cliente validada contra IANA.
- [x] `CONSENT#<noticeVersion>` con versión declarada y `acceptedAt` del servidor.
- [x] Compromisos fijos sin `MOVEMENT`: no se contabilizan como gasto ejecutado.

## Persistencia

- [x] Una única `TransactWriteItems`; ninguna escritura parcial ante fallo de una fila.
- [x] Máximo 94 compromisos (100 acciones − 6 fijas); el exceso se rechaza completo.
- [x] Sin `Scan`; todas las claves usan `GetItem`/`Put` por partición conocida.
- [x] Particiones usadas (`USER#`, `EMAIL#`, `IDEM#`) cubiertas por la política IAM vigente.
- [x] Perfiles antiguos legibles: `PROFILE` conserva `displayName` y los campos heredados.

## Pruebas y entrega

- [x] Pruebas unitarias reales de dominio, aplicación, adaptador y transporte.
- [x] Pruebas negativas: 400/409/422/429/503 pertinentes al caso.
- [x] Contrato OpenAPI y matriz H06 verificados con el validador del repositorio.
- [ ] API en dos instancias, TLS público y despliegue: `NOT_RUN` hasta el Gate de release.
- [ ] Gate individual de release y rollback aprobados antes de publicar en AWS.
- [x] Dependencias del cliente Mobile no constituyen Gate del Backend.

**Criterio de cierre:** ninguna casilla de despliegue se marca por existir este documento.
La verificación HTTP real y la comprobación en DynamoDB quedan pendientes del Gate de
publicación y se registrarán con evidencia del comando y su salida.
