"""Pruebas de las herramientas de CI de RFC-008.

Cubren el detector de cambios, el Quality Gate (exito, fallo, cancelacion, omision
justificada e injustificada, resultado desconocido y artefacto ausente), el Hardening
Report, la evaluacion de mutation testing y las reglas SAST propias.

No ejecutan GitHub Actions ni invocan AWS.
"""

from __future__ import annotations

import json
import sys
import tempfile
import unittest
from pathlib import Path

HERRAMIENTAS = Path(__file__).resolve().parents[1]
if str(HERRAMIENTAS) not in sys.path:
    sys.path.insert(0, str(HERRAMIENTAS))

import detectar_cambios_ci as detector
import evaluar_mutacion
import hardening_report
import quality_gate_ci as gate
import verificar_fixture_semgrep as fixture_semgrep

RAIZ = HERRAMIENTAS.parent
SHA = "a" * 40


def _crear_artefactos(base: Path, jobs: list[str], sha: str = SHA) -> Path:
    artefactos = base / "artefactos"
    mapa = {
        "calidad": [("cobertura-{sha}", "coverage.out"), ("binario-{sha}", "binario.sha256"), ("mutacion-{sha}", "mutacion.json")],
        "seguridad": [("dependencias-{sha}", "dependencias.txt")],
        "cadena-suministro": [("sbom-spdx-{sha}", "sbom.spdx.json")],
        "sast-semgrep": [("semgrep-{sha}", "semgrep.json")],
        "integracion": [("integracion-{sha}", "integracion.txt")],
        "iac": [("iac-devsecops-{sha}", "iac-devsecops.json")],
    }
    for job in jobs:
        for nombre, archivo in mapa.get(job, []):
            directorio = artefactos / nombre.format(sha=sha)
            directorio.mkdir(parents=True, exist_ok=True)
            contenido = '{"results": []}' if archivo.endswith(".json") else f"{job}: ok\n"
            (directorio / archivo).write_text(contenido, encoding="utf-8")
    artefactos.mkdir(parents=True, exist_ok=True)
    return artefactos


def _needs(resultados: dict[str, str]) -> dict:
    base = {
        "detectar": "success",
        "calidad": "success",
        "seguridad": "success",
        "secretos": "success",
        "iac": "success",
        "contratos": "success",
        "cadena-suministro": "success",
        "sast-semgrep": "success",
        "integracion": "success",
    }
    base.update(resultados)
    return {job: {"result": resultado, "outputs": {}} for job, resultado in base.items()}


PERFIL_COMPLETO = {"go": True, "api": True, "sdd": True, "infra": True, "completo": True}
PERFIL_SOLO_DOCS = {"go": False, "api": False, "sdd": False, "infra": False, "completo": False}


class DetectorDeCambios(unittest.TestCase):
    def test_cambio_go_activa_solo_go(self) -> None:
        perfil = detector.clasificar(["cmd/server/main.go", "internal/domain/money/money.go"])
        self.assertTrue(perfil["go"])
        self.assertFalse(perfil["api"])
        self.assertFalse(perfil["infra"])

    def test_cambio_de_workflow_activa_todo(self) -> None:
        perfil = detector.clasificar([".github/workflows/go-backend.yml"])
        self.assertEqual(perfil, detector.PERFIL_COMPLETO)

    def test_cambio_de_herramienta_activa_todo(self) -> None:
        self.assertEqual(detector.clasificar(["herramientas/quality_gate_ci.py"]), detector.PERFIL_COMPLETO)

    def test_dependencias_activan_todo(self) -> None:
        self.assertEqual(detector.clasificar(["go.mod"]), detector.PERFIL_COMPLETO)
        self.assertEqual(detector.clasificar(["go.sum"]), detector.PERFIL_COMPLETO)

    def test_solo_documentacion_no_activa_nada(self) -> None:
        self.assertEqual(detector.clasificar(["README.md", "memoria.md"]), PERFIL_SOLO_DOCS)

    def test_ruta_desconocida_activa_todo(self) -> None:
        self.assertEqual(detector.clasificar(["Makefile"]), detector.PERFIL_COMPLETO)

    def test_infra_y_contratos(self) -> None:
        self.assertTrue(detector.clasificar(["infra/terraform/main.tf"])["infra"])
        perfil = detector.clasificar(["sdd/03-endpoints/openapi/nexum-api-v1.json"])
        self.assertTrue(perfil["api"])
        self.assertTrue(perfil["sdd"])

    def test_sin_cambios_no_activa_nada(self) -> None:
        self.assertEqual(detector.clasificar([]), PERFIL_SOLO_DOCS)

    def test_workflow_dispatch_no_es_determinable(self) -> None:
        self.assertIsNone(detector.archivos_desde_git("workflow_dispatch", "", ""))

    def test_salida_en_formato_github(self) -> None:
        with tempfile.TemporaryDirectory() as temporal:
            salida = Path(temporal) / "out.txt"
            detector.escribir_salidas(detector.PERFIL_COMPLETO, salida)
            lineas = salida.read_text(encoding="utf-8").splitlines()
            self.assertIn("go=true", lineas)
            self.assertIn("completo=true", lineas)
            self.assertEqual(len(lineas), 5)


