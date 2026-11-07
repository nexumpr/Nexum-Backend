"""Pruebas estáticas de I-030 (balanceo HTTPS con ALB interno y CloudFront).

Comprueban invariantes de seguridad, disponibilidad y **estructura** del código
`.tf`: los bloques se extraen con conteo de llaves en lugar de buscar cadenas
sueltas, de modo que un atributo en el bloque equivocado no pase la prueba.

`terraform validate` no evalúa preconditions ni relaciones semánticas entre
recursos; estas pruebas cubren lo que el validador no ve. La verificación real en
AWS es `NOT_RUN_AWS`: no se ejecutó plan ni apply.
"""

from __future__ import annotations

import re
import unittest
from pathlib import Path

RAIZ = Path(__file__).resolve().parents[3]
MODULO = RAIZ / "infra" / "terraform" / "modules" / "balanceo"
ENV = RAIZ / "infra" / "terraform" / "envs" / "h08i"
RED = RAIZ / "infra" / "terraform" / "modules" / "red"
DEFINICIONES = RAIZ / "infra" / "terraform"


# --- Utilidades de análisis estructural -----------------------------------


def _leer(p: Path) -> str:
    return p.read_text(encoding="utf-8")


def _sin_comentarios(texto: str) -> str:
    """Elimina comentarios `#` y `//` fuera de cadenas."""
    salida, i, en_cadena = [], 0, False
    while i < len(texto):
        c = texto[i]
        if c == '"' and (i == 0 or texto[i - 1] != "\\"):
            en_cadena = not en_cadena
        if not en_cadena and c == "#":
            while i < len(texto) and texto[i] != "\n":
                i += 1
            continue
        if not en_cadena and texto.startswith("//", i):
            while i < len(texto) and texto[i] != "\n":
                i += 1
            continue
        salida.append(c)
        i += 1
    return "".join(salida)


def bloque(texto: str, cabecera: str) -> str:
    """Devuelve el interior del primer bloque que empieza por `cabecera {`."""
    limpio = _sin_comentarios(texto)
    inicio = limpio.find(cabecera)
    if inicio < 0:
        raise AssertionError(f"no se encontró el bloque {cabecera!r}")
    apertura = limpio.index("{", inicio + len(cabecera) - 1)
    nivel = 0
    for pos in range(apertura, len(limpio)):
        if limpio[pos] == "{":
            nivel += 1
        elif limpio[pos] == "}":
            nivel -= 1
            if nivel == 0:
                return limpio[apertura + 1 : pos]
    raise AssertionError(f"bloque {cabecera!r} sin cierre")


def atributos(cuerpo: str) -> dict[str, str]:
    """Atributos de primer nivel de un bloque, como `clave -> valor`."""
    resultado, nivel, buffer = {}, 0, ""
    for linea in cuerpo.splitlines():
        limpia = linea.strip()
        if nivel == 0 and re.match(r"^[a-zA-Z_][a-zA-Z0-9_]*\s*=", limpia):
            clave, valor = limpia.split("=", 1)
            resultado[clave.strip()] = valor.split("#")[0].strip()
            continue
        nivel += limpia.count("{") - limpia.count("}")
        buffer = buffer  # noqa: PLW0127 - documenta que el buffer no se usa
    return resultado


def recursos(texto: str) -> dict[tuple[str, str], str]:
    """Mapa (tipo, nombre) -> cuerpo del bloque `resource`."""
    limpio = _sin_comentarios(texto)
    encontrados = {}
    for m in re.finditer(r'resource\s+"([a-z0-9_]+)"\s+"([a-zA-Z0-9_-]+)"\s*\{', limpio):
        tipo, nombre = m.group(1), m.group(2)
        apertura = limpio.index("{", m.end() - 1)
        nivel = 0
        for pos in range(apertura, len(limpio)):
            if limpio[pos] == "{":
                nivel += 1
            elif limpio[pos] == "}":
                nivel -= 1
                if nivel == 0:
                    encontrados[(tipo, nombre)] = limpio[apertura + 1 : pos]
                    break
    return encontrados


