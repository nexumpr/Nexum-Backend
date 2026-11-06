"""Pruebas de la decisión de viabilidad FinOps (revisión 2).

Cada requisito bloqueante de la Corrección C tiene su prueba: cuenta no
verificada, plan Paid, expiración menor a 45 días, créditos insuficientes,
impuestos sin determinar, TLS sin resolver, precios sin verificar y topología
autocertificada. Sólo la evidencia completa produce ``VIABLE``.
"""

from __future__ import annotations

import json
import sys
import tempfile
import unittest
from datetime import date
from pathlib import Path

RAIZ = Path(__file__).resolve().parents[3]
sys.path.insert(0, str(RAIZ / "infra" / "finops"))

import viabilidad  # noqa: E402

EVIDENCIA_REAL = RAIZ / "infra" / "finops" / "evidencia"
PRECIOS = RAIZ / "infra" / "finops" / "precios_us-east-2.json"
TOPOLOGIA = EVIDENCIA_REAL / "topologia_c_2026-11-06.json"
HOY = date(2026, 11, 6)


def _codigos(decision: dict) -> set[str]:
    return {m["codigo"] for m in decision["motivos"]}


def _escribir(directorio: Path, identificador: str, contenido: dict) -> None:
    (directorio / f"{identificador}_2026-11-06.json").write_text(
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
                "estado_plan": "ACTIVE",
                "fecha_expiracion": "2027-05-04T01:22:26Z",
                "creditos_declarados_usd": "120",
                "creditos_verificados_usd": "120",
                "impuestos": "DETERMINADO",
            },
        },
        "servicios": {
            "id": "servicios",
            "fecha": "2026-11-06",
            "estado": "VERIFIED_AWS",
            "datos": {
                "redis_oss_sku": "cache.t4g.micro redis 7.1 (redis7)",
                "cloudfront_vpc_origin_habilitado": True,
                "cuota_vcpu_on_demand": 16,
                "az_con_t4g_micro": ["us-east-2a", "us-east-2b", "us-east-2c"],
                "dynamodb_limites": {"lecturas_max_cuenta": 80000},
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
            "datos": {"servicios_consultados": 0},
        },
    }


def _catalogo_totalmente_verificado(destino: Path) -> Path:
    catalogo = json.loads(PRECIOS.read_text(encoding="utf-8"))
    for item in catalogo["items"].values():
        if item["estado"] != "UNKNOWN":
            item["estado"] = "VERIFIED_AWS"
    ruta = destino / "precios_verificados.json"
    ruta.write_text(json.dumps(catalogo, ensure_ascii=False), encoding="utf-8")
    return ruta


def _topologia_resuelta(destino: Path) -> Path:
    topologia = json.loads(TOPOLOGIA.read_text(encoding="utf-8"))
    topologia["tls"]["aceptacion_humana_documentada"] = True
    ruta = destino / "topologia.json"
    ruta.write_text(json.dumps(topologia, ensure_ascii=False), encoding="utf-8")
    return ruta


class EvidenciaReal(unittest.TestCase):
    def test_evidencia_actual_bloquea(self) -> None:
        decision = viabilidad.evaluar(EVIDENCIA_REAL, PRECIOS, TOPOLOGIA, hoy=HOY)
        self.assertEqual(decision["estado"], "BLOCKED_FINOPS")
        codigos = _codigos(decision)
        for esperado in (
            "IMPUESTOS_NO_DETERMINADOS",
            "VPC_ORIGIN_NO_CONFIRMADO",
            "COSTO_OBSERVADO_NO_VERIFICADO",
            "PRECIOS_NO_VERIFICADOS",
            "COSTO_VERIFICADO_DESCONOCIDO",
            "COBERTURA_NO_CALCULABLE",
            "TLS_NO_RESUELTO",
        ):
            self.assertIn(esperado, codigos, decision["motivos"])

    def test_financiero_separa_calculado_y_verificado(self) -> None:
        decision = viabilidad.evaluar(EVIDENCIA_REAL, PRECIOS, TOPOLOGIA, hoy=HOY)
        fin = decision["resumen_financiero"]
        self.assertIsNotNone(fin["costo_bruto_calculado_usd"])
        self.assertIsNone(fin["costo_bruto_verificado_usd"])
        self.assertEqual(fin["cobertura_creditos"], "UNKNOWN")
        self.assertEqual(fin["impuestos"], "UNKNOWN")
        self.assertTrue(fin["riesgo_cobro_personal"])
        self.assertEqual(fin["politica_gasto_personal"], "CERO")
        self.assertTrue(fin["partidas_sin_verificar"])

    def test_creditos_verificados_vienen_de_aws(self) -> None:
        decision = viabilidad.evaluar(EVIDENCIA_REAL, PRECIOS, TOPOLOGIA, hoy=HOY)
        self.assertEqual(decision["resumen_financiero"]["creditos_verificados_usd"], "120.0000")


