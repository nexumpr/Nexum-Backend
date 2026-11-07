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
        """Mínimo privilegio: -@all + categorías necesarias; sin admin/dangerous."""
        usuario = self.recursos[("aws_elasticache_user", "aplicacion")]
        self.assertIn(
            'access_string = "on ~nexum:* -@all +@read +@write +@keyspace +@scripting +@transaction +@connection -keys -scan -randomkey -restore -migrate -sort"',
            usuario,
        )
        self.assertNotIn("+@all", usuario)
        self.assertNotIn("+@admin", usuario)
        self.assertNotIn("+@dangerous", usuario)
        self.assertNotIn("@transactions", usuario)
        # Categorías que A-080 necesita: lecturas, escrituras, TTL/keyspace,
        # scripts (rate limiting atómico), transacciones y conexión.
        for categoria in ("+@read", "+@write", "+@keyspace", "+@scripting", "+@transaction", "+@connection"):
            self.assertIn(categoria, usuario)
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


class UsuarioDefaultDeshabilitado(unittest.TestCase):
    """ElastiCache añade un usuario `default` con `on ~* +@all` a todos los grupos
    y no permite borrarlo: hay que sustituirlo explícitamente (doc oficial RBAC)."""

    @classmethod
    def setUpClass(cls) -> None:
        cls.main = _leer(MODULO / "main.tf")
        cls.recursos = recursos(cls.main)

    def test_existe_usuario_default_explicito(self) -> None:
        self.assertIn(("aws_elasticache_user", "default_deshabilitado"), self.recursos)

    def test_user_name_es_default(self) -> None:
        usuario = atributos(self.recursos[("aws_elasticache_user", "default_deshabilitado")])
        self.assertEqual(usuario.get("user_name"), '"default"')
        self.assertEqual(usuario.get("engine"), '"redis"')

    def test_default_esta_deshabilitado_off(self) -> None:
        """`off` impide cualquier acceso, incluido anónimo."""
        usuario = atributos(self.recursos[("aws_elasticache_user", "default_deshabilitado")])
        self.assertEqual(usuario.get("access_string"), '"off ~* -@all"')

    def test_default_sin_permiso_de_claves_ni_comandos(self) -> None:
        cuerpo = self.recursos[("aws_elasticache_user", "default_deshabilitado")]
        access = atributos(cuerpo)["access_string"]
        self.assertTrue(access.startswith('"off'), "debe empezar por off")
        self.assertIn("-@all", access)
        self.assertNotIn("+@", access)
        self.assertNotIn('"on ', access)

    def test_default_sin_password_y_sin_acceso_anonimo_util(self) -> None:
        cuerpo = self.recursos[("aws_elasticache_user", "default_deshabilitado")]
        modo = atributos(bloque(cuerpo, "authentication_mode"))
        self.assertEqual(modo.get("type"), '"no-password-required"')
        self.assertNotIn("passwords", cuerpo)

    def test_grupo_referencia_ambas_identidades(self) -> None:
        cuerpo = self.recursos[("aws_elasticache_user_group", "aplicacion")]
        self.assertIn("aws_elasticache_user.aplicacion.user_id", cuerpo)
        self.assertIn("aws_elasticache_user.default_deshabilitado.user_id", cuerpo)
        ids = re.findall(r"aws_elasticache_user\.([a-z_]+)\.user_id", cuerpo)
        self.assertEqual(sorted(ids), ["aplicacion", "default_deshabilitado"])

    def test_grupo_aplicado_al_grupo_de_replicacion(self) -> None:
        self.assertIn(
            "user_group_ids = [aws_elasticache_user_group.aplicacion.user_group_id]",
            self.main,
        )

    def test_sin_passwords_embebidas(self) -> None:
        self.assertEqual(self.main.count("passwords = ["), 1)
        self.assertIn("passwords = [var.password_aplicacion]", self.main)
        for archivo in (MODULO / "main.tf", MODULO / "variables.tf"):
            texto = _leer(archivo)
            self.assertNotRegex(texto, r'passwords\s*=\s*\[\s*"[^"]{16,}"')

    def test_tls_cifrado_y_red_privada_intactos(self) -> None:
        grupo = atributos(self.recursos[("aws_elasticache_replication_group", "redis")])
        self.assertEqual(grupo.get("transit_encryption_enabled"), "var.cifrado_en_transito")
        self.assertEqual(grupo.get("at_rest_encryption_enabled"), "var.cifrado_en_reposo")
        self.assertEqual(grupo.get("security_group_ids"), "[var.sg_redis_id]")
        self.assertNotIn("cidr_blocks", self.main)
        self.assertIn("var.sg_redis_id != var.sg_backend_id && var.cifrado_en_transito", self.main)

    def test_no_se_anade_servicio_facturable(self) -> None:
        """Dos usuarios y un grupo no tienen coste; no aparece ningún recurso nuevo."""
        tipos = {t for t, _ in self.recursos}
        self.assertEqual(
            tipos,
            {
                "terraform_data",
                "aws_elasticache_subnet_group",
                "aws_elasticache_parameter_group",
                "aws_elasticache_user",
                "aws_elasticache_user_group",
                "aws_elasticache_replication_group",
            },
        )


