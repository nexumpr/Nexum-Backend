# Ratificación humana H06, H07 y apertura limitada H08-A

**Estado tras ejecutar instalador con frase expresa:** H06 `APPROVED_DOCUMENTAL_BASELINE`, H07 `APPROVED_DOCUMENTAL_PUBLICADO`, H08-A `AUTHORIZED_LOCAL_ONLY`; H08-I y H08-E `BLOCKED_PENDING_PHASE_GATE`.

**Evidencia textual de ratificación:** el humano ejecuta `py -3.13 <ZIP> --instalar --ratificar APRUEBO_H06_H07_Y_AUTORIZO_H08A_LOCAL` en su Windows, con GitHub actor `nexumpr`, repo privado y baseline limpio. El instalador se niega a instalar o publicar si falta la frase. El registro es el comando en Warp y el commit H08-A revisado; NO constituye firma criptográfica ni autorización AWS.

**Evidencia histórica externa compartida:** commit `dc23e9bc9b432631ce5346adc201cd96851edecc`, PR #1 corregido, run principal `42138280237` success, 103 specs/412 docs, OpenAPI 75 e integridad SHA Windows/Linux PASS (5-nov-2026). **No se revalidó contra APIs de GitHub desde el ChatGPT del usuario.**

**Limitaciones formalmente aceptadas:** fuente docente PDF y prototipo Android no cotejados nuevamente; los 75 endpoints son baseline extensible; Redis precio/eligibilidad, permisos CloudFront, IVA, datos y TLS siguen pre-deploy BLOCKED. No hay código Go en marcha ni servicio AWS; nunca marcarlos VERIFIED.

**Gate siguiente:** H08-A/A-000 primer PR local. Los demás A en orden con autorización humana por spec; I-AWS y E-endpoints necesitan Gates propios.
