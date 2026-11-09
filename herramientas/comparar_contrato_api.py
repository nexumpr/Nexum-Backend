"""Comparador de contratos OpenAPI: detecta cambios incompatibles (I-150, INF-150-05).

Uso previsto en CI: se extrae el contrato de ``origin/main`` a un archivo temporal y se
compara con el del PR. No usa red ni servicios de pago y no modifica ningun contrato.

Codigos: 0 compatible, 1 incompatible, 2 no se pudo comparar (falla cerrado).
"""

from __future__ import annotations

import argparse
import json
import re
import sys
from pathlib import Path
from typing import Any, Mapping, Sequence

METODOS = ("get", "post", "put", "patch", "delete")

# Codigos de ruptura reconocidos
CODIGOS = (
    "RUTA_ELIMINADA",
    "METODO_ELIMINADO",
    "OPERATION_ID_CAMBIADO",
    "PARAMETRO_REQUERIDO_NUEVO",
    "CAMPO_REQUERIDO_NUEVO",
    "TIPO_CAMBIADO",
    "ENUM_REDUCIDO",
    "RESPUESTA_ELIMINADA",
    "ESQUEMA_RESPUESTA_CAMBIADO",
    "PROPIEDAD_RESPUESTA_ELIMINADA",
    "TIPO_RESPUESTA_CAMBIADO",
    "ENUM_RESPUESTA_REDUCIDO",
    "REQUERIDO_RESPUESTA_NUEVO",
    "SEGURIDAD_ENDURECIDA",
    "SEGURIDAD_RELAJADA",
    "SCOPES_CAMBIADOS",
)


class ErrorComparacion(Exception):
    """No se pudo comparar (codigo 2)."""


def cargar(ruta: Path | str) -> dict[str, Any]:
    try:
        return json.loads(Path(ruta).read_text(encoding="utf-8"))
    except OSError as exc:
        raise ErrorComparacion(f"No se puede leer {ruta}: {exc}") from exc
    except json.JSONDecodeError as exc:
        raise ErrorComparacion(f"{ruta} no es JSON valido: {exc}") from exc


def operaciones(contrato: Mapping[str, Any]) -> dict[tuple[str, str], Mapping[str, Any]]:
    resultado: dict[tuple[str, str], Mapping[str, Any]] = {}
    for ruta, item in (contrato.get("paths") or {}).items():
        for metodo, operacion in (item or {}).items():
            if metodo in METODOS:
                resultado[(ruta, metodo)] = operacion
    return resultado


def _esquema(contrato: Mapping[str, Any], referencia: Any) -> Mapping[str, Any]:
    """Acepta un $ref suelto o el objeto schema completo; un $ref roto falla cerrado."""
    destino = referencia
    if isinstance(referencia, Mapping):
        destino = referencia.get("$ref")
    if not isinstance(destino, str) or not destino.startswith("#/components/schemas/"):
        return referencia if isinstance(referencia, Mapping) and "$ref" not in referencia else {}
    nombre = destino.rsplit("/", 1)[-1]
    resuelto = ((contrato.get("components") or {}).get("schemas") or {}).get(nombre)
    if resuelto is None:
        raise ErrorComparacion(f"$ref irresoluble: {destino}")
    return resuelto


def _ref_respuesta(cuerpo: Any) -> str:
    esquema = ((cuerpo or {}).get("content") or {}).get("application/json", {}).get("schema") or {}
    return str(esquema.get("$ref") or "")


def _seguridad_efectiva(contrato: Mapping[str, Any], operacion: Mapping[str, Any]) -> list[dict[str, list[str]]]:
    """`security` de la operacion si existe (incluido []), si no la global."""
    valor = operacion["security"] if "security" in operacion else contrato.get("security")
    if not isinstance(valor, list):
        return []
    normal: list[dict[str, list[str]]] = []
    for requisito in valor:
        if isinstance(requisito, Mapping):
            normal.append({str(k): sorted(map(str, v or [])) for k, v in requisito.items()})
    return normal


def _campos_requeridos(esquema: Mapping[str, Any]) -> set[str]:
    return set(esquema.get("required") or [])


def _propiedades(esquema: Mapping[str, Any]) -> Mapping[str, Any]:
    return esquema.get("properties") or {}


def _tipo(propiedad: Any) -> Any:
    if isinstance(propiedad, Mapping):
        return propiedad.get("type")
    return None


def _enums(propiedad: Any) -> set[Any]:
    if isinstance(propiedad, Mapping) and isinstance(propiedad.get("enum"), list):
        return set(propiedad["enum"])
    return set()


