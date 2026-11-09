"""Pruebas adversas de publicacion y compatibilidad de la API (I-150)."""

from __future__ import annotations

import copy
import csv
import hashlib
import json
import sys
import tempfile
import unittest
from pathlib import Path

RAIZ = Path(__file__).resolve().parents[3]
sys.path.insert(0, str(RAIZ / "herramientas"))

import verificar_publicacion_i150 as pub  # noqa: E402

FIXTURES = RAIZ / "tests" / "fixtures" / "publicacion"
OPENAPI = RAIZ / "sdd" / "03-endpoints" / "openapi" / "nexum-api-v1.json"
MATRIZ = RAIZ / "sdd" / "03-endpoints" / "MATRIZ_OPERACIONES_H06.csv"


def _fixture(nombre: str) -> dict:
    return copy.deepcopy(json.loads((FIXTURES / nombre).read_text(encoding="utf-8")))


def _json(datos: object) -> Path:
    manejador, ruta = tempfile.mkstemp(suffix=".json")
    with open(manejador, "w", encoding="utf-8") as fh:
        json.dump(datos, fh, ensure_ascii=False)
    return Path(ruta)


class ContratoOpenAPI(unittest.TestCase):
    def test_contrato_valido(self) -> None:
        resultado = pub.validar_contrato()
        self.assertEqual(resultado["estado"], "CONTRATO_VALIDO")
        self.assertEqual(resultado["openapi"], "3.1.0")
        self.assertGreater(resultado["rutas"], 50)
        self.assertTrue(resultado["errores_con_traceid"])

    def test_version_distinta_se_rechaza(self) -> None:
        datos = json.loads(OPENAPI.read_text(encoding="utf-8"))
        datos["openapi"] = "3.0.3"
        self.assertEqual(pub.validar_contrato(datos)["estado"], "CONTRATO_RECHAZADO")

    def test_ruta_fuera_de_api_v1(self) -> None:
        datos = json.loads(OPENAPI.read_text(encoding="utf-8"))
        datos["paths"]["/v2/algo"] = datos["paths"]["/api/v1/auth/login"]
        resultado = pub.validar_contrato(datos)
        self.assertEqual(resultado["estado"], "CONTRATO_RECHAZADO")
        self.assertTrue(any("/v2/algo" in p for p in resultado["problemas"]))

    def test_url_aws_real_antes_de_desplegar(self) -> None:
        datos = json.loads(OPENAPI.read_text(encoding="utf-8"))
        datos["servers"][0]["url"] = "https://abc.cloudfront.net"
        datos["servers"][0].pop("x-nexum-placeholder", None)
        self.assertEqual(pub.validar_contrato(datos)["estado"], "CONTRATO_RECHAZADO")

    def test_url_sin_https(self) -> None:
        datos = json.loads(OPENAPI.read_text(encoding="utf-8"))
        datos["servers"][0]["url"] = "http://api.example.invalid"
        self.assertEqual(pub.validar_contrato(datos)["estado"], "CONTRATO_RECHAZADO")

    def test_dinero_como_number_se_rechaza(self) -> None:
        datos = json.loads(OPENAPI.read_text(encoding="utf-8"))
        datos["components"]["schemas"]["Roto"] = {
            "type": "object",
            "properties": {"amount": {"type": "number"}},
        }
        resultado = pub.validar_contrato(datos)
        self.assertEqual(resultado["estado"], "CONTRATO_RECHAZADO")
        self.assertTrue(any("number" in p for p in resultado["problemas"]))

    def test_sin_esquema_de_error_con_traceid(self) -> None:
        datos = json.loads(OPENAPI.read_text(encoding="utf-8"))
        datos["components"]["schemas"] = {
            k: v for k, v in datos["components"]["schemas"].items() if "traceId" not in json.dumps(v)
        }
        resultado = pub.validar_contrato(datos)
        self.assertFalse(resultado["errores_con_traceid"])
        self.assertEqual(resultado["estado"], "CONTRATO_RECHAZADO")

    def test_archivo_inexistente_es_error(self) -> None:
        with self.assertRaises(pub.ErrorUso):
            pub.validar_contrato(ruta=Path(tempfile.mkdtemp()) / "no.json")


