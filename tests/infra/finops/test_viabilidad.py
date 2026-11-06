"""Pruebas de la decisión de viabilidad FinOps (VIABLE vs BLOCKED_FINOPS)."""

from __future__ import annotations

import json
import sys
import tempfile
import unittest
from datetime import date
from pathlib import Path

RAIZ = Path(__file__).resolve().parents[3]
sys.path.insert(0, str(RAIZ / "infra" / "finops"))

import modelo_costos  # noqa: E402
import viabilidad  # noqa: E402

EVIDENCIA_REAL = RAIZ / "infra" / "finops" / "evidencia"
PRECIOS = RAIZ / "infra" / "finops" / "precios_us-east-2.json"
TOPOLOGIA = EVIDENCIA_REAL / "topologia_c_2026-11-06.json"
HOY = date(2026, 11, 6)


def _codigos(decision: dict) -> set[str]:
    return {m["codigo"] for m in decision["motivos"]}


def _escribir(dir_destino: Path, identificador: str, contenido: dict) -> None:
    (dir_destino / f"{identificador}_2026-11-06.json").write_text(
        json.dumps(contenido, ensure_ascii=False), encoding="utf-8"
    )


def _evidencia_verificada() -> dict[str, dict]:
    return {
        "cuenta": {
            "id": "cuenta",
            "fecha": "2026-11-06",
            "estado": "VERIFIED_AWS",
            "datos": {
                "plan": "FREE",
                "fecha_alta": "2026-05-01",
                "fecha_expiracion": "2027-05-01",
                "creditos_declarados_usd": "120",
                "creditos_verificados_usd": "120",
            },
        },
        "servicios": {
            "id": "servicios",
            "fecha": "2026-11-06",
            "estado": "VERIFIED_AWS",
            "datos": {
                "redis_oss_sku": "cache.t4g.micro redis 7.x",
                "cloudfront_vpc_origin_habilitado": True,
                "cuota_vcpu_on_demand": 8,
                "t4g_micro_disponible_az": True,
                "dynamodb_limites": "ok",
            },
        },
        "costos_observados": {
            "id": "costos_observados",
            "fecha": "2026-11-06",
            "estado": "VERIFIED_AWS",
            "datos": {"unblended_cost_usd": "0.00"},
        },
        "free_tier": {
            "id": "free_tier",
            "fecha": "2026-11-06",
            "estado": "VERIFIED_AWS",
            "datos": {"servicios_consultados": 5},
        },
    }


