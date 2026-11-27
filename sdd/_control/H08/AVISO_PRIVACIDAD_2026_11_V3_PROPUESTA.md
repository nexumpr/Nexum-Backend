# Aviso de Privacidad de Nexum — propuesta 2026-11-v3 (revisión completa)

| Campo | Valor |
|---|---|
| Documento | `sdd/_control/H08/AVISO_PRIVACIDAD_2026_11_V3_PROPUESTA.md` |
| Versión propuesta del aviso | **2026-11-v3** |
| Versión del documento | 1.0.0 |
| Fecha | 2026-11-24 |
| Estado | **PROPUESTA — NO PUBLICADA, NO APROBADA** |
| Frase propuesta | `APRUEBO_AVISO_PRIVACIDAD_E015_2026_11_V3_SHA256_1CEC4609FF2F3FC052D3DEB29271BA8AAED322D1EC97AE9757E222CC8C0563B9` |
| Artefacto canónico | `dist/deuda/aviso_2026_11_v3.txt` |
| SHA-256 del artefacto | **`1CEC4609FF2F3FC052D3DEB29271BA8AAED322D1EC97AE9757E222CC8C0563B9`** |
| Tamaño | 13 662 bytes, UTF-8 sin BOM, saltos de línea LF |
| Alcance | Documento y texto. **No** publica, no sustituye consentimientos, no cambia código, no toca AWS |

> **Sustitución íntegra.** Esta propuesta **reemplaza por completo** al borrador
> `internal/application/privacy/aviso_2026_11_v3.txt` (8 123 bytes, SHA-256
> `E925596ADFF1B3EBB9A17064D44F5F6B84BFBDFB874396CD51EFC6C0AC3F8D83`), que nunca se publicó y
> omitía funciones ya desplegadas. No se remite a él ni se reutiliza su texto.
>
> **Aviso vigente hoy:** `2026-11-v2` (6 761 bytes, SHA-256
> `68F1052A4BCAB70A6D8803552DCCC67D8B5A23C5B383B5480A83F31E408505C9`), servido por
> `internal/application/privacy/notice.go` (`VersionAviso = "2026-11-v2"`). **No se toca** en esta
> unidad.

---

## 1. Por qué hace falta una versión nueva

La auditoría del 2026-11-24 marcó E015 como `NO_CONFORME_SEMANTICA`: el aviso `2026-11-v2`
declara expresamente **no disponibles** funciones que ya están publicadas (movimientos, pagos
recurrentes, metas, presupuestos, aportaciones, eliminación de cuenta, exportación, dashboard,
analítica), y el borrador `v3` de 8 123 bytes tampoco las cubre con exactitud. Un aviso que
describe mal el tratamiento es un defecto de transparencia, no una simple omisión editorial.

Esta revisión se ajusta a la **release consolidada** (75 operaciones) y distingue con precisión:

1. lo que está disponible y se ejecuta;
2. lo que está disponible pero **desactivado por defecto** (modo demostración);
3. lo que está **condicionado a una habilitación** (notificaciones push por FCM);
4. lo que **no** se hace (publicidad, venta, IA externa, audio, datos bancarios).

## 2. Texto íntegro (copia de lectura)

> El artefacto con valor legal y hash es `dist/deuda/aviso_2026_11_v3.txt`. Este bloque reproduce
> su contenido para revisión. Si se copia a un archivo, debe conservarse UTF-8 sin BOM y LF para
> que el SHA-256 coincida.

