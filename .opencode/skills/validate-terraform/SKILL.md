---
name: Validate Terraform Nexum
description: "Establecer validaciones Terraform read-only y revisión humana de cambios AWS, sin apply/destroy ni pérdida de datos."
metadata:
  opencode/autoinvoke: false
---

# Skill — validate-terraform

**Estado:** H03-D / REVIEW. **Invocación:** no automática; la carga sigue técnicamente denegada hasta cambio autorizado y comprobado. No crea código, specs ni servicios.

## Disparador y objetivo

Al revisar IaC de infraestructura en H05 o validar código Terraform autorizado en H08.

## Entradas requeridas

- Arquitectura y IaC aprobadas; cuentas/región y límites SCP documentados.
- Costos, propuesta de plan, state backend no legado, impacto y autorización correspondiente.
- Evidencia diferenciada de local, plan y AWS ejecutado.

## Precondiciones y fase

En H03–H07 no ejecutar Terraform ni crear `infra/`; desde H08 un `terraform init` puede instalar plugins/cambiar state y requiere permiso explícito.

## Alcance y archivos autorizados

Instrucción y checklist en H03; criterios en H05; implementación solo H08; acceso cloud exclusivamente con permisos humanos por operación.

## Flujo operativo detallado

1. Comprobar cuentas de trabajo: `713797602090`/`us-east-2` y estado Free (observar, no asumir actualidad).
2. Revisar IaC contra dos instancias reales, balanceador TLS, DynamoDB, Redis, IAM, observabilidad, backup y encendido/apagado seguro.
3. Pedir al humano revisión de `.tfstate`, plan y secretos solo en local privado; no volcarlos a chat/registro.
4. En fase habilitada: revisión `fmt`, `validate`, escáner IaC, plan explícito de solo lectura y diff; `plan` puede contactar AWS y generar archivos, no ejecutar sin permiso.
5. Bloquear recursos destructivos, costos no autorizados, plan incompleto o inconsistencia con SDD; jamás `apply` automático.

## Comandos y controles seguros

En H08 autorizado: `terraform fmt -check -recursive` y `terraform validate` sobre root preparado; nunca `terraform apply`, `terraform destroy` ni `terraform init` en H03.

## Criterios de error y escalamiento

Plan que destruye datos, segunda instancia simulada, uso accidental de state legado, SCP bypass o costos no resueltos -> `BLOCKED`.

## Trazabilidad mínima

GOV-01..05, GOV-10..16, R-023..029, R-036..038, R-040..045.

## Contrato transversal de seguridad y gobierno

- **Autoridad:** leer `constitution.md` (`GOV-01..18`, `R-001..060`), `AGENTS.md` y `memoria.md` antes de usar la skill. Si falta una fuente, detener la acción y registrar `BLOCKED`.
- **Fase única:** H03-D solo crea instrucciones y verifica estructura; H04/H05/H06 son especificación por fases; H07 publicación autorizada; H08+ implementación aprobada. Nunca crear documentos de fases no abiertas ni código o infraestructura por invocar una skill.
- **Estado inicial:** la skill es **documentación** y no se ejecuta automáticamente. `opencode.json` y los ocho agentes mantienen `skill: deny`, `shell: deny`, `subagent: deny` y prohibiciones de operaciones mutantes. Ningún paso de esta skill eleva permisos o autoriza romper esas denegaciones.
- **Privacidad:** nunca abrir, copiar o imprimir `.env`, claves/token JWT, credenciales AWS/SSH, `.tfstate`, parámetros secretos, dump financieros ni respaldos. Nunca invocar proveedores de pago ni servicios externos por defecto.
- **Decisiones:** un agente no autoaprueba; toda excepción, costo, cambio de alcance o mutación requiere autorización humana específica. Distinguir `DRAFT`, `REVIEW`, `APPROVED` y `NOT_RUN` de resultados realmente verificados.
- **Comandos seguros:** los citados son ejemplos de validación para el **responsable humano** en PowerShell 7, únicamente en la fase en que estén permitidos. No autorizan ejecutar shell desde OpenCode.

## Formato obligatorio del resultado

```text
SKILL / FASE / ESTADO DOCUMENTAL:
OBJETIVO, FUENTES Y VERSIONES:
REQUISITOS (GOV/R y fuente del producto/docente):
ENTRADAS / RUTAS PERMITIDAS / RUTAS EXCLUIDAS:
VALIDACIONES: comando | alcance | PASS/FAIL/BLOCKED/NOT_RUN | evidencia
RIESGOS / DECISIONES ABIERTAS:
AUTORIZACIÓN HUMANA REQUERIDA:
SIGUIENTE PASO ÚNICO:
```
