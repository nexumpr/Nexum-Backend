# Nexum-Backend — SDD rector del Backend

**Corte documental:** 2026-11-05 · **Estado:** H03 y H04 aprobados, H05 aprobado documentalmente con condiciones; H06 instalado/validado localmente y pendiente de Gate humano; H07 preparado para revisión, **sin autorización para publicar ni desplegar**.

> **ENTRADA VIGENTE:** [`gobierno/H07/00_LEER_ANTES_DE_PUBLICAR.md`](gobierno/H07/00_LEER_ANTES_DE_PUBLICAR.md), [`gobierno/H07/ESTADO_Y_GATES.md`](gobierno/H07/ESTADO_Y_GATES.md) y [`constitution.md`](constitution.md). Los documentos de fases anteriores contienen historia contextual, no permisos para saltar gates.

## Objetivo y límites

Backend Go de Nexum (finanzas personales): REST/JSON/HTTPS, JWT/sesiones, arquitectura limpia, DynamoDB NoSQL, Redis OSS real compartido, dos instancias Go independientes y simultáneas, CloudFront + ALB privado, AWS/Terraform, CI/CD Backend, DevSecOps, pruebas, observabilidad y recuperación. El equipo móvil es dueño de Android, pantallas, Play Store, CI/CD móvil y aprobación de su cliente; el Backend entrega contratos verificables sin depender de su aprobación.

**Cero dinero personal.** Cuenta AWS Free, nunca subir a Paid de manera automática; restricciones de créditos, fecha de caducidad, permisos AWS y presupuesto se validan **antes** del primer `apply`. La cifra histórica de ~USD 84 / 45 días es estimación no certificada. La automatización IaC completa es obligatoria y todavía no está implementada.

## Estado y rutas

| Lote | Contenido | Estado |
|---|---|---|
| H03 | `constitution.md`, `AGENTS.md`, `memoria.md`, 8 agentes, 9 skills | APPROVED (sin ampliar permisos OpenCode) |
| H04 | `sdd/01-arquitectura-general/`, 12 specs × 4 documentos | APPROVED |
| H05 | `sdd/02-infraestructura/`, 16 specs × 4 documentos y anexos FinOps/IaC | APPROVED_DOCUMENTAL_CONDICIONADO; AWS NOT_RUN |
| H06 | `sdd/03-endpoints/`, 75 contratos × 4 docs, OpenAPI 3.1 | INSTALLED/CONTRACT_PASS; GATE_PENDIENTE |
| H07 | `gobierno/H07/`, evidencia, baseline, CI documental y publicación | REVIEW_READY, NO GATE APROBADO |
| H08+ | Implementación Go/Terraform, despliegue y pruebas reales por releases | NOT_STARTED |

El catálogo contiene 75 operaciones actuales (33 maqueta, 23 soporte, 12 anunciadas, 7 opcionales). **No es un límite futuro**; nuevas necesidades se integran por RFC, contratos y versionado, sin cambiar silenciosamente lo ya aprobado.

## Validar localmente, sin AWS ni GitHub

Desde la raíz en PowerShell 7:

```powershell
py -3.13 herramientas/validar_sdd_integral.py
py -3.13 sdd/03-endpoints/herramientas/validar_h06.py
```

Los PASS son **documentales**, no prueban infraestructura, endpoints HTTP, ML/IA, correo/push ni costos. Las aprobaciones y pendientes están en [`gobierno/H07/ESTADO_Y_GATES.md`](gobierno/H07/ESTADO_Y_GATES.md).

## Gobierno GitHub

El propietario correcto será **`nexumpr`**; no utilizar otra cuenta. No ejecutar `gh repo create`, `git push`, cambios AWS o permisos OpenCode elevados sin Gate H06/H07 y autorización humana **específica de publicación**. La propuesta de ramas, PR y revisión está en [`gobierno/H07/GUIA_PUBLICACION_GITHUB.md`](gobierno/H07/GUIA_PUBLICACION_GITHUB.md). Nunca versionar `.env`, state Terraform, reportes privados, respaldos, datos reales o secretos.

**Fuentes pendientes de cotejo independiente:** el prototipo Android vigente y el PDF original académico. El inventario de H04/H06 es trazable documentalmente, pero la integridad de sus bytes con esas fuentes no se ha revalidado en esta entrega.
