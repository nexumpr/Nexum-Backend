"""Pruebas del escaneo IaC DevSecOps (I-100)."""

from __future__ import annotations

import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

SCRIPT = Path(__file__).resolve().parents[2] / "herramientas" / "verificar_iac_devsecops.py"


def _raiz(archivos: dict[str, str]) -> Path:
    raiz = Path(tempfile.mkdtemp(prefix="iac-ds-"))
    for nombre, contenido in archivos.items():
        ruta = raiz / nombre
        ruta.parent.mkdir(parents=True, exist_ok=True)
        ruta.write_text(contenido, encoding="utf-8")
    return raiz


def _ejecutar(raiz: Path) -> subprocess.CompletedProcess:
    return subprocess.run(
        [sys.executable, str(SCRIPT), "--raiz", str(raiz), "--sin-git"],
        capture_output=True, text=True, timeout=60,
    )


LIMPIO = """
resource "aws_s3_bucket" "state" { bucket = "nexum-state" }
resource "aws_s3_bucket_server_side_encryption_configuration" "state" {
  bucket = aws_s3_bucket.state.id
  rule { apply_server_side_encryption_by_default { sse_algorithm = "AES256" } }
}
resource "aws_security_group_rule" "interno" {
  type = "ingress"
  cidr_blocks = ["10.0.0.0/16"]
}
"""


class EscaneoIaC(unittest.TestCase):
    def test_configuracion_limpia_pasa(self) -> None:
        r = _ejecutar(_raiz({"main.tf": LIMPIO}))
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertIn("IAC_DEVSECOPS_PASS", r.stdout)

    def test_security_group_publico_falla(self) -> None:
        contenido = LIMPIO + '\nresource "aws_security_group_rule" "abierto" {\n  type = "ingress"\n  cidr_blocks = ["0.0.0.0/0"]\n}\n'
        r = _ejecutar(_raiz({"main.tf": contenido}))
        self.assertEqual(r.returncode, 1)
        self.assertIn("SG_PUBLICO", r.stdout)

    def test_iam_wildcard_falla(self) -> None:
        contenido = LIMPIO + '\nresource "aws_iam_policy" "malo" {\n  policy = jsonencode({ Statement = [{ Action = "*", Resource = "*" }] })\n}\n'
        r = _ejecutar(_raiz({"iam.tf": contenido}))
        self.assertEqual(r.returncode, 1)
        self.assertIn("IAM_WILDCARD", r.stdout)

    def test_force_destroy_falla(self) -> None:
        contenido = LIMPIO + '\nresource "aws_s3_bucket" "persistente" {\n  bucket = "x"\n  force_destroy = true\n}\n'
        r = _ejecutar(_raiz({"main.tf": contenido}))
        self.assertEqual(r.returncode, 1)
        self.assertIn("FORCE_DESTROY", r.stdout)

    def test_acl_publica_falla(self) -> None:
        contenido = LIMPIO + '\nresource "aws_s3_bucket_acl" "publico" {\n  acl = "public-read"\n}\n'
        r = _ejecutar(_raiz({"main.tf": contenido}))
        self.assertEqual(r.returncode, 1)
        self.assertIn("ACL_PUBLICA", r.stdout)

    def test_bucket_sin_cifrado_falla(self) -> None:
        contenido = 'resource "aws_s3_bucket" "state" { bucket = "nexum-state" }\n'
        r = _ejecutar(_raiz({"main.tf": contenido}))
        self.assertEqual(r.returncode, 1)
        self.assertIn("S3_SIN_CIFRADO", r.stdout)

    def test_salida_json_contiene_hallazgos(self) -> None:
        raiz = _raiz({"main.tf": LIMPIO + '\nresource "aws_security_group_rule" "abierto" {\n  cidr_blocks = ["::/0"]\n}\n'})
        salida = Path(tempfile.mkdtemp(prefix="iac-json-")) / "hallazgos.json"
        r = subprocess.run(
            [sys.executable, str(SCRIPT), "--raiz", str(raiz), "--sin-git", "--json", str(salida)],
            capture_output=True, text=True, timeout=60,
        )
        self.assertEqual(r.returncode, 1)
        self.assertIn("SG_PUBLICO", salida.read_text(encoding="utf-8"))


if __name__ == "__main__":
    unittest.main()
