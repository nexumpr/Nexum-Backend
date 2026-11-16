"""Clasifica TODAS las diferencias contractuales E005-E075 (sin dejar ninguna sin clase).

Entradas (solo lectura):
  - sdd/_control/H08/PREPARACION_E005_E075/matriz_e005_e075.json  (auditoria vs SDD)
  - sdd/03-endpoints/openapi/nexum-api-v1.json                    (contrato vigente)

Salida:
  - sdd/_control/H08/CLASIFICACION_E005_E075.json                 (trazabilidad por endpoint)

Clases: ESENCIAL | COMPATIBILIDAD | OPCIONAL | OBSOLETA.

Criterio (explicito, no heuristico oculto):
  1. Codigo de exito distinto  -> COMPATIBILIDAD (exige decision HTTP individual).
  2. Referencia `SESSIONHASH`  -> OBSOLETA (enmienda editorial transversal).
  3. Renombrado conocido        -> COMPATIBILIDAD.
  4. Base del campo YA expuesta por el contrato -> OPCIONAL (refinamiento de presentacion).
  5. Campo material declarado en OVERRIDES -> la clase indicada (ESENCIAL si sin el la
     operacion no puede cumplir su funcion de forma verificable).
  6. Resto -> OPCIONAL POSPUESTA: el contrato vigente cubre la funcion y el campo es
     refinamiento no requerido por el contrato aprobado.

`OPCIONAL` NO significa resuelto: significa POSPUESTO y registrado, reversible por RFC.
`ESENCIAL`/`COMPATIBILIDAD` pendientes impiden CONTRACT_FROZEN.
"""

from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path

MATRIZ = Path("sdd/_control/H08/PREPARACION_E005_E075/matriz_e005_e075.json")
OPENAPI = Path("sdd/03-endpoints/openapi/nexum-api-v1.json")
SALIDA = Path("sdd/_control/H08/CLASIFICACION_E005_E075.json")
SPECS = Path("sdd/03-endpoints")
PERSISTENCIA = ("E005", "E006", "E007", "E008", "E009", "E010", "E011")
PATRON_OBSOLETO = "SESSIONHASH"

# Codigos de exito: decision propuesta y motivo (no aprobada).
HTTP_DECISION = {
    "E017": ("201", "Crea un consentimiento: recurso nuevo. Conservar el contrato aprobado."),
    "E031": ("204", "Borrado idempotente sin cuerpo. Conservar el contrato aprobado."),
    "E037": ("200", "Conservar hasta confirmar con Gate que la operacion crea un movimiento."),
    "E042": ("204", "Borrado idempotente sin cuerpo. Conservar el contrato aprobado."),
    "E063": ("202", "Conservar 202 mientras no se demuestre que read-all es sincrono."),
}

# Renombrados que rompen el contrato aprobado si se adoptan.
RENOMBRADOS = {
    "E006": {"resetToken": "token"},
    "E075": {"transcript": "text"},
}

# Campos ESENCIALES: solo cuando el contrato vigente NO puede expresar la funcion y
# la necesidad esta demostrada por un diseno aprobado (no por la maqueta).
OVERRIDES: dict[str, tuple[str, str]] = {
    "E055:hasEnoughData": (
        "ESENCIAL",
        "El contrato no distingue 'sin historial suficiente' de 'sin patrones': items[] vacio es ambiguo. "
        "El diseno ML (RFC-016 6) exige un estado explicito de datos insuficientes.",
    ),
    # Preferencias de notificacion: la maqueta pide granularidad que 3 booleanos no expresan.
    "E058:enabled": ("COMPATIBILIDAD", "Sustituye al conjunto pushEnabled/emailEnabled/reminderEnabled del SDD."),
    "E058:budgetAlerts": ("COMPATIBILIDAD", "Preferencia granular sin equivalente en el contrato vigente."),
    "E058:goalUpdates": ("COMPATIBILIDAD", "Preferencia granular sin equivalente en el contrato vigente."),
    "E058:recurringReminders": ("COMPATIBILIDAD", "Preferencia granular sin equivalente en el contrato vigente."),
    "E058:streakReminders": ("COMPATIBILIDAD", "Preferencia granular sin equivalente en el contrato vigente."),
    "E058:quietHours": ("COMPATIBILIDAD", "Horario de silencio sin equivalente en el contrato vigente."),
    "E058:timeZone": ("COMPATIBILIDAD", "El contrato vigente no declara zona horaria del usuario."),
}

# Contenedores cuyo nombre o forma difieren del contrato aprobado.
ESTRUCTURALES = {
    "E055": ("COMPATIBILIDAD", "La auditoria usa recommendations[]; el SDD ya expone items[] + nextCursor. "
                               "Renombrar el contenedor rompe el cliente: se conserva items[]."),
    "E021": ("OPCIONAL", "Agregado de arranque: el SDD puede componerlo desde otros endpoints."),
    "E051": ("OPCIONAL", "Agregado de panel: refinamiento de presentacion."),
    "E074": ("OPCIONAL", "Datos de demostracion: no afecta al contrato de produccion."),
    "E020": ("OPCIONAL", "Forma del documento de exportacion: decision de producto."),
}


