"""Pruebas del extractor de precios AWS (Price List) — revisión 2.

Cubren los dos errores financieros detectados en la sesión anterior:

- ElastiCache tomaba la tarifa de *extended support* (``ExtendedSupportYr3``) en
  lugar de ``USE2-NodeUsage``.
- DynamoDB tomaba el tramo gratuito (0 USD) en lugar del precio marginal.

Además verifican que no se inventan precios y que la evidencia ya verificada no
se reconsulta.
"""

from __future__ import annotations

import json
import sys
import unittest
from pathlib import Path

RAIZ = Path(__file__).resolve().parents[3]
sys.path.insert(0, str(RAIZ / "infra" / "finops"))

import consultas_readonly as cr  # noqa: E402
import precios_api  # noqa: E402


def _item(
    familia: str,
    atributos: dict,
    dimensiones: list[dict],
    *,
    termino: str = "JRTCKXETXF",
) -> dict:
    return {
        "product": {"productFamily": familia, "attributes": atributos},
        "terms": {
            "OnDemand": {
                termino: {
                    "effectiveDate": "2026-11-01T00:00:00Z",
                    "priceDimensions": {
                        f"dim{indice}": {
                            "unit": dimension["unit"],
                            "beginRange": dimension.get("begin", "0"),
                            "endRange": dimension.get("end", "Inf"),
                            "description": dimension.get("desc", ""),
                            "pricePerUnit": {"USD": dimension["usd"]},
                        }
                        for indice, dimension in enumerate(dimensiones)
                    },
                }
            }
        },
    }


class Normalizacion(unittest.TestCase):
    def test_quita_ceros_a_la_derecha(self) -> None:
        self.assertEqual(precios_api.normalizar_importe("0.0084000000"), "0.0084")
        self.assertEqual(precios_api.normalizar_importe("0.0225000000"), "0.0225")
        self.assertEqual(precios_api.normalizar_importe("0"), "0")

    def test_rechaza_negativos_y_no_numericos(self) -> None:
        with self.assertRaises(precios_api.PrecioNoExtraible):
            precios_api.normalizar_importe("-1")
        with self.assertRaises(precios_api.PrecioNoExtraible):
            precios_api.normalizar_importe("abc")

    def test_sin_punto_flotante(self) -> None:
        self.assertEqual(precios_api.normalizar_importe("0.0000001250"), "0.000000125")


class SeleccionDeTramo(unittest.TestCase):
    def setUp(self) -> None:
        self.consulta_ddb = cr.catalogo_por_id()["precio_ddb_storage"]

    def test_no_toma_el_tramo_gratuito(self) -> None:
        items = [
            _item(
                "Database Storage",
                {"usagetype": "USE2-TimedStorage-ByteHrs"},
                [
                    {"unit": "GB-Mo", "usd": "0.0000000000", "begin": "0", "end": "25"},
                    {"unit": "GB-Mo", "usd": "0.2500000000", "begin": "25", "end": "Inf"},
                ],
            )
        ]
        precio, detalle = precios_api.extraer_precio(items, self.consulta_ddb)
        self.assertEqual(precio, "0.25")
        self.assertEqual(detalle["escalon_inicio"], "25")
        self.assertEqual(detalle["escalones_observados"], 2)

    def test_toma_el_primer_tramo_si_no_es_gratuito(self) -> None:
        items = [
            _item(
                "Database Storage",
                {"usagetype": "USE2-TimedStorage-ByteHrs"},
                [
                    {"unit": "GB-Mo", "usd": "0.2500000000", "begin": "0", "end": "Inf"},
                ],
            )
        ]
        precio, _ = precios_api.extraer_precio(items, self.consulta_ddb)
        self.assertEqual(precio, "0.25")

    def test_falla_si_no_hay_coincidencia(self) -> None:
        items = [
            _item(
                "Database Storage",
                {"usagetype": "USE2-Storage"},
                [{"unit": "GB-Mo", "usd": "9.99"}],
            )
        ]
        with self.assertRaises(precios_api.PrecioNoExtraible):
            precios_api.extraer_precio(items, self.consulta_ddb)

    def test_falla_si_la_unidad_no_coincide(self) -> None:
        items = [
            _item(
                "Database Storage",
                {"usagetype": "USE2-TimedStorage-ByteHrs"},
                [{"unit": "Hrs", "usd": "0.25"}],
            )
        ]
        with self.assertRaises(precios_api.PrecioNoExtraible):
            precios_api.extraer_precio(items, self.consulta_ddb)


