"""Revalidación de precios contra AWS Price List (sólo lectura).

Toma las consultas ``precio_*`` del catálogo de ``consultas_readonly.py``,
ejecuta ``aws pricing get-products`` y extrae el precio on-demand exacto del
ítem que cumple familia, atributos y unidad. Nunca infiere un precio: si no hay
coincidencia, el SKU queda ``NOT_VERIFIED`` con el motivo y la cantidad de
ítems examinados.

Escribe la evidencia sanitizada en ``evidencia/precios_<fecha>.json`` y, con
``--aplicar``, actualiza ``precios_us-east-2.json`` marcando el SKU como
``VERIFIED_AWS`` con la consulta, la fecha y la unidad de origen.
"""

from __future__ import annotations

import argparse
import json
import sys
from decimal import Decimal, InvalidOperation
from pathlib import Path

import consultas_readonly as cr
import sanitizar

RAIZ = Path(__file__).resolve().parents[2]
RUTA_PRECIOS = Path(__file__).with_name("precios_us-east-2.json")
DIR_EVIDENCIA = Path(__file__).with_name("evidencia")
FECHA_CORTE = "2026-11-06"

# consulta de precio -> (clave del catálogo, factor para expresar el precio en
# la unidad del catálogo). El factor es una cadena decimal exacta.
MAPEO: dict[str, tuple[str, str]] = {
    "precio_ec2_t4g_micro": ("EC2_T4G_MICRO_HORA", "1"),
    "precio_ebs_gp3": ("EBS_GP3_GB_MES", "1"),
    "precio_ebs_snapshot": ("EBS_SNAPSHOT_GB_MES", "1"),
    "precio_elasticache_redis": ("ELASTICACHE_REDIS_T4G_MICRO_HORA", "1"),
    "precio_alb_hora": ("ALB_HORA", "1"),
    "precio_alb_lcu": ("ALB_LCU_HORA", "1"),
    "precio_ddb_storage": ("DDB_STORAGE_GB_MES", "1"),
    # DynamoDB publica el precio por unidad de solicitud; el catálogo trabaja
    # por millón de unidades.
    "precio_ddb_rru": ("DDB_RRU_MILLON", "1000000"),
    "precio_ddb_wru": ("DDB_WRU_MILLON", "1000000"),
    "precio_ddb_pitr": ("DDB_PITR_GB_MES", "1"),
    "precio_s3_standard": ("S3_STANDARD_GB_MES", "1"),
    # S3 publica por solicitud; el catálogo trabaja por mil solicitudes.
    "precio_s3_solicitudes": ("S3_SOLICITUDES_LOTE", "1000"),
    "precio_cloudwatch_logs_ingesta": ("CW_LOGS_INGESTA_GB", "1"),
    "precio_cw_logs_storage": ("CW_LOGS_ALMACEN_GB_MES", "1"),
    "precio_cw_metrica": ("CW_METRICA_MES", "1"),
    "precio_cw_alarma": ("CW_ALARMA_MES", "1"),
    "precio_secret_manager": ("SECRETS_SECRETO_MES", "1"),
    "precio_ddb_backup": ("DDB_BACKUP_GB_MES", "1"),
    "precio_ddb_restore": ("DDB_RESTORE_GB", "1"),
    "precio_interaz": ("ELASTICACHE_INTERAZ_GB", "1"),
    "precio_ipv4_publica": ("IPV4_HORA", "1"),
    "precio_cloudfront_egreso": ("EGRESO_INTERNET_GB", "1"),
}


class PrecioNoExtraible(ValueError):
    """El producto no expone un precio on-demand utilizable."""


def normalizar_importe(texto: str) -> str:
    """Convierte ``0.0084000000`` en ``0.0084`` conservando exactitud decimal."""

    try:
        valor = Decimal(str(texto))
    except (InvalidOperation, ValueError) as exc:
        raise PrecioNoExtraible(f"importe no decimal: {texto!r}") from exc
    if valor < 0:
        raise PrecioNoExtraible(f"importe negativo: {texto!r}")
    return format(valor.normalize(), "f")


def _iterar_productos(texto_salida: str) -> list[dict]:
    datos = json.loads(texto_salida or "{}")
    crudos = datos.get("PriceList") or []
    productos: list[dict] = []
    for crudo in crudos:
        if isinstance(crudo, str):
            try:
                productos.append(json.loads(crudo))
            except json.JSONDecodeError:
                continue
        elif isinstance(crudo, dict):
            productos.append(crudo)
    return productos