def cargar(ruta: Path):
    return json.loads(ruta.read_text(encoding="utf-8"))


def campos_del_esquema(op: dict, componentes: dict, codigo: str) -> set[str]:
    base: set[str] = set()

    def aplanar(sch, prefijo=""):
        if not isinstance(sch, dict):
            return
        if "$ref" in sch:
            nombre = sch["$ref"].split("/")[-1]
            if nombre not in componentes:
                return
            aplanar(componentes[nombre], prefijo)
            return
        for nombre, sub in (sch.get("properties") or {}).items():
            ruta = f"{prefijo}{nombre}"
            base.add(ruta)
            if isinstance(sub, dict):
                if "$ref" in sub:
                    nombre_ref = sub["$ref"].split("/")[-1]
                    if nombre_ref in componentes:
                        aplanar(componentes[nombre_ref], ruta + ".")
                else:
                    sub_tipo = sub.get("type")
                    if sub_tipo == "array" and isinstance(sub.get("items"), dict):
                        aplanar(sub["items"], ruta + ".")
                    elif isinstance(sub.get("properties"), dict):
                        aplanar(sub, ruta + ".")
        for comp in ("allOf", "oneOf", "anyOf"):
            for sub in sch.get(comp) or []:
                aplanar(sub, prefijo)

    respuestas = op.get("responses") or {}
    contenido = ((respuestas.get(codigo) or {}).get("content") or {}).get("application/json") or {}
    if isinstance(contenido.get("schema"), dict):
        aplanar(contenido["schema"])
    return base


def clasificar(raiz: Path) -> dict:
    matriz = cargar(raiz / MATRIZ)
    api = cargar(raiz / OPENAPI)
    componentes = (api.get("components") or {}).get("schemas") or {}
    operaciones = []
    resumen_clases = {"ESENCIAL": 0, "COMPATIBILIDAD": 0, "OPCIONAL": 0, "OBSOLETA": 0}

    for fila in matriz:
        eid = fila["id"]
        metodo = (fila.get("sdd_method") or "").lower()
        ruta = fila.get("sdd_path") or ""
        op = ((api.get("paths") or {}).get(ruta) or {}).get(metodo) or {}
        campos = campos_del_esquema(op, componentes, str(fila.get("sdd_success")))
        bases = {c.split(".")[0] for c in campos}

        diferencias = []

        def agregar(campo, tipo, clase, necesidad, decision, motivo):
            resumen_clases[clase] += 1
            diferencias.append(
                {
                    "campo": campo,
                    "tipo": tipo,
                    "clase": clase,
                    "necesidad_android": necesidad,
                    "decision": decision,
                    "motivo": motivo,
                }
            )

        # 1) Codigo de exito.
        if str(fila.get("pdf_success")) != str(fila.get("sdd_success")):
            objetivo, motivo = HTTP_DECISION.get(eid, (str(fila["sdd_success"]), "Sin decision registrada."))
            agregar(
                "http_success",
                "HTTP",
                "COMPATIBILIDAD",
                "El cliente distingue creacion, exito con cuerpo y borrado segun el codigo.",
                f"CONSERVAR {objetivo}",
                motivo,
            )

        # 2) Persistencia heredada.
        if eid in PERSISTENCIA:
            carpeta = next((p for p in (raiz / SPECS).glob(f"{eid}-*") if p.is_dir()), None)
            hits = 0
            if carpeta:
                hits = sum(md.read_text(encoding="utf-8", errors="replace").count(PATRON_OBSOLETO) for md in carpeta.glob("*.md"))
            if hits:
                agregar(
                    "persistencia",
                    "PERSISTENCIA",
                    "OBSOLETA",
                    "Ninguna: es referencia documental al modelo anterior.",
                    "SUSTITUIR por USER#id/SESSION#id y REFRESH#digest/SESSION",
                    f"{hits} referencias a {PATRON_OBSOLETO} contradicen el modelo verificado en E003/E004.",
                )

        for tipo, lista in (
            ("REQUEST", fila.get("pdf_only_request_candidates") or []),
            ("RESPONSE", fila.get("pdf_only_response_candidates") or []),
        ):
            for campo in lista:
                base = campo.replace("[]", "").split(".")[0]
                clave = f"{eid}:{campo.replace('[]', '')}"
                if campo.replace("[]", "") in (RENOMBRADOS.get(eid) or {}):
                    agregar(
                        campo,
                        tipo,
                        "COMPATIBILIDAD",
                        "El cliente envia el nombre del campo; renombrarlo rompe la integracion.",
                        f"CONSERVAR {RENOMBRADOS[eid][campo.replace('[]', '')]}",
                        "Renombrado respecto al contrato aprobado.",
                    )
                elif clave in OVERRIDES:
                    clase, motivo = OVERRIDES[clave]
                    agregar(campo, tipo, clase, "Ver motivo.", "DECIDIR EN RFC-016", motivo)
                elif base in bases:
                    agregar(
                        campo,
                        tipo,
                        "OPCIONAL",
                        "El contrato ya expone la misma base: el candidato es refinamiento.",
                        "POSPUESTA",
                        "La base del campo ya existe en el contrato aprobado.",
                    )
                elif eid in ESTRUCTURALES and tipo == "RESPONSE" and "." not in campo:
                    clase, motivo = ESTRUCTURALES[eid]
                    agregar(campo, tipo, clase, "Ver motivo.", "DECIDIR EN RFC-016", motivo)
                else:
                    agregar(
                        campo,
                        tipo,
                        "OPCIONAL",
                        "Refinamiento de presentacion no requerido por el contrato aprobado.",
                        "POSPUESTA",
                        "El contrato vigente cubre la funcion; el campo puede anadirse por Gate.",
                    )

        esenciales = [d for d in diferencias if d["clase"] == "ESENCIAL"]
        compatibilidad = [d for d in diferencias if d["clase"] == "COMPATIBILIDAD"]
        operaciones.append(
            {
                "id": eid,
                "operacion": f"{metodo.upper()} {ruta}",
                "operationId": op.get("operationId"),
                "seguridad": "public" if op.get("security") == [] else "bearer",
                "diferencias": diferencias,
                "resumen": {
                    "ESENCIAL": len(esenciales),
                    "COMPATIBILIDAD": len(compatibilidad),
                    "OPCIONAL": sum(1 for d in diferencias if d["clase"] == "OPCIONAL"),
                    "OBSOLETA": sum(1 for d in diferencias if d["clase"] == "OBSOLETA"),
                },
                # El congelamiento no depende de esta herramienta: exige RFC aprobada y aplicada.
                "contract_frozen": "NOT_VERIFIED",
                "motivos_no_congelado": (
                    (["RFC-016 no aprobada"] if esenciales or compatibilidad else [])
                    + (["Enmienda de persistencia no aplicada"] if any(d["clase"] == "OBSOLETA" for d in diferencias) else [])
                    + ["Aprobacion humana de la clasificacion pendiente"]
                ),
            }
        )

    total = sum(len(o["diferencias"]) for o in operaciones)
    sin_clasificar = sum(1 for o in operaciones for d in o["diferencias"] if d["clase"] not in resumen_clases)
    return {
        "documento": "Clasificacion contractual E005-E075",
        "generado_por": "herramientas/clasificar_contratos_e005.py",
        "baseline": "sdd/03-endpoints/openapi/nexum-api-v1.json",
        "reglas": {
            "http": "codigo de exito distinto -> COMPATIBILIDAD con decision individual",
            "persistencia": "SESSIONHASH -> OBSOLETA",
            "renombrado": "RENOMBRADOS -> COMPATIBILIDAD",
            "base_presente": "base del campo ya en el contrato -> OPCIONAL",
            "override": "campo material listado -> clase indicada",
            "resto": "OPCIONAL POSPUESTA (contrato vigente cubre la funcion)",
        },
        "resumen": {
            "operaciones": len(operaciones),
            "diferencias": total,
            "por_clase": resumen_clases,
            "sin_clasificar": sin_clasificar,
        },
        "operaciones": operaciones,
    }


