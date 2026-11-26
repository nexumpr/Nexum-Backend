"""Pruebas del harness de certificación: cobertura, ejecutor real y regresiones.

El ejecutor se prueba contra un servidor HTTP local de prueba (``http.server``) con
respuestas mínimas del contrato. No hay red externa, Docker ni producción: el
transporte del cliente se redirige a 127.0.0.1 sólo dentro de la prueba.
"""

from __future__ import annotations

import contextlib
import http.server
import io
import json
import sys
import tempfile
import threading
import unittest
import urllib.error
import urllib.parse
import urllib.request
from pathlib import Path
from unittest import mock

RAIZ = Path(__file__).resolve().parents[3]
sys.path.insert(0, str(RAIZ / "herramientas" / "certificacion"))

import certificar_75 as cli  # noqa: E402
import escenarios  # noqa: E402
import nucleo  # noqa: E402

OPENAPI = RAIZ / "sdd" / "03-endpoints" / "openapi" / "nexum-api-v1.json"


# --------------------------------------------------------------------------- #
# Inventario y cobertura
# --------------------------------------------------------------------------- #

class Inventario(unittest.TestCase):
    def test_exactamente_75_operaciones(self) -> None:
        operaciones = cli.inventario(OPENAPI)
        self.assertEqual(75, len(operaciones))
        self.assertTrue(all(eid.startswith("E") for eid in operaciones))

    def test_metodo_y_ruta_presentes(self) -> None:
        operaciones = cli.inventario(OPENAPI)
        op = operaciones["E002"]
        self.assertEqual("POST", op.metodo)
        self.assertEqual("/api/v1/auth/login", op.ruta)


class Cobertura(unittest.TestCase):
    def _correr(self, *args: str) -> tuple[int, dict]:
        buffer = io.StringIO()
        with contextlib.redirect_stdout(buffer):
            rc = cli.main(list(args))
        return rc, json.loads(buffer.getvalue())

    def test_dry_run_exige_cobertura_exacta(self) -> None:
        rc, resumen = self._correr("--openapi", str(OPENAPI))
        self.assertEqual(0, rc)
        self.assertTrue(resumen["cobertura_exacta"])
        self.assertEqual("COBERTURA_EXACTA", resumen["estado"])
        self.assertEqual(75, resumen["operaciones_openapi"])
        self.assertEqual(75, resumen["escenarios"])
        self.assertEqual([], resumen["problemas"])

    def test_escenario_faltante_falla_cerrado(self) -> None:
        operaciones = cli.inventario(OPENAPI)

        class Falso:
            ESCENARIOS = {}
            ORDEN_EJECUCION = ()

        exacta, problemas = cli.cobertura(operaciones, Falso)
        self.assertFalse(exacta)
        self.assertTrue(any(p.startswith("FALTA") for p in problemas))

    def test_run_sin_autorizacion_no_ejecuta(self) -> None:
        rc, resumen = self._correr("--openapi", str(OPENAPI), "--run")
        self.assertEqual(2, rc)
        self.assertEqual("MUTACIONES_NO_AUTORIZADAS", resumen["estado"])
        self.assertFalse(resumen["ejecutado_contra_produccion"])

    def test_salida_no_expone_secretos(self) -> None:
        _, resumen = self._correr("--openapi", str(OPENAPI))
        texto = json.dumps(resumen)
        for prohibido in ("Bearer ", "password", "refreshToken", "accessToken", "token="):
            self.assertNotIn(prohibido, texto)

    def test_rc_distintos(self) -> None:
        self.assertEqual((0, 1, 2), (cli.RC_OK, cli.RC_FALLO, cli.RC_INCOMPLETA))

    def test_negativos_configuracion_rc2(self) -> None:
        casos = (
            ("--run", "--allow-mutations"),
            ("--run", "--allow-mutations", "--base-url", "http://127.0.0.1:1"),
            ("--run", "--allow-mutations", "--base-url", "https://127.0.0.1:1", "--solo", "E999"),
            ("--run", "--allow-mutations", "--base-url", "https://127.0.0.1:1", "--reanudar", "no-existe.json"),
            ("--openapi", str(RAIZ / "no-existe-openapi.json")),
            ("--run", "--allow-delete-account", "--base-url", "https://127.0.0.1:1"),
        )
        for caso in casos:
            with self.subTest(caso=caso):
                rc, _ = self._correr(*caso)
                self.assertEqual(2, rc)


