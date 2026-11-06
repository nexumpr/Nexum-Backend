"""Pruebas del recolector de evidencia AWS — revisión 2.

Verifican los defectos corregidos:

- lectura desde el directorio de evidencia sanitizada (antes todo llegaba vacío);
- nombres de campo reales de AWS (``accountPlanType``,
  ``accountPlanRemainingCredits``, ``VpcOriginList.Items``, ``freeTierUsages``);
- ``list-vpc-origins`` no se interpreta como elegibilidad de VPC Origins;
- no se inventan créditos ni se marca VERIFIED_AWS sin respuesta válida.
"""

from __future__ import annotations

import json
import sys
import tempfile
import unittest
from pathlib import Path

RAIZ = Path(__file__).resolve().parents[3]
sys.path.insert(0, str(RAIZ / "infra" / "finops"))

import evidencia_aws as ea  # noqa: E402

PLAN_FREE = {
    "accountId": "000000000000",
    "accountPlanType": "FREE",
    "accountPlanStatus": "ACTIVE",
    "accountPlanExpirationDate": "2027-05-04T01:22:26.960000+00:00",
    "accountPlanRemainingCredits": {"amount": 120.0, "unit": "USD"},
}
IDENTIDAD = {
    "UserId": "AROAXXXX:sesion",
    "Account": "000000000000",
    "Arn": "arn:aws:sts::000000000000:assumed-role/Rol/sesion",
}
REDIS = {
    "CacheEngineVersions": [
        {
            "Engine": "redis",
            "EngineVersion": "6.2.6",
            "CacheParameterGroupFamily": "redis6.x",
        },
        {
            "Engine": "redis",
            "EngineVersion": "7.1",
            "CacheParameterGroupFamily": "redis7",
        },
    ]
}
ZONAS = {
    "InstanceTypeOfferings": [
        {"InstanceType": "t4g.micro", "Location": "us-east-2a"},
        {"InstanceType": "t4g.micro", "Location": "us-east-2b"},
    ]
}
CUOTA = {"Quota": {"Value": 16.0, "QuotaCode": "L-1216C47A"}}
ORIGINS_VACIO = {"VpcOriginList": {"Items": [], "MaxItems": 100, "Quantity": 0}}
DDB = {
    "AccountMaxReadCapacityUnits": 80000,
    "AccountMaxWriteCapacityUnits": 80000,
    "TableMaxReadCapacityUnits": 40000,
    "TableMaxWriteCapacityUnits": 40000,
}
FREE_TIER = {"freeTierUsages": []}

TODAS_PASS = {
    identificador: {"estado": "PASS"}
    for identificador in (
        "sts_identidad",
        "plan_free",
        "free_tier_uso",
        "cuota_ec2_vcpu",
        "tipos_instancia",
        "redis_versiones",
        "cloudfront_vpc_origins",
        "ddb_limites",
    )
}


def _escribir(directorio: Path, identificador: str, contenido: dict) -> None:
    (directorio / f"{identificador}.json").write_text(
        json.dumps(contenido), encoding="utf-8"
    )


