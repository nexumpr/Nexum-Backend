"""Pruebas de la autorización de despliegue I-090."""

from __future__ import annotations

import subprocess
import sys
import unittest
from pathlib import Path

SCRIPT = Path(__file__).resolve().parents[2] / "herramientas" / "verificar_autorizacion_despliegue.py"
SHA = "a" * 40
SHA12 = SHA[:12]
DIGEST = "b" * 64
RELEASE = f"v0.1.0+{SHA12}"


class AutorizacionDespliegue(unittest.TestCase):
    def _ejecutar(self, **cambios: str) -> subprocess.CompletedProcess:
        valores = {
            "--sha": SHA,
            "--release-id": RELEASE,
            "--frase": f"AUTORIZO_DESPLIEGUE_{SHA}",
            "--confirmar": "SI",
            "--habilitado": "true",
            "--sha256-binario": DIGEST,
        }
        valores.update(cambios)
        argumentos = [sys.executable, str(SCRIPT)]
        for clave, valor in valores.items():
            argumentos += [clave, valor]
        return subprocess.run(argumentos, capture_output=True, text=True, timeout=60)

    def test_autorizacion_valida_pasa(self) -> None:
        r = self._ejecutar()
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertIn("AUTORIZACION_DESPLIEGUE_PASS", r.stdout)

    def test_frase_de_otro_sha_falla(self) -> None:
        r = self._ejecutar(**{"--frase": "AUTORIZO_DESPLIEGUE_" + "c" * 40})
        self.assertEqual(r.returncode, 1)
        self.assertIn("FRASE_NO_CORRESPONDE_AL_SHA", r.stdout)

    def test_release_no_coincide_con_sha_falla(self) -> None:
        r = self._ejecutar(**{"--release-id": "v0.1.0+000000000000"})
        self.assertEqual(r.returncode, 1)
        self.assertIn("RELEASE_NO_CORRESPONDE_AL_SHA", r.stdout)

    def test_release_mal_formado_falla(self) -> None:
        r = self._ejecutar(**{"--release-id": "0.1.0"})
        self.assertEqual(r.returncode, 1)
        self.assertIn("RELEASE_ID_INVALIDO", r.stdout)

    def test_sha_corto_falla(self) -> None:
        r = self._ejecutar(**{"--sha": "abc123"})
        self.assertEqual(r.returncode, 1)
        self.assertIn("SHA_INVALIDO", r.stdout)

    def test_digest_invalido_falla(self) -> None:
        r = self._ejecutar(**{"--sha256-binario": "xyz"})
        self.assertEqual(r.returncode, 1)
        self.assertIn("SHA256_BINARIO_INVALIDO", r.stdout)

    def test_confirmacion_ausente_falla(self) -> None:
        r = self._ejecutar(**{"--confirmar": "NO"})
        self.assertEqual(r.returncode, 1)
        self.assertIn("CONFIRMACION_AUSENTE", r.stdout)

    def test_cd_no_habilitado_falla(self) -> None:
        r = self._ejecutar(**{"--habilitado": "false"})
        self.assertEqual(r.returncode, 1)
        self.assertIn("CD_NO_HABILITADO", r.stdout)

    def test_menos_de_dos_nodos_falla(self) -> None:
        r = self._ejecutar(**{"--total-nodos": "1"})
        self.assertEqual(r.returncode, 1)
        self.assertIn("NODOS_INSUFICIENTES", r.stdout)

    def test_uso_incorrecto_devuelve_2(self) -> None:
        r = subprocess.run([sys.executable, str(SCRIPT)], capture_output=True, text=True, timeout=60)
        self.assertEqual(r.returncode, 2)


if __name__ == "__main__":
    unittest.main()
