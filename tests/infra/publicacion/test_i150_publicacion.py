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

import comparar_contrato_api as cmp  # noqa: E402
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


class ParidadDeContratos(unittest.TestCase):
    """JSON, YAML y matriz deben describir las mismas 75 operaciones."""

    def test_paridad_ok(self) -> None:
        resultado = pub.validar_paridad()
        self.assertEqual(resultado["estado"], "PARIDAD_OK")
        self.assertEqual(resultado["operaciones_json"], 75)
        self.assertEqual(resultado["operaciones_yaml"], 75)
        self.assertEqual(resultado["rutas_json"], 55)
        self.assertEqual(resultado["rutas_yaml"], 55)
        self.assertEqual(resultado["filas_matriz"], 75)

    def test_estructura_yaml_expone_rutas(self) -> None:
        estructura = pub.estructura_yaml(pub.YAML.read_text(encoding="utf-8"))
        self.assertEqual(len(estructura["paths"]), 55)
        self.assertIn("/api/v1/health/live", estructura["paths"])

    def test_estructura_yaml_cuenta_operaciones(self) -> None:
        estructura = pub.estructura_yaml(pub.YAML.read_text(encoding="utf-8"))
        operaciones = sum(
            1 for item in estructura["paths"].values() for m in item if m in pub.METODOS_OPENAPI
        )
        self.assertEqual(operaciones, 75)


class ContenidoYModeracion(unittest.TestCase):
    """INF-150-07: sin medios ni texto libre en el catalogo actual."""

    def test_escaneo_actual_sin_media(self) -> None:
        resultado = pub.escanear_contenido()
        self.assertEqual(resultado["estado"], "CONTENIDO_CUBIERTO")
        self.assertEqual(resultado["operaciones_con_media"], [])

    def test_texto_libre_de_usuario_detectado(self) -> None:
        # E028/E030 (descripcion de movimiento) y E075 (texto de voz).
        libre = " ".join(pub.escanear_contenido()["operaciones_con_texto_libre"])
        for identificador in ("E028", "E030", "E075"):
            self.assertIn(identificador, libre)

    def test_no_declara_moderacion_operativa(self) -> None:
        resultado = pub.escanear_contenido()
        self.assertFalse(resultado["moderacion_operativa"])
        self.assertIn("no acredita", resultado["advertencia"])

    def test_diseno_de_moderacion_documentado(self) -> None:
        self.assertTrue((RAIZ / "sdd" / "_control" / "H08" / "MODERACION_CDN_I150.md").is_file())

    def test_frontera_documentada(self) -> None:
        texto = (RAIZ / "sdd" / "_control" / "H08" / "FRONTERA_RESPONSABILIDAD.md").read_text(encoding="utf-8")
        for elemento in ("android", "Play Store", "landing", "Ninguno"):
            self.assertIn(elemento, texto)


class GeneradorDeComandos(unittest.TestCase):
    def test_genera_un_curl_por_prueba_sin_ejecutar(self) -> None:
        datos = _fixture("evidencia.json")
        comandos = pub.generar_comandos(datos)
        self.assertEqual(len(comandos), len(datos["pruebas"]))
        self.assertTrue(all("curl" in c for c in comandos))
        self.assertTrue(any("POST" in c for c in comandos))

    def test_comandos_usan_marcador_y_no_url_real(self) -> None:
        comandos = pub.generar_comandos(_fixture("evidencia.json"))
        relativos = [c for c in comandos if "https://" not in c.split("curl", 1)[1].split("-X", 1)[1]]
        self.assertTrue(relativos)
        self.assertTrue(all("URL_PUBLICA_HTTPS" in c for c in relativos))

    def test_cli_evidencia_con_comandos(self) -> None:
        codigo, resultado = pub.ejecutar(
            ["evidencia", "--datos", str(FIXTURES / "evidencia.json"), "--comandos"]
        )
        self.assertEqual(codigo, 0)
        self.assertEqual(len(resultado["comandos"]), 13)


