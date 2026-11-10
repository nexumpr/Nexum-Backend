"""Pruebas del verificador del plan de bootstrap (I-070).

No ejecutan Terraform ni consultan AWS: construyen estructuras
`terraform show -json` sinteticas y comprueban que el verificador falla cerrado.
"""

from __future__ import annotations

import hashlib
import json
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

SCRIPT = Path(__file__).resolve().parents[2] / "herramientas" / "verificar_plan_bootstrap.py"

DIRECCIONES = (
    "aws_s3_bucket.state",
    "aws_s3_bucket_versioning.state",
    "aws_s3_bucket_server_side_encryption_configuration.state",
    "aws_s3_bucket_public_access_block.state",
    "aws_s3_bucket_ownership_controls.state",
    "aws_s3_bucket_policy.state",
    "aws_s3_bucket_lifecycle_configuration.state",
    "terraform_data.validacion_bootstrap",
)
BUCKET = "nexum-h08i-tfstate-713797602090"


def _cambio(direccion: str, acciones: list[str], proveedor: str | None = None) -> dict:
    tipo = direccion.split(".")[0]
    return {
        "address": direccion,
        "type": tipo,
        "provider_name": proveedor
        or ("registry.terraform.io/hashicorp/aws" if tipo.startswith("aws_") else "terraform.io/builtin/terraform"),
        "change": {"actions": list(acciones)},
    }


def plan_valido() -> dict:
    return {
        "applyable": True,
        "complete": True,
        "errored": False,
        "variables": {
            "sufijo_cuenta": {"value": "713797602090"},
            "region": {"value": "us-east-2"},
        },
        "planned_values": {
            "root_module": {
                "resources": [
                    {"address": "aws_s3_bucket.state", "values": {"bucket": BUCKET}},
                ]
            }
        },
        "resource_changes": [_cambio(d, ["create"]) for d in DIRECCIONES],
    }


class VerificadorPlanBootstrap(unittest.TestCase):
    def setUp(self) -> None:
        self.dir = Path(tempfile.mkdtemp(prefix="plan-bootstrap-"))
        self.tfplan = self.dir / "bootstrap.tfplan"
        self.tfplan.write_bytes(b"plan sintetico")
        self.plan_json = self.dir / "plan.json"

    def _sha(self) -> str:
        return hashlib.sha256(self.tfplan.read_bytes()).hexdigest().upper()

    def _ejecutar(self, plan: dict | str, sha: str) -> subprocess.CompletedProcess:
        contenido = plan if isinstance(plan, str) else json.dumps(plan)
        self.plan_json.write_text(contenido, encoding="utf-8")
        return subprocess.run(
            [
                sys.executable,
                str(SCRIPT),
                "--plan-json",
                str(self.plan_json),
                "--tfplan",
                str(self.tfplan),
                "--sha-esperado",
                sha,
            ],
            capture_output=True,
            text=True,
        )

    def test_plan_valido_pasa(self) -> None:
        proceso = self._ejecutar(plan_valido(), self._sha())
        self.assertEqual(proceso.returncode, 0, proceso.stdout + proceso.stderr)
        self.assertIn('"resultado": "PASS"', proceso.stdout)

    def test_sha_distinto_falla(self) -> None:
        proceso = self._ejecutar(plan_valido(), "0" * 64)
        self.assertEqual(proceso.returncode, 1)
        self.assertIn("SHA_NO_COINCIDE", proceso.stdout)

    def test_plan_erroreado_falla(self) -> None:
        plan = plan_valido()
        plan["errored"] = True
        plan["applyable"] = False
        plan["complete"] = False
        proceso = self._ejecutar(plan, self._sha())
        self.assertEqual(proceso.returncode, 1)
        for marca in ("PLAN_ERROREADO", "PLAN_INCOMPLETO", "PLAN_NO_APLICABLE"):
            self.assertIn(marca, proceso.stdout)

    def test_destruccion_falla(self) -> None:
        plan = plan_valido()
        plan["resource_changes"][0]["change"]["actions"] = ["delete"]
        proceso = self._ejecutar(plan, self._sha())
        self.assertEqual(proceso.returncode, 1)
        self.assertIn("DESTRUCCION aws_s3_bucket.state", proceso.stdout)

    def test_reemplazo_falla(self) -> None:
        plan = plan_valido()
        plan["resource_changes"][0]["change"]["actions"] = ["delete", "create"]
        proceso = self._ejecutar(plan, self._sha())
        self.assertEqual(proceso.returncode, 1)
        self.assertIn("DESTRUCCION aws_s3_bucket.state", proceso.stdout)

    def test_recurso_faltante_falla(self) -> None:
        plan = plan_valido()
        plan["resource_changes"] = plan["resource_changes"][1:]
        proceso = self._ejecutar(plan, self._sha())
        self.assertEqual(proceso.returncode, 1)
        self.assertIn("RECURSO_FALTANTE aws_s3_bucket.state", proceso.stdout)

    def test_recurso_extra_falla(self) -> None:
        plan = plan_valido()
        plan["resource_changes"].append(_cambio("aws_dynamodb_table.lock", ["create"]))
        proceso = self._ejecutar(plan, self._sha())
        self.assertEqual(proceso.returncode, 1)
        self.assertIn("RECURSO_NO_PREVISTO aws_dynamodb_table.lock", proceso.stdout)

    def test_proveedor_inesperado_falla(self) -> None:
        plan = plan_valido()
        plan["resource_changes"][0]["provider_name"] = "registry.terraform.io/hashicorp/random"
        proceso = self._ejecutar(plan, self._sha())
        self.assertEqual(proceso.returncode, 1)
        self.assertIn("PROVEEDOR_INESPERADO aws_s3_bucket.state", proceso.stdout)

    def test_sufijo_vacio_falla(self) -> None:
        plan = plan_valido()
        plan["variables"]["sufijo_cuenta"]["value"] = ""
        proceso = self._ejecutar(plan, self._sha())
        self.assertEqual(proceso.returncode, 1)
        self.assertIn("SUFIJO_CUENTA_VACIO", proceso.stdout)

    def test_bucket_inesperado_falla(self) -> None:
        plan = plan_valido()
        plan["planned_values"]["root_module"]["resources"][0]["values"]["bucket"] = "otro-bucket"
        proceso = self._ejecutar(plan, self._sha())
        self.assertEqual(proceso.returncode, 1)
        self.assertIn("BUCKET_INESPERADO", proceso.stdout)

    def test_sha_malformado_es_uso_invalido(self) -> None:
        proceso = self._ejecutar(plan_valido(), "ABC")
        self.assertEqual(proceso.returncode, 2)
        self.assertIn("USO_INVALIDO", proceso.stdout)

    def test_json_invalido_es_uso_invalido(self) -> None:
        proceso = self._ejecutar("{no es json", self._sha())
        self.assertEqual(proceso.returncode, 2)
        self.assertIn("USO_INVALIDO", proceso.stdout)

    def test_archivo_inexistente_es_uso_invalido(self) -> None:
        self.tfplan.unlink()
        proceso = self._ejecutar(plan_valido(), "A" * 64)
        self.assertEqual(proceso.returncode, 2)


if __name__ == "__main__":
    unittest.main()
