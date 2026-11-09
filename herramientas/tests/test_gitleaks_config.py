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
        self.assertIn("[a-f0-9]{64}", self.sin_comentarios)

    def test_no_excluye_carpetas_ni_reglas(self) -> None:
        for prohibido in ("disabledRules", "stopwords", "'''sdd/'''", "'''sdd\\'"):
            self.assertNotIn(prohibido, self.sin_comentarios)
        # Cada ruta autorizada es un archivo concreto, nunca un prefijo de carpeta.
        rutas = re.findall(r"paths\s*=\s*\[\s*'''(.*?)'''", self.sin_comentarios)
        self.assertEqual(len(rutas), 2, "solo el manifiesto y la prueba de I-120")
        self.assertEqual(len([r for r in rutas if "BASELINE_SHA256" in r]), 1)
        self.assertEqual(len([r for r in rutas if "tests/infra/operacion" in r]), 1)
        for ruta in rutas:
            self.assertTrue(ruta.endswith("$"), f"la ruta debe estar anclada: {ruta}")

    def test_patron_cubre_todos_los_documentos_secretos_sin_ocultar_credenciales(self) -> None:
        """El manifiesto declara ruta + digest; la excepción sólo cubre ese formato."""
        rutas = re.findall(r"regexes\s*=\s*\[\s*'''(.*?)'''", self.sin_comentarios)
        self.assertEqual(len(rutas), 2, "manifiesto + constante de estado de I-120")
        candidatos = [r for r in rutas if "[a-f0-9]{64}" in r]
        self.assertEqual(len(candidatos), 1)
        patron = candidatos[0]
        self.assertIn("[a-f0-9]{64}", patron)
        self.assertIn("BASELINE_SHA256", self.sin_comentarios)
        # Debe aceptar los documentos existentes y rechazar cualquier otra línea.
        self.assertRegex(
            '  "infra/terraform/policies/terraform-state-access.json.tftpl": "' + "a" * 64 + '"',
            patron.replace("\\\\", "\\"),
        )
        self.assertRegex(
            '  "sdd/_control/H08/ROTACION_SECRETOS_I060.md": "' + "b" * 64 + '"',
            patron.replace("\\\\", "\\"),
        )
        self.assertNotRegex(
            '  "sdd/_control/H08/ROTACION_SECRETOS_I060.md": "AKIAIOSFODNN7EXAMPLE"',
            patron.replace("\\\\", "\\"),
        )

    def test_sin_credenciales_reales_en_la_configuracion(self) -> None:
        self.assertNotRegex(self.texto, r"AKIA[0-9A-Z]{16}")
        self.assertNotRegex(self.texto, r"ghp_[A-Za-z0-9]{20,}")


if __name__ == "__main__":
    unittest.main()


class ExcepcionI120(unittest.TestCase):
    """La constante de estado de I-120 no puede abrir un agujero de seguridad."""

    def test_excepcion_acotada_a_la_prueba_de_operacion(self) -> None:
        texto = _leer()
        bloque = texto.split("Constante de estado BLOQUEADO_*")[1].split("[[rules.allowlists]]")[0]
        self.assertIn('condition = "AND"', bloque)
        self.assertIn("tests/infra/operacion", bloque)
        self.assertIn('regexTarget = "secret"', bloque)
        self.assertIn("BLOQUEADO_", bloque)

    def test_patron_solo_acepta_constantes_de_estado(self) -> None:
        import re

        patron = re.compile(r"^BLOQUEADO_[A-Z0-9]+$")
        self.assertTrue(patron.match("BLOQUEADO_RFC004"))
        for valor in (
            "cf73cb1ac2eb61137e33abbaced62405b445cdd4bedf269c0a64f23475a9fb19",
            "AKIAIOSFODNN7EXAMPLE",
            "sk-ant-api03-abcdefghijklmnopqrstuvwxyz",
            "BLOQUEADO_rfc004",
        ):
            self.assertIsNone(patron.match(valor), valor)

    def test_no_existe_allowlist_global_de_ruta(self) -> None:
        texto = _leer()
        self.assertNotIn("[allowlist]", texto)
        self.assertIn("generic-api-key", texto)