class ComparadorDeContratos(unittest.TestCase):
    """INF-150-05: cambios incompatibles detectados y excepciones por RFC."""

    @classmethod
    def setUpClass(cls) -> None:
        cls.base = json.loads(OPENAPI.read_text(encoding="utf-8"))
        cls.actual = json.loads(OPENAPI.read_text(encoding="utf-8"))

    def _roturas(self, mutar) -> list[dict]:
        actual = copy.deepcopy(self.actual)
        mutar(actual)
        return cmp.comparar(self.base, actual)

    def test_contratos_iguales_son_compatibles(self) -> None:
        self.assertEqual(cmp.comparar(self.base, self.actual), [])

    def test_ruta_eliminada(self) -> None:
        roturas = self._roturas(lambda d: d["paths"].pop("/api/v1/auth/login"))
        self.assertTrue(any(r["codigo"] == "RUTA_ELIMINADA" for r in roturas))

    def test_metodo_eliminado(self) -> None:
        roturas = self._roturas(lambda d: d["paths"]["/api/v1/auth/login"].pop("post"))
        # Si la ruta queda sin metodos se reporta como ruta eliminada; ambos son ruptura.
        self.assertTrue(any(r["codigo"] in ("METODO_ELIMINADO", "RUTA_ELIMINADA") for r in roturas))

    def test_operation_id_cambiado(self) -> None:
        roturas = self._roturas(
            lambda d: d["paths"]["/api/v1/auth/login"]["post"].update({"operationId": "otro"})
        )
        self.assertTrue(any(r["codigo"] == "OPERATION_ID_CAMBIADO" for r in roturas))

    def test_parametro_requerido_nuevo(self) -> None:
        def mutar(contrato):
            contrato["paths"]["/api/v1/auth/login"]["post"].setdefault("parameters", []).append(
                {"name": "obligatorio", "in": "query", "required": True}
            )
        self.assertTrue(any(r["codigo"] == "PARAMETRO_REQUERIDO_NUEVO" for r in self._roturas(mutar)))

    def test_respuesta_eliminada(self) -> None:
        roturas = self._roturas(
            lambda d: d["paths"]["/api/v1/auth/login"]["post"]["responses"].pop("200")
        )
        self.assertTrue(any(r["codigo"] == "RESPUESTA_ELIMINADA" for r in roturas))

    def test_tipo_cambiado(self) -> None:
        base = {
            "openapi": "3.1.0",
            "paths": {"/api/v1/x": {"post": {
                "operationId": "nexum_e001",
                "requestBody": {"content": {"application/json": {"schema": {"$ref": "#/components/schemas/A"}}}},
                "responses": {"200": {"description": "ok"}},
            }}},
            "components": {"schemas": {"A": {"type": "object", "properties": {"campo": {"type": "string"}}}}},
        }
        actual = copy.deepcopy(base)
        actual["components"]["schemas"]["A"]["properties"]["campo"]["type"] = "integer"
        roturas = cmp.comparar(base, actual)
        self.assertTrue(any(r["codigo"] == "TIPO_CAMBIADO" for r in roturas))
        self.assertTrue(any(r["codigo"] == "TIPO_CAMBIADO" for r in roturas))

    def test_enum_reducido(self) -> None:
        base = {
            "openapi": "3.1.0",
            "paths": {"/api/v1/x": {"post": {
                "operationId": "nexum_e001",
                "requestBody": {"content": {"application/json": {"schema": {"$ref": "#/components/schemas/A"}}}},
                "responses": {"200": {"description": "ok"}},
            }}},
            "components": {"schemas": {"A": {"type": "object", "properties": {
                "estado": {"type": "string", "enum": ["a", "b"]}
            }}}},
        }
        actual = copy.deepcopy(base)
        actual["components"]["schemas"]["A"]["properties"]["estado"]["enum"] = ["a"]
        roturas = cmp.comparar(base, actual)
        self.assertTrue(any(r["codigo"] == "ENUM_REDUCIDO" for r in roturas))
        self.assertTrue(any(r["codigo"] == "ENUM_REDUCIDO" for r in roturas))

    def test_seguridad_endurecida(self) -> None:
        roturas = self._roturas(
            lambda d: d["paths"]["/api/v1/auth/login"]["post"].update({"security": [{"bearerAuth": []}]})
        )
        self.assertTrue(any(r["codigo"] == "SEGURIDAD_ENDURECIDA" for r in roturas))

    def test_excepcion_aprobada_cubre_la_rotura(self) -> None:
        roturas = [{"codigo": "RUTA_ELIMINADA", "ruta": "/api/v1/x", "metodo": "*"}]
        excepciones = [{"codigo": "RUTA_ELIMINADA", "ruta": "/api/v1/x", "metodo": "*", "estado": "APPROVED", "rfc": "RFC-008"}]
        restantes, cubiertas = cmp.aplicar_excepciones(roturas, excepciones)
        self.assertEqual(restantes, [])
        self.assertEqual(len(cubiertas), 1)

    def test_excepcion_sin_rfc_no_aplica(self) -> None:
        datos = {"excepciones": [{"codigo": "RUTA_ELIMINADA", "ruta": "/x", "metodo": "*", "estado": "APPROVED"}]}
        self.assertEqual(cmp.excepciones_aprobadas(datos), [])

    def test_cli_compatible_y_no_comparable(self) -> None:
        self.assertEqual(
            cmp.main(["--base", str(OPENAPI), "--actual", str(OPENAPI)]), 0
        )
        self.assertEqual(cmp.main(["--base", "no-existe.json", "--actual", str(OPENAPI)]), 2)

    def test_ci_ejecuta_el_comparador(self) -> None:
        workflow = (RAIZ / ".github" / "workflows" / "go-backend.yml").read_text(encoding="utf-8")
        self.assertIn("comparar_contrato_api.py", workflow)
        self.assertIn("origin/main", workflow)


