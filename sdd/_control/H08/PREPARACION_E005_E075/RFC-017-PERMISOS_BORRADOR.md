# RFC-017 — Permisos permanentes de OpenCode para E005–E075

**DRAFT / SIN APROBACIÓN / NO APLICADO.** Separado de RFC-016 de contratos.

## Alcance de autorización propuesta
- Conservar archivo local de Windows y reconciliarlo con `main` y candidato de esta preparación.
- Normalizar la política V2: `permissions` ordenadas con `ask` en acciones sensibles; `policies` para prohibiciones duras, sin `allow` que sugiera un permiso inexistente.
- Autorizar **solicitud interactiva** para `terraform apply`, publicación S3, STS rol de publicación, ASG y edición SDD protegida; siempre exigir Gate/RFC humano separado. ASK NO ES AUTORIZACIÓN AWS.
- Prohibiciones duras: destruir Terraform, eliminar tablas/instancias, push directo main, force push, secretos, claves, tfstate/tfvars, Docker y WSL.
- Lecturas AWS permitidas con mínimo privilegio y solo comandos `describe/list` específicos; consulta DynamoDB con datos personales continúa ASK.

## Riesgos y controles
- En permisos V2 la última regla coincidente prevalece; se probará con motor `opencode debug config` en Windows, no solo simulación Python.
- La eliminación de `policies` hard deny de `terraform apply` requiere aprobación expresa de este RFC; no se cambiará globalmente ni se concederán privilegios IAM más amplios.
- Si durante pruebas se detecta `deny` institucional superior, detener, no intentar eludirlo; registrar limitación.
- La configuración permanente no reduce los Gates por SHA40/plan SHA256/releases/S3/ASG.

**Resultado solicitado:** aprobar exclusivamente integrar una configuración de permisos interactiva y sus tests, sin AWS mutante y sin tocar código Go. La aprobación de RFC-017 no autoriza `terraform apply`, `instance refresh`, PR merge ni despliegues.
