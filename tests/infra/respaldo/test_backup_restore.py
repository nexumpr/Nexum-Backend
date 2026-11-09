"""Pruebas adversas de respaldo y restauracion de I-130 (todo local, sin AWS)."""

from __future__ import annotations

import json
import sys
import tempfile
import unittest
from decimal import Decimal
from pathlib import Path

RAIZ = Path(__file__).resolve().parents[3]
sys.path.insert(0, str(RAIZ / "herramientas"))

import verificar_backup_restore as br  # noqa: E402

INVENTARIO = RAIZ / "infra" / "terraform" / "respaldo" / "inventario_backup.json"
CATALOGO = RAIZ / "infra" / "finops" / "precios_us-east-2.json"


def _inv() -> dict:
    return br.cargar_inventario(INVENTARIO)


def _cat() -> dict:
    return br.cargar_catalogo(CATALOGO)


def _json(datos: object) -> Path:
    manejador, ruta = tempfile.mkstemp(suffix=".json")
    with open(manejador, "w", encoding="utf-8") as fh:
        json.dump(datos, fh, ensure_ascii=False)
    return Path(ruta)


def _solicitud(**extra) -> dict:
    base = {
        "tabla_origen": "NEXUM_CORE_h08i",
        "tabla_destino": "NEXUM_CORE_RESTORE_h08i",
        "punto_restauracion": "2026-11-09T00:00:00Z",
        "gb_restauracion": "2",
    }
    base.update(extra)
    return base


class ObjetivosRTOyRPO(unittest.TestCase):
    def test_objetivos_propuestos_no_medidos(self) -> None:
        resultado = br.validar_objetivos(_inv())
        self.assertEqual(resultado["rto_horas"], "4")
        self.assertEqual(resultado["rpo_horas"], "24")
        self.assertFalse(resultado["medido"])
        self.assertEqual(resultado["estado"], br.RTO_RPO_NO_MEDIDO)

    def test_objetivo_no_medido_no_es_cumplido(self) -> None:
        self.assertIn("no es un objetivo cumplido", br.validar_objetivos(_inv())["advertencia"])

    def test_objetivos_no_positivos_son_error(self) -> None:
        inventario = _inv()
        inventario["objetivos"]["rto_horas"] = "0"
        with self.assertRaises(br.ErrorInventario):
            br.validar_objetivos(inventario)

    def test_objetivo_medido_se_declara(self) -> None:
        inventario = _inv()
        inventario["objetivos"]["medido"] = True
        self.assertEqual(br.validar_objetivos(inventario)["estado"], "MEDIDO")


class InventarioDeActivos(unittest.TestCase):
    def test_inventario_valido(self) -> None:
        resultado = br.verificar_inventario(_inv())
        self.assertTrue(resultado["ok"])
        self.assertEqual(resultado["activos"], 4)

    def test_activo_sin_mecanismo_se_detecta(self) -> None:
        inventario = _inv()
        del inventario["activos"][0]["mecanismo"]
        resultado = br.verificar_inventario(inventario)
        self.assertFalse(resultado["ok"])
        self.assertEqual(resultado["estado"], "INVENTARIO_INCOMPLETO")

    def test_inventario_vacio_es_error(self) -> None:
        ruta = _json({"activos": []})
        with self.assertRaises(br.ErrorInventario):
            br.cargar_inventario(ruta)

    def test_state_exige_versionado_cifrado_y_proteccion(self) -> None:
        politica = _inv()["politica"]
        self.assertIn("Versionado y cifrado", politica["state"])
        activos = {a["nombre"]: a for a in _inv()["activos"]}
        self.assertEqual(activos["terraform-state"]["mecanismo"], "versionado + SSE-S3")
        self.assertTrue(activos["terraform-state"]["protegido"])

    def test_secretos_siguen_bloqueados_por_rfc004(self) -> None:
        activos = {a["nombre"]: a for a in _inv()["activos"]}
        self.assertEqual(activos["secretos-bootstrap"]["estado"], "BLOQUEADO_RFC004")


