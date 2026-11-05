---
name: Assess AWS Cost Nexum
description: "Analizar factibilidad verificable del costo de uso cero en AWS Free con 2 instancias reales, balanceador y Redis."
metadata:
  opencode/autoinvoke: false
---

# Skill — assess-aws-cost

**Estado:** H03-D / REVIEW. **Invocación:** no automática; la carga sigue técnicamente denegada hasta cambio autorizado y comprobado. No crea código, specs ni servicios.

## Disparador y objetivo

Ante cualquier decisión AWS que pueda cobrar, especialmente H05/I-000 y posterior autorización de despliegues.

## Entradas requeridas

- Documento del profesor resumido o cotejado, plan Free y SCP documentados.
- Precios oficiales de Ohio con fecha y restricciones del plan; saldo observado con fecha solo como antecedente.
- Tiempo de encendido propuesto y topología completa con servicios asociados.

## Precondiciones y fase

No inventar precios; USD 120 observados el 2026-11-05 no son saldo vigente ni prueban costo de uso cero.

## Alcance y archivos autorizados

En H03 solo se define procedimiento; H05 compara costos sin crear recursos; H08+ permisos de consulta/facturación o cambios siempre específicos.

## Flujo operativo detallado

1. Separar costo **bruto de uso** de saldo/factura neta cubierta por créditos.
2. Listar cómputo dos nodos, LB/LCU, IPv4, EBS, Redis, DynamoDB, datos, CloudWatch, transferencia, DNS y presupuestos.
3. Consultar páginas oficiales por región/fecha y estado Free cuando el bloque H05 esté autorizado.
4. Construir escenarios 45 días: runtime temporal, continuo, costos residuales OFF y reserva; supuestos explícitos y sensibilidad.
5. Si es imposible cumplir costo cero y dos instancias, bloquear ADR y elevar decisión al responsable/profesor; nunca degradar requisito o activar Paid.

## Comandos y controles seguros

En H05 consultar documentación oficial y reportes financieros con acceso específicamente autorizado; **no** usar `aws` ni consultas de facturación en H03.

## Criterios de error y escalamiento

No hay precios confiables, plan incompatible, saldo desconocido o costo bruto positivo no consentido -> `BLOCKED`.

## Trazabilidad mínima

GOV-02..05, GOV-10, GOV-15, R-023..029, R-036..038, R-049.

## Contrato transversal de seguridad y gobierno

- **Autoridad:** leer `constitution.md` (`GOV-01..18`, `R-001..060`), `AGENTS.md` y `memoria.md` antes de usar la skill. Si falta una fuente, detener la acción y registrar `BLOCKED`.
- **Fase única:** H03-D solo crea instrucciones y verifica estructura; H04/H05/H06 son especificación por fases; H07 publicación autorizada; H08+ implementación aprobada. Nunca crear documentos de fases no abiertas ni código o infraestructura por invocar una skill.
- **Estado inicial:** la skill es **documentación** y no se ejecuta automáticamente. `opencode.json` y los ocho agentes mantienen `skill: deny`, `shell: deny`, `subagent: deny` y prohibiciones de operaciones mutantes. Ningún paso de esta skill eleva permisos o autoriza romper esas denegaciones.
- **Privacidad:** nunca abrir, copiar o imprimir `.env`, claves/token JWT, credenciales AWS/SSH, `.tfstate`, parámetros secretos, dump financieros ni respaldos. Nunca invocar proveedores de pago ni servicios externos por defecto.
- **Decisiones:** un agente no autoaprueba; toda excepción, costo, cambio de alcance o mutación requiere autorización humana específica. Distinguir `DRAFT`, `REVIEW`, `APPROVED` y `NOT_RUN` de resultados realmente verificados.
- **Comandos seguros:** los citados son ejemplos de validación para el **responsable humano** en PowerShell 7, únicamente en la fase en que estén permitidos. No autorizan ejecutar shell desde OpenCode.

## Formato obligatorio del resultado

```text
SKILL / FASE / ESTADO DOCUMENTAL:
OBJETIVO, FUENTES Y VERSIONES:
REQUISITOS (GOV/R y fuente del producto/docente):
ENTRADAS / RUTAS PERMITIDAS / RUTAS EXCLUIDAS:
VALIDACIONES: comando | alcance | PASS/FAIL/BLOCKED/NOT_RUN | evidencia
RIESGOS / DECISIONES ABIERTAS:
AUTORIZACIÓN HUMANA REQUERIDA:
SIGUIENTE PASO ÚNICO:
```
