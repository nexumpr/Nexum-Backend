"""Contrato de CI de I-080 (INF-080-01..06) — pruebas estructurales.

Verifican el workflow real y los documentos de gobierno sin ejecutar GitHub Actions
ni invocar AWS. Los resultados de ejecución se registran en REPORTE_EVIDENCIA_CI.md;
estas pruebas no los sustituyen.
"""

from __future__ import annotations

import re
import unittest
from pathlib import Path

RAIZ = Path(__file__).resolve().parents[3]
WORKFLOW = RAIZ / ".github" / "workflows" / "go-backend.yml"
CONTROL = RAIZ / "sdd" / "_control" / "H08"
MATRIZ = CONTROL / "MATRIZ_JOBS_CI.md"
GATES = CONTROL / "GATES_PR.md"
REPORTE = CONTROL / "REPORTE_EVIDENCIA_CI.md"

JOBS = {
    "calidad": "Formato, vet, build y pruebas",
    "seguridad": "Analisis estatico y vulnerabilidades",
    "secretos": "Escaneo de secretos",
    "iac": "Validacion Terraform e IaC",
    "contratos": "Contratos OpenAPI y herramientas",
    "cadena-suministro": "SBOM de dependencias",
}


def _leer(ruta: Path) -> str:
    return ruta.read_text(encoding="utf-8")


def _sin_comentarios(texto: str) -> str:
    return "\n".join(linea for linea in texto.splitlines() if not linea.lstrip().startswith("#"))