class Costes(unittest.TestCase):
    def test_coste_respaldo_exacto(self) -> None:
        resultado = br.coste_respaldo(_inv(), _cat())
        self.assertEqual(resultado["horas_mes"], "730")
        self.assertGreater(Decimal(resultado["total_hora_usd"]), Decimal("0"))
        self.assertEqual(resultado["costo_observado"], "NOT_VERIFIED")
        self.assertIn("secretos-bootstrap", resultado["no_computable"])
        self.assertIn("NOT_VERIFIED", resultado["advertencia"])

    def test_opcional_se_separa_de_lo_residente(self) -> None:
        resultado = br.coste_respaldo(_inv(), _cat())
        self.assertGreater(Decimal(resultado["opcional_hora_usd"]), Decimal("0"))
        self.assertEqual(
            Decimal(resultado["residente_hora_usd"]) + Decimal(resultado["opcional_hora_usd"]),
            Decimal(resultado["total_hora_usd"]),
        )

    def test_secretos_no_se_computan(self) -> None:
        resultado = br.coste_respaldo(_inv(), _cat())
        self.assertEqual(resultado["no_computable"], {"secretos-bootstrap": "BLOQUEADO_RFC004"})
        self.assertNotIn("secretos-bootstrap", resultado["por_activo_hora_usd"])

    def test_coste_restauracion_por_gb(self) -> None:
        # 0.15 USD/GB x 2 GB = 0.3000
        self.assertEqual(br.coste_restauracion("2", _cat()), "0.3000")

    def test_restauracion_sin_gb_es_error(self) -> None:
        for valor in ("0", "-1", None):
            with self.subTest(valor=valor):
                with self.assertRaises((br.ErrorUso, br.ErrorInventario)):
                    br.coste_restauracion(valor, _cat())

    def test_sku_sin_precio_no_se_infiere(self) -> None:
        catalogo = _cat()
        catalogo["items"]["DDB_RESTORE_GB"]["precio_usd"] = None
        with self.assertRaises(br.ErrorInventario):
            br.coste_restauracion("1", catalogo)


