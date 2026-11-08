# I-090 — Plan de rollback

| Campo | Valor |
|---|---|
| Spec | `sdd/02-infraestructura/I-090-cd-backend` (INF-090-03, INF-090-05) |
| Estado | `IMPLEMENTADO_LOCAL`; ensayo **NOT_RUN** (sin AWS) |
| Alcance | Rollback de aplicación y de esquema DynamoDB |

## 1. Cuándo se aborta el despliegue

El workflow aborta y ejecuta rollback si:

- antes del cambio no hay **2 nodos `InService`** o no hay **2 targets `healthy`**;
- el `instance refresh` queda `Failed`, `Cancelled`, `RollbackInProgress` o `RollbackFailed`;
- tras el rollout no hay 2 nodos `InService` y 2 targets `healthy`;
- `GET https://<api>/api/v1/health/live` no devuelve **200**.

## 2. Rollback de aplicación (máquina de estados)

1. **No ejecutar rollback si no hubo mutación AWS**: el workflow crea `.mutado` justo antes del primer
   cambio; sin ese marcador no se toca AWS.
2. **Capturar antes de mutar**: versión real del Launch Template y `max_size` desde AWS (no se confía en
   el input `lt_version_previa`; si se aporta, debe coincidir con la real o se aborta).
3. **Cancelar el refresh activo**: se identifica el `InstanceRefreshId` concreto. Si está en `Pending`,
   `InProgress`, `Cancelling` o `Baking`, se ejecuta `cancel-instance-refresh` y se consulta AWS en cada
   iteración, comprobando el **mismo ID** hasta estado terminal (`Cancelled`/`Failed`/`Successful`). Si en
   el timeout no hay confirmación → `ROLLBACK_BLOCKED_ACTIVE_REFRESH`, se conserva `max=3` y no se lanza
   otro refresh. Si el estado es `RollbackInProgress` se distingue y se espera su resultado; no se
   cancela ni se solapa otro refresh.
4. **Mantener capacidad**: `max_size` permanece en **3** durante la recuperación; **no** se restaura a 2
   antes del refresh de rollback.
5. Aplicar la **versión previa real** del Launch Template y lanzar el refresh con
   `MinHealthyPercentage=100` y `MaxHealthyPercentage=150`, guardando su `InstanceRefreshId`.
6. Esperar con un indicador explícito de éxito: sólo `Successful`/`RollbackSuccessful` confirman;
   `Failed`/`Cancelled`/`RollbackFailed` o estado desconocido → `ROLLBACK_FAILED`; timeout →
   `ROLLBACK_FAILED` con `max=3` conservado.
7. Verificar la versión del LT aplicada, 2 targets `healthy` y la configuración previa.
8. Sólo entonces restaurar `max_size=2` y registrar la evidencia del rollback.
9. **Nunca** se revierten tablas, índices, TTL ni datos DynamoDB: el rollback de aplicación es
   independiente del de esquema.

El rollback de aplicación **no** toca DynamoDB ni el state de Terraform.

## 2.1 Capacidad durante el rollout y el rollback

El ASG no puede mantener 2 sanos con `max=2` mientras reemplaza instancias. El CD eleva `max` a 3
durante el rollout (capacidad transitoria) y lo restaura a 2 al terminar **o** al entrar en rollback. El simulador local
`herramientas/simular_despliegue.py` modela la decisión (ABORTAR/ROLLBACK/DESPLEGAR) y exige
`max_final=2` en todos los casos; sus pruebas cubren refresh fallido, nodos insuficientes, HTTP y
readiness.

## 3. Rollback de esquema DynamoDB

- Las migraciones deben ser **compatibles hacia atrás**: la versión anterior de la app debe poder leer
  y escribir el esquema nuevo.
- Prohibido eliminar atributos, índices o tablas en el mismo release que cambia el código.
- TTL y GSI se versionan; un cambio de GSI exige plan revisado y ventana propia.
- El rollback de esquema es **independiente** del rollback de aplicación y requiere autorización
  humana específica con `PLAN_ROLLBACK` revisado.

## 4. Datos persistentes

- No se borra el bucket de state, la tabla `NEXUM_CORE` ni Redis para "arreglar" un despliegue.
- El state y los datos se preservan; cualquier retirada exige Gate independiente (I-070/I-130).
- `prevent_destroy` se mantiene.

## 5. Limitaciones declaradas

- El rollback **no se ha ensayado**: `AWS_NOT_RUN`. Se ensayará en el Gate de despliegue AWS.
- El presupuesto no cubre doble capacidad sostenida; el rollout usa `MinHealthyPercentage=100`, no
  blue-green con duplicación total.
- Si faltan permisos OIDC/ASG, el rollback también falla: se registra como bloqueo, no se simula.

## 6. Criterio de cierre

Un release sólo se declara cerrado con: 2 targets sanos, health HTTP 200, evidencia registrada,
coste observado y errores documentados. Sin ensayo de rollback en AWS, el estado máximo es
`IMPLEMENTADO_LOCAL`.
