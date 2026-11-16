"""Pruebas de herramientas/clasificar_contratos_e005.py.

Garantizan que NINGUNA diferencia queda sin clase y que las reglas de clasificacion
se aplican como estan documentadas. No aprueban contratos.
"""

from __future__ import annotations

import json
import shutil
import sys
import tempfile
import unittest
from pathlib import Path

RAIZ = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(RAIZ / "herramientas"))

import clasificar_contratos_e005 as c  # noqa: E402


def api(exito="202", seguridad=None):
    return {
        "openapi": "3.1.0",
        "paths": {
            "/api/v1/auth/password/forgot": {
                "post": {
                    "operationId": "nexum_e005",
                    "security": seguridad if seguridad is not None else [],
                    "requestBody": {"content": {"application/json": {"schema": {"$ref": "#/components/schemas/EmailRequest"}}}},
                    "responses": {
                        exito: {"content": {"application/json": {"schema": {"$ref": "#/components/schemas/Accepted"}}}}
                    },
                }
            }
        },
        "components": {
            "schemas": {
                "EmailRequest": {"type": "object", "properties": {"email": {"type": "string"}}},
                "Accepted": {
                    "type": "object",
                    "properties": {"status": {"type": "string"}, "requestId": {"type": "string"}},
                },
            }
        },
    }


def fila(**extra):
    base = {
        "id": "E005",
        "folder": "E005-auth-password-forgot",
        "sdd_method": "POST",
        "sdd_path": "/api/v1/auth/password/forgot",
        "pdf_method": "POST",
        "pdf_path": "/api/v1/auth/password/forgot",
        "sdd_success": "202",
        "pdf_success": "202",
        "sdd_request_schema": "EmailRequest",
        "sdd_request_fields": ["email"],
        "sdd_response_fields": ["status", "requestId"],
        "pdf_only_request_candidates": ["locale"],
        "pdf_only_response_candidates": ["accepted"],
    }
    base.update(extra)
    return base


class Arbol:
    def __init__(self, matriz=None, api_datos=None, con_spec=True):
        self.raiz = Path(tempfile.mkdtemp(prefix="nexum-clas-"))
        (self.raiz / "sdd/03-endpoints/openapi").mkdir(parents=True)
        (self.raiz / "sdd/_control/H08/PREPARACION_E005_E075").mkdir(parents=True)
        (self.raiz / c.OPENAPI).write_text(json.dumps(api_datos or api()), encoding="utf-8")
        matriz = matriz if matriz is not None else [fila()]
        (self.raiz / c.MATRIZ).write_text(json.dumps(matriz), encoding="utf-8")
        for f in matriz:
            carpeta = self.raiz / "sdd/03-endpoints" / f["folder"]
            carpeta.mkdir(parents=True, exist_ok=True)
            for doc in ("spec.md", "plan.md", "tasks.md", "checklist.md"):
                texto = "SESSIONHASH#digest\n" if con_spec else "REFRESH#digest/SESSION\n"
                (carpeta / doc).write_text(texto * 40, encoding="utf-8")

    def cerrar(self):
        shutil.rmtree(self.raiz, ignore_errors=True)


class Clasificacion(unittest.TestCase):
    def test_repositorio_real_no_deja_diferencias_sin_clase(self):
        r = c.clasificar(RAIZ)
        self.assertEqual(r["resumen"]["sin_clasificar"], 0)
        self.assertEqual(r["resumen"]["operaciones"], 71)
        self.assertGreater(r["resumen"]["diferencias"], 700)

    def test_las_cinco_http_son_compatibilidad(self):
        r = c.clasificar(RAIZ)
        con_http = {
            o["id"]: [d for d in o["diferencias"] if d["tipo"] == "HTTP"]
            for o in r["operaciones"]
        }
        for eid in ("E017", "E031", "E037", "E042", "E063"):
            self.assertEqual(len(con_http[eid]), 1, eid)
            self.assertEqual(con_http[eid][0]["clase"], "COMPATIBILIDAD", eid)
        for eid in ("E005", "E006", "E012"):
            self.assertEqual(con_http[eid], [], eid)

    def test_persistencia_heredada_es_obsoleta(self):
        r = c.clasificar(RAIZ)
        for o in r["operaciones"]:
            if o["id"] in c.PERSISTENCIA:
                self.assertEqual(o["resumen"]["OBSOLETA"], 1, o["id"])

    def test_campo_material_es_esencial(self):
        arbol = Arbol()
        try:
            r = c.clasificar(arbol.raiz)
            clases = {(d["campo"], d["clase"]) for d in r["operaciones"][0]["diferencias"]}
            self.assertIn(("persistencia", "OBSOLETA"), clases)
            # `locale` es material (idioma del correo): COMPATIBILIDAD, no opcional.
            self.assertIn(("locale", "COMPATIBILIDAD"), clases)
            # `accepted` si es derivable del contrato: opcional confirmada.
            self.assertIn(("accepted", "OPCIONAL"), clases)
        finally:
            arbol.cerrar()

    def test_cero_pendientes_de_decision(self):
        r = c.clasificar(RAIZ)
        self.assertEqual(r["resumen"]["por_decision"]["PENDIENTE_DE_DECISION"], 0)
        self.assertEqual(r["resumen"]["operaciones_con_pendientes"], 0)

    def test_renombrado_es_compatibilidad(self):
        datos = api()
        datos["paths"]["/api/v1/auth/password/forgot"]["post"]["operationId"] = "nexum_e006"
        matriz = [fila(id="E006", pdf_only_request_candidates=["resetToken"])]
        arbol = Arbol(matriz=matriz, api_datos=datos)
        try:
            r = c.clasificar(arbol.raiz)
            d = [x for x in r["operaciones"][0]["diferencias"] if x["campo"] == "resetToken"][0]
            self.assertEqual(d["clase"], "COMPATIBILIDAD")
            self.assertIn("token", d["decision"])
        finally:
            arbol.cerrar()

    def test_http_se_conserva_por_defecto_sin_decision(self):
        matriz = [fila(id="E012", sdd_success="200", pdf_success="201")]
        arbol = Arbol(matriz=matriz)
        try:
            r = c.clasificar(arbol.raiz)
            http = [d for d in r["operaciones"][0]["diferencias"] if d["tipo"] == "HTTP"][0]
            self.assertEqual(http["decision"], "CONSERVAR 200")
        finally:
            arbol.cerrar()

    def test_ninguna_operacion_se_declara_congelada(self):
        r = c.clasificar(RAIZ)
        for o in r["operaciones"]:
            self.assertEqual(o["contract_frozen"], "NOT_VERIFIED", o["id"])
            self.assertTrue(o["motivos_no_congelado"], o["id"])


if __name__ == "__main__":
    unittest.main()