class Publicacion(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.base = _fixture("publicacion.json")

    def test_plan_valido_con_checksum_real(self) -> None:
        resultado = pub.validar_publicacion(self.base)
        self.assertEqual(resultado["estado"], "PUBLICACION_VALIDA")
        self.assertTrue(resultado["checksum_verificado"])
        esperado = hashlib.sha256(OPENAPI.read_bytes()).hexdigest()
        self.assertEqual(self.base["artefacto_openapi"]["sha256"], esperado)

    def test_checksum_incorrecto(self) -> None:
        datos = copy.deepcopy(self.base)
        datos["artefacto_openapi"]["sha256"] = "0" * 64
        resultado = pub.validar_publicacion(datos)
        self.assertEqual(resultado["estado"], "PUBLICACION_RECHAZADA")
        self.assertTrue(any("checksum" in p for p in resultado["problemas"]))

    def test_artefacto_inexistente(self) -> None:
        datos = copy.deepcopy(self.base)
        datos["artefacto_openapi"]["ruta"] = "sdd/no-existe.json"
        self.assertEqual(pub.validar_publicacion(datos)["estado"], "PUBLICACION_RECHAZADA")

    def test_url_sin_https(self) -> None:
        datos = copy.deepcopy(self.base)
        datos["url_publica"] = "http://api.nexum.example"
        self.assertEqual(pub.validar_publicacion(datos)["estado"], "PUBLICACION_RECHAZADA")

    def test_cors_comodin_se_rechaza(self) -> None:
        datos = copy.deepcopy(self.base)
        datos["cors"]["origenes"] = ["*"]
        resultado = pub.validar_publicacion(datos)
        self.assertEqual(resultado["estado"], "PUBLICACION_RECHAZADA")
        self.assertTrue(any("'*'" in p for p in resultado["problemas"]))

    def test_cors_vacio_se_rechaza(self) -> None:
        datos = copy.deepcopy(self.base)
        datos["cors"]["origenes"] = []
        self.assertEqual(pub.validar_publicacion(datos)["estado"], "PUBLICACION_RECHAZADA")

    def test_ejemplos_con_credenciales(self) -> None:
        datos = copy.deepcopy(self.base)
        datos["ejemplos"]["con_credenciales"] = True
        self.assertEqual(pub.validar_publicacion(datos)["estado"], "PUBLICACION_RECHAZADA")

    def test_sin_datos_es_error(self) -> None:
        with self.assertRaises(pub.ErrorUso):
            pub.validar_publicacion(None)


class Compatibilidad(unittest.TestCase):
    def test_matriz_compatible(self) -> None:
        resultado = pub.validar_compatibilidad()
        self.assertEqual(resultado["estado"], "MATRIZ_COMPATIBLE")
        self.assertEqual(resultado["filas"], 75)
        self.assertEqual(resultado["clases"], {"M": 33, "S": 23, "E": 12, "O": 7})

    def test_identificador_faltante(self) -> None:
        with MATRIZ.open(encoding="utf-8") as manejador:
            filas = list(csv.DictReader(manejador))
        filas.pop()
        self.assertEqual(pub.validar_compatibilidad(filas)["estado"], "MATRIZ_INCOMPATIBLE")

    def test_clase_desconocida(self) -> None:
        with MATRIZ.open(encoding="utf-8") as manejador:
            filas = list(csv.DictReader(manejador))
        filas[0]["clase_h04"] = "X"
        resultado = pub.validar_compatibilidad(filas)
        self.assertEqual(resultado["estado"], "MATRIZ_INCOMPATIBLE")
        self.assertTrue(any("clase desconocida" in p for p in resultado["problemas"]))

    def test_conteo_de_clases_alterado(self) -> None:
        with MATRIZ.open(encoding="utf-8") as manejador:
            filas = list(csv.DictReader(manejador))
        filas[0]["clase_h04"] = "S"
        self.assertEqual(pub.validar_compatibilidad(filas)["estado"], "MATRIZ_INCOMPATIBLE")

    def test_ruta_fuera_de_api_v1(self) -> None:
        with MATRIZ.open(encoding="utf-8") as manejador:
            filas = list(csv.DictReader(manejador))
        filas[0]["ruta"] = "/v2/auth/register"
        self.assertEqual(pub.validar_compatibilidad(filas)["estado"], "MATRIZ_INCOMPATIBLE")

    def test_metodo_invalido(self) -> None:
        with MATRIZ.open(encoding="utf-8") as manejador:
            filas = list(csv.DictReader(manejador))
        filas[0]["metodo"] = "FETCH"
        self.assertEqual(pub.validar_compatibilidad(filas)["estado"], "MATRIZ_INCOMPATIBLE")

    def test_204_sin_esquemas_es_legitimo(self) -> None:
        with MATRIZ.open(encoding="utf-8") as manejador:
            filas = list(csv.DictReader(manejador))
        sin_cuerpo = [f for f in filas if f["http_exito"] == "204"]
        self.assertTrue(sin_cuerpo)
        self.assertEqual(pub.validar_compatibilidad(filas)["estado"], "MATRIZ_COMPATIBLE")

    def test_columna_obligatoria_vacia(self) -> None:
        with MATRIZ.open(encoding="utf-8") as manejador:
            filas = list(csv.DictReader(manejador))
        filas[3]["regla_clave"] = ""
        self.assertEqual(pub.validar_compatibilidad(filas)["estado"], "MATRIZ_INCOMPATIBLE")


class EvidenciaHTTP(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.base = _fixture("evidencia.json")

    def test_plan_valido_y_no_ejecutado(self) -> None:
        resultado = pub.validar_evidencia(self.base)
        self.assertEqual(resultado["estado"], "PLAN_VALIDO")
        self.assertEqual(resultado["requeridas"], 13)
        self.assertEqual(resultado["ejecucion_aws"], "NOT_RUN")
        self.assertEqual(resultado["declaran_pass_sin_aws"], [])

    def test_faltan_pruebas(self) -> None:
        datos = copy.deepcopy(self.base)
        datos["pruebas"] = [p for p in datos["pruebas"] if p["id"] != "dos-nodos"]
        resultado = pub.validar_evidencia(datos)
        self.assertEqual(resultado["estado"], "PLAN_RECHAZADO")
        self.assertIn("dos-nodos", resultado["faltantes"])

    def test_pass_sin_aws_se_rechaza(self) -> None:
        datos = copy.deepcopy(self.base)
        datos["pruebas"][0]["estado"] = "PASS"
        resultado = pub.validar_evidencia(datos)
        self.assertEqual(resultado["estado"], "PLAN_RECHAZADO")
        self.assertIn("health-live", resultado["declaran_pass_sin_aws"])

    def test_pass_sin_huella_se_rechaza(self) -> None:
        datos = copy.deepcopy(self.base)
        for prueba in datos["pruebas"]:
            if prueba["id"] == "login":
                prueba["estado"] = "PASS"
                prueba["requiere_aws"] = False
        resultado = pub.validar_evidencia(datos)
        self.assertEqual(resultado["estado"], "PLAN_RECHAZADO")
        self.assertTrue(any("huella" in p for p in resultado["problemas"]))

    def test_cliente_no_independiente(self) -> None:
        datos = copy.deepcopy(self.base)
        datos["pruebas"][2]["cliente_independiente"] = False
        self.assertEqual(pub.validar_evidencia(datos)["estado"], "PLAN_RECHAZADO")

    def test_estado_desconocido(self) -> None:
        datos = copy.deepcopy(self.base)
        datos["pruebas"][4]["estado"] = "OK"
        self.assertEqual(pub.validar_evidencia(datos)["estado"], "PLAN_RECHAZADO")

    def test_sin_datos_es_error(self) -> None:
        with self.assertRaises(pub.ErrorUso):
            pub.validar_evidencia(None)


class DeprecacionYAlcance(unittest.TestCase):
    def test_politica_valida(self) -> None:
        self.assertEqual(
            pub.validar_deprecacion(_fixture("deprecacion.json"))["estado"], "POLITICA_VALIDA"
        )

    def test_version_invalida(self) -> None:
        datos = _fixture("deprecacion.json")
        datos["version_api"] = "1.0"
        self.assertEqual(pub.validar_deprecacion(datos)["estado"], "POLITICA_RECHAZADA")

    def test_breaking_change_sin_major(self) -> None:
        datos = _fixture("deprecacion.json")
        datos["politica"]["breaking_change"] = "minor"
        self.assertEqual(pub.validar_deprecacion(datos)["estado"], "POLITICA_RECHAZADA")

    def test_sin_rfc_ni_ci(self) -> None:
        datos = _fixture("deprecacion.json")
        datos["politica"]["requiere_rfc"] = False
        datos["politica"]["detectado_en_ci"] = False
        problemas = pub.validar_deprecacion(datos)["problemas"]
        self.assertTrue(any("RFC" in p for p in problemas))
        self.assertTrue(any("CI" in p for p in problemas))

    def test_alcance_separado(self) -> None:
        self.assertEqual(pub.validar_alcance(_fixture("alcance.json"))["estado"], "ALCANCE_SEPARADO")

    def test_mobile_como_gate_se_rechaza(self) -> None:
        datos = _fixture("alcance.json")
        datos["gate_mobile_requerido"] = True
        self.assertEqual(pub.validar_alcance(datos)["estado"], "ALCANCE_INCORRECTO")

    def test_bloqueo_por_mobile_se_rechaza(self) -> None:
        datos = _fixture("alcance.json")
        datos["bloquea_por_mobile"] = True
        self.assertEqual(pub.validar_alcance(datos)["estado"], "ALCANCE_INCORRECTO")

    def test_falta_exclusion(self) -> None:
        datos = _fixture("alcance.json")
        datos["fuera_de_alcance"] = ["android"]
        self.assertEqual(pub.validar_alcance(datos)["estado"], "ALCANCE_INCORRECTO")


class CliYSeguridad(unittest.TestCase):
    def test_cli_contrato_y_compatibilidad(self) -> None:
        self.assertEqual(pub.main(["contrato"]), 0)
        self.assertEqual(pub.main(["compatibilidad"]), 0)

    def test_cli_publicacion(self) -> None:
        self.assertEqual(
            pub.main(["publicacion", "--datos", str(FIXTURES / "publicacion.json")]), 0
        )

    def test_cli_evidencia_rechazada(self) -> None:
        datos = _fixture("evidencia.json")
        datos["pruebas"][0]["estado"] = "PASS"
        self.assertEqual(pub.main(["evidencia", "--datos", str(_json(datos))]), 1)

    def test_cli_comando_desconocido(self) -> None:
        self.assertEqual(pub.main(["publicar-en-aws"]), 2)

    def test_no_consulta_aws(self) -> None:
        codigo, resultado = pub.ejecutar(["contrato"])
        self.assertEqual(codigo, 0)
        self.assertFalse(resultado["aws_consultado"])

    def test_no_importa_aws_ni_procesos(self) -> None:
        fuente = (RAIZ / "herramientas" / "verificar_publicacion_i150.py").read_text(encoding="utf-8")
        for prohibido in (
            "import boto3", "import botocore", "from botocore",
            "import subprocess", "from subprocess", "os.system",
            "urllib.request", "requests.",
        ):
            self.assertNotIn(prohibido, fuente)


if __name__ == "__main__":  # pragma: no cover
    unittest.main()
