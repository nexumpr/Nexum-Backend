"""Pruebas del umbral de cobertura del dominio (I-080).

Usan perfiles sintéticos con el formato exacto de `go test -coverprofile`.
No ejecutan Go ni consultan servicios: solo verifican el cálculo, la deduplicación
de bloques repetidos y el fallo cerrado.
"""

from __future__ import annotations

import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

SCRIPT = Path(__file__).resolve().parents[2] / "herramientas" / "cobertura_dominio.py"
PREFIJO = "github.com/nexumpr/Nexum-Backend/internal/domain"


def _linea(archivo: str, inicio: str, fin: str, num: int, cuenta: int) -> str:
    return f"{PREFIJO}/{archivo}:{inicio},{fin} {num} {cuenta}"


class CoberturaDominio(unittest.TestCase):
    def _perfil(self, contenido: str) -> Path:
        carpeta = Path(tempfile.mkdtemp(prefix="cov-i080-"))
        ruta = carpeta / "coverage.out"
        ruta.write_text(contenido, encoding="utf-8")
        return ruta

    def _ejecutar(self, perfil: Path, umbral: float | None = None) -> subprocess.CompletedProcess:
        argumentos = [sys.executable, str(SCRIPT), str(perfil)]
        if umbral is not None:
            argumentos += ["--umbral", str(umbral)]
        return subprocess.run(argumentos, capture_output=True, text=True, timeout=60)

    def test_cobertura_suficiente_pasa(self) -> None:
        perfil = self._perfil(
            "mode: set\n"
            + _linea("money/money.go", "10.1", "11.2", 4, 1)
            + "\n"
            + _linea("money/money.go", "20.1", "21.2", 1, 0)
            + "\n"
        )
        r = self._ejecutar(perfil, 80)
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertIn("COBERTURA_DOMINIO_PASS", r.stdout)
        self.assertIn("80.0%", r.stdout)

    def test_cobertura_insuficiente_falla(self) -> None:
        perfil = self._perfil(
            "mode: set\n"
            + _linea("money/money.go", "10.1", "11.2", 1, 1)
            + "\n"
            + _linea("money/money.go", "20.1", "21.2", 9, 0)
            + "\n"
        )
        r = self._ejecutar(perfil, 80)
        self.assertEqual(r.returncode, 1)
        self.assertIn("COBERTURA_DOMINIO_FAIL", r.stdout)

    def test_deduplica_bloques_repetidos_entre_binarios(self) -> None:
        # El mismo bloque aparece dos veces: una sin ejecutar y otra ejecutada.
        # Debe contarse una sola vez y como cubierto (semántica de go tool cover).
        perfil = self._perfil(
            "mode: set\n"
            + _linea("cache/keys.go", "1.1", "2.2", 10, 0)
            + "\n"
            + _linea("cache/keys.go", "1.1", "2.2", 10, 1)
            + "\n"
        )
        r = self._ejecutar(perfil, 80)
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertIn("100.0%", r.stdout)
        self.assertNotIn("200", r.stdout)

    def test_perfil_sin_bloques_de_dominio_falla_cerrado(self) -> None:
        perfil = self._perfil(
            "mode: set\n"
            "github.com/nexumpr/Nexum-Backend/internal/system/context.go:1.1,2.2 3 1\n"
        )
        r = self._ejecutar(perfil, 80)
        self.assertEqual(r.returncode, 1)
        self.assertIn("no contiene bloques de internal/domain/", r.stdout)

    def test_perfil_inexistente_falla(self) -> None:
        r = self._ejecutar(Path(tempfile.mkdtemp(prefix="cov-i080-")) / "no-existe.out")
        self.assertEqual(r.returncode, 1)
        self.assertIn("COBERTURA_DOMINIO_FAIL", r.stdout)

    def test_umbral_invalido_es_error_de_uso(self) -> None:
        perfil = self._perfil("mode: set\n" + _linea("money/money.go", "1.1", "2.2", 1, 1) + "\n")
        r = self._ejecutar(perfil, 0)
        self.assertEqual(r.returncode, 2)

    def test_umbral_exacto_se_acepta(self) -> None:
        perfil = self._perfil(
            "mode: set\n"
            + _linea("money/money.go", "1.1", "2.2", 8, 1)
            + "\n"
            + _linea("money/money.go", "3.1", "4.2", 2, 0)
            + "\n"
        )
        r = self._ejecutar(perfil, 80)
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)


if __name__ == "__main__":
    unittest.main()