class RequisitosBloqueantes(unittest.TestCase):
    def _evaluar(self, datos: dict[str, dict], catalogo: Path, topologia: Path, hoy=HOY) -> dict:
        with tempfile.TemporaryDirectory() as tmp:
            dir_tmp = Path(tmp)
            for identificador, contenido in datos.items():
                _escribir(dir_tmp, identificador, contenido)
            return viabilidad.evaluar(dir_tmp, catalogo, topologia, hoy=hoy)

    def test_evidencia_completa_es_viable(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            dir_tmp = Path(tmp)
            catalogo = _catalogo_totalmente_verificado(dir_tmp)
            topologia = _topologia_resuelta(dir_tmp)
            decision = self._evaluar(_evidencia_verificada(), catalogo, topologia)
        self.assertEqual(decision["estado"], "VIABLE", decision["motivos"])
        self.assertEqual(decision["motivos"], [])
        self.assertEqual(decision["resumen_financiero"]["cobertura_creditos"], "TOTAL")

    def test_plan_paid_bloquea(self) -> None:
        datos = _evidencia_verificada()
        datos["cuenta"]["datos"]["plan"] = "PAID"
        with tempfile.TemporaryDirectory() as tmp:
            dir_tmp = Path(tmp)
            decision = self._evaluar(
                datos,
                _catalogo_totalmente_verificado(dir_tmp),
                _topologia_resuelta(dir_tmp),
            )
        self.assertIn("PLAN_NO_FREE", _codigos(decision))

    def test_vigencia_menor_a_45_dias_bloquea(self) -> None:
        datos = _evidencia_verificada()
        datos["cuenta"]["datos"]["fecha_expiracion"] = "2026-12-01"
        with tempfile.TemporaryDirectory() as tmp:
            dir_tmp = Path(tmp)
            decision = self._evaluar(
                datos,
                _catalogo_totalmente_verificado(dir_tmp),
                _topologia_resuelta(dir_tmp),
            )
        self.assertIn("VIGENCIA_INSUFICIENTE_45_DIAS", _codigos(decision))

    def test_vigencia_justa_de_45_dias_no_bloquea(self) -> None:
        datos = _evidencia_verificada()
        datos["cuenta"]["datos"]["fecha_expiracion"] = "2026-12-21"
        with tempfile.TemporaryDirectory() as tmp:
            dir_tmp = Path(tmp)
            decision = self._evaluar(
                datos,
                _catalogo_totalmente_verificado(dir_tmp),
                _topologia_resuelta(dir_tmp),
            )
        self.assertNotIn("VIGENCIA_INSUFICIENTE_45_DIAS", _codigos(decision))

    def test_creditos_insuficientes_bloquean(self) -> None:
        datos = _evidencia_verificada()
        datos["cuenta"]["datos"]["creditos_verificados_usd"] = "50"
        with tempfile.TemporaryDirectory() as tmp:
            dir_tmp = Path(tmp)
            decision = self._evaluar(
                datos,
                _catalogo_totalmente_verificado(dir_tmp),
                _topologia_resuelta(dir_tmp),
            )
        codigos = _codigos(decision)
        self.assertIn("CREDITOS_INSUFICIENTES", codigos)
        self.assertIn("RIESGO_COBRO_PERSONAL", codigos)

    def test_creditos_cero_bloquean(self) -> None:
        datos = _evidencia_verificada()
        datos["cuenta"]["datos"]["creditos_verificados_usd"] = "0"
        with tempfile.TemporaryDirectory() as tmp:
            dir_tmp = Path(tmp)
            decision = self._evaluar(
                datos,
                _catalogo_totalmente_verificado(dir_tmp),
                _topologia_resuelta(dir_tmp),
            )
        self.assertIn("CREDITOS_NO_VERIFICADOS", _codigos(decision))

    def test_impuestos_no_determinados_bloquean(self) -> None:
        datos = _evidencia_verificada()
        datos["cuenta"]["datos"]["impuestos"] = "UNKNOWN"
        with tempfile.TemporaryDirectory() as tmp:
            dir_tmp = Path(tmp)
            decision = self._evaluar(
                datos,
                _catalogo_totalmente_verificado(dir_tmp),
                _topologia_resuelta(dir_tmp),
            )
        self.assertIn("IMPUESTOS_NO_DETERMINADOS", _codigos(decision))

    def test_tls_sin_resolver_bloquea(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            dir_tmp = Path(tmp)
            decision = self._evaluar(
                _evidencia_verificada(),
                _catalogo_totalmente_verificado(dir_tmp),
                TOPOLOGIA,
            )
        self.assertIn("TLS_NO_RESUELTO", _codigos(decision))

    def test_vpc_origin_no_confirmado_bloquea(self) -> None:
        datos = _evidencia_verificada()
        datos["servicios"]["datos"]["cloudfront_vpc_origin_habilitado"] = None
        with tempfile.TemporaryDirectory() as tmp:
            dir_tmp = Path(tmp)
            decision = self._evaluar(
                datos,
                _catalogo_totalmente_verificado(dir_tmp),
                _topologia_resuelta(dir_tmp),
            )
        self.assertIn("VPC_ORIGIN_NO_CONFIRMADO", _codigos(decision))

    def test_cuota_insuficiente_bloquea(self) -> None:
        datos = _evidencia_verificada()
        datos["servicios"]["datos"]["cuota_vcpu_on_demand"] = 1
        with tempfile.TemporaryDirectory() as tmp:
            dir_tmp = Path(tmp)
            decision = self._evaluar(
                datos,
                _catalogo_totalmente_verificado(dir_tmp),
                _topologia_resuelta(dir_tmp),
            )
        self.assertIn("CUOTA_EC2_INSUFICIENTE", _codigos(decision))

    def test_una_sola_zona_bloquea(self) -> None:
        datos = _evidencia_verificada()
        datos["servicios"]["datos"]["az_con_t4g_micro"] = ["us-east-2a"]
        with tempfile.TemporaryDirectory() as tmp:
            dir_tmp = Path(tmp)
            decision = self._evaluar(
                datos,
                _catalogo_totalmente_verificado(dir_tmp),
                _topologia_resuelta(dir_tmp),
            )
        self.assertIn("TIPO_INSTANCIA_NO_CONFIRMADO", _codigos(decision))

    def test_precios_sin_verificar_bloquean(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            dir_tmp = Path(tmp)
            decision = self._evaluar(
                _evidencia_verificada(),
                PRECIOS,
                _topologia_resuelta(dir_tmp),
            )
        codigos = _codigos(decision)
        self.assertIn("PRECIOS_NO_VERIFICADOS", codigos)
        self.assertIn("COSTO_VERIFICADO_DESCONOCIDO", codigos)
        self.assertIn("COBERTURA_NO_CALCULABLE", codigos)

    def test_topologia_autocertificada_bloquea(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            dir_tmp = Path(tmp)
            topologia = json.loads(TOPOLOGIA.read_text(encoding="utf-8"))
            topologia["estado_verificacion"]["runtime"] = "AWS_DEPLOYMENT_VERIFIED"
            ruta = dir_tmp / "topologia.json"
            ruta.write_text(json.dumps(topologia, ensure_ascii=False), encoding="utf-8")
            decision = self._evaluar(
                _evidencia_verificada(),
                _catalogo_totalmente_verificado(dir_tmp),
                ruta,
            )
        self.assertIn("TOPOLOGIA_INCUMPLE", _codigos(decision))

    def test_una_instancia_bloquea(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            dir_tmp = Path(tmp)
            topologia = json.loads(TOPOLOGIA.read_text(encoding="utf-8"))
            topologia["instancias"] = topologia["instancias"][:1]
            topologia["tls"]["aceptacion_humana_documentada"] = True
            ruta = dir_tmp / "topologia.json"
            ruta.write_text(json.dumps(topologia, ensure_ascii=False), encoding="utf-8")
            decision = self._evaluar(
                _evidencia_verificada(),
                _catalogo_totalmente_verificado(dir_tmp),
                ruta,
            )
        self.assertIn("TOPOLOGIA_INCUMPLE", _codigos(decision))

    def test_runtime_siempre_not_run_sin_evidencia(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            dir_tmp = Path(tmp)
            decision = self._evaluar(
                _evidencia_verificada(),
                _catalogo_totalmente_verificado(dir_tmp),
                _topologia_resuelta(dir_tmp),
            )
        self.assertEqual(decision["revision_topologia"]["estado_despliegue"], "NOT_RUN")


if __name__ == "__main__":
    unittest.main()
