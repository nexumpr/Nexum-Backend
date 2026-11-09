"""Contrato estatico del modulo de operacion de I-120.

Estas pruebas no ejecutan `terraform plan` (denegado por politica) ni AWS: leen
el codigo declarativo y el inventario para demostrar que las guardas existen y
que fallan cerrado. La evaluacion real de los `precondition` se hace con
`terraform validate` y con la revision humana del plan.
"""

from __future__ import annotations

import json
import re
import unittest
from pathlib import Path

RAIZ = Path(__file__).resolve().parents[3]
MODULO = RAIZ / "infra" / "terraform" / "modules" / "operacion"
ENV = RAIZ / "infra" / "terraform" / "envs" / "h08i" / "operacion.tf"
INVENTARIO = RAIZ / "infra" / "terraform" / "operacion" / "inventario.json"
CONTROL = RAIZ / "sdd" / "_control" / "H08"
OPERADOR = RAIZ / "herramientas" / "nexum_operacion.py"


def _leer(ruta: Path) -> str:
    return ruta.read_text(encoding="utf-8")


def _inventario() -> dict:
    return json.loads(INVENTARIO.read_text(encoding="utf-8"))


class ModuloDeGuardas(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.main = _leer(MODULO / "main.tf")
        cls.vars = _leer(MODULO / "variables.tf")
        cls.outs = _leer(MODULO / "outputs.tf")
        cls.env = _leer(ENV)

    def test_el_modulo_no_crea_recursos_aws(self) -> None:
        self.assertNotIn('resource "aws_', self.main)
        self.assertNotIn("provider \"aws\"", self.main)

    def test_usa_terraform_data_para_las_guardas(self) -> None:
        self.assertIn('resource "terraform_data" "guardas_operativas"', self.main)
        self.assertIn("required_version", self.main)
        self.assertIn(">= 1.4.0", self.main)

    def test_precondition_de_deny_por_defecto(self) -> None:
        self.assertIn("length(local.denegados) == 0", self.main)
        self.assertIn("deny por defecto", self.main)

    def test_precondition_de_core_no_suspendible(self) -> None:
        self.assertIn("length(local.core_suspendibles) == 0", self.main)
        self.assertIn("nunca se suspende", self.main)

    def test_precondition_de_allowlist_desconocida(self) -> None:
        self.assertIn("length(local.allowlist_desconocida) == 0", self.main)

    def test_componentes_core_obligatorios(self) -> None:
        for componente in ("computo", "datos", "estado"):
            self.assertIn(f'"{componente}"', self.main)
        self.assertIn("length(local.componentes_core_faltantes) == 0", self.main)

    def test_core_suspendible_se_clasifica_como_denegado(self) -> None:
        self.assertIn('recurso.criticidad == "core" && recurso.suspendible', self.main)

    def test_auxiliar_fuera_de_allowlist_se_deniega(self) -> None:
        self.assertIn('recurso.criticidad == "auxiliary" && !contains(var.allowlist_auxiliar, nombre)', self.main)

    def test_etiqueta_incoherente_se_deniega(self) -> None:
        self.assertIn("recurso.etiqueta_criticidad != recurso.criticidad", self.main)

    def test_destroy_datos_y_estado_son_imposibles(self) -> None:
        for nombre in ("destroy_datos", "destroy_estado"):
            bloque = self.vars.split(f'variable "{nombre}"')[1].split("variable ")[0]
            self.assertIn("default     = false", bloque)
            self.assertIn(f"var.{nombre} == false", bloque)
        self.assertIn("var.destroy_datos == false && var.destroy_estado == false", self.main)
        self.assertIn("destroy-data esta prohibido", self.vars)
        self.assertIn("destroy-state esta prohibido", self.vars)

    def test_denegados_no_se_toleran(self) -> None:
        bloque = self.vars.split('variable "max_denegados"')[1].split("variable ")[0]
        self.assertIn("default     = 0", bloque)
        self.assertIn("var.max_denegados == 0", bloque)

    def test_allowlist_se_declara_explicita_en_el_entorno(self) -> None:
        lista = self.env.split("allowlist_auxiliar = [")[1].split("]")[0]
        for nombre in (
            "interface-endpoint-secretsmanager",
            "interface-endpoint-ssm",
            "interface-endpoint-ssmmessages",
        ):
            self.assertIn(f'"{nombre}"', lista)
        # La allowlist no puede derivarse del inventario: eso anularia la revision.
        self.assertNotIn("for item in local.inventario_operacion.items", lista)

    def test_entorno_lee_el_inventario_como_fuente_unica(self) -> None:
        self.assertIn("jsondecode(file(", self.env)
        self.assertIn("operacion/inventario.json", self.env)
        self.assertIn('module "operacion"', self.env)

    def test_outputs_declaran_politica_y_estado(self) -> None:
        for salida in ("core", "suspendibles", "denegados", "politica", "estado_despliegue"):
            self.assertIn(f'output "{salida}"', self.outs)
        self.assertIn('destroy_all            = "INEXISTENTE"', self.outs)
        self.assertIn("minimo_nodos_publicado = 2", self.outs)


class InventarioDeclarativo(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.inv = _inventario()
        cls.env = _leer(ENV)
        cls.items = {item["nombre"]: item for item in cls.inv["items"]}

    def test_cada_recurso_declara_criticidad_coherente(self) -> None:
        for item in self.inv["items"]:
            self.assertIn(item["criticidad"], ("core", "auxiliary"), item["nombre"])
            self.assertEqual(item["etiqueta_criticidad"], item["criticidad"], item["nombre"])

    def test_core_nunca_es_suspendible(self) -> None:
        for item in self.inv["items"]:
            if item["criticidad"] == "core":
                self.assertFalse(item["suspendible"], item["nombre"])

    def test_auxiliares_suspendibles_estan_en_la_allowlist_del_entorno(self) -> None:
        for item in self.inv["items"]:
            if item["criticidad"] == "auxiliary":
                self.assertTrue(item["suspendible"], item["nombre"])
                self.assertIn(f'"{item["nombre"]}"', self.env)

    def test_core_incluye_los_dos_nodos_alb_redis_dynamo_y_state(self) -> None:
        self.assertEqual(self.items["asg-backend"]["criticidad"], "core")
        partidas = self.items["asg-backend"]["partidas"]
        self.assertTrue(any(p["sku"] == "EC2_T4G_MICRO_HORA" and p["cantidad"] == "2" for p in partidas))
        for nombre in ("alb-interno", "redis-oss", "dynamodb-core", "terraform-state-s3"):
            self.assertEqual(self.items[nombre]["criticidad"], "core", nombre)

    def test_partidas_variables_no_llevan_cantidad(self) -> None:
        for item in self.inv["items"]:
            for partida in item.get("partidas", []):
                if partida["base"] == "variable":
                    self.assertIsNone(
                        partida["cantidad"],
                        f"{item['nombre']}/{partida['sku']}: una partida variable no puede fijar cantidad",
                    )

    def test_bases_de_partida_acotadas(self) -> None:
        validas = {"hora", "mes", "existencia", "variable"}
        for item in self.inv["items"]:
            for partida in item.get("partidas", []):
                self.assertIn(partida["base"], validas, item["nombre"])

    def test_politica_declara_lo_prohibido(self) -> None:
        politica = self.inv["politica"]
        self.assertIn("destroy-data", politica["prohibido"])
        self.assertIn("destroy-state", politica["prohibido"])
        self.assertIn("destroy-all", politica["prohibido"])
        self.assertEqual(politica["deny_por_defecto"].split()[0], "Un")
        self.assertIn("2 targets sanos", politica["dos_nodos"])

    def test_umbrales_de_credito_coinciden_con_la_spec(self) -> None:
        umbrales = self.inv["umbrales_credito_usd"]
        self.assertEqual(umbrales["aviso"], "45")
        self.assertEqual(umbrales["bloqueo_auxiliares"], "35")
        self.assertEqual(umbrales["incidente_critico"], "20")

    def test_ningun_recurso_afirma_estar_creado(self) -> None:
        estados = {item["estado_recurso"] for item in self.inv["items"]}
        self.assertTrue(estados <= {"DECLARADO_NO_CREADO", "PRESUPUESTADO_NO_CREADO", "BLOQUEADO_RFC004"}, estados)

    def test_inventario_y_operador_usan_el_mismo_archivo(self) -> None:
        operador = _leer(OPERADOR)
        self.assertIn("operacion", operador)
        self.assertIn("inventario.json", operador)
        self.assertIn("precios_us-east-2.json", operador)


class AlcanceDelPlanDeclarativo(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.main = _leer(MODULO / "main.tf")
        cls.vars = _leer(MODULO / "variables.tf")
        cls.outs = _leer(MODULO / "outputs.tf")
        cls.env = _leer(ENV)
        cls.inv = _inventario()

    def test_variable_de_cambios_propuestos(self) -> None:
        self.assertIn('variable "cambios_propuestos"', self.vars)
        self.assertIn("direccion = string", self.vars)
        self.assertIn("accion    = string", self.vars)
        self.assertIn("default = []", self.vars)

    def test_no_se_admiten_delete_ni_destroy(self) -> None:
        bloque = self.vars.split('variable "cambios_propuestos"')[1].split("variable ")[0]
        self.assertIn('contains(["create", "update", "read", "no-op"]', bloque)
        self.assertNotIn('"delete"', bloque)
        self.assertNotIn('"destroy"', bloque)

    def test_precondition_de_cambios_sobre_core(self) -> None:
        self.assertIn("length(local.cambios_sobre_core) == 0", self.main)
        self.assertIn("length(local.cambios_fuera_de_allowlist) == 0", self.main)

    def test_capacidad_minima_declarativa(self) -> None:
        self.assertIn("c.desired == null || c.desired >= 2", self.vars)
        self.assertIn("doble instancia", self.vars)

    def test_output_alcance_sin_delete(self) -> None:
        bloque = self.outs.split('output "alcance_plan"')[1]
        self.assertRegex(bloque, r"permite_delete\s+= false")
        self.assertRegex(bloque, r"permite_destroy\s+= false")
        self.assertRegex(bloque, r"minimo_nodos\s+= 2")

    def test_entorno_no_declara_plan_aplicado(self) -> None:
        self.assertIn("cambios_propuestos = []", self.env)
        self.assertIn("alcance_plan", self.env)

    def test_inventario_declara_impacto_core(self) -> None:
        permitidos = {"NINGUNO", "VENTANA_DECLARADA", "BLOQUEANTE"}
        for item in self.inv["items"]:
            self.assertIn(item.get("impacto_core", "NINGUNO"), permitidos, item["nombre"])
        self.assertEqual(
            self.cinv("interface-endpoint-ssm")["impacto_core"], "VENTANA_DECLARADA"
        )

    def test_ningun_core_declara_impacto_bloqueante(self) -> None:
        for item in self.inv["items"]:
            if item["criticidad"] == "core":
                self.assertNotEqual(item.get("impacto_core"), "BLOQUEANTE", item["nombre"])

    @classmethod
    def cinv(cls, nombre: str) -> dict:
        return {item["nombre"]: item for item in cls.inv["items"]}[nombre]


class DocumentosOperativos(unittest.TestCase):
    def test_maquina_de_estados_documentada(self) -> None:
        texto = _leer(CONTROL / "MAQUINA_ESTADOS_RUNTIME.md")
        for estado in ("OFF", "STARTING", "ON_2_HEALTHY", "DRAINING", "ERROR", "COST_REVIEW"):
            self.assertIn(estado, texto)
        self.assertIn("actor", texto)
        self.assertIn("timestamp", texto)

    def test_runbook_prohibe_clicks_y_destroys_amplios(self) -> None:
        texto = _leer(CONTROL / "RUNBOOK_START_STOP.md")
        self.assertIn("nexum_operacion.py", texto)
        self.assertIn("destroy-data", texto)
        self.assertIn("AWS_NOT_RUN", texto)

    def test_matriz_inf_120_01_06_existe(self) -> None:
        texto = _leer(CONTROL / "MATRIZ_INF_120.md")
        for contrato in (
            "INF-120-01", "INF-120-02", "INF-120-03",
            "INF-120-04", "INF-120-05", "INF-120-06",
        ):
            self.assertIn(contrato, texto)
        self.assertIn("BLOCKED_I130", texto)

    def test_inventario_residual_enumerado(self) -> None:
        texto = _leer(CONTROL / "INVENTARIO_RESIDUAL.md")
        self.assertIn("residual", texto.lower())
        self.assertIn("ALB", texto)
        self.assertIn("Redis", texto)
        self.assertIn("DynamoDB", texto)
        self.assertIn("no se declara", texto.lower())


if __name__ == "__main__":  # pragma: no cover
    unittest.main()
