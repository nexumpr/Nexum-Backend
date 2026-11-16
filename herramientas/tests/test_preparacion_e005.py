"""Pruebas de herramientas/verificar_preparacion_e005.py.

Cubren las cuatro capas del verificador (integridad, deteccion, decisiones y deuda
documental) y demuestran que funciona ANTES y DESPUES de una enmienda autorizada:
una discrepancia resuelta no se declara FAIL solo por dejar de existir.
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

import verificar_preparacion_e005 as v  # noqa: E402

CARPETA_E005 = "E005-auth-password-forgot"


def api_minima(exito="202", esquema_respuesta="Accepted"):
    return {
        "openapi": "3.1.0",
        "paths": {
            "/api/v1/auth/password/forgot": {
                "post": {
                    "operationId": "nexum_e005",
                    "requestBody": {
                        "content": {"application/json": {"schema": {"$ref": "#/components/schemas/EmailRequest"}}}
                    },
                    "responses": {
                        exito: {
                            "content": {
                                "application/json": {"schema": {"$ref": f"#/components/schemas/{esquema_respuesta}"}}
                            }
                        }
                    },
                }
            }
        },
        "components": {
            "schemas": {
                "EmailRequest": {
                    "type": "object",
                    "additionalProperties": False,
                    "properties": {"email": {"type": "string"}},
                    "required": ["email"],
                },
                "Accepted": {
                    "type": "object",
                    "additionalProperties": False,
                    "properties": {"status": {"type": "string"}, "requestId": {"type": "string"}},
                    "required": ["status", "requestId"],
                },
            }
        },
    }


def fila_e005(**extra):
    base = {
        "id": "E005",
        "folder": CARPETA_E005,
        "pdf_pages": [21],
        "pdf_class": "Anunciado/no funcional",
        "sdd_method": "POST",
        "sdd_path": "/api/v1/auth/password/forgot",
        "pdf_method": "POST",
        "pdf_path": "/api/v1/auth/password/forgot",
        "sdd_success": "202",
        "pdf_success": "202",
        "sdd_security": "public",
        "sdd_request_schema": "EmailRequest",
        "pdf_request_fields": ["email", "locale"],
        "sdd_request_fields": ["email"],
        "pdf_response_fields": ["accepted", "message", "retryAfterSeconds"],
        "sdd_response_fields": ["status", "requestId"],
        "pdf_only_request_candidates": ["locale"],
        "pdf_only_response_candidates": ["accepted", "message", "retryAfterSeconds"],
        "differences": ["ENTRADA_CANDIDATA", "SALIDA_CANDIDATA", "PERSISTENCIA_OBSOLETA"],
    }
    base.update(extra)
    return base


def decision(eid, tipo, vigente, objetivo, estado="PENDIENTE"):
    return {
        "id": eid,
        "tipo": tipo,
        "valor_vigente": vigente,
        "valor_objetivo": objetivo,
        "estado": estado,
        "rfc": "RFC-016",
    }


class Arbol:
    """Arbol sintetico con la estructura y los archivos que exige el verificador."""

    def __init__(self, matriz=None, api=None, decisiones=None, con_ficha=True, con_spec=True, heredada=True):
        self.raiz = Path(tempfile.mkdtemp(prefix="nexum-pre-"))
        sdd = self.raiz / "sdd"
        (sdd / "03-endpoints/openapi").mkdir(parents=True)
        (sdd / "_control/H08/PREPARACION_E005_E075/fichas").mkdir(parents=True)
        (sdd / "03-endpoints/openapi/nexum-api-v1.json").write_text(
            json.dumps(api or api_minima()), encoding="utf-8"
        )
        self.matriz = matriz if matriz is not None else [fila_e005()]
        (sdd / "_control/H08/PREPARACION_E005_E075/matriz_e005_e075.json").write_text(
            json.dumps(self.matriz), encoding="utf-8"
        )
        (sdd / "_control/H08/DECISIONES_CONTRATO_E005_E075.json").write_text(
            json.dumps({"decisiones": decisiones or []}), encoding="utf-8"
        )
        if con_ficha:
            for fila in self.matriz:
                (sdd / "_control/H08/PREPARACION_E005_E075/fichas" / f"{fila['id']}.md").write_text(
                    "# ficha\n", encoding="utf-8"
                )
        for fila in self.matriz:
            carpeta = sdd / "03-endpoints" / fila["folder"]
            carpeta.mkdir(parents=True, exist_ok=True)
            for doc in ("spec.md", "plan.md", "tasks.md", "checklist.md"):
                carpeta.joinpath(doc).write_text(("SESSIONHASH\n" if heredada else "REFRESH#\n") * 30, encoding="utf-8")
        for i in range(6, 12):
            carpeta = sdd / "03-endpoints" / f"E{i:03d}-x"
            carpeta.mkdir(parents=True, exist_ok=True)
            for doc in ("spec.md", "plan.md", "tasks.md", "checklist.md"):
                carpeta.joinpath(doc).write_text(("SESSIONHASH\n" if heredada else "REFRESH#\n") * 30, encoding="utf-8")
        if not con_spec:
            shutil.rmtree(sdd / "03-endpoints" / CARPETA_E005)

    def cerrar(self):
        shutil.rmtree(self.raiz, ignore_errors=True)


class Resolucion(unittest.TestCase):
    def test_ref_irresoluble_falla_cerrado(self):
        with self.assertRaises(v.ErrorVerificacion):
            v.resolver({"$ref": "#/components/schemas/NoExiste"}, {"Otro": {}})

    def test_ref_externo_no_soportado(self):
        with self.assertRaises(v.ErrorVerificacion):
            v.resolver({"$ref": "https://ejemplo/esquema.json"}, {})

    def test_aplanar_arrays_y_anidados(self):
        comp = {
            "A": {
                "type": "object",
                "properties": {
                    "items": {"type": "array", "items": {"$ref": "#/components/schemas/B"}},
                    "simple": {"type": "string"},
                },
            },
            "B": {"type": "object", "properties": {"id": {"type": "string"}}},
        }
        campos = v.aplanar({"$ref": "#/components/schemas/A"}, comp)
        self.assertTrue({"items", "items.id", "simple"} <= campos)

    def test_aplanar_esquema_compuesto(self):
        comp = {
            "A": {
                "allOf": [
                    {"type": "object", "properties": {"uno": {"type": "string"}}},
                    {"type": "object", "properties": {"dos": {"type": "string"}}},
                ]
            }
        }
        self.assertEqual(v.aplanar({"$ref": "#/components/schemas/A"}, comp), {"uno", "dos"})

    def test_normalizar_quita_corchetes(self):
        self.assertEqual(v.normalizar("sessions[].id"), "sessions.id")


class Integridad(unittest.TestCase):
    def test_repositorio_real_pasa(self):
        r = v.verificar(RAIZ)
        self.assertEqual(r["resultado"], "PREPARACION_CONTRATO_PASS", r["problemas"][:10])
        self.assertEqual(r["operaciones"], 71)
        self.assertEqual(len(r["discrepancias"]), 5)
        self.assertEqual(r["decisiones"], 12)
        # Tras los Gates de 2026-11-16 las 12 decisiones estan aprobadas y aplicadas.
        self.assertEqual(r["decisiones_aprobadas"], 12)
        self.assertEqual(r["pendientes"], [])

    def test_arbol_sintetico_valido_pasa(self):
        arbol = Arbol()
        try:
            r = v.verificar(arbol.raiz)
            self.assertEqual(r["resultado"], "PREPARACION_CONTRATO_PASS", r["problemas"])
            self.assertEqual(r["discrepancias"], [])
            self.assertEqual(len(r["pendientes"]), 7)  # deuda SESSIONHASH en E005-E011
        finally:
            arbol.cerrar()

    def test_candidatos_ausentes_no_son_error(self):
        # `locale`, `accepted`, `message`, `retryAfterSeconds` no estan en el contrato:
        # son candidatos de auditoria, no defectos del backend.
        arbol = Arbol()
        try:
            r = v.verificar(arbol.raiz)
            self.assertEqual(r["problemas"], [])
            entrada = r["detalle"][0]
            self.assertIn("locale", entrada["candidatos_request_ausentes"])
            self.assertIn("accepted", entrada["candidatos_response_ausentes"])
        finally:
            arbol.cerrar()

    def test_codigo_de_exito_ausente_falla(self):
        arbol = Arbol(matriz=[fila_e005(sdd_success="201")])
        try:
            self.assertTrue(any("EXITO_AUSENTE_OPENAPI" in p for p in v.verificar(arbol.raiz)["problemas"]))
        finally:
            arbol.cerrar()

    def test_campo_declarado_inexistente_falla(self):
        arbol = Arbol(matriz=[fila_e005(sdd_response_fields=["status", "campoInventado"])])
        try:
            self.assertIn("CAMPO_RESPUESTA_NO_DECLARADO E005 campoInventado", v.verificar(arbol.raiz)["problemas"])
        finally:
            arbol.cerrar()

    def test_esquema_request_inexistente_falla(self):
        arbol = Arbol(matriz=[fila_e005(sdd_request_schema="NoExiste")])
        try:
            self.assertTrue(any("ESQUEMA_REQUEST_AUSENTE" in p for p in v.verificar(arbol.raiz)["problemas"]))
        finally:
            arbol.cerrar()

    def test_sin_contenido_no_exige_esquema(self):
        api = api_minima()
        del api["paths"]["/api/v1/auth/password/forgot"]["post"]["requestBody"]
        arbol = Arbol(
            matriz=[fila_e005(sdd_request_schema="sin contenido", sdd_request_fields=[], pdf_only_request_candidates=[])],
            api=api,
        )
        try:
            problemas = v.verificar(arbol.raiz)["problemas"]
            self.assertFalse(any("ESQUEMA_REQUEST" in p or "CUERPO" in p for p in problemas), problemas)
        finally:
            arbol.cerrar()

    def test_sin_contenido_con_cuerpo_real_falla(self):
        # Contradiccion: la matriz declara ausencia de cuerpo y el OpenAPI lo tiene.
        arbol = Arbol(
            matriz=[fila_e005(sdd_request_schema="sin contenido", sdd_request_fields=[], pdf_only_request_candidates=[])]
        )
        try:
            self.assertIn("CUERPO_INESPERADO E005", v.verificar(arbol.raiz)["problemas"])
        finally:
            arbol.cerrar()

    def test_ficha_ausente_falla(self):
        arbol = Arbol(con_ficha=False)
        try:
            self.assertIn("FICHA_AUSENTE E005", v.verificar(arbol.raiz)["problemas"])
        finally:
            arbol.cerrar()

    def test_carpeta_spec_ausente_falla(self):
        arbol = Arbol(con_spec=False)
        try:
            self.assertTrue(any(p.startswith("CARPETA_SPEC_AUSENTE") for p in v.verificar(arbol.raiz)["problemas"]))
        finally:
            arbol.cerrar()

    def test_ruta_inexistente_falla(self):
        arbol = Arbol(matriz=[fila_e005(sdd_path="/api/v1/no-existe", pdf_path="/api/v1/no-existe")])
        try:
            self.assertTrue(any(p.startswith("RUTA_AUSENTE_OPENAPI") for p in v.verificar(arbol.raiz)["problemas"]))
        finally:
            arbol.cerrar()

    def test_operacion_duplicada_falla(self):
        arbol = Arbol(matriz=[fila_e005(), fila_e005()])
        try:
            self.assertIn("ID_DUPLICADO E005", v.verificar(arbol.raiz)["problemas"])
        finally:
            arbol.cerrar()


class Decisiones(unittest.TestCase):
    """El contrato debe seguir al valor vigente mientras la decision esta PENDIENTE."""

    def _arbol(self, estado, exito="202", **kw):
        matriz = [fila_e005(sdd_success=exito, pdf_success="200" if exito != "200" else "201")]
        decisiones = [decision("E005", "HTTP_EXITO", "202", "200", estado)]
        return Arbol(matriz=matriz, api=api_minima(exito=exito), decisiones=decisiones, **kw)

    def test_pendiente_y_contrato_vigente_pasa(self):
        arbol = self._arbol("PENDIENTE", exito="202")
        try:
            r = v.verificar(arbol.raiz)
            self.assertEqual(r["resultado"], "PREPARACION_CONTRATO_PASS")
            self.assertEqual(r["decisiones_aprobadas"], 0)
            self.assertTrue(any(p.startswith("E005:HTTP_EXITO") for p in r["pendientes"]))
        finally:
            arbol.cerrar()

    def test_pendiente_y_contrato_cambiado_falla(self):
        # Cambio de contrato sin Gate: el verificador debe detectarlo.
        arbol = self._arbol("PENDIENTE", exito="200")
        try:
            problemas = v.verificar(arbol.raiz)["problemas"]
            self.assertTrue(any("CONTRATO_FUERA_DE_DECISION E005:HTTP_EXITO" in p for p in problemas), problemas)
        finally:
            arbol.cerrar()

    def test_aprobada_y_enmienda_aplicada_pasa(self):
        arbol = self._arbol("APROBADA", exito="200")
        try:
            r = v.verificar(arbol.raiz)
            self.assertEqual(r["resultado"], "PREPARACION_CONTRATO_PASS", r["problemas"])
            self.assertEqual(r["decisiones_aprobadas"], 1)
        finally:
            arbol.cerrar()

    def test_aprobada_y_enmienda_no_aplicada_falla(self):
        arbol = self._arbol("APROBADA", exito="202")
        try:
            problemas = v.verificar(arbol.raiz)["problemas"]
            self.assertTrue(any("CONTRATO_FUERA_DE_DECISION E005:HTTP_EXITO" in p for p in problemas), problemas)
        finally:
            arbol.cerrar()

    def test_decision_sin_hallazgo_falla(self):
        arbol = Arbol(decisiones=[decision("E005", "HTTP_EXITO", "202", "200", "PENDIENTE")])
        try:
            # Sin discrepancia SDD/PDF no hay hallazgo que la decision gobierne.
            self.assertIn("DECISION_SIN_HALLAZGO E005:HTTP_EXITO", v.verificar(arbol.raiz)["problemas"])
        finally:
            arbol.cerrar()

    def test_estado_de_decision_invalido_falla_cerrado(self):
        arbol = Arbol(decisiones=[decision("E005", "HTTP_EXITO", "202", "200", "QUIZA")])
        try:
            with self.assertRaises(v.ErrorVerificacion):
                v.verificar(arbol.raiz)
        finally:
            arbol.cerrar()

    def test_decision_duplicada_falla_cerrado(self):
        d = decision("E005", "HTTP_EXITO", "202", "200")
        arbol = Arbol(decisiones=[d, dict(d)])
        try:
            with self.assertRaises(v.ErrorVerificacion):
                v.verificar(arbol.raiz)
        finally:
            arbol.cerrar()

    def test_decisiones_ausentes_falla_cerrado(self):
        arbol = Arbol()
        try:
            (arbol.raiz / v.DECISIONES).unlink()
            with self.assertRaises(v.ErrorVerificacion):
                v.verificar(arbol.raiz)
        finally:
            arbol.cerrar()


class DeudaDocumental(unittest.TestCase):
    def test_herencia_pendiente_no_bloquea(self):
        arbol = Arbol(decisiones=[decision("E005", "PERSISTENCIA", "SESSIONHASH", "REFRESH#")])
        try:
            r = v.verificar(arbol.raiz)
            self.assertEqual(r["resultado"], "PREPARACION_CONTRATO_PASS", r["problemas"])
            self.assertTrue(any("E005:PERSISTENCIA" in p for p in r["pendientes"]))
        finally:
            arbol.cerrar()

    def test_herencia_aprobada_y_no_corregida_falla(self):
        arbol = Arbol(decisiones=[decision("E005", "PERSISTENCIA", "SESSIONHASH", "REFRESH#", "APROBADA")])
        try:
            self.assertTrue(any(p.startswith("PERSISTENCIA_NO_CORREGIDA E005") for p in v.verificar(arbol.raiz)["problemas"]))
        finally:
            arbol.cerrar()

    def test_herencia_aprobada_y_corregida_pasa(self):
        arbol = Arbol(heredada=False, decisiones=[decision("E005", "PERSISTENCIA", "SESSIONHASH", "REFRESH#", "APROBADA")])
        try:
            r = v.verificar(arbol.raiz)
            self.assertEqual(r["resultado"], "PREPARACION_CONTRATO_PASS", r["problemas"])
            self.assertFalse(r["pendientes"])
        finally:
            arbol.cerrar()


if __name__ == "__main__":
    unittest.main()