def _cuerpo(operacion: Mapping[str, Any]) -> Mapping[str, Any]:
    cuerpo = (operacion.get("requestBody") or {}).get("content") or {}
    for tipo in ("application/json", "application/merge-patch+json"):
        if tipo in cuerpo:
            return cuerpo[tipo].get("schema") or {}
    return {}


def comparar(base: Mapping[str, Any], actual: Mapping[str, Any]) -> list[dict[str, Any]]:
    roturas: list[dict[str, Any]] = []
    ops_base = operaciones(base)
    ops_actual = operaciones(actual)

    rutas_base = {ruta for ruta, _ in ops_base}
    rutas_actual = {ruta for ruta, _ in ops_actual}
    for ruta in sorted(rutas_base - rutas_actual):
        roturas.append({"codigo": "RUTA_ELIMINADA", "ruta": ruta, "metodo": "*"})

    for clave in sorted(ops_base):
        ruta, metodo = clave
        if clave not in ops_actual:
            if ruta in rutas_actual:
                roturas.append({"codigo": "METODO_ELIMINADO", "ruta": ruta, "metodo": metodo})
            continue
        anterior = ops_base[clave]
        nueva = ops_actual[clave]

        if anterior.get("operationId") != nueva.get("operationId"):
            roturas.append(
                {
                    "codigo": "OPERATION_ID_CAMBIADO",
                    "ruta": ruta,
                    "metodo": metodo,
                    "detalle": f"{anterior.get('operationId')} -> {nueva.get('operationId')}",
                }
            )

        params_previos = {(p.get("name"), p.get("in")): bool(p.get("required")) for p in anterior.get("parameters") or [] if isinstance(p, Mapping)}
        params_nuevos = {(p.get("name"), p.get("in")): bool(p.get("required")) for p in nueva.get("parameters") or [] if isinstance(p, Mapping)}
        for nombre, requerido in params_nuevos.items():
            if requerido and not params_previos.get(nombre, False):
                roturas.append(
                    {
                        "codigo": "PARAMETRO_REQUERIDO_NUEVO",
                        "ruta": ruta,
                        "metodo": metodo,
                        "detalle": str(nombre),
                    }
                )

        esquema_previo = _esquema(base, _cuerpo(anterior))
        esquema_nuevo = _esquema(actual, _cuerpo(nueva))
        requeridos_previos = _campos_requeridos(esquema_previo)
        requeridos_nuevos = _campos_requeridos(esquema_nuevo)
        for campo in sorted(requeridos_nuevos - requeridos_previos):
            roturas.append(
                {"codigo": "CAMPO_REQUERIDO_NUEVO", "ruta": ruta, "metodo": metodo, "detalle": campo}
            )

        propiedades_previas = _propiedades(esquema_previo)
        propiedades_nuevas = _propiedades(esquema_nuevo)
        for campo, propiedad in propiedades_nuevas.items():
            previa = propiedades_previas.get(campo)
            if previa is None:
                continue
            if _tipo(previa) != _tipo(propiedad):
                roturas.append(
                    {
                        "codigo": "TIPO_CAMBIADO",
                        "ruta": ruta,
                        "metodo": metodo,
                        "detalle": f"{campo}: {_tipo(previa)} -> {_tipo(propiedad)}",
                    }
                )
            enums_previos = _enums(previa)
            enums_nuevos = _enums(propiedad)
            if enums_previos and enums_previos - enums_nuevos:
                roturas.append(
                    {
                        "codigo": "ENUM_REDUCIDO",
                        "ruta": ruta,
                        "metodo": metodo,
                        "detalle": f"{campo}: {sorted(map(str, enums_previos - enums_nuevos))}",
                    }
                )

        respuestas_previas = (anterior.get("responses") or {})
        respuestas_nuevas = (nueva.get("responses") or {})
        for codigo in sorted(set(respuestas_previas) - set(respuestas_nuevas)):
            roturas.append(
                {"codigo": "RESPUESTA_ELIMINADA", "ruta": ruta, "metodo": metodo, "detalle": codigo}
            )
        for codigo in sorted(set(respuestas_previas) & set(respuestas_nuevas)):
            ref_previo = _ref_respuesta(respuestas_previas[codigo])
            ref_nuevo = _ref_respuesta(respuestas_nuevas[codigo])
            if ref_previo and ref_nuevo and ref_previo != ref_nuevo:
                roturas.append(
                    {
                        "codigo": "ESQUEMA_RESPUESTA_CAMBIADO",
                        "ruta": ruta,
                        "metodo": metodo,
                        "detalle": f"{codigo}: {ref_previo} -> {ref_nuevo}",
                    }
                )
                continue
            esquema_previo = _esquema(base, ref_previo or respuestas_previas[codigo])
            esquema_nuevo = _esquema(actual, ref_nuevo or respuestas_nuevas[codigo])
            props_previas = _propiedades(esquema_previo)
            props_nuevas = _propiedades(esquema_nuevo)
            for campo in sorted(set(props_previas) - set(props_nuevas)):
                roturas.append(
                    {
                        "codigo": "PROPIEDAD_RESPUESTA_ELIMINADA",
                        "ruta": ruta,
                        "metodo": metodo,
                        "detalle": f"{codigo}.{campo}",
                    }
                )
            for campo in sorted(set(props_previas) & set(props_nuevas)):
                if _tipo(props_previas[campo]) != _tipo(props_nuevas[campo]):
                    roturas.append(
                        {
                            "codigo": "TIPO_RESPUESTA_CAMBIADO",
                            "ruta": ruta,
                            "metodo": metodo,
                            "detalle": f"{codigo}.{campo}: {_tipo(props_previas[campo])} -> {_tipo(props_nuevas[campo])}",
                        }
                    )
                enums_previos = _enums(props_previas[campo])
                enums_nuevos = _enums(props_nuevas[campo])
                if enums_previos and enums_previos - enums_nuevos:
                    roturas.append(
                        {
                            "codigo": "ENUM_RESPUESTA_REDUCIDO",
                            "ruta": ruta,
                            "metodo": metodo,
                            "detalle": f"{codigo}.{campo}",
                        }
                    )
            for campo in sorted(_campos_requeridos(esquema_nuevo) - _campos_requeridos(esquema_previo)):
                roturas.append(
                    {
                        "codigo": "REQUERIDO_RESPUESTA_NUEVO",
                        "ruta": ruta,
                        "metodo": metodo,
                        "detalle": f"{codigo}.{campo}",
                    }
                )

        seguridad_prev = _seguridad_efectiva(base, anterior)
        seguridad_nueva = _seguridad_efectiva(actual, nueva)
        if seguridad_prev != seguridad_nueva:
            if seguridad_nueva and not seguridad_prev:
                roturas.append({"codigo": "SEGURIDAD_ENDURECIDA", "ruta": ruta, "metodo": metodo})
            elif seguridad_prev and not seguridad_nueva:
                roturas.append({"codigo": "SEGURIDAD_RELAJADA", "ruta": ruta, "metodo": metodo})
            else:
                scopes_previos = {s for req in seguridad_prev for v in req.values() for s in v}
                scopes_nuevos = {s for req in seguridad_nueva for v in req.values() for s in v}
                if scopes_previos != scopes_nuevos:
                    roturas.append(
                        {
                            "codigo": "SCOPES_CAMBIADOS",
                            "ruta": ruta,
                            "metodo": metodo,
                            "detalle": f"{sorted(scopes_previos)} -> {sorted(scopes_nuevos)}",
                        }
                    )
                else:
                    roturas.append(
                        {"codigo": "SEGURIDAD_ENDURECIDA", "ruta": ruta, "metodo": metodo,
                         "detalle": "esquema de seguridad distinto"}
                    )

    return roturas