def _dimensiones(items: list[dict], consulta: cr.Consulta) -> list[dict]:
    """Dimensiones de precio on-demand que cumplen familia, atributos y unidad."""

    encontradas: list[dict] = []
    familias: set[str] = set()
    for item in items:
        producto = item.get("product") or {}
        familia = producto.get("productFamily")
        familias.add(str(familia))
        if consulta.familia_producto and familia != consulta.familia_producto:
            continue
        atributos = producto.get("attributes") or {}
        if any(
            str(atributos.get(campo)) != str(valor)
            for campo, valor in consulta.atributos_esperados
        ):
            continue
        for termino in (item.get("terms") or {}).get("OnDemand", {}).values():
            for dimension in (termino.get("priceDimensions") or {}).values():
                if consulta.unidad_esperada and dimension.get("unit") != consulta.unidad_esperada:
                    continue
                usd = (dimension.get("pricePerUnit") or {}).get("USD")
                if usd is None:
                    continue
                encontradas.append(
                    {
                        "usd": usd,
                        "unidad": dimension.get("unit"),
                        "inicio": Decimal(str(dimension.get("beginRange") or "0")),
                        "fin": dimension.get("endRange"),
                        "descripcion": dimension.get("description"),
                        "usagetype": atributos.get("usagetype"),
                        "effective_date": termino.get("effectiveDate"),
                    }
                )
    return encontradas, familias


def _elegir_escalon(dimensiones: list[dict]) -> dict:
    """Elige el escalón aplicable a volúmenes pequeños.

    Se ordena por ``beginRange`` y se toma el **primer escalón con precio
    distinto de cero**. Así se evita publicar $0 cuando el primer tramo es una
    franquicia gratuita (p. ej. 25 GB de DynamoDB o 5 GB de S3) pero se respeta
    el primer tramo de pago cuando no hay franquicia.
    """

    ordenadas = sorted(dimensiones, key=lambda d: d["inicio"])
    for dimension in ordenadas:
        if Decimal(str(dimension["usd"])) > 0:
            return dimension
    return ordenadas[0]


def extraer_precio(productos: list[dict], consulta: cr.Consulta) -> tuple[str, dict]:
    """Devuelve el precio on-demand marginal del ítem solicitado."""

    dimensiones, familias = _dimensiones(productos, consulta)
    if not dimensiones:
        detalle = (
            f"sin ítem con familia {consulta.familia_producto!r} "
            f"y atributos {dict(consulta.atributos_esperados)}"
        )
        if familias:
            detalle += f"; familias observadas: {sorted(f for f in familias if f)}"
        raise PrecioNoExtraible(detalle)

    elegida = _elegir_escalon(dimensiones)
    return normalizar_importe(elegida["usd"]), {
        "usagetype": elegida["usagetype"],
        "unidad_origen": elegida["unidad"],
        "descripcion_aws": elegida["descripcion"],
        "effective_date": elegida["effective_date"],
        "escalon_inicio": str(elegida["inicio"]),
        "escalon_fin": elegida["fin"],
        "escalones_observados": len(dimensiones),
    }


def ejecutar_consulta(
    consulta: cr.Consulta, profile: str, dir_crudo: Path, timeout: int = 180
) -> tuple[str | None, dict]:
    comando = consulta.comando(profile, cr.REGION_COMERCIAL_DATOS)
    resultado = cr.ejecutar(
        consulta,
        profile,
        cr.REGION_COMERCIAL_DATOS,
        dir_crudo,
        DIR_EVIDENCIA,
        account_id=None,
        aprobar_facturable=False,
        timeout=timeout,
    )
    if resultado["estado"] != "PASS":
        return None, {"estado": resultado["estado"], "motivo": resultado.get("motivo")}

    crudo = dir_crudo / f"{consulta.id}.raw.txt"
    productos = _iterar_productos(crudo.read_text(encoding="utf-8"))
    try:
        precio, detalle = extraer_precio(productos, consulta)
    except PrecioNoExtraible as exc:
        return None, {
            "estado": "NOT_VERIFIED",
            "motivo": str(exc),
            "items_examinados": len(productos),
            "comando": " ".join(comando),
        }
    detalle.update(
        {
            "estado": "VERIFIED_AWS",
            "items_examinados": len(productos),
            "comando": " ".join(comando),
        }
    )
    return precio, detalle


