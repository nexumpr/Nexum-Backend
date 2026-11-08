"""Contrato DevSecOps de I-100 (INF-100-01..06).

Verifica que el CI ejecuta los controles y que la documentación declara los
límites reales (DAST NOT_RUN, sin declarar seguridad verificada en cloud).
"""

from __future__ import annotations

import re
import unittest
from pathlib import Path

RAIZ = Path(__file__).resolve().parents[3]
CONTROL = RAIZ / "sdd" / "_control" / "H08"
CI = RAIZ / ".github" / "workflows" / "go-backend.yml"
CD = RAIZ / ".github" / "workflows" / "cd-backend.yml"
AMENAZAS = CONTROL / "MODELO_AMENAZAS.md"
MATRIZ = CONTROL / "MATRIZ_ESCANEOS.md"
CRITERIOS = CONTROL / "CRITERIOS_VULNERABILIDADES.md"
GATE = CONTROL / "GATE_H08I_I100.md"
ADVERSARIAL = RAIZ / "tests" / "security" / "adversarial_test.go"
IAC = RAIZ / "herramientas" / "verificar_iac_devsecops.py"


def _leer(ruta: Path) -> str:
    return ruta.read_text(encoding="utf-8")


def _sin_comentarios(texto: str) -> str:
    return "\n".join(linea for linea in texto.splitlines() if not linea.lstrip().startswith("#"))