RAIZ = Path(__file__).resolve().parents[1]
REGISTRO_EXCEPCIONES = RAIZ / "sdd" / "_control" / "H08" / "EXCEPCIONES_COMPATIBILIDAD_API.json"

# Autorizaciones de otras materias no habilitan cambios incompatibles de API.
RFC_FUERA_DE_ALCANCE_API = {
    "RFC-004": "secretos de bootstrap",
    "RFC-005": "TLS/CloudFront de I-030",
    "RFC-006": "excepcion WAF temporal de I-030",
    "RFC-007": "notificacion de alarmas CloudWatch",
}

ALCANCE_REQUERIDO = "API_COMPATIBILIDAD"


def _registro(ruta: Path | None = None) -> list[dict[str, Any]]:
    """Registro de excepciones de API; si no existe, no hay ninguna autorizada."""
    archivo = Path(ruta) if ruta else REGISTRO_EXCEPCIONES
    if not archivo.is_file():
        return []
    datos = json.loads(archivo.read_text(encoding="utf-8"))
    lista = datos.get("excepciones") if isinstance(datos, Mapping) else None
    return lista if isinstance(lista, list) else []


def _excepcion_autorizada(excepcion: Mapping[str, Any], hoy: str | None, registro: list[dict[str, Any]]) -> bool:
    """Autorizacion humana explicita, vigente y acotada al contrato API."""
    if excepcion.get("estado") != "APPROVED":
        return False
    rfc = str(excepcion.get("rfc", ""))
    if rfc in RFC_FUERA_DE_ALCANCE_API:
        return False
    for autorizada in registro:
        if not isinstance(autorizada, Mapping):
            continue
        if autorizada.get("alcance") != ALCANCE_REQUERIDO:
            continue
        if autorizada.get("codigo") != excepcion.get("codigo"):
            continue
        if autorizada.get("ruta") != excepcion.get("ruta"):
            continue
        if str(autorizada.get("metodo", "")).lower() != str(excepcion.get("metodo", "")).lower():
            continue
        if autorizada.get("rfc") != rfc:
            continue
        if not autorizada.get("aprobado_por"):
            continue
        documento = RAIZ / str(autorizada.get("gate_documento", ""))
        if not documento.is_file():
            continue
        contenido = documento.read_text(encoding="utf-8")
        if "APRUEBO" not in contenido or rfc not in contenido:
            continue
        vigencia = str(autorizada.get("vigencia_hasta", ""))
        referencia = hoy or "2026-11-09"
        if not vigencia or vigencia < referencia:
            continue
        return True
    return False


