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
        for entrada in ("release_id", "sha", "run_id", "sha256_binario", "autorizacion", "lt_version", "confirmar"):
            self.assertIn(f"{entrada}:", self.texto, entrada)
            bloque = self.texto.split(f"{entrada}:", 1)[1]
            self.assertIn("required: true", bloque.split("type:", 1)[0], entrada)
        self.assertIn("lt_version_previa:", self.texto)
        bloque = self.texto.split("lt_version_previa:", 1)[1]
        self.assertIn("required: false", bloque.split("type:", 1)[0])

    def test_inputs_no_se_interpolan_en_scripts(self) -> None:
        en_run = False
        for linea in self.crudo.splitlines():
            desnuda = linea.strip()
            if desnuda.startswith("run:"):
                en_run = desnuda.endswith("|")
                continue
            if en_run and linea.startswith("      - name:"):
                en_run = False
            if en_run:
                self.assertNotIn("${{ inputs.", linea, linea)
                self.assertNotIn("${{ github.actor }}", linea, linea)

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
        self.assertIn('= "$IN_DIGEST"', self.texto)

    def test_ci_publica_el_binario(self) -> None:
        self.assertIn("name: binario-${{ github.sha }}", self.ci)
        self.assertIn("GOOS=linux GOARCH=arm64 go build", self.ci)
        self.assertIn("dist/binario.sha256", self.ci)

    def test_autorizacion_ligada_al_sha(self) -> None:
        self.assertIn("verificar_autorizacion_despliegue.py", self.texto)
        self.assertIn('--frase "$IN_AUTORIZACION"', self.texto)
        self.assertIn('--lt-version "$IN_LT_VERSION"', self.texto)
        ayuda = _leer(AYUDA)
        self.assertIn('f"AUTORIZO_DESPLIEGUE_{sha}"', ayuda)
        self.assertIn("CONFIRMACION_AUSENTE", ayuda)
        self.assertIn("CD_NO_HABILITADO", ayuda)
        self.assertIn("LT_VERSION_INVALIDA", ayuda)

    def test_entrega_a_ec2_verificable_y_falla_cerrado(self) -> None:
        self.assertIn("ARTIFACT_BUCKET", self.texto)
        self.assertIn("ARTIFACT_DELIVERY_BLOCKED", self.texto)
        self.assertIn("s3api put-object", self.texto)
        self.assertIn('--if-none-match "*"', self.texto)
        self.assertIn("decidir_publicacion_s3.py", self.texto)
        self.assertIn("verificar_lt_release.py", self.texto)
        self.assertIn("verificar_instancias_release.py", self.texto)
        self.assertIn("--launch-template LaunchTemplateName=\"$LT_NAME\",Version=\"$IN_LT_VERSION\"", self.texto)

    def test_inmutabilidad_s3_sin_sobrescritura(self) -> None:
        self.assertIn("ARTIFACT_IMMUTABILITY", _leer(RAIZ / "herramientas" / "decidir_publicacion_s3.py"))
        self.assertNotIn("s3api delete-object", self.texto)
        self.assertNotIn("--force", self.texto)

    def test_verifica_run_de_ci_verde(self) -> None:
        self.assertIn("verificar_run_ci.py", self.texto)
        self.assertIn("actions/runs/$IN_RUN_ID", self.texto)
        self.assertIn("--workflow-path .github/workflows/go-backend.yml", self.texto)
        self.assertIn("--evento push", self.texto)
        self.assertIn("--artefactos-json artefactos.json", self.texto)
        ayuda = _leer(RAIZ / "herramientas" / "verificar_run_ci.py")
        for marca in ("RUN_NO_VERDE", "RUN_SHA_DISTINTO", "RUN_RAMA_NO_AUTORIZADA", "RUN_REPOSITORIO_DISTINTO",
                      "RUN_WORKFLOW_NO_AUTORIZADO", "RUN_EVENTO_NO_AUTORIZADO", "ARTEFACTO_FALTANTE"):
            self.assertIn(marca, ayuda)

    def test_verifica_entorno_protegido_falla_cerrado(self) -> None:
        self.assertIn("verificar_entorno_protegido.py", self.texto)
        self.assertIn("ENVIRONMENT_PROTECTION_CONFIRMED", self.texto)
        self.assertIn("environments/aws-deploy", self.texto)
        ayuda = _leer(RAIZ / "herramientas" / "verificar_entorno_protegido.py")
        self.assertIn("SIN_REVISORES_OBLIGATORIOS", ayuda)
        self.assertIn("ENTORNO_DISTINTO", ayuda)

    def test_oidc_coincide_con_i060(self) -> None:
        self.assertIn("environment: aws-deploy", self.texto)
        iam = _leer(RAIZ / "infra" / "terraform" / "modules" / "iam" / "main.tf")
        self.assertIn("environment:aws-deploy", iam)
        self.assertNotIn("audience:", self.texto)
        self.assertNotIn("AWS_ACCESS_KEY_ID", self.texto)
        self.assertNotIn("AWS_SECRET_ACCESS_KEY", self.texto)


