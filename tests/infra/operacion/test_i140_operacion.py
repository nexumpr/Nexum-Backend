"""Pruebas adversas de operacion e incidentes de I-140 (alcance ampliado)."""

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
MATRIZ = RAIZ / "sdd" / "_control" / "H08" / "MATRIZ_INF_140.md"


def _fixture(nombre: str) -> dict:
    return copy.deepcopy(json.loads((FIXTURES / nombre).read_text(encoding="utf-8")))


def _json(datos: object) -> Path:
    manejador, ruta = tempfile.mkstemp(suffix=".json")
    with open(manejador, "w", encoding="utf-8") as fh:
        json.dump(datos, fh, ensure_ascii=False)
    return Path(ruta)


class Runbooks(unittest.TestCase):
    def test_doce_procedimientos_completos(self) -> None:
        resultado = op.validar_runbooks(RUNBOOK.read_text(encoding="utf-8"))
        self.assertEqual(resultado["estado"], "RUNBOOKS_COMPLETOS")
        self.assertEqual(resultado["requeridos"], 12)
        self.assertEqual(resultado["presentes"], 12)
        self.assertEqual(resultado["campos_por_runbook"], 13)
        self.assertTrue(resultado["todos_documentados_sin_ejecutar"])

    def test_procedimientos_exigidos(self) -> None:
        esperados = {
            "deploy-rollback", "health-fail", "instancia-perdida", "dynamo-throttle",
            "redis-unavailable", "https-certificados-cloudfront", "rotacion-secretos-jwt",
            "auth-inesperado", "incidente-seguridad-datos", "contencion-presupuestal",
            "restauracion-continuidad", "eliminacion-iac",
        }
        self.assertEqual(set(op.RUNBOOKS_REQUERIDOS), esperados)

    def test_campo_faltante_se_detecta(self) -> None:
        texto = RUNBOOK.read_text(encoding="utf-8").replace("- **recuperacion**", "- (recuperacion)", 1)
        resultado = op.validar_runbooks(texto)
        self.assertEqual(resultado["estado"], "RUNBOOKS_INCOMPLETOS")
        self.assertIn("recuperacion", resultado["faltantes"]["deploy-rollback"])

    def test_runbook_faltante_se_detecta(self) -> None:
        texto = RUNBOOK.read_text(encoding="utf-8").replace("## eliminacion-iac", "## otro")
        self.assertIn("eliminacion-iac", op.validar_runbooks(texto)["faltantes"])

    def test_comando_aws_peligroso_se_detecta(self) -> None:
        texto = RUNBOOK.read_text(encoding="utf-8").replace(
            "- **mitigacion**: Abortar el refresh", "- **mitigacion**: aws ec2 terminate-instances", 1
        )
        resultado = op.validar_runbooks(texto)
        self.assertEqual(resultado["estado"], "RUNBOOKS_INCOMPLETOS")
        self.assertIn("deploy-rollback", resultado["comandos_peligrosos"])

    def test_runbook_inexistente_es_error(self) -> None:
        with self.assertRaises(op.ErrorUso):
            op.validar_runbooks(ruta=Path(tempfile.mkdtemp()) / "no-existe.md")

    def test_ruta_alternativa_no_cae_al_defecto(self) -> None:
        with self.assertRaises(op.ErrorUso):
            op.ejecutar(["runbooks", "--runbook", str(Path(tempfile.mkdtemp()) / "otro.md")])


class Incidentes(unittest.TestCase):
    def test_matriz_valida(self) -> None:
        resultado = op.validar_incidentes(_fixture("incidentes.json"))
        self.assertEqual(resultado["estado"], "INCIDENTES_VALIDOS")
        self.assertEqual(resultado["severidades"], ["S1", "S2", "S3"])
        self.assertEqual(resultado["ratificacion"], "PENDIENTE_DECISION_HUMANA")

    def test_severidad_desconocida(self) -> None:
        datos = _fixture("incidentes.json")
        datos["incidentes"][0]["severidad"] = "S9"
        self.assertEqual(op.validar_incidentes(datos)["estado"], "INCIDENTES_RECHAZADOS")

    def test_incidente_sin_propietario(self) -> None:
        datos = _fixture("incidentes.json")
        del datos["incidentes"][1]["propietario"]
        self.assertTrue(any("sin propietario" in p for p in op.validar_incidentes(datos)["problemas"]))

    def test_s1_sin_escalamiento(self) -> None:
        datos = _fixture("incidentes.json")
        datos["incidentes"][0]["escalamiento"] = ""
        self.assertTrue(any("S1 exige escalamiento" in p for p in op.validar_incidentes(datos)["problemas"]))

    def test_datos_sensibles_se_rechazan(self) -> None:
        datos = _fixture("incidentes.json")
        datos["incidentes"][0]["contiene_datos_sensibles"] = True
        resultado = op.validar_incidentes(datos)
        self.assertEqual(resultado["estado"], "INCIDENTES_RECHAZADOS")
        self.assertTrue(any("datos sensibles" in p for p in resultado["problemas"]))

    def test_evidencia_aws_simulada_se_rechaza(self) -> None:
        datos = _fixture("incidentes.json")
        datos["incidentes"][2]["evidencia_aws_simulada"] = True
        self.assertTrue(
            any("AWS simulada" in p for p in op.validar_incidentes(datos)["problemas"])
        )

    def test_regla_contradictoria_de_tiempos(self) -> None:
        datos = _fixture("incidentes.json")
        datos["incidentes"][0]["tiempo_respuesta"] = 120
        datos["incidentes"][1]["tiempo_respuesta"] = 30
        datos["incidentes"][2]["tiempo_respuesta"] = 60
        self.assertTrue(any("contradiccion" in p for p in op.validar_incidentes(datos)["problemas"]))

    def test_falta_severidad(self) -> None:
        datos = _fixture("incidentes.json")
        datos["incidentes"] = datos["incidentes"][:2]
        self.assertTrue(any("faltan severidades" in p for p in op.validar_incidentes(datos)["problemas"]))

    def test_ciclo_incompleto(self) -> None:
        datos = _fixture("incidentes.json")
        del datos["incidentes"][2]["reapertura"]
        self.assertTrue(any("falta reapertura" in p for p in op.validar_incidentes(datos)["problemas"]))

    def test_sin_datos_es_error(self) -> None:
        with self.assertRaises(op.ErrorUso):
            op.validar_incidentes(None)


