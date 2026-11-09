# Gate H08-I — RFC-009 publicación del repositorio

| Campo | Valor |
|---|---|
| Documento | `sdd/_control/H08/GATE_RFC009_PUBLICACION.md` |
| Fecha | 2026-11-09 |
| Instrumento | `sdd/_control/H08/RFC-009-PUBLICACION_REPOSITORIO.md` |
| Estado | **APRUEBO_REPO_PUBLICO_CON_RFC_009 REGISTRADO — EN EJECUCIÓN** |
| Autorización | `APRUEBO_REPO_PUBLICO_CON_RFC_009` (mensaje humano independiente) |

## Alcance

Cambiar `nexumpr/Nexum-Backend` de privado a público, y habilitar acto seguido la protección de
`main` con checks obligatorios y aprobación para colaboradores externos.

## No autoriza

AWS, Terraform remoto, despliegue, cambio de plan de GitHub, pagos, force push ni reescritura de
historial.

## Verificación

- Barrido de secretos sin baseline: 8 falsos positivos, cero credenciales reales.
- Historial sin archivos sensibles (`.env`, `.pem`, `.key`, `tfstate`, `tfvars`, `credentials`).
- Tras la publicación: `Go Backend CI` 12/12 y SDD documental en verde.