```text
Aviso de Privacidad de Nexum

Versión: 2026-11-v3. La fecha de entrada en vigor se publica en el campo effectiveAt de este endpoint.

1. Responsable del tratamiento
El responsable del tratamiento es Leonardo Isaac Barrera Tejeda, con domicilio en Carretera Tula Tepeji 7, Localidad El Sesenta y Dos, C.P. 42836, Tula de Allende, Hidalgo, México. Para asuntos de privacidad y derechos ARCO puede contactarse el correo nexumproyectomov@gmail.com.

2. Naturaleza del servicio
Nexum es una aplicación Android de gestión de finanzas personales. No es un banco, entidad financiera ni sociedad fintech; no se conecta a cuentas bancarias ni solicita claves interbancarias, números de tarjeta, NIP ni contraseñas bancarias. Los importes, resúmenes, análisis y recomendaciones que muestra son estimaciones calculadas a partir de los datos que el titular registra. No constituyen asesoría financiera, fiscal, contable ni de inversión, y no garantizan rendimientos ni ahorros.

3. Datos personales que trata Nexum
3.1 Identificación y cuenta: nombre visible, correo electrónico, hash Argon2id de la contraseña, estado de verificación del correo, estado de la cuenta, fecha de alta, versiones de avisos aceptadas, decisiones de consentimiento con su fecha y finalidad, progreso de configuración inicial, idioma y zona horaria.
3.2 Datos financieros declarados por el titular: ingreso base (importe, moneda y frecuencia), compromisos de pago fijos declarados al registrarse, movimientos (tipo, importe, moneda, categoría, descripción opcional y fecha), reglas de pago recurrente (concepto, importe, categoría, frecuencia, próxima fecha y estado), metas de ahorro (nombre, importe objetivo, ahorro inicial, fecha objetivo y estado), aportaciones a metas con su reparto y estado, y límites presupuestarios por categoría y periodo.
3.3 Preferencias y contenido: preferencias personales (idioma y zona horaria IANA), preferencias de notificación por canal y horarios de silencio, recordatorios (título, fecha de vencimiento, zona horaria y frecuencia), y la consulta de contenidos educativos y preguntas frecuentes, que no procesan datos personales.
3.4 Dispositivos y notificaciones: si el titular habilita las notificaciones push y el servicio está disponible, se registra un token de mensajería del dispositivo cifrado en reposo, una huella del token, la etiqueta del dispositivo, la plataforma y las fechas de alta, uso y baja. Nexum conserva también las notificaciones mostradas en la aplicación (título, mensaje, tipo, fecha y estado de leída).
3.5 Datos técnicos y de seguridad: dirección IP, marcas de tiempo, identificadores opacos de sesión, hash de los tokens de renovación, tokens de acceso de vida corta, datos del cliente cuando los envía (por ejemplo, versión de la app o plataforma), trazas de correlación y registros de errores sin contraseñas ni tokens.
3.6 Datos derivados: totales, resúmenes por periodo, distribución por categoría, patrones de gasto, rachas de ahorro y recomendaciones calculados por Nexum a partir de los datos del propio titular. No se usan para publicidad ni para elaborar perfiles comerciales.
3.7 Datos que Nexum NO recoge: credenciales bancarias, números de tarjeta, identificadores oficiales, datos biométricos, contactos, ubicación precisa, audio almacenado en servidores, contenido de otras aplicaciones ni datos de terceros.

4. Funciones cubiertas por este aviso
Este aviso se publica con la versión del servicio que habilita las funciones descritas. Si una función no está habilitada en tu versión de la aplicación o en el servicio, no se ejecuta ni recoge datos; el aviso se actualizará cuando se habilite.
4.1 Registro, inicio y cierre de sesión, renovación de tokens, cambio y recuperación de contraseña, verificación del correo electrónico, listado y revocación de sesiones propias.
4.2 Perfil, configuración inicial (onboarding), consentimientos, preferencias personales y consulta del aviso de privacidad vigente.
4.3 Ingreso base, movimientos, pagos recurrentes, metas de ahorro, aportaciones y presupuestos, incluidos sus cálculos, listados, edición y eliminación.
4.4 Tablero (dashboard), resumen financiero, analítica de gasto y recomendaciones. Las recomendaciones usan un modelo de agrupamiento K-Means que se ejecuta en la infraestructura de Nexum con los datos del propio titular; no se envían datos a proveedores externos de inteligencia artificial ni se usan modelos generativos de terceros.
4.5 Notificaciones dentro de la aplicación, recordatorios y registro de dispositivos para notificaciones push. El envío de push por Firebase Cloud Messaging (Google) solo se activa si el servicio y su salida de red están habilitados y aprobados; mientras no lo estén, Nexum no envía notificaciones push ni comunica el token a ese proveedor.
4.6 Comprobaciones de salud del servicio (vivacidad y preparación), carga inicial de los datos del titular para la app (bootstrap) y modo de demostración. Las comprobaciones de salud no procesan datos personales. El modo de demostración está desactivado por defecto, no inserta datos ni procesa información personal y se limita a informar un estado de configuración.
4.7 Interpretación de texto de voz. El reconocimiento de voz se realiza en el dispositivo Android; Nexum recibe únicamente el texto resultante, no el audio, y no lo persiste. Ninguna operación financiera se ejecuta sin confirmación explícita del titular.
4.8 Exportación de datos propios y eliminación de la cuenta, descritas en las secciones 10 y 11.

5. Finalidades del tratamiento
- Primarias: crear y autenticar la cuenta; mantener, renovar y revocar sesiones; verificar el correo; recuperar el acceso; prestar las funciones financieras solicitadas; calcular totales, resúmenes, presupuestos, analítica y recomendaciones sobre los datos del titular; aplicar preferencias y recordatorios; registrar consentimientos y configuración inicial; proteger la cuenta y prevenir abuso; atender solicitudes de derechos; cumplir obligaciones legales y de seguridad.
- Secundarias opcionales: notificaciones y recordatorios. Requieren la habilitación del titular y pueden revocarse en cualquier momento.
- Nexum no vende, alquila, cede ni intercambia datos personales con terceros comerciales, redes publicitarias, corredores de datos ni proveedores de inteligencia artificial externos. No muestra publicidad de terceros ni usa los datos financieros para publicidad.

6. Consentimiento y versiones
No se marcan consentimientos por omisión. Cada decisión se registra con la versión del aviso, la finalidad, la decisión y la fecha UTC. El titular puede consultar su historial y revocar consentimientos desde la aplicación. Una versión sustancialmente nueva del aviso requerirá aceptación; el historial conserva la versión aceptada.

7. Infraestructura y encargados del tratamiento
Nexum se aloja en Amazon Web Services (AWS), región us-east-2 (Ohio, Estados Unidos). Servicios utilizados: EC2 (cómputo), Application Load Balancer y CloudFront (entrega HTTPS), DynamoDB (base de datos), ElastiCache Redis (caché y control de cuotas), S3 (artefactos, secretos cifrados y estado de infraestructura), SES (correo transaccional de verificación y recuperación) y CloudWatch (registros y métricas). AWS actúa como encargado del tratamiento por instrucción de Nexum.
Si el envío de notificaciones push se habilita, Firebase Cloud Messaging de Google actuará como encargado únicamente para entregar el mensaje al dispositivo, con el token de mensajería y el contenido de la notificación. Nexum no utiliza Google Cloud Platform como plataforma de datos, MongoDB Atlas, redes publicitarias ni proveedores externos de inteligencia artificial. La transferencia internacional a Estados Unidos se realiza para las finalidades descritas y con las medidas contractuales y de seguridad disponibles.

8. Seguridad
Medidas verificables en el código y la configuración: comunicaciones HTTPS; contraseñas con Argon2id; JWT RS256 de vida corta; rotación de tokens de renovación con detección de reutilización; revocación de sesiones y época de revocación; permisos IAM acotados; secretos fuera del código; cifrado en reposo según la configuración de los servicios de AWS; y registros que omiten contraseñas, tokens y datos financieros innecesarios. Ningún sistema es infalible. Si se detecta un incidente que afecte datos personales, se comunicará por los canales disponibles y, cuando corresponda, a las autoridades.

9. Conservación
Los datos de la cuenta se conservan mientras esté activa y sean necesarios para las finalidades descritas y las obligaciones aplicables. No se publica un plazo único de conservación porque depende del tipo de dato y del servicio. Los registros técnicos de vida corta (ventanas de idempotencia, tokens de un solo uso, códigos de verificación y recuperación, notificaciones y recordatorios ya atendidos) tienen plazos limitados y dejan de ser válidos al vencer; su eliminación física por el proveedor de nube es asíncrona y se realiza de mejor esfuerzo. No se mantiene un historial comercial ni se elaboran perfiles publicitarios. La configuración de recuperación de la base de datos y los registros operativos se define en la operación de AWS y no se usan para operación normal.

10. Eliminación de la cuenta y de los datos
La eliminación de la cuenta se solicita desde la aplicación con confirmación expresa y verificación de la contraseña. Al aceptarse, la cuenta queda bloqueada de inmediato, se revocan las sesiones activas y los datos dejan de ser accesibles. La supresión de los datos asociados se ejecuta por lotes mediante un proceso interno con estado verificable y reanudable; la solicitud no se considera completada hasta que ese proceso confirma la eliminación de los datos que debe suprimir. Se conserva un registro mínimo de la solicitud, sin datos de contacto ni financieros, por un plazo limitado y con fines de trazabilidad. La solicitud también puede realizarse por correo a nexumproyectomov@gmail.com.
Los movimientos, metas, aportaciones, presupuestos, recordatorios y dispositivos que el titular elimina dejan de aparecer en sus listados, totales, análisis y exportación. Nexum puede conservar un registro técnico mínimo de la operación para integridad y auditoría, sin el texto libre de descripción, que se suprime con la eliminación de la cuenta o al vencer su plazo. Este aviso no promete un plazo máximo de eliminación física distinto del que la infraestructura garantiza.

11. Derechos de los titulares
El titular puede ejercer los derechos de acceso, rectificación, cancelación, oposición y portabilidad sobre sus datos. Desde la aplicación puede consultar y corregir su perfil, ingreso, movimientos, metas, aportaciones, presupuestos y preferencias; consultar su historial de consentimientos; exportar sus datos; y solicitar la eliminación de su cuenta. Para el resto de solicitudes puede escribir a nexumproyectomov@gmail.com. El responsable responderá por ese canal y podrá solicitar información adicional para verificar la identidad antes de ejecutar cambios sensibles.

12. Permisos del dispositivo
El micrófono y las notificaciones son opcionales. Si se niega un permiso, la función correspondiente no se habilita y las demás funciones siguen operativas. El reconocimiento de voz se realiza en el dispositivo y el backend no almacena audio. Las notificaciones push requieren el permiso del sistema operativo y pueden desactivarse en cualquier momento.

13. Límites y advertencias
- Una notificación push ya entregada al dispositivo no puede recuperarse ni borrarse de forma remota por Nexum; desactivar las notificaciones impide envíos futuros, pero el sistema operativo puede conservar las ya recibidas.
- Pueden existir copias de respaldo administradas por AWS con ventanas de retención limitadas. Una copia puede conservar datos suprimidos hasta que expire su ventana, y una restauración podría reintroducirlos temporalmente; el proceso de supresión se repetiría sobre ellos.
- La eliminación física por TTL es de mejor esfuerzo y puede completarse después del vencimiento lógico.
- Los resúmenes, análisis y recomendaciones son estimaciones sobre los datos declarados; no son asesoría profesional ni garantía de resultados.
- La información se aloja en Estados Unidos; el titular puede oponerse a la transferencia solicitando la eliminación de su cuenta, sin perjuicio de las obligaciones legales aplicables.

14. Ausencia de publicidad y venta de datos
Nexum no vende, alquila, cede ni comparte datos personales con terceros para publicidad, perfilado comercial, corredores de datos ni entrenamiento de modelos de terceros. No se usan redes publicitarias ni se muestra publicidad de terceros en la aplicación.

15. Versión y cambios
Este aviso tiene un identificador de versión y una fecha de entrada en vigor. La versión vigente y su contenido íntegro están disponibles en este endpoint. Si se cambia el aviso de forma sustancial, se publicará una nueva versión y se solicitará la aceptación. El historial de consentimientos conserva la versión aceptada.

16. Menores de edad
Nexum no está dirigido a menores de 18 años y no solicita ni pretende recoger sus datos. Si el responsable detecta una cuenta de un menor sin autorización, la eliminará conforme al procedimiento de la sección 10.

17. Contacto
Para dudas, solicitudes o quejas relacionadas con los datos personales, el canal de privacidad es nexumproyectomov@gmail.com. El responsable responderá por ese canal.
```

