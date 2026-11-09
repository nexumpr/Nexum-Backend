"""Pruebas adversas de integridad financiera tras una restauracion (I-130)."""

from __future__ import annotations

import copy
import json
import sys
import tempfile
import unittest
from pathlib import Path

RAIZ = Path(__file__).resolve().parents[3]
sys.path.insert(0, str(RAIZ / "herramientas"))

import verificar_integridad_financiera as fi  # noqa: E402

FIXTURE = RAIZ / "tests" / "fixtures" / "respaldo" / "dataset_sintetico.json"


def _datos() -> dict:
    return copy.deepcopy(json.loads(FIXTURE.read_text(encoding="utf-8")))


def _json(datos: object) -> Path:
    manejador, ruta = tempfile.mkstemp(suffix=".json")
    with open(manejador, "w", encoding="utf-8") as fh:
        json.dump(datos, fh, ensure_ascii=False)
    return Path(ruta)


def _codigos(resultado: dict) -> set[str]:
    return set(resultado["codigos"])


class DatasetValido(unittest.TestCase):
    def test_fixture_sintetico_es_integro(self) -> None:
        resultado = fi.validar_dataset(_datos())
        self.assertEqual(resultado["estado"], "INTEGRIDAD_OK")
        self.assertEqual(resultado["violaciones"], [])
        self.assertEqual(resultado["alcance"], "DATOS_SINTETICOS")

    def test_resumen_cuenta_entidades(self) -> None:
        resumen = fi.validar_dataset(_datos())["resumen"]
        self.assertEqual(resumen["usuarios"], 2)
        self.assertEqual(resumen["metas"], 1)
        self.assertEqual(resumen["movimientos"], 3)
        self.assertEqual(resumen["cuentas_eliminadas"], 1)

    def test_sin_datos_personales_en_fixture(self) -> None:
        texto = FIXTURE.read_text(encoding="utf-8")
        for marcador in ("@", "dni", "telefono", "iban", "tarjeta"):
            self.assertNotIn(marcador, texto.lower())


class DineroExacto(unittest.TestCase):
    def test_float_en_dinero_se_detecta(self) -> None:
        datos = _datos()
        datos["movimientos"][0]["importe_centavos"] = 300.5
        self.assertIn("MONEY_FLOAT", _codigos(fi.validar_dataset(datos)))

    def test_booleano_como_dinero_se_detecta(self) -> None:
        datos = _datos()
        datos["metas"][0]["saved_centavos"] = True
        self.assertIn("MONEY_BOOL", _codigos(fi.validar_dataset(datos)))

    def test_negativo_sin_reversion_se_detecta(self) -> None:
        datos = _datos()
        datos["movimientos"][0]["importe_centavos"] = -50
        self.assertIn("MONEY_NEGATIVO", _codigos(fi.validar_dataset(datos)))

    def test_texto_como_dinero_se_detecta(self) -> None:
        datos = _datos()
        datos["metas"][0]["saved_centavos"] = "1000"
        self.assertIn("MONEY_FLOAT", _codigos(fi.validar_dataset(datos)))


class DuplicadosYReferencias(unittest.TestCase):
    def test_movimiento_duplicado(self) -> None:
        datos = _datos()
        datos["movimientos"][1]["id"] = datos["movimientos"][0]["id"]
        self.assertIn("MOVIMIENTO_DUPLICADO", _codigos(fi.validar_dataset(datos)))

    def test_idempotencia_duplicada(self) -> None:
        datos = _datos()
        datos["movimientos"][1]["idempotency_key"] = "idem-sint-1"
        self.assertIn("IDEMPOTENCIA_DUPLICADA", _codigos(fi.validar_dataset(datos)))

    def test_referencia_a_meta_inexistente(self) -> None:
        datos = _datos()
        datos["movimientos"][0]["meta_id"] = "m-inexistente"
        self.assertIn("REFERENCIA_ROTA", _codigos(fi.validar_dataset(datos)))

    def test_aislamiento_por_usuario(self) -> None:
        datos = _datos()
        datos["movimientos"][0]["user_id"] = "u-sint-2"
        self.assertIn("AISLAMIENTO_USUARIO", _codigos(fi.validar_dataset(datos)))

    def test_asignacion_de_otro_usuario(self) -> None:
        datos = _datos()
        datos["metas"][0]["allocations"][0]["user_id"] = "u-sint-2"
        self.assertIn("AISLAMIENTO_USUARIO", _codigos(fi.validar_dataset(datos)))


