"""Verificador de la preparacion contractual E005-E075 (solo lectura).

Separa cuatro responsabilidades que NO deben confundirse:

1. INTEGRIDAD: la matriz preparada coincide con el OpenAPI vigente (metodo, ruta,
   operationId, esquema y campos declarados, parametros, carpeta y documentos).
2. DETECCION: diferencias entre el SDD aprobado y la auditoria V6.0.2. Es
   informativa: son candidatos que requieren clasificacion funcional, no errores.
3. DECISIONES: `sdd/_control/H08/DECISIONES_CONTRATO_E005_E075.json` fija, por
   diferencia, el valor VIGENTE y el OBJETIVO. Mientras la decision esta
   PENDIENTE el contrato debe seguir en el valor vigente (un cambio sin Gate se
   detecta); cuando esta APROBADA el contrato debe estar en el objetivo.
4. DEUDA DOCUMENTAL: referencias heredadas (SESSIONHASH) en specs aprobadas. Se
   reporta como pendiente y solo se exige erradicarla cuando la decision esta
   APROBADA.

El verificador funciona igual ANTES y DESPUES de una enmienda autorizada: no
codifica las discrepancias conocidas como invariantes permanentes.

No ejecuta AWS, no modifica contratos y no sustituye al Gate humano.
"""

from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path

MATRIZ = Path("sdd/_control/H08/PREPARACION_E005_E075/matriz_e005_e075.json")
FICHAS = Path("sdd/_control/H08/PREPARACION_E005_E075/fichas")
DECISIONES = Path("sdd/_control/H08/DECISIONES_CONTRATO_E005_E075.json")
OPENAPI = Path("sdd/03-endpoints/openapi/nexum-api-v1.json")
SPECS = Path("sdd/03-endpoints")

PERSISTENCIA_HEREDADA = ("E005", "E006", "E007", "E008", "E009", "E010", "E011")
PATRON_OBSOLETO = "SESSIONHASH"


class ErrorVerificacion(Exception):
    pass


def cargar_json(ruta: Path):
    if not ruta.is_file():
        raise ErrorVerificacion(f"FALTA_ARCHIVO {ruta.as_posix()}")
    try:
        return json.loads(ruta.read_text(encoding="utf-8"))
    except json.JSONDecodeError as ex:
        raise ErrorVerificacion(f"JSON_INVALIDO {ruta.as_posix()} {ex}") from ex


def resolver(schema: dict, componentes: dict, profundidad: int = 0) -> dict:
    if profundidad > 20:
        raise ErrorVerificacion("REF_CIRCULAR")
    if not isinstance(schema, dict):
        raise ErrorVerificacion("ESQUEMA_INVALIDO")
    if "$ref" not in schema:
        return schema
    ref = schema["$ref"]
    prefijo = "#/components/schemas/"
    if not isinstance(ref, str) or not ref.startswith(prefijo):
        raise ErrorVerificacion(f"REF_NO_SOPORTADO {ref}")
    nombre = ref[len(prefijo):]
    if nombre not in componentes:
        raise ErrorVerificacion(f"REF_IRRESOLUBLE {ref}")
    return resolver(componentes[nombre], componentes, profundidad + 1)


def aplanar(schema: dict, componentes: dict, prefijo: str = "", profundidad: int = 0) -> set[str]:
    """Rutas de propiedades normalizando arrays: `items[].id` -> `items.id`."""
    if profundidad > 10:
        return set()
    schema = resolver(schema, componentes)
    campos: set[str] = set()
    props = schema.get("properties")
    if not isinstance(props, dict):
        for comp in ("allOf", "oneOf", "anyOf"):
            if isinstance(schema.get(comp), list):
                for sub in schema[comp]:
                    campos |= aplanar(sub, componentes, prefijo, profundidad + 1)
        return campos
    for nombre, sub in props.items():
        ruta = f"{prefijo}{nombre}"
        campos.add(ruta)
        sub_resuelto = resolver(sub, componentes)
        es_array = sub_resuelto.get("type") == "array" or "items" in sub_resuelto
        if es_array and isinstance(sub_resuelto.get("items"), dict):
            campos |= aplanar(sub_resuelto["items"], componentes, ruta + ".", profundidad + 1)
        elif isinstance(sub_resuelto.get("properties"), dict):
            campos |= aplanar(sub_resuelto, componentes, ruta + ".", profundidad + 1)
        for comp in ("allOf", "oneOf", "anyOf"):
            if isinstance(sub_resuelto.get(comp), list):
                campos |= aplanar(sub_resuelto, componentes, ruta + ".", profundidad + 1)
    return campos


