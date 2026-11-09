"""Comparador de contratos OpenAPI: detecta cambios incompatibles (I-150, INF-150-05).

Uso previsto en CI: se extrae el contrato de ``origin/main`` a un archivo temporal y se
compara con el del PR. No usa red ni servicios de pago y no modifica ningun contrato.

Codigos: 0 compatible, 1 incompatible, 2 no se pudo comparar (falla cerrado).
"""

from __future__ import annotations

import argparse
import json
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
    "SEGURIDAD_ENDURECIDA",
    "SEGURIDAD_RELAJADA",
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
    """Acepta un $ref suelto o el objeto schema completo con $ref."""
    destino = referencia
    if isinstance(referencia, Mapping):
        destino = referencia.get("$ref")
    if not isinstance(destino, str) or not destino.startswith("#/components/schemas/"):
        return referencia if isinstance(referencia, Mapping) and "$ref" not in referencia else {}
    nombre = destino.rsplit("/", 1)[-1]
    return ((contrato.get("components") or {}).get("schemas") or {}).get(nombre, {})


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

        respuestas_previas = set((anterior.get("responses") or {}).keys())
        respuestas_nuevas = set((nueva.get("responses") or {}).keys())
        for codigo in sorted(respuestas_previas - respuestas_nuevas):
            roturas.append(
                {"codigo": "RESPUESTA_ELIMINADA", "ruta": ruta, "metodo": metodo, "detalle": codigo}
            )

        seguridad_prev = bool(anterior.get("security"))
        seguridad_nueva = bool(nueva.get("security"))
        seguridad_global_prev = bool(base.get("security"))
        seguridad_global_nueva = bool(actual.get("security"))
        if (seguridad_nueva or seguridad_global_nueva) and not (seguridad_prev or seguridad_global_prev):
            roturas.append({"codigo": "SEGURIDAD_ENDURECIDA", "ruta": ruta, "metodo": metodo})
        if (seguridad_prev or seguridad_global_prev) and not (seguridad_nueva or seguridad_global_nueva):
            roturas.append({"codigo": "SEGURIDAD_RELAJADA", "ruta": ruta, "metodo": metodo})

    return roturas


def excepciones_aprobadas(datos: Mapping[str, Any] | None) -> list[dict[str, Any]]:
    if not datos:
        return []
    lista = datos.get("excepciones") if isinstance(datos, Mapping) else None
    if not isinstance(lista, list):
        raise ErrorComparacion("'excepciones' debe ser una lista.")
    aprobadas: list[dict[str, Any]] = []
    for excepcion in lista:
        if not isinstance(excepcion, Mapping):
            raise ErrorComparacion("excepcion malformada")
        if excepcion.get("estado") != "APPROVED" or not excepcion.get("rfc"):
            continue
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
