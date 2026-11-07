"""Pruebas de la configuración de gitleaks (A-100 + excepción I-030).

La excepción del digest RFC-004 debe ser **mínima y no ocultar credenciales**:
gitleaks v8.30.1 evalúa las allowlists globales en modo OR por defecto y, en ese
modo, `checkCommitOrPathAllowed` descarta **todos** los hallazgos de la ruta que
coincide. Estas pruebas exigen que la excepción esté dentro de la regla
`generic-api-key`, con `condition = "AND"` y con ruta + patrón de línea.

La verificación dinámica con credenciales sintéticas se ejecuta fuera del
repositorio (entorno temporal) y su resultado se registra en `memoria.md`.
"""

from __future__ import annotations

import re
import unittest
from pathlib import Path

RAIZ = Path(__file__).resolve().parents[2]
CFG = RAIZ / ".gitleaks.toml"


def _leer() -> str:
    return CFG.read_text(encoding="utf-8")


class ConfiguracionGitleaks(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.texto = _leer()
        cls.sin_comentarios = "\n".join(
            linea for linea in cls.texto.splitlines() if not linea.lstrip().startswith("#")
        )

    def test_usa_reglas_por_defecto(self) -> None:
        self.assertIn("[extend]", self.sin_comentarios)
        self.assertIn("useDefault = true", self.sin_comentarios)

    def test_no_hay_allowlist_global(self) -> None:
        """Una allowlist global en OR oculta el archivo completo."""
        self.assertNotRegex(self.sin_comentarios, r"(?m)^\s*\[\[allowlists\]\]")

    def test_excepcion_dentro_de_la_regla_generic_api_key(self) -> None:
        self.assertRegex(self.sin_comentarios, r'(?m)^\[\[rules\]\]\s*\nid\s*=\s*"generic-api-key"')
        self.assertRegex(self.sin_comentarios, r"(?m)^\[\[rules\.allowlists\]\]")

    def test_condicion_and_obligatoria(self) -> None:
        self.assertIn('condition = "AND"', self.sin_comentarios)
        self.assertNotIn('condition = "OR"', self.sin_comentarios)

    def test_exige_ruta_y_patron_de_linea(self) -> None:
        self.assertIn("paths = ", self.sin_comentarios)
        self.assertIn('regexTarget = "line"', self.sin_comentarios)
        self.assertIn("regexes = ", self.sin_comentarios)
        self.assertIn("RFC-004-SECRETOS_BOOTSTRAP", self.sin_comentarios)
        self.assertIn("[a-f0-9]{64}", self.sin_comentarios)

    def test_no_excluye_carpetas_ni_reglas(self) -> None:
        for prohibido in ("disabledRules", "stopwords", "'''sdd/'''", "'''sdd\\'"):
            self.assertNotIn(prohibido, self.sin_comentarios)
        # La ruta autorizada es exactamente un archivo, no un prefijo.
        rutas = re.findall(r"paths\s*=\s*\[\s*'''(.*?)'''", self.sin_comentarios)
        self.assertEqual(len(rutas), 1)
        self.assertIn("BASELINE_SHA256", rutas[0])
        self.assertTrue(rutas[0].endswith("$"), "la ruta debe estar anclada al final")

    def test_sin_credenciales_reales_en_la_configuracion(self) -> None:
        self.assertNotRegex(self.texto, r"AKIA[0-9A-Z]{16}")
        self.assertNotRegex(self.texto, r"ghp_[A-Za-z0-9]{20,}")


if __name__ == "__main__":
    unittest.main()
