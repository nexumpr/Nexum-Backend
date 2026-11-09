"""Pruebas de RTO/RPO, vigencia de backups y recuperacion de state (I-130)."""

from __future__ import annotations

import json
import sys
import tempfile
import unittest
from datetime import date
from pathlib import Path

RAIZ = Path(__file__).resolve().parents[3]
sys.path.insert(0, str(RAIZ / "herramientas"))

import verificar_backup_restore as br  # noqa: E402
import verificar_recuperacion as rec  # noqa: E402

INVENTARIO = RAIZ / "infra" / "terraform" / "respaldo" / "inventario_backup.json"


def _inv() -> dict:
    return br.cargar_inventario(INVENTARIO)


def _json(datos: object) -> Path:
    manejador, ruta = tempfile.mkstemp(suffix=".json")
    with open(manejador, "w", encoding="utf-8") as fh:
        json.dump(datos, fh, ensure_ascii=False)
    return Path(ruta)


def _fases(duracion: int = 10) -> list[dict]:
    return [{"nombre": nombre, "duracion_min": duracion} for nombre in rec.FASES_RECUPERACION]


class MatrizRTOyRPO(unittest.TestCase):
    def test_seis_capas_presentes(self) -> None:
        matriz = rec.matriz_rto_rpo(_inv())
        for capa in (
            "objetivo_propuesto", "objetivo_ratificado", "objetivo_medido",
            "capacidad_teorica", "simulacion_local", "resultado_aws",
        ):
            self.assertIn(capa, matriz)

    def test_sin_ratificacion_queda_pendiente(self) -> None:
        matriz = rec.matriz_rto_rpo(_inv())
        self.assertEqual(matriz["estado"], rec.RTO_RPO_PENDING)
        self.assertFalse(matriz["objetivo_ratificado"]["aprobado"])

    def test_ratificacion_se_refleja(self) -> None:
        inventario = _inv()
        inventario["ratificacion_objetivos"] = {"aprobado": True, "fecha": "2026-11-09", "referencia": "RFC-X"}
        self.assertEqual(rec.matriz_rto_rpo(inventario)["estado"], "RATIFICADO")

    def test_simulacion_no_acredita_cumplimiento(self) -> None:
        matriz = rec.matriz_rto_rpo(_inv())
        self.assertFalse(matriz["simulacion_local"]["acredita_cumplimiento"])
        self.assertEqual(matriz["resultado_aws"]["estado"], "NOT_RUN")

    def test_objetivo_medido_solo_por_restore_real(self) -> None:
        objetivos = rec.matriz_rto_rpo(_inv())["objetivo_medido"]
        self.assertFalse(objetivos["medido_declarado"])
        self.assertFalse(objetivos["medido_verificable"])

    def test_medido_declarado_no_acredita(self) -> None:
        inventario = _inv()
        inventario["objetivos"]["medido"] = True
        objetivos = rec.matriz_rto_rpo(inventario)["objetivo_medido"]
        self.assertTrue(objetivos["medido_declarado"])
        self.assertFalse(objetivos["medido_verificable"])

    def test_ratificacion_sin_referencia_no_cuenta(self) -> None:
        inventario = _inv()
        inventario["ratificacion_objetivos"] = {"aprobado": True}
        self.assertEqual(rec.matriz_rto_rpo(inventario)["estado"], rec.RTO_RPO_PENDING)