class CodigoSalida(unittest.TestCase):
    def _resultados(self, estados: dict[str, str]) -> dict[str, nucleo.Resultado]:
        resultados: dict[str, nucleo.Resultado] = {}
        for eid, estado in estados.items():
            resultado = nucleo.Resultado(eid, "", "GET", "/api/v1/x")
            if estado == nucleo.PASS:
                resultado.ok("ok")
            elif estado in nucleo.ESTADOS_FALLO:
                resultado.fallo("fallo", "", estado)
            else:
                resultado.bloqueo("bloqueo", "", nucleo.BLOCKED_EXTERNAL)
            resultados[eid] = resultado
        return resultados

    def test_0_solo_con_75_pass(self) -> None:
        operaciones = cli.inventario(OPENAPI)
        resultados = self._resultados({eid: nucleo.PASS for eid in operaciones})
        self.assertEqual(0, cli.codigo_certificacion(resultados, operaciones, True))

    def test_1_con_fallo(self) -> None:
        operaciones = cli.inventario(OPENAPI)
        estados = {eid: nucleo.PASS for eid in operaciones}
        estados["E031"] = nucleo.FAIL
        resultados = self._resultados(estados)
        self.assertEqual(1, cli.codigo_certificacion(resultados, operaciones, True))

    def test_2_con_bloqueo_not_run_o_incompleto(self) -> None:
        operaciones = cli.inventario(OPENAPI)
        for estado in (nucleo.BLOCKED_EXTERNAL, nucleo.NOT_RUN):
            with self.subTest(estado=estado):
                estados = {eid: nucleo.PASS for eid in operaciones}
                estados["E031"] = estado
                resultados = self._resultados(estados)
                self.assertEqual(2, cli.codigo_certificacion(resultados, operaciones, True))
        resultados = self._resultados({eid: nucleo.PASS for eid in list(operaciones)[:-1]})
        self.assertEqual(2, cli.codigo_certificacion(resultados, operaciones, False))


# --------------------------------------------------------------------------- #
# Servidor HTTP local de prueba
# --------------------------------------------------------------------------- #

class _Manejador(http.server.BaseHTTPRequestHandler):
    peticiones: list[str] = []
    codigo_live = 200
    codigo_ready = 200

    def _responder(self, codigo: int, cuerpo: dict) -> None:
        datos = json.dumps(cuerpo).encode("utf-8")
        self.send_response(codigo)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(datos)))
        self.end_headers()
        self.wfile.write(datos)

    def _salud(self, codigo: int) -> None:
        if codigo == 200:
            self._responder(200, {
                "status": "ok",
                "service": "nexum-backend",
                "timestamp": "2026-11-24T00:00:00.000Z",
            })
        else:
            self._responder(codigo, {
                "error": {
                    "code": "INTERNAL_ERROR",
                    "message": "Solicitud no completada.",
                    "traceId": "018f90ef-08e5-7a68-b09f-2b67a47c92f0",
                    "details": [],
                }
            })

    def do_GET(self) -> None:  # noqa: N802 - API de http.server
        type(self).peticiones.append(self.path)
        if self.path.startswith("/api/v1/health/live"):
            self._salud(type(self).codigo_live)
        elif self.path.startswith("/api/v1/health/ready"):
            self._salud(type(self).codigo_ready)
        else:
            self._responder(404, {
                "error": {
                    "code": "NOT_FOUND",
                    "message": "Recurso no encontrado.",
                    "traceId": "018f90ef-08e5-7a68-b09f-2b67a47c92f0",
                    "details": [],
                }
            })

    def log_message(self, format: str, *args: object) -> None:  # noqa: A002
        return


