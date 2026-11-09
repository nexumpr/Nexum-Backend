"""Pruebas adversas de operacion, incidentes y drills de I-140."""

from __future__ import annotations

import copy
import json
import sys
import tempfile
import unittest
from pathlib import Path

RAIZ = Path(__file__).resolve().parents[3]
sys.path.insert(0, str(RAIZ / "herramientas"))

import verificar_operacion_i140 as op  # noqa: E402

FIXTURES = RAIZ / "tests" / "fixtures" / "operacion"
RUNBOOK = RAIZ / "sdd" / "_control" / "H08" / "RUNBOOK_OPERATIVO.md"


def _fixture(nombre: str) -> dict:
    return copy.deepcopy(json.loads((FIXTURES / nombre).read_text(encoding="utf-8")))


def _json(datos: object) -> Path:
    manejador, ruta = tempfile.mkstemp(suffix=".json")
    with open(manejador, "w", encoding="utf-8") as fh:
        json.dump(datos, fh, ensure_ascii=False)
    return Path(ruta)


class Runbooks(unittest.TestCase):
    def test_runbook_completo(self) -> None:
        resultado = op.validar_runbooks(RUNBOOK.read_text(encoding="utf-8"))
        self.assertEqual(resultado["estado"], "RUNBOOKS_COMPLETOS")
        self.assertEqual(resultado["runbooks_presentes"], len(op.RUNBOOKS_REQUERIDOS))
        self.assertTrue(resultado["todos_documentados_sin_ejecutar"])

    def test_diez_procedimientos_exigidos(self) -> None:
        self.assertEqual(len(op.RUNBOOKS_REQUERIDOS), 10)
        for nombre in ("deployment", "health-fail", "dynamo-throttle", "redis-unavailable",
                       "certificados", "rotacion-secretos", "auth-inesperado", "presupuesto",
                       "incidente", "eliminacion-iac"):
            self.assertIn(nombre, op.RUNBOOKS_REQUERIDOS)

    def test_runbook_faltante_se_detecta(self) -> None:
        texto = RUNBOOK.read_text(encoding="utf-8").replace("## redis-unavailable", "## otro")
        resultado = op.validar_runbooks(texto)
        self.assertEqual(resultado["estado"], "RUNBOOKS_INCOMPLETOS")
        self.assertIn("redis-unavailable", resultado["faltantes"])

    def test_campo_obligatorio_faltante(self) -> None:
        texto = RUNBOOK.read_text(encoding="utf-8").replace("- **rollback**", "- (rollback)", 1)
        resultado = op.validar_runbooks(texto)
        self.assertEqual(resultado["estado"], "RUNBOOKS_INCOMPLETOS")
        self.assertIn("rollback", resultado["faltantes"]["deployment"])

    def test_runbook_escrito_no_acredita_ejecucion(self) -> None:
        resultado = op.validar_runbooks(RUNBOOK.read_text(encoding="utf-8"))
        self.assertEqual(resultado["ejecucion_aws"], "NOT_RUN")
        self.assertIn("no acredita", resultado["advertencia"])

    def test_runbook_inexistente_es_error(self) -> None:
        with self.assertRaises(op.ErrorUso):
            op.validar_runbooks(ruta=Path(tempfile.mkdtemp()) / "no-existe.md")

    def test_ruta_alternativa_no_cae_al_runbook_por_defecto(self) -> None:
        with self.assertRaises(op.ErrorUso):
            op.ejecutar(["runbooks", "--runbook", str(Path(tempfile.mkdtemp()) / "otro.md")])


class Prioridades(unittest.TestCase):
    def test_fixture_coherente(self) -> None:
        resultado = op.validar_prioridades(_fixture("prioridades.json"))
        self.assertEqual(resultado["estado"], "PRIORIDADES_COHERENTES")
        self.assertEqual(resultado["ratificacion"], "PENDIENTE_DE_RATIFICACION_HUMANA")

    def test_falta_un_nivel(self) -> None:
        datos = _fixture("prioridades.json")
        del datos["prioridades"]["S2"]
        self.assertEqual(op.validar_prioridades(datos)["estado"], "PRIORIDADES_INCOMPLETAS")

    def test_tiempos_no_crecientes(self) -> None:
        datos = _fixture("prioridades.json")
        datos["prioridades"]["S1"]["tiempo_notificacion_min"] = 120
        resultado = op.validar_prioridades(datos)
        self.assertEqual(resultado["estado"], "PRIORIDADES_INCOHERENTES")
        self.assertTrue(any("crecientes" in p for p in resultado["problemas"]))

    def test_tiempo_no_entero(self) -> None:
        datos = _fixture("prioridades.json")
        datos["prioridades"]["S2"]["tiempo_notificacion_min"] = "60"
        self.assertIn("S2: tiempo_notificacion_min invalido", op.validar_prioridades(datos)["problemas"])

    def test_campo_obligatorio_ausente(self) -> None:
        datos = _fixture("prioridades.json")
        del datos["prioridades"]["S3"]["rollback"]
        self.assertTrue(any("S3: falta rollback" in p for p in op.validar_prioridades(datos)["problemas"]))

    def test_sin_datos_es_error(self) -> None:
        with self.assertRaises(op.ErrorUso):
            op.validar_prioridades(None)


