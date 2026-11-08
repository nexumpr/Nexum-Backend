"""Pruebas de la comprobación de vigencia de RFC-006 (I-080)."""

from __future__ import annotations

import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

SCRIPT = Path(__file__).resolve().parents[2] / "herramientas" / "verificar_vigencia_rfc006.py"

RFC_OK = """# RFC-006
| Estado | **APROBADA (2026-11-08)** - aceptacion temporal |
- Caduca el **2026-12-02** (fecha de evaluacion) o en el Gate de despliegue AWS.
"""

IGNORE_OK = """misconfigurations:
  - id: AWS-0011
    paths:
      - "modules/balanceo/main.tf"
    expired_at: 2026-12-02T00:00:00Z
    statement: >-
      RFC-006 aprobada con APRUEBO_RFC_006_WAF_TEMPORAL.
"""


class VigenciaRFC006(unittest.TestCase):
    def _raiz(self, rfc: str | None = RFC_OK, ignore: str | None = IGNORE_OK, gates: dict[str, str] | None = None) -> Path:
        raiz = Path(tempfile.mkdtemp(prefix="rfc006-"))
        control = raiz / "sdd" / "_control" / "H08"
        control.mkdir(parents=True)
        if rfc is not None:
            (control / "RFC-006-EXCEPCION_WAF_I030.md").write_text(rfc, encoding="utf-8")
        if ignore is not None:
            (raiz / ".trivyignore.yaml").write_text(ignore, encoding="utf-8")
        for nombre, contenido in (gates or {}).items():
            (control / nombre).write_text(contenido, encoding="utf-8")
        return raiz

    def _ejecutar(self, raiz: Path, fecha: str | None = None) -> subprocess.CompletedProcess:
        argumentos = [sys.executable, str(SCRIPT), "--raiz", str(raiz)]
        if fecha:
            argumentos += ["--fecha", fecha]
        return subprocess.run(argumentos, capture_output=True, text=True, timeout=60)

    def test_vigente_pasa(self) -> None:
        r = self._ejecutar(self._raiz(), "2026-11-08")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertIn("RFC006_VIGENCIA_PASS", r.stdout)

    def test_limite_exacto_de_caducidad_pasa(self) -> None:
        r = self._ejecutar(self._raiz(), "2026-12-02")
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)

    def test_caducada_falla(self) -> None:
        r = self._ejecutar(self._raiz(), "2026-12-03")
        self.assertEqual(r.returncode, 1)
        self.assertIn("RFC006_CADUCADA", r.stdout)

    def test_revocada_falla(self) -> None:
        rfc = RFC_OK.replace("| Estado | **APROBADA (2026-11-08)** - aceptacion temporal |",
                             "| Estado | **REVOCADA (2026-11-20)** |")
        r = self._ejecutar(self._raiz(rfc=rfc), "2026-11-08")
        self.assertEqual(r.returncode, 1)
        self.assertIn("RFC006_NO_APROBADA", r.stdout)

    def test_caducidades_distintas_fallan(self) -> None:
        ignore = IGNORE_OK.replace("2026-12-02T00:00:00Z", "2026-12-01T00:00:00Z")
        r = self._ejecutar(self._raiz(ignore=ignore), "2026-11-08")
        self.assertEqual(r.returncode, 1)
        self.assertIn("CADUCIDADES_DISTINTAS", r.stdout)

    def test_ignore_sin_aws0011_falla(self) -> None:
        ignore = "misconfigurations:\n  - id: AWS-0054\n    paths:\n      - \"modules/balanceo/main.tf\"\n"
        r = self._ejecutar(self._raiz(ignore=ignore), "2026-11-08")
        self.assertEqual(r.returncode, 1)
        self.assertIn("IGNORE_SIN_AWS0011", r.stdout)

    def test_ruta_no_autorizada_falla(self) -> None:
        ignore = IGNORE_OK.replace('"modules/balanceo/main.tf"', '"modules/red/main.tf"')
        r = self._ejecutar(self._raiz(ignore=ignore), "2026-11-08")
        self.assertEqual(r.returncode, 1)
        self.assertIn("AWS0011_RUTA_NO_AUTORIZADA", r.stdout)

    def test_gate_de_despliegue_aprobado_falla(self) -> None:
        gates = {"GATE_DESPLIEGUE_AWS.md": "| Estado | APRUEBO_DESPLIEGUE |"}
        r = self._ejecutar(self._raiz(gates=gates), "2026-11-08")
        self.assertEqual(r.returncode, 1)
        self.assertIn("GATE_DESPLIEGUE_AWS_PRESENTE", r.stdout)

    def test_sin_rfc_falla(self) -> None:
        r = self._ejecutar(self._raiz(rfc=None), "2026-11-08")
        self.assertEqual(r.returncode, 1)
        self.assertIn("FALTA_RFC006", r.stdout)

    def test_fecha_invalida_devuelve_2(self) -> None:
        r = self._ejecutar(self._raiz(), "03-12-2026")
        self.assertEqual(r.returncode, 2)


if __name__ == "__main__":
    unittest.main()