@contextlib.contextmanager
def servidor_local(codigo_live: int = 200, codigo_ready: int = 200):
    _Manejador.peticiones = []
    _Manejador.codigo_live = codigo_live
    _Manejador.codigo_ready = codigo_ready
    servidor = http.server.ThreadingHTTPServer(("127.0.0.1", 0), _Manejador)
    servidor.daemon_threads = True
    hilo = threading.Thread(target=servidor.serve_forever, daemon=True)
    hilo.start()
    try:
        yield servidor.server_address[1], _Manejador.peticiones
    finally:
        servidor.shutdown()
        servidor.server_close()
        hilo.join(timeout=5)


def transporte_local(puerto: int):
    """Redirige el HTTPS del cliente a 127.0.0.1 para la prueba; no toca el núcleo."""

    def transporte(_cliente, metodo, url, datos, cabeceras, timeout):
        partes = urllib.parse.urlsplit(url)
        destino = urllib.parse.urlunsplit(
            ("http", f"127.0.0.1:{puerto}", partes.path, partes.query, "")
        )
        peticion = urllib.request.Request(destino, data=datos, method=metodo, headers=cabeceras)
        try:
            with urllib.request.urlopen(peticion, timeout=timeout) as respuesta:
                return respuesta.status, dict(respuesta.headers), respuesta.read()
        except urllib.error.HTTPError as exc:
            return exc.code, dict(exc.headers or {}), exc.read()

    return transporte


class EjecutorLocal(unittest.TestCase):
    def _correr(self, *args: str) -> tuple[int, dict]:
        buffer = io.StringIO()
        with contextlib.redirect_stdout(buffer):
            rc = cli.main(list(args))
        return rc, json.loads(buffer.getvalue())

    def test_run_hace_peticiones_y_no_es_ejecutor_pendiente(self) -> None:
        with servidor_local() as (puerto, peticiones), tempfile.TemporaryDirectory() as tmp:
            salida = Path(tmp) / "resultados.json"
            with mock.patch.object(nucleo.Cliente, "_transporte_urllib", transporte_local(puerto)):
                rc, resumen = self._correr(
                    "--run", "--allow-mutations",
                    "--base-url", f"https://127.0.0.1:{puerto}",
                    "--solo", "E072",
                    "--out", str(salida),
                )
            self.assertEqual(2, rc)  # E072 solo no completa las 75
            self.assertEqual("CERTIFICACION_INCOMPLETA", resumen["estado"])
            self.assertEqual("PASS", resumen["resultados"]["E072"]["estado"])
            self.assertTrue(resumen["llamadas"])
            self.assertEqual("E072", resumen["llamadas"][0]["eid"])
            self.assertTrue(peticiones)
            self.assertTrue(peticiones[0].startswith("/api/v1/health/live"))
            self.assertNotIn("EJECUTOR_PENDIENTE", json.dumps(resumen))
            texto = json.dumps(resumen)
            for prohibido in ("Bearer ", "password", "refreshToken", "accessToken", "@example.invalid"):
                self.assertNotIn(prohibido, texto)
            self.assertTrue(salida.exists())
            escrito = json.loads(salida.read_text(encoding="utf-8"))
            self.assertEqual("PASS", escrito["resultados"]["E072"]["estado"])

    def test_run_con_500_devuelve_rc1(self) -> None:
        with servidor_local(codigo_live=500) as (puerto, peticiones):
            with mock.patch.object(nucleo.Cliente, "_transporte_urllib", transporte_local(puerto)):
                rc, resumen = self._correr(
                    "--run", "--allow-mutations",
                    "--base-url", f"https://127.0.0.1:{puerto}",
                    "--solo", "E072",
                )
            self.assertEqual(1, rc)
            self.assertEqual("FAIL", resumen["resultados"]["E072"]["estado"])
            self.assertTrue(peticiones)

    def test_reanudacion_hereda_pass_sin_repetir_llamada(self) -> None:
        with servidor_local() as (puerto, peticiones), tempfile.TemporaryDirectory() as tmp:
            previo = Path(tmp) / "previo.json"
            with mock.patch.object(nucleo.Cliente, "_transporte_urllib", transporte_local(puerto)):
                rc1, resumen1 = self._correr(
                    "--run", "--allow-mutations",
                    "--base-url", f"https://127.0.0.1:{puerto}",
                    "--solo", "E072",
                    "--out", str(previo),
                )
                self.assertEqual(2, rc1)
                self.assertEqual("PASS", resumen1["resultados"]["E072"]["estado"])
                del peticiones[:]
                rc2, resumen2 = self._correr(
                    "--run", "--allow-mutations",
                    "--base-url", f"https://127.0.0.1:{puerto}",
                    "--solo", "E073",
                    "--reanudar", str(previo),
                )
            self.assertEqual(2, rc2)
            self.assertEqual("PASS", resumen2["resultados"]["E072"]["estado"])
            self.assertEqual("reanudado", resumen2["resultados"]["E072"]["origen"])
            self.assertEqual("PASS", resumen2["resultados"]["E073"]["estado"])
            self.assertFalse(any("health/live" in ruta for ruta in peticiones))
            self.assertTrue(any("health/ready" in ruta for ruta in peticiones))


