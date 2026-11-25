# D-E062-01 — Identificadores deterministas de E062: UUID v8 (RFC 9562)

| Campo | Valor |
|---|---|
| Documento | `sdd/_control/H08/DECISION_E062_UUID_V8.md` |
| Fecha | 2026-11-24 |
| Unidad / rama | U1 · `.worktrees/fix-o4` · `fix/h08e-oleada4-regularizacion` (PR #143) |
| Ámbito | ID de notificación que produce `pushoutbox` y que consumen E061/E062 |
| Estado | **PROPUESTO — NO APROBADO** |
| Frase de aprobación propuesta | `APRUEBO_DECISION_E062_UUID_V8_SHA256` |

## 1. Contrato aplicable

El OpenAPI de E062 (`PATCH /api/v1/notifications/{notificationId}/read`) declara
`notificationId` con `format: uuid`. **No** impone `pattern`, versión ni algoritmo
de derivación. Por tanto, cualquier UUID canónico con guiones es contractual, y la
forma hex de 32 caracteres que se emitía antes no lo era.

## 2. Decisión

Los identificadores se derivan con **UUID v8 (RFC 9562 §5.8) y SHA-256**, usando
un namespace propio de Nexum:

- Paquete: `pkg/uuidv8` (`UUID`, `Parse`, `MustParse`, `New`, `String`, `EsV8`).
- Derivación: `SHA-256(namespace_bytes || name_bytes)`.
- Nombre: `userID + "|" + eventID` (con `TrimSpace`, igual que la deduplicación).
- Bits: se toman los 16 primeros bytes del digest; versión 8 en el nibble alto del
  byte 6 (`b[6] = (b[6] & 0x0F) | 0x80`) y variante RFC 4122 en el byte 8
  (`b[8] = (b[8] & 0x3F) | 0x80`).
- Formato: 8-4-4-4-12 canónico en minúsculas (36 caracteres).
- `NamespaceNexum = e882a8d5-8a71-8fbf-ac40-81f9cac964c7`, calculado una sola vez
  como los 16 primeros bytes de `SHA-256("nexum:uuidv8:namespace:v1")` con los
  bits de versión/variante fijados, y congelado. No es un namespace DNS/URL/OID
  de la IANA.

## 3. Alternativas

| Alternativa | Resultado |
|---|---|
| UUID v5 (SHA-1) | **Descartada.** Introduce `crypto/sha1` y dispara gosec G401/G505. Evitarlo exigiría `#nosec`, prohibido por el Gate de la fase (control 8). |
| Hex de 32 caracteres | **Descartada.** No cumple `format: uuid`; E062 responde 400 `invalid_uuid`. |
| UUID v4 aleatorio | **Descartada.** Rompe el determinismo por `(userID, eventID)`: un reintento del mismo evento generaría otro ID y duplicaría la bandeja. |
| UUID v7 | **Descartada.** Su componente temporal no aporta nada aquí y complica la reproducibilidad del valor. |

## 4. Garantías

- Determinista: el mismo `(userID, eventID)` produce siempre el mismo ID, en
  cualquier nodo y tras reinicios.
- Distinto por usuario y por evento; con namespace distinto, distinto ID.
- No expone el `userID` en claro ni contiene secretos.
- No cambia la deduplicación ni `IDEM#`: se mantiene por `(userID, eventID)` y la
  escritura condicional; el UUID solo identifica el recurso de la bandeja.

## 5. Compatibilidad

No existen IDs productivos con la forma antigua: no hay productor de notificaciones
en producción antes de esta oleada. No se requiere migración ni doble lectura. Los
IDs ya persistidos en pruebas no son datos productivos.

## 6. Verificación prevista (RC)

```powershell
gofmt -l pkg internal
go build ./...
go vet ./...
go test ./pkg/uuidv8 ./internal/application/pushoutbox ./tests/unit/transport -run 'PUSH|E062|UUID' -count=1
go run honnef.co/go/tools/cmd/staticcheck@v0.8.2 ./...
go run github.com/securego/gosec/v2/cmd/gosec@v2.30.0 -quiet ./pkg/... ./internal/application/pushoutbox/...
```

Criterio: gosec **0 hallazgos** (sin `#nosec`) y `pkg/uuidv5` fuera del commit.

## 7. Aprobación

Pendiente. La frase `APRUEBO_DECISION_E062_UUID_V8_SHA256` **no** ha sido
concedida; este documento no aprueba por sí solo ni autoriza merge, release o
despliegue.