class ViabilidadFinOps(unittest.TestCase):
    def test_evidencia_actual_bloquea(self) -> None:
        decision = viabilidad.evaluar(EVIDENCIA_REAL, PRECIOS, TOPOLOGIA, hoy=HOY)
        self.assertEqual(decision["estado"], "BLOCKED_FINOPS")
        codigos = _codigos(decision)
        for esperado in (
            "EVIDENCIA_CUENTA_NO_VERIFICADA",
            "PLAN_NO_DETERMINADO",
            "VIGENCIA_NO_DETERMINADA",
            "CREDITOS_NO_VERIFICADOS",
            "REDIS_OSS_NO_CONFIRMADO",
            "VPC_ORIGIN_NO_CONFIRMADO",
            "CUOTA_EC2_INSUFICIENTE",
            "COSTO_OBSERVADO_NO_VERIFICADO",
            "PRECIOS_NO_VERIFICADOS",
        ):
            self.assertIn(esperado, codigos)

    def test_decision_incluye_desembolso_cero_y_bruto_conocido(self) -> None:
        decision = viabilidad.evaluar(EVIDENCIA_REAL, PRECIOS, TOPOLOGIA, hoy=HOY)
        self.assertEqual(decision["resumen_financiero"]["desembolso_personal_usd"], "0.0000")
        self.assertEqual(decision["resumen_financiero"]["total_bruto_45d_usd"], "84.0580")
        self.assertEqual(decision["resumen_financiero"]["impuestos"], "UNKNOWN")

    def test_evidencia_completa_es_viable(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            dir_tmp = Path(tmp)
            for identificador, contenido in _evidencia_verificada().items():
                _escribir(dir_tmp, identificador, contenido)
            catalogo = json.loads(PRECIOS.read_text(encoding="utf-8"))
            for item in catalogo["items"].values():
                if item["estado"] != "UNKNOWN":
                    item["estado"] = "VERIFIED_AWS"
            ruta_precios = dir_tmp / "precios.json"
            ruta_precios.write_text(json.dumps(catalogo, ensure_ascii=False), encoding="utf-8")

            decision = viabilidad.evaluar(dir_tmp, ruta_precios, TOPOLOGIA, hoy=HOY)
            self.assertEqual(decision["estado"], "VIABLE", decision["motivos"])
            self.assertEqual(decision["motivos"], [])
            self.assertIn("IVA/impuestos no determinados", " ".join(decision["condiciones"]))

    def test_plan_paid_bloquea(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            dir_tmp = Path(tmp)
            datos = _evidencia_verificada()
            datos["cuenta"]["datos"]["plan"] = "PAID"
            for identificador, contenido in datos.items():
                _escribir(dir_tmp, identificador, contenido)
            decision = viabilidad.evaluar(dir_tmp, PRECIOS, TOPOLOGIA, hoy=HOY)
            self.assertIn("PLAN_NO_FREE", _codigos(decision))

    def test_vigencia_vencida_bloquea(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            dir_tmp = Path(tmp)
            datos = _evidencia_verificada()
            datos["cuenta"]["datos"]["fecha_expiracion"] = "2026-10-01"
            for identificador, contenido in datos.items():
                _escribir(dir_tmp, identificador, contenido)
            decision = viabilidad.evaluar(dir_tmp, PRECIOS, TOPOLOGIA, hoy=HOY)
            self.assertIn("VIGENCIA_VENCIDA", _codigos(decision))

    def test_creditos_cero_bloquean(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            dir_tmp = Path(tmp)
            datos = _evidencia_verificada()
            datos["cuenta"]["datos"]["creditos_verificados_usd"] = "0"
            for identificador, contenido in datos.items():
                _escribir(dir_tmp, identificador, contenido)
            decision = viabilidad.evaluar(dir_tmp, PRECIOS, TOPOLOGIA, hoy=HOY)
            codigos = _codigos(decision)
            self.assertIn("CREDITOS_NO_VERIFICADOS", codigos)
            self.assertIn("CREDITOS_INSUFICIENTES", codigos)

    def test_topologia_incumple_bloquea(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            dir_tmp = Path(tmp)
            for identificador, contenido in _evidencia_verificada().items():
                _escribir(dir_tmp, identificador, contenido)
            topologia = json.loads(TOPOLOGIA.read_text(encoding="utf-8"))
            topologia["instancias"] = topologia["instancias"][:1]
            ruta_topologia = dir_tmp / "topologia.json"
            ruta_topologia.write_text(json.dumps(topologia, ensure_ascii=False), encoding="utf-8")

            decision = viabilidad.evaluar(dir_tmp, PRECIOS, ruta_topologia, hoy=HOY)
            self.assertIn("TOPOLOGIA_INCUMPLE", _codigos(decision))
            self.assertFalse(decision["revision_topologia"]["diseno_valido"])

    def test_precio_verificado_parcial_no_es_viable(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            dir_tmp = Path(tmp)
            for identificador, contenido in _evidencia_verificada().items():
                _escribir(dir_tmp, identificador, contenido)
            catalogo = json.loads(PRECIOS.read_text(encoding="utf-8"))
            for item in catalogo["items"].values():
                if item["estado"] != "UNKNOWN":
                    item["estado"] = "VERIFIED_AWS"
            catalogo["items"]["REDIS_OSS_SKU_FALTANTE"] = catalogo["items"].pop(
                "ELASTICACHE_REDIS_T4G_MICRO_HORA"
            )
            ruta_precios = dir_tmp / "precios.json"
            ruta_precios.write_text(json.dumps(catalogo, ensure_ascii=False), encoding="utf-8")

            decision = viabilidad.evaluar(dir_tmp, ruta_precios, TOPOLOGIA, hoy=HOY)
            self.assertIn("PRECIOS_NO_VERIFICADOS", _codigos(decision))
            self.assertIn("COSTO_BRUTO_DESCONOCIDO", _codigos(decision))
            self.assertEqual(decision["estado"], "BLOCKED_FINOPS")

    def test_costos_calculados_con_precios_reales_del_repo(self) -> None:
        catalogo = modelo_costos.cargar_precios(PRECIOS)
        resumen = modelo_costos.calcular_todo(catalogo, "0")
        self.assertEqual(
            resumen["escenarios"]["C_CLOUDFRONT_VPC_ORIGIN_PRIVADO"]["total_bruto_45d_usd"], "84.0580"
        )


if __name__ == "__main__":
    unittest.main()