# --------------------------------------------------------------------------- #
# Orden y regresiones de los defectos corregidos
# --------------------------------------------------------------------------- #

class Orden(unittest.TestCase):
    def test_dependencias_antes_que_dependientes(self) -> None:
        posicion = {eid: indice for indice, eid in enumerate(escenarios.ORDEN_EJECUCION)}
        for eid, escenario in escenarios.ESCENARIOS.items():
            for dependencia in escenario.depende_de:
                with self.subTest(eid=eid, dependencia=dependencia):
                    self.assertLess(posicion[dependencia], posicion[eid])


class _ContextoFalso:
    """Contexto mínimo para ejercitar una función de escenario sin red."""

    def __init__(
        self,
        respuestas: list[nucleo.Respuesta],
        *,
        datos: dict | None = None,
        autorizacion: nucleo.Autorizacion | None = None,
        opciones: nucleo.Opciones | None = None,
    ) -> None:
        self.a = nucleo.Sesion("A", "a@example.invalid", "x", user_id="u1")
        self.b = nucleo.Sesion("B", "b@example.invalid", "y")
        self.escenarios = escenarios.ESCENARIOS
        self.autorizacion = autorizacion or nucleo.Autorizacion(True, True)
        self.opciones = opciones or nucleo.Opciones(
            espera_ocurrencia_s=5, intervalo_poll_s=1, fcm_receipt_file=""
        )
        self.datos = dict(datos or {})
        self.advertencias: list[str] = []
        self.bloqueos: list[str] = []
        self.pedidas: list[str] = []
        self._respuestas = list(respuestas)
        self.resultados: dict[str, nucleo.Resultado] = {}

    def reg(self, eid: str) -> nucleo.Resultado:
        if eid not in self.resultados:
            escenario = self.escenarios[eid]
            self.resultados[eid] = nucleo.Resultado(eid, escenario.titulo, escenario.metodo, escenario.ruta)
        return self.resultados[eid]

    def pedir(self, eid: str, **kwargs: object) -> nucleo.Respuesta:
        self.pedidas.append(eid)
        if not self._respuestas:
            raise AssertionError(f"petición inesperada: {eid}")
        if len(self._respuestas) > 1:
            return self._respuestas.pop(0)
        return self._respuestas[0]

    def exigir_contrato(self, eid, respuesta, resultado, esperado) -> bool:  # noqa: ANN001
        if respuesta.codigo in tuple(esperado):
            return True
        resultado.fallo(f"HTTP en {esperado}", str(respuesta.codigo), nucleo.FAIL)
        return False

    def etiqueta_peticion(self, eid: str, respuesta: nucleo.Respuesta) -> str:
        return f"{eid}->{respuesta.codigo}"

    def login_auxiliar(self, sesion: nucleo.Sesion, resultado: nucleo.Resultado) -> bool:
        return sesion is self.a

    def category_id(self) -> str:
        return "otros"

    def notice_version(self) -> str:
        return "2026-11"


