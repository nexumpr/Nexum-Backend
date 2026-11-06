"""Verifica que la evidencia versionada no contenga datos sensibles.

El directorio ``evidencia/`` contiene dos tipos de archivo:

- **agregados fechados** (``cuenta_*``, ``servicios_*``, ``free_tier_*``,
  ``precios_*``, ``costos_observados_*``, ``topologia_*``): los construye este
  proyecto y declaran ``estado``;
- **volcados sanitizados de una consulta** (``<id>.json``): salida de AWS ya
  enmascarada, sin campo de estado propio.

Ambos deben estar libres de secretos; sólo los agregados deben declarar estado.
"""

from __future__ import annotations

import json
import sys
import unittest
from pathlib import Path

RAIZ = Path(__file__).resolve().parents[3]
sys.path.insert(0, str(RAIZ / "infra" / "finops"))

import sanitizar  # noqa: E402

DIR_EVIDENCIA = RAIZ / "infra" / "finops" / "evidencia"
DIR_RESULTADOS = RAIZ / "infra" / "finops" / "resultados"
PREFIJOS_AGREGADOS = (
    "cuenta_",
    "servicios_",
    "free_tier_",
    "precios_",
    "costos_observados_",
)


class EvidenciaSanitizada(unittest.TestCase):
    def _archivos(self) -> list[Path]:
        archivos: list[Path] = []
        for directorio in (DIR_EVIDENCIA, DIR_RESULTADOS):
            if directorio.is_dir():
                archivos.extend(sorted(directorio.glob("*.json")))
        return archivos

    def _agregados(self) -> list[Path]:
        """Agregados fechados: ``prefijo_<YYYY-MM-DD>.json``.

        Los volcados de una sola consulta (``free_tier_uso.json``,
        ``sts_identidad.json``) no llevan fecha ni campo ``estado``.
        """

        import re

        patron = re.compile(r"^(" + "|".join(PREFIJOS_AGREGADOS) + r")\d{4}-\d{2}-\d{2}\.json$")
        return [
            a for a in sorted(DIR_EVIDENCIA.glob("*.json")) if patron.match(a.name)
        ]

    def test_hay_evidencia_versionada(self) -> None:
        self.assertGreaterEqual(len(self._archivos()), 5)

    def test_hay_agregados_con_estado(self) -> None:
        self.assertGreaterEqual(len(self._agregados()), 5, [a.name for a in self._agregados()])

    def test_sin_patrones_sensibles(self) -> None:
        for archivo in self._archivos():
            texto = archivo.read_text(encoding="utf-8")
            hallazgos = sanitizar.contiene_datos_sensibles(texto)
            self.assertEqual(hallazgos, [], f"{archivo.name} contiene {hallazgos}")

    def test_sin_claves_privadas_ni_credenciales(self) -> None:
        for archivo in self._archivos():
            texto = archivo.read_text(encoding="utf-8").lower()
            for prohibido in (
                "begin rsa private key",
                "aws_secret_access_key",
                "sessiontoken",
            ):
                self.assertNotIn(prohibido, texto, f"{archivo.name} contiene {prohibido}")

    def test_sin_numero_de_cuenta(self) -> None:
        import re

        patron = re.compile(r"\b\d{12}\b")
        for archivo in self._archivos():
            texto = archivo.read_text(encoding="utf-8")
            self.assertIsNone(patron.search(texto), f"{archivo.name} expone un id de cuenta")

    def test_los_agregados_declaran_estado(self) -> None:
        for archivo in self._agregados():
            datos = json.loads(archivo.read_text(encoding="utf-8"))
            self.assertIn("estado", datos, f"{archivo.name} sin campo estado")
            self.assertIn(
                datos["estado"],
                {"VERIFIED_AWS", "NOT_VERIFIED", "NOT_RUN", "FAIL", "BLOCKED"},
                f"{archivo.name}: estado inesperado {datos['estado']}",
            )

    def test_resultados_declaran_decision(self) -> None:
        viabilidad = DIR_RESULTADOS / "viabilidad_i000.json"
        self.assertTrue(viabilidad.is_file(), "falta viabilidad_i000.json")
        datos = json.loads(viabilidad.read_text(encoding="utf-8"))
        self.assertIn(datos["estado"], {"VIABLE", "BLOCKED_FINOPS"})
        self.assertIn("resumen_financiero", datos)
        self.assertIn("riesgo_cobro_personal", datos["resumen_financiero"])

        costos = DIR_RESULTADOS / "costos_45d.json"
        self.assertTrue(costos.is_file(), "falta costos_45d.json")
        self.assertIn("escenario_seleccionado", costos.read_text(encoding="utf-8"))

    def test_topologia_no_se_autocertifica(self) -> None:
        ruta = DIR_EVIDENCIA / "topologia_c_2026-11-06.json"
        datos = json.loads(ruta.read_text(encoding="utf-8"))
        self.assertEqual(datos["estado_verificacion"]["runtime"], "NOT_RUN")
        self.assertFalse(datos["estado_verificacion"]["recursos_creados"])

    def test_no_hay_evidencia_de_despliegue(self) -> None:
        self.assertEqual(list(DIR_EVIDENCIA.glob("despliegue_runtime_*.json")), [])


if __name__ == "__main__":
    unittest.main()
