# `egreso_fcm` — propuesta de egreso HTTPS, deshabilitada

Módulo de preparación para el egreso a `oauth2.googleapis.com` y
`fcm.googleapis.com`. **No crea recursos por defecto** (`habilitar = false`) y
**no está instanciado** en `envs/h08i`.

## Qué crearía si se habilita

1. Una o dos subredes públicas dedicadas, sin `map_public_ip_on_launch`.
2. Route table pública con `0.0.0.0/0` al IGW ya existente de I-010.
3. EIP + NAT Gateway por AZ (1 o 2).
4. Ruta por defecto de cada route table privada al NAT correspondiente.
5. Opcionalmente, una regla de egreso TCP 443 a `0.0.0.0/0` en el SG del backend
   (`habilitar_regla_egreso_backend = true`), que Seguridad debe aprobar.

No crea EC2, no abre SSH, no asigna IPv4 pública a instancias y no modifica el
ALB, CloudFront, DynamoDB, Redis ni el bucket de secretos.

## Coste y decisión

La comparativa y la recomendación condicionada están en
`sdd/_control/H08/PROPUESTA_EGRESO_FCM.md`. El módulo no debe aplicarse sin
`APRUEBO_GATE_FINOPS_FCM` y sin un plan Terraform revisado por SHA-256.

## Limitación de seguridad

NAT Gateway no permite allowlist por dominio. La regla de egreso 443 es un
riesgo residual; para restringir hosts hace falta un proxy de egreso con
allowlist (opción comparada en la propuesta).