class RegresionesDefectos(unittest.TestCase):
    def test_sin_hacks_de_datetime_ni_reanudar(self) -> None:
        fuente = (RAIZ / "herramientas" / "certificacion" / "escenarios.py").read_text(encoding="utf-8")
        for prohibido in ("__import__", "__dict__", "ctx.opciones.reanudar", "del evidencia"):
            self.assertNotIn(prohibido, fuente)

    def test_e060_replay_acepta_204_404_y_409(self) -> None:
        for codigo in (204, 404, 409):
            with self.subTest(codigo=codigo):
                ctx = _ContextoFalso(
                    [
                        nucleo.Respuesta(codigo, None, {}, 1, ""),
                    ],
                    datos={"device_id": "d1"},
                )
                ctx.datos["device_id"] = "d1"
                escenarios.e060(ctx)
                resultado = ctx.reg("E060")
                comprobaciones = [c for c in resultado.checks if c["check"] == "segunda revocación idempotente"]
                self.assertTrue(comprobaciones, resultado.checks)
                self.assertTrue(comprobaciones[0]["ok"])

    def test_e020_perfil_de_cualquier_pagina_y_familias_unidas(self) -> None:
        ctx = _ContextoFalso([
            nucleo.Respuesta(200, {
                "profile": {"id": "u1", "email": "a@example.invalid"},
                "movements": [{"id": "m1"}],
                "goals": [],
                "consents": [],
                "nextCursor": "c2",
            }, {}, 1, ""),
            nucleo.Respuesta(200, {
                "movements": [],
                "goals": [{"id": "g1"}],
                "consents": [{"id": "c1"}],
            }, {}, 1, ""),
        ])
        escenarios.e020(ctx)
        resultado = ctx.reg("E020")
        fallos = {c["check"] for c in resultado.checks if not c["ok"]}
        self.assertNotIn("profile presente", fallos)
        self.assertNotIn("movements[]", fallos)
        self.assertNotIn("goals[]", fallos)
        self.assertNotIn("consents[]", fallos)

    def test_e026_no_siembra_sin_permiso_del_escenario_sembrador(self) -> None:
        ctx = _ContextoFalso([], autorizacion=nucleo.Autorizacion(False, False))
        escenarios.e026(ctx)
        resultado = ctx.reg("E026")
        self.assertTrue(resultado.bloqueado())
        self.assertEqual([], ctx.pedidas)
        self.assertTrue(any(c["check"] == "mutaciones autorizadas" and not c["ok"] for c in resultado.checks))

    def test_e061_correlaciona_por_evento_no_por_titulo(self) -> None:
        def pagina(creado: str) -> nucleo.Respuesta:
            return nucleo.Respuesta(200, {
                "items": [{
                    "id": "n1",
                    "title": "Recordatorio sintético de certificación",
                    "message": "m",
                    "isRead": False,
                    "createdAt": creado,
                }],
                "unreadCount": 1,
            }, {}, 1, "")

        datos = {
            "reminder_id": "r1",
            "notificaciones_previas": set(),
            "reminder_due": "2026-01-01T12:00:00.000Z",
        }
        ctx_ok = _ContextoFalso([pagina("2026-01-01T12:00:01.000Z")], datos=dict(datos))
        escenarios.e061(ctx_ok)
        ok = [c for c in ctx_ok.reg("E061").checks if c["check"] == "ocurrencia correlacionada con el evento"]
        self.assertTrue(ok and ok[0]["ok"])

        ctx_malo = _ContextoFalso([pagina("2026-01-01T11:00:00.000Z")], datos=dict(datos))
        escenarios.e061(ctx_malo)
        malo = [c for c in ctx_malo.reg("E061").checks if c["check"] == "ocurrencia correlacionada con el evento"]
        self.assertTrue(malo and not malo[0]["ok"])
        self.assertEqual(nucleo.CONTENT_MISMATCH, malo[0]["estado"])

    def test_e055_has_enough_data_true_con_items_vacio_es_estado_valido(self) -> None:
        ctx = _ContextoFalso(
            [nucleo.Respuesta(200, {"items": [], "hasEnoughData": True}, {}, 1, "")],
            datos={"analitica_periodo": "2026-01"},
        )
        escenarios.e055(ctx)
        resultado = ctx.reg("E055")
        estado = [c for c in resultado.checks if c["check"] == "estado sin patrones documentado"]
        self.assertTrue(estado and estado[0]["ok"])
        self.assertFalse(resultado.bloqueado())

    def test_avisos_de_acumulacion_en_reejecuciones(self) -> None:
        fuente = (RAIZ / "herramientas" / "certificacion" / "escenarios.py").read_text(encoding="utf-8")
        self.assertIn("E026 siembra", fuente)
        self.assertIn("cada reejecución acumula", fuente)
        self.assertIn("E052 siembra", fuente)


