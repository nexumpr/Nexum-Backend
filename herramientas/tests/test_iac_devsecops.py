"""Pruebas del escaneo IaC DevSecOps (I-100)."""

from __future__ import annotations

import json
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


def _ejecutar(raiz: Path, sin_git: bool = True, json_salida: Path | None = None) -> subprocess.CompletedProcess:
    argumentos = [sys.executable, str(SCRIPT), "--raiz", str(raiz)]
    if sin_git:
        argumentos.append("--sin-git")
    if json_salida is not None:
        argumentos += ["--json", str(json_salida)]
    return subprocess.run(argumentos, capture_output=True, text=True, timeout=60)


def _reglas(salida: Path) -> list[str]:
    return [h["regla"] for h in json.loads(salida.read_text(encoding="utf-8"))["hallazgos"]]


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

    def test_bucket_sin_cifrado_no_se_oculta_por_referencia_ajena(self) -> None:
        contenido = """
resource "aws_s3_bucket" "secure" { bucket = "secure" }
resource "aws_s3_bucket" "exposed" { bucket = "exposed" }
resource "aws_s3_bucket_server_side_encryption_configuration" "secure" {
  bucket = aws_s3_bucket.secure.id
  rule { apply_server_side_encryption_by_default { sse_algorithm = "AES256" } }
}
resource "aws_s3_bucket_policy" "ajena" {
  bucket = aws_s3_bucket.exposed.id
  policy = jsonencode({ Version = "2012-10-17" })
}
"""
        salida = Path(tempfile.mkdtemp(prefix="iac-json-")) / "h.json"
        r = _ejecutar(_raiz({"main.tf": contenido}), json_salida=salida)
        self.assertEqual(r.returncode, 1)
        reglas = _reglas(salida)
        # Sólo `exposed` es un hallazgo; `secure` no aparece como bucket sin cifrado.
        self.assertIn("S3_SIN_CIFRADO", reglas)
        self.assertEqual(reglas.count("S3_SIN_CIFRADO"), 1)

    def test_ambos_buckets_cifrados_pasan(self) -> None:
        contenido = """
resource "aws_s3_bucket" "a" { bucket = "a" }
resource "aws_s3_bucket" "b" { bucket = "b" }
resource "aws_s3_bucket_server_side_encryption_configuration" "a" {
  bucket = aws_s3_bucket.a.id
  rule { apply_server_side_encryption_by_default { sse_algorithm = "AES256" } }
}
resource "aws_s3_bucket_server_side_encryption_configuration" "b" {
  bucket = aws_s3_bucket.b.id
  rule { apply_server_side_encryption_by_default { sse_algorithm = "aws:kms" } }
}
"""
        r = _ejecutar(_raiz({"main.tf": contenido}))
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)

    def test_cifrado_sin_sse_no_protege(self) -> None:
        contenido = """
resource "aws_s3_bucket" "state" { bucket = "state" }
resource "aws_s3_bucket_server_side_encryption_configuration" "vacio" {
  bucket = aws_s3_bucket.state.id
}
"""
        r = _ejecutar(_raiz({"main.tf": contenido}))
        self.assertEqual(r.returncode, 1)
        self.assertIn("S3_SIN_CIFRADO", r.stdout)

    def test_cifrado_dinamico_no_se_asume_seguro(self) -> None:
        contenido = """
resource "aws_s3_bucket" "state" { bucket = "state" }
resource "aws_s3_bucket_server_side_encryption_configuration" "c" {
  bucket = var.bucket_dinamico
  rule { apply_server_side_encryption_by_default { sse_algorithm = "AES256" } }
}
"""
        r = _ejecutar(_raiz({"main.tf": contenido}))
        self.assertEqual(r.returncode, 1)
        self.assertIn("S3_CIFRADO_NO_VERIFICABLE", r.stdout)

    def test_egress_publico_no_es_ingreso_publico(self) -> None:
        contenido = """
resource "aws_security_group" "sg" {
  ingress {
    cidr_blocks = ["10.0.0.0/16"]
  }
  egress {
    cidr_blocks = ["0.0.0.0/0"]
  }
}
"""
        r = _ejecutar(_raiz({"main.tf": contenido}))
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)

    def test_ingress_publico_en_security_group_falla(self) -> None:
        contenido = """
resource "aws_security_group" "sg" {
  ingress {
    cidr_blocks = ["0.0.0.0/0"]
  }
  egress {
    cidr_blocks = ["10.0.0.0/16"]
  }
}
"""
        r = _ejecutar(_raiz({"main.tf": contenido}))
        self.assertEqual(r.returncode, 1)
        self.assertIn("SG_PUBLICO", r.stdout)

    def test_ingress_privado_y_egress_publico_en_archivo_no_confunde(self) -> None:
        contenido = """
resource "aws_security_group_rule" "entrada" {
  type = "ingress"
  cidr_blocks = ["10.0.0.0/16"]
}
resource "aws_security_group_rule" "salida" {
  type = "egress"
  cidr_blocks = ["0.0.0.0/0"]
}
"""
        r = _ejecutar(_raiz({"main.tf": contenido}))
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)

    def test_ingress_publico_en_rule_falla(self) -> None:
        contenido = """
resource "aws_security_group_rule" "entrada" {
  type = "ingress"
  cidr_blocks = ["0.0.0.0/0"]
}
"""
        r = _ejecutar(_raiz({"main.tf": contenido}))
        self.assertEqual(r.returncode, 1)
        self.assertIn("SG_PUBLICO", r.stdout)

    def test_vpc_ingress_rule_ipv6_falla(self) -> None:
        contenido = """
resource "aws_vpc_security_group_ingress_rule" "v6" {
  cidr_ipv6 = "::/0"
}
"""
        r = _ejecutar(_raiz({"main.tf": contenido}))
        self.assertEqual(r.returncode, 1)
        self.assertIn("SG_PUBLICO", r.stdout)

    def test_comentario_con_cidr_publico_no_falla(self) -> None:
        contenido = LIMPIO + "\n# cidr_blocks = [\"0.0.0.0/0\"]\n// cidr_ipv4 = \"0.0.0.0/0\"\n"
        r = _ejecutar(_raiz({"main.tf": contenido}))
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)

    def test_iam_wildcard_en_lista_y_escalar_falla(self) -> None:
        lista = LIMPIO + '\nresource "aws_iam_policy" "m" {\n  policy = jsonencode({ Statement = [{ Action = ["*"], Resource = ["*"] }] })\n}\n'
        escalar = LIMPIO + '\nresource "aws_iam_policy" "m" {\n  policy = jsonencode({ Statement = [{ Action = "*", Resource = "*" }] })\n}\n'
        for contenido in (lista, escalar):
            r = _ejecutar(_raiz({"iam.tf": contenido}))
            self.assertEqual(r.returncode, 1)
            self.assertIn("IAM_WILDCARD", r.stdout)

    def test_force_destroy_y_acl_publica(self) -> None:
        contenido = LIMPIO + (
            '\nresource "aws_s3_bucket" "p" { bucket = "p"\n  force_destroy = true\n}\n'
            'resource "aws_s3_bucket_acl" "a" { acl = "public-read" }\n'
        )
        r = _ejecutar(_raiz({"main.tf": contenido}))
        self.assertEqual(r.returncode, 1)
        self.assertIn("FORCE_DESTROY", r.stdout)
        self.assertIn("ACL_PUBLICA", r.stdout)

    def test_fallo_de_git_falla_cerrado(self) -> None:
        r = _ejecutar(_raiz({"main.tf": LIMPIO}), sin_git=False)
        self.assertEqual(r.returncode, 1)
        self.assertIn("GIT_CHECK_FAILED", r.stdout)

    def test_salida_json_contiene_error_git_nulo(self) -> None:
        raiz = _raiz({"main.tf": LIMPIO + '\nresource "aws_security_group_rule" "a" {\n  type = "ingress"\n  cidr_blocks = ["::/0"]\n}\n'})
        salida = Path(tempfile.mkdtemp(prefix="iac-json-")) / "h.json"
        r = _ejecutar(raiz, json_salida=salida)
        self.assertEqual(r.returncode, 1)
        datos = json.loads(salida.read_text(encoding="utf-8"))
        self.assertIsNone(datos["git_error"])
        self.assertIn("SG_PUBLICO", [h["regla"] for h in datos["hallazgos"]])


if __name__ == "__main__":
    unittest.main()
