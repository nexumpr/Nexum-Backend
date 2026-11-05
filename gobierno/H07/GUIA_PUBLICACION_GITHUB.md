# Procedimiento local seguro (Windows 11 / Warp / PowerShell 7)

**No ejecutar pasos de escritura remota antes de Gate H06, Gate H07 y autorización explícita de publicar.** No crear directamente desde este ZIP; primero instalar, validar y observar estado local real. Cuenta destinataria prevista `nexumpr`, repositorio privado nuevo `Nexum-Backend`, no legacy.

## Solo lectura (se puede ejecutar ahora)

```powershell
cd C:\Proyectos\Nexum\Nexum-Backend
py -3.13 herramientas/validar_sdd_integral.py
py -3.13 sdd/03-endpoints/herramientas/validar_h06.py
gh auth status -h github.com
git status --short
git remote -v
```

Verificar que la sesión activa de GitHub corresponda **a `nexumpr`**; no ejecutar `gh auth switch` ni cambiar la sesión de otra cuenta por defecto. Si hay `.git`/remote previo, **detenerse y revisar**; no sobreescribir historia ni `git push --force`.

## Únicamente tras aprobación de publicación

Confirmar actor `nexumpr`, que no existe repositorio activo `nexumpr/Nexum-Backend`, y que no se entregan logs, respaldos, `.aws`, keys ni `tfstate`. Si ya existe, no sobrescribir; revisar con el propietario. Para repositorio local aún sin `.git`:

```powershell
cd C:\Proyectos\Nexum\Nexum-Backend
git init -b main
git add -- README.md AGENTS.md constitution.md memoria.md opencode.json .gitignore .opencode sdd gobierno herramientas .github
git diff --cached --stat
git diff --cached --name-only
git status --short
```

**Revisión humana obligatoria del staging.** `git diff --cached --name-only` es más importante que `.gitignore`, revisar secretos con herramientas offline antes de `commit`. Si todo está correcto:

```powershell
git commit -m "docs: baseline inicial SDD Nexum Backend H07"
gh repo create nexumpr/Nexum-Backend --private --source . --remote origin
git push -u origin main
```

`gh repo create` y `git push` son acciones **remotas** y deben ejecutarse solo después de la aprobación. Estos comandos no se ejecutan por ningún instalador. Si ya existe repositorio local, revisar `git status`, historia y remoto; no usar `git init` ni `gh repo create` a ciegas. Tras publicar, configurar protección de main, revisión por PR, CI documental obligatorio y permisos mínimos. Proponer `feature/arquitectura-A-xxx`, `feature/infra-I-xxx`, `feature/api-E-xxx`, PR con dueño y QA; integración secuencial conforme gates.

## Gate de bloqueo

Falla de validación, secreto, conflicto de hashes, cuenta GitHub incorrecta, repositorio remoto existente o fuente pendiente no revisada = NO PUSH hasta resolución. CI documental no sustituye revisión humana ni pruebas runtime.