def excepciones_aprobadas(
    datos: Mapping[str, Any] | None, hoy: str | None = None, registro: list[dict[str, Any]] | None = None
) -> list[dict[str, Any]]:
    """Solo cuentan excepciones con autorizacion humana especifica de API."""
    if not datos:
        return []
    lista = datos.get("excepciones") if isinstance(datos, Mapping) else None
    if not isinstance(lista, list):
        raise ErrorComparacion("'excepciones' debe ser una lista.")
    autorizadas = registro if registro is not None else _registro()
    aprobadas: list[dict[str, Any]] = []
    for excepcion in lista:
        if not isinstance(excepcion, Mapping):
            raise ErrorComparacion("excepcion malformada")
        if _excepcion_autorizada(excepcion, hoy, autorizadas):
            aprobadas.append(excepcion)
    return aprobadas


def aplicar_excepciones(
    roturas: list[dict[str, Any]], excepciones: list[dict[str, Any]]
) -> tuple[list[dict[str, Any]], list[dict[str, Any]]]:
    cubiertas: list[dict[str, Any]] = []
    restantes: list[dict[str, Any]] = []
    for rotura in roturas:
        cubierta = next(
            (
                exc
                for exc in excepciones
                if exc.get("codigo") == rotura["codigo"]
                and exc.get("ruta") == rotura["ruta"]
                and str(exc.get("metodo", "")).lower() in (rotura["metodo"], "*")
            ),
            None,
        )
        if cubierta:
            cubiertas.append({**rotura, "rfc": cubierta["rfc"]})
        else:
            restantes.append(rotura)
    return restantes, cubiertas


def construir_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        prog="comparar_contrato_api.py",
        description="Detecta cambios incompatibles entre dos contratos OpenAPI (I-150).",
    )
    parser.add_argument("--base", required=True, help="Contrato de la rama base (main)")
    parser.add_argument("--actual", required=True, help="Contrato del PR")
    parser.add_argument("--excepciones", default=None)
    parser.add_argument("--salida", default=None)
    return parser


def ejecutar(argv: Sequence[str] | None = None) -> tuple[int, dict[str, Any]]:
    args = construir_parser().parse_args(list(argv) if argv is not None else None)
    base = cargar(args.base)
    actual = cargar(args.actual)
    excepciones = excepciones_aprobadas(cargar(args.excepciones) if args.excepciones else None)

    roturas = comparar(base, actual)
    restantes, cubiertas = aplicar_excepciones(roturas, excepciones)
    resultado = {
        "comando": "comparar-contrato",
        "operaciones_base": len(operaciones(base)),
        "operaciones_actual": len(operaciones(actual)),
        "roturas": restantes,
        "cubiertas_por_rfc": cubiertas,
        "estado": "COMPATIBLE" if not restantes else "INCOMPATIBLE",
        "advertencia": (
            "Un cambio incompatible exige nueva version de API o RFC aprobada; "
            "no se modifica H06 para hacer pasar el check."
        ),
    }
    if args.salida:
        Path(args.salida).write_text(
            json.dumps(resultado, ensure_ascii=False, indent=2, sort_keys=True) + "\n",
            encoding="utf-8",
        )
    return (0 if not restantes else 1), resultado


def main(argv: Sequence[str] | None = None) -> int:
    try:
        codigo, resultado = ejecutar(argv)
    except ErrorComparacion as exc:
        print(f"NO_COMPARABLE: {exc}", file=sys.stderr)
        return 2
    print(json.dumps(resultado, ensure_ascii=False, indent=2, sort_keys=True))
    return codigo


if __name__ == "__main__":  # pragma: no cover
    raise SystemExit(main())