class LecturaDeEvidencia(unittest.TestCase):
    def test_lee_del_directorio_de_evidencia(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            evidencia = Path(tmp) / "evidencia"
            crudo = Path(tmp) / "crudo"
            evidencia.mkdir()
            crudo.mkdir()
            _escribir(evidencia, "plan_free", PLAN_FREE)
            datos = ea._leer("plan_free", evidencia, crudo)
            self.assertEqual(datos["accountPlanType"], "FREE")

    def test_usa_el_crudo_si_falta_la_evidencia(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            evidencia = Path(tmp) / "evidencia"
            crudo = Path(tmp) / "crudo"
            evidencia.mkdir()
            crudo.mkdir()
            (crudo / "plan_free.raw.txt").write_text(json.dumps(PLAN_FREE), encoding="utf-8")
            datos = ea._leer("plan_free", evidencia, crudo)
            self.assertEqual(datos["accountPlanStatus"], "ACTIVE")

    def test_devuelve_none_si_no_hay_nada(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            evidencia = Path(tmp) / "evidencia"
            crudo = Path(tmp) / "crudo"
            evidencia.mkdir()
            crudo.mkdir()
            self.assertIsNone(ea._leer("plan_free", evidencia, crudo))


class Cuenta(unittest.TestCase):
    def _construir(self, plan: dict | None, resultados: dict | None = None) -> dict:
        with tempfile.TemporaryDirectory() as tmp:
            evidencia = Path(tmp) / "evidencia"
            crudo = Path(tmp) / "crudo"
            evidencia.mkdir()
            crudo.mkdir()
            _escribir(evidencia, "sts_identidad", IDENTIDAD)
            if plan is not None:
                _escribir(evidencia, "plan_free", plan)
            return ea.construir_cuenta(crudo, resultados or TODAS_PASS, evidencia)

    def test_plan_free_verificado(self) -> None:
        cuenta = self._construir(PLAN_FREE)
        self.assertEqual(cuenta["estado"], "VERIFIED_AWS")
        self.assertEqual(cuenta["datos"]["plan"], "FREE")
        self.assertEqual(cuenta["datos"]["estado_plan"], "ACTIVE")
        self.assertEqual(cuenta["datos"]["fecha_expiracion"][:10], "2027-05-04")

    def test_creditos_tomados_de_aws(self) -> None:
        cuenta = self._construir(PLAN_FREE)
        self.assertEqual(cuenta["datos"]["creditos_verificados_usd"], "120.0")
        self.assertEqual(cuenta["datos"]["creditos_unidad"], "USD")
        self.assertIn("accountPlanRemainingCredits", cuenta["datos"]["fuente_creditos"])

    def test_creditos_declarados_no_son_verificados(self) -> None:
        cuenta = self._construir(PLAN_FREE)
        self.assertEqual(cuenta["datos"]["creditos_declarados_usd"], "120")

    def test_sin_plan_no_se_marca_verificado(self) -> None:
        cuenta = self._construir(None)
        self.assertEqual(cuenta["estado"], "NOT_VERIFIED")
        self.assertIsNone(cuenta["datos"]["plan"])

    def test_plan_paid_se_reporta_tal_cual(self) -> None:
        paid = dict(PLAN_FREE)
        paid["accountPlanType"] = "PAID"
        cuenta = self._construir(paid)
        self.assertEqual(cuenta["datos"]["plan"], "PAID")

    def test_impuestos_no_se_asumen_determinados(self) -> None:
        self.assertEqual(self._construir(PLAN_FREE)["datos"]["impuestos"], "UNKNOWN")

    def test_no_expone_numero_de_cuenta(self) -> None:
        texto = json.dumps(self._construir(PLAN_FREE))
        self.assertNotIn("000000000000", texto)


class Servicios(unittest.TestCase):
    def _construir(self, origenes: dict | None = None, versiones: dict | None = None) -> dict:
        with tempfile.TemporaryDirectory() as tmp:
            evidencia = Path(tmp) / "evidencia"
            crudo = Path(tmp) / "crudo"
            evidencia.mkdir()
            crudo.mkdir()
            _escribir(evidencia, "redis_versiones", versiones or REDIS)
            _escribir(evidencia, "tipos_instancia", ZONAS)
            _escribir(evidencia, "cuota_ec2_vcpu", CUOTA)
            _escribir(evidencia, "cloudfront_vpc_origins", origenes or ORIGINS_VACIO)
            _escribir(evidencia, "ddb_limites", DDB)
            return ea.construir_servicios(crudo, TODAS_PASS, evidencia)

    def test_redis_elige_la_version_mas_alta(self) -> None:
        servicios = self._construir()
        self.assertIn("7.1", servicios["datos"]["redis_oss_sku"])
        self.assertIn("redis7", servicios["datos"]["redis_oss_sku"])

    def test_zonas_y_cuota(self) -> None:
        servicios = self._construir()
        self.assertEqual(servicios["datos"]["az_con_t4g_micro"], ["us-east-2a", "us-east-2b"])
        self.assertEqual(servicios["datos"]["cuota_vcpu_on_demand"], 16)
        self.assertEqual(servicios["estado"], "VERIFIED_AWS")

    def test_vpc_origins_no_se_deduce_de_list(self) -> None:
        servicios = self._construir()
        self.assertIsNone(servicios["datos"]["cloudfront_vpc_origin_habilitado"])
        self.assertEqual(servicios["datos"]["vpc_origins_observados"], 0)
        self.assertIn("no demuestra", servicios["datos"]["cloudfront_vpc_origin_nota"])

    def test_una_zona_sola_no_verifica_servicios(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            evidencia = Path(tmp) / "evidencia"
            crudo = Path(tmp) / "crudo"
            evidencia.mkdir()
            crudo.mkdir()
            _escribir(evidencia, "redis_versiones", REDIS)
            _escribir(
                evidencia,
                "tipos_instancia",
                {"InstanceTypeOfferings": [{"InstanceType": "t4g.micro", "Location": "us-east-2a"}]},
            )
            _escribir(evidencia, "cuota_ec2_vcpu", CUOTA)
            _escribir(evidencia, "cloudfront_vpc_origins", ORIGINS_VACIO)
            _escribir(evidencia, "ddb_limites", DDB)
            servicios = ea.construir_servicios(crudo, TODAS_PASS, evidencia)
        self.assertEqual(servicios["estado"], "NOT_VERIFIED")

    def test_limites_dynamodb_reales(self) -> None:
        servicios = self._construir()
        self.assertEqual(servicios["datos"]["dynamodb_limites"]["lecturas_max_cuenta"], 80000)


class FreeTier(unittest.TestCase):
    def _construir(self, datos: dict, estado: str = "PASS") -> dict:
        with tempfile.TemporaryDirectory() as tmp:
            evidencia = Path(tmp) / "evidencia"
            crudo = Path(tmp) / "crudo"
            evidencia.mkdir()
            crudo.mkdir()
            _escribir(evidencia, "free_tier_uso", datos)
            return ea.construir_free_tier(crudo, {"free_tier_uso": {"estado": estado}}, evidencia)

    def test_lista_vacia_no_prueba_ausencia_de_cargos(self) -> None:
        free = self._construir(FREE_TIER)
        self.assertEqual(free["estado"], "VERIFIED_AWS")
        self.assertEqual(free["datos"]["servicios_consultados"], 0)
        self.assertIn("no prueba", free["nota"])

    def test_consulta_fallida_es_not_run(self) -> None:
        free = self._construir(FREE_TIER, estado="FAIL")
        self.assertEqual(free["estado"], "NOT_RUN")


class TemporalWindows(unittest.TestCase):
    def test_directorio_temporal_es_absoluto(self) -> None:
        ruta = ea.directorio_crudo_temporal()
        self.assertTrue(ruta.is_absolute())
        self.assertNotIn("$env", str(ruta))

    def test_directorio_temporal_fuera_del_repo(self) -> None:
        ruta = ea.directorio_crudo_temporal().resolve()
        self.assertFalse(str(ruta).lower().startswith(str(RAIZ.resolve()).lower()))


if __name__ == "__main__":
    unittest.main()