class MedicionDeRecuperacion(unittest.TestCase):
    def test_trece_fases_dentro_de_rto(self) -> None:
        resultado = rec.medir_recuperacion(
            {"fases": _fases(10), "perdida_maxima_horas": "20"}, _inv()
        )
        self.assertEqual(resultado["estado"], "CUMPLE_SIMULADO")
        self.assertEqual(resultado["duracion_total_min"], "130.0000")

    def test_duracion_supera_rto(self) -> None:
        resultado = rec.medir_recuperacion(
            {"fases": _fases(30), "perdida_maxima_horas": "20"}, _inv()
        )
        self.assertEqual(resultado["estado"], "INCUMPLE_SIMULADO")
        self.assertFalse(resultado["cumple_rto_simulado"])

    def test_perdida_supera_rpo(self) -> None:
        resultado = rec.medir_recuperacion(
            {"fases": _fases(5), "perdida_maxima_horas": "30"}, _inv()
        )
        self.assertFalse(resultado["cumple_rpo_simulado"])
        self.assertEqual(resultado["estado"], "INCUMPLE_SIMULADO")

    def test_fase_faltante(self) -> None:
        fases = _fases(5)[:-1]
        resultado = rec.medir_recuperacion({"fases": fases, "perdida_maxima_horas": "1"}, _inv())
        self.assertIn("cierre", resultado["fases_faltantes"])
        self.assertEqual(resultado["estado"], "INCUMPLE_SIMULADO")

    def test_fase_fuera_de_orden(self) -> None:
        fases = _fases(5)
        fases[0], fases[1] = fases[1], fases[0]
        resultado = rec.medir_recuperacion({"fases": fases, "perdida_maxima_horas": "1"}, _inv())
        self.assertFalse(resultado["orden_correcto"])
        self.assertEqual(resultado["estado"], "INCUMPLE_SIMULADO")

    def test_fase_desconocida(self) -> None:
        fases = _fases(5) + [{"nombre": "fase-inventada", "duracion_min": 1}]
        resultado = rec.medir_recuperacion({"fases": fases, "perdida_maxima_horas": "1"}, _inv())
        self.assertIn("fase-inventada", resultado["fases_desconocidas"])

    def test_duracion_negativa_es_error(self) -> None:
        fases = _fases(5)
        fases[0]["duracion_min"] = -1
        with self.assertRaises(br.ErrorUso):
            rec.medir_recuperacion({"fases": fases, "perdida_maxima_horas": "1"}, _inv())

    def test_sin_solicitud_es_error(self) -> None:
        with self.assertRaises(br.ErrorUso):
            rec.medir_recuperacion(None, _inv())


