# RFC-018 — A6: época de revocación de sesiones (`sessionEpoch` / `epo`)

| Campo | Valor |
|---|---|
| Documento | `sdd/_control/H08/RFC-018-A6-EPOCA_REVOCACION.md` |
| Fecha | 2026-11-17 |
| Estado | **PROPUESTO — pendiente de `APRUEBO_RFC_018_A6_EPOCA_REVOCACION`** |
| Autoridad de origen | RFC-016 §17.3 (A6) y contrato E006 |
| Alcance | Código compartido de E002, E003, middleware de sesión y E006 |
| Aprobador | Responsable del proyecto |

## 1. Motivo

E006 (`POST /api/v1/auth/password/reset`) debe revocar **todas** las sesiones anteriores al
cambio de contraseña. La alternativa de recorrer y revocar N sesiones en una transacción está
limitada por el máximo de 100 acciones de `TransactWriteItems` y no cubre más de ~30 sesiones
con las acciones restantes. RFC-016 §17.3 define una **época de revocación** O(1):
`sessionEpoch` en el perfil y claim `epo` en el JWT. Este RFC registra el cambio de código
compartido que hace efectiva esa decisión.

Sin A6, E006 cambiaría el hash pero los access/refresh tokens anteriores seguirían válidos:
eso incumpliría el contrato y la obligación de seguridad de E006.

## 2. Diseño implementado

| Componente | Cambio |
|---|---|
| `USER#<id>/PROFILE` | Nuevo atributo numérico `sessionEpoch` (ausente = 0). E006 lo incrementa exactamente una vez dentro de la transacción. |
| E002 login | Lee `sessionEpoch` del perfil, lo persiste en `USER#<id>/SESSION#<id>` y emite el claim `epo` del JWT. |
| E003 refresh | Exige que la época de la sesión coincida con la del perfil (`verifyUserState`) y añade un `ConditionCheck` del perfil con la época vigente dentro de la propia transacción de rotación. Propaga la época al access token nuevo. |
| Middleware | Tras validar firma, `sub` y `sid`, lee `USER#<id>/PROFILE` con `ConsistentRead` y compara `epo` con `sessionEpoch`. Desalineado ⇒ **401**; DynamoDB inaccesible ⇒ **503** (nunca autoriza por defecto). |
| E006 | La transacción actualiza hash, `passwordChangedAt`, `sessionEpoch` y `version` con CAS. No recorre sesiones. |

## 3. Compatibilidad

| Caso | Tratamiento |
|---|---|
| Perfil sin `sessionEpoch` | 0 |
| Sesión sin `sessionEpoch` | 0 |
| JWT sin `epo` | 0 |
| Tras el primer cambio de contraseña | `sessionEpoch = 1`; toda sesión/token con época 0 queda inválido |
| Sesiones nuevas | Nacen con la época vigente y siguen operando |
| Logout y revocación por replay | Sin cambios; siguen siendo independientes de la época |

Un token legado (sin `epo`) sigue siendo válido antes del primer cambio de contraseña, porque
0 == 0. No se invalida ninguna sesión existente por desplegar A6.

## 4. Consecuencias

- **Coste:** una lectura consistente adicional de `PROFILE` por petición autenticada. No se
  añade caché Redis: una caché introduciría una ventana en la que una sesión revocada seguiría
  autorizando; el RFC-016 §17.3 permite la variante con caché + `DEL`, pero se descarta por
  seguridad y se declara el coste real.
- **Disponibilidad:** si DynamoDB no responde, las rutas autenticadas responden 503. Es el
  comportamiento fail-closed exigido; no hay autorización por defecto.
- **Sin tablas, índices, permisos IAM ni recursos nuevos:** el perfil ya es accesible con
  `GetItem` sobre `USER#*` y la sesión ya se lee con consistencia fuerte.
- **Reversión:** desplegar un binario anterior ignora `sessionEpoch`/`epo`; el atributo
  permanece y no rompe nada. Revertir E006 no requiere migración de datos.

## 5. Alternativas descartadas

1. **Revocar 30 sesiones por transacción:** límite artificial e incumple «todas las sesiones»
   para >30; requeriría localizar sesiones por familia sin índice. Descartada.
2. **Caché Redis de la época con TTL:** abre una ventana de aceptación de tokens revocados y
   exige `DEL` en cada cambio; contradice el requisito de revocación inmediata en ambas
   instancias. Descartada.
3. **Comparar `jti` con `accessJti`:** E003 rota el jti en cada refresh; invalidaría tokens
   legítimos de sesiones activas (T19). Descartada.

## 6. Evidencia de implementación

- Código y pruebas descritos en `GATE_H08E_E006.md`.
- `go build ./...` y `go vet ./...` PASS en local (2026-11-17).
- Ejecución de pruebas: autoridad CI Linux (Windows Application Control bloquea binarios de
  prueba en el host). Estado `NOT_RUN` local.
- `AWS_NOT_RUN`: ningún recurso, permiso ni secreto modificado por este RFC.

## 7. Frase de aprobación solicitada

```
APRUEBO_RFC_018_A6_EPOCA_REVOCACION
```