class PlanDeRestauracion(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.inv = _inv()
        cls.cat = _cat()

    def test_plan_valido_a_tabla_nueva(self) -> None:
        resultado = br.plan_restauracion(self.inv, self.cat, _solicitud())
        self.assertEqual(resultado["estado"], "PLAN_LOCAL_VALIDADO")
        self.assertEqual(resultado["alcance_aws"], br.RESTORE_AWS)
        self.assertFalse(resultado["autoriza_ejecucion_real"])
        self.assertEqual(resultado["coste_restauracion_usd"], "0.3000")

    def test_sin_solicitud_se_deniega(self) -> None:
        with self.assertRaises(br.ErrorUso):
            br.plan_restauracion(self.inv, self.cat, None)

    def test_restaurar_sobre_la_tabla_activa_se_deniega(self) -> None:
        with self.assertRaises(br.ErrorUso):
            br.plan_restauracion(
                self.inv, self.cat,
                _solicitud(tabla_destino="NEXUM_CORE_h08i"),
            )

    def test_destino_fuera_del_patron_se_deniega(self) -> None:
        with self.assertRaises(br.ErrorUso):
            br.plan_restauracion(self.inv, self.cat, _solicitud(tabla_destino="temp-restore"))

    def test_sobreescribir_se_deniega(self) -> None:
        with self.assertRaises(br.ErrorUso):
            br.plan_restauracion(self.inv, self.cat, _solicitud(sobreescribir=True))

    def test_destino_que_es_activo_del_inventario_se_deniega(self) -> None:
        with self.assertRaises(br.ErrorUso):
            br.plan_restauracion(
                self.inv, self.cat,
                _solicitud(tabla_destino="NEXUM_CORE_RESTORE_dynamodb-core"),
            )

    def test_solicitud_incompleta_se_deniega(self) -> None:
        with self.assertRaises(br.ErrorUso):
            br.plan_restauracion(self.inv, self.cat, {"tabla_origen": "NEXUM_CORE_h08i"})

    def test_plan_incluye_medicion_de_rto(self) -> None:
        pasos = " ".join(br.plan_restauracion(self.inv, self.cat, _solicitud())["pasos"])
        self.assertIn("duracion real", pasos)
        self.assertIn("RTO", pasos)


class InvariantesSinteticas(unittest.TestCase):
    def test_invariante_correcta(self) -> None:
        datos = {
            "saved_centavos": "1000",
            "opening_saved_centavos": "400",
            "allocations_centavos": ["300", "300"],
        }
        resultado = br.verificar_restore_sintetico(datos)
        self.assertEqual(resultado["estado"], "INVARIANTES_OK")

    def test_invariante_rota(self) -> None:
        datos = {
            "saved_centavos": "1001",
            "opening_saved_centavos": "400",
            "allocations_centavos": ["300", "300"],
        }
        self.assertEqual(br.verificar_restore_sintetico(datos)["estado"], "INVARIANTES_ROTAS")

    def test_asignacion_negativa_se_deniega(self) -> None:
        datos = {
            "saved_centavos": "100",
            "opening_saved_centavos": "200",
            "allocations_centavos": ["-100"],
        }
        with self.assertRaises(br.ErrorUso):
            br.verificar_restore_sintetico(datos)

    def test_sin_datos_se_deniega(self) -> None:
        with self.assertRaises(br.ErrorUso):
            br.verificar_restore_sintetico(None)

    def test_no_acredita_restore_real(self) -> None:
        resultado = br.verificar_restore_sintetico(
            {"saved_centavos": "0", "opening_saved_centavos": "0", "allocations_centavos": []}
        )
        self.assertEqual(resultado["alcance_aws"], br.RESTORE_AWS)


class BorradoDeCuenta(unittest.TestCase):
    def _politica(self, **extra) -> dict:
        base = {
            "datos_activos": "borrado inmediato",
            "backups": "retencion documentada hasta caducidad",
            "borrado_inmediato_backups": False,
            "afirma_borrado_inmutable": False,
        }
        base.update(extra)
        return base

    def test_borrado_diferido_documentado(self) -> None:
        resultado = br.evaluar_borrado_cuenta(self._politica())
        self.assertEqual(resultado["estado"], "BORRADO_DIFERIDO_DOCUMENTADO")
        self.assertFalse(resultado["afirma_borrado_inmediato_de_copias"])

    def test_no_se_puede_afirmar_borrado_inmediato_de_copias(self) -> None:
        with self.assertRaises(br.ErrorUso):
            br.evaluar_borrado_cuenta(self._politica(borrado_inmediato_backups=True))

    def test_no_se_puede_afirmar_borrado_inmutable(self) -> None:
        with self.assertRaises(br.ErrorUso):
            br.evaluar_borrado_cuenta(self._politica(afirma_borrado_inmutable=True))

    def test_politica_incompleta_se_deniega(self) -> None:
        with self.assertRaises(br.ErrorUso):
            br.evaluar_borrado_cuenta({"datos_activos": "borrado inmediato"})
        with self.assertRaises(br.ErrorUso):
            br.evaluar_borrado_cuenta(None)


class SeguridadYCLI(unittest.TestCase):
    def test_no_importa_aws_ni_procesos(self) -> None:
        fuente = (RAIZ / "herramientas" / "verificar_backup_restore.py").read_text(encoding="utf-8")
        for prohibido in (
            "import boto3", "import botocore", "from botocore",
            "import subprocess", "from subprocess", "os.system",
        ):
            with self.subTest(prohibido=prohibido):
                self.assertNotIn(prohibido, fuente)

    def test_comandos_destructivos_prohibidos(self) -> None:
        for comando in ("destroy-data", "destroy-state", "destroy-all", "restore-in-place"):
            with self.subTest(comando=comando):
                with self.assertRaises(br.ErrorUso):
                    br.ejecutar([comando])

    def test_comando_desconocido_se_deniega(self) -> None:
        with self.assertRaises(br.ErrorUso):
            br.ejecutar(["borrar-todo"])

    def test_estado_no_consulta_aws(self) -> None:
        codigo, resultado = br.ejecutar(["estado"])
        self.assertEqual(codigo, 0)
        self.assertFalse(resultado["aws_consultado"])
        self.assertFalse(resultado["terraform_ejecutado"])
        self.assertEqual(resultado["restauracion_aws"], br.RESTORE_AWS)

    def test_cli_plan_restauracion(self) -> None:
        codigo, resultado = br.ejecutar(
            ["plan-restauracion", "--solicitud", str(_json(_solicitud()))]
        )
        self.assertEqual(codigo, 0)
        self.assertEqual(resultado["estado"], "PLAN_LOCAL_VALIDADO")

    def test_main_devuelve_2_en_prohibido(self) -> None:
        self.assertEqual(br.main(["destroy-data"]), 2)

    def test_main_devuelve_1_con_invariantes_rotas(self) -> None:
        datos = _json({
            "saved_centavos": "1",
            "opening_saved_centavos": "2",
            "allocations_centavos": ["0"],
        })
        self.assertEqual(br.main(["verificar-restore", "--datos", str(datos)]), 1)


if __name__ == "__main__":  # pragma: no cover
    unittest.main()
