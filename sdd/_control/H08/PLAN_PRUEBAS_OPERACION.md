# Plan de pruebas operativas — I-140

| Campo | Valor |
|---|---|
| Spec | `I-140-operacion` |
| Versión | 0.1.0 |
| Estado | **IMPLEMENTADO_LOCAL — NO DESPLEGADO** |
| Ejecución | `NOT_RUN` en AWS; validación local de coherencia |

## 1. Drills previstos (cinco, todos `NOT_RUN`)

| ID | Componente | Degradación esperada | Criterio de éxito | Rollback | AWS requerido |
|---|---|---|---|---|---|
| `aislar-instancia-a` | Cómputo | 1 target sano; el ALB envía todo a B | Responde 200 con 1 nodo y el ASG repone el segundo | Restaurar capacidad con la LT vigente | Sí |
| `reemplazo-target` | Balanceo | Un target sale del pool durante el refresh | `HealthyHostCount` vuelve a 2 sin 5xx | Cancelar el refresh y conservar la LT previa | Sí |
| `fallo-redis` | Caché | Lecturas sin caché; autenticación fail-closed | Sin validación de credenciales sin Redis y sin 5xx sostenidos | Restablecer el endpoint anterior | Sí |
| `cuota-dynamo` | Datos | Throttling controlado | Backoff efectivo, sin `Scan` y sin pérdida de datos | Detener la carga sintética | Sí |
| `rotacion-jwt` | Identidad | Tokens previos dejan de validar | Sesiones revocadas no se reactivan y el token nuevo funciona | Restaurar claves anteriores en ventana | Sí |

## 2. Condiciones comunes

- Datos **sintéticos** y entorno autorizado.
- Ningún drill puede destruir tablas, apagar el core ni dejar el ASG por debajo de 2.
- Cada drill exige Gate humano y ventana; sin él permanece `NOT_RUN`.
- La evidencia incluye duración, errores observados y estado final.

## 3. Validación local

```powershell
py -3.13 herramientas/verificar_operacion_i140.py drills --datos tests/fixtures/operacion/drills.json
```

Rechaza drills faltantes, estados `EJECUTADO_AWS` en fase local y acciones destructivas.

## 4. Lo que este plan no afirma

- No se ejecutó ningún drill: `NOT_RUN`.
- No se declara tolerancia a fallos medida.
- No sustituye pruebas de carga ni DAST, que tienen su propio Gate.