if __name__ == "__main__":
    unittest.main()

class ReintentosDelCliente(unittest.TestCase):
    """Reglas de repetición del cliente HTTP del harness.

    Un error de red puede ocurrir después de que el servidor procesó la petición:
    repetir una mutación duplicaría el efecto. El 429, en cambio, se emite antes de
    procesar, así que repetirlo es seguro en cualquier método.
    """

    def _cliente(self, metodo: str, guion: list):
        llamadas: list[str] = []

        def transporte(m, url, datos, hdrs, timeout):
            llamadas.append(m)
            paso = guion[min(len(llamadas) - 1, len(guion) - 1)]
            if isinstance(paso, Exception):
                raise paso
            return paso

        escenario = escenarios.Escenario(
            eid="E900", titulo="prueba", metodo=metodo, ruta="/x", fases=("publico",), funcion=lambda ctx: None
        )
        cliente = nucleo.Cliente(
            "https://example.invalid",
            nucleo.Autorizacion(True, True),
            {"E900": escenario},
            None,
            transporte=transporte,
            timeout=5,
        )
        return cliente, llamadas

    def test_mutacion_no_se_reintenta_tras_error_de_red(self) -> None:
        cliente, llamadas = self._cliente("POST", [ConnectionError("reset")])
        with mock.patch.object(nucleo.time, "sleep", lambda *_: None):
            respuesta = cliente.pedir("E900", cuerpo={}, validar_payload=False)
        self.assertEqual(respuesta.codigo, 0)
        self.assertEqual(len(llamadas), 1, "una mutación no puede repetirse a ciegas")

    def test_lectura_se_reintenta_tras_error_de_red(self) -> None:
        cliente, llamadas = self._cliente("GET", [ConnectionError("reset"), (200, {}, b"{}")])
        with mock.patch.object(nucleo.time, "sleep", lambda *_: None):
            respuesta = cliente.pedir("E900", validar_payload=False)
        self.assertEqual((respuesta.codigo, len(llamadas)), (200, 2))

    def test_retry_after_se_respeta(self) -> None:
        cliente, llamadas = self._cliente(
            "POST", [(429, {"Retry-After": "3"}, b"{}"), (202, {}, b"{}")]
        )
        esperas: list[float] = []
        with mock.patch.object(nucleo.time, "sleep", lambda s: esperas.append(s)):
            respuesta = cliente.pedir("E900", cuerpo={}, validar_payload=False)
        self.assertEqual(respuesta.codigo, 202)
        self.assertEqual(len(llamadas), 2)
        self.assertEqual(esperas, [3.0], "debe esperar exactamente lo pedido por el servidor")

    def test_retry_after_excesivo_no_se_recorta(self) -> None:
        cliente, llamadas = self._cliente("POST", [(429, {"Retry-After": "600"}, b"{}")])
        with mock.patch.object(nucleo.time, "sleep", lambda *_: None):
            respuesta = cliente.pedir("E900", cuerpo={}, validar_payload=False)
        self.assertEqual(respuesta.codigo, 429)
        self.assertEqual(len(llamadas), 1, "no se reintenta antes del tiempo pedido")
