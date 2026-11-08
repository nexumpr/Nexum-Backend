"""Pruebas de las herramientas de decisión del CD I-090 (sin AWS)."""

from __future__ import annotations

import base64
import json
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

RAIZ = Path(__file__).resolve().parents[2] / "herramientas"
SHA = "a" * 40
DIGEST = "b" * 64
RELEASE = f"v0.1.0+{SHA[:12]}"
BUCKET = "nexum-artefactos"
URI = f"s3://{BUCKET}/releases/{RELEASE}/nexum-backend-linux-arm64"
USERDATA = base64.b64encode(f"BINARIO_URI=\"{URI}\"\nBINARIO_SHA=\"{DIGEST}\"\n".encode()).decode()


def _json(datos: dict) -> Path:
    carpeta = Path(tempfile.mkdtemp(prefix="cd-i090-"))
    ruta = carpeta / "datos.json"
    ruta.write_text(json.dumps(datos), encoding="utf-8")
    return ruta


def _ejecutar(script: str, *args: str) -> subprocess.CompletedProcess:
    return subprocess.run([sys.executable, str(RAIZ / script), *args], capture_output=True, text=True, timeout=60)


class RunCI(unittest.TestCase):
    def _run(self, datos: dict, sha: str = SHA, artefactos: dict | None = None) -> subprocess.CompletedProcess:
        args = [str(_json(datos)), sha, "--workflow-path", ".github/workflows/go-backend.yml", "--evento", "push", "--rama", "main"]
        if artefactos is not None:
            args += ["--artefactos-json", str(_json(artefactos)), "--artefactos", f"binario-{sha},sbom-spdx-{sha}"]
        return _ejecutar("verificar_run_ci.py", *args)

    def _valido(self, **cambios: object) -> dict:
        datos = {
            "status": "completed",
            "conclusion": "success",
            "head_sha": SHA,
            "head_branch": "main",
            "event": "push",
            "path": ".github/workflows/go-backend.yml",
            "repository": {"full_name": "nexumpr/Nexum-Backend"},
        }
        datos.update(cambios)
        return datos

    def test_run_autentico_con_artefactos_pasa(self) -> None:
        artefactos = {"artifacts": [{"name": f"binario-{SHA}"}, {"name": f"sbom-spdx-{SHA}"}]}
        r = self._run(self._valido(), artefactos=artefactos)
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)

    def test_workflow_incorrecto_falla(self) -> None:
        r = self._run(self._valido(path=".github/workflows/otro.yml"))
        self.assertEqual(r.returncode, 1)
        self.assertIn("RUN_WORKFLOW_NO_AUTORIZADO", r.stdout)

    def test_evento_incorrecto_falla(self) -> None:
        r = self._run(self._valido(event="pull_request"))
        self.assertEqual(r.returncode, 1)
        self.assertIn("RUN_EVENTO_NO_AUTORIZADO", r.stdout)

    def test_artefacto_faltante_falla(self) -> None:
        artefactos = {"artifacts": [{"name": f"binario-{SHA}"}]}
        r = self._run(self._valido(), artefactos=artefactos)
        self.assertEqual(r.returncode, 1)
        self.assertIn("ARTEFACTO_FALTANTE sbom-spdx", r.stdout)


class ReleaseEnLT(unittest.TestCase):
    def _lt(self, userdata: str | None = USERDATA, version: int = 7) -> dict:
        return {"LaunchTemplateVersions": [{"VersionNumber": version, "LaunchTemplateData": {"UserData": userdata}}]}

    def _run(self, datos: dict, version: str = "7") -> subprocess.CompletedProcess:
        return _ejecutar(
            "verificar_lt_release.py", str(_json(datos)),
            "--bucket", BUCKET, "--release-id", RELEASE, "--sha256", DIGEST, "--version", version,
        )

    def test_lt_con_release_pasa(self) -> None:
        r = self._run(self._lt())
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)

    def test_lt_sin_uri_falla(self) -> None:
        userdata = base64.b64encode(f"BINARIO_SHA=\"{DIGEST}\"".encode()).decode()
        r = self._run(self._lt(userdata))
        self.assertEqual(r.returncode, 1)
        self.assertIn("LT_SIN_URI_DEL_RELEASE", r.stdout)

    def test_lt_sin_digest_falla(self) -> None:
        userdata = base64.b64encode(f"BINARIO_URI=\"{URI}\"".encode()).decode()
        r = self._run(self._lt(userdata))
        self.assertEqual(r.returncode, 1)
        self.assertIn("LT_SIN_DIGEST_DEL_RELEASE", r.stdout)

    def test_lt_version_distinta_falla(self) -> None:
        r = self._run(self._lt(), version="8")
        self.assertEqual(r.returncode, 1)
        self.assertIn("LT_VERSION_DISTINTA", r.stdout)


