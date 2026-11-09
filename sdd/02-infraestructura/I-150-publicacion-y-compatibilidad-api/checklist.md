# I-150-publicacion-y-compatibilidad-api — Publicación y compatibilidad de contratos API

| Campo | Valor |
|---|---|
| Etapa | H05 — Infraestructura AWS |
| Versión | 0.1.0 |
| Estado | **DRAFT / REVIEW — NO APROBADO** |
| Trazabilidad | A-040, A-110, E001..E075, I-030, I-090 |
| Implementación | H08+, tras H07 aprobado y autorización humana por release |

## Checklist de revisión (cada punto requiere evidencia, no basta la presencia de este archivo)

### Integridad y autoridad

- [ ] H04 tiene aprobación humana y el contenido coincide con las reglas superiores.
- [ ] Se analizó el PDF docente v3 y el catálogo V6.0.2 sin asumir GCP/MongoDB.
- [ ] Se mantiene el alcance de Backend, no se exige validación Mobile.
- [ ] No se crearon recursos ni artefactos de código/EP antes de H08/H06.

### Decisiones técnicas

- [ ] `INF-150-01` **Contrato:** diseño justificado, referencias, supuestos y evidencia trazable.
- [ ] `INF-150-02` **Descubrimiento:** diseño justificado, referencias, supuestos y evidencia trazable.
- [ ] `INF-150-03` **Compatibilidad:** diseño justificado, referencias, supuestos y evidencia trazable.
- [ ] `INF-150-04` **Evidencia HTTP:** diseño justificado, referencias, supuestos y evidencia trazable.
- [ ] `INF-150-05` **Cambio:** diseño justificado, referencias, supuestos y evidencia trazable.
- [ ] `INF-150-06` **Alcance:** diseño justificado, referencias, supuestos y evidencia trazable.
- [ ] `INF-150-07` **CDN/moderación:** diseño justificado, referencias, supuestos y evidencia trazable.

### Pruebas y cobertura de diseño

- [ ] API accesible HTTPS y OpenAPI 3.1 con hash.
- [ ] Matriz endpoints E001..E075 categorizada y rastreable.
- [ ] Pruebas HTTP reales del Backend sin confirmación Mobile.
- [ ] 4 endpoints adicionales más registro/login/logout evaluables según PDF.
- [ ] Política CI de breaking changes y versionado de release.

### Riesgos, economía y aprobaciones

- [ ] Resuelto o escalado formalmente: Rutas opcionales asumidas obligatorias sin evidencia.
- [ ] Resuelto o escalado formalmente: Equipo Mobile convertido indebidamente en gate.
- [ ] Resuelto o escalado formalmente: Función de upload futura sin moderación/CDN.
- [ ] Costos potenciales (incluido apagado) identificados por servicio en I-000; no se afirma $0 por usar créditos.
- [ ] SCP y disponibilidad de servicios relevantes documentados (no simulados).
- [ ] Gate I-GO tiene resolución expresa del responsable, o esta spec continúa DRAFT/BLOCKED.

### Ejecución futura H08 (NO ejecutada en este lote)

- [ ] Pruebas AWS/CI/HTTP y sus evidencias ejecutadas realmente tras aprobación y release; no cambiar a PASS antes.
- [x] OpenAPI 3.1.0 con `/api/v1`, `traceId` y dinero como string.
- [x] Artefacto con checksum verificado por el validador.
- [x] CORS con allowlist; `*` prohibido.
- [x] Matriz de 75 operaciones con clases 33/23/12/7.
- [x] 13 pruebas HTTP en `NOT_RUN`; prohibido `PASS` sin AWS y sin huella.
- [x] SemVer, deprecacion y deteccion de breaking changes en CI.
- [x] Mobile y landing fuera del gate.
- [ ] HTTP real, TLS y dos nodos: `NOT_RUN` (requieren Gate de despliegue).
- [x] Paridad JSON/YAML/matriz con 75 operaciones y `operationId` trazados.
- [x] Matriz ampliada con pruebas positiva y negativa por operacion.
- [x] Comparador de breaking changes ejecutado en CI contra `origin/main`.
- [x] Cuatro niveles de evidencia HTTP separados; comandos `curl` generados sin ejecutar.
- [x] Frontera de responsabilidad documentada y validada.
- [x] Inventario de contenido: 0 medios y 3 operaciones con texto libre con criterio documentado.
- [x] Paridad semantica por operacion (metodo, ruta, operationId, seguridad, parametros, respuestas).
- [x] Breaking changes en respuestas y seguridad efectiva con scopes.
- [x] Excepciones solo con RFC ratificada verificable.
- [x] Moderacion: tres operaciones documentadas y `BLOCKED_DEPENDENCY` para servicio externo.
- [x] PyYAML fijado y usado con rechazo de claves duplicadas.
- [x] Paridad semantica de `components.schemas` YAML/JSON.
- [x] Excepciones de breaking changes solo con autorizacion humana especifica de API.
- [x] RFC de infraestructura (incluida RFC-006) rechazadas como excepcion de API.