class QualityGate(unittest.TestCase):
    def _evaluar(self, resultados: dict[str, str], perfil: dict, jobs_con_artefactos: list[str]):
        temporal = Path(tempfile.mkdtemp())
        artefactos = _crear_artefactos(temporal, jobs_con_artefactos)
        return gate.evaluar(_needs(resultados), perfil, artefactos, SHA)

    def test_todo_correcto_pasa(self) -> None:
        reporte = self._evaluar({}, PERFIL_COMPLETO, list(gate.ARTEFACTOS))
        self.assertEqual(reporte["estado"], "PASS", reporte["fallos"])
        self.assertEqual(reporte["fallos"], [])

    def test_fallo_bloquea(self) -> None:
        reporte = self._evaluar({"calidad": "failure"}, PERFIL_COMPLETO, list(gate.ARTEFACTOS))
        self.assertEqual(reporte["estado"], "FAIL")
        self.assertTrue(any("calidad" in fallo for fallo in reporte["fallos"]))

    def test_cancelacion_bloquea(self) -> None:
        reporte = self._evaluar({"seguridad": "cancelled"}, PERFIL_COMPLETO, list(gate.ARTEFACTOS))
        self.assertEqual(reporte["estado"], "FAIL")
        self.assertTrue(any("cancelled" in fallo for fallo in reporte["fallos"]))

    def test_omision_injustificada_bloquea(self) -> None:
        reporte = self._evaluar({"calidad": "skipped"}, PERFIL_COMPLETO, [])
        self.assertEqual(reporte["estado"], "FAIL")
        self.assertTrue(any("omitido injustificadamente" in fallo for fallo in reporte["fallos"]))

    def test_omision_justificada_pasa_y_se_registra(self) -> None:
        reporte = self._evaluar(
            {"calidad": "skipped", "seguridad": "skipped", "iac": "skipped", "contratos": "skipped",
             "cadena-suministro": "skipped", "sast-semgrep": "skipped", "integracion": "skipped"},
            PERFIL_SOLO_DOCS,
            [],
        )
        self.assertEqual(reporte["estado"], "PASS", reporte["fallos"])
        omitidos = {item["job"] for item in reporte["jobs_omitidos_justificados"]}
        self.assertIn("calidad", omitidos)

    def test_resultado_ausente_bloquea(self) -> None:
        needs = _needs({})
        needs.pop("iac")
        temporal = Path(tempfile.mkdtemp())
        artefactos = _crear_artefactos(temporal, list(gate.ARTEFACTOS))
        reporte = gate.evaluar(needs, PERFIL_COMPLETO, artefactos, SHA)
        self.assertEqual(reporte["estado"], "FAIL")
        self.assertTrue(any("sin resultado" in fallo for fallo in reporte["fallos"]))

    def test_resultado_desconocido_bloquea(self) -> None:
        reporte = self._evaluar({"contratos": "neutral"}, PERFIL_COMPLETO, list(gate.ARTEFACTOS))
        self.assertEqual(reporte["estado"], "FAIL")
        self.assertTrue(any("desconocido" in fallo for fallo in reporte["fallos"]))

    def test_artefacto_ausente_bloquea(self) -> None:
        reporte = self._evaluar({}, PERFIL_COMPLETO, ["calidad"])
        self.assertEqual(reporte["estado"], "FAIL")
        self.assertTrue(any("artefacto exigido" in fallo for fallo in reporte["fallos"]))

    def test_deteccion_fallida_bloquea(self) -> None:
        reporte = self._evaluar({"detectar": "failure"}, PERFIL_COMPLETO, list(gate.ARTEFACTOS))
        self.assertEqual(reporte["estado"], "FAIL")
        self.assertTrue(any("detectar" in fallo for fallo in reporte["fallos"]))

    def test_perfil_completo_exige_todos(self) -> None:
        reporte = self._evaluar(
            {job: "skipped" for job in gate.DEPENDENCIAS},
            PERFIL_COMPLETO,
            [],
        )
        self.assertEqual(reporte["estado"], "FAIL")
        self.assertEqual(len(reporte["fallos"]), len(gate.DEPENDENCIAS))

    def test_cli_devuelve_uno_ante_fallo(self) -> None:
        with tempfile.TemporaryDirectory() as temporal:
            base = Path(temporal)
            (base / "needs.json").write_text(json.dumps(_needs({"calidad": "failure"})), encoding="utf-8")
            (base / "perfil.json").write_text(json.dumps(PERFIL_COMPLETO), encoding="utf-8")
            codigo = gate.main([
                "--needs", str(base / "needs.json"),
                "--perfil", str(base / "perfil.json"),
                "--artefactos", str(base / "nada"),
                "--sha", SHA,
            ])
            self.assertEqual(codigo, 1)

    def test_cli_falla_si_el_contexto_es_ilegible(self) -> None:
        self.assertEqual(
            gate.main([
                "--needs", "no-existe.json",
                "--perfil", "no-existe.json",
                "--artefactos", "nada",
                "--sha", SHA,
            ]),
            1,
        )


