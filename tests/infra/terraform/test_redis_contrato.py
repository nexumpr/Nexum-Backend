"""Pruebas estáticas de I-050 (Redis OSS compartido en ElastiCache).

Analizan la estructura del módulo (bloques y atributos) y la coherencia con
I-000 (1 nodo `cache.t4g.micro`), I-010 (SG y subredes) y A-080 (prefijo de
claves). La verificación real en AWS es `NOT_RUN_AWS`: no se ejecutó plan ni
apply.
"""

from __future__ import annotations

import re
import unittest
from pathlib import Path

from test_balanceo_contrato import _leer, _sin_comentarios, atributos, bloque, recursos

RAIZ = Path(__file__).resolve().parents[3]
MODULO = RAIZ / "infra" / "terraform" / "modules" / "redis"
ENV = RAIZ / "infra" / "terraform" / "envs" / "h08i"
RED = RAIZ / "infra" / "terraform" / "modules" / "red"


class ServicioYTopologia(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.main = _leer(MODULO / "main.tf")
        cls.vars = _leer(MODULO / "variables.tf")
        cls.outs = _leer(MODULO / "outputs.tf")
        cls.recursos = recursos(cls.main)

    def test_un_solo_grupo_y_sin_cluster_duplicado(self) -> None:
        self.assertEqual(sum(1 for t, _ in self.recursos if t == "aws_elasticache_replication_group"), 1)
        self.assertNotIn("aws_elasticache_cluster", {t for t, _ in self.recursos})
        self.assertNotIn("aws_elasticache_serverless_cache", {t for t, _ in self.recursos})

    def test_motor_redis_oss_verificado(self) -> None:
        self.assertIn('default     = "redis"', self.vars)
        self.assertIn("engine         = var.motor", self.main)
        self.assertIn('var.motor == "redis"', self.vars)
        self.assertIn('default     = "7.1"', self.vars)

    def test_un_nodo_t4g_micro_sin_replicas(self) -> None:
        grupo = self.recursos[("aws_elasticache_replication_group", "redis")]
        atributos_grupo = atributos(grupo)
        self.assertEqual(atributos_grupo.get("node_type"), "var.tipo_nodo")
        self.assertEqual(atributos_grupo.get("num_cache_clusters"), "var.num_nodos")
        self.assertIn('default     = "cache.t4g.micro"', self.vars)
        self.assertIn("var.tipo_nodo == \"cache.t4g.micro\"", self.vars)
        self.assertIn("var.num_nodos == 1", self.vars)

    def test_no_se_declara_alta_disponibilidad(self) -> None:
        grupo = atributos(self.recursos[("aws_elasticache_replication_group", "redis")])
        self.assertEqual(grupo.get("automatic_failover_enabled"), "var.habilitar_failover")
        self.assertEqual(grupo.get("multi_az_enabled"), "var.habilitar_failover")
        self.assertIn("var.habilitar_failover == (var.num_nodos > 1)", self.main)
        self.assertIn('default     = false', self.vars)
        self.assertIn("alta_disponibilidad = false", self.outs)

    def test_endpoint_compartido_expuesto(self) -> None:
        self.assertIn("primary_endpoint_address", self.outs)
        self.assertIn("output \"endpoint_primario\"", self.outs)


class Seguridad(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.main = _leer(MODULO / "main.tf")
        cls.vars = _leer(MODULO / "variables.tf")
        cls.outs = _leer(MODULO / "outputs.tf")
        cls.recursos = recursos(cls.main)

    def test_tls_y_cifrado_en_reposo(self) -> None:
        grupo = atributos(self.recursos[("aws_elasticache_replication_group", "redis")])
        self.assertEqual(grupo.get("transit_encryption_enabled"), "var.cifrado_en_transito")
        self.assertEqual(grupo.get("at_rest_encryption_enabled"), "var.cifrado_en_reposo")
        self.assertRegex(self.vars, r'variable "cifrado_en_transito"[\s\S]*?default     = true')
        self.assertRegex(self.vars, r'variable "cifrado_en_reposo"[\s\S]*?default     = true')
        self.assertIn("var.cifrado_en_transito && var.cifrado_en_reposo", self.main)

    def test_rbac_sin_auth_token_en_texto(self) -> None:
        self.assertIn(("aws_elasticache_user", "aplicacion"), self.recursos)
        self.assertIn(("aws_elasticache_user_group", "aplicacion"), self.recursos)
        self.assertIn("user_group_ids = [aws_elasticache_user_group.aplicacion.user_group_id]", self.main)
        self.assertNotIn("auth_token", _sin_comentarios(self.main))
        self.assertIn("user_group_ids", self.outs)

    def test_acl_acotada_al_espacio_nexum(self) -> None:
        usuario = self.recursos[("aws_elasticache_user", "aplicacion")]
        self.assertIn('access_string = "on ~nexum:* +@all -@admin -@dangerous"', usuario)
        # El prefijo del ACL debe coincidir con el espacio de claves real (A-080).
        claves = _leer(RAIZ / "internal" / "domain" / "cache" / "keys.go")
        self.assertIn('CachePrefix = "nexum:v1"', claves)

    def test_password_sensible_y_sin_valor(self) -> None:
        fragmento = self.vars.split('variable "password_aplicacion"')[1].split("}")[0]
        self.assertIn("sensitive   = true", fragmento)
        self.assertIn('default     = ""', fragmento)
        self.assertIn("length(var.password_aplicacion) >= 16", self.main)
        self.assertNotRegex(self.vars, r"password_aplicacion\s*=\s*\"[^\"]{16,}\"")

    def test_red_privada_sin_sg_propio(self) -> None:
        grupo = self.recursos[("aws_elasticache_subnet_group", "redis")]
        self.assertIn("subnet_ids  = local.subredes", grupo)
        self.assertIn("security_group_ids   = [var.sg_redis_id]", self.main)
        self.assertNotIn("aws_security_group", {t for t, _ in self.recursos})
        self.assertNotIn("cidr_blocks", self.main)
        self.assertNotIn("0.0.0.0/0", self.main)
        self.assertIn("var.sg_redis_id != var.sg_backend_id", self.main)

    def test_puerto_6379_de_i010(self) -> None:
        self.assertIn("var.puerto == 6379", self.vars)
        self.assertIn("port           = var.puerto", self.main)
        red = _leer(RED / "main.tf")
        self.assertIn('resource "aws_vpc_security_group_ingress_rule" "redis_desde_backend"', red)
        self.assertIn("from_port                    = 6379", red)


class CacheYTtl(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.main = _leer(MODULO / "main.tf")
        cls.recursos = recursos(cls.main)

    def test_eviccion_solo_de_claves_con_ttl(self) -> None:
        grupo = atributos(self.recursos[("aws_elasticache_parameter_group", "redis")])
        self.assertEqual(grupo.get("family"), '"redis7"')
        self.assertIn('value = "volatile-lru"', self.main)

    def test_sin_snapshots_porque_dynamodb_es_la_verdad(self) -> None:
        grupo = atributos(self.recursos[("aws_elasticache_replication_group", "redis")])
        self.assertEqual(grupo.get("snapshot_retention_limit"), "0")
        self.assertNotIn("final_snapshot_identifier", self.main)

    def test_estado_no_se_declara_desplegado(self) -> None:
        self.assertIn('value       = "NOT_RUN_AWS"', _leer(MODULO / "outputs.tf"))


class ComposicionDelEntorno(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.env = _leer(ENV / "main.tf")
        cls.env_vars = _leer(ENV / "variables.tf")

    def test_modulo_redis_compuesto(self) -> None:
        self.assertIn('module "redis"', self.env)
        self.assertIn('source = "../../modules/redis"', self.env)
        self.assertIn('sg_redis_id   = module.red.security_groups["redis"]', self.env)
        self.assertIn("subnet_ids    = module.red.subredes_privadas", self.env)

    def test_password_sensible_en_el_entorno(self) -> None:
        fragmento = self.env_vars.split('variable "redis_password_aplicacion"')[1].split("}")[0]
        self.assertIn("sensitive   = true", fragmento)
        self.assertIn('default     = ""', fragmento)

    def test_salidas_expuestas(self) -> None:
        for salida in ("endpoint", "topologia", "seguridad", "cache", "estado_despliegue"):
            self.assertIn(salida, self.env)


if __name__ == "__main__":
    unittest.main()