class Drills(unittest.TestCase):
    def test_doce_drills_validos(self) -> None:
        resultado = op.validar_drills(_fixture("drills.json"))
        self.assertEqual(resultado["estado"], "PLAN_VALIDO")
        self.assertEqual(resultado["requeridos"], 12)
        self.assertEqual(resultado["ejecucion_aws"], "NOT_RUN")
        self.assertEqual(resultado["declaran_aws_sin_evidencia"], [])

    def test_estados_permitidos(self) -> None:
        self.assertEqual(
            set(op.ESTADOS_DRILL),
            {"LOCAL_PASS", "LOCAL_FAIL", "NOT_RUN_AWS", "BLOCKED_DEPENDENCY"},
        )

    def test_drill_faltante(self) -> None:
        datos = _fixture("drills.json")
        datos["drills"] = [d for d in datos["drills"] if d["id"] != "riesgo-rollback"]
        resultado = op.validar_drills(datos)
        self.assertEqual(resultado["estado"], "PLAN_RECHAZADO")
        self.assertIn("riesgo-rollback", resultado["faltantes"])

    def test_estado_desconocido(self) -> None:
        datos = _fixture("drills.json")
        datos["drills"][0]["estado"] = "EJECUTADO_AWS"
        self.assertTrue(any("estado desconocido" in p for p in op.validar_drills(datos)["problemas"]))

    def test_demuestra_aws_se_rechaza(self) -> None:
        datos = _fixture("drills.json")
        datos["drills"][0]["demuestra_aws"] = True
        resultado = op.validar_drills(datos)
        self.assertEqual(resultado["estado"], "PLAN_RECHAZADO")
        self.assertIn("caida-instancia-a", resultado["declaran_aws_sin_evidencia"])

    def test_drill_con_aws_no_puede_pasar_en_local(self) -> None:
        datos = _fixture("drills.json")
        datos["drills"][3]["estado"] = "LOCAL_PASS"
        self.assertTrue(
            any("no puede declararse LOCAL_PASS" in p for p in op.validar_drills(datos)["problemas"])
        )

    def test_accion_destructiva(self) -> None:
        datos = _fixture("drills.json")
        datos["drills"][2]["accion"] = "destroy del target group"
        resultado = op.validar_drills(datos)
        self.assertIn("reemplazo-target", resultado["drills_mutantes"])

    def test_exige_datos_sinteticos(self) -> None:
        datos = _fixture("drills.json")
        datos["drills"][5]["datos_sinteticos"] = False
        self.assertTrue(
            any("datos sinteticos" in p for p in op.validar_drills(datos)["problemas"])
        )

    def test_sin_datos_es_error(self) -> None:
        with self.assertRaises(op.ErrorUso):
            op.validar_drills(None)


