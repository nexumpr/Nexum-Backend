"""Alineación FinOps de I-110 con las líneas CloudWatch de I-000."""

from __future__ import annotations

import re
import unittest
from pathlib import Path

RAIZ = Path(__file__).resolve().parents[3]
MODULO = RAIZ / "infra" / "terraform" / "modules" / "observabilidad"
MATRIZ = RAIZ / "infra" / "finops" / "MATRIZ_COSTOS_SERVICIOS_I000.md"


def _leer(ruta: Path) -> str:
    return ruta.read_text(encoding="utf-8")


def _recursos(texto: str) -> list[tuple[str, str, str]]:
    """Devuelve (tipo, nombre, bloque) por cada `resource` de nivel superior."""

    marcas = list(re.finditer(r'^resource "([^"]+)" "([^"]+)" \{', texto, re.M))
    bloques: list[tuple[str, str, str]] = []
    for indice, marca in enumerate(marcas):
        fin = marcas[indice + 1].start() if indice + 1 < len(marcas) else len(texto)
        bloques.append((marca.group(1), marca.group(2), texto[marca.start() : fin]))
    return bloques


class AlineacionI110(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.main = _leer(MODULO / "main.tf")
        cls.vars = _leer(MODULO / "variables.tf")
        cls.finops = _leer(MATRIZ)

    def test_presupuesto_i000_cubre_logs(self) -> None:
        self.assertIn("CloudWatch ingesta | 3 | GB", self.finops)
        self.assertIn("CloudWatch retención | 4.5 | GB-mes", self.finops)
        self.assertIn("CloudWatch alarmas | 3 | alarma-mes", self.finops)

    def test_metric_filters_dentro_de_metricas_presupuestadas(self) -> None:
        # La línea presupuestada de I-000 cubre los filtros base (siempre
        # creados). Los 8 filtros FCM son opt-in (`count = 0` por defecto) y no
        # consumen esa línea hasta que un Gate los active; se comprueba que
        # ninguno se cree sin la variable.
        filtros = [
            (nombre, bloque)
            for tipo, nombre, bloque in _recursos(self.main)
            if tipo == "aws_cloudwatch_log_metric_filter"
        ]
        base = [nombre for nombre, bloque in filtros if not re.search(r"^\s*count\s*=", bloque, re.M)]
        fcm = [nombre for nombre, bloque in filtros if "var.habilitar_metricas_fcm" in bloque]
        self.assertEqual(len(base), 4, base)
        self.assertEqual(len(fcm), 8, fcm)
        self.assertLessEqual(len(base), 4.5)
        for nombre, bloque in filtros:
            if nombre in fcm:
                self.assertRegex(bloque, r"count\s*=\s*var\.habilitar_metricas_fcm", nombre)

    def test_alarmas_incrementales_documentadas(self) -> None:
        alarmas = [
            (nombre, bloque)
            for tipo, nombre, bloque in _recursos(self.main)
            if tipo == "aws_cloudwatch_metric_alarm"
        ]
        base = [nombre for nombre, bloque in alarmas if not re.search(r"^\s*count\s*=", bloque, re.M)]
        condicionales = [nombre for nombre, bloque in alarmas if "var.redis_replication_group_id" in bloque or "var.dynamodb_table_name" in bloque]
        fcm = [nombre for nombre, bloque in alarmas if "var.habilitar_alarmas_fcm" in bloque]
        self.assertEqual(len(base), 6, base)
        self.assertEqual(len(condicionales), 2, condicionales)
        self.assertEqual(len(fcm), 6, fcm)
        # I-000 presupuesta 3 alarmas (0.3000 USD). Se declaran 6 base y hasta
        # 2 condicionales de Redis/DynamoDB (+0.2000) y 6 FCM opt-in (+0.6000):
        # incremento maximo 0.5000 USD sobre el presupuesto de 3 alarmas.
        incremental = 0.20 + 0.60 - 0.30
        self.assertAlmostEqual(incremental, 0.50, places=4)
        # Margen vigente de I-000 (6.9387 USD) cubre el incremento; no se asume medido.
        self.assertGreater(6.9387, incremental)
        self.assertIn("límite de gasto**", self.main + self.vars + _leer(RAIZ / "sdd" / "_control" / "H08" / "MATRIZ_METRICAS_ALERTAS.md"))

    def test_coste_fcm_declarado_coincide_con_los_recursos(self) -> None:
        # El output declara el coste mensual en USD de los filtros/alarmas FCM.
        # Se recalcula desde el número real de recursos para que una cifra no
        # quede desfasada cuando se añada o retire un filtro/alarma.
        salida = _leer(MODULO / "outputs.tf")
        filtros = [b for t, _, b in _recursos(self.main) if t == "aws_cloudwatch_log_metric_filter" and "var.habilitar_metricas_fcm" in b]
        alarmas = [b for t, _, b in _recursos(self.main) if t == "aws_cloudwatch_metric_alarm" and "var.habilitar_alarmas_fcm" in b]
        esperado = f"{len(filtros) * 0.30 + len(alarmas) * 0.10:.2f}"
        self.assertIn(f"{len(filtros)} filtros y {len(alarmas)} alarmas", salida)
        self.assertIn(f"{esperado} si se habilitan", salida)
        self.assertIn("DOC_ONLY", salida)

    def test_retencion_y_volumen(self) -> None:
        bloque = self.vars.split('variable "retencion_dias"')[1].split("}")[0]
        self.assertIn("default     = 7", bloque)
        self.assertIn("var.retencion_dias <= 30", self.main)

    def test_sin_costes_no_presupuestados(self) -> None:
        texto = self.main.lower()
        for prohibido in ("aws_xray", "access_logs", "kms_key", "aws_sns_topic"):
            self.assertNotIn(prohibido, texto, prohibido)
        self.assertIn("NOT_CONFIGURED_BLOCKED_RFC007_SNS_KMS", _leer(MODULO / "outputs.tf"))

    def test_aws_no_aplicado(self) -> None:
        self.assertIn('"NOT_RUN_AWS"', _leer(MODULO / "outputs.tf"))


if __name__ == "__main__":
    unittest.main()
