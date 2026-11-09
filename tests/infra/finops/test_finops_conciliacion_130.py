"""Conciliacion FinOps de I-130 contra el modelo H05: sin doble conteo."""

from __future__ import annotations

import json
import sys
import tempfile
import unittest
from decimal import Decimal
from pathlib import Path

RAIZ = Path(__file__).resolve().parents[3]
sys.path.insert(0, str(RAIZ / "herramientas"))

import conciliar_finops_130 as cf  # noqa: E402

MODELO = RAIZ / "sdd" / "02-infraestructura" / "FINOPS_45_DIAS_MODELO_V3.json"
INVENTARIO = RAIZ / "infra" / "terraform" / "respaldo" / "inventario_backup.json"
CATALOGO = RAIZ / "infra" / "finops" / "precios_us-east-2.json"


def _modelo() -> dict:
    return cf.cargar_json(MODELO)


def _inv() -> dict:
    return cf.cargar_json(INVENTARIO)


def _cat() -> dict:
    return cf.cargar_json(CATALOGO)


def _json(datos: object) -> Path:
    manejador, ruta = tempfile.mkstemp(suffix=".json")
    with open(manejador, "w", encoding="utf-8") as fh:
        json.dump(datos, fh, ensure_ascii=False)
    return Path(ruta)


