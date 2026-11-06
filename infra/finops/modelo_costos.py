"""Motor determinista de costos a 45 días (I-000 INF-000-03/INF-000-04).

Reglas:
- Importes con ``Decimal``; nunca ``float``.
- Un SKU con precio ``null``/``UNKNOWN`` produce total desconocido, nunca 0.
- El total bruto solo se publica si TODAS las partidas requeridas tienen precio.
- Créditos, impuestos y desembolso personal se informan por separado.
"""

from __future__ import annotations

import argparse
import json
import sys
from decimal import Decimal
from pathlib import Path

import dinero
import escenarios

RAIZ = Path(__file__).resolve().parents[2]
RUTA_PRECIOS = Path(__file__).with_name("precios_us-east-2.json")
ESTADOS_CON_PRECIO = {"VERIFIED_AWS", "DOC_ONLY", "HIPOTESIS", "SUPUESTO", "DISENO_DOC"}

# Partidas que dejan de facturarse cuando el runtime se apaga.
LINEAS_APAGABLES = {
    "EC2_GO",
    "ALB_FIJO",
    "ALB_LCU",
    "REDIS_OSS",
    "EGRESO",
    "REDIS_INTERAZ",
    "CW_INGESTA",
    "CW_METRICAS",
}

# Reservas de modelado que no corresponden a facturación AWS del entorno.
LINEAS_NO_AWS = {"CI_CD", "RESERVA_CF"}


def cargar_precios(ruta: Path | None = None) -> dict:
    """Carga el catálogo de precios validando su forma mínima."""
    destino = Path(ruta) if ruta else RUTA_PRECIOS
    datos = json.loads(destino.read_text(encoding="utf-8"))
    items = datos.get("items")
    if not isinstance(items, dict) or not items:
        raise ValueError("catalogo de precios sin items")
    for clave, item in items.items():
        if "precio_usd" not in item or "estado" not in item:
            raise ValueError(f"sku incompleto: {clave}")
        if item["precio_usd"] is not None:
            dinero.dec(item["precio_usd"])  # valida exactitud y rechaza float
        if item["estado"] not in ESTADOS_CON_PRECIO | {"UNKNOWN"}:
            raise ValueError(f"estado de evidencia desconocido en {clave}: {item['estado']}")
    return datos


def _precio_utilizable(item: dict) -> bool:
    if item.get("precio_usd") is None:
        return False
    return item.get("estado") in ESTADOS_CON_PRECIO


def calcular_escenario(escenario: escenarios.Escenario, catalogo: dict) -> dict:
    """Calcula partidas y totales de un escenario sin inventar tarifas."""

    items = catalogo["items"]
    detalle: list[dict] = []
    razones_sin_precio: list[str] = []

    for partida in escenario.partidas:
        item = items.get(partida.sku)
        cantidad = dinero.dec(partida.cantidad)
        if item is None:
            detalle.append(
                {
                    "clave": partida.clave,
                    "concepto": partida.concepto,
                    "cantidad": partida.cantidad,
                    "unidad": partida.unidad,
                    "sku": partida.sku,
                    "precio_unitario_usd": None,
                    "estado_evidencia": "UNKNOWN",
                    "subtotal_usd": None,
                    "requerida": partida.requerida,
                    "nota": "SKU ausente del catalogo",
                }
            )
            if partida.requerida and cantidad > 0:
                razones_sin_precio.append(f"{partida.clave}: SKU ausente")
            continue

        utilizable = _precio_utilizable(item)
        estado = item["estado"]
        subtotal = None
        if utilizable:
            if cantidad == 0:
                subtotal = dinero.cuantizar(0)
            else:
                subtotal = dinero.multiplicar(cantidad, item["precio_usd"])
        elif cantidad > 0 and partida.requerida:
            razones_sin_precio.append(f"{partida.clave}: precio {estado}")

        detalle.append(
            {
                "clave": partida.clave,
                "concepto": partida.concepto,
                "cantidad": partida.cantidad,
                "unidad": partida.unidad,
                "sku": partida.sku,
                "precio_unitario_usd": item.get("precio_usd"),
                "estado_evidencia": estado,
                "subtotal_usd": None if subtotal is None else dinero.formatear(subtotal),
                "requerida": partida.requerida,
                "nota": partida.nota or item.get("nota", ""),
            }
        )

    conocidos = [dinero.dec(d["subtotal_usd"]) for d in detalle if d["subtotal_usd"] is not None]
    total_conocido = dinero.sumar(conocidos) if conocidos else dinero.cuantizar(0)
    partidas_excluidas = [
        d for d in detalle if dinero.dec(d["cantidad"]) == 0 and d["estado_evidencia"] != "UNKNOWN"
    ]

    total_bruto = None if razones_sin_precio else dinero.formatear(total_conocido)
    residual, partidas_residuales = calcular_residual(detalle)

    return {
        "escenario": escenario.clave,
        "nombre": escenario.nombre,
        "descripcion": escenario.descripcion,
        "horas_encendido": escenario.horas_encendido,
        "dias_ventana": escenarios.DIAS_VENTANA,
        "cumple_24x7": escenario.cumple_24x7,
        "cumple_dos_instancias": escenario.cumple_dos_instancias,
        "valido_para_demostracion": escenario.valido_para_demostracion,
        "condiciones": list(escenario.condiciones),
        "partidas": detalle,
        "partidas_excluidas_por_diseno": [d["clave"] for d in partidas_excluidas],
        "total_conocido_usd": dinero.formatear(total_conocido),
        "total_bruto_45d_usd": total_bruto,
        "residual_apagado_usd": residual,
        "partidas_residuales": partidas_residuales,
        "partidas_sin_precio": razones_sin_precio,
        "grados_evidencia": _contar_grados(detalle),
    }


