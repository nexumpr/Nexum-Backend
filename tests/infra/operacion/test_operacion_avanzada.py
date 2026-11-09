"""Pruebas adversas avanzadas de I-120: alcance del plan, idempotencia y DRY_RUN."""

from __future__ import annotations

import concurrent.futures
import json
import sys
import tempfile
import unittest
from decimal import Decimal
from pathlib import Path

RAIZ = Path(__file__).resolve().parents[3]
sys.path.insert(0, str(RAIZ / "herramientas"))

import nexum_operacion as op  # noqa: E402

INVENTARIO = RAIZ / "infra" / "terraform" / "operacion" / "inventario.json"
CATALOGO = RAIZ / "infra" / "finops" / "precios_us-east-2.json"
AUX = ["interface-endpoint-secretsmanager", "interface-endpoint-ssm", "interface-endpoint-ssmmessages"]
FRASE_SUSP = "APRUEBO_SUSPENDER_AUX_H08I"


def _inv() -> dict:
    return op.cargar_inventario(INVENTARIO)


def _cat() -> dict:
    return op.cargar_catalogo(CATALOGO)


def _plan_aux() -> Path:
    return _json(
        [
            {"direccion": "interface-endpoint-secretsmanager", "accion": "update"},
            {"direccion": "interface-endpoint-ssm", "accion": "update"},
            {"direccion": "interface-endpoint-ssmmessages", "accion": "update"},
        ]
    )


def _json(datos: object) -> Path:
    manejador, ruta = tempfile.mkstemp(suffix=".json")
    with open(manejador, "w", encoding="utf-8") as fh:
        json.dump(datos, fh, ensure_ascii=False)
    return Path(ruta)


class RegistroDeTransiciones(unittest.TestCase):
    def test_registro_completo_para_inf_120_01(self) -> None:
        registro = op.transicion(
            "OFF", "STARTING", "responsable", "RUNBOOK_START_STOP.md",
            "2026-11-09T00:00:00+00:00",
            entorno="h08i", recurso="runtime", motivo="ventana",
            operacion_id="op-1", autorizacion="APRUEBO_ENCENDER_H08I",
            evidencia="evidencia.json",
        )
        for campo in (
            "estado_anterior", "estado_nuevo", "actor", "timestamp_utc", "entorno",
            "recurso", "motivo", "operacion_id", "autorizacion", "resultado",
            "evidencia", "rollback",
        ):
            self.assertIn(campo, registro, campo)
        self.assertEqual(registro["resultado"], "PLANIFICADO")
        self.assertEqual(registro["estado_anterior"], "OFF")

    def test_off_no_es_estado_del_core(self) -> None:
        self.assertTrue(op.es_estado_auxiliar("OFF"))
        self.assertFalse(op.es_estado_auxiliar("ON_2_HEALTHY"))

    def test_estado_observado_off_marca_incidente(self) -> None:
        _, resultado = op.ejecutar(
            ["estado", "--inventario", str(INVENTARIO), "--catalogo", str(CATALOGO),
             "--observado", str(_json({"estado": "OFF"}))]
        )
        self.assertTrue(resultado["off_sobre_core"])


