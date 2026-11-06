"""Pruebas estáticas de la unidad systemd de I-020.

`systemd-analyze` no está disponible en Windows; esta verificación es estática y
reproducible. La validación real de arranque corresponde a Linux desplegado
(NOT_RUN_AWS / NOT_RUN_LINUX).
"""

from __future__ import annotations

import unittest
from pathlib import Path

RAIZ = Path(__file__).resolve().parents[3]
UNIDAD = RAIZ / "infra" / "systemd" / "nexum-backend.service"


class UnidadEndurecida(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.texto = UNIDAD.read_text(encoding="utf-8")
        cls.lineas = [
            l.strip()
            for l in cls.texto.splitlines()
            if l.strip() and not l.strip().startswith("#")
        ]

    def _valor(self, clave: str) -> str | None:
        for linea in self.lineas:
            if linea.startswith(f"{clave}="):
                return linea.split("=", 1)[1]
        return None

    def test_existe_y_tiene_secciones(self) -> None:
        self.assertTrue(UNIDAD.is_file())
        for seccion in ("[Unit]", "[Service]", "[Install]"):
            self.assertIn(seccion, self.texto)

    def test_usuario_no_privilegiado(self) -> None:
        self.assertEqual(self._valor("User"), "nexum")
        self.assertEqual(self._valor("Group"), "nexum")
        self.assertNotEqual(self._valor("User"), "root")

    def test_execstart_absoluto(self) -> None:
        ejecutable = self._valor("ExecStart")
        self.assertIsNotNone(ejecutable)
        self.assertTrue(ejecutable.startswith("/"), ejecutable)
        self.assertIn("/opt/nexum/bin/nexum-backend", ejecutable)

    def test_reinicio_acotado(self) -> None:
        self.assertEqual(self._valor("Restart"), "on-failure")
        self.assertIsNotNone(self._valor("RestartSec"))
        self.assertIsNotNone(self._valor("StartLimitBurst"))

    def test_cierre_controlado(self) -> None:
        self.assertEqual(self._valor("KillSignal"), "SIGTERM")
        self.assertIsNotNone(self._valor("TimeoutStopSec"))
        self.assertLessEqual(int(self._valor("TimeoutStopSec")), 60)

    def test_limites_para_1_gib(self) -> None:
        memoria = self._valor("MemoryMax")
        self.assertIsNotNone(memoria)
        self.assertRegex(memoria, r"^[0-9]+[MG]$")
        valor_mb = int(memoria[:-1]) * (1024 if memoria.endswith("G") else 1)
        self.assertLessEqual(valor_mb, 768, "debe dejar memoria al sistema operativo")

    def test_endurecimiento(self) -> None:
        obligatorios = {
            "NoNewPrivileges": "true",
            "ProtectSystem": "strict",
            "ProtectHome": "true",
            "PrivateTmp": "true",
            "PrivateDevices": "true",
            "ProtectKernelTunables": "true",
            "ProtectKernelModules": "true",
            "ProtectControlGroups": "true",
            "RestrictSUIDSGID": "true",
            "LockPersonality": "true",
            "RestrictNamespaces": "true",
        }
        for clave, esperado in obligatorios.items():
            self.assertEqual(self._valor(clave), esperado, clave)

    def test_escritura_limitada(self) -> None:
        rutas = self._valor("ReadWritePaths")
        self.assertIsNotNone(rutas)
        for ruta in rutas.split():
            self.assertTrue(ruta.startswith("/"), ruta)
        self.assertNotIn("/", rutas.split(), "no se permite escribir en la raiz")

    def test_sin_secretos_en_la_unidad(self) -> None:
        for patron in ("AKIA", "BEGIN PRIVATE", "password", "secret=", "token="):
            self.assertNotIn(patron, self.texto.lower(), patron)

    def test_environment_file_externo(self) -> None:
        self.assertEqual(self._valor("EnvironmentFile"), "-/etc/nexum/backend.env")

    def test_logs_a_journal(self) -> None:
        self.assertEqual(self._valor("StandardOutput"), "journal")
        self.assertEqual(self._valor("StandardError"), "journal")


if __name__ == "__main__":
    unittest.main()