class ParidadSemanticaYAML(unittest.TestCase):
    """Un cambio solo en el YAML, con el mismo numero de operaciones, se detecta."""

    @classmethod
    def setUpClass(cls) -> None:
        cls.yaml = pub.YAML.read_text(encoding="utf-8")

    def _paridad(self, texto_yaml: str) -> dict:
        ruta = Path(tempfile.mkdtemp()) / "mutado.yaml"
        ruta.write_text(texto_yaml, encoding="utf-8")
        return pub.validar_paridad(ruta_yaml=ruta)

    def test_base_sin_cambios_es_paridad_ok(self) -> None:
        self.assertEqual(self._paridad(self.yaml)["estado"], "PARIDAD_OK")

    def test_post_cambiado_a_put(self) -> None:
        texto = self.yaml.replace("  /api/v1/auth/login:\n    post:", "  /api/v1/auth/login:\n    put:", 1)
        resultado = self._paridad(texto)
        self.assertEqual(resultado["estado"], "PARIDAD_ROTA")
        self.assertTrue(any("PUT" in p or "POST" in p for p in resultado["problemas"]))

    def test_operation_id_cambiado(self) -> None:
        texto = self.yaml.replace("operationId: nexum_e002", "operationId: nexum_e999", 1)
        self.assertEqual(self._paridad(texto)["estado"], "PARIDAD_ROTA")

    def test_esquema_de_respuesta_cambiado(self) -> None:
        texto = self.yaml.replace("$ref: '#/components/schemas/AuthTokens'", "$ref: '#/components/schemas/UserProfile'", 1)
        self.assertEqual(self._paridad(texto)["estado"], "PARIDAD_ROTA")

    def test_autorizacion_modificada(self) -> None:
        texto = self.yaml.replace("      security: []\n      parameters: []", "      security:\n      - BearerAuth: []\n      parameters: []", 1)
        self.assertEqual(self._paridad(texto)["estado"], "PARIDAD_ROTA")

    def test_parametro_obligatorio_adicional(self) -> None:
        texto = self.yaml.replace(
            "      parameters:\n      - name: limit",
            "      parameters:\n      - name: extra\n        in: query\n        required: true\n      - name: limit",
            1,
        )
        self.assertEqual(self._paridad(texto)["estado"], "PARIDAD_ROTA")

    def test_respuesta_eliminada(self) -> None:
        texto = self.yaml.replace("        '409':", "        '499':", 1)
        self.assertEqual(self._paridad(texto)["estado"], "PARIDAD_ROTA")

    def test_ref_irresoluble(self) -> None:
        texto = self.yaml.replace("$ref: '#/components/schemas/AuthTokens'", "$ref: '#/components/schemas/NoExiste'", 1)
        resultado = self._paridad(texto)
        self.assertEqual(resultado["estado"], "PARIDAD_ROTA")
        self.assertTrue(any("irresoluble" in p for p in resultado["problemas"]))

    def test_tabulaciones_rechazadas(self) -> None:
        texto = self.yaml.replace("  /api/v1/auth/login:", "\t/api/v1/auth/login:", 1)
        with self.assertRaises(pub.ErrorUso):
            self._paridad(texto)

    def test_ruta_duplicada_rechazada(self) -> None:
        texto = self.yaml.replace(
            "  /api/v1/auth/refresh:",
            "  /api/v1/auth/login:\n    post:\n      operationId: nexum_e002\n  /api/v1/auth/refresh:",
            1,
        )
        with self.assertRaises(pub.ErrorUso):
            self._paridad(texto)