class HardeningReport(unittest.TestCase):
    def test_se_genera_aunque_todo_falte(self) -> None:
        with tempfile.TemporaryDirectory() as temporal:
            base = Path(temporal)
            (base / "needs.json").write_text(
                json.dumps(_needs({"calidad": "failure", "seguridad": "skipped"})), encoding="utf-8"
            )
            (base / "perfil.json").write_text(json.dumps(PERFIL_COMPLETO), encoding="utf-8")
            (base / "artefactos").mkdir()
            codigo = hardening_report.main([
                "--needs", str(base / "needs.json"),
                "--perfil", str(base / "perfil.json"),
                "--artefactos", str(base / "artefactos"),
                "--sha", SHA,
                "--ref", "refs/pull/1/merge",
                "--pr", "1",
                "--salida", str(base / "reporte"),
            ])
            self.assertEqual(codigo, 0)
            markdown = (base / "reporte" / "hardening-report.md").read_text(encoding="utf-8")
            self.assertIn(SHA, markdown)
            self.assertIn("failure", markdown)
            self.assertIn("DAST", markdown)
            self.assertIn("Riesgos residuales", markdown)
            self.assertIn("NO_DISPONIBLE", markdown)

    def test_registra_el_bloqueo_de_mutacion(self) -> None:
        with tempfile.TemporaryDirectory() as temporal:
            base = Path(temporal)
            (base / "needs.json").write_text(json.dumps(_needs({})), encoding="utf-8")
            (base / "perfil.json").write_text(json.dumps(PERFIL_COMPLETO), encoding="utf-8")
            artefactos = _crear_artefactos(base, ["calidad"])
            (artefactos / f"mutacion-{SHA}" / "mutacion.json").write_text(
                json.dumps({"estado": "BLOCKED_TECNICO_MUTACION", "motivos": ["pruebas_en_el_mismo_paquete"]}),
                encoding="utf-8",
            )
            reporte = hardening_report.construir(
                _needs({}), PERFIL_COMPLETO, artefactos, SHA, "refs/heads/x", ""
            )
            self.assertEqual(reporte["mutacion"]["estado"], "BLOCKED_TECNICO_MUTACION")
            self.assertFalse(reporte["mutacion"]["metricas_publicadas"])

    def test_no_declara_aws(self) -> None:
        with tempfile.TemporaryDirectory() as temporal:
            artefactos = _crear_artefactos(Path(temporal), [])
            reporte = hardening_report.construir(_needs({}), PERFIL_COMPLETO, artefactos, SHA, "ref", "")
            self.assertFalse(reporte["aws_consultado"])
            self.assertEqual(reporte["dast"]["estado"], "NOT_RUN")


