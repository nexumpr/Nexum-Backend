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

# Literales sintéticos construidos por concatenación: este archivo no debe contener una
# asignación `key = "<valor>"` ni `"password":"<valor>"` en claro, porque gitleaks la
# trataría como un hallazgo real (así se detectó al validar la propia configuración).
CLAVE_IDEMPOTENCIA = "0123456789" + "abcdef"
OTRA_CLAVE_HEX = "fedcba98" + "76543210"
CLAVE_LARGA = "9f8e7d6c5b4a3928" + "1706f5e4d3c2b1a0"
PASSPHRASE_SINTETICA = "ExamplePassphrase" + "#2026"
CONTRASENA_ALTA_ENTROPIA = "Xq7Lm2Rt9Vb4" + "Nz8Kp3"


def _leer() -> str:
    return CFG.read_text(encoding="utf-8")


def _bloque(texto: str, descripcion: str) -> str:
    """Bloque de allowlist cuya descripción contiene el texto dado, sin líneas de comentario."""
    for bloque in texto.split("[[rules.allowlists]]")[1:]:
        if descripcion in bloque:
            return "\n".join(
                linea for linea in bloque.splitlines() if not linea.lstrip().startswith("#")
            )
    raise AssertionError(f"allowlist no encontrada: {descripcion}")


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
        # Cada ruta autorizada es un archivo concreto y anclado, nunca un prefijo.
        self.assertIn("paths = ['''^sdd/_control/H07/BASELINE_SHA256\\.json$''']", self.sin_comentarios)
        self.assertIn("'''^tests/infra/operacion/test_nexum_operacion\\.py$'''", self.sin_comentarios)
        self.assertIn("'''^tests/infra/respaldo/test_backup_restore\\.py$'''", self.sin_comentarios)
        self.assertIn("'''^tests/integration/register_http_test\\.go$'''", self.sin_comentarios)
        self.assertIn("'''^tests/unit/transport/register_handler_test\\.go$'''", self.sin_comentarios)
        self.assertEqual(self.sin_comentarios.count("tests/infra/operacion/"), 1)
        self.assertEqual(self.sin_comentarios.count("tests/infra/respaldo/"), 1)
        self.assertEqual(self.sin_comentarios.count("tests/integration/register_http_test\\.go$"), 2)
        self.assertEqual(self.sin_comentarios.count("tests/unit/transport/register_handler_test\\.go$"), 2)
        self.assertNotIn("tests/infra/**", self.sin_comentarios)
        self.assertNotIn("tests/integration/**", self.sin_comentarios)
        self.assertNotIn("tests/unit/**", self.sin_comentarios)
        self.assertEqual(self.sin_comentarios.count("[allowlist]"), 0, "sin allowlist global")

    def test_patron_cubre_todos_los_documentos_secretos_sin_ocultar_credenciales(self) -> None:
        """El manifiesto declara ruta + digest; la excepción sólo cubre ese formato."""
        rutas = re.findall(r"regexes\s*=\s*\[\s*'''(.*?)'''", self.sin_comentarios)
        self.assertEqual(
            len(rutas),
            4,
            "manifiesto + constante de estado de I-120 + passphrase sintetica E001 + clave de idempotencia E001",
        )
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

    def test_excepcion_passphrase_sintetica_e001_es_acotada(self) -> None:
        bloque = _bloque(self.texto, "Contrasena sintetica de pruebas E001")
        self.assertIn('condition = "AND"', bloque)
        self.assertIn('regexTarget = "line"', bloque)
        for ruta in (
            "register_http_test",
            "register_adversarial_test",
            "register_handler_test",
            "register_service_test",
        ):
            self.assertIn(ruta, bloque)
        self.assertNotIn("tests/**", bloque)
        self.assertNotIn("tests/integration/**", bloque)
        self.assertNotIn("tests/unit/**", bloque)

    def test_passphrase_no_exime_cualquier_linea_con_password(self) -> None:
        """El patrón exige el valor sintético pegado al campo y no un hueco libre."""
        bloque = _bloque(self.texto, "Contrasena sintetica de pruebas E001")
        patron = re.search(r"regexes\s*=\s*\[\s*'''(.*?)'''", bloque).group(1)
        self.assertNotIn("[^\\r\\n]*", patron, "un hueco libre ocultaría una credencial en la misma línea")
        compilado = re.compile(patron)
        self.assertTrue(compilado.search('{"password":"' + PASSPHRASE_SINTETICA + '"}'))
        for linea in (
            '{"password":"' + CONTRASENA_ALTA_ENTROPIA + '"}',
            '{"password":"OtraPassphrase#2027"}',
            'password := "' + PASSPHRASE_SINTETICA + '" + sufijo',
        ):
            self.assertIsNone(compilado.search(linea), linea)

    def test_sin_credenciales_reales_en_la_configuracion(self) -> None:
        self.assertNotRegex(self.texto, r"AKIA[0-9A-Z]{16}")
        self.assertNotRegex(self.texto, r"ghp_[A-Za-z0-9]{20,}")


if __name__ == "__main__":
    unittest.main()