## 3. Correspondencia función → mención

| Función (endpoint) | Dónde se menciona | Qué se dice | Estado |
|---|---|---|---|
| E001 registro, E002 login, E003 refresh, E004 logout, E005/E006 recuperación, E007 cambio, E008/E009 sesiones, E010/E011 verificación | §3.1, §3.5, §4.1, §8 | Identificación, hash Argon2id, sesiones, tokens, revocación, verificación | Disponible |
| E012/E013 perfil, E018/E019 onboarding, E016/E017 consentimientos, E068/E069 preferencias, E015 aviso | §3.1, §3.3, §4.2, §6 | Datos de perfil, progreso, consentimientos con versión, preferencias | Disponible |
| E024/E025 ingreso base | §3.2, §4.3 | Importe, moneda, frecuencia | Disponible |
| E027–E031 movimientos | §3.2, §4.3, §10, §11 | Tipo, importe, categoría, descripción opcional, fecha; edición y borrado | Disponible |
| E032–E037 pagos recurrentes y registro de pago | §3.2, §4.3 | Concepto, importe, frecuencia, próxima fecha, estado; pago derivado | Disponible |
| E038–E042 metas | §3.2, §4.3, §11 | Objetivo, ahorro inicial, fecha, estado; tombstone de meta | Disponible |
| E043–E047 aportaciones | §3.2, §4.3, §11 | Reparto entre metas, estado, reversión | Disponible |
| E048–E050 presupuestos y uso | §3.2, §4.3 | Límite por categoría/periodo y consumo | Disponible |
| E051 dashboard | §4.4 | Agregados del tablero | Disponible |
| E026 resumen financiero | §3.6, §4.4 | Ingresos, gastos, ahorro y neto calculados | Disponible |
| E052–E056 analítica | §3.6, §4.4 | Insights, categorías, patrones, racha | Disponible (Oleada 4 en regularización; contrato aprobado) |
| E055 ML K-Means | §3.6, §4.4, §7 | Modelo local con datos del titular; **sin proveedores externos de IA**; no es asesoría | Disponible cuando se integre; el diseño es local |
| E057/E058 preferencias de notificación | §3.3, §4.5 | Canales, opt-in y horarios de silencio | Disponible |
| E059/E060 dispositivos push | §3.4, §4.5, §7, §13 | Token cifrado, huella, etiqueta; alta y baja | Registro disponible; **envío FCM condicionado** |
| E061–E063 notificaciones | §3.4, §4.5, §13 | Título, mensaje, tipo, leída; push ya entregado no se recupera | Disponible en la app |
| E064–E067 recordatorios | §3.3, §4.5 | Título, vencimiento, zona, frecuencia | Disponible |
| E072/E073 salud | §4.6 | Vivacidad y preparación; sin datos personales | Disponible |
| E021 bootstrap | §4.6 | Carga inicial de perfil, resumen y versión de catálogo del titular | Disponible |
| E074 demostración | §4.6 | Desactivada por defecto; no inserta datos; solo informa configuración | Desactivada por defecto |
| E075 voz | §4.7, §12 | Texto en el dispositivo; sin audio ni persistencia; confirmación obligatoria | Disponible cuando se integre |
| E020 exportación | §4.8, §11 | Datos propios en JSON; sin datos de otros usuarios | Disponible |
| E014 eliminación | §4.8, §10, §11 | Bloqueo inmediato, revocación, purga verificable, recibo mínimo | Disponible |
| AWS (EC2, ALB, CloudFront, DynamoDB, Redis, S3, CloudWatch) | §7 | Infraestructura y encargado | Disponible |
| SES | §7 | Correo transaccional de verificación y recuperación | Disponible |
| Firebase Cloud Messaging (Google) | §4.5, §7 | Encargado **solo si** el egreso y el servicio se habilitan | Condicional explícito |
| Retención, copias y TTL | §9, §13 | Plazos por tipo; respaldo limitado; TTL best-effort | Límite declarado |
| Derechos ARCO y portabilidad | §11 | In-app y correo; verificación de identidad | Disponible |
| Publicidad y venta | §5, §14 | Prohibidas; sin redes ni corredores | Declaración |
| Menores | §16 | No dirigido a menores de 18 | Declaración |
| Transferencia internacional | §7, §13 | AWS us-east-2; derecho de oposición con la eliminación | Declaración |

