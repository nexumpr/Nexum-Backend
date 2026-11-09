"""Conciliacion FinOps de I-130 contra el modelo H05 (sin duplicar costos).

El modelo V3 ya presupuesta PITR, backup, restore y S3 del state. Este programa
calcula el **incremento real** de I-130 comparando por SKU y concepto, y separa
lo recurrente de lo eventual y lo habilitado de lo opcional.

No consulta AWS ni modifica el catalogo ni el modelo.

Codigos: 0 sin bloqueo presupuestario local, 1 bloqueo, 2 uso invalido.
"""

from __future__ import annotations

import argparse
import json
import sys
from decimal import Decimal, ROUND_HALF_UP
from pathlib import Path
from typing import Any, Mapping, Sequence

RAIZ = Path(__file__).resolve().parents[1]
MODELO_DEFECTO = RAIZ / "sdd" / "02-infraestructura" / "FINOPS_45_DIAS_MODELO_V3.json"
INVENTARIO_DEFECTO = RAIZ / "infra" / "terraform" / "respaldo" / "inventario_backup.json"
CATALOGO_DEFECTO = RAIZ / "infra" / "finops" / "precios_us-east-2.json"

CUATRO_DECIMALES = Decimal("0.0001")
HORAS_MES = Decimal("730")
HORAS_45_DIAS = Decimal("1080")
TERCIOS = Decimal("0.0055")  # 0.0055 = 3 GB / 545 GB-mes equivalentes del modelo H05

# SKU del inventario -> concepto del modelo H05 (V3)
CONCEPTO_POR_SKU = {
    "DDB_PITR_GB_MES": "DynamoDB PITR",
    "DDB_BACKUP_GB_MES": "DynamoDB backup",
    "S3_STANDARD_GB_MES": "S3 Terraform-state",
}

EVENTUAL = {"DDB_RESTORE_GB"}


class ErrorConciliacion(Exception):
    """Modelo o inventario incompletos (codigo 2)."""


def cuantizar(valor: Decimal) -> Decimal:
    return valor.quantize(CUATRO_DECIMALES, rounding=ROUND_HALF_UP)


def cargar_json(ruta: Path | str) -> Any:
    try:
        return json.loads(Path(ruta).read_text(encoding="utf-8"))
    except OSError as exc:
        raise ErrorConciliacion(f"No se puede leer {ruta}: {exc}") from exc
    except json.JSONDecodeError as exc:
        raise ErrorConciliacion(f"{ruta} no es JSON valido: {exc}") from exc


def _recorrer_partidas(nodo: Any) -> list[Mapping[str, Any]]:
    encontradas: list[Mapping[str, Any]] = []
    if isinstance(nodo, Mapping):
        if nodo.get("partida") and "precio_unitario_usd" in nodo:
            encontradas.append(nodo)
        for valor in nodo.values():
            encontradas.extend(_recorrer_partidas(valor))
    elif isinstance(nodo, list):
        for valor in nodo:
            encontradas.extend(_recorrer_partidas(valor))
    return encontradas


def partidas_modelo(modelo: Mapping[str, Any]) -> dict[str, Mapping[str, Any]]:
    por_concepto: dict[str, Mapping[str, Any]] = {}
    for partida in _recorrer_partidas(modelo):
        por_concepto.setdefault(str(partida["partida"]), partida)
    return por_concepto


def importe_45d(cantidad: Any, precio: Any, unidad: str) -> Decimal:
    """Mes -> 1080/730; por GB restaurado es eventual (una vez)."""
    cantidad_d = Decimal(str(cantidad))
    precio_d = Decimal(str(precio))
    if unidad == "GB-mes":
        return cantidad_d * precio_d * HORAS_45_DIAS / HORAS_MES
    if unidad == "GB-restaurado":
        return cantidad_d * precio_d
    return cantidad_d * precio_d


def lineas_inventario(inventario: Mapping[str, Any]) -> list[dict[str, Any]]:
    lineas: list[dict[str, Any]] = []
    for activo in inventario.get("activos", []):
        for partida in activo.get("partidas", []) or []:
            sku = partida.get("sku")
            if partida.get("cantidad") is None:
                continue
            lineas.append(
                {
                    "sku": sku,
                    "concepto": CONCEPTO_POR_SKU.get(sku, "no conciliado"),
                    "cantidad": Decimal(str(partida["cantidad"])),
                    "base": partida.get("base"),
                    "activo": activo.get("nombre"),
                    "estado_activo": activo.get("estado"),
                }
            )
    return lineas