class VigenciaDeBackup(unittest.TestCase):
    def _solicitud(self, **extra) -> dict:
        base = {
            "tipo": "PITR",
            "tabla_activa": "NEXUM_CORE_h08i",
            "backup": {
                "id": "backup-sint-1",
                "tabla": "NEXUM_CORE_h08i",
                "fecha": "2026-11-05",
                "retencion_dias": 30,
            },
        }
        base["backup"].update(extra.pop("backup", {}))
        base.update(extra)
        return base

    def test_backup_disponible(self) -> None:
        resultado = rec.evaluar_backup(self._solicitud(), date(2026, 11, 9))
        self.assertEqual(resultado["estado"], "DECLARADO_VIGENTE")
        self.assertEqual(resultado["alcance"], "BACKUP_AWS_NOT_VERIFIED")
        self.assertTrue(resultado["valido"])
        self.assertEqual(resultado["edad_dias"], 4)

    def test_backup_inexistente(self) -> None:
        self.assertEqual(rec.evaluar_backup({"tipo": "PITR", "tabla_activa": "X"}, None)["estado"], "NO_EXISTE")

    def test_backup_caducado(self) -> None:
        resultado = rec.evaluar_backup(self._solicitud(), date(2027, 1, 9))
        self.assertEqual(resultado["estado"], "FUERA_DE_RETENCION")
        self.assertTrue(resultado["caducidad_automatica"])
        self.assertFalse(resultado["valido"])

    def test_backup_de_otra_tabla(self) -> None:
        solicitud = self._solicitud(backup={"tabla": "OTRA_TABLA"})
        self.assertEqual(rec.evaluar_backup(solicitud, date(2026, 11, 9))["estado"], "TABLA_NO_COINCIDE")

    def test_retencion_invalida(self) -> None:
        solicitud = self._solicitud(backup={"retencion_dias": 90})
        self.assertEqual(rec.evaluar_backup(solicitud, date(2026, 11, 9))["estado"], "RETENCION_INVALIDA")

    def test_fecha_invalida_es_error(self) -> None:
        solicitud = self._solicitud(backup={"fecha": "05/11/2026"})
        with self.assertRaises(br.ErrorUso):
            rec.evaluar_backup(solicitud, date(2026, 11, 9))

    def test_sin_solicitud_es_error(self) -> None:
        with self.assertRaises(br.ErrorUso):
            rec.evaluar_backup(None)

    def test_tipo_desconocido_se_rechaza(self) -> None:
        solicitud = self._solicitud()
        solicitud["tipo"] = "snapshot-magico"
        with self.assertRaises(br.ErrorUso):
            rec.evaluar_backup(solicitud, date(2026, 11, 9))

    def test_fecha_futura_se_rechaza(self) -> None:
        solicitud = self._solicitud(backup={"fecha": "2026-12-01"})
        self.assertEqual(
            rec.evaluar_backup(solicitud, date(2026, 11, 9))["estado"], "FECHA_FUTURA"
        )

    def test_pitr_no_admite_mas_de_35_dias(self) -> None:
        solicitud = self._solicitud(backup={"retencion_dias": 40})
        self.assertEqual(
            rec.evaluar_backup(solicitud, date(2026, 11, 9))["estado"], "RETENCION_INVALIDA"
        )

    def test_on_demand_admite_retencion_mayor_y_no_caduca_solo(self) -> None:
        solicitud = self._solicitud(tipo="on-demand", backup={"retencion_dias": 90})
        resultado = rec.evaluar_backup(solicitud, date(2026, 11, 9))
        self.assertEqual(resultado["estado"], "DECLARADO_VIGENTE")
        self.assertFalse(resultado["caducidad_automatica"])

    def test_on_demand_fuera_de_retencion_no_caduca_solo(self) -> None:
        solicitud = self._solicitud(tipo="on-demand", backup={"fecha": "2026-01-01", "retencion_dias": 30})
        resultado = rec.evaluar_backup(solicitud, date(2026, 11, 9))
        self.assertEqual(resultado["estado"], "FUERA_DE_RETENCION")
        self.assertFalse(resultado["caducidad_automatica"])
        self.assertIn("no caduca solo", resultado["motivo"])

    def test_punto_fuera_de_ventana(self) -> None:
        solicitud = self._solicitud(punto_restauracion="2026-01-01T00:00:00Z")
        self.assertEqual(
            rec.evaluar_backup(solicitud, date(2026, 11, 9))["estado"], "PUNTO_FUERA_DE_VENTANA"
        )

    def test_punto_invalido_es_error(self) -> None:
        solicitud = self._solicitud(punto_restauracion="01/01/2026")
        with self.assertRaises(br.ErrorUso):
            rec.evaluar_backup(solicitud, date(2026, 11, 9))

    def test_evidencia_aws_verificada(self) -> None:
        solicitud = self._solicitud(
            evidencia_aws={"verificado_por_aws": True, "huella_sha256": "a" * 64}
        )
        self.assertEqual(
            rec.evaluar_backup(solicitud, date(2026, 11, 9))["alcance"], "BACKUP_AWS_VERIFIED"
        )

    def test_sin_evidencia_aws_no_se_declara_verificado(self) -> None:
        self.assertEqual(
            rec.evaluar_backup(self._solicitud(), date(2026, 11, 9))["alcance"],
            "BACKUP_AWS_NOT_VERIFIED",
        )


