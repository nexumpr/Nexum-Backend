# Propuesta de actualización del aviso de privacidad E015 — Oleada 2

| Campo | Valor |
|---|---|
| Documento | `sdd/_control/H08/PROPUESTA_AVISO_E015_OLEADA_2.md` |
| Versión | 1.0.0 |
| Fecha | 2026-11-18 |
| Estado | **DRAFT — no aprobado, no aplicado** |
| Versión vigente | `2026-11-v2` · SHA-256 `68f1052a4bcab70a6d8803552dccc67d8b5a23c5b383b5480a83f31e408505c9` |
| Versión propuesta | `2026-11-v3` · hash **pendiente** de aprobación del texto |
| Aprobador | Responsable de Nexum-Backend |
| Frase requerida | `APRUEBO_AVISO_PRIVACIDAD_E015_2026_11_V3_SHA256_<HASH>` |

> El texto aprobado, su hash, `VersionAviso`, `TextoResumen`, `E015_EFFECTIVE_AT` y el repositorio
> `nexumpr/nexum-privacidad` **no se modifican** con este documento. Solo se inventaría el impacto
> y se propone redacción.

## 1. Motivo

La Oleada 2 habilita movimientos, pagos recurrentes y metas, y la eliminación de cuenta desde la
app (E014). El aviso vigente afirma lo contrario en dos lugares y describe la eliminación como
manual. Publicar esos endpoints sin actualizar el aviso produciría una discrepancia entre la
funcionalidad real y la información al titular.

## 2. Inventario de afirmaciones afectadas (`aviso_2026_11_v2.txt`)

| Línea | Texto vigente (extracto) | Estado tras Oleada 2 | Acción |
|---|---|---|---|
| 11 | «Funciones disponibles actualmente (registro, autenticación, sesión, perfil, consentimientos, configuración inicial, ingreso base, preferencias y contenido de ayuda)» | Incompleto | Añadir movimientos, pagos recurrentes, metas y eliminación de cuenta |
| 12–15 | Categorías de datos descritas | Incompleto | Añadir datos financieros: movimientos, recurrentes, metas |
| 16 | «Funciones previstas que aún no están disponibles: movimientos, presupuestos, metas de ahorro y aportaciones, exportación de datos, eliminación automática de cuenta, notificaciones push, análisis de patrones y registro por voz» | **Falso** para movimientos, metas y eliminación automática | Retirar esas tres; conservar presupuestos, aportaciones, exportación, notificaciones, análisis y voz |
| 20 | Finalidades primarias | Incompleto | Añadir registro/consulta de movimientos, recurrentes y metas y cálculo de resúmenes |
| 21 | Secundarias: notificaciones y análisis «aún no disponibles» | Correcto | Sin cambio |
| 31 | Seguridad (HTTPS, Argon2id, JWT, rotación, revocación) | Correcto | Sin cambio |
| 33–34 | Conservación | Correcto pero incompleto | Mencionar que la supresión se ejecuta por lotes con estado verificable |
| 36 | «La eliminación automática de la cuenta desde la app aún no está disponible…» | **Falso** | Reemplazar por la descripción real de E014 |
| 39 | Derechos: canales manuales | Incompleto | Añadir la eliminación desde la app como vía |
| 42–43 | Micrófono y notificaciones; voz no disponible | Correcto | Sin cambio |
| 3, 45 | Identificador de versión | Cambia | `2026-11-v3` |

## 3. Redacción propuesta (DRAFT, sujeta a aprobación)

**2.1 (encabezado y viñetas).** «Funciones disponibles actualmente (registro, autenticación,
sesión, perfil, consentimientos, configuración inicial, ingreso base, preferencias, contenido de
ayuda, movimientos, pagos recurrentes, metas de ahorro y eliminación de cuenta):»

- «Datos financieros: movimientos (tipo, importe, moneda, categoría, descripción y fecha),
  pagos recurrentes (concepto, importe, frecuencia, próxima fecha y estado) y metas de ahorro
  (nombre, importe objetivo, ahorro inicial, fecha objetivo y estado).»
- «Solicitud de eliminación: identificador de la solicitud, fecha, estado del proceso y conteo
  de registros suprimidos. No se conserva el contenido de los datos eliminados.»

**2.2.** «Funciones previstas que aún no están disponibles: presupuestos, aportaciones
periódicas a metas, exportación de datos, notificaciones push, análisis de patrones y registro
por voz. Cuando se habiliten, este aviso se actualizará y se solicitará la aceptación
correspondiente.»

**3 (primarias).** Añadir: «registrar, consultar, corregir y eliminar movimientos, pagos
recurrentes y metas; calcular resúmenes y totales a partir de ellos».

**7 (conservación y eliminación).** Sustituir el párrafo de eliminación manual por:

