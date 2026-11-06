# Publicación correctiva H07 — GitHub existente (NO crear otro repositorio)

Repositorio real privado `nexumpr/Nexum-Backend`, commit inicial `84be72d`. La publicación original ya se ejecutó, aunque el primer CI falló. **No recrear ni eliminar `main`, no usar --force, no editar el repo archivado legado.**

El paquete `Nexum_H07_CORRECCION_ESTRUCTURAL_CI_PUBLICACION_FINAL_20261105.zip` tiene un instalador ZIP ejecutable para Python 3.13 que valida exactamente el baseline existente, crea respaldo fuera del repo y trabaja sobre la rama `fix/h07-estructura-ci-integridad-20261105`.

```powershell
cd C:\Proyectos\Nexum\Nexum-Backend
$z = "$HOME\Downloads\Nexum_H07_CORRECCION_ESTRUCTURAL_CI_PUBLICACION_FINAL_20261105.zip"
py -3.13 $z --preflight
py -3.13 $z --instalar
py -3.13 $z --verificar
py -3.13 $z --publicar
```

`--publicar` requiere `git`, `gh` autenticado como `nexumpr` y repositorio remoto privado correcto. Crea commit/PR, espera el CI **del commit del PR** y solo integra a `main` si pasa realmente. Si las comprobaciones fallan o no aparecen, detiene el merge. No crea ni toca AWS. Si está bloqueado, inspeccionar salida del comando, nunca forzar ni desactivar seguridad.

Si GitHub exige aprobación de otro revisor o reglas de rama, el proceso informará `BLOCKED`; concluir por el flujo normal de GitHub tras las aprobaciones pertinentes. Tras el merge, verificar en la web y con `gh run list -R nexumpr/Nexum-Backend --limit 5`; `main` debe tener CI verde. El Gate H07 se ratifica después, por decisión humana expresa, no automáticamente por un script.