class Aceptacion(unittest.TestCase):
    def test_matriz_completa(self) -> None:
        resultado = op.validar_matriz(MATRIZ.read_text(encoding="utf-8"))
        self.assertEqual(resultado["estado"], "MATRIZ_COMPLETA")
        self.assertEqual(resultado["conteos"], {"A": 12, "I": 16, "E": 75})
        self.assertEqual(resultado["faltantes_en_matriz"], {})

    def test_conteos_del_sdd(self) -> None:
        reales = op.inventario_sdd()
        self.assertEqual(len(reales["A"]), 12)
        self.assertEqual(len(reales["I"]), 16)
        self.assertEqual(len(reales["E"]), 75)
        self.assertEqual(reales["A"][0], "A-000")
        self.assertEqual(reales["E"][0], "E001")

    def test_id_ausente_en_la_matriz(self) -> None:
        texto = MATRIZ.read_text(encoding="utf-8").replace("I-150", "I-XXX")
        resultado = op.validar_matriz(texto)
        self.assertEqual(resultado["estado"], "MATRIZ_INCOMPLETA")
        self.assertIn("I-150", resultado["faltantes_en_matriz"]["I"])

    def test_matriz_avisa_de_contratos_sin_handler(self) -> None:
        texto = MATRIZ.read_text(encoding="utf-8")
        self.assertIn("sin handler funcional", texto)
        self.assertIn("no demuestran", op.validar_matriz(texto)["advertencia"])

    def test_matriz_inexistente_es_error(self) -> None:
        with self.assertRaises(op.ErrorUso):
            op.validar_matriz(ruta=Path(tempfile.mkdtemp()) / "no.md")


class InventarioYAcceso(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.inv = op.inventario_infraestructura()

    def test_inventario_trazable(self) -> None:
        self.assertEqual(self.inv["estado"], "INVENTARIO_TRAZABLE")
        self.assertGreater(self.inv["total"], 15)
        self.assertEqual(self.inv["problemas"], [])
        self.assertEqual(self.inv["existe_en_aws"], "NOT_VERIFIED")

    def test_cada_recurso_tiene_fuente_owner_y_criticidad(self) -> None:
        for recurso in self.inv["recursos"]:
            self.assertTrue(recurso["fuente"], recurso["nombre"])
            self.assertTrue(recurso["owner"], recurso["nombre"])
            self.assertIn(recurso["criticidad"], ("CORE_24X7", "AUXILIAR_AUTORIZABLE"))
            self.assertFalse(recurso["verificado_aws"], recurso["nombre"])

    def test_nucleo_24x7_incluye_dos_ec2_alb_redis_y_dynamo(self) -> None:
        nombres = {r["nombre"] for r in self.inv["recursos"] if r["criticidad"] == "CORE_24X7"}
        for esperado in ("asg-backend", "alb-interno", "redis-oss", "dynamodb-core", "terraform-state-s3"):
            self.assertIn(esperado, nombres)
        self.assertGreaterEqual(self.inv["core_24x7"], 12)

    def test_auxiliares_no_incluyen_redis(self) -> None:
        auxiliares = [r["nombre"] for r in self.inv["recursos"] if r["criticidad"] == "AUXILIAR_AUTORIZABLE"]
        self.assertFalse(any("redis" in nombre for nombre in auxiliares))

    def test_inventario_no_consulta_aws(self) -> None:
        self.assertFalse(self.inv["aws_consultado"])
        self.assertIn("NOT_VERIFIED", self.inv["advertencia"])

    def test_acceso_conforme(self) -> None:
        resultado = op.validar_acceso(_fixture("acceso.json"))
        self.assertEqual(resultado["estado"], "ACCESO_CONFORME")

    def test_root_ssh_y_claves_se_rechazan(self) -> None:
        for campo in ("shell_root", "ssh_publico", "claves_estaticas", "cambio_scp"):
            datos = _fixture("acceso.json")
            datos[campo] = True
            with self.subTest(campo=campo):
                self.assertEqual(op.validar_acceso(datos)["estado"], "ACCESO_RECHAZADO")

    def test_auditoria_y_minimo_privilegio_obligatorios(self) -> None:
        for campo in ("minimo_privilegio", "auditoria", "identidades_temporales", "ssm_solo_en_ventana"):
            datos = _fixture("acceso.json")
            datos[campo] = False
            with self.subTest(campo=campo):
                self.assertEqual(op.validar_acceso(datos)["estado"], "ACCESO_RECHAZADO")


class CliYSeguridad(unittest.TestCase):
    def test_cli_runbooks(self) -> None:
        self.assertEqual(op.main(["runbooks"]), 0)

    def test_cli_incidentes(self) -> None:
        self.assertEqual(
            op.main(["incidentes", "--datos", str(FIXTURES / "incidentes.json")]), 0
        )

    def test_cli_drills_rechazado(self) -> None:
        datos = _fixture("drills.json")
        datos["drills"][0]["demuestra_aws"] = True
        self.assertEqual(op.main(["drills", "--datos", str(_json(datos))]), 1)

    def test_cli_aceptacion_e_inventario(self) -> None:
        self.assertEqual(op.main(["aceptacion"]), 0)
        self.assertEqual(op.main(["inventario"]), 0)

    def test_cli_comando_desconocido(self) -> None:
        self.assertEqual(op.main(["destroy-all"]), 2)

    def test_no_importa_aws_ni_procesos(self) -> None:
        fuente = (RAIZ / "herramientas" / "verificar_operacion_i140.py").read_text(encoding="utf-8")
        for prohibido in (
            "import boto3", "import botocore", "from botocore",
            "import subprocess", "from subprocess", "os.system",
        ):
            self.assertNotIn(prohibido, fuente)


if __name__ == "__main__":  # pragma: no cover
    unittest.main()