def _contar_grados(detalle: list[dict]) -> dict[str, int]:
    conteo: dict[str, int] = {}
    for linea in detalle:
        estado = linea["estado_evidencia"]
        conteo[estado] = conteo.get(estado, 0) + 1
    return dict(sorted(conteo.items()))


def calcular_residual(detalle: list[dict]) -> tuple[str | None, list[str]]:
    """Costo que persiste con el runtime apagado (datos, backups, observabilidad).

    Devuelve ``None`` si alguna partida persistente requerida tiene precio
    desconocido: no se asume cero.
    """

    residuales: list[Decimal] = []
    claves: list[str] = []
    desconocidas: list[str] = []
    for linea in detalle:
        if linea["clave"] in LINEAS_APAGABLES or linea["clave"] in LINEAS_NO_AWS:
            continue
        if dinero.dec(linea["cantidad"]) <= 0:
            continue
        if linea["subtotal_usd"] is None:
            desconocidas.append(linea["clave"])
            continue
        residuales.append(dinero.dec(linea["subtotal_usd"]))
        claves.append(linea["clave"])
    if desconocidas:
        return None, claves + [f"DESCONOCIDO:{c}" for c in desconocidas]
    total = dinero.sumar(residuales) if residuales else dinero.cuantizar(0)
    return dinero.formatear(total), claves



def calcular_creditos(
    bruto_usd: str | None,
    creditos_verificados_usd: str,
    creditos_declarados_usd: str | None = None,
) -> dict:
    """Separa bruto, créditos y cobro neto; los créditos no son un precio."""

    verificados = dinero.dec(creditos_verificados_usd)
    declarados = dinero.dec(creditos_declarados_usd) if creditos_declarados_usd is not None else None
    if bruto_usd is None:
        neto = None
        suficiente = None
    else:
        bruto = dinero.dec(bruto_usd)
        neto = dinero.formatear(dinero.maximo_cero(bruto - verificados))
        suficiente = dinero.dec(bruto) <= verificados
    return {
        "creditos_declarados_usd": None if declarados is None else dinero.formatear(declarados),
        "creditos_verificados_usd": dinero.formatear(verificados),
        "creditos_verificados": dinero.dec(creditos_verificados_usd) > 0,
        "cobro_neto_usd": neto,
        "creditos_cubren_bruto": suficiente,
        "impuestos": "UNKNOWN",
        "desembolso_personal_usd": "0.0000",
    }


def calcular_todo(catalogo: dict, creditos_verificados_usd: str = "0") -> dict:
    """Calcula todos los escenarios y agrega la separación financiera."""

    resultados = {
        clave: calcular_escenario(esc, catalogo)
        for clave, esc in escenarios.escenarios().items()
    }
    seleccionado = resultados["C_CLOUDFRONT_VPC_ORIGIN_PRIVADO"]
    financiero = calcular_creditos(
        seleccionado["total_bruto_45d_usd"],
        creditos_verificados_usd,
        catalogo.get("creditos_declarados_usd"),
    )
    return {
        "version": "1.0.0",
        "fecha_corte": catalogo.get("fecha_corte"),
        "region_precios": catalogo.get("region_precios"),
        "escenario_seleccionado": seleccionado["escenario"],
        "escenarios": resultados,
        "financiero": financiero,
        "nota": (
            "total_bruto_45d_usd es null cuando falta precio de una partida requerida; "
            "un SKU desconocido no se trata como cero"
        ),
    }


def _texto(resumen: dict) -> str:
    lineas = []
    for clave, esc in resumen["escenarios"].items():
        bruto = esc["total_bruto_45d_usd"]
        lineas.append(
            f"{clave}: horas={esc['horas_encendido']} 24x7={esc['cumple_24x7']} "
            f"bruto45d={bruto if bruto is not None else 'DESCONOCIDO'} "
            f"sin_precio={len(esc['partidas_sin_precio'])}"
        )
    fin = resumen["financiero"]
    lineas.append(
        f"neto={fin['cobro_neto_usd']} creditos_verificados={fin['creditos_verificados_usd']} "
        f"impuestos={fin['impuestos']} desembolso_personal={fin['desembolso_personal_usd']}"
    )
    return "\n".join(lineas)


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description="Modelo de costos FinOps I-000 (45 dias)")
    parser.add_argument("--precios", type=Path, default=RUTA_PRECIOS)
    parser.add_argument("--creditos-verificados", default="0")
    parser.add_argument("--salida", type=Path, default=None, help="ruta JSON de salida")
    parser.add_argument("--texto", action="store_true", help="resumen legible en consola")
    args = parser.parse_args(argv)

    catalogo = cargar_precios(args.precios)
    resumen = calcular_todo(catalogo, args.creditos_verificados)
    texto = json.dumps(resumen, indent=2, ensure_ascii=False, sort_keys=True)
    if args.salida:
        args.salida.parent.mkdir(parents=True, exist_ok=True)
        args.salida.write_text(texto + "\n", encoding="utf-8")
    if args.texto:
        print(_texto(resumen))
    else:
        print(texto)
    return 0


if __name__ == "__main__":
    sys.exit(main())