class RespuestasYSeguridad(unittest.TestCase):
    """El comparador cubre respuestas y seguridad efectiva."""

    @staticmethod
    def _contrato(seguridad_op=None, seguridad_global=None, respuesta=None) -> dict:
        operacion = {
            "operationId": "nexum_e001",
            "responses": {"200": respuesta or {"description": "ok", "content": {"application/json": {"schema": {"$ref": "#/components/schemas/A"}}}}},
        }
        if seguridad_op is not None:
            operacion["security"] = seguridad_op
        contrato = {
            "openapi": "3.1.0",
            "paths": {"/api/v1/x": {"post": operacion}},
            "components": {"schemas": {"A": {"type": "object", "properties": {
                "campo": {"type": "string"},
                "estado": {"type": "string", "enum": ["a", "b"]},
            }, "required": ["campo"]}}},
        }
        if seguridad_global is not None:
            contrato["security"] = seguridad_global
        return contrato

    def test_propiedad_de_respuesta_eliminada(self) -> None:
        actual = self._contrato()
        actual["components"]["schemas"]["A"]["properties"].pop("estado")
        roturas = cmp.comparar(self._contrato(), actual)
        self.assertTrue(any(r["codigo"] == "PROPIEDAD_RESPUESTA_ELIMINADA" for r in roturas))

    def test_tipo_de_respuesta_cambiado(self) -> None:
        actual = self._contrato()
        actual["components"]["schemas"]["A"]["properties"]["campo"]["type"] = "integer"
        roturas = cmp.comparar(self._contrato(), actual)
        self.assertTrue(any(r["codigo"] == "TIPO_RESPUESTA_CAMBIADO" for r in roturas))

    def test_enum_de_respuesta_reducido(self) -> None:
        actual = self._contrato()
        actual["components"]["schemas"]["A"]["properties"]["estado"]["enum"] = ["a"]
        roturas = cmp.comparar(self._contrato(), actual)
        self.assertTrue(any(r["codigo"] == "ENUM_RESPUESTA_REDUCIDO" for r in roturas))

    def test_requerido_nuevo_en_respuesta(self) -> None:
        actual = self._contrato()
        actual["components"]["schemas"]["A"]["required"] = ["campo", "estado"]
        roturas = cmp.comparar(self._contrato(), actual)
        self.assertTrue(any(r["codigo"] == "REQUERIDO_RESPUESTA_NUEVO" for r in roturas))

    def test_esquema_de_respuesta_cambiado(self) -> None:
        actual = self._contrato(respuesta={"description": "ok", "content": {"application/json": {"schema": {"$ref": "#/components/schemas/B"}}}})
        actual["components"]["schemas"]["B"] = {"type": "object", "properties": {}}
        roturas = cmp.comparar(self._contrato(), actual)
        self.assertTrue(any(r["codigo"] == "ESQUEMA_RESPUESTA_CAMBIADO" for r in roturas))

    def test_seguridad_global_cambiada(self) -> None:
        base = self._contrato(seguridad_global=[])
        actual = self._contrato(seguridad_global=[{"BearerAuth": []}])
        roturas = cmp.comparar(base, actual)
        self.assertTrue(any(r["codigo"] == "SEGURIDAD_ENDURECIDA" for r in roturas))

    def test_seguridad_de_operacion_cambiada(self) -> None:
        base = self._contrato(seguridad_op=[{"BearerAuth": []}])
        actual = self._contrato(seguridad_op=[])
        roturas = cmp.comparar(base, actual)
        self.assertTrue(any(r["codigo"] == "SEGURIDAD_RELAJADA" for r in roturas))

    def test_scopes_cambiados(self) -> None:
        base = self._contrato(seguridad_op=[{"BearerAuth": ["read"]}])
        actual = self._contrato(seguridad_op=[{"BearerAuth": ["read", "write"]}])
        roturas = cmp.comparar(base, actual)
        self.assertTrue(any(r["codigo"] == "SCOPES_CAMBIADOS" for r in roturas))

    def test_herencia_global_no_marca_ruptura_si_es_igual(self) -> None:
        base = self._contrato(seguridad_global=[{"BearerAuth": []}])
        actual = self._contrato(seguridad_global=[{"BearerAuth": []}])
        self.assertEqual(cmp.comparar(base, actual), [])

    def test_ref_irresoluble_falla_cerrado(self) -> None:
        actual = self._contrato()
        actual["components"]["schemas"] = {}
        with self.assertRaises(cmp.ErrorComparacion):
            cmp.comparar(self._contrato(), actual)

    def test_rfc_inexistente_no_aprueba_excepcion(self) -> None:
        datos = {"excepciones": [{"codigo": "RUTA_ELIMINADA", "ruta": "/x", "metodo": "*", "estado": "APPROVED", "rfc": "RFC-999"}]}
        self.assertEqual(cmp.excepciones_aprobadas(datos), [])

    def test_rfc_de_infraestructura_no_autoriza_api(self) -> None:
        for rfc in ("RFC-004", "RFC-005", "RFC-006", "RFC-007"):
            with self.subTest(rfc=rfc):
                datos = {"excepciones": [{"codigo": "RUTA_ELIMINADA", "ruta": "/x", "metodo": "*", "estado": "APPROVED", "rfc": rfc}]}
                self.assertEqual(cmp.excepciones_aprobadas(datos), [])

    def test_rfc_006_no_autoriza_cambio_de_seguridad(self) -> None:
        datos = {"excepciones": [{"codigo": "SCOPES_CAMBIADOS", "ruta": "/api/v1/movements", "metodo": "get", "estado": "APPROVED", "rfc": "RFC-006"}]}
        self.assertEqual(cmp.excepciones_aprobadas(datos), [])

    def test_aprobado_no_basta_sin_autorizacion_humana(self) -> None:
        datos = {"excepciones": [{"codigo": "RUTA_ELIMINADA", "ruta": "/x", "metodo": "*", "estado": "APPROVED", "rfc": "RFC-API-001"}]}
        self.assertEqual(cmp.excepciones_aprobadas(datos), [])

    def test_registro_inexistente_no_autoriza_nada(self) -> None:
        self.assertEqual(cmp._registro(Path(tempfile.mkdtemp()) / "no.json"), [])

    def test_excepcion_de_otra_ruta_no_aplica(self) -> None:
        registro = [{
            "alcance": "API_COMPATIBILIDAD", "codigo": "RUTA_ELIMINADA", "ruta": "/otra",
            "metodo": "*", "rfc": "RFC-API-001", "aprobado_por": "responsable",
            "gate_documento": "sdd/_control/H08/GATE_H08I_I150.md", "vigencia_hasta": "2027-01-01",
        }]
        datos = {"excepciones": [{"codigo": "RUTA_ELIMINADA", "ruta": "/x", "metodo": "*", "estado": "APPROVED", "rfc": "RFC-API-001"}]}
        self.assertEqual(cmp.excepciones_aprobadas(datos, registro=registro), [])

    def test_alcance_distinto_de_api_no_aplica(self) -> None:
        registro = [{
            "alcance": "INFRAESTRUCTURA", "codigo": "RUTA_ELIMINADA", "ruta": "/x",
            "metodo": "*", "rfc": "RFC-API-001", "aprobado_por": "responsable",
            "gate_documento": "sdd/_control/H08/GATE_H08I_I150.md", "vigencia_hasta": "2027-01-01",
        }]
        datos = {"excepciones": [{"codigo": "RUTA_ELIMINADA", "ruta": "/x", "metodo": "*", "estado": "APPROVED", "rfc": "RFC-API-001"}]}
        self.assertEqual(cmp.excepciones_aprobadas(datos, registro=registro), [])

    def test_autorizacion_vencida_no_aplica(self) -> None:
        registro = [{
            "alcance": "API_COMPATIBILIDAD", "codigo": "RUTA_ELIMINADA", "ruta": "/x",
            "metodo": "*", "rfc": "RFC-API-001", "aprobado_por": "responsable",
            "gate_documento": "sdd/_control/H08/GATE_H08I_I150.md", "vigencia_hasta": "2026-01-01",
        }]
        datos = {"excepciones": [{"codigo": "RUTA_ELIMINADA", "ruta": "/x", "metodo": "*", "estado": "APPROVED", "rfc": "RFC-API-001"}]}
        self.assertEqual(cmp.excepciones_aprobadas(datos, hoy="2026-11-09", registro=registro), [])

    def test_breaking_sin_excepcion_hace_fallar_el_cli(self) -> None:
        actual = json.loads(OPENAPI.read_text(encoding="utf-8"))
        actual["paths"].pop("/api/v1/auth/login")
        ruta_actual = _json(actual)
        self.assertEqual(cmp.main(["--base", str(OPENAPI), "--actual", str(ruta_actual)]), 1)

    def test_contrato_base_ausente_falla_cerrado(self) -> None:
        with self.assertRaises(cmp.ErrorComparacion):
            cmp.cargar("no-existe.json")


