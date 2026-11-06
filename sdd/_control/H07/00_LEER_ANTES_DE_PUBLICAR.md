# H07 — leer antes de modificar o publicar

La publicación inicial **YA ocurrió** en `nexumpr/Nexum-Backend` (commit `84be72d`). Este documento no autoriza recrear el repositorio. Está pendiente integrar por PR la corrección H07 autorizada: estructura auxiliar dentro de `sdd/_control/H07`, CRLF/LF determinista, SHA integral y estado único para OpenCode.

**Norma:** solo tres bloques de specs `sdd/01-arquitectura-general`, `sdd/02-infraestructura` y `sdd/03-endpoints`; `_control` es archivo auxiliar de gobierno, no un cuarto conjunto. Ninguna de las 103 especificaciones se modifica por este cambio.

Leer en orden `constitution.md`, `AGENTS.md`, `memoria.md`, `ESTADO_Y_GATES.md` y `CAUSA_RAIZ_CI_Y_MIGRACION.md`. Para publicar usar `GUIA_PUBLICACION_GITHUB.md`. No hacer `push --force`, `terraform apply`, cambiar `opencode.json` ni activar Paid. Gates H06/H07 permanecen pendientes hasta ratificación humana inequívoca.
