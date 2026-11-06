"""Pruebas del catálogo de consultas AWS de sólo lectura."""

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
        resultado = cr.ejecutar(
            ce,
            "nexum",
            "us-east-2",
            Path("."),
            Path("."),
            account_id="000000000000",
            aprobar_facturable=False,
        )
        self.assertEqual(resultado["estado"], "NOT_RUN")
        self.assertIn("facturable", resultado["motivo"])

    def test_consulta_que_requiere_cuenta_falla_sin_cuenta(self) -> None:
        presupuestos = cr.catalogo_por_id()["presupuestos"]
        resultado = cr.ejecutar(
            presupuestos, "nexum", "us-east-2", Path("."), Path("."), account_id=None
        )
        self.assertEqual(resultado["estado"], "NOT_RUN")
        self.assertIn("account id", resultado["motivo"])

    def test_comando_no_expone_perfil_ni_cuenta_en_el_repositorio(self) -> None:
        describir = [cr.describir(c) for c in cr.CATALOGO]
        texto = json.dumps(describir)
        self.assertNotRegex(texto, r"\b\d{12}\b", "el catalogo no debe contener un id de cuenta")
        self.assertIn("{profile}", texto)

    def test_todas_las_consultas_declaran_costo(self) -> None:
        for consulta in cr.CATALOGO:
            self.assertTrue(consulta.clasificacion, consulta.id)
            self.assertTrue(consulta.fuente_costo, consulta.id)
            self.assertTrue(consulta.justificacion_costo, consulta.id)

    def test_ids_unicos(self) -> None:
        ids = [c.id for c in cr.CATALOGO]
        self.assertEqual(len(ids), len(set(ids)))


if __name__ == "__main__":
    unittest.main()