class ControlesEnCI(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.texto = _sin_comentarios(_leer(CI))

    def test_sast_sca_y_lint_presentes(self) -> None:
        for herramienta in ("gosec@v2.30.0", "govulncheck@v1.9.0", "staticcheck@v0.8.2"):
            self.assertIn(herramienta, self.texto)
        self.assertIn("gosec@v2.30.0 -quiet ./cmd/... ./internal/... ./pkg/...", self.texto)
        self.assertNotIn("-exclude", self.texto)
        self.assertNotIn("-nosec", self.texto)

    def test_prueba_negativa_de_gosec(self) -> None:
        self.assertIn("tests/fixtures/security/gosecinseguro", self.texto)
        self.assertIn("verificar_fixture_gosec.py", self.texto)
        fixture = RAIZ / "tests" / "fixtures" / "security" / "gosecinseguro" / "main.go"
        self.assertTrue(fixture.is_file())
        self.assertIn("math/rand", _leer(fixture))

    def test_secretos_y_sbom(self) -> None:
        self.assertIn("gitleaks", self.texto)
        self.assertIn("--exit-code 1", self.texto)
        self.assertIn("syft@v1.57.0", self.texto)
        self.assertIn("spdx-json", self.texto)

    def test_iac_trivy_y_escaner_devsecops(self) -> None:
        self.assertIn("trivy config", self.texto)
        self.assertIn("--severity HIGH,CRITICAL", self.texto)
        self.assertIn("verificar_iac_devsecops.py", self.texto)
        self.assertIn("iac-devsecops-${{ github.sha }}", self.texto)

    def test_pruebas_adversariales_en_ci_y_race(self) -> None:
        self.assertIn("./tests/security/...", self.texto)
        self.assertIn("go test -race ./cmd/... ./tests/unit/... ./tests/security/...", self.texto)

    def test_permisos_minimos_y_acciones_fijadas(self) -> None:
        self.assertRegex(self.texto, r"(?m)^permissions:\s*\n\s+contents:\s*read\s*$")
        usos = re.findall(r"(?m)^\s+uses:\s*(\S+)", self.texto)
        for uso in usos:
            self.assertRegex(uso, r"^[\w./-]+@[0-9a-f]{40}$", uso)

    def test_sin_dast_ni_escaneo_de_produccion(self) -> None:
        for prohibido in ("zap", "owasp", "nikto", "nmap", "burp"):
            self.assertNotIn(prohibido, self.texto.lower())
        self.assertNotIn("continue-on-error", self.texto)
        self.assertNotIn("|| true", self.texto)


class EscanerIaCPropio(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.texto = _leer(IAC)

    def test_reglas_exigidas_por_el_sdd(self) -> None:
        for regla in ("SG_PUBLICO", "IAM_WILDCARD", "S3_SIN_CIFRADO", "S3_CIFRADO_NO_VERIFICABLE",
                      "FORCE_DESTROY", "ACL_PUBLICA", "TFSTATE_RASTREADO", "GIT_CHECK_FAILED"):
            self.assertIn(regla, self.texto, regla)
        for variante in ("cidr_ipv4", "cidr_ipv6", "ipv6_cidr_blocks"):
            self.assertIn(variante, self.texto, variante)

    def test_falla_cerrado(self) -> None:
        self.assertIn("IAC_DEVSECOPS_FAIL", self.texto)
        self.assertIn("return 1", self.texto)
        self.assertIn("S3_CIFRADO_NO_VERIFICABLE", self.texto)


class DocumentacionDeSeguridad(unittest.TestCase):
    def test_modelo_de_amenazas_cubre_superficies_y_stride(self) -> None:
        texto = _leer(AMENAZAS)
        for superficie in ("S1", "S2", "S3", "S4", "S5", "S6"):
            self.assertIn(f"| {superficie} |", texto, superficie)
        for letra in ("**S**poofing", "**T**ampering", "**R**epudiation", "**I**nformation disclosure", "**D**oS", "**E**levation"):
            self.assertIn(letra, texto, letra)
        for riesgo in ("IDOR", "Token replay", "Manipulación monetaria", "SSRF"):
            self.assertIn(riesgo, texto, riesgo)
        self.assertIn("Riesgo residual", texto)
        self.assertIn("10 min", texto)

    def test_modelo_tiene_matriz_verificable_y_relacion_e001_e075(self) -> None:
        texto = _leer(AMENAZAS)
        self.assertIn("Matriz verificable de amenazas y controles", texto)
        for columna in ("Superficie", "Amenaza", "Spec/ruta afectada", "Control", "Prueba (P/N)", "Evidencia", "Sev.", "Responsable", "Estado"):
            self.assertIn(columna, texto, columna)
        self.assertIn("T-01", texto)
        self.assertIn("T-22", texto)
        self.assertIn("E001–E075", texto)
        self.assertIn("NOT_IMPLEMENTED / NOT_RUN", texto)
        self.assertIn("No hay endpoints HTTP implementados", texto)

    def test_modelo_registra_defectos_a030_a080(self) -> None:
        texto = _leer(AMENAZAS)
        self.assertIn("D-100-01", texto)
        self.assertIn("D-100-02", texto)
        self.assertIn("X-Forwarded-For", texto)
        self.assertIn("no está cableado", texto)
        self.assertIn("RATE_LIMIT_DISTRIBUTED_VERIFIED", texto)
        self.assertIn("no se declara", texto.lower())

    def test_tests_de_rate_limit_distinguen_local_y_simulado(self) -> None:
        ruta = RAIZ / "tests" / "security" / "rate_limit_test.go"
        self.assertTrue(ruta.is_file())
        texto = _leer(ruta)
        self.assertIn("TestCaracterizaDefectoHTTPRateLimitLocalYConfiaXFF", texto)
        self.assertIn("TestInfraRateLimitCoordinaPorCacheCompartida", texto)
        self.assertIn("NO demuestra Redis real", texto)

    def test_matriz_declara_ejecutado_y_not_run(self) -> None:
        texto = _leer(MATRIZ)
        for herramienta in ("gosec", "govulncheck", "staticcheck", "gitleaks", "Trivy", "syft"):
            self.assertIn(herramienta, texto, herramienta)
        self.assertIn("DAST", texto)
        self.assertIn("NOT_RUN", texto)
        self.assertIn("PENDIENTE_DE_DISPONIBILIDAD", texto)
        self.assertIn("NO ejecutadas", texto)
        self.assertIn("Checksum ≠ firma", texto)
        self.assertIn("preparar_dast.py", texto)
        self.assertIn("GOSEC_FIXTURE_PASS", texto)

    def test_criterios_de_vulnerabilidad(self) -> None:
        texto = _leer(CRITERIOS)
        for severidad in ("CRITICAL", "HIGH", "MEDIUM", "LOW/INFO"):
            self.assertIn(severidad, texto)
        self.assertIn("RFC firmada", texto)
        self.assertIn("caducidad", texto)
        self.assertIn("Nunca desactivar", texto)
        self.assertIn("NOT_RUN", texto)

    def test_gate_no_declara_seguridad_verificada(self) -> None:
        texto = _leer(GATE)
        self.assertIn("IMPLEMENTADO_LOCAL", texto)
        self.assertIn("NOT_RUN", texto)
        self.assertIn("No se declara", texto)
        self.assertIn("DAST", texto)

    def test_harness_dast_exige_evidencia_verificable(self) -> None:
        texto = _leer(RAIZ / "herramientas" / "preparar_dast.py")
        for marca in ("APRUEBO_GATE_DAST", "EVIDENCIA_FUERA_DE_CONTROL", "EVIDENCIA_CADUCADA",
                      "PLAN_TECNICO_PREPARADO_NO_AUTORIZADO", "GATE_DAST_AUSENTE",
                      "NEXUM_TEST_MODE", "no verificada por el gobierno"):
            self.assertIn(marca, texto, marca)
        self.assertNotIn("permitir-remoto", texto)


class PruebasAdversariales(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.texto = _leer(ADVERSARIAL)

    def test_cubre_los_ataques_exigidos(self) -> None:
        for nombre in (
            "TestJWTRechazaFirmaManipuladaYAlgNone",
            "TestJWTExpiradoSeRechaza",
            "TestAuthMiddlewareRechazaSinTokenYConTokenInvalido",
            "TestSesionLogoutYReplayRevocan",
            "TestIdempotenciaConcurrenteYConflictos",
            "TestCORSRechazaOrigenNoAutorizado",
            "TestCabecerasDeSeguridad",
            "TestBodyGrandeSeRechaza",
            "TestRateLimiterDevuelve429ConRetryAfter",
        ):
            self.assertIn(f"func {nombre}(", self.texto, nombre)


if __name__ == "__main__":
    unittest.main()
