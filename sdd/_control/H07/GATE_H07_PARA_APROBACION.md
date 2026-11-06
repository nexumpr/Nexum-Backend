# Gate H07 — revisión de publicación documental

**Estado: PUBLICACIÓN INICIAL YA REALIZADA; GATE HUMANO PENDIENTE.** El commit inicial `84be72d` existe en el repositorio privado; el workflow inicial falló. La corrección de CI y estructura fue autorizada por el usuario, pero todavía debe obtener validación remota real.

Criterios de aceptación: [ ] H06 Gate ratificado; [ ] 103 specs/412 documentos íntegros; [ ] tres bloques y control auxiliar dentro de `sdd`; [ ] SHA integral estable Windows+Linux; [ ] OpenAPI 75 válido; [ ] tests de fallo de SHA y de enlaces; [ ] PR revisado; [ ] GitHub Actions `success` en PR **y main**; [ ] sin secretos o archivos externos; [ ] estatus y restricciones inequívocos para OpenCode; [ ] ratificación humana expresa.

**No permite:** crear AWS, cargar Terraform en producción, modificar permisos de OpenCode, afirmar que backend o integraciones funcionan ni cerrar riesgos de Free. Las verificaciones ejecutadas deben quedar con URL/commit/evidencia, sin marcar `PASS` antes de observarlas. Procedimiento corregido: `GUIA_PUBLICACION_GITHUB.md`.
