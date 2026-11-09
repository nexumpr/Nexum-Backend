# Acceso operacional seguro — I-140 (INF-140-05)

| Campo | Valor |
|---|---|
| Spec | `I-140-operacion` |
| Versión | 0.1.0 |
| Estado | **IMPLEMENTADO_LOCAL — NO DESPLEGADO** |
| Verificación AWS | `NOT_VERIFIED` |

## 1. Reglas

| Regla | Estado exigido |
|---|---|
| Identidades temporales (SSO/OIDC) | Obligatorio |
| Mínimo privilegio documentado | Obligatorio |
| SSM solo dentro de ventana autorizada | Obligatorio |
| Auditoría de accesos (CloudTrail) | Obligatorio, `NOT_VERIFIED` en cuenta |
| Shell de root | Prohibido |
| SSH público | Prohibido |
| Claves de acceso estáticas | Prohibidas |
| Cambio de SCP | Prohibido sin RFC y Gate |
| Eludir permisos o SCP | Prohibido |
| Sesiones AWS inventadas | Prohibido |

## 2. Procedimiento de acceso

1. Autenticarse con identidad temporal (SSO/OIDC) y MFA.
2. Verificar el entorno y el Gate aplicable.
3. Abrir SSM **solo** dentro de la ventana autorizada y con registro en CloudTrail.
4. Ejecutar únicamente comandos de diagnóstico aprobados; sin cambios de datos.
5. Cerrar la sesión y registrar evidencia (hora, actor, comandos, resultado).
6. Si IAM/SCP deniega: **detenerse**, registrar `BLOCKED` y escalar; nunca buscar otra identidad,
   región o cuenta.

## 3. Recuperación y secretos

- La recuperación de nodos depende de SSM en ventana y del agente operativo (pendiente en I-020).
- Los secretos no se copian al repositorio, al state ni a logs; rotación según `RFC-004`.
- `AWS_SCP_NOT_VERIFIED`: la SCP real no se ha inspeccionado.

## 4. Dependencias

I-010 (red y endpoints), I-020 (cómputo y SSM), I-060 (IAM runtime y OIDC), I-090 (entorno de
despliegue). Ninguna se modifica desde I-140.

## 5. Validación local

```powershell
py -3.13 herramientas/verificar_operacion_i140.py acceso --datos tests/fixtures/operacion/acceso.json
```

Rechaza root, SSH público, claves estáticas, cambio de SCP, SSM fuera de ventana y ausencia de
auditoría o mínimo privilegio.

## 6. Límites

- No se ha validado ningún acceso real: `NOT_VERIFIED`.
- No se declara SSM operativo.
- No se abre SSH público en ninguna circunstancia.
