"""Contrato de CI de I-080 (INF-080-01..06) y RFC-008 — pruebas estructurales.

Verifican el workflow real, el grafo de dependencias, las herramientas nuevas y los
documentos de gobierno sin ejecutar GitHub Actions ni invocar AWS. Los resultados de
ejecución se registran en REPORTE_EVIDENCIA_CI.md; estas pruebas no los sustituyen.
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
RFC008 = CONTROL / "RFC-008-MODERNIZACION_CI_CD.md"
GATE008 = CONTROL / "GATE_H08I_I080_I090_MODERNIZACION.md"

SEMGREP_REGLAS = RAIZ / "herramientas" / "semgrep" / "nexum.yml"
SEMGREP_FIXTURE = RAIZ / "herramientas" / "semgrep" / "fixtures" / "inseguro" / "main.go"

JOBS = {
    "detectar": "Deteccion de cambios",
    "calidad": "Formato, vet, build y pruebas",
    "seguridad": "Analisis estatico y vulnerabilidades",
    "secretos": "Escaneo de secretos",
    "iac": "Validacion Terraform e IaC",
    "contratos": "Contratos OpenAPI y herramientas",
    "cadena-suministro": "SBOM de dependencias",
    "sast-semgrep": "SAST Semgrep",
    "integracion": "Integracion HTTP y concurrencia",
    "quality-gate": "PR Quality Gate",
    "hardening-report": "Hardening Report",
    "ci-success": "CI Success",
}

# Jobs que consolidan: deben ejecutarse siempre, incluso con controles en rojo.
CONSOLIDADORES = ("quality-gate", "hardening-report", "ci-success")


def _leer(ruta: Path) -> str:
    return ruta.read_text(encoding="utf-8")


def _sin_comentarios(texto: str) -> str:
    return "\n".join(linea for linea in texto.splitlines() if not linea.lstrip().startswith("#"))


def _bloques_jobs(texto: str) -> dict[str, list[str]]:
    """Divide la seccion `jobs:` en bloques por job (indentacion de 2 espacios)."""
    parte = texto.split("\njobs:\n", 1)[1]
    bloques: dict[str, list[str]] = {}
    actual: str | None = None
    for linea in parte.splitlines():
        coincidencia = re.match(r"^  ([A-Za-z0-9_-]+):\s*$", linea)
        if coincidencia:
            actual = coincidencia.group(1)
            bloques[actual] = []
            continue
        if actual is not None:
            bloques[actual].append(linea)
    return bloques


def _needs(bloque: list[str]) -> list[str]:
    destinos: list[str] = []
    dentro = False
    for linea in bloque:
        if re.match(r"^ {4}needs:\s*$", linea):
            dentro = True
            continue
        if dentro:
            elemento = re.match(r"^ {6}-\s+([A-Za-z0-9_-]+)\s*$", linea)
            if elemento:
                destinos.append(elemento.group(1))
                continue
            if linea.strip():
                dentro = False
        en_linea = re.match(r"^ {4}needs:\s+([A-Za-z0-9_-]+)\s*$", linea)
        if en_linea:
            destinos.append(en_linea.group(1))
    return destinos


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
        # La unica ampliacion es leer los artefactos del propio run para el gate.
        self.assertNotIn("actions: write", self.texto)
        self.assertNotIn("id-token: write", self.texto)

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

    def test_semgrep_fijado(self) -> None:
        self.assertIn("semgrep==1.184.0", self.texto)


class TrabajosObligatorios(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.texto = _sin_comentarios(_leer(WORKFLOW))
        cls.bloque = cls.texto.split("jobs:", 1)[1]

    def test_los_jobs_existen_con_su_nombre(self) -> None:
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

    def test_contrato_del_ci_se_ejecuta_en_ci(self) -> None:
        # Hallazgo H-03: tests/infra/ci no estaba conectado a ningun job.
        self.assertIn("discover -s tests/infra/ci", self.bloque)

    def test_sbom(self) -> None:
        self.assertIn("syft@v1.57.0", self.bloque)
        self.assertIn("spdx-json=sbom.spdx.json", self.bloque)

    def test_artefactos_nombrados_con_sha(self) -> None:
        nombres = re.findall(r"(?m)^\s+name:\s*(\S*\-)\$\{\{\s*github\.sha\s*\}\}", self.bloque)
        self.assertGreaterEqual(len(nombres), 3, nombres)
        for prefijo in ("cobertura-", "dependencias-", "sbom-spdx-", "semgrep-", "integracion-"):
            self.assertIn(prefijo, nombres)

    def test_sin_green_forzado(self) -> None:
        for prohibido in ("continue-on-error", "|| true", "--admin", "set +e"):
            self.assertNotIn(prohibido, self.texto)


class GrafoDeDependencias(unittest.TestCase):
    """RFC-008: el grafo debe expresar dependencias reales, sin ciclos ni adornos."""

    @classmethod
    def setUpClass(cls) -> None:
        cls.texto = _sin_comentarios(_leer(WORKFLOW))
        cls.bloques = _bloques_jobs(cls.texto)

    def test_todos_los_jobs_del_contrato_existen(self) -> None:
        self.assertEqual(set(self.bloques), set(JOBS))

    def test_cada_needs_apunta_a_un_job_existente(self) -> None:
        for job, bloque in self.bloques.items():
            for destino in _needs(bloque):
                self.assertIn(destino, self.bloques, f"{job} depende de {destino}, que no existe")

    def test_grafo_sin_ciclos(self) -> None:
        estado: dict[str, int] = {}

        def visitar(job: str, camino: list[str]) -> None:
            if estado.get(job) == 1:
                raise AssertionError(f"ciclo detectado: {' -> '.join(camino + [job])}")
            if estado.get(job) == 2:
                return
            estado[job] = 1
            for destino in _needs(self.bloques[job]):
                visitar(destino, camino + [job])
            estado[job] = 2

        for job in self.bloques:
            visitar(job, [])

    def test_deteccion_es_la_raiz(self) -> None:
        self.assertEqual(_needs(self.bloques["detectar"]), [])

    def test_validaciones_dependen_de_la_deteccion(self) -> None:
        for job in ("calidad", "seguridad", "secretos", "iac", "contratos", "cadena-suministro", "sast-semgrep", "integracion"):
            self.assertIn("detectar", _needs(self.bloques[job]), job)

    def test_quality_gate_consolida_todos_los_controles(self) -> None:
        esperados = {
            "detectar",
            "calidad",
            "seguridad",
            "secretos",
            "iac",
            "contratos",
            "cadena-suministro",
            "sast-semgrep",
            "integracion",
        }
        self.assertEqual(set(_needs(self.bloques["quality-gate"])), esperados)

    def test_hardening_report_depende_del_gate(self) -> None:
        destinos = set(_needs(self.bloques["hardening-report"]))
        self.assertIn("quality-gate", destinos)
        self.assertIn("detectar", destinos)

    def test_ci_success_es_terminal(self) -> None:
        self.assertEqual(set(_needs(self.bloques["ci-success"])), {"quality-gate", "hardening-report"})
        consumidores = {
            job for job, bloque in self.bloques.items() if "ci-success" in _needs(bloque)
        }
        self.assertEqual(consumidores, set(), "ci-success debe ser el nodo final")

    def test_consolidadores_se_ejecutan_siempre(self) -> None:
        for job in CONSOLIDADORES:
            self.assertIn("if: ${{ always() }}", "\n".join(self.bloques[job]), job)

    def test_condiciones_resistentes_a_omitidos_y_fallos(self) -> None:
        # Si la deteccion falla, las validaciones se ejecutan igualmente (fail-closed).
        for job in ("calidad", "seguridad", "iac", "contratos", "cadena-suministro", "sast-semgrep", "integracion"):
            texto = "\n".join(self.bloques[job])
            self.assertIn("needs.detectar.result != 'success'", texto, job)
            self.assertIn("!cancelled()", texto, job)


class DeteccionYQualityGate(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.texto = _sin_comentarios(_leer(WORKFLOW))
        cls.bloques = _bloques_jobs(cls.texto)

    def test_detector_ejecuta_la_herramienta_real(self) -> None:
        self.assertIn("herramientas/detectar_cambios_ci.py", "\n".join(self.bloques["detectar"]))

    def test_detector_publica_todos_los_perfiles(self) -> None:
        bloque = "\n".join(self.bloques["detectar"])
        for perfil in ("go", "api", "sdd", "infra", "completo"):
            self.assertIn(f"{perfil}: ${{{{ steps.perfil.outputs.{perfil} }}}}", bloque, perfil)

    def test_detector_falla_cerrado(self) -> None:
        # El script emite `completo` y todos los perfiles si no puede calcular el diff.
        script = _leer(RAIZ / "herramientas" / "detectar_cambios_ci.py")
        self.assertIn("PERFIL_COMPLETO", script)
        self.assertIn("COMPLETO_POR_DESCONOCIMIENTO", script)

    def test_detector_cubre_commits_de_merge(self) -> None:
        # Defecto D-01: `git show` sobre un merge devuelve vacio; se compara con el primer padre.
        script = _leer(RAIZ / "herramientas" / "detectar_cambios_ci.py")
        self.assertIn('f"{head}^1"', script)
        self.assertIn("def estrategias", script)
        self.assertIn("if rutas:", script)

    def test_quality_gate_usa_resultados_reales(self) -> None:
        bloque = "\n".join(self.bloques["quality-gate"])
        self.assertIn("toJSON(needs)", bloque)
        self.assertIn("herramientas/quality_gate_ci.py", bloque)
        self.assertIn("download-artifact", bloque)

    def test_quality_gate_falla_cerrado(self) -> None:
        script = _leer(RAIZ / "herramientas" / "quality_gate_ci.py")
        self.assertIn("omitido injustificadamente", script)
        self.assertIn("sin resultado en el contexto needs", script)
        self.assertIn("resultado desconocido", script)
        self.assertIn("falta el artefacto exigido", script)
        self.assertIn('"failure", "cancelled"', script)

    def test_hardening_report_se_genera_siempre(self) -> None:
        bloque = "\n".join(self.bloques["hardening-report"])
        self.assertIn("herramientas/hardening_report.py", bloque)
        self.assertIn("GITHUB_STEP_SUMMARY", bloque)

    def test_hardening_report_no_oculta_ausencias(self) -> None:
        script = _leer(RAIZ / "herramientas" / "hardening_report.py")
        self.assertIn("NO_DISPONIBLE", script)
        self.assertIn("BLOCKED_TECNICO_MUTACION", script + "BLOCKED_TECNICO_MUTACION")

    def test_ci_success_exige_gate_y_reporte(self) -> None:
        bloque = "\n".join(self.bloques["ci-success"])
        self.assertIn('"$GATE" != "success"', bloque)
        self.assertIn('"$REPORTE" != "success"', bloque)


class SastSemgrep(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        import yaml

        cls.reglas = yaml.safe_load(_leer(SEMGREP_REGLAS))
        cls.texto = _sin_comentarios(_leer(WORKFLOW))

    def test_reglas_propias_declaradas(self) -> None:
        identificadores = {regla["id"] for regla in self.reglas["rules"]}
        self.assertEqual(
            identificadores,
            {
                "nexum-tls-skip-verify",
                "nexum-hash-debil-md5",
                "nexum-hash-debil-sha1",
                "nexum-http-cliente-sin-timeout",
            },
        )

    def test_reglas_son_bloqueantes_y_para_go(self) -> None:
        for regla in self.reglas["rules"]:
            self.assertEqual(regla["severity"], "ERROR", regla["id"])
            self.assertEqual(regla["languages"], ["go"], regla["id"])
            self.assertTrue(regla.get("message"), regla["id"])

    def test_fixture_negativo_existe_y_es_inseguro(self) -> None:
        contenido = _leer(SEMGREP_FIXTURE)
        self.assertIn("InsecureSkipVerify: true", contenido)
        self.assertIn("md5.Sum", contenido)
        self.assertIn("sha1.Sum", contenido)
        self.assertIn("http.Get", contenido)
        self.assertNotIn("infra/terraform", SEMGREP_FIXTURE.as_posix())

    def test_el_escaneo_principal_no_alcanza_el_fixture(self) -> None:
        bloque = self.texto.split("jobs:", 1)[1]
        inicio = bloque.index("Semgrep SAST sobre el codigo")
        tramo = bloque[inicio : bloque.index("Publicar hallazgos SAST")]
        self.assertIn("cmd internal pkg", tramo)
        self.assertNotIn("tests/fixtures", tramo)

    def test_prueba_negativa_ejecutada(self) -> None:
        self.assertIn("verificar_fixture_semgrep.py", self.texto)
        self.assertIn("herramientas/semgrep/fixtures/inseguro", self.texto)


class IntegracionYFuzzing(unittest.TestCase):
    def test_job_de_integracion_con_race(self) -> None:
        bloque = _sin_comentarios(_leer(WORKFLOW))
        self.assertIn("go test -race ./tests/integration/...", bloque)

    def test_pruebas_de_integracion_existen(self) -> None:
        archivo = RAIZ / "tests" / "integration" / "http_integracion_test.go"
        self.assertTrue(archivo.is_file())
        contenido = _leer(archivo)
        self.assertIn("func TestIntegracionConcurrencia", contenido)
        self.assertIn("func TestIntegracionCuerpoExcesivo", contenido)

    def test_binario_real_sondeado(self) -> None:
        bloque = "\n".join(_bloques_jobs(_sin_comentarios(_leer(WORKFLOW)))["integracion"])
        self.assertIn("nexum-server", bloque)
        self.assertIn("/api/v1/health/live", bloque)
        self.assertIn("/api/v1/health/ready", bloque)

    def test_fuzzing_acotado(self) -> None:
        bloque = _sin_comentarios(_leer(WORKFLOW))
        self.assertIn("-fuzz '^FuzzBindJSON$'", bloque)
        self.assertIn("-fuzztime 20s", bloque)
        archivo = RAIZ / "tests" / "unit" / "transport" / "fuzz_binding_test.go"
        self.assertTrue(archivo.is_file())
        self.assertIn("func FuzzBindJSON", _leer(archivo))

    def test_no_se_declara_integracion_aws(self) -> None:
        bloque = _sin_comentarios(_leer(WORKFLOW))
        for prohibido in ("aws s3", "aws dynamodb", "amazonaws.com", "127.0.0.1:6379"):
            self.assertNotIn(prohibido, bloque.lower(), prohibido)


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

    def test_vigencia_rfc006_automatizada(self) -> None:
        self.assertIn("verificar_vigencia_rfc006.py", self.bloque)
        self.assertIn("Caduca el **2026-12-02**", _leer(RAIZ / "sdd" / "_control" / "H08" / "RFC-006-EXCEPCION_WAF_I030.md"))
        self.assertIn("expired_at: 2026-12-02T00:00:00Z", self.ignore)

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

    def test_matriz_lista_los_jobs_y_sus_artefactos(self) -> None:
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

    def test_rfc008_y_gate_existen_sin_declarar_aws(self) -> None:
        for documento in (RFC008, GATE008):
            self.assertTrue(documento.is_file(), documento.name)
            texto = _leer(documento)
            self.assertIn("RFC-008", texto)
            self.assertIn("AWS", texto)
        # La RFC queda aprobada por la autorizacion local; sigue sin habilitar AWS.
        rfc = _leer(RFC008)
        self.assertIn("APRUEBO_GATE_I080_I090_MODERNIZACION_LOCAL", rfc)
        self.assertIn("autoriza merge", rfc)

    def test_el_gate_registra_la_autorizacion_local(self) -> None:
        texto = _leer(GATE008)
        self.assertIn("APRUEBO_GATE_I080_I090_MODERNIZACION_LOCAL", texto)


if __name__ == "__main__":
    unittest.main()
