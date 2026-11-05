# Evidencia local y criterios de validación H07

Se compuso una copia de trabajo con ZIP base y parches H05 V2, V3, V4 y H06. **No se ha accedido al disco del usuario por red.** La herramienta `herramientas/validar_sdd_integral.py` efectúa controles offline: archivos de raíz, 8 agentes, 9 skills, 12/16/75 especificaciones y sus 412 documentos, H06 OpenAPI/catalogo, matrices, integridad SHA de baseline y detección de rutas sensibles incluidas en snapshot. No compara el PDF original de 2026 ni ejecuta AWS/GitHub.

**Interpretación de resultado:** `SDD_DOCUMENTAL_PASS` significa correspondencia con el ZIP baseline y requisitos estructurales; **no** significa H06/H07 aprobados, ni infraestructura validada, ni servicios 24/7, ni código Go en producción. Los datos AWS Free, CloudFront, Redis y ML de la matriz de pendientes siguen en `BLOCKED/NOT_RUN` hasta evidencia futura.

**Fuente de hashes:** `BASELINE_SHA256.json` generado de archivos de esta copia compuesta, excepto él mismo. Para evolución controlada usar RFC firmado y reevaluación; jamás regenerar silenciosamente el manifest para ocultar deriva.