class ModeracionTextoLibre(unittest.TestCase):
    """INF-150-07: las tres operaciones documentadas y una nueva no pasa desapercibida."""

    def test_operaciones_documentadas(self) -> None:
        documento = (RAIZ / "sdd" / "_control" / "H08" / "MODERACION_CDN_I150.md").read_text(encoding="utf-8")
        for identificador in ("E028", "E030", "E075"):
            self.assertIn(identificador, documento)
        self.assertIn("BLOCKED_DEPENDENCY", documento)
        self.assertIn("moderacion_operativa", documento)

    def test_no_se_declara_no_aplica(self) -> None:
        documento = (RAIZ / "sdd" / "_control" / "H08" / "MATRIZ_INF_150.md").read_text(encoding="utf-8")
        self.assertNotIn("NO_APLICA_HOY", documento)

    def test_operacion_nueva_con_texto_sin_diseno_se_rechaza(self) -> None:
        contrato = {
            "openapi": "3.1.0",
            "paths": {"/api/v1/nuevo": {"post": {
                "operationId": "nexum_e900",
                "x-nexum-catalog-id": "E900",
                "requestBody": {"content": {"application/json": {"schema": {"$ref": "#/components/schemas/Nuevo"}}}},
                "responses": {"200": {"description": "ok"}},
            }}},
            "components": {"schemas": {"Nuevo": {"type": "object", "properties": {"texto": {"type": "string"}}}}},
        }
        resultado = pub.escanear_contenido(contrato)
        self.assertEqual(resultado["estado"], "CONVENIO_INVALIDO" if False else "CONTENIDO_SIN_DISENO")
        self.assertTrue(any("E900" in p for p in resultado["problemas"]))

    def test_moderacion_nunca_operativa(self) -> None:
        self.assertFalse(pub.escanear_contenido()["moderacion_operativa"])


