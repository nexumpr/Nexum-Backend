"""Verifica que la evidencia versionada no contenga datos sensibles."""

from __future__ import annotations

import sys
import unittest
from pathlib import Path

RAIZ = Path(__file__).resolve().parents[3]
sys.path.insert(0, str(RAIZ / "infra" / "finops"))

import sanitizar  # noqa: E402

DIR_EVIDENCIA = RAIZ / "infra" / "finops" / "evidencia"
DIR_RESULTADOS = RAIZ / "infra" / "finops" / "resultados"


class EvidenciaSanitizada(unittest.TestCase):
    def _archivos(self) -> list[Path]:
        archivos: list[Path] = []
        for directorio in (DIR_EVIDENCIA, DIR_RESULTADOS):
            if directorio.is_dir():
                archivos.extend(sorted(directorio.glob("*.json")))
        return archivos

    def test_hay_evidencia_versionada(self) -> None:
        self.assertGreaterEqual(len(self._archivos()), 5)

    def test_sin_patrones_sensibles(self) -> None:
        for archivo in self._archivos():
            texto = archivo.read_text(encoding="utf-8")
            hallazgos = sanitizar.contiene_datos_sensibles(texto)
            self.assertEqual(hallazgos, [], f"{archivo.name} contiene {hallazgos}")

    def test_sin_claves_privadas_ni_credenciales(self) -> None:
        for archivo in self._archivos():
            texto = archivo.read_text(encoding="utf-8").lower()
            for prohibido in ("begin rsa private key", "aws_secret_access_key", "sessiontoken"):
                self.assertNotIn(prohibido, texto, f"{archivo.name} contiene {prohibido}")

    def test_la_evidencia_declara_su_estado(self) -> None:
        for archivo in sorted(DIR_EVIDENCIA.glob("*.json")):
            if archivo.name.startswith("topologia"):
                continue
            texto = archivo.read_text(encoding="utf-8")
            self.assertIn('"estado"', texto, f"{archivo.name} sin campo estado")

    def test_resultados_declaran_decision(self) -> None:
        viabilidad = DIR_RESULTADOS / "viabilidad_i000.json"
        self.assertTrue(viabilidad.is_file(), "falta viabilidad_i000.json")
        self.assertIn('"estado"', viabilidad.read_text(encoding="utf-8"))
        costos = DIR_RESULTADOS / "costos_45d.json"
        self.assertTrue(costos.is_file(), "falta costos_45d.json")
        self.assertIn('"escenario_seleccionado"', costos.read_text(encoding="utf-8"))


if __name__ == "__main__":
    unittest.main()