class Mutacion(unittest.TestCase):
    def test_no_publica_metricas_sin_condiciones(self) -> None:
        reporte = evaluar_mutacion.evaluar()
        self.assertIn(reporte["estado"], {"VIABLE", "BLOCKED_TECNICO_MUTACION"})
        self.assertFalse(reporte["metricas_publicadas"])
        if reporte["estado"] == "BLOCKED_TECNICO_MUTACION":
            self.assertTrue(reporte["motivos"])
            self.assertTrue(reporte["condiciones"])

    def test_los_paquetes_prioritarios_existen(self) -> None:
        for paquete in evaluar_mutacion.PAQUETES_PRIORITARIOS:
            self.assertTrue((RAIZ / paquete).is_dir(), paquete)


class ReglasSemgrep(unittest.TestCase):
    def test_fixture_valido_pasa(self) -> None:
        with tempfile.TemporaryDirectory() as temporal:
            ruta = Path(temporal) / "semgrep.json"
            ruta.write_text(
                json.dumps({
                    "results": [
                        {"check_id": "herramientas.semgrep.nexum-tls-skip-verify", "path": "tests/fixtures/semgrep/inseguro/main.go"},
                        {"check_id": "herramientas.semgrep.nexum-hash-debil-md5", "path": "tests/fixtures/semgrep/inseguro/main.go"},
                        {"check_id": "herramientas.semgrep.nexum-hash-debil-sha1", "path": "tests/fixtures/semgrep/inseguro/main.go"},
                        {"check_id": "herramientas.semgrep.nexum-http-cliente-sin-timeout", "path": "tests/fixtures/semgrep/inseguro/main.go"},
                    ]
                }),
                encoding="utf-8",
            )
            correcto, problemas = fixture_semgrep.verificar(ruta, 1)
            self.assertTrue(correcto, problemas)

    def test_sin_hallazgos_falla(self) -> None:
        with tempfile.TemporaryDirectory() as temporal:
            ruta = Path(temporal) / "semgrep.json"
            ruta.write_text(json.dumps({"results": []}), encoding="utf-8")
            correcto, _ = fixture_semgrep.verificar(ruta, 1)
            self.assertFalse(correcto)

    def test_codigo_cero_falla(self) -> None:
        with tempfile.TemporaryDirectory() as temporal:
            ruta = Path(temporal) / "semgrep.json"
            ruta.write_text(json.dumps({"results": []}), encoding="utf-8")
            correcto, problemas = fixture_semgrep.verificar(ruta, 0)
            self.assertFalse(correcto)
            self.assertTrue(any("codigo de salida" in problema for problema in problemas))

    def test_regla_ausente_falla(self) -> None:
        with tempfile.TemporaryDirectory() as temporal:
            ruta = Path(temporal) / "semgrep.json"
            ruta.write_text(
                json.dumps({"results": [
                    {"check_id": "x.nexum-tls-skip-verify", "path": "tests/fixtures/semgrep/inseguro/main.go"}
                ]}),
                encoding="utf-8",
            )
            correcto, problemas = fixture_semgrep.verificar(ruta, 1)
            self.assertFalse(correcto)
            self.assertTrue(any("no detectaron" in problema for problema in problemas))


if __name__ == "__main__":
    unittest.main()
