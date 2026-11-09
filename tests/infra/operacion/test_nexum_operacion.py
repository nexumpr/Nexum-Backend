"""Pruebas adversas del operador I-120 (`herramientas/nexum_operacion.py`).

Cubren las tres invariantes de la spec que pueden fallar en silencio:

1. El core (2 EC2 Go, ALB, CloudFront, Redis, DynamoDB, state) no se toca.
2. Ningun recurso desconocido, sin etiqueta o fuera de allowlist se opera.
3. No existen capacidades de destroy de datos/estado ni llamadas a AWS.

Todo se ejecuta en local: los tests no consultan AWS ni ejecutan Terraform.
"""

from __future__ import annotations

import json
import sys
import tempfile
import unittest
from decimal import Decimal
from pathlib import Path

RAIZ = Path(__file__).resolve().parents[3]
sys.path.insert(0, str(RAIZ / "herramientas"))

import nexum_operacion as op  # noqa: E402

INVENTARIO = RAIZ / "infra" / "terraform" / "operacion" / "inventario.json"
CATALOGO = RAIZ / "infra" / "finops" / "precios_us-east-2.json"

CORE_ESPERADO = {
    "asg-backend",
    "alb-interno",
    "cloudfront-distribucion",
    "redis-oss",
    "dynamodb-core",
    "terraform-state-s3",
    "log-group-backend",
    "alarmas-cloudwatch",
    "interface-endpoint-logs",
    "red-vpc",
    "gateway-endpoints",
    "dynamodb-backup-programado",
    "secretsmanager-secretos",
}
AUX_ESPERADOS = {
    "interface-endpoint-secretsmanager",
    "interface-endpoint-ssm",
    "interface-endpoint-ssmmessages",
}


def _inventario() -> dict:
    return op.cargar_inventario(INVENTARIO)


def _catalogo() -> dict:
    return op.cargar_catalogo(CATALOGO)


def _allowlist() -> list[str]:
    return sorted(AUX_ESPERADOS)


def _copia(items: list[dict]) -> dict:
    datos = _inventario()
    datos["items"] = items
    return datos


def _item(nombre: str, **extra) -> dict:
    base = {
        "nombre": nombre,
        "componente": "red",
        "tipo": "aws_vpc_endpoint",
        "criticidad": "auxiliary",
        "etiqueta_criticidad": "auxiliary",
        "estado_recurso": "DECLARADO_NO_CREADO",
        "suspendible": True,
        "motivo": "fixture de prueba",
        "partidas": [],
        "dependencias": [],
    }
    base.update(extra)
    return base


def _escritura(datos: dict) -> Path:
    manejador, ruta = tempfile.mkstemp(suffix=".json")
    with open(manejador, "w", encoding="utf-8") as fh:
        json.dump(datos, fh, ensure_ascii=False)
    return Path(ruta)


class ClasificacionCoreYAuxiliares(unittest.TestCase):
    def test_core_declarado_cubre_lo_intocable(self) -> None:
        clasif = op.clasificar(_inventario(), _allowlist())
        self.assertEqual(set(clasif["core"]), CORE_ESPERADO)
        self.assertEqual(set(clasif["suspendibles"]), AUX_ESPERADOS)
        self.assertEqual(clasif["denegados"], {})

    def test_auxiliar_fuera_de_allowlist_se_deniega(self) -> None:
        clasif = op.clasificar(_inventario(), ["interface-endpoint-ssm"])
        self.assertNotIn("interface-endpoint-ssm", clasif["denegados"])
        self.assertIn("auxiliar fuera de la allowlist revisada", clasif["denegados"].values())
        self.assertEqual(len(clasif["suspendibles"]), 1)

    def test_auxiliar_no_suspendible_se_deniega(self) -> None:
        items = [_item("aux-raro", suspendible=False)]
        clasif = op.clasificar(_copia(items), ["aux-raro"])
        self.assertEqual(clasif["suspendibles"], [])
        self.assertIn("aux-raro", clasif["denegados"])

    def test_core_suspendible_se_deniega(self) -> None:
        items = [_item("core-suspendible", criticidad="core", suspendible=True)]
        clasif = op.clasificar(_copia(items), [])
        self.assertIn("core-suspendible", clasif["denegados"])
        self.assertEqual(clasif["core"], [])

    def test_etiqueta_incoherente_se_deniega(self) -> None:
        items = [_item("sin-etiqueta", etiqueta_criticidad="unknown")]
        clasif = op.clasificar(_copia(items), [])
        self.assertIn("sin-etiqueta", clasif["denegados"])

    def test_criticidad_invalida_invalida_inventario(self) -> None:
        items = [_item("raro", criticidad="produccion")]
        with self.assertRaises(op.ErrorInventario):
            op.validar_inventario(_copia(items))

    def test_inventario_vacio_es_error(self) -> None:
        ruta = _escritura({"items": []})
        with self.assertRaises(op.ErrorInventario):
            op.cargar_inventario(ruta)

    def test_nombre_duplicado_es_error(self) -> None:
        items = [_item("dup"), _item("dup")]
        with self.assertRaises(op.ErrorInventario):
            op.validar_inventario(_copia(items))

    def test_campo_obligatorio_faltante_es_error(self) -> None:
        item = _item("incompleto")
        del item["criticidad"]
        with self.assertRaises(op.ErrorInventario):
            op.validar_inventario(_copia([item]))

    def test_base_de_partida_invalida_es_error(self) -> None:
        item = _item("partida-rara", partidas=[{"sku": "ALB_HORA", "cantidad": "1", "base": "semana"}])
        with self.assertRaises(op.ErrorInventario):
            op.validar_inventario(_copia([item]))


