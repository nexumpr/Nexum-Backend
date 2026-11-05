# H07 — Ruta de entrada para el equipo Backend

**Estado:** paquete documental en `REVIEW_READY`, NO `APPROVED`; no da permiso de publicar ni de ejecutar infraestructura.

1. Leer `constitution.md`, `AGENTS.md` (ADENDA vigente), `memoria.md`, este archivo y `ESTADO_Y_GATES.md`.
2. Ejecutar `py -3.13 herramientas/validar_sdd_integral.py` y `py -3.13 sdd/03-endpoints/herramientas/validar_h06.py`. Ambas validaciones son locales y de solo lectura.
3. Revisar `MATRIZ_COBERTURA_Y_RIESGOS.md`, `FUENTES_Y_LIMITACIONES.md`, `GATE_H06_PARA_APROBACION.md` y `GATE_H07_PARA_APROBACION.md`.
4. **Después de aprobaciones humanas distintas de H06 y H07**, revisar `GUIA_PUBLICACION_GITHUB.md` y pedir autorización para `gh repo create`/`git push`. No interpretar un PASS como autorización.
5. Trabajo equipo Backend: primero cerrar arquitectura general A-000..A-110; luego infraestructura I-000..I-150; luego implementar operaciones E001.. y expandir catálogo solo por RFC. Gate de cada entrega, PR y revisión humana.

Este repositorio **no** incluye backend Go operativo, recursos AWS, pruebas HTTP reales ni CI de despliegue. El workflow `.github/workflows/sdd-validacion.yml` valida únicamente el SDD al publicarlo, no transforma el diseño en una aplicación funcionando.
