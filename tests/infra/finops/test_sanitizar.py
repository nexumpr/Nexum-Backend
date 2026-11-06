"""Pruebas del sanitizador de evidencia AWS."""

from __future__ import annotations

import json
import sys
import unittest
from pathlib import Path

RAIZ = Path(__file__).resolve().parents[3]
sys.path.insert(0, str(RAIZ / "infra" / "finops"))

import sanitizar  # noqa: E402


class Sanitizacion(unittest.TestCase):
    def test_enmascara_cuenta_en_arn(self) -> None:
        texto = "arn:aws:iam::123456789012:user/nexum-operator"
        limpio = sanitizar.sanitizar(texto)
        self.assertNotIn("123456789012", limpio)
        self.assertIn("<ARN>", limpio)

    def test_enmascara_cuenta_suelta(self) -> None:
        limpio = sanitizar.sanitizar("cuenta 123456789012 lista")
        self.assertNotIn("123456789012", limpio)
        self.assertIn("<ACCOUNT_ID>", limpio)

    def test_enmascara_claves_y_tokens(self) -> None:
        texto = "AKIAIOSFODNN7EXAMPLE Bearer abc.def.ghi eyJhbGciOiJSUzI1NiJ9.eyJzdWIiOiIxIn0.abcdefgh"
        limpio = sanitizar.sanitizar(texto)
        self.assertNotIn("AKIAIOSFODNN7EXAMPLE", limpio)
        self.assertNotIn("eyJhbGciOiJSUzI1NiJ9", limpio)

    def test_enmascara_correo_e_ip(self) -> None:
        limpio = sanitizar.sanitizar("contacto persona@ejemplo.com desde 10.20.30.40")
        self.assertNotIn("persona@ejemplo.com", limpio)
        self.assertNotIn("10.20.30.40", limpio)

    def test_campos_sensibles_en_diccionario(self) -> None:
        datos = {"Account": "123456789012", "Arn": "arn:aws:iam::123456789012:role/x", "UserId": "AIDAXXX"}
        limpio = sanitizar.sanitizar(datos)
        self.assertEqual(limpio["Account"], "<ACCOUNT_ID>")
        self.assertEqual(limpio["Arn"], "<ARN>")
        self.assertEqual(limpio["UserId"], "<REDACTED>")

    def test_sanitizar_json_conserva_estructura(self) -> None:
        original = json.dumps({"Account": "123456789012", "Plan": "FREE", "Uso": [{"ip": "1.2.3.4"}]})
        limpio = json.loads(sanitizar.sanitizar_json(original))
        self.assertEqual(limpio["Plan"], "FREE")
        self.assertEqual(limpio["Account"], "<ACCOUNT_ID>")

    def test_no_quedan_patrones_sensibles(self) -> None:
        texto = "arn:aws:s3:::bucket 123456789012 AKIAIOSFODNN7EXAMPLE persona@ejemplo.com 10.0.0.1"
        limpio = sanitizar.sanitizar(texto)
        self.assertEqual(sanitizar.contiene_datos_sensibles(limpio), [])

    def test_texto_plano_no_json(self) -> None:
        limpio = sanitizar.sanitizar_json("error con cuenta 123456789012")
        self.assertNotIn("123456789012", limpio)


if __name__ == "__main__":
    unittest.main()
