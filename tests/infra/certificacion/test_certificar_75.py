"""Pruebas del harness de certificación: inventario y cobertura verificables, sin red."""

from __future__ import annotations

import contextlib
import io
import json
import sys
import unittest
from pathlib import Path

RAIZ = Path(__file__).resolve().parents[3]
sys.path.insert(0, str(RAIZ / "herramientas" / "certificacion"))

import certificar_75 as cli  # noqa: E402

OPENAPI = RAIZ / "sdd" / "03-endpoints" / "openapi" / "nexum-api-v1.json"


class Inventario(unittest.TestCase):
    def test_exactamente_75_operaciones(self) -> None:
        operaciones = cli.inventario(OPENAPI)
        self.assertEqual(75, len(operaciones))
        self.assertTrue(all(eid.startswith("E") for eid in operaciones))

    def test_metodo_y_ruta_presentes(self) -> None:
        operaciones = cli.inventario(OPENAPI)
        op = operaciones["E002"]
        self.assertEqual("POST", op.metodo)
        self.assertEqual("/api/v1/auth/login", op.ruta)


class Cobertura(unittest.TestCase):
    def _correr(self, *args: str) -> tuple[int, dict]:
        buffer = io.StringIO()
        with contextlib.redirect_stdout(buffer):
            rc = cli.main(list(args))
        return rc, json.loads(buffer.getvalue())

    def test_dry_run_exige_cobertura_exacta(self) -> None:
        rc, resumen = self._correr("--openapi", str(OPENAPI))
        self.assertIn(rc, (0, 2))
        self.assertEqual(75, resumen["operaciones_openapi"])
        # Con cobertura incompleta el RC debe ser 2, nunca 0.
        if not resumen["cobertura_exacta"]:
            self.assertEqual(2, rc)
            self.assertTrue(resumen["problemas"])

    def test_escenario_faltante_falla_cerrado(self) -> None:
        operaciones = cli.inventario(OPENAPI)
        class Falso:
            ESCENARIOS = {}
            ORDEN_EJECUCION = ()
        exacta, problemas = cli.cobertura(operaciones, Falso)
        self.assertFalse(exacta)
        self.assertTrue(any(p.startswith("FALTA") for p in problemas))

    def test_run_sin_autorizacion_no_ejecuta(self) -> None:
        rc, resumen = self._correr("--openapi", str(OPENAPI), "--run")
        self.assertEqual(2, rc)
        self.assertEqual("MUTACIONES_NO_AUTORIZADAS", resumen["estado"])
        self.assertFalse(resumen["ejecutado_contra_produccion"])

    def test_salida_no_expone_secretos(self) -> None:
        _, resumen = self._correr("--openapi", str(OPENAPI))
        texto = json.dumps(resumen)
        for prohibido in ("Bearer ", "password", "refreshToken", "accessToken", "token="):
            self.assertNotIn(prohibido, texto)

    def test_rc_distintos(self) -> None:
        self.assertEqual((0, 1, 2), (cli.RC_OK, cli.RC_FALLO, cli.RC_INCOMPLETA))


if __name__ == "__main__":
    unittest.main()
