"""Pruebas de comportamiento del diagnostico FCM del bootstrap.

No usan AWS ni secretos reales: crean fixtures sinteticos en directorios temporales.
Cubren el parser de /proc/<MainPID>/environ y la traduccion del journal a estados fijos.
"""
from __future__ import annotations

import base64
import json
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

RAIZ = Path(__file__).resolve().parents[3]
PLANTILLA = RAIZ / "infra" / "terraform" / "modules" / "computo" / "user-data.sh.tftpl"


def _bloque(marcador: str) -> str:
    texto = PLANTILLA.read_text(encoding="utf-8")
    inicio = texto.index(marcador)
    inicio = texto.index("\n", inicio) + 1
    fin = texto.index("\nPY\n", inicio)
    return texto[inicio:fin]


def _ejecutar(script: str, *args: str, entrada: str = "") -> str:
    with tempfile.TemporaryDirectory() as directorio:
        ruta = Path(directorio) / "script.py"
        ruta.write_text(script, encoding="utf-8")
        proceso = subprocess.run(
            [sys.executable, str(ruta), *args],
            input=entrada,
            capture_output=True,
            text=True,
            timeout=30,
        )
        return proceso.stdout


def _clave_b64() -> str:
    return base64.b64encode(bytes(range(32))).decode("ascii")


class ParserProcEnviron(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.parser = _bloque('python3 - "/proc/$pid/environ" <<\'PY\'')

    def _entorno(self, pares: dict[str, str]) -> str:
        return "\0".join(f"{clave}={valor}" for clave, valor in pares.items()) + "\0"

    def _ejecutar(self, pares: dict[str, str]) -> str:
        with tempfile.NamedTemporaryFile("w", encoding="utf-8", delete=False) as manejador:
            manejador.write(self._entorno(pares))
            ruta = manejador.name
        try:
            return _ejecutar(self.parser, ruta)
        finally:
            Path(ruta).unlink(missing_ok=True)

    def test_variables_presentes_y_formatos(self) -> None:
        with tempfile.NamedTemporaryFile("w", encoding="utf-8", delete=False) as fcm:
            json.dump({"project_id": "proyecto-sintetico", "type": "service_account"}, fcm)
            ruta_fcm = fcm.name
        try:
            salida = self._ejecutar({
                "NEXUM_FCM_PROJECT_ID": "proyecto-sintetico",
                "NEXUM_FCM_CREDENTIALS_FILE": ruta_fcm,
                "NEXUM_PUSH_TOKEN_KEY_B64": _clave_b64(),
                "NEXUM_NOTIFICATION_CURSOR_KEY_B64": _clave_b64(),
            })
        finally:
            Path(ruta_fcm).unlink(missing_ok=True)
        self.assertIn("project_id=PRESENTE", salida)
        self.assertIn("credentials_file=PRESENTE", salida)
        self.assertIn("credentials_json=AUSENTE", salida)
        self.assertIn("push_key=PRESENTE", salida)
        self.assertIn("cursor_key=PRESENTE", salida)
        self.assertIn("project_match=YES", salida)
        self.assertIn("push_key_b64_32=YES", salida)
        self.assertIn("cursor_key_b64_32=YES", salida)

    def test_variables_ausentes(self) -> None:
        salida = self._ejecutar({})
        for nombre in ("project_id", "credentials_file", "credentials_json", "push_key", "cursor_key"):
            self.assertIn(f"{nombre}=AUSENTE", salida)
        self.assertIn("project_match=NO", salida)
        self.assertIn("push_key_b64_32=NO", salida)
        self.assertIn("cursor_key_b64_32=NO", salida)

    def test_credentials_json_presente_se_detecta(self) -> None:
        salida = self._ejecutar({
            "NEXUM_FCM_PROJECT_ID": "proyecto-sintetico",
            "NEXUM_FCM_CREDENTIALS_JSON": "{}",
        })
        self.assertIn("credentials_json=PRESENTE", salida)
        self.assertIn("project_match=NO", salida)

    def test_ruta_absoluta_y_bajo_etc_nexum(self) -> None:
        salida = self._ejecutar({
            "NEXUM_FCM_PROJECT_ID": "proyecto-sintetico",
            "NEXUM_FCM_CREDENTIALS_FILE": "/etc/nexum/fcm-service-account.json",
        })
        self.assertIn("file_abs=YES", salida)
        self.assertIn("file_under_etc_nexum=YES", salida)
        self.assertIn("project_match=NO", salida)

    def test_entorno_ilegible(self) -> None:
        salida = _ejecutar(self.parser, "/ruta/que/no/existe")
        self.assertIn("env_effective=UNREADABLE", salida)


class JournalAllowlist(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.parser = _bloque(
            'journalctl -u "$SERVICIO" --no-pager -n 200 2>/dev/null | python3 - <<\'PY\''
        )

    def _ejecutar(self, lineas: str) -> str:
        return _ejecutar(self.parser, entrada=lineas)

    def test_modo_sin_emisor(self) -> None:
        self.assertIn("MODE_SIN_EMISOR", self._ejecutar("nivel=warn FCM Oleada 4: worker iniciado modo=sin-emisor\n"))

    def test_modo_http_v1(self) -> None:
        self.assertIn("MODE_HTTP_V1", self._ejecutar("modo=http-v1-oauth2-jwt-rs256\n"))

    def test_configuracion_no_disponible(self) -> None:
        salida = self._ejecutar(
            "FCM Oleada 4: credencial no inyectada clase=FCM_CREDENCIAL_AUSENTE\n"
            "FCM Oleada 4: emisor no disponible clase=FCM_WORKER_NO_INICIADO\n"
        )
        self.assertIn("CONFIG_CREDENCIAL_AUSENTE", salida)
        self.assertIn("CONFIG_EMISOR_NO_DISPONIBLE", salida)

    def test_journal_vacio(self) -> None:
        self.assertIn("SIN_ESTADOS_CONOCIDOS", self._ejecutar(""))

    def test_no_filtra_lineas_arbitrarias(self) -> None:
        salida = self._ejecutar("token=no-debe-salir\npassword=no-debe-salir\n")
        self.assertIn("SIN_ESTADOS_CONOCIDOS", salida)
        self.assertNotIn("no-debe-salir", salida)


class CodigosFCM(unittest.TestCase):
    def test_allowlist_y_validate_only(self) -> None:
        texto = PLANTILLA.read_text(encoding="utf-8")
        self.assertIn('"validate_only": True', texto)
        for codigo in ("UNREGISTERED", "INVALID_ARGUMENT", "PERMISSION_DENIED", "SENDER_ID_MISMATCH"):
            self.assertIn(codigo, texto)
        self.assertNotIn("print(exc.read()", texto)
        self.assertNotIn("message=", texto)


if __name__ == "__main__":
    unittest.main()
