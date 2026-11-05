# Gate H07 — primera publicación del SDD

**Estado: REVIEW_READY; NO APPROVED.** Requisito previo: aprobación expresa del Gate H06.

Condiciones documentales: [ ] validación integral del repositorio instalado (PASS local Windows) [ ] fichero de baseline sin deriva [ ] no archivos confidenciales en `git status --short` ni `git diff --cached --name-only` [ ] cotejo de original docente/prototipo identificado como pendiente y no ocultado [ ] cuenta y destino `nexumpr/Nexum-Backend` verificados [ ] repo privado, no reutilizar legacy [ ] reglas de PR revisadas [ ] autorización explícita del usuario para crear/subir.

**Lo que valida H07:** estructura, trazabilidad, hashes, seguridad básica del staging, reglas de colaboración, historia y fuentes. **Lo que no valida:** operación AWS, SKU Redis, servicios Free reales, infraestructura automatizada, APIs HTTP, coste $0, ML real o cumplimiento de cada pantalla móvil.

Tras aprobarlo, abrir repositorio privado nuevo y publicar solo SDD inicial, no modificar archivo ajeno, no ampliar permisos OpenCode y no hacer merges automáticos. Toda operación `gh repo create`/`git push` exige autorización humana específica. El workflow CI documental debe pasar para proteger `main` una vez publicado.
