# Estado único y Gates — Nexum-Backend (activación H08-A)

**Estado aplicable solamente tras ejecución explícita de la frase de ratificación del ZIP H08-A y CI en main.** Nunca asumir que el archivo por sí solo firma o ejecuta el Gate.

| Lote | Estado | Límite |
|---|---|---|
| H03 | APPROVED | Constitución, 8 agentes, 9 skills base; RFC-001 adapta perfiles en H08-A |
| H04 | APPROVED | 12 specs A-000..A-110 sin cambios de contenidos |
| H05 | APPROVED_DOCUMENTAL_CONDICIONADO | I-000..I-150 intactas; AWS real bloqueada hasta FinOps, IAM, TLS y despliegue autorizado |
| H06 | APPROVED_DOCUMENTAL_BASELINE | 75 contratos iniciales, posible ampliación RFC; fuentes externas no cotejadas de nuevo; implementación NO verificada |
| H07 | APPROVED_DOCUMENTAL_PUBLICADO | PR #1 y `main` dc23e9b, GitHub Actions success run 42138280237 (evidencia del usuario) |
| H08-A | AUTHORIZED_LOCAL_ONLY | Permisos mínimos Go, una spec por vez, primer objetivo A-000; AWS/GitHub mutante bloqueados |
| H08-I / H08-E | NOT_AUTHORIZED | Requerirán Gates independientes y nuevas políticas; 0 pesos personales |

El Gate H08-A se registra en `sdd/_control/H08/GATE_H06_H07_H08A_RATIFICACION.md`. Shell con ASK **no** es un sandbox: el usuario revisa cada orden. El registro de `main` debe seguir limpio; los cambios de H08-A se publican como PR y pasan checks.