class SinDobleConteo(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.resultado = cf.conciliar(_modelo(), _inv(), _cat())
        cls.filas = {fila["sku"]: fila for fila in cls.resultado["filas"]}

    def test_incremento_sobre_el_modelo_es_cero(self) -> None:
        self.assertEqual(self.resultado["incremento_i130_45d_usd"], "0.0000")

    def test_todas_las_lineas_ya_estaban_presupuestadas(self) -> None:
        for fila in self.resultado["filas"]:
            self.assertTrue(fila["presupuestado_en_modelo"], fila["sku"])
            self.assertEqual(fila["incremento_sobre_modelo_usd"], "0.0000", fila["sku"])

    def test_las_lineas_si_tienen_importe_propio(self) -> None:
        # El importe existe, pero no se suma al total del modelo.
        total_lineas = sum(Decimal(f["importe_i130_3gb_usd"]) for f in self.resultado["filas"])
        self.assertGreater(total_lineas, Decimal("0.9"))
        self.assertEqual(self.resultado["incremento_i130_45d_usd"], "0.0000")

    def test_pitr_backup_y_state_mapeados(self) -> None:
        self.assertEqual(self.filas["DDB_PITR_GB_MES"]["concepto"], "DynamoDB PITR")
        self.assertEqual(self.filas["DDB_BACKUP_GB_MES"]["concepto"], "DynamoDB backup")
        self.assertEqual(self.filas["S3_STANDARD_GB_MES"]["concepto"], "S3 Terraform-state")

    def test_delta_2_vs_3_gb(self) -> None:
        self.assertEqual(self.resultado["delta_2_vs_3gb_45d_usd"], "-0.4779")
        self.assertEqual(self.filas["DDB_PITR_GB_MES"]["delta_2_vs_3_usd"], "-0.2959")
        self.assertEqual(self.filas["DDB_BACKUP_GB_MES"]["delta_2_vs_3_usd"], "-0.1479")

    def test_recurrente_y_eventual_separados(self) -> None:
        # Sin restore en el inventario, todo el importe de I-130 es recurrente.
        self.assertEqual(self.resultado["eventual_i130_45d_usd"], "0.0000")
        self.assertEqual(self.resultado["recurrente_i130_45d_usd"], "1.4336")

    def test_backup_opcional_se_marca_no_habilitado(self) -> None:
        self.assertEqual(self.filas["DDB_BACKUP_GB_MES"]["estado_gasto"], "OPCIONAL_NO_HABILITADO")

    def test_estados_de_evidencia(self) -> None:
        estados = self.resultado["estados_evidencia"]
        self.assertEqual(estados["verificado_en_cuenta"], "NOT_VERIFIED")
        self.assertEqual(estados["consumo_real"], "NOT_VERIFIED")
        self.assertEqual(estados["presupuestado"], "MODELO_H05_V3")

    def test_no_se_declara_sin_bloqueo_absoluto(self) -> None:
        self.assertEqual(self.resultado["decision"], "VIABILIDAD_LOCAL_SIN_EVIDENCIA_DE_CUENTA")
        self.assertIn("NOT_VERIFIED", self.resultado["advertencia"])

    def test_no_consulta_aws(self) -> None:
        self.assertFalse(self.resultado["aws_consultado"])


class Bloqueos(unittest.TestCase):
    def test_margen_negativo_bloquea(self) -> None:
        original = dict(cf.PRESUPUESTO_VIGENTE)
        try:
            cf.PRESUPUESTO_VIGENTE["creditos_declarados_usd"] = "100.0000"
            resultado = cf.conciliar(_modelo(), _inv(), _cat())
        finally:
            cf.PRESUPUESTO_VIGENTE.update(original)
        self.assertTrue(resultado["bloqueo_presupuestario_local"])
        self.assertEqual(resultado["decision"], "BLOCKED_FINOPS")

    def test_sku_sin_precio_es_error(self) -> None:
        catalogo = _cat()
        catalogo["items"]["DDB_PITR_GB_MES"]["precio_usd"] = None
        with self.assertRaises(cf.ErrorConciliacion):
            cf.conciliar(_modelo(), _inv(), catalogo)

    def test_modelo_inexistente_es_error(self) -> None:
        with self.assertRaises(cf.ErrorConciliacion):
            cf.cargar_json("no-existe.json")

    def test_cli_sale_cero_sin_bloqueo(self) -> None:
        self.assertEqual(cf.main([]), 0)

    def test_cli_sale_uno_con_bloqueo(self) -> None:
        original = dict(cf.PRESUPUESTO_VIGENTE)
        try:
            cf.PRESUPUESTO_VIGENTE["creditos_declarados_usd"] = "100.0000"
            codigo = cf.main([])
        finally:
            cf.PRESUPUESTO_VIGENTE.update(original)
        self.assertEqual(codigo, 1)


class PresupuestoVigente(unittest.TestCase):
    """La conciliacion usa el presupuesto ratificado de I-000, no la base historica."""

    @classmethod
    def setUpClass(cls) -> None:
        cls.resultado = cf.conciliar(_modelo(), _inv(), _cat())
        cls.gate = (RAIZ / "sdd" / "_control" / "H08" / "GATE_H08I_I000.md").read_text(encoding="utf-8")

    def test_fuente_del_presupuesto_es_el_gate_de_i000(self) -> None:
        self.assertIn("GATE_H08I_I000", self.resultado["modelo_historico_superado_por"])
        for cifra in ("102.7830", "113.0613", "120.0000"):
            self.assertIn(cifra, self.gate, cifra)

    def test_margen_teorico_calculado_desde_el_presupuesto_vigente(self) -> None:
        self.assertEqual(self.resultado["presupuesto_vigente_bruto_45d_usd"], "102.7830")
        self.assertEqual(self.resultado["presupuesto_vigente_con_margen_45d_usd"], "113.0613")
        self.assertEqual(self.resultado["margen_local_45d_usd"], "6.9387")

    def test_margen_no_es_dinero_disponible(self) -> None:
        self.assertFalse(self.resultado["margen_teorico_es_dinero_disponible"])
        self.assertIsNone(self.resultado["creditos_verificados_usd"])

    def test_incremento_sigue_siendo_cero(self) -> None:
        self.assertEqual(self.resultado["incremento_i130_45d_usd"], "0.0000")

    def test_decision_no_afirma_ausencia_de_bloqueo(self) -> None:
        self.assertEqual(self.resultado["decision"], "VIABILIDAD_LOCAL_SIN_EVIDENCIA_DE_CUENTA")
        self.assertNotIn("NO_BLOCKED", self.resultado["decision"])

if __name__ == "__main__":  # pragma: no cover
    unittest.main()
