"""Pruebas estáticas de I-040 (DynamoDB, tabla única A-060).

Analizan la estructura del módulo (bloques y atributos) y no sólo cadenas:
un atributo declarado en el bloque equivocado debe fallar. `terraform validate`
no evalúa preconditions ni relaciones con la matriz física, así que estas
pruebas cubren ese hueco. La verificación real en AWS es `NOT_RUN_AWS`.
"""

from __future__ import annotations

import re
import unittest
from pathlib import Path

from test_balanceo_contrato import _leer, _sin_comentarios, atributos, bloque, recursos

RAIZ = Path(__file__).resolve().parents[3]
MODULO = RAIZ / "infra" / "terraform" / "modules" / "dynamodb"
ENV = RAIZ / "infra" / "terraform" / "envs" / "h08i"
RED = RAIZ / "infra" / "terraform" / "modules" / "red"
MATRIZ = RAIZ / "sdd" / "02-infraestructura" / "MATRIZ_FISICA_DYNAMODB_E001_E075.md"


class TablaUnica(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.main = _leer(MODULO / "main.tf")
        cls.vars = _leer(MODULO / "variables.tf")
        cls.outs = _leer(MODULO / "outputs.tf")
        cls.recursos = recursos(cls.main)

    def test_una_sola_tabla(self) -> None:
        self.assertEqual(sum(1 for t, _ in self.recursos if t == "aws_dynamodb_table"), 1)
        self.assertIn(("aws_dynamodb_table", "core"), self.recursos)

    def test_nombre_nexum_core_con_entorno(self) -> None:
        self.assertIn('nombre_tabla = "${var.nombre_tabla}_${upper(var.entorno)}"', self.main)
        self.assertIn('default     = "NEXUM_CORE"', self.vars)
        self.assertIn('var.nombre_tabla == "NEXUM_CORE"', self.vars)

    def test_on_demand_sin_capacidad_provisionada(self) -> None:
        cuerpo = self.recursos[("aws_dynamodb_table", "core")]
        self.assertEqual(atributos(cuerpo).get("billing_mode"), '"PAY_PER_REQUEST"')
        self.assertNotIn("read_capacity", cuerpo)
        self.assertNotIn("write_capacity", cuerpo)

    def test_claves_pk_sk_tipo_string(self) -> None:
        cuerpo = self.recursos[("aws_dynamodb_table", "core")]
        claves = atributos(cuerpo)
        self.assertEqual(claves.get("hash_key"), '"PK"')
        self.assertEqual(claves.get("range_key"), '"SK"')
        self.assertIn('{ name = "PK", type = "S" }', self.main)
        self.assertIn('{ name = "SK", type = "S" }', self.main)

    def test_atributos_dinamicos_sin_duplicados(self) -> None:
        self.assertIn('dynamic "attribute"', self.main)
        self.assertIn("length(distinct([for a in local.atributos : a.name])) == length(local.atributos)", self.main)

    def test_gsi1_justificado_por_movimientos(self) -> None:
        self.assertRegex(self.main, r'name\s+= "GSI1"')
        self.assertRegex(self.main, r'hash_key\s+= "GSI1PK"')
        self.assertRegex(self.main, r'range_key\s+= "GSI1SK"')
        self.assertRegex(self.main, r'projection_type\s+= "ALL"')

    def test_gsi2_solo_optativo(self) -> None:
        """INF-040-07 prohíbe GSI especulativos: GSI2 requiere worker confirmado."""
        self.assertRegex(self.main, r'name\s+= "GSI2"')
        self.assertIn("var.habilitar_gsi2 ? [", self.main)
        self.assertIn('default     = false', self.vars)
        self.assertIn('var.habilitar_gsi2 == contains([for i in local.indices : i.name], "GSI2")', self.main)

    def test_ttl_expiresat_habilitado(self) -> None:
        cuerpo = self.recursos[("aws_dynamodb_table", "core")]
        ttl = atributos(bloque(cuerpo, "ttl"))
        self.assertEqual(ttl.get("attribute_name"), "var.atributo_ttl")
        self.assertEqual(ttl.get("enabled"), "true")
        self.assertIn('default     = "expiresAt"', self.vars)
        self.assertIn('var.atributo_ttl == "expiresAt"', self.vars)

    def test_cifrado_en_reposo(self) -> None:
        cuerpo = self.recursos[("aws_dynamodb_table", "core")]
        sse = atributos(bloque(cuerpo, "server_side_encryption"))
        self.assertEqual(sse.get("enabled"), "true")

    def test_pitr_presupuestado_y_configurable(self) -> None:
        cuerpo = self.recursos[("aws_dynamodb_table", "core")]
        pitr = atributos(bloque(cuerpo, "point_in_time_recovery"))
        self.assertEqual(pitr.get("enabled"), "var.pitr_habilitado")
        self.assertIn("pitr_habilitado", self.vars)
        self.assertIn('default     = true', self.vars)

    def test_sin_streams(self) -> None:
        cuerpo = self.recursos[("aws_dynamodb_table", "core")]
        self.assertEqual(atributos(cuerpo).get("stream_enabled"), "false")
        self.assertNotIn("stream_view_type", cuerpo)

    def test_proteccion_borrado_por_entorno(self) -> None:
        cuerpo = self.recursos[("aws_dynamodb_table", "core")]
        self.assertEqual(atributos(cuerpo).get("deletion_protection_enabled"), "var.proteccion_borrado")
        self.assertIn("proteccion_borrado", self.vars)
        self.assertIn('default     = false', self.vars)

    def test_sin_recursos_ajenos_a_i040(self) -> None:
        tipos = {tipo for tipo, _ in self.recursos}
        self.assertEqual(tipos, {"terraform_data", "aws_dynamodb_table"})
        for prohibido in (
            "aws_dynamodb_global_table",
            "aws_dynamodb_table_item",
            "aws_dynamodb_resource_policy",
            "aws_backup_plan",
            "aws_kms_key",
            "aws_dynamodb_tag",
        ):
            self.assertNotIn(prohibido, tipos)

    def test_etiqueta_de_spec(self) -> None:
        self.assertIn('Spec = "I-040-dynamodb"', self.main)

    def test_estado_no_se_declara_desplegado(self) -> None:
        self.assertIn('value       = "NOT_RUN_AWS"', self.outs)


class MatrizFisicaCoherente(unittest.TestCase):
    """La IaC debe reproducir la matriz física de H05, no reinterpretarla."""

    @classmethod
    def setUpClass(cls) -> None:
        cls.matriz = _leer(MATRIZ)
        cls.main = _leer(MODULO / "main.tf")

    def test_nombre_y_claves_de_la_matriz(self) -> None:
        self.assertIn("NEXUM_CORE_<env>", self.matriz)
        self.assertIn("`PK` (S)", self.matriz)
        self.assertIn("`SK` (S)", self.matriz)
        self.assertIn("`expiresAt` (N TTL)", self.matriz)
        self.assertIn("`PAY_PER_REQUEST`", self.matriz)

    def test_prefijos_gsi_de_la_matriz(self) -> None:
        self.assertIn("GSI1PK=USER#<id>#TYPE#<movType>", self.matriz)
        self.assertIn("GSI2PK=DUE#<yyyy-mm-dd>#<shard>", self.matriz)
        # El adaptador Go ya emite exactamente esos prefijos (A-060).
        keys = _leer(RAIZ / "internal" / "domain" / "dynamo" / "keys.go")
        self.assertIn("TypePrefix      = \"#TYPE#\"", keys)
        self.assertIn("gsi1pk = fmt.Sprintf(\"%s%s%s%s\", PrefixUser, userID, TypePrefix", keys)

    def test_ttl_de_la_matriz_es_el_del_modulo(self) -> None:
        self.assertIn('default     = "expiresAt"', _leer(MODULO / "variables.tf"))


class IntegracionConLaRed(unittest.TestCase):
    """DynamoDB se alcanza por el gateway endpoint de I-010, sin NAT."""

    @classmethod
    def setUpClass(cls) -> None:
        cls.red = _leer(RED / "main.tf")

    def test_gateway_endpoint_dynamodb_existente(self) -> None:
        self.assertIn('resource "aws_vpc_endpoint" "dynamodb"', self.red)
        self.assertIn('service_name      = "com.amazonaws.${var.region}.dynamodb"', self.red)
        self.assertRegex(self.red, r'vpc_endpoint_type\s+= "Gateway"')

    def test_backend_puede_salir_443_al_endpoint(self) -> None:
        self.assertIn('resource "aws_vpc_security_group_egress_rule" "backend_dynamodb_443"', self.red)


class ComposicionDelEntorno(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.env = _leer(ENV / "main.tf")
        cls.env_vars = _leer(ENV / "variables.tf")

    def test_modulo_dynamodb_compuesto(self) -> None:
        self.assertIn('module "dynamodb"', self.env)
        self.assertIn('source = "../../modules/dynamodb"', self.env)
        for entrada in (
            "dynamodb_nombre_tabla",
            "dynamodb_atributo_ttl",
            "dynamodb_pitr_habilitado",
            "dynamodb_proteccion_borrado",
            "dynamodb_habilitar_gsi2",
        ):
            self.assertIn(entrada, self.env)

    def test_salidas_expuestas(self) -> None:
        for salida in ("table_name", "table_arn", "indices", "ttl", "pitr", "estado_despliegue"):
            self.assertIn(salida, self.env)

    def test_gsi2_desactivado_por_defecto_en_el_entorno(self) -> None:
        self.assertIn('variable "dynamodb_habilitar_gsi2"', self.env_vars)
        fragmento = self.env_vars.split('variable "dynamodb_habilitar_gsi2"')[1].split("}")[0]
        self.assertIn("false", fragmento)


if __name__ == "__main__":
    unittest.main()