> «La eliminación de la cuenta puede solicitarse desde la app, con confirmación expresa y
> verificación de la contraseña. Al aceptarse la solicitud, la cuenta queda bloqueada de
> inmediato, se revocan las sesiones activas y los datos dejan de ser accesibles. La supresión de
> los datos asociados se ejecuta por lotes mediante un proceso interno con estado verificable y
> reanudable; la solicitud no se considera completada hasta que ese proceso confirma la
> eliminación de los datos que debe suprimir. Ciertos registros técnicos de vida corta (por
> ejemplo, ventanas de idempotencia y tokens de un solo uso) dejan de ser válidos al vencer su
> plazo y quedan inutilizables; su eliminación física la realiza el proveedor de nube de forma
> asíncrona, por lo que puede completarse después de ese vencimiento. Este aviso no promete un
> plazo máximo de eliminación física distinto del que la infraestructura garantiza. Se conserva un
> registro mínimo de la solicitud, sin datos de contacto ni financieros, por un plazo limitado y
> con fines de trazabilidad. También puede solicitarse por correo a nexumproyectomov@gmail.com.»

**8.** Añadir: «La eliminación de la cuenta se ejerce directamente desde la app».

**10.** «Versión `2026-11-v3`».

### 3.1 Distinciones que el texto y el código deben respetar

| Concepto | Qué garantiza Nexum | Qué **no** promete |
|---|---|---|
| Bloqueo de la cuenta | Inmediato, al confirmar la transacción | — |
| Revocación de sesiones | Inmediata y efectiva en ambos nodos (estado + época) | Borrado físico de las sesiones en ese instante |
| Supresión de datos localizables | Por lotes, con estado persistido y verificación previa al cierre | Instantaneidad |
| Vencimiento lógico de artefactos efímeros | Plazo calculado por la aplicación | Eliminación física en ese plazo |
| Eliminación física | Confirmada solo cuando `DeleteItem` se ejecutó y la relectura no devuelve el ítem | Plazo máximo por parte de DynamoDB TTL, que es asíncrono |
| Retención del tombstone | Metadatos mínimos (solicitud, fecha, conteo), 90 días, sin contacto ni finanzas | Conservación indefinida |
| Estado `PURGED` | Se registra solo tras verificar el catálogo localizable | Que no queden artefactos efímeros no localizables pendientes de TTL |

Por tanto, queda **prohibido** en el aviso y en la documentación afirmar que un registro TTL se
elimina físicamente «en un máximo de 24 horas». Si en el futuro una obligación exigiera un plazo
concreto de eliminación efectiva, se implementaría un mecanismo explícito y verificable (barrido
por referencia localizable) con su propio Gate, sin apoyarse en TTL.

## 4. Impacto técnico del cambio

| Elemento | Acción |
|---|---|
| `internal/application/privacy/aviso_2026_11_v3.txt` | Archivo nuevo (no sobrescribe v2) |
| `VersionAviso` | `2026-11-v3` |
| `TextoResumen` | Actualizar la lista de funciones y quitar «eliminación automática pendiente» |
| `EffectiveAtMinimo` | Se mantiene `2026-11-18T00:00:00Z` |
| `E015_EFFECTIVE_AT` (bundle RFC-004) | Nueva fecha en el Gate de despliegue, escritura condicional |
| Hash | Se calcula al aprobar el texto y se registra en el Gate; la prueba lo compara |
| `requiresAcceptance` | `true`: nueva versión sustancial → el cliente debe pedir aceptación |
| Consentimientos | Los registros previos conservan `2026-11-v2`; no se reescriben |
| `nexumpr/nexum-privacidad` | **No se toca**; la URL pública sigue apuntando al endpoint de la API |

## 5. Plan de versionado y aprobación

1. Aprobar la redacción (`APRUEBO_AVISO_PRIVACIDAD_E015_2026_11_V3_SHA256_<HASH>` tras conocer el
   hash del texto final).
2. Implementar el archivo v3, actualizar constantes y pruebas de hash.
3. PR propio, CI verde, Gate de merge.
4. En la release agrupada, actualizar `E015_EFFECTIVE_AT` en el bundle (Gate de apply) y verificar
   `GET /privacy/notices/current` con la versión y el hash nuevos.

## 6. Riesgos

| Riesgo | Tratamiento |
|---|---|
| Anunciar E014 antes de publicar el aviso v3 | El Gate de la oleada exige el aviso aprobado antes de declarar E014 disponible |
| Cliente que no revalida `requiresAcceptance` | Se documenta en el contrato de integración Android; el backend registra la versión aceptada |
| Deriva entre texto y código | Prueba de hash + verificación HTTP tras el despliegue |
| Texto legal con promesas no verificadas | La redacción distingue bloqueo y revocación inmediatos, supresión verificada por lotes, vencimiento lógico y eliminación física asíncrona del proveedor; no promete plazos que la infraestructura no garantice |
