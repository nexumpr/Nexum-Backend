"""Pruebas de la prueba negativa de gosec y del harness DAST (I-100)."""

from __future__ import annotations

import json
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

RAIZ = Path(__file__).resolve().parents[2] / "herramientas"
GOSEC = RAIZ / "verificar_fixture_gosec.py"
DAST = RAIZ / "preparar_dast.py"


def _archivo(texto: str) -> Path:
    ruta = Path(tempfile.mkdtemp(prefix="i100-")) / "salida.txt"
    ruta.write_text(texto, encoding="utf-8")
    return ruta


class FixtureGosec(unittest.TestCase):
    def _run(self, codigo: int, texto: str) -> subprocess.CompletedProcess:
        return subprocess.run(
            [sys.executable, str(GOSEC), str(_archivo(texto)), str(codigo)],
            capture_output=True, text=True, timeout=60,
        )

    def test_detecta_g404_con_fallo(self) -> None:
        r = self._run(1, "G404: Use of weak random number generator")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertIn("GOSEC_FIXTURE_PASS", r.stdout)

    def test_codigo_cero_no_es_deteccion(self) -> None:
        r = self._run(0, "G404")
        self.assertEqual(r.returncode, 1)
        self.assertIn("no falló", r.stdout)

    def test_sin_regla_no_es_deteccion(self) -> None:
        r = self._run(1, "error de compilación")
        self.assertEqual(r.returncode, 1)
        self.assertIn("no aparece la regla", r.stdout)


class HarnessDAST(unittest.TestCase):
    def _run(self, **cambios: str) -> subprocess.CompletedProcess:
        valores = {
            "--objetivo": "http://127.0.0.1:8080",
            "--autorizacion": "AUTORIZO_DAST_127.0.0.1",
            "--datos-sinteticos": "SI",
            "--ventana-inicio": "2026-11-10T10:00:00",
            "--ventana-fin": "2026-11-10T12:00:00",
            "--rps": "2",
            "--alcance": "/api/v1/health,/api/v1/auth",
            "--gate": "APRUEBO_GATE_DAST",
        }
        valores.update(cambios)
        argumentos = [sys.executable, str(DAST)]
        for clave, valor in valores.items():
            argumentos += [clave, valor]
        return subprocess.run(argumentos, capture_output=True, text=True, timeout=60)

    def test_plan_valido_se_prepara_sin_ejecutar(self) -> None:
        salida = Path(tempfile.mkdtemp(prefix="dast-")) / "plan.json"
        r = self._run(**{"--salida": str(salida)})
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertIn("DAST_PLAN_PASS", r.stdout)
        plan = json.loads(salida.read_text(encoding="utf-8"))
        self.assertEqual(plan["estado"], "PLAN_PREPARADO_NO_EJECUTADO")
        self.assertEqual(plan["datos"], "sinteticos")

    def test_sin_gate_queda_bloqueado(self) -> None:
        r = self._run(**{"--gate": ""})
        self.assertEqual(r.returncode, 1)
        self.assertIn("DAST_BLOCKED_AUTHORIZATION", r.stdout)

    def test_objetivo_publico_sin_permiso_falla(self) -> None:
        r = self._run(**{"--objetivo": "https://ejemplo.com", "--autorizacion": "AUTORIZO_DAST_ejemplo.com"})
        self.assertEqual(r.returncode, 1)
        self.assertIn("OBJETIVO_NO_AISLADO", r.stdout)

    def test_datos_no_sinteticos_fallan(self) -> None:
        r = self._run(**{"--datos-sinteticos": "NO"})
        self.assertEqual(r.returncode, 1)
        self.assertIn("DATOS_NO_SINTETICOS", r.stdout)

    def test_rps_excesivo_falla(self) -> None:
        r = self._run(**{"--rps": "50"})
        self.assertEqual(r.returncode, 1)
        self.assertIn("RPS_FUERA_DE_LIMITE", r.stdout)

    def test_ventana_excesiva_falla(self) -> None:
        r = self._run(**{"--ventana-fin": "2026-11-12T12:00:00"})
        self.assertEqual(r.returncode, 1)
        self.assertIn("VENTANA_EXCESIVA", r.stdout)

    def test_autorizacion_de_otro_host_falla(self) -> None:
        r = self._run(**{"--autorizacion": "AUTORIZO_DAST_otro.local"})
        self.assertEqual(r.returncode, 1)
        self.assertIn("AUTORIZACION_NO_CORRESPONDE_AL_OBJETIVO", r.stdout)


if __name__ == "__main__":
    unittest.main()
