# Contrato de inicio H08-A — Arquitectura Go local

## Ratificación y límites
La instalación exige escribir explícitamente `APRUEBO_H06_H07_Y_AUTORIZO_H08A_LOCAL` ante el instalador. El comando no se ejecuta por OpenCode; constituye aceptación humana de la apertura documental **solo** para H08-A. Queda registro en commit y salida de Warp. Si no se ejecuta, este documento no autoriza acciones.

- H06: se ratifica el **baseline documental** de 75 operaciones (33+23+12+7), de las que las 7 opcionales no son automáticamente obligatorias. Un endpoint nuevo E076+ requiere RFC. Validación de implementación, fuente docente y maqueta real siguen pendientes; no ocultar esa limitación.
- H07: se ratifica el SDD publicado en repo privado, PR #1, `main` `dc23e9bc9b432631ce5346adc201cd96851edecc`, GitHub Actions `success` run `42138280237`, con evidencia proporcionada por responsable. El CI futuro debe pasar en PR y main.
- H08-A: solo implementación local de **arquitectura general A-000..A-110** con Go, Clean Architecture, pruebas y control humano por cada spec. El primer objetivo es A-000; no iniciar A-010 hasta aprobación del responsable.

## Prohibiciones duras
- Ningún acceso AWS, `terraform plan/apply`, gasto, Docker/WSL, despliegue, GitHub mutante automático, creación de infraestructura, editar documentos SDD sin RFC, tocar Mobile o terceros.
- H05 permanece DOCUMENTAL_CONDICIONADO. AWS requiere cotización oficial Redis, permisos VPC Origin/IAM, TLS, saldo/vencimiento e impuestos. **Nunca Paid y 0 MXN personales**.
- No permitir accesos a `.env`, `.aws`, `.ssh`, `.pem`, `tfstate`, secretos, datos financieros privados. Shell queda ASK para operaciones locales, no sandbox físico; auditar manualmente comandos y no seleccionar «Allow always».

## Rutas de edición de código permitidas H08-A
`cmd/**`, `internal/**`, `pkg/**`, `tests/**`, `go.mod`, `go.sum`; cada mutación `ask`. El resto de fuentes, configs, SDD y CI no son editables por agentes. Si se necesita otro archivo se tramita RFC, no se suavizan permisos.

## Modelo de trabajo obligatorio
1. Crear rama de feature desde main verificado (operador humano).
2. Orquestador lee A-000 completa, traza GOV/R, propone diseño + archivos + pruebas (sin escribir en la primera intervención).
3. Responsable aprueba una spec a la vez; agente Go implementa con confirmaciones por archivo y comando.
4. QA verifica pruebas y límites, guardian comprueba contrato; operador crea PR manual, espera CI y aprueba merge.
5. Cambios futuros de H08-I y H08-E necesitan RFC/Gate nuevos y permisos por fase, no se liberan con este acuerdo.

**Evidencias que NO existen todavía:** pruebas del motor de permisos en el Windows 11 del usuario después de aplicar este paquete, Go compila, HTTP público, AWS desplegada; estados NOT_RUN hasta ejecutar allí.
