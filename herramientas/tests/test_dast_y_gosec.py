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


def _control(evidencia: str | None) -> Path:
    raiz = Path(tempfile.mkdtemp(prefix="gate-dast-"))
    if evidencia is not None:
        (raiz / "GATE_DAST.md").write_text(evidencia, encoding="utf-8")
    return raiz


EVIDENCIA = """# Gate DAST
Estado: APRUEBO_GATE_DAST
Objetivo: 127.0.0.1
Caduca el **2026-12-31**
"""


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
    def _run(self, evidencia: str | None = EVIDENCIA, **cambios: str) -> subprocess.CompletedProcess:
        control = _control(evidencia)
        valores = {
            "--objetivo": "http://127.0.0.1:8080",
            "--autorizacion": "AUTORIZO_DAST_127.0.0.1",
            "--datos-sinteticos": "SI",
            "--ventana-inicio": "2026-11-10T10:00:00",
            "--ventana-fin": "2026-11-10T12:00:00",
            "--rps": "2",
            "--alcance": "/api/v1/health,/api/v1/auth",
            "--raiz-control": str(control),
            "--fecha": "2026-11-08",
        }
        if evidencia is not None:
            valores["--evidencia-autorizacion"] = str(control / "GATE_DAST.md")
        valores.update(cambios)
        argumentos = [sys.executable, str(DAST)]
        for clave, valor in valores.items():
            argumentos += [clave, valor]
        return subprocess.run(argumentos, capture_output=True, text=True, timeout=60)

    def test_plan_autorizado_se_prepara_sin_ejecutar(self) -> None:
        salida = Path(tempfile.mkdtemp(prefix="dast-")) / "plan.json"
        r = self._run(**{"--salida": str(salida)})
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertIn("DAST_PLAN_PASS", r.stdout)
        plan = json.loads(salida.read_text(encoding="utf-8"))
        self.assertEqual(plan["estado"], "PLAN_AUTORIZADO_NO_EJECUTADO")
        self.assertEqual(plan["datos"], "sinteticos")

    def test_sin_evidencia_queda_bloqueado(self) -> None:
        r = self._run(evidencia=None)
        self.assertEqual(r.returncode, 1)
        self.assertIn("DAST_BLOCKED_AUTHORIZATION", r.stdout)

    def test_evidencia_fuera_de_control_se_rechaza(self) -> None:
        fuera = _archivo(EVIDENCIA)
        r = self._run(**{"--evidencia-autorizacion": str(fuera)})
        self.assertEqual(r.returncode, 1)
        self.assertIn("EVIDENCIA_FUERA_DE_CONTROL", r.stdout)

    def test_evidencia_caducada_se_rechaza(self) -> None:
        r = self._run(**{"--fecha": "2027-01-01"})
        self.assertEqual(r.returncode, 1)
        self.assertIn("EVIDENCIA_CADUCADA", r.stdout)

    def test_evidencia_sin_frase_se_rechaza(self) -> None:
        r = self._run(evidencia="Objetivo: 127.0.0.1\nCaduca el **2026-12-31**\n")
        self.assertEqual(r.returncode, 1)
        self.assertIn("EVIDENCIA_SIN_FRASE_GATE", r.stdout)

    def test_objetivo_publico_falla(self) -> None:
        evidencia = "Estado: APRUEBO_GATE_DAST\nObjetivo: ejemplo.com\nCaduca el **2026-12-31**\n"
        r = self._run(evidencia=evidencia, **{"--objetivo": "https://ejemplo.com", "--autorizacion": "AUTORIZO_DAST_ejemplo.com"})
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
