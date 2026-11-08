"""Pruebas de la verificación de la prueba negativa del escáner IaC (I-080)."""

from __future__ import annotations

import json
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

SCRIPT = Path(__file__).resolve().parents[2] / "herramientas" / "verificar_fixture_iac.py"


def _informe(hallazgos: list[tuple[str, str]], objetivo: str = "tests/fixtures/iac/inseguro/main.tf") -> str:
    return json.dumps(
        {
            "Results": [
                {
                    "Target": objetivo,
                    "Misconfigurations": [
                        {"ID": identificador, "Severity": severidad, "Title": "fixture"}
                        for identificador, severidad in hallazgos
                    ],
                }
            ]
        }
    )


class VerificarFixtureIaC(unittest.TestCase):
    def _ruta(self, contenido: str) -> Path:
        carpeta = Path(tempfile.mkdtemp(prefix="iac-i080-"))
        ruta = carpeta / "trivy.json"
        ruta.write_text(contenido, encoding="utf-8")
        return ruta

    def _ejecutar(self, ruta: Path, codigo: str) -> subprocess.CompletedProcess:
        return subprocess.run(
            [sys.executable, str(SCRIPT), str(ruta), codigo],
            capture_output=True,
            text=True,
            timeout=60,
        )

    def test_detecta_high_con_exit_1(self) -> None:
        r = self._ejecutar(self._ruta(_informe([("AWS-0086", "HIGH")])), "1")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertIn("FIXTURE_IAC_PASS", r.stdout)
        self.assertIn("AWS-0086", r.stdout)

    def test_detecta_critical_con_exit_1(self) -> None:
        r = self._ejecutar(self._ruta(_informe([("AWS-0054", "CRITICAL")])), "1")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)

    def test_exit_0_no_se_acepta_como_deteccion(self) -> None:
        r = self._ejecutar(self._ruta(_informe([("AWS-0086", "HIGH")])), "0")
        self.assertEqual(r.returncode, 1)
        self.assertIn("se esperaba exit 1", r.stdout)

    def test_solo_low_no_cuenta_como_deteccion_bloqueante(self) -> None:
        r = self._ejecutar(self._ruta(_informe([("AWS-0001", "LOW"), ("AWS-0002", "MEDIUM")])), "1")
        self.assertEqual(r.returncode, 1)
        self.assertIn("no hay hallazgos HIGH/CRITICAL", r.stdout)

    def test_informe_inexistente_falla(self) -> None:
        r = self._ejecutar(Path(tempfile.mkdtemp(prefix="iac-i080-")) / "no.json", "1")
        self.assertEqual(r.returncode, 1)
        self.assertIn("no existe el informe", r.stdout)

    def test_informe_invalido_falla(self) -> None:
        r = self._ejecutar(self._ruta("{no es json"), "1")
        self.assertEqual(r.returncode, 1)
        self.assertIn("informe invalido", r.stdout)

    def test_codigo_no_numerico_falla(self) -> None:
        r = self._ejecutar(self._ruta(_informe([("AWS-0086", "HIGH")])), "uno")
        self.assertEqual(r.returncode, 1)
        self.assertIn("no numerico", r.stdout)

    def test_uso_incorrecto_devuelve_2(self) -> None:
        r = subprocess.run([sys.executable, str(SCRIPT)], capture_output=True, text=True, timeout=60)
        self.assertEqual(r.returncode, 2)


if __name__ == "__main__":
    unittest.main()