def normalizar(campo: str) -> str:
    return campo.replace("[]", "").strip()


def decisiones_por_id(datos: list) -> dict[str, dict]:
    indice: dict[str, dict] = {}
    for d in datos:
        if not isinstance(d, dict) or "id" not in d or "tipo" not in d:
            raise ErrorVerificacion("DECISION_INVALIDA")
        if d.get("estado") not in ("PENDIENTE", "APROBADA", "RECHAZADA"):
            raise ErrorVerificacion(f"DECISION_ESTADO_INVALIDO {d.get('id')} {d.get('estado')}")
        clave = f"{d['id']}:{d['tipo']}"
        if clave in indice:
            raise ErrorVerificacion(f"DECISION_DUPLICADA {clave}")
        indice[clave] = d
    return indice


def verificar(raiz: Path) -> dict:
    matriz = cargar_json(raiz / MATRIZ)
    if not isinstance(matriz, list) or not matriz:
        raise ErrorVerificacion("MATRIZ_VACIA")
    api = cargar_json(raiz / OPENAPI)
    if api.get("openapi") != "3.1.0":
        raise ErrorVerificacion(f"OPENAPI_VERSION {api.get('openapi')}")
    decisiones = decisiones_por_id(cargar_json(raiz / DECISIONES).get("decisiones"))
    rutas = api.get("paths") or {}
    componentes = (api.get("components") or {}).get("schemas") or {}
    specs_dir = raiz / SPECS

    problemas: list[str] = []
    detalle: list[dict] = []
    discrepancias: list[dict] = []
    pendientes: list[str] = []
    ids: set[str] = set()

    def resolver_decision(eid: str, tipo: str, valor_actual: str) -> None:
        d = decisiones.get(f"{eid}:{tipo}")
        if not d:
            return
        estado = d["estado"]
        esperado = d.get("valor_objetivo") if estado == "APROBADA" else d.get("valor_vigente")
        if estado == "RECHAZADA":
            esperado = d.get("valor_vigente")
        if esperado is None:
            problemas.append(f"DECISION_SIN_VALOR {eid}:{tipo}")
            return
        if str(valor_actual) != str(esperado):
            problemas.append(
                f"CONTRATO_FUERA_DE_DECISION {eid}:{tipo} actual={valor_actual} esperado={esperado} estado={estado}"
            )
        if estado == "PENDIENTE":
            pendientes.append(f"{eid}:{tipo} (vigente={d.get('valor_vigente')} objetivo={d.get('valor_objetivo')})")

    for fila in matriz:
        eid = fila.get("id", "")
        if not eid:
            problemas.append("FILA_SIN_ID")
            continue
        if eid in ids:
            problemas.append(f"ID_DUPLICADO {eid}")
        ids.add(eid)

        metodo = (fila.get("sdd_method") or "").lower()
        ruta = fila.get("sdd_path") or ""
        entrada: dict = {"id": eid, "ruta": ruta, "metodo": metodo.upper(), "hallazgos": []}

        if (fila.get("pdf_method") or "").upper() != (fila.get("sdd_method") or "").upper():
            problemas.append(f"METODO_DISCREPANTE {eid}")
        if (fila.get("pdf_path") or "") != ruta:
            problemas.append(f"RUTA_DISCREPANTE {eid}")

        if ruta not in rutas:
            problemas.append(f"RUTA_AUSENTE_OPENAPI {eid} {ruta}")
            detalle.append(entrada)
            continue
        if metodo not in rutas[ruta]:
            problemas.append(f"METODO_AUSENTE_OPENAPI {eid} {metodo.upper()} {ruta}")
            detalle.append(entrada)
            continue

        op = rutas[ruta][metodo]
        if op.get("operationId") != f"nexum_{eid.lower()}":
            problemas.append(f"OPERATION_ID_DISCREPANTE {eid} {op.get('operationId')}")

        exito = str(fila.get("sdd_success") or "")
        respuestas = op.get("responses") or {}
        codigos_ok = sorted(str(c) for c in respuestas if str(c).startswith("2"))
        if exito not in respuestas:
            problemas.append(f"EXITO_AUSENTE_OPENAPI {eid} {exito} (2xx={codigos_ok})")

        # Deteccion: diferencia entre el SDD y la auditoria de prototipo.
        pdf_exito = str(fila.get("pdf_success") or "")
        if pdf_exito and pdf_exito != exito:
            discrepancias.append({"id": eid, "tipo": "HTTP_EXITO", "sdd": exito, "pdf": pdf_exito})
            entrada["hallazgos"].append(f"HTTP_DISCREPANCIA SDD={exito} PDF={pdf_exito}")
            resolver_decision(eid, "HTTP_EXITO", exito)

        # Request: esquema declarado o ausencia justificada de cuerpo.
        esquema_req = (fila.get("sdd_request_schema") or "").strip()
        campos_req: set[str] = set()
        cuerpo = (op.get("requestBody") or {}).get("content", {}).get("application/json", {})
        sin_cuerpo = esquema_req.lower() in ("", "sin contenido", "ninguno", "none")
        if sin_cuerpo:
            if op.get("requestBody"):
                problemas.append(f"CUERPO_INESPERADO {eid}")
        elif esquema_req not in componentes:
            problemas.append(f"ESQUEMA_REQUEST_AUSENTE {eid} {esquema_req}")
        else:
            campos_req = {normalizar(c) for c in aplanar({"$ref": f"#/components/schemas/{esquema_req}"}, componentes)}
            esperado = (cuerpo or {}).get("schema")
            if isinstance(esperado, dict):
                resuelto = esperado.get("$ref", "").split("/")[-1]
                if resuelto and resuelto != esquema_req:
                    problemas.append(f"REQUEST_SCHEMA_DISCREPANTE {eid} {resuelto} != {esquema_req}")
            else:
                problemas.append(f"REQUEST_BODY_AUSENTE {eid}")

        parametros = {
            (p or {}).get("name")
            for p in (op.get("parameters") or [])
            if isinstance(p, dict) and (p or {}).get("name")
        }
        for p in rutas[ruta].get("parameters") or []:
            if isinstance(p, dict) and p.get("name"):
                parametros.add(p["name"])

        for campo in (normalizar(c) for c in (fila.get("sdd_request_fields") or [])):
            if not campo:
                continue
            if campo in campos_req or campo.split(".")[0] in parametros:
                continue
            problemas.append(f"CAMPO_REQUEST_NO_DECLARADO {eid} {campo}")

        campos_resp: set[str] = set()
        if exito in respuestas:
            contenido = (respuestas[exito] or {}).get("content") or {}
            esquema_resp = (contenido.get("application/json") or {}).get("schema")
            if isinstance(esquema_resp, dict):
                campos_resp = {normalizar(c) for c in aplanar(esquema_resp, componentes)}
            elif exito != "204":
                entrada["hallazgos"].append("RESPUESTA_SIN_ESQUEMA")
        for campo in (fila.get("sdd_response_fields") or []):
            campo_norm = normalizar(campo)
            if campo_norm and campo_norm not in campos_resp:
                problemas.append(f"CAMPO_RESPUESTA_NO_DECLARADO {eid} {campo_norm}")

        # Candidatos del prototipo: informativos, nunca error por si mismos.
        req_ok = {c.split(".")[0] for c in campos_req}
        resp_ok = {c.split(".")[0] for c in campos_resp}
        entrada["candidatos_request_presentes"] = [
            c for c in (fila.get("pdf_only_request_candidates") or []) if normalizar(c).split(".")[0] in req_ok
        ]
        entrada["candidatos_request_ausentes"] = [
            c for c in (fila.get("pdf_only_request_candidates") or []) if normalizar(c).split(".")[0] not in req_ok
        ]
        entrada["candidatos_response_presentes"] = [
            c for c in (fila.get("pdf_only_response_candidates") or []) if normalizar(c).split(".")[0] in resp_ok
        ]
        entrada["candidatos_response_ausentes"] = [
            c for c in (fila.get("pdf_only_response_candidates") or []) if normalizar(c).split(".")[0] not in resp_ok
        ]
        entrada["esquema_request"] = esquema_req
        entrada["exito"] = exito
        entrada["exito_openapi"] = codigos_ok
        detalle.append(entrada)

    # Deuda documental: referencias heredadas en E005-E011.
    persistencia = []
    for eid in PERSISTENCIA_HEREDADA:
        carpetas = sorted(p for p in specs_dir.glob(f"{eid}-*") if p.is_dir())
        if not carpetas:
            problemas.append(f"SPEC_AUSENTE {eid}")
            continue
        hits = sum(md.read_text(encoding="utf-8", errors="replace").count(PATRON_OBSOLETO) for md in carpetas[0].glob("*.md"))
        persistencia.append({"id": eid, "carpeta": carpetas[0].name, "ocurrencias": hits})
        decision = decisiones.get(f"{eid}:PERSISTENCIA") or decisiones.get("E005-E011:PERSISTENCIA")
        if hits:
            pendientes.append(f"{eid}:PERSISTENCIA ({hits} referencias {PATRON_OBSOLETO})")
        if decision:
            if decision["estado"] == "APROBADA" and hits:
                problemas.append(f"PERSISTENCIA_NO_CORREGIDA {eid} {hits}")

    # Trazabilidad documental.
    fichas = {p.stem for p in (raiz / FICHAS).glob("E*.md")} if (raiz / FICHAS).is_dir() else set()
    if len(fichas) != len(matriz):
        problemas.append(f"FICHAS_INCOMPLETAS {len(fichas)} != {len(matriz)}")
    for fila in matriz:
        eid = fila.get("id", "")
        if eid not in fichas:
            problemas.append(f"FICHA_AUSENTE {eid}")
        carpeta = fila.get("folder") or ""
        destino = specs_dir / carpeta
        if not carpeta or not destino.is_dir():
            problemas.append(f"CARPETA_SPEC_AUSENTE {eid} {carpeta}")
            continue
        for doc in ("spec.md", "plan.md", "tasks.md", "checklist.md"):
            if not (destino / doc).is_file():
                problemas.append(f"DOC_AUSENTE {eid} {doc}")

    # Toda decision debe corresponder a una deteccion o a deuda conocida.
    detectadas = {f"{d['id']}:{d['tipo']}" for d in discrepancias}
    conocidas = detectadas | {f"{p['id']}:PERSISTENCIA" for p in persistencia}
    for clave in decisiones:
        if clave not in conocidas:
            problemas.append(f"DECISION_SIN_HALLAZGO {clave}")

    return {
        "operaciones": len(matriz),
        "problemas": problemas,
        "detalle": detalle,
        "discrepancias": discrepancias,
        "pendientes": pendientes,
        "decisiones": len(decisiones),
        "decisiones_aprobadas": sum(1 for d in decisiones.values() if d["estado"] == "APROBADA"),
        "persistencia": persistencia,
        "resultado": "PREPARACION_CONTRATO_PASS" if not problemas else "PREPARACION_CONTRATO_FAIL",
    }


