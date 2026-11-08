"""Pruebas de las verificaciones de run de CI y entorno protegido (I-090)."""

from __future__ import annotations

import json
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

RAIZ = Path(__file__).resolve().parents[2]
RUN = RAIZ / "herramientas" / "verificar_run_ci.py"
ENTORNO = RAIZ / "herramientas" / "verificar_entorno_protegido.py"
SHA = "a" * 40


def _json(datos: dict) -> Path:
    carpeta = Path(tempfile.mkdtemp(prefix="i090-"))
    ruta = carpeta / "datos.json"
    ruta.write_text(json.dumps(datos), encoding="utf-8")
    return ruta


class RunCI(unittest.TestCase):
    def _run(self, datos: dict, sha: str = SHA) -> subprocess.CompletedProcess:
        return subprocess.run(
            [sys.executable, str(RUN), str(_json(datos)), sha],
            capture_output=True, text=True, timeout=60,
        )

    def _valido(self, **cambios: object) -> dict:
        datos = {
            "status": "completed",
            "conclusion": "success",
            "head_sha": SHA,
            "head_branch": "main",
            "repository": {"full_name": "nexumpr/Nexum-Backend"},
        }
        datos.update(cambios)
        return datos

    def test_run_verde_pasa(self) -> None:
        r = self._run(self._valido())
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertIn("RUN_CI_PASS", r.stdout)

    def test_run_fallido_se_rechaza(self) -> None:
        r = self._run(self._valido(conclusion="failure"))
        self.assertEqual(r.returncode, 1)
        self.assertIn("RUN_NO_VERDE", r.stdout)

    def test_run_de_otro_sha_se_rechaza(self) -> None:
        r = self._run(self._valido(head_sha="b" * 40))
        self.assertEqual(r.returncode, 1)
        self.assertIn("RUN_SHA_DISTINTO", r.stdout)

    def test_run_de_otra_rama_se_rechaza(self) -> None:
        r = self._run(self._valido(head_branch="feature/x"))
        self.assertEqual(r.returncode, 1)
        self.assertIn("RUN_RAMA_NO_AUTORIZADA", r.stdout)

    def test_run_de_otro_repositorio_se_rechaza(self) -> None:
        r = self._run(self._valido(repository={"full_name": "otro/repo"}))
        self.assertEqual(r.returncode, 1)
        self.assertIn("RUN_REPOSITORIO_DISTINTO", r.stdout)

    def test_json_inexistente_falla(self) -> None:
        r = subprocess.run([sys.executable, str(RUN), "no-existe.json", SHA], capture_output=True, text=True, timeout=60)
        self.assertEqual(r.returncode, 1)


class EntornoProtegido(unittest.TestCase):
    def _run(self, datos: dict) -> subprocess.CompletedProcess:
        return subprocess.run(
            [sys.executable, str(ENTORNO), str(_json(datos))],
            capture_output=True, text=True, timeout=60,
        )

    def test_con_revisores_pasa(self) -> None:
        datos = {
            "name": "aws-deploy",
            "protection_rules": [
                {"type": "required_reviewers", "reviewers": [{"type": "User", "reviewer": {"login": "responsable"}}]}
            ],
        }
        r = self._run(datos)
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)

    def test_sin_revisores_falla(self) -> None:
        r = self._run({"name": "aws-deploy", "protection_rules": []})
        self.assertEqual(r.returncode, 1)
        self.assertIn("SIN_REVISORES_OBLIGATORIOS", r.stdout)

    def test_revisores_vacios_falla(self) -> None:
        r = self._run({"name": "aws-deploy", "protection_rules": [{"type": "required_reviewers", "reviewers": []}]})
        self.assertEqual(r.returncode, 1)
        self.assertIn("SIN_REVISORES_OBLIGATORIOS", r.stdout)

    def test_entorno_distinto_falla(self) -> None:
        r = self._run({"name": "produccion", "protection_rules": [{"type": "required_reviewers", "reviewers": [{"x": 1}]}]})
        self.assertEqual(r.returncode, 1)
        self.assertIn("ENTORNO_DISTINTO", r.stdout)


if __name__ == "__main__":
    unittest.main()
