# Migración segura de opencode.json — candidato permanente

**ESTADO: PROPUESTA / NO INSTALADA.** La configuración original del ZIP se mantiene intacta en el árbol de `main`. La configuración activa de Windows también debe conservarse intacta hasta respaldo/diff y autorización; `opencode.permanente.CANDIDATO.json` se construyó a partir de la copia local aportada, no del ZIP.

## Diferencias demostradas

- ZIP: 151 permisos + 33 políticas; configuración local: 161 permisos + 43 políticas.
- Local incorpora 10 entradas AWS (3 consultas ASG, 4 mutaciones ASG, protección, ELBv2 y 2 EC2; contar entradas exactas en JSON) y 10 policies ALLOW paralelas.
- Hard policy `shell:terraform apply*` en origen deniega apply a pesar de permissions ASK; se elimina **solo en el candidato** para permitir ASK después de Gate.
- Se mantienen denies para `terraform destroy`, `git push origin main`, force push, archivos secretos y eliminación de recursos AWS.
- Reglas especificas AWS ASK deben aparecer después de `aws *`/`aws * update-*`/`aws * put-*` de permission; el orden determina resultado (última coincidencia). Policies no conceden permiso.
- `spec.md`, `plan.md` y OpenAPI pasan de deny a ask en la propuesta. No modifica condición de RFC previa ni permite silenciosamente ediciones aprobadas.

## Verificación realizada

`verificaciones_permisos_estaticas.json` evalúa 20+ casos con un simulador de coincidencias de reglas. **No sustituye el motor real de OpenCode**, ni demuestra funcionamiento del shell scanner PowerShell. Verificar versión, `opencode debug config`, agentes, denied/ask/allow contra solicitudes no destructivas.

## Integración

1. Respaldar la configuración local fuera del control de versiones y calcular SHA256 antes de cualquier edición.
2. Comparar candidato con `opencode.json` actual de Windows. No copiar completo sin reconciliación de tres vías y control de nuevas reglas locales.
3. Solicitar RFC/Gate para cambiar hard-deny de Terraform apply a ASK, editar APPROVED vía ASK y AWS sensibles.
4. En la rama autorizada, incorporar cambios mínimos. Conservar `deny` absolutos. Actualizar BASELINE_SHA256 usando blobs versionados conforme a H07.
5. Probar motor real de OpenCode sin ejecutar mutaciones AWS ni Git mutante para simular permisos. Si falla, devolver configuraciones previas por procedimiento autorizado (no `git reset --hard`).
6. Aprobar PR/merge separados del Gate AWS por ejecución. Retirar aprobaciones guardadas demasiado amplias si las hubiera.