class YAMLDuplicadosYSchemas(unittest.TestCase):
    """PyYAML con rechazo de duplicados y paridad de components.schemas."""

    @classmethod
    def setUpClass(cls) -> None:
        cls.yaml = pub.YAML.read_text(encoding="utf-8")

    def _paridad(self, texto_yaml: str) -> dict:
        ruta = Path(tempfile.mkdtemp()) / "mutado.yaml"
        ruta.write_text(texto_yaml, encoding="utf-8")
        return pub.validar_paridad(ruta_yaml=ruta)

    def test_duplicado_de_ruta_rechazado(self) -> None:
        texto = self.yaml.replace(
            "  /api/v1/auth/refresh:",
            "  /api/v1/auth/login:\n    post:\n      operationId: nexum_e002\n  /api/v1/auth/refresh:",
            1,
        )
        with self.assertRaises(pub.ErrorUso):
            self._paridad(texto)

    def test_duplicado_de_operacion_rechazado(self) -> None:
        texto = self.yaml.replace(
            "    post:\n      tags:\n      - identidad\n      operationId: nexum_e002",
            "    post:\n      tags:\n      - identidad\n      operationId: nexum_e002\n    post:\n      operationId: nexum_e002",
            1,
        )
        with self.assertRaises(pub.ErrorUso):
            self._paridad(texto)

    def test_duplicado_de_propiedad_en_schema_rechazado(self) -> None:
        texto = self.yaml.replace(
            "      properties:\n        email:",
            "      properties:\n        email:\n          type: string\n        email:",
            1,
        )
        with self.assertRaises(pub.ErrorUso):
            self._paridad(texto)

    def test_tipo_de_propiedad_cambiado_en_yaml(self) -> None:
        texto = self.yaml.replace(
            "    AuthTokens:\n      type: object",
            "    AuthTokens:\n      type: array",
            1,
        )
        resultado = self._paridad(texto)
        self.assertEqual(resultado["estado"], "PARIDAD_ROTA")
        self.assertTrue(any("schema" in p for p in resultado["problemas"]))

    def test_enum_cambiado_en_yaml(self) -> None:
        texto = self.yaml.replace("          enum:\n          - accepted\n", "          enum:\n          - accepted\n          - rejected\n", 1)
        resultado = self._paridad(texto)
        self.assertEqual(resultado["estado"], "PARIDAD_ROTA")
        self.assertTrue(any("schema" in p for p in resultado["problemas"]))

    def test_yaml_invalido_rechazado(self) -> None:
        with self.assertRaises(pub.ErrorUso):
            self._paridad("openapi: 3.1.0\n  mal: [sin cerrar")

    def test_pyyaml_disponible_y_fijado(self) -> None:
        import yaml

        self.assertEqual(yaml.__version__, "6.0.2")
        requisitos = (RAIZ / "herramientas" / "requirements.txt").read_text(encoding="utf-8")
        self.assertIn("pyyaml==6.0.2", requisitos)
        workflow = (RAIZ / ".github" / "workflows" / "go-backend.yml").read_text(encoding="utf-8")
        self.assertIn("herramientas/requirements.txt", workflow)


if __name__ == "__main__":  # pragma: no cover
    unittest.main()
