"""Pruebas del catálogo de consultas AWS de sólo lectura (revisión 2).

Cubren los defectos corregidos:

- ``servicequotas`` no existe en AWS CLI: el servicio es ``service-quotas``.
- El comando no puede llevar dos ``--region`` (antes se emitía us-east-2 y
  us-east-1 a la vez) ni repetir ``--filters``.
- El código de Price List de Elastic Load Balancing es ``AWSELB``.
- El directorio temporal no puede ser la cadena literal ``$env:TEMP`` ni el
  repositorio.
"""

from __future__ import annotations

import json
import sys
import unittest
from pathlib import Path

RAIZ = Path(__file__).resolve().parents[3]
sys.path.insert(0, str(RAIZ / "infra" / "finops"))

import consultas_readonly as cr  # noqa: E402


class CatalogoReadonly(unittest.TestCase):
    def test_catalogo_solo_lectura(self) -> None:
        self.assertEqual(cr.validar_catalogo(), [])

    def test_ninguna_consulta_muta(self) -> None:
        for consulta in cr.CATALOGO:
            self.assertTrue(
                consulta.operacion.startswith(cr.VERBOS_LECTURA),
                f"{consulta.id} usa {consulta.operacion}",
            )
            for arg in consulta.argumentos:
                self.assertFalse(arg.startswith(cr.VERBOS_PROHIBIDOS), f"{consulta.id}: {arg}")

    def test_cost_explorer_marcado_facturable(self) -> None:
        ce = cr.catalogo_por_id()["costo_explorer"]
        self.assertEqual(ce.clasificacion, "FACTURABLE_POR_SOLICITUD")
        self.assertTrue(ce.requiere_aprobacion_especifica)
        self.assertEqual(ce.costo_estimado_usd, "0.01")

    def test_cost_explorer_no_se_ejecuta_sin_aprobacion(self) -> None:
        ce = cr.catalogo_por_id()["costo_explorer"]
        with __import__("tempfile").TemporaryDirectory() as tmp:
            resultado = cr.ejecutar(
                ce, "nexum", "us-east-2", Path(tmp), Path(tmp), account_id="000000000000"
            )
        self.assertEqual(resultado["estado"], "NOT_RUN")
        self.assertIn("facturable", resultado["motivo"])

    def test_consulta_que_requiere_cuenta_falla_sin_cuenta(self) -> None:
        presupuestos = cr.catalogo_por_id()["presupuestos"]
        with __import__("tempfile").TemporaryDirectory() as tmp:
            resultado = cr.ejecutar(presupuestos, "nexum", "us-east-2", Path(tmp), Path(tmp))
        self.assertEqual(resultado["estado"], "NOT_RUN")
        self.assertIn("account id", resultado["motivo"])

    def test_describir_no_expone_cuenta_ni_perfil(self) -> None:
        texto = json.dumps([cr.describir(c) for c in cr.CATALOGO])
        self.assertNotRegex(texto, r"\b\d{12}\b")
        self.assertIn("{profile}", texto)

    def test_todas_las_consultas_declaran_costo(self) -> None:
        for consulta in cr.CATALOGO:
            self.assertTrue(consulta.clasificacion, consulta.id)
            self.assertTrue(consulta.fuente_costo, consulta.id)
            self.assertTrue(consulta.justificacion_costo, consulta.id)

    def test_ids_unicos(self) -> None:
        ids = [c.id for c in cr.CATALOGO]
        self.assertEqual(len(ids), len(set(ids)))


