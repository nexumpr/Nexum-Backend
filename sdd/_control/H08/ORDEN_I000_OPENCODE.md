# Orden ejecutable para OpenCode — I-000 (NO MÁS PLANES REITERADOS)

Agente: `nexum-orchestrator`. Fase: H08-I. Spec activa: **I-000**, ninguna otra.

1. Lee constitution.md, AGENTS.md, memoria.md, Gate H08-I y los cuatro archivos I-000. Comprueba `git status` y que estás en la rama de gobierno. Si los cambios del instalador están staged, **ejecuta tú** validación SDD, pruebas de permisos, commit, `git push` de esta rama, crea PR con `gh pr create`, comprueba CI, y solicita aprobación humana para merge (después hazlo tú). NO devuelvas comandos para que el usuario los teclee.
2. Tras integrar la apertura, desde main limpio crea tú `feat/h08i-i000-finops`; presenta plan I-000 resumido y solicita una sola aprobación humana de la spec; luego implementa directamente en `infra/finops/**` y `tests/infra/finops/**` respetando SDD.
3. Realiza todas las comprobaciones locales y consultas AWS **solo lectura** autorizadas. Emite evidencias sanitizadas y costos correctos, no publiques cuenta, ARNs, tokens ni credenciales. Si falta capacidad, registra BLOCKED; no inventes PASS. Soluciona errores de pruebas. Crea commit, push y PR por tu cuenta, verifica GitHub Actions y detente para aprobación humana del cierre I-000.
4. Prohibido avanzar I-010, ejecutar terraform init/plan/apply/destroy, mutaciones AWS o actualizar a Paid. No pedir al usuario comandos que puedes ejecutar con shell. No repetir la documentación completa del SDD, una spec a la vez. Respeta ASK y solicita aprobación de herramientas cuando corresponda.
