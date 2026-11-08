# I-060 — Secretos, rotación y Terraform state

| Campo | Valor |
|---|---|
| Estado | **RFC-004 PROPUESTO; mecanismo de entrega no aprobado** |
| Alcance | Registrar separación de responsabilidades y bloqueo; no implementar ni cargar secretos |

## Inventario/decisión

| Material | Estado actual | IAM concedido por I-060 |
|---|---|---|
| JWT signing private key (RS256, `kid`) | A-050 define el provider; fuente persistente/rotación en producción requiere secreto aprobado | Ninguno: sin `secretsmanager:GetSecretValue`, SSM o KMS |
| Redis RBAC password | I-050 usa password auth; RFC-004 no aprobó fuente de bootstrap | Ninguno: sin S3 secrets, Secrets Manager, SSM ni `elasticache:Connect` |
| Refresh tokens | El diseño guarda hashes/familias en DynamoDB; el token no es IAM secret | Ninguno adicional |
| Binario y unidad systemd | `s3://PENDIENTE-I080`; I-080 debe aprobar bucket y object keys | Variable vacía; `s3:GetObject` sólo se habilitará para ARN exactos de esos objetos |
| Terraform state / plan | Backend local en H08i; backend/locking real pertenece a I-070 | Roles OIDC sin permisos de state |

## Hallazgo Redis: la contraseña actual acabaría en state

La documentación versionada del provider AWS **5.100.0** para `aws_elasticache_user` declara que todos los
argumentos, incluidas contraseñas, se guardan en texto plano en el raw state. La marca `sensitive` sólo redacta
ciertas salidas; no evita state, planes guardados ni copias del state.

La documentación del provider 6.41.0 presenta `passwords_wo` como write-only, pero exige Terraform **1.11+**.
Este proyecto permite Terraform `>=1.4,<2.0` y bloquea AWS provider en **5.100.0**. No se actualizan provider,
Terraform, ni I-050 en el Gate I-060.

**Control operativo:** I-060 no crea state remoto ni ejecuta plan/apply; no pasar ningún valor real a
`redis_password_aplicacion`, variables de entorno de Terraform, `.tfvars` o CI. El acceso a la consola/página
de state debe tratarse como acceso a secretos si ese valor llegara a introducirse.

## RFC-004 (PROPUESTO, no aprobado)

I-060 **no** implementa ni selecciona opción A (Secrets Manager permanente) u opción B (bundle S3 SSE-S3). El
runtime no recibe permisos de secretos. Opciones, costos, rotación y riesgo residual permanecen en
`sdd/_control/H08/RFC-004-SECRETOS_BOOTSTRAP.md` hasta una decisión humana expresa.

## Rotación/revocación por etapa

1. **Antes del despliegue:** aprobar RFC-004 y resolver el almacenamiento Redis password fuera de Terraform state.
2. **Al habilitarlo:** definir quién genera/rota el secreto, su `kid`/versión y la revocación de la versión previa;
   no poner el valor en output, plan, logs, Git ni evidencia.
3. **Incidente:** revocar sesiones/roles temporales y rotar el secreto desde el servicio aprobado; el procedimiento
   debe preservar consistencia con las dos instancias del ASG.
4. **Verificación:** documentar sólo IDs/fechas enmascarados y prueba AWS real bajo Gate de despliegue.

No se afirma que exista actualmente una ruta de rotación ejecutable en AWS. El mecanismo real continúa bloqueado.
