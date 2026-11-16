"""Pruebas de herramientas/verificar_permisos_opencode.py (estatico, sin motor).

Demuestran la semantica de resolucion (ultima coincidencia + denegacion dura) y que
el suelo de seguridad del `opencode.json` versionado se mantiene. No sustituyen la
prueba en el motor real de OpenCode, que exige sesion y aprobacion humana.
"""

from __future__ import annotations

import json
import sys
import unittest
from pathlib import Path

RAIZ = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(RAIZ / "herramientas"))

import verificar_permisos_opencode as vp  # noqa: E402

CONFIG = RAIZ / "opencode.json"


def config_base(permisos, policies=None):
    return {"permissions": permisos, "policies": policies or []}


def p(accion, recurso, efecto):
    return {"action": accion, "resource": recurso, "effect": efecto}


class Resolucion(unittest.TestCase):
    def test_ultima_coincidencia_prevalece(self):
        pol = vp.Politica(
            config_base(
                [
                    p("shell", "aws *", "deny"),
                    p("shell", "aws * put-*", "deny"),
                    p("shell", "aws s3api put-object *", "ask"),
                ]
            )
        )
        self.assertEqual(pol.resolver("shell", "aws s3api put-object --bucket b")[0], "ask")
        self.assertEqual(pol.resolver("shell", "aws s3api delete-object --bucket b")[0], "deny")

    def test_sin_coincidencia_deniega(self):
        pol = vp.Politica(config_base([p("shell", "git status*", "allow")]))
        self.assertEqual(pol.resolver("shell", "curl http://x")[0], "deny")

    def test_policy_endurece_sobre_allow(self):
        pol = vp.Politica(
            config_base(
                [p("shell", "terraform apply*", "ask")],
                [{"action": "tool.use", "resource": "shell:terraform apply*", "effect": "deny"}],
            )
        )
        self.assertEqual(pol.resolver("shell", "terraform apply plan.tfplan")[0], "deny")

    def test_policy_no_concede_permiso(self):
        pol = vp.Politica(
            config_base(
                [p("shell", "aws *", "deny")],
                [{"action": "tool.use", "resource": "shell:aws s3api put-object *", "effect": "allow"}],
            )
        )
        self.assertEqual(pol.resolver("shell", "aws s3api put-object --bucket b")[0], "deny")

    def test_permiso_allow_sin_policy_es_suficiente(self):
        # Evidencia del motor real: `git status` se ejecuto sin entrada en `policies`.
        pol = vp.Politica(config_base([p("shell", "git status*", "allow")]))
        self.assertEqual(pol.resolver("shell", "git status --porcelain")[0], "allow")

    def test_comodin_no_atraviesa_de_forma_laxa(self):
        pol = vp.Politica(config_base([p("shell", "git push origin main*", "deny"), p("shell", "git push *", "ask")]))
        self.assertEqual(pol.resolver("shell", "git push origin main")[0], "ask")
        pol2 = vp.Politica(
            config_base([p("shell", "git push *", "ask"), p("shell", "git push origin main*", "deny")])
        )
        self.assertEqual(pol2.resolver("shell", "git push origin main")[0], "deny")

    def test_config_invalida_falla_cerrado(self):
        with self.assertRaises(ValueError):
            vp.Politica({"permissions": [{"action": "shell", "resource": "x"}], "policies": []})
        with self.assertRaises(ValueError):
            vp.Politica({"permissions": [p("shell", "x", "permitir")], "policies": []})