def main(argv: list[str] | None = None) -> int:
    p = argparse.ArgumentParser(description="Verifica la preparacion contractual E005-E075.")
    p.add_argument("--root", default=None)
    p.add_argument("--json", default=None)
    p.add_argument("--detalle", action="store_true", help="Muestra candidatos ausentes por operacion.")
    a = p.parse_args(argv)
    raiz = Path(a.root) if a.root else Path(__file__).resolve().parents[1]
    try:
        r = verificar(raiz)
    except ErrorVerificacion as ex:
        print(f"PREPARACION_CONTRATO_ERROR {ex}")
        return 2
    if a.json:
        Path(a.json).write_text(json.dumps(r, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    if a.detalle:
        for d in r["detalle"]:
            if d["hallazgos"] or d["candidatos_request_ausentes"] or d["candidatos_response_ausentes"]:
                print(
                    f"{d['id']}: HTTP {d['exito']} | req {d['candidatos_request_ausentes']} "
                    f"| resp {d['candidatos_response_ausentes']} | {d['hallazgos']}"
                )
    refs = sum(x["ocurrencias"] for x in r["persistencia"])
    print(
        f"{r['resultado']}: {r['operaciones']} operaciones; "
        f"discrepancias={len(r['discrepancias'])}; decisiones={r['decisiones']} "
        f"(aprobadas={r['decisiones_aprobadas']}, pendientes={len(r['pendientes'])}); "
        f"{PATRON_OBSOLETO}={refs} referencias"
    )
    for d in r["discrepancias"]:
        print(f" - discrepancia {d['id']} {d['tipo']}: SDD={d['sdd']} PDF={d['pdf']}")
    for p_ in r["pendientes"]:
        print(f" - pendiente {p_}")
    for p_ in r["problemas"][:40]:
        print(" !", p_)
    return 0 if not r["problemas"] else 1


if __name__ == "__main__":
    sys.exit(main())
