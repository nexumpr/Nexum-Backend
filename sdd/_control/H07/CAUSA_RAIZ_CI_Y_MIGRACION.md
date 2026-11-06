# Registro de incidente: primera publicación H07 y reparación sin ocultamiento

**Incidente 2026-11-05:** commit `84be72d` publicado en repositorio privado `nexumpr/Nexum-Backend`; workflow `sdd-validacion.yml` run `42133317544` falló en `validar` exit code 2. El paquete original se probó en Windows, no sobre un checkout limpio de Git/Linux con normalización de finales de línea.

**Causa raíz técnica reproducida:** `sdd/02-infraestructura/FINOPS_45_DIAS_DESGLOSE_V2.csv` tenía 32 secuencias CRLF; manifiesto SHA del baseline anterior medía esos bytes, pero Git normaliza a LF durante el checkout de Linux. Hash distinto, aunque celdas CSV equivalentes. Las otras 102 specs, los 75 contratos y OpenAPI no se alteran por este arreglo.

**Causa de gobierno:** se introdujo `gobierno/H07/` en raíz sin autorización previa. El usuario aprobó reubicar gobierno auxiliar dentro del SDD; queda `sdd/_control/H07/` **no equiparable a un cuarto bloque de specs**. Se elimina `gobierno/` en raíz, con referencias internas actualizadas. El documento de autoridad sigue siendo `constitution.md`.

**Corrección exacta:**

1. Normalizar CSV indicado a LF (contenido tabular sin cambio semántico) y versionar `.gitattributes` para mantener LF desde Windows y Linux.
2. Reubicar los 14 documentos auxiliares H07 y su manifest, actualizar referencias en validadores/documentos.
3. Reescribir `README.md`, `AGENTS.md` y `memoria.md` como documentos **completos** con una sola tabla vigente; la versión anterior permanece accesible en el historial Git.
4. Aumentar rigor: SHA-256 en todos los documentos del baseline, detección de archivos extra, pruebas unitarias negativas e igualdad entre índice Git y baseline en CI. Nunca sustituir `FAIL` por `PASS` sin comprobar.
5. Crear rama/PR, correr CI de checkout limpio, hacer merge solo si `success`. El Gate H06 y H07 **no se marcan APPROVED** por corrección o publicación.

**Pendientes reconocidos:** no se ha implementado Go/AWS; permisos OpenCode siguen restrictivos; gastos AWS, Redis SKU, TLS/origen y requisitos externos permanecen pendientes donde aplica. Ningún proveedor externo ni seguridad real queda certificado por el CI documental.