class SueloDeGobierno(unittest.TestCase):
    def test_opencode_versionado_cumple_el_suelo(self):
        r = vp.verificar(CONFIG, objetivo_permanente=False)
        self.assertEqual(r["fallos"], [])
        self.assertEqual(r["resultado"], "PERMISOS_PASS")
        self.assertGreaterEqual(r["casos"], 33)

    def test_detecta_destruccion_concedida(self):
        datos = json.loads(CONFIG.read_text(encoding="utf-8"))
        datos["permissions"] = [x for x in datos["permissions"] if x["resource"] != "terraform destroy*"]
        datos["permissions"].append(p("shell", "terraform destroy*", "allow"))
        datos["policies"] = [x for x in datos["policies"] if x["resource"] != "shell:terraform destroy*"]
        tmp = RAIZ / "dist/tmp/pre-e005/permisos_inseguro.json"
        tmp.parent.mkdir(parents=True, exist_ok=True)
        tmp.write_text(json.dumps(datos, indent=2), encoding="utf-8")
        try:
            r = vp.verificar(tmp, objetivo_permanente=False)
            self.assertEqual(r["resultado"], "PERMISOS_FAIL")
            self.assertTrue(any("terraform destroy" in f for f in r["fallos"]))
        finally:
            tmp.unlink(missing_ok=True)

    def test_detecta_lectura_de_env_concedida(self):
        datos = json.loads(CONFIG.read_text(encoding="utf-8"))
        datos["permissions"].append(p("read", "*.env*", "allow"))
        datos["policies"] = [x for x in datos["policies"] if x["resource"] != "read:*.env*"]
        tmp = RAIZ / "dist/tmp/pre-e005/permisos_env.json"
        tmp.parent.mkdir(parents=True, exist_ok=True)
        tmp.write_text(json.dumps(datos, indent=2), encoding="utf-8")
        try:
            r = vp.verificar(tmp, objetivo_permanente=False)
            self.assertEqual(r["resultado"], "PERMISOS_FAIL")
            self.assertTrue(any(".env" in f for f in r["fallos"]))
        finally:
            tmp.unlink(missing_ok=True)


class ConstructorPermanente(unittest.TestCase):
    """El constructor debe ser determinista y fallar cerrado si la base cambia."""

    BASE = RAIZ / "dist/tmp/pre-e005/base.head.json"

    def _base(self):
        import subprocess

        return subprocess.run(
            ["git", "show", "HEAD:opencode.json"], cwd=RAIZ, capture_output=True, check=True
        ).stdout.decode("utf-8")

    def test_determinista(self):
        import construir_opencode_permanente as cp

        texto = self._base()
        a = cp.construir(texto)
        b = cp.construir(texto)
        self.assertEqual(a, b)
        datos = cp.validar(a)
        self.assertEqual(len(datos["permissions"]), 184)
        self.assertEqual(len(datos["policies"]), 47)

    def test_base_sin_ancla_falla(self):
        import construir_opencode_permanente as cp

        with self.assertRaises(cp.ErrorConstruccion):
            cp.construir('{"permissions": [], "policies": []}')

    def test_ancla_duplicada_falla(self):
        import construir_opencode_permanente as cp

        # Base sintetica con el ancla T1 repetida: el generador debe fallar, no elegir una.
        ancla = '      "resource": "sdd/**/spec.md",\n      "effect": "deny"'
        base = '{"permissions": [\n' + ancla + ",\n" + ancla + '\n], "policies": []}'
        with self.assertRaises(cp.ErrorConstruccion):
            cp.construir(base)

    def test_resultado_es_json_valido_y_conserva_denegaciones(self):
        import construir_opencode_permanente as cp

        datos = json.loads(cp.construir(self._base()))
        claves = {(e["action"], e["resource"], e["effect"]) for e in datos["permissions"]}
        for accion, recurso in [
            ("shell", "terraform destroy*"),
            ("shell", "git push origin main*"),
            ("shell", "git push --force*"),
            ("shell", "docker *"),
            ("shell", "wsl *"),
            ("edit", "constitution.md"),
            ("read", "*.env*"),
        ]:
            self.assertIn((accion, recurso, "deny"), claves, f"{accion} {recurso}")
        self.assertNotIn(("shell", "aws *", "allow"), claves)


if __name__ == "__main__":
    unittest.main()

