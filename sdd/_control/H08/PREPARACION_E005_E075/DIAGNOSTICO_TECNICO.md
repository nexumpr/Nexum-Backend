# Diagnóstico técnico preventivo E005–E075 — 2026-11-16

**Alcance real:** inspección estática del ZIP, SDD y auditoría V6.0.2. Sin consultas ni mutaciones AWS. No se ejecutó ningún despliegue.

## Verificaciones demostrables

- ZIP contiene 75 carpetas de endpoints con `spec.md`, `plan.md`, `tasks.md`, `checklist.md`.
- `validar_h06.py` de la copia original pasa: 75 operaciones, 300 specs, OpenAPI 3.1 válido.
- Go productivo presente: E001–E004 en `internal/application/identity`, handlers HTTP y adaptadores DynamoDB; no se modifican.
- Sesión autoritativa E003/E004: `USER#id / SESSION#id` más localizador `REFRESH#digest / SESSION`.
- CI ya dispone de jobs Go, OpenAPI/contratos, seguridad, SDD; CD controlado de releases ARM64/S3/LT/ASG. No se verifica ejecución actual de GitHub en esta copia.

## Bloqueos/decisiones

### CRÍTICO para E005 (antes de publicar)

- No se identificó en código Go de esta copia un adaptador operativo de entrega de correo (SMTP/SES). E005 requiere envío real; no puede declararse completo con una respuesta 202 ficticia. Verificar un proveedor existente/autorizado y su coste/identidad remitente/secreto durante el Gate E005. Si falta, escoger opción mínima autorizada y segura; no activar recursos AWS por anticipación.
- E005: respuesta SDD `Accepted {status,requestId}` contradice propuesta V6.0.2 `{accepted,message,retryAfterSeconds}`. Decidir con RFC y compatibilidad Android.

### IMPORTANTE transversal

- E006: `token` versus `resetToken`, salida incompatible; normalizar antes de implementar E006.
- 5 discrepancias en códigos HTTP SDD/PDF: E017, E031, E037, E042, E063; requieren decisión contractual, no reemplazo automático.
- E005–E011: `SESSIONHASH#digest` heredado; patrón de E003/E004 debe quedar claro y evitar crear tablas/índices nuevos.
- E043/E044/E047 y E028/E033/E037: atómica de aportaciones, reversión parcial, doble descuento recurrente, monto en centavos e idempotencia; revisar límites de DynamoDB `TransactWriteItems` y plan de grandes lotes.
- Definir antes del release si las rutas futuras/optativas se mantienen en catálogo o se priorizan para entrega académica; no afirmar que la maqueta tiene pantallas que aún no existen.

### RIESGO DE PERMISOS

- `opencode.json` local tiene 10 reglas ASK y 10 policies ALLOW adicionales frente al ZIP. Las policies ALLOW no anulan hard-denies. El bloque `shell:terraform apply*` hard-deny evita convertir apply en ASK. La configuración candidata usa ASK en permissions para apply y mutaciones autorizables, conservando hard-denies sobre destrucción y operaciones prohibidas.
- **No verificada contra el OpenCode instalado en Windows**: hacer `opencode --version` y `opencode debug config`; verificar escáner PowerShell y agentes en sandbox sin mutar AWS.
- No elegir `Allow always` sobre operaciones sensibles. El Gate documental es adicional a la aprobación interactiva.

## Estado

**BLOCKED para declarar preparación universal terminada**, porque la RFC contractual no está aprobada/integrada y la configuración candidata no está validada en el OpenCode del usuario. Esto no implica que E001–E004 estén caídos; su último estado productivo verificado es independiente.