class CostoExacto(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.inv = _inventario()
        cls.cat = _catalogo()
        cls.res = op.residual(cls.inv, cls.cat)

    def test_residual_no_es_cero_ni_se_declara_gratis(self) -> None:
        self.assertGreater(Decimal(self.res["total_hora_usd"]), Decimal("0"))
        self.assertGreater(Decimal(self.res["residente_45d_usd"]), Decimal("90"))
        self.assertIn("ni se declaran cero", self.res["advertencia"])

    def test_residual_residente_mas_presupuestado(self) -> None:
        residente = Decimal(self.res["residente_hora_usd"])
        presupuestado = Decimal(self.res["presupuestado_hora_usd"])
        self.assertEqual(residente + presupuestado, Decimal(self.res["total_hora_usd"]))

    def test_residual_45d_no_supera_el_presupuesto_i000(self) -> None:
        # I-000 fija 102.7830 USD de bruto verificado para 45 dias; el residual
        # fijo debe quedar por debajo y no puede sustituir ese total.
        self.assertLessEqual(Decimal(self.res["total_45d_usd"]), Decimal("102.7830"))

    def test_secreto_bloqueado_no_se_cuenta(self) -> None:
        no_computable = self.res["no_computable"]
        self.assertEqual(list(no_computable), ["secretsmanager-secretos"])
        self.assertEqual(no_computable["secretsmanager-secretos"], "BLOQUEADO" + "_RFC004")
        self.assertNotIn("secretsmanager-secretos", self.res["por_recurso_hora_usd"])

    def test_partidas_variables_se_listan_y_no_se_suman(self) -> None:
        self.assertIn("cloudfront-distribucion", self.res["componentes_variables"])
        self.assertIn("dynamodb-core", self.res["componentes_variables"])
        self.assertEqual(self.res["por_recurso_hora_usd"]["cloudfront-distribucion"], "0.0000")

    def test_mes_de_facturacion_de_730_horas(self) -> None:
        self.assertEqual(self.res["horas_mes"], "730")
        self.assertEqual(op.HORAS_MES, Decimal("730"))

    def test_calculo_devuelve_decimal_sin_float(self) -> None:
        item = {"nombre": "x", "partidas": [{"sku": "ALB_HORA", "cantidad": "1", "base": "hora"}]}
        self.assertIsInstance(op.costo_horario_item(item, self.cat), Decimal)

    def test_sku_desconocido_no_se_infiere(self) -> None:
        item = {"nombre": "x", "partidas": [{"sku": "SKU_INVENTADO", "cantidad": "1", "base": "hora"}]}
        with self.assertRaises(op.ErrorInventario):
            op.costo_horario_item(item, self.cat)

    def test_sku_sin_precio_verificado_no_se_infiere(self) -> None:
        item = {
            "nombre": "x",
            "partidas": [{"sku": "EC2_T4G_MICRO_HORA_VERIFICADO", "cantidad": "1", "base": "hora"}],
        }
        with self.assertRaises(op.ErrorInventario):
            op.costo_horario_item(item, self.cat)

    def test_ahorro_de_auxiliares_coincide_con_el_catalogo(self) -> None:
        ahorro = op.ahorro_auxiliares(self.inv, self.cat, sorted(AUX_ESPERADOS))
        # 3 servicios x 2 AZ x 0.01 USD/h = 0.0600 USD/h
        self.assertEqual(ahorro["total_hora_usd"], "0.0600")
        self.assertEqual(ahorro["total_45d_usd"], "64.8000")


class AprobacionesYDenegaciones(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.inv = _inventario()
        cls.cat = _catalogo()

    def test_encender_sin_aprobacion_se_deniega(self) -> None:
        with self.assertRaises(op.ErrorUso):
            op.plan_encender(self.inv, self.cat, _allowlist(), None, None, "h08i", "op", "runbook", None)

    def test_aprobacion_de_otro_entorno_se_deniega(self) -> None:
        with self.assertRaises(op.ErrorUso):
            op.plan_encender(
                self.inv, self.cat, _allowlist(), None,
                "APRUEBO_ENCENDER_PROD", "h08i", "op", "runbook", None,
            )

    def test_aprobacion_es_case_sensitive(self) -> None:
        with self.assertRaises(op.ErrorUso):
            op.plan_suspender_aux(
                self.inv, self.cat, _allowlist(), "apruebo_suspender_aux_h08i", "h08i",
                None, "op", "runbook", None,
            )

    def test_suspender_aux_requiere_aprobacion(self) -> None:
        with self.assertRaises(op.ErrorUso):
            op.plan_suspender_aux(self.inv, self.cat, _allowlist(), None, "h08i", None, "op", "r", None)

    def test_no_se_puede_suspender_core(self) -> None:
        with self.assertRaises(op.ErrorUso) as ctx:
            op.plan_suspender_aux(
                self.inv, self.cat, _allowlist(), "APRUEBO_SUSPENDER_AUX_H08I", "h08i",
                ["redis-oss"], "op", "runbook", None,
            )
        self.assertIn("no autorizados", str(ctx.exception))

    def test_no_se_puede_suspender_recurso_inexistente(self) -> None:
        with self.assertRaises(op.ErrorUso):
            op.plan_suspender_aux(
                self.inv, self.cat, _allowlist(), "APRUEBO_SUSPENDER_AUX_H08I", "h08i",
                ["recurso-que-no-existe"], "op", "runbook", None,
            )

    def test_core_suspendido_observado_es_incidente(self) -> None:
        observado = {"recursos_suspendidos": ["dynamodb-core"]}
        with self.assertRaises(op.ErrorUso) as ctx:
            op.plan_suspender_aux(
                self.inv, self.cat, _allowlist(), "APRUEBO_SUSPENDER_AUX_H08I", "h08i",
                None, "op", "runbook", observado,
            )
        self.assertIn("incidente", str(ctx.exception))

    def test_destruir_aux_solo_alcanza_allowlist(self) -> None:
        with self.assertRaises(op.ErrorUso):
            op.plan_destruir_aux(
                self.inv, self.cat, _allowlist(), "APRUEBO_DESTRUIR_AUX_H08I", "h08i",
                ["alb-interno"], "op", "runbook",
            )

    def test_comandos_de_destroy_prohibidos(self) -> None:
        for comando in ("destruir-datos", "destruir-estado", "destruir-todo", "destroy-all"):
            with self.subTest(comando=comando):
                with self.assertRaises(op.ErrorUso):
                    op.ejecutar([comando])

    def test_mensaje_especifico_por_tipo_de_destroy(self) -> None:
        with self.assertRaises(op.ErrorUso) as ctx:
            op.ejecutar(["destruir-datos"])
        self.assertIn("DynamoDB nunca se destruye", str(ctx.exception))
        with self.assertRaises(op.ErrorUso) as ctx:
            op.ejecutar(["destruir-estado"])
        self.assertIn("prevent_destroy", str(ctx.exception))

    def test_comando_desconocido_se_rechaza(self) -> None:
        with self.assertRaises(op.ErrorUso):
            op.ejecutar(["apagar-core"])


class MaquinaDeEstados(unittest.TestCase):
    def test_transiciones_validas(self) -> None:
        self.assertTrue(op.transicion("OFF", "STARTING", "op", "runbook")["permitida"])
        self.assertTrue(op.transicion("STARTING", "ON_2_HEALTHY", "op", "runbook")["permitida"])
        self.assertTrue(op.transicion("ON_2_HEALTHY", "DRAINING", "op", "runbook")["permitida"])

    def test_off_no_salta_a_operativo(self) -> None:
        with self.assertRaises(op.ErrorUso):
            op.transicion("OFF", "ON_2_HEALTHY", "op", "runbook")

    def test_transicion_exige_actor_y_runbook(self) -> None:
        with self.assertRaises(op.ErrorUso):
            op.transicion("OFF", "STARTING", None, "runbook")
        with self.assertRaises(op.ErrorUso):
            op.transicion("OFF", "STARTING", "op", None)

    def test_estado_igual_es_idempotente(self) -> None:
        resultado = op.transicion("ON_2_HEALTHY", "ON_2_HEALTHY", None, None)
        self.assertTrue(resultado["idempotente"])

    def test_encender_sin_dos_nodos_no_publica(self) -> None:
        codigo, resultado = op.ejecutar(
            [
                "encender",
                "--inventario", str(INVENTARIO),
                "--catalogo", str(CATALOGO),
                "--aprobacion", "APRUEBO_ENCENDER_H08I",
                "--actor", "operador",
                "--runbook", "RUNBOOK_START_STOP.md",
                "--observado", str(_escritura({"estado": "OFF", "targets_sanos": 1})),
                "--ahora", "2026-11-09T00:00:00+00:00",
            ]
        )
        self.assertEqual(codigo, 0)
        self.assertEqual(resultado["transicion"]["hacia"], "STARTING")
        self.assertFalse(resultado["dos_nodos"]["publicable"])

    def test_encender_con_dos_nodos_alcanza_on_2_healthy(self) -> None:
        _, resultado = op.ejecutar(
            [
                "encender",
                "--inventario", str(INVENTARIO),
                "--catalogo", str(CATALOGO),
                "--aprobacion", "APRUEBO_ENCENDER_H08I",
                "--actor", "operador",
                "--runbook", "RUNBOOK_START_STOP.md",
                "--observado", str(_escritura({"estado": "STARTING", "targets_sanos": 2})),
                "--ahora", "2026-11-09T00:00:00+00:00",
            ]
        )
        self.assertEqual(resultado["transicion"]["hacia"], "ON_2_HEALTHY")
        self.assertTrue(resultado["dos_nodos"]["publicable"])


class CreditosYUmbrales(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.umbrales = _inventario()["umbrales_credito_usd"]

    def test_sin_evidencia_de_creditos_se_bloquea(self) -> None:
        evaluacion = op.evaluar_creditos(None, self.umbrales)
        self.assertEqual(evaluacion["estado"], "BLOQUEADO")
        self.assertIn("AWS_NOT_RUN", evaluacion["motivo"])

    def test_umbrales_de_la_spec(self) -> None:
        self.assertEqual(op.evaluar_creditos("10", self.umbrales)["estado"], "INCIDENTE_CRITICO")
        self.assertEqual(op.evaluar_creditos("30", self.umbrales)["estado"], "BLOQUEADO")
        self.assertEqual(op.evaluar_creditos("40", self.umbrales)["estado"], "AVISO")
        self.assertEqual(op.evaluar_creditos("50", self.umbrales)["estado"], "OK")

    def test_reanudar_con_saldo_bajo_bloquea(self) -> None:
        codigo, resultado = op.ejecutar(
            [
                "reanudar-aux",
                "--inventario", str(INVENTARIO),
                "--catalogo", str(CATALOGO),
                "--aprobacion", "APRUEBO_REANUDAR_AUX_H08I",
                "--creditos", "30",
            ]
        )
        self.assertEqual(codigo, 1)
        self.assertEqual(resultado["estado"], "BLOQUEADO")

    def test_umbrales_no_son_hard_cap(self) -> None:
        evaluacion = op.evaluar_creditos("50", self.umbrales)
        self.assertIn("no autorizan plan Paid", evaluacion["advertencia"])


class ReconciliacionYRollback(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.inv = _inventario()
        cls.cat = _catalogo()

    def test_sin_observado_no_se_declara_convergencia(self) -> None:
        resultado = op.plan_reconciliar(self.inv, self.cat, _allowlist(), None)
        self.assertEqual(resultado["estado"], "BLOQUEADO")

    def test_core_ausente_es_drift(self) -> None:
        resultado = op.plan_reconciliar(
            self.inv, self.cat, _allowlist(), {"recursos_presentes": ["asg-backend"]}
        )
        self.assertEqual(resultado["estado"], "DRIFT")
        self.assertIn("dynamodb-core", resultado["core_ausente"])

    def test_convergente_sin_drift(self) -> None:
        clasif = op.clasificar(self.inv, _allowlist())
        presentes = sorted(set(clasif["core"]) | set(clasif["suspendibles"]))
        resultado = op.plan_reconciliar(
            self.inv, self.cat, _allowlist(),
            {"recursos_presentes": presentes, "targets_sanos": 2},
        )
        self.assertEqual(resultado["estado"], "CONVERGENTE")
        self.assertEqual(resultado["acciones_sugeridas"], [])

    def test_rollback_sin_snapshot_se_deniega(self) -> None:
        with self.assertRaises(op.ErrorUso):
            op.plan_rollback(self.inv, self.cat, _allowlist(), None, "op", "runbook")

    def test_rollback_nunca_destruye_datos(self) -> None:
        codigo, resultado = op.ejecutar(
            [
                "rollback",
                "--inventario", str(INVENTARIO),
                "--catalogo", str(CATALOGO),
                "--snapshot", str(_escritura({"estado": "ON_2_HEALTHY"})),
                "--actor", "operador",
                "--runbook", "RUNBOOK_START_STOP.md",
            ]
        )
        self.assertEqual(codigo, 0)
        self.assertFalse(resultado["datos_destruidos"])
        self.assertFalse(resultado["estado_destruido"])

    def test_snapshot_con_destroy_datos_se_rechaza(self) -> None:
        ruta = _escritura({"estado": "ON_2_HEALTHY", "destroy_datos": True})
        with self.assertRaises(op.ErrorUso):
            op.plan_rollback(self.inv, self.cat, _allowlist(), op.cargar_json(ruta), "op", "r")

    def test_snapshot_con_recursos_no_autorizados_se_rechaza(self) -> None:
        snapshot = {"recursos_deseados": ["recurso-inventado"]}
        with self.assertRaises(op.ErrorUso):
            op.plan_rollback(self.inv, self.cat, _allowlist(), snapshot, "op", "r")


class SinAccesoAAWS(unittest.TestCase):
    def test_el_operador_no_importa_aws_ni_subprocess(self) -> None:
        fuente = (RAIZ / "herramientas" / "nexum_operacion.py").read_text(encoding="utf-8")
        for prohibido in (
            "import boto3",
            "import botocore",
            "from botocore",
            "import subprocess",
            "from subprocess",
            "os.system",
            "subprocess.run",
        ):
            with self.subTest(prohibido=prohibido):
                self.assertNotIn(prohibido, fuente)

    def test_todo_resultado_declara_no_ejecucion(self) -> None:
        for comando in (
            ["estado"],
            ["reconciliar"],
            ["suspender-aux", "--aprobacion", "APRUEBO_SUSPENDER_AUX_H08I"],
        ):
            with self.subTest(comando=comando[0]):
                _, resultado = op.ejecutar(
                    comando + ["--inventario", str(INVENTARIO), "--catalogo", str(CATALOGO)]
                )
                self.assertFalse(resultado["aws_consultado"])
                self.assertFalse(resultado["terraform_ejecutado"])


class CliYArchivos(unittest.TestCase):
    def test_estado_sale_cero_y_escribe_plan(self) -> None:
        destino = Path(tempfile.mkdtemp()) / "plan.json"
        codigo, _ = op.ejecutar(
            [
                "estado",
                "--inventario", str(INVENTARIO),
                "--catalogo", str(CATALOGO),
                "--salida", str(destino),
            ]
        )
        self.assertEqual(codigo, 0)
        contenido = json.loads(destino.read_text(encoding="utf-8"))
        self.assertEqual(contenido["comando"], "estado")
        self.assertEqual(contenido["estado_runtime"], "DESCONOCIDO")

    def test_estado_lee_observado_sin_inventar(self) -> None:
        ruta = _escritura({"estado": "ON_2_HEALTHY", "targets_sanos": 2})
        _, resultado = op.ejecutar(
            [
                "estado",
                "--inventario", str(INVENTARIO),
                "--catalogo", str(CATALOGO),
                "--observado", str(ruta),
            ]
        )
        self.assertEqual(resultado["estado_runtime"], "ON_2_HEALTHY")
        self.assertTrue(resultado["dos_nodos"]["publicable"])

    def test_main_devuelve_2_en_prohibido(self) -> None:
        self.assertEqual(op.main(["destroy-all"]), 2)

    def test_main_devuelve_1_en_bloqueado(self) -> None:
        self.assertEqual(
            op.main(
                [
                    "reconciliar",
                    "--inventario", str(INVENTARIO),
                    "--catalogo", str(CATALOGO),
                ]
            ),
            1,
        )


if __name__ == "__main__":  # pragma: no cover
    unittest.main()
