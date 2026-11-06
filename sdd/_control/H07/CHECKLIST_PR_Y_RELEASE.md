# Plantilla PR (arquitectura → infraestructura → endpoints)

- [ ] Referencia exacta A-xxx / I-xxx / E-xxx, requerimiento GOV/R, archivo fuente y alcance. ¿Cambio de SDD? adjuntar RFC y Gate humano.
- [ ] Plan de pruebas reproducible sin secretos; tests PASS reales / NOT_RUN declarados.
- [ ] `validar_sdd_integral.py` PASS, `validar_h06.py` PASS, sin deriva de baseline no autorizada.
- [ ] Revisión de dependencias, JWT, dinero exacto, DynamoDB, Redis, idempotencia, privacidad, ML si aplica.
- [ ] Si modifica AWS: terraform plan revisado, Free/FinOps/costo/prerrequisitos reales, IAM, estado y rollback, autorización de apply, nunca Paid.
- [ ] Revisor distinto del autor y aprobación explícita de merge; una operación funcional nueva por release.
- [ ] Git status/staging no incluye respaldo, estado Terraform, secretos, JSON de auditoría privada ni datos de usuario.
- [ ] Evidencia HTTP real + dos instancias y DynamoDB/Redis cuando corresponda; si no ha corrido, `NOT_RUN`.

**Rama base protegida:** `main`; ramas de funcionalidad `feature/A-*`, `feature/I-*`, `feature/E-*`. No merge directo, no force push, no bypass de checks sin RFC/Gate.
