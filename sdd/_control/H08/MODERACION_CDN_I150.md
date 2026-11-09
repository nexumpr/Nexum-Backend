# Moderación de contenido y CDN — I-150 (INF-150-07)

| Campo | Valor |
|---|---|
| Spec | `I-150-publicacion-y-compatibilidad-api` |
| Versión | 0.1.0 |
| Estado | **DISEÑO NO APLICADO** |
| Moderación operativa | **NO** (ningún servicio desplegado) |
| Resultado del escaneo | **0** operaciones con medios · **3** con texto libre de usuario (E028, E030, E075) |

## 1. Escaneo de los 75 contratos

`verificar_publicacion_i150.py contenido` recorre los `requestBody` de las 75 operaciones y clasifica:

| Categoría | Criterio | Encontrado |
|---|---|---|
| Medios | `format` `binary`, `base64` o `byte` en el esquema de entrada | **0** |
| Texto libre de usuario | campo `description` (E028, E030) o `text` de voz (E075) | **3** |
| Datos financieros estructurados | importes en centavos, fechas y enums | 75 operaciones |

Conclusión: **no hay medios** (archivos, imagen o vídeo) en el catálogo, por lo que no se contrata
servicio de moderación de medios ni se crea distribución CDN adicional. Sí existe **texto libre de
usuario** en tres operaciones, que exige controles de validación (no de moderación de medios):

| Operación | Campo | Tratamiento previsto |
|---|---|---|
| E028 `POST /api/v1/movements` | `description` | límite de longitud, normalización y descarte de caracteres de control; sin HTML |
| E030 `PATCH /api/v1/movements/{movementId}` | `description` | mismas reglas que E028 en la actualización |
| E075 `POST /api/v1/voice/parse` | `text` | límite de longitud, sin persistencia del texto crudo y sin PII en logs |

Estas reglas se aplican en el handler cuando H08-E se autorice; hoy el contrato está documentado y la
validación de contenido **no** está implementada.

## 2. Criterio de activación futura

Si un RFC añade una operación que acepte contenido de usuario:

1. Validación previa al almacenamiento (tipo MIME real, tamaño y firma).
2. Moderación del contenido aplicable **antes** de persistir; rechazo con error contractual.
3. Protección de datos: sin datos personales en logs ni en nombres de objeto.
4. CDN de AWS cuando el contenido sea público, con control de acceso y cacheo acotado.
5. Estimación de costos (almacenamiento, transferencia, moderación) y Gate de despliegue propio.
6. Pruebas negativas de contenido no permitido y de tamaño excedido.

Mientras no exista esa operación, este documento **no** acredita moderación alguna. El escaneo
automático (`verificar_publicacion_i150.py contenido`) devuelve `CONTENIDO_CUBIERTO` con
`moderacion_operativa = false`.

## 3. Límites

- No se implementan endpoints ficticios de subida.
- No se despliega ni contrata nada.
- `verificar_publicacion_i150.py contenido` devuelve `CONTENIDO_CUBIERTO` con
  `moderacion_operativa = false` y lista las tres operaciones con texto libre.