### 3.1 Lo que el aviso NO promete (verificación negativa)

| No prometido | Comprobación |
|---|---|
| Conexión bancaria, tarjetas o claves interbancarias | §2 y §3.7 lo excluyen |
| Asesoría financiera, fiscal o de inversión | §2, §4.4 y §13 lo niegan |
| Venta, alquiler o cesión a anunciantes | §5 y §14 lo prohíben |
| Modelos generativos o IA de terceros | §4.4, §7 y §14: el modelo es local |
| Almacenamiento de audio | §4.7, §3.7 y §12 |
| Eliminación física inmediata o plazo único | §9, §10 y §13 |
| Recuperación remota de un push ya entregado | §13 |
| Cifrado o controles no verificados | §8 remite a la configuración de AWS y no promete más |
| Publicación del aviso | Estado `PROPUESTA`; este documento no lo publica |

## 4. Requisitos para publicarlo (ninguno ejecutado aquí)

1. **Aprobación humana** de esta versión con la frase del §5, que incluye el SHA-256 exacto.
2. **Código** (fuera del alcance de esta unidad; propiedad del orquestador):
   - añadir `internal/application/privacy/aviso_2026_11_v3.txt` con el artefacto del §2;
   - cambiar `//go:embed` y `VersionAviso` a `2026-11-v3`;
   - actualizar `TextoResumen`, que hoy afirma que movimientos, presupuestos, metas,
     exportación, eliminación, notificaciones y análisis **no** están disponibles (sería falso);
   - actualizar la prueba de hash (`notice_test.go`) al valor aprobado;
   - mantener `RequiresAcceptance = true` y `URLAnuncio`.
