# Checklist de arranque real (no simulado)

- [ ] Instalación en Windows con ratificación explícita, salida `INSTALL_PASS` y respaldo.
- [ ] Validador documental local PASS y tests de permisos PASS (static model, no runtime OpenCode).
- [ ] GitHub PR de configuración H08-A aprobado; checks PASS; merge ordinario; CI de main SUCCESS.
- [ ] `opencode --version` muestra 2.x en Warp y el usuario tiene proveedor/modelo autorizado sin gasto personal.
- [ ] SESIÓN NUEVA OpenCode: el orquestador descubre los seis subagentes elegidos, y verifica que las acciones Go pidan confirmación.
- [ ] En sesión real, intento controlado de leer SDD (ALLOW), intentar edición SDD (DENY), intento de `aws --version` (DENY); no realizar un `terraform apply` real para probar bloqueos.
- [ ] Primer prompt A-000 reporta plan sin escribir ni ejecutar. Responsable aprueba su propuesta antes de implementación.

Hasta marcar las verificaciones de runtime como PASS, el equipo puede leer y planificar pero no confiar en permisos del motor para iniciar edición automática. Ningún check obliga a compartir claves, datos personales o facturación.