def revalidar(
    profile: str,
    dir_crudo: Path,
    catalogo: dict | None = None,
    aplicar: bool = False,
    forzar: bool = False,
) -> dict:
    por_id = cr.catalogo_por_id()
    catalogo = catalogo or json.loads(RUTA_PRECIOS.read_text(encoding="utf-8"))
    evidencia: dict[str, dict] = {}
    actualizados: list[str] = []
    omitidos: list[str] = []

    for consulta_id, (clave_sku, factor) in MAPEO.items():
        consulta = por_id[consulta_id]
        item_actual = catalogo["items"].get(clave_sku) or {}
        if (
            not forzar
            and item_actual.get("estado") == "VERIFIED_AWS"
            and item_actual.get("precio_usd") is not None
        ):
            # Evidencia existente válida: no se repite la consulta AWS.
            omitidos.append(clave_sku)
            evidencia[clave_sku] = {
                "consulta": consulta_id,
                "sku": clave_sku,
                "estado": "VERIFIED_AWS",
                "precio_catalogo_usd": item_actual.get("precio_usd"),
                "fuente": item_actual.get("fuente"),
                "reutilizado": True,
            }
            continue

        precio, detalle = ejecutar_consulta(consulta, profile, dir_crudo)
        registro = {
            "consulta": consulta_id,
            "sku": clave_sku,
            "descripcion": consulta.descripcion,
            "region_datos": consulta.region_datos,
            "unidad_esperada": consulta.unidad_esperada,
            "precio_publicado_usd": precio,
            "factor_escala": factor,
            **detalle,
        }
        if precio is not None:
            valor = normalizar_importe(str(Decimal(precio) * Decimal(factor)))
            registro["precio_catalogo_usd"] = valor
            item = catalogo["items"].get(clave_sku)
            if item is None:
                registro["estado"] = "NOT_VERIFIED"
                registro["motivo"] = f"SKU {clave_sku} ausente del catálogo"
            else:
                actualizados.append(clave_sku)
                if aplicar:
                    item["precio_usd"] = valor
                    item["estado"] = "VERIFIED_AWS"
                    item["fuente"] = (
                        f"aws pricing get-products {consulta_id} "
                        f"(serviceCode={consulta.codigo_servicio}, "
                        f"usagetype={registro.get('usagetype')})"
                    )
                    item["fecha_fuente"] = FECHA_CORTE
        evidencia[clave_sku] = registro

    resumen = {
        "id": "precios",
        "fecha": FECHA_CORTE,
        "estado": "VERIFIED_AWS",
        "fuente": "aws pricing get-products (Price List Query API, endpoint us-east-1)",
        "region_datos": cr.REGION_COMERCIAL_DATOS,
        "total_skus": len(MAPEO),
        "verificados": sum(1 for r in evidencia.values() if r["estado"] == "VERIFIED_AWS"),
        "consultados_ahora": sorted(actualizados),
        "reutilizados": sorted(omitidos),
        "no_verificados": sorted(
            sku for sku, r in evidencia.items() if r["estado"] != "VERIFIED_AWS"
        ),
        "skus": evidencia,
        "sanitizado": True,
    }
    if aplicar:
        RUTA_PRECIOS.write_text(
            json.dumps(catalogo, indent=2, ensure_ascii=False) + "\n", encoding="utf-8"
        )
        resumen["catalogo_actualizado"] = sorted(actualizados)
    return resumen


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description="Revalidación de precios AWS I-000")
    parser.add_argument("--profile", default="nexum")
    parser.add_argument("--dir-crudo", type=Path, default=cr.CRUDO_POR_DEFECTO)
    parser.add_argument("--aplicar", action="store_true", help="actualiza el catálogo")
    parser.add_argument(
        "--forzar", action="store_true", help="reconsulta SKUs ya VERIFIED_AWS"
    )
    parser.add_argument("--salida", type=Path, default=None)
    args = parser.parse_args(argv)

    resumen = revalidar(
        args.profile, args.dir_crudo, aplicar=args.aplicar, forzar=args.forzar
    )
    texto = json.dumps(resumen, indent=2, ensure_ascii=False, sort_keys=True)
    destino = args.salida or (DIR_EVIDENCIA / f"precios_{FECHA_CORTE}.json")
    destino.parent.mkdir(parents=True, exist_ok=True)
    destino.write_text(texto + "\n", encoding="utf-8")

    print(f"verificados={resumen['verificados']}/{resumen['total_skus']}")
    for clave, registro in sorted(resumen["skus"].items()):
        estado = registro["estado"]
        precio = registro.get("precio_catalogo_usd") or registro.get("precio_publicado_usd")
        print(f"  {clave:36s} {estado:12s} {precio}")
    if resumen["no_verificados"]:
        print(f"NO_VERIFICADOS: {', '.join(resumen['no_verificados'])}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
