"""Control FinOps diario de I-120: umbrales, forecast y fallo cerrado."""

from __future__ import annotations

import sys
import unittest
from datetime import date
from decimal import Decimal
from pathlib import Path

RAIZ = Path(__file__).resolve().parents[3]
sys.path.insert(0, str(RAIZ / "herramientas"))

import verificar_finops_diario as fin  # noqa: E402

INVENTARIO = RAIZ / "infra" / "terraform" / "operacion" / "inventario.json"


def _evidencia(**extra) -> dict:
    base = {
        "tipo_cuenta": "FREE",
        "plan": "FREE",
        "plan_activo": True,
        "creditos_usd": "120.0",
        "consumo_usd": "20.0",
        "forecast_usd": "60.0",
        "horizonte_dias": 45,
        "targets_sanos": 2,
        "verificado_creditos_usd": True,
        "verificado_consumo_usd": True,
        "verificado_forecast_usd": True,
        "verificado_plan_activo": True,
        "cuenta_verificada": True,
        "scp_verificado": True,
        "fuente": "EVIDENCIA_AUTORIZADA",
    }
    base.update(extra)
    return base


class Umbrales(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.umbrales = fin.umbrales_de_inventario(INVENTARIO)

    def test_umbrales_coinciden_con_la_spec(self) -> None:
        self.assertEqual(self.umbrales["aviso"], "45")
        self.assertEqual(self.umbrales["bloqueo_auxiliares"], "35")
        self.assertEqual(self.umbrales["revision_urgente"], "30")
        self.assertEqual(self.umbrales["incidente_critico"], "20")

    def test_niveles_por_saldo_restante(self) -> None:
        casos = {
            "50": "OK",
            "45": "AVISO",
            "40": "AVISO",
            "35": "BLOQUEADO",
            "31": "BLOQUEADO",
            "30": "REVISION_URGENTE",
            "21": "REVISION_URGENTE",
            "19.99": "INCIDENTE_CRITICO",
        }
        for restante, esperado in casos.items():
            with self.subTest(restante=restante):
                self.assertEqual(fin.nivel_por_saldo(Decimal(restante), self.umbrales), esperado)


class FalloCerrado(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.umbrales = fin.umbrales_de_inventario(INVENTARIO)

    def test_sin_evidencia_bloquea(self) -> None:
        resultado = fin.evaluar({"plan": "FREE"}, self.umbrales)
        self.assertEqual(resultado["nivel"], "BLOQUEADO_SIN_DATOS")
        self.assertTrue(resultado["bloquea_auxiliares"])
        self.assertTrue(resultado["requiere_escalado"])
        self.assertIn("no declarar aws_credits", " ".join(resultado["acciones"]).lower())

    def test_plan_inactivo_bloquea(self) -> None:
        resultado = fin.evaluar(_evidencia(plan_activo=False), self.umbrales)
        self.assertEqual(resultado["nivel"], "BLOQUEADO_SIN_DATOS")
        self.assertIn("plan_activo=false", resultado["motivo"])

    def test_sin_creditos_no_se_asume_saldo(self) -> None:
        evidencia = _evidencia()
        evidencia.pop("creditos_usd")
        resultado = fin.evaluar(evidencia, self.umbrales)
        self.assertEqual(resultado["nivel"], "BLOQUEADO_SIN_DATOS")
        self.assertNotIn("restante_usd", resultado)

    def test_evidencia_no_verificada_se_declara(self) -> None:
        resultado = fin.evaluar(_evidencia(verificado_creditos_usd=False), self.umbrales)
        self.assertIn("creditos_usd", resultado["datos_no_verificados"])

    def test_horizonte_invalido_es_error(self) -> None:
        with self.assertRaises(fin.ErrorEvidencia):
            fin.evaluar(_evidencia(horizonte_dias=0), self.umbrales)


class Decisiones(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.umbrales = fin.umbrales_de_inventario(INVENTARIO)

    def test_saldo_sano_no_bloquea(self) -> None:
        resultado = fin.evaluar(_evidencia(), self.umbrales)
        self.assertEqual(resultado["nivel"], "OK")
        self.assertFalse(resultado["bloquea_auxiliares"])

    def test_aviso_no_bloquea_por_si_solo(self) -> None:
        resultado = fin.evaluar(
            _evidencia(creditos_usd="60.0", consumo_usd="18.0", forecast_usd="30.0"), self.umbrales
        )
        self.assertEqual(resultado["nivel"], "AVISO")
        self.assertFalse(resultado["bloquea_auxiliares"])

    def test_bloqueo_por_saldo(self) -> None:
        resultado = fin.evaluar(
            _evidencia(creditos_usd="40.0", consumo_usd="8.0", forecast_usd="20.0"), self.umbrales
        )
        self.assertEqual(resultado["nivel"], "BLOQUEADO")
        self.assertTrue(resultado["bloquea_auxiliares"])

    def test_forecast_excedido_bloquea_despliegues(self) -> None:
        resultado = fin.evaluar(
            _evidencia(creditos_usd="120.0", consumo_usd="10.0", forecast_usd="130.0"),
            self.umbrales,
        )
        self.assertEqual(resultado["nivel"], "BLOQUEO_FORECAST")
        self.assertTrue(resultado["bloquea_auxiliares"])
        self.assertTrue(resultado["requiere_escalado"])

    def test_vencimiento_antes_del_horizonte_bloquea(self) -> None:
        resultado = fin.evaluar(
            _evidencia(creditos_vencimiento="2026-12-01"), self.umbrales, date(2026, 11, 9)
        )
        self.assertTrue(resultado["bloquea_auxiliares"])
        self.assertEqual(resultado["dias_para_vencer"], 22)

    def test_vencimiento_suficiente_no_bloquea(self) -> None:
        resultado = fin.evaluar(
            _evidencia(creditos_vencimiento="2027-05-04"), self.umbrales, date(2026, 11, 9)
        )
        self.assertFalse(resultado["bloquea_auxiliares"])

    def test_no_apaga_el_core_ni_cambia_plan(self) -> None:
        resultado = fin.evaluar(_evidencia(), self.umbrales)
        texto = " ".join(resultado["no_hace"])
        self.assertIn("no apaga el core", texto)
        self.assertIn("no cambia el plan Free", texto)
        self.assertIn("hard cap", texto)

    def test_dos_nodos_simulados(self) -> None:
        self.assertTrue(fin.evaluar(_evidencia(targets_sanos=2), self.umbrales)["dos_nodos_ok"]["cumple"])
        self.assertFalse(fin.evaluar(_evidencia(targets_sanos=1), self.umbrales)["dos_nodos_ok"]["cumple"])
        self.assertFalse(fin.evaluar(_evidencia(targets_sanos=None), self.umbrales)["dos_nodos_ok"]["cumple"])

    def test_no_declara_costo_observado(self) -> None:
        resultado = fin.evaluar(_evidencia(), self.umbrales)
        self.assertIn("no es costo observado", resultado["motivo"])


class ControlesNuevos(unittest.TestCase):
    """Fail-closed de evidencia no verificada, cifras invalidas y plan."""

    @classmethod
    def setUpClass(cls) -> None:
        cls.umbrales = fin.umbrales_de_inventario(INVENTARIO)

    def test_credito_no_verificado_bloquea(self) -> None:
        resultado = fin.evaluar(_evidencia(verificado_creditos_usd=False), self.umbrales)
        self.assertEqual(resultado["nivel"], "BLOQUEADO_SIN_DATOS")
        self.assertTrue(resultado["bloquea_auxiliares"])
        self.assertIn("creditos_usd", resultado["motivo"])

    def test_consumo_no_verificado_bloquea(self) -> None:
        resultado = fin.evaluar(_evidencia(verificado_consumo_usd=False), self.umbrales)
        self.assertEqual(resultado["nivel"], "BLOQUEADO_SIN_DATOS")

    def test_forecast_no_verificado_bloquea(self) -> None:
        resultado = fin.evaluar(_evidencia(verificado_forecast_usd=False), self.umbrales)
        self.assertEqual(resultado["nivel"], "BLOQUEADO_SIN_DATOS")

    def test_plan_activo_no_verificado_bloquea(self) -> None:
        resultado = fin.evaluar(_evidencia(verificado_plan_activo=False), self.umbrales)
        self.assertEqual(resultado["nivel"], "BLOQUEADO_SIN_DATOS")

    def test_plan_distinto_al_aprobado_bloquea(self) -> None:
        resultado = fin.evaluar(_evidencia(plan="PAID"), self.umbrales)
        self.assertEqual(resultado["nivel"], "BLOQUEADO_PLAN_NO_APROBADO")
        self.assertTrue(resultado["bloquea_auxiliares"])

    def test_creditos_caducados_bloquean(self) -> None:
        resultado = fin.evaluar(
            _evidencia(creditos_vencimiento="2026-01-01"), self.umbrales, date(2026, 11, 9)
        )
        self.assertEqual(resultado["nivel"], "BLOQUEADO_CREDITOS_VENCIDOS")
        self.assertTrue(resultado["creditos_vencidos"])

    def test_saldo_negativo_es_error(self) -> None:
        with self.assertRaises(fin.ErrorEvidencia):
            fin.evaluar(_evidencia(consumo_usd="500.0"), self.umbrales)

    def test_nan_e_infinito_rechazados(self) -> None:
        for valor in ("NaN", "Infinity", "-Infinity", "inf"):
            with self.subTest(valor=valor):
                with self.assertRaises(fin.ErrorEvidencia):
                    fin.evaluar(_evidencia(creditos_usd=valor), self.umbrales)

    def test_restante_explicito_no_descuenta_dos_veces(self) -> None:
        resultado = fin.evaluar(
            _evidencia(creditos_usd="120.0", consumo_usd="20.0", creditos_restante_usd="100.0"),
            self.umbrales,
        )
        self.assertEqual(resultado["restante_usd"], "100.0000")
        self.assertEqual(resultado["saldo_calculado_desde"], "creditos_restante_usd")

    def test_restante_negativo_es_error(self) -> None:
        with self.assertRaises(fin.ErrorEvidencia):
            fin.evaluar(_evidencia(creditos_restante_usd="-1"), self.umbrales)

    def test_cuenta_y_scp_sin_verificar_bloquean(self) -> None:
        resultado = fin.evaluar(_evidencia(cuenta_verificada=False), self.umbrales)
        self.assertTrue(resultado["bloquea_auxiliares"])
        self.assertIn("cuenta_verificada", resultado["cuenta_sin_verificar"])

    def test_evidencia_sintetica_nunca_autoriza_ejecucion_real(self) -> None:
        resultado = fin.evaluar(_evidencia(), self.umbrales)
        self.assertFalse(resultado["autoriza_ejecucion_real"])
        self.assertFalse(resultado["es_prueba_independiente_de_facturacion"])
        self.assertEqual(resultado["procedencia_evidencia"], "DECLARADA_LOCAL")

    def test_umbrales_exactos_en_los_limites(self) -> None:
        casos = {
            "45": "AVISO",
            "35": "BLOQUEADO",
            "30": "REVISION_URGENTE",
            "19.99": "INCIDENTE_CRITICO",
            "46": "OK",
        }
        for restante, esperado in casos.items():
            with self.subTest(restante=restante):
                resultado = fin.evaluar(
                    _evidencia(creditos_restante_usd=restante, forecast_usd="10.0"),
                    self.umbrales,
                )
                self.assertEqual(resultado["nivel"], esperado)


class Cli(unittest.TestCase):
    def test_main_bloqueante_devuelve_1(self) -> None:
        import json
        import tempfile

        ruta = Path(tempfile.mkdtemp()) / "evidencia.json"
        ruta.write_text(
            json.dumps(_evidencia(creditos_usd="10.0", consumo_usd="0.0", forecast_usd="5.0")),
            encoding="utf-8",
        )
        self.assertEqual(fin.main(["--evidencia", str(ruta)]), 1)

    def test_main_sano_devuelve_0(self) -> None:
        import json
        import tempfile

        ruta = Path(tempfile.mkdtemp()) / "evidencia.json"
        ruta.write_text(json.dumps(_evidencia()), encoding="utf-8")
        self.assertEqual(fin.main(["--evidencia", str(ruta)]), 0)

    def test_main_sin_fichero_devuelve_2(self) -> None:
        self.assertEqual(fin.main(["--evidencia", "no-existe.json"]), 2)


if __name__ == "__main__":  # pragma: no cover
    unittest.main()
