"""Pruebas del simulador de despliegue I-090 (sin AWS)."""

from __future__ import annotations

import json
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

SCRIPT = Path(__file__).resolve().parents[2] / "herramientas" / "simular_despliegue.py"


def _ejecutar(entrada: dict) -> dict:
    carpeta = Path(tempfile.mkdtemp(prefix="sim-i090-"))
    ruta = carpeta / "entrada.json"
    ruta.write_text(json.dumps(entrada), encoding="utf-8")
    r = subprocess.run([sys.executable, str(SCRIPT), str(ruta)], capture_output=True, text=True, timeout=60)
    if r.returncode != 0:
        raise AssertionError(r.stdout + r.stderr)
    return json.loads(r.stdout)


def _base(**cambios: object) -> dict:
    datos = {
        "in_service": 2,
        "healthy_antes": 2,
        "max_size": 3,
        "refresh": "Successful",
        "healthy_despues": 2,
        "http_live": 200,
        "http_ready": 200,
        "readiness_required": False,
    }
    datos.update(cambios)
    return datos


class SimuladorDespliegue(unittest.TestCase):
    def test_despliegue_sano(self) -> None:
        resultado = _ejecutar(_base())
        self.assertEqual(resultado["accion"], "DESPLEGAR")
        self.assertEqual(resultado["max_final"], 2)

    def test_menos_de_dos_antes_aborta_sin_mutar(self) -> None:
        resultado = _ejecutar(_base(healthy_antes=1))
        self.assertEqual(resultado["accion"], "ABORTAR")
        self.assertFalse(resultado["mutar"])

    def test_max_size_2_aborta_por_capacidad_transitoria(self) -> None:
        # El workflow sube max a 3; si no lo hizo, no debe mutar.
        resultado = _ejecutar(_base(max_size=2))
        self.assertEqual(resultado["accion"], "ABORTAR")
        self.assertIn("MAX_SIZE_INESPERADO", resultado["motivos"])

    def test_refresh_fallido_activa_rollback(self) -> None:
        resultado = _ejecutar(_base(refresh="Failed"))
        self.assertEqual(resultado["accion"], "ROLLBACK")
        self.assertIn("REFRESH_NO_EXITOSO", resultado["motivos"])

    def test_un_nodo_despues_activa_rollback(self) -> None:
        resultado = _ejecutar(_base(healthy_despues=1))
        self.assertEqual(resultado["accion"], "ROLLBACK")
        self.assertIn("NODOS_INSUFICIENTES_DESPUES", resultado["motivos"])

    def test_http_live_no_200_activa_rollback(self) -> None:
        resultado = _ejecutar(_base(http_live=503))
        self.assertEqual(resultado["accion"], "ROLLBACK")
        self.assertIn("HTTP_LIVE_NO_200", resultado["motivos"])

    def test_ready_obligatoria_no_200_activa_rollback(self) -> None:
        resultado = _ejecutar(_base(readiness_required=True, http_ready=503))
        self.assertEqual(resultado["accion"], "ROLLBACK")
        self.assertIn("HTTP_READY_NO_200", resultado["motivos"])

    def test_ready_no_obligatoria_no_bloquea(self) -> None:
        resultado = _ejecutar(_base(readiness_required=False, http_ready=503))
        self.assertEqual(resultado["accion"], "DESPLEGAR")

    def test_toda_salida_restaura_max_2(self) -> None:
        for entrada in (_base(), _base(refresh="Failed"), _base(healthy_antes=1)):
            self.assertEqual(_ejecutar(entrada)["max_final"], 2)


if __name__ == "__main__":
    unittest.main()