class BalanceadorInterno(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.main = _leer(MODULO / "main.tf")
        cls.vars = _leer(MODULO / "variables.tf")
        cls.outs = _leer(MODULO / "outputs.tf")
        cls.recursos = recursos(cls.main)

    def test_alb_es_interno_estructural(self) -> None:
        cuerpo = self.recursos[("aws_lb", "principal")]
        atributos_alb = atributos(cuerpo)
        self.assertEqual(atributos_alb.get("internal"), "true")
        self.assertEqual(atributos_alb.get("load_balancer_type"), '"application"')
        self.assertNotIn("internet-facing", _sin_comentarios(self.main))

    def test_listener_http_80_estructural(self) -> None:
        cuerpo = self.recursos[("aws_lb_listener", "http_privado")]
        atributos_listener = atributos(cuerpo)
        self.assertEqual(atributos_listener.get("port"), "var.puerto_alb")
        self.assertEqual(atributos_listener.get("protocol"), '"HTTP"')
        self.assertIn("target_group_arn = aws_lb_target_group.backend.arn", cuerpo)
        # El puerto queda fijado en 80 por validación de variable.
        self.assertIn("var.puerto_alb == 80", self.vars)

    def test_target_group_8080_matcher_200_estructural(self) -> None:
        cuerpo = self.recursos[("aws_lb_target_group", "backend")]
        atributos_tg = atributos(cuerpo)
        self.assertEqual(atributos_tg.get("port"), "var.puerto_backend")
        self.assertEqual(atributos_tg.get("protocol"), '"HTTP"')
        self.assertEqual(atributos_tg.get("target_type"), '"instance"')
        salud = atributos(bloque(cuerpo, "health_check"))
        self.assertEqual(salud.get("matcher"), '"200"')
        self.assertEqual(salud.get("path"), "var.health_check_path")

    def test_health_check_no_usa_readiness(self) -> None:
        self.assertIn('default     = "/api/v1/health/live"', self.vars)
        self.assertIn("!endswith(var.health_check_path, \"/ready\")", self.vars)
        self.assertIn("!endswith(var.health_check_path, \"/ready\")", self.main)
        self.assertNotIn('default     = "/api/v1/health/ready"', self.vars)

    def test_deregistration_delay_acotado(self) -> None:
        self.assertIn("default     = 30", self.vars)
        self.assertIn("var.deregistration_delay <= 60", self.vars)
        self.assertEqual(
            atributos(self.recursos[("aws_lb_target_group", "backend")]).get("deregistration_delay"),
            "var.deregistration_delay",
        )

    def test_sin_stickiness(self) -> None:
        cuerpo = self.recursos[("aws_lb_target_group", "backend")]
        self.assertNotIn("stickiness", cuerpo)
        self.assertNotIn("lb_cookie", self.main)

    def test_asociacion_al_asg_sin_servidores_extra(self) -> None:
        self.assertIn(("aws_autoscaling_attachment", "backend"), self.recursos)
        self.assertEqual(
            atributos(self.recursos[("aws_autoscaling_attachment", "backend")]).get("autoscaling_group_name"),
            "var.asg_name",
        )
        for prohibido in ("aws_instance", "aws_launch_template", "aws_autoscaling_group"):
            self.assertNotIn(prohibido, {tipo for tipo, _ in self.recursos})

    def test_dos_zonas_distintas(self) -> None:
        self.assertIn("local.zonas_count == 2", self.main)
        self.assertIn("length(distinct(values(var.subnet_ids))) == 2", self.vars)
        self.assertIn("length(var.subnet_ids) == 2", self.vars)
        self.assertEqual(
            atributos(self.recursos[("aws_lb", "principal")]).get("subnets"), "local.subredes"
        )
        self.assertIn("[for az in sort(keys(var.subnet_ids)) : var.subnet_ids[az]]", self.main)

    def test_sg_alb_reutilizado_de_i010(self) -> None:
        self.assertEqual(
            atributos(self.recursos[("aws_lb", "principal")]).get("security_groups"),
            "[var.sg_alb_id]",
        )
        self.assertNotIn("aws_security_group", {tipo for tipo, _ in self.recursos})
        env = _leer(ENV / "main.tf")
        self.assertIn('sg_alb_id     = module.red.security_groups["alb"]', env)
        self.assertIn("var.sg_alb_id != var.sg_backend_id", self.main)


class RedDeI010Compatible(unittest.TestCase):
    """El ALB depende de reglas que pertenecen a I-010: deben seguir existiendo."""

    @classmethod
    def setUpClass(cls) -> None:
        cls.red = _leer(RED / "main.tf")
        cls.recursos_red = recursos(cls.red)

    def test_ingress_80_desde_prefix_list_cloudfront(self) -> None:
        self.assertIn('name = "com.amazonaws.global.cloudfront.origin-facing"', self.red)
        cuerpo = self.recursos_red[("aws_vpc_security_group_ingress_rule", "alb_cloudfront_80")]
        atributos_regla = atributos(cuerpo)
        self.assertEqual(atributos_regla.get("from_port"), "80")
        self.assertEqual(atributos_regla.get("to_port"), "80")
        self.assertIn("prefix_list_id", atributos_regla)
        self.assertNotIn("cidr_ipv4", atributos_regla)

    def test_egress_alb_al_backend_8080(self) -> None:
        cuerpo = self.recursos_red[("aws_vpc_security_group_egress_rule", "alb_backend_8080")]
        self.assertIn("aws_security_group.backend.id", atributos(cuerpo).get("referenced_security_group_id", ""))

    def test_igw_sin_rutas_publicas(self) -> None:
        self.assertIn(("aws_internet_gateway", "principal"), self.recursos_red)
        self.assertNotIn("gateway_id = aws_internet_gateway.principal.id", self.red)

    def test_nacl_egress_efimeros_para_vpc_origins(self) -> None:
        """VPC origins no evalúa ingress NACL, pero sí el egress de retorno."""
        self.assertIn(("aws_network_acl_rule", "egress_efimeros"), self.recursos_red)
        self.assertIn("from_port      = 1024", self.red)
        self.assertIn("to_port        = 65535", self.red)


class CloudFront(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.main = _leer(MODULO / "main.tf")
        cls.vars = _leer(MODULO / "variables.tf")
        cls.outs = _leer(MODULO / "outputs.tf")
        cls.recursos = recursos(cls.main)

    def _origin(self) -> str:
        distribucion = self.recursos[("aws_cloudfront_distribution", "api")]
        return bloque(distribucion, "origin")

    def test_domain_name_es_el_dns_del_alb(self) -> None:
        """`DomainName` debe ser un nombre DNS del origen, no el ARN del VPC origin."""
        origen = self._origin()
        self.assertEqual(atributos(origen).get("domain_name"), "aws_lb.principal.dns_name")

    def test_arn_del_vpc_origin_no_se_usa_como_dns(self) -> None:
        origen = self._origin()
        self.assertNotIn("aws_cloudfront_vpc_origin.alb.arn", origen)
        self.assertNotIn("arn", atributos(origen).get("domain_name", ""))
        # Tampoco en ningún otro `domain_name` del módulo.
        for valor in re.findall(r"domain_name\s*=\s*(.+)", _sin_comentarios(self.main)):
            self.assertNotIn(".arn", valor)

    def test_vpc_origin_config_referencia_el_id(self) -> None:
        config = bloque(self._origin(), "vpc_origin_config")
        self.assertEqual(atributos(config).get("vpc_origin_id"), "aws_cloudfront_vpc_origin.alb.id")
        self.assertNotIn("vpc_origin_id = aws_lb.principal.arn", self.main)

    def test_vpc_origin_http_only_hacia_el_alb(self) -> None:
        cuerpo = self.recursos[("aws_cloudfront_vpc_origin", "alb")]
        config = atributos(bloque(cuerpo, "vpc_origin_endpoint_config"))
        self.assertEqual(config.get("origin_protocol_policy"), '"http-only"')
        self.assertEqual(config.get("arn"), "aws_lb.principal.arn")
        self.assertEqual(config.get("http_port"), "var.puerto_alb")

    def test_https_publico_y_redireccion(self) -> None:
        distribucion = self.recursos[("aws_cloudfront_distribution", "api")]
        cache = atributos(bloque(distribucion, "default_cache_behavior"))
        self.assertEqual(cache.get("viewer_protocol_policy"), '"redirect-to-https"')
        certificado = atributos(bloque(distribucion, "viewer_certificate"))
        self.assertEqual(certificado.get("cloudfront_default_certificate"), "true")
        self.assertEqual(atributos(distribucion).get("enabled"), "true")
        self.assertNotIn('viewer_protocol_policy = "allow-all"', self.main)

    def test_sin_cache_para_api_financiera(self) -> None:
        self.assertIn('name = "Managed-CachingDisabled"', self.main)
        self.assertIn('name = "Managed-AllViewer"', self.main)
        cache = atributos(bloque(self.recursos[("aws_cloudfront_distribution", "api")], "default_cache_behavior"))
        self.assertEqual(cache.get("cache_policy_id"), "data.aws_cloudfront_cache_policy.sin_cache.id")
        self.assertEqual(cache.get("origin_request_policy_id"), "data.aws_cloudfront_origin_request_policy.api.id")
        self.assertNotIn("forwarded_values", self.main)
        self.assertNotIn("CacheEverything", self.main)

    def test_metodos_rest_completos(self) -> None:
        cache = bloque(self.recursos[("aws_cloudfront_distribution", "api")], "default_cache_behavior")
        permitidos = atributos(cache).get("allowed_methods", "")
        for metodo in ("GET", "HEAD", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"):
            self.assertIn(f'"{metodo}"', permitidos)
        self.assertNotIn("allowed_methods = [\"GET\", \"HEAD\"]", cache)

    def test_sin_acm_sin_route53_sin_listener_443(self) -> None:
        tipos = {tipo for tipo, _ in self.recursos}
        for prohibido in ("aws_acm_certificate", "aws_certificatemanager_certificate"):
            self.assertNotIn(prohibido, tipos)
        self.assertNotIn("route53", self.main.lower())
        self.assertNotIn("minimum_protocol_version", self.main)
        self.assertNotIn("acm_certificate_arn", self.main)

    def test_certificado_por_defecto_sin_sobreafirmar_tls(self) -> None:
        """AWS fija TLSv1 con el certificado por defecto: no se puede prometer TLS 1.2."""
        self.assertIn("TLSv1 (fijada por AWS con certificado por defecto, no configurable)", self.outs)
        self.assertIn("tls_app_hasta_alb       = false", self.outs)

    def test_sin_waf_ni_logging_de_pago(self) -> None:
        self.assertNotIn("web_acl_id", self.main)
        self.assertNotIn("logging_config", self.main)

    def test_price_class_barata_por_defecto(self) -> None:
        self.assertIn('default     = "PriceClass_100"', self.vars)

    def test_estado_no_se_declara_desplegado(self) -> None:
        self.assertIn('value       = "NOT_RUN_AWS"', self.outs)


class ArquitecturaD2Intacta(unittest.TestCase):
    """Invariantes de la decisión D2 ratificada que no deben cambiar."""

    @classmethod
    def setUpClass(cls) -> None:
        cls.main = _leer(MODULO / "main.tf")
        cls.recursos = recursos(cls.main)

    def test_un_alb_un_vpc_origin_una_distribucion(self) -> None:
        self.assertEqual(sum(1 for t, _ in self.recursos if t == "aws_lb"), 1)
        self.assertEqual(sum(1 for t, _ in self.recursos if t == "aws_cloudfront_vpc_origin"), 1)
        self.assertEqual(sum(1 for t, _ in self.recursos if t == "aws_cloudfront_distribution"), 1)

    def test_region_es_us_east_2(self) -> None:
        self.assertIn('var.region == "us-east-2"', self.main)

    def test_sin_recursos_de_red_adicionales(self) -> None:
        for prohibido in (
            "aws_nat_gateway",
            "aws_eip",
            "aws_vpc_endpoint",
            "aws_subnet",
            "aws_vpc",
            "aws_route",
        ):
            self.assertNotIn(prohibido, {tipo for tipo, _ in self.recursos})


class ComposicionDelEntorno(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.env = _leer(ENV / "main.tf")
        cls.env_vars = _leer(ENV / "variables.tf")

    def test_modulo_balanceo_compuesto(self) -> None:
        self.assertIn('module "balanceo"', self.env)
        self.assertIn('source = "../../modules/balanceo"', self.env)
        self.assertIn("subnet_ids    = module.red.subredes_privadas", self.env)
        self.assertIn("asg_name      = module.computo.asg_name", self.env)
        self.assertIn("vpc_id   = module.red.vpc_id", self.env)

    def test_provider_soporta_vpc_origin(self) -> None:
        self.assertIn('version = "~> 5.72"', self.env)
        self.assertNotIn('version = "~> 5.60"', self.env)

    def test_salidas_clave(self) -> None:
        for salida in (
            "alb_es_interno",
            "cloudfront_domain",
            "vpc_origin_id",
            "health_check",
            "estado_despliegue",
        ):
            self.assertIn(salida, self.env)

    def test_sin_recursos_prohibidos_en_el_repo_terraform(self) -> None:
        tipos = set()
        for archivo in DEFINICIONES.rglob("*.tf"):
            tipos |= {tipo for tipo, _ in recursos(_leer(archivo))}
        for prohibido in (
            "aws_acm_certificate",
            "aws_certificatemanager_certificate",
            "aws_route53_zone",
            "aws_route53_record",
            "aws_wafv2_web_acl",
            "aws_nat_gateway",
            "aws_eip",
        ):
            self.assertNotIn(prohibido, tipos)


if __name__ == "__main__":
    unittest.main()