class InstanciasRelease(unittest.TestCase):
    def _run(self, lineas: list[str], instancias: int = 2) -> subprocess.CompletedProcess:
        carpeta = Path(tempfile.mkdtemp(prefix="ud-"))
        ruta = carpeta / "userdata.txt"
        ruta.write_text("\n".join(lineas), encoding="utf-8")
        return _ejecutar(
            "verificar_instancias_release.py", str(ruta),
            "--bucket", BUCKET, "--release-id", RELEASE, "--sha256", DIGEST, "--instancias", str(instancias),
        )

    def test_dos_instancias_correctas_pasan(self) -> None:
        r = self._run([USERDATA, USERDATA])
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)

    def test_instancia_con_release_antiguo_falla(self) -> None:
        vieja = base64.b64encode(b"s3://bucket/releases/viejo/nexum-backend-linux-arm64\n" + DIGEST.encode()).decode()
        r = self._run([USERDATA, vieja])
        self.assertEqual(r.returncode, 1)
        self.assertIn("INSTANCIA_SIN_URI 2", r.stdout)

    def test_numero_de_instancias_distinto_falla(self) -> None:
        r = self._run([USERDATA])
        self.assertEqual(r.returncode, 1)
        self.assertIn("INSTANCIAS_INESPERADAS", r.stdout)


class ObjetoS3(unittest.TestCase):
    def test_checksum_correcto_pasa(self) -> None:
        esperado = base64.b64encode(bytes.fromhex(DIGEST)).decode()
        r = _ejecutar("verificar_objeto_s3.py", str(_json({"ChecksumSHA256": esperado})), "--sha256", DIGEST)
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)

    def test_checksum_distinto_falla(self) -> None:
        r = _ejecutar("verificar_objeto_s3.py", str(_json({"ChecksumSHA256": "AAAA"})), "--sha256", DIGEST)
        self.assertEqual(r.returncode, 1)
        self.assertIn("CHECKSUM_NO_COINCIDE", r.stdout)


class EstadoASG(unittest.TestCase):
    def _run(self, datos: dict) -> subprocess.CompletedProcess:
        salida = Path(tempfile.mkdtemp(prefix="asg-")) / "salida.json"
        return _ejecutar("estado_asg.py", str(_json(datos)), "--minimo", "2", "--nodos", "2", "--salida", str(salida))

    def test_dos_nodos_y_lt_pasa(self) -> None:
        datos = {"AutoScalingGroups": [{"MinSize": 2, "MaxSize": 2, "LaunchTemplate": {"Version": "7"},
                                        "Instances": [{"InstanceId": "i-1", "LifecycleState": "InService"},
                                                      {"InstanceId": "i-2", "LifecycleState": "InService"}]}]}
        r = self._run(datos)
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertIn('"launch_template_version": "7"', r.stdout)

    def test_un_nodo_falla(self) -> None:
        datos = {"AutoScalingGroups": [{"MinSize": 2, "MaxSize": 2, "LaunchTemplate": {"Version": "7"},
                                        "Instances": [{"InstanceId": "i-1", "LifecycleState": "InService"}]}]}
        r = self._run(datos)
        self.assertEqual(r.returncode, 1)
        self.assertIn("NODOS_IN_SERVICE", r.stdout)


class DecisionRollback(unittest.TestCase):
    def _run(self, datos: dict) -> subprocess.CompletedProcess:
        salida = Path(tempfile.mkdtemp(prefix="rb-")) / "salida.json"
        return _ejecutar("decidir_rollback.py", str(_json(datos)), "--salida", str(salida))

    def test_refresh_activo_exige_cancelar(self) -> None:
        r = self._run({"InstanceRefreshes": [{"InstanceRefreshId": "r1", "Status": "InProgress"}]})
        self.assertEqual(r.returncode, 0)
        self.assertIn("CANCELAR_Y_ESPERAR", r.stdout)

    def test_sin_refresh_activo_procede(self) -> None:
        r = self._run({"InstanceRefreshes": [{"InstanceRefreshId": "r1", "Status": "Failed"}]})
        self.assertEqual(r.returncode, 0)
        self.assertIn("PROCEDE", r.stdout)


class InyeccionDeInputs(unittest.TestCase):
    def _auth(self, **extra: str) -> subprocess.CompletedProcess:
        argumentos = [
            "verificar_autorizacion_despliegue.py",
            "--sha", SHA,
            "--release-id", RELEASE,
            "--frase", f"AUTORIZO_DESPLIEGUE_{SHA}",
            "--confirmar", "SI",
            "--habilitado", "true",
            "--sha256-binario", DIGEST,
        ]
        for clave, valor in extra.items():
            argumentos += [clave, valor]
        return _ejecutar(*argumentos)

    def test_sha_con_metacaracteres_falla(self) -> None:
        r = self._auth(**{"--sha": SHA + "; rm -rf /"})
        self.assertEqual(r.returncode, 1)
        self.assertIn("SHA_INVALIDO", r.stdout)

    def test_frase_con_sustitucion_falla(self) -> None:
        r = self._auth(**{"--frase": "AUTORIZO_DESPLIEGUE_$(id)"})
        self.assertEqual(r.returncode, 1)
        self.assertIn("FRASE_NO_CORRESPONDE_AL_SHA", r.stdout)

    def test_release_con_comillas_falla(self) -> None:
        r = self._auth(**{"--release-id": 'v1.0.0+aaaaaaaaaaaa"; whoami; echo "'})
        self.assertEqual(r.returncode, 1)
        self.assertIn("RELEASE_ID_INVALIDO", r.stdout)

    def test_confirmar_con_comando_falla(self) -> None:
        r = self._auth(**{"--confirmar": "SI; reboot"})
        self.assertEqual(r.returncode, 1)
        self.assertIn("CONFIRMACION_AUSENTE", r.stdout)


if __name__ == "__main__":
    unittest.main()