3. **Bundle RFC-004:** fijar un `E015_EFFECTIVE_AT` nuevo (RFC3339, UTC, no anterior a
   `2026-11-18T00:00:00Z`, coherente con la fecha de aprobación). Sin esa variable el endpoint
   responde 503 por diseño.
4. **Consentimientos:** no se marcan por omisión ni se migran decisiones. La nueva versión obliga
   a aceptar; el historial conserva la versión anterior. **No se sustituyen consentimientos.**
5. **Pruebas:** hash exacto, versión, `effectiveAt` válido, `requiresAcceptance=true`, contenido
   sin datos personales, y una comprobación de que las funciones declaradas como disponibles
   existen en el catálogo (evitar otra regresión semántica).
6. **Publicación:** release y despliegue con sus Gates propios; este documento no autoriza
   ninguno.

## 5. Frase propuesta (no concedida)

```
APRUEBO_AVISO_PRIVACIDAD_E015_2026_11_V3_SHA256_1CEC4609FF2F3FC052D3DEB29271BA8AAED322D1EC97AE9757E222CC8C0563B9
```

Al concederse autorizaría: adoptar este texto como aviso `2026-11-v3`, incrustarlo en el binario,
publicar la versión con su `effectiveAt` y exigir aceptación. **No** autoriza por sí sola release,
despliegue ni modificación de otros endpoints.

