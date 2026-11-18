# Gate H08-E — OLEADA 1 (15 endpoints)

| Campo | Valor |
|---|---|
| Documento | `sdd/_control/H08/GATE_H08E_OLEADA_1.md` |
| Versión | 1.2.2 |
| Fecha | 2026-11-18 |
| Base de trabajo | `main` `88ef920f74da375b588f3e850a760aa0530df050` (PR #80 fusionado) |
| Baseline funcional vigente | `v0.15.0+8f13c86d2ca0` · E001–E011 verificados |
| Instrumento de gobierno | `RFC-019-OLEADA_E007_E011.md` (excepción reutilizable E-1/E-2) y `RFC-020-FRECUENCIAS_E023_E024_E025.md` v1.1.0 (propuesta incluida en este Gate) |
| Estado | **PROPUESTO — pendiente de la frase humana de §15** |
| Aprobador | Responsable de Nexum-Backend |

Este Gate **no se autoaprueba**. Su presencia en el repositorio no autoriza ninguna mutación. La
aprobación se concede únicamente con la frase exacta de §15 y cubre el alcance descrito aquí, no
las operaciones reservadas de §3.

## 1. Alcance cerrado: 15 endpoints

Se implementan, integran y verifican **exclusivamente** las siguientes operaciones del OpenAPI
vigente (`sdd/03-endpoints/openapi/nexum-api-v1.{yaml,json}`). Los códigos indicados son el éxito
contractual; los errores documentados se respetan sin inventar respuestas.

| ID | Método y ruta | Éxito | Seguridad | If-Match |
|---|---|---|---|---|
| E012 | `GET /api/v1/users/me` | 200 | Bearer + sesión | no |
| E013 | `PATCH /api/v1/users/me` | 200 | Bearer + sesión | **obligatorio** |
| E015 | `GET /api/v1/privacy/notices/current` | 200 | público | no |
| E016 | `GET /api/v1/users/me/consents` | 200 | Bearer + sesión | no |
| E017 | `POST /api/v1/users/me/consents` | 201 | Bearer + sesión | no |
| E018 | `GET /api/v1/users/me/onboarding` | 200 | Bearer + sesión | no |
| E019 | `PATCH /api/v1/users/me/onboarding` | 200 | Bearer + sesión | no (CAS interno, 409) |
| E022 | `GET /api/v1/catalogs/categories` | 200 | público | no |
| E023 | `GET /api/v1/catalogs/frequencies` | 200 | público | no |
| E024 | `GET /api/v1/income` | 200 | Bearer + sesión | no |
| E025 | `PUT /api/v1/income` | 200 | Bearer + sesión | **obligatorio** |
| E068 | `GET /api/v1/users/me/preferences` | 200 | Bearer + sesión | no |
| E069 | `PATCH /api/v1/users/me/preferences` | 200 | Bearer + sesión | **obligatorio** |
| E070 | `GET /api/v1/help/faq` | 200 | público | no |
| E071 | `GET /api/v1/education/tips` | 200 | público | no |

`operationId`: `nexum_e012`, `nexum_e013`, `nexum_e015`, `nexum_e016`, `nexum_e017`,
`nexum_e018`, `nexum_e019`, `nexum_e022`, `nexum_e023`, `nexum_e024`, `nexum_e025`,
`nexum_e068`, `nexum_e069`, `nexum_e070`, `nexum_e071`.

### 1.1 Fuera de alcance

- E001–E011: no se modifica código, contrato, persistencia ni configuración.
- E014, E020, E021, E026–E067, E072–E075: siguen `NOT_AUTHORIZED`.
- Oleada 2 y posteriores: **prohibido iniciarlas** sin autorización humana posterior.

## 2. Lo que este Gate autoriza

1. Crear un PR de andamiaje documental/técnico (`chore/h08e-oleada1-scaffold`) con:
   - el registro de montaje de la oleada (`cmd/server/registro_oleada1.go`) y su llamada única en
     `cmd/server/main.go`;
   - helpers HTTP compartidos (ETag/If-Match, límite de tasa, validación de locale/zona IANA);
   - `time/tzdata` embebido para validar zonas IANA en cualquier entorno;
   - este Gate, `RFC-020` y `DISENO_OLEADA_1.md`.
2. Desarrollar los 15 endpoints en **tres líneas aisladas** con worktrees y ramas propias (§4).
3. Ejecutar localmente `gofmt`, `go build`, `go vet`, `go test ./... -count=1`, `go test -race`,
   `govulncheck`, `gosec`, Semgrep y las pruebas de contrato/SDD que ya existen.
4. Abrir **un PR por endpoint** (15 PR) más el PR de andamiaje, con CI completo.
5. Construir una rama de ensayo de integración de los 15 cambios antes del primer merge.
6. Actualizar `memoria.md`, el Gate de despliegue, `tasks.md`/`checklist.md` de cada spec y el
   manifiesto `BASELINE_SHA256.json` (escritor único: el orquestador).
7. Publicar en AWS **una sola release agrupada**, sólo después de los Gates de §3.

## 3. Lo que este Gate NO autoriza

- **Merge**: cada PR exige `APRUEBO_MERGE_PR_<N>_SHA_<SHA40>` sobre su HEAD exacto.
- **Terraform plan**: exige permiso de herramienta y Gate propio (el plan puede leerse, no aplicarse).
- **Terraform apply/destroy/import/state**: denegados; el apply exige
  `APRUEBO_APPLY_<ALCANCE>_SHA256_<HASH>` sobre el `.tfplan` exacto.
- **Publicación S3**, **instance refresh**, cambios IAM, ASG, CloudFront, DynamoDB, Redis o SES:
  exigen el Gate de despliegue de la oleada y frase específica.
- Crear recursos AWS nuevos, tablas, índices, buckets, roles o endpoints de red.
- Cambiar `constitution.md`, reglas `deny` de `opencode.json`, workflows de CI o E001–E011.
- Declarar `ANDROID_ACCEPTED`: la aceptación móvil es independiente y no se solicita aquí.
- Modificar `sdd/**/spec.md` o `sdd/**/plan.md` salvo RFC aprobada (ninguna lo requiere hoy:
  los cambios de contrato van por OpenAPI + RFC-020; las specs ya contienen las enmiendas A1–A8).

## 4. Organización concurrente (RFC-019 E-1)

| Línea | Worktree | Ramas por endpoint | Endpoints |
|---|---|---|---|
| A | `.worktrees/oleada1-perfil` | `feat/h08e-e012-users-me`, `feat/h08e-e013-users-me`, `feat/h08e-e015-privacy-notice`, `feat/h08e-e016-consents`, `feat/h08e-e017-consents`, `feat/h08e-e018-onboarding`, `feat/h08e-e019-onboarding` | E012, E013, E015, E016, E017, E018, E019 |
| B | `.worktrees/oleada1-catalogos` | `feat/h08e-e022-categories`, `feat/h08e-e023-frequencies`, `feat/h08e-e024-income`, `feat/h08e-e025-income` | E022, E023, E024, E025 |
| C | `.worktrees/oleada1-preferencias` | `feat/h08e-e068-preferences`, `feat/h08e-e069-preferences`, `feat/h08e-e070-faq`, `feat/h08e-e071-tips` | E068, E069, E070, E071 |

Reglas de convivencia:

- **Un escritor por worktree**. Nunca dos agentes editan el mismo árbol.
- El **orquestador** es el único escritor de: `cmd/server/main.go`,
  `cmd/server/registro_oleada1.go`, `internal/transport/http/router.go`, OpenAPI, Terraform,
  `memoria.md`, `BASELINE_SHA256.json` y documentos de control globales.
- Cada endpoint entrega su `cmd/server/eNNN.go` con un `init()` que registra su montaje en el
  registro común (sin I/O ni lectura de entorno). El registro detecta IDs duplicados y monta en
  orden determinista por ID; un test comprueba que no hay rutas duplicadas y que el conjunto final
  es exactamente el autorizado. Así el PR queda **conectado** al fusionarse sin tocar `main.go`.
- Los paquetes de dominio/aplicación/adaptadores/pruebas de cada endpoint son propiedad de su
  línea. Nombres de archivo distintos evitan conflictos de merge.
- **Inventario de PR: 17** = 1 documental + 1 de andamiaje + 15 de endpoint. No se abre un PR
  adicional para corregir CI; cada fallo se resuelve dentro de su PR.
- Correcciones de CI/DevSecOps sólo si son indispensables para los endpoints autorizados; nunca
  para rebajar controles, desactivar escáneres o silenciar hallazgos.
- Ninguna aprobación es implícita: cada merge exige su frase por SHA40 y cada operación AWS su Gate.
- Máximo 8 endpoints en desarrollo/CI activo; el resto permanece en cola dentro de su línea.
- **E015** se sirve desde el backend (sin web ni repositorio externo). Su contenido íntegro tiene
  SHA-256 propio y requiere aprobación separada; si no se aprueba, el PR de E015 no se fusiona y
  el contador máximo es 25/75. Los otros 14 endpoints no se bloquean.

## 5. Preflight verificado (2026-11-18, solo lectura)

| Comprobación | Resultado |
|---|---|
| `origin/main` | `88ef920f74da375b588f3e850a760aa0530df050` |
| PR #80 / PR #79 | `MERGED` (merge `88ef920` / `8f13c86`) |
| CI de `main` en `88ef920` | Go Backend CI run `44232671725` **success** · SDD `44232671722` **success** |
| PR abiertos | 0 |
| Cuenta/región AWS | `713797602090` · `us-east-2` · `assumed-role/AccountFullAccessRole` |
| ASG `nexum-h08i-asg-backend` | Min 2 / Desired 2 / Max 2 · LT `$Latest` = **v10** · `HealthCheckType=EC2` |
| Instancias | `i-0665404e779ef7902` e `i-0a5dd963809ed3c2c`, `InService`/`Healthy`, LT v10, ambas en `us-east-2a` (limitación declarada) |
| Target group | 2 targets `healthy` en 8080 |
| HTTPS | `GET /api/v1/health/live` → 200 `{"status":"UP"}` |
| Go local | `go1.27.4` · `go build ./...` OK · `go test ./... -count=1` **PASS** (14 paquetes) · `gofmt -l` limpo |
| `opencode.json` local | 192 reglas de permiso / `policies` con deny de `terraform destroy/import/state`, `git push main`, `git reset --hard`, `docker`, `wsl`; `terraform apply *.tfplan` en `ask`; configuración local **no versionada** (84 líneas de diferencia con `HEAD`) |

## 6. Decisiones de diseño vinculantes

### D-E012 — Perfil propio
`GetItem` de `USER#<sub>/PROFILE` con `ConsistentRead`. La identidad sale sólo de `sub`/`sid`
verificados. Se serializa un DTO cerrado con `id`, `email`, `displayName`, `emailVerified`,
`createdAt`, `version`; nunca `passwordHash`, `sessionEpoch`, `status` ni atributos internos.

### D-E013 — Modificación de perfil
`If-Match` obligatorio con ETag **fuerte** `"v<version>"`. Un validador débil `W/"vN"` se rechaza
con **412** (RFC 9110 exige comparación fuerte para If-Match). Ausente → **428**;
formato inválido → **400**; versión distinta de la persistida → **412**. Sólo `displayName`
(1–100 runes, sin recortar el valor salvo espacios de borde), `unknown fields` → **400**.
`UpdateItem` condicional `attribute_exists(PK) AND #version = :expected` con
`SET displayName, updatedAt ADD #version :one`. La respuesta es el perfil actualizado con la
versión incrementada y ETag `"v<version>"`. Perfil ausente → **404**. Conflicto de transacción
agotados los reintentos → **409**. `{}` sin cambios → 200 sin escritura (misma versión).

### D-E015 — Aviso de privacidad servido por la API

**Decisión de arquitectura:** Nexum **no tendrá aplicación web ni portal de privacidad**. La única
web es la landing publicitaria, ajena a este backend. E015 se sirve desde el propio backend Go en
`GET /api/v1/privacy/notices/current`; Android muestra el aviso dentro de su interfaz y registra la
versión aceptada mediante E017. El repositorio `nexumpr/nexum-privacidad` pertenece a otro
integrante: **no se modifica, no se usa como dependencia y no bloquea esta oleada**.

**Respuesta contractual:**

| Campo | Valor |
|---|---|
| `version` | `2026-11-v2` |
| `effectiveAt` | `2026-11-20T00:00:00Z` (fecha propuesta de entrada en vigor; se valida en el Gate de release y no se calcula por petición) |
| `text` | resumen legible (abajo) |
| `url` | `https://nexumpr.github.io/nexum-privacidad/` |
| `content` | texto íntegro versionado (abajo) |
| `requiresAcceptance` | `true` |

**Datos del responsable (autorizados por el responsable del proyecto):** Leonardo Isaac Barrera
Tejeda, Carretera Tula Tepeji 7, Localidad El Sesenta y Dos, C.P. 42836, Tula de Allende, Hidalgo,
México; canal de privacidad y derechos ARCO: `nexumproyectomov@gmail.com`.

**Página pública de privacidad:** `https://nexumpr.github.io/nexum-privacidad/` pertenece a otro
integrante y se usa como URL pública para Google Play. Se verificó que está publicada, pero su
contenido puede citar infraestructura o plazos obsoletos (GCP/MongoDB, 30 días). **No se modifica
ese repositorio ni se presenta su contenido como aprobado**: la fuente contractual es el `content`
versionado de E015. La coordinación con el integrante responsable queda registrada como dependencia
externa, sin bloquear la disponibilidad del API.

**Texto de `text` (resumen):**
> Nexum trata tu nombre, correo y contraseña (hash Argon2id) para autenticarte; tu ingreso base y
> compromisos declarados para calcular su equivalente mensual; y datos técnicos de sesión para
> proteger tu cuenta. La infraestructura es AWS `us-east-2`; no vendemos tus datos ni los cedemos a
> redes publicitarias. Algunas funciones (movimientos, presupuestos, metas, exportación, eliminación
> automática, notificaciones y análisis) aún no están disponibles; este aviso se actualizará cuando
> lo estén. El aviso íntegro, con finalidades, derechos, conservación y versiones, está en el campo
> `content` y en la URL de este endpoint.

**Texto íntegro de `content` (aprobación separada):** el bloque siguiente es el contenido exacto
que se incorporará como recurso versionado del backend. Se medirá en UTF-8 con salto de línea
final; **6724 bytes**; SHA-256
**`ddfb0b1865e0e77a47f74c025e36bdbc102c6d424fc137c703f4316d69b66380`**. Una prueba de contrato
recalculará el SHA y fallará si el texto cambia.

```text
Aviso de Privacidad de Nexum

Versión: 2026-11-v2. La fecha de entrada en vigor se publica en el campo effectiveAt de este endpoint.

1. Responsable del tratamiento
El responsable del tratamiento es Leonardo Isaac Barrera Tejeda, con domicilio en Carretera Tula Tepeji 7, Localidad El Sesenta y Dos, C.P. 42836, Tula de Allende, Hidalgo, México. Para asuntos de privacidad y derechos ARCO puede contactarse el correo nexumproyectomov@gmail.com.

Nexum es una aplicación Android de gestión de finanzas personales. No es un banco, entidad financiera ni sociedad fintech; no se conecta a cuentas bancarias ni solicita claves interbancarias, números de tarjeta o contraseñas bancarias. Los importes y proyecciones que muestra son estimaciones calculadas a partir de los datos que el usuario declara.

2. Datos que trata Nexum
2.1 Funciones disponibles actualmente (registro, autenticación, sesión, perfil, consentimientos, configuración inicial, ingreso base, preferencias y contenido de ayuda):
- Identificación y cuenta: nombre visible, correo electrónico, hash Argon2id de la contraseña, estado de verificación del correo, estado de la cuenta, versiones de avisos aceptadas, preferencias de idioma y zona horaria, y progreso de configuración inicial.
- Ingreso base: importe, moneda y frecuencia.
- Compromisos de pago recurrentes declarados en el registro: concepto, importe, categoría y frecuencia.
- Datos técnicos de sesión: dirección IP, marcas de tiempo, identificadores opacos de sesión, tokens de acceso y renovación, y datos opcionales del dispositivo cuando el cliente los envía.
2.2 Funciones previstas que aún no están disponibles: movimientos, presupuestos, metas de ahorro y aportaciones, exportación de datos, eliminación automática de cuenta, notificaciones push, análisis de patrones y registro por voz. Cuando se habiliten, este aviso se actualizará y se solicitará la aceptación correspondiente. Nexum no recoge hoy esos datos a través de la app y no se describen como si ya existieran.
- No se recogen datos bancarios, números de tarjeta, identificadores oficiales, datos biométricos, contactos, ubicación precisa ni audio almacenado en servidores.

3. Finalidades
- Primarias: crear y autenticar la cuenta; mantener la sesión y permitir su cierre y revocación; calcular y mostrar el ingreso base y su equivalente mensual; registrar consentimientos y configuración inicial; aplicar preferencias; proteger la cuenta y prevenir abuso; atender solicitudes de derechos; cumplir obligaciones legales.
- Secundarias opcionales: notificaciones y análisis. Aún no están disponibles; cuando se habiliten, requerirán consentimiento y podrán revocarse.
- No se venden, alquilan ni ceden datos a terceros comerciales ni se usan para publicidad de terceros.

4. Consentimiento y versiones
No se marcan consentimientos por omisión. Cada decisión se registra con la versión del aviso, la finalidad, la decisión y la fecha UTC. El usuario puede consultar el historial y revocar consentimientos desde la app. Una versión sustancialmente nueva requerirá aceptación.

5. Infraestructura y proveedores
Nexum se aloja en Amazon Web Services (AWS), región us-east-2 (Ohio, Estados Unidos). Servicios utilizados: EC2 (cómputo), Application Load Balancer y CloudFront (entrega HTTPS), DynamoDB (base de datos), ElastiCache Redis (caché y control de cuotas), S3 (artefactos y estado de infraestructura), SES (correos de verificación y recuperación) y CloudWatch (registros y métricas). AWS actúa como encargado del tratamiento por instrucción de Nexum. No se utiliza Google Cloud Platform ni MongoDB Atlas. No se transfieren datos a redes publicitarias.

6. Seguridad
Medidas verificables en el código y la configuración: comunicaciones HTTPS; contraseñas con Argon2id; JWT RS256 de vida corta; rotación de tokens de renovación con detección de reutilización; revocación de sesiones; permisos IAM acotados; y registros que omiten contraseñas y tokens. La configuración de cifrado en reposo y de copias de seguridad depende de los servicios de AWS y se revisa en la operación; este aviso no promete controles que no estén verificados. Ningún sistema es infalible; si se detecta un incidente que afecte los datos, se comunicará por los canales disponibles y, cuando corresponda, a las autoridades.

7. Conservación y eliminación
Los datos de la cuenta se conservan mientras esté activa y sean necesarios para las finalidades descritas y las obligaciones aplicables. No se publica un plazo único de conservación porque depende del tipo de dato y del servicio. Las copias técnicas de recuperación y los registros operativos se rigen por la configuración de AWS y los procedimientos de operación; no se usan para operación normal ni para elaborar perfiles.

La eliminación automática de la cuenta desde la app aún no está disponible. Para solicitar la eliminación de la cuenta y de los datos asociados, el titular puede escribir a nexumproyectomov@gmail.com. La solicitud se registra, se verifica la identidad del titular, se revocan las sesiones activas y se ejecuta el procedimiento de supresión aplicable. Los datos no se consideran eliminados hasta que el responsable confirme la ejecución; este aviso no promete purgas automáticas que no estén verificadas.

8. Derechos de los titulares
El titular puede ejercer los derechos de acceso, rectificación, cancelación, oposición y portabilidad sobre sus datos. Desde la app puede consultar y corregir su perfil, ingreso y preferencias, y consultar su historial de consentimientos. Para el resto de solicitudes puede escribir a nexumproyectomov@gmail.com. El responsable responderá por ese canal y podrá solicitar información adicional para verificar la identidad antes de ejecutar cambios sensibles.

9. Permisos del dispositivo
El micrófono y las notificaciones son opcionales. El registro por voz aún no está disponible; cuando se habilite, el reconocimiento se realizará en el dispositivo y el backend no almacenará audio. Si se niega un permiso, la función correspondiente no se habilita; las demás funciones disponibles siguen operativas.

10. Versión y cambios
Este aviso tiene un identificador de versión y una fecha de entrada en vigor. La versión vigente y su contenido íntegro están disponibles en este endpoint. Si se cambia el aviso de forma sustancial, se publicará una nueva versión y se solicitará la aceptación. El historial de consentimientos conserva la versión aceptada.

11. Contacto
Para dudas, solicitudes o quejas relacionadas con los datos personales, el canal de privacidad es nexumproyectomov@gmail.com. El responsable responderá por ese canal.
```

**Hechos deliberadamente no afirmados y pendientes:**

- No se citan GCP ni MongoDB Atlas.
- No se afirma TLS 1.3 (riesgo H-01 sigue abierto y se registra aparte); sólo se declara HTTPS.
- No se afirma que la eliminación automática de cuenta ya exista: E014 no está implementado y el
  texto lo declara como función prevista.
- No se afirma que la sección Ayuda permita contactar al responsable: el canal es el correo de
  privacidad autorizado, no la FAQ.
- No se afirman plazos concretos de conservación, purgas automáticas ni cifrado de todos los
  servicios. Se describen únicamente medidas verificables en código/configuración y se remite la
  configuración de cifrado y copias a la operación de AWS.
- **Datos del responsable autorizados:** nombre, domicilio y correo de privacidad constan en el
  aviso; no son datos inventados.
- **PENDIENTE_DECISION_HUMANA:** aprobación expresa de este texto (SHA-256) y de la fecha
  `2026-11-20T00:00:00Z`. Hasta esa frase, el PR #91 no se fusiona.
- **effectiveAt:** constante inmutable por versión `2026-11-20T00:00:00Z`; no se calcula con
  `time.Now()` por petición ni se fija retroactivamente. El Gate de release la valida antes de
  promover.

**Aprobación de contenido:** el desarrollo de E015 (handler, proveedor, DTO y pruebas) queda
autorizado con el Gate de §15, pero **la adopción del texto** requiere la frase separada:

```
APRUEBO_CONTENIDO_E015_VERSION_2026_11_V2_SHA256_ddfb0b1865e0e77a47f74c025e36bdbc102c6d424fc137c703f4316d69b66380
```

Si el responsable no aprueba el contenido, el PR de E015 no se fusiona y el endpoint no se
despliega; los otros 14 endpoints no se bloquean. No se sirve un 503 ni un texto provisional.

### D-E016 — Historial de consentimientos
`Query` sobre `PK=USER#<sub>` con `begins_with(SK, "CONSENT#")`, sin `Scan`, orden descendente por
`SK` y paginación por cursor opaco firmado (HMAC-SHA256 derivado de la clave existente,
`DeriveCursorKey(..., "e016")`; dominio ligado a `limit`). `limit` 1..100 (defecto 25); cursor
inválido, de otro usuario o emitido con otro `limit` → **400**.
Los items heredados de E001 (`CONSENT#<noticeVersion>`, con `acceptedAt`) se proyectan como
`purpose=privacyNotice`, `granted=true`, `recordedAt=acceptedAt` e `id` determinista derivado del
`SK`. No se reescriben ni se borran.

### D-E017 — Registro de consentimiento
Append-only. Se crea `USER#<sub>` / `CONSENT#<RFC3339Nano>#<uuid>` con `id`, `noticeVersion`,
`purpose`, `granted`, `recordedAt` fijado por el servidor, `source="api"` y `createdAt`.
`purpose` ∈ `privacyNotice|notifications|analytics`; `noticeVersion` no vacío ≤ 50 runes.
`user` inexistente o no `ACTIVE` → **404**/**403** según corresponda sin enumerar. Respuesta
**201** con `Consent`. Nunca se sustituye el historial por un booleano. `Idempotency-Key` no es
exigida por el contrato; un reintento idéntico crea un registro nuevo (append-only), hecho que se
documenta en la prueba de comportamiento.

### D-E018 — Onboarding
`GetItem` de `USER#<sub>/ONBOARDING`. Proyección al contrato:
`completedSteps` (lista persistida; ausente = `[]`), `done` derivado de que estén todos los pasos
requeridos, `version`, `microphone`/`notifications` booleanos (`granted` → `true`; `pending` o
`denied` → `false`), `configured`, `completedAt` (nullable).
Pasos canónicos admitidos: `profile`, `income`, `privacy`, `permissions`. Un paso desconocido en
E019 → **422**.

### D-E019 — Actualización de onboarding
`OnboardingPatch.completedSteps` es obligatorio (puede ser `[]`). Se normaliza a lista sin
duplicados y en el orden canónico. `microphone`/`notifications`/`configured` opcionales; al
escribirlos se persiste `granted`/`denied`. CAS interno: se lee `version`, se escribe con
`#version = :leida` y `ADD #version :one`; si otro nodo gana la carrera se reintenta una vez y,
si persiste, **409**. `done` y `completedAt` se recalculan en la misma escritura. No hay
`If-Match` en el contrato: no se exige 428; el 409 cubre el conflicto real.

### D-E022 — Catálogo de categorías
Catálogo **estático versionado en código** (`contentVersion = "2026-11-v1"`), servido con
`Cache-Control: public, max-age=300` y caché Redis de lectura con TTL 300 s bajo clave pública
`nexum:v1:catalog:categories:<version>`. H06 autoriza expresamente “recursos estáticos
versionados” para catálogos (DYNAMODB_ACCESOS_H06 §4). No se crea la partición `PUBLIC#` ni se
amplía IAM. Ids estables alineados con los valores que E001 ya persiste:
`Vivienda`, `Servicios`, `Transporte`, `Comida`, `Otros` (tipo `expense`), más `Ingreso`
(`income`) y `Ahorro` (`saving`). `PageOfCategory` con `items` y sin `nextCursor` (catálogo
pequeño, sin parámetros de paginación en el contrato).

### D-E023 — Catálogo de frecuencias
Catálogo estático versionado con las **7 frecuencias canónicas de E001** (RFC-020):
`weekly`→Semanal, `fortnightly`→Quincenal, `monthly`→Mensual, `bimonthly`→Bimestral,
`quarterly`→Trimestral, `semiannual`→Semestral, `annual`→Anual. El enum publicado del esquema
conserva además `biweekly` y `yearly` (9 valores) para no reducir el contrato, pero el catálogo
**no** los emite como ítems. Mismo tratamiento de caché que E022. Sin parámetros de paginación en
el contrato: una página.

### D-E024 — Ingreso base
`GetItem` de `USER#<sub>/INCOME` con `ConsistentRead`. `amount` = `FormatCentsToDecimal
(amountCents)`; `currency` = `MXN`; `frequency` = valor canónico persistido; `version`.
Item ausente → **404**. No se recalcula el equivalente mensual en lectura; se expone el ingreso
base, no los movimientos.

### D-E025 — Configurar ingreso base
`If-Match` obligatorio (mismas reglas 428/400/412 que E013). `amount` decimal string con dos
decimales, > 0 y ≤ 12 dígitos enteros (se parsea con `httputil.ParseDecimalToCents`, sin float);
`currency` debe ser `MXN`; `frequency` validada con el enum publicado de **9** valores de RFC-020;
los alias `biweekly`→`fortnightly` y `yearly`→`annual` se normalizan antes de persistir y la
respuesta emite sólo el valor canónico. `UpdateItem` condicional con `#version = :expected`, `SET
amountCents, currency, frequency, monthlyEquivalentCents, updatedAt ADD #version :one`; el
equivalente mensual se recalcula con el motor exacto de A-070. Respuesta 200 con el `Income`
actualizado y ETag `"v<version>"`.

### D-E068 — Preferencias personales
Al primer acceso se materializan los valores por defecto con un `PutItem` condicional
`attribute_not_exists(PK)`: `locale=es-MX`, `timezone=America/Mexico_City`, `version=1`. La
respuesta incluye `locale`, `timezone`, `version` y el alias `timeZone` con el mismo valor (A8).
La creación perezosa es idempotente y no altera datos existentes.

### D-E069 — Modificación de preferencias
`If-Match` obligatorio (428/400/412). Se acepta `timezone` o `timeZone`; si llegan ambos con
valores distintos → **422**. `locale` ∈ `es-MX|en-US`. `timezone` validada con
`time.LoadLocation` (IANA real, se rechaza `Local`, `UTC` no es zona IANA de usuario y nombres
fuera de la base; `time/tzdata` embebido). `UpdateItem` condicional con versión y `ADD version 1`;
respuesta 200 con `UserPreferences` y ETag. Item ausente se materializa antes de comparar, de modo
que `"v1"` es un ETag válido tras la primera lectura o en el primer PATCH.

### D-E070 / D-E071 — Contenido público curado
Contenido **estático versionado en código** (`2026-11-v1`), en español, redactado para el producto:
10 preguntas frecuentes y 8 consejos educativos reales, sin marcas, productos financieros ni
consejos de inversión personalizados. Sin integración externa, sin `example.invalid`, sin
placeholders. `PageOfFaq`/`PageOfTip` con `items`; sin parámetros de paginación en el contrato se
devuelve una sola página y se omite `nextCursor`. Caché pública Redis TTL 300 s.

### D-429 — Límite de tasa
Middleware compartido sobre `cache.AuthRateLimiter` (script Lua atómico, clave hasheada):
- rutas públicas: 120 req/min por IP (`clientip` ya existente); si Redis falla → fail-open
  (se sirve la lectura, A-080).
- rutas autenticadas: 120 req/min por usuario; si Redis falla → fail-open para lecturas y
  escrituras no-auth, sin degradar autenticación.
Respuesta 429 con `Retry-After` y `ErrorEnvelope`. Si Redis no está configurado, se omite el
limitador y se documenta `429_NOT_EMITTED` (no se simula).

## 7. RFC-020 — Frecuencias canónicas (propuesta incluida)

`E001` admite 7 frecuencias (`weekly`, `fortnightly`, `monthly`, `bimonthly`, `quarterly`,
`semiannual`, `annual`); `E023`, `E024` y `E025` declaran sólo 4
(`weekly|biweekly|monthly|yearly`). Con el contrato actual, un usuario que se registró con
`fortnightly` recibiría de E024 un valor fuera del enum, y E023 no cubriría las frecuencias que
usarán los recurrentes de la Oleada 2.

`RFC-020-FRECUENCIAS_E023_E024_E025.md` (v1.1.0) propone, de forma **aditiva y compatible** (el
comparador de contrato sólo bloquea reducciones de enum):
1. `Frequency.id` (E023): enum publicado de **9** valores (7 canónicos + `biweekly`/`yearly`),
   con etiquetas en español para los 7 canónicos. El catálogo emite sólo los 7 canónicos.
2. `Income.frequency` (E024): enum publicado de **9** valores; la respuesta emite sólo los 7
   canónicos, normalizando alias históricos si existieran.
3. `IncomeUpdate.frequency` (E025): enum publicado de **9** valores; acepta los 9 y normaliza
   `biweekly`→`fortnightly` y `yearly`→`annual` antes de persistir y responder.
**No se elimina ningún valor actual**: el conjunto previo `{weekly, biweekly, monthly, yearly}` es
subconjunto del nuevo. El diccionario de datos del SDD se actualiza en el mismo PR.

## 8. Persistencia, concurrencia y aislamiento

| Endpoint | Item físico | Operación | Concurrencia |
|---|---|---|---|
| E012/E013 | `USER#<id>/PROFILE` | GetItem / UpdateItem CAS | E013 único escritor de PROFILE |
| E015 | estático | sin DynamoDB | n/a |
| E016/E017 | `USER#<id>/CONSENT#...` | Query append-only / PutItem | append-only, sin pérdida |
| E018/E019 | `USER#<id>/ONBOARDING` | GetItem / UpdateItem CAS | E019 con reintento y 409 |
| E022/E023 | estático + Redis | lectura | caché pública |
| E024/E025 | `USER#<id>/INCOME` | GetItem / UpdateItem CAS | E025 único escritor |
| E068/E069 | `USER#<id>/PREFERENCES` | PutItem condicional / UpdateItem CAS | E069 único escritor |
| E070/E071 | estático + Redis | lectura | caché pública |

Invariantes:
- Ninguna consulta usa `Scan`; todas son `GetItem` o `Query` con `PK` de la identidad verificada.
- E013/E025/E069 nunca sobrescriben el item completo: `UpdateItem` conserva
  `emailVerified`, `sessionEpoch`, `passwordHash`, `status`, `privacy*` y cualquier atributo ajeno.
- Toda `ConditionExpression` declara **sólo** valores usados en su expresión (regresión E003).
- Los cursores se firman con HMAC derivado de la clave existente; no se introducen secretos nuevos.
- Aislamiento probado con dos usuarios: ninguna lectura o escritura cruza `PK`.
- Carreras probadas con dos instancias lógicas sobre el mismo almacén (no se declara AWS hasta el
  Gate de despliegue).

## 9. Pruebas exigidas por endpoint

Cada PR incluye, como mínimo:

1. **Contrato**: esquema exacto, campos requeridos, `unknown fields` → 400, `null`/tipo inválido.
2. **Camino feliz** con persistencia real de adaptador (doble con estado cuando no haya AWS).
3. **Errores documentados**: 400/401/403/404/409/412/422/428/429/500/503 según aplique.
4. **Propiedad**: usuario B no lee ni modifica recursos de A (404/403 sin enumeración).
5. **Concurrencia**: dos escritores sobre el mismo item; CAS y versión; sin actualizaciones
   perdidas; sin valores de expresión no usados.
6. **Regresión E001–E011**: registro, login, refresh/replay (409), logout, recuperación,
   sesiones, verificación de correo.
7. **Contrato OpenAPI**: `validar_h06.py`, paridad YAML/JSON, comparador de compatibilidad.
8. **Seguridad**: sin `passwordHash`, tokens, `sessionEpoch` ni PII en logs/respuestas; gosec y
   Semgrep limpios; gitleaks sin hallazgos nuevos.

## 10. Integración y merges

1. El PR de andamiaje se fusiona primero, con `APRUEBO_MERGE_PR_<N>_SHA_<SHA40>`.
2. Los 15 PR se abren contra `main` una vez el andamiaje esté en `main`.
3. Antes del primer merge de endpoint, el orquestador construye
   `chore/h08e-oleada1-integracion` con los 15 cambios listos y ejecuta la suite completa, las
   pruebas cruzadas (E012/E013, E016/E017, E018/E019, E024/E025, E068/E069), la regresión
   E001–E011 y la comprobación de exactamente 15 rutas nuevas.
4. Se presenta **una solicitud agrupada** con las frases
   `APRUEBO_MERGE_PR_<N>_SHA_<SHA40>` de cada PR, su endpoint, CI y mergeabilidad.
5. Se fusiona secuencialmente actualizando `main`; si un rebase cambia el HEAD, la aprobación de
   ese PR queda invalidada y se repite CI + Gate.
6. Al terminar, CI de `main` debe estar verde en el SHA exacto antes de la release.

## 11. Release agrupada (RFC-019 E-2)

Sólo tras el Gate de despliegue:

- un único binario ARM64 del CI de `main`, con SHA-256 verificado contra `dist/binario.sha256`;
- publicación inmutable en S3 (`If-None-Match: *`, SSE `AES256`, checksum SHA-256) con el rol
  publicador;
- un único plan Terraform que sólo cambie Launch Template e IAM de artefactos (2 ARN nuevos);
- Gate de apply con SHA-256 completo del `.tfplan`;
- un único `instance refresh` con `MinHealthyPercentage=100`; capacidad transitoria declarada;
- verificación HTTPS **individual** de los endpoints desplegados y regresión E001–E011;
- contador oficial sólo con evidencia: 11 → **26/75** si los 15 están funcionales; si el contenido
  de E015 no se aprueba, el contador máximo verificable es **25/75** y se declara así, sin
  redondear ni contar un 503.
- **No se declara 26/75** hasta completar prueba HTTP y persistencia real de cada endpoint.

Riesgos de release declarados: capacidad `t4g.micro` en `us-east-2b` (procedimiento documentado
de restricción temporal de AZ y restauración 2/2/2), drift de Redis `default` no aplicado,
`opencode.json` local no versionado (el apply exigirá la política temporal del Gate de apply).

## 12. Costos y FinOps

- **No se crean recursos permanentes nuevos.** La release reutiliza bucket S3, tabla DynamoDB,
  Redis, ALB, CloudFront, ASG y Launch Template existentes.
- **Costo incremental estimado (supuestos declarados):** 2 objetos S3 de ~20 MB dentro del bucket
  ya presupuestado (almacenamiento y solicitudes por uso), tráfico de verificación HTTPS y logs
  CloudWatch por uso. No se dispone de medición de facturación de esta oleada:
  `AWS_COST_REAL = NOT_VERIFIED`; no se declara `0.0000 USD` como hecho.
- **Cero dinero personal:** no se cambia el plan Free, no se activan servicios pagados ni se usan
  créditos fuera de la cuenta `713797602090`.
- El costo recurrente de la infraestructura no cambia por el código; sólo el consumo por uso.
- No se usa el saldo histórico como presupuesto ni se garantiza factura neta cero.

## 13. Disponibilidad y AZ

- El ASG conserva `Min 2 / Desired 2 / Max 2` y ambas subredes configuradas.
- En la última evidencia de solo lectura, **las dos instancias estaban en `us-east-2a`**. Eso
  **no** acredita dos AZ activas: el diseño y las subredes lo permiten, pero el despliegue
  multi-AZ efectivo sigue `NOT_VERIFIED`.
- El release debe declarar la AZ de cada instancia y no afirmar redundancia multi-AZ hasta
  observar dos AZ distintas con dos targets `healthy`.

## 14. Definición de terminado

Un endpoint está terminado cuando: código real conectado, contrato exacto, pruebas verdes, PR con
CI 13/13, merge autorizado, release desplegada, HTTPS verificado y evidencia registrada. Los
estados `IMPLEMENTED`, `CI_GREEN`, `MERGED`, `AWS_DEPLOYED`, `HTTPS_VERIFIED`,
`FUNCTIONAL_VERIFIED` y `ANDROID_ACCEPTED` se registran por separado; ninguno se infiere del
anterior.

## 15. Aprobación humana solicitada (una sola frase)

La frase siguiente aprueba **conjuntamente**: (a) este Gate de alcance; (b) el uso de RFC-019 en
su forma reutilizable E-1/E-2 para esta oleada; y (c) `RFC-020` v1.1.0 con la ampliación aditiva
del enum de frecuencias (9 valores publicados, 7 canónicos emitidos).

**No** aprueba el contenido de E015. Esa adopción se solicita por separado con la frase de D-E015:
`APRUEBO_CONTENIDO_E015_VERSION_2026_11_V2_SHA256_ddfb0b1865e0e77a47f74c025e36bdbc102c6d424fc137c703f4316d69b66380`.

```
APRUEBO_GATE_OLEADA_1_RFC019_E012_E013_E015_E016_E017_E018_E019_E022_E023_E024_E025_E068_E069_E070_E071
```

Esta frase **no** autoriza merges, `terraform plan/apply`, publicación S3, `instance refresh` ni
recursos AWS. Cada una de esas operaciones conserva su Gate propio.

## 16. Estado por endpoint

Se actualiza en cada checkpoint; nunca se adelanta un estado.

| Endpoint | Código | PR | CI | Merge | Release | HTTPS | Funcional |
|---|---|---|---|---|---|---|---|
| E012 | NOT_STARTED | — | — | — | — | — | — |
| E013 | NOT_STARTED | — | — | — | — | — | — |
| E015 | CONTENIDO_Y_CONTACTO_PENDIENTES | — | — | — | — | — | — |
| E016 | NOT_STARTED | — | — | — | — | — | — |
| E017 | NOT_STARTED | — | — | — | — | — | — |
| E018 | NOT_STARTED | — | — | — | — | — | — |
| E019 | NOT_STARTED | — | — | — | — | — | — |
| E022 | NOT_STARTED | — | — | — | — | — | — |
| E023 | NOT_STARTED | — | — | — | — | — | — |
| E024 | NOT_STARTED | — | — | — | — | — | — |
| E025 | NOT_STARTED | — | — | — | — | — | — |
| E068 | NOT_STARTED | — | — | — | — | — | — |
| E069 | NOT_STARTED | — | — | — | — | — | — |
| E070 | NOT_STARTED | — | — | — | — | — | — |
| E071 | NOT_STARTED | — | — | — | — | — | — |

## 17. Registro

| Fecha | Cambio | Estado |
|---|---|---|
| 2026-11-18 | Creación del Gate de Oleada 1, preflight verificado y 15 endpoints declarados | PROPUESTO |
| 2026-11-18 | **v1.1.0:** corrige RFC-020 (9 valores publicados, sin eliminar `biweekly`/`yearly`), separa el contenido de E015 del alcance y registra `BLOCKED_CONTENIDO_EXTERNO` con R-E015-01/02/03; FinOps incremental estimado en lugar de `0.0000`; disponibilidad AZ declarada `NOT_VERIFIED` | PROPUESTO |
| 2026-11-18 | **v1.2.0:** E015 deja de depender de `nexumpr/nexum-privacidad`; el aviso se sirve desde la API (`/api/v1/privacy/notices/current`) con versión `2026-11-v2`. La adopción del texto exige frase separada; sin ella el PR de E015 no se fusiona. Sin web adicional ni 503 | PROPUESTO |
| 2026-11-18 | **v1.2.1:** aviso corregido para no afirmar funciones no implementadas (eliminación automática, notificaciones, analítica, voz), no prometer canal de Ayuda ni plazos de conservación no verificados y no afirmar TLS 1.3. `content` de **6724 bytes** UTF-8, SHA-256 `29b27611…8a06`; canal de contacto y `effectiveAt` quedan como decisiones humanas del release | **PROPUESTO** |
| 2026-11-18 | **v1.2.2:** datos del responsable autorizados (nombre, domicilio, correo), `url` pública `https://nexumpr.github.io/nexum-privacidad/`, `effectiveAt` inmutable `2026-11-20T00:00:00Z` validado por prueba y `content` final de **6724 bytes** SHA-256 `ddfb0b18.6380`. Se registra que la página pública es de otro integrante y puede citar infraestructura obsoleta; no se modifica ni se usa como fuente contractual | **PROPUESTO** |