def conciliar(
    modelo: Mapping[str, Any],
    inventario: Mapping[str, Any],
    catalogo: Mapping[str, Any],
    cantidad_referencia: Decimal = Decimal("3"),
) -> dict[str, Any]:
    conceptos = partidas_modelo(modelo)
    precios = catalogo["items"]
    filas: list[dict[str, Any]] = []
    incremento = Decimal("0")
    recurrente = Decimal("0")
    eventual = Decimal("0")

    for linea in lineas_inventario(inventario):
        sku = linea["sku"]
        if sku not in precios:
            raise ErrorConciliacion(f"SKU sin catalogo: {sku}")
        precio = precios[sku].get("precio_usd")
        if precio is None:
            raise ErrorConciliacion(f"SKU sin precio verificado: {sku}")
        concepto = linea["concepto"]
        modelo_partida = conceptos.get(concepto)

        # Importe I-130 con la cantidad de referencia (3 GB) y con 2 GB.
        unidad = "GB-restaurado" if sku in EVENTUAL else "GB-mes"
        importe_3 = importe_45d(cantidad_referencia, precio, unidad)
        importe_2 = importe_45d(Decimal("2"), precio, unidad)
        importe_modelo = (
            importe_45d(modelo_partida.get("cantidad"), modelo_partida.get("precio_unitario_usd"), unidad)
            if modelo_partida
            else Decimal("0")
        )
        ya_presupuestado = modelo_partida is not None
        delta = importe_3 - importe_modelo if ya_presupuestado else importe_3

        if sku in EVENTUAL:
            eventual += importe_3
        else:
            recurrente += importe_3
        incremento += delta if ya_presupuestado else importe_3

        filas.append(
            {
                "sku": sku,
                "concepto": concepto,
                "presupuestado_en_modelo": ya_presupuestado,
                "cantidad_modelo": str(modelo_partida.get("cantidad")) if modelo_partida else None,
                "importe_modelo_45d_usd": str(cuantizar(importe_modelo)),
                "importe_i130_3gb_usd": str(cuantizar(importe_3)),
                "importe_i130_2gb_usd": str(cuantizar(importe_2)),
                "delta_2_vs_3_usd": str(cuantizar(importe_2 - importe_3)),
                "incremento_sobre_modelo_usd": str(cuantizar(delta)),
                "naturaleza": "EVENTUAL" if sku in EVENTUAL else "RECURRENTE",
                "estado_gasto": (
                    "OPCIONAL_NO_HABILITADO" if linea["estado_activo"] == "OPCIONAL_NO_CREADO" else "HABILITADO"
                ),
            }
        )

    total_modelo = Decimal(str(modelo.get("estimado_total_45d_usd", "0")))
    creditos = Decimal(str(modelo.get("creditos_declarados_usd", "0")))
    margen = creditos - (total_modelo + incremento)
    return {
        "filas": filas,
        "total_modelo_45d_usd": str(cuantizar(total_modelo)),
        "incremento_i130_45d_usd": str(cuantizar(incremento)),
        "recurrente_i130_45d_usd": str(cuantizar(recurrente)),
        "eventual_i130_45d_usd": str(cuantizar(eventual)),
        "delta_2_vs_3gb_45d_usd": str(cuantizar(recurrente / cantidad_referencia * (Decimal("2") - cantidad_referencia))),
        "creditos_declarados_usd": str(cuantizar(creditos)),
        "margen_local_45d_usd": str(cuantizar(margen)),
        "estados_evidencia": {
            "estimado": "CALCULADO",
            "presupuestado": "MODELO_H05_V3",
            "verificado_en_cuenta": "NOT_VERIFIED",
            "consumo_real": "NOT_VERIFIED",
        },
        "bloqueo_presupuestario_local": margen < 0,
        "decision": (
            "BLOCKED_FINOPS"
            if margen < 0
            else "VIABILIDAD_LOCAL_SIN_EVIDENCIA_DE_CUENTA"
        ),
        "advertencia": (
            "Calculo local contra el modelo H05. No se anaden partidas nuevas ni se duplican "
            "lineas ya presupuestadas. Creditos y consumo real siguen NOT_VERIFIED."
        ),
        "aws_consultado": False,
    }


def construir_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        prog="conciliar_finops_130.py",
        description="Conciliacion FinOps de I-130 contra el modelo H05. Sin AWS.",
    )
    parser.add_argument("--modelo", default=str(MODELO_DEFECTO))
    parser.add_argument("--inventario", default=str(INVENTARIO_DEFECTO))
    parser.add_argument("--catalogo", default=str(CATALOGO_DEFECTO))
    parser.add_argument("--salida", default=None)
    return parser


def ejecutar(argv: Sequence[str] | None = None) -> tuple[int, dict[str, Any]]:
    args = construir_parser().parse_args(list(argv) if argv is not None else None)
    resultado = conciliar(
        cargar_json(args.modelo), cargar_json(args.inventario), cargar_json(args.catalogo)
    )
    resultado["comando"] = "conciliar"
    if args.salida:
        Path(args.salida).write_text(
            json.dumps(resultado, ensure_ascii=False, indent=2, sort_keys=True) + "\n",
            encoding="utf-8",
        )
    return (1 if resultado["bloqueo_presupuestario_local"] else 0), resultado


def main(argv: Sequence[str] | None = None) -> int:
    try:
        codigo, resultado = ejecutar(argv)
    except ErrorConciliacion as exc:
        print(f"CONCILIACION_INVALIDA: {exc}", file=sys.stderr)
        return 2
    print(json.dumps(resultado, ensure_ascii=False, indent=2, sort_keys=True))
    return codigo


if __name__ == "__main__":  # pragma: no cover
    raise SystemExit(main())