class Drills(unittest.TestCase):
    def test_plan_valido_y_no_ejecutado(self) -> None:
        resultado = op.validar_drills(_fixture("drills.json"))
        self.assertEqual(resultado["estado"], "PLAN_VALIDO")
        self.assertEqual(resultado["ejecucion_aws"], "NOT_RUN")
        self.assertEqual(resultado["requeridos"], 5)

    def test_drill_faltante(self) -> None:
        datos = _fixture("drills.json")
        datos["drills"] = [d for d in datos["drills"] if d["id"] != "rotacion-jwt"]
        resultado = op.validar_drills(datos)
        self.assertEqual(resultado["estado"], "PLAN_RECHAZADO")
        self.assertIn("rotacion-jwt", resultado["faltantes"])

    def test_estado_ejecutado_aws_se_rechaza(self) -> None:
        datos = _fixture("drills.json")
        datos["drills"][0]["estado"] = "EJECUTADO_AWS"
        resultado = op.validar_drills(datos)
        self.assertEqual(resultado["estado"], "PLAN_RECHAZADO")
        self.assertTrue(any("EJECUTADO_AWS" in p for p in resultado["problemas"]))

    def test_estado_desconocido(self) -> None:
        datos = _fixture("drills.json")
        datos["drills"][1]["estado"] = "TERMINADO"
        self.assertTrue(any("estado desconocido" in p for p in op.validar_drills(datos)["problemas"]))

    def test_accion_destructiva_se_rechaza(self) -> None:
        datos = _fixture("drills.json")
        datos["drills"][0]["accion"] = "destroy de la tabla"
        resultado = op.validar_drills(datos)
        self.assertEqual(resultado["estado"], "PLAN_RECHAZADO")
        self.assertIn("aislar-instancia-a", resultado["drills_mutantes"])

    def test_drill_con_aws_exige_datos_sinteticos(self) -> None:
        datos = _fixture("drills.json")
        datos["drills"][2]["datos_sinteticos"] = False
        self.assertTrue(
            any("datos sinteticos" in p for p in op.validar_drills(datos)["problemas"])
        )

    def test_campo_faltante(self) -> None:
        datos = _fixture("drills.json")
        del datos["drills"][4]["rollback"]
        self.assertTrue(any("rotacion-jwt: falta rollback" in p for p in op.validar_drills(datos)["problemas"]))

    def test_sin_datos_es_error(self) -> None:
        with self.assertRaises(op.ErrorUso):
            op.validar_drills(None)


class InventarioYAcceso(unittest.TestCase):
    def test_inventario_derivado_del_codigo(self) -> None:
        resultado = op.inventario_desde_codigo()
        self.assertEqual(resultado["estado"], "INVENTARIO_DERIVADO")
        self.assertGreater(resultado["recursos_declarados"], 0)
        self.assertEqual(resultado["existe_en_aws"], "NOT_VERIFIED")

    def test_nombres_no_prueban_existencia(self) -> None:
        self.assertIn("no prueba existencia", op.inventario_desde_codigo()["advertencia"])

    def test_inventario_vacio_con_raiz_sin_tf(self) -> None:
        vacio = Path(tempfile.mkdtemp())
        self.assertEqual(op.inventario_desde_codigo(vacio)["estado"], "INVENTARIO_VACIO")

    def test_acceso_conforme(self) -> None:
        resultado = op.validar_acceso(_fixture("acceso.json"))
        self.assertEqual(resultado["estado"], "ACCESO_CONFORME")

    def test_root_se_rechaza(self) -> None:
        datos = _fixture("acceso.json")
        datos["shell_root"] = True
        self.assertEqual(op.validar_acceso(datos)["estado"], "ACCESO_RECHAZADO")

    def test_ssh_publico_se_rechaza(self) -> None:
        datos = _fixture("acceso.json")
        datos["ssh_publico"] = True
        self.assertTrue(any("SSH" in p for p in op.validar_acceso(datos)["problemas"]))

    def test_claves_estaticas_se_rechazan(self) -> None:
        datos = _fixture("acceso.json")
        datos["claves_estaticas"] = True
        self.assertEqual(op.validar_acceso(datos)["estado"], "ACCESO_RECHAZADO")

    def test_identidades_temporales_obligatorias(self) -> None:
        datos = _fixture("acceso.json")
        datos["identidades_temporales"] = False
        self.assertTrue(
            any("identidades temporales" in p for p in op.validar_acceso(datos)["problemas"])
        )

    def test_cambio_scp_y_ssm_fuera_de_ventana(self) -> None:
        datos = _fixture("acceso.json")
        datos["cambio_scp"] = True
        datos["ssm_solo_en_ventana"] = False
        problemas = op.validar_acceso(datos)["problemas"]
        self.assertTrue(any("SCP" in p for p in problemas))
        self.assertTrue(any("SSM" in p for p in problemas))


class CliYSeguridad(unittest.TestCase):
    def test_cli_runbooks(self) -> None:
        self.assertEqual(op.main(["runbooks"]), 0)

    def test_cli_drills_rechazado(self) -> None:
        datos = _fixture("drills.json")
        datos["drills"][0]["estado"] = "EJECUTADO_AWS"
        self.assertEqual(op.main(["drills", "--datos", str(_json(datos))]), 1)

    def test_cli_inventario(self) -> None:
        self.assertEqual(op.main(["inventario"]), 0)

    def test_cli_comando_desconocido(self) -> None:
        self.assertEqual(op.main(["destroy-all"]), 2)

    def test_no_consulta_aws(self) -> None:
        codigo, resultado = op.ejecutar(["inventario"])
        self.assertEqual(codigo, 0)
        self.assertFalse(resultado["aws_consultado"])

    def test_no_importa_aws_ni_procesos(self) -> None:
        fuente = (RAIZ / "herramientas" / "verificar_operacion_i140.py").read_text(encoding="utf-8")
        for prohibido in (
            "import boto3", "import botocore", "from botocore",
            "import subprocess", "from subprocess", "os.system",
        ):
            self.assertNotIn(prohibido, fuente)


if __name__ == "__main__":  # pragma: no cover
    unittest.main()