## 6. Riesgos y límites de esta propuesta

| ID | Riesgo | Mitigación |
|---|---|---|
| R-AV-01 | Declarar disponible algo que aún no está integrado (Oleadas 4/5) | El aviso se publica **después** del release consolidado; si se publica antes, deben retirarse las funciones no integradas o el Gate debe declararlo |
| R-AV-02 | Prometer cifrado o retención no verificados | §8 y §9 remiten a la configuración de AWS y no fijan plazos no medidos |
| R-AV-03 | El texto quede obsoleto al cambiar una función | Prueba de correspondencia función→catálogo y revisión en cada release que añada datos |
| R-AV-04 | FCM se habilite sin actualizar el aviso | La cláusula condicional ya lo cubre; el Gate de egreso FCM debe verificar esta mención |
| R-AV-05 | El hash del `.txt` no coincide por BOM o CRLF | El artefacto se guarda UTF-8 sin BOM y LF; el hash se recalcula antes de aprobar |
| R-AV-06 | Se interprete como aprobación legal | Estado `PROPUESTA`; la frase y la revisión humana son obligatorias |

## 7. Trazabilidad

- `sdd/03-endpoints/E015-privacy-notices-current/{spec,plan,tasks,checklist}.md`
- `sdd/_control/H08/MATRIZ_AUDITORIA_54_ENDPOINTS.md` (E015 `NO_CONFORME_SEMANTICA`)
- `sdd/_control/H08/POLITICA_PRIVACIDAD_BACKUPS.md`, `POLITICA_BACKUPS.md`, `MATRIZ_RETENCION.md`
- `sdd/_control/H08/ML_E055_CONTRATO_Y_DISENO.md` (modelo local, sin PII en features)
- `sdd/_control/H08/RFC-016-CONTRATOS_E005_E075.md` §17.6 (preferencias y `quietHours`)
- `sdd/_control/H08/GATE_H08E_OLEADA_3.md` §12.1 (release consolidada)
- `sdd/_control/H08/GATE_H08E_REGULARIZACION_OLEADAS_4_5.md` §5 (la aprobación legal de E015 queda excluida)
- `internal/application/privacy/notice.go`, `aviso_2026_11_v2.txt`
- `dist/deuda/aviso_2026_11_v3.txt` (artefacto canónico)