class AlcanceDelPlan(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.clasif = op.clasificar(_inv(), AUX)

    def test_cambio_sobre_core_se_deniega(self) -> None:
        with self.assertRaises(op.ErrorUso):
            op.exigir_alcance_plan(
                [{"direccion": "aws_ec2_instance.api", "accion": "delete"}],
                self.clasif["core"], self.clasif["suspendibles"],
            )

    def test_recurso_no_declarado_se_deniega(self) -> None:
        with self.assertRaises(op.ErrorUso):
            op.exigir_alcance_plan(
                [{"direccion": "aws_s3_bucket.otro", "accion": "delete"}],
                self.clasif["core"], self.clasif["suspendibles"],
            )

    def test_delete_sobre_auxiliar_se_deniega(self) -> None:
        with self.assertRaises(op.ErrorUso):
            op.exigir_alcance_plan(
                [{"direccion": "interface-endpoint-ssm", "accion": "delete"}],
                self.clasif["core"], self.clasif["suspendibles"],
            )

    def test_capacidad_por_debajo_de_dos_se_deniega(self) -> None:
        with self.assertRaises(op.ErrorUso) as ctx:
            op.exigir_alcance_plan(
                [{"direccion": "interface-endpoint-ssm", "accion": "update", "desired": 1}],
                self.clasif["core"], self.clasif["suspendibles"],
            )
        self.assertIn("doble instancia", str(ctx.exception))

    def test_plan_limpio_se_acepta(self) -> None:
        resultado = op.exigir_alcance_plan(
            [{"direccion": "interface-endpoint-ssm", "accion": "update"}],
            self.clasif["core"], self.clasif["suspendibles"],
        )
        self.assertTrue(resultado["ok"])

    def test_plan_malformado_falla_cerrado(self) -> None:
        with self.assertRaises(op.ErrorUso):
            op.exigir_alcance_plan(
                [{"direccion": "interface-endpoint-ssm"}],
                self.clasif["core"], self.clasif["suspendibles"],
            )

    def test_sin_plan_no_se_asume_alcance(self) -> None:
        resultado = op.verificar_alcance_plan(None, self.clasif["core"], self.clasif["suspendibles"])
        self.assertFalse(resultado["verificado"])


class EjecucionDeshabilitada(unittest.TestCase):
    def test_apply_devuelve_codigo_2(self) -> None:
        codigo, resultado = op.ejecutar(
            ["apply", "--inventario", str(INVENTARIO), "--catalogo", str(CATALOGO),
             "--allowlist", ",".join(AUX), "--plan-cambios", str(_plan_aux())]
        )
        self.assertEqual(codigo, 2)
        self.assertEqual(resultado["estado"], "EJECUCION_DESHABILITADA")
        self.assertTrue(resultado["dry_run"])
        self.assertFalse(resultado["ejecutado_en_aws"])

    def test_apply_no_se_habilita_con_force(self) -> None:
        with self.assertRaises(op.ErrorUso):
            op.ejecutar(["apply", "--force", "--inventario", str(INVENTARIO)])

    def test_flags_de_fuerza_rechazados(self) -> None:
        for flag in ("--force", "--yes"):
            with self.subTest(flag=flag):
                with self.assertRaises(op.ErrorUso):
                    op.ejecutar(["estado", flag, "--inventario", str(INVENTARIO)])

    def test_alias_ingles_disponibles(self) -> None:
        for comando in ("status", "reconcile"):
            _, resultado = op.ejecutar(
                [comando, "--inventario", str(INVENTARIO), "--catalogo", str(CATALOGO)]
            )
            self.assertIn(resultado["comando"], ("estado", "reconciliar"))

    def test_suspend_aux_alias(self) -> None:
        _, resultado = op.ejecutar(
            ["suspend-aux", "--inventario", str(INVENTARIO), "--catalogo", str(CATALOGO),
             "--aprobacion", FRASE_SUSP, "--actor", "op", "--runbook", "runbook",
             "--allowlist", ",".join(AUX), "--plan-cambios", str(_plan_aux())]
        )
        self.assertEqual(resultado["comando"], "suspender-aux")


class IdempotenciaYReintentos(unittest.TestCase):
    def test_suspender_dos_veces_es_idempotente(self) -> None:
        observado = _json({"recursos_suspendidos": AUX, "estado": "ON_2_HEALTHY"})
        codigo, resultado = op.ejecutar(
            ["suspender-aux", "--inventario", str(INVENTARIO), "--catalogo", str(CATALOGO),
             "--aprobacion", FRASE_SUSP, "--actor", "op", "--runbook", "runbook",
             "--allowlist", ",".join(AUX), "--plan-cambios", str(_json([])),
             "--observado", str(observado)]
        )
        self.assertEqual(codigo, 0)
        self.assertTrue(resultado["idempotente"])
        self.assertEqual(sorted(resultado["ya_suspendidos"]), sorted(AUX))

    def test_reintento_recupera_fallo_transitorio(self) -> None:
        intentos = {"n": 0}

        def accion() -> str:
            intentos["n"] += 1
            if intentos["n"] < 3:
                raise op.ErrorOperacion("timeout simulado")
            return "ok"

        valor, errores = op.ejecutar_con_reintentos(accion, intentos=3)
        self.assertEqual(valor, "ok")
        self.assertEqual(len(errores), 2)

    def test_reintento_agotado_falla(self) -> None:
        def accion() -> str:
            raise op.ErrorOperacion("fallo persistente")

        with self.assertRaises(op.ErrorOperacion):
            op.ejecutar_con_reintentos(accion, intentos=2)

    def test_fallo_parcial_reporta_rollback(self) -> None:
        resultado = op.simular_aplicacion(AUX, fallos=["interface-endpoint-ssm"])
        self.assertEqual(resultado["resultado"], "PARCIAL")
        self.assertEqual(resultado["fallidos"], ["interface-endpoint-ssm"])
        self.assertIn("rollback", resultado["rollback"])
        self.assertFalse(resultado["datos_destruidos"])

    def test_concurrencia_estable(self) -> None:
        clasif = op.clasificar(_inv(), AUX)
        with concurrent.futures.ThreadPoolExecutor(max_workers=25) as pool:
            resultados = list(pool.map(lambda _: op.clasificar(_inv(), AUX), range(25)))
        for resultado in resultados:
            self.assertEqual(resultado, clasif)


class ImpactoYAuxiliares(unittest.TestCase):
    def test_impacto_bloqueante_rechaza_suspension(self) -> None:
        inventario = _inv()
        for item in inventario["items"]:
            if item["nombre"] == "interface-endpoint-ssm":
                item["impacto_core"] = "BLOQUEANTE"
        with self.assertRaises(op.ErrorUso) as ctx:
            op.plan_suspender_aux(
                inventario, _cat(), AUX, FRASE_SUSP, "h08i", None, "op", "runbook", None
            )
        self.assertIn("BLOQUEANTE", str(ctx.exception))

    def test_ventana_declarada_permite_suspension(self) -> None:
        _, resultado = op.ejecutar(
            ["suspender-aux", "--inventario", str(INVENTARIO), "--catalogo", str(CATALOGO),
             "--aprobacion", FRASE_SUSP, "--actor", "op", "--runbook", "runbook",
             "--allowlist", ",".join(AUX), "--plan-cambios", str(_plan_aux())]
        )
        self.assertFalse(set(resultado["objetivo"]) & set(resultado["no_tocado"]))

    def test_destroy_aux_con_plan_sobre_core_se_deniega(self) -> None:
        plan = _json([{"direccion": "redis-oss", "accion": "delete"}])
        with self.assertRaises(op.ErrorUso):
            op.ejecutar(
                ["destroy-aux", "--inventario", str(INVENTARIO), "--catalogo", str(CATALOGO),
                 "--aprobacion", "APRUEBO_DESTRUIR_AUX_H08I", "--actor", "op",
                 "--runbook", "runbook", "--plan-cambios", str(plan)]
            )

    def test_autorizacion_requerida_para_procedencia(self) -> None:
        with self.assertRaises(op.ErrorUso):
            op.cargar_autorizacion(None, FRASE_SUSP, "h08i", "suspender-aux")
        with self.assertRaises(op.ErrorUso):
            op.cargar_autorizacion(
                _json({"frase": "OTRA", "entorno": "h08i", "operacion": "suspender-aux", "aprobador": "x"}),
                FRASE_SUSP, "h08i", "suspender-aux",
            )

    def test_autorizacion_valida_no_habilita_ejecucion(self) -> None:
        ruta = _json({
            "frase": FRASE_SUSP, "entorno": "h08i", "operacion": "suspender-aux",
            "aprobador": "responsable", "fecha": "2026-11-09", "referencia": "PR #32",
        })
        registro = op.cargar_autorizacion(ruta, FRASE_SUSP, "h08i", "suspender-aux")
        self.assertEqual(registro["habilitacion_ejecucion"], "DENEGADA_EN_I120_LOCAL")


class Preservacion(unittest.TestCase):
    def test_rollback_preserva_datos_y_state(self) -> None:
        _, resultado = op.ejecutar(
            ["rollback", "--inventario", str(INVENTARIO), "--catalogo", str(CATALOGO),
             "--snapshot", str(_json({"estado": "ON_2_HEALTHY", "recursos_deseados": ["redis-oss"]})),
             "--actor", "op", "--runbook", "runbook"]
        )
        self.assertFalse(resultado["datos_destruidos"])
        self.assertFalse(resultado["estado_destruido"])
        self.assertIn("terraform-state-s3", resultado["core_protegido"])

    def test_residual_sin_creditos_no_declara_neto(self) -> None:
        resultado = op.residual(_inv(), _cat())
        self.assertIsNone(resultado["neto_con_creditos_usd"])
        self.assertIsNone(resultado["creditos_aplicables_usd"])
        self.assertGreater(Decimal(resultado["total_45d_usd"]), Decimal("0"))

    def test_reanudar_bloquea_sin_pronostico(self) -> None:
        codigo, resultado = op.ejecutar(
            ["reanudar-aux", "--inventario", str(INVENTARIO), "--catalogo", str(CATALOGO),
             "--aprobacion", "APRUEBO_REANUDAR_AUX_H08I", "--creditos", "90",
             "--allowlist", ",".join(AUX), "--plan-cambios", str(_plan_aux()),
             "--actor", "op", "--runbook", "runbook"]
        )
        self.assertEqual(codigo, 1)
        self.assertEqual(resultado["pronostico"]["estado"], "BLOQUEADO")

    def test_reanudar_ok_con_pronostico_cubierto(self) -> None:
        codigo, resultado = op.ejecutar(
            ["reanudar-aux", "--inventario", str(INVENTARIO), "--catalogo", str(CATALOGO),
             "--aprobacion", "APRUEBO_REANUDAR_AUX_H08I", "--creditos", "90",
             "--allowlist", ",".join(AUX), "--plan-cambios", str(_plan_aux()),
             "--pronostico", "40", "--actor", "op", "--runbook", "runbook"]
        )
        self.assertEqual(codigo, 0)
        self.assertEqual(resultado["estado"], "PLANIFICADO")


class AllowlistExplicita(unittest.TestCase):
    """Sin allowlist revisada no se autoriza ningun auxiliar (fail-closed)."""

    @classmethod
    def setUpClass(cls) -> None:
        cls.inv = _inv()

    def test_allowlist_none_no_autoriza_nada(self) -> None:
        clasif = op.clasificar(self.inv, None)
        self.assertEqual(clasif["suspendibles"], [])
        self.assertTrue(set(AUX) <= set(clasif["denegados"]))

    def test_allowlist_vacia_no_autoriza_nada(self) -> None:
        clasif = op.clasificar(self.inv, [])
        self.assertEqual(clasif["suspendibles"], [])
        self.assertEqual(len(clasif["denegados"]), len(AUX))

    def test_auxiliar_etiquetado_sin_allowlist_no_se_autoriza(self) -> None:
        clasif = op.clasificar(self.inv, [])
        for nombre in AUX:
            self.assertIn(nombre, clasif["denegados"])
            self.assertNotIn(nombre, clasif["suspendibles"])
        # El inventario sigue siendo visible, pero sin autorizacion de cambio.
        self.assertTrue(set(AUX) <= set(clasif["auxiliares"]))

    def test_allowlist_de_un_solo_auxiliar(self) -> None:
        clasif = op.clasificar(self.inv, ["interface-endpoint-ssm"])
        self.assertEqual(clasif["suspendibles"], ["interface-endpoint-ssm"])

    def test_allowlist_multiple(self) -> None:
        clasif = op.clasificar(self.inv, AUX)
        self.assertEqual(clasif["suspendibles"], sorted(AUX))

    def test_core_en_allowlist_se_rechaza(self) -> None:
        with self.assertRaises(op.ErrorUso) as ctx:
            op.validar_allowlist(self.inv, ["redis-oss"])
        self.assertIn("core", str(ctx.exception))

    def test_allowlist_desconocida_se_rechaza(self) -> None:
        with self.assertRaises(op.ErrorUso):
            op.validar_allowlist(self.inv, ["recurso-inexistente"])

    def test_auxiliar_desconocido_no_se_autoriza(self) -> None:
        clasif = op.clasificar(self.inv, AUX + ["aux-inventado"])
        self.assertNotIn("aux-inventado", clasif["suspendibles"])
        self.assertIn("aux-inventado", clasif["denegados"])

    def test_cli_sin_allowlist_deniega(self) -> None:
        with self.assertRaises(op.ErrorUso):
            op.ejecutar(
                ["suspender-aux", "--inventario", str(INVENTARIO), "--catalogo", str(CATALOGO),
                 "--aprobacion", FRASE_SUSP, "--actor", "op", "--runbook", "runbook",
                 "--plan-cambios", str(_plan_aux())]
            )

    def test_cli_con_allowlist_vacia_deniega(self) -> None:
        with self.assertRaises(op.ErrorUso):
            op.ejecutar(
                ["suspender-aux", "--inventario", str(INVENTARIO), "--catalogo", str(CATALOGO),
                 "--aprobacion", FRASE_SUSP, "--actor", "op", "--runbook", "runbook",
                 "--allowlist", "", "--plan-cambios", str(_plan_aux())]
            )

    def test_lectura_con_allowlist_vacia_funciona(self) -> None:
        codigo, resultado = op.ejecutar(
            ["estado", "--inventario", str(INVENTARIO), "--catalogo", str(CATALOGO)]
        )
        self.assertEqual(codigo, 0)
        self.assertEqual(resultado["clasificacion"]["suspendibles"], [])


class PlanObligatorio(unittest.TestCase):
    """Sin plan evaluado no se autoriza ninguna modificacion."""

    @classmethod
    def setUpClass(cls) -> None:
        cls.clasif = op.clasificar(_inv(), AUX)

    def test_plan_none_se_rechaza(self) -> None:
        with self.assertRaises(op.ErrorUso):
            op.exigir_alcance_plan(None, self.clasif["core"], self.clasif["suspendibles"])

    def test_plan_vacio_con_modificacion_se_rechaza(self) -> None:
        with self.assertRaises(op.ErrorUso) as ctx:
            op.exigir_alcance_plan([], self.clasif["core"], self.clasif["suspendibles"])
        self.assertIn("plan vacio", str(ctx.exception))

    def test_plan_vacio_justificado_si_no_hay_cambios(self) -> None:
        resultado = op.exigir_alcance_plan(
            [], self.clasif["core"], self.clasif["suspendibles"], requiere_cambios=False
        )
        self.assertTrue(resultado["ok"])

    def test_plan_valido_marca_local_y_aws_no_verificado(self) -> None:
        resultado = op.exigir_alcance_plan(
            [{"direccion": "interface-endpoint-ssm", "accion": "update"}],
            self.clasif["core"], self.clasif["suspendibles"],
        )
        self.assertEqual(resultado["estado"], "PLAN_LOCAL_VALIDADO")
        self.assertEqual(resultado["alcance_aws"], "PLAN_AWS_NO_VERIFICADO")
        self.assertEqual(resultado["plan_terraform_real"], "TERRAFORM_REAL_PLAN_NOT_VERIFIED")
        self.assertTrue(op.TERRAFORM_REAL_PLAN_NOT_VERIFIED)

    def test_plan_con_core_se_rechaza(self) -> None:
        with self.assertRaises(op.ErrorUso):
            op.exigir_alcance_plan(
                [{"direccion": "alb-interno", "accion": "update"}],
                self.clasif["core"], self.clasif["suspendibles"],
            )

    def test_plan_con_recurso_fuera_de_allowlist_se_rechaza(self) -> None:
        clasif = op.clasificar(_inv(), ["interface-endpoint-ssm"])
        with self.assertRaises(op.ErrorUso):
            op.exigir_alcance_plan(
                [{"direccion": "interface-endpoint-secretsmanager", "accion": "update"}],
                clasif["core"], clasif["suspendibles"],
            )

    def test_destruccion_indirecta_se_rechaza(self) -> None:
        for accion in ("delete", "destroy", "forget"):
            with self.subTest(accion=accion):
                with self.assertRaises(op.ErrorUso):
                    op.exigir_alcance_plan(
                        [{"direccion": "interface-endpoint-ssm", "accion": accion}],
                        self.clasif["core"], self.clasif["suspendibles"],
                    )

    def test_reduccion_de_asg_a_uno_se_rechaza(self) -> None:
        with self.assertRaises(op.ErrorUso):
            op.exigir_alcance_plan(
                [{"direccion": "interface-endpoint-ssm", "accion": "update", "desired_capacity": 1}],
                self.clasif["core"], self.clasif["suspendibles"],
            )

    def test_estado_aws_no_verificado_siempre(self) -> None:
        resultado = op.verificar_alcance_plan(
            [{"direccion": "interface-endpoint-ssm", "accion": "update"}],
            self.clasif["core"], self.clasif["suspendibles"],
        )
        self.assertEqual(resultado["alcance_aws"], "PLAN_AWS_NO_VERIFICADO")
        self.assertNotIn("aws_verificado", resultado)

    def test_lectura_sin_plan_no_exige_alcance(self) -> None:
        codigo, resultado = op.ejecutar(
            ["reconciliar", "--inventario", str(INVENTARIO), "--catalogo", str(CATALOGO)]
        )
        self.assertEqual(codigo, 1)
        self.assertEqual(resultado["estado"], "BLOQUEADO")


if __name__ == "__main__":  # pragma: no cover
    unittest.main()
