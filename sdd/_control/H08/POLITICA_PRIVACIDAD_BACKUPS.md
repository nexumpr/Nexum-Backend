# Política de privacidad de datos respaldados — I-130 (INF-130-05)

| Campo | Valor |
|---|---|
| Spec | `I-130-backup-recuperacion` |
| Versión | 0.1.0 |
| Estado | **IMPLEMENTADO_LOCAL — NO DESPLEGADO** |
| Ratificación de plazos | **PENDIENTE** |

## 1. Siete conceptos que no se confunden

| # | Concepto | Comportamiento implementado | Afirmación permitida |
|---|---|---|---|
| 1 | Eliminación de cuenta solicitada | Se registra la solicitud con fecha | Sí |
| 2 | Desactivación de acceso | Sesiones revocadas dejan de ser válidas | Sí |
| 3 | Eliminación de datos activos | Borrado de los ítems activos | Sí |
| 4 | Expiración de copias | PITR caduca ≤ 35 d; backup según retención | Sí, «retención documentada» |
| 5 | Retención por obligación aplicable | Se documenta; el plazo lo ratifica el responsable | Solo como propuesta |
| 6 | Eliminación definitiva | Tras expirar todas las copias | Sí, condicionada a 4 |
| 7 | Restauración de datos eliminados | **Bloqueada**: una copia antigua no reactiva cuentas ni sesiones | Sí, verificado por prueba |

## 2. Regla crítica

Una restauración **no** puede devolver a la vida cuentas eliminadas ni sesiones revocadas. El validador
`verificar_integridad_financiera.py reactivacion` compara el dataset restaurado con el registro de
eliminaciones y revocaciones:

- `CUENTA_ELIMINADA_REACTIVADA` si una cuenta eliminada aparece activa;
- `SESION_REVOCADA_REACTIVADA` si una sesión revocada aparece válida.

Ambas son violaciones bloqueantes (código 1).

## 3. Conciliación de eliminaciones

Tras un restore aislado se debe ejecutar una conciliación que:

1. aplique de nuevo las eliminaciones posteriores al punto de restauración;
2. revoque las sesiones emitidas y revocadas después de ese punto;
3. registre cada reaplicación con evidencia;
4. no publique datos personales en el informe.

El mecanismo de conciliación con datos reales depende de A-070 (borrado de cuenta) y de los endpoints
correspondientes; **no se modifica ningún endpoint E001–E075** desde I-130. Se registra la dependencia
para su Gate.

## 4. Lo que no se afirma

- No se declara borrado irreversible mientras existan copias dentro de retención.
- No se fijan plazos de conservación: quedan como propuesta hasta ratificación humana.
- No se declara cumplimiento legal: solo se documenta la política propuesta.