# Categorías ACL oficiales de Redis OSS 7.x (`ACL CAT`): las del núcleo, sin los
# módulos opcionales (json/search/…) que ElastiCache no expone. `all` es además un
# pseudo-grupo aceptado por Redis en `+@all`/`-@all`.
CATEGORIAS_REDIS_7 = frozenset(
    {
        "keyspace", "read", "write", "set", "sortedset", "list", "hash", "string",
        "bitmap", "hyperloglog", "geo", "stream", "pubsub", "admin", "fast", "slow",
        "blocking", "dangerous", "connection", "transaction", "scripting",
    }
)
PSEUDO_CATEGORIAS = frozenset({"all"})


def categorias_de_acl(access_string: str) -> tuple[set[str], set[str]]:
    """Separa categorías concedidas/denegadas y comandos sueltos de un access string.

    Devuelve (categorias, comandos). Las categorías se devuelven sin el prefijo
    `@` y en minúsculas; los comandos sueltos sin el `+`/`-`.
    """

    categorias: set[str] = set()
    comandos: set[str] = set()
    for token in access_string.split():
        if token.startswith("@") or token[1:2] == "@":
            categorias.add(token.lstrip("+-@").lower())
        elif token[:1] in "+-" and not token.startswith("~"):
            comandos.add(token[1:].lower())
    return categorias, comandos


def categorias_invalidas(access_string: str) -> set[str]:
    """Categorías declaradas que no existen en Redis OSS 7.x."""
    categorias, _ = categorias_de_acl(access_string)
    return {c for c in categorias if c not in CATEGORIAS_REDIS_7 | PSEUDO_CATEGORIAS}


class ACLDeAplicacion(unittest.TestCase):
    """La ACL debe usar categorías reales de Redis OSS 7.x.

    `terraform validate` NO valida la semántica del access string: un nombre
    inexistente (p. ej. `@transactions` frente a `@transaction`) sólo fallaría al
    crear el usuario en AWS. Estas pruebas cierran ese hueco.
    """

    @classmethod
    def setUpClass(cls) -> None:
        cls.main = _leer(MODULO / "main.tf")
        cls.recursos = recursos(cls.main)
        cls.cuerpo = cls.recursos[("aws_elasticache_user", "aplicacion")]
        cls.access = atributos(cls.cuerpo)["access_string"].strip('"')

    def test_categorias_pertenecen_a_redis_oss_7(self) -> None:
        self.assertEqual(categorias_invalidas(self.access), set())

    def test_transacciones_en_singular(self) -> None:
        """Regresión: `@transactions` no existe en Redis y rompería el apply."""
        categorias, _ = categorias_de_acl(self.access)
        self.assertIn("transaction", categorias)
        self.assertNotIn("transactions", categorias)
        self.assertNotIn("@transactions", self.access)

    def test_detector_rechaza_categorias_inexistentes(self) -> None:
        self.assertEqual(categorias_invalidas("on ~* +@transactions"), {"transactions"})
        self.assertEqual(categorias_invalidas("on ~* +@transaction"), set())
        self.assertEqual(categorias_invalidas("on ~* +@inventada -@otra"), {"inventada", "otra"})

    def test_categorias_requeridas_por_a080(self) -> None:
        categorias, _ = categorias_de_acl(self.access)
        for requerida in ("read", "write", "keyspace", "scripting", "transaction", "connection"):
            self.assertIn(requerida, categorias)

    def test_sin_permisos_administrativos(self) -> None:
        self.assertNotIn("+@all", self.access)
        for prohibida in ("admin", "dangerous"):
            self.assertNotIn(prohibida, categorias_de_acl(self.access)[0])
        # Nunca se concede todo y se recorta después: se parte de -@all.
        self.assertTrue(self.access.startswith("on ~nexum:* -@all"), self.access)

    def test_comandos_peligrosos_denegados(self) -> None:
        _, comandos = categorias_de_acl(self.access)
        for denegado in ("keys", "scan", "randomkey", "restore", "migrate", "sort"):
            self.assertIn(denegado, comandos, f"{denegado} debe estar denegado explícitamente")
        self.assertNotIn("keys", categorias_de_acl(self.access)[0])

    def test_espacio_de_claves_nexum(self) -> None:
        self.assertIn("~nexum:*", self.access)
        self.assertNotIn("~*", self.access.replace("~nexum:*", ""))


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