class AportacionesYReversiones(unittest.TestCase):
    def test_suma_de_asignaciones_rota(self) -> None:
        datos = _datos()
        datos["metas"][0]["allocations"][0]["importe_centavos"] = 299
        self.assertIn("SUMA_ASIGNACIONES", _codigos(fi.validar_dataset(datos)))

    def test_centavos_no_distribuidos_sin_motivo(self) -> None:
        datos = _datos()
        datos["metas"][0]["allocations"][0]["importe_centavos"] = 299
        datos["metas"][0]["no_distribuido_centavos"] = 1
        self.assertIn("CENTAVOS_NO_DISTRIBUIDOS", _codigos(fi.validar_dataset(datos)))

    def test_centavos_no_distribuidos_con_motivo_se_acepta(self) -> None:
        datos = _datos()
        datos["metas"][0]["allocations"][0]["importe_centavos"] = 299
        datos["metas"][0]["no_distribuido_centavos"] = 1
        datos["metas"][0]["motivo_no_distribuido"] = "redondeo indivisible de 3 metas"
        self.assertNotIn("CENTAVOS_NO_DISTRIBUIDOS", _codigos(fi.validar_dataset(datos)))

    def test_reversion_que_no_netea(self) -> None:
        datos = _datos()
        datos["movimientos"][2]["importe_centavos"] = -250
        self.assertIn("REVERSION_INCONSISTENTE", _codigos(fi.validar_dataset(datos)))

    def test_reversion_sin_original(self) -> None:
        datos = _datos()
        datos["movimientos"][2]["original_id"] = "mov-inexistente"
        self.assertIn("REVERSION_INCONSISTENTE", _codigos(fi.validar_dataset(datos)))


class PrivacidadTrasRestore(unittest.TestCase):
    def test_cuenta_eliminada_reactivada(self) -> None:
        datos = _datos()
        datos["usuarios"][1]["estado"] = "activo"
        self.assertIn("CUENTA_ELIMINADA_REACTIVADA", _codigos(fi.validar_dataset(datos)))

    def test_sesion_revocada_reactivada(self) -> None:
        datos = _datos()
        datos["sesiones"][1]["estado"] = "activa"
        self.assertIn("SESION_REVOCADA_REACTIVADA", _codigos(fi.validar_dataset(datos)))

    def test_resumen_sin_reactivacion(self) -> None:
        resultado = fi.resumen_reactivacion(_datos())
        self.assertEqual(resultado["estado"], "SIN_REACTIVACION")

    def test_resumen_detecta_reactivacion(self) -> None:
        datos = _datos()
        datos["usuarios"][1]["estado"] = "activo"
        resultado = fi.resumen_reactivacion(datos)
        self.assertEqual(resultado["estado"], "REACTIVACION_DETECTADA")
        self.assertIn("u-sint-2", resultado["cuentas_reactivadas"])


class TTLYConfiguracion(unittest.TestCase):
    def test_movimiento_sin_ttl(self) -> None:
        datos = _datos()
        datos["movimientos"][0].pop("expiresAt")
        self.assertIn("TTL_AUSENTE", _codigos(fi.validar_dataset(datos)))

    def test_restore_no_recupera_configuracion(self) -> None:
        config = {"ttl_atributo": "expiresAt"}
        resultado = fi.verificar_config_post_restore(config)
        self.assertEqual(resultado["estado"], "RECONFIGURACION_PENDIENTE")
        self.assertIn("streams_habilitado", resultado["faltantes"])
        self.assertIn("pitr_habilitado", resultado["faltantes"])

    def test_configuracion_completa(self) -> None:
        config = {campo: True for campo in fi.CONFIG_REQUERIDA}
        self.assertEqual(
            fi.verificar_config_post_restore(config)["estado"], "CONFIGURACION_COMPLETA"
        )

    def test_config_ausente_es_error(self) -> None:
        with self.assertRaises(fi.ErrorUso):
            fi.verificar_config_post_restore(None)


class CliYSeguridad(unittest.TestCase):
    def test_cli_validar_ok(self) -> None:
        self.assertEqual(fi.main(["validar"]), 0)

    def test_cli_validar_detecta_violacion(self) -> None:
        datos = _datos()
        datos["movimientos"][0]["importe_centavos"] = 1.5
        self.assertEqual(fi.main(["validar", "--datos", str(_json(datos))]), 1)

    def test_cli_config_pendiente_devuelve_1(self) -> None:
        config = _json({"ttl_atributo": "expiresAt"})
        self.assertEqual(fi.main(["config-post-restore", "--config", str(config)]), 1)

    def test_cli_comando_desconocido(self) -> None:
        self.assertEqual(fi.main(["borrar-todo"]), 2)

    def test_no_importa_aws_ni_procesos(self) -> None:
        fuente = (RAIZ / "herramientas" / "verificar_integridad_financiera.py").read_text(encoding="utf-8")
        for prohibido in (
            "import boto3", "import botocore", "from botocore",
            "import subprocess", "from subprocess", "os.system",
        ):
            self.assertNotIn(prohibido, fuente)


if __name__ == "__main__":  # pragma: no cover
    unittest.main()