class DisparadoresYPermisos(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.crudo = _leer(WORKFLOW)
        cls.texto = _sin_comentarios(cls.crudo)

    def test_pull_request_a_main_y_release(self) -> None:
        bloque = self.texto.split("pull_request:", 1)[1].split("push:", 1)[0]
        self.assertIn("main", bloque)
        self.assertIn("release/**", bloque)

    def test_push_solo_a_main(self) -> None:
        bloque = self.texto.split("push:", 1)[1].split("permissions:", 1)[0]
        self.assertIn("main", bloque)
        self.assertNotIn("release/**", bloque)

    def test_permisos_de_solo_lectura(self) -> None:
        self.assertRegex(self.texto, r"(?m)^permissions:\s*\n\s+contents:\s*read\s*$")
        self.assertNotIn("contents: write", self.texto)
        self.assertNotIn("pull_request_target", self.texto)

    def test_sin_secretos_ni_credenciales_cloud(self) -> None:
        for prohibido in (
            "${{ secrets.",
            "secrets.",
            "AWS_ACCESS_KEY_ID",
            "AWS_SECRET_ACCESS_KEY",
            "configure-aws-credentials",
            "aws-actions",
        ):
            self.assertNotIn(prohibido, self.crudo, prohibido)

    def test_sin_docker(self) -> None:
        self.assertNotIn("docker", self.texto.lower())

    def test_concurrencia_configurada(self) -> None:
        self.assertIn("concurrency:", self.texto)
        self.assertIn("cancel-in-progress: true", self.texto)


class AccionesFijadasPorSha(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.texto = _leer(WORKFLOW)

    def test_todas_las_acciones_estan_pinneadas_por_sha(self) -> None:
        usos = re.findall(r"(?m)^\s+uses:\s*(\S+)", self.texto)
        self.assertGreaterEqual(len(usos), 4, usos)
        for uso in usos:
            self.assertRegex(uso, r"^[\w./-]+@[0-9a-f]{40}$", uso)

    def test_herramientas_go_con_version_exacta(self) -> None:
        for herramienta in (
            "honnef.co/go/tools/cmd/staticcheck@v0.8.2",
            "golang.org/x/vuln/cmd/govulncheck@v1.9.0",
            "github.com/securego/gosec/v2/cmd/gosec@v2.30.0",
            "github.com/zricethezav/gitleaks/v8@v8.30.1",
            "github.com/anchore/syft/cmd/syft@v1.57.0",
        ):
            self.assertIn(herramienta, self.texto)

    def test_terraform_fijado(self) -> None:
        self.assertIn("terraform_version: 1.16.6", self.texto)


class TrabajosObligatorios(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.texto = _sin_comentarios(_leer(WORKFLOW))
        cls.bloque = cls.texto.split("jobs:", 1)[1]

    def test_los_seis_jobs_existen_con_su_nombre(self) -> None:
        for job, nombre in JOBS.items():
            self.assertRegex(self.bloque, rf"(?m)^  {re.escape(job)}:\s*$", job)
            self.assertIn(f"name: {nombre}", self.bloque, nombre)

    def test_calidad_cubre_formato_vet_staticcheck_build_y_pruebas(self) -> None:
        for comando in (
            "gofmt -l",
            "go vet ./...",
            "staticcheck",
            "go build -trimpath -buildvcs=true",
            "go test ./... -count=1",
            "go test -race",
        ):
            self.assertIn(comando, self.bloque, comando)

    def test_cobertura_dominio_umbral_80(self) -> None:
        self.assertIn("cobertura_dominio.py coverage.out --umbral 80", self.bloque)
        self.assertIn("-coverpkg=all", self.bloque)

    def test_seguridad_sin_exclusiones_y_dependencias(self) -> None:
        self.assertIn("go mod verify", self.bloque)
        self.assertIn("go list -m all", self.bloque)
        self.assertNotIn("-exclude", self.bloque)
        self.assertNotIn("-nosec", self.bloque)

    def test_secretos_bloquean_el_job(self) -> None:
        self.assertIn("gitleaks", self.bloque)
        self.assertIn("--exit-code 1", self.bloque)

    def test_iac_sin_backend_remoto_ni_mutaciones(self) -> None:
        self.assertIn("terraform fmt -check -recursive infra/terraform", self.bloque)
        self.assertIn("terraform init -backend=false -input=false", self.bloque)
        self.assertIn("terraform validate", self.bloque)
        self.assertIn("TF_DATA_DIR", self.bloque)
        for prohibido in ("-backend=true", "terraform apply", "terraform destroy", "force-unlock", "migrate-state"):
            self.assertNotIn(prohibido, self.bloque, prohibido)

    def test_contratos_openapi_y_herramientas(self) -> None:
        self.assertIn("validar_h06.py", self.bloque)
        self.assertIn("discover -s herramientas/tests", self.bloque)

    def test_sbom(self) -> None:
        self.assertIn("syft@v1.57.0", self.bloque)
        self.assertIn("spdx-json=sbom.spdx.json", self.bloque)

    def test_artefactos_nombrados_con_sha(self) -> None:
        nombres = re.findall(r"(?m)^\s+name:\s*(\S*\-)\$\{\{\s*github\.sha\s*\}\}", self.bloque)
        self.assertGreaterEqual(len(nombres), 3, nombres)
        for prefijo in ("cobertura-", "dependencias-", "sbom-spdx-"):
            self.assertIn(prefijo, nombres)

    def test_sin_green_forzado(self) -> None:
        for prohibido in ("continue-on-error", "|| true", "--admin", "set +e"):
            self.assertNotIn(prohibido, self.texto)


class EscaneoIaCReal(unittest.TestCase):
    """INF-080-01: escaneo de seguridad IaC independiente de `terraform validate`."""

    @classmethod
    def setUpClass(cls) -> None:
        cls.texto = _sin_comentarios(_leer(WORKFLOW))
        cls.bloque = cls.texto.split("jobs:", 1)[1]
        cls.ignore = _leer(RAIZ / ".trivyignore.yaml")
        cls.fixture = RAIZ / "tests" / "fixtures" / "iac" / "inseguro" / "main.tf"

    def test_workflow_instala_y_ejecuta_trivy(self) -> None:
        self.assertRegex(self.texto, r"aquasecurity/setup-trivy@[0-9a-f]{40}")
        self.assertIn("trivy config", self.bloque)
        self.assertIn("version: v0.76.2", self.bloque)

    def test_escanea_los_roots_y_modulos_terraform(self) -> None:
        self.assertIn("infra/terraform", self.bloque)
        self.assertIn("--severity HIGH,CRITICAL", self.bloque)
        self.assertIn("--exit-code 1", self.bloque)

    def test_usa_excepciones_acotadas_no_globales(self) -> None:
        self.assertIn("--ignorefile .trivyignore.yaml", self.bloque)
        self.assertNotIn("--ignore-policy", self.bloque)
        ids = re.findall(r"(?m)^\s*-\s+id:\s*(\S+)\s*$", self.ignore)
        self.assertEqual(set(ids), {"AWS-0011", "AWS-0054", "AWS-0132"}, ids)
        self.assertEqual(len(re.findall(r"(?m)^\s*statement:", self.ignore)), 3)
        self.assertNotIn('"*"', self.ignore)
        self.assertNotIn("id: *", self.ignore)
        self.assertIn("RFC-006", self.ignore)
        self.assertIn("expired_at: 2026-12-02T00:00:00Z", self.ignore)

    def test_rfc006_aprobada_con_alcance_y_caducidad(self) -> None:
        rfc = RAIZ / "sdd" / "_control" / "H08" / "RFC-006-EXCEPCION_WAF_I030.md"
        self.assertTrue(rfc.is_file())
        texto = _leer(rfc)
        self.assertIn("AWS-0011", texto)
        self.assertIn("APROBADA (2026-11-08)", texto)
        self.assertIn("APRUEBO_RFC_006_WAF_TEMPORAL", texto)
        for seccion in (
            "Motivo económico y técnico",
            "Riesgo HIGH aceptado",
            "Controles compensatorios reales",
            "Condiciones para el primer despliegue AWS",
            "Vigencia y caducidad",
            "Alternativas y coste",
            "Aprobación requerida y concedida",
        ):
            self.assertIn(seccion, texto, seccion)
        self.assertIn("No autoriza", texto)
        self.assertIn("2026-12-02", texto)

    def test_el_escaneo_no_se_sustituye_por_terraform_validate(self) -> None:
        self.assertIn("terraform validate", self.bloque)
        self.assertIn("trivy config", self.bloque)
        # El job falla si Trivy encuentra HIGH/CRITICAL; no hay soft-fail.
        self.assertNotIn("continue-on-error", self.texto)
        self.assertNotIn("--exit-code 0", self.bloque)

    def test_prueba_negativa_con_fixture_aislado(self) -> None:
        self.assertIn("tests/fixtures/iac/inseguro", self.bloque)
        self.assertIn("verificar_fixture_iac.py", self.bloque)
        self.assertTrue(self.fixture.is_file())
        contenido = _leer(self.fixture)
        self.assertIn('acl    = "public-read"', contenido)
        self.assertIn("block_public_acls       = false", contenido)
        # El fixture no vive bajo la infraestructura desplegable.
        self.assertNotIn("infra/terraform", self.fixture.as_posix())

    def test_sin_credenciales_aws_en_el_escaneo(self) -> None:
        inicio = self.bloque.index("Instalar Trivy")
        fin = self.bloque.index("Pruebas de contrato Terraform")
        tramo = self.bloque[inicio:fin]
        for prohibido in ("secrets.", "AWS_ACCESS_KEY", "AWS_SECRET", "configure-aws-credentials", "id-token"):
            self.assertNotIn(prohibido, tramo)


class DocumentosDeGobierno(unittest.TestCase):
    def test_gates_pr_declara_checks_y_estado_bloqueado(self) -> None:
        texto = _leer(GATES)
        for nombre in JOBS.values():
            self.assertIn(nombre, texto, nombre)
        self.assertIn("BLOQUEADO_PLAN_GITHUB", texto)
        self.assertIn("HTTP 403", texto)
        self.assertIn("allow_force_pushes", texto)
        self.assertIn('"allow_force_pushes": false', texto)
        self.assertIn("required_approving_review_count", texto)
        self.assertIn("RFC", texto)

    def test_matriz_lista_los_seis_jobs_y_sus_artefactos(self) -> None:
        texto = _leer(MATRIZ)
        for nombre in JOBS.values():
            self.assertIn(nombre, texto, nombre)
        for artefacto in ("cobertura-<sha>", "dependencias-<sha>", "sbom-spdx-<sha>"):
            self.assertIn(artefacto, texto)
        self.assertIn("NOT_RUN", texto)

    def test_reporte_vincula_evidencia_a_un_sha(self) -> None:
        texto = _leer(REPORTE)
        self.assertIn("IMPLEMENTADO_LOCAL", texto)
        self.assertIn("RFC-006", texto)
        self.assertIn("AWS-0011", texto)
        self.assertIn("42458688828", texto)
        self.assertIn("6497915f4c8e8d90a1f7f60c7f67d6353c6a69e0", texto)
        self.assertIn("NOT_RUN", texto)
        self.assertIn("si el head del pr cambia", texto.lower())


if __name__ == "__main__":
    unittest.main()
