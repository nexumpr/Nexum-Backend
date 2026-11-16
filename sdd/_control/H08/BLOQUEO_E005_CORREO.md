# Bloqueo E005 — proveedor real de correo

| Campo | Valor |
|---|---|
| Documento | `sdd/_control/H08/BLOQUEO_E005_CORREO.md` |
| Fecha | 2026-11-16 |
| Código | `BLOCKER_E005_MAIL_PROVIDER` |
| Estado | **ABIERTO — decisión humana pendiente** |
| Consultas AWS | `NOT_RUN` (ninguna llamada a AWS en esta unidad) |
| Efecto | Bloquea la **entrega funcional** de E005. **No** bloquea la preparación documental ni CI |

---

## 1. Hecho verificado

`POST /api/v1/auth/password/forgot` debe enviar un correo real con un token de un solo uso.

- **Búsqueda en el árbol Go** (`smtp`, `net/smtp`, `ses.`, `SendEmail`, `mail.`): **0 coincidencias**.
- No existe adaptador de correo, ni puerto, ni configuración asociada.
- `sdd/03-endpoints/E005-auth-password-forgot/spec.md` exige expresamente
  *«envío mediante adaptador verificado»* y que la respuesta sea **indistinguible** para correo
  existente e inexistente (no enumeración).

Conclusión: devolver `202 Accepted` sin entrega real sería una respuesta exitosa ficticia y está
prohibida (`constitution.md` §2.3, §11; `R-041`). **E005 no puede declararse implementado sin un
proveedor que envíe de verdad.**

## 2. Restricción de red que condiciona cualquier opción

Arquitectura vigente (I-010, ya integrada):

| Elemento | Estado |
|---|---|
| NAT Gateway | **0** (rechazado por FinOps) |
| IPv4 públicas en instancias | **0** |
| Rutas públicas en subredes | **0** |
| Internet Gateway | adjunto, **sin rutas públicas** (requisito de CloudFront VPC Origins) |
| Egress permitido | `443` solo a prefijos administrados de **S3 y DynamoDB** (gateway endpoints) |
| Interface endpoints | `logs` permanente; `secretsmanager`, `ssm`, `ssmmessages` por ventana |

Consecuencia: **las instancias no tienen salida a Internet**. Un cliente SMTP/HTTPS hacia un
proveedor externo no funcionaría sin abrir una ruta nueva.

## 3. Alternativas evaluadas

### A. Amazon SES

| Requisito | Estado | Nota |
|---|---|---|
| Identidad remitente verificada (dominio o correo) | **PENDIENTE** | Verificar un correo exige acceso al buzón; verificar dominio exige DNS (no hay dominio propio en el proyecto) |
| Salida del sandbox (envío a destinatarios no verificados) | **PENDIENTE** | Solicitud a AWS, aprobación manual |
| Permiso IAM `ses:SendEmail` en el rol runtime | **NO EXISTE** | Requiere cambio de IaC (I-060) |
| Camino de red hasta SES | **BLOQUEADO / NOT_VERIFIED** | No hay ruta a Internet. SE documenta que **SES no dispone de VPC interface endpoint** según conocimiento del agente: **NO VERIFICADO en esta unidad** — debe confirmarse en el Gate con documentación oficial antes de descartar |
| Coste | **NO VERIFICADO** | Precio público por 1.000 correos + posible cargo por adjuntos; Free plan y créditos no consultados aquí |
| Secreto | No aplica con IAM + SES API | Ventaja frente a SMTP |

### B. SMTP de un proveedor externo (corporativo o personal)

| Requisito | Estado |
|---|---|
| Credenciales | **Rechazado**: obligaría a guardar usuario/contraseña en el bundle de secretos (RFC-004) y a usar una cuenta ajena al proyecto |
| Camino de red | **BLOQUEADO** igualmente |
| Dependencia | Tercero sin contrato ni garantía |

### C. Cola interna + worker de envío

No resuelve la entrega: añade recursos (SQS + worker), sigue necesitando salida a Internet y
rompe la restricción de dos nodos si el worker es una tercera instancia.

### D. Responder `202` sin enviar

**PROHIBIDO.** Sería evidencia falsa.

## 4. Lo que se necesita decidir (en este orden)

1. **Camino de red**: elegir entre (a) verificar si SES admite VPC endpoint y usarlo,
   (b) autorizar un NAT Gateway con coste acotado, o (c) posponer E005.
   → Cualquiera de (a)/(b) es **mutación de infraestructura**: exige Gate de Terraform, plan y
   aprobación por SHA-256.
2. **Identidad remitente**: dominio propio (coste de registro y DNS) o correo verificable.
3. **Salida del sandbox de SES** si se elige SES.
4. **Permiso IAM** `ses:SendEmail` acotado a la identidad verificada.
5. **Coste** dentro de créditos y del presupuesto de I-000 (`AWS_COST_REAL = NOT_VERIFIED`).

## 5. Impacto en el plan

| Actividad | ¿Bloqueada por esto? |
|---|---|
| Integrar documentación y verificadores (este PR) | **No** |
| Aprobar RFC-016/017 | **No** |
| Implementar E006–E075 (los que no envían correo) | **No** |
| Implementar E005 completo | **Sí** |
| Declarar `E005_PUBLICADO` o `READY_FOR_E005` sin reservas | **Sí** |

**No se ha creado ningún recurso AWS, no se ha verificado ninguna identidad y no se ha consultado
precio alguno.** Este documento no elige proveedor: registra el bloqueo y las decisiones que lo
desbloquean.