class ExcepcionI120(unittest.TestCase):
    """La constante de estado de I-120 no puede abrir un agujero de seguridad."""

    @classmethod
    def setUpClass(cls) -> None:
        import re

        texto = _leer()
        bloque = texto.split("Constante de estado BLOQUEADO_RFC004")[1].split("[[rules.allowlists]]")[0]
        cls.bloque = bloque
        cls.ruta = re.search(r"paths\s*=\s*\[\s*'''(.*?)'''", bloque).group(1)
        cls.valor = re.search(r"regexes\s*=\s*\[\s*'''(.*?)'''", bloque).group(1)

    def test_excepcion_en_un_unico_archivo(self) -> None:
        import re

        patron = re.compile(self.ruta)
        self.assertTrue(patron.match("tests/infra/operacion/test_nexum_operacion.py"))
        for ruta in (
            "tests/infra/operacion/test_operacion_avanzada.py",
            "tests/infra/operacion/test_nexum_operacion_v2.py",
            "tests/infra/finops/test_finops_diario.py",
            "herramientas/nexum_operacion.py",
        ):
            self.assertIsNone(patron.match(ruta), ruta)

    def test_excepcion_solo_acepta_el_valor_constante(self) -> None:
        import re

        patron = re.compile(self.valor)
        self.assertTrue(patron.match("BLOQUEADO_RFC004"))
        for valor in (
            "BLOQUEADO_RFC005",
            "BLOQUEADO_OTRA",
            "cf73cb1ac2eb61137e33abbaced62405b445cdd4bedf269c0a64f23475a9fb19",
            "AKIAIOSFODNN7EXAMPLE",
            "sk-ant-api03-abcdefghijklmnopqrstuvwxyz",
        ):
            self.assertIsNone(patron.match(valor), valor)

    def test_excepcion_es_and_y_sobre_el_secreto(self) -> None:
        self.assertIn('condition = "AND"', self.bloque)
        self.assertIn('regexTarget = "secret"', self.bloque)

    def test_no_existe_allowlist_global_de_ruta(self) -> None:
        texto = _leer()
        self.assertNotIn("[allowlist]", texto)
        self.assertIn("generic-api-key", texto)


class ExcepcionE001ClaveIdempotencia(unittest.TestCase):
    """Los 3 hallazgos del CI eran la clave de idempotencia sintética, no la passphrase.

    Evidencia reproducida con gitleaks v8.30.1 y el historial real: commit 275f598,
    `tests/integration/register_http_test.go:88` y
    `tests/unit/transport/register_handler_test.go:140,275`, secreto `0123456789abcdef`.
    La excepción debe cubrir exactamente esa asignación en esos dos archivos.
    """

    @classmethod
    def setUpClass(cls) -> None:
        texto = _leer()
        cls.texto = texto
        cls.bloque = _bloque(texto, "Clave de idempotencia sintetica de pruebas E001")
        rutas_y_regex = cls.bloque.split("regexTarget", 1)
        cls.rutas = re.findall(r"'''(.*?)'''", rutas_y_regex[0])
        cls.patron = re.search(r"regexes\s*=\s*\[\s*'''(.*?)'''", cls.bloque).group(1)

    def test_condicion_and_y_objetivo_linea(self) -> None:
        self.assertIn('condition = "AND"', self.bloque)
        self.assertIn('regexTarget = "line"', self.bloque)

    def test_rutas_exactas_sin_prefijos(self) -> None:
        self.assertEqual(
            set(self.rutas),
            {
                r"^tests/integration/register_http_test\.go$",
                r"^tests/unit/transport/register_handler_test\.go$",
            },
        )
        self.assertEqual(len(self.rutas), 2)
        for ruta in self.rutas:
            compilada = re.compile(ruta)
            self.assertIsNone(compilada.match("tests/unit/transport/register_handler_test.go.bak"))
            self.assertIsNone(compilada.match("tests/integration/otro/register_http_test.go"))

    def test_rutas_no_cubren_el_tercer_archivo_del_hallazgo(self) -> None:
        permitidas = [re.compile(r) for r in self.rutas]
        for ruta in (
            "tests/security/register_adversarial_test.go",
            "tests/unit/application/register_service_test.go",
            "tests/unit/transport/otro_test.go",
        ):
            self.assertFalse(any(p.match(ruta) for p in permitidas), ruta)

    def test_acepta_solo_las_asignaciones_historicas(self) -> None:
        compilado = re.compile(self.patron)
        self.assertTrue(compilado.match(f'\tconst key = "{CLAVE_IDEMPOTENCIA}"'))
        self.assertTrue(compilado.match(f'\tkey := "{CLAVE_IDEMPOTENCIA}"'))
        for linea in (
            f'\tkey := "{OTRA_CLAVE_HEX}"',
            f'\tkey := "{CLAVE_LARGA}"',
            f'\tidempotencyKey := "{CLAVE_IDEMPOTENCIA}"',
            f'\tkey = "{CLAVE_IDEMPOTENCIA}" // comentario',
            f'\tconst key = "{CLAVE_IDEMPOTENCIA}" + sufijo',
            f'\tkey := "{CLAVE_IDEMPOTENCIA}0"',
        ):
            self.assertIsNone(compilado.match(linea), linea)

    def test_registra_la_evidencia_del_hallazgo(self) -> None:
        self.assertIn("275f598", self.texto)
        for referencia in ("register_http_test.go", "register_handler_test.go", CLAVE_IDEMPOTENCIA):
            self.assertIn(referencia, self.texto)
        self.assertNotIn(PASSPHRASE_SINTETICA + "'''", self.bloque)