class RecuperacionDeState(unittest.TestCase):
    def _solicitud(self, **extra) -> dict:
        base = {
            "version_id": "v-2026-11-01",
            "sha256_esperado": "a" * 64,
            "sha256_observado": "a" * 64,
            "aprobacion": "APRUEBO_STATE_RECOVERY",
            "copia_state_actual": "s3://state/terraform.tfstate.prev",
            "token_escritura": "lock-1",
            "registra_secretos": False,
            "gate": "GATE_DESPLIEGUE",
        }
        base.update(extra)
        return base

    def test_plan_valido_es_sintetico_y_no_autoriza(self) -> None:
        resultado = rec.plan_recuperacion_state(self._solicitud())
        self.assertEqual(resultado["estado"], "PLAN_SINTETICO_EVALUADO")
        self.assertEqual(resultado["faltantes"], [])
        self.assertEqual(resultado["autorizacion_real"], "DENEGADA_EN_I130_LOCAL")
        self.assertEqual(resultado["alcance_aws"], rec.STATE_AWS_NOT_VERIFIED)
        self.assertFalse(any(resultado["precondiciones_verificadas"].values()))

    def test_sin_version_se_rechaza(self) -> None:
        resultado = rec.plan_recuperacion_state(self._solicitud(version_id=None))
        self.assertEqual(resultado["estado"], "RECUPERACION_RECHAZADA")
        self.assertIn("version_identificada", resultado["faltantes"])

    def test_integridad_rota(self) -> None:
        resultado = rec.plan_recuperacion_state(self._solicitud(sha256_observado="b" * 64))
        self.assertIn("integridad_declarada", resultado["faltantes"])

    def test_sin_aprobacion(self) -> None:
        resultado = rec.plan_recuperacion_state(self._solicitud(aprobacion=None))
        self.assertIn("aprobacion_declarada", resultado["faltantes"])

    def test_sin_copia_previa_no_hay_reversion(self) -> None:
        resultado = rec.plan_recuperacion_state(self._solicitud(copia_state_actual=None))
        self.assertIn("reversion_declarada", resultado["faltantes"])

    def test_secretos_en_logs_se_rechaza(self) -> None:
        resultado = rec.plan_recuperacion_state(self._solicitud(registra_secretos=True))
        self.assertIn("sin_secretos_en_logs", resultado["faltantes"])

    def test_sin_gate_se_rechaza(self) -> None:
        resultado = rec.plan_recuperacion_state(self._solicitud(gate=None))
        self.assertIn("gate_referenciado", resultado["faltantes"])

    def test_sin_solicitud_es_error(self) -> None:
        with self.assertRaises(br.ErrorUso):
            rec.plan_recuperacion_state(None)


class ConcurrenciaYCLI(unittest.TestCase):
    def test_escritor_unico(self) -> None:
        resultado = rec.verificar_concurrencia({"token_activo": "lock-1", "solicitante": "lock-1"})
        self.assertTrue(resultado["permite"])

    def test_escritor_concurrente(self) -> None:
        resultado = rec.verificar_concurrencia({"token_activo": "lock-2", "solicitante": "lock-1"})
        self.assertFalse(resultado["permite"])
        self.assertEqual(resultado["estado"], "ESCRITOR_CONCURRENTE")

    def test_cli_matriz_sale_cero(self) -> None:
        self.assertEqual(rec.main(["matriz-rto-rpo"]), 0)

    def test_cli_backup_caducado_sale_uno(self) -> None:
        solicitud = _json({
            "tipo": "PITR",
            "tabla_activa": "NEXUM_CORE_h08i",
            "backup": {"id": "b1", "tabla": "NEXUM_CORE_h08i", "fecha": "2020-01-01", "retencion_dias": 7},
        })
        self.assertEqual(rec.main(["estado-backup", "--solicitud", str(solicitud)]), 1)

    def test_cli_state_rechazado_sale_uno(self) -> None:
        self.assertEqual(
            rec.main(["plan-state", "--solicitud", str(_json({"version_id": "v1"}))]), 1
        )

    def test_cli_comando_desconocido(self) -> None:
        self.assertEqual(rec.main(["destroy-all"]), 2)

    def test_no_importa_aws_ni_procesos(self) -> None:
        fuente = (RAIZ / "herramientas" / "verificar_recuperacion.py").read_text(encoding="utf-8")
        for prohibido in (
            "import boto3", "import botocore", "from botocore",
            "import subprocess", "from subprocess", "os.system",
        ):
            self.assertNotIn(prohibido, fuente)


if __name__ == "__main__":  # pragma: no cover
    unittest.main()
