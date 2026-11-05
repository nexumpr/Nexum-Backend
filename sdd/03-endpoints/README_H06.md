# H06 — Contratos íntegros de Backend Nexum

**Estado:** REVIEW / a la espera de aprobación humana de Gate H06. **Fuente:** H04 `TRAZABILIDAD_75_RUTAS.md` y catálogo previo Nexum V6.0.2. El prototipo original actualizado y PDF docente NO se volvieron a adjuntar en esta ronda; la trazabilidad es documental a las fuentes retenidas y deberá cotejarse con originales en H07 cuando estén disponibles. Las rutas **no están implementadas**.

## Inventario y artefactos

- **75** operaciones propuestas, **33 M / 23 S / 12 E / 7 O**, con IDs estables `E001..E075`; cada operación cuenta con `spec.md`, `plan.md`, `tasks.md`, `checklist.md` (**300 archivos**).
- `openapi/nexum-api-v1.yaml` y `openapi/nexum-api-v1.json`: mismo contrato OpenAPI 3.1, esquemas tipados, error uniforme, ejemplos sintéticos, seguridad, rutas, parámetros y códigos.
- `MATRIZ_OPERACIONES_H06.csv`: alcance, rutas, modelo, auth, resultado, idempotencia y carpeta trazados a H04.
- `DYNAMODB_ACCESOS_H06.md`, `POLITICAS_TRANSVERSALES_H06.md`, `IA_ML_Y_NOTIFICACIONES_H06.md`, `PRUEBAS_CONTRATOS_H06.md`, `MATRIZ_COBERTURA_H06.md`, `SECUENCIA_IMPLEMENTACION_H08_REFERENCIA.md` y `GATE_H06_PENDIENTE.md`.
- `herramientas/validar_h06.py`: valida catálogo, carpetas y contrato JSON con librería estándar.
- `APROBACION_H05_DOCUMENTAL_CONDICIONADA_20261105.md` en H05 registra aprobación del usuario sin autorizar despliegue ni declarar costos/permisos verificables.

## Invariantes rectores

Backend Go Clean Architecture / AWS Ohio, DynamoDB, Redis real compartido, JWT/revocación, HTTPS y dos nodos separados; IaC/CI/CD 100% automatizables con autorización humana. Cero dinero personal, no AWS Paid, cero permisos de escritura para H06. Una funcionalidad nueva por release autorizado en H08+; nunca depender del equipo móvil para un Gate.

**Interpretación del catálogo:** M=maqueta, S=soporte, E=esperada o simulada, O=opcional histórico. E/S que atienden exigencia académica son implementaciones obligatorias, no demos. Las 7 O están descritas completamente pero su implementación futura requiere decisión explícita en Gate/release, excepto cuando ya sea requisito docente ratificado.

## Validación documental

Desde el repo local: `py -3.13 sdd/03-endpoints/herramientas/validar_h06.py`. No se necesitan AWS CLI, GitHub, contenedores, Docker, WSL ni librerías externas.

**Significado del resultado:** un `PASS` documental confirma integridad y coherencia de contratos, **nunca HTTP en ejecución, DynamoDB, Redis, AWS o costos**.