class CapacidadYReadiness(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.texto = _sin_comentarios(_leer(CD))
        cls.politica = _leer(POLITICA)
        cls.rollback = _leer(ROLLBACK)

    def test_capacidad_transitoria_max_3_y_restauracion(self) -> None:
        self.assertIn("--max-size 3", self.texto)
        self.assertIn("--max-size 2", self.texto)
        self.assertIn("max_size_actual", self.texto)
        self.assertIn("test \"$MAX\" -eq 2", self.texto)
        # El rollback también restaura max 2.
        rollback_step = self.texto.split("Rollback seguro", 1)[1]
        self.assertIn("--max-size 2", rollback_step)

    def test_documenta_coste_transitorio_y_bloqueo(self) -> None:
        for texto in (self.politica, self.rollback):
            self.assertIn("transitoria", texto.lower())
        self.assertIn("0.0084", self.politica)
        self.assertIn("BLOCKED", self.politica)

    def test_readiness_obligatoria_sin_variable_de_conveniencia(self) -> None:
        self.assertIn("/api/v1/health/ready", self.texto)
        self.assertIn('GET /api/v1/health/ready ->', self.texto)
        self.assertEqual(self.texto.count('test "$CODIGO" = "200"'), 2)
        # No hay variable opcional que convierta un despliegue incompleto en exitoso.
        self.assertNotIn("READINESS_REQUIRED", self.texto)
        self.assertNotIn("readiness_requerida", self.texto)

    def test_simulador_de_rollback_y_capacidad(self) -> None:
        simulador = RAIZ / "herramientas" / "simular_despliegue.py"
        self.assertTrue(simulador.is_file())
        texto = _leer(simulador)
        for marca in ("ABORTAR", "ROLLBACK", "DESPLEGAR", "MAX_SIZE_INESPERADO", "HTTP_READY_NO_200"):
            self.assertIn(marca, texto)


class EvidenciaIntegra(unittest.TestCase):
    def test_evidencia_deploy_campos_requeridos(self) -> None:
        texto = _leer(EVIDENCIA)
        for campo in ("Release ID", "SHA completo", "Run de CI", "sha256_binario", "Autorizador", "Nodos `InService`", "Health HTTP", "Coste observado", "Rollback ejecutado"):
            self.assertIn(campo, texto, campo)
        self.assertIn("NOT_RUN", texto)
        self.assertIn("No se declara `DEPLOYED_VERIFIED`", texto)


class DespliegueSanoyRollback(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.texto = _sin_comentarios(_leer(CD))

    def test_exige_dos_nodos_sanos_antes_y_despues(self) -> None:
        self.assertEqual(self.texto.count("estado_asg.py"), 3)
        self.assertEqual(self.texto.count("--nodos 2"), 3)
        self.assertGreaterEqual(self.texto.count("select(.TargetHealth.State==\"healthy\")"), 2)
        self.assertGreaterEqual(self.texto.count('test "$SALUDABLES" -eq 2'), 2)

    def test_instance_refresh_conserva_capacidad(self) -> None:
        self.assertIn('"MinHealthyPercentage":100', self.texto)
        self.assertIn("start-instance-refresh", self.texto)
        for estado in ("Failed", "Cancelled", "RollbackInProgress", "RollbackFailed"):
            self.assertIn(estado, self.texto)

    def test_health_http_https(self) -> None:
        self.assertIn("/api/v1/health/live", self.texto)
        self.assertIn("--proto '=https'", self.texto)
        self.assertIn('test "$CODIGO" = "200"', self.texto)

    def test_rollback_seguro_ordenado(self) -> None:
        self.assertIn("if: failure()", self.texto)
        self.assertIn("SIN_MUTACION_AWS", self.texto)
        self.assertIn(".mutado", self.texto)
        self.assertIn("LT_ANTERIOR", self.texto)
        self.assertIn('--launch-template LaunchTemplateName="$LT_NAME",Version="$LT_ANTERIOR"', self.texto)
        self.assertIn("cancel-instance-refresh", self.texto)
        self.assertIn("estado_refresh.py", self.texto)
        self.assertIn("REFRESH_ID", self.texto)
        self.assertIn("ROLLBACK_BLOCKED_ACTIVE_REFRESH", self.texto)
        self.assertIn("ROLLBACK_FAILED", self.texto)
        self.assertIn("inicio-rollback.json", self.texto)
        self.assertIn("test \"$VERSION\" = \"$LT_ANTERIOR\"", self.texto)
        self.assertNotIn('Version="${{ inputs.lt_version_previa }}"', self.texto)
        # No se usa el JSON inicial ni el primer refresh de la lista para autorizar el rollback.
        rollback = self.texto.split("Rollback seguro", 1)[1]
        self.assertNotIn("InstanceRefreshes[0]", rollback)
        # No se restaura max 2 antes del refresh de rollback.
        self.assertLess(rollback.index("--max-size 3"), rollback.index("--max-size 2"))

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
