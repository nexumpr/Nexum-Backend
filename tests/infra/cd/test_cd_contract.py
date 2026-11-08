"""Contrato de CD de I-090 (INF-090-01..06).

Verifica el workflow real y su documentación sin ejecutar despliegues ni invocar AWS.
La ejecución real permanece NOT_RUN.
"""

from __future__ import annotations

import re
import unittest
from pathlib import Path

RAIZ = Path(__file__).resolve().parents[3]
CD = RAIZ / ".github" / "workflows" / "cd-backend.yml"
CI = RAIZ / ".github" / "workflows" / "go-backend.yml"
CONTROL = RAIZ / "sdd" / "_control" / "H08"
POLITICA = CONTROL / "POLITICA_RELEASES.md"
ROLLBACK = CONTROL / "PLAN_ROLLBACK.md"
EVIDENCIA = CONTROL / "EVIDENCIA_DEPLOY.md"
AYUDA = RAIZ / "herramientas" / "verificar_autorizacion_despliegue.py"


def _leer(ruta: Path) -> str:
    return ruta.read_text(encoding="utf-8")


def _sin_comentarios(texto: str) -> str:
    return "\n".join(linea for linea in texto.splitlines() if not linea.lstrip().startswith("#"))


class DisparadorYVersion(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.crudo = _leer(CD)
        cls.texto = _sin_comentarios(cls.crudo)

    def test_solo_workflow_dispatch(self) -> None:
        self.assertIn("workflow_dispatch:", self.texto)
        for prohibido in ("pull_request:", "push:", "workflow_run:", "pull_request_target", "schedule:"):
            self.assertNotIn(prohibido, self.texto, prohibido)

    def test_ref_main_y_condicion_de_dispatch(self) -> None:
        self.assertIn("github.ref == 'refs/heads/main'", self.texto)
        self.assertIn("github.event_name == 'workflow_dispatch'", self.texto)

    def test_inputs_de_release_obligatorios(self) -> None:
        for entrada in ("release_id", "sha", "run_id", "sha256_binario", "autorizacion", "lt_version_previa", "confirmar"):
            self.assertIn(f"{entrada}:", self.texto, entrada)
            bloque = self.texto.split(f"{entrada}:", 1)[1]
            self.assertIn("required: true", bloque.split("type:", 1)[0], entrada)

    def test_concurrencia_sin_cancelar_despliegues(self) -> None:
        self.assertIn("cancel-in-progress: false", self.texto)


class SeguridadDelWorkflow(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.texto = _sin_comentarios(_leer(CD))

    def test_permisos_minimos(self) -> None:
        self.assertRegex(self.texto, r"(?m)^permissions:\s*\n\s+contents:\s*read\s*$")
        self.assertIn("actions: read", self.texto)
        self.assertIn("id-token: write", self.texto)
        self.assertNotIn("contents: write", self.texto)
        self.assertNotIn("pull-requests: write", self.texto)

    def test_sin_secretos_ni_credenciales_estaticas(self) -> None:
        for prohibido in ("${{ secrets.", "secrets.", "AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", "access-key-id", "secret-access-key"):
            self.assertNotIn(prohibido, self.texto, prohibido)

    def test_oidc_con_rol_por_variable(self) -> None:
        self.assertIn("aws-actions/configure-aws-credentials@e1253824e5c10ff9df46874f81ed3ec929e19cfd", self.texto)
        self.assertIn("role-to-assume: ${{ vars.AWS_DEPLOY_ROLE_ARN }}", self.texto)
        self.assertIn("aws-region: us-east-2", self.texto)

    def test_acciones_fijadas_por_sha(self) -> None:
        usos = re.findall(r"(?m)^\s+uses:\s*(\S+)", self.texto)
        self.assertGreaterEqual(len(usos), 3)
        for uso in usos:
            self.assertRegex(uso, r"^[\w./-]+@[0-9a-f]{40}$", uso)

    def test_entorno_protegido(self) -> None:
        self.assertIn("environment: aws-deploy", self.texto)
        self.assertIn("vars.CD_HABILITADO == 'true'", self.texto)

    def test_sin_terraform_ni_docker(self) -> None:
        for prohibido in ("terraform", "docker", "aws cloudformation"):
            self.assertNotIn(prohibido, self.texto.lower())


class PromocionYAutorizacion(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.texto = _sin_comentarios(_leer(CD))
        cls.ci = _sin_comentarios(_leer(CI))

    def test_promueve_artefacto_verificado_por_sha(self) -> None:
        self.assertIn("binario-${{ inputs.sha }}", self.texto)
        self.assertIn("sbom-spdx-${{ inputs.sha }}", self.texto)
        self.assertIn("sha256sum", self.texto)
        self.assertIn('= "${{ inputs.sha256_binario }}"', self.texto)

    def test_ci_publica_el_binario(self) -> None:
        self.assertIn("name: binario-${{ github.sha }}", self.ci)
        self.assertIn("GOOS=linux GOARCH=arm64 go build", self.ci)
        self.assertIn("dist/binario.sha256", self.ci)

    def test_autorizacion_ligada_al_sha(self) -> None:
        self.assertIn("verificar_autorizacion_despliegue.py", self.texto)
        self.assertIn('--frase "${{ inputs.autorizacion }}"', self.texto)
        ayuda = _leer(AYUDA)
        self.assertIn('f"AUTORIZO_DESPLIEGUE_{sha}"', ayuda)
        self.assertIn("CONFIRMACION_AUSENTE", ayuda)
        self.assertIn("CD_NO_HABILITADO", ayuda)


class DespliegueSanoyRollback(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.texto = _sin_comentarios(_leer(CD))

    def test_exige_dos_nodos_sanos_antes_y_despues(self) -> None:
        self.assertEqual(self.texto.count("select(.LifecycleState==\"InService\")"), 2)
        self.assertEqual(self.texto.count("select(.TargetHealth.State==\"healthy\")"), 2)
        self.assertEqual(self.texto.count('test "$IN_SERVICE" -eq 2'), 2)
        self.assertEqual(self.texto.count('test "$SALUDABLES" -eq 2'), 2)

    def test_instance_refresh_conserva_capacidad(self) -> None:
        self.assertIn('"MinHealthyPercentage":100', self.texto)
        self.assertIn("start-instance-refresh", self.texto)
        for estado in ("Failed", "Cancelled", "RollbackInProgress", "RollbackFailed"):
            self.assertIn(estado, self.texto)

    def test_health_http_https(self) -> None:
        self.assertIn("/api/v1/health/live", self.texto)
        self.assertIn("--proto '=https'", self.texto)
        self.assertIn('test "$CODIGO" = "200"', self.texto)

    def test_rollback_a_la_version_previa(self) -> None:
        self.assertIn("if: failure() && inputs.lt_version_previa != ''", self.texto)
        self.assertIn("update-auto-scaling-group", self.texto)
        self.assertIn('Version="${{ inputs.lt_version_previa }}"', self.texto)

    def test_evidencia_con_sha(self) -> None:
        self.assertIn("evidencia-deploy-${{ inputs.sha }}", self.texto)
        self.assertIn("desplegado_por", self.texto)
        self.assertIn("sha256_binario", self.texto)


class DocumentosDeCD(unittest.TestCase):
    def test_politica_de_releases(self) -> None:
        texto = _leer(POLITICA)
        for ancla in ("SemVer", "/api/v1", "+<sha12>", "workflow_dispatch", "aws-deploy", "CD_HABILITADO", "AUTORIZO_DESPLIEGUE_", "PENDIENTE_APLICACION_HUMANA", "NOT_RUN"):
            self.assertIn(ancla, texto, ancla)

    def test_plan_de_rollback(self) -> None:
        texto = _leer(ROLLBACK)
        for ancla in ("Launch Template", "MinHealthyPercentage=100", "compatibles hacia atrás", "independiente", "no se ha ensayado", "prevent_destroy"):
            self.assertIn(ancla, texto, ancla)

    def test_evidencia_deploy_sin_declarar_ejecucion(self) -> None:
        texto = _leer(EVIDENCIA)
        self.assertIn("NOT_RUN", texto)
        self.assertIn("DEPLOYED_VERIFIED", texto)
        self.assertIn("total_count: 0", _leer(POLITICA))
        self.assertIn("No se declara `DEPLOYED_VERIFIED`", texto)


if __name__ == "__main__":
    unittest.main()