def main(argv: list[str] | None = None) -> int:
    p = argparse.ArgumentParser(description="Clasifica diferencias contractuales E005-E075.")
    p.add_argument("--root", default=None)
    p.add_argument("--salida", default=None)
    p.add_argument("--resumen", action="store_true")
    a = p.parse_args(argv)
    raiz = Path(a.root) if a.root else Path(__file__).resolve().parents[1]
    r = clasificar(raiz)
    destino = Path(a.salida) if a.salida else raiz / SALIDA
    destino.parent.mkdir(parents=True, exist_ok=True)
    destino.write_text(json.dumps(r, ensure_ascii=False, indent=2) + "\n", encoding="utf-8", newline="\n")
    s = r["resumen"]
    print(
        f"CLASIFICACION_OK operaciones={s['operaciones']} diferencias={s['diferencias']} "
        f"sin_clasificar={s['sin_clasificar']} "
        f"esencial={s['por_clase']['ESENCIAL']} compatibilidad={s['por_clase']['COMPATIBILIDAD']} "
        f"opcional={s['por_clase']['OPCIONAL']} obsoleta={s['por_clase']['OBSOLETA']}"
    )
    if a.resumen:
        for o in r["operaciones"]:
            if o["resumen"]["ESENCIAL"] or o["resumen"]["COMPATIBILIDAD"]:
                print(
                    f" {o['id']}: E={o['resumen']['ESENCIAL']} C={o['resumen']['COMPATIBILIDAD']} "
                    f"O={o['resumen']['OPCIONAL']} X={o['resumen']['OBSOLETA']}"
                )
    return 0 if s["sin_clasificar"] == 0 else 1


if __name__ == "__main__":
    sys.exit(main())