class ComandoExacto(unittest.TestCase):
    """Inspecciona argv, no sólo el verbo."""

    def _comando(self, consulta_id: str) -> list[str]:
        return cr.catalogo_por_id()[consulta_id].comando("nexum", "us-east-2")

    def test_un_solo_region_en_todas_las_consultas(self) -> None:
        for consulta in cr.CATALOGO:
            comando = consulta.comando("nexum", "us-east-2")
            self.assertEqual(
                comando.count("--region"), 1, f"{consulta.id} repite --region: {comando}"
            )

    def test_pricing_usa_endpoint_us_east_1(self) -> None:
        for consulta in cr.CATALOGO:
            if not consulta.es_pricing:
                continue
            comando = consulta.comando("nexum", "us-east-2")
            indice = comando.index("--region")
            self.assertEqual(comando[indice + 1], "us-east-1", consulta.id)
            self.assertNotIn("us-east-2", comando[indice + 1 : indice + 2])

    def test_pricing_usa_un_unico_filters(self) -> None:
        for consulta in cr.CATALOGO:
            if not consulta.es_pricing:
                continue
            comando = consulta.comando("nexum", "us-east-2")
            self.assertEqual(
                comando.count("--filters"), 1, f"{consulta.id}: {comando}"
            )
            self.assertTrue(consulta.filtros, consulta.id)

    def test_filtros_bien_formados(self) -> None:
        for consulta in cr.CATALOGO:
            if not consulta.es_pricing:
                continue
            comando = consulta.comando("nexum", "us-east-2")
            indice = comando.index("--filters")
            tokens = []
            for token in comando[indice + 1 :]:
                if token.startswith("--"):
                    break
                tokens.append(token)
            self.assertEqual(len(tokens), len(consulta.filtros), consulta.id)
            for token in tokens:
                self.assertTrue(token.startswith("Type=TERM_MATCH,Field="), token)
                self.assertIn(",Value=", token)

    def test_sin_campos_de_filtro_duplicados(self) -> None:
        for consulta in cr.CATALOGO:
            campos = [f.campo for f in consulta.filtros]
            self.assertEqual(len(campos), len(set(campos)), consulta.id)

    def test_precios_regionales_anclados_a_ohio(self) -> None:
        for consulta in cr.CATALOGO:
            if not consulta.es_pricing or consulta.region_datos != "us-east-2":
                continue
            pares = {(f.campo, f.valor) for f in consulta.filtros}
            anclado = ("location", "US East (Ohio)") in pares or any(
                campo == "usagetype" and valor.startswith("USE2-") for campo, valor in pares
            )
            self.assertTrue(anclado, f"{consulta.id} no está anclado a us-east-2")

    def test_cloudfront_es_global_sin_region_comercial(self) -> None:
        consulta = cr.catalogo_por_id()["precio_cloudfront_egreso"]
        self.assertEqual(consulta.region_datos, "global")
        self.assertNotIn("location", {f.campo for f in consulta.filtros})


class CodigosDeServicio(unittest.TestCase):
    def test_elb_usa_awselb(self) -> None:
        consultas = [c for c in cr.CATALOGO if c.codigo_servicio]
        elb = [c for c in consultas if "ELB" in (c.codigo_servicio or "")]
        self.assertTrue(elb)
        for consulta in elb:
            self.assertEqual(consulta.codigo_servicio, "AWSELB")
        self.assertNotIn("ElasticLoadBalancing", {c.codigo_servicio for c in consultas})

    def test_todos_los_codigos_estan_validados(self) -> None:
        for consulta in cr.CATALOGO:
            if consulta.es_pricing:
                self.assertIn(consulta.codigo_servicio, cr.CODIGOS_SERVICIO_VALIDADOS)

    def test_service_quotas_no_usa_el_nombre_invalido(self) -> None:
        comando = cr.catalogo_por_id()["cuota_ec2_vcpu"].comando("nexum", "us-east-2")
        self.assertEqual(comando[1], "service-quotas")
        self.assertNotIn("servicequotas", comando)

    def test_ninguna_consulta_usa_el_alias_invalido(self) -> None:
        for consulta in cr.CATALOGO:
            self.assertNotEqual(consulta.servicio_api, "servicequotas", consulta.id)


class RutasTemporales(unittest.TestCase):
    def test_directorio_crudo_no_es_literal_de_powershell(self) -> None:
        ruta = str(cr.CRUDO_POR_DEFECTO)
        self.assertNotIn("$env", ruta)
        self.assertNotIn("%TEMP%", ruta)
        self.assertTrue(Path(ruta).is_absolute(), ruta)

    def test_directorio_crudo_fuera_del_repositorio(self) -> None:
        ruta = Path(cr.CRUDO_POR_DEFECTO).resolve()
        repo = RAIZ.resolve()
        self.assertFalse(
            str(ruta).lower().startswith(str(repo).lower()),
            f"el volcado crudo no puede vivir en el repositorio: {ruta}",
        )

    def test_directorio_crudo_existe(self) -> None:
        self.assertTrue(Path(cr.CRUDO_POR_DEFECTO).is_dir())

    def test_funcion_de_directorio_reproduce_la_anterior(self) -> None:
        self.assertEqual(
            Path(cr.directorio_crudo_por_defecto()).resolve(),
            Path(cr.CRUDO_POR_DEFECTO).resolve(),
        )


if __name__ == "__main__":
    unittest.main()
