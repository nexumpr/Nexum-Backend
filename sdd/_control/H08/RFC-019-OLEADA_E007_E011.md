# RFC-019 — Excepción de gobierno para oleadas concurrentes de endpoints (E007–E011)

| Campo | Valor |
|---|---|
| Documento | `sdd/_control/H08/RFC-019-OLEADA_E007_E011.md` |
| Versión | 1.0.0 |
| Fecha | 2026-11-17 |
| Estado | **PROPUESTO — no vigente hasta la frase de aprobación** |
| Autoridad que la motiva | Instrucción humana de la sesión de trabajo del 2026-11-17: «AUTORIZO INICIAR EL DESARROLLO CONTROLADO DE E007, E008, E009, E010 Y E011», con objetivo de entrega funcional en AWS el 2026-11-20 |
| Aprobador | Responsable de Nexum-Backend |
| Frase de aprobación | `APRUEBO_RFC_019_OLEADA_E007_E011` |

## 1. Problema que resuelve

El gobierno vigente contiene dos reglas que impiden materialmente la oleada solicitada:

| Regla | Ubicación | Efecto literal |
|---|---|---|
| «Una spec técnica a la vez» / «Solo un lote está activo» | `AGENTS.md` §«Operación autónoma», `constitution.md` R-007 | Impide desarrollar E008..E011 mientras E007 no esté cerrado |
| «Una funcionalidad o endpoint nuevo por release autorizado» | `constitution.md` §14; tabla de las 71 specs `sdd/03-endpoints/**/spec.md` | Impide un único release con cinco endpoints |

Ninguna de las dos es una regla de seguridad ni de integridad de datos: son reglas de
**secuenciación** pensadas para reducir el riesgo de trabajo no revisado. La instrucción humana
del 2026-11-17 pide explícitamente tres líneas concurrentes y una publicación agrupada, y pide
resolver la limitación «explícitamente antes de publicar una release agrupada». Esta RFC es ese
instrumento.

## 2. Alcance de la excepción (lo que autoriza)

### E-1. Desarrollo concurrente de specs

Se permite mantener **varias specs de endpoint en desarrollo simultáneo**, cada una en su propio
worktree Git y su propia rama, siempre que:

1. cada endpoint tenga su **PR independiente**;
2. cada PR tenga **Gate propio** (`GATE_H08E_E00N.md`) con su alcance local;
3. no se compartan archivos entre ramas sin coordinación del orquestador (§4);
4. el orquestador consolide `memoria.md` y los manifiestos globales.

### E-2. Release agrupado

Se permite **una sola release con varios endpoints** en lugar de una release por endpoint, siempre
que se cumplan **todas** estas condiciones:

1. todos los endpoints del release están **fusionados en `main`** y con CI verde sobre el `main`
   definitivo;
2. existe **un único artefacto ARM64 canónico** generado por el CI de ese `main`, con SHA-256
   verificado contra `dist/binario.sha256`;
3. existe **un único plan Terraform guardado** con SHA-256 calculado y verificado, que solo
   modifique Launch Template e IAM de artefactos (o nada, si no hay cambios);
4. el `apply` se autoriza con **frase humana que cita el SHA-256 del plan**, no con una
   autorización genérica;
5. existe **un único `instance refresh`** con `MinHealthyPercentage=100` y capacidad transitoria
   declarada;
6. cada endpoint conserva su **evidencia HTTPS individual** en el Gate de despliegue;
7. el Gate de despliegue registra, por endpoint, el estado `DEPLOYED` / `AWS_VERIFIED` separado.

### E-3. Reutilización

La excepción es **reutilizable** para oleadas posteriores (E012+) siempre que el responsable la
invoque por su identificador (`RFC-019`) y el alcance de la nueva oleada se declare por escrito
antes de empezar. No requiere una RFC nueva por oleada; sí requiere un Gate por oleada.

## 3. Lo que esta RFC NO autoriza

- No autoriza saltarse el **Gate de merge** por HEAD SHA de cada PR.
- No autoriza saltarse el **Gate de apply** por SHA-256 del plan.
- No autoriza `terraform apply`, `destroy`, `import` o `state` sin Gate específico.
- No autoriza modificar E001–E006 ni sus contratos.
- No autoriza crear recursos AWS nuevos (DynamoDB, Redis, SES, NAT, ALB, CloudFront, EC2
  adicionales).
- No autoriza relajar ninguna regla `deny` de `opencode.json` ni de la política de herramientas.
- No autoriza desplegar sin CI verde, ni declarar `AWS_VERIFIED` sin prueba HTTP real.
- No autoriza a los agentes aprobar en nombre del responsable.
- No modifica `constitution.md` (sigue requiriendo su propio procedimiento de versión) ni los
  `spec.md`/`plan.md` aprobados.

## 4. Reglas de convivencia entre líneas concurrentes

| Área | Propietario | Regla |
|---|---|---|
| Dominio, aplicación, adaptadores y pruebas de cada endpoint | agente de su línea | libre dentro de su worktree |
| `cmd/server/main.go`, `cmd/server/e00*.go` | orquestador | cada línea entrega su función `montarE00N` y el orquestador la conecta |
| `internal/transport/http/router.go` | orquestador | E009 propone el soporte de rutas parametrizadas; el orquestador lo integra con pruebas de regresión |
| `sdd/03-endpoints/openapi/**` | orquestador | solo si una diferencia real lo exige, con RFC |
| `memoria.md`, `BASELINE_SHA256.json` | orquestador | un solo escritor |
| Terraform, IAM, ASG, S3 | orquestador | nunca desde una línea de endpoint |

## 5. Riesgos y mitigación

| Riesgo | Mitigación |
|---|---|
| Conflictos de merge entre ramas | PRs pequeños y tempranos; orquestador fusiona en orden y verifica compatibilidad con el `main` actualizado |
| Release agrupado que oculta un endpoint roto | evidencia HTTPS **individual** por endpoint en el Gate de despliegue |
| Capacidad `t4g.micro` insuficiente en `us-east-2b` | rollback preparado, disponibilidad nunca por debajo de dos instancias sanas sin autorización explícita |
| Deriva documental entre ramas | `memoria.md` y manifiestos solo los escribe el orquestador |
| Autorización implícita | toda excepción exige frase exacta; nada se infiere del silencio |

## 6. Vigencia y revocación

- Entra en vigor con la frase `APRUEBO_RFC_019_OLEADA_E007_E011`.
- Se revoca con `REVOCO_RFC_019` o al cerrarse la entrega del 2026-11-20, lo que ocurra antes.
- La revocación no invalida trabajo ya fusionado ni evidencias ya registradas.

## 7. Registro

| Fecha | Cambio | Estado |
|---|---|---|
| 2026-11-17 | Creación. Excepción E-1 (concurrencia) y E-2 (release agrupado) para E007–E011 | PROPUESTO |
