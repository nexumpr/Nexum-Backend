# `egreso_fcm` — NAT Gateway administrado en 1 AZ (deshabilitado por defecto)

Módulo de preparación para el egreso a `oauth2.googleapis.com` y
`fcm.googleapis.com`. **No crea recursos por defecto** (`habilitar = false`):
todos los recursos tienen `count = 0`. En `envs/h08i` el bloque `module
"egreso_fcm"` también nace con `count = 0`; activarlo es cambiar
`habilitar_egreso_fcm` a `true`, no reescribir el módulo.

## Qué crea cuando se habilita

1. Una **subred pública nueva y dedicada** al NAT en una sola AZ
   (`egreso_fcm_az`, por defecto `us-east-2a`), con
   `map_public_ip_on_launch = false`.
2. Route table pública con `0.0.0.0/0` al IGW **existente** de I-010. El IGW
   ya está adjunto porque CloudFront VPC Origins lo exige, pero antes no tenía
   ruta por defecto.
3. Un EIP y **un único** NAT Gateway (`connectivity_type = "public"`).
4. Ruta `0.0.0.0/0 -> NAT` en las route tables privadas de I-010 que se pasen
   (`private_route_table_ids`). Los gateway endpoints S3/DynamoDB conservan la
   prioridad por prefijo más específico.
5. Regla de egreso **TCP 443** en el SG del backend. Si se aporta una prefix
   list real (`prefix_list_egreso_https_id`), se usa; si no, se abre a
   `cidr_egreso_https` (por defecto `0.0.0.0/0`). **No existe una prefix list
   administrada por AWS para Google FCM/OAuth y este módulo no la inventa.**

No crea EC2, no abre SSH, no asigna IPv4 pública a las instancias y no modifica
el ALB, CloudFront, DynamoDB, Redis ni el bucket de secretos. Las dos EC2 del
backend siguen privadas y el ALB sigue interno.

## DNS

La VPC de I-010 declara `enable_dns_support = true` y
`enable_dns_hostnames = true`. Sin ambos, los nombres públicos de Google no
resolverían. Este módulo no muta la VPC; la comprobación está en
`tests/infra/terraform/test_egreso_fcm_contrato.py`.

## Coste y decisión

La comparativa, la tabla FinOps con horizonte de 8 y 45 días y los precios
`NOT_VERIFIED` están en
`sdd/_control/H08/PROPUESTA_EGRESO_FCM.md` y
`sdd/_control/H08/MATRIZ_FINOPS_EGRESO_FCM.md`. El módulo no debe aplicarse sin
Gate FinOps/Seguridad y sin un plan Terraform revisado por SHA-256.

## Límites de seguridad y disponibilidad

- **Punto único de fallo:** un solo NAT en una sola AZ. Si esa AZ falla, el
  egreso FCM/OAuth de ambos nodos se pierde aunque las EC2 sigan sanas.
- NAT no permite allowlist por dominio. La regla 443 a Internet es un riesgo
  residual; para restringir hosts hace falta un proxy de egreso con allowlist
  (opción comparada en la propuesta).
- El EIP del NAT es una IPv4 pública en uso. Su facturación se modela por
  separado y queda `NOT_VERIFIED` en la tabla.
