# Evidencia y límites de validación — H07

**Evidencia compartida por el usuario:** ejecución en Windows 11 de `py -3.13 herramientas/validar_sdd_integral.py` con `H06_CONTRACT_PASS` 75 operaciones y `SDD_DOCUMENTAL_PASS` 12+16+75, 412 docs; hash H07 local PASS. Repo privado creado con `gh repo create`, primer commit `84be72d` publicado. Captura de GitHub Actions run `42133317544` revela job `validar` fallido con exit code 2.

**Reproducción externa del fallo** sobre el snapshot subido: `FINOPS_45_DIAS_DESGLOSE_V2.csv` es el único archivo CRLF; Git lo normaliza a LF al llevarlo a Linux y el SHA del baseline anterior no coincide. Este hallazgo no prueba otro fallo de la API; tampoco garantiza que no existan pendientes distintos en CI hasta observar su ejecución real.

**Remediación:** normalizar CSV a LF, introducir `.gitattributes`, conservar controles SHA-256 y generar manifiesto de bytes del nuevo baseline, mover documentos auxiliares bajo `sdd`, integrar mediante PR con CI. Ninguna spec A/I/E ni su texto funcional debe reescribirse. Las salidas de prueba en el ZIP son locales; **el CI futuro en GitHub es NOT_RUN hasta que el usuario ejecute la publicación y comparta la evidencia**.

No se han ejecutado recursos AWS ni pruebas HTTP reales. El prototipo y PDF original no se cotejaron de nuevo de forma independiente; no declarar cumplimiento completo contra esas fuentes.
