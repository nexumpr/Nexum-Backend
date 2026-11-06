# RFC-001 — Permisos mínimos OpenCode 2.x para H08-A

**Decisión:** reemplazar perfiles H03-C de solo lectura por perfiles H08-A con controles explícitos, conservando definiciones previas en Git. El usuario aprueba la fase mediante CLI `--instalar --ratificar APRUEBO_H06_H07_Y_AUTORIZO_H08A_LOCAL`.

## Alcance de cambio
`opencode.json`, ocho perfiles `.opencode/agents`, nueve skills `.opencode/skills`, instrucciones rectoras y evidencias H08. No toca los 412 archivos base A/I/E ni el código fuente actual; no crea AWS.

## Permisos (v2)
`read/glob/grep/list` públicos del proyecto ALLOW, external_directory DENY, secrets DENY; `edit` sólo `cmd/internal/pkg/tests/go.mod/go.sum` ASK; `shell` ASK en general con prohibición explícita de AWS, Terraform, GitHub mutante y comandos destructivos; `subagent` solo seis perfiles por ASK; `skill` solo seis skills por ASK. Especialistas AWS/FinOps permanecen consulta, sin herramientas mutantes. Los agentes personalizados aplican sus propias reglas tras las generales; las denegaciones de `policies` no pueden elevarse con una aprobación de herramienta.

## Riesgos y verificación faltante
- No existe un aislamiento físico del host: OpenCode puede solicitar comandos arbitrarios locales; cada aprobación requiere lectura humana. Las globs/policies no son defensa infalible frente a scripts indirectos ni códigos externos.
- La configuración fue contrastada con el esquema V2 documentado y pruebas de semántica en Python, **no** ejecutada sobre el binario OpenCode del usuario; ejecutar prueba real antes de implementar.
- Evitar sesiones viejas/`Allow always` guardados: abrir nueva sesión, no habilitar autorización permanente, comprobar que pide permiso al escribir código y deniega AWS y SDD.

## Plan de reversión
Revertir por PR el commit de activación y volver a configuración H03 restrictiva; no `--force`, no reset destructivo, no modificar `main` fuera de PR.
