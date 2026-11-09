"""Contrato Terraform de I-110: logs, métricas, alarmas, presupuesto y dashboard."""

from __future__ import annotations

import re
import unittest
from pathlib import Path

from test_balanceo_contrato import _leer, recursos

RAIZ = Path(__file__).resolve().parents[3]
MODULO = RAIZ / "infra" / "terraform" / "modules" / "observabilidad"
ENV = RAIZ / "infra" / "terraform" / "envs" / "h08i"


class LogsYRetencion(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.main = _leer(MODULO / "main.tf")
        cls.vars = _leer(MODULO / "variables.tf")
        cls.r = recursos(cls.main)

    def test_grupo_de_logs_con_retencion_finita(self) -> None:
        self.assertIn(("aws_cloudwatch_log_group", "backend"), self.r)
        self.assertIn("retention_in_days = var.retencion_dias", self.main)
        self.assertIn('default     = 7', self.vars.split('variable "retencion_dias"')[1].split("}")[0])

    def test_retencion_validada_y_acotada(self) -> None:
        bloque = self.vars.split('variable "retencion_dias"')[1]
        self.assertIn("contains(", bloque)
        self.assertIn("3653", bloque)
        self.assertIn("var.retencion_dias <= 30", self.main)
        self.assertIn("no debe superar 30 d", self.main)

    def test_cuatro_metric_filters_canonicos(self) -> None:
        filtros = [n for t, n in self.r if t == "aws_cloudwatch_log_metric_filter"]
        self.assertEqual(sorted(filtros), ["auth_401", "errores_5xx", "rate_limit_429", "readiness_no_200"])
        for patron in ('{ $.status >= 500 }', '{ $.status = 401 }', '{ $.status = 429 }',
                       '{ $.route = \\"/api/v1/health/ready\\" && $.status != 200 }'):
            self.assertIn(patron, self.main, patron)


class AlarmasYPresupuesto(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.main = _leer(MODULO / "main.tf")
        cls.vars = _leer(MODULO / "variables.tf")
        cls.r = recursos(cls.main)

    def test_cinco_alarmas_con_umbral_y_runbook(self) -> None:
        alarmas = {n for t, n in self.r if t == "aws_cloudwatch_metric_alarm"}
        self.assertEqual(alarmas, {"targets_sanos", "errores_5xx", "auth_401", "readiness_no_200",
                                   "rate_limit_429", "redis_conexiones", "dynamodb_throttles", "logs_sin_eventos"})
        self.assertRegex(self.main, r'metric_name\s*=\s*"HealthyHostCount"')
        self.assertRegex(self.main, r"threshold\s*=\s*2")
        self.assertIn('comparison_operator = "LessThanThreshold"', self.main)
        self.assertIn("treat_missing_data  = \"breaching\"", self.main)
        for runbook in ("#A1", "#A2", "#A3", "#A4", "#A5", "#A6", "#A7", "#A8"):
            self.assertIn(f"MATRIZ_METRICAS_ALERTAS.md{runbook}", self.main)

    def test_notificacion_bloqueada_por_coste_cmk(self) -> None:
        self.assertNotIn(("aws_sns_topic", "alertas"), self.r)
        self.assertNotIn("alarm_actions", self.main)
        self.assertIn('"NOT_CONFIGURED_BLOCKED_RFC007_SNS_KMS"', _leer(MODULO / "outputs.tf"))
        self.assertIn("RFC007", _leer(MODULO / "outputs.tf"))

    def test_presupuesto_declarativo_sin_notificacion_sns(self) -> None:
        self.assertIn(("aws_budgets_budget", "mensual"), self.r)
        self.assertRegex(self.main, r'limit_amount\s*=\s*tostring\(var\.presupuesto_usd\)')
        self.assertNotIn("notification {", self.main)
        self.assertIn("límite de gasto", _leer(MODULO / "outputs.tf") + _leer(RAIZ / "sdd" / "_control" / "H08" / "MATRIZ_METRICAS_ALERTAS.md"))

    def test_sin_recursos_de_cost_extra(self) -> None:
        tipos = {t for t, _ in self.r}
        self.assertNotIn("aws_xray_sampling_rule", tipos)
        self.assertNotIn("aws_s3_bucket", tipos)
        texto = self.main.lower()
        self.assertNotIn("aws_dynamodb_table", texto)
        self.assertNotIn("access_logs", texto)
        self.assertNotIn("0.0.0.0/0", texto)


class DashboardYCableado(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.main = _leer(MODULO / "main.tf")
        cls.env = _leer(ENV / "main.tf")
        cls.outs = _leer(MODULO / "outputs.tf")

    def test_dashboard_condicional_sin_pii(self) -> None:
        self.assertIn(("aws_cloudwatch_dashboard", "observabilidad"), [(t, n) for t, n in recursos(self.main)])
        self.assertIn("count          = var.habilitar_dashboard ? 1 : 0", self.main)
        self.assertIn("fields @timestamp, severity, route, status, latencyMs, traceId", self.main)
        self.assertNotIn("@nexum.com", self.main)
        self.assertNotIn("authorization", self.main.lower())

    def test_cableado_en_entorno(self) -> None:
        self.assertIn('module "observabilidad"', self.env)
        self.assertIn('source = "../../modules/observabilidad"', self.env)
        self.assertIn("alb_arn          = module.balanceo.alb_arn", self.env)
        self.assertIn("target_group_arn = module.balanceo.target_group_arn", self.env)
        self.assertIn('output "observabilidad"', self.env)

    def test_estado_no_aplicado(self) -> None:
        self.assertIn('"NOT_RUN_AWS"', self.outs)
        self.assertIn("NOT_RUN_AWS", self.env + self.outs)


if __name__ == "__main__":
    unittest.main()