---

## Reconciliación de trazabilidad H09-1 (2026-11-27)

Verificación de solo lectura. **No** aprueba, publica ni modifica el texto legal.

| Artefacto | Bytes | SHA-256 | Estado |
|---|---:|---|---|
| `internal/application/privacy/aviso_2026_11_v3.txt` | 8 123 | `E925596ADFF1B3EBB9A17064D44F5F6B84BFBDFB874396CD51EFC6C0AC3F8D83` | Único preservado en Git; borrador **superado** por esta propuesta; no publicable |
| `dist/deuda/aviso_2026_11_v3.txt` | 13 662 (declarado) | `1CEC4609FF2F3FC052D3DEB29271BA8AAED322D1EC97AE9757E222CC8C0563B9` (declarado) | **NO EXISTE** en el repositorio ni en `dist/`; no materializado |

- La búsqueda de `1CEC4609…` en el árbol no devuelve ningún archivo: es un **hash declarado sin artefacto**.
- La afirmación de este documento de que sustituye al borrador de 8 123 bytes es correcta; por tanto **no** debe reutilizarse `E925…` como si fuera la v3 propuesta.
- Consecuencia: la frase de aprobación propuesta **no puede ejercerse** hasta que el texto de 13 662 bytes se materialice, se versione y su SHA-256 se recalcule desde el archivo real. Cualquier Gate legal previo a eso sería inválido.
- `memoria.md` conserva `E925…` porque documenta el borrador histórico; no se altera esa referencia.