class ExcluyeSoporteExtendido(unittest.TestCase):
    def test_elasticache_elige_nodeusage_no_extended_support(self) -> None:
        consulta = cr.catalogo_por_id()["precio_elasticache_redis"]
        items = [
            _item(
                "Cache Instance",
                {
                    "instanceType": "cache.t4g.micro",
                    "cacheEngine": "Redis",
                    "usagetype": "USE2-ExtendedSupportYr3-NodeUsage:cache.t4g.micro",
                },
                [{"unit": "Hrs", "usd": "0.0260000000"}],
            ),
            _item(
                "Cache Instance",
                {
                    "instanceType": "cache.t4g.micro",
                    "cacheEngine": "Redis",
                    "usagetype": "USE2-NodeUsage:cache.t4g.micro",
                },
                [{"unit": "Hrs", "usd": "0.0160000000"}],
            ),
        ]
        precio, detalle = precios_api.extraer_precio(items, consulta)
        self.assertEqual(precio, "0.016")
        self.assertEqual(detalle["usagetype"], "USE2-NodeUsage:cache.t4g.micro")

    def test_elasticache_rechaza_si_solo_hay_soporte_extendido(self) -> None:
        consulta = cr.catalogo_por_id()["precio_elasticache_redis"]
        items = [
            _item(
                "Cache Instance",
                {
                    "instanceType": "cache.t4g.micro",
                    "cacheEngine": "Redis",
                    "usagetype": "USE2-ExtendedSupportYr3-NodeUsage:cache.t4g.micro",
                },
                [{"unit": "Hrs", "usd": "0.026"}],
            )
        ]
        with self.assertRaises(precios_api.PrecioNoExtraible):
            precios_api.extraer_precio(items, consulta)


class Mapeo(unittest.TestCase):
    def test_cada_consulta_de_precio_tiene_sku(self) -> None:
        precio_ids = {c.id for c in cr.CATALOGO if c.id.startswith("precio_")}
        self.assertEqual(precio_ids, set(precios_api.MAPEO))

    def test_los_sku_mapeados_existen_en_el_catalogo(self) -> None:
        catalogo = json.loads(precios_api.RUTA_PRECIOS.read_text(encoding="utf-8"))
        for clave, _ in precios_api.MAPEO.values():
            self.assertIn(clave, catalogo["items"], clave)

    def test_dynamodb_escala_a_millon(self) -> None:
        self.assertEqual(precios_api.MAPEO["precio_ddb_rru"][1], "1000000")
        self.assertEqual(precios_api.MAPEO["precio_ddb_wru"][1], "1000000")
        self.assertEqual(precios_api.MAPEO["precio_alb_hora"][1], "1")


class ReutilizacionDeEvidencia(unittest.TestCase):
    def test_no_reconsulta_skus_ya_verificados(self) -> None:
        catalogo = json.loads(precios_api.RUTA_PRECIOS.read_text(encoding="utf-8"))
        resumen = precios_api.revalidar(
            "nexum", Path("."), catalogo=catalogo, aplicar=False, forzar=False
        )
        self.assertIn("EC2_T4G_MICRO_HORA", resumen["reutilizados"])
        self.assertEqual(resumen["consultados_ahora"], [])

    def test_estado_actual_del_catalogo_es_verificado(self) -> None:
        catalogo = json.loads(precios_api.RUTA_PRECIOS.read_text(encoding="utf-8"))
        for clave, _ in precios_api.MAPEO.values():
            item = catalogo["items"][clave]
            self.assertEqual(item["estado"], "VERIFIED_AWS", clave)
            self.assertIsNotNone(item["precio_usd"], clave)
            self.assertEqual(item["fecha_fuente"], "2026-11-06", clave)

    def test_fuente_cita_la_consulta(self) -> None:
        catalogo = json.loads(precios_api.RUTA_PRECIOS.read_text(encoding="utf-8"))
        fuente = catalogo["items"]["ALB_HORA"]["fuente"]
        self.assertIn("aws pricing get-products", fuente)
        self.assertIn("AWSELB", fuente)


if __name__ == "__main__":
    unittest.main()
